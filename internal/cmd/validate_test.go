package cmd

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	zedtesting "github.com/authzed/zed/internal/testing"
)

var durationRegex = regexp.MustCompile(`\([\d.]*µs\)`)

func stripDuration(s string) string {
	return durationRegex.ReplaceAllString(s, "(Xµs)")
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
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
			expectStr: `validate-test/standard-validation.yaml
Success! - 1 relationships loaded, 2 assertions run, 0 expected relations validated
validate-test/external-schema.yaml
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
			expectStr: `error:  Expected relation or permission document:1#viewer@user:maria to exist           
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
			expectStr: "error:  For object and permission/relation `document:1#viewer`, missing expected subject\n" +
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

			res, shouldError, err := validateCmdFunc(cmd, tc.files)
			if tc.expectErr == "" {
				require.NoError(err)
			}
			require.Equal(tc.expectNonZeroStatusCode, shouldError)
			require.Equal(stripDuration(tc.expectStr), stripDuration(res))
		})
	}
}
