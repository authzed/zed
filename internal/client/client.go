package client

import (
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	zgrpcutil "github.com/authzed/zed/internal/grpcutil"
	"github.com/authzed/zed/internal/storage"
)

// Client defines an interface for making calls to SpiceDB API.
type Client interface {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
}

// NewClient defines an (overridable) means of creating a new client.
var NewClient = newGRPCClient

func newGRPCClient(cmd *cobra.Command) (Client, error) {
	configStore, secretStore := DefaultStorage()
	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
		configStore,
		secretStore,
	)
	if err != nil {
		return nil, err
	}
	log.Trace().Interface("token", token).Send()

	dialOpts, err := DialOptsFromFlags(cmd, token)
	if err != nil {
		return nil, err
	}

	client, err := authzed.NewClient(token.Endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}

	return client, err
}

// DefaultStorage returns the default configured config store and secret store.
func DefaultStorage() (storage.ConfigStore, storage.SecretStore) {
	var home string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		home = filepath.Join(xdg, "zed")
	} else {
		homedir, _ := homedir.Dir()
		home = filepath.Join(homedir, ".zed")
	}
	return &storage.JSONConfigStore{ConfigPath: home},
		&storage.KeychainSecretStore{ConfigPath: home}
}

func certOption(cmd *cobra.Command, token storage.Token) (opt grpc.DialOption, err error) {
	verification := grpcutil.VerifyCA
	if cobrautil.MustGetBool(cmd, "no-verify-ca") {
		verification = grpcutil.SkipVerifyCA
	}

	if certBytes, ok := token.Certificate(); ok {
		return grpcutil.WithCustomCertBytes(verification, certBytes)
	}
	return grpcutil.WithSystemCerts(verification)
}

// DialOptsFromFlags returns the dial options from the CLI-specified flags.
func DialOptsFromFlags(cmd *cobra.Command, token storage.Token) ([]grpc.DialOption, error) {
	grpc.WithChainUnaryInterceptor()

	interceptors := []grpc.UnaryClientInterceptor{
		zgrpcutil.LogDispatchTrailers,
	}

	if !cobrautil.MustGetBool(cmd, "skip-version-check") {
		interceptors = append(interceptors, zgrpcutil.CheckServerVersion)
	}

	opts := []grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}

	if cobrautil.MustGetBool(cmd, "insecure") || (token.IsInsecure()) {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		opts = append(opts, grpcutil.WithInsecureBearerToken(token.APIToken))
	} else {
		opts = append(opts, grpcutil.WithBearerToken(token.APIToken))
		certOpt, err := certOption(cmd, token)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS cert: %w", err)
		}
		opts = append(opts, certOpt)
	}

	return opts, nil
}
