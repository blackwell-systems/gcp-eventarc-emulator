package server

import (
	"context"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testParent = "projects/test-project/locations/us-central1"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	s := NewStorage()
	t.Cleanup(s.Clear)
	return s
}

// TestStorageCreateTrigger_Success verifies that a new trigger is stored and
// returned with server-assigned fields (name, uid, create_time, update_time).
func TestStorageCreateTrigger_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	trigger := &eventarcpb.Trigger{
		ServiceAccount: "sa@project.iam.gserviceaccount.com",
	}

	got, err := s.CreateTrigger(ctx, testParent, "my-trigger", trigger)
	if err != nil {
		t.Fatalf("CreateTrigger: unexpected error: %v", err)
	}
	if got.GetName() != testParent+"/triggers/my-trigger" {
		t.Errorf("Name = %q, want %q", got.GetName(), testParent+"/triggers/my-trigger")
	}
	if got.GetUid() == "" {
		t.Error("Uid should not be empty after create")
	}
	if got.GetCreateTime() == nil {
		t.Error("CreateTime should not be nil after create")
	}
	if got.GetUpdateTime() == nil {
		t.Error("UpdateTime should not be nil after create")
	}
	if got.GetServiceAccount() != trigger.GetServiceAccount() {
		t.Errorf("ServiceAccount = %q, want %q", got.GetServiceAccount(), trigger.GetServiceAccount())
	}
	if s.TriggerCount() != 1 {
		t.Errorf("TriggerCount = %d, want 1", s.TriggerCount())
	}
}

// TestStorageCreateTrigger_AlreadyExists verifies that creating a trigger with
// a duplicate name returns an AlreadyExists error.
func TestStorageCreateTrigger_AlreadyExists(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	trigger := &eventarcpb.Trigger{}
	if _, err := s.CreateTrigger(ctx, testParent, "dup-trigger", trigger); err != nil {
		t.Fatalf("first CreateTrigger: unexpected error: %v", err)
	}

	_, err := s.CreateTrigger(ctx, testParent, "dup-trigger", trigger)
	if err == nil {
		t.Fatal("expected AlreadyExists error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("error code = %v, want AlreadyExists", err)
	}
}

// TestStorageGetTrigger_NotFound verifies that looking up a non-existent
// trigger returns a NotFound error.
func TestStorageGetTrigger_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := s.GetTrigger(ctx, testParent+"/triggers/does-not-exist")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageDeleteTrigger_NotFound verifies that deleting a non-existent
// trigger returns a NotFound error.
func TestStorageDeleteTrigger_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	err := s.DeleteTrigger(ctx, testParent+"/triggers/ghost")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageListTriggers_Pagination verifies that pageSize and pageToken
// correctly partition the full result set.
func TestStorageListTriggers_Pagination(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Create 5 triggers.
	for i := 0; i < 5; i++ {
		id := "trigger-" + string(rune('a'+i))
		if _, err := s.CreateTrigger(ctx, testParent, id, &eventarcpb.Trigger{}); err != nil {
			t.Fatalf("CreateTrigger %s: %v", id, err)
		}
	}

	// Page 1: first 2.
	page1, nextToken, err := s.ListTriggers(ctx, testParent, 2, "", "name", "")
	if err != nil {
		t.Fatalf("ListTriggers page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page1 len = %d, want 2", len(page1))
	}
	if nextToken == "" {
		t.Error("expected non-empty nextToken after page1")
	}

	// Page 2: next 2.
	page2, nextToken2, err := s.ListTriggers(ctx, testParent, 2, nextToken, "name", "")
	if err != nil {
		t.Fatalf("ListTriggers page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
	if nextToken2 == "" {
		t.Error("expected non-empty nextToken after page2")
	}

	// Page 3: last 1.
	page3, nextToken3, err := s.ListTriggers(ctx, testParent, 2, nextToken2, "name", "")
	if err != nil {
		t.Fatalf("ListTriggers page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3 len = %d, want 1", len(page3))
	}
	if nextToken3 != "" {
		t.Errorf("expected empty nextToken at end, got %q", nextToken3)
	}

	// Verify no duplicates across pages.
	seen := make(map[string]bool)
	for _, pg := range [][]*eventarcpb.Trigger{page1, page2, page3} {
		for _, tr := range pg {
			if seen[tr.GetName()] {
				t.Errorf("duplicate trigger %q in paginated results", tr.GetName())
			}
			seen[tr.GetName()] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("total unique triggers across pages = %d, want 5", len(seen))
	}
}

// TestStorageListProviders_ReturnsDefaults verifies that the default set of
// well-known providers is available via ListProviders.
func TestStorageListProviders_ReturnsDefaults(t *testing.T) {
	s := NewStorage()
	ctx := context.Background()

	providers, nextToken, err := s.ListProviders(ctx, testParent, 0, "", "", "")
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if nextToken != "" {
		t.Errorf("nextToken = %q, want empty (all defaults fit on one page)", nextToken)
	}
	if len(providers) != len(defaultProviders) {
		t.Errorf("provider count = %d, want %d", len(providers), len(defaultProviders))
	}

	// All returned names should use the requested parent.
	for _, p := range providers {
		if !startsWith(p.GetName(), testParent+"/providers/") {
			t.Errorf("provider name %q does not have expected prefix", p.GetName())
		}
		if p.GetDisplayName() == "" {
			t.Errorf("provider %q has empty DisplayName", p.GetName())
		}
	}
}

// startsWith is a helper that avoids importing strings in the test file.
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
