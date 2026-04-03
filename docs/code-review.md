# Code Review — GCP Eventarc Emulator

Generated: 2026-04-02

---

## Bugs & Correctness

### [MEDIUM] HTTP server error silently dropped in goroutine
**File:** `internal/gateway/gateway.go` (around the `gw.Start()` goroutine)

The HTTP server's `ListenAndServe` error is either marked `//nolint:errcheck` or logged via `log.Fatalf` inside a goroutine that can exit without the process noticing. If the HTTP server fails to bind or crashes mid-run, the process may continue with only the gRPC server active and no indication to the operator.

**Suggested fix:** Use `log.Fatalf` (already done in cmd mains) or propagate via an error channel back to the main goroutine.

---

## Code Duplication

### [HIGH] Pagination logic — 9 identical occurrences
**Files:** `internal/storage/storage.go`, `storage_channel.go`, `storage_pipeline.go`, `storage_messagebus.go` (and others)

Every `List*` method contains the same ~25-line block:
```go
offset := 0
if req.GetPageToken() != "" {
    n, err := strconv.Atoi(req.GetPageToken())
    if err != nil || n < 0 {
        return nil, status.Errorf(codes.InvalidArgument, "invalid page_token")
    }
    offset = n
}
pageSize := int(req.GetPageSize())
if pageSize <= 0 || pageSize > 100 {
    pageSize = 100
}
end := offset + pageSize
if end > len(items) {
    end = len(items)
}
nextToken := ""
if end < len(items) {
    nextToken = strconv.Itoa(end)
}
```

**Proposed abstraction:**
```go
// internal/storage/pagination.go
func PaginatePage[T any](items []T, pageToken string, pageSize int32) (page []T, nextToken string, err error) {
    offset := 0
    if pageToken != "" {
        offset, err = strconv.Atoi(pageToken)
        if err != nil || offset < 0 {
            return nil, "", status.Errorf(codes.InvalidArgument, "invalid page_token")
        }
    }
    size := int(pageSize)
    if size <= 0 || size > 100 {
        size = 100
    }
    end := offset + size
    if end > len(items) {
        end = len(items)
    }
    if end < len(items) {
        nextToken = strconv.Itoa(end)
    }
    return items[offset:end], nextToken, nil
}
```

### [HIGH] Clone functions — 13 near-identical functions
**Files:** All `internal/storage/storage_*.go` files

Each storage file defines a dedicated clone function:
```go
func cloneTrigger(t *eventarcpb.Trigger) *eventarcpb.Trigger {
    return proto.Clone(t).(*eventarcpb.Trigger)
}
```

13 variants of this single-line wrapper exist. With Go generics:
```go
// internal/storage/clone.go
func cloneProto[T proto.Message](m T) T {
    return proto.Clone(m).(T)
}
```

Eliminates all 13 dedicated clone functions.

### [MEDIUM] UID / etag generation — ~15 occurrences
**Files:** Multiple storage files

`fmt.Sprintf("%x", rand.Uint64())` appears ~15 times for generating UIDs and etags. Extract into:
```go
func newUID() string  { return fmt.Sprintf("%x", rand.Uint64()) }
func newEtag() string { return fmt.Sprintf("%x", rand.Uint64()) }
```

### [MEDIUM] Delete pattern — 7 identical methods in `internal/server/server.go`
All 7 `Delete*` methods follow the exact same 5-step pattern:
1. Parse parent from name
2. Get resource (to return in LRO)
3. Delete resource from storage
4. `s.lro.CreateDone(parent, resource)`
5. Return LRO

The only variation is the resource type and the storage method called. Could be reduced with a generic helper or at minimum a comment noting the pattern.

### [LOW] Duplicate `getEnv` / `getEnvInt` / `getEnvPort` helpers
**Files:** `cmd/server/main.go`, `cmd/server-rest/main.go`, `cmd/server-dual/main.go`

All three cmd mains define identical private helper functions. These could live in a shared `internal/cmdutil` package, or the flag defaults could be computed via a shared `config` struct. Low priority since cmd packages are intentionally self-contained.

---

## Abstraction Opportunities

### Required-field validation (~50 callsites in `server.go`)
Every RPC validates required fields with the same pattern:
```go
if req.GetName() == "" {
    return nil, status.Errorf(codes.InvalidArgument, "name is required")
}
```

**Proposed helper:**
```go
func requireField(value, fieldName string) error {
    if value == "" {
        return status.Errorf(codes.InvalidArgument, "%s is required", fieldName)
    }
    return nil
}
```

Reduces 50 if-blocks to single `requireField` calls.

### Parent extraction inconsistency
`parentFromName()` and `parentFromResource()` coexist in `server.go`, plus `parentFromChannel()` and `parentFromChannelConnection()` in `publisher.go` — all doing string splitting. Unify into one `extractParent(name string) string` helper in a shared util.

---

## Dead Code

### `NormalizeTriggerResource()` — `internal/authz/permissions.go`
Exported function that appears to have no callers. If it was used by an older IAM integration that was replaced, it can be removed. Verify with `grep -r NormalizeTriggerResource .` before deleting.

---

## Design Inconsistencies

### IAM permission strings hardcoded (~50 sites) vs. constants in `authz/permissions.go`
`server.go` passes raw string literals like `"eventarc.triggers.get"` to `checkPermission`. The `authz` package already defines these as constants/mappings. Server code should reference the constants rather than re-defining strings inline — prevents typo bugs and makes permission audits easier.

### Mixed logger usage at startup
All three `cmd/*/main.go` files mix:
```go
log.Printf("GCP Eventarc Emulator v%s", version)  // stdlib, always prints
lgr.Info("Log level: %s", *logLevel)               // leveled logger
```

Startup lines using `log.Printf` bypass the leveled logger, so they always appear even at `--log-level error`. Standardize to `lgr.Info(...)` throughout, or accept that startup banner always appears (which is reasonable — but should be intentional).

### Duplicate nil-logger guard pattern
`internal/router`, `internal/dispatcher`, and `internal/publisher` each implement the same optional-logger pattern:
```go
if lgr == nil {
    lgr = logger.New("info")
}
```

This could be a `logger.OrDefault(lgr)` helper:
```go
func OrDefault(lgr *Logger) *Logger {
    if lgr != nil { return lgr }
    return New("info")
}
```

### Inconsistent "Ready" log placement
- `cmd/server-dual`: logs "Ready" only after `readyCh` is closed (correct — gRPC confirmed serving)
- `cmd/server-rest`: logs "Ready" inside the HTTP goroutine (fires before HTTP is actually accepting, races with listener bind)
- `cmd/server`: logs "Ready" before calling `grpcServer.Serve()` (fires before gRPC accepts connections)

Standardize: all servers should log "Ready" only after the listener confirms it is serving.

---

## Summary

| Category | Count | Priority |
|---|---|---|
| Bugs / correctness | 1 | Medium |
| Code duplication | 5 clusters | High |
| Abstraction opportunities | 3 | Medium |
| Dead code | 1 | Low |
| Design inconsistencies | 4 | Medium |

**Highest-value refactors (bang for buck):**
1. Generic `PaginatePage[T]` helper — eliminates 9 copy-paste blocks (~2 hrs)
2. Generic `cloneProto[T]` helper — eliminates 13 wrapper functions (~1 hr)
3. `requireField` validation helper — removes ~50 if-blocks (~1.5 hrs)
4. Standardize `log.Printf` → `lgr.*` at startup — trivial, fixes log-level correctness (~30 min)
5. `logger.OrDefault` helper — removes duplicate nil-checks (~30 min)

**Code quality highlights (already good):**
- Thread-safety with `sync.RWMutex` throughout storage layer
- Consistent `proto.Clone()` usage prevents aliasing bugs
- Clear separation of concerns across packages
- Comprehensive gRPC status codes with proper error details
