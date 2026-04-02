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

// newTestEventWithExtras builds a CloudEvent with subject and an extension attribute.
func newTestEventWithExtras() cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType("google.cloud.test.v1.eventFired")
	e.SetSource("//test.googleapis.com/projects/p/topics/t")
	e.SetID("ext-test-id")
	e.SetSubject("test-subject")
	e.SetExtension("testkey", "testval")
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
		gotMethod        string
		gotBody          []byte
		gotContentType   string
		gotCeSpecversion string
		gotCeType        string
		gotCeSource      string
		gotCeId          string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		gotCeSpecversion = r.Header.Get("Ce-Specversion")
		gotCeType = r.Header.Get("Ce-Type")
		gotCeSource = r.Header.Get("Ce-Source")
		gotCeId = r.Header.Get("Ce-Id")
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
	// Binary mode: Content-Type is the event data content type, not the CloudEvents envelope type.
	if gotContentType != "application/json" {
		t.Errorf("expected content type %q, got %q", "application/json", gotContentType)
	}
	_ = gotBody // body may be empty for an event with no data — that's fine
	if gotCeSpecversion == "" {
		t.Error("expected Ce-Specversion header to be present and non-empty")
	}
	if gotCeType == "" {
		t.Error("expected Ce-Type header to be present and non-empty")
	}
	if gotCeSource == "" {
		t.Error("expected Ce-Source header to be present and non-empty")
	}
	if gotCeId == "" {
		t.Error("expected Ce-Id header to be present and non-empty")
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

func TestDispatch_ContentType_IsBinaryMode(t *testing.T) {
	var gotContentType string
	var gotCeSpecversion string
	var gotCeType string
	var gotCeSource string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotCeSpecversion = r.Header.Get("Ce-Specversion")
		gotCeType = r.Header.Get("Ce-Type")
		gotCeSource = r.Header.Get("Ce-Source")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := NewDispatcher(nil)
	trigger := triggerWithHTTPEndpoint(srv.URL)

	_, err := d.Dispatch(context.Background(), trigger, newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Binary mode: Content-Type is the data content type, not cloudevents+json.
	if gotContentType != "application/json" {
		t.Errorf("expected content type %q, got %q", "application/json", gotContentType)
	}
	if gotCeSpecversion != "1.0" {
		t.Errorf("expected Ce-Specversion %q, got %q", "1.0", gotCeSpecversion)
	}
	if gotCeType != "google.cloud.pubsub.topic.v1.messagePublished" {
		t.Errorf("expected Ce-Type %q, got %q", "google.cloud.pubsub.topic.v1.messagePublished", gotCeType)
	}
	if gotCeSource == "" {
		t.Error("expected Ce-Source header to be present")
	}
}

func TestDispatch_BinaryMode_Headers(t *testing.T) {
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(nil)
	trigger := triggerWithHTTPEndpoint(srv.URL)
	event := newTestEventWithExtras()

	_, err := d.Dispatch(context.Background(), trigger, event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		header string
		want   string
	}{
		{"Ce-Specversion", "1.0"},
		{"Ce-Type", "google.cloud.test.v1.eventFired"},
		{"Ce-Source", "//test.googleapis.com/projects/p/topics/t"},
		{"Ce-Id", "ext-test-id"},
		{"Ce-Subject", "test-subject"},
		{"Ce-Testkey", "testval"},
	}

	for _, c := range checks {
		got := capturedHeaders.Get(c.header)
		if got == "" {
			t.Errorf("expected header %q to be present, got empty", c.header)
		} else if got != c.want {
			t.Errorf("header %q: expected %q, got %q", c.header, c.want, got)
		}
	}
}
