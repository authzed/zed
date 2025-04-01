package cmd

import (
	"fmt"
	"os"

	"github.com/gookit/color"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/authzed/authzed-go/pkg/responsemeta"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
)

func versionCmdFunc(cmd *cobra.Command, _ []string) error {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		color.Disable()
	}

	includeRemoteVersion := cobrautil.MustGetBool(cmd, "include-remote-version")
	hasContext := false
	if includeRemoteVersion {
		configStore, secretStore := client.DefaultStorage()
		_, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)
		hasContext = err == nil
	}

	if hasContext && includeRemoteVersion {
		green := color.FgGreen.Render
		fmt.Print(green("client: "))
	}

	console.Println(cobrautil.UsageVersion("zed", cobrautil.MustGetBool(cmd, "include-deps")))

	if hasContext && includeRemoteVersion {
		client, err := client.NewClient(cmd)
		if err != nil {
			return err
		}

		// NOTE: we ignore the error here, as it may be due to a schema not existing, or
		// the client being unable to connect, etc. We just treat all such cases as an unknown
		// version.
		var headerMD metadata.MD
		_, _ = client.ReadSchema(cmd.Context(), &v1.ReadSchemaRequest{}, grpc.Header(&headerMD))
		version := headerMD.Get(string(responsemeta.ServerVersion))

		blue := color.FgLightBlue.Render
		fmt.Print(blue("service: "))
		if len(version) == 1 {
			console.Println(version[0])
		} else {
			console.Println("(unknown)")
		}
	}

	return nil
}
