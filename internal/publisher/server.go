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
	"strings"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishingpb "cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/logger"
)

// ChannelChecker is the interface needed by the publisher to validate
// that a channel exists before routing events to it.
type ChannelChecker interface {
	GetChannelExists(ctx context.Context, channelName string) (bool, error)
}

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
	channels   ChannelChecker
	logger     *logger.Logger
}

// NewServer creates a new publisher Server.
// An optional *logger.Logger may be supplied as the fourth argument;
// if omitted or nil, defaults to info level.
func NewServer(router RouterMatcher, dispatcher EventDispatcher, channels ChannelChecker, log ...*logger.Logger) *Server {
	var lgr *logger.Logger
	if len(log) > 0 {
		lgr = log[0]
	}
	return &Server{
		router:     router,
		dispatcher: dispatcher,
		channels:   channels,
		logger:     logger.OrDefault(lgr),
	}
}

// PublishEvents implements PublisherServer.
// It unpacks each event, matches against triggers under the channel's parent
// location, and dispatches to every matching trigger.
// Errors per-event or per-dispatch are logged but do not fail the RPC.
func (s *Server) PublishEvents(ctx context.Context, req *publishingpb.PublishEventsRequest) (*publishingpb.PublishEventsResponse, error) {
	if req.GetChannel() != "" {
		exists, err := s.channels.GetChannelExists(ctx, req.GetChannel())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "channel lookup failed: %v", err)
		}
		if !exists {
			return nil, status.Errorf(codes.NotFound, "Channel %q not found", req.GetChannel())
		}
	}
	if len(req.GetEvents()) == 0 {
		s.logger.Info("publisher: PublishEvents called with 0 events for channel=%s", req.GetChannel())
	}
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
			s.logger.Error("publisher: failed to unpack CloudEvent from Any: %v, skipping", err)
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
		s.logger.Error("publisher: router.Match error (parent=%s): %v", parent, err)
		return
	}
	s.logger.Debug("publish: matched %d triggers channel=%s", len(triggers), parent)
	if len(triggers) > 0 {
		s.logger.Info("publisher: %d trigger(s) matched for parent=%s type=%s",
			len(triggers), parent, event.Type())
	}
	for _, t := range triggers {
		s.logger.Debug("publish: dispatch trigger=%s", t.GetName())
		statusCode, err := s.dispatcher.Dispatch(ctx, t, event)
		if err != nil {
			s.logger.Error("publisher: dispatch error (trigger=%s): %v", t.GetName(), err)
		} else if statusCode >= 400 {
			s.logger.Warn("publisher: non-2xx dispatch status %d for trigger %s", statusCode, t.GetName())
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
