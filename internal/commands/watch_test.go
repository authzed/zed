package commands

import (
	"context"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
	zedtesting "github.com/authzed/zed/internal/testing"
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

type mockWatchClient struct {
	client.Client
	grpc.ServerStreamingClient[v1.WatchResponse]
	callCounter int
}

var _ v1.WatchServiceClient = (*mockWatchClient)(nil)

func (m *mockWatchClient) Recv() (*v1.WatchResponse, error) {
	update1 := &v1.RelationshipUpdate{
		Operation: v1.RelationshipUpdate_OPERATION_CREATE,
		Relationship: &v1.Relationship{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   "object1",
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "alice",
				},
			},
		},
	}
	update2 := &v1.RelationshipUpdate{
		Operation: v1.RelationshipUpdate_OPERATION_CREATE,
		Relationship: &v1.Relationship{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   "object2",
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "alice",
				},
			},
		},
	}

	response1 := &v1.WatchResponse{
		Updates:        []*v1.RelationshipUpdate{update1},
		ChangesThrough: &v1.ZedToken{Token: "revision1"},
	}
	response2 := &v1.WatchResponse{
		Updates:        []*v1.RelationshipUpdate{update2},
		ChangesThrough: &v1.ZedToken{Token: "revision2"},
	}

	switch m.callCounter {
	case 0:
		m.callCounter++
		return response1, nil
	case 1:
		m.callCounter++
		return nil, status.Error(codes.Unavailable, "simulated error")
	case 2:
		m.callCounter++
		return response2, nil
	default:
		return nil, io.EOF
	}
}

func (m *mockWatchClient) Watch(_ context.Context, _ *v1.WatchRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[v1.WatchResponse], error) {
	return m, nil
}

func TestWatchCmdFunc(t *testing.T) {
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)

	watchErr := make(chan error, 1)

	receivedResponses := make([]*v1.WatchResponse, 0)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		watchErr <- watchCmdFuncImpl(cmd, &mockWatchClient{}, func(resp *v1.WatchResponse) {
			receivedResponses = append(receivedResponses, resp)
		})
	}()

	time.Sleep(1 * time.Second)

	cancel()

	wg.Wait()

	err := <-watchErr
	require.ErrorIs(t, err, io.EOF)

	require.Len(t, receivedResponses, 2)
	require.Equal(t, "object1", receivedResponses[0].Updates[0].Relationship.Resource.ObjectId)
	require.Equal(t, `token:"revision1"`, receivedResponses[0].ChangesThrough.String())
	require.Equal(t, "object2", receivedResponses[1].Updates[0].Relationship.Resource.ObjectId)
	require.Equal(t, `token:"revision2"`, receivedResponses[1].ChangesThrough.String())
}
