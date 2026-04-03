# GCP Eventarc Emulator

[![Blackwell Systems](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![Go Reference](https://pkg.go.dev/badge/github.com/blackwell-systems/gcp-eventarc-emulator.svg)](https://pkg.go.dev/github.com/blackwell-systems/gcp-eventarc-emulator)
[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

> **Production-grade GCP Eventarc emulator** — full API surface (47 RPCs), CloudEvent routing with CEL conditions, and multi-protocol support (gRPC + REST + CloudEvents). Run Eventarc locally with no GCP credentials.

Implements the full Eventarc v1 API surface (47 RPCs) plus the Publishing service, including CloudEvent routing, CEL-based trigger matching, and HTTP delivery in binary content mode. Optional IAM enforcement integrates with the local [GCP IAM control plane](https://github.com/blackwell-systems/gcp-iam-control-plane).

Enables local development, integration testing, and CI pipelines for event-driven systems without requiring access to GCP.

## Key Capabilities

- Full Eventarc API surface (47 RPCs including Publishing + Operations)
- CloudEvent routing with attribute filters and CEL condition evaluation
- HTTP delivery in CloudEvents binary content mode (`ce-*` headers)
- Triple protocol support: gRPC, REST (grpc-gateway), and CloudEvents
- Optional IAM enforcement via local GCP IAM emulator
- Drop-in compatibility with GCP SDKs (no code changes required)

## Quick Start

**Prerequisites:** Go 1.24+

**For Docker-based workflows** (demo, sdk-demo): Docker with Compose plugin v2
(`docker compose version`). Docker Desktop includes this by default. On Docker
Engine, install the Compose plugin: https://docs.docker.com/compose/install/

```bash
go install github.com/blackwell-systems/gcp-eventarc-emulator/cmd/server-dual@latest
server-dual
```

gRPC on `:9085`, REST on `:8085`.

**Verify it works:**
```bash
# Create a trigger
curl -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers?triggerId=my-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://localhost:3000/webhook"}
    }
  }'

# List triggers
curl "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"

# List providers
curl "http://localhost:8085/v1/projects/my-project/locations/us-central1/providers"
```

> **Tip:** Pipe curl output through `| jq .` for formatted JSON.

### Verify gRPC connectivity

```bash
grpcurl -plaintext localhost:9085 list
# -> google.cloud.eventarc.v1.Eventarc
# -> google.cloud.eventarc.publishing.v1.Publisher
# -> google.longrunning.Operations
```

Install grpcurl: https://github.com/fullstorydev/grpcurl

---

## Architecture

```
                   ┌─────────────────────────────┐
                   │     GCP SDK / gRPC Client    │
                   └──────────────┬──────────────┘
                                  │
              ┌───────────────────┼───────────────────┐
              │                   │                    │
   ┌──────────▼──────────┐  ┌────▼─────┐  ┌──────────▼──────────┐
   │  Eventarc Service   │  │   LRO    │  │  Publisher Service  │
   │  (40 RPCs: CRUD     │  │  Store   │  │  (PublishEvents,    │
   │   for 8 resources)  │  │          │  │   PublishChannel-   │
   │                     │  │          │  │   ConnectionEvents) │
   └──────────┬──────────┘  └──────────┘  └──────────┬──────────┘
              │                                       │
              │              ┌───────────┐            │
              │              │   Router  │◄───────────┘
              │              │ (match by │
              │              │  filters  │
              │              │  + CEL)   │
              │              └─────┬─────┘
              │                    │
              │           ┌────────▼────────┐
              │           │   Dispatcher    │
              │           │  (HTTP POST,    │
              │           │   binary mode,  │
              │           │   ce-* headers) │
              │           └────────┬────────┘
              │                    │
              │           ┌────────▼────────┐
              │           │  HTTP Endpoint  │
              │           │  (your service) │
              └───────────┘  └───────────────┘
```

1. **Configure** — Create triggers with event filters and HTTP destinations via the Eventarc gRPC/REST API
2. **Publish** — Send CloudEvents via the Publisher gRPC service
3. **Route** — The router matches events against trigger filters and optional CEL conditions
4. **Deliver** — The dispatcher HTTP POSTs to destinations using CloudEvents binary content mode (`ce-*` headers)

---

## API Coverage

Implements the full Eventarc v1 API surface across triggers, channels, message buses, pipelines, and providers, along with the Publishing and Operations services (47 RPCs total).

The API surface is compatible with the official GCP Eventarc clients and follows the same request/response structure. See [docs/api.md](docs/api.md) for the full list of supported operations.

---

## Server Variants

> **Not sure which to pick?** Use `server-dual`. It does everything the others do.

| Variant | Protocols | Best For |
|---------|-----------|----------|
| `server-dual` | gRPC + REST | Most users — works with SDKs and curl |
| `server` | gRPC only | Go/Python/Java SDK users who want minimal overhead |
| `server-rest` | REST/HTTP only | Shell scripts, curl, non-Go languages without gRPC |

### Install

**Docker (recommended):**
```bash
docker build -t gcp-eventarc-emulator .
docker run -p 9085:9085 -p 8085:8085 gcp-eventarc-emulator
```

> Note: The `docker compose` examples require Docker Compose plugin v2 (not the
> standalone `docker-compose` v1). Verify with `docker compose version`.

**Go install:**
```bash
# gRPC only
go install github.com/blackwell-systems/gcp-eventarc-emulator/cmd/server@latest

# REST API only
go install github.com/blackwell-systems/gcp-eventarc-emulator/cmd/server-rest@latest

# Both protocols (recommended)
go install github.com/blackwell-systems/gcp-eventarc-emulator/cmd/server-dual@latest
```

### Run Server

> **Note:** Go accepts both `-flag` and `--flag`. Examples use `--` for clarity.

**gRPC server:**
```bash
server --port 9085
```

**REST server:**
```bash
server-rest
# gRPC (internal) on :9086, HTTP on :8085
```

**Dual protocol server:**
```bash
server-dual
# gRPC on :9085, HTTP on :8085
```

---

## Use with GCP SDKs

Point your existing GCP SDK code at the emulator. No code changes needed beyond the connection setup.

**Go:**
```go
import (
    eventarc "cloud.google.com/go/eventarc/apiv1"
    "google.golang.org/api/option"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

conn, _ := grpc.NewClient("localhost:9085",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
client, _ := eventarc.NewClient(ctx, option.WithGRPCConn(conn))
defer client.Close()

// Use client normally — API is identical to real GCP
```

**Python:**
```python
from google.cloud import eventarc_v1
import grpc

channel = grpc.insecure_channel("localhost:9085")
client = eventarc_v1.EventarcClient(
    transport=eventarc_v1.transports.EventarcGrpcTransport(channel=channel)
)
```

## Use with REST API

```bash
# Create a trigger that routes Pub/Sub events to your local service
curl -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers?triggerId=pubsub-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://localhost:3000/events"}
    }
  }'

# Create a channel
curl -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/channels?channelId=my-channel" \
  -H "Content-Type: application/json" \
  -d '{}'

# List all triggers
curl "http://localhost:8085/v1/projects/test/locations/us-central1/triggers"
```

---

## Event Routing & Delivery

### How Events Are Matched

When a CloudEvent is published via the Publisher service:

1. **Attribute filters** — Each trigger's `eventFilters` are matched against the event's `type`, `source`, and extension attributes (exact match, all must pass)
2. **CEL conditions** — If the trigger has a `condition` field, it's evaluated as a [CEL expression](https://github.com/google/cel-spec) against the event attributes
3. **Delivery** — Matching triggers' destinations receive the event via HTTP POST

### Binary Content Mode

Events are delivered using **CloudEvents binary content mode** (matching real GCP Eventarc behavior):

```
POST /webhook HTTP/1.1
Content-Type: application/json
Ce-Specversion: 1.0
Ce-Type: google.cloud.pubsub.topic.v1.messagePublished
Ce-Source: //pubsub.googleapis.com/projects/my-project/topics/my-topic
Ce-Id: abc-123
Ce-Subject: my-subject

{"subscription":"...","message":{"data":"base64..."}}
```

Event attributes go in `Ce-*` HTTP headers; the payload goes in the body. This matches how real Eventarc delivers to Cloud Run and HTTP endpoints.

### Authorization Token for Cloud Run

Set the `EVENTARC_EMULATOR_TOKEN` environment variable to include an `Authorization: Bearer` header on all dispatched requests:

```bash
EVENTARC_EMULATOR_TOKEN=my-test-token server-dual
```

If unset, no authorization header is added (fine for local HTTP test servers).

---

## Usage Modes

**Standalone** — Run independently for Eventarc-only testing:
```bash
server-dual
```

**With IAM Enforcement** — Run with IAM checks:
```bash
IAM_MODE=strict IAM_EMULATOR_HOST=localhost:8080 server-dual
# Now requires valid permissions for all operations
```

**Orchestrated Ecosystem** — Use with [GCP IAM Control Plane](https://github.com/blackwell-systems/gcp-iam-control-plane):
```bash
gcp-emulator start
# Eventarc + Secret Manager + KMS + IAM emulator
# Single policy file, unified authorization
```

---

## IAM Integration

Optional permission checks using the [GCP IAM Emulator](https://github.com/blackwell-systems/gcp-iam-emulator). All 39 Eventarc operations have IAM permission mappings — see [docs/api.md](docs/api.md) for the full list.

### Enforcement Modes

| Scenario | `off` | `permissive` | `strict` |
|----------|-------|--------------|----------|
| No IAM emulator | Allow | Allow | Deny |
| IAM unavailable | Allow | Allow | Deny |
| No principal | Allow | Deny | Deny |
| Permission denied | Allow | Deny | Deny |

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `EVENTARC_EMULATOR_HOST` | `localhost:9085` | gRPC host:port |
| `EVENTARC_HTTP_PORT` | `8085` | HTTP/REST port (server-rest, server-dual) |
| `EVENTARC_EMULATOR_TOKEN` | *(unset)* | Bearer token for dispatched HTTP requests |
| `GCP_MOCK_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |
| `IAM_MODE` | `off` | IAM enforcement: off, permissive, strict |
| `IAM_EMULATOR_HOST` | `localhost:8080` | IAM emulator address |

> **Note:** `IAM_MODE` and `GCP_MOCK_LOG_LEVEL` use legacy prefixes for
> backward compatibility. A future release will standardize on `EVENTARC_` prefix.

---

## REST Gateway

The REST API is powered by [grpc-gateway v2](https://github.com/grpc-ecosystem/grpc-gateway), which transcodes HTTP/JSON requests to gRPC using the official Eventarc proto HTTP annotations. This means:

- REST paths match the real GCP Eventarc REST API exactly
- Request/response JSON matches GCP's format
- Both the Eventarc service and Publisher service are exposed

---

## Demo

Run the full loop — trigger creation, event publishing, and binary content mode delivery — using Docker:

```bash
# Start the emulator and a webhook receiver
docker compose up -d

# Run the demo script
./examples/demo.sh

# See the delivered CloudEvent (binary content mode, Ce-* headers)
docker compose logs webhook

# Tear down
docker compose down
```

The demo creates triggers, channels, message buses, pipelines, and enrollments, then publishes a CloudEvent and shows it arriving at the webhook receiver with `Ce-*` headers.

---

## SDK Demo

Run the full SDK workflow — list providers, create a trigger, publish a CloudEvent, and delete the trigger — using the official `cloud.google.com/go/eventarc` SDK:

```bash
# Terminal 1: start emulator
go run ./cmd/server-dual

# Terminal 2: start webhook receiver (separate module)
cd examples/webhook-receiver && go run main.go

# Terminal 3: run the SDK demo
cd examples/sdk-demo && EVENTARC_EMULATOR_HOST=localhost:9085 go run main.go
```

The sdk-demo automatically creates `my-channel` before publishing. If you prefer to pre-create it manually:

```bash
curl -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels?channelId=my-channel" \
  -H "Content-Type: application/json" -d '{}'
```

---

## Testing

```bash
# Run all tests
go test ./...

# With race detector
go test -race ./...

# Integration tests only
go test -v -run TestIntegration ./...
```

## Differences from GCP

- In-memory storage (no persistence)
- Immediate LRO resolution (no async operations)
- Optional IAM enforcement (off by default)
- No regional replication or constraints
- Static provider list (seeded at startup)

Designed for local development and testing — not production use.

---

## Disclaimer

This project is not affiliated with, endorsed by, or sponsored by Google LLC or Google Cloud Platform. "Google Cloud", "Eventarc", and related trademarks are property of Google LLC. This is an independent open-source implementation for testing and development purposes.

## Maintained By

Maintained by **Dayna Blackwell** — founder of Blackwell Systems, building reference infrastructure for cloud and AI systems.

[GitHub](https://github.com/blackwell-systems) · [LinkedIn](https://linkedin.com/in/dayna-blackwell) · [Blog](https://blog.blackwell-systems.com)

## Related Projects

- [**GCP IAM Control Plane**](https://github.com/blackwell-systems/gcp-iam-control-plane) — CLI to orchestrate the Local IAM Control Plane
- [GCP Secret Manager Emulator](https://github.com/blackwell-systems/gcp-secret-manager-emulator) — IAM-enforced Secret Manager emulator
- [GCP KMS Emulator](https://github.com/blackwell-systems/gcp-kms-emulator) — IAM-enforced KMS emulator
- [GCP IAM Emulator](https://github.com/blackwell-systems/gcp-iam-emulator) — Policy engine for IAM enforcement
- [gcp-emulator-auth](https://github.com/blackwell-systems/gcp-emulator-auth) — Enforcement proxy library

---

## License

Apache License 2.0 — See [LICENSE](LICENSE) for details.
