package bats

import (
	"context"

	"go.temporal.io/server/tools/umpire"
	"go.temporal.io/server/tools/umpire/pitcher"
	"google.golang.org/grpc"
)

// UnaryServerInterceptor returns a gRPC unary interceptor that injects faults via the global pitcher
// and records moves via the global umpire.
// This interceptor should be installed in the test cluster to enable fault injection and move tracking.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Record the move in the scorebook (if umpire is configured)
		u := umpire.Get()
		if u != nil {
			u.RecordMove(ctx, req)
		}

		// Check if pitcher is configured for fault injection
		p := pitcher.Get()
		if p == nil {
			// No pitcher configured, pass through
			return handler(ctx, req)
		}

		// Try to make a play based on the request type
		playMade, err := p.MakePlay(ctx, req, req)
		if err != nil {
			// Play was made (fault injected), return the error
			_ = playMade
			return nil, err
		}

		// No play made, continue with normal execution
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream interceptor that injects faults via the global pitcher.
// For streams, we inject faults when the stream is first opened.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Check if pitcher is configured
		p := pitcher.Get()
		if p == nil {
			// No pitcher configured, pass through
			return handler(srv, ss)
		}

		// For streams, we use a placeholder request type based on the method name
		// In practice, we'd need to know the request type for each stream method
		// For now, we skip fault injection for streams (can be enhanced later)

		// No fault injected for streams yet, continue with normal execution
		return handler(srv, ss)
	}
}
