package main

import (
	"fmt"
	"os"

	"github.com/authzed/authzed-go"
	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

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

	rootCmd.PersistentFlags().String("endpoint", "", "authzed API gRPC endpoint")
	rootCmd.PersistentFlags().String("tenant", "", "tenant to query")
	rootCmd.PersistentFlags().String("token", "", "token used to authenticate to authzed")
	rootCmd.PersistentFlags().Bool("insecure", false, "connect over a plaintext connection")
	rootCmd.PersistentFlags().Bool("no-verify-ca", false, "do not attempt to verify the server's certificate chain and host name")

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
		Args: cobra.ExactArgs(2),
	}

	var deleteTokenCmd = &cobra.Command{
		Use:  "delete-token <name>",
		RunE: deleteTokenCmdFunc,
		Args: cobra.ExactArgs(1),
	}

	var renameTokenCmd = &cobra.Command{
		Use:  "rename-token <old> <new>",
		RunE: renameTokenCmdFunc,
		Args: cobra.ExactArgs(2),
	}

	var getTokensCmd = &cobra.Command{
		Use:  "get-tokens",
		RunE: getTokensCmdFunc,
		Args: cobra.ExactArgs(0),
	}

	getTokensCmd.Flags().Bool("reveal-tokens", false, "display secrets in results")

	var setContextCmd = &cobra.Command{
		Use:  "set-context <name> <tenant> <key name>",
		RunE: setContextCmdFunc,
		Args: cobra.ExactArgs(3),
	}

	var deleteContextCmd = &cobra.Command{
		Use:  "delete-context <name>",
		RunE: deleteContextCmdFunc,
		Args: cobra.ExactArgs(1),
	}

	var renameContextCmd = &cobra.Command{
		Use:  "rename-context <old> <new>",
		RunE: renameContextCmdFunc,
		Args: cobra.ExactArgs(2),
	}

	var getContextsCmd = &cobra.Command{
		Use:  "get-contexts",
		RunE: getContextsCmdFunc,
		Args: cobra.ExactArgs(0),
	}

	var useContextCmd = &cobra.Command{
		Use:  "use-context <name>",
		RunE: useContextCmdFunc,
		Args: cobra.ExactArgs(1),
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
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              describeCmdFunc,
	}

	describeCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(describeCmd)

	var checkCmd = &cobra.Command{
		Use:               "check <user:id> <object:id> <relation>",
		Short:             "check a relation between a user and an object",
		Args:              cobra.ExactArgs(3),
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              checkCmdFunc,
	}

	checkCmd.Flags().Bool("json", false, "output as JSON")
	checkCmd.Flags().String("revision", "", "optional revision at which to check")

	rootCmd.AddCommand(checkCmd)

	var expandCmd = &cobra.Command{
		Use:               "expand <object:id> <relation>",
		Short:             "expand a relation on an object",
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              expandCmdFunc,
	}

	expandCmd.Flags().Bool("json", false, "output as JSON")
	expandCmd.Flags().String("revision", "", "optional revision at which to check")

	rootCmd.AddCommand(expandCmd)

	var createCmd = &cobra.Command{
		Use:               "create <user:id> <object:id> relation",
		Short:             "create a relationship between a user and an object",
		Args:              cobra.ExactArgs(3),
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              writeCmdFunc(api.RelationTupleUpdate_CREATE),
	}

	createCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(createCmd)

	var touchCmd = &cobra.Command{
		Use:               "touch <user:id> <object:id> relation",
		Short:             "touch a relationship between a user and an object",
		Args:              cobra.ExactArgs(3),
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              writeCmdFunc(api.RelationTupleUpdate_TOUCH),
	}

	touchCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(touchCmd)

	var deleteCmd = &cobra.Command{
		Use:               "delete <user:id> <object:id> relation",
		Short:             "delete a relationship between a user and an object",
		Args:              cobra.ExactArgs(3),
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              writeCmdFunc(api.RelationTupleUpdate_DELETE),
	}

	deleteCmd.Flags().Bool("json", false, "output as JSON")

	rootCmd.AddCommand(deleteCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func CurrentContext(
	cmd *cobra.Command,
	ccs storage.ContextConfigStore,
	ts storage.TokenStore,
) (tenant, token, endpoint string, err error) {
	currentTenant, currentToken, currentEndpoint, err := storage.CurrentCredentials(contextConfigStore, tokenStore)
	if err != nil {
		return "", "", "", err
	}

	tenant = stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "tenant"), currentTenant)
	token = stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "token"), currentToken)
	endpoint = stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "endpoint"), currentEndpoint)

	return
}

func ClientFromFlags(cmd *cobra.Command, endpoint, token string) (*authzed.Client, error) {
	var opts []grpc.DialOption
	if !cobrautil.MustGetBool(cmd, "insecure") {
		tlsOpt := authzed.SystemCerts(authzed.VerifyCA)
		if cobrautil.MustGetBool(cmd, "no-verify-ca") {
			tlsOpt = authzed.SystemCerts(authzed.SkipVerifyCA)
		}
		opts = append(opts, tlsOpt)
	}

	opts = append(opts, authzed.Token(token))

	return authzed.NewClient(endpoint, opts...)
}
