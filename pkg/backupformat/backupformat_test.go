package backupformat

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func TestWriteAndRead(t *testing.T) {
	simpleRel := &v1.Relationship{
		Resource: &v1.ObjectReference{
			ObjectType: gofakeit.Noun(),
			ObjectId:   gofakeit.UUID(),
		},
		Relation: gofakeit.Noun(),
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: gofakeit.Noun(),
				ObjectId:   gofakeit.FirstName(),
			},
		},
	}

	relWithCaveatName := simpleRel.CloneVT()
	relWithCaveatName.OptionalCaveat = &v1.ContextualizedCaveat{
		CaveatName: gofakeit.Noun(),
	}

	relWithSimpleContext := relWithCaveatName.CloneVT()
	flatContext, err := structpb.NewStruct(map[string]any{
		"nullVal":  nil,
		"intVal":   123,
		"floatVal": 123.45,
		"boolVal":  true,
	})
	require.NoError(t, err)
	relWithSimpleContext.OptionalCaveat.Context = flatContext

	relWithNestedContext := relWithCaveatName.CloneVT()
	nestedContext, err := structpb.NewStruct(map[string]any{
		"obj1": map[string]any{
			"obj2": map[string]any{
				"obj3": gofakeit.Noun(),
			},
		},
	})
	require.NoError(t, err)
	relWithNestedContext.OptionalCaveat.Context = nestedContext

	testCases := []struct {
		name                   string
		schemaSize             int
		numRandomRelationships int
		extraRelationships     []*v1.Relationship
	}{
		{"base", 1, 1, nil},
		{"big", 50, 1000, nil},
		{"caveats", 1, 0, []*v1.Relationship{
			relWithCaveatName,
			relWithSimpleContext,
			relWithNestedContext,
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			fields := make([]gofakeit.Field, tc.schemaSize)
			for i := range fields {
				fields[i].Name = gofakeit.Noun()
				fields[i].Function = "noun"
			}
			schemaBytes, err := gofakeit.JSON(&gofakeit.JSONOptions{
				Type:   "object",
				Fields: fields,
			})
			require.NoError(err)

			expectedSchema := string(schemaBytes)
			expectedZedtoken := base64.StdEncoding.EncodeToString(gofakeit.ImageJpeg(10, 10))

			expectedRels := make([]*v1.Relationship, 0, tc.numRandomRelationships+len(tc.extraRelationships))
			expectedRels = append(expectedRels, tc.extraRelationships...)

			for i := 0; i < tc.numRandomRelationships; i++ {
				expectedRels = append(expectedRels, &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: gofakeit.Noun(),
						ObjectId:   gofakeit.UUID(),
					},
					Relation: gofakeit.Noun(),
					Subject: &v1.SubjectReference{
						Object: &v1.ObjectReference{
							ObjectType: gofakeit.Noun(),
							ObjectId:   gofakeit.FirstName(),
						},
					},
				})
			}

			buf := bytes.Buffer{}
			enc, err := NewEncoder(&buf, expectedSchema, &v1.ZedToken{
				Token: expectedZedtoken,
			})
			require.NoError(err)

			for _, rel := range expectedRels {
				require.NoError(enc.Append(rel, ""))
			}
			require.NoError(enc.Close())
			require.NotEmpty(buf.Bytes())

			dec, err := NewDecoder(bytes.NewReader(buf.Bytes()))
			require.NoError(err)

			require.Equal(expectedSchema, dec.Schema())
			require.Equal(expectedZedtoken, dec.ZedToken().Token)

			for _, expected := range expectedRels {
				rel, err := dec.Next()
				require.NoError(err)
				requireRelationshipEqual(require, expected, rel)
			}

			require.NoError(dec.Close())
		})
	}
}

func requireRelationshipEqual(require *require.Assertions, expected, received *v1.Relationship) {
	require.Equal(expected.Resource.ObjectType, received.Resource.ObjectType)
	require.Equal(expected.Resource.ObjectId, received.Resource.ObjectId)
	require.Equal(expected.Relation, received.Relation)
	require.Equal(expected.Subject.Object.ObjectType, received.Subject.Object.ObjectType)
	require.Equal(expected.Subject.Object.ObjectId, received.Subject.Object.ObjectId)
	require.Equal(expected.Subject.OptionalRelation, received.Subject.OptionalRelation)

	if expected.OptionalCaveat == nil {
		require.Nil(received.OptionalCaveat)
	} else {
		require.Equal(expected.OptionalCaveat.CaveatName, received.OptionalCaveat.CaveatName)
		require.Equal(expected.OptionalCaveat.Context.AsMap(), received.OptionalCaveat.Context.AsMap())
	}
}
