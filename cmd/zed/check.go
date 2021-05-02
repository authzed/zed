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

func checkCmdFunc(cmd *cobra.Command, args []string) error {
	userNS, userID, err := SplitObject(args[0])
	if err != nil {
		return err
	}

	objectNS, objectID, err := SplitObject(args[1])
	if err != nil {
		return err
	}

	relation := args[2]

	tenant, token, endpoint, err := CurrentContext(cmd, contextConfigStore, tokenStore)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}

	client, err := ClientFromFlags(cmd, endpoint, token)
	if err != nil {
		return err
	}

	request := &api.CheckRequest{
		TestUserset: &api.ObjectAndRelation{
			Namespace: stringz.Join("/", tenant, objectNS),
			ObjectId:  objectID,
			Relation:  relation,
		},
		User: &api.User{UserOneof: &api.User_Userset{
			Userset: &api.ObjectAndRelation{
				Namespace: stringz.Join("/", tenant, userNS),
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

	fmt.Println(resp.IsMember)

	return nil
}
