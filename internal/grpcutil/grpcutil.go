package grpcutil

import (
	"context"

	"github.com/authzed/authzed-go/pkg/responsemeta"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Compile-time assertion that LogDispatchTrailers implements the
// grpc.UnaryClientInterceptor interface.
var _ grpc.UnaryClientInterceptor = grpc.UnaryClientInterceptor(LogDispatchTrailers)

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
