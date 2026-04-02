// Package lro implements a long-running operation store for the GCP Eventarc
// emulator. All operations are resolved immediately (Done: true) — no async
// polling is required for local development.
//
// Resolved import path: cloud.google.com/go/longrunning@v0.8.0
// Sub-package:          cloud.google.com/go/longrunning/autogen/longrunningpb
package lro

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Store manages long-running operations and implements OperationsServer.
// In emulator mode all operations complete immediately (Done: true).
type Store struct {
	longrunningpb.UnimplementedOperationsServer

	mu  sync.RWMutex
	ops map[string]*longrunningpb.Operation // key: operation name
}

// NewStore creates an empty LRO store.
func NewStore() *Store {
	return &Store{
		ops: make(map[string]*longrunningpb.Operation),
	}
}

// CreateDone creates a pre-resolved operation with the given resource as
// response. The operation name is formatted as:
//
//	{parent}/operations/{uuid}
//
// where uuid is a random hex string for traceability.
func (s *Store) CreateDone(parent string, resource proto.Message) (*longrunningpb.Operation, error) {
	uuid, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("lro: generate uuid: %w", err)
	}

	name := parent + "/operations/" + uuid

	anyVal, err := anypb.New(resource)
	if err != nil {
		return nil, fmt.Errorf("lro: pack resource into anypb: %w", err)
	}

	op := &longrunningpb.Operation{
		Name: name,
		Done: true,
		Result: &longrunningpb.Operation_Response{
			Response: anyVal,
		},
	}

	s.mu.Lock()
	s.ops[name] = op
	s.mu.Unlock()

	return op, nil
}

// GetOperation implements OperationsServer. Returns the operation by name or
// NotFound if it does not exist.
func (s *Store) GetOperation(_ context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	s.mu.RLock()
	op, ok := s.ops[req.GetName()]
	s.mu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "operation %q not found", req.GetName())
	}
	return op, nil
}

// DeleteOperation implements OperationsServer. Removes the operation or
// returns NotFound if it does not exist.
func (s *Store) DeleteOperation(_ context.Context, req *longrunningpb.DeleteOperationRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	_, ok := s.ops[req.GetName()]
	if ok {
		delete(s.ops, req.GetName())
	}
	s.mu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "operation %q not found", req.GetName())
	}
	return &emptypb.Empty{}, nil
}

// CancelOperation implements OperationsServer. All operations are already
// done, so this is a no-op that returns success.
func (s *Store) CancelOperation(_ context.Context, _ *longrunningpb.CancelOperationRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// ListOperations implements OperationsServer. Returns all operations whose
// name has the given req.Name as a prefix.
func (s *Store) ListOperations(_ context.Context, req *longrunningpb.ListOperationsRequest) (*longrunningpb.ListOperationsResponse, error) {
	prefix := req.GetName()

	s.mu.RLock()
	var ops []*longrunningpb.Operation
	for name, op := range s.ops {
		if prefix == "" || strings.HasPrefix(name, prefix) {
			ops = append(ops, op)
		}
	}
	s.mu.RUnlock()

	return &longrunningpb.ListOperationsResponse{
		Operations: ops,
	}, nil
}

// WaitOperation implements OperationsServer. Because all operations are
// already done, this simply delegates to GetOperation.
func (s *Store) WaitOperation(ctx context.Context, req *longrunningpb.WaitOperationRequest) (*longrunningpb.Operation, error) {
	return s.GetOperation(ctx, &longrunningpb.GetOperationRequest{
		Name: req.GetName(),
	})
}

// randomHex returns n random bytes encoded as a hex string.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
