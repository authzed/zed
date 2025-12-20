package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/authzed/authzed-go/pkg/responsemeta"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestGetClientVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		includeDeps bool
		expectStr   string
	}{
		{
			name:        "without dependencies",
			includeDeps: false,
			expectStr:   "zed (devel)",
		},
		{
			name:        "with dependencies",
			includeDeps: true,
			expectStr:   "github.com/authzed/zed/internal/cmd.test (devel)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.BoolFlag{FlagName: "include-deps", FlagValue: tt.includeDeps})

			result := getClientVersion(cmd)
			t.Log(result)
			require.Contains(t, result, tt.expectStr)
		})
	}
}

func TestGetServerVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		serverVersionInHeader  string
		serverVersionInTrailer string
		expectVersion          string
		expectErr              bool
	}{
		{
			name:                  "valid server version in header",
			serverVersionInHeader: "v1.2.3",
			expectVersion:         "v1.2.3",
			expectErr:             false,
		},
		{
			name:                   "valid server version in trailer",
			serverVersionInTrailer: "v1.2.3",
			expectVersion:          "v1.2.3",
			expectErr:              false,
		},
		{
			name:          "empty server version in header and trailer",
			expectVersion: "(unknown)",
			expectErr:     false,
		},
		{
			name:                   "inconsistent server versions in header and trailer",
			serverVersionInHeader:  "v1.0.0",
			serverVersionInTrailer: "v1.0.1",
			expectErr:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t)

			mockClient := &mockSchemaServiceClient{
				serverVersionInHeader:  tt.serverVersionInHeader,
				serverVersionInTrailer: tt.serverVersionInTrailer,
			}

			result, err := getServerVersion(cmd, mockClient)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectVersion, result)
			}
		})
	}
}

type mockSchemaServiceClient struct {
	v1.SchemaServiceClient
	serverVersionInHeader  string
	serverVersionInTrailer string
}

var _ v1.SchemaServiceClient = (*mockSchemaServiceClient)(nil)

func (m *mockSchemaServiceClient) ReadSchema(_ context.Context, _ *v1.ReadSchemaRequest, opts ...grpc.CallOption) (*v1.ReadSchemaResponse, error) {
	for _, opt := range opts {
		if headerOpt, ok := opt.(grpc.HeaderCallOption); ok {
			if m.serverVersionInHeader != "" {
				*headerOpt.HeaderAddr = metadata.MD{
					string(responsemeta.ServerVersion): []string{m.serverVersionInHeader},
				}
			} else {
				*headerOpt.HeaderAddr = metadata.MD{}
			}
		}
		if trailerOpt, ok := opt.(grpc.TrailerCallOption); ok {
			if m.serverVersionInTrailer != "" {
				*trailerOpt.TrailerAddr = metadata.MD{
					string(responsemeta.ServerVersion): []string{m.serverVersionInTrailer},
				}
			} else {
				*trailerOpt.TrailerAddr = metadata.MD{}
			}
		}
	}
	return &v1.ReadSchemaResponse{}, nil
}

func (m *mockSchemaServiceClient) WriteSchema(_ context.Context, _ *v1.WriteSchemaRequest, _ ...grpc.CallOption) (*v1.WriteSchemaResponse, error) {
	return nil, nil
}

func TestVersionCommandWithUnavailableServer(t *testing.T) {
	// Note: Not running in parallel because we modify globals (client.NewClient, os.Stdout, os.Stderr)

	// This test verifies issue #554 is fixed: when SpiceDB is unavailable,
	// zed version should not produce noisy retry error logs

	// Save and restore original client.NewClient
	originalNewClient := client.NewClient
	t.Cleanup(func() {
		client.NewClient = originalNewClient
	})

	// Mock client.NewClient to return Unavailable error
	client.NewClient = func(_ *cobra.Command) (client.Client, error) {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}

	// Capture stdout to check output
	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	// Capture stderr to verify no retry errors are logged
	oldStderr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	// Create command with all required flags
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "include-remote-version", FlagValue: true},
		zedtesting.BoolFlag{FlagName: "include-deps", FlagValue: false},
		zedtesting.UintFlag{FlagName: "max-retries", FlagValue: 10},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: ""},
		zedtesting.StringFlag{FlagName: "token", FlagValue: ""},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: ""},
		zedtesting.StringFlag{FlagName: "hostname-override", FlagValue: ""},
		zedtesting.StringFlag{FlagName: "permissions-system", FlagValue: ""},
		zedtesting.StringFlag{FlagName: "proxy", FlagValue: ""},
		zedtesting.StringFlag{FlagName: "request-id", FlagValue: ""},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: false},
		zedtesting.BoolFlag{FlagName: "no-verify-ca", FlagValue: false},
		zedtesting.BoolFlag{FlagName: "skip-version-check", FlagValue: false},
		zedtesting.IntFlag{FlagName: "max-message-size", FlagValue: 0},
	)

	// Run the version command
	err := versionCmdFunc(cmd, nil)

	// Close writers and read captured outputs
	wOut.Close()
	wErr.Close()
	var stdout bytes.Buffer
	_, _ = stdout.ReadFrom(rOut)
	var stderr bytes.Buffer
	_, _ = stderr.ReadFrom(rErr)

	// Command should succeed despite unavailable server
	require.NoError(t, err)

	// Stdout should contain client version and service: (unknown)
	output := stdout.String()
	require.Contains(t, output, "zed")
	require.Contains(t, output, "service:")
	require.Contains(t, output, "(unknown)")

	// Stderr should be empty (no retry error logs)
	stderrOutput := stderr.String()
	require.NotContains(t, stderrOutput, "retrying gRPC call")
	require.NotContains(t, stderrOutput, "Unavailable")
}
