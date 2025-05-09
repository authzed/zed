package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// note: these tests mess with global variables, so do not run in parallel with other tests.
func TestCommandOutput(t *testing.T) {
	cases := []struct {
		name                  string
		flagErrorContains     string
		expectUsageContains   string
		expectFlagErrorCalled bool
		expectStdErrorMsg     string
		command               []string
	}{
		{
			name:                  "prints usage on invalid command error",
			command:               []string{"zed", "madeupcommand"},
			expectFlagErrorCalled: true,
			flagErrorContains:     "unknown command",
			expectUsageContains:   "zed [command]",
		},
		{
			name:                  "prints usage on invalid flag error",
			command:               []string{"zed", "version", "--madeupflag"},
			expectFlagErrorCalled: true,
			flagErrorContains:     "unknown flag: --madeupflag",
			expectUsageContains:   "zed version [flags]",
		},
		{
			name:                  "prints usage on parameter validation error",
			command:               []string{"zed", "validate"},
			expectFlagErrorCalled: true,
			flagErrorContains:     "requires at least 1 arg(s), only received 0",
			expectUsageContains:   "zed validate <validation_file_or_schema_file> [flags]",
		},
		{
			name:                  "prints correct usage",
			command:               []string{"zed", "perm", "check"},
			expectFlagErrorCalled: true,
			flagErrorContains:     "accepts 3 arg(s), received 0",
			expectUsageContains:   "zed permission check <resource:id> <permission> <subject:id>",
		},
		{
			name:                  "does not print usage on command error",
			command:               []string{"zed", "validate", uuid.NewString()},
			expectFlagErrorCalled: false,
			expectStdErrorMsg:     "terminated with errors",
		},
	}

	zl := cobrazerolog.New(cobrazerolog.WithPreRunLevel(zerolog.DebugLevel))

	rootCmd := InitialiseRootCmd(zl)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var flagErrorCalled bool
			testFlagError := func(cmd *cobra.Command, err error) error {
				require.ErrorContains(t, err, tt.flagErrorContains)
				require.Contains(t, cmd.UsageString(), tt.expectUsageContains)
				flagErrorCalled = true
				return errParsing
			}
			stderrFile := setupOutputForTest(t, testFlagError, tt.command...)

			err := handleError(rootCmd, rootCmd.ExecuteContext(t.Context()))
			require.Error(t, err)
			stdErrBytes, err := os.ReadFile(stderrFile)
			require.NoError(t, err)
			if tt.expectStdErrorMsg != "" {
				require.Contains(t, string(stdErrBytes), tt.expectStdErrorMsg)
			} else {
				require.Len(t, stdErrBytes, 0)
			}
			require.Equal(t, tt.expectFlagErrorCalled, flagErrorCalled)
		})
	}
}

func setupOutputForTest(t *testing.T, testFlagError func(cmd *cobra.Command, err error) error, args ...string) string {
	t.Helper()

	originalLevel := zerolog.GlobalLevel()
	originalFlagError := flagError
	originalArgs := os.Args
	originalStderr := os.Stderr
	t.Cleanup(func() {
		zerolog.SetGlobalLevel(originalLevel)
		flagError = originalFlagError
		os.Args = originalArgs
		os.Stderr = originalStderr
	})

	if len(args) > 0 {
		os.Args = args
	}
	flagError = testFlagError
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	tempStdErrFileName := filepath.Join(t.TempDir(), uuid.NewString())
	tempStdErr, err := os.Create(tempStdErrFileName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tempStdErr.Close()
		_ = os.Remove(tempStdErrFileName)
	})

	os.Stderr = tempStdErr
	return tempStdErrFileName
}
