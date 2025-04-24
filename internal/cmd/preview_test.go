package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestPreview(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		files     []string
		out       string
		expectErr string
		expectStr string
	}{
		`file_not_found`: {
			files: []string{
				filepath.Join("preview-test", "nonexistent.zed"),
			},
			expectErr: `no such file or directory`,
		},
		`happy_path`: {
			files: []string{
				filepath.Join("preview-test", "composable-schema-root.zed"),
			},
			expectStr: `definition user {}

definition resource {
	relation user: user
	permission view = user
}
`,
		},
		`cannot_be_compiled_because_using_reserved_keyword`: {
			files: []string{
				filepath.Join("preview-test", "composable-schema-invalid-root.zed"),
			},
			expectErr: "line 4, column 12: Expected identifier, found token TokenTypeKeyword",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require := require.New(t)

			tempOutFile := filepath.Join(t.TempDir(), "out.zed")
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.StringFlag{FlagName: "out", FlagValue: tempOutFile})

			err := schemaCompileCmdFunc(cmd, tc.files)
			if tc.expectErr == "" {
				require.NoError(err)
				tempOutString, err := os.ReadFile(tempOutFile)
				require.NoError(err)
				require.Equal(tc.expectStr, string(tempOutString))
			} else {
				require.Error(err)
				require.Contains(err.Error(), tc.expectErr)
			}
		})
	}
}
