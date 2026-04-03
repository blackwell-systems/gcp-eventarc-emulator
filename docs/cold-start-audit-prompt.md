# Cold-Start UX Audit Prompt

**Metadata:**
- Audit Date: 2026-04-02
- Tool: gcp-eventarc-emulator (binaries: `server`, `server-rest`, `server-dual`)
- Tool Version: v0.1.0
- Sandbox mode: local
- Sandbox: host machine running directly from /Users/dayna.blackwell/code/gcp-eventarc-emulator with no Docker; all state is in-memory only
- Exec prefix: `cd /Users/dayna.blackwell/code/gcp-eventarc-emulator &&`

---

You are performing a UX audit of gcp-eventarc-emulator — a tool that provides a production-grade local emulator for GCP Eventarc with a full API surface (47 RPCs), CloudEvent routing with CEL conditions, and multi-protocol support (gRPC + REST + CloudEvents), enabling local development and integration testing without GCP credentials.

You are acting as a **new user** encountering this tool for the first time.

Sandbox: host machine running directly from /Users/dayna.blackwell/code/gcp-eventarc-emulator; all emulator state is in-memory and disappears when the server process exits.

Run all commands from the project root: `cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && <command>`

**Important:** Many audit areas require a running server. Start the server in the background at the beginning of each such area and kill it when done. Use `go run ./cmd/server-dual` as the server for all REST and gRPC tests.

## Audit Areas

### 1. Discovery

Explore the project as a new user would — README, help text, version flags.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && head -60 README.md

cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual --help
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server --help
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-rest --help

cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual --version
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server --version
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-rest --version
```

Note:
- Does `--help` output make the three server variants and their differences clear?
- Is the env var table present and readable?
- Does `--version` print cleanly and exit 0?
- Is the deprecated `--port` flag on `server` called out clearly in help text?

---

### 2. First Run — Startup and Port Bindings

Start the dual-protocol server and observe startup output.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

curl -s http://localhost:8085/v1/projects/test/locations/us-central1/triggers
curl -s http://localhost:8085/v1/projects/test/locations/us-central1/providers

cat /tmp/emulator.log

kill %1 2>/dev/null
```

Note:
- What does the startup log look like? JSON structured or plain text?
- Does it clearly state which ports are bound (gRPC :9085, HTTP :8085)?
- Is there a "ready" signal a user can reliably wait for?
- Does the first `curl` for triggers return an empty list or an error?
- Does the providers list return seeded data on first run?

---

### 3. REST API — Core Resource Lifecycle

Exercise create/list/get/delete for triggers, channels, providers, and other resource types via the REST API at port 8085. This area covers the primary user path shown in the README quick-start.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

BASE="http://localhost:8085/v1/projects/my-project/locations/us-central1"

# List providers (seeded at startup)
curl -s "$BASE/providers"

# Get a specific provider
curl -s "$BASE/providers/pubsub.googleapis.com"

# Create a trigger
curl -s -X POST "$BASE/triggers?triggerId=audit-trigger" \
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
curl -s "$BASE/triggers"

# Get trigger by name
curl -s "$BASE/triggers/audit-trigger"

# Create a channel
curl -s -X POST "$BASE/channels?channelId=audit-channel" \
  -H "Content-Type: application/json" \
  -d '{}'

# List channels
curl -s "$BASE/channels"

# Get channel by name
curl -s "$BASE/channels/audit-channel"

# Get Google Channel Config (singleton — no Create endpoint)
curl -s "$BASE/googleChannelConfig"

# Create a message bus
curl -s -X POST "$BASE/messageBuses?messageBusId=audit-bus" \
  -H "Content-Type: application/json" \
  -d '{}'

# List message buses
curl -s "$BASE/messageBuses"

# Create a pipeline
curl -s -X POST "$BASE/pipelines?pipelineId=audit-pipeline" \
  -H "Content-Type: application/json" \
  -d '{
    "destinations": [
      {"httpEndpoint": {"uri": "http://localhost:3000/pipeline"}}
    ]
  }'

# List pipelines
curl -s "$BASE/pipelines"

# Create an enrollment
curl -s -X POST "$BASE/enrollments?enrollmentId=audit-enrollment" \
  -H "Content-Type: application/json" \
  -d '{
    "celMatch": "message.type == '\''google.cloud.pubsub.topic.v1.messagePublished'\''",
    "messageBus": "projects/my-project/locations/us-central1/messageBuses/audit-bus",
    "destination": "projects/my-project/locations/us-central1/pipelines/audit-pipeline"
  }'

# List enrollments
curl -s "$BASE/enrollments"

# Delete the trigger
curl -s -w "\nHTTP %{http_code}\n" -X DELETE "$BASE/triggers/audit-trigger"

# Verify deletion — expect NOT_FOUND
curl -s -w "\nHTTP %{http_code}\n" "$BASE/triggers/audit-trigger"

kill %1 2>/dev/null
```

Note:
- Do create responses return the full resource or just an LRO envelope?
- Is the JSON structure consistent across resource types (e.g., same envelope for channels, triggers, buses)?
- Does the delete response body provide any confirmation message or just an empty body?
- Are resource names in responses the fully-qualified GCP form (e.g., `projects/my-project/locations/us-central1/triggers/audit-trigger`)?
- Does get-after-delete return 404 / NOT_FOUND?

---

### 4. LRO REST Gateway

All Create/Update/Delete operations return Long-Running Operations. Verify the LRO REST endpoint at `/v1/operations/{name}` is accessible and returns the expected structure.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

BASE="http://localhost:8085/v1/projects/my-project/locations/us-central1"

# Create a trigger and capture the operation name
RESP=$(curl -s -X POST "$BASE/triggers?triggerId=lro-test-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://localhost:3000/webhook"}
    }
  }')
echo "$RESP"

# Extract the operation name
OP_NAME=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('name',''))" 2>/dev/null)
echo "Operation name: $OP_NAME"

# Poll the operation via its REST URL
if [ -n "$OP_NAME" ]; then
  curl -s "http://localhost:8085/v1/$OP_NAME"
fi

# List operations
curl -s "$BASE/operations"

# Get a nonexistent operation
curl -s -w "\nHTTP %{http_code}\n" "http://localhost:8085/v1/operations/does-not-exist"

kill %1 2>/dev/null
```

Note:
- Does the create response include a `name` field that looks like an operation path?
- Does `GET /v1/<operation-name>` return a valid LRO with `done: true` and `response` containing the trigger?
- Is `response` the embedded trigger proto (as GCP SDKs expect via `anypb.Any`)?
- Is the operations list endpoint accessible at `$BASE/operations`?
- Does a nonexistent operation return 404?

---

### 5. Event Publishing and Routing

Test the full CloudEvent delivery loop: publish → route → dispatch → HTTP delivery in binary content mode. The webhook receiver at `examples/webhook-receiver` must be running.

```
# Start webhook receiver (port 3000)
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./examples/webhook-receiver > /tmp/webhook.log 2>&1 &
WEBHOOK_PID=$!
sleep 1

# Start emulator
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
EMULATOR_PID=$!
sleep 2

BASE="http://localhost:8085/v1/projects/my-project/locations/us-central1"

# Create a channel to publish into
curl -s -X POST "$BASE/channels?channelId=publish-channel" \
  -H "Content-Type: application/json" \
  -d '{}'

# Create a trigger that routes Pub/Sub events to the webhook receiver
curl -s -X POST "$BASE/triggers?triggerId=publish-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://localhost:3000/events"}
    }
  }'

# Publish a matching CloudEvent
curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/channels/publish-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "audit-evt-001",
        "source": "//pubsub.googleapis.com/projects/my-project/topics/my-topic",
        "specVersion": "1.0",
        "type": "google.cloud.pubsub.topic.v1.messagePublished",
        "attributes": {
          "subject": {"ceString": "my-subject"},
          "time": {"ceTimestamp": "2026-04-02T12:00:00Z"}
        },
        "textData": "{\"subscription\":\"projects/my-project/subscriptions/my-sub\",\"message\":{\"data\":\"SGVsbG8gZnJvbSBFdmVudGFyYyE=\"}}"
      }
    ]
  }'

sleep 1

# Check webhook received the event with Ce-* headers
cat /tmp/webhook.log

# Now publish a NON-matching event type — should NOT reach the webhook
curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/channels/publish-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "audit-evt-002",
        "source": "//test.source",
        "specVersion": "1.0",
        "type": "test.unmatched.event.v1",
        "textData": "this should not be delivered"
      }
    ]
  }'

sleep 1
echo "Webhook log after non-matching event (should show no new entry):"
cat /tmp/webhook.log

kill $EMULATOR_PID $WEBHOOK_PID
```

Note:
- Does the publishEvents call return HTTP 200?
- Does the webhook log show all expected Ce-* headers: Ce-Type, Ce-Source, Ce-Id, Ce-Specversion, Ce-Subject?
- Is the event payload in the body as valid JSON?
- Does the non-matching event type produce no new webhook entry (proving routing works correctly)?

---

### 6. Channel Validation — Publishing to Nonexistent Channel

Verify that publishing to a channel that does not exist returns a clear NOT_FOUND error.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

curl -s -w "\nHTTP %{http_code}\n" -X POST \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/does-not-exist:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "bad-evt-001",
        "source": "//test.source",
        "specVersion": "1.0",
        "type": "test.event.v1",
        "textData": "hello"
      }
    ]
  }'

kill %1 2>/dev/null
```

Note:
- Is the HTTP status 404?
- Does the response body include a gRPC status code (5 = NOT_FOUND) and a human-readable message?
- Does the error identify which channel was not found by name?
- Would a new user understand what went wrong from this response alone?

---

### 7. Trigger Validation — Creating Without Required Fields

Verify that creating a trigger without a destination, creating a duplicate, or supplying an empty parent returns a clear INVALID_ARGUMENT or ALREADY_EXISTS error.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

BASE="http://localhost:8085/v1/projects/my-project/locations/us-central1"

# Create trigger with no destination field
curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/triggers?triggerId=bad-trigger-1" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ]
  }'

# Create trigger with completely empty body
curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/triggers?triggerId=bad-trigger-2" \
  -H "Content-Type: application/json" \
  -d '{}'

# Create trigger with missing triggerId query param
curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/triggers" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [{"attribute": "type", "value": "test.event.v1"}],
    "destination": {"httpEndpoint": {"uri": "http://localhost:3000/webhook"}}
  }'

# Create a valid trigger then try to create it again (duplicate)
curl -s -X POST "$BASE/triggers?triggerId=dup-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [{"attribute": "type", "value": "test.event.v1"}],
    "destination": {"httpEndpoint": {"uri": "http://localhost:3000/webhook"}}
  }'
curl -s -w "\nHTTP %{http_code}\n" -X POST "$BASE/triggers?triggerId=dup-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [{"attribute": "type", "value": "test.event.v1"}],
    "destination": {"httpEndpoint": {"uri": "http://localhost:3000/webhook"}}
  }'

kill %1 2>/dev/null
```

Note:
- Does missing destination return 400 / INVALID_ARGUMENT? Does the error name the missing field?
- Does missing triggerId return an error? Which one?
- Does the duplicate trigger return 409 / ALREADY_EXISTS?
- Are error responses structured as `{"code": N, "message": "...", "status": "..."}` (gRPC-gateway convention)?

---

### 8. gRPC API — Core Operations via grpcurl

Verify the gRPC surface is accessible, reflection is enabled, and core operations work.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

# Check if grpcurl is available
which grpcurl || echo "grpcurl not found — install: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"

# List services via gRPC server reflection
grpcurl -plaintext localhost:9085 list

# List methods on the Eventarc service
grpcurl -plaintext localhost:9085 list google.cloud.eventarc.v1.Eventarc

# List providers via gRPC
grpcurl -plaintext \
  -d '{"parent":"projects/my-project/locations/us-central1"}' \
  localhost:9085 google.cloud.eventarc.v1.Eventarc/ListProviders

# Create a trigger via gRPC
grpcurl -plaintext \
  -d '{
    "parent": "projects/my-project/locations/us-central1",
    "trigger_id": "grpc-audit-trigger",
    "trigger": {
      "event_filters": [{"attribute": "type", "value": "test.grpc.event.v1"}],
      "destination": {"http_endpoint": {"uri": "http://localhost:3000/webhook"}}
    }
  }' \
  localhost:9085 google.cloud.eventarc.v1.Eventarc/CreateTrigger

# Get the trigger back via gRPC
grpcurl -plaintext \
  -d '{"name":"projects/my-project/locations/us-central1/triggers/grpc-audit-trigger"}' \
  localhost:9085 google.cloud.eventarc.v1.Eventarc/GetTrigger

# List triggers via gRPC
grpcurl -plaintext \
  -d '{"parent":"projects/my-project/locations/us-central1"}' \
  localhost:9085 google.cloud.eventarc.v1.Eventarc/ListTriggers

# Delete the trigger via gRPC
grpcurl -plaintext \
  -d '{"name":"projects/my-project/locations/us-central1/triggers/grpc-audit-trigger"}' \
  localhost:9085 google.cloud.eventarc.v1.Eventarc/DeleteTrigger

# List Publisher service methods
grpcurl -plaintext localhost:9085 list google.cloud.eventarc.publishing.v1.Publisher

# List Operations service methods
grpcurl -plaintext localhost:9085 list google.longrunning.Operations

kill %1 2>/dev/null
```

Note:
- Is gRPC reflection enabled? (`grpcurl list` must work without a .proto file.)
- Do gRPC field names use snake_case as expected (e.g., `event_filters`, `http_endpoint`)?
- Is the LRO in CreateTrigger's response marked `done: true` with the trigger embedded in `response`?
- Are the Publisher and Operations services listed alongside Eventarc?

---

### 9. SDK Demo

Run the Go SDK demo end-to-end using the real `cloud.google.com/go/eventarc` client library. The webhook receiver must be running to catch the dispatched event. Note: the sdk-demo publishes to a channel named `my-channel` which it does not create itself — create it via REST first.

```
# Start webhook receiver
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./examples/webhook-receiver > /tmp/webhook-sdk.log 2>&1 &
WEBHOOK_PID=$!
sleep 1

# Start emulator
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator-sdk.log 2>&1 &
EMULATOR_PID=$!
sleep 2

# Create the channel the sdk-demo will publish into (my-channel)
curl -s -X POST \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels?channelId=my-channel" \
  -H "Content-Type: application/json" \
  -d '{}'

# Run the SDK demo
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator/examples/sdk-demo && \
  EVENTARC_EMULATOR_HOST=localhost:9085 \
  WEBHOOK_URI=http://localhost:3000/events \
  go run main.go

# Check the webhook receiver log for a delivered CloudEvent with Ce-* headers
cat /tmp/webhook-sdk.log

kill $EMULATOR_PID $WEBHOOK_PID
```

Note:
- Does the SDK demo run to completion with exit 0?
- Are all 8 steps (List providers, Get provider, Create trigger, Get trigger, List triggers, Update trigger, Publish, Delete trigger) printed?
- Does the webhook log show the dispatched event with Ce-* headers?
- Is there any step in the sdk-demo that fails silently without surfacing the error to the user (via `must()`)?
- Does the README sdk-demo usage section accurately reflect what's needed (e.g., the need to pre-create `my-channel`)?

---

### 10. Env Vars — EVENTARC_EMULATOR_TOKEN and IAM_MODE

**Token injection (EVENTARC_EMULATOR_TOKEN):**

```
# Start webhook receiver
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./examples/webhook-receiver > /tmp/webhook-token.log 2>&1 &
WEBHOOK_PID=$!
sleep 1

# Start emulator with a bearer token
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  EVENTARC_EMULATOR_TOKEN=my-secret-test-token \
  go run ./cmd/server-dual > /tmp/emulator-token.log 2>&1 &
EMULATOR_PID=$!
sleep 2

BASE="http://localhost:8085/v1/projects/my-project/locations/us-central1"

curl -s -X POST "$BASE/channels?channelId=token-channel" -H "Content-Type: application/json" -d '{}'

curl -s -X POST "$BASE/triggers?triggerId=token-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [{"attribute": "type", "value": "test.token.event.v1"}],
    "destination": {"httpEndpoint": {"uri": "http://localhost:3000/events"}}
  }'

curl -s -X POST "$BASE/channels/token-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [{
      "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
      "id": "token-evt-001",
      "source": "//test.source",
      "specVersion": "1.0",
      "type": "test.token.event.v1",
      "textData": "hello token"
    }]
  }'

sleep 1

# Webhook log should show: Authorization: Bearer my-secret-test-token
cat /tmp/webhook-token.log

kill $EMULATOR_PID $WEBHOOK_PID
```

**IAM permissive mode (no IAM emulator running):**

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  IAM_MODE=permissive \
  go run ./cmd/server-dual > /tmp/emulator-iam-permissive.log 2>&1 &
EMULATOR_PID=$!
sleep 2

# Request without a principal — README says "Deny" in permissive mode without a principal
curl -s -w "\nHTTP %{http_code}\n" \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"

cat /tmp/emulator-iam-permissive.log

kill $EMULATOR_PID
```

**IAM strict mode (no IAM emulator running):**

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  IAM_MODE=strict \
  go run ./cmd/server-dual > /tmp/emulator-iam-strict.log 2>&1 &
EMULATOR_PID=$!
sleep 2

# README says: strict + IAM unavailable = Deny
curl -s -w "\nHTTP %{http_code}\n" \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"

cat /tmp/emulator-iam-strict.log

kill $EMULATOR_PID
```

Note:
- Does the webhook log show `Authorization: Bearer my-secret-test-token` when the env var is set?
- Does the startup log indicate which IAM_MODE is active?
- In permissive mode with no IAM emulator, does behavior match the README table (Allow when IAM unavailable)?
- In strict mode with no IAM emulator, are requests denied? Is the error response clear?
- Does `IAM_MODE` vs `EVENTARC_EMULATOR_IAM_MODE` naming match — README uses `IAM_MODE` but the user prompt says `EVENTARC_EMULATOR_IAM_MODE`. Which is correct?

---

### 11. Log Level Flag

Verify the `--log-level` flag and `GCP_MOCK_LOG_LEVEL` env var work correctly, and that invalid values fail cleanly.

```
# Debug level — should be verbose
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  go run ./cmd/server-dual --log-level debug > /tmp/emulator-debug.log 2>&1 &
EMULATOR_PID=$!
sleep 2

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"

kill $EMULATOR_PID
cat /tmp/emulator-debug.log

# Error level — should be very quiet
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  GCP_MOCK_LOG_LEVEL=error \
  go run ./cmd/server-dual > /tmp/emulator-error.log 2>&1 &
EMULATOR_PID=$!
sleep 2

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"

kill $EMULATOR_PID
cat /tmp/emulator-error.log

# Invalid log level — should fail at startup
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  go run ./cmd/server-dual --log-level banana > /tmp/emulator-invalid-loglevel.log 2>&1
echo "Exit: $?"
cat /tmp/emulator-invalid-loglevel.log

# Conflict: both flag and env var set — which wins?
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  GCP_MOCK_LOG_LEVEL=error \
  go run ./cmd/server-dual --log-level debug > /tmp/emulator-conflict.log 2>&1 &
EMULATOR_PID=$!
sleep 2
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"
kill $EMULATOR_PID
cat /tmp/emulator-conflict.log
```

Note:
- Does `--log-level debug` produce significantly more output than the default `info`?
- Is the log format structured (JSON) or plain text?
- Does an invalid `--log-level` value produce an error and non-zero exit, or does it silently fall back?
- When both `--log-level` flag and `GCP_MOCK_LOG_LEVEL` env var are set, which wins? Is this documented?

---

### 12. Edge Cases and Error Handling

Test boundary behavior for both the server binary and the REST API.

**Server binary edge cases:**

```
# Unknown flag — should print usage and exit non-zero
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  go run ./cmd/server-dual --unknown-flag 2>&1; echo "Exit: $?"

# Invalid port value — should fail at startup with a clear message
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  go run ./cmd/server-dual --grpc-port notaport 2>&1; echo "Exit: $?"

# Out-of-range port value
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  go run ./cmd/server-dual --grpc-port 99999 2>&1; echo "Exit: $?"

# Deprecated --port flag on server binary (should work but warn)
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
  go run ./cmd/server --port 9085 > /tmp/emulator-deprecated.log 2>&1 &
EMULATOR_PID=$!
sleep 2
curl -s http://localhost:9085 2>&1 || true
kill $EMULATOR_PID
cat /tmp/emulator-deprecated.log
```

**REST API edge cases:**

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator.log 2>&1 &
sleep 2

# GET nonexistent trigger
curl -s -w "\nHTTP %{http_code}\n" \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/does-not-exist"

# GET nonexistent channel
curl -s -w "\nHTTP %{http_code}\n" \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/does-not-exist"

# GET nonexistent operation
curl -s -w "\nHTTP %{http_code}\n" \
  "http://localhost:8085/v1/operations/does-not-exist"

# DELETE nonexistent trigger
curl -s -w "\nHTTP %{http_code}\n" -X DELETE \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/does-not-exist"

# Malformed JSON body
curl -s -w "\nHTTP %{http_code}\n" -X POST \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers?triggerId=malformed" \
  -H "Content-Type: application/json" \
  -d 'this is not json'

# POST with no Content-Type header
curl -s -w "\nHTTP %{http_code}\n" -X POST \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers?triggerId=no-ct" \
  -d '{"eventFilters":[],"destination":{"httpEndpoint":{"uri":"http://localhost:3000"}}}'

# Totally unknown path
curl -s -w "\nHTTP %{http_code}\n" "http://localhost:8085/totally/unknown/path"

# No arguments to the server binary (should start with defaults, not print help)
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual &
sleep 2
kill %1 2>/dev/null

kill %1 2>/dev/null
```

Note:
- Are all NOT_FOUND responses the same JSON structure?
- Is the malformed JSON error actionable (does it point to the parse problem location)?
- Does an unknown path return 404 with a gRPC-gateway error body, or something else?
- Does deleting a nonexistent resource return 404 (NOT_FOUND) or silently succeed (200)?
- Does the deprecated `--port` flag on `server` log a deprecation warning at startup?

---

### 13. Test Suite

Run the test suite to see what baseline coverage exists and whether tests pass cleanly.

```
# Unit tests only (fast)
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go test ./internal/... -v 2>&1 | tail -50

# Integration tests
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go test -v -run TestIntegration ./... 2>&1

# Full suite with race detector
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go test -race ./... 2>&1 | tail -20
```

Note:
- Do all tests pass?
- Are test names descriptive enough that a new user can understand what they cover?
- Is there a `make test` or similar convenience target in the Makefile?
- Is it clear from the README how to run just unit vs. integration tests?

---

### 14. Output Review

Evaluate the visual quality and consistency of all output across the tool.

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual --help
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server --help
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-rest --help
```

With server running:

```
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && go run ./cmd/server-dual > /tmp/emulator-output.log 2>&1 &
sleep 2

# Review formatted JSON output for each resource type
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/providers" | python3 -m json.tool
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers" | python3 -m json.tool

# Create and inspect a trigger response
curl -s -X POST \
  "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers?triggerId=output-review" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [{"attribute": "type", "value": "test.event.v1"}],
    "destination": {"httpEndpoint": {"uri": "http://localhost:3000/webhook"}}
  }' | python3 -m json.tool

# Inspect startup log format
cat /tmp/emulator-output.log

kill %1 2>/dev/null
```

Evaluate:
- Is the server startup log readable? Does it include timestamps, log level labels, and port numbers?
- Are REST JSON responses pretty-printed or minified by default?
- Is field naming in JSON consistently camelCase (matching GCP API conventions)?
- Do error responses use the same `{"code": N, "message": "...", "status": "..."}` envelope everywhere?
- Is there any color in the server output? Is color appropriate for a server log?
- Is the `--help` layout easy to scan? Are env vars clearly separated from flags?
- Is terminology consistent between the README, `--help` text, and API response field names (e.g., "IAM_MODE" vs "EVENTARC_EMULATOR_IAM_MODE")?
- Do the three server variants' `--help` texts call out their differences clearly enough that a new user can choose the right one?

---

Run ALL commands listed. Do not skip areas.
Note exact output, errors, exit codes, and behavior at each step.
Describe color usage (e.g. "server startup logs appear in plain text with no color; REST responses are minified JSON").

You are not trying to find every possible issue — you are discovering what friction a new user would naturally encounter by following the help text and trying obvious commands.

## Findings Format

For each issue found, use:

### [AREA] Finding Title
- **Severity**: UX-critical / UX-improvement / UX-polish
- **What happens**: What the user actually sees
- **Expected**: What better behavior looks like
- **Repro**: Exact command(s)

Severity guide:
- **UX-critical**: Broken, misleading, or completely missing behavior that blocks the user
- **UX-improvement**: Confusing or unhelpful behavior that a user would notice and dislike
- **UX-polish**: Minor friction, inconsistency, or missed opportunity for clarity

## Report

- Group findings by area
- Include a summary table at the top: total count by severity
- Write the complete report to docs/cold-start-audit.md using the Write tool

IMPORTANT: Run ALL commands from the project root: `cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && <command>`.
Do not bypass the sandbox — do not run the emulator against any external GCP project or production state.
All state is in-memory; restart the server fresh for each area that requires it.
