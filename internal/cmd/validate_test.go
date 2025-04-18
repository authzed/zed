package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestValidatePreRun(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: "invalid"},
	)

	err := validatePreRunE(cmd, []string{})
	require.ErrorContains(err, "schema-type must be one of \"\", \"standard\", \"composable\". received: invalid")
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		schemaTypeFlag string
		files          []string
		expectErr      string
	}{
		`standard_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
			},
		},
		`external_schema_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "external-schema.yaml"),
			},
		},
		`multiple_files_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
				filepath.Join("validate-test", "external-schema.yaml"),
			},
		},
		`multiple_files_with_one_failure_fails`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
				filepath.Join("validate-test", "invalid-schema.zed"),
			},
			expectErr: "Unexpected token at root level",
		},
		`schema_only_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "schema-only.zed"),
			},
		},
		`invalid_schema_fails`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "invalid-schema.zed"),
			},
			expectErr: "Unexpected token at root level",
		},
		`composable_schema_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "composable-schema-root.zed"),
			},
		},
		`composable_schema_only_without_flag_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "only-passes-composable.zed"),
			},
		},
		`standard_only_without_flag_passes`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "only-passes-standard.zed"),
			},
		},
		`standard_only_with_composable_flag_fails`: {
			schemaTypeFlag: "composable",
			files: []string{
				filepath.Join("validate-test", "only-passes-standard.zed"),
			},
			expectErr: "some error",
		},
		`composable_only_with_standard_flag_fails`: {
			schemaTypeFlag: "standard",
			files: []string{
				filepath.Join("validate-test", "only-passes-composable.zed"),
			},
			expectErr: "some error",
		},
		`without_schema_fails`: {
			schemaTypeFlag: "",
			files: []string{
				filepath.Join("validate-test", "missing-schema.yaml"),
			},
			expectErr: "Unexpected token at root level",
		},
		`composable_in_validation_yaml_with_standard_fails`: {
			schemaTypeFlag: "standard",
			files: []string{
				filepath.Join("validate-test", "external-and-composable.yaml"),
			},
			expectErr: "some error",
		},
		`composable_in_validation_yaml_with_composable_passes`: {
			schemaTypeFlag: "composable",
			files: []string{
				filepath.Join("validate-test", "external-and-composable.yaml"),
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require := require.New(t)
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.StringFlag{FlagName: "schema-type", FlagValue: tc.schemaTypeFlag},
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
