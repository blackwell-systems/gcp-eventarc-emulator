// Package server implements the Eventarc gRPC server.
// NOTE: This stub is created by Wave 1 Agent A solely to satisfy the pre-commit
// lint gate (go vet ./...). Wave 2 Agent B owns this package and will replace
// this file with the full implementation.
package server

import "cloud.google.com/go/eventarc/apiv1/eventarcpb"

// Server implements the Eventarc gRPC server.
type Server struct {
	eventarcpb.UnimplementedEventarcServer
}

// NewServer creates a new Server instance.
func NewServer() (*Server, error) {
	return &Server{}, nil
}
