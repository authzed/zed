package main

import (
	"context"
	"fmt"
	"os"

	v0 "github.com/authzed/authzed-go/proto/authzed/api/v0"
	"github.com/authzed/authzed-go/v0"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/authzed/zed/internal/printers"
	"github.com/authzed/zed/internal/storage"
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
	Use:               "permission <subcommand>",
	Short:             "perform queries on the Permissions in a Permissions System",
	PersistentPreRunE: persistentPreRunE,
}

var checkCmd = &cobra.Command{
	Use:               "check <subject:id> <permission> <object:id>",
	Short:             "check that a Permission exists for a Subject",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: persistentPreRunE,
	RunE:              checkCmdFunc,
}

var expandCmd = &cobra.Command{
	Use:               "expand <permission> <object:id>",
	Short:             "expand the structure of a Permission",
	Args:              cobra.ExactArgs(2),
	PersistentPreRunE: persistentPreRunE,
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

	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "permissions-system"),
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.Secret)...)
	if err != nil {
		return err
	}

	request := &v0.CheckRequest{
		TestUserset: &v0.ObjectAndRelation{
			Namespace: stringz.Join("/", token.System, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
		User: &v0.User{UserOneof: &v0.User_Userset{
			Userset: &v0.ObjectAndRelation{
				Namespace: stringz.Join("/", token.System, subjectNS),
				ObjectId:  subjectID,
				Relation:  subjectRel,
			},
		}},
	}

	if zedToken := cobrautil.MustGetString(cmd, "revision"); zedToken != "" {
		request.AtRevision = &v0.Zookie{Token: zedToken}
	}
	log.Trace().Interface("request", request).Send()

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

	fmt.Println(resp.Membership == v0.CheckResponse_MEMBER)

	return nil
}

func expandCmdFunc(cmd *cobra.Command, args []string) error {
	relation := args[0]

	var objectNS, objectID string
	err := stringz.SplitExact(args[1], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "permissions-system"),
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.Secret)...)
	if err != nil {
		return err
	}

	request := &v0.ExpandRequest{
		Userset: &v0.ObjectAndRelation{
			Namespace: stringz.Join("/", token.System, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
	}

	if zedToken := cobrautil.MustGetString(cmd, "revision"); zedToken != "" {
		request.AtRevision = &v0.Zookie{Token: zedToken}
	}
	log.Trace().Interface("request", request).Send()

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
