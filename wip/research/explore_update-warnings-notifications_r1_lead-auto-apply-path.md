# Lead: Auto-apply background path and inbox sink

## Findings

### How the background subprocess is spawned

`trigger.go:MaybeSpawnAutoApply` is the entry point. It checks for pending cache entries, acquires a non-blocking flock to prevent duplicate spawns, then runs `exec.Command(binary, "apply-updates")` via `spawnDetached`. The `spawnDetached` function sets `cmd.Stdin = nil`, `cmd.Stdout = nil`, `cmd.Stderr = nil`, isolates the process group with `SETPGID`, and calls `cmd.Start()` without waiting. The parent process never sees the output of this subprocess.

The background process is a full second tsuku process. It runs `cmd_apply_updates.go`, which immediately redirects its own `os.Stdout` and `os.Stderr` to `/dev/null` at the very start of the command handler. This is a double-silence: the parent discards all output from the subprocess, and the subprocess itself also redirects its own stdio away.

### What `cmd_apply_updates.go` does

The command (`apply-updates`) calls `updates.MaybeAutoApply(cfg, userCfg, nil, installFn, nil)`. It passes `nil` for both `projectCfg` and the telemetry client. Note that the `PersistentPreRun` hook from `main.go` is explicitly skipped for `apply-updates` (it appears in the `skip` map), so no prior notifications are displayed and no further spawning occurs.

### What `MaybeAutoApply` currently writes to notices

`internal/updates/apply.go:MaybeAutoApply` handles two outcomes per tool:

**Success path:**
- Calls `notices.RemoveNotice(noticesDir, entry.Tool)` to clear any prior failure notice for that tool.
- No new notice is written. The success is silently consumed in the background.

**Failure path:**
- Performs auto-rollback (calls `mgr.Activate` to restore previous version).
- Constructs a `notices.Notice` with `Kind: notices.KindAutoApplyResult`, `Shown: false`, and writes it to `$TSUKU_HOME/notices/<toolname>.json`.
- This persists on disk until the tool is successfully updated, rolled back manually, or removed.

### What is silently dropped in the background path

Several events in the background path produce no notice and no output:

1. **Successful updates** -- the only signal is the removal of a prior failure notice. If the user never had a failure notice, `tsuku notices` shows nothing. The success is invisible to the user unless they run `tsuku list` and notice the version changed. (The `renderUnshownNotices` function in `notify.go` does handle the case `n.Error == ""` for tools, printing `"<tool> has been updated to <version>"`, but this requires a notice file with `Shown=false` to exist -- and `MaybeAutoApply` never writes one for success.)
2. **Version check errors** (cache entries with `e.Error != ""`) -- `IsPendingEntry` filters these out silently; the error string sits in the cache file but never surfaces.
3. **Lock contention** -- when `state.json.lock` is held, `MaybeAutoApply` returns `nil` silently with no notice.
4. **Spawn errors** in `MaybeSpawnAutoApply` -- logged at debug level only; no user-visible signal.
5. **Config/state read errors** inside `apply-updates` -- the command handler returns `nil` on error (lines like `return nil` after failed `config.DefaultConfig()`).

### The `notices.Notice` data model

`internal/notices/notices.go` defines:
- `Tool` string
- `AttemptedVersion` string
- `Error` string (empty string means success)
- `Timestamp` time.Time
- `Shown` bool
- `ConsecutiveFailures` int
- `Kind` string (`""` or `"auto_apply_result"`)

The `Shown` field semantically distinguishes two lifecycle behaviors: failures are persistent (stay until cleared by a successful install), while successes are show-once (deleted by `cmd_notices.go` after display). However, `MaybeAutoApply` never writes a success notice, so the show-once lifecycle for tool update successes is dead code in the current background path.

### How notices are consumed

`cmd/tsuku/cmd_notices.go:noticesCmd` reads all notices, displays them in a table, then deletes success notices (where `n.Error == ""`). `internal/updates/notify.go:renderUnshownNotices` displays unshown notices on stderr during `PersistentPreRun` (called before every normal command), then marks each notice as shown. Failure notices persist across `renderUnshownNotices` calls (they get `Shown=true` but stay on disk); success notices are deleted by `noticesCmd`.

### Where `DisplayNotifications` is called vs. the background process

`DisplayNotifications` is called in `PersistentPreRun` of normal user-facing commands. It is explicitly skipped for `apply-updates` and `check-updates`. This means notices written by the background subprocess are only surfaced the next time the user runs any normal tsuku command -- which is the intended deferred-display pattern.

### The "inbox sink" design gap

Currently only install failures are routed to the notices inbox. The inbox primitives (`WriteNotice`, `KindAutoApplyResult`) are already in place. The gap is that:
1. Successful background updates don't write a notice (user gets no feedback).
2. There is no generic mechanism to write non-update events (e.g., installation warnings, dependency resolution issues) to the inbox -- anything generated during `runInstallWithTelemetry` inside the background process writes to the already-redirected `/dev/null` stderr.
3. The `installFn` passed to `MaybeAutoApply` is a closure that calls `runInstallWithTelemetry` -- all progress output, warnings, and non-fatal errors inside the installer go to stderr, which is `/dev/null` in the background process.

## Implications

### Success notices are the most immediate gap

Routing success events to notices is a single well-scoped change: write a `Notice{Error: "", Shown: false, Kind: KindAutoApplyResult}` on the success path in `MaybeAutoApply`. The display layer in `renderUnshownNotices` already has the `n.Error == ""` branch that handles this correctly. This would close the "silent success" problem with minimal risk.

### Persistent vs. single-view semantics fit naturally

The existing `Shown` bool plus the `cmd_notices.go` deletion logic already implement the two lifecycle modes the exploration describes -- persistent (failure, stays until cleared) and single-view (success, deleted after display). No schema changes are needed to support both types. A new `KindAutoApplyResult` success notice would flow through `renderUnshownNotices` and be displayed once on the next command invocation.

### Routing installer warnings requires a different approach

The install path (`runInstallWithTelemetry`) writes to stderr using the progress/log system, not to the notices inbox. In the background subprocess, stderr is `/dev/null`. To capture non-fatal warnings from the installer, the routing would need to either: (a) pass a callback or writer into the install flow that can capture structured warning events, or (b) wrap the install function so it intercepts log output. Neither is trivial -- it would require changes across the install and action layers. This is a separate, larger effort from the success-notice gap.

### The `MaybeAutoApply` function is the right injection point

`MaybeAutoApply` already has access to `noticesDir` and the outcome of each install attempt. All notice writes for the background path should happen here. The function's return value (`[]ApplyResult`) is also used by the foreground path's `DisplayNotifications`, but in the background subprocess it's discarded. This is architecturally clean -- the function already handles both contexts, it just needs to write more notices.

### A "channel" abstraction isn't strictly necessary at this layer

The background path can be fully covered by expanding what `MaybeAutoApply` writes to the notices directory without introducing a new abstraction. The notices directory already acts as a persistent queue. A more general "sink" abstraction would only be needed if non-update code paths (e.g., the install command, the recipe loader) need to route warnings to the inbox in background mode. That is a broader refactor than fixing the specific auto-apply gaps.

## Surprises

### Success notices are already handled in the display layer but never written

`renderUnshownNotices` has explicit handling for `n.Error == ""` tool notices (line 83-85 in `notify.go`): it prints `"<tool> has been updated to <version>"`. This means the display half of success notice support was written, but the write half in `MaybeAutoApply` was never added. The system is half-implemented -- the display path exists, the write path is missing.

### `apply-updates` passes `nil` for projectCfg

`cmd_apply_updates.go` passes `nil` as the third argument (`projectCfg`) to `MaybeAutoApply`. This means project-level pin overrides (`.tsuku.toml`) are not applied in the background subprocess. This may be intentional (background subprocess lacks a working directory context) but it's undocumented and could cause surprise if the user has project-level pins.

### Double `/dev/null` redirect is redundant but harmless

Both `spawnDetached` (in the parent) and `cmd_apply_updates.go` (in the child) silence output. The child's self-redirect is redundant since the parent already set the subprocess's stdout/stderr to nil (which maps to `/dev/null` on Unix). It adds defense in depth but also suggests these two layers were written independently.

### `ConsecutiveFailures` throttle logic is mentioned in comments but not enforced

The `Notice.ConsecutiveFailures` field comment says "notices with fewer than 3 consecutive failures are suppressed (Shown=true)" but `MaybeAutoApply` always sets `Shown: false` when writing failure notices. There's no code that reads the previous `ConsecutiveFailures` value and applies the suppression logic before writing. The throttle described in the field comment is not currently implemented.

## Open Questions

1. Should successful background updates write a single-view notice unconditionally, or only after a prior failure (i.e., as a "recovery" notice)?
2. Should version check errors (cache entries with `e.Error != ""`) surface as notices, or remain silent since they're transient network/resolution failures?
3. Is the missing `projectCfg` in the background subprocess intentional? If a user has a `.tsuku.toml` with a pin, should the background process respect it?
4. Should `ConsecutiveFailures` throttling be implemented as described in the field comment? If yes, where does the counter increment happen?
5. For the broader goal of routing installer warnings: is the right approach a structured warning callback in the install API, or a log interceptor, or something else?
6. The `cmd_notices.go` display format is a fixed-width table. Is this the intended surface for the new "inbox" UX, or should `tsuku notices` be redesigned as part of this work?

## Summary

The auto-apply background path (`apply-updates` subprocess) already writes failure notices to `$TSUKU_HOME/notices/` via `MaybeAutoApply`, and the display layer in `renderUnshownNotices` already has dead code for success notices that are never written -- the write half of success notice support is simply missing from `MaybeAutoApply`. The most direct path to the "inbox sink" design for background updates is adding a `Notice{Error: ""}` write on the success branch of `MaybeAutoApply`, which would immediately activate the existing display logic and close the silent-success gap with minimal risk. The bigger open question is how to capture non-fatal warnings from inside `runInstallWithTelemetry`, since those go to a `/dev/null`-redirected stderr and would require changes to the install API to route structured events through the notices inbox.
