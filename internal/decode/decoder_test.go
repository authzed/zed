package decode

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.out, tt.in)
		})
	}
}

func TestUnmarshalAsYAMLOrSchema(t *testing.T) {
	tests := []struct {
		name      string
		in        []byte
		outSchema string
		wantErr   bool
	}{
		{
			name: "valid yaml",
			in: []byte(`
schema:
  definition user {}
`),
			outSchema: `definition user {}`,
			wantErr:   false,
		},
		{
			name:      "valid schema",
			in:        []byte(`definition user {}`),
			outSchema: `definition user {}`,
			wantErr:   false,
		},
		{
			name: "invalid yaml",
			in: []byte(`
schema: ""
relationships:
	some: key
		bad: indentation
			`),
			outSchema: "",
			wantErr:   true,
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
			name: "schema with permission named schema",
			in: []byte(`definition parent {
	relation owner: user

	permission manage = owner
}

definition child {
	relation parent: parent

	permission schema = parent->manage
}

definition user {}`),
			outSchema: `definition parent {
	relation owner: user

	permission manage = owner
}

definition child {
	relation parent: parent

	permission schema = parent->manage
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
			vFile, err := UnmarshalValidationFile(tt.in)
			require.Equal(t, tt.wantErr, err != nil)
			if !tt.wantErr {
				// TODO: this test has gotten kinda vacuous for non-yaml stuff.
				require.Equal(t, tt.outSchema, vFile.Schema.Schema)
			}
		})
	}
}
