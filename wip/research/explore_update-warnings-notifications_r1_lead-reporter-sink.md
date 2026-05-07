# Lead: Reporter/progress system and sink abstraction

## Findings

### The Reporter interface

`internal/progress/reporter.go` defines a `Reporter` interface with six methods:

- `Status(msg string)` — transient spinner line, TTY-only no-op on non-TTY
- `Log(format string, args ...any)` — permanent output line
- `Warn(format string, args ...any)` — like Log but prepends `"warning: "`
- `DeferWarn(format string, args ...any)` — queues a warning for end-of-operation flush
- `FlushDeferred()` — emits all queued DeferWarn messages and clears the queue
- `Stop()` — terminates the background spinner goroutine

Two implementations exist: `ttyReporter` (the production one, TTY-aware) and `NoopReporter` (discards all output, used in tests and in `install.Manager` / `executor.Executor` when no reporter is set via their nil-guard `getReporter()` methods).

The `ttyReporter` checks `term.IsTerminal(fd)` at construction time and uses that single bit to decide whether to run the spinner and whether `Status` is a no-op.

### How reporter is constructed and passed

There are two distinct reporter-construction patterns:

**Interactive paths** (`install.go`, `update.go`): `progress.NewTTYReporter(os.Stderr)` is created at the command layer and threaded through `installWithDependencies → install.Manager.SetReporter → executor.Executor.SetReporter → actions.ExecutionContext.Reporter`. The caller owns the lifecycle (calls `Stop()` and `FlushDeferred()` after the install completes). `update.go` uses `runInstallWithExternalReporter` so the same reporter emits both install progress and the post-install outcome line without mixing streams.

**Background auto-apply path** (`cmd_apply_updates.go`): `os.Stdout` and `os.Stderr` are both redirected to `/dev/null` before the `installFn` callback is built. That callback calls `runInstallWithTelemetry`, which constructs `progress.NewTTYReporter(os.Stderr)` — but `os.Stderr` is already `/dev/null`. The TTY check returns false (devnull is not a terminal), so the reporter is in non-TTY mode; `Status` is a no-op, `Log`/`Warn` write to devnull. All inline output is silently discarded.

The background process spawned by `MaybeSpawnAutoApply` runs `tsuku apply-updates` as a completely detached subprocess with `cmd.Stdin/Stdout/Stderr = nil`.

### Where DeferWarn, Log, Warn are called in the install/update flow

The only `DeferWarn` call site in the entire codebase is in `install_deps.go`:

```go
reporter.DeferWarn("To use the installed tool, add this to your shell profile:\n  export PATH=\"%s:$PATH\"", cfg.CurrentDir)
```

This fires for every explicit top-level install. In background mode this is silently lost.

`reporter.Warn` is called at roughly 15 sites across `install_deps.go`, `install_lib.go`, and `internal/actions/` (pip, nix-portable, cargo-build, etc.) for non-fatal issues: version resolution fallback, post-install phase failures, state update failures, shell cache rebuild failures, library checksum issues, and so on.

`reporter.Log` is called for progress lines and the final `✅ tool@version` / `❌ tool@version` outcomes.

### What the notices package already provides

`internal/notices/notices.go` stores structured `Notice` values as JSON files in `$TSUKU_HOME/notices/<toolname>.json`. The current `Notice` struct tracks: tool name, attempted version, error (empty string means success), timestamp, `Shown` flag, `ConsecutiveFailures`, and `Kind` (empty = update result, `"auto_apply_result"` = background auto-apply).

Notices are written in two places today:
- `updates/apply.go` (`MaybeAutoApply`) writes failure notices with `Kind = KindAutoApplyResult`
- `cmd/tsuku/update.go` writes success notices after manual update

The `tsuku notices` command reads them. `DisplayNotifications` (`updates/notify.go`) renders unshown notices to stderr at the start of the next interactive command. Notices persist until the tool is updated/rolled back/removed.

The notices package does not currently handle warnings — only top-level success/failure outcomes.

### Bypass: direct fmt.Fprintf to os.Stderr

`update.go`'s `warnShellInitChanges` function calls `fmt.Fprintf(os.Stderr, "Warning: ...")` directly, bypassing the reporter entirely. In background mode this would also go to devnull. There are similar raw `fmt.Fprintf(os.Stderr, ...)` warning calls in `main.go`, `outdated.go`, `create.go`, and other command files — all terminal-only, not persisted.

### Where a sink abstraction could fit

The `Reporter` interface is already the right abstraction layer. Every internal package (`actions`, `executor`, `install.Manager`) accepts `progress.Reporter` through dependency injection — there are no global singletons or package-level `fmt.Fprintf` calls in the install path (the exceptions are in top-level command handlers, not the install engine).

Adding a sink would not require changing call sites in `actions/`, `executor/`, or `install/`. It would only require a new `Reporter` implementation. Two options:

**Option A — `inboxReporter` as a new `Reporter` implementation**: A concrete type in `internal/progress` that, instead of writing to an `io.Writer`, calls a callback (or writes to a `notices.Writer` interface). `Warn` and `DeferWarn` both persist to the inbox; `Log`, `Status`, and `Stop` are no-ops. The background path in `cmd_apply_updates.go` would construct this instead of `NewTTYReporter`. Zero call-site changes.

**Option B — `fanoutReporter` wrapper**: A wrapper `Reporter` that holds a slice of inner reporters and dispatches each method call to all of them. The interactive path could use `fanout(ttyReporter, maybeInboxReporter)` to simultaneously display and persist. This is the "also inbox for events warranting persistence" case from the exploration context.

Neither option requires touching `install_deps.go`, the `actions/` package, `executor/`, or `install/`.

The `DeferWarn` + `FlushDeferred` pattern has a lifecycle mismatch in the background context: `FlushDeferred` in background mode is called but flushes to devnull. An `inboxReporter` could treat `DeferWarn` as immediately persistent (no deferred queue needed) since there is no terminal to wait for.

## Implications

- A new `inboxReporter` implementation of `progress.Reporter` is sufficient for the background path. It can be added to `internal/progress/` with no changes to any call site.
- The single change point is `cmd_apply_updates.go`: replace the implicit `NewTTYReporter(os.Stderr)` (created inside `runInstallWithTelemetry`) with an explicit `inboxReporter`. This requires splitting the reporter construction out of `runInstallWithTelemetry` — the `runInstallWithExternalReporter` function already exists for exactly this purpose.
- `warnShellInitChanges` and similar raw `fmt.Fprintf` calls in `update.go` are not in the background path today (they're only called from the interactive `update` command), but if the background auto-apply path is ever extended to run shell-init diff checks, those calls would need to be routed through the reporter.
- The `Notice` struct needs at least one new field (or a new Kind value) to represent per-tool inline warnings (vs. the current top-level success/failure notices). Alternatively warnings could be accumulated in the `Error` field, but that conflates failures with non-fatal warnings.
- Lifecycle semantics differ by notice type: persistent errors stay until tool update/rollback/remove; per-install warnings (like "PATH not set") should clear after one view. The current `Shown` flag already models single-view, but it applies per-tool notice, not per-message.

## Surprises

- The background process redirects `os.Stderr` to `/dev/null` before calling `runInstallWithTelemetry`. This means `NewTTYReporter(os.Stderr)` is called with a non-TTY handle (devnull). The reporter is constructed and functions normally, it just has nowhere to write. The silent loss is not a bug in the reporter — it's a deliberate design choice in `cmd_apply_updates.go` to run silently. This makes the fix cleaner: swap the reporter, not the devnull redirect.
- `DeferWarn` has exactly one call site in production code (the PATH suggestion), which means the "deferred queue" pattern is underused. The queuing mechanism may be more important as a semantic signal ("this is a post-install advisory") than for TTY timing reasons.
- The `Spinner` type in `spinner.go` is marked deprecated in favor of `Reporter`/`NewTTYReporter`, and is not used in the install path. The migration is complete for install/update.
- `fanoutReporter` doesn't exist yet, but the interface cleanly supports it with zero changes to callers.

## Open Questions

- Should `inboxReporter` persist every `Warn` call as a separate notice entry, or accumulate them and flush on operation completion? Per-warn persistence is simpler but could generate many small files; accumulation requires lifecycle coordination.
- What is the right `Notice` shape for inline warnings? Should warnings extend the existing `Notice` with a `[]string` warnings field, or be a separate type with a different file schema?
- Should `reporter.Log` calls (e.g., `"✅ tool@version"`) also be persisted in background mode, or only `Warn`/`DeferWarn`? Persisting `Log` would let users audit what the background process installed, but could be noisy.
- The `cmd_apply_updates.go` devnull redirect also blocks any future `fmt.Fprintf(os.Stderr, ...)` calls that get added to `runInstallWithTelemetry` from persisting. Should the devnull redirect be removed entirely (relying on the `inboxReporter` to suppress terminal output), or kept as a belt-and-suspenders guard?
- Does the two-notice-type distinction (persistent error vs. single-view) map cleanly onto the warn vs. fatal-error distinction in `Reporter`, or does the caller need an explicit API to mark certain warnings as "persistent until tool updated"?

## Summary

The `Reporter` interface is already threaded through the full install stack via dependency injection, with every call site using the same interface regardless of context. The background auto-apply path silently discards all reporter output by redirecting stderr to devnull before reporter construction; the fix is a new `inboxReporter` implementation that persists warns to the notices directory, constructed via the existing `runInstallWithExternalReporter` entry point — no call-site changes required. The main open question is the right `Notice` schema for per-install inline warnings versus the current top-level success/failure notice model.
