package server

import (
	"context"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// newTestServer creates a Server with a clean Storage and no IAM (allow all).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	srv.storage.Clear()
	return srv
}

// minimalTrigger returns a Trigger with required fields for creation.
func minimalTrigger() *eventarcpb.Trigger {
	return &eventarcpb.Trigger{
		Destination: &eventarcpb.Destination{
			Descriptor_: &eventarcpb.Destination_HttpEndpoint{
				HttpEndpoint: &eventarcpb.HttpEndpoint{
					Uri: "http://localhost:8080/event",
				},
			},
		},
	}
}

// -------------------------------------------------------------------------
// TestCreateTrigger_Success
// -------------------------------------------------------------------------

func TestCreateTrigger_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	req := &eventarcpb.CreateTriggerRequest{
		Parent:    "projects/my-project/locations/us-central1",
		TriggerId: "my-trigger",
		Trigger:   minimalTrigger(),
	}

	op, err := srv.CreateTrigger(ctx, req)
	if err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}
	if op == nil {
		t.Fatal("expected non-nil operation")
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately; got Done=false")
	}

	// The trigger should now be retrievable.
	getResp, err := srv.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{
		Name: "projects/my-project/locations/us-central1/triggers/my-trigger",
	})
	if err != nil {
		t.Fatalf("GetTrigger after create: %v", err)
	}
	if getResp.GetName() != "projects/my-project/locations/us-central1/triggers/my-trigger" {
		t.Errorf("unexpected trigger name: %s", getResp.GetName())
	}
}

// -------------------------------------------------------------------------
// TestCreateTrigger_MissingParent
// -------------------------------------------------------------------------

func TestCreateTrigger_MissingParent(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	cases := []struct {
		name    string
		req     *eventarcpb.CreateTriggerRequest
		wantMsg string
	}{
		{
			name: "empty parent",
			req: &eventarcpb.CreateTriggerRequest{
				Parent:    "",
				TriggerId: "t",
				Trigger:   minimalTrigger(),
			},
			wantMsg: "parent is required",
		},
		{
			name: "empty trigger_id",
			req: &eventarcpb.CreateTriggerRequest{
				Parent:    "projects/p/locations/l",
				TriggerId: "",
				Trigger:   minimalTrigger(),
			},
			wantMsg: "trigger_id is required",
		},
		{
			name: "nil trigger",
			req: &eventarcpb.CreateTriggerRequest{
				Parent:    "projects/p/locations/l",
				TriggerId: "t",
				Trigger:   nil,
			},
			wantMsg: "trigger is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := srv.CreateTrigger(ctx, tc.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got: %v", err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument, got %s", st.Code())
			}
		})
	}
}

// -------------------------------------------------------------------------
// TestGetTrigger_NotFound
// -------------------------------------------------------------------------

func TestGetTrigger_NotFound(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	_, err := srv.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{
		Name: "projects/p/locations/l/triggers/does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

// -------------------------------------------------------------------------
// TestDeleteTrigger_Success
// -------------------------------------------------------------------------

func TestDeleteTrigger_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	triggerName := parent + "/triggers/to-delete"

	// Create first.
	_, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "to-delete",
		Trigger:   minimalTrigger(),
	})
	if err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}

	// Delete it.
	op, err := srv.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{
		Name: triggerName,
	})
	if err != nil {
		t.Fatalf("DeleteTrigger: %v", err)
	}
	if !op.Done {
		t.Errorf("expected delete LRO to be done immediately")
	}

	// Should now be gone.
	_, err = srv.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: triggerName})
	if err == nil {
		t.Fatal("expected NotFound after delete, got nil error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

// -------------------------------------------------------------------------
// TestListTriggers_Empty
// -------------------------------------------------------------------------

func TestListTriggers_Empty(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	resp, err := srv.ListTriggers(ctx, &eventarcpb.ListTriggersRequest{
		Parent: "projects/p/locations/l",
	})
	if err != nil {
		t.Fatalf("ListTriggers: %v", err)
	}
	if len(resp.GetTriggers()) != 0 {
		t.Errorf("expected 0 triggers, got %d", len(resp.GetTriggers()))
	}
	if resp.GetNextPageToken() != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.GetNextPageToken())
	}
}

// -------------------------------------------------------------------------
// TestGetProvider_ReturnsProvider
// -------------------------------------------------------------------------

func TestGetProvider_ReturnsProvider(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	// Use a known seeded provider ID.
	resp, err := srv.GetProvider(ctx, &eventarcpb.GetProviderRequest{
		Name: "projects/my-project/locations/us-central1/providers/pubsub.googleapis.com",
	})
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil provider")
	}
	if resp.GetName() == "" {
		t.Error("expected non-empty provider name")
	}
}

// -------------------------------------------------------------------------
// Additional coverage: UpdateTrigger, ListTriggers with items
// -------------------------------------------------------------------------

func TestUpdateTrigger_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/proj/locations/us-east1"

	// Create.
	_, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "upd-trigger",
		Trigger:   minimalTrigger(),
	})
	if err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}

	// Update labels.
	updated := &eventarcpb.Trigger{
		Name:   parent + "/triggers/upd-trigger",
		Labels: map[string]string{"env": "test"},
	}
	op, err := srv.UpdateTrigger(ctx, &eventarcpb.UpdateTriggerRequest{
		Trigger:    updated,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"labels"}},
	})
	if err != nil {
		t.Fatalf("UpdateTrigger: %v", err)
	}
	if !op.Done {
		t.Errorf("expected update LRO to be done immediately")
	}
}

func TestListTriggers_WithItems(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/us"
	for i := 0; i < 3; i++ {
		_, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
			Parent:    parent,
			TriggerId: "t" + string(rune('a'+i)),
			Trigger:   minimalTrigger(),
		})
		if err != nil {
			t.Fatalf("CreateTrigger[%d]: %v", i, err)
		}
	}

	resp, err := srv.ListTriggers(ctx, &eventarcpb.ListTriggersRequest{Parent: parent})
	if err != nil {
		t.Fatalf("ListTriggers: %v", err)
	}
	if len(resp.GetTriggers()) != 3 {
		t.Errorf("expected 3 triggers, got %d", len(resp.GetTriggers()))
	}
}
