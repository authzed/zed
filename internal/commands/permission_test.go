package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/spiceerrors"
	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
	zedtesting "github.com/authzed/zed/internal/testing"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

type mockCheckClient struct {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
	v1.ExperimentalServiceClient

	t              *testing.T
	validProtoText bool
}

func (m *mockCheckClient) CheckPermission(_ context.Context, _ *v1.CheckPermissionRequest, _ ...grpc.CallOption) (*v1.CheckPermissionResponse, error) {
	debugInfo := &v1.DebugInformation{}
	protoText := debugInfo.String()
	if !m.validProtoText {
		protoText = "invalid"
	}

	err := spiceerrors.WithCodeAndDetailsAsError(fmt.Errorf("test"), codes.ResourceExhausted, &errdetails.ErrorInfo{
		Reason: v1.ErrorReason_name[int32(v1.ErrorReason_ERROR_REASON_MAXIMUM_DEPTH_EXCEEDED)],
		Domain: "test",
		Metadata: map[string]string{
			"debug_trace_proto_text": protoText,
		},
	})
	return &v1.CheckPermissionResponse{}, err
}

func TestCheckErrorWithDebugInformation(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockCheckClient{t: t, validProtoText: true}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	cmd := &cobra.Command{}
	cmd.Flags().String("revision", "", "optional revision at which to check")
	_ = cmd.Flags().MarkHidden("revision")
	cmd.Flags().Bool("explain", false, "requests debug information from SpiceDB and prints out a trace of the requests")
	cmd.Flags().Bool("schema", false, "requests debug information from SpiceDB and prints out the schema used")
	cmd.Flags().Bool("error-on-no-permission", false, "if true, zed will return exit code 1 if subject does not have unconditional permission")
	cmd.Flags().String("caveat-context", "", "the caveat context to send along with the check, in JSON form")
	registerConsistencyFlags(cmd.Flags())

	err := checkCmdFunc(cmd, []string{"object:1", "perm", "object:2"})
	require.NotNil(t, err)
	require.ErrorContains(t, err, "test")
}

func TestCheckErrorWithInvalidDebugInformation(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockCheckClient{t: t, validProtoText: false}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	cmd := &cobra.Command{}
	cmd.Flags().String("revision", "", "optional revision at which to check")
	_ = cmd.Flags().MarkHidden("revision")
	cmd.Flags().Bool("explain", false, "requests debug information from SpiceDB and prints out a trace of the requests")
	cmd.Flags().Bool("schema", false, "requests debug information from SpiceDB and prints out the schema used")
	cmd.Flags().Bool("error-on-no-permission", false, "if true, zed will return exit code 1 if subject does not have unconditional permission")
	cmd.Flags().String("caveat-context", "", "the caveat context to send along with the check, in JSON form")
	registerConsistencyFlags(cmd.Flags())

	err := checkCmdFunc(cmd, []string{"object:1", "perm", "object:2"})
	require.NotNil(t, err)
	require.ErrorContains(t, err, "unknown field: invalid")
}

func TestLookupResourcesCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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

	c, err := zedtesting.ClientFromConn(conn)(nil)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	var updates []*v1.RelationshipUpdate
	for i := 0; i < 10; i++ {
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: tuple.MustParseV1Rel(fmt.Sprintf("test/resource:%d#reader@test/user:1", i)),
		})
	}

	_, err = c.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{Updates: updates})
	require.NoError(t, err)

	// we override this to obtain the results being printed and validate them
	previous := console.Println
	defer func() {
		console.Println = previous
	}()
	var count int
	console.Println = func(values ...any) {
		count += len(values)
	}

	// use test callback to make sure pagination is correct
	var receivedPageSizes []uint
	newLookupResourcesPageCallbackForTests = func(readPageSize uint) {
		receivedPageSizes = append(receivedPageSizes, readPageSize)
	}
	defer func() {
		newLookupResourcesPageCallbackForTests = nil
	}()

	// test no page size, server computes returns all resources in one go
	cmd := testLookupResourcesCommand(t, 0)
	err = lookupResourcesCmdFunc(cmd, []string{"test/resource", "read", "test/user:1"})
	require.NoError(t, err)
	require.Equal(t, 10, count)
	require.EqualValues(t, []uint{10}, receivedPageSizes)

	// use page size same as number of elements
	count = 0
	receivedPageSizes = nil
	cmd = testLookupResourcesCommand(t, 10)
	err = lookupResourcesCmdFunc(cmd, []string{"test/resource", "read", "test/user:1"})
	require.NoError(t, err)
	require.Equal(t, 10, count)
	require.EqualValues(t, []uint{10, 0}, receivedPageSizes)

	// use odd page size
	count = 0
	receivedPageSizes = nil
	cmd = testLookupResourcesCommand(t, 3)
	err = lookupResourcesCmdFunc(cmd, []string{"test/resource", "read", "test/user:1"})
	require.NoError(t, err)
	// page-limit is a global maximum number of results to print, not a per-request page size
	require.Equal(t, 3, count)
	// callback receives the number of streamed results per request; the first request
	// reads 4 items but only 3 are printed due to the global limit, the second reads 1 and exits
	require.EqualValues(t, []uint{3, 1}, receivedPageSizes)
}

func TestLookupResourcesPageLimitStopsStream(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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

	c, err := zedtesting.ClientFromConn(conn)(nil)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	// Create 15 relationships so we have more data than our page limit
	var updates []*v1.RelationshipUpdate
	for i := 0; i < 15; i++ {
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: tuple.MustParseV1Rel(fmt.Sprintf("test/resource:%d#reader@test/user:1", i)),
		})
	}

	_, err = c.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{Updates: updates})
	require.NoError(t, err)

	// Override console.Println to count printed results
	previous := console.Println
	defer func() {
		console.Println = previous
	}()
	var count int
	console.Println = func(values ...any) {
		count += len(values)
	}

	// Test case 1: Page limit of 5 should stop after 5 results, even though 15 are available
	count = 0
	cmd := testLookupResourcesCommand(t, 5)
	err = lookupResourcesCmdFunc(cmd, []string{"test/resource", "read", "test/user:1"})
	require.NoError(t, err)
	require.Equal(t, 5, count, "Should stop at page limit of 5, but got %d results", count)

	// Test case 2: Page limit of 7 should stop after 7 results
	count = 0
	cmd = testLookupResourcesCommand(t, 7)
	err = lookupResourcesCmdFunc(cmd, []string{"test/resource", "read", "test/user:1"})
	require.NoError(t, err)
	require.Equal(t, 7, count, "Should stop at page limit of 7, but got %d results", count)

	// Test case 3: Page limit of 20 (more than available) should return all 15 results
	count = 0
	cmd = testLookupResourcesCommand(t, 20)
	err = lookupResourcesCmdFunc(cmd, []string{"test/resource", "read", "test/user:1"})
	require.NoError(t, err)
	require.Equal(t, 15, count, "Should return all 15 results when page limit (20) is higher than available data")
}

func testLookupResourcesCommand(t *testing.T, limit uint32) *cobra.Command {
	return zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "consistency-full", FlagValue: true},
		zedtesting.StringFlag{FlagName: "consistency-at-least"},
		zedtesting.BoolFlag{FlagName: "consistency-min-latency", FlagValue: false},
		zedtesting.StringFlag{FlagName: "consistency-at-exactly"},
		zedtesting.StringFlag{FlagName: "revision"},
		zedtesting.StringFlag{FlagName: "caveat-context"},
		zedtesting.UintFlag32{FlagName: "page-limit", FlagValue: limit},
		zedtesting.BoolFlag{FlagName: "json"},
		zedtesting.StringFlag{FlagName: "cursor"},
		zedtesting.BoolFlag{FlagName: "show-cursor", FlagValue: false})
}
