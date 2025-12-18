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
