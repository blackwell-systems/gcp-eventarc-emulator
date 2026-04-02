package server

import (
	"context"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestStorage creates a Storage with the new maps pre-initialized for tests.
// This allows tests to run before Agent E wires up the struct fields in
// storage.go; we inject the maps directly here.
func newTestStorageWithBuses() *Storage {
	s := NewStorage()
	s.messageBuses = make(map[string]*eventarcpb.MessageBus)
	s.enrollments = make(map[string]*eventarcpb.Enrollment)
	return s
}

func TestStorageCreateMessageBus_Success(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	mb, err := s.CreateMessageBus(ctx, "projects/p/locations/l", "my-bus", &eventarcpb.MessageBus{
		DisplayName: "My Bus",
	})
	if err != nil {
		t.Fatalf("CreateMessageBus: unexpected error: %v", err)
	}
	if mb.Name != "projects/p/locations/l/messageBuses/my-bus" {
		t.Errorf("Name = %q, want %q", mb.Name, "projects/p/locations/l/messageBuses/my-bus")
	}
	if mb.Uid == "" {
		t.Error("Uid is empty, want non-empty")
	}
	if mb.Etag == "" {
		t.Error("Etag is empty, want non-empty")
	}
	if mb.CreateTime == nil {
		t.Error("CreateTime is nil, want non-nil")
	}
	if mb.UpdateTime == nil {
		t.Error("UpdateTime is nil, want non-nil")
	}
	if mb.DisplayName != "My Bus" {
		t.Errorf("DisplayName = %q, want %q", mb.DisplayName, "My Bus")
	}
}

func TestStorageCreateMessageBus_AlreadyExists(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	_, err := s.CreateMessageBus(ctx, "projects/p/locations/l", "my-bus", &eventarcpb.MessageBus{})
	if err != nil {
		t.Fatalf("first CreateMessageBus: unexpected error: %v", err)
	}

	_, err = s.CreateMessageBus(ctx, "projects/p/locations/l", "my-bus", &eventarcpb.MessageBus{})
	if err == nil {
		t.Fatal("second CreateMessageBus: expected AlreadyExists error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("second CreateMessageBus: got code %v, want AlreadyExists", status.Code(err))
	}
}

func TestStorageGetMessageBus_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	_, err := s.GetMessageBus(ctx, "projects/p/locations/l/messageBuses/nonexistent")
	if err == nil {
		t.Fatal("GetMessageBus: expected NotFound error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		t.Errorf("GetMessageBus: got code %v, want NotFound", status.Code(err))
	}
}

func TestStorageDeleteMessageBus_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	err := s.DeleteMessageBus(ctx, "projects/p/locations/l/messageBuses/nonexistent")
	if err == nil {
		t.Fatal("DeleteMessageBus: expected NotFound error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		t.Errorf("DeleteMessageBus: got code %v, want NotFound", status.Code(err))
	}
}

func TestStorageListMessageBuses_Pagination(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	parent := "projects/p/locations/l"
	busIDs := []string{"bus-a", "bus-b", "bus-c", "bus-d", "bus-e"}
	for _, id := range busIDs {
		if _, err := s.CreateMessageBus(ctx, parent, id, &eventarcpb.MessageBus{}); err != nil {
			t.Fatalf("CreateMessageBus(%s): %v", id, err)
		}
	}

	// Page 1: expect 2 results and a next token.
	page1, nextToken, err := s.ListMessageBuses(ctx, parent, 2, "")
	if err != nil {
		t.Fatalf("ListMessageBuses page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page1: got %d results, want 2", len(page1))
	}
	if nextToken == "" {
		t.Error("page1: expected non-empty nextToken")
	}

	// Page 2: expect 2 results.
	page2, nextToken2, err := s.ListMessageBuses(ctx, parent, 2, nextToken)
	if err != nil {
		t.Fatalf("ListMessageBuses page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2: got %d results, want 2", len(page2))
	}
	if nextToken2 == "" {
		t.Error("page2: expected non-empty nextToken")
	}

	// Page 3: expect 1 result and empty next token.
	page3, nextToken3, err := s.ListMessageBuses(ctx, parent, 2, nextToken2)
	if err != nil {
		t.Fatalf("ListMessageBuses page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3: got %d results, want 1", len(page3))
	}
	if nextToken3 != "" {
		t.Errorf("page3: expected empty nextToken, got %q", nextToken3)
	}

	// Verify all names unique across pages.
	seen := map[string]bool{}
	for _, mb := range append(append(page1, page2...), page3...) {
		if seen[mb.GetName()] {
			t.Errorf("duplicate name in pagination: %s", mb.GetName())
		}
		seen[mb.GetName()] = true
	}
	if len(seen) != 5 {
		t.Errorf("total paginated results = %d, want 5", len(seen))
	}
}

func TestStorageCreateEnrollment_Success(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	en, err := s.CreateEnrollment(ctx, "projects/p/locations/l", "my-enrollment", &eventarcpb.Enrollment{
		DisplayName: "My Enrollment",
		CelMatch:    "true",
		MessageBus:  "projects/p/locations/l/messageBuses/my-bus",
		Destination: "projects/p/locations/l/pipelines/my-pipe",
	})
	if err != nil {
		t.Fatalf("CreateEnrollment: unexpected error: %v", err)
	}
	if en.Name != "projects/p/locations/l/enrollments/my-enrollment" {
		t.Errorf("Name = %q, want %q", en.Name, "projects/p/locations/l/enrollments/my-enrollment")
	}
	if en.Uid == "" {
		t.Error("Uid is empty, want non-empty")
	}
	if en.Etag == "" {
		t.Error("Etag is empty, want non-empty")
	}
	if en.CreateTime == nil {
		t.Error("CreateTime is nil, want non-nil")
	}
	if en.UpdateTime == nil {
		t.Error("UpdateTime is nil, want non-nil")
	}
	if en.DisplayName != "My Enrollment" {
		t.Errorf("DisplayName = %q, want %q", en.DisplayName, "My Enrollment")
	}
	if en.CelMatch != "true" {
		t.Errorf("CelMatch = %q, want %q", en.CelMatch, "true")
	}
}

func TestStorageGetEnrollment_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	_, err := s.GetEnrollment(ctx, "projects/p/locations/l/enrollments/nonexistent")
	if err == nil {
		t.Fatal("GetEnrollment: expected NotFound error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		t.Errorf("GetEnrollment: got code %v, want NotFound", status.Code(err))
	}
}

func TestStorageListMessageBusEnrollments_FiltersByBus(t *testing.T) {
	ctx := context.Background()
	s := newTestStorageWithBuses()

	busA := "projects/p/locations/l/messageBuses/bus-a"
	busB := "projects/p/locations/l/messageBuses/bus-b"
	parent := "projects/p/locations/l"

	// Create 2 enrollments for bus A.
	for _, id := range []string{"enr-1", "enr-2"} {
		_, err := s.CreateEnrollment(ctx, parent, id, &eventarcpb.Enrollment{
			MessageBus: busA,
		})
		if err != nil {
			t.Fatalf("CreateEnrollment(%s): %v", id, err)
		}
	}

	// Create 1 enrollment for bus B.
	_, err := s.CreateEnrollment(ctx, parent, "enr-3", &eventarcpb.Enrollment{
		MessageBus: busB,
	})
	if err != nil {
		t.Fatalf("CreateEnrollment(enr-3): %v", err)
	}

	// ListMessageBusEnrollments for bus A should return exactly 2 names.
	names, nextToken, err := s.ListMessageBusEnrollments(ctx, busA, 100, "")
	if err != nil {
		t.Fatalf("ListMessageBusEnrollments: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("got %d enrollment names for bus A, want 2; names: %v", len(names), names)
	}
	if nextToken != "" {
		t.Errorf("expected empty nextToken, got %q", nextToken)
	}

	// Verify the returned values are strings (resource names), not structs.
	for _, n := range names {
		if !isEnrollmentName(n, parent) {
			t.Errorf("result %q does not look like an enrollment resource name", n)
		}
	}

	// bus B should return exactly 1.
	namesB, _, err := s.ListMessageBusEnrollments(ctx, busB, 100, "")
	if err != nil {
		t.Fatalf("ListMessageBusEnrollments busB: %v", err)
	}
	if len(namesB) != 1 {
		t.Errorf("got %d enrollment names for bus B, want 1", len(namesB))
	}
}

// isEnrollmentName is a simple helper to check a string looks like a valid
// enrollment resource name under the given parent.
func isEnrollmentName(name, parent string) bool {
	prefix := parent + "/enrollments/"
	return len(name) > len(prefix) && name[:len(prefix)] == prefix
}
