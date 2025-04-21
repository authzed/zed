package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		files     []string
		expectErr string
	}{
		`standard_passes`: {
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
			},
		},
		`external_schema_passes`: {
			files: []string{
				filepath.Join("validate-test", "external-schema.yaml"),
			},
		},
		`multiple_files_passes`: {
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
				filepath.Join("validate-test", "external-schema.yaml"),
			},
		},
		`multiple_files_with_one_failure_fails`: {
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
				filepath.Join("validate-test", "invalid-schema.zed"),
			},
			expectErr: "Unexpected token at root level",
		},
		`schema_only_passes`: {
			files: []string{
				filepath.Join("validate-test", "schema-only.zed"),
			},
		},
		`invalid_schema_fails`: {
			files: []string{
				filepath.Join("validate-test", "invalid-schema.zed"),
			},
			expectErr: "Unexpected token at root level",
		},
		`standard_only_without_flag_passes`: {
			files: []string{
				filepath.Join("validate-test", "only-passes-standard.zed"),
			},
		},
		`without_schema_fails`: {
			files: []string{
				filepath.Join("validate-test", "missing-schema.yaml"),
			},
			expectErr: "either schema or schemaFile must be present",
		},
		// TODO capture errors on string and assert on them?
		//`assertions_fail`: {
		//	files: []string{
		//		filepath.Join("validate-test", "failed-assertions.yaml"),
		//	},
		//	expectErr: "",
		//},
		`invalid_url_fails`: {
			files: []string{
				"http://%zz",
			},
			expectErr: "invalid URL escape",
		},
		`url_does_not_exist_fails`: {
			files: []string{
				"https://unknown-url",
			},
			expectErr: "Get \"https://unknown-url\": dial tcp: lookup unknown-url",
		},
		// TODO: https://github.com/authzed/zed/issues/487
		//`url_passes`: {
		//	files: []string{
		//		"https://play.authzed.com/s/iksdFvCtvnkR/schema",
		//	},
		//},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require := require.New(t)
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
				zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
				zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
			)

			err := validateCmdFunc(cmd, tc.files)
			if tc.expectErr == "" {
				require.NoError(err)
			} else {
				require.ErrorContains(err, tc.expectErr)
			}
		})
	}
}
