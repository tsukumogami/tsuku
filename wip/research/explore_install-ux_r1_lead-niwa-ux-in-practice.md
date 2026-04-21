# Lead: niwa UX in practice

## Findings

### Reporter Implementation (Reporter struct)
**Location**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-3/public/niwa/internal/workspace/reporter.go`

The `Reporter` is a stateful writer that bridges TTY and non-TTY modes with a background spinner goroutine.

**Core Architecture**:
- Two modes: TTY (spinner active) vs. non-TTY (direct logging)
- TTY auto-detection via `term.IsTerminal()` on file descriptor, or explicit `NewReporterWithTTY(w, isTTY bool)`
- Background spinner goroutine ticks every ~100ms (after immediate first render)
- Braille spinner frames: `{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}` (10 frames, cycles)

**Key Methods**:
- **`Status(msg)`**: Updates transient status + starts spinner (TTY only; no-op on non-TTY)
  - Writes: `\r\033[K<frame> <msg>` every ~100ms via background goroutine
  - Frame counter increments each tick
  - Goroutine started lazily on first Status call, stopped by Log/Warn
- **`Log(format, ...)`**: Permanent log line (newline always appended)
  - TTY: calls `stopSpinner()` first (sends close signal, waits for goroutine, clears line with `\r\033[K`)
  - Then writes: `<format>\n`
  - Non-TTY: writes directly (no ANSI sequences)
- **`Warn(format, ...)`**: Prepends "warning: " and delegates to Log
- **`Defer(format, ...)`**: Queues message in `r.deferred` slice (for post-operation summary)
- **`DeferWarn(format, ...)`**: Queues "warning: " + format
- **`FlushDeferred()`**: Emits all deferred messages via Log (each on its own line), clears buffer
- **`Writer()`**: Returns `io.Writer` adapter that calls `Log` on Write, strips trailing newlines

**Goroutine Lifecycle**:
1. `Status()` creates channels `spinStop`, `spinDone` and spawns `spinLoop()`
2. `spinLoop()` immediately calls `doTick()` (no delay on first display)
3. Ticks every 100ms or until `spinStop` is closed
4. `stopSpinner()` closes `spinStop`, waits for `spinDone`, clears the spinner line, nulls the channels
5. Goroutine exits after line is cleared; no lingering state

**ANSI Sequences**:
- CR (carriage return): `\r` — moves cursor to start of line
- Erase: `\033[K` (aka `\x1b[K`) — clears from cursor to end of line
- Combined as `\r\033[K` before each status update (overwrites previous line in place)

### Multi-Step Apply/Create Operation Sequence
**Location**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-3/public/niwa/internal/workspace/apply.go`

The `Applier.runPipeline()` orchestrates the main steps with Reporter calls interleaved:

**Step 2a (Sync global config)** — lines 609–614:
```go
a.Reporter.Status("syncing config...")
if syncErr := SyncConfigDir(a.GlobalConfigDir, a.Reporter, a.AllowDirty); syncErr != nil {
    a.Reporter.Warn("could not sync config: %v", syncErr)  // stops spinner, logs warning
    return nil, fmt.Errorf("syncing global config: %w", syncErr)
}
```
- **TTY Output**: Spinner shows "⠋ syncing config..." (animates every 100ms)
- **Non-TTY Output**: No output (Status is no-op)
- When Warn is called: spinner goroutine stopped, line cleared, "warning: could not sync config: ..." logged permanently

**Step 3 (Clone repos concurrently)** — lines 814–861:
```go
a.Reporter.Status(fmt.Sprintf("cloning repos... (0/%d done)", total))  // initial
// ... spawn workers in pool, submit jobs
for done := 0; done < total; done++ {
    r := <-results
    if r.err != nil && cloneErr == nil {
        cloneErr = fmt.Errorf("cloning repo %s: %w", r.name, r.err)
        cancel()
    }
    if cloneErr == nil {
        if r.syncWarn != "" {
            a.Reporter.DeferWarn("%s", r.syncWarn)  // queue, don't log yet
        }
        repoStates[r.name] = RepoState{URL: r.cloneURL, Cloned: r.cloned || ...}
    }
    a.Reporter.Status(fmt.Sprintf("cloning repos... (%d/%d done)", done+1, total))  // update counter
}
```
- **TTY Output Sequence**:
  1. Status call 1: `⠋ cloning repos... (0/5 done)` appears, spinner starts
  2. Background goroutine ticks every 100ms: `⠙ cloning repos... (0/5 done)` → `⠹ cloning repos... (0/5 done)` → ...
  3. Status call 2: message changes in-place: `\r\033[K⠐ cloning repos... (1/5 done)`
  4. Repeats for each result until done
  5. Last status: `⠏ cloning repos... (5/5 done)`
  6. Next Log/DeferWarn triggers spinner stop (line clears, final status disappears)
- **Non-TTY Output**: All Status calls are no-ops; no output during cloning
- **Deferred warnings**: Sync warnings queued via DeferWarn (shown later at end)

**Step 6.75 (Run setup scripts)** — lines 1030–1046:
```go
for _, cr := range classified {
    result := RunSetupScripts(repoDir, setupDir, a.Reporter)
    if result.Error != nil {
        a.Reporter.DeferWarn("setup script %s/%s failed for %s: %v", ...)
    }
}
```
- Setup script output routed through Reporter.Status() if TTY (via `runCmdWithReporter` in gitutil.go)
- Warnings deferred for later flush

**Final Log Sequence (Apply)** — lines 379–388:
```go
n := len(result.repoStates)
if n == 1 {
    a.Reporter.Log("applied %s (1 repo)", filepath.Base(instanceRoot))
} else {
    a.Reporter.Log("applied %s (%d repos)", filepath.Base(instanceRoot), n)
}
for _, w := range result.warnings {
    a.Reporter.DeferWarn("%s", w)
}
a.Reporter.FlushDeferred()
```
- `Log()` call stops spinner (if active) and outputs summary line: `"applied myws-1 (5 repos)"`
- `DeferWarn()` queues warnings collected during pipeline
- `FlushDeferred()` outputs all warnings: each on its own line with "warning: " prefix

**Create operation** — lines 255–264 — follows same pattern (different summary line).

### Git Output Handling
**Location**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-3/public/niwa/internal/workspace/gitutil.go`

**`runGitWithReporter(r *Reporter, cmd)`** — lines 53–85:
- Pipes git stdout+stderr through line scanner
- ANSI escape sequences stripped unconditionally
- Lines classified:
  - If starts with "fatal:" / "error:" / "warning:" → routed through `r.Warn()` (stops spinner, logs permanently)
  - All other lines (progress, status) → discarded silently (niwa emits own completion messages)
- If cmd fails and error lines captured, error is enriched with those lines

**Example**: `git clone` output "Cloning into 'foo'..." is discarded; "fatal: couldn't read username" is logged as warning.

**`runCmdWithReporter(r *Reporter, cmd)`** — lines 93–116:
- All output (no line classification) routed through `r.Status()` for transient display
- Useful for setup scripts whose output format isn't predictable
- On non-TTY, Status is no-op, so script output is silent in CI

### Config Dir Sync Example
**Location**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-3/public/niwa/internal/workspace/sync.go` (called via `SyncConfigDir`)

When the workspace config repo needs pulling:
- Fetch is attempted via `FetchRepo()` (uses `runGitWithReporter`)
- If fetch succeeds but pull fails, `SyncRepo()` returns `SyncResult{Action: "fetch-failed", Reason: ...}`
- Caller queues a DeferWarn for later flush

### TTY vs. Non-TTY Behavior
**TTY Mode** (interactive terminal):
- Status: spinner animates in place every 100ms
- Log/Warn: stops spinner, clears line, outputs permanent text
- Deferred: buffered during operation, flushed at end (summary-time)
- Result: Live progress updates, clean final output block

**Non-TTY Mode** (pipes, CI, redirected output):
- Status: no-op (no output)
- Log/Warn: direct output (no ANSI sequences, no CR)
- Deferred: same buffering, flushed at end
- Result: Only permanent messages visible; clean log for CI consumption

### Tests Verifying Output Format
**Location**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-3/public/niwa/internal/workspace/reporter_test.go`

Key test assertions:
- `TestReporterTTYLogClearsStatus`: After Status + Log, output ends with `\r\033[Klog line\n`
- `TestReporterSpinnerTickFormat`: Single tick writes `\r\033[K<frame> <msg>`
- `TestReporterNonTTYLog`: Non-TTY Log outputs plain text with no ANSI sequences
- `TestReporterWriterNoDoubleNewline`: Writer strips trailing newline to avoid double-newlines in output

## Implications

1. **In-place Status Updates Are Automatic**: niwa doesn't manually clear/redraw; the background goroutine handles 100ms tick rate. Tsuku's approach of explicit cursor positioning would need replacement with Reporter-like goroutine pattern.

2. **Deferred Output Enables Clean Summaries**: Warnings accumulated during operation (via DeferWarn) are not shown inline. They flush after the operation's summary line, creating a clean visual block. Tsuku's current verbose per-step logging (with no summary) is the opposite.

3. **Log() Is a "Commitment"**: Calling Log stops the spinner, ensuring the message is permanent. In Tsuku's executor.go, raw fmt.Printf() doesn't interact with transient state; Reporter forces ordering: stop spinner → output line → continue (or no more ticks).

4. **Non-TTY Fallback Is Transparent**: Piping/CI detection happens once (at Reporter construction); all downstream code uses Status/Log/Defer uniformly. No conditional logic in callers about TTY mode.

5. **Git Output Is Surgically Filtered**: niwa discards progress noise ("Cloning into...", "Already up to date") but preserves diagnostics. Tsuku's progress.go Writer doesn't filter; it just counts bytes. niwa's approach prevents CI logs from being cluttered.

6. **Writer() Adapter Standardizes Subprocess Output**: Instead of threads passing their own progress writers, subprocesses are given Reporter.Writer(), which adapts Write calls into Log calls. Stops spinner, then outputs. Prevents mixed TTY/non-TTY in subprocess output.

## Surprises

1. **No Custom Progress Bar**: Despite the mention of "braille spinner" in the spec, there's no separate progress-bar widget. All visual output (including download progress, if added) would use Status() with manually formatted counter strings like `"cloning repos... (3/5 done)"`. This is deliberately simple: one status line, no parallel widgets.

2. **Goroutine Coordination Is Careful**: The spinLoop sends select on both a ticker and a stop channel. The orchestrator waits for goroutine exit via `<-done` before returning from stopSpinner. No race conditions from goroutine leaks or missed messages.

3. **DeferWarn Doesn't Stop Spinner**: Deferred messages are queued without stopping the goroutine. Only Log/Warn (immediate output) trigger stopSpinner. This keeps the spinner visible during the pipeline until the final summary line.

4. **Sync Failures Are Hard Errors in Some Cases**: When OverlayURL is registered in state and sync fails, it's a hard error (no `DeferWarn`). But when overlay discovery is fresh and the repo doesn't exist, it silently skips with no output. Implies: if you've explicitly set an overlay, niwa assumes you want it; if discovery is tentative, failures are benign.

## Open Questions

1. **Download Progress Integration**: The lead mentions "Download progress should be unified into the same status channel — no separate progress bar widget." niwa doesn't show download progress in apply/create. Where does download happen? (During git clone, which is piped through `runGitWithReporter`.) How should tsuku's separate progress.go Writer be merged into Reporter? Should it be a separate Status call that gets overwritten like the clone counter, or is there a fancier format envisioned (e.g., `"cloning repos... (3/5 done) [50%]"`)?

2. **Error Handling Checkpoint**: The clone orchestrator (lines 814–861) collects a `cloneErr` and cancels workers on first error. Do warnings from failed syncs (syncWarn) contribute to the final exit code, or are they just logged? Deferred warnings don't stop the operation.

3. **Workspace Root Disclosures**: niwa has a one-time notice system (DisclosedNotices) for provider shadows. Is this a pattern Tsuku should adopt for recurring warnings?

## Summary

niwa's Reporter provides a reference implementation of TTY-aware status lines with a background spinner goroutine that ticks every 100ms, redraws in place with `\r\033[K`, and stops when Log/Warn is called. All output is routed through Status/Log/Defer/DeferWarn, ensuring TTY vs. non-TTY degradation is automatic; git and subprocess output is filtered surgically to suppress progress noise while preserving errors. Warnings are deferred and flushed after a summary line, creating clean output blocks. Tsuku's replacement of raw fmt.Printf() in executor.go should adopt the Reporter pattern (goroutine + channels for tick synchronization, immediate rendering on first Status call, and careful wait for goroutine exit on stop).

