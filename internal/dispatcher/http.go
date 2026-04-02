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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	cloudEventsJSONContentType = "application/cloudevents+json"
	defaultCloudRunBaseURL     = "http://localhost"
	defaultTimeout             = 10 * time.Second
)

// Dispatcher sends CloudEvents to trigger destinations via HTTP.
type Dispatcher struct {
	client *http.Client
}

// NewDispatcher creates a Dispatcher with the given HTTP client.
// If client is nil, http.DefaultClient is used with a 10-second timeout.
func NewDispatcher(client *http.Client) *Dispatcher {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &Dispatcher{client: client}
}

// Dispatch POSTs the CloudEvent (JSON-marshaled) to the trigger's destination.
// Returns the HTTP response status code and any transport error.
// Non-2xx responses are not errors; the caller decides how to handle them.
func (d *Dispatcher) Dispatch(ctx context.Context, trigger *eventarcpb.Trigger, event cloudevents.Event) (int, error) {
	url, err := destinationURL(trigger)
	if err != nil {
		return 0, err
	}

	body, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("dispatcher: marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("dispatcher: build request: %w", err)
	}
	req.Header.Set("Content-Type", cloudEventsJSONContentType)

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("dispatcher: send request: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("dispatching event to %s, status: %d", url, resp.StatusCode)
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
