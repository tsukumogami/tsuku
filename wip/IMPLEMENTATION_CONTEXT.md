## Bug

The "Update Dashboard" workflow on main fails after merging PR #1957 with:

```
error: load queue: queue entry 51 (a2ps): invalid queue entry: invalid status "generated"
```

Affected workflow runs: 22552866703, 22552941330, 22552946714, 22553067099.

The last successful run was on commit `44a8e84c50f` (before PR #1957 merged). All runs since `1d13c5a38` (the squash merge of #1957) fail.

## Root cause

PR #1957 bulk-updated `data/queues/priority-queue.json`, changing 1227 queue entries from `"pending"` to `"generated"`. The `"generated"` status does not exist in the valid status set defined in `internal/batch/queue_entry.go`:

```go
var validStatuses = map[string]bool{
    StatusPending:        true, // "pending"
    StatusSuccess:        true, // "success"
    StatusFailed:         true, // "failed"
    StatusBlocked:        true, // "blocked"
    StatusRequiresManual: true, // "requires_manual"
    StatusExcluded:       true, // "excluded"
}
```

The dashboard's `loadQueue()` function (`internal/dashboard/dashboard.go`, lines 226-230) validates every entry on load and fails on the first invalid one. The same validation runs in `cmd/queue-maintain/main.go`.

## What needs to change

Either:

1. **Add `"generated"` as a valid status** in `internal/batch/queue_entry.go` (new constant + entry in `validStatuses`), plus update consumers that switch on status values (`loadCurated` in dashboard.go, `computeQueueStatus`, and any orchestrator/requeue code that filters by status).

2. **Revert the 1227 entries** back to a valid status (e.g. `"pending"` or `"success"`) if `"generated"` was unintentional.

Option 1 is the better path if "generated" represents a real intermediate state (recipe generated but not yet merged), which fills a gap between `"pending"` and `"success"`.

## Files involved

- `internal/batch/queue_entry.go` -- status constants and validation
- `internal/dashboard/dashboard.go` -- `loadQueue()` and `computeQueueStatus()`
- `data/queues/priority-queue.json` -- 1227 entries with invalid status
