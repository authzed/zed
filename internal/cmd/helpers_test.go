package cmd

import (
	"bufio"
	"context"
	"os"
	"testing"

	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/spicedb/pkg/cmd/datastore"
	"github.com/authzed/spicedb/pkg/cmd/server"
	"github.com/authzed/spicedb/pkg/cmd/util"
	"google.golang.org/grpc"

	"github.com/authzed/zed/internal/client"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/authzed/zed/pkg/backupformat"
)

func mapRelationshipTuplesToCLIOutput(t *testing.T, input []string) []string {
	t.Helper()

	return lo.Map[string, string](input, func(item string, _ int) string {
		return replaceRelString(item)
	})
}

func readLines(t *testing.T, fileName string) []string {
	t.Helper()

	f, err := os.Open(fileName)
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

type stringFlag struct {
	flagName  string
	flagValue string
}

type boolFlag struct {
	flagName  string
	flagValue bool
}

func createTestCobraCommandWithFlagValue(t *testing.T, flagAndValues ...any) *cobra.Command {
	t.Helper()

	c := cobra.Command{}
	for _, flagAndValue := range flagAndValues {
		switch f := flagAndValue.(type) {
		case stringFlag:
			c.Flags().String(f.flagName, f.flagValue, "")
		case boolFlag:
			c.Flags().Bool(f.flagName, f.flagValue, "")
		default:
			t.Fatalf("unknown flag type: %T", f)
		}
	}

	c.SetContext(context.Background())
	return &c
}

func createTestBackup(t *testing.T, schema string, relationships []string) string {
	t.Helper()

	f, err := os.CreateTemp("", "test-backup")
	require.NoError(t, err)
	defer f.Close()
	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})

	avroWriter, err := backupformat.NewEncoder(f, schema, &v1.ZedToken{Token: "test"})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, avroWriter.Close())
	}()

	for _, rel := range relationships {
		r := tuple.ParseRel(rel)
		require.NotNil(t, r)
		require.NoError(t, avroWriter.Append(r))
	}

	return f.Name()
}

func clientFromConn(conn *grpc.ClientConn) func(cmd *cobra.Command) (client.Client, error) {
	return func(cmd *cobra.Command) (client.Client, error) {
		return &authzed.ClientWithExperimental{
			Client: authzed.Client{
				SchemaServiceClient:      v1.NewSchemaServiceClient(conn),
				PermissionsServiceClient: v1.NewPermissionsServiceClient(conn),
				WatchServiceClient:       v1.NewWatchServiceClient(conn),
			},
			ExperimentalServiceClient: v1.NewExperimentalServiceClient(conn),
		}, nil
	}
}

func newServer(ctx context.Context, t *testing.T) server.RunnableServer {
	t.Helper()

	ds, err := datastore.NewDatastore(ctx,
		datastore.DefaultDatastoreConfig().ToOption(),
		datastore.WithRequestHedgingEnabled(false),
	)
	require.NoError(t, err, "unable to start memdb datastore")

	configOpts := []server.ConfigOption{
		server.WithGRPCServer(util.GRPCServerConfig{
			Network: util.BufferedNetwork,
			Enabled: true,
		}),
		server.WithGRPCAuthFunc(func(ctx context.Context) (context.Context, error) {
			return ctx, nil
		}),
		server.WithHTTPGateway(util.HTTPServerConfig{HTTPEnabled: false}),
		server.WithMetricsAPI(util.HTTPServerConfig{HTTPEnabled: true}),
		server.WithDispatchCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithNamespaceCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithClusterDispatchCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithDatastore(ds),
	}

	srv, err := server.NewConfigWithOptionsAndDefaults(configOpts...).Complete(ctx)
	require.NoError(t, err)

	return srv
}
