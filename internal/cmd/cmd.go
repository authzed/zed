package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
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

var flagError = flagErrorFunc

func flagErrorFunc(cmd *cobra.Command, err error) error {
	cmd.Println(err)
	cmd.Println(cmd.UsageString())
	return errParsing
}

// InitialiseRootCmd This function is utilised to generate docs for zed
func InitialiseRootCmd(zl *cobrazerolog.Builder) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "zed",
		Short: "SpiceDB CLI, built by AuthZed",
		Long:  "A command-line client for managing SpiceDB clusters.",
		Example: `
zed context list
zed context set dev localhost:80 testpresharedkey --insecure
zed context set prod grpc.authzed.com:443 tc_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
zed context use dev
zed permission check --explain document:firstdoc writer user:emilia
`,
		PersistentPreRunE: cobrautil.CommandStack(
			zl.RunE(),
			SyncFlagsCmdFunc,
			commands.InjectRequestID,
		),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.SetFlagErrorFunc(func(command *cobra.Command, err error) error {
		return flagError(command, err)
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
	rootCmd.PersistentFlags().Int("max-message-size", 0, "maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed")
	rootCmd.PersistentFlags().String("proxy", "", "specify a SOCKS5 proxy address")
	rootCmd.PersistentFlags().Uint("max-retries", 10, "maximum number of sequential retries to attempt when a request fails")
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
		Args:              commands.ValidationWrapper(cobra.MaximumNArgs(1)),
		RunE:              contextUseCmdFunc,
		ValidArgsFunction: ContextGet,
	})

	// Register CLI-only commands.
	registerContextCmd(rootCmd)
	registerImportCmd(rootCmd)
	registerValidateCmd(rootCmd)
	registerBackupCmd(rootCmd)
	registerMCPCmd(rootCmd)

	// Register shared commands.
	commands.RegisterPermissionCmd(rootCmd)

	relCmd := commands.RegisterRelationshipCmd(rootCmd)

	commands.RegisterWatchCmd(rootCmd)
	commands.RegisterWatchRelationshipCmd(relCmd)

	schemaCmd := commands.RegisterSchemaCmd(rootCmd)
	schemaCompileCmd := registerAdditionalSchemaCmds(schemaCmd)
	registerPreviewCmd(rootCmd, schemaCompileCmd)

	return rootCmd
}

func Run() {
	if err := runWithoutExit(); err != nil {
		os.Exit(1)
	}
}

func runWithoutExit() error {
	zl := cobrazerolog.New(cobrazerolog.WithPreRunLevel(zerolog.DebugLevel))

	rootCmd := InitialiseRootCmd(zl)

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

	return handleError(rootCmd, rootCmd.ExecuteContext(ctx))
}

func handleError(command *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	// this snippet of code is taken from Command.ExecuteC in order to determine the command that was ultimately
	// parsed. This is necessary to be able to print the proper command-specific usage
	var findErr error
	var cmdToExecute *cobra.Command
	args := os.Args[1:]
	if command.TraverseChildren {
		cmdToExecute, _, findErr = command.Traverse(args)
	} else {
		cmdToExecute, _, findErr = command.Find(args)
	}
	if findErr != nil {
		cmdToExecute = command
	}

	if errors.Is(err, commands.ValidationError{}) {
		_ = flagError(cmdToExecute, err)
	} else if err != nil && strings.Contains(err.Error(), "unknown command") {
		_ = flagError(cmdToExecute, err)
	} else if !errors.Is(err, errParsing) {
		log.Err(err).Msg("terminated with errors")
	}

	return err
}
