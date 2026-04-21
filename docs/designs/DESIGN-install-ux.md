---
status: Proposed
problem: |
  tsuku install and tsuku update emit 20–50+ lines per install via raw fmt.Printf()
  in the executor and 30+ action implementations. The output bypasses --quiet
  inconsistently, is not TTY-aware, and uses a separate progress.Writer widget for
  downloads that doesn't coordinate with step output. The architectural decisions
  needed are: the Reporter interface contract, how it wires through ExecutionContext,
  whether actions gain a UserLabel(params) method or the executor constructs messages
  from plan metadata, what non-TTY output looks like, and how download progress
  integrates into the unified status channel.
---

# DESIGN: Install UX

## Status

Proposed

## Context and Problem Statement

Today `tsuku install` and `tsuku update` emit a scrolling log of every step name,
every download start, and every sub-action. A typical install with one dependency
produces 30–50 lines; tools with multiple transitive dependencies exceed 100.

The core problems:

1. **No TTY awareness.** The executor uses raw `fmt.Printf()` for all step output. `--quiet`
   suppresses `printInfof()` calls but not the executor's `fmt.Printf()` calls, so even quiet
   mode produces step-by-step noise when piped.

2. **No in-place updates.** Each action produces a new line. There's no mechanism to
   overwrite the current status as progress moves forward.

3. **Fragmented output channels.** Step progress goes via `fmt.Printf()` to stdout. Download
   progress uses a separate `progress.Writer` widget that renders a progress bar alongside
   (not integrated with) the step name. The result: three messages for one download — the step
   name line, the URL line, and the progress bar.

4. **Step names instead of semantic descriptions.** Actions emit their internal name
   (`"download_file"`, `"extract"`) with a step counter. Peer CLIs (cargo, brew, npm) show
   semantic phases ("Downloading", "Compiling") without action names or step counts.

niwa's `Reporter` (in `internal/workspace/reporter.go`) is the reference implementation for
the target UX: a background goroutine that ticks a braille spinner at 100ms, rewrites in-place
with `\r\033[K`, auto-detects TTY at construction (Status is a no-op on non-TTY), and uses
`DeferWarn`/`FlushDeferred` for post-operation summaries.

The exploration established the reference architecture and wiring point. Open decisions are
the interface contract, the description abstraction, non-TTY output granularity, and how
download progress integrates into the unified channel.

## Decision Drivers

- **Unified output channel**: The user requirement is one status mechanism for steps and
  downloads. No separate progress bar widget.
- **TTY degradation must be structural**: Non-TTY silence should be automatic from the
  Reporter construction, not a flag passed through every call site.
- **Minimize call site changes**: ExecutionContext already has a `Logger` field (line 429)
  proving the pattern. Adding a `Reporter` field to ExecutionContext propagates to all
  actions without signature changes.
- **Happy-path information density**: Peer CLIs show semantic phases, not action names.
  `"Step 2/6: extract"` has no user value. Dropping step names and step counters is ~70%
  of the noise reduction.
- **Long-running actions need feedback**: Build actions (`cargo_build`, `go_build`,
  `cmake_build`) run 10–300 seconds. A spinner alone may be insufficient; the design needs
  a strategy for these.
- **Non-TTY consumers (CI, pipes)**: niwa's pattern is fully silent on non-TTY except for
  Log lines. Whether tsuku should be silent-except-summary or emit per-phase text lines for
  CI is an open design decision.

## Decisions Already Made

From the exploration's convergence round:

- **Adopt niwa's Reporter pattern as the reference architecture**: Do not invent a new
  abstraction. The goroutine lifecycle (spinStop/spinDone channels, immediate first tick,
  wait for goroutine exit before returning from stopSpinner) and the TTY auto-detection model
  are established reference behavior.

- **Unify download progress into Reporter.Status()**: Replace the `progress.Writer`
  instantiation in `httputil.HTTPDownload()` with a progress callback that calls
  `Reporter.Status(...)`. The download progress bar widget is eliminated.

- **Wire Reporter through ExecutionContext**: Add a `Reporter` interface field to
  `internal/executor/executor.go`'s `ExecutionContext` struct alongside the existing
  `Logger` field. This propagates to all 384+ action callsites with zero signature changes.

- **Eliminate step names and step counters from happy-path output**: The `"Step N/M:
  action_name"` pattern is replaced. Users see semantic phases; action names are suppressed
  unless `--verbose` is set.
