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

func registerPermissionCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(permissionCmd)

	permissionCmd.AddCommand(checkCmd)
	checkCmd.Flags().Bool("json", false, "output as JSON")
	checkCmd.Flags().String("revision", "", "optional revision at which to check")

	permissionCmd.AddCommand(expandCmd)
	expandCmd.Flags().Bool("json", false, "output as JSON")
	expandCmd.Flags().String("revision", "", "optional revision at which to check")
}

var permissionCmd = &cobra.Command{
	Use:   "permission <subcommand>",
	Short: "perform queries on the Permissions in a Permissions System",
}

var checkCmd = &cobra.Command{
	Use:               "check <subject:id> <permission> <object:id>",
	Short:             "check that a Permission exists for a Subject",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              checkCmdFunc,
}

var expandCmd = &cobra.Command{
	Use:               "expand <permission> <object:id>",
	Short:             "expand the structure of a Permission",
	Args:              cobra.ExactArgs(2),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              expandCmdFunc,
}

func parseSubject(s string) (namespace, id, relation string, err error) {
	err = stringz.SplitExact(s, ":", &namespace, &id)
	if err != nil {
		return
	}
	err = stringz.SplitExact(id, "#", &id, &relation)
	if err != nil {
		relation = "..."
		err = nil
	}
	return
}

func checkCmdFunc(cmd *cobra.Command, args []string) error {
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
			Namespace: stringz.Join("/", token.System, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
		User: &api.User{UserOneof: &api.User_Userset{
			Userset: &api.ObjectAndRelation{
				Namespace: stringz.Join("/", token.System, subjectNS),
				ObjectId:  subjectID,
				Relation:  subjectRel,
			},
		}},
	}

	if zedToken := cobrautil.MustGetString(cmd, "revision"); zedToken != "" {
		request.AtRevision = &api.Zookie{Token: zedToken}
	}
	fmt.Println(request)

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
			Namespace: stringz.Join("/", token.System, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
	}

	if zedToken := cobrautil.MustGetString(cmd, "revision"); zedToken != "" {
		request.AtRevision = &api.Zookie{Token: zedToken}
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
