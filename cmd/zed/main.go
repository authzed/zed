package main

import (
	"os"
	"path/filepath"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/grpcutil"
	"github.com/jzelinskie/cobrautil"
	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	zgrpcutil "github.com/authzed/zed/internal/grpcutil"
	"github.com/authzed/zed/internal/storage"
)

func atLeastAsFresh(zedtoken string) *v1.Consistency {
	return &v1.Consistency{
		Requirement: &v1.Consistency_AtLeastAsFresh{
			AtLeastAsFresh: &v1.ZedToken{Token: zedtoken},
		},
	}
}

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

func dialOptsFromFlags(cmd *cobra.Command, token storage.Token) []grpc.DialOption {
	grpc.WithChainUnaryInterceptor()

	interceptors := []grpc.UnaryClientInterceptor{
		zgrpcutil.LogDispatchTrailers,
	}

	if !cobrautil.MustGetBool(cmd, "skip-version-check") {
		interceptors = append(interceptors, zgrpcutil.CheckServerVersion)
	}

	opts := []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(interceptors...),
	}

	if cobrautil.MustGetBool(cmd, "insecure") && cobrautil.MustGetString(cmd, "cafile") != "" {
		panic("cafile flag cannot be combined with insecure")
	}

	if cobrautil.MustGetBool(cmd, "insecure") || (token.IsInsecure()) {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		opts = append(opts, grpcutil.WithInsecureBearerToken(token.APIToken))
	} else if cobrautil.MustGetString(cmd, "cafile") != "" {
		opts = append(opts, grpcutil.WithBearerToken(token.APIToken))
		opts = append(opts, grpcutil.WithCustomCerts(cobrautil.MustGetString(cmd, "cafile"), cobrautil.MustGetBool(cmd, "no-verify-ca")))
	} else {
		opts = append(opts, grpcutil.WithBearerToken(token.APIToken))
		opts = append(opts, grpcutil.WithSystemCerts(cobrautil.MustGetBool(cmd, "no-verify-ca")))
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
	rootCmd.PersistentFlags().Bool("skip-version-check", false, "if true, no version check is performed against the server")
	rootCmd.PersistentFlags().Bool("no-verify-ca", false, "do not attempt to verify the server's certificate chain and host name")
	rootCmd.PersistentFlags().String("cafile", "", "Use the contents of file as a CA Trust Bundle (PEM-formatted DER)")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	_ = rootCmd.PersistentFlags().MarkHidden("debug") // This cannot return its error.

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "display zed version information",
		RunE: cobrautil.CommandStack(
			LogCmdFunc,
			versionCmdFunc,
		),
	}
	cobrautil.RegisterVersionFlags(versionCmd.Flags())
	versionCmd.Flags().Bool("include-remote-version", true, "whether to display the version of Authzed or SpiceDB for the current context")
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
	registerValidateCmd(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
