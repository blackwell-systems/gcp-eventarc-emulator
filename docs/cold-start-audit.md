# Cold-Start UX Audit — gcp-eventarc-emulator v0.1.0

**Date:** 2026-04-02
**Auditor:** Cold-start audit agent (zero prior knowledge)
**Sandbox:** Local host, in-memory server, no GCP credentials

---

## Summary

| Severity | Count |
|---|---|
| UX-critical | 5 |
| UX-improvement | 12 |
| UX-polish | 9 |
| **Total** | **26** |

---

## Area 1: Discovery

### [DISCOVERY] README RPC count is inconsistent

- **Severity**: UX-polish
- **What happens**: The opening tagline says "47 RPCs" but the paragraph immediately below says "40 RPCs": _"Implements the full Eventarc v1 API surface (40 RPCs) plus the Publishing service"_. The Key Capabilities section also says "47 RPCs including Publishing + Operations".
- **Expected**: A single consistent number across all occurrences, or a clear breakdown (e.g., "40 Eventarc + 3 Publishing + 5 Operations = 48, rounded to 47").
- **Repro**: `head -15 README.md`

---

### [DISCOVERY] `--help` does not distinguish variant use-cases clearly enough

- **Severity**: UX-improvement
- **What happens**: Each binary's `--help` header states the variant name (`gRPC + REST server`, `gRPC-only server`, `REST-only server`) but gives no guidance on when to choose each. A new user encounters three binaries with no explanation of the trade-offs.
- **Expected**: A one-sentence "When to use this:" blurb under each header, e.g. "Use `server-dual` (recommended) for full REST+gRPC access. Use `server` for gRPC-only workloads. Use `server-rest` only when an external gRPC backend is unavailable."
- **Repro**: `go run ./cmd/server-dual --help`, `go run ./cmd/server --help`, `go run ./cmd/server-rest --help`

---

### [DISCOVERY] `EVENTARC_EMULATOR_HOST` description differs across variants and is missing from `server-rest`

- **Severity**: UX-polish
- **What happens**: `server-dual` describes `EVENTARC_EMULATOR_HOST` as "gRPC host:port". `server` says "gRPC server host:port". `server-rest` omits `EVENTARC_EMULATOR_HOST` entirely from its env var table even though users may set it expecting it to work. The semantics differ per binary but these differences are not explained.
- **Expected**: Consistent description, or an explicit note that `EVENTARC_EMULATOR_HOST` is not applicable to `server-rest`.
- **Repro**: Compare `go run ./cmd/server-dual --help` vs `go run ./cmd/server --help` vs `go run ./cmd/server-rest --help`

---

### [DISCOVERY] `--version` output is identical across all three variants

- **Severity**: UX-polish
- **What happens**: All three variants print `GCP Eventarc Emulator v0.1.0` with no indication of which binary is running.
- **Expected**: `GCP Eventarc Emulator v0.1.0 (server-dual)` — matching the subtitle already present in `--help`.
- **Repro**: `go run ./cmd/server-dual --version && go run ./cmd/server --version && go run ./cmd/server-rest --version`

---

### [DISCOVERY] Deprecated `--port` flag is not visually distinct in `server --help`

- **Severity**: UX-improvement
- **What happens**: The `server` binary lists `--port` alphabetically between `--log-level` and `--version` with `(deprecated: use --grpc-port)` in the description field. There is no dedicated deprecated section and no runtime warning when the flag is used.
- **Expected**: A distinct `Deprecated Flags:` section below the main flags, or a `[DEPRECATED]` tag prefix. Using `--port` at runtime should emit a warning to stderr.
- **Repro**: `go run ./cmd/server --help`

---

## Area 2: First Run

### [FIRST-RUN] Startup log is plain `log` format with no structure and no log-level labels

- **Severity**: UX-improvement
- **What happens**: The startup log uses Go's standard `log` package: `2026/04/02 19:02:14 <message>`. There is no JSON structure, no log level label (e.g., `INFO`), and no key=value fields. Log aggregators and `grep` pipelines cannot easily parse it.
- **Expected**: Structured log output (JSON or logfmt) with level, timestamp, and fields. Even plain-text output should prefix each line with its level (`INFO`, `WARN`, etc.).
- **Repro**: `go run ./cmd/server-dual > /tmp/emulator.log 2>&1 & sleep 2 && cat /tmp/emulator.log`

---

### [FIRST-RUN] No machine-readable "ready" signal

- **Severity**: UX-improvement
- **What happens**: The server prints `Ready to accept both gRPC and REST requests` as a human-readable plain-text line. CI scripts that health-check the server must either `sleep` or grep for this exact string, which is brittle to wording changes.
- **Expected**: A stable, structured "ready" line — e.g., `{"event":"ready","grpc_port":9085,"http_port":8085}` — or a `/healthz` HTTP endpoint that returns 200 when the server is ready.
- **Repro**: `go run ./cmd/server-dual > /tmp/emulator.log 2>&1 & sleep 2 && cat /tmp/emulator.log`

---

### [FIRST-RUN] REST endpoint hint in startup log uses literal `{project}` and `{location}` placeholders

- **Severity**: UX-polish
- **What happens**: The startup log prints: `REST: http://localhost:8085/v1/projects/{project}/locations/{location}/triggers`. The curly-brace placeholders are not copy-pasteable.
- **Expected**: Either show a concrete example (`http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers`) or just the base URL (`REST: http://localhost:8085/v1/`).
- **Repro**: `go run ./cmd/server-dual > /tmp/emulator.log 2>&1 & sleep 2 && cat /tmp/emulator.log`

---

## Area 3: REST API — Core Resource Lifecycle

### [REST-API] Create responses wrap the resource in an LRO envelope with no explanation in Quick Start

- **Severity**: UX-improvement
- **What happens**: Every Create call returns an LRO envelope (`{"name":"operations/...","done":true,"response":{...}}`). The actual resource is nested inside `response`. The README Quick Start shows a Create call but does not show or explain the LRO envelope, so new users see unexpected output on their first command.
- **Expected**: The Quick Start should show the actual response shape and explain that all mutating operations return an LRO. A brief note such as "The resource is in the `.response` field" would immediately orient a new user.
- **Repro**: `curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers?triggerId=t1" -H "Content-Type: application/json" -d '{...}'`

---

### [REST-API] Zero-value fields included in all responses

- **Severity**: UX-polish
- **What happens**: REST responses include many empty or zero-value fields: `"operator":""`, `"serviceAccount":""`, `"networkConfig":null`, `"transport":null`, `"channel":""`, `"conditions":{}`, `"eventDataContentType":""`, `"satisfiesPzs":false`, `"retryPolicy":null`, `"etag":""`. A new user doing a simple create/get sees 20+ fields when they set 2.
- **Expected**: Configure grpc-gateway with `EmitUnpopulated: false` or `omitempty` to omit zero-value fields, matching GCP's real Eventarc REST API behavior.
- **Repro**: `curl -s "$BASE/triggers/audit-trigger" | python3 -m json.tool`

---

### [REST-API] Delete returns the deleted resource body inside an LRO instead of Empty

- **Severity**: UX-improvement
- **What happens**: `DELETE /triggers/{id}` returns HTTP 200 with an LRO containing the full trigger in `response`. GCP's real Eventarc `DeleteTrigger` LRO resolves to `google.protobuf.Empty`. Returning the full resource on delete looks like the trigger still exists.
- **Expected**: Delete LRO `response` should be `{"@type":"type.googleapis.com/google.protobuf.Empty"}` or omit the `response` field entirely, consistent with GCP behavior.
- **Repro**: `curl -s -w "\nHTTP %{http_code}\n" -X DELETE "$BASE/triggers/audit-trigger"`

---

### [REST-API] NOT_FOUND error messages use inconsistent bracket styles across resource types

- **Severity**: UX-polish
- **What happens**: Trigger and channel NOT_FOUND errors use square brackets: `"Trigger [projects/...] not found"`. Operation NOT_FOUND uses double quotes: `"operation \"does-not-exist\" not found"`. The styles differ.
- **Expected**: A single error message template across all resource types, e.g., always use square brackets or always use double-quotes.
- **Repro**: `curl -s "$BASE/triggers/x"` vs `curl -s "http://localhost:8085/v1/operations/x"`

---

## Area 4: LRO REST Gateway

### [LRO] Polling an LRO by its own `name` field returns NOT_FOUND

- **Severity**: UX-critical
- **What happens**: Every Create/Update/Delete response includes `"name":"projects/my-project/locations/us-central1/operations/..."`. Following the GCP LRO pattern, a user does `GET /v1/<name>` to poll the operation. This returns `{"code":5,"message":"Not Found","details":[]}`. The REST route for the operation name is not registered.
- **Expected**: `GET /v1/projects/{project}/locations/{location}/operations/{id}` should return the LRO object. GCP SDKs that call `op.Wait()` rely on this endpoint. The gRPC `GetOperation` works, but the corresponding REST route does not.
- **Repro**:
  ```bash
  RESP=$(curl -s -X POST "$BASE/triggers?triggerId=lro-test" -H "Content-Type: application/json" -d '{...}')
  OP_NAME=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['name'])")
  curl -s "http://localhost:8085/v1/$OP_NAME"
  # Returns: {"code":5,"message":"Not Found","details":[]}
  ```

---

### [LRO] Operations list endpoint (`$BASE/operations`) returns NOT_FOUND

- **Severity**: UX-critical
- **What happens**: `GET /v1/projects/my-project/locations/us-central1/operations` returns `{"code":5,"message":"Not Found","details":[]}` with HTTP 404. The route is not registered.
- **Expected**: This endpoint should return `{"operations":[...]}` (even if empty), consistent with the `google.longrunning.Operations` REST surface.
- **Repro**: `curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/operations"`

---

### [LRO] LRO behavior is not documented near the REST gateway section

- **Severity**: UX-improvement
- **What happens**: "Immediate LRO resolution" is buried in "Differences from GCP" at the bottom of the README. A new user who tries to poll an LRO REST URL hits NOT_FOUND (the bug above) before reading this note.
- **Expected**: A dedicated "LRO Behavior" note near the REST Gateway section explaining: (1) all LROs resolve to `done: true` immediately, (2) the resource is in the `response` field, (3) REST polling endpoints for operations are not yet implemented.
- **Repro**: Search README for LRO documentation — found only in "Differences from GCP" section.

---

## Area 5: Event Publishing and Routing

### [ROUTING] `go run ./examples/webhook-receiver` fails with "does not contain package"

- **Severity**: UX-critical
- **What happens**: Running `go run ./examples/webhook-receiver` from the project root prints:
  `main module (...) does not contain package .../examples/webhook-receiver`
  The webhook-receiver is a separate Go module (`examples/webhook-receiver/go.mod`) and must be run from its own directory.
- **Expected**: The README Quick Start and `sdk-demo/main.go` source comment both show this command as if it can be run from the project root. The docs must specify `cd examples/webhook-receiver && go run main.go`, or the webhook-receiver should be moved into the main module.
- **Repro**: `go run ./examples/webhook-receiver` (from project root)

---

### [ROUTING] `Ce-Time` header absent from dispatched events

- **Severity**: UX-improvement
- **What happens**: The webhook receiver shows: `Ce-Id`, `Ce-Source`, `Ce-Specversion`, `Ce-Subject`, `Ce-Type`. The `Ce-Time` header is absent even when the publish payload included `"time": {"ceTimestamp": "2026-04-02T12:00:00Z"}` in event attributes.
- **Expected**: If a `time` attribute is provided in the published event, it should be forwarded as a `Ce-Time` header in the dispatched request. The CloudEvents spec treats `time` as an optional but standard context attribute.
- **Repro**: Publish an event with a `time` attribute and inspect the webhook receiver log.

---

## Area 6: Channel Validation

Channel NOT_FOUND behavior is clear and correct. Publishing to a nonexistent channel returns HTTP 404 with a full resource name in the error message. No issues found.

---

## Area 7: Trigger Validation

### [VALIDATION] Validation returns only the first error, not all constraint violations

- **Severity**: UX-improvement
- **What happens**: Creating a trigger with both `destination` and `eventFilters` missing only reports `"trigger.destination is required"`. The second missing field (`eventFilters`) is not reported until the first error is fixed. Single-error responses force users to fix issues one at a time.
- **Expected**: Return all validation errors in a single response using the `details` array (gRPC status details), consistent with how real GCP validates and returns multiple constraint violations.
- **Repro**: `curl -X POST "$BASE/triggers?triggerId=bad" -H "Content-Type: application/json" -d '{}'` — only one error returned.

---

### [VALIDATION] Missing `triggerId` error uses snake_case in a REST camelCase context

- **Severity**: UX-polish
- **What happens**: `POST /triggers` with no `?triggerId=` query parameter returns `{"code":3,"message":"trigger_id is required","details":[]}`. The error uses protobuf snake_case (`trigger_id`) but the REST API surface uses camelCase everywhere (`triggerId`, `eventFilters`, `httpEndpoint`).
- **Expected**: `"message":"triggerId is required"` to be consistent with REST naming conventions.
- **Repro**: `curl -s -X POST "$BASE/triggers" -H "Content-Type: application/json" -d '{...}'`

---

## Area 8: gRPC API

gRPC reflection is enabled and fully functional. All three services (`Eventarc`, `Publisher`, `Operations`) are listed via `grpcurl list`. CRUD operations work correctly over gRPC. One issue found:

### [GRPC] Delete via gRPC returns the deleted resource in `response` instead of Empty

- **Severity**: UX-improvement
- **What happens**: `DeleteTrigger` via gRPC returns an LRO with `done: true` and `response` containing the full Trigger proto. GCP's real `DeleteTrigger` LRO resolves to `google.protobuf.Empty`. This diverges from GCP behavior at the gRPC layer, not just the REST layer.
- **Expected**: Delete LRO `response` should be `google.protobuf.Empty`.
- **Repro**: `grpcurl -plaintext -d '{"name":"..."}' localhost:9085 google.cloud.eventarc.v1.Eventarc/DeleteTrigger`

---

### [GRPC] `grpcurl` not mentioned in README as a diagnostic tool

- **Severity**: UX-polish
- **What happens**: The README has no mention of `grpcurl`. A new user who wants to verify gRPC connectivity or explore available methods must discover this tool independently.
- **Expected**: A short "Verify gRPC connectivity" snippet using `grpcurl -plaintext localhost:9085 list` in the Quick Start or a "Development Tools" section.
- **Repro**: `grep grpcurl README.md` — not found.

---

## Area 9: SDK Demo

### [SDK-DEMO] README does not document that `my-channel` must be pre-created before running the sdk-demo

- **Severity**: UX-critical
- **What happens**: The sdk-demo publishes to `projects/my-project/locations/us-central1/channels/my-channel` but does not create the channel itself. Running the sdk-demo against a fresh emulator without pre-creating `my-channel` causes `must publish events: rpc error: code = NotFound` and exits non-zero. The `sdk-demo/main.go` source comment also omits this step.
- **Expected**: The README or `main.go` source comment should include:
  ```bash
  curl -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels?channelId=my-channel" \
    -H "Content-Type: application/json" -d '{}'
  ```
  before the `go run main.go` step.
- **Repro**: Run `go run ./examples/sdk-demo/main.go` against a fresh emulator without pre-creating `my-channel`.

---

### [SDK-DEMO] SDK demo completion message does not include the event ID to identify the delivery in webhook logs

- **Severity**: UX-improvement
- **What happens**: After publishing, the demo prints "event dispatched; check webhook receiver logs for delivery confirmation" but does not print the event ID (`sdk-evt-<timestamp>`). A user with multiple triggers active (or stale triggers from prior tests) sees multiple entries in the webhook log and cannot immediately identify which one belongs to the current sdk-demo run.
- **Expected**: Include the event ID in the completion message: `→ event id: sdk-evt-1775181935798676000 — search webhook logs for Ce-Id: <id>`.
- **Repro**: Run the full audit sequence and observe the webhook log after the sdk-demo.

---

## Area 10: Env Vars

### [ENV-VARS] Startup log does not confirm `EVENTARC_EMULATOR_TOKEN` is active

- **Severity**: UX-improvement
- **What happens**: When `EVENTARC_EMULATOR_TOKEN` is set, the startup log shows `IAM mode: off` but makes no mention of the token. A user who sets the env var has no way to confirm it was picked up without sending a live test event and inspecting the webhook log.
- **Expected**: A startup log line such as `Token injection: enabled` (or `Bearer token: set`) so users can confirm configuration without a live test.
- **Repro**: `EVENTARC_EMULATOR_TOKEN=my-secret-test-token go run ./cmd/server-dual` — observe startup log.

---

### [ENV-VARS] `IAM_MODE=permissive` allows unauthenticated requests contrary to README documentation

- **Severity**: UX-critical
- **What happens**: The README IAM table states "No principal → Deny" for `permissive` mode. Running `IAM_MODE=permissive go run ./cmd/server-dual` and sending a plain `curl` (no Authorization header, no principal) returns HTTP 200 with results — the request is allowed, not denied.
- **Expected**: `permissive` mode with no principal should return HTTP 403 / gRPC PERMISSION_DENIED, matching the documented behavior table.
- **Repro**: `IAM_MODE=permissive go run ./cmd/server-dual` then `curl http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers` — returns HTTP 200.

---

### [ENV-VARS] Env var prefix is inconsistent across configuration variables

- **Severity**: UX-polish
- **What happens**: Some env vars use the `EVENTARC_` prefix (`EVENTARC_EMULATOR_HOST`, `EVENTARC_HTTP_PORT`, `EVENTARC_EMULATOR_TOKEN`), while others use unrelated prefixes (`GCP_MOCK_LOG_LEVEL`) or no prefix at all (`IAM_MODE`). This increases the chance of name collisions with existing environment variables.
- **Expected**: All env vars should share a common prefix (e.g., `EVENTARC_`) and be documented consistently. `IAM_MODE` → `EVENTARC_IAM_MODE`, `GCP_MOCK_LOG_LEVEL` → `EVENTARC_LOG_LEVEL`.
- **Repro**: `go run ./cmd/server-dual --help` — env var section.

---

## Area 11: Log Level Flag

### [LOG-LEVEL] `--log-level debug` produces no additional output beyond startup lines

- **Severity**: UX-critical
- **What happens**: `go run ./cmd/server-dual --log-level debug` produces exactly 8 lines — identical to `info` and `error` levels. After sending a request, no debug lines appear (no per-request log, no routing decisions, no dispatch results). The flag is accepted and reflected in startup (`Log level: debug`) but has no observable effect on verbosity.
- **Expected**: `debug` level should emit per-request logs (method, path, latency, status), routing decisions ("N triggers matched for type=X"), dispatch results ("dispatching to http://..., status: 200"), and internal state changes.
- **Repro**: `go run ./cmd/server-dual --log-level debug > /tmp/debug.log 2>&1 & sleep 2 && curl -s "$BASE/triggers" && cat /tmp/debug.log` — 8 lines regardless.

---

### [LOG-LEVEL] `--log-level error` does not suppress startup info lines

- **Severity**: UX-improvement
- **What happens**: Setting `GCP_MOCK_LOG_LEVEL=error` still prints all 8 startup info lines. For CI environments that pipe server logs to a file and grep only for errors, the startup lines are noise at every run.
- **Expected**: Startup lines should be emitted at `info` level and suppressed at `error` level. Alternatively, add a `--quiet` flag to suppress startup output independently of runtime log level.
- **Repro**: `GCP_MOCK_LOG_LEVEL=error go run ./cmd/server-dual` — all startup lines still print.

---

### [LOG-LEVEL] Flag takes precedence over env var but this is undocumented

- **Severity**: UX-polish
- **What happens**: When both `GCP_MOCK_LOG_LEVEL=error` and `--log-level debug` are set, the flag wins. This is reasonable but is documented nowhere — not in `--help` nor in the README.
- **Expected**: A note in `--help` next to each flag: `(env: GCP_MOCK_LOG_LEVEL; flag takes precedence)`.
- **Repro**: `GCP_MOCK_LOG_LEVEL=error go run ./cmd/server-dual --log-level debug` — `Log level: debug`.

---

## Area 12: Edge Cases and Error Handling

### [EDGE] Out-of-range port value (`--grpc-port 99999`) fails at runtime, not at flag parse time

- **Severity**: UX-improvement
- **What happens**: `--grpc-port 99999` is accepted without error during flag parsing. The server starts and prints all startup lines before failing: `Failed to listen on gRPC port: listen tcp: address 99999: invalid port`.
- **Expected**: Port range validation (1-65535) should happen immediately during flag parsing, before any startup output, and exit with a clean usage-style error.
- **Repro**: `go run ./cmd/server-dual --grpc-port 99999`

---

### [EDGE] Malformed JSON error message quotes raw Go parser internals

- **Severity**: UX-improvement
- **What happens**: POSTing non-JSON returns `{"code":3,"message":"invalid character 'h' in literal true (expecting 'r')","details":[]}`. This is the raw Go `encoding/json` error. A new user who accidentally sends a plain string (like `"hello"` without braces) may not understand the error.
- **Expected**: Prefix with context: `"request body is not valid JSON: invalid character 'h'..."` to make clear the entire body was rejected.
- **Repro**: `curl -s -X POST "$BASE/triggers?triggerId=bad" -H "Content-Type: application/json" -d 'this is not json'`

---

### [EDGE] Deprecated `--port` flag on `server` binary does not emit a runtime deprecation warning

- **Severity**: UX-improvement
- **What happens**: `go run ./cmd/server --port 9085` starts successfully with no deprecation warning in the log. The only indication that `--port` is deprecated is in `--help`.
- **Expected**: Using `--port` should print to stderr: `WARNING: --port is deprecated; use --grpc-port instead`.
- **Repro**: `go run ./cmd/server --port 9085 > /tmp/log.log 2>&1 & sleep 2 && cat /tmp/log.log` — no deprecation warning.

---

### [EDGE] Unknown path error body omits the requested path

- **Severity**: UX-polish
- **What happens**: `GET /totally/unknown/path` returns `{"code":5,"message":"Not Found","details":[]}`. The error does not identify what was not found.
- **Expected**: `"message": "No route found for GET /totally/unknown/path"` to help diagnose path typos.
- **Repro**: `curl -s -w "\nHTTP %{http_code}\n" "http://localhost:8085/totally/unknown/path"`

---

## Area 13: Test Suite

All tests pass (`go test ./...`, `go test -race ./...`). Test names are descriptive. One minor gap:

### [TESTS] No `make test-integration` or `make test-race` Makefile targets

- **Severity**: UX-polish
- **What happens**: The Makefile has `test: go test ./...` but no targets for the race detector or integration-only runs, even though the README documents both patterns explicitly (`go test -race ./...`, `go test -v -run TestIntegration ./...`).
- **Expected**: Add `test-race` and `test-integration` targets to the Makefile to match the README testing section.
- **Repro**: `cat Makefile`

---

## Area 14: Output Review

### [OUTPUT] REST responses are minified JSON with no pretty-print option

- **Severity**: UX-polish
- **What happens**: All REST responses are compact single-line JSON. GCP's API supports `?prettyPrint=true` to opt into formatted output.
- **Expected**: Support `?prettyPrint=true` query parameter (matching GCP API conventions), or document that `| jq .` is the recommended local pattern. The README Quick Start shows bare `curl` commands with no formatting suggestion.
- **Repro**: `curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/providers"` — single-line output.

---

### [OUTPUT] `--help` displays single-dash flags while README uses double-dash

- **Severity**: UX-polish
- **What happens**: `--help` shows `-grpc-port`, `-http-port`, `-log-level` (Go `flag` package default, single dash). The README shows `--grpc-port`, `--log-level` (double dash). Both work, but the inconsistency can confuse users copying from one source.
- **Expected**: Either update `--help` to display double-dash flags (via a custom `flag.Usage`) or add a note that `-flag` and `--flag` are equivalent.
- **Repro**: `go run ./cmd/server-dual --help` (shows `-grpc-port`) vs README (shows `--grpc-port`).

---

*End of audit report.*
