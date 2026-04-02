// Package integration provides end-to-end integration tests for the
// GCP Eventarc emulator using an in-process bufconn transport.
package integration_test

import (
	"context"
	"net"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// setupTestServer starts an in-process gRPC server backed by bufconn.
// Returns the server, the listener, and a cleanup function.
func setupTestServer(t *testing.T) (*grpc.Server, *bufconn.Listener, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)

	srv, err := server.NewServer()
	if err != nil {
		t.Fatalf("server.NewServer() error: %v", err)
	}

	grpcSrv := grpc.NewServer()
	eventarcpb.RegisterEventarcServer(grpcSrv, srv)

	go grpcSrv.Serve(lis) //nolint:errcheck

	cleanup := func() {
		grpcSrv.Stop()
		lis.Close()
	}
	return grpcSrv, lis, cleanup
}

// setupTestClient creates a gRPC client that dials over the provided bufconn listener.
// Returns the EventarcClient, the underlying connection, and a cleanup function.
func setupTestClient(t *testing.T, lis *bufconn.Listener) (eventarcpb.EventarcClient, *grpc.ClientConn, func()) {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() error: %v", err)
	}
	return eventarcpb.NewEventarcClient(conn), conn, func() { conn.Close() }
}

// testTrigger returns a minimal valid trigger for use in tests.
func testTrigger() *eventarcpb.Trigger {
	return &eventarcpb.Trigger{
		EventFilters: []*eventarcpb.EventFilter{
			{Attribute: "type", Value: "google.cloud.pubsub.topic.v1.messagePublished"},
		},
		Destination: &eventarcpb.Destination{
			Descriptor_: &eventarcpb.Destination_HttpEndpoint{
				HttpEndpoint: &eventarcpb.HttpEndpoint{Uri: "http://localhost:8080/handler"},
			},
		},
	}
}

// unpackTrigger extracts a Trigger proto from a completed LRO response.
func unpackTrigger(t *testing.T, op *longrunningpb.Operation) *eventarcpb.Trigger {
	t.Helper()
	if !op.Done {
		t.Fatal("expected operation to be DONE")
	}
	var trigger eventarcpb.Trigger
	if err := op.GetResponse().UnmarshalTo(&trigger); err != nil {
		t.Fatalf("UnmarshalTo(Trigger) error: %v", err)
	}
	return &trigger
}

// -------------------------------------------------------------------------
// TestIntegration_TriggerCRUD exercises the full create/get/list/update/delete
// lifecycle for a single trigger.
// -------------------------------------------------------------------------

func TestIntegration_TriggerCRUD(t *testing.T) {
	_, lis, cleanup := setupTestServer(t)
	t.Cleanup(cleanup)

	client, _, connCleanup := setupTestClient(t, lis)
	t.Cleanup(connCleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"
	triggerID := "my-trigger"
	triggerName := parent + "/triggers/" + triggerID

	var createdTrigger *eventarcpb.Trigger

	t.Run("Create", func(t *testing.T) {
		op, err := client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
			Parent:    parent,
			TriggerId: triggerID,
			Trigger:   testTrigger(),
		})
		if err != nil {
			t.Fatalf("CreateTrigger() error: %v", err)
		}
		createdTrigger = unpackTrigger(t, op)
		if createdTrigger.GetName() != triggerName {
			t.Errorf("Trigger.Name = %q, want %q", createdTrigger.GetName(), triggerName)
		}
		if createdTrigger.GetUid() == "" {
			t.Error("Trigger.Uid must not be empty")
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := client.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: triggerName})
		if err != nil {
			t.Fatalf("GetTrigger() error: %v", err)
		}
		if got.GetName() != triggerName {
			t.Errorf("GetTrigger().Name = %q, want %q", got.GetName(), triggerName)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := client.ListTriggers(ctx, &eventarcpb.ListTriggersRequest{Parent: parent})
		if err != nil {
			t.Fatalf("ListTriggers() error: %v", err)
		}
		found := false
		for _, tr := range resp.GetTriggers() {
			if tr.GetName() == triggerName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListTriggers() did not contain %q", triggerName)
		}
	})

	t.Run("Update", func(t *testing.T) {
		// Update labels on the trigger.
		updated := proto.Clone(createdTrigger).(*eventarcpb.Trigger)
		updated.Labels = map[string]string{"env": "test"}

		op, err := client.UpdateTrigger(ctx, &eventarcpb.UpdateTriggerRequest{
			Trigger: updated,
		})
		if err != nil {
			t.Fatalf("UpdateTrigger() error: %v", err)
		}
		got := unpackTrigger(t, op)
		if got.GetLabels()["env"] != "test" {
			t.Errorf("UpdateTrigger() labels[env] = %q, want %q", got.GetLabels()["env"], "test")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		op, err := client.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{Name: triggerName})
		if err != nil {
			t.Fatalf("DeleteTrigger() error: %v", err)
		}
		if !op.Done {
			t.Error("DeleteTrigger() returned operation not Done")
		}
	})

	t.Run("GetAfterDelete", func(t *testing.T) {
		_, err := client.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: triggerName})
		if err == nil {
			t.Fatal("GetTrigger() after delete expected error, got nil")
		}
		if code := status.Code(err); code != codes.NotFound {
			t.Errorf("GetTrigger() after delete status = %v, want NotFound", code)
		}
	})
}

// -------------------------------------------------------------------------
// TestIntegration_ListProviders verifies that static provider data is seeded.
// -------------------------------------------------------------------------

func TestIntegration_ListProviders(t *testing.T) {
	_, lis, cleanup := setupTestServer(t)
	t.Cleanup(cleanup)

	client, _, connCleanup := setupTestClient(t, lis)
	t.Cleanup(connCleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"

	resp, err := client.ListProviders(ctx, &eventarcpb.ListProvidersRequest{Parent: parent})
	if err != nil {
		t.Fatalf("ListProviders() error: %v", err)
	}
	if len(resp.GetProviders()) == 0 {
		t.Error("ListProviders() returned 0 providers, want at least 1")
	}
}

// -------------------------------------------------------------------------
// TestIntegration_GetProvider verifies a known provider can be retrieved.
// -------------------------------------------------------------------------

func TestIntegration_GetProvider(t *testing.T) {
	_, lis, cleanup := setupTestServer(t)
	t.Cleanup(cleanup)

	client, _, connCleanup := setupTestClient(t, lis)
	t.Cleanup(connCleanup)

	ctx := context.Background()
	// pubsub.googleapis.com is always seeded.
	providerName := "projects/test-project/locations/us-central1/providers/pubsub.googleapis.com"

	got, err := client.GetProvider(ctx, &eventarcpb.GetProviderRequest{Name: providerName})
	if err != nil {
		t.Fatalf("GetProvider(%q) error: %v", providerName, err)
	}
	if got.GetName() != providerName {
		t.Errorf("Provider.Name = %q, want %q", got.GetName(), providerName)
	}
}

// -------------------------------------------------------------------------
// TestIntegration_CreateTrigger_MissingParent checks that missing parent
// returns InvalidArgument.
// -------------------------------------------------------------------------

func TestIntegration_CreateTrigger_MissingParent(t *testing.T) {
	_, lis, cleanup := setupTestServer(t)
	t.Cleanup(cleanup)

	client, _, connCleanup := setupTestClient(t, lis)
	t.Cleanup(connCleanup)

	ctx := context.Background()

	_, err := client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    "", // missing
		TriggerId: "t1",
		Trigger:   testTrigger(),
	})
	if err == nil {
		t.Fatal("CreateTrigger() with empty parent expected error, got nil")
	}
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("CreateTrigger() missing parent status = %v, want InvalidArgument", code)
	}
}

// -------------------------------------------------------------------------
// TestIntegration_CreateTrigger_Duplicate checks that creating the same
// trigger twice returns AlreadyExists.
// -------------------------------------------------------------------------

func TestIntegration_CreateTrigger_Duplicate(t *testing.T) {
	_, lis, cleanup := setupTestServer(t)
	t.Cleanup(cleanup)

	client, _, connCleanup := setupTestClient(t, lis)
	t.Cleanup(connCleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"
	triggerID := "dup-trigger"

	// First create should succeed.
	_, err := client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: triggerID,
		Trigger:   testTrigger(),
	})
	if err != nil {
		t.Fatalf("CreateTrigger() first call error: %v", err)
	}

	// Second create should return AlreadyExists.
	_, err = client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: triggerID,
		Trigger:   testTrigger(),
	})
	if err == nil {
		t.Fatal("CreateTrigger() duplicate expected error, got nil")
	}
	if code := status.Code(err); code != codes.AlreadyExists {
		t.Errorf("CreateTrigger() duplicate status = %v, want AlreadyExists", code)
	}
}
