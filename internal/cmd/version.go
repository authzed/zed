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

func getClientVersion(cmd *cobra.Command) string {
	includeDeps := cobrautil.MustGetBool(cmd, "include-deps")
	return cobrautil.UsageVersion("zed", includeDeps)
}

func getServerVersion(cmd *cobra.Command, spiceClient v1.SchemaServiceClient) (string, error) {
	var headerMD, trailerMD metadata.MD
	// NOTE: we ignore the error here, as it may be due to a schema not existing, or
	// the client being unable to connect, etc. We just treat all such cases as an unknown
	// version.
	// NOTE: the client already has the request header set by a middleware.
	_, _ = spiceClient.ReadSchema(cmd.Context(), &v1.ReadSchemaRequest{}, grpc.Header(&headerMD), grpc.Trailer(&trailerMD))
	versionFromHeader := headerMD.Get(string(responsemeta.ServerVersion))
	versionFromTrailer := trailerMD.Get(string(responsemeta.ServerVersion))
	if len(versionFromHeader) == 1 && len(versionFromTrailer) == 1 && versionFromHeader[0] != versionFromTrailer[0] {
		return "", fmt.Errorf("mismatched server versions in header (%s) and trailer (%s)", versionFromHeader[0], versionFromTrailer[0])
	}

	if len(versionFromHeader) == 1 && versionFromHeader[0] != "" {
		return versionFromHeader[0], nil
	}
	if len(versionFromTrailer) == 1 && versionFromTrailer[0] != "" {
		return versionFromTrailer[0], nil
	}
	return "(unknown)", nil
}

func versionCmdFunc(cmd *cobra.Command, _ []string) error {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		color.Disable()
	}

	includeRemoteVersion := cobrautil.MustGetBool(cmd, "include-remote-version")
	if includeRemoteVersion {
		green := color.FgGreen.Render
		fmt.Print(green("client: "))
	}

	console.Println(getClientVersion(cmd))

	if includeRemoteVersion {
		configStore, secretStore := client.DefaultStorage()
		_, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)

		if err == nil {
			spiceClient, err := client.NewClient(cmd)
			if err != nil {
				return err
			}
			serverVersion, err := getServerVersion(cmd, spiceClient)
			if err != nil {
				return err
			}

			blue := color.FgLightBlue.Render
			fmt.Print(blue("service: "))
			console.Println(serverVersion)
		}
	}

	return nil
}
