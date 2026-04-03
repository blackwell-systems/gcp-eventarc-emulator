// Package integration provides end-to-end integration tests for the
// GCP Eventarc emulator using an in-process bufconn transport.
package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/dispatcher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/publisher"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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

// =========================================================================
// Full-stack setup: Eventarc + Publisher + Router + Dispatcher on bufconn
// =========================================================================

// setupFullStack starts a gRPC server with Eventarc, Publisher, and LRO
// services registered. Returns both clients and a cleanup function.
func setupFullStack(t *testing.T) (eventarcpb.EventarcClient, publishingpb.PublisherClient, *grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)

	srv, err := server.NewServer()
	if err != nil {
		t.Fatalf("server.NewServer() error: %v", err)
	}

	rtr := router.NewRouter(srv.Storage())
	dsp := dispatcher.NewDispatcher(nil)
	pub := publisher.NewServer(rtr, dsp, srv.Storage())

	grpcSrv := server.NewGRPCServer(srv, pub)
	go grpcSrv.Serve(lis) //nolint:errcheck

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

	cleanup := func() {
		conn.Close()
		grpcSrv.Stop()
		lis.Close()
	}
	return eventarcpb.NewEventarcClient(conn), publishingpb.NewPublisherClient(conn), conn, cleanup
}

// unpackChannel extracts a Channel from a completed LRO.
func unpackChannel(t *testing.T, op *longrunningpb.Operation) *eventarcpb.Channel {
	t.Helper()
	if !op.Done {
		t.Fatal("expected operation to be DONE")
	}
	var ch eventarcpb.Channel
	if err := op.GetResponse().UnmarshalTo(&ch); err != nil {
		t.Fatalf("UnmarshalTo(Channel) error: %v", err)
	}
	return &ch
}

// unpackMessageBus extracts a MessageBus from a completed LRO.
func unpackMessageBus(t *testing.T, op *longrunningpb.Operation) *eventarcpb.MessageBus {
	t.Helper()
	if !op.Done {
		t.Fatal("expected operation to be DONE")
	}
	var mb eventarcpb.MessageBus
	if err := op.GetResponse().UnmarshalTo(&mb); err != nil {
		t.Fatalf("UnmarshalTo(MessageBus) error: %v", err)
	}
	return &mb
}

// unpackPipeline extracts a Pipeline from a completed LRO.
func unpackPipeline(t *testing.T, op *longrunningpb.Operation) *eventarcpb.Pipeline {
	t.Helper()
	if !op.Done {
		t.Fatal("expected operation to be DONE")
	}
	var p eventarcpb.Pipeline
	if err := op.GetResponse().UnmarshalTo(&p); err != nil {
		t.Fatalf("UnmarshalTo(Pipeline) error: %v", err)
	}
	return &p
}

// =========================================================================
// Channel CRUD
// =========================================================================

func TestIntegration_ChannelCRUD(t *testing.T) {
	eventarcClient, _, _, cleanup := setupFullStack(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"
	channelID := "my-channel"
	channelName := parent + "/channels/" + channelID

	t.Run("Create", func(t *testing.T) {
		op, err := eventarcClient.CreateChannel(ctx, &eventarcpb.CreateChannelRequest{
			Parent:    parent,
			ChannelId: channelID,
			Channel:   &eventarcpb.Channel{},
		})
		if err != nil {
			t.Fatalf("CreateChannel() error: %v", err)
		}
		ch := unpackChannel(t, op)
		if ch.GetName() != channelName {
			t.Errorf("Channel.Name = %q, want %q", ch.GetName(), channelName)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := eventarcClient.GetChannel(ctx, &eventarcpb.GetChannelRequest{Name: channelName})
		if err != nil {
			t.Fatalf("GetChannel() error: %v", err)
		}
		if got.GetName() != channelName {
			t.Errorf("GetChannel().Name = %q, want %q", got.GetName(), channelName)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := eventarcClient.ListChannels(ctx, &eventarcpb.ListChannelsRequest{Parent: parent})
		if err != nil {
			t.Fatalf("ListChannels() error: %v", err)
		}
		if len(resp.GetChannels()) != 1 {
			t.Errorf("ListChannels() returned %d channels, want 1", len(resp.GetChannels()))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		op, err := eventarcClient.DeleteChannel(ctx, &eventarcpb.DeleteChannelRequest{Name: channelName})
		if err != nil {
			t.Fatalf("DeleteChannel() error: %v", err)
		}
		if !op.Done {
			t.Error("DeleteChannel() returned operation not Done")
		}
	})

	t.Run("GetAfterDelete", func(t *testing.T) {
		_, err := eventarcClient.GetChannel(ctx, &eventarcpb.GetChannelRequest{Name: channelName})
		if code := status.Code(err); code != codes.NotFound {
			t.Errorf("GetChannel() after delete status = %v, want NotFound", code)
		}
	})
}

// =========================================================================
// MessageBus CRUD
// =========================================================================

func TestIntegration_MessageBusCRUD(t *testing.T) {
	eventarcClient, _, _, cleanup := setupFullStack(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"
	busID := "my-bus"
	busName := parent + "/messageBuses/" + busID

	t.Run("Create", func(t *testing.T) {
		op, err := eventarcClient.CreateMessageBus(ctx, &eventarcpb.CreateMessageBusRequest{
			Parent:       parent,
			MessageBusId: busID,
			MessageBus:   &eventarcpb.MessageBus{},
		})
		if err != nil {
			t.Fatalf("CreateMessageBus() error: %v", err)
		}
		mb := unpackMessageBus(t, op)
		if mb.GetName() != busName {
			t.Errorf("MessageBus.Name = %q, want %q", mb.GetName(), busName)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := eventarcClient.GetMessageBus(ctx, &eventarcpb.GetMessageBusRequest{Name: busName})
		if err != nil {
			t.Fatalf("GetMessageBus() error: %v", err)
		}
		if got.GetName() != busName {
			t.Errorf("GetMessageBus().Name = %q, want %q", got.GetName(), busName)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := eventarcClient.ListMessageBuses(ctx, &eventarcpb.ListMessageBusesRequest{Parent: parent})
		if err != nil {
			t.Fatalf("ListMessageBuses() error: %v", err)
		}
		if len(resp.GetMessageBuses()) != 1 {
			t.Errorf("ListMessageBuses() returned %d, want 1", len(resp.GetMessageBuses()))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		op, err := eventarcClient.DeleteMessageBus(ctx, &eventarcpb.DeleteMessageBusRequest{Name: busName})
		if err != nil {
			t.Fatalf("DeleteMessageBus() error: %v", err)
		}
		if !op.Done {
			t.Error("DeleteMessageBus() returned operation not Done")
		}
	})
}

// =========================================================================
// Pipeline CRUD
// =========================================================================

func TestIntegration_PipelineCRUD(t *testing.T) {
	eventarcClient, _, _, cleanup := setupFullStack(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"
	pipelineID := "my-pipeline"
	pipelineName := parent + "/pipelines/" + pipelineID

	t.Run("Create", func(t *testing.T) {
		op, err := eventarcClient.CreatePipeline(ctx, &eventarcpb.CreatePipelineRequest{
			Parent:     parent,
			PipelineId: pipelineID,
			Pipeline:   &eventarcpb.Pipeline{},
		})
		if err != nil {
			t.Fatalf("CreatePipeline() error: %v", err)
		}
		p := unpackPipeline(t, op)
		if p.GetName() != pipelineName {
			t.Errorf("Pipeline.Name = %q, want %q", p.GetName(), pipelineName)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := eventarcClient.GetPipeline(ctx, &eventarcpb.GetPipelineRequest{Name: pipelineName})
		if err != nil {
			t.Fatalf("GetPipeline() error: %v", err)
		}
		if got.GetName() != pipelineName {
			t.Errorf("GetPipeline().Name = %q, want %q", got.GetName(), pipelineName)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		op, err := eventarcClient.DeletePipeline(ctx, &eventarcpb.DeletePipelineRequest{Name: pipelineName})
		if err != nil {
			t.Fatalf("DeletePipeline() error: %v", err)
		}
		if !op.Done {
			t.Error("DeletePipeline() returned operation not Done")
		}
	})
}

// =========================================================================
// GoogleChannelConfig (singleton)
// =========================================================================

func TestIntegration_GoogleChannelConfig(t *testing.T) {
	eventarcClient, _, _, cleanup := setupFullStack(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	configName := "projects/test-project/locations/us-central1/googleChannelConfig"

	t.Run("GetReturnsZeroValue", func(t *testing.T) {
		got, err := eventarcClient.GetGoogleChannelConfig(ctx, &eventarcpb.GetGoogleChannelConfigRequest{Name: configName})
		if err != nil {
			t.Fatalf("GetGoogleChannelConfig() error: %v", err)
		}
		if got.GetName() != configName {
			t.Errorf("Name = %q, want %q", got.GetName(), configName)
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := eventarcClient.UpdateGoogleChannelConfig(ctx, &eventarcpb.UpdateGoogleChannelConfigRequest{
			GoogleChannelConfig: &eventarcpb.GoogleChannelConfig{
				Name:          configName,
				CryptoKeyName: "projects/test-project/locations/us-central1/keyRings/my-ring/cryptoKeys/my-key",
			},
		})
		if err != nil {
			t.Fatalf("UpdateGoogleChannelConfig() error: %v", err)
		}
		if updated.GetCryptoKeyName() == "" {
			t.Error("UpdateGoogleChannelConfig() CryptoKeyName is empty after update")
		}
	})
}

// =========================================================================
// End-to-end: PublishEvents → Router → Dispatcher → HTTP destination
// =========================================================================

func TestIntegration_PublishEvents_EndToEnd(t *testing.T) {
	eventarcClient, pubClient, _, cleanup := setupFullStack(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	parent := "projects/test-project/locations/us-central1"

	// Start a test HTTP server to receive dispatched events.
	received := make(chan *http.Request, 1)
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Store a copy of the request with the body re-readable.
		clone := r.Clone(r.Context())
		clone.Body = io.NopCloser(io.Reader(nil))
		clone.Header = r.Header.Clone()
		// Use the original request but store body separately.
		r.Header.Set("X-Test-Body", string(body))
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()

	// Create a trigger pointing at our test server.
	triggerID := "e2e-trigger"
	_, err := eventarcClient.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: triggerID,
		Trigger: &eventarcpb.Trigger{
			EventFilters: []*eventarcpb.EventFilter{
				{Attribute: "type", Value: "test.event.v1"},
			},
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_HttpEndpoint{
					HttpEndpoint: &eventarcpb.HttpEndpoint{Uri: dest.URL},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateTrigger() error: %v", err)
	}

	// Create the channel that events will be published to.
	_, err = eventarcClient.CreateChannel(ctx, &eventarcpb.CreateChannelRequest{
		Parent:    parent,
		ChannelId: "test-channel",
		Channel:   &eventarcpb.Channel{},
	})
	if err != nil {
		t.Fatalf("CreateChannel() error: %v", err)
	}

	// Build a CloudEvent proto to publish.
	ceProto := &publishingpb.CloudEvent{
		Id:          fmt.Sprintf("test-id-%d", 1),
		Source:      "//test.source/projects/test-project",
		SpecVersion: "1.0",
		Type:        "test.event.v1",
		Data:        &publishingpb.CloudEvent_TextData{TextData: `{"key":"value"}`},
	}
	ceAny, err := anypb.New(ceProto)
	if err != nil {
		t.Fatalf("anypb.New(CloudEvent) error: %v", err)
	}

	// Publish via the Publisher gRPC service.
	resp, err := pubClient.PublishEvents(ctx, &publishingpb.PublishEventsRequest{
		Channel: parent + "/channels/test-channel",
		Events:  []*anypb.Any{ceAny},
	})
	if err != nil {
		t.Fatalf("PublishEvents() error: %v", err)
	}
	_ = resp

	// Verify the event was dispatched to our HTTP endpoint.
	select {
	case req := <-received:
		// Check binary content mode headers.
		if got := req.Header.Get("Ce-Type"); got != "test.event.v1" {
			t.Errorf("Ce-Type = %q, want %q", got, "test.event.v1")
		}
		if got := req.Header.Get("Ce-Specversion"); got != "1.0" {
			t.Errorf("Ce-Specversion = %q, want %q", got, "1.0")
		}
		if got := req.Header.Get("Ce-Source"); got == "" {
			t.Error("Ce-Source header is empty")
		}
		if got := req.Header.Get("Ce-Id"); got == "" {
			t.Error("Ce-Id header is empty")
		}
		// Check body was delivered.
		body := req.Header.Get("X-Test-Body")
		if body == "" {
			t.Error("dispatched request body is empty")
		}
		// Verify it's valid JSON.
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(body), &parsed); err != nil {
			t.Errorf("dispatched body is not valid JSON: %v (body=%q)", err, body)
		}
	default:
		t.Fatal("no HTTP request received at destination — event was not dispatched")
	}
}
