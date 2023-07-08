package commands

import (
	"strings"
	"testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
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
			"res:123#rel@resource:1234#anotherrel",
			"res:123 rel resource:1234#anotherrel",
		},
		{
			"res:123#rel@resource:1234[caveat_name]",
			"res:123 rel resource:1234[caveat_name]",
		},
		{
			"res:123#rel@resource:1234[caveat_name:{\"num\":1234}]",
			"res:123 rel resource:1234[caveat_name:{\"num\":1234}]",
		},
		{
			"res:123#rel@resource:1234[caveat_name:{\"name\":\"##@@##@@\"}]",
			"res:123 rel resource:1234[caveat_name:{\"name\":\"##@@##@@\"}]",
		},
	} {
		tt := tt
		t.Run(tt.rawRel, func(t *testing.T) {
			rel := tuple.ParseRel(tt.rawRel)
			out, err := relationshipToString(rel)
			require.NoError(t, err)
			require.Equal(t, tt.expected, out)
		})
	}
}

func TestArgsToRelationship(t *testing.T) {
	for _, tt := range []struct {
		args []string
		expected *v1.Relationship
	}{
		{
			args:     []string{"res:123", "rel", "sub:1234"},
			expected: &v1.Relationship{
				Resource:       &v1.ObjectReference{
					ObjectType: "res",
					ObjectId:   "123",
				},
				Relation:       "rel",
				Subject:        &v1.SubjectReference{
					Object:           &v1.ObjectReference{
						ObjectType: "sub",
						ObjectId: "1234",
					},
				},
			},
		},
		{
			args:     []string{"res:123", "rel", "sub:1234#rel"},
			expected: &v1.Relationship{
				Resource:       &v1.ObjectReference{
					ObjectType: "res",
					ObjectId:   "123",
				},
				Relation:       "rel",
				Subject:        &v1.SubjectReference{
					Object:           &v1.ObjectReference{
						ObjectType: "sub",
						ObjectId: "1234",
					},
					OptionalRelation: "rel",
				},
			},
		},
		{
			args:     []string{"res:123", "rel", `sub:1234#rel[only_certain_days:{"allowed_days":["friday","saturday"]}]`},
			expected: &v1.Relationship{
				Resource:       &v1.ObjectReference{
					ObjectType: "res",
					ObjectId:   "123",
				},
				Relation:       "rel",
				Subject:        &v1.SubjectReference{
					Object:           &v1.ObjectReference{
						ObjectType: "sub",
						ObjectId: "1234",
					},
					OptionalRelation: "rel",
				},
				OptionalCaveat: &v1.ContextualizedCaveat{
					CaveatName: "only_certain_days",
					Context:    &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"allowed_days": structpb.NewListValue(&structpb.ListValue{
								Values: []*structpb.Value{
									structpb.NewStringValue("friday"),
									structpb.NewStringValue("saturday"),
								},
							}),
						},
					},
				},
			},
		},
	}{
		tt := tt
		t.Run(strings.Join(tt.args, " "), func (t *testing.T) {
			rel, err := argsToRelationship(tt.args)
			require.NoError(t, err)
			t.Log(rel)
			require.True(t, proto.Equal(rel, tt.expected))
		})
	}
}
