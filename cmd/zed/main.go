package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/authzed/authzed-go"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/authzed/zed/internal/storage"
	"github.com/authzed/zed/internal/version"
)

func TokenFromFlags(cmd *cobra.Command) (storage.Token, error) {
	token, err := storage.CurrentToken(storage.DefaultConfigStore, storage.DefaultTokenStore)
	if err != nil {
		if errors.Is(err, storage.ErrConfigNotFound) {
			return storage.Token{}, errors.New("must first save a token: see `zed token save --help`")
		}
		return storage.Token{}, err
	}

	token = storage.Token{
		System:   stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "permissions-system"), token.System),
		Endpoint: stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "endpoint"), token.Endpoint),
		Prefix:   "",
		Secret:   stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "token"), token.Secret),
	}

	return token, nil
}

func ClientFromFlags(cmd *cobra.Command, endpoint, token string) (*authzed.Client, error) {
	var opts []grpc.DialOption

	if cobrautil.MustGetBool(cmd, "insecure") {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, authzed.Token(token)) // Tokens are only used for secure endpoints.

		tlsOpt := authzed.SystemCerts(authzed.VerifyCA)
		if cobrautil.MustGetBool(cmd, "no-verify-ca") {
			tlsOpt = authzed.SystemCerts(authzed.SkipVerifyCA)
		}
		opts = append(opts, tlsOpt)
	}

	return authzed.NewClient(endpoint, opts...)
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "zed",
		Short: "The Authzed CLI",
		Long:  "A client for managing Authzed from your command line.",
	}

	rootCmd.PersistentFlags().String("endpoint", "", "authzed gRPC API endpoint")
	rootCmd.PersistentFlags().String("permissions-system", "", "permissions system to query")
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

	// Register root-level aliases
	rootCmd.AddCommand(&cobra.Command{
		Use:               "login <system> <token>",
		Short:             "an alias for `zed context set`",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              contextSetCmdFunc,
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:               "use <system>",
		Short:             "an alias for `zed context use`",
		PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
		RunE:              contextUseCmdFunc,
	})

	registerContextCmd(rootCmd)
	registerSchemaCmd(rootCmd)
	registerPermissionCmd(rootCmd)
	registerRelationshipCmd(rootCmd)
	registerPlugins(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
