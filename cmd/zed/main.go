package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/authzed/grpcutil"
	"github.com/jzelinskie/cobrautil"
	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	zgrpcutil "github.com/authzed/zed/internal/grpcutil"
	"github.com/authzed/zed/internal/storage"
	"github.com/authzed/zed/internal/version"
)

func defaultStorage() (storage.ConfigStore, storage.SecretStore) {
	var home string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		home = filepath.Join(xdg, "zed")
	} else {
		homedir, _ := homedir.Dir()
		home = filepath.Join(homedir, ".zed")
	}
	return storage.JSONConfigStore{ConfigPath: home}, storage.KeychainSecretStore{ConfigPath: home}
}

func dialOptsFromFlags(cmd *cobra.Command, token string) []grpc.DialOption {
	opts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(zgrpcutil.LogDispatchTrailers),
	}

	if cobrautil.MustGetBool(cmd, "insecure") {
		opts = append(opts, grpc.WithInsecure())
		opts = append(opts, grpcutil.WithInsecureBearerToken(token))
	} else {
		opts = append(opts, grpcutil.WithBearerToken(token))
		tlsOpt := grpcutil.WithSystemCerts(cobrautil.MustGetBool(cmd, "no-verify-ca"))
		opts = append(opts, tlsOpt)
	}

	return opts
}

var (
	SyncFlagsCmdFunc = cobrautil.SyncViperPreRunE("ZED")
	LogCmdFunc       = cobrautil.ZeroLogRunE("log", zerolog.DebugLevel)
)

func main() {
	rootCmd := &cobra.Command{
		Use:               "zed",
		Short:             "The Authzed CLI",
		Long:              "A client for managing Authzed from your command line.",
		PersistentPreRunE: SyncFlagsCmdFunc,
	}

	cobrautil.RegisterZeroLogFlags(rootCmd.PersistentFlags(), "log")

	rootCmd.PersistentFlags().String("endpoint", "", "authzed gRPC API endpoint")
	rootCmd.PersistentFlags().String("permissions-system", "", "permissions system to query")
	rootCmd.PersistentFlags().String("token", "", "token used to authenticate to authzed")
	rootCmd.PersistentFlags().Bool("insecure", false, "connect over a plaintext connection")
	rootCmd.PersistentFlags().Bool("no-verify-ca", false, "do not attempt to verify the server's certificate chain and host name")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	_ = rootCmd.PersistentFlags().MarkHidden("debug") // This cannot return its error.

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "display zed version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.UsageVersion(cobrautil.MustGetBool(cmd, "include-deps")))
		},
	}
	versionCmd.Flags().Bool("include-deps", false, "include dependencies' versions")
	rootCmd.AddCommand(versionCmd)

	// Register root-level aliases
	rootCmd.AddCommand(&cobra.Command{
		Use:   "use <context>",
		Short: "an alias for `zed context use`",
		Args:  cobra.MaximumNArgs(1),
		RunE: cobrautil.CommandStack(
			LogCmdFunc,
			contextUseCmdFunc,
		),
	})

	registerContextCmd(rootCmd)
	registerSchemaCmd(rootCmd)
	registerPermissionCmd(rootCmd)
	registerRelationshipCmd(rootCmd)
	registerExperimentCmd(rootCmd)
	registerImportCmd(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
