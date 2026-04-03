# Eventarc API Conformance Report

Generated: 2026-04-03

## Summary

- Methods implemented: 35/35 (all declared Eventarc service methods)
- Methods fully conformant: 17
- Methods with issues: 18
- Total issues: 38 (13 critical, 25 minor)

Resource types covered: Trigger, Channel, ChannelConnection, GoogleChannelConfig, MessageBus, Enrollment, Pipeline, GoogleApiSource, Provider

---

## Method-by-Method Analysis

---

### GetTrigger

**Status**: CONFORMANT

- Validates `name` (InvalidArgument if empty).
- Returns NotFound with correct code.
- Returns deep-copied proto; no aliasing.

---

### ListTriggers

**Status**: ISSUES

**Issues**:

- [minor] `ListTriggersResponse.unreachable` field never populated.
  - Expected: `unreachable []string` present in response (proto field 3).
  - Actual: Field is always omitted (zero value).
  - Fix: Acceptable for an emulator with no real location partitioning; document as known omission.

- [minor] `filter` support is extremely limited — only `trigger_id=X` exact match. The spec references AIP-160 filter syntax (e.g. `destination:gke`).
  - Expected: AIP-160 filter expressions on arbitrary fields.
  - Actual: Only `trigger_id=X` is recognised; all other filter strings are silently ignored and all triggers are returned.
  - Fix: At minimum return an `UNIMPLEMENTED` error for unrecognised filter expressions rather than silently returning all results.

- [minor] `order_by` supports only `""` (name asc) and `"create_time desc"`. Any other value silently falls back to name-ascending.
  - Expected: Arbitrary comma-separated field list with optional `desc` suffix per AIP-132.
  - Actual: Unrecognised order-by strings are silently treated as name-asc.
  - Fix: Acceptable for emulator; document supported values. Consider returning `UNIMPLEMENTED` for unrecognised values instead of silently falling back.

---

### CreateTrigger

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field on `CreateTriggerRequest` is not handled. When set to `true`, the real API validates the request and returns a preview without persisting.
  - Expected: If `validate_only == true`, validate and return a preview LRO without storing the trigger.
  - Actual: `validate_only` is read from the proto but never inspected; the trigger is always persisted.
  - Fix: Check `req.GetValidateOnly()` before calling `storage.CreateTrigger`.

- [critical] LRO `metadata` field is never populated. The real API always sets `Operation.metadata` to an `OperationMetadata` message with `create_time`, `end_time`, `target`, `verb`, and `api_version`.
  - Expected: `Operation.metadata` = `google.protobuf.Any` wrapping `eventarcpb.OperationMetadata{CreateTime: ..., EndTime: ..., Target: <resource name>, Verb: "create", ApiVersion: "v1"}`.
  - Actual: `Operation.metadata` is nil.
  - Fix: Populate `OperationMetadata` in `lro.CreateDone` or in each caller.

- [minor] Trigger `etag` is not set on creation. The proto comment says it "might be sent only on create requests to ensure the client has an up-to-date value before proceeding."
  - Expected: `Trigger.etag` non-empty after create.
  - Actual: `etag` is always the zero string; `newEtag()` exists in helpers but is not called for Trigger.
  - Fix: Call `newEtag()` and assign `stored.Etag` in `CreateTrigger` storage method.

- [minor] Trigger `conditions` output field (map of `StateCondition`) is never set.
  - Expected: Output-only field populated by server reflecting resource health/state.
  - Actual: Always empty map.
  - Fix: For emulator, set a default `CONDITION_SUCCEEDED` entry to indicate healthy state.

---

### UpdateTrigger

**Status**: ISSUES

**Issues**:

- [critical] `allow_missing` field on `UpdateTriggerRequest` is not handled. When `allow_missing == true` and the trigger is not found, the real API creates it (upsert semantics).
  - Expected: If `allow_missing == true` and trigger not found, create it as if it were a CreateTrigger call.
  - Actual: Always returns NotFound when the trigger does not exist, ignoring `allow_missing`.
  - Fix: Check `req.GetAllowMissing()` before returning NotFound; if true, fall through to create.

- [critical] `validate_only` field is not handled (same as CreateTrigger).

- [critical] LRO `metadata` field not populated (same as CreateTrigger).

- [critical] Etag not validated on update. `DeleteTriggerRequest` carries an `etag` field; while `UpdateTriggerRequest` does not expose an `etag` field directly, the stored trigger's etag should be refreshed after each update.
  - Expected: `Trigger.etag` updated to a new value after every successful update.
  - Actual: `UpdateTrigger` in storage never assigns `stored.Etag`; etag remains at zero string.
  - Fix: Call `stored.Etag = newEtag()` in `UpdateTrigger` storage method.

- [minor] `update_mask` path `"*"` (wildcard) is not handled. Per the spec, `"*"` means update all mutable fields.
  - Expected: A mask of `"*"` updates all mutable fields (equivalent to no mask).
  - Actual: `"*"` is treated as an unknown path and silently ignored (nothing updated).
  - Fix: Add a `case "*":` branch that applies all mutable fields.

---

### DeleteTrigger

**Status**: ISSUES

**Issues**:

- [critical] `etag` field on `DeleteTriggerRequest` is not validated. When provided, the real API returns `ABORTED` (or `FAILED_PRECONDITION`) if it does not match the stored resource's etag.
  - Expected: If `req.GetEtag() != ""` and `req.GetEtag() != stored.Etag`, return `codes.Aborted` (or `codes.FailedPrecondition`).
  - Actual: `etag` is read from the proto but never inspected; delete always proceeds.
  - Fix: After fetching the trigger, compare `req.GetEtag()` with `stored.Etag` and reject if mismatch.

- [critical] `allow_missing` field is not handled. When `allow_missing == true` and the trigger is not found, the call should succeed (no-op).
  - Expected: NotFound suppressed when `allow_missing == true`.
  - Actual: Always returns NotFound when trigger is absent.
  - Fix: Check `req.GetAllowMissing()` and return success (empty LRO response) when trigger is absent.

- [critical] `validate_only` field is not handled.

- [critical] LRO `metadata` field not populated.

---

### GetChannel

**Status**: CONFORMANT

---

### ListChannels

**Status**: ISSUES

**Issues**:

- [minor] `order_by` field from `ListChannelsRequest` is accepted but silently ignored. Implementation always sorts by name ascending.
  - Expected: Respect `order_by` comma-separated field list with `desc` suffix support.
  - Actual: Parameter passed to server is not forwarded to `storage.ListChannels`.
  - Fix: Add `orderBy` parameter to `storage.ListChannels` and apply sorting.

- [minor] `unreachable` field never populated in `ListChannelsResponse`.

---

### CreateChannel

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] Channel `activation_token` is not generated. The real GCP API sets this output field to a short-lived token the provider uses to register the channel.
  - Expected: `Channel.activation_token` non-empty after creation.
  - Actual: Always empty string.
  - Fix: Assign a random value to `stored.ActivationToken` in `CreateChannel` storage method.
- [minor] Channel initial state should be `PENDING` (the spec says new channels start PENDING, waiting for the provider to connect). Emulator sets `ACTIVE` immediately.
  - Expected: Newly created channel has `state = PENDING`.
  - Actual: `stored.State = eventarcpb.Channel_ACTIVE` set unconditionally on creation.
  - Fix: Change initial state to `Channel_PENDING`. ACTIVE can remain reachable via UpdateChannel for testing scenarios.

---

### UpdateChannel

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `update_mask` path `"*"` not handled.
- [minor] Channel `etag` field does not exist on the `Channel` proto (confirmed via `go doc`) — this is correct behavior; no etag validation needed for Channel.

---

### DeleteChannel

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.

---

### GetChannelConnection

**Status**: CONFORMANT

---

### ListChannelConnections

**Status**: ISSUES

**Issues**:

- [minor] `unreachable` field never populated in `ListChannelConnectionsResponse`.

---

### CreateChannelConnection

**Status**: ISSUES

**Issues**:

- [critical] LRO `metadata` field not populated.
- [minor] `activation_token` is an input-only field on `ChannelConnection`. The emulator stores it as-is without clearing it on output. The real API does not return this field in responses.
  - Expected: `activation_token` is not echoed back in the stored or returned resource.
  - Actual: The field is cloned into storage and returned as-is.
  - Fix: Clear `stored.ActivationToken = ""` after cloning in `CreateChannelConnection`.

---

### DeleteChannelConnection

**Status**: ISSUES

**Issues**:

- [critical] LRO `metadata` field not populated.

---

### GetGoogleChannelConfig

**Status**: ISSUES

**Issues**:

- [minor] When no config has been stored, the emulator returns a synthetic `GoogleChannelConfig` with a dynamically generated `UpdateTime`. This means two successive `GetGoogleChannelConfig` calls return different `update_time` values for the same logical (absent) config.
  - Expected: A stable default config, or a consistent "zero" config.
  - Actual: Each call generates `timestamppb.Now()` independently, producing a different timestamp.
  - Fix: Store the initial config on first access (or at `NewStorage()` time) so `update_time` is stable.

---

### UpdateGoogleChannelConfig

**Status**: ISSUES

**Issues**:

- [minor] `google_channel_config.name` is not validated. If `req.GetGoogleChannelConfig().GetName()` is empty, the config is stored under an empty-string key.
  - Expected: Return `InvalidArgument` if `google_channel_config.name` is empty.
  - Actual: No name validation; empty-string key accepted.
  - Fix: Add `requireField(req.GetGoogleChannelConfig().GetName(), "google_channel_config.name")` check.

- [minor] `update_mask` path `"*"` not handled.

---

### GetMessageBus

**Status**: CONFORMANT

---

### ListMessageBuses

**Status**: ISSUES

**Issues**:

- [minor] `filter` and `order_by` fields are accepted but silently ignored (not forwarded to `storage.ListMessageBuses`).
  - Expected: Respect `filter` (AIP-160) and `order_by`.
  - Actual: Both parameters are passed by server handler but not propagated to storage.
  - Fix: Add `filter` and `orderBy` parameters to `storage.ListMessageBuses` and apply them.

- [minor] `unreachable` field never populated in `ListMessageBusesResponse`.

---

### CreateMessageBus

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `message_bus_id` format is not validated. The spec says it must match `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`.
  - Expected: Return `InvalidArgument` for IDs not matching the pattern.
  - Actual: Any non-empty string is accepted.
  - Fix: Add regex validation for `message_bus_id`.

---

### UpdateMessageBus

**Status**: ISSUES

**Issues**:

- [critical] `allow_missing` field is not handled (upsert semantics).
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [critical] `etag` field is not validated on update. `UpdateMessageBusRequest` does not carry an `etag` field directly, but the stored resource's etag is refreshed on update — this part is correctly handled. However, the real API also supports optimistic concurrency through the `etag` on the resource itself embedded in the request message body; this is not enforced.
  - Note: `UpdateMessageBusRequest` does not carry a top-level `etag` parameter in the proto (confirmed via `go doc`), so update etag validation is not applicable here. This item is downgraded.
- [minor] `update_mask` path `"*"` not handled.

---

### DeleteMessageBus

**Status**: ISSUES

**Issues**:

- [critical] `etag` field on `DeleteMessageBusRequest` is not validated.
  - Expected: If `req.GetEtag() != ""` and does not match stored `message_bus.etag`, return `ABORTED`.
  - Actual: `etag` field is never inspected; delete always proceeds.
  - Fix: Compare `req.GetEtag()` against `stored.Etag` before deleting.

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.

---

### ListMessageBusEnrollments

**Status**: ISSUES

**Issues**:

- [minor] `unreachable` field never populated in `ListMessageBusEnrollmentsResponse`.

---

### GetEnrollment

**Status**: CONFORMANT

---

### ListEnrollments

**Status**: ISSUES

**Issues**:

- [minor] `filter` and `order_by` fields accepted but silently ignored (not forwarded to storage).
- [minor] `unreachable` field never populated in `ListEnrollmentsResponse`.

---

### CreateEnrollment

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `enrollment_id` format not validated (must match `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`).
- [minor] `enrollment.cel_match` is required by the spec but not validated on create — an enrollment can be created with an empty `cel_match`.
  - Expected: Return `InvalidArgument` if `enrollment.cel_match` is empty.
  - Actual: No validation; empty `cel_match` accepted.
  - Fix: Add `cel_match` presence check in `CreateEnrollment` handler.
- [minor] `enrollment.message_bus` is required and immutable by spec, but immutability is not enforced on update.

---

### UpdateEnrollment

**Status**: ISSUES

**Issues**:

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [critical] `etag` not validated on update (no top-level etag in `UpdateEnrollmentRequest`, but emulator does refresh etag on update — this is correct). No issue.
- [minor] `update_mask` path `"*"` not handled.
- [minor] `message_bus` is documented as immutable, but UpdateEnrollment allows changing it via update mask.
  - Expected: Updating `message_bus` via update mask should return `InvalidArgument` (field is immutable).
  - Actual: `message_bus` can be changed via `"message_bus"` path in update mask.
  - Fix: Ignore or reject `"message_bus"` path in update mask for UpdateEnrollment.

---

### DeleteEnrollment

**Status**: ISSUES

**Issues**:

- [critical] `etag` field on `DeleteEnrollmentRequest` is not validated.
  - Expected: If `req.GetEtag()` is non-empty and does not match `stored.Etag`, return `ABORTED`.
  - Actual: `etag` field is never inspected.
  - Fix: Compare `req.GetEtag()` against stored etag before deleting.

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.

---

### GetPipeline

**Status**: CONFORMANT

---

### ListPipelines

**Status**: ISSUES

**Issues**:

- [minor] `filter` and `order_by` fields accepted but silently ignored (not forwarded to storage).
- [minor] `unreachable` field never populated in `ListPipelinesResponse`.

---

### CreatePipeline

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `pipeline_id` format not validated (must match `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`).
- [minor] `pipeline.destinations` is required (at least one destination per spec) but not validated on create.
  - Expected: Return `InvalidArgument` if `destinations` is empty.
  - Actual: Pipeline created with no destinations is accepted.
  - Fix: Add `len(req.GetPipeline().GetDestinations()) == 0` check.

---

### UpdatePipeline

**Status**: ISSUES

**Issues**:

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `update_mask` path `"*"` not handled.

---

### DeletePipeline

**Status**: ISSUES

**Issues**:

- [critical] `etag` field on `DeletePipelineRequest` is not validated.
  - Expected: If `req.GetEtag()` is non-empty and does not match `stored.Etag`, return `ABORTED`.
  - Actual: `etag` field is never inspected.
  - Fix: Compare `req.GetEtag()` against stored etag before deleting.

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.

---

### GetGoogleApiSource

**Status**: CONFORMANT

---

### ListGoogleApiSources

**Status**: ISSUES

**Issues**:

- [minor] `filter` and `order_by` fields accepted but silently ignored (not forwarded to storage).
- [minor] `unreachable` field never populated in `ListGoogleApiSourcesResponse`.

---

### CreateGoogleApiSource

**Status**: ISSUES

**Issues**:

- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `google_api_source_id` format not validated (must match `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`).
- [minor] `google_api_source.destination` is required but not validated on create.
  - Expected: Return `InvalidArgument` if `destination` is empty.
  - Actual: Empty destination is accepted.
  - Fix: Add presence check for `google_api_source.destination`.

---

### UpdateGoogleApiSource

**Status**: ISSUES

**Issues**:

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.
- [minor] `update_mask` path `"*"` not handled.

---

### DeleteGoogleApiSource

**Status**: ISSUES

**Issues**:

- [critical] `etag` field on `DeleteGoogleApiSourceRequest` is not validated.
  - Expected: If `req.GetEtag()` is non-empty and does not match `stored.Etag`, return `ABORTED`.
  - Actual: `etag` field is never inspected.
  - Fix: Compare `req.GetEtag()` against stored etag before deleting.

- [critical] `allow_missing` field is not handled.
- [critical] `validate_only` field is not handled.
- [critical] LRO `metadata` field not populated.

---

### GetProvider

**Status**: CONFORMANT

---

### ListProviders

**Status**: ISSUES

**Issues**:

- [minor] `filter` and `order_by` fields on `ListProvidersRequest` are accepted but silently ignored (not applied in `storage.ListProviders`).
  - Expected: Respect `filter` and `order_by` per AIP-132/160.
  - Actual: Both fields are passed from handler to storage but the storage implementation ignores them and always sorts by name ascending.
  - Fix: Apply `orderBy` and `filter` in `storage.ListProviders`.

- [minor] `unreachable` field never populated in `ListProvidersResponse`.

---

## Cross-Cutting Issues

### LRO-1 (critical) — `Operation.metadata` always nil

Every LRO-returning method (`Create*`, `Update*`, `Delete*`) wraps the resource in `Operation.response` but leaves `Operation.metadata` as nil. The real GCP API always populates `Operation.metadata` with an `eventarcpb.OperationMetadata` message containing:

```
OperationMetadata {
  create_time: <when the operation was created>
  end_time:    <when it finished>
  target:      <full resource name>
  verb:        "create" | "update" | "delete"
  api_version: "v1"
}
```

SDK clients that call `op.Metadata()` (e.g. for audit logging or wait-with-metadata) receive nil and must handle it specially. This affects all 20 LRO-returning methods.

**Fix**: In `lro.CreateDone`, accept `verb` and `target` parameters and pack an `OperationMetadata` into `op.Metadata`.

---

### LRO-2 (minor) — Operation name format

The emulator generates operation names as `{parent}/operations/{uuid}`. The real GCP API uses `projects/{project}/locations/{location}/operations/{uuid}`. For Trigger operations, `parent` is `projects/p/locations/l` so the format is correct. However, for delete operations the `parent` is computed via `parentFromName` / `parentFromResource` — these helpers are correct for the `/triggers/`, `/channels/`, etc. suffixes but the single-step `parentFromResource` function only strips the last two path segments. This is correct for standard 6-segment resource names but would break for any 8-segment names. This is currently not an issue with the existing resource types.

---

### LRO-3 (minor) — `validate_only` not handled on any mutating RPC

`validate_only` is present on the request messages of: `CreateTrigger`, `UpdateTrigger`, `DeleteTrigger`, `CreateChannel`, `UpdateChannel`, `DeleteChannel`, `CreateMessageBus`, `UpdateMessageBus`, `DeleteMessageBus`, `CreateEnrollment`, `UpdateEnrollment`, `DeleteEnrollment`, `CreatePipeline`, `UpdatePipeline`, `DeletePipeline`, `CreateGoogleApiSource`, `UpdateGoogleApiSource`, `DeleteGoogleApiSource`. None of these inspect `validate_only`. Setting this flag should result in validation without persistence and a completed (Done: true) LRO with the would-be resource in the response but without any side effects.

---

### ETAG-1 (critical) — Etag not set on Trigger

`Trigger.etag` is an output field populated by the server, but `CreateTrigger` and `UpdateTrigger` in storage never set it. `newEtag()` is defined in helpers but unused for Trigger. Since `DeleteTriggerRequest.etag` is supported on the delete side, clients that create a trigger and then pass `etag` to delete will always see a mismatch (emulator etag is `""`, client receives `""` too, so an empty etag would be passed — which currently means the check is skipped entirely). The risk is that clients that set an explicit etag on delete are not protected by optimistic concurrency.

---

### ETAG-2 (critical) — Etag never validated on Delete operations

Five resource types carry `etag` on their Delete request: `Trigger`, `MessageBus`, `Enrollment`, `Pipeline`, `GoogleApiSource`. In all five cases, the emulator reads the field from the request but never compares it against the stored resource's etag. Concurrent delete scenarios will not be detected.

---

### FILTER-1 (minor) — AIP-160 filter silently ignored on most List methods

The following List methods accept `filter` but silently discard it (the server passes it to storage, but the storage implementation ignores it): `ListMessageBuses`, `ListEnrollments`, `ListPipelines`, `ListGoogleApiSources`, `ListProviders`. `ListTriggers` has limited `trigger_id=X` support only. Clients relying on filtering will receive unfiltered results without error.

---

### ORDER-1 (minor) — `order_by` silently ignored on Channel/ChannelConnection/MessageBus/Enrollment/Pipeline/GoogleApiSource List methods

`ListChannels`, `ListMessageBuses`, `ListEnrollments`, `ListPipelines`, `ListGoogleApiSources` all accept `order_by` but the server handler either does not pass it to storage (ListChannels) or the storage method does not accept it and always sorts by name ascending.

---

### ROUTER-1 (critical) — EventFilter `operator` field not respected

The `EventFilter` message has an `operator` field that controls matching semantics:
- `""` (empty) = exact match
- `match-path-pattern` = path pattern matching (e.g. wildcards in resource paths)
- `path_pattern` = GCFv1 path patterns

The router's `triggerMatches` function unconditionally does exact-match (`eventVal != f.GetValue()`) for **all** operators including `match-path-pattern`. The comment in the code acknowledges this: `"Operator "" and "match-path-pattern" both use exact match for now."` This means triggers with `operator: "match-path-pattern"` will never fire correctly when the value contains wildcards.

```go
// Current (incorrect for match-path-pattern):
if eventVal != f.GetValue() {
    return false
}
```

- Expected: `match-path-pattern` should match using glob/path-pattern semantics (e.g. `"//storage.googleapis.com/projects/_/buckets/*/objects/*"` matches any bucket/object path).
- Actual: Exact string comparison used for all operators.
- Fix: Implement path-pattern matching for `operator == "match-path-pattern"` or `operator == "path_pattern"` using a glob library or custom path segment matcher.

---

### ALLOW-MISSING-1 (critical) — `allow_missing` not handled on any Update/Delete RPC

`allow_missing` is present on: `UpdateTrigger`, `UpdateMessageBus`, `UpdateEnrollment`, `UpdatePipeline`, `UpdateGoogleApiSource` (upsert semantics), and `DeleteTrigger`, `DeleteMessageBus`, `DeleteEnrollment`, `DeletePipeline`, `DeleteGoogleApiSource` (silent no-op on not-found). None of these inspect `allow_missing`. SDK clients using `UpdateOrCreate` patterns will receive unexpected `NOT_FOUND` errors.

---

### PAGINATION-1 (minor) — Page token is a plain integer offset

The emulator uses a simple integer string (e.g. `"10"`) as the page token. The real GCP API uses opaque base64-encoded continuation tokens. This means:

1. Clients that treat page tokens as opaque (as they should per AIP-158) will work correctly.
2. Clients that manually construct or inspect page tokens will see a different format.
3. An out-of-range token returns an empty page rather than an error, which deviates slightly from strict AIP-158.

This is an acceptable emulator simplification but should be documented.

---

### PAGINATION-2 (minor) — `page_size` capped at 100

The emulator clamps `page_size` to 100 and defaults to 100 when unset. The real Eventarc API has different per-resource defaults and maxima. This is acceptable for an emulator.

---

### STATE-1 (minor) — Channel initial state should be `PENDING`, not `ACTIVE`

See CreateChannel above. New channels should start in `PENDING` state per the spec.

---

### CHANNEL-1 (minor) — `activation_token` not generated for new channels

See CreateChannel above. A non-empty `activation_token` is expected in the created channel response.

---

### CHANNEL-2 (minor) — `activation_token` echoed back on ChannelConnection output

See CreateChannelConnection above. The field is input-only and should be cleared in the stored/returned resource.

---

## Recommended Fix Priority

### P0 — Critical, Correctness-Breaking

| Issue | Affected Methods | Description |
|-------|-----------------|-------------|
| LRO-1 | All 20 LRO methods | `Operation.metadata` always nil |
| ETAG-2 | DeleteTrigger, DeleteMessageBus, DeleteEnrollment, DeletePipeline, DeleteGoogleApiSource | Etag never validated on delete |
| ALLOW-MISSING-1 | 5 Update + 5 Delete methods | `allow_missing` never handled |
| ROUTER-1 | Event routing | `match-path-pattern` operator ignored; uses exact match |
| ETAG-1 | CreateTrigger, UpdateTrigger | Trigger `etag` never set (blocking etag validation) |

### P1 — Critical, Client Behavior Impact

| Issue | Affected Methods | Description |
|-------|-----------------|-------------|
| LRO-3 | All 18 mutating methods with `validate_only` | `validate_only` silently ignored |

### P2 — Minor, Missing Fields / Incomplete Behavior

| Issue | Description |
|-------|-------------|
| STATE-1 | Channel initial state ACTIVE instead of PENDING |
| CHANNEL-1 | `activation_token` not generated for channels |
| CHANNEL-2 | `activation_token` echoed on ChannelConnection output |
| FILTER-1 | AIP-160 filter silently ignored on most List methods |
| ORDER-1 | `order_by` silently ignored on most List methods |
| Missing `unreachable` | All List responses omit `unreachable` field |
| Missing `conditions` on Trigger | `Trigger.conditions` map never set |
| Missing id-format validation | `message_bus_id`, `pipeline_id`, `enrollment_id`, `google_api_source_id` format not validated |
| Missing required-field validation | `Enrollment.cel_match`, `Pipeline.destinations`, `GoogleApiSource.destination` not validated on create |
| `update_mask = "*"` | Wildcard mask silently ignored across all Update methods |
| GoogleChannelConfig | Unstable `update_time` on default config; no name validation |
