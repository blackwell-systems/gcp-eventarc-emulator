# Cold-Start UX Audit Prompt

**Metadata:**
- Audit Date: 2026-04-02
- Tool: gcp-eventarc-emulator
- Tool Version: see `go run ./cmd/server-dual --help` (no --version flag; version implied by module)
- Sandbox mode: local
- Sandbox: Go project at /Users/dayna.blackwell/code/gcp-eventarc-emulator. The server is started with `go run ./cmd/server-dual` (or pre-built binary). gRPC port 9085, HTTP REST port 8085. Env var EVENTARC_EMULATOR_HOST=localhost:9085.
- Exec prefix: (none — run commands directly from /Users/dayna.blackwell/code/gcp-eventarc-emulator; start the server in background before REST/SDK tests)

---

You are performing a UX audit of gcp-eventarc-emulator — a tool that provides a production-grade local emulator for GCP Eventarc with full API surface (47 RPCs), CloudEvent routing with CEL conditions, and multi-protocol support (gRPC + REST + CloudEvents), enabling local development without GCP credentials.

You are acting as a **new user** encountering this tool for the first time.

Sandbox: Go project at /Users/dayna.blackwell/code/gcp-eventarc-emulator. The server is started with `go run ./cmd/server-dual`. gRPC port 9085, HTTP REST port 8085. Env var EVENTARC_EMULATOR_HOST=localhost:9085. All state is in-memory only.

Run all commands from the project root: `/Users/dayna.blackwell/code/gcp-eventarc-emulator`

## Audit Areas

### 1. Discovery — Help text and server variants

Evaluate clarity, completeness, and consistency of help output across all three server variants.

```bash
go run ./cmd/server --help
go run ./cmd/server-rest --help
go run ./cmd/server-dual --help
```

Also check for a version flag:
```bash
go run ./cmd/server-dual --version
go run ./cmd/server --version
```

Check the README for first-run orientation:
```bash
head -60 README.md
```

Note: Do all three variants explain their differences? Is the flag naming consistent? Does help text explain env vars?

---

### 2. First Run — Starting the server

Start the dual-protocol server in the background (it serves both gRPC on :9085 and HTTP on :8085):

```bash
go run ./cmd/server-dual &
sleep 3
```

Verify it is listening on both ports:
```bash
curl -s http://localhost:8085/v1/projects/test/locations/us-central1/triggers
curl -s http://localhost:8085/v1/projects/test/locations/us-central1/providers
```

Also try starting with a non-default port via flag:
```bash
go run ./cmd/server-dual --grpc-port 19085 --http-port 18085 &
sleep 2
curl -s http://localhost:18085/v1/projects/test/locations/us-central1/triggers
kill %2
```

Try starting with the log-level flag:
```bash
go run ./cmd/server-dual --log-level debug &
sleep 2
kill %2
```

Check env var override for log level:
```bash
GCP_MOCK_LOG_LEVEL=debug go run ./cmd/server-dual &
sleep 2
kill %2
```

Note: What does startup output look like? Does it clearly indicate which ports are active? Is there a readiness signal?

---

### 3. REST API — Core CRUD workflow

With the server running on default ports, exercise the full REST API surface as a new user following the README quick-start.

**Providers (read-only, seeded at startup):**
```bash
curl -s http://localhost:8085/v1/projects/my-project/locations/us-central1/providers | python3 -m json.tool
curl -s http://localhost:8085/v1/projects/my-project/locations/us-central1/providers/pubsub.googleapis.com | python3 -m json.tool
```

**Channels:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels?channelId=my-channel" \
  -H "Content-Type: application/json" \
  -d '{}' | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels" | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/my-channel" | python3 -m json.tool
```

**Triggers — create, list, get:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers?triggerId=pubsub-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://localhost:3000/events"}
    }
  }' | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers" | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger" | python3 -m json.tool
```

**Trigger — update:**
```bash
curl -s -X PATCH "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "labels": {"env": "local"}
  }' | python3 -m json.tool
```

**Message buses:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/messageBuses?messageBusId=my-bus" \
  -H "Content-Type: application/json" \
  -d '{}' | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/messageBuses" | python3 -m json.tool
```

**Pipelines:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/pipelines?pipelineId=my-pipeline" \
  -H "Content-Type: application/json" \
  -d '{
    "destinations": [
      {"httpEndpoint": {"uri": "http://localhost:3000/pipeline"}}
    ]
  }' | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/pipelines" | python3 -m json.tool
```

**Enrollments:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/enrollments?enrollmentId=my-enrollment" \
  -H "Content-Type: application/json" \
  -d '{
    "celMatch": "message.type == '\''google.cloud.pubsub.topic.v1.messagePublished'\''",
    "messageBus": "projects/my-project/locations/us-central1/messageBuses/my-bus",
    "destination": "projects/my-project/locations/us-central1/pipelines/my-pipeline"
  }' | python3 -m json.tool

curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/enrollments" | python3 -m json.tool
```

**Google Channel Config (singleton):**
```bash
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/googleChannelConfig" | python3 -m json.tool
```

**Long-Running Operations:**
```bash
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/operations" | python3 -m json.tool
```

---

### 4. Event Publishing and Delivery — end-to-end routing

Start a webhook receiver to observe event delivery. In a new background process:

```bash
go run ./examples/webhook-receiver/main.go &
sleep 1
```

Publish a CloudEvent that matches the pubsub-trigger created in area 3:

```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/my-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "evt-001",
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
  }' | python3 -m json.tool
```

Check that the webhook receiver (running in the background) logged the delivered event with Ce-* headers in its stdout.

Publish a second event with a non-matching type (should NOT be delivered to pubsub-trigger):
```bash
curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/my-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "evt-002",
        "source": "//storage.googleapis.com/projects/my-project/buckets/my-bucket",
        "specVersion": "1.0",
        "type": "google.cloud.storage.object.v1.finalized",
        "textData": "{}"
      }
    ]
  }' | python3 -m json.tool
```

Note: Does the API give feedback about whether routing occurred? Does the response body differ between matched and unmatched events?

---

### 5. SDK Demo — Go SDK integration

The sdk-demo uses the official GCP Go SDK against the emulator (no GCP credentials required). It needs a channel named `my-channel` to exist and a webhook receiver at `http://localhost:3000/events`.

Ensure server and webhook receiver are still running from previous areas, then run:

```bash
cd /Users/dayna.blackwell/code/gcp-eventarc-emulator/examples/sdk-demo && \
  EVENTARC_EMULATOR_HOST=localhost:9085 \
  WEBHOOK_URI=http://localhost:3000/events \
  go run main.go
```

Note: Does the output clearly label each step? Does it show LRO wait behavior? Are error messages from the SDK helpful when things go wrong?

---

### 6. Docker — Container-based workflow

Stop any background processes from prior areas first, then test the Docker workflow from scratch:

```bash
docker compose up -d
sleep 5
```

Verify both containers started:
```bash
docker compose ps
docker compose logs emulator
docker compose logs webhook
```

Run the demo script (which exercises the full REST API against the Docker environment):
```bash
./examples/demo.sh
```

After the demo runs, inspect the webhook logs to confirm event delivery:
```bash
docker compose logs webhook
```

Tear down:
```bash
docker compose down
```

Note: Does `demo.sh` have clear output indicating each step? Are errors actionable? Does `docker compose logs webhook` show clean Ce-* header formatting?

---

### 7. Env Var Configuration — Runtime behavior via environment

Test the bearer token dispatch header:

```bash
EVENTARC_EMULATOR_TOKEN=my-test-token go run ./cmd/server-dual &
sleep 2
```

Create a trigger pointing to the local webhook receiver:
```bash
curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers?triggerId=token-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [{"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}],
    "destination": {"httpEndpoint": {"uri": "http://localhost:3000/events"}}
  }' | python3 -m json.tool
```

Publish an event and check the webhook receiver logs for the Authorization header:
```bash
curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/channels/my-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "token-test-evt",
        "source": "//pubsub.googleapis.com/projects/test/topics/t",
        "specVersion": "1.0",
        "type": "google.cloud.pubsub.topic.v1.messagePublished",
        "textData": "{}"
      }
    ]
  }' | python3 -m json.tool
```

Kill the server and check IAM mode env var (no IAM emulator present — should fail with strict):
```bash
kill %1
IAM_MODE=strict go run ./cmd/server-dual &
sleep 2
curl -s http://localhost:8085/v1/projects/test/locations/us-central1/triggers
kill %1
```

Note: Does the server log or error message explain why the request was denied in strict mode without an IAM emulator? Is the IAM mode documented at startup?

---

### 8. Destructive Operations — Delete and cleanup

With the server running and resources from area 3 still present (or recreated):

**Delete a trigger:**
```bash
curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger" | python3 -m json.tool
```

**Verify deletion (should return 404 or empty):**
```bash
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger"
```

**Delete a channel:**
```bash
curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/my-channel" | python3 -m json.tool
```

**Delete a pipeline:**
```bash
curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/pipelines/my-pipeline" | python3 -m json.tool
```

**Delete an enrollment:**
```bash
curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/enrollments/my-enrollment" | python3 -m json.tool
```

**Delete a message bus:**
```bash
curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/messageBuses/my-bus" | python3 -m json.tool
```

Note: Delete operations return LROs — does the response format make it obvious the operation succeeded? What does a second delete of the same resource return?

**Delete an already-deleted resource (idempotency check):**
```bash
curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger"
```

---

### 9. Error Handling and Edge Cases

**No arguments (bare binary):**
```bash
go run ./cmd/server-dual
```
(should start normally, not error — but note what the startup UX looks like without flags)

**Unknown flag:**
```bash
go run ./cmd/server-dual --blorp
```

**Invalid log level:**
```bash
go run ./cmd/server-dual --log-level verbose
```

**Malformed JSON body:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers?triggerId=bad" \
  -H "Content-Type: application/json" \
  -d 'not-json'
```

**Missing required fields (trigger with no eventFilters or destination):**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers?triggerId=empty-trigger" \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Missing triggerId query param:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers" \
  -H "Content-Type: application/json" \
  -d '{"eventFilters": [{"attribute": "type", "value": "foo"}], "destination": {"httpEndpoint": {"uri": "http://localhost:3000"}}}'
```

**Nonexistent resource (GET):**
```bash
curl -s "http://localhost:8085/v1/projects/test/locations/us-central1/triggers/does-not-exist"
```

**Nonexistent provider:**
```bash
curl -s "http://localhost:8085/v1/projects/test/locations/us-central1/providers/nonexistent.provider.com"
```

**Publish to a nonexistent channel:**
```bash
curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/channels/no-such-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "x", "source": "//test", "specVersion": "1.0", "type": "test.event", "textData": "{}"
      }
    ]
  }'
```

**Wrong HTTP method:**
```bash
curl -s -X GET "http://localhost:8085/v1/projects/test/locations/us-central1/triggers" -X DELETE
curl -s -X PUT "http://localhost:8085/v1/projects/test/locations/us-central1/triggers"
```

---

### 10. Output Review — Formatting and consistency

Re-run key commands and evaluate output quality:

```bash
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/providers" | python3 -m json.tool
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers" | python3 -m json.tool
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels" | python3 -m json.tool
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/messageBuses" | python3 -m json.tool
curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/pipelines" | python3 -m json.tool
```

Assess:
- JSON field naming: camelCase vs snake_case consistency
- LRO response format: are `name`, `done`, `response`, `metadata` fields present and meaningful?
- Empty list response: `{}` vs `{"triggers": []}` — which does the API return?
- Error response format: is there a `code`, `message`, `status` structure (gRPC status style)?
- Server startup logs: are port bindings clearly stated? Is there a "ready" message?
- `demo.sh` colored output: step labels in cyan, success ticks in green, info in yellow — is the color scheme readable and consistent?

---

Run ALL commands listed. Do not skip areas.
Note exact output, errors, exit codes, and behavior at each step.
Describe what the server logs show during each operation.

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

IMPORTANT: Run ALL commands from the project root `/Users/dayna.blackwell/code/gcp-eventarc-emulator`.
Do not bypass the sandbox — do not run the emulator against production GCP state.
All state is in-memory and resets when the server process is killed.
