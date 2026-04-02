package router

import (
	"context"
	"fmt"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// StorageReader is the subset of *server.Storage needed by the router.
// Defined as an interface to avoid circular imports between router and server packages.
type StorageReader interface {
	ListAllTriggers(ctx context.Context, parent string) ([]*eventarcpb.Trigger, error)
}

// Router matches CloudEvents against Eventarc triggers.
type Router struct {
	storage StorageReader
}

// NewRouter creates a new Router backed by the given StorageReader.
func NewRouter(storage StorageReader) *Router {
	return &Router{storage: storage}
}

// Match returns all triggers in the given project/location whose
// event_filters all match the provided CloudEvent.
// Attributes are read from event.Type(), event.Source(), event.Subject(),
// and event.Extensions() for custom attributes.
// A trigger with zero event_filters matches all events.
func (r *Router) Match(ctx context.Context, parent string, event cloudevents.Event) ([]*eventarcpb.Trigger, error) {
	triggers, err := r.storage.ListAllTriggers(ctx, parent)
	if err != nil {
		return nil, err
	}

	var matched []*eventarcpb.Trigger
	for _, t := range triggers {
		if triggerMatches(t, event) {
			matched = append(matched, t)
		}
	}
	return matched, nil
}

// triggerMatches returns true if all of the trigger's event_filters match the event.
// A trigger with no filters matches every event.
func triggerMatches(trigger *eventarcpb.Trigger, event cloudevents.Event) bool {
	for _, f := range trigger.GetEventFilters() {
		eventVal := attrValue(event, f.GetAttribute())
		// Operator "" and "match-path-pattern" both use exact match for now.
		if eventVal != f.GetValue() {
			return false
		}
	}
	return true
}

// attrValue extracts a named attribute from a CloudEvent.
// Checks standard attributes (type, source, subject, id, datacontenttype)
// then falls back to Extensions().
// Matching is case-sensitive per the CloudEvents spec.
func attrValue(event cloudevents.Event, attr string) string {
	switch attr {
	case "type":
		return event.Type()
	case "source":
		return event.Source()
	case "subject":
		return event.Subject()
	case "id":
		return event.ID()
	case "datacontenttype":
		return event.DataContentType()
	default:
		exts := event.Extensions()
		if exts == nil {
			return ""
		}
		v, ok := exts[attr]
		if !ok {
			return ""
		}
		// Extensions values are interface{} — the SDK normalizes strings to
		// typed values (e.g. types.URIRef for URLs). Use fmt.Sprintf to get
		// the canonical string representation regardless of the underlying type.
		return fmt.Sprintf("%s", v)
	}
}
