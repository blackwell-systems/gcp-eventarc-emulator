package publisher

import (
	"context"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/types/known/anypb"
)

// --- mock implementations ---

type mockRouter struct {
	triggers   []*eventarcpb.Trigger
	err        error
	lastParent string
}

func (m *mockRouter) Match(_ context.Context, parent string, _ cloudevents.Event) ([]*eventarcpb.Trigger, error) {
	m.lastParent = parent
	return m.triggers, m.err
}

type dispatchCall struct {
	trigger *eventarcpb.Trigger
	event   cloudevents.Event
}

type mockDispatcher struct {
	calls      []dispatchCall
	statusCode int
	err        error
}

func (m *mockDispatcher) Dispatch(_ context.Context, trigger *eventarcpb.Trigger, event cloudevents.Event) (int, error) {
	m.calls = append(m.calls, dispatchCall{trigger: trigger, event: event})
	return m.statusCode, m.err
}

// makeProtoCloudEvent creates an anypb.Any wrapping a publishingpb.CloudEvent
// proto message — the format used inside PublishEventsRequest.Events.
func makeProtoCloudEvent(t *testing.T, eventType, source, subject string) *anypb.Any {
	t.Helper()
	attrs := map[string]*publishingpb.CloudEvent_CloudEventAttributeValue{}
	if subject != "" {
		attrs["subject"] = &publishingpb.CloudEvent_CloudEventAttributeValue{
			Attr: &publishingpb.CloudEvent_CloudEventAttributeValue_CeString{
				CeString: subject,
			},
		}
	}
	ce := &publishingpb.CloudEvent{
		Id:          "test-id",
		Source:      source,
		SpecVersion: "1.0",
		Type:        eventType,
		Attributes:  attrs,
	}
	a, err := anypb.New(ce)
	if err != nil {
		t.Fatalf("makeProtoCloudEvent: anypb.New: %v", err)
	}
	return a
}

// TestPublishEvents_MatchingTrigger_Dispatches verifies that when the router
// returns a matching trigger the dispatcher is called exactly once.
func TestPublishEvents_MatchingTrigger_Dispatches(t *testing.T) {
	trigger := &eventarcpb.Trigger{Name: "projects/p/locations/l/triggers/t1"}
	rtr := &mockRouter{triggers: []*eventarcpb.Trigger{trigger}}
	dsp := &mockDispatcher{statusCode: 200}

	srv := NewServer(rtr, dsp)

	req := &publishingpb.PublishEventsRequest{
		Channel: "projects/p/locations/l/channels/c",
		Events: []*anypb.Any{
			makeProtoCloudEvent(t, "google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", ""),
		},
	}

	resp, err := srv.PublishEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("PublishEvents returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("PublishEvents returned nil response")
	}
	if len(dsp.calls) != 1 {
		t.Errorf("expected 1 dispatch call, got %d", len(dsp.calls))
	}
	if dsp.calls[0].trigger.GetName() != trigger.GetName() {
		t.Errorf("dispatched to wrong trigger: got %s, want %s", dsp.calls[0].trigger.GetName(), trigger.GetName())
	}
	if rtr.lastParent != "projects/p/locations/l" {
		t.Errorf("router called with wrong parent: got %s, want projects/p/locations/l", rtr.lastParent)
	}
}

// TestPublishEvents_NoMatchingTriggers_ReturnsEmpty verifies that when the
// router returns no triggers the RPC still succeeds and no dispatch occurs.
func TestPublishEvents_NoMatchingTriggers_ReturnsEmpty(t *testing.T) {
	rtr := &mockRouter{triggers: nil}
	dsp := &mockDispatcher{statusCode: 200}

	srv := NewServer(rtr, dsp)

	req := &publishingpb.PublishEventsRequest{
		Channel: "projects/p/locations/l/channels/c",
		Events: []*anypb.Any{
			makeProtoCloudEvent(t, "google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", ""),
		},
	}

	resp, err := srv.PublishEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("PublishEvents returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("PublishEvents returned nil response")
	}
	if len(dsp.calls) != 0 {
		t.Errorf("expected 0 dispatch calls, got %d", len(dsp.calls))
	}
}

// TestPublishChannelConnectionEvents_Dispatches verifies that
// PublishChannelConnectionEvents derives the correct parent and dispatches.
func TestPublishChannelConnectionEvents_Dispatches(t *testing.T) {
	trigger := &eventarcpb.Trigger{Name: "projects/partner/locations/us-central1/triggers/t1"}
	rtr := &mockRouter{triggers: []*eventarcpb.Trigger{trigger}}
	dsp := &mockDispatcher{statusCode: 200}

	srv := NewServer(rtr, dsp)

	req := &publishingpb.PublishChannelConnectionEventsRequest{
		ChannelConnection: "projects/partner/locations/us-central1/channelConnections/conn1",
		Events: []*anypb.Any{
			makeProtoCloudEvent(t, "partner.event.v1", "//partner.example.com/source", "subject"),
		},
	}

	resp, err := srv.PublishChannelConnectionEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("PublishChannelConnectionEvents returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("PublishChannelConnectionEvents returned nil response")
	}
	if len(dsp.calls) != 1 {
		t.Errorf("expected 1 dispatch call, got %d", len(dsp.calls))
	}
	if rtr.lastParent != "projects/partner/locations/us-central1" {
		t.Errorf("router called with wrong parent: got %s, want projects/partner/locations/us-central1", rtr.lastParent)
	}
}
