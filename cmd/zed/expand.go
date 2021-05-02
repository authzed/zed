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

func expandCmdFunc(cmd *cobra.Command, args []string) error {
	objectNS, objectID, err := SplitObject(args[0])
	if err != nil {
		return err
	}

	relation := args[1]

	tenant, token, endpoint, err := CurrentContext(cmd, contextConfigStore, tokenStore)
	if err != nil {
		return err
	}

	client, err := ClientFromFlags(cmd, endpoint, token)
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
