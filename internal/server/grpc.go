package server

import (
	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
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
func NewGRPCServer(srv *Server, pub PublisherRegistrar) *grpc.Server {
	grpcSrv := grpc.NewServer()
	eventarcpb.RegisterEventarcServer(grpcSrv, srv)
	longrunningpb.RegisterOperationsServer(grpcSrv, srv.LROStore())
	publishingpb.RegisterPublisherServer(grpcSrv, pub)
	reflection.Register(grpcSrv)
	return grpcSrv
}
