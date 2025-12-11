package client_test

import (
	"context"
	"net"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/storage"
	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestGetTokenWithCLIOverride(t *testing.T) {
	require := require.New(t)
	testCert, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(err)
	t.Cleanup(func() {
		_ = testCert.Close()
		_ = os.Remove(testCert.Name())
	})
	_, err = testCert.Write([]byte("hi"))
	require.NoError(err)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "t1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: testCert.Name(), Changed: true},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: true},
		zedtesting.BoolFlag{FlagName: "no-verify-ca", FlagValue: true, Changed: true},
	)

	bTrue := true
	bFalse := false

	// cli args take precedence when defined
	to, err := client.GetTokenWithCLIOverride(cmd, storage.Token{})
	require.NoError(err)
	require.True(to.AnyValue())
	require.Equal("t1", to.APIToken)
	require.Equal("e1", to.Endpoint)
	require.Equal([]byte("hi"), to.CACert)
	require.Equal(&bTrue, to.Insecure)
	require.Equal(&bTrue, to.NoVerifyCA)

	// storage token takes precedence when defined
	cmd = zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "", Changed: false},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "", Changed: false},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: false},
		zedtesting.BoolFlag{FlagName: "no-verify-ca", FlagValue: true, Changed: false},
	)
	to, err = client.GetTokenWithCLIOverride(cmd, storage.Token{
		APIToken:   "t2",
		Endpoint:   "e2",
		CACert:     []byte("bye"),
		Insecure:   &bFalse,
		NoVerifyCA: &bFalse,
	})
	require.NoError(err)
	require.True(to.AnyValue())
	require.Equal("t2", to.APIToken)
	require.Equal("e2", to.Endpoint)
	require.Equal([]byte("bye"), to.CACert)
	require.Equal(&bFalse, to.Insecure)
	require.Equal(&bFalse, to.NoVerifyCA)
}

func TestGetCurrentTokenWithCLIOverrideWithoutConfigFile(t *testing.T) {
	// When we refactored the token setting logic, we broke the workflow where zed is used without a saved
	// configuration. This asserts that that workflow works.
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "t1", Changed: true},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: true},
	)

	configStore := &storage.JSONConfigStore{ConfigPath: "/not/a/valid/path"}
	secretStore := &storage.KeychainSecretStore{ConfigPath: "/not/a/valid/path"}
	token, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)

	// cli args take precedence when defined
	require.NoError(err)
	require.True(token.AnyValue())
	require.Equal("t1", token.APIToken)
	require.Equal("e1", token.Endpoint)
	require.NotNil(token.Insecure)
	require.True(*token.Insecure)
}

func TestGetCurrentTokenWithCLIOverrideWithoutSecretFile(t *testing.T) {
	// When we refactored the token setting logic, we broke the workflow where zed is used without a saved
	// context. This asserts that that workflow works.
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "t1", Changed: true},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: true},
	)

	bTrue := true

	tmpDir := t.TempDir()
	configPath := path.Join(tmpDir, "config.json")
	err := os.WriteFile(configPath, []byte("{}"), 0o600)
	require.NoError(err)

	configStore := &storage.JSONConfigStore{ConfigPath: tmpDir}
	// NOTE: we put this path in the tmpdir as well because getting the token attempts to
	// create the dir if it doesn't already exist, which will probably error.
	secretStore := &storage.KeychainSecretStore{ConfigPath: path.Join(tmpDir, "/not/a/valid/path")}
	token, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)

	// cli args take precedence when defined
	require.NoError(err)
	require.True(token.AnyValue())
	require.Equal("t1", token.APIToken)
	require.Equal("e1", token.Endpoint)
	require.Equal(&bTrue, token.Insecure)
}

type fakeServer struct {
	v1.UnimplementedSchemaServiceServer
	v1.UnimplementedExperimentalServiceServer
	v1.UnimplementedWatchServiceServer
	v1.UnimplementedPermissionsServiceServer
	testFunc func()
}

func (fss *fakeServer) ReadSchema(_ context.Context, _ *v1.ReadSchemaRequest) (*v1.ReadSchemaResponse, error) {
	fss.testFunc()
	return nil, status.Error(codes.Unavailable, "")
}

func (fss *fakeServer) ImportBulkRelationships(grpc.ClientStreamingServer[v1.ImportBulkRelationshipsRequest, v1.ImportBulkRelationshipsResponse]) error {
	fss.testFunc()
	return status.Errorf(codes.Aborted, "")
}

func (fss *fakeServer) Watch(*v1.WatchRequest, grpc.ServerStreamingServer[v1.WatchResponse]) error {
	fss.testFunc()
	return status.Errorf(codes.Unavailable, "")
}

func TestRetries(t *testing.T) {
	ctx := t.Context()
	var callCount uint
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()

	fakeServer := &fakeServer{testFunc: func() {
		callCount++
	}}
	v1.RegisterSchemaServiceServer(s, fakeServer)

	go func() {
		_ = s.Serve(lis)
	}()
	t.Cleanup(s.Stop)

	secure := true
	retries := uint(2)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "skip-version-check", FlagValue: true, Changed: true},
		zedtesting.UintFlag{FlagName: "max-retries", FlagValue: retries, Changed: true},
		zedtesting.StringFlag{FlagName: "proxy", FlagValue: "", Changed: true},
		zedtesting.StringFlag{FlagName: "hostname-override", FlagValue: "", Changed: true},
		zedtesting.IntFlag{FlagName: "max-message-size", FlagValue: 1000, Changed: true},
		zedtesting.StringSliceFlag{FlagName: "extra-header", FlagValue: []string{}, Changed: false},
	)
	dialOpts, err := client.DialOptsFromFlags(cmd, storage.Token{Insecure: &secure})
	require.NoError(t, err)

	dialOpts = append(dialOpts, grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}))

	c, err := authzed.NewClient("passthrough://bufnet", dialOpts...)
	require.NoError(t, err)

	t.Run("read_schema", func(t *testing.T) {
		_, err = c.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		grpcutil.RequireStatus(t, codes.Unavailable, err)
		require.Equal(t, retries, callCount)
	})
}

func TestDoesNotRetry(t *testing.T) {
	ctx := t.Context()
	var callCount uint
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()

	fakeServer := &fakeServer{testFunc: func() {
		callCount++
	}}
	v1.RegisterPermissionsServiceServer(s, fakeServer)
	v1.RegisterExperimentalServiceServer(s, fakeServer)
	v1.RegisterWatchServiceServer(s, fakeServer)

	go func() {
		_ = s.Serve(lis)
	}()
	t.Cleanup(s.Stop)

	secure := true
	retries := uint(2)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "skip-version-check", FlagValue: true, Changed: true},
		zedtesting.UintFlag{FlagName: "max-retries", FlagValue: retries, Changed: true},
		zedtesting.StringFlag{FlagName: "proxy", FlagValue: "", Changed: true},
		zedtesting.StringFlag{FlagName: "hostname-override", FlagValue: "", Changed: true},
		zedtesting.IntFlag{FlagName: "max-message-size", FlagValue: 1000, Changed: true},
		zedtesting.StringSliceFlag{FlagName: "extra-header", FlagValue: []string{}, Changed: false},
	)
	dialOpts, err := client.DialOptsFromFlags(cmd, storage.Token{Insecure: &secure})
	require.NoError(t, err)

	dialOpts = append(dialOpts, grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}))

	c, err := authzed.NewClientWithExperimentalAPIs("passthrough://bufnet", dialOpts...)
	require.NoError(t, err)

	t.Run("import_bulk", func(t *testing.T) {
		ibc, err := c.ImportBulkRelationships(ctx)
		require.NoError(t, err)
		err = ibc.SendMsg(&v1.ImportBulkRelationshipsRequest{})
		// TODO: this occasionally flakes with an unexpected EOF
		require.NoError(t, err)
		_, err = ibc.CloseAndRecv()
		grpcutil.RequireStatus(t, codes.Aborted, err)
		require.Equal(t, uint(1), callCount)
	})

	t.Run("watch", func(t *testing.T) {
		callCount = 0
		watchReq, err := c.Watch(ctx, &v1.WatchRequest{})
		require.NoError(t, err)
		resp, err := watchReq.Recv()
		require.Nil(t, resp)
		grpcutil.RequireStatus(t, codes.Unavailable, err)
		require.Equal(t, uint(1), callCount)
	})
}
