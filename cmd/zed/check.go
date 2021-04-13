package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/jzelinskie/zed/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

func checkCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 3 {
		return errors.New("invalid number of arguments")
	}

	splitUser := strings.Split(args[0], ":")
	if len(splitUser) != 2 {
		return errors.New(`user must be in format "mytenant/usernamespace:userid"`)
	}

	splitObject := strings.Split(args[1], ":")
	if len(splitObject) != 2 {
		return errors.New(`user must be in format "mytenant/objectnamespace:objectid"`)
	}

	tenant, token, err := config.CurrentCredentials(
		cobrautil.MustGetString(cmd, "tenant"),
		cobrautil.MustGetString(cmd, "token"),
	)
	if err != nil {
		return err
	}

	client, err := NewClient(
		token,
		cobrautil.MustGetString(cmd, "endpoint"),
	)
	if err != nil {
		return err
	}

	request := &api.CheckRequest{
		TestUserset: &api.ObjectAndRelation{
			Namespace: stringz.Join("/", tenant, splitObject[0]),
			ObjectId:  splitObject[1],
			Relation:  args[2],
		},
		User: &api.User{UserOneof: &api.User_Userset{
			Userset: &api.ObjectAndRelation{
				Namespace: stringz.Join("/", tenant, splitUser[0]),
				ObjectId:  splitUser[1],
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

	if cobrautil.MustGetBool(cmd, "json") || !terminal.IsTerminal(int(os.Stdout.Fd())) {
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
