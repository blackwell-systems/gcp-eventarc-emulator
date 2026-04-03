# Cold-Start UX Audit — gcp-eventarc-emulator

**Date:** 2026-04-02
**Version:** v0.1.0
**Auditor:** Cold-start new user simulation
**Scope:** All 16 audit areas from `docs/cold-start-audit-prompt.md`

---

## Summary

| Severity | Count |
|----------|-------|
| UX-critical | 3 |
| UX-improvement | 8 |
| UX-polish | 9 |
| **Total** | **20** |

---

## Area 1: Discovery

### [Discovery] `--log-level debug` produces no debug output

- **Severity**: UX-critical
- **What happens**: Running `server-dual --log-level debug` logs `[INFO] Log level: debug` at startup, but after making API requests no debug-level entries appear. The log output is identical to the default `info` level. The debug setting is accepted and announced but has no observable effect.
- **Expected**: Debug level should emit per-request logs (method name, status code, latency) and any other internal diagnostic information. A user who sets `--log-level debug` to troubleshoot routing or dispatch issues sees nothing extra.
- **Repro**:
  ```
  cd /Users/dayna.blackwell/code/gcp-eventarc-emulator && \
    go run ./cmd/server-dual --log-level debug > /tmp/emulator-debug.log 2>&1 &
  sleep 2
  curl -s http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers
  kill %1
  cat /tmp/emulator-debug.log
  # Output: 8 lines total, no [DEBUG] entries, same output as --log-level info
  ```

---

### [Discovery] `EVENTARC_EMULATOR_TOKEN` is absent from `--help` env vars table

- **Severity**: UX-improvement
- **What happens**: The `--help` output for all three server variants lists four environment variables: `EVENTARC_EMULATOR_HOST`, `EVENTARC_HTTP_PORT`, `GCP_MOCK_LOG_LEVEL`, and `IAM_MODE`. `EVENTARC_EMULATOR_TOKEN` — the variable that injects a bearer token into dispatched HTTP delivery requests — is not listed. A new user looking at `--help` to understand configuration options will not discover this feature.
- **Expected**: All supported environment variables should appear in the `--help` env vars table, including `EVENTARC_EMULATOR_TOKEN` with a brief description such as `Bearer token injected into dispatched HTTP delivery requests`.
- **Repro**:
  ```
  go run ./cmd/server-dual --help 2>&1 | grep -i token
  # Output: (no output — EVENTARC_EMULATOR_TOKEN not listed)
  ```

---

### [Discovery] `--help` flag listing uses single-dash style; README uses double-dash

- **Severity**: UX-polish
- **What happens**: The Go `flag` package displays flags with a single dash (e.g., `-grpc-port`, `-log-level`) in `--help` output. The README prose uses double-dash style (e.g., `--log-level`, `--grpc-port`). Both forms work at the command line, but the inconsistency between help text and README examples creates confusion for new users trying to copy flags.
- **Expected**: Flag listings in `--help` should consistently use the double-dash convention to match the README and common CLI expectations.
- **Repro**:
  ```
  go run ./cmd/server-dual --help
  # Flags section shows: -grpc-port, -http-port, -log-level (single dash)
  # README shows: --log-level, --grpc-port in prose (double dash)
  ```

---

## Area 2: First Run — Startup and Port Bindings

The startup sequence works. The server prints a clear "ready" signal: `[INFO] Ready to accept both gRPC and REST requests`. Port bindings are announced. Triggers return an empty list on first call. Providers return seeded data on first call.

### [First Run] Port announcement lines appear in non-deterministic order

- **Severity**: UX-polish
- **What happens**: The HTTP gateway and gRPC server lines (`[INFO] HTTP gateway listening at :8085` and `[INFO] gRPC server listening at [::]:9085`) appear in different order across runs. In some runs gRPC is announced first; in others HTTP is announced first.
- **Expected**: Port announcements should appear in a consistent, logical order (gRPC first, then HTTP, or the reverse) so that log tailing and scripted startup detection are predictable.
- **Repro**:
  ```
  go run ./cmd/server-dual
  # Run multiple times; lines 4 and 5 swap order
  ```

---

### [First Run] Startup log "REST:" example URL hardcodes `my-project`

- **Severity**: UX-polish
- **What happens**: The startup log always prints:
  ```
  [INFO] REST: http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers
  ```
  This is a hardcoded example URL, not reflective of any user configuration. A new user may assume this path is tied to their setup, or may try to use it literally before understanding the project/location are placeholders.
- **Expected**: Label the line explicitly as an example, e.g., `[INFO] REST example: http://localhost:8085/v1/projects/<project>/locations/<location>/...`, or omit the hardcoded project path.
- **Repro**:
  ```
  go run ./cmd/server-dual
  # Startup always shows: my-project/locations/us-central1/triggers regardless of any config
  ```

---

## Area 3: REST API — Core Resource Lifecycle

All create/list/get/delete operations for triggers, channels, message buses, pipelines, and enrollments work correctly. Resources are created with fully-qualified GCP-style names. Get-after-delete returns 404 / NOT_FOUND as expected.

### [REST API] Empty string and null fields pollute JSON responses

- **Severity**: UX-polish
- **What happens**: REST responses include zero-value proto fields that the real GCP API omits. For example, a trigger response includes `"operator": ""`, `"serviceAccount": ""`, `"channel": ""`, `"eventDataContentType": ""`, `"etag": ""`, `"networkConfig": null`, `"transport": null`, `"retryPolicy": null`, and `"conditions": {}`. Provider event type entries include `"filteringAttributes": []` and `"eventSchemaUri": ""`.
- **Expected**: Zero-value proto fields should be omitted from JSON output using `omitempty` or the grpc-gateway `EmitUnpopulated: false` option. This matches GCP API behavior and reduces noise for developers inspecting responses.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/providers" | python3 -m json.tool
  # Shows: "filteringAttributes": [], "eventSchemaUri": "" for every event type entry
  ```

---

### [REST API] Delete LRO response uses `{"value":{}}` inside `response` field

- **Severity**: UX-polish
- **What happens**: A successful delete returns HTTP 200 with:
  ```json
  {"name":"...operations/...","metadata":null,"done":true,"response":{"@type":"type.googleapis.com/google.protobuf.Empty","value":{}}}
  ```
  The real GCP API returns `{}` as the `response` value for delete LROs. The `{"value":{}}` representation is an artifact of how `google.protobuf.Empty` is marshalled to JSON and may confuse developers comparing emulator behavior to real GCP.
- **Expected**: The `response` field for delete LROs should be `{}` rather than `{"@type":"...Empty","value":{}}`, or at minimum this behavior should be noted in documentation.
- **Repro**:
  ```
  curl -s -w "\nHTTP %{http_code}\n" -X DELETE \
    "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/audit-trigger"
  # Returns LRO with response: {"@type":"...Empty","value":{}}
  ```

---

## Area 4: Health Endpoints

`/healthz` and `/readyz` both return `{"status":"ok"}` with HTTP 200 on the dual-protocol and REST-only servers. The gRPC-only server (`server`) has no HTTP port so health probes via curl return HTTP 000 (connection refused), which is expected.

### [Health] Health endpoints not documented in `--help` or README

- **Severity**: UX-improvement
- **What happens**: `/healthz` and `/readyz` work correctly but are not mentioned in `--help` output or anywhere in the README. A new user setting up container health checks or a readiness probe has no way to discover these endpoints from the tool's own documentation.
- **Expected**: The `--help` output (or at minimum the README) should list `/healthz` and `/readyz` as available endpoints. Ideally they also appear in the startup log alongside the port announcements.
- **Repro**:
  ```
  go run ./cmd/server-dual --help 2>&1 | grep -i health
  grep -i healthz README.md
  # Both return no output
  ```

---

### [Health] gRPC-only server has no documented alternative for health probing

- **Severity**: UX-improvement
- **What happens**: Users who choose `server` (gRPC-only) cannot use HTTP health probes. The `--help` text does not mention this limitation and does not suggest using gRPC health probes (`grpc.health.v1`) or switching to `server-dual` for health-probe support. A user setting up `server` in a container orchestration environment will discover this limitation only by failing.
- **Expected**: The `server` binary's `--help` or startup log should note that no HTTP health endpoint is available and suggest `server-dual` if HTTP health probes are needed.
- **Repro**:
  ```
  go run ./cmd/server --help
  # No mention of health probes or absence of /healthz
  ```

---

## Area 6: LRO REST Gateway

LRO polling works correctly. `GET /v1/<operation-name>` returns the full LRO with `done: true` and the resource embedded in `response`. The `$BASE/operations` list endpoint is accessible. Nonexistent operations return 404.

### [LRO] NOT_FOUND error messages use inconsistent quoting style across resource types

- **Severity**: UX-polish
- **What happens**: NOT_FOUND errors use different formatting depending on resource type:
  - Trigger: `"Trigger [projects/.../triggers/name] not found"` — square brackets
  - Channel: `"Channel \"projects/.../channels/name\" not found"` — escaped double quotes
  - Operation: `"operation [does-not-exist] not found"` — square brackets, no project path, lowercase resource name
- **Expected**: All NOT_FOUND error messages should use the same formatting for the resource name (e.g., square brackets or double quotes, consistently). Operations should also include the fully-qualified name to aid debugging.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/operations/does-not-exist"
  curl -s "http://localhost:8085/v1/projects/p/locations/l/channels/x"
  curl -s "http://localhost:8085/v1/projects/p/locations/l/triggers/x"
  # Three different quoting and casing styles
  ```

---

## Area 7: Event Publishing and Routing

The full CloudEvent delivery loop works correctly end-to-end:

- `publishEvents` returns HTTP 200 with `{}`.
- Matching events are delivered to the webhook with all expected `Ce-*` headers: `Ce-Id`, `Ce-Source`, `Ce-Specversion`, `Ce-Subject`, `Ce-Type`.
- Non-matching events (wrong type) are silently dropped and never delivered to the webhook.
- Event body is delivered as valid JSON in the HTTP body with `Content-Type: text/plain`.

---

## Area 8: Channel Validation

Publishing to a nonexistent channel returns HTTP 404 with a clear NOT_FOUND error message naming the missing channel. No issues.

---

## Area 9: Trigger Validation

Validation is strong across all tested cases. Missing destination returns 400 with field violations. Empty body returns 400 with multiple field violations. Missing `triggerId` returns 400. Duplicate trigger returns 409 with ALREADY_EXISTS.

### [Validation] Error responses omit the `"status"` string field from gRPC-gateway convention

- **Severity**: UX-polish
- **What happens**: gRPC-gateway error responses conventionally include three fields: `code` (integer), `message` (string), and `status` (string, e.g., `"NOT_FOUND"`). The emulator returns only `code` and `message`:
  ```json
  {"code":5,"message":"Trigger [...] not found","details":[]}
  ```
  The string `"status"` field is absent. Clients that pattern-match on `response.status === "NOT_FOUND"` will not find it.
- **Expected**: Add the `"status"` string field matching the gRPC status name to all error responses (e.g., `"status":"NOT_FOUND"`, `"status":"ALREADY_EXISTS"`, `"status":"INVALID_ARGUMENT"`).
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers/nope"
  # Returns: {"code":5,"message":"...","details":[]} — no "status" field
  ```

---

## Area 10: gRPC API

gRPC reflection is enabled and works without a `.proto` file. All three services are exposed: `google.cloud.eventarc.v1.Eventarc` (40 methods), `google.cloud.eventarc.publishing.v1.Publisher` (3 methods), `google.longrunning.Operations` (5 methods). Snake_case field names work as expected. The LRO in `CreateTrigger` is marked `done: true` with the trigger embedded in `response`. No issues found.

---

## Area 11: SDK Demo

### [SDK Demo] Delete trigger step fails with proto mismatch — sdk-demo exits non-zero

- **Severity**: UX-critical
- **What happens**: The SDK demo runs steps 1-7 successfully (list providers, get provider, create trigger, get trigger, list triggers, update trigger, publish event), then fails at step 8 (Delete trigger) with:
  ```
  2026/04/02 20:23:09 ERROR wait for delete: proto: mismatched message type: got "google.cloud.eventarc.v1.Trigger", want "google.protobuf.Empty"
  exit status 1
  ```
  The `DeleteTrigger` LRO `response` field contains a `Trigger` proto instead of a `google.protobuf.Empty`. The Go SDK's `WaitOperation` call expects `Empty` for delete operations and fails to unmarshal.
- **Expected**: The `DeleteTrigger` LRO response should contain `google.protobuf.Empty`, not a `Trigger`. The sdk-demo should complete all 8 steps and exit 0.
- **Repro**:
  ```
  # Start emulator
  go run ./cmd/server-dual &
  sleep 2
  # Create my-channel (or let sdk-demo create it)
  curl -s -X POST "http://localhost:8085/v1/projects/my-project/locations/us-central1/channels?channelId=my-channel" \
    -H "Content-Type: application/json" -d '{}'
  # Run SDK demo
  cd /Users/dayna.blackwell/code/gcp-eventarc-emulator/examples/sdk-demo && \
    EVENTARC_EMULATOR_HOST=localhost:9085 \
    WEBHOOK_URI=http://localhost:3000/events \
    go run main.go
  # Exit code 1, ERROR at step 8
  ```

---

### [SDK Demo] `go run ./examples/webhook-receiver` from project root fails with unhelpful error

- **Severity**: UX-improvement
- **What happens**: The webhook receiver lives in `examples/webhook-receiver/` with its own `go.mod`. Running `go run ./examples/webhook-receiver` from the project root fails with:
  ```
  main module (github.com/blackwell-systems/gcp-eventarc-emulator) does not contain package
  github.com/blackwell-systems/gcp-eventarc-emulator/examples/webhook-receiver
  ```
  The error message does not explain the module boundary issue. The README correctly documents `cd examples/webhook-receiver && go run main.go`, but a user running all commands from the project root will hit this error with no actionable guidance.
- **Expected**: Either add a `go.work` workspace file to allow running from the project root, or document in the error output (or README) that the webhook-receiver is a separate module requiring `cd examples/webhook-receiver` first.
- **Repro**:
  ```
  go run ./examples/webhook-receiver
  # Error: main module does not contain package ... (no mention of separate go.mod)
  ```

---

### [SDK Demo] README SDK Demo section does not mention `WEBHOOK_URI` env var

- **Severity**: UX-improvement
- **What happens**: The README SDK Demo section shows:
  ```bash
  cd examples/sdk-demo && EVENTARC_EMULATOR_HOST=localhost:9085 go run main.go
  ```
  The actual command requires `WEBHOOK_URI=http://localhost:3000/events` as well (the sdk-demo uses this to configure the trigger destination). Without `WEBHOOK_URI`, the sdk-demo creates a trigger pointing to an empty or default URI, and event delivery will silently fail or reach the wrong endpoint.
- **Expected**: The README should include `WEBHOOK_URI=http://localhost:3000/events` in the sdk-demo run command, matching the audit prompt's correct invocation.
- **Repro**:
  ```
  # README command (missing WEBHOOK_URI):
  cd examples/sdk-demo && EVENTARC_EMULATOR_HOST=localhost:9085 go run main.go
  # Correct command used in audit:
  EVENTARC_EMULATOR_HOST=localhost:9085 WEBHOOK_URI=http://localhost:3000/events go run main.go
  ```

---

## Area 12: Env Vars — EVENTARC_EMULATOR_TOKEN and IAM_MODE

`EVENTARC_EMULATOR_TOKEN` works correctly. The `Authorization: Bearer <token>` header is injected into dispatched delivery requests and appears in webhook logs. The startup log correctly reports the active `IAM mode:` at startup.

### [IAM] Strict mode with no IAM emulator returns HTTP 400 (FAILED_PRECONDITION) instead of HTTP 403

- **Severity**: UX-improvement
- **What happens**: When `IAM_MODE=strict` and no IAM emulator is reachable, all requests return:
  ```json
  {"code":9,"message":"IAM_MODE is active but no IAM emulator is reachable. Start the IAM emulator or set IAM_MODE=off.","details":[]}
  HTTP 400
  ```
  gRPC code 9 is `FAILED_PRECONDITION`. While the error message is helpful, a developer expecting a 403 (permission denied) when IAM enforcement is active will be confused by 400. The README table says strict + no IAM emulator = "Deny" without specifying the error code.
- **Expected**: Consider returning `PERMISSION_DENIED` (code 7, HTTP 403) for all IAM denials including the misconfiguration case. This would match the "Deny" semantics documented in the README table and match what a user would expect from an IAM enforcement layer.
- **Repro**:
  ```
  IAM_MODE=strict go run ./cmd/server-dual &
  sleep 2
  curl -s -w "\nHTTP %{http_code}\n" \
    "http://localhost:8085/v1/projects/my-project/locations/us-central1/triggers"
  # Returns HTTP 400, not 403
  ```

---

### [IAM] README IAM table rows "No IAM emulator" and "IAM unavailable" are ambiguous duplicates

- **Severity**: UX-polish
- **What happens**: The README IAM enforcement table has two rows that both show `Allow` for `permissive` mode:
  - "No IAM emulator" → permissive: Allow
  - "IAM unavailable" → permissive: Allow

  These appear to describe the same condition. The distinction (if any) between "not configured" and "configured but unreachable" is not explained.
- **Expected**: Collapse these two rows into one, or add a note explaining the distinction between the two failure modes (e.g., `IAM_EMULATOR_HOST` not set vs. set but unreachable).
- **Repro**: Read `README.md` section "IAM Integration / Enforcement Modes".

---

## Area 13: Log Level Flag

### [Log Level] `--log-level debug` behavior is a duplicate of the Area 1 finding

This finding is documented under Area 1 (Discovery) where a new user first encounters it. See: **[Discovery] `--log-level debug` produces no debug output**.

### [Log Level] Flag-vs-env-var precedence documented in `--help` but not in README

- **Severity**: UX-polish
- **What happens**: The `--log-level` flag in `--help` explicitly states `(env: GCP_MOCK_LOG_LEVEL; flag takes precedence)`. This is helpful. The README Configuration section lists `GCP_MOCK_LOG_LEVEL` as an env var but does not mention that the `--log-level` flag overrides it. Behavior is correct — flag wins — but a user who sets the env var and a flag simultaneously may not know which takes precedence.
- **Expected**: The README Configuration / Environment Variables table should note flag precedence for `GCP_MOCK_LOG_LEVEL` (and any other overridable env vars).
- **Repro**:
  ```
  GCP_MOCK_LOG_LEVEL=error go run ./cmd/server-dual --log-level debug
  # Result: debug wins; log shows "[INFO] Log level: debug"
  # README does not document this precedence
  ```

---

## Area 14: Edge Cases and Error Handling

### [Edge Cases] Invalid port type error says "parse error" with no type guidance

- **Severity**: UX-improvement
- **What happens**: Passing a non-numeric port value gives:
  ```
  invalid value "notaport" for flag -grpc-port: parse error
  ```
  The message "parse error" is terse and does not tell the user what type is expected (integer).
- **Expected**: The error should say something like `invalid value "notaport" for flag -grpc-port: expected integer`, matching the clearer style of the out-of-range error (`must be in range 1-65535`).
- **Repro**:
  ```
  go run ./cmd/server-dual --grpc-port notaport
  # Error: "parse error" — no type hint
  go run ./cmd/server-dual --grpc-port 99999
  # Error: "must be in range 1-65535" — clearer
  ```

---

### [Edge Cases] Malformed JSON error exposes raw Go parse internals

- **Severity**: UX-improvement
- **What happens**: Sending a non-JSON body returns:
  ```json
  {"code":3,"message":"invalid character 'h' in literal true (expecting 'r')","details":[]}
  ```
  This is a raw `encoding/json` error. It mentions a character but gives no hint about what was expected at that position or that the body must be a JSON object.
- **Expected**: The error message should be more user-facing: e.g., `"request body must be valid JSON"` with optionally the underlying parse detail appended. The current message leaks implementation details without helping the user fix their input.
- **Repro**:
  ```
  curl -s -X POST "http://localhost:8085/v1/projects/p/locations/l/triggers?triggerId=t" \
    -H "Content-Type: application/json" \
    -d 'this is not json'
  # Returns: "invalid character 'h' in literal true (expecting 'r')"
  ```

---

### [Edge Cases] DELETE nonexistent resource returns 404 (correct)

No issue. Deleting a nonexistent trigger returns HTTP 404 with NOT_FOUND. This is correct GCP behavior.

### [Edge Cases] Unknown path returns 404 with minimal body

- **Severity**: UX-polish
- **What happens**: A completely unknown path returns:
  ```json
  {"code":5,"message":"Not Found","details":[]}
  HTTP 404
  ```
  The message is generic `"Not Found"` with no hint about the valid path structure.
- **Expected**: The 404 for unknown paths could include a hint such as `"path not recognized; REST API paths follow the pattern /v1/projects/{project}/locations/{location}/..."`. This is a minor quality-of-life note.
- **Repro**:
  ```
  curl -s -w "\nHTTP %{http_code}\n" "http://localhost:8085/totally/unknown/path"
  # Returns: {"code":5,"message":"Not Found","details":[]}
  ```

---

## Area 15: Test Suite

All tests pass with no race conditions detected:

```
go test ./...         # all pass
go test -race ./...   # all pass
go test -v -run TestIntegration ./...  # passes
```

`make test`, `make test-race`, and `make test-integration` targets exist in the Makefile.

### [Tests] No test files in any `cmd/` package

- **Severity**: UX-polish
- **What happens**: All three `cmd/` packages report `[no test files]`. Startup flag parsing, `--version` output, and `--log-level` behavior are untested. The fact that `--log-level debug` has no observable effect on verbosity is not caught by any existing test.
- **Expected**: Smoke tests for each `cmd/` entry point would catch regressions in startup behavior, flag validation, and version output.
- **Repro**:
  ```
  go test ./...
  # ?   .../cmd/server      [no test files]
  # ?   .../cmd/server-dual [no test files]
  # ?   .../cmd/server-rest [no test files]
  ```

---

## Area 16: Output Review

### [Output] REST responses are minified JSON by default with no pretty-print option

- **Severity**: UX-polish
- **What happens**: All REST responses are minified single-line JSON. There is no `?prettyPrint=true` query parameter (a GCP API convention). The README includes a tip to pipe through `| jq .`, but users reading raw curl output in a terminal will see dense, unreadable responses.
- **Expected**: Support a `?prettyPrint=true` query parameter (matching GCP API convention) for developer ergonomics during local development and testing.
- **Repro**:
  ```
  curl -s "http://localhost:8085/v1/projects/my-project/locations/us-central1/providers"
  # Output: single-line minified JSON only
  ```

---

### [Output] Startup log mixes two formats: bare timestamp on banner line, `[INFO]` on all others

- **Severity**: UX-polish
- **What happens**: The first startup line has no log level label:
  ```
  2026/04/02 20:25:39 GCP Eventarc Emulator v0.1.0 (gRPC + REST)
  ```
  All subsequent lines have `[INFO]`:
  ```
  2026/04/02 20:25:39 [INFO] Log level: info
  ```
  This inconsistency is minor but visible in terminals and log aggregators that parse the log level label.
- **Expected**: All startup log lines should use the same format. Either add `[INFO]` to the banner line or remove it from subsequent lines.
- **Repro**:
  ```
  go run ./cmd/server-dual
  # Line 1: no [INFO]; lines 2-8 have [INFO]
  ```

---

### [Output] `server` binary startup log differs in terminology and structure from `server-dual`

- **Severity**: UX-polish
- **What happens**: The `server` (gRPC-only) startup log uses different terminology than `server-dual`:
  - `server`: `"Server listening at [::]:9085"`, `"Ready to accept connections"`, `"Publisher service registered"`
  - `server-dual`: `"gRPC server listening at [::]:9085"`, `"HTTP gateway listening at :8085"`, `"Ready to accept both gRPC and REST requests"`

  The `server` banner line also omits the `(gRPC + REST)` / `(gRPC-only)` suffix seen in `server-dual`. The `"Publisher service registered"` line appears only in `server`, not `server-dual`, even though both expose the Publisher service.
- **Expected**: Startup log lines should follow a consistent structure across server variants with variant-appropriate protocol labels. The banner line should include the protocol suffix for all variants.
- **Repro**:
  ```
  go run ./cmd/server
  go run ./cmd/server-dual
  # Compare startup log structure — different terminology throughout
  ```
