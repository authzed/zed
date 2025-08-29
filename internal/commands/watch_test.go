package commands

import (
	"context"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

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

type mockWatchServer struct {
	v1.UnimplementedWatchServiceServer
	sendOnce uint
}

func (mws *mockWatchServer) Watch(_ *v1.WatchRequest, stream grpc.ServerStreamingServer[v1.WatchResponse]) error {
	update := &v1.RelationshipUpdate{
		Operation: v1.RelationshipUpdate_OPERATION_CREATE,
		Relationship: &v1.Relationship{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   "1",
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

	response := &v1.WatchResponse{
		Updates:        []*v1.RelationshipUpdate{update},
		ChangesThrough: &v1.ZedToken{Token: "revision1"},
	}

	if mws.sendOnce == 0 {
		mws.sendOnce++
		return stream.Send(response)
	}

	return nil
}

func TestWatchCmdFunc(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()

	mockServer := &mockWatchServer{}
	v1.RegisterWatchServiceServer(s, mockServer)

	go func() {
		_ = s.Serve(lis)
	}()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithInsecure(), // nolint:staticcheck
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client.NewClient = zedtesting.ClientFromConn(conn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "log-level", FlagValue: "trace", Changed: true},
	)
	cmd.SetContext(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	watchErr := make(chan error, 1)

	go func() {
		defer wg.Done()
		watchErr <- watchCmdFunc(cmd, []string{})
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()

	wg.Wait()

	err = <-watchErr
	require.ErrorContains(t, err, "EOF")
}
