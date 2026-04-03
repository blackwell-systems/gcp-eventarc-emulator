// Package server implements a gRPC emulator for Google Cloud Eventarc API.
//
// This package provides a complete mock implementation of the Eventarc v1 API
// for local development and testing. It implements the EventarcServer interface
// with in-memory storage, eliminating the need for GCP credentials or network access.
//
// The server supports Trigger CRUD (returning LROs) and read-only Provider access.
// All operations are thread-safe (Storage is guarded by sync.RWMutex internally).
//
// For standalone usage, see cmd/server. For embedded testing, import this package
// directly and create a server with NewServer().
package server

import (
	"context"
	"fmt"
	"strings"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	emulatorauth "github.com/blackwell-systems/gcp-emulator-auth"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/lro"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the EventarcServer interface.
// It provides a mock implementation of GCP Eventarc for local development.
//
// The server maintains in-memory storage of triggers and providers with
// thread-safe access. All gRPC methods are implemented to match GCP Eventarc
// behavior for common operations. LRO-returning operations resolve immediately.
//
// Usage:
//
//	srv, err := server.NewServer()
//	grpcServer := server.NewGRPCServer(srv, publisher)
type Server struct {
	eventarcpb.UnimplementedEventarcServer
	storage   *Storage
	lro       *lro.Store
	iamClient *emulatorauth.Client
	iamMode   emulatorauth.AuthMode
}

// NewServer creates a new mock Eventarc server.
func NewServer() (*Server, error) {
	s := &Server{
		storage: NewStorage(),
		lro:     lro.NewStore(),
	}

	config := emulatorauth.LoadFromEnv()
	s.iamMode = config.Mode

	if config.Mode.IsEnabled() {
		client, err := emulatorauth.NewClient(config.Host, config.Mode, "gcp-eventarc-emulator")
		if err != nil {
			return nil, fmt.Errorf("failed to connect to IAM emulator: %w", err)
		}
		s.iamClient = client
	}

	return s, nil
}

// Storage returns the underlying storage (useful for testing).
func (s *Server) Storage() *Storage {
	return s.storage
}

// LROStore returns the underlying LRO store.
// Callers should register it on the gRPC server:
//
//	longrunningpb.RegisterOperationsServer(grpcSrv, srv.LROStore())
func (s *Server) LROStore() *lro.Store {
	return s.lro
}

// IAMMode returns the active IAM enforcement mode as a string.
// Returns "off", "permissive", or "strict".
func (s *Server) IAMMode() string {
	return string(s.iamMode)
}

// checkPermission verifies the caller has permission to perform the operation.
// If iamClient is nil (IAM disabled), all requests are allowed.
func (s *Server) checkPermission(ctx context.Context, permission string, resource string) error {
	if s.iamClient == nil {
		return nil // IAM disabled, allow all
	}

	principal := emulatorauth.ExtractPrincipalFromContext(ctx)

	allowed, err := s.iamClient.CheckPermission(ctx, principal, resource, permission)
	if err != nil {
		// Detect connection-refused / unavailable errors and return a user-friendly message.
		errMsg := err.Error()
		if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "Unavailable") {
			return status.Errorf(codes.FailedPrecondition,
				"IAM_MODE is active but no IAM emulator is reachable. "+
					"Start the IAM emulator or set IAM_MODE=off.")
		}
		return status.Errorf(codes.Internal, "IAM check failed: %v", err)
	}

	if !allowed {
		displayPrincipal := principal
		if displayPrincipal == "" {
			displayPrincipal = "(no principal)"
		}
		return status.Errorf(codes.PermissionDenied,
			"Permission denied: principal '%s' lacks '%s' on resource '%s'",
			displayPrincipal, permission, resource)
	}

	return nil
}

// parentFromName extracts the parent resource from a full trigger name.
// e.g. "projects/p/locations/l/triggers/t" → "projects/p/locations/l"
func parentFromName(name string) string {
	// Strip the last two segments ("/triggers/<id>")
	idx := strings.LastIndex(name, "/triggers/")
	if idx < 0 {
		return name
	}
	return name[:idx]
}

// parentFromResource extracts the parent from a full resource name by
// stripping the last two path segments ("/{resourceType}/{id}").
// e.g. "projects/p/locations/l/channels/c" → "projects/p/locations/l"
func parentFromResource(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx < 0 {
		return name
	}
	name = name[:idx]
	idx = strings.LastIndex(name, "/")
	if idx < 0 {
		return name
	}
	return name[:idx]
}

// -------------------------------------------------------------------------
// Trigger RPCs
// -------------------------------------------------------------------------

// GetTrigger returns the trigger with the given full resource name.
func (s *Server) GetTrigger(ctx context.Context, req *eventarcpb.GetTriggerRequest) (*eventarcpb.Trigger, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if err := s.checkPermission(ctx, "eventarc.triggers.get", req.GetName()); err != nil {
		return nil, err
	}

	return s.storage.GetTrigger(ctx, req.GetName())
}

// ListTriggers lists triggers under the given parent with optional pagination.
func (s *Server) ListTriggers(ctx context.Context, req *eventarcpb.ListTriggersRequest) (*eventarcpb.ListTriggersResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}

	if err := s.checkPermission(ctx, "eventarc.triggers.list", req.GetParent()); err != nil {
		return nil, err
	}

	triggers, nextToken, err := s.storage.ListTriggers(
		ctx,
		req.GetParent(),
		req.GetPageSize(),
		req.GetPageToken(),
		req.GetOrderBy(),
		req.GetFilter(),
	)
	if err != nil {
		return nil, err
	}

	return &eventarcpb.ListTriggersResponse{
		Triggers:      triggers,
		NextPageToken: nextToken,
	}, nil
}

// CreateTrigger creates a new trigger and returns a completed LRO.
func (s *Server) CreateTrigger(ctx context.Context, req *eventarcpb.CreateTriggerRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetTriggerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "trigger_id is required")
	}
	if req.GetTrigger() == nil {
		return nil, status.Error(codes.InvalidArgument, "trigger is required")
	}
	if req.GetTrigger().GetDestination() == nil {
		return nil, status.Error(codes.InvalidArgument, "trigger.destination is required")
	}
	if len(req.GetTrigger().GetEventFilters()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "trigger.event_filters must not be empty: at least one event filter is required")
	}

	if err := s.checkPermission(ctx, "eventarc.triggers.create", req.GetParent()); err != nil {
		return nil, err
	}

	trigger, err := s.storage.CreateTrigger(ctx, req.GetParent(), req.GetTriggerId(), req.GetTrigger())
	if err != nil {
		return nil, err
	}

	return s.lro.CreateDone(req.GetParent(), trigger)
}

// UpdateTrigger updates an existing trigger and returns a completed LRO.
func (s *Server) UpdateTrigger(ctx context.Context, req *eventarcpb.UpdateTriggerRequest) (*longrunningpb.Operation, error) {
	if req.GetTrigger() == nil || req.GetTrigger().GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "trigger.name is required")
	}

	parent := parentFromName(req.GetTrigger().GetName())

	if err := s.checkPermission(ctx, "eventarc.triggers.update", req.GetTrigger().GetName()); err != nil {
		return nil, err
	}

	trigger, err := s.storage.UpdateTrigger(ctx, req.GetTrigger(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}

	return s.lro.CreateDone(parent, trigger)
}

// DeleteTrigger deletes an existing trigger and returns a completed LRO.
func (s *Server) DeleteTrigger(ctx context.Context, req *eventarcpb.DeleteTriggerRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	parent := parentFromName(req.GetName())

	if err := s.checkPermission(ctx, "eventarc.triggers.delete", req.GetName()); err != nil {
		return nil, err
	}

	trigger, err := s.storage.GetTrigger(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteTrigger(ctx, req.GetName()); err != nil {
		return nil, err
	}

	return s.lro.CreateDone(parent, trigger)
}

// -------------------------------------------------------------------------
// Provider RPCs (read-only, no LRO)
// -------------------------------------------------------------------------

// GetProvider returns the provider with the given full resource name.
func (s *Server) GetProvider(ctx context.Context, req *eventarcpb.GetProviderRequest) (*eventarcpb.Provider, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if err := s.checkPermission(ctx, "eventarc.providers.get", req.GetName()); err != nil {
		return nil, err
	}

	return s.storage.GetProvider(ctx, req.GetName())
}

// ListProviders lists providers under the given parent.
func (s *Server) ListProviders(ctx context.Context, req *eventarcpb.ListProvidersRequest) (*eventarcpb.ListProvidersResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}

	if err := s.checkPermission(ctx, "eventarc.providers.list", req.GetParent()); err != nil {
		return nil, err
	}

	providers, nextToken, err := s.storage.ListProviders(
		ctx,
		req.GetParent(),
		req.GetPageSize(),
		req.GetPageToken(),
		req.GetFilter(),
		req.GetOrderBy(),
	)
	if err != nil {
		return nil, err
	}

	return &eventarcpb.ListProvidersResponse{
		Providers:     providers,
		NextPageToken: nextToken,
	}, nil
}

// -------------------------------------------------------------------------
// Channel RPCs
// -------------------------------------------------------------------------

// GetChannel returns the channel with the given full resource name.
func (s *Server) GetChannel(ctx context.Context, req *eventarcpb.GetChannelRequest) (*eventarcpb.Channel, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.channels.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetChannel(ctx, req.GetName())
}

// ListChannels lists channels under the given parent.
func (s *Server) ListChannels(ctx context.Context, req *eventarcpb.ListChannelsRequest) (*eventarcpb.ListChannelsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.channels.list", req.GetParent()); err != nil {
		return nil, err
	}
	channels, nextToken, err := s.storage.ListChannels(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListChannelsResponse{
		Channels:      channels,
		NextPageToken: nextToken,
	}, nil
}

// CreateChannel creates a new channel and returns a completed LRO.
func (s *Server) CreateChannel(ctx context.Context, req *eventarcpb.CreateChannelRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetChannelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "channel_id is required")
	}
	if req.GetChannel() == nil {
		return nil, status.Error(codes.InvalidArgument, "channel is required")
	}
	if err := s.checkPermission(ctx, "eventarc.channels.create", req.GetParent()); err != nil {
		return nil, err
	}
	channel, err := s.storage.CreateChannel(ctx, req.GetParent(), req.GetChannelId(), req.GetChannel())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), channel)
}

// UpdateChannel updates an existing channel and returns a completed LRO.
func (s *Server) UpdateChannel(ctx context.Context, req *eventarcpb.UpdateChannelRequest) (*longrunningpb.Operation, error) {
	if req.GetChannel() == nil || req.GetChannel().GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "channel.name is required")
	}
	parent := parentFromResource(req.GetChannel().GetName())
	if err := s.checkPermission(ctx, "eventarc.channels.update", req.GetChannel().GetName()); err != nil {
		return nil, err
	}
	channel, err := s.storage.UpdateChannel(ctx, req.GetChannel(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, channel)
}

// DeleteChannel deletes an existing channel and returns a completed LRO.
func (s *Server) DeleteChannel(ctx context.Context, req *eventarcpb.DeleteChannelRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, "eventarc.channels.delete", req.GetName()); err != nil {
		return nil, err
	}
	channel, err := s.storage.GetChannel(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteChannel(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, channel)
}

// -------------------------------------------------------------------------
// ChannelConnection RPCs
// -------------------------------------------------------------------------

// GetChannelConnection returns the channel connection with the given full resource name.
func (s *Server) GetChannelConnection(ctx context.Context, req *eventarcpb.GetChannelConnectionRequest) (*eventarcpb.ChannelConnection, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.channelConnections.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetChannelConnection(ctx, req.GetName())
}

// ListChannelConnections lists channel connections under the given parent.
func (s *Server) ListChannelConnections(ctx context.Context, req *eventarcpb.ListChannelConnectionsRequest) (*eventarcpb.ListChannelConnectionsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.channelConnections.list", req.GetParent()); err != nil {
		return nil, err
	}
	conns, nextToken, err := s.storage.ListChannelConnections(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListChannelConnectionsResponse{
		ChannelConnections: conns,
		NextPageToken:      nextToken,
	}, nil
}

// CreateChannelConnection creates a new channel connection and returns a completed LRO.
func (s *Server) CreateChannelConnection(ctx context.Context, req *eventarcpb.CreateChannelConnectionRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetChannelConnectionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "channel_connection_id is required")
	}
	if req.GetChannelConnection() == nil {
		return nil, status.Error(codes.InvalidArgument, "channel_connection is required")
	}
	if err := s.checkPermission(ctx, "eventarc.channelConnections.create", req.GetParent()); err != nil {
		return nil, err
	}
	conn, err := s.storage.CreateChannelConnection(ctx, req.GetParent(), req.GetChannelConnectionId(), req.GetChannelConnection())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), conn)
}

// DeleteChannelConnection deletes an existing channel connection and returns a completed LRO.
func (s *Server) DeleteChannelConnection(ctx context.Context, req *eventarcpb.DeleteChannelConnectionRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, "eventarc.channelConnections.delete", req.GetName()); err != nil {
		return nil, err
	}
	conn, err := s.storage.GetChannelConnection(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteChannelConnection(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, conn)
}

// -------------------------------------------------------------------------
// GoogleChannelConfig RPCs
// -------------------------------------------------------------------------

// GetGoogleChannelConfig returns the google channel config for the given name (singleton).
func (s *Server) GetGoogleChannelConfig(ctx context.Context, req *eventarcpb.GetGoogleChannelConfigRequest) (*eventarcpb.GoogleChannelConfig, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.googleChannelConfigs.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetGoogleChannelConfig(ctx, req.GetName())
}

// UpdateGoogleChannelConfig updates the google channel config and returns it directly (no LRO).
func (s *Server) UpdateGoogleChannelConfig(ctx context.Context, req *eventarcpb.UpdateGoogleChannelConfigRequest) (*eventarcpb.GoogleChannelConfig, error) {
	if req.GetGoogleChannelConfig() == nil {
		return nil, status.Error(codes.InvalidArgument, "google_channel_config is required")
	}
	if err := s.checkPermission(ctx, "eventarc.googleChannelConfigs.update", req.GetGoogleChannelConfig().GetName()); err != nil {
		return nil, err
	}
	return s.storage.UpdateGoogleChannelConfig(ctx, req.GetGoogleChannelConfig(), req.GetUpdateMask())
}

// -------------------------------------------------------------------------
// MessageBus RPCs
// -------------------------------------------------------------------------

// GetMessageBus returns the message bus with the given full resource name.
func (s *Server) GetMessageBus(ctx context.Context, req *eventarcpb.GetMessageBusRequest) (*eventarcpb.MessageBus, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.messageBuses.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetMessageBus(ctx, req.GetName())
}

// ListMessageBuses lists message buses under the given parent.
func (s *Server) ListMessageBuses(ctx context.Context, req *eventarcpb.ListMessageBusesRequest) (*eventarcpb.ListMessageBusesResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.messageBuses.list", req.GetParent()); err != nil {
		return nil, err
	}
	buses, nextToken, err := s.storage.ListMessageBuses(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListMessageBusesResponse{
		MessageBuses:  buses,
		NextPageToken: nextToken,
	}, nil
}

// ListMessageBusEnrollments lists enrollments associated with the given message bus.
func (s *Server) ListMessageBusEnrollments(ctx context.Context, req *eventarcpb.ListMessageBusEnrollmentsRequest) (*eventarcpb.ListMessageBusEnrollmentsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.messageBuses.list", req.GetParent()); err != nil {
		return nil, err
	}
	enrollments, nextToken, err := s.storage.ListMessageBusEnrollments(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListMessageBusEnrollmentsResponse{
		Enrollments:   enrollments,
		NextPageToken: nextToken,
	}, nil
}

// CreateMessageBus creates a new message bus and returns a completed LRO.
func (s *Server) CreateMessageBus(ctx context.Context, req *eventarcpb.CreateMessageBusRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetMessageBusId() == "" {
		return nil, status.Error(codes.InvalidArgument, "message_bus_id is required")
	}
	if req.GetMessageBus() == nil {
		return nil, status.Error(codes.InvalidArgument, "message_bus is required")
	}
	if err := s.checkPermission(ctx, "eventarc.messageBuses.create", req.GetParent()); err != nil {
		return nil, err
	}
	bus, err := s.storage.CreateMessageBus(ctx, req.GetParent(), req.GetMessageBusId(), req.GetMessageBus())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), bus)
}

// UpdateMessageBus updates an existing message bus and returns a completed LRO.
func (s *Server) UpdateMessageBus(ctx context.Context, req *eventarcpb.UpdateMessageBusRequest) (*longrunningpb.Operation, error) {
	if req.GetMessageBus() == nil || req.GetMessageBus().GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "message_bus.name is required")
	}
	parent := parentFromResource(req.GetMessageBus().GetName())
	if err := s.checkPermission(ctx, "eventarc.messageBuses.update", req.GetMessageBus().GetName()); err != nil {
		return nil, err
	}
	bus, err := s.storage.UpdateMessageBus(ctx, req.GetMessageBus(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, bus)
}

// DeleteMessageBus deletes an existing message bus and returns a completed LRO.
func (s *Server) DeleteMessageBus(ctx context.Context, req *eventarcpb.DeleteMessageBusRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, "eventarc.messageBuses.delete", req.GetName()); err != nil {
		return nil, err
	}
	bus, err := s.storage.GetMessageBus(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteMessageBus(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, bus)
}

// -------------------------------------------------------------------------
// Enrollment RPCs
// -------------------------------------------------------------------------

// GetEnrollment returns the enrollment with the given full resource name.
func (s *Server) GetEnrollment(ctx context.Context, req *eventarcpb.GetEnrollmentRequest) (*eventarcpb.Enrollment, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.enrollments.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetEnrollment(ctx, req.GetName())
}

// ListEnrollments lists enrollments under the given parent.
func (s *Server) ListEnrollments(ctx context.Context, req *eventarcpb.ListEnrollmentsRequest) (*eventarcpb.ListEnrollmentsResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.enrollments.list", req.GetParent()); err != nil {
		return nil, err
	}
	enrollments, nextToken, err := s.storage.ListEnrollments(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListEnrollmentsResponse{
		Enrollments:   enrollments,
		NextPageToken: nextToken,
	}, nil
}

// CreateEnrollment creates a new enrollment and returns a completed LRO.
func (s *Server) CreateEnrollment(ctx context.Context, req *eventarcpb.CreateEnrollmentRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetEnrollmentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "enrollment_id is required")
	}
	if req.GetEnrollment() == nil {
		return nil, status.Error(codes.InvalidArgument, "enrollment is required")
	}
	if err := s.checkPermission(ctx, "eventarc.enrollments.create", req.GetParent()); err != nil {
		return nil, err
	}
	enrollment, err := s.storage.CreateEnrollment(ctx, req.GetParent(), req.GetEnrollmentId(), req.GetEnrollment())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), enrollment)
}

// UpdateEnrollment updates an existing enrollment and returns a completed LRO.
func (s *Server) UpdateEnrollment(ctx context.Context, req *eventarcpb.UpdateEnrollmentRequest) (*longrunningpb.Operation, error) {
	if req.GetEnrollment() == nil || req.GetEnrollment().GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "enrollment.name is required")
	}
	parent := parentFromResource(req.GetEnrollment().GetName())
	if err := s.checkPermission(ctx, "eventarc.enrollments.update", req.GetEnrollment().GetName()); err != nil {
		return nil, err
	}
	enrollment, err := s.storage.UpdateEnrollment(ctx, req.GetEnrollment(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, enrollment)
}

// DeleteEnrollment deletes an existing enrollment and returns a completed LRO.
func (s *Server) DeleteEnrollment(ctx context.Context, req *eventarcpb.DeleteEnrollmentRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, "eventarc.enrollments.delete", req.GetName()); err != nil {
		return nil, err
	}
	enrollment, err := s.storage.GetEnrollment(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteEnrollment(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, enrollment)
}

// -------------------------------------------------------------------------
// Pipeline RPCs
// -------------------------------------------------------------------------

// GetPipeline returns the pipeline with the given full resource name.
func (s *Server) GetPipeline(ctx context.Context, req *eventarcpb.GetPipelineRequest) (*eventarcpb.Pipeline, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.pipelines.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetPipeline(ctx, req.GetName())
}

// ListPipelines lists pipelines under the given parent.
func (s *Server) ListPipelines(ctx context.Context, req *eventarcpb.ListPipelinesRequest) (*eventarcpb.ListPipelinesResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.pipelines.list", req.GetParent()); err != nil {
		return nil, err
	}
	pipelines, nextToken, err := s.storage.ListPipelines(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListPipelinesResponse{
		Pipelines:     pipelines,
		NextPageToken: nextToken,
	}, nil
}

// CreatePipeline creates a new pipeline and returns a completed LRO.
func (s *Server) CreatePipeline(ctx context.Context, req *eventarcpb.CreatePipelineRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetPipelineId() == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline_id is required")
	}
	if req.GetPipeline() == nil {
		return nil, status.Error(codes.InvalidArgument, "pipeline is required")
	}
	if err := s.checkPermission(ctx, "eventarc.pipelines.create", req.GetParent()); err != nil {
		return nil, err
	}
	pipeline, err := s.storage.CreatePipeline(ctx, req.GetParent(), req.GetPipelineId(), req.GetPipeline())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), pipeline)
}

// UpdatePipeline updates an existing pipeline and returns a completed LRO.
func (s *Server) UpdatePipeline(ctx context.Context, req *eventarcpb.UpdatePipelineRequest) (*longrunningpb.Operation, error) {
	if req.GetPipeline() == nil || req.GetPipeline().GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline.name is required")
	}
	parent := parentFromResource(req.GetPipeline().GetName())
	if err := s.checkPermission(ctx, "eventarc.pipelines.update", req.GetPipeline().GetName()); err != nil {
		return nil, err
	}
	pipeline, err := s.storage.UpdatePipeline(ctx, req.GetPipeline(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, pipeline)
}

// DeletePipeline deletes an existing pipeline and returns a completed LRO.
func (s *Server) DeletePipeline(ctx context.Context, req *eventarcpb.DeletePipelineRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, "eventarc.pipelines.delete", req.GetName()); err != nil {
		return nil, err
	}
	pipeline, err := s.storage.GetPipeline(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeletePipeline(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, pipeline)
}

// -------------------------------------------------------------------------
// GoogleApiSource RPCs
// -------------------------------------------------------------------------

// GetGoogleApiSource returns the google api source with the given full resource name.
func (s *Server) GetGoogleApiSource(ctx context.Context, req *eventarcpb.GetGoogleApiSourceRequest) (*eventarcpb.GoogleApiSource, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if err := s.checkPermission(ctx, "eventarc.googleApiSources.get", req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetGoogleApiSource(ctx, req.GetName())
}

// ListGoogleApiSources lists google api sources under the given parent.
func (s *Server) ListGoogleApiSources(ctx context.Context, req *eventarcpb.ListGoogleApiSourcesRequest) (*eventarcpb.ListGoogleApiSourcesResponse, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if err := s.checkPermission(ctx, "eventarc.googleApiSources.list", req.GetParent()); err != nil {
		return nil, err
	}
	sources, nextToken, err := s.storage.ListGoogleApiSources(ctx, req.GetParent(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return &eventarcpb.ListGoogleApiSourcesResponse{
		GoogleApiSources: sources,
		NextPageToken:    nextToken,
	}, nil
}

// CreateGoogleApiSource creates a new google api source and returns a completed LRO.
func (s *Server) CreateGoogleApiSource(ctx context.Context, req *eventarcpb.CreateGoogleApiSourceRequest) (*longrunningpb.Operation, error) {
	if req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent is required")
	}
	if req.GetGoogleApiSourceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "google_api_source_id is required")
	}
	if req.GetGoogleApiSource() == nil {
		return nil, status.Error(codes.InvalidArgument, "google_api_source is required")
	}
	if err := s.checkPermission(ctx, "eventarc.googleApiSources.create", req.GetParent()); err != nil {
		return nil, err
	}
	source, err := s.storage.CreateGoogleApiSource(ctx, req.GetParent(), req.GetGoogleApiSourceId(), req.GetGoogleApiSource())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), source)
}

// UpdateGoogleApiSource updates an existing google api source and returns a completed LRO.
func (s *Server) UpdateGoogleApiSource(ctx context.Context, req *eventarcpb.UpdateGoogleApiSourceRequest) (*longrunningpb.Operation, error) {
	if req.GetGoogleApiSource() == nil || req.GetGoogleApiSource().GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "google_api_source.name is required")
	}
	parent := parentFromResource(req.GetGoogleApiSource().GetName())
	if err := s.checkPermission(ctx, "eventarc.googleApiSources.update", req.GetGoogleApiSource().GetName()); err != nil {
		return nil, err
	}
	source, err := s.storage.UpdateGoogleApiSource(ctx, req.GetGoogleApiSource(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, source)
}

// DeleteGoogleApiSource deletes an existing google api source and returns a completed LRO.
func (s *Server) DeleteGoogleApiSource(ctx context.Context, req *eventarcpb.DeleteGoogleApiSourceRequest) (*longrunningpb.Operation, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, "eventarc.googleApiSources.delete", req.GetName()); err != nil {
		return nil, err
	}
	src, err := s.storage.GetGoogleApiSource(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteGoogleApiSource(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, src)
}
