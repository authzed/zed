package main

import (
	"context"
	"fmt"
	"io"
	"os"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
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

	permissionCmd.AddCommand(lookupCmd)
	lookupCmd.Flags().Bool("json", false, "output as JSON")
	lookupCmd.Flags().String("revision", "", "optional revision at which to check")
}

var permissionCmd = &cobra.Command{
	Use:               "permission <subcommand>",
	Short:             "perform queries on the Permissions in a Permissions System",
	PersistentPreRunE: persistentPreRunE,
}

var checkCmd = &cobra.Command{
	Use:               "check <object:id> <permission> <subject:id>",
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

var lookupCmd = &cobra.Command{
	Use:               "lookup <object> <permission> <subject:id>",
	Short:             "lookup the Object Instances for which the Subject has Permission",
	Args:              cobra.ExactArgs(3),
	PersistentPreRunE: persistentPreRunE,
	RunE:              lookupCmdFunc,
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
	var objectNS, objectID string
	err := stringz.SplitExact(args[0], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	relation := args[1]

	subjectNS, subjectID, subjectRel, err := parseSubject(args[2])
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

	request := &v1.CheckPermissionRequest{
		Resource: &v1.ObjectReference{
			ObjectType: nsPrefix(objectNS, token.System),
			ObjectId:   objectID,
		},
		Permission: relation,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: nsPrefix(subjectNS, token.System),
				ObjectId:   subjectID,
			},
			OptionalRelation: subjectRel,
		},
	}

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_AtLeastAsFresh{&v1.ZedToken{Token: zedtoken}},
		}
	}
	log.Trace().Interface("request", request).Send()

	resp, err := client.CheckPermission(context.Background(), request)
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

	fmt.Println(resp.Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION)
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

	request := &v1.ExpandPermissionTreeRequest{
		Resource: &v1.ObjectReference{
			ObjectType: nsPrefix(objectNS, token.System),
			ObjectId:   objectID,
		},
		Permission: relation,
	}

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_AtLeastAsFresh{&v1.ZedToken{Token: zedtoken}},
		}
	}
	log.Trace().Interface("request", request).Send()

	resp, err := client.ExpandPermissionTree(context.Background(), request)
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
	printers.TreeNodeTree(tp, resp.TreeRoot)
	fmt.Println(tp.String())

	return nil
}

func lookupCmdFunc(cmd *cobra.Command, args []string) error {
	objectNS := args[0]
	relation := args[1]
	subjectNS, subjectID, subjectRel, err := parseSubject(args[2])
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

	request := &v1.LookupResourcesRequest{
		ResourceObjectType: nsPrefix(objectNS, token.System),
		Permission:         relation,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: nsPrefix(subjectNS, token.System),
				ObjectId:   subjectID,
			},
			OptionalRelation: subjectRel,
		},
	}

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_AtLeastAsFresh{&v1.ZedToken{Token: zedtoken}},
		}
	}
	log.Trace().Interface("request", request).Send()

	respStream, err := client.LookupResources(context.Background(), request)
	if err != nil {
		return err
	}

	for {
		resp, err := respStream.Recv()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		default:
			if cobrautil.MustGetBool(cmd, "json") || !term.IsTerminal(int(os.Stdout.Fd())) {
				prettyProto, err := prettyProto(resp)
				if err != nil {
					return err
				}

				fmt.Println(string(prettyProto))
				return nil
			}

			fmt.Println(resp.ResourceObjectId)
			return nil
		}
	}
}
