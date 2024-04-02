package commands

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/authzed/zed/internal/client"
	zedtesting "github.com/authzed/zed/internal/testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const testSchema = `definition test/resource {
	relation reader: test/user
	relation writer: test/user
}

definition test/user {}`

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

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
			`res:123#rel@resource:1234[caveat_name:{"num":1234}]`,
			`res:123 rel resource:1234[caveat_name:{"num":1234}]`,
		},
		{
			`res:123#rel@resource:1234[caveat_name:{"name":"##@@##@@"}]`,
			`res:123 rel resource:1234[caveat_name:{"name":"##@@##@@"}]`,
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
			require.True(t, proto.Equal(rel, tt.expected))
		})
	}
}

func TestParseRelationshipLine(t *testing.T) {
	for _, tt := range []struct {
		input    string
		expected []string
		err      string
	}{
		{
			input: "   ",
			err:   "to have 3 arguments, but got 0",
		},
		{
			input: "res:1 ",
			err:   "to have 3 arguments, but got 1",
		},
		{
			input: "res:1 foo",
			err:   "to have 3 arguments, but got 2",
		},
		{
			input: "res:1 foo ",
			err:   "to have 3 arguments, but got 2",
		},
		{
			input: "res:1 foo ",
			err:   "to have 3 arguments, but got 2",
		},
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
			resource, relation, subject, err := parseRelationshipLine(tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, []string{resource, relation, subject})
		})
	}
}

func TestWriteRelationshipsArgs(t *testing.T) {
	f, err := os.CreateTemp("", "spicedb-")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})

	isTerm := false
	originalFunc := isFileTerminal
	isFileTerminal = func(_ *os.File) bool {
		return isTerm
	}
	defer func() {
		isFileTerminal = originalFunc
	}()

	// returns accepts anything if input file is not a terminal
	require.Nil(t, StdinOrExactArgs(3)(&cobra.Command{}, nil))

	// if both STDIN and CLI args are provided, CLI args take precedence
	require.ErrorContains(t, StdinOrExactArgs(3)(&cobra.Command{}, []string{"a", "b"}), "accepts 3 arg(s), received 2")

	isTerm = true
	// checks there is 3 input arguments in case of tty
	require.ErrorContains(t, StdinOrExactArgs(3)(&cobra.Command{}, nil), "accepts 3 arg(s), received 0")
	require.Nil(t, StdinOrExactArgs(3)(&cobra.Command{}, []string{"a", "b", "c"}))
}

func TestWriteRelationshipCmdFuncFromTTY(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockClient{t: t, expectedWrites: []*v1.WriteRelationshipsRequest{{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
					Relationship: tuple.ParseRel(`resource:1#view@user:1[cav:{"letters": ["a", "b", "c"]}]`),
				},
			},
		}}}, nil
	}

	originalFunc := isFileTerminal
	isFileTerminal = func(_ *os.File) bool {
		return true
	}
	defer func() {
		isFileTerminal = originalFunc
	}()

	tty, err := os.CreateTemp("", "spicedb-")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.Remove(tty.Name())
	})

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	f := writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH, tty)
	cmd := &cobra.Command{}
	cmd.Flags().Int("batch-size", 100, "")
	cmd.Flags().Bool("json", true, "")
	cmd.Flags().String("caveat", `cav:{"letters": ["a", "b", "c"]}`, "")

	err = f(cmd, []string{"resource:1", "view", "user:1"})
	require.NoError(t, err)
}

func TestWriteRelationshipCmdFuncArgsTakePrecedence(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockClient{t: t, expectedWrites: []*v1.WriteRelationshipsRequest{{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
					Relationship: tuple.ParseRel("resource:1#viewer@user:1"),
				},
			},
		}}}, nil
	}

	originalFunc := isFileTerminal
	isFileTerminal = func(_ *os.File) bool {
		return false
	}
	defer func() {
		isFileTerminal = originalFunc
	}()

	fi := fileFromStrings(t, []string{
		"resource:1 viewer user:3",
	})
	defer func() {
		require.NoError(t, fi.Close())
	}()
	t.Cleanup(func() {
		_ = os.Remove(fi.Name())
	})

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	f := writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH, fi)
	cmd := &cobra.Command{}
	cmd.Flags().Int("batch-size", 100, "")
	cmd.Flags().Bool("json", true, "")
	cmd.Flags().String("caveat", "", "")

	err := f(cmd, []string{"resource:1", "viewer", "user:1"})
	require.NoError(t, err)
}

func TestWriteRelationshipCmdFuncFromStdin(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockClient{t: t, expectedWrites: []*v1.WriteRelationshipsRequest{{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
					Relationship: tuple.ParseRel("resource:1#viewer@user:1"),
				},
				{
					Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
					Relationship: tuple.ParseRel("resource:1#viewer@user:2"),
				},
			},
		}}}, nil
	}

	fi := fileFromStrings(t, []string{
		"resource:1 viewer user:1",
		"resource:1 viewer user:2",
	})
	defer func() {
		require.NoError(t, fi.Close())
	}()
	t.Cleanup(func() {
		_ = os.Remove(fi.Name())
	})

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	f := writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH, fi)
	cmd := &cobra.Command{}
	cmd.Flags().Int("batch-size", 100, "")
	cmd.Flags().Bool("json", true, "")
	cmd.Flags().String("caveat", "", "")

	err := f(cmd, nil)
	require.NoError(t, err)
}

func TestWriteRelationshipCmdFuncFromStdinBatch(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockClient{t: t, expectedWrites: []*v1.WriteRelationshipsRequest{
			{
				Updates: []*v1.RelationshipUpdate{
					{
						Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
						Relationship: tuple.ParseRel(`resource:1#viewer@user:1[cav:{"letters": ["a", "b", "c"]}]`),
					},
				},
			},
			{
				Updates: []*v1.RelationshipUpdate{
					{
						Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
						Relationship: tuple.ParseRel(`resource:1#viewer@user:2[cav:{"letters": ["a", "b", "c"]}]`),
					},
				},
			},
		}}, nil
	}

	fi := fileFromStrings(t, []string{
		`resource:1 viewer user:1[cav:{"letters": ["a", "b", "c"]}]`,
		`resource:1 viewer user:2[cav:{"letters": ["a", "b", "c"]}]`,
	})
	defer func() {
		require.NoError(t, fi.Close())
	}()
	t.Cleanup(func() {
		_ = os.Remove(fi.Name())
	})

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	f := writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH, fi)
	cmd := &cobra.Command{}
	cmd.Flags().Int("batch-size", 1, "")
	cmd.Flags().Bool("json", true, "")
	cmd.Flags().String("caveat", "", "")

	err := f(cmd, nil)
	require.NoError(t, err)
}

func TestWriteRelationshipCmdFuncFromFailsWithCaveatArg(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockClient{t: t, expectedWrites: []*v1.WriteRelationshipsRequest{
			{
				Updates: []*v1.RelationshipUpdate{
					{
						Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
						Relationship: tuple.ParseRel(`resource:1#viewer@user:1[cav:{"letters": ["a", "b", "c"]}]`),
					},
				},
			},
		}}, nil
	}

	fi := fileFromStrings(t, []string{
		`resource:1 viewer user:1[cav:{"letters": ["a", "b", "c"]}]`,
	})
	defer func() {
		_ = fi.Close()
	}()
	t.Cleanup(func() {
		_ = os.Remove(fi.Name())
	})

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	f := writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH, fi)
	cmd := &cobra.Command{}
	cmd.Flags().Int("batch-size", 1, "")
	cmd.Flags().Bool("json", true, "")
	cmd.Flags().String("caveat", `cav:{"letters": ["a", "b", "c"]}`, "")

	err := f(cmd, nil)
	require.ErrorContains(t, err, "cannot specify a caveat in both the relationship and the --caveat flag")
}

func fileFromStrings(t *testing.T, strings []string) *os.File {
	t.Helper()

	fi, err := os.CreateTemp("", "spicedb-")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, fi.Close())
	}()

	for _, data := range strings {
		_, err = fi.WriteString(data + "\n")
		require.NoError(t, err)
	}
	require.NoError(t, fi.Sync())

	file, err := os.Open(fi.Name())
	require.NoError(t, err)
	return file
}

func TestBuildRelationshipsFilter(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *v1.RelationshipFilter
	}{
		{
			name:     "resource type",
			args:     []string{"res"},
			expected: &v1.RelationshipFilter{ResourceType: "res"},
		},
		{
			name:     "resource type, resource ID",
			args:     []string{"res:123"},
			expected: &v1.RelationshipFilter{ResourceType: "res", OptionalResourceId: "123"},
		},
		{
			name:     "resource type, resource ID, relation",
			args:     []string{"res:123", "view"},
			expected: &v1.RelationshipFilter{ResourceType: "res", OptionalResourceId: "123", OptionalRelation: "view"},
		},
		{
			name: "resource type, resource ID, relation, subject type",
			args: []string{"res:123", "view", "sub"},
			expected: &v1.RelationshipFilter{
				ResourceType:          "res",
				OptionalResourceId:    "123",
				OptionalRelation:      "view",
				OptionalSubjectFilter: &v1.SubjectFilter{SubjectType: "sub"},
			},
		},
		{
			name: "resource type, resource ID, relation, subject type, subject ID",
			args: []string{"res:123", "view", "sub:321"},
			expected: &v1.RelationshipFilter{
				ResourceType:          "res",
				OptionalResourceId:    "123",
				OptionalRelation:      "view",
				OptionalSubjectFilter: &v1.SubjectFilter{SubjectType: "sub", OptionalSubjectId: "321"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{}
			cmd.Flags().String("subject-filter", "", "")

			filter, err := buildRelationshipsFilter(cmd, tt.args)
			require.NoError(t, err)
			require.Equal(t, tt.expected.ResourceType, filter.ResourceType, "resource types do not match")
			require.Equal(t, tt.expected.OptionalResourceId, filter.OptionalResourceId, "resource IDs do not match")
			require.Equal(t, tt.expected.OptionalRelation, filter.OptionalRelation, "relations do not match")

			if tt.expected.OptionalSubjectFilter != nil {
				require.Equal(t, tt.expected.OptionalSubjectFilter.SubjectType, filter.OptionalSubjectFilter.SubjectType, "subject types do not match")
				require.Equal(t, tt.expected.OptionalSubjectFilter.OptionalSubjectId, filter.OptionalSubjectFilter.OptionalSubjectId, "subject IDs do not match")
			}
		})
	}
}

type mockClient struct {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
	v1.ExperimentalServiceClient

	t              *testing.T
	expectedWrites []*v1.WriteRelationshipsRequest
}

func (m *mockClient) WriteRelationships(_ context.Context, in *v1.WriteRelationshipsRequest, _ ...grpc.CallOption) (*v1.WriteRelationshipsResponse, error) {
	if len(m.expectedWrites) == 0 {
		require.Fail(m.t, "received unexpected write call")
	}
	expectedWrite := m.expectedWrites[0]
	m.expectedWrites = m.expectedWrites[1:]
	require.True(m.t, proto.Equal(expectedWrite, in))
	return &v1.WriteRelationshipsResponse{WrittenAt: &v1.ZedToken{Token: "test"}}, nil
}

func TestBulkDeleteForcing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		require.NoError(t, srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)

	originalClient := client.NewClient
	defer func() {
		client.NewClient = originalClient
	}()

	client.NewClient = zedtesting.ClientFromConn(conn)
	testCmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "subject-filter"},
		zedtesting.UintFlag{FlagName: "optional-limit", FlagValue: 1},
		zedtesting.BoolFlag{FlagName: "force", FlagValue: true})
	c, err := client.NewClient(testCmd)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	_, err = c.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel("test/resource:1#reader@test/user:1"),
			},
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel("test/resource:1#writer@test/user:2"),
			},
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel("test/resource:1#writer@test/user:3"),
			},
		},
	})
	require.NoError(t, err)

	err = bulkDeleteRelationships(testCmd, []string{"test/resource:1"})
	require.NoError(t, err)

	assertRelationshipsEmpty(ctx, t, c, &v1.RelationshipFilter{ResourceType: "test/resource"})
}

func TestBulkDeleteNotForcing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		require.NoError(t, srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)

	originalClient := client.NewClient
	defer func() {
		client.NewClient = originalClient
	}()

	client.NewClient = zedtesting.ClientFromConn(conn)
	testCmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "subject-filter"},
		zedtesting.UintFlag{FlagName: "optional-limit", FlagValue: 1},
		zedtesting.BoolFlag{FlagName: "force", FlagValue: false})
	c, err := client.NewClient(testCmd)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	_, err = c.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel("test/resource:1#reader@test/user:1"),
			},
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel("test/resource:1#writer@test/user:2"),
			},
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel("test/resource:1#writer@test/user:3"),
			},
		},
	})
	require.NoError(t, err)

	err = bulkDeleteRelationships(testCmd, []string{"test/resource:1"})
	require.ErrorContains(t, err, "could not delete test/resource")
	assertRelationshipCount(ctx, t, c, &v1.RelationshipFilter{ResourceType: "test/resource"}, 3)
}

func assertRelationshipsEmpty(ctx context.Context, t *testing.T, c client.Client, filter *v1.RelationshipFilter) {
	t.Helper()

	assertRelationshipCount(ctx, t, c, filter, 0)
}

func assertRelationshipCount(ctx context.Context, t *testing.T, c client.Client, filter *v1.RelationshipFilter, count int) {
	t.Helper()

	rrCli, err := c.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
		RelationshipFilter: filter,
	})
	require.NoError(t, err)

	relCount := 0
	for {
		_, err = rrCli.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)
		relCount++
	}

	require.NoError(t, rrCli.CloseSend())
	require.Equal(t, count, relCount)
}
