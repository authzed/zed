package grpcutil

import (
	"context"
	"time"

	"github.com/authzed/authzed-go/pkg/requestmeta"
	"github.com/authzed/authzed-go/pkg/responsemeta"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/authzed/spicedb/pkg/releases"
)

// Compile-time assertion that LogDispatchTrailers and CheckServerVersion implement the
// grpc.UnaryClientInterceptor interface.
var (
	_ grpc.UnaryClientInterceptor = grpc.UnaryClientInterceptor(LogDispatchTrailers)
	_ grpc.UnaryClientInterceptor = grpc.UnaryClientInterceptor(CheckServerVersion)
)

// CheckServerVersion implements a gRPC unary interceptor that requests the server version
// from SpiceDB and, if found, compares it to the current released version.
func CheckServerVersion(
	ctx context.Context,
	method string,
	req, reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	callOpts ...grpc.CallOption,
) error {
	var headerMD metadata.MD
	ctx = requestmeta.AddRequestHeaders(ctx, requestmeta.RequestServerVersion)
	err := invoker(ctx, method, req, reply, cc, append(callOpts, grpc.Header(&headerMD))...)
	if err != nil {
		return err
	}

	version := headerMD.Get(string(responsemeta.ServerVersion))
	if len(version) == 0 {
		log.Debug().Msg("error reading server version response header; it may be disabled on the server")
	} else if len(version) == 1 {
		currentVersion := version[0]

		rctx, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		state, _, release, cerr := releases.CheckIsLatestVersion(rctx, func() (string, error) {
			return currentVersion, nil
		}, releases.GetLatestRelease)
		if cerr != nil {
			log.Debug().Err(cerr).Msg("error looking up currently released version")
		} else {
			switch state {
			case releases.UnreleasedVersion:
				log.Warn().Str("version", currentVersion).Msg("not calling a released version of SpiceDB")
				return nil

			case releases.UpdateAvailable:
				log.Warn().Str("this-version", currentVersion).Str("latest-released-version", release.Version).Msgf("the version of SpiceDB being called is out of date. See: %s", release.ViewURL)
				return nil

			case releases.UpToDate:
				log.Debug().Str("latest-released-version", release.Version).Msg("the version of SpiceDB being called is the latest released version")
				return nil

			case releases.Unknown:
				log.Warn().Str("unknown-released-version", release.Version).Msg("unable to check for a new SpiceDB version")
				return nil

			default:
				panic("Unknown state for CheckAndLogRunE")
			}
		}
	}

	return err
}

// LogDispatchTrailers implements a gRPC unary interceptor that logs the
// dispatch metadata that is present in response trailers from SpiceDB.
func LogDispatchTrailers(
	ctx context.Context,
	method string,
	req, reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	callOpts ...grpc.CallOption,
) error {
	var trailerMD metadata.MD
	err := invoker(ctx, method, req, reply, cc, append(callOpts, grpc.Trailer(&trailerMD))...)
	log.Trace().Interface("trailers", trailerMD).Msg("parsed trailers")

	dispatchCount, trailerErr := responsemeta.GetIntResponseTrailerMetadata(
		trailerMD,
		responsemeta.DispatchedOperationsCount,
	)
	if trailerErr != nil {
		log.Debug().Err(trailerErr).Msg("error reading dispatched operations trailer")
	}

	cachedCount, trailerErr := responsemeta.GetIntResponseTrailerMetadata(
		trailerMD,
		responsemeta.CachedOperationsCount,
	)
	if trailerErr != nil {
		log.Debug().Err(trailerErr).Msg("error reading cached operations trailer")
	}

	log.Debug().
		Int("dispatch", dispatchCount).
		Int("cached", cachedCount).
		Msg("extracted response dispatch metadata")

	return err
}
