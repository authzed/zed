package cmd

import (
	"path/filepath"
	"testing"

	zedtesting "github.com/authzed/zed/internal/testing"

	"github.com/stretchr/testify/require"
)

func TestValidateStandardValidation(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "standard-validation.yaml")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.NoError(err)
}

func TestValidateExternalSchema(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "external-schema.yaml")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.NoError(err)
}

func TestValidateMultipleFiles(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "standard-validation.yaml")
	f2 := filepath.Join("validate-test", "external-schema.yaml")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f, f2})
	require.NoError(err)
}

func TestValidateMultipleFilesWithOneFailure(t *testing.T) {
	// Helps ensure that we're actually validating both of the files
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "standard-validation.yaml")
	f2 := filepath.Join("validate-test", "invalid-schema.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f, f2})
	require.ErrorContains(err, "Unexpected token at root level")
}

func TestValidateSchemaOnly(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "schema-only.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.NoError(err)
}

func TestValidateComposableSchema(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "composable-schema-root.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.NoError(err)
}

func TestValidateInvalidSchemaFails(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "invalid-schema.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.ErrorContains(err, "Unexpected token at root level")
}

func TestValidateComposableOnlyWithoutFlagPasses(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "only-passes-composable.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.NoError(err)
}

func TestValidateStandardOnlyWithoutFlagPasses(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "only-passes-standard.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.NoError(err)
}

func TestValidateStandardOnlyWithComposableFlagFails(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: "composable"},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "only-passes-standard.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.ErrorContains(err, "some error")
}

func TestValidateComposableOnlyWithStandardFlagFails(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: "standard"},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "only-passes-composable.zed")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.ErrorContains(err, "some error")
}

func TestValidateFileWithoutSchemaFails(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("validate-test", "missing-schema.yaml")

	// Run the validation and assert we don't have errors
	err := validateCmdFunc(cmd, []string{f})
	require.ErrorContains(err, "some error")
}

// TODO: add a test for a schema with imports in a validation yaml

