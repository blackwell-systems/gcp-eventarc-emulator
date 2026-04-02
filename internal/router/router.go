package router

import (
	"context"
	"fmt"
	"log"

	"github.com/google/cel-go/cel"

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
// If trigger.Condition is non-empty, it is evaluated as a CEL expression
// against the event attributes; triggers with a false or error result are skipped.
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

// conditionLabelKey is the Labels key used to store an optional CEL condition
// expression on a trigger. This is an emulator extension — real Eventarc
// triggers do not have a first-class condition field in the v1 API.
const conditionLabelKey = "eventarc-emulator/condition"

// triggerMatches returns true if all of the trigger's event_filters match the event
// and (if set) the trigger's condition label CEL expression evaluates to true.
// A trigger with no filters and no condition matches every event.
func triggerMatches(trigger *eventarcpb.Trigger, event cloudevents.Event) bool {
	for _, f := range trigger.GetEventFilters() {
		eventVal := attrValue(event, f.GetAttribute())
		// Operator "" and "match-path-pattern" both use exact match for now.
		if eventVal != f.GetValue() {
			return false
		}
	}

	condition := trigger.GetLabels()[conditionLabelKey]
	if condition == "" {
		return true
	}
	return evalCELCondition(condition, event)
}

// evalCELCondition evaluates a CEL expression against a CloudEvent's attributes.
// The environment exposes: type, source, subject, id (all strings), plus every
// extension attribute as a string. Returns false on any compilation or runtime error.
func evalCELCondition(condition string, event cloudevents.Event) bool {
	// Build the CEL variable declarations from known attributes + extensions.
	attrs := eventAttrsAsStrings(event)

	// Declare one string variable per attribute present in this event.
	vars := make([]cel.EnvOption, 0, len(attrs))
	for k := range attrs {
		vars = append(vars, cel.Variable(k, cel.StringType))
	}

	env, err := cel.NewEnv(vars...)
	if err != nil {
		log.Printf("router: CEL env creation failed for condition %q: %v", condition, err)
		return false
	}

	ast, issues := env.Compile(condition)
	if issues != nil && issues.Err() != nil {
		log.Printf("router: CEL compile error for condition %q: %v", condition, issues.Err())
		return false
	}

	prg, err := env.Program(ast)
	if err != nil {
		log.Printf("router: CEL program creation failed for condition %q: %v", condition, err)
		return false
	}

	// Convert attrs to map[string]any for the activation.
	activation := make(map[string]any, len(attrs))
	for k, v := range attrs {
		activation[k] = v
	}

	out, _, err := prg.Eval(activation)
	if err != nil {
		log.Printf("router: CEL eval error for condition %q: %v", condition, err)
		return false
	}

	result, ok := out.Value().(bool)
	if !ok {
		log.Printf("router: CEL condition %q did not return a bool", condition)
		return false
	}
	return result
}

// eventAttrsAsStrings returns a map of all CloudEvent attributes (standard +
// extensions) as strings, suitable for use as a CEL activation.
func eventAttrsAsStrings(event cloudevents.Event) map[string]string {
	m := map[string]string{
		"type":    event.Type(),
		"source":  event.Source(),
		"subject": event.Subject(),
		"id":      event.ID(),
	}
	for k, v := range event.Extensions() {
		m[k] = fmt.Sprintf("%s", v)
	}
	return m
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
