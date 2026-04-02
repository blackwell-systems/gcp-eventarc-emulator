package lro_test

import (
	"context"
	"strings"
	"testing"

	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/lro"
)

const testParent = "projects/my-project/locations/us-central1"

// TestCreateDone_ReturnsImmediatelyDone verifies that CreateDone returns an
// operation with Done: true and the name embedded under the parent path.
func TestCreateDone_ReturnsImmediatelyDone(t *testing.T) {
	s := lro.NewStore()

	op, err := s.CreateDone(testParent, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("CreateDone: unexpected error: %v", err)
	}
	if !op.Done {
		t.Errorf("expected Done=true, got false")
	}
	if !strings.HasPrefix(op.Name, testParent+"/operations/") {
		t.Errorf("operation name %q does not have expected prefix %q", op.Name, testParent+"/operations/")
	}
}

// TestCreateDone_ResponseUnpackable verifies that the response packed into
// anypb.Any can be unpacked back to the original message type.
func TestCreateDone_ResponseUnpackable(t *testing.T) {
	s := lro.NewStore()

	op, err := s.CreateDone(testParent, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("CreateDone: unexpected error: %v", err)
	}

	respResult, ok := op.Result.(*longrunningpb.Operation_Response)
	if !ok {
		t.Fatalf("expected Operation_Response result, got %T", op.Result)
	}

	var got emptypb.Empty
	if err := respResult.Response.UnmarshalTo(&got); err != nil {
		t.Errorf("UnmarshalTo: %v", err)
	}
}

// TestGetOperation_NotFound verifies that GetOperation returns a NotFound
// gRPC status for an operation name that was never created.
func TestGetOperation_NotFound(t *testing.T) {
	s := lro.NewStore()

	_, err := s.GetOperation(context.Background(), &longrunningpb.GetOperationRequest{
		Name: testParent + "/operations/nonexistent",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected codes.NotFound, got %v", st.Code())
	}
}

// TestDeleteOperation_NotFound verifies that DeleteOperation returns NotFound
// when the named operation does not exist.
func TestDeleteOperation_NotFound(t *testing.T) {
	s := lro.NewStore()

	_, err := s.DeleteOperation(context.Background(), &longrunningpb.DeleteOperationRequest{
		Name: testParent + "/operations/nonexistent",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected codes.NotFound, got %v", st.Code())
	}
}

// TestCancelOperation_NoOp verifies that CancelOperation is a no-op that
// always returns success regardless of whether the operation exists.
func TestCancelOperation_NoOp(t *testing.T) {
	s := lro.NewStore()

	// Cancel a nonexistent operation — should still succeed.
	_, err := s.CancelOperation(context.Background(), &longrunningpb.CancelOperationRequest{
		Name: testParent + "/operations/nonexistent",
	})
	if err != nil {
		t.Errorf("CancelOperation (nonexistent): unexpected error: %v", err)
	}

	// Cancel an existing operation — should also succeed.
	op, err := s.CreateDone(testParent, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("CreateDone: %v", err)
	}
	_, err = s.CancelOperation(context.Background(), &longrunningpb.CancelOperationRequest{
		Name: op.Name,
	})
	if err != nil {
		t.Errorf("CancelOperation (existing): unexpected error: %v", err)
	}
}

// TestWaitOperation_ReturnsImmediately verifies that WaitOperation returns the
// same operation as GetOperation without blocking.
func TestWaitOperation_ReturnsImmediately(t *testing.T) {
	s := lro.NewStore()

	op, err := s.CreateDone(testParent, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("CreateDone: %v", err)
	}

	got, err := s.WaitOperation(context.Background(), &longrunningpb.WaitOperationRequest{
		Name: op.Name,
	})
	if err != nil {
		t.Fatalf("WaitOperation: unexpected error: %v", err)
	}
	if got.Name != op.Name {
		t.Errorf("WaitOperation returned name %q, want %q", got.Name, op.Name)
	}
	if !got.Done {
		t.Errorf("WaitOperation: expected Done=true")
	}
}

// TestListOperations_FiltersByPrefix verifies that ListOperations returns only
// operations whose name has the given req.Name as a prefix.
func TestListOperations_FiltersByPrefix(t *testing.T) {
	s := lro.NewStore()

	parent1 := "projects/proj-a/locations/us-central1"
	parent2 := "projects/proj-b/locations/us-central1"

	op1, err := s.CreateDone(parent1, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("CreateDone parent1: %v", err)
	}
	_, err = s.CreateDone(parent2, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("CreateDone parent2: %v", err)
	}

	// Filter by parent1 prefix — should return exactly one operation.
	resp, err := s.ListOperations(context.Background(), &longrunningpb.ListOperationsRequest{
		Name: parent1,
	})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if len(resp.Operations) != 1 {
		t.Errorf("expected 1 operation for parent1, got %d", len(resp.Operations))
	}
	if len(resp.Operations) == 1 && resp.Operations[0].Name != op1.Name {
		t.Errorf("unexpected operation name: %q", resp.Operations[0].Name)
	}

	// Empty prefix — should return all operations.
	respAll, err := s.ListOperations(context.Background(), &longrunningpb.ListOperationsRequest{
		Name: "",
	})
	if err != nil {
		t.Fatalf("ListOperations (all): %v", err)
	}
	if len(respAll.Operations) != 2 {
		t.Errorf("expected 2 operations total, got %d", len(respAll.Operations))
	}
}
