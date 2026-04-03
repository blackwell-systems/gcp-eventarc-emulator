# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`logger.OrDefault` helper** — `internal/logger` now exports `OrDefault(lgr *Logger) *Logger`; replaces copy-pasted nil-guard blocks in router, dispatcher, and publisher constructors
- **`PaginatePage[T]` generic helper** — extracted from 9 duplicated `List*` pagination blocks across the storage layer into a single generic function in `internal/server/storage_helpers.go`
- **`cloneProto[T]` generic helper** — replaces 13 per-type proto clone wrappers with a single generic function; `cloneProvider` preserved as-is (rewrites the `Name` field post-clone)
- **`newUID` / `newEtag` helpers** — `fmt.Sprintf("%x", rand.Uint64())` extracted into named helpers, eliminating ~15 scattered occurrences
- **`requireField` validation helper** — ~45 inline `if req.GetX() == ""` blocks in `server.go` replaced with a single `requireField(value, fieldName)` call
- **`perm()` IAM constant wrapper** — replaces ~40 hardcoded IAM permission strings in `server.go` with lookups via `authz.GetPermission`; panics on unknown operation names so typos surface at test time rather than silently passing

### Fixed

- **Malformed JSON request bodies** — REST gateway now returns `{"code":3,"message":"request body is not valid JSON"}` (HTTP 400) instead of leaking raw proto parse internals (e.g. `proto: (line 1:15): unexpected end of string`) to callers
- **Startup log bypassed `--log-level`** — All three server binaries used `log.Printf` for the startup banner, which always printed regardless of `--log-level`; replaced with `lgr.Info` so `--log-level error` suppresses it
- **"Ready" log fired before listener was serving** — `cmd/server` and `cmd/server-rest` logged "Ready" before `grpcServer.Serve` / the HTTP listener confirmed binding; both now use a `readyCh` channel to gate the "Ready" line on confirmed readiness (matching `cmd/server-dual`'s existing pattern)
- **Dead code removed** — `NormalizeTriggerResource` (and its test `TestNormalizeTriggerResource`) removed from `internal/authz/permissions.go`; the function had no production callers

### Added

- **`/healthz` and `/readyz` endpoints** — REST gateway (`server-rest`, `server-dual`) now exposes `GET /healthz` and `GET /readyz`; both return `{"status":"ok"}` with HTTP 200, giving CI scripts and container orchestrators a machine-readable readiness signal
- **Leveled logger** (`internal/logger`) — New `Logger` type with `Debug`/`Info`/`Warn`/`Error` methods and `IsDebug() bool`; all three server binaries now propagate `--log-level` to the router, dispatcher, and publisher via variadic constructors so `--log-level debug` actually produces debug output
- **Port range validation** — `--grpc-port` and `--http-port` flags are now validated to be in the 1–65535 range; out-of-range values exit with a descriptive error message
- **`--help` binary blurbs** — Each server binary's `--help` output now includes a "When to use:" line distinguishing `server`, `server-rest`, and `server-dual`
- **`grpcurl` Quick Start tip** — README now includes a "Verify gRPC connectivity" sub-section with the `grpcurl list` command and install link
- **jq pretty-print tip** — README's first curl example now shows the `| jq .` pipe for readable JSON output
- **Backward-compatibility env var note** — Environment variable table now documents that `IAM_MODE` and `GCP_MOCK_LOG_LEVEL` are accepted alongside the newer prefixed forms
- **Makefile `test-race` and `test-integration` targets** — `make test-race` runs `go test -race ./...`; `make test-integration` runs `go test -run Integration ./...`

### Fixed

- **LRO project-scoped GET path** — `GET /v1/projects/{project}/locations/{location}/operations/{name}` now routes correctly; previously the REST gateway only registered `/v1/operations/{name}` so project-scoped LRO polls (the default SDK path) returned 404
- **LRO LIST 404** — `GET /v1/projects/{project}/locations/{location}/operations` now returns the operations list; previously this path was unregistered
- **IAM permissive allows unauthenticated** — `checkPermission` now returns `PERMISSION_DENIED` when the request carries no principal, even in `permissive` mode; previously an empty principal was forwarded to the IAM emulator which returned allowed=true
- **`--log-level debug` had no effect** — Debug log calls in the router, dispatcher, and publisher were unconditional `log.Printf` calls; they now go through the leveled logger and are gated on the configured level
- **Delete LRO response type (all 7 operations)** — All `Delete*` methods now return `google.protobuf.Empty` in the LRO response; previously they returned the deleted resource, which caused SDK `Wait()` to fail unmarshalling
- **`CreateTrigger` multi-error validation** — Field violations for missing `destination` and `event_filters` are now returned together in a single `INVALID_ARGUMENT` with `BadRequest` details; previously only one error was returned
- **LRO error message format** — `GetOperation` and `DeleteOperation` not-found errors now use `[name]` bracket format instead of `%q` quoted format, matching real GCP API style
- **README RPC count** — Fixed "40 RPCs" → "47 RPCs" in the README introduction paragraph
- **Single-dash vs double-dash flag note** — README Run Server section now notes that Go flags accept both `-flag` and `--flag`
- **`--version` includes binary name** — `--version` output now appends `(server)`, `(server-rest)`, or `(server-dual)` to distinguish binaries

- **gRPC debug interceptor** — `--log-level debug` now logs every gRPC method call and outcome (`[DEBUG] grpc: /service/Method → ok`), making it useful for troubleshooting CRUD operations, not just event routing

### Fixed

- **Delete LRO response type (regression)** — All 7 `Delete*` methods again return the deleted resource in the LRO response; a round-2 fix incorrectly changed them to return `google.protobuf.Empty`, breaking `deleteOp.Wait()` in the GCP Go SDK (`DeleteTriggerOperation.Wait()` expects the deleted resource)

- **Docker support** — Multi-stage `Dockerfile` (~17MB final image) and `docker-compose.yml` wiring the emulator with a webhook receiver for one-command local demos
- **SDK demo** (`examples/sdk-demo`) — End-to-end example using the official `cloud.google.com/go/eventarc` and `cloud.google.com/go/eventarc/publishing` SDK clients against the emulator; proves drop-in compatibility without GCP credentials
- **curl demo** (`examples/demo.sh`) — Shell script demonstrating the full event flow via REST: providers, channels, triggers, message buses, pipelines, enrollments, and CloudEvent publish → delivery
- **Webhook receiver** (`examples/webhook-receiver`) — Tiny Go HTTP server that pretty-prints incoming CloudEvents showing `Ce-*` headers and body
- **LRO REST gateway** — `GET /v1/operations/{name}` now routes to the LRO store via a buf-generated grpc-gateway handler (`internal/gen/google/longrunning/operations.pb.gw.go`); `GetOperation`, `DeleteOperation`, `CancelOperation`, and `WaitOperation` are all accessible over REST; previously all operations endpoints returned 404
- **`--version` flag** — All three server binaries (`server`, `server-rest`, `server-dual`) now print the version and exit when `--version` is passed
- **Provider event types** — Seeded GCP providers (`pubsub.googleapis.com`, `storage.googleapis.com`, `firebase.googleapis.com`) now include representative `EventType` entries matching their real GCP counterparts

### Fixed

- **Delete LRO response type** — All 7 delete operations (`DeleteTrigger`, `DeleteChannel`, `DeleteChannelConnection`, `DeleteMessageBus`, `DeleteEnrollment`, `DeletePipeline`, `DeleteGoogleApiSource`) now return the deleted resource in the LRO response instead of `google.protobuf.Empty`, matching real GCP Eventarc SDK expectations (`deleteOp.Wait()` returns the deleted resource)
- **Channel existence validation** — `PublishEvents` now returns `NOT_FOUND` when the target channel does not exist; previously it silently returned 200 and dropped the events
- **Trigger input validation** — `CreateTrigger` now returns `INVALID_ARGUMENT` when `destination` is nil or `event_filters` is empty; previously invalid triggers were accepted and silently failed on every publish
- **Channel state** — Newly created channels now have `State: ACTIVE`; previously `State` was always `STATE_UNSPECIFIED`
- **`GoogleChannelConfig.update_time`** — The singleton config now returns a non-null `update_time`; previously the field was always null
- **IAM strict mode error** — Connection-refused errors from the IAM emulator are now returned as `FAILED_PRECONDITION` with a user-friendly message instead of leaking raw gRPC internals to the client
- **Startup log consistency** — All three server variants now log in the same order: version, log level, IAM mode, port bindings, then "Ready"; the "Ready" line is emitted only after the gRPC listener confirms it is serving
- **Invalid log level** — Passing an unrecognized `--log-level` value now exits with an error message; previously the flag was silently ignored
- **Routing feedback** — `matchAndDispatch` now logs matched trigger count before dispatching; `PublishEvents` logs a warning when called with zero events
- **SDK demo** — Removed the unconditional "event routed to trigger destination" claim; added a local (non-Docker) usage section to the package comment
- **Webhook receiver** — Now prints the `Authorization` header when present, making `EVENTARC_EMULATOR_TOKEN` bearer injection visible in demo output
- **README prerequisites** — Added Docker Compose plugin v2 as an explicit prerequisite for Docker-based workflows; clarified that `docker compose` (plugin v2) is required, not the standalone `docker-compose` v1

## [0.1.0] - 2026-04-02

### Added

- **Eventarc gRPC service** — All 40 RPCs across 8 resource types:
  - Trigger CRUD (5 RPCs)
  - Channel CRUD (5 RPCs)
  - ChannelConnection CRUD (4 RPCs)
  - GoogleChannelConfig Get/Update (2 RPCs)
  - MessageBus CRUD + ListEnrollments (6 RPCs)
  - Enrollment CRUD (5 RPCs)
  - Pipeline CRUD (5 RPCs)
  - GoogleApiSource CRUD (5 RPCs)
  - Provider Get/List (2 RPCs) with seeded default GCP providers
- **Publisher gRPC service** — `PublishEvents` and `PublishChannelConnectionEvents` RPCs; unpacks proto CloudEvent to SDK event, routes, and dispatches
- **Long-running operations** — All mutating operations return `google.longrunning.Operation` (resolved immediately); full `OperationsServer` (Get, List, Delete, Cancel, Wait)
- **CloudEvent routing** — Attribute filter matching on trigger `matchingCriteria` (type, source, custom extensions); CEL condition evaluation via `cel-go` for advanced filtering
- **HTTP delivery** — Binary content mode (`ce-*` headers, raw data body) matching real GCP Eventarc behavior
- **Authorization token** — `EVENTARC_EMULATOR_TOKEN` env var adds `Authorization: Bearer` header to dispatched requests (for Cloud Run targets)
- **REST gateway** — grpc-gateway v2 transcoding for both Eventarc and Publisher services; REST paths match the real GCP API
- **Three server variants** — `cmd/server` (gRPC only), `cmd/server-rest` (REST only), `cmd/server-dual` (gRPC + REST)
- **IAM integration** — Optional pre-flight authorization via [gcp-emulator-auth](https://github.com/blackwell-systems/gcp-emulator-auth) with 36 permission mappings; off/permissive/strict modes
- **In-memory storage** — Thread-safe with `sync.RWMutex`, `proto.Clone` for safe copies, integer-offset pagination, sorted list results
- **Integration tests** — End-to-end tests via bufconn covering Trigger CRUD, Channel CRUD, MessageBus CRUD, Pipeline CRUD, GoogleChannelConfig, Provider queries, error cases, and full-stack PublishEvents → Router → Dispatcher → HTTP delivery
- **CI pipeline** — GitHub Actions with test (matrix: ubuntu/macOS/windows, Go 1.24/1.25), lint (golangci-lint), format check, go vet, and build verification for all three server variants

[0.1.0]: https://github.com/blackwell-systems/gcp-eventarc-emulator/releases/tag/v0.1.0
