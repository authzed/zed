package backupformat

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
)

var testRelationships = []string{
	`test/resource:1#reader@test/user:1`,
	`test/resource:2#reader@test/user:2`,
	`test/resource:3#reader@test/user:3`,
}

func toV1Rels(t *testing.T, rs ...string) (v1s []*v1.Relationship) {
	t.Helper()
	for _, r := range rs {
		v1s = append(v1s, tuple.MustParseV1Rel(r))
	}
	return v1s
}

func toStrs(t *testing.T, rs ...*v1.Relationship) (strs []string) {
	t.Helper()
	for _, r := range rs {
		strs = append(strs, tuple.MustV1RelString(r))
	}
	return strs
}

func TestPrefixFiltererRelationships(t *testing.T) {
	for _, tt := range []struct {
		name   string
		prefix string
		input  []string
		output []string
		err    string
	}{
		{
			name:   "empty prefix keeps all",
			prefix: "",
			input:  testRelationships,
			output: testRelationships,
			err:    "",
		},
		{
			name:   "matching prefix keeps all",
			prefix: "test",
			input:  testRelationships,
			output: testRelationships,
			err:    "",
		},
		{
			name:   "matching prefix with slash keeps all",
			prefix: "test/",
			input:  testRelationships,
			output: testRelationships,
			err:    "",
		},
		{
			name:   "matching partial prefix keeps all",
			prefix: "te",
			input:  testRelationships,
			output: testRelationships,
			err:    "",
		},
		{
			name:   "non-present prefix removes all",
			prefix: "blahblahblah",
			input:  testRelationships,
			output: nil,
			err:    "",
		},
		{
			name:   "both sides of a relationship must match prefix",
			prefix: "test",
			input:  []string{`test/resource:1#reader@nottest/user:1`},
			output: nil,
			err:    "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rw := &PrefixFilterer{Prefix: tt.prefix}
			var results []*v1.Relationship
			for _, r := range toV1Rels(t, tt.input...) {
				rewritten, err := rw.RewriteRelationship(r)
				if err != nil {
					require.ErrorContains(t, err, tt.err)
					return
				} else if rewritten == nil {
					continue
				}
				results = append(results, rewritten)
			}
			require.ElementsMatch(t, tt.output, toStrs(t, results...))
			require.Equal(t, uint64(len(tt.output)), rw.kept)
		})
	}
}

func TestPrefixReplacerRelationships(t *testing.T) {
	for _, tt := range []struct {
		name         string
		replacements map[string]string
		input        []string
		output       []string
		err          string
	}{
		{
			name:         "empty replacements is a noop",
			replacements: nil,
			input:        testRelationships,
			output:       testRelationships,
			err:          "",
		},
		{
			name:         "noop replacement is a noop",
			replacements: map[string]string{"test": "test"},
			input:        testRelationships,
			output:       testRelationships,
			err:          "",
		},
		{
			name:         "replacement only applies to matches",
			replacements: map[string]string{"test": "replaced"},
			input: []string{
				`test/resource:1#reader@test/user:1`,
				`nope/resource:2#reader@nope/user:2`,
			},
			output: []string{
				`replaced/resource:1#reader@replaced/user:1`,
				`nope/resource:2#reader@nope/user:2`,
			},
			err: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rw := &PrefixReplacer{replacements: tt.replacements}
			var results []*v1.Relationship
			for _, r := range toV1Rels(t, tt.input...) {
				rewritten, err := rw.RewriteRelationship(r)
				if err != nil {
					require.ErrorContains(t, err, tt.err)
					return
				} else if rewritten == nil {
					continue
				}
				results = append(results, rewritten)
			}

			require.ElementsMatch(t, tt.output, toStrs(t, results...))
		})
	}
}

func TestNoopRewriter(t *testing.T) {
	t.Run("schema passthrough", func(t *testing.T) {
		rw := &NoopRewriter{}
		schema := "definition test/user {}"
		result, err := rw.RewriteSchema(schema)
		require.NoError(t, err)
		require.Equal(t, schema, result)
	})

	t.Run("relationship cloning", func(t *testing.T) {
		rw := &NoopRewriter{}
		original := tuple.MustParseV1Rel("test/resource:1#reader@test/user:1")
		result, err := rw.RewriteRelationship(original)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, tuple.MustV1RelString(original), tuple.MustV1RelString(result))
		// Verify it's a clone, not the same pointer
		require.NotSame(t, original, result)
	})
}

func TestLegacyRewriter(t *testing.T) {
	t.Run("rewrites missing allowed types", func(t *testing.T) {
		rw := &LegacyRewriter{}
		schema := "definition user { relation foo /* missing allowed types */}"
		result, err := rw.RewriteSchema(schema)
		require.NoError(t, err)
		require.Contains(t, result, "/* deleted missing allowed type error */")
		require.NotContains(t, result, "/* missing allowed types */")
	})

	t.Run("rewrites short relation names", func(t *testing.T) {
		rw := &LegacyRewriter{}
		schema := "definition user {relation ab: user}"
		result, err := rw.RewriteSchema(schema)
		require.NoError(t, err)
		require.Contains(t, result, "/* deleted short relation name */")
		require.NotContains(t, result, "relation ab")
	})

	t.Run("rewrites multiple issues", func(t *testing.T) {
		rw := &LegacyRewriter{}
		schema := `definition user {
			relation ab: user
			relation foo /* missing allowed types */
		}`
		result, err := rw.RewriteSchema(schema)
		require.NoError(t, err)
		require.Contains(t, result, "/* deleted short relation name */")
		require.Contains(t, result, "/* deleted missing allowed type error */")
	})

	t.Run("passthrough for valid schema", func(t *testing.T) {
		rw := &LegacyRewriter{}
		schema := "definition test/user { relation viewer: test/user }"
		result, err := rw.RewriteSchema(schema)
		require.NoError(t, err)
		require.Equal(t, schema, result)
	})

	t.Run("relationship cloning", func(t *testing.T) {
		rw := &LegacyRewriter{}
		original := tuple.MustParseV1Rel("test/resource:1#reader@test/user:1")
		result, err := rw.RewriteRelationship(original)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, tuple.MustV1RelString(original), tuple.MustV1RelString(result))
		require.NotSame(t, original, result)
	})
}

func TestPrefixFiltererSchema(t *testing.T) {
	for _, tt := range []struct {
		name         string
		prefix       string
		inputSchema  string
		expectSchema string
		expectError  string
	}{
		{
			name:         "empty prefix returns schema as-is",
			prefix:       "",
			inputSchema:  "definition test/user {}",
			expectSchema: "definition test/user {}",
		},
		{
			name:         "matching prefix keeps definitions",
			prefix:       "test",
			inputSchema:  "definition test/user {}\n\ndefinition test/resource {}",
			expectSchema: "definition test/user {}\n\ndefinition test/resource {}",
		},
		{
			name:         "non-matching prefix filters out definitions",
			prefix:       "test",
			inputSchema:  "definition foo/user {}\n\ndefinition bar/resource {}",
			expectSchema: "",
			expectError:  "filtered all definitions from schema",
		},
		{
			name:         "mixed prefixes filters correctly",
			prefix:       "test",
			inputSchema:  "definition test/user {}\n\ndefinition foo/resource {}",
			expectSchema: "definition test/user {}",
		},
		{
			name:         "filters caveats with prefix",
			prefix:       "test",
			inputSchema:  "caveat test/one(a int) { a == 1 }\n\ncaveat foo/two(a int) { a == 2 }",
			expectSchema: "caveat test/one(a int) {\n\ta == 1\n}",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rw := &PrefixFilterer{Prefix: tt.prefix}
			result, err := rw.RewriteSchema(tt.inputSchema)
			if tt.expectError != "" {
				require.ErrorContains(t, err, tt.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectSchema, result)
			}
		})
	}
}

func TestPrefixReplacerSchema(t *testing.T) {
	for _, tt := range []struct {
		name         string
		replacements map[string]string
		inputSchema  string
		expectSchema string
		expectError  string
	}{
		{
			name:         "empty replacements is noop",
			replacements: map[string]string{},
			inputSchema:  "definition test/user {}",
			expectSchema: "definition test/user {}",
		},
		{
			name:         "replaces definition names",
			replacements: map[string]string{"test": "prod"},
			inputSchema:  "definition test/user {}",
			expectSchema: "definition prod/user {}",
		},
		{
			name:         "replaces relation type references",
			replacements: map[string]string{"test": "prod"},
			inputSchema:  "definition test/user {}\n\ndefinition test/resource {\n\trelation viewer: test/user\n}",
			expectSchema: "definition prod/user {}\n\ndefinition prod/resource {\n\trelation viewer: prod/user\n}",
		},
		{
			name:         "replaces caveats",
			replacements: map[string]string{"test": "prod"},
			inputSchema:  "caveat test/one(a int) {\n\ta == 1\n}",
			expectSchema: "caveat prod/one(a int) {\n\ta == 1\n}",
		},
		{
			name:         "only replaces matching prefixes",
			replacements: map[string]string{"test": "prod"},
			inputSchema:  "definition test/user {}\n\ndefinition other/resource {}",
			expectSchema: "definition prod/user {}\n\ndefinition other/resource {}",
		},
		{
			name:         "handles multiple replacements",
			replacements: map[string]string{"test": "prod", "dev": "staging"},
			inputSchema:  "definition test/user {}\n\ndefinition dev/resource {}",
			expectSchema: "definition prod/user {}\n\ndefinition staging/resource {}",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rw := &PrefixReplacer{replacements: tt.replacements}
			result, err := rw.RewriteSchema(tt.inputSchema)
			if tt.expectError != "" {
				require.ErrorContains(t, err, tt.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectSchema, result)
			}
		})
	}
}

func TestChainRewriter(t *testing.T) {
	t.Run("chains schema rewriters", func(t *testing.T) {
		// Chain: replace "test" -> "prod", then filter to only "prod" prefix
		chain := &ChainRewriter{
			rewriters: []Rewriter{
				&PrefixReplacer{replacements: map[string]string{"test": "prod"}},
				&PrefixFilterer{Prefix: "prod"},
			},
		}

		schema := "definition test/user {}\n\ndefinition other/resource {}"
		result, err := chain.RewriteSchema(schema)
		require.NoError(t, err)
		require.Contains(t, result, "prod/user")
		require.NotContains(t, result, "test/user")
		require.NotContains(t, result, "other/resource")
	})

	t.Run("chains relationship rewriters", func(t *testing.T) {
		// Chain: replace "test" -> "prod", then filter to only "prod" prefix
		chain := &ChainRewriter{
			rewriters: []Rewriter{
				&PrefixReplacer{replacements: map[string]string{"test": "prod"}},
				&PrefixFilterer{Prefix: "prod"},
			},
		}

		rels := toV1Rels(t,
			"test/resource:1#reader@test/user:1",
			"other/resource:2#reader@other/user:2",
		)

		var results []*v1.Relationship
		for _, rel := range rels {
			result, err := chain.RewriteRelationship(rel)
			require.NoError(t, err)
			if result != nil {
				results = append(results, result)
			}
		}

		require.Len(t, results, 1)
		require.Equal(t, "prod/resource:1#reader@prod/user:1", tuple.MustV1RelString(results[0]))
	})

	t.Run("stops chain when relationship filtered", func(t *testing.T) {
		// Filter should return nil, stopping further processing
		chain := &ChainRewriter{
			rewriters: []Rewriter{
				&PrefixFilterer{Prefix: "nonexistent"},
				&PrefixReplacer{replacements: map[string]string{"test": "prod"}},
			},
		}

		rel := tuple.MustParseV1Rel("test/resource:1#reader@test/user:1")
		result, err := chain.RewriteRelationship(rel)
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("propagates schema errors", func(t *testing.T) {
		// Filter that removes all definitions will error
		chain := &ChainRewriter{
			rewriters: []Rewriter{
				&PrefixFilterer{Prefix: "nonexistent"},
			},
		}

		schema := "definition test/user {}"
		_, err := chain.RewriteSchema(schema)
		require.ErrorContains(t, err, "filtered all definitions")
	})

	t.Run("empty chain acts as noop", func(t *testing.T) {
		chain := &ChainRewriter{rewriters: []Rewriter{}}

		schema := "definition test/user {}"
		result, err := chain.RewriteSchema(schema)
		require.NoError(t, err)
		require.Equal(t, schema, result)

		rel := tuple.MustParseV1Rel("test/resource:1#reader@test/user:1")
		relResult, err := chain.RewriteRelationship(rel)
		require.NoError(t, err)
		require.NotNil(t, relResult)
		require.Equal(t, tuple.MustV1RelString(rel), tuple.MustV1RelString(relResult))
	})
}
