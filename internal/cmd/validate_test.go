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

	"github.com/authzed/zed/internal/zedtesting"
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

var durationRegex = regexp.MustCompile(`\([\d.]*[µmn]?s\)`)

func stripDuration(s string) string {
	return durationRegex.ReplaceAllString(s, "(Xs)")
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func TestValidatePreRun(t *testing.T) {
	t.Parallel()

	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: false},
		zedtesting.StringFlag{FlagName: "type", FlagValue: ""},
	)

	err := validatePreRunE(cmd, []string{})
	require.NoError(t, err)
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		files                   []string
		expectErr               string
		expectStr               string
		expectNonZeroStatusCode bool // when there is an error with the validation process OR an error in the schema
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
			expectStr: filepath.Join("validate-test", "standard-validation.yaml") + `
Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated
` + filepath.Join("validate-test", "invalid-schema.zed") + "\nerror: parse error in `invalid-schema.zed`, line 1, column 1: Unexpected token at root level: TokenTypeIdentifier                             \n 1 > something something {}\n   > ^~~~~~~~~\n 2 | \n\n\n",
			expectNonZeroStatusCode: true,
		},
		`schema_only_passes`: {
			files: []string{
				filepath.Join("validate-test", "schema-only.zed"),
			},
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`schema_with_relation_named_schema_passes`: {
			files: []string{
				filepath.Join("validate-test", "schema-relation-named-schema.zed"),
			},
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`invalid_schema_fails`: {
			files: []string{
				filepath.Join("validate-test", "invalid-schema.zed"),
			},
			expectStr:               "error: parse error in `invalid-schema.zed`, line 1, column 1: Unexpected token at root level: TokenTypeIdentifier                             \n 1 > something something {}\n   > ^~~~~~~~~\n 2 | \n\n\n",
			expectNonZeroStatusCode: true,
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
			expectErr:               "invalid URL escape",
			expectNonZeroStatusCode: true,
		},
		`url_does_not_exist_fails`: {
			files: []string{
				"https://unknown-url",
			},
			expectErr:               "Get \"https://unknown-url\": dial tcp: lookup unknown-url",
			expectNonZeroStatusCode: true,
		},
		`missing_relation_fails`: {
			files: []string{
				filepath.Join("validate-test", "missing-relation.zed"),
			},
			expectNonZeroStatusCode: true,
			expectStr: "error: parse error in `missing-relation.zed`, line 2, column 21: relation/permission `write` not found under definition `test`                   \n" +
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
			expectStr: "error: parse error in `missing-relation.yaml`, line 4, column 21: relation/permission `write` not found under definition `test`                   \n" +
				"  6 |   definition user {}\n" +
				"  7 |   definition test {\n" +
				"  8 |     relation viewer: user\n" +
				"  9 >     permission view = write\n" +
				"    >                       ^~~~~\n" +
				" 10 |   }\n" +
				" 11 | \n\n\n",
		},
		// TODO: https://github.com/authzed/zed/issues/487
		// `url_passes`: {
		//	files: []string{
		//		"https://play.authzed.com/s/iksdFvCtvnkR/schema",
		//	},
		// },
		`composable_schema_passes`: {
			files: []string{
				filepath.Join("validate-test", "composable-schema-root.zed"),
			},
			expectStr: "Success! - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`composable_in_validation_yaml_with_composable_passes`: {
			files: []string{
				filepath.Join("validate-test", "external-and-composable.yaml"),
			},
			expectStr: "Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated\n",
		},
		`warnings_in_composable_fail`: {
			files: []string{
				filepath.Join("validate-test", "composable-schema-warning-root.zed"),
			},
			expectStr: "warning: Permission \"edit\" references itself, which will cause an error to be raised due to infinite recursion (permission-references-itself)\n  1 | use partial\n  2 | \n  3 | partial edit_partial {\n  4 >     permission edit = edit\n    >                       ^~~~\n  5 | }\n  6 | \n\ncomplete - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`warnings_point_at_correct_line_in_zed`: {
			files: []string{
				filepath.Join("validate-test", "warnings-point-at-right-line.zed"),
			},
			expectStr: "warning: Permission \"delete_resource\" references parent type \"resource\" in its name; it is recommended to drop the suffix (relation-name-references-parent)\n 23 |  permission can_admin = admin\n 24 | \n 25 |  /** delete_resource allows a user to delete the resource. */\n 26 >  permission delete_resource = can_admin\n    >             ^~~~~~~~~~~~~~~\n 27 | }\n 28 | \n\ncomplete - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`warnings_point_at_correct_line_in_yaml`: {
			files: []string{
				filepath.Join("validate-test", "warnings-point-at-right-line.yaml"),
			},
			expectStr: "warning: Permission \"delete_resource\" references parent type \"resource\" in its name; it is recommended to drop the suffix (relation-name-references-parent)\n 23 |     /** can_admin allows a user to administer the resource */\n 24 |     permission can_admin = admin\n 25 | \n 26 >     /** delete_resource allows a user to delete the resource. */\n    >         ^~~~~~~~~~~~~~~\n 27 |     permission delete_resource = can_admin\n 28 |   }\n\ncomplete - 0 relationships loaded, 0 assertions run, 0 expected relations validated\n",
		},
		`missing_import_file_fails`: {
			files: []string{
				filepath.Join("validate-test", "nonexistant-import.zed"),
			},
			expectNonZeroStatusCode: true,
			expectStr:               "error: parse error in `nonexistant-import.zed`, line 7, column 1: failed to read import \"doesnotexist.zed\": open doesnotexist.zed: no such file or\ndirectory                                                                       \n 4 | \n 5 | definition resource {}\n 6 | \n 7 > import \"doesnotexist.zed\"\n 8 | \n\n\n",
		},
		`error_in_imported_file_fails_with_correct_file_and_line_pointer`: {
			files: []string{
				filepath.Join("validate-test", "nested-composable-schema.zed"),
			},
			expectNonZeroStatusCode: true,
			expectStr:               "error: parse error in `nonexistant-import.zed`, line 7, column 1: failed to read import \"doesnotexist.zed\": open doesnotexist.zed: no such file or\ndirectory                                                                       \n 4 | \n 5 | definition resource {}\n 6 | \n 7 > import \"doesnotexist.zed\"\n 8 | \n\n\n",
		},
		`error_in_imported_file_fails_with_correct_file_and_line_pointer_2`: {
			files: []string{
				filepath.Join("validate-test", "composable-schema-imports-file-with-error.zed"),
			},
			expectNonZeroStatusCode: true,
			expectStr:               "error: parse error in `composable-schema-imported-with-error.zed`, line 5, column 23: relation/permission `unknownrel` not found under definition `group`             \n  2 | definition user {}\n  3 | \n  4 | definition group {\n  5 >     permission view = unknownrel\n    >                       ^~~~~~~~~~\n  6 | }\n  7 | \n\n\n",
		},
		`yaml_with_composable_schemaFile_with_import_error`: {
			files: []string{
				filepath.Join("validate-test", "external-composable-with-error.yaml"),
			},
			expectNonZeroStatusCode: true,
			expectStr:               "error: parse error in `composable-schema-with-import-error-imported.zed`, line 5, column 23: relation/permission `unknownrel` not found under definition `group`             \n 3 | definition group {\n 4 |     relation member: user\n 5 |     permission view = unknownrel\n 6 > }\n 7 | \n\n\n",
		},
		`yaml_content_with_zed_extension_gives_hint`: {
			files: []string{
				filepath.Join("validate-test", "yaml-with-zed-extension.zed"),
			},
			expectStr:               "error: file \"" + filepath.Join("validate-test", "yaml-with-zed-extension.zed") + "\" has a .zed extension but appears to be a YAML validation file.\n  Rename the file to use a .yaml extension, or use --type yaml to override:\n    zed validate " + filepath.Join("validate-test", "yaml-with-zed-extension.zed") + " --type yaml\n\n",
			expectNonZeroStatusCode: true,
		},
		`yaml_with_schemaFile_escape_attempt_fails`: {
			files: []string{
				filepath.Join("validate-test", "external-schema-escape.yaml"),
			},
			expectNonZeroStatusCode: true,
			expectErr:               `schema filepath "../some-schema.zed" must be local to where the command was invoked`,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require := require.New(t)
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
				zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
				zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: false},
				zedtesting.StringFlag{FlagName: "type", FlagValue: ""},
			)

			res, shouldError, err := validateCmdFunc(cmd, tc.files)
			if tc.expectErr == "" {
				require.NoError(err)
				require.Equal(normalizeNewlines(stripDuration(tc.expectStr)), normalizeNewlines(stripDuration(res)))
			} else {
				require.Error(err)
				require.Contains(err.Error(), tc.expectErr)
			}
			require.Equal(tc.expectNonZeroStatusCode, shouldError, "non-zero status code value didn't match expectation")
		})
	}
}

func TestFailOnWarn(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Run once with fail-on-warn set to false
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100}, zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
		zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: false},
		zedtesting.StringFlag{FlagName: "type", FlagValue: ""},
	)

	_, shouldError, _ := validateCmdFunc(cmd, []string{filepath.Join("validate-test", "schema-with-warnings.zed")})
	require.False(shouldError, "validation pass should not fail without fail-on-warn")

	// Run again with fail-on-warn set to true
	cmd = zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "force-color", FlagValue: false},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
		zedtesting.BoolFlag{FlagName: "fail-on-warn", FlagValue: true},
		zedtesting.StringFlag{FlagName: "type", FlagValue: ""},
	)

	_, shouldError, _ = validateCmdFunc(cmd, []string{filepath.Join("validate-test", "schema-with-warnings.zed")})
	require.True(shouldError, "validation pass should fail with fail-on-warn")
}
