package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/commands"
)

var (
	SyncFlagsCmdFunc = cobrautil.SyncViperPreRunE("ZED")
	errParsing       = errors.New("parsing error")
)

func init() {
	// NOTE: this is mostly to set up logging in the case where
	// the command doesn't exist or the construction of the command
	// errors out before the PersistentPreRunE setup in the below function.
	// It helps keep log output visually consistent for a user even in
	// exceptional cases.
	var output io.Writer

	if isatty.IsTerminal(os.Stdout.Fd()) {
		output = zerolog.ConsoleWriter{Out: os.Stderr}
	} else {
		output = os.Stderr
	}

	l := zerolog.New(output).With().Timestamp().Logger()

	log.Logger = l
}

func Run() {
	zl := cobrazerolog.New(cobrazerolog.WithPreRunLevel(zerolog.DebugLevel))

	rootCmd := &cobra.Command{
		Use:   "zed",
		Short: "SpiceDB client, by AuthZed",
		Long:  "A command-line client for managing SpiceDB clusters, built by AuthZed",
		PersistentPreRunE: cobrautil.CommandStack(
			zl.RunE(),
			SyncFlagsCmdFunc,
			commands.InjectRequestID,
		),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		cmd.Println(err)
		cmd.Println(cmd.UsageString())
		return errParsing
	})

	zl.RegisterFlags(rootCmd.PersistentFlags())

	rootCmd.PersistentFlags().String("endpoint", "", "spicedb gRPC API endpoint")
	rootCmd.PersistentFlags().String("permissions-system", "", "permissions system to query")
	rootCmd.PersistentFlags().String("hostname-override", "", "override the hostname used in the connection to the endpoint")
	rootCmd.PersistentFlags().String("token", "", "token used to authenticate to SpiceDB")
	rootCmd.PersistentFlags().String("certificate-path", "", "path to certificate authority used to verify secure connections")
	rootCmd.PersistentFlags().Bool("insecure", false, "connect over a plaintext connection")
	rootCmd.PersistentFlags().Bool("skip-version-check", false, "if true, no version check is performed against the server")
	rootCmd.PersistentFlags().Bool("no-verify-ca", false, "do not attempt to verify the server's certificate chain and host name")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	rootCmd.PersistentFlags().String("request-id", "", "optional id to send along with SpiceDB requests for tracing")
	rootCmd.PersistentFlags().Int("max-message-size", 0, "maximum size (bytes) of a gRPC message that can be sent or received by zed")
	_ = rootCmd.PersistentFlags().MarkHidden("debug") // This cannot return its error.

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Display zed and SpiceDB version information",
		RunE:  versionCmdFunc,
	}
	cobrautil.RegisterVersionFlags(versionCmd.Flags())
	versionCmd.Flags().Bool("include-remote-version", true, "whether to display the version of Authzed or SpiceDB for the current context")
	rootCmd.AddCommand(versionCmd)

	// Register root-level aliases
	rootCmd.AddCommand(&cobra.Command{
		Use:               "use <context>",
		Short:             "Alias for `zed context use`",
		Args:              cobra.MaximumNArgs(1),
		RunE:              contextUseCmdFunc,
		ValidArgsFunction: ContextGet,
	})

	// Register CLI-only commands.
	registerContextCmd(rootCmd)
	registerImportCmd(rootCmd)
	registerValidateCmd(rootCmd)
	registerBackupCmd(rootCmd)

	// Register shared commands.
	commands.RegisterPermissionCmd(rootCmd)

	relCmd := commands.RegisterRelationshipCmd(rootCmd)

	commands.RegisterWatchCmd(rootCmd)
	commands.RegisterWatchRelationshipCmd(relCmd)

	schemaCmd := commands.RegisterSchemaCmd(rootCmd)
	registerAdditionalSchemaCmds(schemaCmd)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	go func() {
		select {
		case <-signalChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, errParsing) {
			log.Err(err).Msg("terminated with errors")
		}

		os.Exit(1)
	}
}
