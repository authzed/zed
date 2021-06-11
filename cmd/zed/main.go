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
		Name:     stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "permissions-system"), token.Name),
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
	rootCmd.PersistentFlags().String("permissions-system", "", "permission system to query")
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

	// Register token subcommands & flags
	rootCmd.AddCommand(tokenCmd)

	tokenCmd.AddCommand(tokenListCmd)
	tokenListCmd.Flags().Bool("reveal-tokens", false, "display secrets in results")

	tokenCmd.AddCommand(tokenSaveCmd)

	tokenCmd.AddCommand(tokenDeleteCmd)

	tokenCmd.AddCommand(tokenUseCmd)

	// Register schema subcommands & flags
	rootCmd.AddCommand(schemaCmd)

	schemaCmd.AddCommand(schemaReadCmd)
	schemaReadCmd.Flags().Bool("json", false, "output as JSON")

	// Register permission subcommands & flags
	rootCmd.AddCommand(permissionCmd)

	permissionCmd.AddCommand(checkCmd)
	checkCmd.Flags().Bool("json", false, "output as JSON")
	checkCmd.Flags().String("revision", "", "optional revision at which to check")

	permissionCmd.AddCommand(expandCmd)
	expandCmd.Flags().Bool("json", false, "output as JSON")
	expandCmd.Flags().String("revision", "", "optional revision at which to check")

	// Register relationship subcommands & flags
	rootCmd.AddCommand(relationshipCmd)

	createCmd.Flags().Bool("json", false, "output as JSON")
	relationshipCmd.AddCommand(createCmd)

	touchCmd.Flags().Bool("json", false, "output as JSON")
	relationshipCmd.AddCommand(touchCmd)

	deleteCmd.Flags().Bool("json", false, "output as JSON")
	relationshipCmd.AddCommand(deleteCmd)

	plugins := []struct{ name, description string }{
		{"testserver", "local testing server"},
	}
	for _, plugin := range plugins {
		binaryName := fmt.Sprintf("zed-%s", plugin.name)
		if commandIsAvailable(binaryName) {
			rootCmd.AddCommand(&cobra.Command{
				Use:                plugin.name,
				Short:              plugin.description,
				RunE:               pluginCmdFunc(binaryName),
				DisableFlagParsing: true, // Passes flags as args to the subcommand.
			})
		}
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
