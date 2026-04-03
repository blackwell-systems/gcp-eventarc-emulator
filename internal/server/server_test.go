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
		EventFilters: []*eventarcpb.EventFilter{
			{Attribute: "type", Value: "google.cloud.pubsub.topic.v1.messagePublished"},
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

// -------------------------------------------------------------------------
// Channel tests
// -------------------------------------------------------------------------

func TestCreateChannel_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	op, err := srv.CreateChannel(ctx, &eventarcpb.CreateChannelRequest{
		Parent:    parent,
		ChannelId: "my-channel",
		Channel:   &eventarcpb.Channel{},
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately")
	}

	ch, err := srv.GetChannel(ctx, &eventarcpb.GetChannelRequest{
		Name: parent + "/channels/my-channel",
	})
	if err != nil {
		t.Fatalf("GetChannel after create: %v", err)
	}
	if ch.GetName() != parent+"/channels/my-channel" {
		t.Errorf("unexpected channel name: %s", ch.GetName())
	}
}

func TestGetChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	_, err := srv.GetChannel(ctx, &eventarcpb.GetChannelRequest{
		Name: "projects/p/locations/l/channels/does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

func TestListChannels_Empty(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	resp, err := srv.ListChannels(ctx, &eventarcpb.ListChannelsRequest{
		Parent: "projects/p/locations/l",
	})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(resp.GetChannels()) != 0 {
		t.Errorf("expected 0 channels, got %d", len(resp.GetChannels()))
	}
	if resp.GetNextPageToken() != "" {
		t.Errorf("expected empty next_page_token")
	}
}

// -------------------------------------------------------------------------
// ChannelConnection tests
// -------------------------------------------------------------------------

func TestCreateChannelConnection_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	op, err := srv.CreateChannelConnection(ctx, &eventarcpb.CreateChannelConnectionRequest{
		Parent:              parent,
		ChannelConnectionId: "my-conn",
		ChannelConnection:   &eventarcpb.ChannelConnection{},
	})
	if err != nil {
		t.Fatalf("CreateChannelConnection: %v", err)
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately")
	}
}

// -------------------------------------------------------------------------
// GoogleChannelConfig tests
// -------------------------------------------------------------------------

func TestGetGoogleChannelConfig_Default(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	cfg, err := srv.GetGoogleChannelConfig(ctx, &eventarcpb.GetGoogleChannelConfigRequest{
		Name: "projects/p/locations/l/googleChannelConfig",
	})
	if err != nil {
		t.Fatalf("GetGoogleChannelConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestUpdateGoogleChannelConfig_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	name := "projects/p/locations/l/googleChannelConfig"
	cfg, err := srv.UpdateGoogleChannelConfig(ctx, &eventarcpb.UpdateGoogleChannelConfigRequest{
		GoogleChannelConfig: &eventarcpb.GoogleChannelConfig{
			Name:          name,
			CryptoKeyName: "projects/p/locations/l/keyRings/kr/cryptoKeys/k",
		},
	})
	if err != nil {
		t.Fatalf("UpdateGoogleChannelConfig: %v", err)
	}
	if cfg.GetCryptoKeyName() != "projects/p/locations/l/keyRings/kr/cryptoKeys/k" {
		t.Errorf("expected crypto_key_name to be updated, got: %s", cfg.GetCryptoKeyName())
	}
}

// -------------------------------------------------------------------------
// MessageBus tests
// -------------------------------------------------------------------------

func TestCreateMessageBus_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	op, err := srv.CreateMessageBus(ctx, &eventarcpb.CreateMessageBusRequest{
		Parent:       parent,
		MessageBusId: "my-bus",
		MessageBus:   &eventarcpb.MessageBus{},
	})
	if err != nil {
		t.Fatalf("CreateMessageBus: %v", err)
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately")
	}

	mb, err := srv.GetMessageBus(ctx, &eventarcpb.GetMessageBusRequest{
		Name: parent + "/messageBuses/my-bus",
	})
	if err != nil {
		t.Fatalf("GetMessageBus after create: %v", err)
	}
	if mb.GetName() != parent+"/messageBuses/my-bus" {
		t.Errorf("unexpected message bus name: %s", mb.GetName())
	}
}

func TestGetMessageBus_NotFound(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	_, err := srv.GetMessageBus(ctx, &eventarcpb.GetMessageBusRequest{
		Name: "projects/p/locations/l/messageBuses/does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

// -------------------------------------------------------------------------
// Enrollment tests
// -------------------------------------------------------------------------

func TestCreateEnrollment_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	op, err := srv.CreateEnrollment(ctx, &eventarcpb.CreateEnrollmentRequest{
		Parent:       parent,
		EnrollmentId: "my-enrollment",
		Enrollment:   &eventarcpb.Enrollment{CelMatch: "message.type == \"test\""},
	})
	if err != nil {
		t.Fatalf("CreateEnrollment: %v", err)
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately")
	}

	en, err := srv.GetEnrollment(ctx, &eventarcpb.GetEnrollmentRequest{
		Name: parent + "/enrollments/my-enrollment",
	})
	if err != nil {
		t.Fatalf("GetEnrollment after create: %v", err)
	}
	if en.GetName() != parent+"/enrollments/my-enrollment" {
		t.Errorf("unexpected enrollment name: %s", en.GetName())
	}
}

// -------------------------------------------------------------------------
// Pipeline tests
// -------------------------------------------------------------------------

// minimalPipeline returns a Pipeline with required fields for creation.
func minimalPipeline() *eventarcpb.Pipeline {
	return &eventarcpb.Pipeline{
		Destinations: []*eventarcpb.Pipeline_Destination{
			{
				DestinationDescriptor: &eventarcpb.Pipeline_Destination_HttpEndpoint_{
					HttpEndpoint: &eventarcpb.Pipeline_Destination_HttpEndpoint{Uri: "http://localhost:8080/events"},
				},
			},
		},
	}
}

func TestCreatePipeline_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	op, err := srv.CreatePipeline(ctx, &eventarcpb.CreatePipelineRequest{
		Parent:     parent,
		PipelineId: "my-pipeline",
		Pipeline:   minimalPipeline(),
	})
	if err != nil {
		t.Fatalf("CreatePipeline: %v", err)
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately")
	}

	pl, err := srv.GetPipeline(ctx, &eventarcpb.GetPipelineRequest{
		Name: parent + "/pipelines/my-pipeline",
	})
	if err != nil {
		t.Fatalf("GetPipeline after create: %v", err)
	}
	if pl.GetName() != parent+"/pipelines/my-pipeline" {
		t.Errorf("unexpected pipeline name: %s", pl.GetName())
	}
}

// -------------------------------------------------------------------------
// GoogleApiSource tests
// -------------------------------------------------------------------------

// minimalGoogleApiSource returns a GoogleApiSource with required fields for creation.
func minimalGoogleApiSource() *eventarcpb.GoogleApiSource {
	return &eventarcpb.GoogleApiSource{
		Destination: "projects/my-project/locations/us-central1/messageBuses/my-bus",
	}
}

func TestCreateGoogleApiSource_Success(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/my-project/locations/us-central1"
	op, err := srv.CreateGoogleApiSource(ctx, &eventarcpb.CreateGoogleApiSourceRequest{
		Parent:            parent,
		GoogleApiSourceId: "my-source",
		GoogleApiSource:   minimalGoogleApiSource(),
	})
	if err != nil {
		t.Fatalf("CreateGoogleApiSource: %v", err)
	}
	if !op.Done {
		t.Errorf("expected operation to be done immediately")
	}

	src, err := srv.GetGoogleApiSource(ctx, &eventarcpb.GetGoogleApiSourceRequest{
		Name: parent + "/googleApiSources/my-source",
	})
	if err != nil {
		t.Fatalf("GetGoogleApiSource after create: %v", err)
	}
	if src.GetName() != parent+"/googleApiSources/my-source" {
		t.Errorf("unexpected google api source name: %s", src.GetName())
	}
}

// -------------------------------------------------------------------------
// TestCreateTrigger_NoDestination
// -------------------------------------------------------------------------

// TestCreateTrigger_NoDestination verifies that CreateTrigger returns
// InvalidArgument when the trigger has no destination set.
func TestCreateTrigger_NoDestination(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	req := &eventarcpb.CreateTriggerRequest{
		Parent:    "projects/p/locations/l",
		TriggerId: "t1",
		Trigger: &eventarcpb.Trigger{
			EventFilters: []*eventarcpb.EventFilter{
				{Attribute: "type", Value: "google.cloud.pubsub.topic.v1.messagePublished"},
			},
		},
	}

	_, err := srv.CreateTrigger(ctx, req)
	if err == nil {
		t.Fatal("expected InvalidArgument error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

// -------------------------------------------------------------------------
// TestCreateTrigger_NoEventFilters
// -------------------------------------------------------------------------

// TestCreateTrigger_NoEventFilters verifies that CreateTrigger returns
// InvalidArgument when the trigger has no event_filters.
func TestCreateTrigger_NoEventFilters(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	req := &eventarcpb.CreateTriggerRequest{
		Parent:    "projects/p/locations/l",
		TriggerId: "t2",
		Trigger: &eventarcpb.Trigger{
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_HttpEndpoint{
					HttpEndpoint: &eventarcpb.HttpEndpoint{
						Uri: "http://localhost:8080/event",
					},
				},
			},
		},
	}

	_, err := srv.CreateTrigger(ctx, req)
	if err == nil {
		t.Fatal("expected InvalidArgument error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

// -------------------------------------------------------------------------
// Conformance fix tests: validate_only, allow_missing, etag, required fields
// -------------------------------------------------------------------------

// TestCreateTrigger_ValidateOnly verifies that validate_only=true returns a
// synthetic LRO without persisting the trigger.
func TestCreateTrigger_ValidateOnly(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	op, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:       parent,
		TriggerId:    "vo-trigger",
		Trigger:      minimalTrigger(),
		ValidateOnly: true,
	})
	if err != nil {
		t.Fatalf("CreateTrigger(validate_only): %v", err)
	}
	if op == nil {
		t.Fatal("expected non-nil operation")
	}
	if !op.Done {
		t.Errorf("expected Done=true for validate_only LRO")
	}
	// The trigger must NOT have been persisted.
	_, err = srv.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{
		Name: parent + "/triggers/vo-trigger",
	})
	if err == nil {
		t.Fatal("expected NotFound after validate_only create, got nil error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

// TestLROMetadata_Create verifies that the LRO metadata contains correct
// verb and api_version fields after creating a trigger.
func TestLROMetadata_Create(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	op, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "meta-trigger",
		Trigger:   minimalTrigger(),
	})
	if err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}
	if op.Metadata == nil {
		t.Fatal("expected non-nil op.Metadata")
	}
	meta := &eventarcpb.OperationMetadata{}
	if err := op.Metadata.UnmarshalTo(meta); err != nil {
		t.Fatalf("UnmarshalTo OperationMetadata: %v", err)
	}
	if meta.Verb != "create" {
		t.Errorf("expected Verb=%q, got %q", "create", meta.Verb)
	}
	if meta.ApiVersion != "v1" {
		t.Errorf("expected ApiVersion=%q, got %q", "v1", meta.ApiVersion)
	}
}

// TestDeleteTrigger_EtagMismatch verifies that DeleteTrigger returns Aborted
// when the provided etag does not match the stored etag.
func TestDeleteTrigger_EtagMismatch(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	triggerName := parent + "/triggers/etag-trigger"
	_, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "etag-trigger",
		Trigger:   minimalTrigger(),
	})
	if err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}

	_, err = srv.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{
		Name: triggerName,
		Etag: "wrong-etag",
	})
	if err == nil {
		t.Fatal("expected error for etag mismatch, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Aborted {
		t.Errorf("expected Aborted, got %s", st.Code())
	}
}

// TestDeleteTrigger_EtagMatch verifies that DeleteTrigger succeeds when the
// provided etag matches the stored etag.
func TestDeleteTrigger_EtagMatch(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	triggerName := parent + "/triggers/etag-match-trigger"
	_, err := srv.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "etag-match-trigger",
		Trigger:   minimalTrigger(),
	})
	if err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}

	// Retrieve to get the stored etag.
	trigger, err := srv.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: triggerName})
	if err != nil {
		t.Fatalf("GetTrigger: %v", err)
	}

	op, err := srv.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{
		Name: triggerName,
		Etag: trigger.GetEtag(),
	})
	if err != nil {
		t.Fatalf("DeleteTrigger with correct etag: %v", err)
	}
	if !op.Done {
		t.Errorf("expected Done=true")
	}
}

// TestDeleteTrigger_AllowMissing verifies that DeleteTrigger with
// allow_missing=true on a non-existent trigger returns success.
func TestDeleteTrigger_AllowMissing(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	op, err := srv.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{
		Name:         "projects/p/locations/l/triggers/does-not-exist",
		AllowMissing: true,
	})
	if err != nil {
		t.Fatalf("DeleteTrigger(allow_missing): %v", err)
	}
	if !op.Done {
		t.Errorf("expected Done=true")
	}
}

// TestUpdateTrigger_AllowMissing verifies that UpdateTrigger with
// allow_missing=true on a non-existent trigger creates it (upsert).
func TestUpdateTrigger_AllowMissing(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	triggerName := parent + "/triggers/new-via-update"

	op, err := srv.UpdateTrigger(ctx, &eventarcpb.UpdateTriggerRequest{
		Trigger:      &eventarcpb.Trigger{Name: triggerName},
		AllowMissing: true,
	})
	if err != nil {
		t.Fatalf("UpdateTrigger(allow_missing): %v", err)
	}
	if !op.Done {
		t.Errorf("expected Done=true")
	}

	// The trigger should now exist.
	_, err = srv.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: triggerName})
	if err != nil {
		t.Fatalf("GetTrigger after allow_missing update: %v", err)
	}
}

// TestDeleteMessageBus_EtagMismatch verifies that DeleteMessageBus returns
// Aborted when the etag does not match.
func TestDeleteMessageBus_EtagMismatch(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	busName := parent + "/messageBuses/etag-bus"
	_, err := srv.CreateMessageBus(ctx, &eventarcpb.CreateMessageBusRequest{
		Parent:       parent,
		MessageBusId: "etag-bus",
		MessageBus:   &eventarcpb.MessageBus{},
	})
	if err != nil {
		t.Fatalf("CreateMessageBus: %v", err)
	}

	_, err = srv.DeleteMessageBus(ctx, &eventarcpb.DeleteMessageBusRequest{
		Name: busName,
		Etag: "wrong-etag",
	})
	if err == nil {
		t.Fatal("expected Aborted for etag mismatch, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Aborted {
		t.Errorf("expected Aborted, got %s", st.Code())
	}
}

// TestDeleteMessageBus_AllowMissing verifies that DeleteMessageBus with
// allow_missing=true on a non-existent bus returns success.
func TestDeleteMessageBus_AllowMissing(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	op, err := srv.DeleteMessageBus(ctx, &eventarcpb.DeleteMessageBusRequest{
		Name:         "projects/p/locations/l/messageBuses/does-not-exist",
		AllowMissing: true,
	})
	if err != nil {
		t.Fatalf("DeleteMessageBus(allow_missing): %v", err)
	}
	if !op.Done {
		t.Errorf("expected Done=true")
	}
}

// TestUpdateMessageBus_AllowMissing verifies that UpdateMessageBus with
// allow_missing=true on a non-existent bus creates it.
func TestUpdateMessageBus_AllowMissing(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	parent := "projects/p/locations/l"
	busName := parent + "/messageBuses/new-via-update"

	op, err := srv.UpdateMessageBus(ctx, &eventarcpb.UpdateMessageBusRequest{
		MessageBus:   &eventarcpb.MessageBus{Name: busName},
		AllowMissing: true,
	})
	if err != nil {
		t.Fatalf("UpdateMessageBus(allow_missing): %v", err)
	}
	if !op.Done {
		t.Errorf("expected Done=true")
	}

	// The bus should now exist.
	_, err = srv.GetMessageBus(ctx, &eventarcpb.GetMessageBusRequest{Name: busName})
	if err != nil {
		t.Fatalf("GetMessageBus after allow_missing update: %v", err)
	}
}

// TestCreateEnrollment_MissingCelMatch verifies that CreateEnrollment returns
// InvalidArgument when enrollment.cel_match is empty.
func TestCreateEnrollment_MissingCelMatch(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	_, err := srv.CreateEnrollment(ctx, &eventarcpb.CreateEnrollmentRequest{
		Parent:       "projects/p/locations/l",
		EnrollmentId: "bad-enrollment",
		Enrollment:   &eventarcpb.Enrollment{},
	})
	if err == nil {
		t.Fatal("expected InvalidArgument, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

// TestCreatePipeline_MissingDestinations verifies that CreatePipeline returns
// InvalidArgument when pipeline.destinations is empty.
func TestCreatePipeline_MissingDestinations(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	_, err := srv.CreatePipeline(ctx, &eventarcpb.CreatePipelineRequest{
		Parent:     "projects/p/locations/l",
		PipelineId: "bad-pipeline",
		Pipeline:   &eventarcpb.Pipeline{},
	})
	if err == nil {
		t.Fatal("expected InvalidArgument, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

// TestCreateGoogleApiSource_MissingDestination verifies that
// CreateGoogleApiSource returns InvalidArgument when destination is empty.
func TestCreateGoogleApiSource_MissingDestination(t *testing.T) {
	ctx := context.Background()
	srv := newTestServer(t)

	_, err := srv.CreateGoogleApiSource(ctx, &eventarcpb.CreateGoogleApiSourceRequest{
		Parent:            "projects/p/locations/l",
		GoogleApiSourceId: "bad-source",
		GoogleApiSource:   &eventarcpb.GoogleApiSource{},
	})
	if err == nil {
		t.Fatal("expected InvalidArgument, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}
