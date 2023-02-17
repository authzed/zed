package commands

import (
	"testing"

	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/stretchr/testify/require"
)

func TestRelationshipToString(t *testing.T) {
	for _, tt := range []struct {
		rawRel   string
		expected string
	}{
		{
			"prefix/res:123#rel@prefix/resource:1234",
			"prefix/res:123 rel prefix/resource:1234",
		},
		{
			"res:123#rel@resource:1234",
			"res:123 rel resource:1234",
		},
		{
			"res:123#rel@resource:1234[caveat_name]",
			"res:123 rel resource:1234[caveat_name]",
		},
		{
			"res:123#rel@resource:1234[caveat_name:{\"num\":1234}]",
			"res:123 rel resource:1234[caveat_name:{\"num\":1234}]",
		},
	} {
		t.Run(tt.rawRel, func(t *testing.T) {
			rel := tuple.ParseRel(tt.rawRel)
			out, err := relationshipToString(rel)
			require.NoError(t, err)
			require.Equal(t, tt.expected, out)
		})
	}
}
