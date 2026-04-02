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

// checkPermission verifies the caller has permission to perform the operation.
// If iamClient is nil (IAM disabled), all requests are allowed.
func (s *Server) checkPermission(ctx context.Context, permission string, resource string) error {
	if s.iamClient == nil {
		return nil // IAM disabled, allow all
	}

	principal := emulatorauth.ExtractPrincipalFromContext(ctx)

	allowed, err := s.iamClient.CheckPermission(ctx, principal, resource, permission)
	if err != nil {
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

	if err := s.storage.DeleteTrigger(ctx, req.GetName()); err != nil {
		return nil, err
	}

	return s.lro.CreateDone(parent, &emptypb.Empty{})
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
