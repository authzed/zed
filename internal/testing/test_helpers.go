package testing

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/spicedb/pkg/cmd/datastore"
	"github.com/authzed/spicedb/pkg/cmd/server"
	"github.com/authzed/spicedb/pkg/cmd/util"

	"github.com/authzed/zed/internal/client"
)

func ClientFromConn(conn *grpc.ClientConn) func(_ *cobra.Command) (client.Client, error) {
	return func(_ *cobra.Command) (client.Client, error) {
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

func NewTestServer(ctx context.Context, t *testing.T) server.RunnableServer {
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
		server.WithMetricsAPI(util.HTTPServerConfig{HTTPEnabled: false}),
		server.WithDispatchCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithNamespaceCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithClusterDispatchCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithDatastore(ds),
	}

	srv, err := server.NewConfigWithOptionsAndDefaults(configOpts...).Complete(ctx)
	require.NoError(t, err)

	return srv
}

type StringFlag struct {
	FlagName  string
	FlagValue string
	Changed   bool
}

type BoolFlag struct {
	FlagName  string
	FlagValue bool
	Changed   bool
}

type IntFlag struct {
	FlagName  string
	FlagValue int
	Changed   bool
}

type UintFlag struct {
	FlagName  string
	FlagValue uint
	Changed   bool
}

type UintFlag32 struct {
	FlagName  string
	FlagValue uint32
	Changed   bool
}

type DurationFlag struct {
	FlagName  string
	FlagValue time.Duration
	Changed   bool
}

func CreateTestCobraCommandWithFlagValue(t *testing.T, flagAndValues ...any) *cobra.Command {
	t.Helper()

	c := cobra.Command{}
	for _, flagAndValue := range flagAndValues {
		switch f := flagAndValue.(type) {
		case StringFlag:
			c.Flags().String(f.FlagName, f.FlagValue, "")
			c.Flag(f.FlagName).Changed = f.Changed
		case BoolFlag:
			c.Flags().Bool(f.FlagName, f.FlagValue, "")
			c.Flag(f.FlagName).Changed = f.Changed
		case IntFlag:
			c.Flags().Int(f.FlagName, f.FlagValue, "")
			c.Flag(f.FlagName).Changed = f.Changed
		case UintFlag:
			c.Flags().Uint(f.FlagName, f.FlagValue, "")
			c.Flag(f.FlagName).Changed = f.Changed
		case UintFlag32:
			c.Flags().Uint32(f.FlagName, f.FlagValue, "")
			c.Flag(f.FlagName).Changed = f.Changed
		case DurationFlag:
			c.Flags().Duration(f.FlagName, f.FlagValue, "")
			c.Flag(f.FlagName).Changed = f.Changed
		default:
			t.Fatalf("unknown flag type: %T", f)
		}
	}

	c.SetContext(t.Context())
	return &c
}
