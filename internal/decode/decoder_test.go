package decode

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/authzed/spicedb/pkg/validationfile"
)

func TestRewriteURL(t *testing.T) {
	tests := []struct {
		name string
		in   url.URL
		out  url.URL
	}{
		{
			name: "gist",
			in: url.URL{
				Scheme: "https",
				Host:   "gist.github.com",
				Path:   "/ecordell/9e2110ac4a1292b899784ed809d44b8f",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "gist.githubusercontent.com",
				Path:   "/ecordell/9e2110ac4a1292b899784ed809d44b8f/raw",
			},
		},
		{
			name: "playground schema",
			in: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/schema",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/download",
			},
		},
		{
			name: "playground relationships",
			in: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/relationships",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/download",
			},
		},
		{
			name: "playground assertions",
			in: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/assertions",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/download",
			},
		},
		{
			name: "playground expected",
			in: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/expected",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "play.authzed.com",
				Path:   "/s/KY7TEKLs5_9R/download",
			},
		},
		{
			name: "pastebin",
			in: url.URL{
				Scheme: "https",
				Host:   "pastebin.com",
				Path:   "/LuCwwBwU",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "pastebin.com",
				Path:   "/raw/LuCwwBwU",
			},
		},
		{
			name: "pastebin raw",
			in: url.URL{
				Scheme: "https",
				Host:   "pastebin.com",
				Path:   "/raw/LuCwwBwU",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "pastebin.com",
				Path:   "/raw/LuCwwBwU",
			},
		},
		{
			name: "direct",
			in: url.URL{
				Scheme: "https",
				Host:   "somethingelse.com",
				Path:   "/any/path",
			},
			out: url.URL{
				Scheme: "https",
				Host:   "somethingelse.com",
				Path:   "/any/path",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rewriteURL(&tt.in)
			require.EqualValues(t, tt.out, tt.in)
		})
	}
}

func TestUnmarshalAsYAMLOrSchema(t *testing.T) {
	tests := []struct {
		name         string
		in           []byte
		isOnlySchema bool
		outSchema    string
		wantErr      bool
	}{
		{
			name: "valid yaml",
			in: []byte(`
schema:
  definition user {}
`),
			outSchema:    `definition user {}`,
			isOnlySchema: false,
			wantErr:      false,
		},
		{
			name:         "valid schema",
			in:           []byte(`definition user {}`),
			isOnlySchema: true,
			outSchema:    `definition user {}`,
			wantErr:      false,
		},
		{
			name:         "invalid yaml",
			in:           []byte(`invalid yaml`),
			isOnlySchema: false,
			outSchema:    "",
			wantErr:      true,
		},
		{
			name: "schema with relation named schema",
			in: []byte(`definition parent {
	relation owner: user

	permission manage = owner
}

definition child {
	relation schema: parent

	permission access = schema->manage
}

definition user {}`),
			isOnlySchema: true,
			outSchema: `definition parent {
	relation owner: user

	permission manage = owner
}

definition child {
	relation schema: parent

	permission access = schema->manage
}

definition user {}`,
			wantErr: false,
		},
		{
			name: "schema with relation named something_schema",
			in: []byte(`definition parent {
	relation owner: user
	permission manage = owner
}

definition child {
	relation something_schema: parent
	permission access = something_schema->manage
}

definition user {}`),
			isOnlySchema: true,
			outSchema: `definition parent {
	relation owner: user
	permission manage = owner
}

definition child {
	relation something_schema: parent
	permission access = something_schema->manage
}

definition user {}`,
			wantErr: false,
		},
		{
			name: "schema with relation named relationships",
			in: []byte(`definition parent {
	relation owner: user
	permission manage = owner
}

definition child {
	relation relationships: parent
	permission access = relationships->manage
}

definition user {}`),
			isOnlySchema: true,
			outSchema: `definition parent {
	relation owner: user
	permission manage = owner
}

definition child {
	relation relationships: parent
	permission access = relationships->manage
}

definition user {}`,
			wantErr: false,
		},
		{
			name: "valid yaml with relation named schema inside",
			in: []byte(`schema: |-
  definition parent {
    relation owner: user
    permission manage = owner
  }

  definition child {
    relation schema: parent
    permission access = schema->manage
  }

  definition user {}
`),
			isOnlySchema: false,
			outSchema: `definition parent {
  relation owner: user
  permission manage = owner
}

definition child {
  relation schema: parent
  permission access = schema->manage
}

definition user {}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := validationfile.ValidationFile{}
			isOnlySchema, err := unmarshalAsYAMLOrSchema("", tt.in, &block)
			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, tt.isOnlySchema, isOnlySchema)
			if !tt.wantErr {
				require.Equal(t, tt.outSchema, block.Schema.Schema)
			}
		})
	}
}

func TestHasYAMLSchemaKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "yaml schema key at start of line",
			input:    "schema:\n  definition user {}",
			expected: true,
		},
		{
			name:     "yaml schema key with space before colon",
			input:    "schema :\n  definition user {}",
			expected: true,
		},
		{
			name:     "relation named schema in definition",
			input:    "definition child {\n\trelation schema: parent\n}",
			expected: false,
		},
		{
			name:     "relation named something_schema in definition",
			input:    "definition child {\n\trelation something_schema: parent\n}",
			expected: false,
		},
		{
			name:     "schema arrow expression",
			input:    "definition child {\n\tpermission access = schema->manage\n}",
			expected: false,
		},
		{
			name:     "no schema at all",
			input:    "definition user {}",
			expected: false,
		},
		{
			name:     "schema in single line comment should not trigger",
			input:    "// schema: this is a comment\ndefinition user {}",
			expected: false,
		},
		{
			name:     "schema in block comment should not trigger",
			input:    "/* schema: block comment */\ndefinition user {}",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasYAMLSchemaKey(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHasYAMLRelationshipsKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "yaml relationships key at start of line",
			input:    "schema:\n  definition user {}\nrelationships:\n  user:1#member@user:2",
			expected: true,
		},
		{
			name:     "no relationships key",
			input:    "schema:\n  definition user {}",
			expected: false,
		},
		{
			name:     "relationships in middle of line",
			input:    "  relationships: user:1#member@user:2",
			expected: false,
		},
		{
			name:     "relation named relationships in definition",
			input:    "definition child {\n\trelation relationships: parent\n}",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasYAMLRelationshipsKey(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHasYAMLSchemaFileKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "yaml schemaFile key at start of line",
			input:    "schemaFile: ./schema.zed\nrelationships:\n  user:1#member@user:2",
			expected: true,
		},
		{
			name:     "no schemaFile key",
			input:    "schema:\n  definition user {}",
			expected: false,
		},
		{
			name:     "relation named schemaFile in definition",
			input:    "definition child {\n\trelation schemaFile: parent\n}",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasYAMLSchemaFileKey(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
