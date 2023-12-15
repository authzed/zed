package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
