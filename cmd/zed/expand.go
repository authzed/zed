package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/authzed/authzed-go"
	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/authzed/zed/internal/printers"
)

// <object:id> relation
func expandCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errors.New("invalid number of arguments")
	}

	objectNS, objectID, err := SplitObject(args[0])
	if err != nil {
		return err
	}

	relation := args[1]

	tenant, token, endpoint, err := CurrentContext(cmd, contextConfigStore, tokenStore)
	if err != nil {
		return err
	}

	tlsOpt := authzed.SystemCerts(authzed.VerifyCA)
	if cobrautil.MustGetBool(cmd, "insecure") {
		tlsOpt = authzed.SystemCerts(authzed.SkipVerifyCA)
	}

	client, err := authzed.NewClient(
		endpoint,
		authzed.Token(token),
		tlsOpt,
	)
	if err != nil {
		return err
	}

	request := &api.ExpandRequest{
		Userset: &api.ObjectAndRelation{
			Namespace: stringz.Join("/", tenant, objectNS),
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

	if cobrautil.MustGetBool(cmd, "json") || !terminal.IsTerminal(int(os.Stdout.Fd())) {
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
