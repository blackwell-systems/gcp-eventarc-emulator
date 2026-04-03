package server

import (
	"context"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/logger"
)

// PublisherRegistrar is the subset of publishingpb.PublisherServer needed
// for registration. Defined here to avoid import cycle server→publisher→server.
type PublisherRegistrar interface {
	publishingpb.PublisherServer
}

// NewGRPCServer creates a grpc.Server with all Eventarc services registered:
//   - eventarcpb.EventarcServer (Trigger/Provider CRUD)
//   - longrunningpb.OperationsServer (LRO polling)
//   - publishingpb.PublisherServer (event publishing)
//   - grpc/reflection (for grpc_cli debugging)
//
// An optional *logger.Logger enables debug-level per-request logging via a
// unary interceptor when the logger is at debug level.
func NewGRPCServer(srv *Server, pub PublisherRegistrar, log ...*logger.Logger) *grpc.Server {
	var opts []grpc.ServerOption
	if len(log) > 0 && log[0] != nil && log[0].IsDebug() {
		lgr := log[0]
		opts = append(opts, grpc.UnaryInterceptor(func(
			ctx context.Context,
			req any,
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (any, error) {
			resp, err := handler(ctx, req)
			if err != nil {
				lgr.Debug("grpc: %s → error: %v", info.FullMethod, err)
			} else {
				lgr.Debug("grpc: %s → ok", info.FullMethod)
			}
			return resp, err
		}))
	}
	grpcSrv := grpc.NewServer(opts...)
	eventarcpb.RegisterEventarcServer(grpcSrv, srv)
	longrunningpb.RegisterOperationsServer(grpcSrv, srv.LROStore())
	publishingpb.RegisterPublisherServer(grpcSrv, pub)
	reflection.Register(grpcSrv)
	return grpcSrv
}
