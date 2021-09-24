package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/authzed/grpcutil"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/authzed/zed/internal/version"
)

func dialOptsFromFlags(cmd *cobra.Command, token string) []grpc.DialOption {
	var opts []grpc.DialOption

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

func nsPrefix(ns, system string) string {
	if strings.Contains(ns, "/") {
		return ns
	}
	return stringz.Join("/", system, ns)
}

var persistentPreRunE = cobrautil.CommandStack(
	cobrautil.SyncViperPreRunE("ZED"),
	cobrautil.ZeroLogPreRunE,
)

func main() {
	rootCmd := &cobra.Command{
		Use:               "zed",
		Short:             "The Authzed CLI",
		Long:              "A client for managing Authzed from your command line.",
		PersistentPreRunE: persistentPreRunE,
	}

	cobrautil.RegisterZeroLogFlags(rootCmd.PersistentFlags())

	rootCmd.PersistentFlags().String("endpoint", "", "authzed gRPC API endpoint")
	rootCmd.PersistentFlags().String("permissions-system", "", "permissions system to query")
	rootCmd.PersistentFlags().String("token", "", "token used to authenticate to authzed")
	rootCmd.PersistentFlags().Bool("insecure", false, "connect over a plaintext connection")
	rootCmd.PersistentFlags().Bool("no-verify-ca", false, "do not attempt to verify the server's certificate chain and host name")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	_ = rootCmd.PersistentFlags().MarkHidden("debug") // This cannot return its error.

	versionCmd := &cobra.Command{
		Use:               "version",
		Short:             "display zed version information",
		PersistentPreRunE: persistentPreRunE,
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
		PersistentPreRunE: persistentPreRunE,
		RunE:              contextSetCmdFunc,
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:               "use <system>",
		Short:             "an alias for `zed context use`",
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: persistentPreRunE,
		RunE:              contextUseCmdFunc,
	})

	registerContextCmd(rootCmd)
	registerSchemaCmd(rootCmd)
	registerPermissionCmd(rootCmd)
	registerRelationshipCmd(rootCmd)
	registerExperimentCmd(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
