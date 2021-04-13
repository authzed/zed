package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/TylerBrock/colorjson"
	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/jzelinskie/zed/internal/config"
	"github.com/jzelinskie/zed/internal/printers"
)

func describeCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid number of arguments")
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

	resp, err := client.ReadConfig(context.Background(), &api.ReadConfigRequest{
		Namespace: stringz.Join("/", tenant, args[0]),
	})
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
	printers.NamespaceTree(tp, resp.GetConfig())
	fmt.Println(tp.String())

	return nil
}

func prettyProto(m proto.Message) ([]byte, error) {
	encoded, err := protojson.Marshal(m)
	if err != nil {
		return nil, err
	}
	var obj interface{}
	err = json.Unmarshal(encoded, &obj)
	if err != nil {
		panic("protojson decode failed: " + err.Error())
	}

	f := colorjson.NewFormatter()
	f.Indent = 2
	pretty, err := f.Marshal(obj)
	if err != nil {
		panic("colorjson encode failed: " + err.Error())
	}

	return pretty, nil
}
