// Package publisher implements the Eventarc Publishing API gRPC service.
//
// It exposes a Server that receives CloudEvents from clients, matches them
// against stored Eventarc triggers via a RouterMatcher, and forwards each match
// to trigger destinations via an EventDispatcher.
//
// The service implements google.cloud.eventarc.publishing.v1.Publisher and is
// registered on the shared gRPC server in cmd/server/main.go via
// server.NewGRPCServer.
package publisher

import (
	"context"
	"fmt"
	"log"
	"strings"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/types/known/anypb"
)

// RouterMatcher is the interface subset of *router.Router needed by the publisher.
// Using an interface avoids an import cycle: publisher→router→server→publisher.
type RouterMatcher interface {
	Match(ctx context.Context, parent string, event cloudevents.Event) ([]*eventarcpb.Trigger, error)
}

// EventDispatcher is the interface subset of *dispatcher.Dispatcher needed by the publisher.
type EventDispatcher interface {
	Dispatch(ctx context.Context, trigger *eventarcpb.Trigger, event cloudevents.Event) (int, error)
}

// Server implements google.cloud.eventarc.publishing.v1.PublisherServer.
type Server struct {
	publishingpb.UnimplementedPublisherServer
	router     RouterMatcher
	dispatcher EventDispatcher
}

// NewServer creates a new publisher Server.
func NewServer(router RouterMatcher, dispatcher EventDispatcher) *Server {
	return &Server{
		router:     router,
		dispatcher: dispatcher,
	}
}

// PublishEvents implements PublisherServer.
// It unpacks each event, matches against triggers under the channel's parent
// location, and dispatches to every matching trigger.
// Errors per-event or per-dispatch are logged but do not fail the RPC.
func (s *Server) PublishEvents(ctx context.Context, req *publishingpb.PublishEventsRequest) (*publishingpb.PublishEventsResponse, error) {
	parent := parentFromChannel(req.GetChannel())
	s.processAnySlice(ctx, parent, req.GetEvents())
	return &publishingpb.PublishEventsResponse{}, nil
}

// PublishChannelConnectionEvents implements PublisherServer (partner use case).
// It derives the parent from the channel_connection resource name, then follows
// the same match-and-dispatch flow as PublishEvents.
func (s *Server) PublishChannelConnectionEvents(ctx context.Context, req *publishingpb.PublishChannelConnectionEventsRequest) (*publishingpb.PublishChannelConnectionEventsResponse, error) {
	parent := parentFromChannelConnection(req.GetChannelConnection())
	s.processAnySlice(ctx, parent, req.GetEvents())
	return &publishingpb.PublishChannelConnectionEventsResponse{}, nil
}

// processAnySlice unpacks each Any-wrapped CloudEvent proto message, converts it
// to a cloudevents.Event, matches against triggers, and dispatches to each match.
// Decode and dispatch errors are logged and skipped; they do not fail the RPC.
func (s *Server) processAnySlice(ctx context.Context, parent string, anySlice []*anypb.Any) {
	for _, a := range anySlice {
		if a == nil {
			continue
		}
		event, err := unpackCloudEvent(a)
		if err != nil {
			log.Printf("publisher: failed to unpack CloudEvent from Any: %v, skipping", err)
			continue
		}
		s.matchAndDispatch(ctx, parent, event)
	}
}

// matchAndDispatch calls router.Match followed by dispatcher.Dispatch for each
// matching trigger. Errors are logged but do not abort processing of remaining
// triggers or events.
func (s *Server) matchAndDispatch(ctx context.Context, parent string, event cloudevents.Event) {
	triggers, err := s.router.Match(ctx, parent, event)
	if err != nil {
		log.Printf("publisher: router.Match error (parent=%s): %v", parent, err)
		return
	}
	for _, t := range triggers {
		statusCode, err := s.dispatcher.Dispatch(ctx, t, event)
		if err != nil {
			log.Printf("publisher: dispatch error (trigger=%s): %v", t.GetName(), err)
		} else if statusCode >= 400 {
			log.Printf("publisher: non-2xx dispatch status %d for trigger %s", statusCode, t.GetName())
		}
	}
}

// unpackCloudEvent unpacks a protobuf Any containing a publishingpb.CloudEvent
// and converts it to a cloudevents.Event (sdk-go type).
// The proto CloudEvent encodes required attrs (id, source, type, specversion) as
// top-level fields and optional attrs (subject, datacontenttype, etc.) in the
// Attributes map as CloudEventAttributeValue values.
func unpackCloudEvent(a *anypb.Any) (cloudevents.Event, error) {
	var protoEvent publishingpb.CloudEvent
	if err := a.UnmarshalTo(&protoEvent); err != nil {
		return cloudevents.Event{}, fmt.Errorf("unpack Any: %w", err)
	}
	return protoCloudEventToSDK(&protoEvent)
}

// protoCloudEventToSDK converts a publishingpb.CloudEvent to a cloudevents.Event.
func protoCloudEventToSDK(p *publishingpb.CloudEvent) (cloudevents.Event, error) {
	e := cloudevents.NewEvent()

	// Required attributes
	e.SetID(p.GetId())
	e.SetSource(p.GetSource())
	e.SetType(p.GetType())
	specVer := p.GetSpecVersion()
	if specVer == "" {
		specVer = "1.0"
	}
	e.SetSpecVersion(specVer)

	// Optional attributes stored in the Attributes map
	for k, v := range p.GetAttributes() {
		if v == nil {
			continue
		}
		val := attrValueString(v)
		switch k {
		case "subject":
			e.SetSubject(val)
		case "datacontenttype":
			e.SetDataContentType(val)
		case "dataschema":
			e.SetDataSchema(val)
		default:
			// Extension attribute
			e.SetExtension(k, val)
		}
	}

	// Data
	switch d := p.GetData().(type) {
	case *publishingpb.CloudEvent_BinaryData:
		contentType := e.DataContentType()
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if err := e.SetData(contentType, d.BinaryData); err != nil {
			return e, fmt.Errorf("set binary data: %w", err)
		}
	case *publishingpb.CloudEvent_TextData:
		contentType := e.DataContentType()
		if contentType == "" {
			contentType = "text/plain"
		}
		if err := e.SetData(contentType, d.TextData); err != nil {
			return e, fmt.Errorf("set text data: %w", err)
		}
	case nil:
		// No data
	}

	return e, nil
}

// attrValueString extracts the string representation of a CloudEventAttributeValue.
// Non-string types are formatted as strings for matching purposes.
func attrValueString(v *publishingpb.CloudEvent_CloudEventAttributeValue) string {
	switch a := v.GetAttr().(type) {
	case *publishingpb.CloudEvent_CloudEventAttributeValue_CeString:
		return a.CeString
	case *publishingpb.CloudEvent_CloudEventAttributeValue_CeUri:
		return a.CeUri
	case *publishingpb.CloudEvent_CloudEventAttributeValue_CeUriRef:
		return a.CeUriRef
	case *publishingpb.CloudEvent_CloudEventAttributeValue_CeBoolean:
		return fmt.Sprintf("%t", a.CeBoolean)
	case *publishingpb.CloudEvent_CloudEventAttributeValue_CeInteger:
		return fmt.Sprintf("%d", a.CeInteger)
	case *publishingpb.CloudEvent_CloudEventAttributeValue_CeTimestamp:
		if a.CeTimestamp != nil {
			return a.CeTimestamp.AsTime().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
		return ""
	default:
		return ""
	}
}

// parentFromChannel derives a projects/.../locations/... parent from a full
// channel name. e.g. "projects/p/locations/l/channels/c" → "projects/p/locations/l"
func parentFromChannel(channel string) string {
	if idx := strings.LastIndex(channel, "/channels/"); idx >= 0 {
		return channel[:idx]
	}
	return channel
}

// parentFromChannelConnection derives a parent from a channel_connection name.
// e.g. "projects/p/locations/l/channelConnections/cc" → "projects/p/locations/l"
func parentFromChannelConnection(cc string) string {
	if idx := strings.LastIndex(cc, "/channelConnections/"); idx >= 0 {
		return cc[:idx]
	}
	return cc
}
