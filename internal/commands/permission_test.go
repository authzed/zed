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

	"github.com/authzed/zed/internal/client"
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
