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

// --- match-path-pattern operator tests ---

func TestMatch_MatchPathPatternOperator_SingleWildcard(t *testing.T) {
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/gcs-trigger",
				EventFilters: []*eventarcpb.EventFilter{
					{
						Attribute: "source",
						Value:     "//storage.googleapis.com/projects/_/buckets/*/objects/*",
						Operator:  "match-path-pattern",
					},
				},
			},
		},
	}
	r := router.NewRouter(store)

	// Should match: pattern has two single wildcards, event has one segment each.
	eMatch := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//storage.googleapis.com/projects/_/buckets/my-bucket/objects/my-obj", "")
	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for single-wildcard path pattern, got %d", len(got))
	}

	// Should NOT match: extra segment after objects wildcard.
	eExtra := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//storage.googleapis.com/projects/_/buckets/my-bucket/objects/foo/bar", "")
	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", eExtra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 matches for extra segment, got %d", len(got2))
	}

	// Should NOT match: different host.
	eWrongHost := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//other.googleapis.com/projects/_/buckets/x/objects/y", "")
	got3, err := r.Match(context.Background(), "projects/p/locations/us-central1", eWrongHost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got3) != 0 {
		t.Fatalf("expected 0 matches for wrong host, got %d", len(got3))
	}
}

func TestMatch_MatchPathPatternOperator_DoubleWildcard(t *testing.T) {
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/gcs-prefix-trigger",
				EventFilters: []*eventarcpb.EventFilter{
					{
						Attribute: "source",
						Value:     "//storage.googleapis.com/projects/**",
						Operator:  "match-path-pattern",
					},
				},
			},
		},
	}
	r := router.NewRouter(store)

	// Should match: multiple segments after "projects".
	eDeep := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//storage.googleapis.com/projects/foo/bar/baz", "")
	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eDeep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for deep path with **, got %d", len(got))
	}

	// Should match: ** matches empty segment sequence (trailing slash).
	eEmpty := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//storage.googleapis.com/projects/", "")
	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", eEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 1 {
		t.Fatalf("expected 1 match for trailing-slash path with **, got %d", len(got2))
	}

	// Should NOT match: different host prefix.
	eWrongHost := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//other.googleapis.com/projects/foo", "")
	got3, err := r.Match(context.Background(), "projects/p/locations/us-central1", eWrongHost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got3) != 0 {
		t.Fatalf("expected 0 matches for wrong host with **, got %d", len(got3))
	}
}

func TestMatch_PathPatternOperatorAlias(t *testing.T) {
	// Same scenario as SingleWildcard but with Operator="path_pattern".
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/alias-trigger",
				EventFilters: []*eventarcpb.EventFilter{
					{
						Attribute: "source",
						Value:     "//storage.googleapis.com/projects/_/buckets/*/objects/*",
						Operator:  "path_pattern",
					},
				},
			},
		},
	}
	r := router.NewRouter(store)

	eMatch := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//storage.googleapis.com/projects/_/buckets/my-bucket/objects/my-obj", "")
	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for path_pattern alias operator, got %d", len(got))
	}

	eNoMatch := newTestEvent("google.cloud.storage.object.v1.finalized",
		"//storage.googleapis.com/projects/_/buckets/my-bucket/objects/foo/bar", "")
	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", eNoMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 matches for extra segment with path_pattern alias, got %d", len(got2))
	}
}

func TestMatch_ExactMatchOperator_Unchanged(t *testing.T) {
	// Verify that operator="" still performs exact match and is not accidentally changed.
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{
				Name: "projects/p/locations/us-central1/triggers/exact-trigger",
				EventFilters: []*eventarcpb.EventFilter{
					{
						Attribute: "source",
						Value:     "//storage.googleapis.com/projects/_/buckets/*/objects/*",
						Operator:  "", // empty operator => exact match
					},
				},
			},
		},
	}
	r := router.NewRouter(store)

	// The literal pattern string should match exactly.
	eExact := newTestEvent("any.type",
		"//storage.googleapis.com/projects/_/buckets/*/objects/*", "")
	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eExact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for exact operator with literal pattern string, got %d", len(got))
	}

	// A path that would match the pattern but not the literal should NOT match.
	ePatternMatch := newTestEvent("any.type",
		"//storage.googleapis.com/projects/_/buckets/my-bucket/objects/my-obj", "")
	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", ePatternMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 matches when exact operator used with non-literal source, got %d", len(got2))
	}
}

// --- CEL condition tests ---

// triggerWithCondition builds a trigger with a CEL condition stored in Labels.
func triggerWithCondition(name, condition string) *eventarcpb.Trigger {
	return &eventarcpb.Trigger{
		Name:   name,
		Labels: map[string]string{"eventarc-emulator/condition": condition},
	}
}

func TestMatch_CEL_EmptyCondition_Matches(t *testing.T) {
	// A trigger with no condition label should match any event (existing behaviour).
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			{Name: "projects/p/locations/us-central1/triggers/no-condition"},
		},
	}
	r := router.NewRouter(store)
	e := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", "")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for empty condition, got %d", len(got))
	}
}

func TestMatch_CEL_TrueCondition_Matches(t *testing.T) {
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			triggerWithCondition("projects/p/locations/us-central1/triggers/cel-true", `type == "google.cloud.pubsub.topic.v1.messagePublished"`),
		},
	}
	r := router.NewRouter(store)
	e := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", "")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for true CEL condition, got %d", len(got))
	}
}

func TestMatch_CEL_FalseCondition_Excludes(t *testing.T) {
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			triggerWithCondition("projects/p/locations/us-central1/triggers/cel-false", `type == "google.cloud.storage.object.v1.finalized"`),
		},
	}
	r := router.NewRouter(store)
	e := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/t", "")

	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 matches for false CEL condition, got %d", len(got))
	}
}

func TestMatch_CEL_ExpressionUsingEventAttrs(t *testing.T) {
	// Compound expression referencing multiple event attributes.
	cond := `type == "google.cloud.pubsub.topic.v1.messagePublished" && source == "//pubsub.googleapis.com/projects/p/topics/my-topic"`
	store := &fakeStorage{
		triggers: []*eventarcpb.Trigger{
			triggerWithCondition("projects/p/locations/us-central1/triggers/cel-compound", cond),
		},
	}
	r := router.NewRouter(store)

	// Both attributes match — should match.
	eMatch := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/my-topic", "")
	got, err := r.Match(context.Background(), "projects/p/locations/us-central1", eMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match for compound CEL (both attrs match), got %d", len(got))
	}

	// Source doesn't match — should be excluded.
	eNoMatch := newTestEvent("google.cloud.pubsub.topic.v1.messagePublished", "//pubsub.googleapis.com/projects/p/topics/other", "")
	got2, err := r.Match(context.Background(), "projects/p/locations/us-central1", eNoMatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("expected 0 matches for compound CEL (source mismatch), got %d", len(got2))
	}
}
