package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestDeterminePrefixForSchema(t *testing.T) {
	tests := []struct {
		name            string
		existingSchema  string
		specifiedPrefix string
		expectedPrefix  string
	}{
		{
			"empty schema",
			"",
			"",
			"",
		},
		{
			"no prefix, none specified",
			`definition user {}`,
			"",
			"",
		},
		{
			"no prefix, one specified",
			`definition user {}`,
			"test",
			"test",
		},
		{
			"prefix found",
			`definition test/user {}`,
			"",
			"test",
		},
		{
			"multiple prefixes found",
			`definition test/user {}
			
			definition something/resource {}`,
			"",
			"",
		},
		{
			"multiple prefixes found, one specified",
			`definition test/user {}
			
			definition something/resource {}`,
			"foobar",
			"foobar",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			found, err := determinePrefixForSchema(t.Context(), test.specifiedPrefix, nil, &test.existingSchema)
			require.NoError(t, err)
			require.Equal(t, test.expectedPrefix, found)
		})
	}
}

func TestRewriteSchema(t *testing.T) {
	tests := []struct {
		name             string
		existingSchema   string
		definitionPrefix string
		expectedSchema   string
	}{
		{
			"empty schema",
			"",
			"",
			"",
		},
		{
			"empty prefix schema",
			"definition user {}",
			"",
			"definition user {}",
		},
		{
			"empty prefix schema with specified",
			`definition user {}
			
			caveat some_caveat(someCondition int) { someCondition == 42 }
			`,
			"test",
			`definition test/user {}

caveat test/some_caveat(someCondition int) {
	someCondition == 42
}`,
		},
		{
			"prefixed schema with specified",
			"definition foo/user {}",
			"test",
			"definition foo/user {}",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			found, err := rewriteSchema(test.existingSchema, test.definitionPrefix)
			require.NoError(t, err)
			require.Equal(t, test.expectedSchema, found)
		})
	}
}

func TestSchemaCompile(t *testing.T) {
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
