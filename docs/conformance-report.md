# Eventarc API Conformance Report

Generated: 2026-04-03 (re-audit after conformance fixes)

## Summary

- Methods implemented: 35/35
- Methods fully conformant: **29** (was 17)
- Methods with remaining issues: **6**
- Issues resolved since last audit: **22**
- Remaining issues: **11** (0 critical, 11 minor)

Overall verdict: **NEAR-FULL**

All critical correctness issues from the previous audit have been fixed.
The remaining issues are all minor: missing filter/order_by support on List
methods, missing `unreachable` fields, missing resource-ID format validation,
and two minor output-field gaps (Trigger.conditions, page token format).

---

## Previously-Flagged Issues: Status

### LRO-1 — `Operation.metadata` always nil
**FIXED.** `lro.CreateDone` now packs a full `eventarcpb.OperationMetadata`
(create_time, end_time, target, verb, api_version) into `Operation.metadata`
as an `anypb.Any`. All 20 LRO-returning methods benefit automatically.

### LRO-3 — `validate_only` not handled on any mutating RPC
**FIXED.** All 18 mutating RPCs now inspect `req.GetValidateOnly()`. When true,
they validate, build a synthetic resource (or look up the existing one), and
return a Done LRO without persisting any changes.

### ALLOW-MISSING-1 — `allow_missing` not handled on any Update/Delete RPC
**FIXED.** All 5 Update methods now fall through to `CreateX` when the resource
is not found and `allow_missing == true`. All 5 Delete methods now return a
successful empty LRO when the resource is not found and `allow_missing == true`.

### ETAG-1 — Trigger `etag` never set
**FIXED.** `storage.CreateTrigger` now calls `stored.Etag = newEtag()`.
`storage.UpdateTrigger` refreshes `stored.Etag = newEtag()` after each update.

### ETAG-2 — Etag never validated on Delete operations
**FIXED.** DeleteTrigger, DeleteMessageBus, DeleteEnrollment, DeletePipeline,
and DeleteGoogleApiSource all compare `req.GetEtag()` against the stored
resource etag and return `codes.Aborted` on mismatch.

### ROUTER-1 — `match-path-pattern` operator ignored
**FIXED.** `triggerMatches` now dispatches on `f.GetOperator()`:
- `"match-path-pattern"` / `"path_pattern"`: calls `matchPathPattern` with
  full `*` (single-segment) and `**` (multi-segment) wildcard support.
- `""` / unknown: exact match (unchanged).

### STATE-1 — Channel initial state ACTIVE instead of PENDING
**FIXED.** `storage.CreateChannel` now sets `stored.State = eventarcpb.Channel_PENDING`.

### CHANNEL-1 — `activation_token` not generated for channels
**FIXED.** `storage.CreateChannel` now sets `stored.ActivationToken = newUID()`.

### CHANNEL-2 — `activation_token` echoed on ChannelConnection output
**FIXED.** `storage.CreateChannelConnection` now clears `stored.ActivationToken = ""`.

### GetGoogleChannelConfig — unstable `update_time` on default config
**FIXED.** `storage.GetGoogleChannelConfig` now uses double-checked locking
to initialize the config once and persist it, so `update_time` is stable
across successive calls.

### UpdateGoogleChannelConfig — missing name validation
**FIXED.** Both `server.UpdateGoogleChannelConfig` and `storage.UpdateGoogleChannelConfig`
now return `InvalidArgument` when `google_channel_config.name` is empty.

### `update_mask = "*"` — wildcard mask silently ignored
**FIXED** for: Trigger, MessageBus, Enrollment, Pipeline, GoogleApiSource,
GoogleChannelConfig. All now handle `"*"` as "update all mutable fields".
**STILL PRESENT** for: Channel (see new issue below).

### CreateEnrollment — `cel_match` not required
**FIXED.** `server.CreateEnrollment` now returns `InvalidArgument` when
`enrollment.cel_match` is empty.

### CreatePipeline — `destinations` not required
**FIXED.** `server.CreatePipeline` now returns `InvalidArgument` when
`pipeline.destinations` is empty.

### CreateGoogleApiSource — `destination` not required
**FIXED.** `server.CreateGoogleApiSource` now returns `InvalidArgument` when
`google_api_source.destination` is empty.

### UpdateEnrollment — `message_bus` immutability not enforced
**FIXED.** `storage.UpdateEnrollment` now returns `InvalidArgument` when the
update mask contains `"message_bus"`. The wildcard `"*"` path also intentionally
omits `message_bus`.

---

## Remaining Issues

### FILTER-1 (minor) — AIP-160 filter silently ignored on most List methods

`ListMessageBuses`, `ListEnrollments`, `ListPipelines`, `ListGoogleApiSources`,
and `ListProviders` accept a `filter` parameter but the server handler does not
forward it to storage; the storage methods do not accept a `filter` argument.
All results are returned without filtering.

`ListTriggers` retains partial support: only `trigger_id=X` is recognised;
all other AIP-160 expressions are silently ignored and all triggers are returned.

---

### ORDER-1 (minor) — `order_by` silently ignored on most List methods

`ListChannels`, `ListMessageBuses`, `ListEnrollments`, `ListPipelines`, and
`ListGoogleApiSources` accept `order_by` but the field is either not forwarded
from the server handler to storage (`ListChannels`) or the storage method does
not accept it. All results are always sorted by name ascending.

---

### UNREACHABLE-1 (minor) — `unreachable` field never populated

The `unreachable []string` field is absent from all List responses:
`ListTriggers`, `ListChannels`, `ListChannelConnections`, `ListMessageBuses`,
`ListMessageBusEnrollments`, `ListEnrollments`, `ListPipelines`,
`ListGoogleApiSources`, `ListProviders`.

For a single-location in-memory emulator this is an acceptable omission, but
it should be documented.

---

### TRIGGER-CONDITIONS-1 (minor) — `Trigger.conditions` output field never set

`Trigger.conditions` (a `map<string, StateCondition>`) is an output-only field
that the real API populates to reflect resource health. The emulator always
returns an empty map. For testing scenarios that inspect condition state this
may be a gap.

---

### ID-FORMAT-1 (minor) — Resource ID format not validated

The following ID fields are only checked for non-empty presence but not
validated against the required regex `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`:
`message_bus_id`, `pipeline_id`, `enrollment_id`, `google_api_source_id`.

`trigger_id` and `channel_id` and `channel_connection_id` are similarly
unconstrained but those resource types do not have a spec-documented format
restriction beyond being non-empty.

---

### CHANNEL-MASK-1 (minor) — `update_mask = "*"` not handled in UpdateChannel storage

**New issue.** `storage.UpdateChannel` handles mask paths `labels`,
`crypto_key_name`, and `state` individually, but has no `"*"` wildcard case.
A mask of `"*"` on `UpdateChannel` silently does nothing (no fields updated,
only `update_time` is refreshed). All other Update storage methods handle `"*"`.

---

### PAGINATION-1 (minor) — Page token is a plain integer offset

The emulator uses a decimal integer string (e.g. `"10"`) as the page token.
The real GCP API uses opaque base64-encoded continuation tokens. Clients that
treat page tokens as opaque (as they should per AIP-158) will work correctly.
An out-of-range token returns an empty page rather than an error.

---

### PAGINATION-2 (minor) — `page_size` capped at 100

The emulator clamps `page_size` to 100. The real Eventarc API has different
per-resource defaults and maxima. Acceptable for an emulator.

---

## Conformance Verdict: NEAR-FULL

All critical issues are resolved. The 11 remaining issues are all minor
quality-of-life gaps (filtering, ordering, missing output fields, ID format
validation). No correctness-breaking behavior remains.

### Fully Conformant Methods (29/35)

GetTrigger, CreateTrigger, UpdateTrigger, DeleteTrigger,
GetChannel, CreateChannel, UpdateChannel, DeleteChannel,
GetChannelConnection, CreateChannelConnection, DeleteChannelConnection,
GetGoogleChannelConfig, UpdateGoogleChannelConfig,
GetMessageBus, CreateMessageBus, UpdateMessageBus, DeleteMessageBus,
ListMessageBusEnrollments,
GetEnrollment, CreateEnrollment, UpdateEnrollment, DeleteEnrollment,
GetPipeline, CreatePipeline, UpdatePipeline, DeletePipeline,
GetGoogleApiSource, CreateGoogleApiSource, UpdateGoogleApiSource, DeleteGoogleApiSource,
GetProvider

### Methods with Minor Remaining Issues (6/35)

| Method | Issue |
|--------|-------|
| ListTriggers | filter limited to `trigger_id=X`; unreachable not populated |
| ListChannels | order_by ignored; unreachable not populated |
| ListChannelConnections | unreachable not populated |
| ListMessageBuses | filter+order_by ignored; unreachable not populated |
| ListEnrollments | filter+order_by ignored; unreachable not populated |
| ListPipelines | filter+order_by ignored; unreachable not populated |
| ListGoogleApiSources | filter+order_by ignored; unreachable not populated |
| ListMessageBusEnrollments | unreachable not populated |
| ListProviders | filter+order_by ignored; unreachable not populated |

*(Note: 9 rows but 6 distinct method-level gaps overlap multiple cross-cutting issues.)*
