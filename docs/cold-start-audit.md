# Cold-Start UX Audit: gcp-eventarc-emulator

Auditor perspective: new user, first encounter with the tool.
Audit date: 2026-04-02
Server version: v0.1.0
Environment: macOS Darwin 24.6.0, Go 1.24, Docker 29.2.1

---

## Summary

| Severity | Count |
|---|---|
| UX-critical | 4 |
| UX-improvement | 9 |
| UX-polish | 7 |
| **Total** | **20** |

---

## Area 1: Discovery — Help Text and Server Variants

### [DISCOVERY] Help text omits all environment variables

- **Severity**: UX-critical
- **What happens**: Running `--help` on all three variants shows only the flag-level options. No env vars are mentioned. A new user who installs `server-dual` and tries to configure it via env vars (the documented approach in the README) has no hint of what env vars exist.
- **Expected**: Help text should list the env vars accepted by each binary, matching the pattern already present in the package-level Go doc comments. At minimum: `EVENTARC_EMULATOR_HOST`, `GCP_MOCK_LOG_LEVEL`, `EVENTARC_HTTP_PORT`, `EVENTARC_EMULATOR_TOKEN`, `IAM_MODE`.
- **Repro**:
  ```
  go run ./cmd/server-dual --help
  go run ./cmd/server-rest --help
  go run ./cmd/server --help
  ```
  Output for `server-dual`:
  ```
  Usage of ...:
    -grpc-port int   gRPC port to listen on (default 9085)
    -http-port int   HTTP port to listen on (default 8085)
    -log-level string  Log level (debug, info, warn, error) (default "info")
  ```
  No env vars listed.

---

### [DISCOVERY] No --version flag on any binary

- **Severity**: UX-improvement
- **What happens**: `go run ./cmd/server-dual --version` exits with status 2 and prints the usage/error: `flag provided but not defined: -version`. Same on `server` and `server-rest`. The version string `0.1.0` is hardcoded in each binary's source but not exposed.
- **Expected**: `--version` should print the version and exit 0, consistent with virtually every CLI tool.
- **Repro**:
  ```
  go run ./cmd/server-dual --version
  ```
  ```
  flag provided but not defined: -version
  exit status 2
  ```

---

### [DISCOVERY] Flag naming inconsistency across server variants

- **Severity**: UX-improvement
- **What happens**: The gRPC-only `server` binary uses `--port` to set its port. The `server-rest` and `server-dual` binaries use `--grpc-port` and `--http-port`. A user who switches from `server` to `server-dual` will find their flags no longer work.
- **Expected**: All variants should use `--grpc-port` (or at least the README should call out the difference explicitly). The README currently says `server --port 9085` for the gRPC variant, which is fine, but there is no hint that this is a different flag name.
- **Repro**:
  ```
  go run ./cmd/server --help        # shows: -port int
  go run ./cmd/server-dual --help   # shows: -grpc-port int
  ```

---

### [DISCOVERY] Help text does not describe what each binary does

- **Severity**: UX-polish
- **What happens**: All three `--help` outputs show only a bare flag list with no description of the binary (gRPC-only vs REST-only vs dual protocol). The binary name in the usage header is a cache path (`/Users/.../cache/.../server`), not a human-readable label.
- **Expected**: Each binary's `--help` should include a one-line description like "Runs both gRPC and REST/HTTP Eventarc APIs simultaneously" so the distinction is visible without consulting the README.
- **Repro**:
  ```
  go run ./cmd/server-dual --help
  ```
  ```
  Usage of /Users/dayna.blackwell/Library/Caches/go-build/.../server-dual:
    -grpc-port int   ...
  ```
  No description of what this variant does differently from the others.

---

## Area 2: First Run — Starting the Server

### [FIRST-RUN] "Ready" message can appear before gRPC server is actually listening

- **Severity**: UX-improvement
- **What happens**: In `server-dual`, the HTTP gateway goroutine logs "Ready to accept both gRPC and REST requests" before the gRPC goroutine has confirmed it is serving. In multiple test runs the "gRPC server listening at [::]:9085" line appeared after the "Ready" line:
  ```
  2026/04/02 18:06:50 HTTP gateway listening at :8088
  2026/04/02 18:06:50 Ready to accept both gRPC and REST requests
  2026/04/02 18:06:50 gRPC: localhost:9088
  2026/04/02 18:06:50 REST: http://localhost:8088/...
  2026/04/02 18:06:50 gRPC server listening at [::]:9088   ← came last
  ```
  Scripts that poll for the "Ready" line and then immediately connect via gRPC could fail.
- **Expected**: The "Ready" message should only be emitted after both goroutines have confirmed they are serving, or the gRPC and HTTP messages should both appear before a single final "Ready" line printed from the main goroutine.
- **Repro**: Start `server-dual` and observe the log ordering.

---

### [FIRST-RUN] Invalid --log-level silently accepted; no validation or warning

- **Severity**: UX-improvement
- **What happens**: `go run ./cmd/server-dual --log-level verbose` starts successfully, logs "Log level: verbose", and serves normally. The `--log-level` flag has no validation. The help text lists valid values as `(debug, info, warn, error)` but passing anything else is silently accepted without effect.
- **Expected**: The binary should reject invalid log levels at startup and print an error listing the valid values, then exit non-zero.
- **Repro**:
  ```
  go run ./cmd/server-dual --log-level verbose
  ```
  ```
  2026/04/02 18:07:19 GCP Eventarc Emulator v0.1.0 (gRPC + REST)
  2026/04/02 18:07:19 Log level: verbose
  2026/04/02 18:07:19 gRPC server listening at [::]:9089
  ...
  ```

---

### [FIRST-RUN] Startup log is inconsistent across server variants

- **Severity**: UX-polish
- **What happens**: Each binary has a different startup message format:
  - `server` (gRPC-only): "GCP Eventarc Emulator v0.1.0", then "Starting on port 9093 with log level: info", then "Publisher service registered", then "Server listening at [::]:9093", then "Ready to accept connections"
  - `server-rest`: "GCP Eventarc Emulator v0.1.0 (REST)", then three "Starting..." lines, then "gRPC backend listening at 127.0.0.1:9092 (internal)", then "HTTP gateway listening at :8092", then "Ready to accept REST requests"
  - `server-dual`: "GCP Eventarc Emulator v0.1.0 (gRPC + REST)", then "Log level: info", then the port bindings, then "Ready to accept both gRPC and REST requests" plus a gRPC URL and REST URL
  The `server` variant prints "Publisher service registered" (an internal detail) but `server-dual` and `server-rest` do not. Only `server-dual` prints the REST URL hint. The "Log level" line only appears in `server-dual` and `server-rest`, not in `server`.
- **Expected**: All variants should follow a consistent startup log pattern: version, protocol mode, log level, port bindings, ready message.
- **Repro**: Start each variant on different ports and compare startup output.

---

## Area 3: REST API — Core CRUD Workflow

### [REST-API] LRO operations/list and operations/get return 404 via REST

- **Severity**: UX-critical
- **What happens**: The README states the emulator supports a full `OperationsServer` (Get, List, Delete, Cancel, Wait). The REST endpoint `GET /v1/projects/{project}/locations/{location}/operations` returns `{"code":5, "message":"Not Found", "details":[]}`. Individual operation GET by name (e.g., `GET /v1/projects/my-project/locations/us-central1/operations/{id}`) also returns 404.
- **Expected**: LRO list and get should be accessible via REST since they are advertised as supported.
- **Repro**:
  ```
  # After creating a trigger (captures the operation name from the response):
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/operations"
  # returns: {"code":5, "message":"Not Found", "details":[]}

  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/operations/5c0a5e452fbe699301b1310342733473"
  # returns: {"code":5, "message":"Not Found", "details":[]}
  ```

---

### [REST-API] Channel state is always STATE_UNSPECIFIED

- **Severity**: UX-polish
- **What happens**: Created channels always show `"state": "STATE_UNSPECIFIED"`. A new user trying to understand whether their channel is active has no signal.
- **Expected**: Channels created in the emulator should have a meaningful state (e.g., `ACTIVE`) after creation, since they are immediately functional for event publishing.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels/my-channel"
  ```
  ```json
  { "state": "STATE_UNSPECIFIED", ... }
  ```

---

### [REST-API] Trigger response includes empty/null fields that GCP production omits

- **Severity**: UX-polish
- **What happens**: Trigger responses include `"operator": ""`, `"transport": null`, `"retryPolicy": null`, `"etag": ""`, `"conditions": {}`, `"eventDataContentType": ""`, `"serviceAccount": ""`, and `"channel": ""`. In real GCP Eventarc, these fields are only returned when set. A user comparing emulator output against GCP production output will see unexpected noise.
- **Expected**: Fields with zero/empty/null values that are not meaningful should be omitted from the JSON response (or the proto serialization should use `omitempty`).
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger"
  ```

---

### [REST-API] GoogleChannelConfig returns null updateTime

- **Severity**: UX-polish
- **What happens**: `GET /v1/projects/my-project/locations/us-central1/googleChannelConfig` returns `"updateTime": null`. The `updateTime` field is `null` rather than being omitted or set to the creation time.
- **Expected**: Either omit the `updateTime` field when not set, or initialize it to a valid timestamp on first access (as is done for other resources).
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/googleChannelConfig"
  ```
  ```json
  { "name": "...", "updateTime": null, ... }
  ```

---

### [REST-API] Providers have empty eventTypes array

- **Severity**: UX-improvement
- **What happens**: All seeded providers (pubsub, storage, run, scheduler, functions) return `"eventTypes": []`. The real GCP API returns the supported event types for each provider. A user who lists providers to discover what event types to filter on gets no useful information.
- **Expected**: Providers should be seeded with their known event types (e.g., `google.cloud.pubsub.topic.v1.messagePublished` for the pubsub provider). This is the primary discovery mechanism for Eventarc event types.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/test/locations/us-central1/providers/pubsub.googleapis.com"
  ```
  ```json
  { "name": "...", "displayName": "Cloud Pub/Sub", "eventTypes": [] }
  ```

---

## Area 4: Event Publishing and Delivery

### [PUBLISHING] Publish to nonexistent channel returns 200 with empty body

- **Severity**: UX-critical
- **What happens**: Publishing events to a channel name that does not exist returns HTTP 200 with an empty JSON body `{}`. The event is silently discarded. There is no error or warning.
- **Expected**: Publishing to a nonexistent channel should return an error (HTTP 404 / gRPC NOT_FOUND), so the caller knows the channel must be created first.
- **Repro**:
  ```
  curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/channels/no-such-channel:publishEvents" \
    -H "Content-Type: application/json" \
    -d '{"events": [{"@type": "...", "id": "x", "source": "//test", "specVersion": "1.0", "type": "test.event", "textData": "{}"}]}'
  ```
  Returns: `{}` with HTTP 200

---

### [PUBLISHING] publishEvents response gives no routing feedback

- **Severity**: UX-improvement
- **What happens**: The `publishEvents` endpoint always returns `{}` regardless of how many triggers matched or whether any delivery was attempted. Both a matching-event publish and a non-matching-event publish return identical responses. Routing errors (e.g., destination unreachable) are only visible in server logs, not in the API response.
- **Expected**: The response could include a `matchedTriggers` count or a routing status. At minimum, routing errors should be surfaced (dispatch errors currently only appear in server logs: `publisher: dispatch error (trigger=...): dispatcher: trigger has no destination`).
- **Repro**:
  ```
  # Matched event → {}
  # Unmatched event → {}
  # Event dispatched to trigger with no destination → {} (but server logs show error)
  curl -s -X POST ".../channels/my-channel:publishEvents" -d '{...matching event...}'
  curl -s -X POST ".../channels/my-channel:publishEvents" -d '{...non-matching event...}'
  ```

---

## Area 5: SDK Demo

### [SDK-DEMO] SDK demo step 7 prints "event routed to trigger destination" unconditionally

- **Severity**: UX-improvement
- **What happens**: The SDK demo hardcodes `fmt.Println("   → event routed to trigger destination")` after calling `PublishEvents`. This message is printed regardless of whether routing actually succeeded. If the webhook receiver is unreachable, the message still says "event routed to trigger destination".
- **Expected**: The step should either omit the routing confirmation (let the webhook log speak for itself) or check server logs for actual delivery confirmation. The current message is misleading in error scenarios.
- **Repro**:
  ```
  cd examples/sdk-demo && EVENTARC_EMULATOR_HOST=localhost:9085 go run main.go
  ```
  Step 7 always prints "→ event routed to trigger destination" without validation.

---

### [SDK-DEMO] SDK demo points new users to Docker for the primary workflow

- **Severity**: UX-improvement
- **What happens**: The SDK demo's package comment says:
  ```
  // Usage:
  //   docker compose up -d        # start emulator + webhook receiver
  //   go run main.go
  ```
  But the demo also works with `EVENTARC_EMULATOR_HOST=localhost:9085` and a locally running server. A new user who doesn't have Docker set up (or has Docker Engine without Compose plugin) hits a dead end trying to follow the primary usage instruction.
- **Expected**: The usage comment should list the non-Docker path as the primary option, with Docker as an alternative.
- **Repro**: Read the file header of `examples/sdk-demo/main.go`.

---

## Area 6: Docker — Container-based Workflow

### [DOCKER] `docker compose` not available as a subcommand on Docker Engine

- **Severity**: UX-critical
- **What happens**: The README, `demo.sh`, and `sdk-demo/main.go` all reference `docker compose up -d`. On Docker Engine 29.2.1 (without Docker Desktop), `docker compose` is not a valid subcommand and fails immediately:
  ```
  unknown shorthand flag: 'd' in -d
  ```
  The standalone `docker-compose` (v1) is also not installed. The prerequisite section of the README lists only "Go 1.24+" and does not mention a Docker Compose requirement or version.
- **Expected**: The README should explicitly list Docker Compose as a prerequisite (with version note: "Docker Desktop, or Docker Engine with Compose plugin v2"). Alternatively, the demo section should note that Docker is optional and provide a non-Docker alternative for the quick start.
- **Repro**:
  ```
  docker compose up -d
  ```
  ```
  unknown shorthand flag: 'd' in -d
  ```

---

## Area 7: Env Var Configuration

### [ENV-VARS] IAM_MODE=strict error message leaks internal gRPC dial details

- **Severity**: UX-improvement
- **What happens**: When `IAM_MODE=strict` is set and no IAM emulator is running, every API request returns:
  ```json
  {"code":13, "message":"IAM check failed: rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing: dial tcp 127.0.0.1:8080: connect: connection refused\"", "details":[]}
  ```
  The gRPC error code is 13 (INTERNAL). The message exposes the internal gRPC transport error text, which tells the user nothing useful about how to fix the situation.
- **Expected**: The error should be user-friendly: `IAM_MODE is 'strict' but no IAM emulator is reachable at localhost:8080. Start the IAM emulator or set IAM_MODE=off.` The gRPC error code should be UNAVAILABLE (14) or FAILED_PRECONDITION (9), not INTERNAL (13).
- **Repro**:
  ```
  IAM_MODE=strict go run ./cmd/server-dual --grpc-port 9088 --http-port 8088 &
  sleep 2
  curl -s http://localhost:8088/v1/projects/test/locations/us-central1/triggers
  ```

---

### [ENV-VARS] Server does not log IAM mode at startup

- **Severity**: UX-improvement
- **What happens**: When `IAM_MODE=strict` is set, the startup log shows no indication that IAM enforcement is active. A user who accidentally sets this env var (or inherits it from their environment) has no way to see it in the startup output:
  ```
  2026/04/02 18:06:50 GCP Eventarc Emulator v0.1.0 (gRPC + REST)
  2026/04/02 18:06:50 Log level: info
  2026/04/02 18:06:50 HTTP gateway listening at :8088
  2026/04/02 18:06:50 Ready to accept both gRPC and REST requests
  ...
  ```
  No mention of IAM mode.
- **Expected**: Startup should log the active IAM mode: `IAM mode: strict (IAM emulator: localhost:8080)`.
- **Repro**: `IAM_MODE=strict go run ./cmd/server-dual ...` — observe startup log.

---

### [ENV-VARS] EVENTARC_EMULATOR_TOKEN silently set but webhook receiver does not display Authorization header

- **Severity**: UX-polish
- **What happens**: When `EVENTARC_EMULATOR_TOKEN=my-test-token` is set, the dispatcher correctly adds `Authorization: Bearer my-test-token` to dispatched requests. However, the bundled webhook receiver only displays `Ce-*` headers — it does not display the `Authorization` header. The README says to use this feature to "simulate the OIDC token that Eventarc adds for Cloud Run targets," but observing it requires a custom receiver.
- **Expected**: The webhook receiver should also print the `Authorization` header (if present) so users can confirm token dispatch is working.
- **Repro**: Start server with `EVENTARC_EMULATOR_TOKEN=my-test-token`, publish a matching event, observe webhook receiver logs — no Authorization header is shown.

---

## Area 8: Destructive Operations

### [DESTRUCTIVE] Delete returns the full deleted resource body — could be misread as "still exists"

- **Severity**: UX-polish
- **What happens**: DELETE operations return an LRO whose `response` field contains the full pre-deletion state of the resource. For example:
  ```json
  {
    "name": "projects/my-project/.../operations/fd45...",
    "done": true,
    "response": {
      "@type": "type.googleapis.com/google.cloud.eventarc.v1.Trigger",
      "name": "projects/my-project/.../triggers/pubsub-trigger",
      ...all fields...
    }
  }
  ```
  A new user scanning this output and seeing the trigger's name inside `response` might initially think the trigger still exists rather than reading this as "the trigger that was deleted."
- **Expected**: This is GCP-compatible behavior, but a log line or a clearer top-level `status` or `message` field ("operation: delete completed") would reduce confusion. Alternatively, the presence of `"done": true` with no `"error"` field is the correct signal — this could be documented more clearly.
- **Repro**:
  ```
  curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger"
  ```

---

### [DESTRUCTIVE] Second delete of same resource returns NOT_FOUND, not 204 — no idempotency

- **Severity**: UX-polish
- **What happens**: Deleting a resource a second time returns an error:
  ```json
  {"code":5, "message":"Trigger [projects/my-project/.../triggers/pubsub-trigger] not found", "details":[]}
  ```
  GCP's real Eventarc API also returns NOT_FOUND on duplicate delete, so this is spec-compatible. However, many users expect delete to be idempotent (HTTP 200/204 on already-deleted resources).
- **Expected**: This is spec-compatible behavior. The finding is that the error format uses square brackets in the resource path (`Trigger [...]`) rather than a gRPC-standard format. This is a minor stylistic inconsistency.
- **Repro**:
  ```
  curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger"
  curl -s -X DELETE "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/pubsub-trigger"
  # Second returns: {"code":5, "message":"Trigger [...] not found", "details":[]}
  ```

---

## Area 9: Error Handling and Edge Cases

### [ERRORS] Creating a trigger with empty body succeeds with no eventFilters or destination

- **Severity**: UX-critical
- **What happens**: `POST .../triggers?triggerId=empty-trigger` with body `{}` succeeds and creates a trigger with `"eventFilters": []` and `"destination": null`. This trigger will match nothing. If any event is later published to a channel in the same namespace, the server logs a dispatch error:
  ```
  publisher: dispatch error (trigger=.../triggers/empty-trigger): dispatcher: trigger has no destination
  ```
  But the REST response for the creation was 200 OK with the trigger object. The client has no indication this trigger is broken.
- **Expected**: Creating a trigger without at least one event filter and a destination should return a validation error (HTTP 400 / gRPC INVALID_ARGUMENT). A trigger with no destination cannot deliver events and will generate server-side errors on every publish.
- **Repro**:
  ```
  curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers?triggerId=empty-trigger" \
    -H "Content-Type: application/json" \
    -d '{}'
  ```
  Returns HTTP 200 with the trigger object (destination: null, eventFilters: []).

---

### [ERRORS] Malformed JSON returns a raw Go parser error message

- **Severity**: UX-improvement
- **What happens**: Sending `not-json` as a request body returns:
  ```json
  {"code":3, "message":"invalid character 'o' in literal null (expecting 'u')", "details":[]}
  ```
  The message is the raw Go JSON parser error string ("invalid character 'o' in literal null (expecting 'u')"), which is technical and confusing for users who may not parse the hint that `not` is being read as the start of `null`.
- **Expected**: A cleaner error like `"request body is not valid JSON"` would be more helpful.
- **Repro**:
  ```
  curl -s -X POST "http://localhost:8085/v1/projects/test/locations/us-central1/triggers?triggerId=bad" \
    -H "Content-Type: application/json" \
    -d 'not-json'
  ```

---

## Area 10: Output Review — Formatting and Consistency

### [OUTPUT] Empty list responses return named array keys (correct), but nextPageToken and unreachable are always present

- **Severity**: UX-polish
- **What happens**: All list responses include `"nextPageToken": ""` and `"unreachable": []` even when empty. While these are valid fields in the GCP response format, always including empty nextPageToken can confuse clients that use it as a signal for pagination (empty string = no more pages, which is correct, but the field should arguably be omitted when empty per proto3 omitempty rules).
- **Expected**: This is spec-compatible. Worth noting that empty list responses use the correct named-key format (`{"triggers": []}`, not `{}`), which is good.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels"
  ```
  ```json
  {"channels": [], "nextPageToken": "", "unreachable": []}
  ```

---

### [OUTPUT] Error message format uses numeric gRPC codes without status names

- **Severity**: UX-polish
- **What happens**: Error responses use `"code": 5` (NOT_FOUND) rather than including a human-readable status name. A user who sees `"code": 5` must know gRPC status codes to understand the meaning.
- **Expected**: Include a `"status"` field with the string name (e.g., `"status": "NOT_FOUND"`) alongside the numeric `"code"` field, matching the gRPC-gateway default error format and GCP's REST API error convention.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/test/locations/us-central1/triggers/does-not-exist"
  ```
  ```json
  {"code":5, "message":"Trigger [...] not found", "details":[]}
  ```
  Missing `"status": "NOT_FOUND"`.
