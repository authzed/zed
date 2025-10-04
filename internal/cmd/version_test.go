package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/authzed/authzed-go/pkg/responsemeta"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

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
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.BoolFlag{FlagName: "include-deps", FlagValue: tt.includeDeps})

			result := getClientVersion(cmd)
			t.Log(result)
			require.Contains(t, result, tt.expectStr)
		})
	}
}

func TestGetServerVersion(t *testing.T) {
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
