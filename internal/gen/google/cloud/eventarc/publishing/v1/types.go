// Package publishingv1 provides grpc-gateway handler registration for the
// Google Cloud Eventarc Publishing v1 API.
//
// This file re-exports gRPC service interfaces and request/response types
// from cloud.google.com/go/eventarc/publishing/apiv1/publishingpb so that
// the generated gateway file (publisher.pb.gw.go) compiles without also
// generating pb.go.
package publishingv1

import (
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	"google.golang.org/grpc"
)

// gRPC service interfaces and constructors.
type PublisherClient = publishingpb.PublisherClient
type PublisherServer = publishingpb.PublisherServer

func NewPublisherClient(cc grpc.ClientConnInterface) PublisherClient {
	return publishingpb.NewPublisherClient(cc)
}

// Request types used by the gateway handlers.
type PublishChannelConnectionEventsRequest = publishingpb.PublishChannelConnectionEventsRequest
type PublishEventsRequest = publishingpb.PublishEventsRequest
type PublishRequest = publishingpb.PublishRequest
