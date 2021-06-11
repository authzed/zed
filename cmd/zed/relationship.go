package main

import (
	"context"
	"fmt"
	"os"

	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var relationshipCmd = &cobra.Command{
	Use:   "relationship <subcommand>",
	Short: "perform CRUD operations on the relationships in a permission system",
}

var createCmd = &cobra.Command{
	Use:               "create <subject:id> relation <object:id>",
	Short:             "create a relationship between a user and an object",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              writeRelationshipCmdFunc(api.RelationTupleUpdate_CREATE),
}

var touchCmd = &cobra.Command{
	Use:               "touch <subject:id> relation <object:id>",
	Short:             "touch a relationship between a user and an object",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              writeRelationshipCmdFunc(api.RelationTupleUpdate_TOUCH),
}

var deleteCmd = &cobra.Command{
	Use:               "delete <subject:id> relation <object:id>",
	Short:             "delete a relationship between a user and an object",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              writeRelationshipCmdFunc(api.RelationTupleUpdate_DELETE),
}

func writeRelationshipCmdFunc(operation api.RelationTupleUpdate_Operation) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		var userNS, userID string
		err := stringz.SplitExact(args[0], ":", &userNS, &userID)
		if err != nil {
			return err
		}

		relation := args[1]

		var objectNS, objectID string
		err = stringz.SplitExact(args[2], ":", &objectNS, &objectID)
		if err != nil {
			return err
		}

		token, err := TokenFromFlags(cmd)
		if err != nil {
			return err
		}

		client, err := ClientFromFlags(cmd, token.Endpoint, token.Secret)
		if err != nil {
			return err
		}

		request := &api.WriteRequest{Updates: []*api.RelationTupleUpdate{{
			Operation: operation,
			Tuple: &api.RelationTuple{
				ObjectAndRelation: &api.ObjectAndRelation{
					Namespace: stringz.Join("/", token.Name, objectNS),
					ObjectId:  objectID,
					Relation:  relation,
				},
				User: &api.User{UserOneof: &api.User_Userset{Userset: &api.ObjectAndRelation{
					Namespace: stringz.Join("/", token.Name, userNS),
					ObjectId:  userID,
					Relation:  "...",
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
