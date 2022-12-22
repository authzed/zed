package main

import (
	"os"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/commands"
)

var SyncFlagsCmdFunc = cobrautil.SyncViperPreRunE("ZED")

func main() {
	zl := cobrazerolog.New()

	rootCmd := &cobra.Command{
		Use:   "zed",
		Short: "SpiceDB client, by AuthZed",
		Long:  "A command-line client for managing SpiceDB clusters, built by AuthZed",
		PersistentPreRunE: cobrautil.CommandStack(
			zl.RunE(),
			SyncFlagsCmdFunc,
		),
	}

	zl.RegisterFlags(rootCmd.PersistentFlags())

	rootCmd.PersistentFlags().String("endpoint", "", "spicedb gRPC API endpoint")
	rootCmd.PersistentFlags().String("permissions-system", "", "permissions system to query")
	rootCmd.PersistentFlags().String("token", "", "token used to authenticate to SpiceDB")
	rootCmd.PersistentFlags().Bool("insecure", false, "connect over a plaintext connection")
	rootCmd.PersistentFlags().Bool("skip-version-check", false, "if true, no version check is performed against the server")
	rootCmd.PersistentFlags().Bool("no-verify-ca", false, "do not attempt to verify the server's certificate chain and host name")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	_ = rootCmd.PersistentFlags().MarkHidden("debug") // This cannot return its error.

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "display zed version information",
		RunE:  versionCmdFunc,
	}
	cobrautil.RegisterVersionFlags(versionCmd.Flags())
	versionCmd.Flags().Bool("include-remote-version", true, "whether to display the version of Authzed or SpiceDB for the current context")
	rootCmd.AddCommand(versionCmd)

	// Register root-level aliases
	rootCmd.AddCommand(&cobra.Command{
		Use:   "use <context>",
		Short: "an alias for `zed context use`",
		Args:  cobra.MaximumNArgs(1),
		RunE:  contextUseCmdFunc,
	})

	// Register CLI-only commands.
	registerContextCmd(rootCmd)
	registerImportCmd(rootCmd)
	registerValidateCmd(rootCmd)

	// Register shared commands.
	commands.RegisterPermissionCmd(rootCmd)
	commands.RegisterRelationshipCmd(rootCmd)

	schemaCmd := commands.RegisterSchemaCmd(rootCmd)
	registerAdditionalSchemaCmds(schemaCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
