package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/TylerBrock/colorjson"
	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/authzed/zed/internal/printers"
)

var schemaCmd = &cobra.Command{
	Use:   "schema <subcommand>",
	Short: "read and write to a permission system's schema",
}

var schemaReadCmd = &cobra.Command{
	Use:               "read <object type>",
	Short:             "read the schema of current permission system",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              schemaReadCmdFunc,
}

// TODO(jzelinskie): eventually make a variant that takes 0 args and returns
// all object definitions in the schema.
func schemaReadCmdFunc(cmd *cobra.Command, args []string) error {
	token, err := TokenFromFlags(cmd)
	if err != nil {
		return err
	}

	client, err := ClientFromFlags(cmd, token.Endpoint, token.ApiToken)
	if err != nil {
		return err
	}

	resp, err := client.ReadConfig(context.Background(), &api.ReadConfigRequest{
		Namespace: stringz.Join("/", token.Name, args[0]),
	})
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
