package commands

import (
	"os"
	"strings"
	"testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/mattn/go-tty"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
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
		args     []string
		expected *v1.Relationship
	}{
		{
			args: []string{"res:123", "rel", "sub:1234"},
			expected: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "res",
					ObjectId:   "123",
				},
				Relation: "rel",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "sub",
						ObjectId:   "1234",
					},
				},
			},
		},
		{
			args: []string{"res:123", "rel", "sub:1234#rel"},
			expected: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "res",
					ObjectId:   "123",
				},
				Relation: "rel",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "sub",
						ObjectId:   "1234",
					},
					OptionalRelation: "rel",
				},
			},
		},
		{
			args: []string{"res:123", "rel", `sub:1234#rel[only_certain_days:{"allowed_days":["friday", "saturday"]}]`},
			expected: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "res",
					ObjectId:   "123",
				},
				Relation: "rel",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "sub",
						ObjectId:   "1234",
					},
					OptionalRelation: "rel",
				},
				OptionalCaveat: &v1.ContextualizedCaveat{
					CaveatName: "only_certain_days",
					Context: &structpb.Struct{
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
	} {
		tt := tt
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			rel, err := argsToRelationship(tt.args)
			require.NoError(t, err)
			t.Log(rel)
			require.True(t, proto.Equal(rel, tt.expected))
		})
	}
}

func TestParseRelationshipLine(t *testing.T) {
	for _, tt := range []struct {
		input    string
		expected []string
	}{
		{
			input:    "res:1 foo sub:1",
			expected: []string{"res:1", "foo", "sub:1"},
		},
		{
			input:    "res:1      foo	sub:1",
			expected: []string{"res:1", "foo", "sub:1"},
		},
		{
			input:    `res:1 foo sub:1[only_certain_days:{"allowed_days": ["friday", "saturday",    "sunday"]}]`,
			expected: []string{"res:1", "foo", `sub:1[only_certain_days:{"allowed_days": ["friday", "saturday",    "sunday"]}]`},
		},
		{
			input:    `res:1 foo sub:1[auth_politely:{"nice_phrases": ["how are you?", "	it's good to see you!"]}]`,
			expected: []string{"res:1", "foo", `sub:1[auth_politely:{"nice_phrases": ["how are you?", "	it's good to see you!"]}]`},
		},
	} {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseRelationshipLine(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteRelationshipsArgs(t *testing.T) {
	// simulate terminal
	f, err := os.CreateTemp("", "spicedb-")
	require.NoError(t, err)

	// returns accepts anything if input file is not a terminal
	require.Nil(t, writeRelationshipsArgs(&cobra.Command{}, nil, f))

	// does not accept both file input and arguments
	require.ErrorContains(t, writeRelationshipsArgs(&cobra.Command{}, []string{"a", "b"}, f), "cannot provide input both via arguments and Stdin")

	// checks there is 3 input arguments in case of tty
	testTTY, err := tty.Open()
	require.NoError(t, err)
	defer require.NoError(t, testTTY.Close())

	require.True(t, term.IsTerminal(int(testTTY.Input().Fd())))
	require.ErrorContains(t, writeRelationshipsArgs(&cobra.Command{}, nil, testTTY.Input()), "accepts 3 arg(s), received 0")
	require.Nil(t, writeRelationshipsArgs(&cobra.Command{}, []string{"a", "b", "c"}, testTTY.Input()))
}
