package cmd

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

const testSchema = `definition test/user {}\ndefinition resource {relation reader: test/user}\n`

var testRelationships = []string{
	`test/user:1#reader@test/resource:1`,
	`test/user:2#reader@test/resource:2`,
	`test/user:3#reader@test/resource:3`,
}

func TestFilterSchemaDefs(t *testing.T) {
	for _, tt := range []struct {
		name         string
		inputSchema  string
		inputPrefix  string
		outputSchema string
		err          string
	}{
		{
			name:         "empty schema returns as is",
			inputSchema:  "",
			inputPrefix:  "",
			outputSchema: "",
			err:          "",
		},
		{
			name:         "no input prefix matches everything",
			inputSchema:  "definition test {}",
			inputPrefix:  "",
			outputSchema: "definition test {}",
			err:          "",
		},
		{
			name:         "filter over schema without prefix filters everything",
			inputSchema:  "definition test {}",
			inputPrefix:  "myprefix",
			outputSchema: "",
			err:          "filtered all definitions from schema",
		},
		{
			name:         "filter over schema with same prefix returns schema as is",
			inputSchema:  "definition myprefix/test {}\n\ndefinition myprefix/test2 {}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}\n\ndefinition myprefix/test2 {}",
			err:          "",
		},
		{
			name:         "filter over schema with different prefixes filters non matching namespaces",
			inputSchema:  "definition myprefix/test {}\n\ndefinition myprefix2/test2 {}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}",
			err:          "",
		},
		{
			name:         "filter over schema caveats with same prefixes returns as is",
			inputSchema:  "caveat myprefix/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "caveat myprefix/one(a int) {\n\ta == 1\n}",
			err:          "",
		},
		{
			name:         "filter over unprefixed schema caveats with a prefix filters out",
			inputSchema:  "caveat one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "",
			err:          "filtered all definitions from schema",
		},
		{
			name:         "filter over schema mixed prefixed/unprefixed caveats filters out",
			inputSchema:  "caveat one(a int) {\n\ta == 1\n}\n\ncaveat myprefix/two(a int) {\n\ta == 2\n}",
			inputPrefix:  "myprefix",
			outputSchema: "caveat myprefix/two(a int) {\n\ta == 2\n}",
			err:          "",
		},
		{
			name:         "filter over schema namespaces and caveats with same prefixes returns as is",
			inputSchema:  "definition myprefix/test {}\n\ncaveat myprefix/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}\n\ncaveat myprefix/one(a int) {\n\ta == 1\n}",
			err:          "",
		},
		{
			name:         "fails on invalid schema",
			inputSchema:  "definition a/test {}\n\ncaveat a/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "a",
			outputSchema: "definition a/test {}\n\ncaveat a/one(a int) {\n\ta == 1\n}",
			err:          "value does not match regex pattern",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			outputSchema, err := filterSchemaDefs(tt.inputSchema, tt.inputPrefix)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.outputSchema, outputSchema)
			}
		})
	}
}

func TestBackupParseRelsCmdFunc(t *testing.T) {
	for _, tt := range []struct {
		name          string
		filter        string
		schema        string
		relationships []string
		output        []string
		err           string
	}{
		{
			name:          "basic test",
			filter:        "test",
			schema:        testSchema,
			relationships: testRelationships,
			output:        mapRelationshipTuplesToCLIOutput(t, testRelationships),
		},
		{
			name:          "filters out",
			filter:        "test",
			schema:        testSchema,
			relationships: append([]string{"foo/user:0#reader@foo/resource:0"}, testRelationships...),
			output:        mapRelationshipTuplesToCLIOutput(t, testRelationships),
		},
		{
			name:          "allows empty backup",
			filter:        "test",
			schema:        testSchema,
			relationships: nil,
			output:        nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			cmd := createTestCobraCommandWithFlagValue(t, "prefix-filter", tt.filter)
			backupName := createTestBackup(t, tt.schema, tt.relationships)
			f, err := os.CreateTemp("", "parse-output")
			require.NoError(t, err)
			defer func() {
				_ = f.Close()
			}()
			t.Cleanup(func() {
				_ = os.Remove(f.Name())
			})

			err = backupParseRelsCmdFunc(cmd, f, []string{backupName})
			require.NoError(t, err)

			lines := readLines(t, f.Name())
			require.Equal(t, tt.output, lines)
		})
	}
}
