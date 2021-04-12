package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"

	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/jzelinskie/cobrautil"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func rootCmdFunc(cmd *cobra.Command, args []string) error {
	print("root")
	return nil
}

func persistentPreRunE(cmd *cobra.Command, args []string) error {
	if err := cobrautil.SyncViperPreRunE("zed")(cmd, args); err != nil {
		return err
	}

	return nil
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "zed",
		Short: "Authzed client",
		Long:  "A client for managing authzed from your command line.",
	}

	var configCmd = &cobra.Command{
		Use:   "config <command> [args...]",
		Short: "configure client contexts and credentials",
	}

	var setTokenCmd = &cobra.Command{
		Use:  "set-token <name> <key>",
		RunE: setTokenCmdFunc,
	}

	var listTokensCmd = &cobra.Command{
		Use:  "list-tokens",
		RunE: listTokensCmdFunc,
	}

	var setContextCmd = &cobra.Command{
		Use:  "set-context <name> <tenant> <key name>",
		RunE: setContextCmdFunc,
	}

	var listContextsCmd = &cobra.Command{
		Use:  "list-contexts",
		RunE: listContextsCmdFunc,
	}

	var useContextCmd = &cobra.Command{
		Use:  "use-context <name>",
		RunE: useContextCmdFunc,
	}

	configCmd.AddCommand(setTokenCmd)
	configCmd.AddCommand(listTokensCmd)
	configCmd.AddCommand(setContextCmd)
	configCmd.AddCommand(listContextsCmd)
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

	describeCmd.PersistentFlags().String("tenant", "", "tenant to query")
	describeCmd.PersistentFlags().String("token", "", "token used to authenticate to authzed")
	describeCmd.PersistentFlags().String("endpoint", "grpc.authzed.com:443", "authzed API gRPC endpoint")

	rootCmd.AddCommand(describeCmd)

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
