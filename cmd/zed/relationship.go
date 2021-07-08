package main

import (
	"context"
	"fmt"
	"os"

	v0 "github.com/authzed/authzed-go/proto/authzed/api/v0"
	"github.com/authzed/authzed-go/v0"
	"github.com/authzed/zed/internal/storage"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func registerRelationshipCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(relationshipCmd)

	relationshipCmd.AddCommand(createCmd)
	createCmd.Flags().Bool("json", false, "output as JSON")

	relationshipCmd.AddCommand(touchCmd)
	touchCmd.Flags().Bool("json", false, "output as JSON")

	relationshipCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().Bool("json", false, "output as JSON")
}

var relationshipCmd = &cobra.Command{
	Use:               "relationship <subcommand>",
	Short:             "perform CRUD operations on the Relationships in a Permissions System",
	PersistentPreRunE: persistentPreRunE,
}

var createCmd = &cobra.Command{
	Use:               "create <subject:id> <relation> <object:id>",
	Short:             "create a Relationship for a Subject",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: persistentPreRunE,
	RunE:              writeRelationshipCmdFunc(v0.RelationTupleUpdate_CREATE),
}

var touchCmd = &cobra.Command{
	Use:               "touch <subject:id> <relation> <object:id>",
	Short:             "idempotently update a Relationship for a Subject",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: persistentPreRunE,
	RunE:              writeRelationshipCmdFunc(v0.RelationTupleUpdate_TOUCH),
}

var deleteCmd = &cobra.Command{
	Use:               "delete <subject:id> <relation> <object:id>",
	Short:             "delete a Relationship",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: persistentPreRunE,
	RunE:              writeRelationshipCmdFunc(v0.RelationTupleUpdate_DELETE),
}

func writeRelationshipCmdFunc(operation v0.RelationTupleUpdate_Operation) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		subjectNS, subjectID, subjectRel, err := parseSubject(args[0])
		if err != nil {
			return err
		}

		relation := args[1]

		var objectNS, objectID string
		err = stringz.SplitExact(args[2], ":", &objectNS, &objectID)
		if err != nil {
			return err
		}

		token, err := storage.DefaultToken(
			cobrautil.MustGetString(cmd, "permissions-system"),
			cobrautil.MustGetString(cmd, "endpoint"),
			cobrautil.MustGetString(cmd, "token"),
		)
		log.Trace().Interface("token", token).Send()

		client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.Secret)...)
		if err != nil {
			return err
		}

		request := &v0.WriteRequest{Updates: []*v0.RelationTupleUpdate{{
			Operation: operation,
			Tuple: &v0.RelationTuple{
				ObjectAndRelation: &v0.ObjectAndRelation{
					Namespace: stringz.Join("/", token.System, objectNS),
					ObjectId:  objectID,
					Relation:  relation,
				},
				User: &v0.User{UserOneof: &v0.User_Userset{Userset: &v0.ObjectAndRelation{
					Namespace: stringz.Join("/", token.System, subjectNS),
					ObjectId:  subjectID,
					Relation:  subjectRel,
				}}},
			},
		}}}

		resp, err := client.Write(context.Background(), request)
		if err != nil {
			return err
		}

		if cobrautil.MustGetBool(cmd, "json") || !term.IsTerminal(int(os.Stdout.Fd())) {
			prettyProto, err := prettyProto(resp)
			if err != nil {
				return err
			}

			fmt.Println(string(prettyProto))
			return nil
		}

		fmt.Println(resp.GetRevision().GetToken())

		return nil
	}
}
