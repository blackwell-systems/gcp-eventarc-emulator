// Package gcp_eventarc_emulator provides the composition entry point for the GCP Eventarc Emulator.
//
// Register wires all Eventarc gRPC services (Eventarc, Publisher, Operations)
// onto an existing grpc.Server, enabling use within the unified gcp-emulator
// or any custom composition layer.
// For standalone use, see cmd/server, cmd/server-rest, or cmd/server-dual.
package gcp_eventarc_emulator

import (
	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/dispatcher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/logger"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/publisher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
)

// Option configures the Eventarc server at registration time.
type Option func(*options)

type options struct {
	logger *logger.Logger
}

// WithLogger sets the logger for the Eventarc service.
func WithLogger(l *logger.Logger) Option {
	return func(o *options) { o.logger = l }
}

// Register adds all Eventarc gRPC services to grpcSrv:
//   - google.cloud.eventarc.v1.Eventarc (Trigger/Channel/Pipeline CRUD)
//   - google.longrunning.Operations (LRO polling)
//   - google.cloud.eventarc.publishing.v1.Publisher (CloudEvent ingestion)
//
// IAM enforcement is configured via the IAM_MODE and IAM_EMULATOR_HOST
// environment variables (same as the standalone binary).
// It does not start a listener — the caller owns the grpc.Server lifecycle.
func Register(grpcSrv *grpc.Server, opts ...Option) error {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	lgr := logger.OrDefault(o.logger)

	srv, err := server.NewServer()
	if err != nil {
		return err
	}

	rtr := router.NewRouter(srv.Storage(), lgr)
	dsp := dispatcher.NewDispatcher(nil, lgr)
	pub := publisher.NewServer(rtr, dsp, srv.Storage(), lgr)

	eventarcpb.RegisterEventarcServer(grpcSrv, srv)
	longrunningpb.RegisterOperationsServer(grpcSrv, srv.LROStore())
	publishingpb.RegisterPublisherServer(grpcSrv, pub)
	reflection.Register(grpcSrv)
	return nil
}
