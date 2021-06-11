package main

import (
	"context"
	"fmt"
	"os"

	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/authzed/zed/internal/printers"
)

var permissionCmd = &cobra.Command{
	Use:   "permission <subcommand>",
	Short: "perform queries on the permissions in a permission system",
}

var checkCmd = &cobra.Command{
	Use:               "check <user:id> <permission> <object:id>",
	Short:             "check that a permission exists between a user and an object",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              checkCmdFunc,
}

var expandCmd = &cobra.Command{
	Use:               "expand <permission> <object:id>",
	Short:             "expand a relation on an object",
	Args:              cobra.ExactArgs(2),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              expandCmdFunc,
}

func checkCmdFunc(cmd *cobra.Command, args []string) error {
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

	request := &api.CheckRequest{
		TestUserset: &api.ObjectAndRelation{
			Namespace: stringz.Join("/", token.Name, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
		User: &api.User{UserOneof: &api.User_Userset{
			Userset: &api.ObjectAndRelation{
				Namespace: stringz.Join("/", token.Name, userNS),
				ObjectId:  userID,
				Relation:  "...",
			},
		}},
	}

	if zookie := cobrautil.MustGetString(cmd, "revision"); zookie != "" {
		request.AtRevision = &api.Zookie{Token: zookie}
	}

	resp, err := client.Check(context.Background(), request)
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

	fmt.Println(resp.Membership == api.CheckResponse_MEMBER)

	return nil
}

func expandCmdFunc(cmd *cobra.Command, args []string) error {
	relation := args[0]

	var objectNS, objectID string
	err := stringz.SplitExact(args[1], ":", &objectNS, &objectID)
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

	request := &api.ExpandRequest{
		Userset: &api.ObjectAndRelation{
			Namespace: stringz.Join("/", token.Name, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
	}

	if zookie := cobrautil.MustGetString(cmd, "revision"); zookie != "" {
		request.AtRevision = &api.Zookie{Token: zookie}
	}

	resp, err := client.Expand(context.Background(), request)
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

	tp := treeprinter.New()
	printers.TreeNodeTree(tp, resp.GetTreeNode())
	fmt.Println(tp.String())

	return nil
}
