# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.3] - 2026-04-05

### Changed

- Updated `emulatorauth.NewClient` to v0.4.0 signature (removed `component` parameter)
- Added `git` to Docker builder stage for private module downloads

### Changed

- Updated `emulatorauth.NewClient` call to v0.4.0 signature (removed `component` parameter)

## [0.2.2] - 2026-04-03

### Added
- Pagination (`page_size`, `page_token`) on ListChannels, ListMessageBuses, ListEnrollments, ListPipelines, ListGoogleApiSources — all now consistent with the existing pagination on ListTriggers

## [0.2.1] - 2026-04-03

### Fixed
- `UpdateChannel` now supports wildcard mask `"*"` to update all mutable fields
- `Trigger.conditions` map now set to `{"Ready": {code: 0}}` on create and update
- Resource ID format validated on all Create methods (`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
- `order_by` now respected on ListChannels, ListMessageBuses, ListEnrollments, ListPipelines, ListGoogleApiSources (supports `create_time desc`; default sorts by name)

## [0.2.0] - 2026-04-03

### Fixed
- **LRO metadata always nil** — All 20 Create/Update/Delete operations now pack `OperationMetadata` with `create_time`, `end_time`, `target`, `verb`, and `api_version` fields
- **Etag never validated on Delete** — DeleteTrigger, DeleteMessageBus, DeleteEnrollment, DeletePipeline, DeleteGoogleApiSource now return `ABORTED` if the provided etag doesn't match the stored resource
- **`allow_missing` ignored** — Update methods now upsert (create if not found) when `allow_missing=true`; Delete methods return success if not found when `allow_missing=true`
- **`match-path-pattern` uses exact match** — EventFilter operator `match-path-pattern` now implements proper glob semantics with `**` (any path sequence) and `*` (single segment)
- **`validate_only` silently ignored** — All 18 mutating RPCs now validate the request and return success without persisting when `validate_only=true`
- **Trigger etag never set** — `CreateTrigger` and `UpdateTrigger` now generate and store etag values
- **Channel initial state was ACTIVE** — Newly created channels now have `State: PENDING` and generate an `activation_token`; `CreateChannelConnection` clears the token (input-only field)
- **`GoogleChannelConfig.update_time` unstable** — `update_time` is now initialized once at first access and remains stable across subsequent Gets
- **Wildcard update mask `"*"` ignored** — All Update methods (Trigger, MessageBus, Enrollment, Pipeline, GoogleApiSource, GoogleChannelConfig) now treat `"*"` as updating all mutable fields
- **`Enrollment.message_bus` mutable** — Explicitly attempting to update `message_bus` via update mask now returns `INVALID_ARGUMENT` (immutable after creation)
- **Missing required field validation** — `CreateEnrollment` validates `cel_match`; `CreatePipeline` validates `destinations`; `CreateGoogleApiSource` validates `destination`

### Added
- `matchPathPattern` and `matchSegs` functions in router for glob path filter evaluation
- 27 new router unit tests covering edge cases for path pattern matching

## [0.1.2] - 2026-04-03

### Added
- `Register()` composition hook for unified `gcp-emulator`
- `NewGatewayHandler()` for mounting Eventarc REST gateway in unified HTTP server
- `gateway.Handler()` method for embedding in parent HTTP multiplexer

### Fixed
- `Register()` no longer calls `reflection.Register`, preventing fatal duplicate registration when composing multiple emulators

## [0.1.0] - 2026-04-02

### Added

- **Eventarc gRPC service** — All 47 RPCs across 8 resource types plus Publishing and Operations:
  - Trigger CRUD (5 RPCs)
  - Channel CRUD (5 RPCs)
  - ChannelConnection CRUD (4 RPCs)
  - GoogleChannelConfig Get/Update (2 RPCs)
  - MessageBus CRUD + ListEnrollments (6 RPCs)
  - Enrollment CRUD (5 RPCs)
  - Pipeline CRUD (5 RPCs)
  - GoogleApiSource CRUD (5 RPCs)
  - Provider Get/List (2 RPCs) with seeded GCP providers (`pubsub`, `storage`, `firebase`) including representative `EventType` entries
  - Publisher `PublishEvents` and `PublishChannelConnectionEvents` RPCs
  - Full `OperationsServer` (Get, List, Delete, Cancel, Wait)
- **CloudEvent routing** — Attribute filter matching on trigger `event_filters` (type, source, custom extensions); CEL condition evaluation via `cel-go` for advanced filtering
- **HTTP delivery** — Binary content mode (`ce-*` headers, raw data body) matching real GCP Eventarc behavior
- **REST gateway** — grpc-gateway v2 transcoding; REST paths match the real GCP API; project-scoped LRO paths (`/v1/projects/{p}/locations/{l}/operations/{id}`) routed correctly
- **Three server variants** — `cmd/server` (gRPC only), `cmd/server-rest` (REST only), `cmd/server-dual` (gRPC + REST)
- **Authorization token** — `EVENTARC_EMULATOR_TOKEN` env var adds `Authorization: Bearer` header to dispatched webhook requests
- **IAM integration** — Optional pre-flight authorization via [gcp-emulator-auth](https://github.com/blackwell-systems/gcp-emulator-auth) with 39 permission mappings; off/permissive/strict modes
- **Leveled logger** — `internal/logger` with `Debug`/`Info`/`Warn`/`Error` methods and `IsDebug()`; all binaries propagate `--log-level` through to router, dispatcher, and publisher
- **gRPC debug interceptor** — `--log-level debug` logs every RPC method and outcome (`[DEBUG] grpc: /service/Method → ok`)
- **`/healthz` and `/readyz` endpoints** — REST and dual servers expose `GET /healthz` and `GET /readyz`; both return `{"status":"ok"}` HTTP 200
- **Port range validation** — `--grpc-port` and `--http-port` validated to 1–65535; out-of-range values exit with a descriptive error
- **In-memory storage** — Thread-safe with `sync.RWMutex`, `proto.Clone` for safe copies, integer-offset pagination, sorted list results
- **Long-running operations** — All mutating operations return `google.longrunning.Operation` (resolved immediately)
- **Docker support** — Multi-stage `Dockerfile` with `VARIANT` build arg (`dual`/`grpc`/`rest`); non-root user; `docker-compose.yml` for one-command local demos
- **GHCR release images** — Published to `ghcr.io/blackwell-systems/gcp-eventarc-emulator` (dual), `-grpc`, and `-rest` on `v*.*.*` tags; `linux/amd64` + `linux/arm64`
- **SDK demo** (`examples/sdk-demo`) — End-to-end example using the official `cloud.google.com/go/eventarc` and `cloud.google.com/go/eventarc/publishing` SDK clients; proves drop-in compatibility without GCP credentials
- **curl demo** (`examples/demo.sh`) — Shell script demonstrating the full event flow via REST
- **Webhook receiver** (`examples/webhook-receiver`) — Tiny Go HTTP server that pretty-prints incoming CloudEvents including `Authorization` header when present
- **Integration tests** — End-to-end tests via bufconn covering Trigger/Channel/MessageBus/Pipeline CRUD, GoogleChannelConfig, Provider queries, error cases, and full-stack PublishEvents → Router → Dispatcher → HTTP delivery
- **CI pipeline** — GitHub Actions with test matrix (ubuntu/macOS/windows × Go 1.24/1.25), lint, format check, go vet, and build verification for all three server variants
- **Makefile** — Per-variant `build`, `run`, `docker-build` targets; `fmt`, `fmt-check`, `test-race`, `test-integration`, `demo`

### Fixed

- **Delete LRO response type** — All 7 `Delete*` methods return the deleted resource in the LRO response, matching GCP Go SDK expectations (`DeleteTriggerOperation.Wait()` returns `*Trigger`, not `Empty`)
- **Malformed JSON request bodies** — REST gateway returns `{"code":3,"message":"request body is not valid JSON"}` (HTTP 400) instead of leaking raw proto parse internals
- **IAM unreachable returns 503** — Connection-refused errors from the IAM emulator return `codes.Unavailable` (HTTP 503) instead of `codes.FailedPrecondition` (HTTP 400)
- **IAM permissive mode unauthenticated** — Requests with no principal are denied in permissive mode; previously forwarded to the IAM emulator which returned allowed
- **Channel existence validation** — `PublishEvents` returns `NOT_FOUND` when the target channel does not exist; previously silently dropped events
- **Trigger input validation** — `CreateTrigger` returns `INVALID_ARGUMENT` with all field violations together when `destination` or `event_filters` are missing
- **Channel state** — Newly created channels have `State: ACTIVE`
- **`GoogleChannelConfig.update_time`** — Singleton config returns a non-null `update_time`
- **`--log-level debug` had no effect on CRUD** — Debug calls went through unconditional `log.Printf`; now gated through the leveled logger and gRPC interceptor
- **"Ready" log fires after listener confirms serving** — All three binaries use a `readyCh` channel; "Ready" only appears after the listener is confirmed up
- **Startup banner respects `--log-level`** — Banner now uses `lgr.Info` so `--log-level error` suppresses it

[Unreleased]: https://github.com/blackwell-systems/gcp-eventarc-emulator/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/blackwell-systems/gcp-eventarc-emulator/compare/v0.1.0...v0.1.2
[0.1.0]: https://github.com/blackwell-systems/gcp-eventarc-emulator/releases/tag/v0.1.0
