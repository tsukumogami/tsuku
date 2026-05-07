<!-- decision:start id="inbox-reporter-accumulation-and-notice-schema" status="assumed" -->
### Decision: InboxReporter Warning Accumulation and Notice Schema

**Context**

tsuku's `notices/` package stores one JSON file per tool at `$TSUKU_HOME/notices/<tool>.json`. The `Notice` struct currently carries `Tool`, `AttemptedVersion`, `Error`, `Timestamp`, `Shown`, `ConsecutiveFailures`, and `Kind`. The display logic in `renderUnshownNotices` branches on `n.Error == ""` (success vs. failure) and `n.Tool == SelfToolName` (self-update vs. tool update). The `Kind` field exists but drives no behavior.

A new `InboxReporter` must implement the `progress.Reporter` interface so the background `apply-updates` subprocess can route `Warn()`/`DeferWarn()` calls to `notices.WriteNotice()` instead of stderr (which is redirected to `/dev/null` in the background path). The swap point is `cmd_apply_updates.go`, which currently calls `runInstallWithTelemetry`; the change is to call `runInstallWithExternalReporter` with an `InboxReporter`.

During a single install, multiple `Warn()`/`DeferWarn()` calls are realistic: a PATH hint (via `DeferWarn` at install success), a version fallback notice (via `Warn` during plan generation), post-install phase failures, and shell cache rebuild failures. The one-file-per-tool schema means the accumulation strategy determines whether early warnings survive to display.

**Assumptions**

- `Stop()` is always called after install completes. In `runInstallWithTelemetry` it is deferred unconditionally. The `InboxReporter` implementation can treat `Stop()` as the definitive flush-and-write signal. If a caller forgets `Stop()`, warnings are silently lost — this is acceptable because `runInstallWithExternalReporter` callers also call `Stop()` (the pattern is established by `runInstallWithTelemetry`'s defer).
- The PATH hint `DeferWarn` in `installWithDependencies` fires only for explicit installs with `parent == ""`. In the background path (`cmd_apply_updates.go`), `isExplicit` is `false`, so PATH hints are not emitted. The accumulation strategy handles them correctly regardless.
- New `Kind` values introduced by this decision do not need to be consumed by `cmd/tsuku/cmd_notices.go` immediately — the tabular display there renders `Error` and `Timestamp` and does not branch on `Kind`. The `renderUnshownNotices` function in `internal/updates/notify.go` is where Kind-based dispatch must be added.
- "Backward compatible" means existing JSON files with no `kind` field deserialize with `Kind == ""` (the zero value `KindUpdateResult`). The existing display branches for `KindUpdateResult` notices continue to use the `Error != ""` convention. New notices written with an explicit `Kind` use `Kind`-based dispatch.

**Chosen: Flush-on-Stop accumulation**

`InboxReporter` holds all `Warn()` and `DeferWarn()` messages in an in-memory slice. On `Stop()`, it writes a single `Notice` to disk via `notices.WriteNotice()`. The `DeferWarn` queue is flushed into the same slice before the write (order: immediate warns first, then deferred warns in enqueue order). One file write per install run, one notice per tool per run.

The `Notice` struct gains one new field:

```go
// Messages holds warnings accumulated during a single install run.
// Used when Kind is set to a warning-class Kind value (e.g., KindVersionFallback).
// Backward compatible: old files without this field deserialize with nil slice.
Messages []string `json:"messages,omitempty"`
```

`Kind` becomes the lifecycle routing key in `renderUnshownNotices`. New Kind constants:

| Constant | Value | Lifecycle |
|----------|-------|-----------|
| `KindUpdateResult` | `""` | Existing: persistent on error, single-view on success (`Error == ""`) |
| `KindAutoApplyResult` | `"auto_apply_result"` | Existing: persistent on error, single-view on success |
| `KindVersionFallback` | `"version_fallback"` | New: single-view (clear after first display, regardless of `Error`) |
| `KindShellInitChange` | `"shell_init_change"` | New: single-view |

`KindAutoApplySuccess` is not needed as a separate constant. Success on the auto-apply path writes `Kind: KindAutoApplyResult, Error: ""`, which is already handled by `renderUnshownNotices` (`n.Error == ""` branch for non-self-update tools). The existing display path works; only the write (currently missing in `MaybeAutoApply` for the success case) needs to be added.

`InboxReporter.Stop()` implementation sketch:

```go
func (r *InboxReporter) Stop() {
    r.mu.Lock()
    msgs := append(r.immediate, r.deferred...)
    r.immediate = nil
    r.deferred = nil
    r.mu.Unlock()

    if len(msgs) == 0 {
        return
    }
    notice := &notices.Notice{
        Tool:      r.toolName,
        Timestamp: time.Now(),
        Shown:     false,
        Kind:      r.kind,   // set at construction time based on install type
        Messages:  msgs,
    }
    _ = notices.WriteNotice(r.noticesDir, notice)
}
```

`FlushDeferred()` appends deferred messages to the immediate slice rather than writing to disk, since the write is deferred to `Stop()`.

**Rationale**

Flush-on-Stop is the only option that preserves all warnings without dropping earlier ones. Last-warn-wins overwrites silently discard earlier warnings — a version fallback notice followed by a shell-cache rebuild warning would leave only the last message visible, with no indication the first occurred. Severity-gated single write requires maintaining a priority ranking that drifts silently as new Kind values are added.

The approach maps cleanly to how `ttyReporter` already works: it accumulates deferred messages in a slice and flushes them on `FlushDeferred()`. `InboxReporter` applies the same model but treats `Stop()` as the write trigger. `Stop()` is the correct trigger (not `FlushDeferred()`) because the caller's defer runs `Stop()` then `FlushDeferred()` — reversing this by writing on `FlushDeferred()` would miss messages added between `FlushDeferred()` and `Stop()`.

One file write per run is atomic. If two warnings are about the same install, they belong in the same notice — a user checking `tsuku notices` sees the full picture for that tool, not a fragmented series of single-warning notices.

The `Messages []string` schema extension is minimally invasive. It requires no migration: old files without the field deserialize cleanly. The `cmd_notices.go` tabular display can render `Messages` as additional lines below the existing columns without breaking the table structure.

**Alternatives Considered**

- **Last-warn-wins overwrite**: Each `Warn()`/`DeferWarn()` call writes/overwrites the notice file immediately. Simple to implement — no accumulation state, no `Stop()` coordination. Rejected because earlier warnings are silently lost when multiple fire in sequence. In the version fallback + PATH hint scenario, whichever fires second overwrites the first. Silent information loss is worse than the added accumulation complexity.

- **Severity-gated single write**: Write only the highest-priority warning per run, determined by Kind priority ranking (`KindVersionFallback > KindShellInitChange > PATH hint`). Lower-priority warnings are discarded. Rejected because the priority ranking is a maintained invariant that silently breaks when new Kind values are added without updating the ranking. It also discards real information — a PATH hint is actionable even if a version fallback notice is also present.

**Consequences**

- `Notice.Messages` field added. Display code in `renderUnshownNotices` and `cmd_notices.go` must be updated to render multi-message notices.
- `Kind` becomes load-bearing in `renderUnshownNotices`. Display and deletion behavior is routed by `Kind`, with `Error != ""` as a fallback for `Kind == ""` notices (backward compat).
- `InboxReporter` is a new type in `internal/progress/` (or a subpackage). It requires `toolName`, `noticesDir`, and `kind` at construction time. The background path constructs it in `cmd_apply_updates.go`.
- `FlushDeferred()` on `InboxReporter` is a no-op for display (no stderr sink) but transfers deferred messages into the accumulation slice so `Stop()` includes them in the write.
- Single-view Kind values (`KindVersionFallback`, `KindShellInitChange`) are cleared by `renderUnshownNotices` after display via `RemoveNotice` rather than `MarkShown`, matching the success-notice pattern.
- The `ConsecutiveFailures` increment path is not part of this decision but remains unused. It can be addressed separately without affecting the accumulation strategy.
<!-- decision:end -->
