// Package dispatcher sends CloudEvents to trigger destinations via HTTP.
//
// It supports two destination types from eventarcpb.Trigger:
//   - HttpEndpoint: posts directly to the configured URI
//   - CloudRun: constructs the URL from EVENTARC_CLOUDRUN_BASE_URL + path
//
// Non-2xx responses are not treated as errors; the caller receives the status
// code and decides how to handle it. Retries are out of scope.
package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/blackwell-systems/gcp-eventarc-emulator/internal/logger"
)

const (
	binaryModeDataContentType = "application/json"
	defaultCloudRunBaseURL    = "http://localhost"
	defaultTimeout            = 10 * time.Second
)

// Dispatcher sends CloudEvents to trigger destinations via HTTP.
type Dispatcher struct {
	client *http.Client
	logger *logger.Logger
}

// NewDispatcher creates a Dispatcher with the given HTTP client.
// If client is nil, http.DefaultClient is used with a 10-second timeout.
// An optional *logger.Logger may be supplied; if omitted or nil, defaults to info level.
func NewDispatcher(client *http.Client, log ...*logger.Logger) *Dispatcher {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	d := &Dispatcher{client: client}
	if len(log) > 0 && log[0] != nil {
		d.logger = log[0]
	} else {
		d.logger = logger.New("info")
	}
	return d
}

// Dispatch POSTs the CloudEvent in binary content mode to the trigger's destination.
// Binary content mode: event attributes are sent as Ce-* HTTP headers, and the
// event data is sent as the request body with Content-Type from DataContentType.
// Returns the HTTP response status code and any transport error.
// Non-2xx responses are not errors; the caller decides how to handle them.
func (d *Dispatcher) Dispatch(ctx context.Context, trigger *eventarcpb.Trigger, event cloudevents.Event) (int, error) {
	url, err := destinationURL(trigger)
	if err != nil {
		return 0, err
	}

	d.logger.Debug("dispatch: POST %s event_id=%s type=%s", url, event.ID(), event.Type())

	// Binary content mode: body is the raw event data bytes.
	body := event.DataEncoded
	if body == nil {
		body = []byte{}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("dispatcher: build request: %w", err)
	}

	// Set Content-Type from the event's DataContentType, defaulting to application/json.
	ct := event.DataContentType()
	if ct == "" {
		ct = binaryModeDataContentType
	}
	req.Header.Set("Content-Type", ct)

	// Required Ce-* headers.
	req.Header.Set("Ce-Specversion", event.SpecVersion())
	req.Header.Set("Ce-Type", event.Type())
	req.Header.Set("Ce-Source", event.Source())
	req.Header.Set("Ce-Id", event.ID())

	// Optional Ce-* headers (only set when non-empty/non-zero).
	if subject := event.Subject(); subject != "" {
		req.Header.Set("Ce-Subject", subject)
	}
	if t := event.Time(); !t.IsZero() {
		req.Header.Set("Ce-Time", t.UTC().Format(time.RFC3339Nano))
	}
	if schema := event.DataSchema(); schema != "" {
		req.Header.Set("Ce-Dataschema", schema)
	}

	// Extension attributes: each key becomes Ce-<key>.
	for k, v := range event.Extensions() {
		req.Header.Set("Ce-"+k, fmt.Sprintf("%v", v))
	}

	// If EVENTARC_EMULATOR_TOKEN is set, add an Authorization: Bearer header
	// to simulate the OIDC token that Eventarc adds for Cloud Run targets.
	if token := os.Getenv("EVENTARC_EMULATOR_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Error("dispatch: send failed url=%s err=%v", url, err)
		return 0, fmt.Errorf("dispatcher: send request: %w", err)
	}
	defer resp.Body.Close()

	d.logger.Info("dispatching event to %s status=%d", url, resp.StatusCode)
	d.logger.Debug("dispatch: response status=%d url=%s", resp.StatusCode, url)
	return resp.StatusCode, nil
}

// destinationURL extracts the destination URL from a trigger.
func destinationURL(trigger *eventarcpb.Trigger) (string, error) {
	dest := trigger.GetDestination()
	if dest == nil {
		return "", fmt.Errorf("dispatcher: trigger has no destination")
	}

	if ep := dest.GetHttpEndpoint(); ep != nil {
		return ep.GetUri(), nil
	}

	if cr := dest.GetCloudRun(); cr != nil {
		base := os.Getenv("EVENTARC_CLOUDRUN_BASE_URL")
		if base == "" {
			base = defaultCloudRunBaseURL
		}
		path := cr.GetPath()
		if path == "" {
			path = "/"
		}
		return base + path, nil
	}

	return "", fmt.Errorf("dispatcher: unsupported destination type")
}
