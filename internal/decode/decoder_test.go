package decode

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestDecoderFromURL(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})
	yamlContent := `---
schema: |-
  definition user {}
relationships: |-
  resource:1#reader@user:1
`
	invalidYamlContent := `---
schemaFile: "./external-schema.zed"
relationships: |-
  resource:1#reader@user:1
`
	schemaContent := "definition user {}\n"

	// Spin up a test HTTP server that serves the file contents based on path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid.yaml":
			_, _ = w.Write([]byte(yamlContent))
		case "/invalid.yaml":
			_, _ = w.Write([]byte(invalidYamlContent))
		case "/valid.zed":
			_, _ = w.Write([]byte(schemaContent))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Run("valid yaml file over http", func(t *testing.T) {
		u, err := url.Parse(srv.URL + "/valid.yaml")
		require.NoError(t, err)

		d, err := DecoderFromURL(u)
		require.NoError(t, err)
		require.Nil(t, d.FS)
		require.YAMLEq(t, yamlContent, string(d.Contents))

		vFile, err := d.UnmarshalAsYAMLOrSchema()
		require.NoError(t, err)
		require.Equal(t, "definition user {}", vFile.Schema.Schema)
		require.Equal(t, "resource:1#reader@user:1", vFile.Relationships.RelationshipsString)
	})

	t.Run("valid zed schema file over http", func(t *testing.T) {
		u, err := url.Parse(srv.URL + "/valid.zed")
		require.NoError(t, err)

		d, err := DecoderFromURL(u)
		require.NoError(t, err)
		require.Nil(t, d.FS)
		require.Equal(t, []byte(schemaContent), d.Contents)

		vFile, err := d.UnmarshalAsYAMLOrSchema()
		require.NoError(t, err)
		require.Equal(t, schemaContent, vFile.Schema.Schema)
	})

	t.Run("invalid yaml file over http", func(t *testing.T) {
		u, err := url.Parse(srv.URL + "/invalid.yaml")
		require.NoError(t, err)

		d, err := DecoderFromURL(u)
		require.NoError(t, err)
		require.Nil(t, d.FS)
		require.YAMLEq(t, invalidYamlContent, string(d.Contents))

		vFile, err := d.UnmarshalAsYAMLOrSchema()
		// arbitrary decision: we could fetch the remote URL, but I don't want to.
		require.ErrorContains(t, err, "cannot resolve schemaFile \"external-schema.zed\": no local filesystem context (remote URL?)")
		require.Nil(t, vFile)
	})
}

func TestUnmarshalYAMLValidationFile(t *testing.T) {
	schemaContent := "definition user {}\ndefinition resource {\nrelation reader: user\n}\n"

	// Write real files to a temp directory so DecoderFromURL -> decoderFromFile is exercised.
	dir := t.TempDir()
	// Change the directory to that directory so that expectations of locality are satisfied
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.zed"), []byte(schemaContent), 0o600))

	tests := []struct {
		name            string
		yamlContent     string
		expectedSchema  string
		expectedRels    string
		expectedErrText string
	}{
		{
			name: "resolves_local_schemaFile",
			yamlContent: `---
schemaFile: "./schema.zed"
relationships: |-
  resource:1#reader@user:1
`,
			expectedSchema: schemaContent,
			expectedRels:   "resource:1#reader@user:1",
		},
		{
			name: "rejects_schemaFile_pointing_to_above_directory",
			yamlContent: `---
schemaFile: "../secret/schema.zed"
relationships: |-
  resource:1#reader@user:1
`,
			expectedErrText: "must be local to where the command was invoked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := filepath.Join(dir, tt.name+".yaml")
			require.NoError(t, os.WriteFile(f, []byte(tt.yamlContent), 0o600))
			u, err := url.Parse(f)
			require.NoError(t, err)

			d, err := DecoderFromURL(u)
			require.NoError(t, err)

			vFile, err := d.UnmarshalYAMLValidationFile()
			if tt.expectedErrText != "" {
				require.ErrorContains(t, err, tt.expectedErrText)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedSchema, vFile.Schema.Schema)
			require.Equal(t, tt.expectedRels, vFile.Relationships.RelationshipsString)
		})
	}
}

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
		wantErr   string
	}{
		{
			name: "valid yaml",
			in: []byte(`
schema:
  definition user {}
`),
			outSchema: `definition user {}`,
		},
		{
			name:      "valid schema",
			in:        []byte(`definition user {}`),
			outSchema: `definition user {}`,
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
			wantErr:   "yaml: line 2: found character that cannot start any token",
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{Contents: tt.in}
			vFile, err := d.UnmarshalAsYAMLOrSchema()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.outSchema, vFile.Schema.Schema)
		})
	}
}
