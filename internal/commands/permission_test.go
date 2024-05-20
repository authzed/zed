package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
	zedtesting "github.com/authzed/zed/internal/testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/stretchr/testify/require"
)

func TestLookupSubjectsCmd(t *testing.T) {
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.BoolFlag{FlagName: "json"},
		zedtesting.StringFlag{FlagName: "caveat-context"},
		zedtesting.BoolFlag{FlagName: "consistency-full", FlagValue: true},
		zedtesting.StringFlag{FlagName: "consistency-at-least"},
		zedtesting.StringFlag{FlagName: "revision"},
		zedtesting.StringFlag{FlagName: "consistency-at-exactly"},
		zedtesting.UintFlag32{FlagName: "page-limit", FlagValue: 1})

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

	c, err := zedtesting.ClientFromConn(conn)(cmd)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	var updates []*v1.RelationshipUpdate
	for i := 0; i < 1000; i++ {
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: tuple.ParseRel(fmt.Sprintf("test/resource:1#reader@test/user:%d", i)),
		})
	}
	wr := &v1.WriteRelationshipsRequest{
		Updates: updates,
	}
	_, err = c.WriteRelationships(ctx, wr)
	require.NoError(t, err)

	originalFunc := console.Printf
	var results []string
	console.Printf = func(format string, a ...interface{}) {
		results = append(results, fmt.Sprintf(format, a...))
	}
	defer func() {
		console.Printf = originalFunc
	}()

	err = lookupSubjectsCmdFunc(cmd, []string{"test/resource:1", "read", "test/user"})
	require.NoError(t, err)
	require.Len(t, results, 1000)
}
