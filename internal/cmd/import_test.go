package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
	zedtesting "github.com/authzed/zed/internal/testing"
)

var fullyConsistent = &v1.Consistency{Requirement: &v1.Consistency_FullyConsistent{FullyConsistent: true}}

func TestImportCmdHappyPath(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "schema-definition-prefix"},
		zedtesting.BoolFlag{FlagName: "schema", FlagValue: true},
		zedtesting.BoolFlag{FlagName: "relationships", FlagValue: true},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("import-test", "happy-path-validation-file.yaml")

	// Set up client
	ctx := t.Context()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		require.NoError(srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(err)

	originalClient := client.NewClient
	defer func() {
		client.NewClient = originalClient
	}()

	client.NewClient = zedtesting.ClientFromConn(conn)

	c, err := zedtesting.ClientFromConn(conn)(cmd)
	require.NoError(err)

	// Run the import and assert we don't have errors
	err = importCmdFunc(cmd, []string{f})
	require.NoError(err)

	// Run a check with full consistency to see whether the relationships
	// and schema are written
	resp, err := c.CheckPermission(ctx, &v1.CheckPermissionRequest{
		Consistency: fullyConsistent,
		Subject:     &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "1"}},
		Permission:  "view",
		Resource:    &v1.ObjectReference{ObjectType: "resource", ObjectId: "1"},
	})
	require.NoError(err)
	require.Equal(resp.Permissionship, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION)
}
