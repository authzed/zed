package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/jzelinskie/cobrautil"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/authzed/zed/internal/storage"
	"github.com/authzed/zed/internal/version"
)

var tokenStore = storage.KeychainTokenStore{}
var contextConfigStore = storage.LocalFsContextConfigStore{}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "zed",
		Short: "Authzed client",
		Long:  "A client for managing authzed from your command line.",
	}

	rootCmd.PersistentFlags().String("endpoint", "grpc.authzed.com:443", "authzed API gRPC endpoint")
	rootCmd.PersistentFlags().String("tenant", "", "tenant to query")
	rootCmd.PersistentFlags().String("token", "", "token used to authenticate to authzed")

	var versionCmd = &cobra.Command{
		Use:               "version",
		Short:             "display zed version information",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.UsageVersion(cobrautil.MustGetBool(cmd, "include-deps")))
		},
	}

	versionCmd.Flags().Bool("include-deps", false, "include dependencies' versions")

	rootCmd.AddCommand(versionCmd)

	var configCmd = &cobra.Command{
		Use:   "config <command> [args...]",
		Short: "configure client contexts and credentials",
	}

	var setTokenCmd = &cobra.Command{
		Use:  "set-token <name> <key>",
		RunE: setTokenCmdFunc,
	}

	var deleteTokenCmd = &cobra.Command{
		Use:  "delete-token <name>",
		RunE: deleteTokenCmdFunc,
	}

	var renameTokenCmd = &cobra.Command{
		Use:  "rename-token <old> <new>",
		RunE: renameTokenCmdFunc,
	}

	var getTokensCmd = &cobra.Command{
		Use:  "get-tokens",
		RunE: getTokensCmdFunc,
	}

	getTokensCmd.Flags().Bool("reveal-tokens", false, "display secrets in results")

	var setContextCmd = &cobra.Command{
		Use:  "set-context <name> <tenant> <key name>",
		RunE: setContextCmdFunc,
	}

	var deleteContextCmd = &cobra.Command{
		Use:  "delete-context <name>",
		RunE: deleteContextCmdFunc,
	}

	var renameContextCmd = &cobra.Command{
		Use:  "rename-context <old> <new>",
		RunE: renameContextCmdFunc,
	}

	var getContextsCmd = &cobra.Command{
		Use:  "get-contexts",
		RunE: getContextsCmdFunc,
	}

	var useContextCmd = &cobra.Command{
		Use:  "use-context <name>",
		RunE: useContextCmdFunc,
	}

	configCmd.AddCommand(getTokensCmd)
	configCmd.AddCommand(setTokenCmd)
	configCmd.AddCommand(renameTokenCmd)
	configCmd.AddCommand(deleteTokenCmd)

	configCmd.AddCommand(getContextsCmd)
	configCmd.AddCommand(setContextCmd)
	configCmd.AddCommand(renameContextCmd)
	configCmd.AddCommand(deleteContextCmd)
	configCmd.AddCommand(useContextCmd)

	rootCmd.AddCommand(configCmd)

	var describeCmd = &cobra.Command{
		Use:               "describe <namespace>",
		Short:             "Describe a namespace",
		Long:              "Describe the relations that form the provided namespace.",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              describeCmdFunc,
	}

	describeCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(describeCmd)

	var checkCmd = &cobra.Command{
		Use:               "check <user:id> <object:id> <relation>",
		Short:             "check a relation between a user and an object",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              checkCmdFunc,
	}

	checkCmd.Flags().Bool("json", false, "output as JSON")
	checkCmd.Flags().String("revision", "", "optional revision at which to check")

	rootCmd.AddCommand(checkCmd)

	var expandCmd = &cobra.Command{
		Use:               "expand <object:id> <relation>",
		Short:             "expand a relation on an object",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              expandCmdFunc,
	}

	expandCmd.Flags().Bool("json", false, "output as JSON")
	expandCmd.Flags().String("revision", "", "optional revision at which to check")

	rootCmd.AddCommand(expandCmd)

	var createCmd = &cobra.Command{
		Use:               "create <user:id> <object:id> relation",
		Short:             "create a relationship between a user and an object",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              writeCmdFunc(api.RelationTupleUpdate_CREATE),
	}

	createCmd.Flags().Bool("json", false, "output as JSON")

	var touchCmd = &cobra.Command{
		Use:               "touch <user:id> <object:id> relation",
		Short:             "touch a relationship between a user and an object",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              writeCmdFunc(api.RelationTupleUpdate_TOUCH),
	}

	touchCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(touchCmd)

	var deleteCmd = &cobra.Command{
		Use:               "delete <user:id> <object:id> relation",
		Short:             "delete a relationship between a user and an object",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              writeCmdFunc(api.RelationTupleUpdate_DELETE),
	}

	deleteCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(deleteCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type Client struct {
	api.ACLServiceClient
	api.NamespaceServiceClient
}

type GrpcMetadataCredentials map[string]string

func (gmc GrpcMetadataCredentials) RequireTransportSecurity() bool { return true }
func (gmc GrpcMetadataCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return gmc, nil
}

func NewClient(token, endpoint string) (*Client, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	creds := credentials.NewTLS(&tls.Config{RootCAs: certPool})
	conn, err := grpc.Dial(
		endpoint,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(GrpcMetadataCredentials{"authorization": "Bearer " + token}),
	)

	return &Client{
		api.NewACLServiceClient(conn),
		api.NewNamespaceServiceClient(conn),
	}, nil
}
