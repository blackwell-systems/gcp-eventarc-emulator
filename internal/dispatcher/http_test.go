package dispatcher

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// newTestEvent builds a minimal CloudEvent for testing.
func newTestEvent() cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType("google.cloud.pubsub.topic.v1.messagePublished")
	e.SetSource("//pubsub.googleapis.com/projects/p/topics/t")
	e.SetID("test-id")
	return e
}

// triggerWithHTTPEndpoint builds a trigger pointing at the given URI.
func triggerWithHTTPEndpoint(uri string) *eventarcpb.Trigger {
	return &eventarcpb.Trigger{
		Destination: &eventarcpb.Destination{
			Descriptor_: &eventarcpb.Destination_HttpEndpoint{
				HttpEndpoint: &eventarcpb.HttpEndpoint{Uri: uri},
			},
		},
	}
}

// triggerWithCloudRun builds a trigger with a Cloud Run destination.
func triggerWithCloudRun(path string) *eventarcpb.Trigger {
	return &eventarcpb.Trigger{
		Destination: &eventarcpb.Destination{
			Descriptor_: &eventarcpb.Destination_CloudRun{
				CloudRun: &eventarcpb.CloudRun{Path: path},
			},
		},
	}
}

func TestDispatch_HttpEndpoint_Success(t *testing.T) {
	var (
		gotMethod      string
		gotBody        []byte
		gotContentType string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(nil)
	trigger := triggerWithHTTPEndpoint(srv.URL)

	code, err := d.Dispatch(context.Background(), trigger, newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("expected status 200, got %d", code)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotContentType != cloudEventsJSONContentType {
		t.Errorf("expected content type %q, got %q", cloudEventsJSONContentType, gotContentType)
	}
	if len(gotBody) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestDispatch_HttpEndpoint_Returns404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	d := NewDispatcher(nil)
	trigger := triggerWithHTTPEndpoint(srv.URL)

	code, err := d.Dispatch(context.Background(), trigger, newTestEvent())
	if err != nil {
		t.Fatalf("non-2xx should not be an error, got: %v", err)
	}
	if code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", code)
	}
}

func TestDispatch_CloudRunDestination_UsesBaseURL(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("EVENTARC_CLOUDRUN_BASE_URL", srv.URL)

	d := NewDispatcher(nil)
	trigger := triggerWithCloudRun("/my-handler")

	code, err := d.Dispatch(context.Background(), trigger, newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("expected status 200, got %d", code)
	}
	if gotPath != "/my-handler" {
		t.Errorf("expected path /my-handler, got %s", gotPath)
	}
}

func TestDispatch_UnsupportedDestination_ReturnsError(t *testing.T) {
	d := NewDispatcher(nil)
	// Trigger with no destination at all.
	trigger := &eventarcpb.Trigger{}

	_, err := d.Dispatch(context.Background(), trigger, newTestEvent())
	if err == nil {
		t.Fatal("expected error for unsupported destination, got nil")
	}
}

func TestDispatch_ContentType_IsCloudEventsJSON(t *testing.T) {
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := NewDispatcher(nil)
	trigger := triggerWithHTTPEndpoint(srv.URL)

	_, err := d.Dispatch(context.Background(), trigger, newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != cloudEventsJSONContentType {
		t.Errorf("expected content type %q, got %q", cloudEventsJSONContentType, gotContentType)
	}
}
