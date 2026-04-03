# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
