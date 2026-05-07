# Lead: Notice taxonomy

## Findings

### Current notice system structure

The notice system lives in `internal/notices/notices.go`. The `Notice` struct has these fields:

- `Tool` — the tool name (also the storage key: `$TSUKU_HOME/notices/<tool>.json`)
- `AttemptedVersion` — version being installed or updated
- `Error` — empty string means success, non-empty means failure
- `Timestamp` — when the notice was written
- `Shown` — cleared (false) = pending display; set (true) = already surfaced or suppressed
- `ConsecutiveFailures` — defined in the struct with a comment promising suppression below 3, but the field is never read or written anywhere in the codebase. The logic is documented, not implemented.
- `Kind` — two values exist: `""` (zero value = `KindUpdateResult`, legacy) and `"auto_apply_result"` (`KindAutoApplyResult`). Kind is written only by `internal/updates/apply.go` (auto-apply failures). Manual update success notices in `cmd/tsuku/update.go` don't set Kind at all.

One notice file per tool. A new failure overwrites the previous notice for the same tool; success notices from manual update also overwrite whatever existed.

### Current inbox-vs-transient split

What goes to the inbox (written to `$TSUKU_HOME/notices/`):

| Event | Written by | Kind | Lifecycle |
|-------|-----------|------|-----------|
| Auto-apply failure | `internal/updates/apply.go` | `auto_apply_result` | Persists; `Shown` stays false until `renderUnshownNotices` marks it |
| Manual update success (version changed) | `cmd/tsuku/update.go` | `""` | Single-view: removed from disk by `cmd_notices.go` when user runs `tsuku notices` |
| Manual `--all` update success (version changed) | `cmd/tsuku/update.go` (runUpdateAll) | `""` | Same as above |
| Self-update success | `internal/updates/self.go` | `""` | Single-view: `renderUnshownNotices` in `notify.go` shows then marks Shown=true; removal happens when? (see surprises) |
| clearAndRecordInstallSuccess (install replaces failure) | `cmd/tsuku/install_deps.go` | `""` | Single-view: written only when a prior failure notice existed |

What is currently transient (printed inline, no inbox):

| Event | Where it prints | Channel |
|-------|----------------|---------|
| Auto-apply success (inline, same run) | `notify.go:DisplayNotifications` step 1 | `os.Stderr`, not inbox |
| "Tool is already at latest version" | `update.go:updateOutcomeMessage`, `reporter.Log` | TTY reporter, not inbox |
| PATH export hint | `install_deps.go:639`, `reporter.DeferWarn` | Deferred stderr, not inbox |
| Shell init changed warning | `update.go:warnShellInitChanges`, `fmt.Fprintf(os.Stderr...)` | Direct stderr, not inbox |
| Recipe validation warnings | `install_deps.go`, `reporter.Warn` | TTY reporter, not inbox |
| Version resolution fallback to 'dev' | `install_deps.go:91`, `reporter.Warn` | TTY reporter, not inbox |
| Checksum dynamic note | `install_deps.go`, `reporter.Log` | TTY reporter, not inbox |

### Lifecycle semantics for failure vs. success notices

Failure notices (Error != ""):
- Written on auto-apply failure in `apply.go`
- `Shown` starts false; `renderUnshownNotices` displays then marks Shown=true on every invocation until the user clears it
- **Never auto-deleted** — they persist until `tsuku notices`, `tsuku update`, `tsuku install`, or `tsuku remove` explicitly calls `RemoveNotice`
- `cmd_notices.go` does NOT delete failure notices; it only deletes success notices (Error == "")
- This means failure notices are effectively persistent errors — they stay in the inbox until the tool status changes

Success notices (Error == ""):
- `cmd_notices.go` removes them after display ("Cleared after viewing")
- `renderUnshownNotices` marks them Shown=true but does NOT remove them — that only happens via `tsuku notices`
- Self-update success: `renderUnshownNotices` marks Shown=true, but the file stays on disk until something calls RemoveNotice. Looking at the code, nothing removes a self-update success notice after marking it shown. This is a gap.

### The Kind field

`Kind` currently has two values. It's written only in one place (`apply.go`) and displayed nowhere in the `tsuku notices` table (the column isn't present in `cmd_notices.go`). It exists as metadata but drives no UI behavior. The `renderUnshownNotices` function in `notify.go` branches on `n.Tool == SelfToolName` and `n.Error == ""` — it does not branch on `Kind`.

### Version fallback event (new, not yet in inbox)

The "version fallback" event described in the lead (resolver skips a release with no asset, falls back to previous) does not exist in the current version resolver. The GitHub provider in `internal/version/provider_github.go` falls back to the first version when no stable version is found (line 152: "Fallback to first version if no stable version found") but this is a stability filter fallback, not an asset-presence fallback. The asset matching in `internal/version/assets.go` returns an error when no asset matches (`formatNoMatchError`) — it does not silently skip to the previous version. There is no version-fallback-due-to-missing-asset path in the current codebase.

### Shell init change warning

`warnShellInitChanges` in `cmd/tsuku/update.go` writes directly to `os.Stderr` with `fmt.Fprintf`. It bypasses the reporter and never touches the inbox. The message is: `"Warning: shell init changed for %s (%s)\n"`. This fires during `tsuku update` (manual) and `tsuku update --all`. It does NOT fire during auto-apply because `apply.go` calls the install func without post-update shell diff detection.

### PATH export hint

`reporter.DeferWarn("To use the installed tool, add this to your shell profile:\n  export PATH=\"%s:$PATH\"", cfg.CurrentDir)` at `install_deps.go:639`. Fires only on explicit, top-level installs (`isExplicit && parent == ""`) and uses `DeferWarn` — it prints after the spinner clears, not to the inbox.

### Available-update summary (non-inbox)

When `auto_apply` is disabled, `renderAvailableSummary` in `notify.go` prints a count-based summary (e.g., "3 updates available. Run 'tsuku update' to apply.") using a sentinel file for deduplication. This is also transient — not inbox-persisted.

### ConsecutiveFailures gap

The field has a documented contract ("suppressed below 3 failures") but is never incremented, read, or acted on. Any auto-apply failure immediately writes a notice with `ConsecutiveFailures: 0`, which by the stated contract should be suppressed. Since the suppression logic isn't implemented, all failures surface immediately.

---

## Proposed taxonomy

### Persistent errors (stay in inbox until tool state changes)

| Event | Rationale |
|-------|-----------|
| Auto-apply failure | User can't see it happen; only inbox delivery works. Stays until tool updates, rolls back, or removes. |
| Self-update failure (if/when tracked) | Background process; user must know. Should persist until `tsuku self-update` succeeds. |
| Version fallback (if implemented) | Silent degradation that affects which binary is running. Warrants persistence so the user sees it next session. |

Persistent means: `Shown` starts false; each `PersistentPreRun` marks shown but does NOT delete. Only a corrective action (successful update, rollback, remove) deletes. The `cmd_notices.go` display leaves failure notices on disk.

### Single-view notifications (cleared after first viewing)

| Event | Rationale |
|-------|-----------|
| Auto-apply success | User wasn't watching; one confirmation is enough. Already works this way via `tsuku notices`. |
| Manual update success (version changed) | Confirmation record if user missed the TTY output. Currently works this way. |
| Self-update success | Good confirmation that background replacement happened. Currently written as single-view but not reliably deleted (gap). |
| Shell init changed (post-update) | Worth one notice so the user can audit, but not worth repeating. Currently inline only; should move to inbox for background updates. |

### Transient noise (no inbox entry)

| Event | Rationale |
|-------|-----------|
| "Tool is already at latest version" | Informational no-op. Printed inline by reporter and forgotten. |
| PATH export hint | One-time setup message. User either acts or doesn't. Repeating it is noise. Possibly suppress after first install too. |
| Recipe validation warnings | Install-time diagnostics; relevant only during that command. |
| Version resolution fallback to 'dev' | Symptom of a recipe issue, not user action needed. Recipe author concern. |
| Available-update summary (count) | Aggregate nudge, not per-tool state. Already handled by sentinel deduplication. |
| Out-of-channel notifications | Throttled already; not inbox material. |

---

### Fields needed to support the taxonomy

The current `Notice` struct supports the taxonomy with one gap: there's no way to distinguish "persistent error" from "single-view notification" by type — the distinction is encoded by convention (`Error != ""` = persistent, `Error == ""` = single-view). This works but is fragile.

To make the taxonomy explicit, the `Kind` field could be extended:

```
KindUpdateResult    = ""               // legacy, backward compat
KindAutoApplyResult = "auto_apply_result"
// proposed additions:
KindSelfUpdate      = "self_update"
KindShellInitChange = "shell_init_change"
KindVersionFallback = "version_fallback"
```

And a `Severity` or `Lifecycle` field could encode "persistent" vs. "single-view" explicitly, rather than deriving it from `Error`. Alternatively, the `Kind` field alone is sufficient if lifecycle rules are defined per-Kind in the display/deletion code.

The `ConsecutiveFailures` field exists for the "flap suppression" use case (don't surface a failure on the first transient network error). To implement it, `apply.go` would need to read the existing notice before writing, increment the counter, and set `Shown = true` when `ConsecutiveFailures < 3`.

---

## Implications

1. The persistent/single-view split is already partially implemented but governed by convention (`Error` field), not an explicit Kind-based dispatch. Formalizing the taxonomy means adding Kind values and routing display/deletion logic through Kind rather than through `Error != ""`.

2. Shell init changes are currently inline-only and would be missed during auto-apply. If this event is inbox-worthy (proposed: single-view), `apply.go` needs to check for shell init diffs post-install, not just `update.go`.

3. The PATH export hint is already implemented correctly as transient. No change needed.

4. Version fallback (as described in the lead) does not exist yet. Implementing it requires changes to the version resolver and a decision on how to surface it — the taxonomy proposes inbox as single-view.

5. The `ConsecutiveFailures` field was designed for flap suppression but is dead code. Implementing it would reduce noise for transient network failures while still surfacing genuine persistent errors after 3 attempts.

6. Self-update success notices leak on disk. `renderUnshownNotices` marks them Shown=true but never removes them. They'll accumulate for each background self-update.

---

## Surprises

1. **ConsecutiveFailures is entirely unimplemented.** The struct comment promises suppression for `< 3` failures, but no code reads or writes the field beyond the struct definition. Every auto-apply failure surfaces immediately.

2. **Self-update success notices are never deleted.** `renderUnshownNotices` marks `Shown=true` and moves on. Nothing downstream removes the file. `cmd_notices.go` only deletes notices where `Error == ""` when the user explicitly runs `tsuku notices`, and self-update notices would accumulate between `tsuku notices` runs.

3. **Kind drives no UI behavior.** The field is set in one place and never used for display routing. It's inert metadata.

4. **Shell init change detection doesn't run during auto-apply.** `warnShellInitChanges` is called only in `cmd/tsuku/update.go`, not in `internal/updates/apply.go`. Users who rely on auto-apply won't see shell init change warnings at all.

5. **The version fallback scenario in the lead doesn't exist yet.** The codebase has no "skip release without platform asset, fall back to previous" logic. This is a proposed feature, not an existing code path.

---

## Open Questions

1. Should `Kind` replace the `Error != ""` convention as the primary lifecycle discriminator? If so, what's the migration path for existing notice files that have no `Kind` field?

2. Where exactly should shell init change detection run for auto-apply? Should `apply.go` call a diff function after each install, and if yes, what data does it need (it doesn't currently have access to old cleanup actions)?

3. What is the correct threshold for `ConsecutiveFailures` before surfacing a notice? The comment says 3, but is that right for all tool types?

4. Should the PATH export hint be suppressed on subsequent installs of the same tool (i.e., upgrades), or always shown on every explicit install? Currently it fires on every explicit top-level install including `tsuku update`.

5. If version fallback is implemented, should it write a notice per-fallback-step (potentially multiple per install if multiple versions are skipped) or a single summary notice with the final resolved version and how far it fell back?

6. Should success notices from `tsuku update` be deleted from disk on `PersistentPreRun` display (same as auto-apply success via inbox), or only on explicit `tsuku notices`? Currently they only clear via `tsuku notices`.

---

## Summary

The current notice system distinguishes persistent errors from single-view notifications by convention (`Error != ""`), not by explicit type, and the `Kind` field exists but drives no behavior. The `ConsecutiveFailures` field is documented but completely unimplemented, meaning all auto-apply failures surface immediately rather than after 3 consecutive failures as the comment intends. The "version fallback" event in the exploration lead has no corresponding code path yet — the version resolver errors out on missing assets rather than silently falling back, so implementing the taxonomy's single-view inbox entry for this event requires new resolver logic first.
