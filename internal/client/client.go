package client

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"

	zgrpcutil "github.com/authzed/zed/internal/grpcutil"
	"github.com/authzed/zed/internal/storage"
)

// Client defines an interface for making calls to SpiceDB API.
type Client interface {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
	v1.ExperimentalServiceClient
}

const (
	defaultRetryExponentialBackoff = 100 * time.Millisecond
	defaultMaxRetryAttemptDuration = 2 * time.Second
	defaultRetryJitterFraction     = 0.5
	importBulkRoute                = "/authzed.api.v1.PermissionsService/ImportBulkRelationships"
)

// NewClient defines an (overridable) means of creating a new client.
var (
	NewClient           = newClientForCurrentContext
	NewClientForContext = newClientForContext
)

func newClientForCurrentContext(cmd *cobra.Command) (Client, error) {
	configStore, secretStore := DefaultStorage()
	token, err := GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)
	if err != nil {
		return nil, err
	}

	dialOpts, err := DialOptsFromFlags(cmd, token)
	if err != nil {
		return nil, err
	}

	if cobrautil.MustGetString(cmd, "proxy") != "" {
		token.Endpoint = withPassthroughTarget(token.Endpoint)
	}

	client, err := authzed.NewClientWithExperimentalAPIs(token.Endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}

	return client, err
}

func newClientForContext(cmd *cobra.Command, contextName string, secretStore storage.SecretStore) (*authzed.Client, error) {
	currentToken, err := storage.GetTokenIfExists(contextName, secretStore)
	if err != nil {
		return nil, err
	}

	token, err := GetTokenWithCLIOverride(cmd, currentToken)
	if err != nil {
		return nil, err
	}

	dialOpts, err := DialOptsFromFlags(cmd, token)
	if err != nil {
		return nil, err
	}

	if cobrautil.MustGetString(cmd, "proxy") != "" {
		token.Endpoint = withPassthroughTarget(token.Endpoint)
	}

	return authzed.NewClient(token.Endpoint, dialOpts...)
}

// GetCurrentTokenWithCLIOverride returns the current token, but overridden by any parameter specified via CLI args
func GetCurrentTokenWithCLIOverride(cmd *cobra.Command, configStore storage.ConfigStore, secretStore storage.SecretStore) (storage.Token, error) {
	// Handle the no-config case separately
	configExists, err := configStore.Exists()
	if err != nil {
		return storage.Token{}, err
	}
	if !configExists {
		return GetTokenWithCLIOverride(cmd, storage.Token{})
	}
	token, err := storage.CurrentToken(
		configStore,
		secretStore,
	)
	if err != nil {
		return storage.Token{}, err
	}

	return GetTokenWithCLIOverride(cmd, token)
}

// GetTokenWithCLIOverride returns the provided token, but overridden by any parameter specified explicitly via command
// flags
func GetTokenWithCLIOverride(cmd *cobra.Command, token storage.Token) (storage.Token, error) {
	overrideToken, err := tokenFromCli(cmd)
	if err != nil {
		return storage.Token{}, err
	}

	result, err := storage.TokenWithOverride(
		overrideToken,
		token,
	)
	if err != nil {
		return storage.Token{}, err
	}

	log.Trace().Bool("context-override-via-cli", overrideToken.AnyValue()).Interface("context", result).Send()
	return result, nil
}

func tokenFromCli(cmd *cobra.Command) (storage.Token, error) {
	certPath := cobrautil.MustGetStringExpanded(cmd, "certificate-path")
	var certBytes []byte
	var err error
	if certPath != "" {
		certBytes, err = os.ReadFile(certPath)
		if err != nil {
			return storage.Token{}, fmt.Errorf("failed to read ceritficate: %w", err)
		}
	}

	explicitInsecure := cmd.Flags().Changed("insecure")
	var notSecure *bool
	if explicitInsecure {
		i := cobrautil.MustGetBool(cmd, "insecure")
		notSecure = &i
	}

	explicitNoVerifyCA := cmd.Flags().Changed("no-verify-ca")
	var notVerifyCA *bool
	if explicitNoVerifyCA {
		nvc := cobrautil.MustGetBool(cmd, "no-verify-ca")
		notVerifyCA = &nvc
	}
	overrideToken := storage.Token{
		APIToken:   cobrautil.MustGetString(cmd, "token"),
		Endpoint:   cobrautil.MustGetString(cmd, "endpoint"),
		Insecure:   notSecure,
		NoVerifyCA: notVerifyCA,
		CACert:     certBytes,
	}
	return overrideToken, nil
}

// DefaultStorage returns the default configured config store and secret store.
var DefaultStorage = defaultStorage

func defaultStorage() (storage.ConfigStore, storage.SecretStore) {
	var home string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		home = filepath.Join(xdg, "zed")
	} else {
		hmdir, _ := homedir.Dir()
		home = filepath.Join(hmdir, ".zed")
	}
	return &storage.JSONConfigStore{ConfigPath: home},
		&storage.KeychainSecretStore{ConfigPath: home}
}

func certOption(token storage.Token) (opt grpc.DialOption, err error) {
	verification := grpcutil.VerifyCA
	if token.HasNoVerifyCA() {
		verification = grpcutil.SkipVerifyCA
	}

	if certBytes, ok := token.Certificate(); ok {
		return grpcutil.WithCustomCertBytes(verification, certBytes)
	}

	return grpcutil.WithSystemCerts(verification)
}

func isNoneOf(routes ...string) func(_ context.Context, c interceptors.CallMeta) bool {
	return func(_ context.Context, c interceptors.CallMeta) bool {
		for _, route := range routes {
			if route == c.FullMethod() {
				return false
			}
		}
		return true
	}
}

// DialOptsFromFlags returns the dial options from the CLI-specified flags.
func DialOptsFromFlags(cmd *cobra.Command, token storage.Token) ([]grpc.DialOption, error) {
	maxRetries := cobrautil.MustGetUint(cmd, "max-retries")
	retryOpts := []retry.CallOption{
		retry.WithBackoff(retry.BackoffExponentialWithJitterBounded(defaultRetryExponentialBackoff,
			defaultRetryJitterFraction, defaultMaxRetryAttemptDuration)),
		retry.WithCodes(codes.ResourceExhausted, codes.Unavailable, codes.Aborted, codes.Unknown, codes.Internal),
		retry.WithMax(maxRetries),
		retry.WithOnRetryCallback(func(_ context.Context, attempt uint, err error) {
			log.Error().Err(err).Uint("attempt", attempt).Msg("retrying gRPC call")
		}),
	}
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		zgrpcutil.LogDispatchTrailers,
		retry.UnaryClientInterceptor(retryOpts...),
	}

	streamInterceptors := []grpc.StreamClientInterceptor{
		zgrpcutil.StreamLogDispatchTrailers,
		selector.StreamClientInterceptor(retry.StreamClientInterceptor(retryOpts...), selector.MatchFunc(isNoneOf(importBulkRoute))),
	}

	if !cobrautil.MustGetBool(cmd, "skip-version-check") {
		unaryInterceptors = append(unaryInterceptors, zgrpcutil.CheckServerVersion)
	}

	opts := []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	}

	proxyAddr := cobrautil.MustGetString(cmd, "proxy")

	if proxyAddr != "" {
		addr, err := url.Parse(proxyAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse socks5 proxy addr: %w", err)
		}

		dialer, err := proxy.SOCKS5("tcp", addr.Host, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create socks5 proxy dialer: %w", err)
		}

		opts = append(opts, grpc.WithContextDialer(func(_ context.Context, addr string) (net.Conn, error) {
			return dialer.Dial("tcp", addr)
		}))
	}

	if token.IsInsecure() {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		opts = append(opts, grpcutil.WithInsecureBearerToken(token.APIToken))
	} else {
		opts = append(opts, grpcutil.WithBearerToken(token.APIToken))
		certOpt, err := certOption(token)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS cert: %w", err)
		}
		opts = append(opts, certOpt)
	}

	hostnameOverride := cobrautil.MustGetString(cmd, "hostname-override")
	if hostnameOverride != "" {
		opts = append(opts, grpc.WithAuthority(hostnameOverride))
	}

	maxMessageSize := cobrautil.MustGetInt(cmd, "max-message-size")
	if maxMessageSize != 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(
			// The default max client message size is 4mb.
			// It's conceivable that a sufficiently complex
			// schema will easily surpass this, so we set the
			// limit higher here.
			grpc.MaxCallRecvMsgSize(maxMessageSize),
			grpc.MaxCallSendMsgSize(maxMessageSize),
		))
	}

	return opts, nil
}

func withPassthroughTarget(endpoint string) string {
	// If it already has a scheme, return as-is
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "passthrough:///" + endpoint
}
