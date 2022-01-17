package main

import (
	"context"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/connector-postgresql/pkg/cmd/importer"
	"github.com/authzed/connector-postgresql/pkg/streams"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/open-policy-agent/opa/ast"
	opacmd "github.com/open-policy-agent/opa/cmd"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/storage"
)

func registerExperimentCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(experimentCmd)

	experimentCmd.AddCommand(experimentImportCmd)

	ctx := context.Background()
	stdio := streams.NewStdIO()
	experimentImportCmd.AddCommand(NewImportPostgresCmd(ctx, stdio))

	experimentCmd.AddCommand(opacmd.RootCommand)
	opacmd.RootCommand.Use = "opa"
	opacmd.RootCommand.PersistentPreRunE = cobrautil.CommandStack(
		SyncFlagsCmdFunc,
		opaPreRunCmdFunc,
	)
}

var experimentCmd = &cobra.Command{
	Use:   "experiment <subcommand>",
	Short: "experimental functionality",
}

var experimentImportCmd = &cobra.Command{
	Use:   "import <subcommand>",
	Short: "import relationships and schemas from external data sources",
}

// NewImportPostgresCmd configures a new cobra command that imports data from postgres
func NewImportPostgresCmd(ctx context.Context, streams streams.IO) *cobra.Command {
	o := importer.NewOptions(streams)
	cmd := &cobra.Command{
		Use:   "postgres",
		Short: "import data from Postgres into SpiceDB",
		Example: `
  zed experiment import postgres "postgres://postgres:password@localhost:5432/postgres?sslmode=disable"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configStore, secretStore := defaultStorage()
			token, err := storage.DefaultToken(
				cobrautil.MustGetString(cmd, "endpoint"),
				cobrautil.MustGetString(cmd, "token"),
				configStore,
				secretStore,
			)
			if err != nil {
				return err
			}
			client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.APIToken)...)
			if err != nil {
				return err
			}
			o.Client = client
			if err := o.Complete(ctx, args); err != nil {
				return err
			}
			return o.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&o.PostgresURI, "postgres", "", "address for the postgres endpoint. example: \"postgres://postgres:password@localhost:5432/postgres?sslmode=disable\"")
	cmd.Flags().BoolVar(&o.DryRun, "dry-run", true, "log tuples that would be written without calling spicedb")
	cmd.Flags().StringVar(&o.MappingFile, "config", "", "path to a file containing the config that maps between pg tables and spicedb relationships")
	cmd.Flags().BoolVar(&o.AppendSchema, "append-schema", true, "append the config's (zed) schema to the schema in spicedb")

	return cmd
}

func opaPreRunCmdFunc(cmd *cobra.Command, args []string) error {
	configStore, secretStore := defaultStorage()
	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
		configStore,
		secretStore,
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.APIToken)...)
	if err != nil {
		return err
	}

	registerAuthzedBuiltins(client)

	return nil
}

func registerAuthzedBuiltins(client *authzed.Client) {
	rego.RegisterBuiltin4(
		&rego.Function{
			Name:    "authzed.check",
			Decl:    types.NewFunction(types.Args(types.S, types.S, types.S, types.S), types.B),
			Memoize: false,
		},
		func(bctx rego.BuiltinContext, subjectTerm, relationTerm, objectTerm, zedtokenTerm *ast.Term) (*ast.Term, error) {
			var subjectStr, relation, objectStr, zedtoken string
			if err := ast.As(subjectTerm.Value, &objectStr); err != nil {
				return nil, err
			}
			if err := ast.As(relationTerm.Value, &relation); err != nil {
				return nil, err
			}
			if err := ast.As(objectTerm.Value, &subjectStr); err != nil {
				return nil, err
			}
			if err := ast.As(zedtokenTerm.Value, &zedtoken); err != nil {
				return nil, err
			}

			var objectNS, objectID string
			err := stringz.SplitExact(objectStr, ":", &objectNS, &objectID)
			if err != nil {
				return nil, err
			}

			subjectNS, subjectID, subjectRel, err := parseSubject(subjectStr)
			if err != nil {
				return nil, err
			}

			request := &v1.CheckPermissionRequest{
				Resource: &v1.ObjectReference{
					ObjectType: objectNS,
					ObjectId:   objectID,
				},
				Permission: relation,
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: subjectNS,
						ObjectId:   subjectID,
					},
					OptionalRelation: subjectRel,
				},
			}

			if zedtoken != "" {
				request.Consistency = atLeastAsFresh(zedtoken)
			}

			resp, err := client.CheckPermission(context.Background(), request)
			if err != nil {
				return nil, err
			}

			isMember := resp.Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION
			value, err := ast.InterfaceToValue(isMember)
			if err != nil {
				return nil, err
			}

			return ast.NewTerm(value), nil
		},
	)
}
