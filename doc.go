// Package gcp_eventarc_emulator provides a production-grade local emulator
// for the Google Cloud Eventarc API.
//
// # Overview
//
// The emulator implements all 47 RPCs of the Eventarc v1 API surface —
// Triggers, Channels, ChannelConnections, MessageBuses, Enrollments, Pipelines,
// GoogleApiSources, Providers, and GoogleChannelConfig — plus the Publishing
// service (PublishEvents, PublishChannelConnectionEvents) and the full
// Operations service for long-running operation polling.
//
// No GCP credentials or network access are required. All state is held
// in-memory with thread-safe storage backed by sync.RWMutex and proto.Clone.
//
// # Architecture
//
// The emulator is structured as three independently runnable server binaries
// sharing a common internal library:
//
//   - cmd/server       — gRPC only (port 9085)
//   - cmd/server-rest  — REST/HTTP only via grpc-gateway (port 8085)
//   - cmd/server-dual  — gRPC + REST simultaneously (recommended)
//
// Internal packages:
//
//   - internal/server     — Eventarc and Operations gRPC service implementation
//   - internal/publisher  — Publishing service (CloudEvent ingestion)
//   - internal/router     — CloudEvent matching against trigger event_filters and CEL conditions
//   - internal/dispatcher — HTTP delivery of matched CloudEvents to trigger destinations
//   - internal/gateway    — grpc-gateway v2 REST transcoding layer
//   - internal/lro        — Long-running operation store (operations resolve immediately)
//   - internal/logger     — Leveled logger (debug/info/warn/error)
//   - internal/authz      — IAM permission mapping for optional enforcement
//
// # Quick Start
//
//	go install github.com/blackwell-systems/gcp-eventarc-emulator/cmd/server-dual@latest
//	server-dual
//
// gRPC on :9085, REST on :8085.
//
// # Docker
//
//	docker pull ghcr.io/blackwell-systems/gcp-eventarc-emulator:latest
//	docker run -p 9085:9085 -p 8085:8085 ghcr.io/blackwell-systems/gcp-eventarc-emulator:latest
//
// Variant images: ghcr.io/blackwell-systems/gcp-eventarc-emulator-grpc and -rest.
//
// # Use with GCP Go SDK
//
//	os.Setenv("EVENTARC_EMULATOR_HOST", "localhost:9085")
//
//	conn, _ := grpc.NewClient("localhost:9085", grpc.WithTransportCredentials(insecure.NewCredentials()))
//	client := eventarc.NewEventarcClient(conn)
//
// # CloudEvent Routing
//
// Published events are matched against all triggers by comparing event
// attributes to trigger event_filters. Filters support exact-match and
// match-path-pattern operators. CEL conditions in trigger.condition are
// evaluated via cel-go for advanced filtering.
//
// Matched events are delivered to trigger destinations in CloudEvents binary
// content mode (ce-* headers, raw data body), matching real GCP behavior.
//
// # IAM Enforcement
//
// Optional IAM enforcement integrates with the local GCP IAM emulator via
// the IAM_MODE environment variable (off/permissive/strict). All 39 Eventarc
// IAM permissions are mapped.
//
// # Health Endpoints
//
// The REST and dual servers expose GET /healthz and GET /readyz, both
// returning {"status":"ok"} with HTTP 200.
//
// See https://github.com/blackwell-systems/gcp-eventarc-emulator for full
// documentation, examples, and the curl/SDK demo.
package gcp_eventarc_emulator
