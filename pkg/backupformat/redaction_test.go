package backupformat

import (
	"bytes"
	"encoding/base64"
	"io"
	"testing"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/stretchr/testify/require"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
)

func TestRedactSchema(t *testing.T) {
	tcs := []struct {
		name         string
		opts         RedactionOptions
		in           string
		out          string
		redactionMap RedactionMap
	}{
		{
			name: "single def",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in:  `definition user {}`,
			out: `definition def0 {}`,
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user": "def0",
				},
				Caveats:   map[string]string{},
				Relations: map[string]string{},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "basic with relations",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			definition resource {
				relation viewer: user
				permission view = viewer
			}`,
			out: "definition def0 {}\n\ndefinition def1 {\n\trelation rel2: def0\n\tpermission rel3 = rel2\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user":     "def0",
					"resource": "def1",
				},
				Caveats: map[string]string{},
				Relations: map[string]string{
					"view":   "rel3",
					"viewer": "rel2",
				},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "no relation rewriting",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   false,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			definition resource {
				relation viewer: user
				permission view = viewer
			}`,
			out: "definition def0 {}\n\ndefinition def1 {\n\trelation viewer: def0\n\tpermission view = viewer\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user":     "def0",
					"resource": "def1",
				},
				Caveats:   map[string]string{},
				Relations: map[string]string{},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "no definition rewriting",
			opts: RedactionOptions{
				RedactDefinitions: false,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			definition resource {
				relation viewer: user
				permission view = viewer
			}`,
			out: "definition user {}\n\ndefinition resource {\n\trelation rel0: user\n\tpermission rel1 = rel0\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{},
				Caveats:     map[string]string{},
				Relations: map[string]string{
					"view":   "rel1",
					"viewer": "rel0",
				},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "basic with caveats",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			caveat some_caveat(is_allowed bool) {
				is_allowed
			}

			definition resource {
				relation viewer: user with some_caveat
				permission view = viewer
			}`,
			out: "definition def0 {}\n\ncaveat cav2(is_allowed bool) {\n\tis_allowed\n}\n\ndefinition def1 {\n\trelation rel3: def0 with cav2\n\tpermission rel4 = rel3\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user":     "def0",
					"resource": "def1",
				},
				Caveats: map[string]string{
					"some_caveat": "cav2",
				},
				Relations: map[string]string{
					"view":   "rel4",
					"viewer": "rel3",
				},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "all expressions",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			definition resource {
				relation viewer: user
				relation editor: user

				permission somethingelse = viewer->editor + nil
				permission view = viewer & (viewer - editor)
			}`,
			out: "definition def0 {}\n\ndefinition def1 {\n\trelation rel2: def0\n\trelation rel3: def0\n\tpermission rel4 = rel2->rel3 + nil\n\tpermission rel5 = rel2 & (rel2 - rel3)\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user":     "def0",
					"resource": "def1",
				},
				Caveats: map[string]string{},
				Relations: map[string]string{
					"editor":        "rel3",
					"somethingelse": "rel4",
					"view":          "rel5",
					"viewer":        "rel2",
				},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "all types",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			definition someothertype {
				relation somerel: user
			}

			definition resource {
				relation viewer: user | user:* | someothertype#somerel
			}`,
			out: "definition def0 {}\n\ndefinition def1 {\n\trelation rel3: def0\n}\n\ndefinition def2 {\n\trelation rel4: def0 | def0:* | def1#rel3\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user":          "def0",
					"someothertype": "def1",
					"resource":      "def2",
				},
				Caveats: map[string]string{},
				Relations: map[string]string{
					"somerel": "rel3",
					"viewer":  "rel4",
				},
				ObjectIDs: map[string]string{},
			},
		},
		{
			name: "same relation name in different definitions",
			opts: RedactionOptions{
				RedactDefinitions: true,
				RedactRelations:   true,
				RedactObjectIDs:   true,
			},
			in: `
			definition user {}

			definition someothertype {
				relation somerel: user
			}
			
			definition resource {
				relation somerel: user
			}`,
			out: "definition def0 {}\n\ndefinition def1 {\n\trelation rel3: def0\n}\n\ndefinition def2 {\n\trelation rel3: def0\n}",
			redactionMap: RedactionMap{
				Definitions: map[string]string{
					"user":          "def0",
					"someothertype": "def1",
					"resource":      "def2",
				},
				Caveats: map[string]string{},
				Relations: map[string]string{
					"somerel": "rel3",
				},
				ObjectIDs: map[string]string{},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			out, redactionMap, err := redactSchema(tc.in, tc.opts)
			require.NoError(t, err)
			require.Equal(t, tc.out, out)
			require.Equal(t, tc.redactionMap, redactionMap)
		})
	}
}

func TestRedactBackup(t *testing.T) {
	exampleSchema := `
	definition user {}

	definition someotheresource {
		relation foo: user
	}

	definition resource {
		relation viewer: user | user:*
		permission view = viewer
	}`

	exampleRelationships := []*v1.Relationship{
		{
			Resource: &v1.ObjectReference{
				ObjectType: "resource",
				ObjectId:   "resource1",
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "user1",
				},
			},
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "resource",
				ObjectId:   "resource2",
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "user2",
				},
			},
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "resource",
				ObjectId:   "resource1",
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "user2",
				},
			},
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "someotheresource",
				ObjectId:   "resource1",
			},
			Relation: "foo",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "user1",
				},
			},
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "resource",
				ObjectId:   "resource3",
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   tuple.PublicWildcard,
				},
			},
		},
	}

	// Write some data.
	buf := bytes.Buffer{}
	enc := NewOcfEncoder(&buf)
	err := enc.WriteSchema(exampleSchema, base64.StdEncoding.EncodeToString(gofakeit.ImageJpeg(10, 10)))
	require.NoError(t, err)

	for _, rel := range exampleRelationships {
		require.NoError(t, enc.Append(rel, ""))
	}
	require.NoError(t, enc.Close())
	require.NotEmpty(t, buf.Bytes())

	// Redact it into a new buffer.
	redactedBuf := bytes.Buffer{}

	decoder, err := NewDecoder(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	r, err := NewRedactor(decoder, &redactedBuf, RedactionOptions{
		RedactDefinitions: true,
		RedactRelations:   true,
		RedactObjectIDs:   true,
	})
	require.NoError(t, err)

	for {
		err := r.Next()
		if err != nil {
			require.Equal(t, io.EOF, err)
			break
		}
	}

	require.NoError(t, r.Close())

	redactionMap := r.RedactionMap().Invert()

	// Validate the redacted data.
	redactedDecoder, err := NewDecoder(bytes.NewReader(redactedBuf.Bytes()))
	require.NoError(t, err)

	schema, err := redactedDecoder.Schema()
	require.NoError(t, err)
	require.Equal(t, "definition def0 {}\n\ndefinition def1 {\n\trelation rel3: def0\n}\n\ndefinition def2 {\n\trelation rel4: def0 | def0:*\n\tpermission rel5 = rel4\n}", schema)

	zedtoken, err := decoder.ZedToken()
	require.NoError(t, err)
	redactedZedtoken, err := redactedDecoder.ZedToken()
	require.NoError(t, err)
	require.Equal(t, zedtoken, redactedZedtoken)

	for _, expected := range exampleRelationships {
		rel, err := redactedDecoder.Next()
		require.NoError(t, err)

		// Validate the redacted relationship.
		require.Equal(t, expected.Resource.ObjectType, redactionMap.Definitions[rel.Resource.ObjectType])
		require.Equal(t, expected.Resource.ObjectId, redactionMap.ObjectIDs[rel.Resource.ObjectId])
		require.Equal(t, expected.Relation, redactionMap.Relations[rel.Relation])
		require.Equal(t, expected.Subject.Object.ObjectType, redactionMap.Definitions[rel.Subject.Object.ObjectType])
		if expected.Subject.Object.ObjectId == tuple.PublicWildcard {
			require.Equal(t, tuple.PublicWildcard, rel.Subject.Object.ObjectId)
		} else {
			require.Equal(t, expected.Subject.Object.ObjectId, redactionMap.ObjectIDs[rel.Subject.Object.ObjectId])
		}
		require.Equal(t, expected.Subject.OptionalRelation, redactionMap.Relations[rel.Subject.OptionalRelation])
	}
}
