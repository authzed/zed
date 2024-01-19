package commands

import (
	"context"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
)

func RegisterSchemaCmd(rootCmd *cobra.Command) *cobra.Command {
	rootCmd.AddCommand(schemaCmd)

	schemaCmd.AddCommand(schemaReadCmd)
	schemaReadCmd.Flags().Bool("json", false, "output as JSON")

	return schemaCmd
}

var (
	schemaCmd = &cobra.Command{
		Use:   "schema <subcommand>",
		Short: "manages Schema for a Permissions System",
	}

	schemaReadCmd = &cobra.Command{
		Use:               "read",
		Short:             "read the Schema of current Permissions System",
		Args:              cobra.ExactArgs(0),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE:              schemaReadCmdFunc,
	}
)

func schemaReadCmdFunc(cmd *cobra.Command, _ []string) error {
	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}
	request := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", request).Msg("requesting schema read")

	resp, err := client.ReadSchema(cmd.Context(), request)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := PrettyProto(resp)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
		return nil
	}

	console.Println(stringz.Join("\n\n", resp.SchemaText))
	return nil
}

// ReadSchema calls read schema for the client and returns the schema found.
func ReadSchema(ctx context.Context, client client.Client) (string, error) {
	request := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", request).Msg("requesting schema read")

	resp, err := client.ReadSchema(ctx, request)
	if err != nil {
		errStatus, ok := status.FromError(err)
		if !ok || errStatus.Code() != codes.NotFound {
			return "", err
		}

		log.Debug().Msg("no schema defined")
		return "", nil
	}

	return resp.SchemaText, nil
}
