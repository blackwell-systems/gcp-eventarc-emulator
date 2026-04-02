// Package eventarcv1 provides grpc-gateway handler registration for the
// Google Cloud Eventarc v1 API.
//
// This file re-exports gRPC service interfaces and request/response types
// from cloud.google.com/go/eventarc/apiv1/eventarcpb so that the generated
// gateway file (eventarc.pb.gw.go) compiles without also generating pb.go.
package eventarcv1

import (
	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc"
)

// gRPC service interfaces and constructors.
type EventarcClient = eventarcpb.EventarcClient
type EventarcServer = eventarcpb.EventarcServer

func NewEventarcClient(cc grpc.ClientConnInterface) EventarcClient {
	return eventarcpb.NewEventarcClient(cc)
}

// Request types used by the gateway handlers.
type GetTriggerRequest = eventarcpb.GetTriggerRequest
type ListTriggersRequest = eventarcpb.ListTriggersRequest
type CreateTriggerRequest = eventarcpb.CreateTriggerRequest
type UpdateTriggerRequest = eventarcpb.UpdateTriggerRequest
type DeleteTriggerRequest = eventarcpb.DeleteTriggerRequest
type GetChannelRequest = eventarcpb.GetChannelRequest
type ListChannelsRequest = eventarcpb.ListChannelsRequest
type CreateChannelRequest = eventarcpb.CreateChannelRequest
type UpdateChannelRequest = eventarcpb.UpdateChannelRequest
type DeleteChannelRequest = eventarcpb.DeleteChannelRequest
type GetProviderRequest = eventarcpb.GetProviderRequest
type ListProvidersRequest = eventarcpb.ListProvidersRequest
type GetChannelConnectionRequest = eventarcpb.GetChannelConnectionRequest
type ListChannelConnectionsRequest = eventarcpb.ListChannelConnectionsRequest
type CreateChannelConnectionRequest = eventarcpb.CreateChannelConnectionRequest
type DeleteChannelConnectionRequest = eventarcpb.DeleteChannelConnectionRequest
type UpdateGoogleChannelConfigRequest = eventarcpb.UpdateGoogleChannelConfigRequest
type GetGoogleChannelConfigRequest = eventarcpb.GetGoogleChannelConfigRequest
type GetMessageBusRequest = eventarcpb.GetMessageBusRequest
type ListMessageBusesRequest = eventarcpb.ListMessageBusesRequest
type ListMessageBusEnrollmentsRequest = eventarcpb.ListMessageBusEnrollmentsRequest
type CreateMessageBusRequest = eventarcpb.CreateMessageBusRequest
type UpdateMessageBusRequest = eventarcpb.UpdateMessageBusRequest
type DeleteMessageBusRequest = eventarcpb.DeleteMessageBusRequest
type GetEnrollmentRequest = eventarcpb.GetEnrollmentRequest
type ListEnrollmentsRequest = eventarcpb.ListEnrollmentsRequest
type CreateEnrollmentRequest = eventarcpb.CreateEnrollmentRequest
type UpdateEnrollmentRequest = eventarcpb.UpdateEnrollmentRequest
type DeleteEnrollmentRequest = eventarcpb.DeleteEnrollmentRequest
type GetPipelineRequest = eventarcpb.GetPipelineRequest
type ListPipelinesRequest = eventarcpb.ListPipelinesRequest
type CreatePipelineRequest = eventarcpb.CreatePipelineRequest
type UpdatePipelineRequest = eventarcpb.UpdatePipelineRequest
type DeletePipelineRequest = eventarcpb.DeletePipelineRequest
type GetGoogleApiSourceRequest = eventarcpb.GetGoogleApiSourceRequest
type ListGoogleApiSourcesRequest = eventarcpb.ListGoogleApiSourcesRequest
type CreateGoogleApiSourceRequest = eventarcpb.CreateGoogleApiSourceRequest
type UpdateGoogleApiSourceRequest = eventarcpb.UpdateGoogleApiSourceRequest
type DeleteGoogleApiSourceRequest = eventarcpb.DeleteGoogleApiSourceRequest
