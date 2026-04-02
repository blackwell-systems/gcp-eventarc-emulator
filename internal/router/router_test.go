package router_test

import (
	"context"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/router"
)

// fakeStorage implements StorageReader for testing.
type fakeStorage struct {
	triggers []*eventarcpb.Trigger
}

func (f *fakeStorage) ListAllTriggers(_ context.Context, _ string) ([]*eventarcpb.Trigger, error) {
	return f.triggers, nil
}

// newTestEvent builds a CloudEvent with common attributes set.
func newTestEvent(eventType, source, subject string) cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetID("test-id-1")
	e.SetType(eventType)
	e.SetSource(source)
	e.SetSubject(subject)
	return e
}

func TestMatch_ExactFilter_Matches(t *testing.T) {
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/t1",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "type", Value: "google.cloud.pubsub.topic.v1.messagePublished"},
				},
			},
		},
	}
	r := router.NewRouter(store)
	e := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", "")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	if got[0].Name != store.triggers[0].Name {
		t.Errorf("expected trigger %q, got %q", store.triggers[0].Name, got[0].Name)
	}
}

func TestMatch_ExactFilter_NoMatch(t *testing.T) {
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/t1",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "type", Value: "google.cloud.storage.object.v1.finalized"},
				},
			},
		},
	}
	r := router.NewRouter(store)
	e := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", "")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(got))
	}
}

func TestMatch_MultipleFilters_AllMustMatch(t *testing.T) {
	trigger := &eventarcpb.Trigger{
		Name: "projects/p/locations/us-central1/triggers/t1",
		EventFilters: []*eventarcpb.EventFilter{
			{Attribute: "type", Value: "google.cloud.pubsub.topic.v1.messagePublished"},
			{Attribute: "source", Value: "//pubsub.googleapis.com/projects/p/topics/my-topic"},
		},
	}
	store := &fakeStorage{triggers: []*eventarcpb.Trigger{trigger}}
	r := router.NewRouter(store)

	// Both filters match.
	eMatch := cloudevents.NewEvent()
	eMatch.SetID("id1")
	eMatch.SetType("google.cloud.pubsub.topic.v1.messagePublished")
	eMatch.SetSource("//pubsub.googleapis.com/projects/p/topics/my-topic")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match (both filters satisfied), got %d", len(got))
	}

	// Only the type filter matches; source does not.
	eNoMatch := cloudevents.NewEvent()
	eNoMatch.SetID("id2")
	eNoMatch.SetType("google.cloud.pubsub.topic.v1.messagePublished")
	eNoMatch.SetSource("//pubsub.googleapis.com/projects/p/topics/other-topic")

	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", eNoMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 matches (source filter not satisfied), got %d", len(got2))
	}
}

func TestMatch_EmptyTriggers_ReturnsEmpty(t *testing.T) {
	store := &fakeStorage{triggers: nil}
	r := router.NewRouter(store)
	e := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", "")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for empty trigger store, got %d", len(got))
	}
}

func TestMatch_NoFilters_MatchesAll(t *testing.T) {
	// A trigger with no event_filters should match any event.
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{Name: "projects/p/locations/us-central1/triggers/catch-all"},
		},
	}
	r := router.NewRouter(store)

	for _, eventType := range []string{
		"google.cloud.pubsub.topic.v1.messagePublished",
		"google.cloud.storage.object.v1.finalized",
		"custom.domain/event.type",
	} {
		e := newTestEvent(eventType, "//example.com/source", "subject")
		got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
		if err != nil {
			t.Fatalf("unexpected error for type %q: %v", eventType, err)
		}
		if len(got) != 1 {
			t.Errorf("expected 1 match for type %q (no filters), got %d", eventType, len(got))
		}
	}
}

func TestAttrValue_StandardAttrs(t *testing.T) {
	// We test attrValue indirectly through Match by filtering on each standard attribute.
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "t-type",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "type", Value: "my.type"},
				},
			},
			{
				Name: "t-source",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "source", Value: "//my.source"},
				},
			},
			{
				Name: "t-subject",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "subject", Value: "my-subject"},
				},
			},
			{
				Name: "t-id",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "id", Value: "my-id"},
				},
			},
			{
				Name: "t-datacontenttype",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "datacontenttype", Value: "application/json"},
				},
			},
		},
	}
	r := router.NewRouter(store)

	e := cloudevents.NewEvent()
	e.SetID("my-id")
	e.SetType("my.type")
	e.SetSource("//my.source")
	e.SetSubject("my-subject")
	e.SetDataContentType("application/json")

	got, err := r.Match(context.Background(), "projects/p/locations/l", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("expected 5 triggers matched (one per standard attr), got %d", len(got))
	}
}

func TestAttrValue_ExtensionAttr(t *testing.T) {
	// Use a custom extension name not reserved by the CloudEvents v1 spec.
	// (Names like "dataschema" are spec attributes and cannot be set as extensions.)
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/ext-trigger",
				EventFilters: []*eventarcpb.EventFilter{
					{Attribute: "customtag", Value: "my-tag-value"},
				},
			},
		},
	}
	r := router.NewRouter(store)

	// Match: custom extension attribute present and equal.
	eMatch := cloudevents.NewEvent()
	eMatch.SetID("id1")
	eMatch.SetType("some.type")
	eMatch.SetSource("//some.source")
	eMatch.SetExtension("customtag", "my-tag-value")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for extension attr, got %d", len(got))
	}

	// No match: extension attribute missing.
	eNoExt := cloudevents.NewEvent()
	eNoExt.SetID("id2")
	eNoExt.SetType("some.type")
	eNoExt.SetSource("//some.source")

	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", eNoExt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 matches when extension attr absent, got %d", len(got2))
	}
}
