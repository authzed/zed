package commands

import (
	"reflect"
	"testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func TestParseRelationshipFilter(t *testing.T) {
	tcs := []struct {
		input    string
		expected *v1.RelationshipFilter
	}{
		{
			input: "resourceType:resourceId",
			expected: &v1.RelationshipFilter{
				ResourceType:       "resourceType",
				OptionalResourceId: "resourceId",
			},
		},
		{
			input: "resourceType:resourceId%",
			expected: &v1.RelationshipFilter{
				ResourceType:             "resourceType",
				OptionalResourceIdPrefix: "resourceId",
			},
		},
		{
			input: "resourceType:resourceId#relation",
			expected: &v1.RelationshipFilter{
				ResourceType:       "resourceType",
				OptionalResourceId: "resourceId",
				OptionalRelation:   "relation",
			},
		},
		{
			input: "resourceType:resourceId#relation@subjectType:subjectId",
			expected: &v1.RelationshipFilter{
				ResourceType:       "resourceType",
				OptionalResourceId: "resourceId",
				OptionalRelation:   "relation",
				OptionalSubjectFilter: &v1.SubjectFilter{
					SubjectType:       "subjectType",
					OptionalSubjectId: "subjectId",
				},
			},
		},
		{
			input: "#relation",
			expected: &v1.RelationshipFilter{
				OptionalRelation: "relation",
			},
		},
		{
			input: "resourceType#relation",
			expected: &v1.RelationshipFilter{
				ResourceType:     "resourceType",
				OptionalRelation: "relation",
			},
		},
		{
			input: ":resourceId#relation",
			expected: &v1.RelationshipFilter{
				OptionalResourceId: "resourceId",
				OptionalRelation:   "relation",
			},
		},
		{
			input: ":resourceId%#relation",
			expected: &v1.RelationshipFilter{
				OptionalResourceIdPrefix: "resourceId",
				OptionalRelation:         "relation",
			},
		},
		{
			input: "resourceType:resourceId#relation@subjectType:subjectId#somerel",
			expected: &v1.RelationshipFilter{
				ResourceType:       "resourceType",
				OptionalResourceId: "resourceId",
				OptionalRelation:   "relation",
				OptionalSubjectFilter: &v1.SubjectFilter{
					SubjectType:       "subjectType",
					OptionalSubjectId: "subjectId",
					OptionalRelation:  &v1.SubjectFilter_RelationFilter{Relation: "somerel"},
				},
			},
		},
		{
			input: "@subjectType:subjectId#somerel",
			expected: &v1.RelationshipFilter{
				OptionalSubjectFilter: &v1.SubjectFilter{
					SubjectType:       "subjectType",
					OptionalSubjectId: "subjectId",
					OptionalRelation:  &v1.SubjectFilter_RelationFilter{Relation: "somerel"},
				},
			},
		},
	}

	for _, tc := range tcs {
		actual, err := parseRelationshipFilter(tc.input)
		if err != nil {
			t.Errorf("parseRelationshipFilter(%s) returned error: %v", tc.input, err)
		}
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("parseRelationshipFilter(%s) = %v, expected %v", tc.input, actual, tc.expected)
		}
	}
}
