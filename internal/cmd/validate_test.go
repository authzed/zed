package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gookit/color"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestMain(m *testing.M) {
	// Ensure that we run tests without color output.
	// This makes test output more predictable when we want
	// to assert about its output.
	// setup
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	// Also disable gookit/color, which is used in the tree printer
	color.Disable()

	// Run the tests
	code := m.Run()

	// teardown
	lipgloss.SetColorProfile(profile)

	os.Exit(code)
}

var durationRegex = regexp.MustCompile(`\([\d.]*[µmn]s\)`)

func stripDuration(s string) string {
	return durationRegex.ReplaceAllString(s, "(Xs)")
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func TestValidatePreRun(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		schemaTypeFlag string
		expectErr      bool
	}{
		`invalid`: {
			schemaTypeFlag: "invalid",
			expectErr:      true,
		},
		`composable`: {
			schemaTypeFlag: "composable",
			expectErr:      false,
		},
		`standard`: {
			schemaTypeFlag: "standard",
			expectErr:      false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
				zedtesting.StringFlag{FlagName: "schema-type", FlagValue: tc.schemaTypeFlag},
				zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: false},
			)

			err := validatePreRunE(cmd, []string{})
			if tc.expectErr {
				require.ErrorContains(t, err, "schema-type must be one of \"\", \"standard\", \"composable\"")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		schemaTypeFlag          string
		files                   []string
		expectErr               string
		expectStr               string
		expectNonZeroStatusCode bool
	}{
		`standard_passes`: {
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
			},
			expectStr: "Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated\n",
		},
		`external_schema_passes`: {
			files: []string{
				filepath.Join("validate-test", "external-schema.yaml"),
			},
			expectStr: "Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated\n",
		},
		`multiple_files_passes`: {
			files: []string{
				filepath.Join("validate-test", "standard-validation.yaml"),
				filepath.Join("validate-test", "external-schema.yaml"),
			},
			expectStr: filepath.Join("validate-test", "standard-validation.yaml") + `
Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated
` + filepath.Join("validate-test", "external-schema.yaml") + `
Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated
total files: 2, successfully validated files: 2
`,
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
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
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
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`without_schema_fails`: {
			files: []string{
				filepath.Join("validate-test", "missing-schema.yaml"),
			},
			expectErr: "either schema or schemaFile must be present",
		},
		`warnings_fail`: {
			files: []string{
				filepath.Join("validate-test", "warnings.zed"),
			},
			expectStr: `warning: Permission "view" references itself, which will cause an error to be raised due to infinite recursion (permission-references-itself)
 1 | definition test {
 2 >  permission view = view
   >                    ^~~~
 3 | }

complete - 0 relationships loaded, 0 assertions run, 0 expected relations validated
`,
		},
		`assertions_fail`: {
			files: []string{
				filepath.Join("validate-test", "failed-assertions.yaml"),
			},
			expectNonZeroStatusCode: true,
			expectStr: `error: parse error in ` + "`document:1#viewer@user:maria`" + `, line 11, column 7: Expected relation or permission document:1#viewer@user:maria to exist           
  8 |   }
  9 | assertions:
 10 |   assertTrue:
 11 >     - "document:1#viewer@user:maria"
    >        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
 12 | 

  Explanation:
  ⨉ document:1 viewer (105.292µs)
  └── ⨉ document:1 view (60.083µs)
  


`,
		},
		`expected_relations_fail`: {
			files: []string{
				filepath.Join("validate-test", "failed-expected-relations.yaml"),
			},
			expectNonZeroStatusCode: true,
			expectStr: "error: parse error in `[user:maria] is <document:1#view>`, line 11, column 7: For object and permission/relation `document:1#viewer`, missing expected subject\n" +
				"`user:maria`                                                                    \n " +
				" 8 |   }\n " +
				" 9 | validation:\n" +
				" 10 |   document:1#viewer:\n" +
				" 11 >     - \"[user:maria] is <document:1#view>\"\n" +
				"    >        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~\n" +
				" 12 | \n\n\n",
		},
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
		`missing_relation_fails`: {
			files: []string{
				filepath.Join("validate-test", "missing-relation.zed"),
			},
			expectNonZeroStatusCode: true,
			expectStr: "error: parse error in `write`, line 2, column 20: relation/permission `write` not found under definition `test`                   \n" +
				" 1 |  definition test {\n" +
				" 2 >   permission view = write\n" +
				"   >                     ^~~~~\n " +
				"3 |  }\n " +
				"4 | \n\n\n",
		},
		`missing_relation_in_yaml_fails`: {
			files: []string{
				filepath.Join("validate-test", "missing-relation.yaml"),
			},
			expectNonZeroStatusCode: true,
			expectStr: "error: parse error in `write`, line 4, column 21: relation/permission `write` not found under definition `test`                   \n" +
				"  6 |   definition user {}\n" +
				"  7 |   definition test {\n" +
				"  8 |     relation viewer: user\n" +
				"  9 >     permission view = write\n" +
				"    >                       ^~~~~\n" +
				" 10 |   }\n" +
				" 11 | \n\n\n",
		},
		// TODO: https://github.com/authzed/zed/issues/487
		//`url_passes`: {
		//	files: []string{
		//		"https://play.authzed.com/s/iksdFvCtvnkR/schema",
		//	},
		//},
		`composable_schema_passes`: {
			files: []string{
				filepath.Join("validate-test", "composable-schema-root.zed"),
			},
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`composable_schema_only_without_flag_passes`: {
			files: []string{
				filepath.Join("validate-test", "only-passes-composable.zed"),
			},
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`standard_only_with_composable_flag_fails`: {
			schemaTypeFlag: "composable",
			files: []string{
				filepath.Join("validate-test", "only-passes-standard.zed"),
			},
			expectErr: "Expected identifier, found token TokenTypeKeyword",
		},
		`composable_only_with_standard_flag_fails`: {
			schemaTypeFlag: "standard",
			files: []string{
				filepath.Join("validate-test", "only-passes-composable.zed"),
			},
			expectErr: "Unexpected token at root level",
		},
		`composable_in_validation_yaml_with_standard_fails`: {
			schemaTypeFlag: "standard",
			files: []string{
				filepath.Join("validate-test", "external-and-composable.yaml"),
			},
			expectErr: "Unexpected token at root level",
		},
		`composable_in_validation_yaml_with_composable_passes`: {
			schemaTypeFlag: "composable",
			files: []string{
				filepath.Join("validate-test", "external-and-composable.yaml"),
			},
			expectStr: "Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated\n",
		},
		`warnings_in_composable_fail`: {
			schemaTypeFlag: "composable",
			files: []string{
				filepath.Join("validate-test", "composable-schema-warning-root.zed"),
			},
			expectStr: `warning: Permission "edit" references itself, which will cause an error to be raised due to infinite recursion (permission-references-itself)
 1 | partial edit_partial {
 2 >     permission edit = edit
   >                       ^~~~
 3 | }
 4 | 

complete - 0 relationships loaded, 0 assertions run, 0 expected relations validated
`,
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
				zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: false},
			)

			res, shouldError, err := validateCmdFunc(cmd, tc.files)
			if tc.expectErr == "" {
				require.NoError(err)
				require.Equal(normalizeNewlines(stripDuration(tc.expectStr)), normalizeNewlines(stripDuration(res)))
			} else {
				require.Error(err)
				require.Contains(err.Error(), tc.expectErr)
			}
			require.Equal(tc.expectNonZeroStatusCode, shouldError)
		})
	}
}

func TestFailOnWarn(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Run once with fail-on-warn set to false
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100}, zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
		zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: false},
	)

	_, shouldError, _ := validateCmdFunc(cmd, []string{filepath.Join("validate-test", "schema-with-warnings.zed")})
	require.False(shouldError, "validation pass should not fail without fail-on-warn")

	// Run again with fail-on-warn set to true
	cmd = zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "schema-type", FlagValue: ""},
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
		zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: true},
	)

	_, shouldError, _ = validateCmdFunc(cmd, []string{filepath.Join("validate-test", "schema-with-warnings.zed")})
	require.True(shouldError, "validation pass should fail with fail-on-warn")
}
