// Package longrunning provides grpc-gateway handler registration for the
// google.longrunning.Operations API.
//
// This file re-exports gRPC service interfaces and request types from
// cloud.google.com/go/longrunning/autogen/longrunningpb so that the generated
// gateway file (operations.pb.gw.go) compiles without also generating pb.go.
package longrunning

import (
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/grpc"
)

// gRPC service interfaces and constructors.
type OperationsClient = longrunningpb.OperationsClient
type OperationsServer = longrunningpb.OperationsServer

func NewOperationsClient(cc grpc.ClientConnInterface) OperationsClient {
	return longrunningpb.NewOperationsClient(cc)
}

// Request types used by the gateway handlers.
type ListOperationsRequest = longrunningpb.ListOperationsRequest
type GetOperationRequest = longrunningpb.GetOperationRequest
type DeleteOperationRequest = longrunningpb.DeleteOperationRequest
type CancelOperationRequest = longrunningpb.CancelOperationRequest
type WaitOperationRequest = longrunningpb.WaitOperationRequest
