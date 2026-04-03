// Package server implements the GCP Eventarc v1 gRPC API as a local emulator.
//
// It provides a complete in-memory implementation of all 47 Eventarc RPCs across
// 8 resource types (Trigger, Channel, ChannelConnection, GoogleChannelConfig,
// MessageBus, Enrollment, Pipeline, GoogleApiSource) plus the Provider read API,
// the Publishing service, and the Operations service for LRO polling.
//
// All mutating RPCs return a google.longrunning.Operation resolved immediately
// (Done: true). Storage is thread-safe via sync.RWMutex; all returned proto
// messages are deep-copied with proto.Clone to prevent aliasing.
//
// For standalone use, see cmd/server, cmd/server-rest, or cmd/server-dual.
// For in-process testing, create a server with NewServer() and register it
// on a bufconn listener — see integration_test.go for a complete example.
package server

import (
	"context"
	"fmt"
	"strings"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	emulatorauth "github.com/blackwell-systems/gcp-emulator-auth"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/authz"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/lro"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
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

	// Permissive mode: no principal → deny (matches documented behavior table).
	if principal == "" && string(s.iamMode) == "permissive" {
		return status.Errorf(codes.PermissionDenied,
			"permissive mode requires authentication: no principal in request")
	}

	allowed, err := s.iamClient.CheckPermission(ctx, principal, resource, permission)
	if err != nil {
		// Detect connection-refused / unavailable errors and return a user-friendly message.
		errMsg := err.Error()
		if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "Unavailable") {
			return status.Errorf(codes.Unavailable,
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

// requireField returns a gRPC InvalidArgument error when value is empty.
// fieldName is used in the error message, e.g. "name is required".
func requireField(value, fieldName string) error {
	if value == "" {
		return status.Errorf(codes.InvalidArgument, "%s is required", fieldName)
	}
	return nil
}

// perm returns the IAM permission string for the named Eventarc operation.
// It panics if the operation is not registered in the authz package, which
// indicates a programming error (missing permission constant).
func perm(operation string) string {
	p, ok := authz.GetPermission(operation)
	if !ok {
		panic("authz: unknown operation: " + operation)
	}
	return p.Permission
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

// isNotFound reports whether err is a gRPC NotFound status.
func isNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

// lastSegment returns the last "/"-delimited segment of a resource name.
func lastSegment(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx < 0 {
		return name
	}
	return name[idx+1:]
}

// -------------------------------------------------------------------------
// Trigger RPCs
// -------------------------------------------------------------------------

// GetTrigger returns the trigger with the given full resource name.
func (s *Server) GetTrigger(ctx context.Context, req *eventarcpb.GetTriggerRequest) (*eventarcpb.Trigger, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, perm("GetTrigger"), req.GetName()); err != nil {
		return nil, err
	}

	return s.storage.GetTrigger(ctx, req.GetName())
}

// ListTriggers lists triggers under the given parent with optional pagination.
func (s *Server) ListTriggers(ctx context.Context, req *eventarcpb.ListTriggersRequest) (*eventarcpb.ListTriggersResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, perm("ListTriggers"), req.GetParent()); err != nil {
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
	var violations []*errdetails.BadRequest_FieldViolation
	if req.GetParent() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{Field: "parent", Description: "parent is required"})
	}
	if req.GetTriggerId() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{Field: "triggerId", Description: "triggerId is required"})
	}
	if req.GetTrigger() == nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{Field: "trigger", Description: "trigger is required"})
	} else {
		if req.GetTrigger().GetDestination() == nil {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{Field: "trigger.destination", Description: "trigger.destination is required"})
		}
		if len(req.GetTrigger().GetEventFilters()) == 0 {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{Field: "trigger.eventFilters", Description: "trigger.event_filters must not be empty"})
		}
	}
	if len(violations) > 0 {
		st := status.New(codes.InvalidArgument, violations[0].Description)
		if br, err := st.WithDetails(&errdetails.BadRequest{FieldViolations: violations}); err == nil {
			return nil, br.Err()
		}
		return nil, st.Err()
	}

	if err := s.checkPermission(ctx, perm("CreateTrigger"), req.GetParent()); err != nil {
		return nil, err
	}

	if req.GetValidateOnly() {
		synthetic := proto.Clone(req.GetTrigger()).(*eventarcpb.Trigger)
		synthetic.Name = fmt.Sprintf("%s/triggers/%s", req.GetParent(), req.GetTriggerId())
		return s.lro.CreateDone(req.GetParent(), synthetic, "create", synthetic.Name)
	}

	trigger, err := s.storage.CreateTrigger(ctx, req.GetParent(), req.GetTriggerId(), req.GetTrigger())
	if err != nil {
		return nil, err
	}

	return s.lro.CreateDone(req.GetParent(), trigger, "create", trigger.GetName())
}

// UpdateTrigger updates an existing trigger and returns a completed LRO.
func (s *Server) UpdateTrigger(ctx context.Context, req *eventarcpb.UpdateTriggerRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetTrigger().GetName(), "trigger.name"); err != nil {
		return nil, err
	}

	parent := parentFromName(req.GetTrigger().GetName())

	if err := s.checkPermission(ctx, perm("UpdateTrigger"), req.GetTrigger().GetName()); err != nil {
		return nil, err
	}

	if req.GetValidateOnly() {
		existing, err := s.storage.GetTrigger(ctx, req.GetTrigger().GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, existing, "update", req.GetTrigger().GetName())
	}

	trigger, err := s.storage.UpdateTrigger(ctx, req.GetTrigger(), req.GetUpdateMask())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			trigger, err = s.storage.CreateTrigger(ctx,
				parent, lastSegment(req.GetTrigger().GetName()), req.GetTrigger())
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return s.lro.CreateDone(parent, trigger, "update", req.GetTrigger().GetName())
}

// DeleteTrigger deletes an existing trigger and returns a completed LRO.
func (s *Server) DeleteTrigger(ctx context.Context, req *eventarcpb.DeleteTriggerRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}

	parent := parentFromName(req.GetName())

	if err := s.checkPermission(ctx, perm("DeleteTrigger"), req.GetName()); err != nil {
		return nil, err
	}

	if req.GetValidateOnly() {
		_, err := s.storage.GetTrigger(ctx, req.GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
	}

	existing, err := s.storage.GetTrigger(ctx, req.GetName())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
		}
		return nil, err
	}
	if reqEtag := req.GetEtag(); reqEtag != "" {
		if existing.GetEtag() != reqEtag {
			return nil, status.Errorf(codes.Aborted,
				"etag mismatch: provided %q does not match stored %q",
				reqEtag, existing.GetEtag())
		}
	}
	if err := s.storage.DeleteTrigger(ctx, req.GetName()); err != nil {
		return nil, err
	}

	return s.lro.CreateDone(parent, existing, "delete", req.GetName())
}

// -------------------------------------------------------------------------
// Provider RPCs (read-only, no LRO)
// -------------------------------------------------------------------------

// GetProvider returns the provider with the given full resource name.
func (s *Server) GetProvider(ctx context.Context, req *eventarcpb.GetProviderRequest) (*eventarcpb.Provider, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, perm("GetProvider"), req.GetName()); err != nil {
		return nil, err
	}

	return s.storage.GetProvider(ctx, req.GetName())
}

// ListProviders lists providers under the given parent.
func (s *Server) ListProviders(ctx context.Context, req *eventarcpb.ListProvidersRequest) (*eventarcpb.ListProvidersResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, perm("ListProviders"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetChannel"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetChannel(ctx, req.GetName())
}

// ListChannels lists channels under the given parent.
func (s *Server) ListChannels(ctx context.Context, req *eventarcpb.ListChannelsRequest) (*eventarcpb.ListChannelsResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListChannels"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := requireField(req.GetChannelId(), "channel_id"); err != nil {
		return nil, err
	}
	if req.GetChannel() == nil {
		return nil, status.Error(codes.InvalidArgument, "channel is required")
	}
	if err := s.checkPermission(ctx, perm("CreateChannel"), req.GetParent()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		synthetic := proto.Clone(req.GetChannel()).(*eventarcpb.Channel)
		synthetic.Name = fmt.Sprintf("%s/channels/%s", req.GetParent(), req.GetChannelId())
		return s.lro.CreateDone(req.GetParent(), synthetic, "create", synthetic.Name)
	}
	channel, err := s.storage.CreateChannel(ctx, req.GetParent(), req.GetChannelId(), req.GetChannel())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), channel, "create", channel.GetName())
}

// UpdateChannel updates an existing channel and returns a completed LRO.
func (s *Server) UpdateChannel(ctx context.Context, req *eventarcpb.UpdateChannelRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetChannel().GetName(), "channel.name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetChannel().GetName())
	if err := s.checkPermission(ctx, perm("UpdateChannel"), req.GetChannel().GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		existing, err := s.storage.GetChannel(ctx, req.GetChannel().GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, existing, "update", req.GetChannel().GetName())
	}
	channel, err := s.storage.UpdateChannel(ctx, req.GetChannel(), req.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, channel, "update", channel.GetName())
}

// DeleteChannel deletes an existing channel and returns a completed LRO.
func (s *Server) DeleteChannel(ctx context.Context, req *eventarcpb.DeleteChannelRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, perm("DeleteChannel"), req.GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		_, err := s.storage.GetChannel(ctx, req.GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
	}
	channel, err := s.storage.GetChannel(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteChannel(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, channel, "delete", channel.GetName())
}

// -------------------------------------------------------------------------
// ChannelConnection RPCs
// -------------------------------------------------------------------------

// GetChannelConnection returns the channel connection with the given full resource name.
func (s *Server) GetChannelConnection(ctx context.Context, req *eventarcpb.GetChannelConnectionRequest) (*eventarcpb.ChannelConnection, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetChannelConnection"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetChannelConnection(ctx, req.GetName())
}

// ListChannelConnections lists channel connections under the given parent.
func (s *Server) ListChannelConnections(ctx context.Context, req *eventarcpb.ListChannelConnectionsRequest) (*eventarcpb.ListChannelConnectionsResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListChannelConnections"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := requireField(req.GetChannelConnectionId(), "channel_connection_id"); err != nil {
		return nil, err
	}
	if req.GetChannelConnection() == nil {
		return nil, status.Error(codes.InvalidArgument, "channel_connection is required")
	}
	if err := s.checkPermission(ctx, perm("CreateChannelConnection"), req.GetParent()); err != nil {
		return nil, err
	}
	conn, err := s.storage.CreateChannelConnection(ctx, req.GetParent(), req.GetChannelConnectionId(), req.GetChannelConnection())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), conn, "create", conn.GetName())
}

// DeleteChannelConnection deletes an existing channel connection and returns a completed LRO.
func (s *Server) DeleteChannelConnection(ctx context.Context, req *eventarcpb.DeleteChannelConnectionRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, perm("DeleteChannelConnection"), req.GetName()); err != nil {
		return nil, err
	}
	conn, err := s.storage.GetChannelConnection(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.storage.DeleteChannelConnection(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, conn, "delete", conn.GetName())
}

// -------------------------------------------------------------------------
// GoogleChannelConfig RPCs
// -------------------------------------------------------------------------

// GetGoogleChannelConfig returns the google channel config for the given name (singleton).
func (s *Server) GetGoogleChannelConfig(ctx context.Context, req *eventarcpb.GetGoogleChannelConfigRequest) (*eventarcpb.GoogleChannelConfig, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetGoogleChannelConfig"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetGoogleChannelConfig(ctx, req.GetName())
}

// UpdateGoogleChannelConfig updates the google channel config and returns it directly (no LRO).
func (s *Server) UpdateGoogleChannelConfig(ctx context.Context, req *eventarcpb.UpdateGoogleChannelConfigRequest) (*eventarcpb.GoogleChannelConfig, error) {
	if req.GetGoogleChannelConfig() == nil {
		return nil, status.Error(codes.InvalidArgument, "google_channel_config is required")
	}
	if req.GetGoogleChannelConfig().GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "google_channel_config.name is required")
	}
	if err := s.checkPermission(ctx, perm("UpdateGoogleChannelConfig"), req.GetGoogleChannelConfig().GetName()); err != nil {
		return nil, err
	}
	return s.storage.UpdateGoogleChannelConfig(ctx, req.GetGoogleChannelConfig(), req.GetUpdateMask())
}

// -------------------------------------------------------------------------
// MessageBus RPCs
// -------------------------------------------------------------------------

// GetMessageBus returns the message bus with the given full resource name.
func (s *Server) GetMessageBus(ctx context.Context, req *eventarcpb.GetMessageBusRequest) (*eventarcpb.MessageBus, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetMessageBus"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetMessageBus(ctx, req.GetName())
}

// ListMessageBuses lists message buses under the given parent.
func (s *Server) ListMessageBuses(ctx context.Context, req *eventarcpb.ListMessageBusesRequest) (*eventarcpb.ListMessageBusesResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListMessageBuses"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListMessageBusEnrollments"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := requireField(req.GetMessageBusId(), "message_bus_id"); err != nil {
		return nil, err
	}
	if req.GetMessageBus() == nil {
		return nil, status.Error(codes.InvalidArgument, "message_bus is required")
	}
	if err := s.checkPermission(ctx, perm("CreateMessageBus"), req.GetParent()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		synthetic := proto.Clone(req.GetMessageBus()).(*eventarcpb.MessageBus)
		synthetic.Name = fmt.Sprintf("%s/messageBuses/%s", req.GetParent(), req.GetMessageBusId())
		return s.lro.CreateDone(req.GetParent(), synthetic, "create", synthetic.Name)
	}
	bus, err := s.storage.CreateMessageBus(ctx, req.GetParent(), req.GetMessageBusId(), req.GetMessageBus())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), bus, "create", bus.GetName())
}

// UpdateMessageBus updates an existing message bus and returns a completed LRO.
func (s *Server) UpdateMessageBus(ctx context.Context, req *eventarcpb.UpdateMessageBusRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetMessageBus().GetName(), "message_bus.name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetMessageBus().GetName())
	if err := s.checkPermission(ctx, perm("UpdateMessageBus"), req.GetMessageBus().GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		existing, err := s.storage.GetMessageBus(ctx, req.GetMessageBus().GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, existing, "update", req.GetMessageBus().GetName())
	}
	bus, err := s.storage.UpdateMessageBus(ctx, req.GetMessageBus(), req.GetUpdateMask())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			bus, err = s.storage.CreateMessageBus(ctx,
				parent, lastSegment(req.GetMessageBus().GetName()), req.GetMessageBus())
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return s.lro.CreateDone(parent, bus, "update", req.GetMessageBus().GetName())
}

// DeleteMessageBus deletes an existing message bus and returns a completed LRO.
func (s *Server) DeleteMessageBus(ctx context.Context, req *eventarcpb.DeleteMessageBusRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, perm("DeleteMessageBus"), req.GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		_, err := s.storage.GetMessageBus(ctx, req.GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
	}
	existing, err := s.storage.GetMessageBus(ctx, req.GetName())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
		}
		return nil, err
	}
	if reqEtag := req.GetEtag(); reqEtag != "" {
		if existing.GetEtag() != reqEtag {
			return nil, status.Errorf(codes.Aborted,
				"etag mismatch: provided %q does not match stored %q",
				reqEtag, existing.GetEtag())
		}
	}
	if err := s.storage.DeleteMessageBus(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, existing, "delete", req.GetName())
}

// -------------------------------------------------------------------------
// Enrollment RPCs
// -------------------------------------------------------------------------

// GetEnrollment returns the enrollment with the given full resource name.
func (s *Server) GetEnrollment(ctx context.Context, req *eventarcpb.GetEnrollmentRequest) (*eventarcpb.Enrollment, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetEnrollment"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetEnrollment(ctx, req.GetName())
}

// ListEnrollments lists enrollments under the given parent.
func (s *Server) ListEnrollments(ctx context.Context, req *eventarcpb.ListEnrollmentsRequest) (*eventarcpb.ListEnrollmentsResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListEnrollments"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := requireField(req.GetEnrollmentId(), "enrollment_id"); err != nil {
		return nil, err
	}
	if req.GetEnrollment() == nil {
		return nil, status.Error(codes.InvalidArgument, "enrollment is required")
	}
	if req.GetEnrollment().GetCelMatch() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "enrollment.cel_match is required")
	}
	if err := s.checkPermission(ctx, perm("CreateEnrollment"), req.GetParent()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		synthetic := proto.Clone(req.GetEnrollment()).(*eventarcpb.Enrollment)
		synthetic.Name = fmt.Sprintf("%s/enrollments/%s", req.GetParent(), req.GetEnrollmentId())
		return s.lro.CreateDone(req.GetParent(), synthetic, "create", synthetic.Name)
	}
	enrollment, err := s.storage.CreateEnrollment(ctx, req.GetParent(), req.GetEnrollmentId(), req.GetEnrollment())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), enrollment, "create", enrollment.GetName())
}

// UpdateEnrollment updates an existing enrollment and returns a completed LRO.
func (s *Server) UpdateEnrollment(ctx context.Context, req *eventarcpb.UpdateEnrollmentRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetEnrollment().GetName(), "enrollment.name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetEnrollment().GetName())
	if err := s.checkPermission(ctx, perm("UpdateEnrollment"), req.GetEnrollment().GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		existing, err := s.storage.GetEnrollment(ctx, req.GetEnrollment().GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, existing, "update", req.GetEnrollment().GetName())
	}
	enrollment, err := s.storage.UpdateEnrollment(ctx, req.GetEnrollment(), req.GetUpdateMask())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			enrollment, err = s.storage.CreateEnrollment(ctx,
				parent, lastSegment(req.GetEnrollment().GetName()), req.GetEnrollment())
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return s.lro.CreateDone(parent, enrollment, "update", req.GetEnrollment().GetName())
}

// DeleteEnrollment deletes an existing enrollment and returns a completed LRO.
func (s *Server) DeleteEnrollment(ctx context.Context, req *eventarcpb.DeleteEnrollmentRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, perm("DeleteEnrollment"), req.GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		_, err := s.storage.GetEnrollment(ctx, req.GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
	}
	existing, err := s.storage.GetEnrollment(ctx, req.GetName())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
		}
		return nil, err
	}
	if reqEtag := req.GetEtag(); reqEtag != "" {
		if existing.GetEtag() != reqEtag {
			return nil, status.Errorf(codes.Aborted,
				"etag mismatch: provided %q does not match stored %q",
				reqEtag, existing.GetEtag())
		}
	}
	if err := s.storage.DeleteEnrollment(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, existing, "delete", req.GetName())
}

// -------------------------------------------------------------------------
// Pipeline RPCs
// -------------------------------------------------------------------------

// GetPipeline returns the pipeline with the given full resource name.
func (s *Server) GetPipeline(ctx context.Context, req *eventarcpb.GetPipelineRequest) (*eventarcpb.Pipeline, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetPipeline"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetPipeline(ctx, req.GetName())
}

// ListPipelines lists pipelines under the given parent.
func (s *Server) ListPipelines(ctx context.Context, req *eventarcpb.ListPipelinesRequest) (*eventarcpb.ListPipelinesResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListPipelines"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := requireField(req.GetPipelineId(), "pipeline_id"); err != nil {
		return nil, err
	}
	if req.GetPipeline() == nil {
		return nil, status.Error(codes.InvalidArgument, "pipeline is required")
	}
	if len(req.GetPipeline().GetDestinations()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "pipeline.destinations must not be empty")
	}
	if err := s.checkPermission(ctx, perm("CreatePipeline"), req.GetParent()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		synthetic := proto.Clone(req.GetPipeline()).(*eventarcpb.Pipeline)
		synthetic.Name = fmt.Sprintf("%s/pipelines/%s", req.GetParent(), req.GetPipelineId())
		return s.lro.CreateDone(req.GetParent(), synthetic, "create", synthetic.Name)
	}
	pipeline, err := s.storage.CreatePipeline(ctx, req.GetParent(), req.GetPipelineId(), req.GetPipeline())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), pipeline, "create", pipeline.GetName())
}

// UpdatePipeline updates an existing pipeline and returns a completed LRO.
func (s *Server) UpdatePipeline(ctx context.Context, req *eventarcpb.UpdatePipelineRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetPipeline().GetName(), "pipeline.name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetPipeline().GetName())
	if err := s.checkPermission(ctx, perm("UpdatePipeline"), req.GetPipeline().GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		existing, err := s.storage.GetPipeline(ctx, req.GetPipeline().GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, existing, "update", req.GetPipeline().GetName())
	}
	pipeline, err := s.storage.UpdatePipeline(ctx, req.GetPipeline(), req.GetUpdateMask())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			pipeline, err = s.storage.CreatePipeline(ctx,
				parent, lastSegment(req.GetPipeline().GetName()), req.GetPipeline())
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return s.lro.CreateDone(parent, pipeline, "update", req.GetPipeline().GetName())
}

// DeletePipeline deletes an existing pipeline and returns a completed LRO.
func (s *Server) DeletePipeline(ctx context.Context, req *eventarcpb.DeletePipelineRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, perm("DeletePipeline"), req.GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		_, err := s.storage.GetPipeline(ctx, req.GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
	}
	existing, err := s.storage.GetPipeline(ctx, req.GetName())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
		}
		return nil, err
	}
	if reqEtag := req.GetEtag(); reqEtag != "" {
		if existing.GetEtag() != reqEtag {
			return nil, status.Errorf(codes.Aborted,
				"etag mismatch: provided %q does not match stored %q",
				reqEtag, existing.GetEtag())
		}
	}
	if err := s.storage.DeletePipeline(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, existing, "delete", req.GetName())
}

// -------------------------------------------------------------------------
// GoogleApiSource RPCs
// -------------------------------------------------------------------------

// GetGoogleApiSource returns the google api source with the given full resource name.
func (s *Server) GetGoogleApiSource(ctx context.Context, req *eventarcpb.GetGoogleApiSourceRequest) (*eventarcpb.GoogleApiSource, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("GetGoogleApiSource"), req.GetName()); err != nil {
		return nil, err
	}
	return s.storage.GetGoogleApiSource(ctx, req.GetName())
}

// ListGoogleApiSources lists google api sources under the given parent.
func (s *Server) ListGoogleApiSources(ctx context.Context, req *eventarcpb.ListGoogleApiSourcesRequest) (*eventarcpb.ListGoogleApiSourcesResponse, error) {
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, perm("ListGoogleApiSources"), req.GetParent()); err != nil {
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
	if err := requireField(req.GetParent(), "parent"); err != nil {
		return nil, err
	}
	if err := requireField(req.GetGoogleApiSourceId(), "google_api_source_id"); err != nil {
		return nil, err
	}
	if req.GetGoogleApiSource() == nil {
		return nil, status.Error(codes.InvalidArgument, "google_api_source is required")
	}
	if req.GetGoogleApiSource().GetDestination() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "google_api_source.destination is required")
	}
	if err := s.checkPermission(ctx, perm("CreateGoogleApiSource"), req.GetParent()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		synthetic := proto.Clone(req.GetGoogleApiSource()).(*eventarcpb.GoogleApiSource)
		synthetic.Name = fmt.Sprintf("%s/googleApiSources/%s", req.GetParent(), req.GetGoogleApiSourceId())
		return s.lro.CreateDone(req.GetParent(), synthetic, "create", synthetic.Name)
	}
	source, err := s.storage.CreateGoogleApiSource(ctx, req.GetParent(), req.GetGoogleApiSourceId(), req.GetGoogleApiSource())
	if err != nil {
		return nil, err
	}
	return s.lro.CreateDone(req.GetParent(), source, "create", source.GetName())
}

// UpdateGoogleApiSource updates an existing google api source and returns a completed LRO.
func (s *Server) UpdateGoogleApiSource(ctx context.Context, req *eventarcpb.UpdateGoogleApiSourceRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetGoogleApiSource().GetName(), "google_api_source.name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetGoogleApiSource().GetName())
	if err := s.checkPermission(ctx, perm("UpdateGoogleApiSource"), req.GetGoogleApiSource().GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		existing, err := s.storage.GetGoogleApiSource(ctx, req.GetGoogleApiSource().GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, existing, "update", req.GetGoogleApiSource().GetName())
	}
	source, err := s.storage.UpdateGoogleApiSource(ctx, req.GetGoogleApiSource(), req.GetUpdateMask())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			source, err = s.storage.CreateGoogleApiSource(ctx,
				parent, lastSegment(req.GetGoogleApiSource().GetName()), req.GetGoogleApiSource())
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return s.lro.CreateDone(parent, source, "update", req.GetGoogleApiSource().GetName())
}

// DeleteGoogleApiSource deletes an existing google api source and returns a completed LRO.
func (s *Server) DeleteGoogleApiSource(ctx context.Context, req *eventarcpb.DeleteGoogleApiSourceRequest) (*longrunningpb.Operation, error) {
	if err := requireField(req.GetName(), "name"); err != nil {
		return nil, err
	}
	parent := parentFromResource(req.GetName())
	if err := s.checkPermission(ctx, perm("DeleteGoogleApiSource"), req.GetName()); err != nil {
		return nil, err
	}
	if req.GetValidateOnly() {
		_, err := s.storage.GetGoogleApiSource(ctx, req.GetName())
		if err != nil {
			return nil, err
		}
		return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
	}
	existing, err := s.storage.GetGoogleApiSource(ctx, req.GetName())
	if err != nil {
		if req.GetAllowMissing() && isNotFound(err) {
			return s.lro.CreateDone(parent, &emptypb.Empty{}, "delete", req.GetName())
		}
		return nil, err
	}
	if reqEtag := req.GetEtag(); reqEtag != "" {
		if existing.GetEtag() != reqEtag {
			return nil, status.Errorf(codes.Aborted,
				"etag mismatch: provided %q does not match stored %q",
				reqEtag, existing.GetEtag())
		}
	}
	if err := s.storage.DeleteGoogleApiSource(ctx, req.GetName()); err != nil {
		return nil, err
	}
	return s.lro.CreateDone(parent, existing, "delete", req.GetName())
}
