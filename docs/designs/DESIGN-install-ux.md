---
status: Proposed
problem: |
  tsuku install and update emit 20–50+ lines per install via raw fmt.Printf() calls
  scattered across the executor and 30+ action implementations. Output bypasses --quiet
  inconsistently, has no TTY awareness, and uses a separate progress.Writer widget for
  downloads that doesn't coordinate with step output. This produces verbose, unstructured
  logs in CI and a cluttered terminal during interactive installs.
decision: |
  Replace all fmt.Printf() progress output with a Reporter interface (TTY-aware,
  goroutine-backed spinner on TTY, per-phase plain-text lines on non-TTY). Wire it
  through ExecutionContext so all 43 action implementations gain structured output
  without signature changes. Add an optional ActionDescriber interface so high-frequency
  actions produce human-readable status text. Unify download progress into Reporter.Status()
  calls (with percentage when Content-Length is present, bytes-only fallback). Build
  actions keep CombinedOutput with captured output shown on failure.
rationale: |
  The decisions reinforce each other: the minimal Reporter interface lets tests inject a
  capturing double across 43 action files; the optional ActionDescriber avoids breaking
  the Action interface for external consumers while incrementally improving status text;
  per-phase Log calls on non-TTY solve the CI hung-process problem for build recipes;
  percentage + bytes fallback for downloads preserves orientation without separate
  infrastructure; and keeping CombinedOutput for builds is the lowest-risk path since the
  pattern is already correct — it only needs spinner bookends added.
---

# DESIGN: Install UX

## Status

Proposed

## Context and Problem Statement

`tsuku install` and `tsuku update` currently emit a scrolling log of every step name,
every download start, and every sub-action. A typical install with one dependency
produces 30–50 lines; tools with multiple transitive dependencies exceed 100.

Four specific problems motivate this design:

**No TTY awareness.** The executor uses raw `fmt.Printf()` for all step output.
`--quiet` suppresses `printInfof()` calls but not the executor's `fmt.Printf()` calls,
so even quiet mode produces step-by-step noise when piped.

**No in-place updates.** Each action produces a new line. There's no mechanism to
overwrite the current status as progress moves forward. A download that takes 30 seconds
produces a static progress bar that scrolls past unchanged.

**Fragmented output channels.** Step progress goes via `fmt.Printf()` to stdout.
Download progress uses a separate `progress.Writer` widget that renders a progress bar
alongside (not integrated with) the step name. A single download produces three separate
messages: the step name line, the URL line, and the progress bar.

**Step names instead of semantic descriptions.** Actions emit their internal names
(`"download_file"`, `"extract"`) with a step counter. Useful during debugging, but noise
during normal use. Peer CLIs (cargo, brew, npm) show semantic phases without action names
or step counts.

niwa's `Reporter` (in `internal/workspace/reporter.go`) establishes the reference UX:
a background goroutine that ticks a braille spinner at 100ms, rewrites in-place with
`\r\033[K`, auto-detects TTY at construction (Status is a no-op on non-TTY), and uses
`DeferWarn`/`FlushDeferred` for post-operation summaries.

## Decision Drivers

- **Unified output channel**: one status mechanism for steps and downloads. No separate
  progress bar widget.
- **Structural TTY detection**: non-TTY silence must be automatic from Reporter
  construction, not conditional at each callsite.
- **Minimal call site changes**: ExecutionContext already has a `Logger` field proving
  the injection pattern; adding a `Reporter` field propagates to all 43 action files
  without signature changes.
- **CI feedback for long builds**: build recipes (`cargo_install`, `npm_install`,
  `configure_make`) run 5–15 minutes. Fully silent CI output is unacceptable for
  these cases.
- **No breaking the Action interface**: the repo is public and external consumers may
  implement `Action` directly without embedding `BaseAction`.

## Decisions Already Made

These choices were made during exploration and are treated as constraints by this design:

- **Adopt niwa's Reporter pattern**: the goroutine lifecycle (spinStop/spinDone channels,
  immediate first tick, wait for goroutine exit) and TTY auto-detection model are the
  established reference behavior.
- **Unify download progress into Reporter.Status()**: the `progress.Writer` progress bar
  widget is eliminated.
- **Wire Reporter through ExecutionContext**: add a `Reporter` interface field alongside
  the existing `Logger` field.
- **Eliminate step names and step counters from happy-path output**: `"Step N/M:
  action_name"` is replaced by semantic descriptions.

## Considered Options

### Decision 1: Reporter Interface Contract

The executor and 43 action implementations currently write progress output via raw
`fmt.Printf()` / `fmt.Println()` calls — 396 occurrences across the actions package
alone. Replacing this with a structured output layer requires deciding how to define
and inject the replacement abstraction into `ExecutionContext`.

The codebase already has two reference points: `internal/progress/spinner.go` (a
TTY-aware goroutine spinner with Start/Stop API) and niwa's `Reporter` struct (goroutine-
based in-place redraws with Status/Log/Warn/Defer/FlushDeferred API). Neither is a
direct fit — niwa's struct is the right behavioral model, but tsuku needs its own definition.

#### Chosen: Minimal Go Interface

Define a `Reporter` interface in tsuku's codebase:

```go
// Reporter handles progress display and log output during tool installation.
// Implementations are TTY-aware; Status is a no-op on non-TTY output.
type Reporter interface {
    // Status sets a transient status message. On TTY output a background goroutine
    // redraws the line with an advancing spinner. On non-TTY output this is a no-op.
    Status(msg string)

    // Log writes a permanent log line. On TTY output the spinner is stopped and
    // its line cleared first. Format follows fmt.Sprintf conventions.
    Log(format string, args ...any)

    // Warn writes a permanent warning line (prepends "warning: ").
    Warn(format string, args ...any)

    // DeferWarn queues a warning for FlushDeferred.
    DeferWarn(format string, args ...any)

    // FlushDeferred prints all deferred messages in order and clears the queue.
    FlushDeferred()
}
```

A concrete `TTYReporter` implements this interface with TTY detection at construction
via `term.IsTerminal`, a background goroutine advancing the spinner at 100ms ticks using
`\r\033[K` in-place redraws (matching niwa's pattern), and `Log`/`Warn` stopping the
goroutine before printing. A `NoopReporter` implements the interface with all methods
as no-ops, used as the zero-value default on `ExecutionContext` to avoid nil panics
during incremental migration.

Injection point: `ExecutionContext.Reporter Reporter` (interface type). The executor
constructs `NewTTYReporter(os.Stderr)` and assigns it before calling `ExecutePlan`.

#### Alternatives Considered

**Concrete struct (embed niwa's pattern)**: Define `type Reporter struct` in tsuku with no
interface. Rejected because it couples all 43 action files to the concrete type; tests
need a non-TTY writer and can't distinguish Status vs Log calls without a wrapper —
which is effectively what an interface provides explicitly.

**io.Writer adapter**: Thin wrapper around `io.Writer` where Status writes `\r+msg` to
the writer. Rejected because a plain `io.Writer` has no concept of a background ticker;
in-place redraws during long operations (where no new Status calls arrive) require the
goroutine to advance the spinner frame independently.

---

### Decision 2: Action Description Strategy

The executor currently prints raw action names during installation (`"Step 2/6:
download_file"`). The new install UX needs meaningful status text like `"Downloading
kubectl 1.29.3 (40 MB)"` for each step. The `Action` interface has no facility for
producing this text.

The executor already contains a partial implementation: `formatActionDescription()` in
`executor.go` is a switch/case generating dry-run descriptions for 8 of the ~35 action
types. This function sits in the wrong package (executor rather than actions), covers a
minority of actions, and drifts out of sync as new actions are added.

The codebase has three prior examples of optional action interfaces: `Decomposable`,
`NetworkValidator`, and `Preflight` — each following the same pattern: separate interface,
type-assertion check at callsite, graceful fallback.

#### Chosen: Optional ActionDescriber Interface

Define a new optional interface in `internal/actions/`:

```go
// ActionDescriber is implemented by actions that can produce a human-readable
// status message for display during installation. The executor checks for this
// interface via type assertion and falls back to the action name if not implemented.
type ActionDescriber interface {
    // StatusMessage returns a short description of what this step will do.
    // Params are the resolved step parameters. Returns "" to use the action name.
    StatusMessage(params map[string]interface{}) string
}
```

Executor callsite:

```go
var msg string
if d, ok := action.(ActionDescriber); ok {
    msg = d.StatusMessage(step.Params)
}
if msg == "" {
    msg = step.Action
}
```

Priority implementations for the first release (covers the majority of recipe steps):
`download_file` ("Downloading {basename(url)}" + size if known), `extract`
("Extracting {archive}"), `install_binaries` ("Installing {binaries}"), build actions
("Building {tool} {version}"), package manager actions ("{pm} install {package}").

#### Alternatives Considered

**Add StatusMessage to Action interface**: Would break external implementors who don't
embed `BaseAction` — an explicit constraint violation for this public repo. Also forces
artificial descriptions for low-value system actions (`group_add`, `service_enable`)
where the action name is already sufficient.

**Executor-constructed switch/case**: Extends the existing `formatActionDescription()`
debt pattern: cross-package knowledge of action semantics that drifts out of sync and
cannot access action-internal logic for field selection. Rejected in favor of keeping
description logic with the action that owns it.

---

### Decision 3: Non-TTY Output Behavior

When stdout/stderr is not a TTY — CI systems, piped output, redirected log files —
tsuku must decide how much installation progress to emit. The Reporter's `Status()` is
already a structural no-op on non-TTY. All remaining output passes through `Reporter.Log()`
or `Reporter.Warn()`, which always emit on both TTY and non-TTY. The question is which
calls use Log vs Status.

Today's executor emits 20–50 raw `fmt.Printf` lines that bypass quiet mode entirely.
Build recipes that compile from source (5–15 minute operations) are the critical case:
fully silent CI output gives operators no way to distinguish a healthy long build from
a deadlocked process.

#### Chosen: Per-Phase Text Lines

In non-TTY mode, emit one `Reporter.Log` line per phase transition per tool. Specifically:
"Downloading \<tool\> \<version\> (\<size\>)", "Building \<tool\> \<version\>",
"Extracting \<tool\>", "Installing \<tool\>", "Verifying \<tool\>", and
"\<tool\> \<version\> installed" as the final success line. These are always Log calls —
they survive on both TTY and non-TTY.

Per-step messages (`"Step 3/7: chmod"`, checksum verification details, dependency path
setup) use `Reporter.Status` — transient on TTY, silent on non-TTY. Warnings and errors
always use `Reporter.Log`/`Reporter.Warn` regardless of mode.

The implementation requires adding phase-awareness to install orchestration in
`install_deps.go` and `executor.go`, but the Reporter interface itself requires no changes.

#### Alternatives Considered

**Fully silent except summary**: Only warnings, errors, and the final "installed" line
appear in CI. Rejected because 5–15 minute build recipes produce zero feedback during
execution, making CI jobs look hung and forcing operators to rely solely on timeout
settings to detect real failures.

**One log line per tool**: One "Installing..." line at start plus completion. Rejected
because it leaves the same 5–15 minute silence gap during the compile phase — exactly
where CI operators most need visibility.

---

### Decision 4: Download Progress Granularity

The current `progress.Writer` widget renders `"[========>] 78% (31.2MB/40MB) 8.5MB/s ETA:
00:01"` independently of step output. The install-ux redesign unifies this into
`Reporter.Status()` calls, replacing the widget. The question is what level of detail
belongs in the unified status string.

Content-Length headers are not always present (some CDNs and redirected URLs omit them).
Downloads range from 2 KB (checksum files) to 500+ MB (large toolchains). The Reporter
tick rate is 100ms, matching the current progress widget.

#### Chosen: Percentage + Bytes When Available, Bytes-Only Fallback

When `resp.ContentLength > 0`: `"Downloading kubectl 1.29.3 (12.5 / 40.0 MB, 78%)"`.
When Content-Length is absent: `"Downloading kubectl 1.29.3 (12.5 MB...)"`. For files
under 100 KB, no byte counter appears — filename and spinner only. On non-TTY: one Log
line at download start with size if known, and one at completion; mid-download counters
are Status calls (no-op on non-TTY).

Implementation: a thin instrumented `io.Writer` that tracks bytes written and calls a
progress callback on each write. The callback calls `reporter.Status(formatted)`. The
Reporter tick loop drives the visual update. This replaces the existing `progress.NewWriter`
call in `download_file.go` and `download.go`.

#### Alternatives Considered

**Bytes transferred only**: Always shows transferred bytes, no percentage. Rejected because
when total size is known, omitting the percentage degrades orientation for no implementation
savings — the conditional format costs nothing and percentage is the most useful single
number mid-download.

**Spinner-only with filename**: No instrumentation of the download path. Rejected because
it fails for large, slow downloads. A 500 MB toolchain on a slow connection can take several
minutes; a spinner alone gives no confidence that transfer is progressing.

---

### Decision 5: Build Action Verbosity

tsuku's source-build recipes (`configure_make`, `cargo_build`, `go_build`) run 10–300
seconds and generate substantial compiler output. All three currently use `CombinedOutput()`:
subprocess stdout/stderr are captured, nothing is shown on success, and the full buffer is
appended to the error on failure. This already satisfies the core constraint (diagnostic
output on failure without re-running) but has no connection to the new Reporter architecture.

An undocumented `TSUKU_DEBUG` env var in `cargo_build.go` already gates success output
display, indicating the design intent to hide build output by default.

#### Chosen: Spinner Only, Output Captured + Shown on Failure

Build subprocess output is captured via `CombinedOutput()`. The Reporter shows a spinner
with a static message ("Building cmake 3.28.1...") produced by the action's
`StatusMessage()` implementation. On success, the spinner clears and a permanent
`Reporter.Log("Built cmake 3.28.1")` is emitted. On failure, captured output is included
in the returned error.

On non-TTY (CI): `Reporter.Status` is a no-op; the build emits one `Reporter.Log` line
at start ("Building cmake 3.28.1...") and one at completion or failure — matching the
per-phase pattern from Decision 3 with no special-casing.

The `TSUKU_DEBUG` env var is preserved and documented as the model for a future
`--verbose` streaming mode. The `"Debug:"` printf statements in `configure_make.go` are
cleaned up during migration.

#### Alternatives Considered

**Transient status passthrough**: Route each compiler line to `Reporter.Status()`, overwriting
in-place on TTY. Rejected because `Status` is a no-op on non-TTY (zero CI benefit),
compiler output arrives in bursts exceeding the 100ms tick rate (most lines missed on TTY),
and it requires replacing `CombinedOutput` with a tee pipe in all three actions for
marginal gain.

**--verbose flag enables full build output streaming**: With `--verbose`, build subprocess
output streams to the terminal. Rejected for the initial migration: it requires wiring
verboseFlag into ExecutionContext, switching to streaming subprocess invocation in three
actions, and handling the spinner interaction. The scope belongs in a follow-on issue once
the core Reporter infrastructure is established.

---

### Decision 6: Output Stream

Install progress output needs a dedicated stream. The choices are stdout (the conventional
output stream for command results) or stderr (the conventional stream for diagnostics and
progress). If progress goes to stdout, piping `tsuku install kubectl | grep installed` would
mix progress noise with any data output. If it goes to stderr, `tsuku install kubectl 2>/dev/null`
suppresses all progress while leaving stdout clean for downstream consumers.

#### Chosen: stderr

`NewTTYReporter(os.Stderr)` writes all progress output to stderr. TTY detection uses
`os.Stderr`'s file descriptor. This matches the industry convention for progress and
diagnostic output (cargo, npm, apt all write progress to stderr), keeps stdout free for
structured output if tsuku ever adds `--json` success reporting, and allows users to
suppress progress with `2>/dev/null` without losing stdout data.

#### Alternatives Considered

**stdout**: Routes progress output to the same stream as structured data. Rejected because
it prevents clean piping — `tsuku install foo | tee install.log` would mix progress lines
with any stdout data. Also inconsistent with the convention established by `--json` error
output (already goes to stdout) and peer tools.

---

## Decision Outcome

**Chosen: Minimal Reporter Interface + Optional ActionDescriber + Per-Phase Logging**

### Summary

The install output path is restructured around a single `Reporter` interface injected
through `ExecutionContext`. Every action that currently calls `fmt.Printf()` migrates
to `ctx.Reporter.Log()` for permanent output or `ctx.Reporter.Status()` for transient
status. The concrete `TTYReporter` runs a background goroutine that advances a braille
spinner at 100ms ticks; the goroutine is started lazily on the first `Status()` call and
stopped (with line cleared) before every `Log()` or `Warn()` call. `NewTTYReporter(w)`
detects TTY once at construction; all downstream code is TTY-unaware. A `NoopReporter`
serves as the safe default during incremental migration.

Human-readable status text comes from an optional `ActionDescriber` interface — actions
opt in by implementing `StatusMessage(params) string`. The executor checks via type
assertion and falls back to the action name. High-frequency actions (`download_file`,
`extract`, `install_binaries`, build actions) implement it in the first release;
coverage expands incrementally. This pattern matches `Decomposable`, `NetworkValidator`,
and `Preflight` already in the codebase.

Phase transitions — not individual steps — produce the visible output. In TTY mode:
a spinner line updates in-place as the install progresses; step details are transient
Status calls that appear in the spinner and disappear. In non-TTY mode (CI, pipes):
one Log line per phase transition per tool ("Downloading", "Extracting", "Building",
"Installing", "Verifying") plus the final success line. This produces 4–6 structured
lines for a simple binary install and 10–20 lines for a multi-dependency source build —
clean enough for CI logs, specific enough to identify the phase where a failure occurred.

Download progress is unified into the Reporter channel. A thin instrumented `io.Writer`
tracks bytes written and calls a callback that invokes `reporter.Status()` with a
formatted string: `"Downloading kubectl 1.29.3 (12.5 / 40.0 MB, 78%)"` when
Content-Length is present, `"Downloading kubectl 1.29.3 (12.5 MB...)"` when absent, and
no byte counter for files under 100 KB. The `progress.Writer` widget is deleted.
Build actions keep their existing `CombinedOutput()` pattern, gaining only a spinner
start call and a final Log call. No subprocess plumbing changes in the first release.

### Rationale

The decisions reinforce each other as a system. The minimal interface and `NoopReporter`
enable incremental migration without big-bang breakage — actions migrate to `ctx.Reporter`
one file at a time without TTY setup in tests. The optional `ActionDescriber` avoids the
one thing that would stall the migration: a forced interface change that breaks external
consumers. Per-phase Log calls on non-TTY solve the build-recipe CI problem (Decision 3)
with the same Reporter interface that drives TTY spinners (Decision 1) — no separate
non-TTY code path. The download instrumentation (Decision 4) slots into the same
Reporter.Status() path with no new methods on the interface. Build action verbosity
(Decision 5) defers the complex --verbose plumbing to a follow-on issue, keeping this
PR focused on the migration that enables everything else.

## Solution Architecture

### Overview

The install output system has one production path: `Reporter.Status()` for transient
messages that animate on TTY and disappear on non-TTY, and `Reporter.Log()`/`Reporter.Warn()`
for permanent messages that appear on both. All actions and orchestration code receive the
Reporter through `ExecutionContext`. The Reporter is constructed once per install invocation
and propagates without further injection.

### Components

```
cmd/tsuku/install_deps.go
  └── constructs Reporter (NewTTYReporter)
  └── passes via ExecutionContext
  └── emits phase-entry Log calls at install orchestration level

internal/executor/executor.go
  └── ExecutionContext.Reporter field (Reporter interface)
  └── emits phase-entry Log calls at executor level
  └── calls action.StatusMessage() (via ActionDescriber type assertion)
  └── calls reporter.Status(msg) before each step

internal/progress/
  └── Reporter interface (new)
  └── TTYReporter struct (goroutine, 100ms tick, \r\033[K, braille frames)
  └── NoopReporter struct (all methods no-op, safe default)
  └── ProgressWriter: thin io.Writer that tracks bytes + calls callback

internal/actions/
  └── ActionDescriber interface (new, optional)
  └── Action implementations: StatusMessage() on high-frequency actions
  └── fmt.Printf calls → ctx.Reporter.Log() / ctx.Reporter.Status()

internal/actions/download_file.go, download.go
  └── replace progress.NewWriter with ProgressWriter + reporter.Status callback
```

### Key Interfaces

**Reporter** (defined in `internal/progress`):
```go
type Reporter interface {
    Status(msg string)
    Log(format string, args ...any)
    Warn(format string, args ...any)
    DeferWarn(format string, args ...any)
    FlushDeferred()
    // Stop terminates the background spinner goroutine and clears the status line.
    // Callers must defer reporter.Stop() immediately after construction.
    // Idempotent: safe to call multiple times.
    Stop()
}
```

**ActionDescriber** (defined in `internal/actions`):
```go
type ActionDescriber interface {
    StatusMessage(params map[string]interface{}) string
}
```

**ProgressWriter callback** (defined in `internal/progress`):
```go
type ProgressWriter struct {
    w        io.Writer
    total    int64     // 0 if Content-Length unknown
    written  int64
    callback func(written, total int64)
}
```

`ProgressWriter.written` must be reset to 0 before each retry attempt. The download path
in `download_file.go` retries via `doDownloadFileHTTP` — if `written` is not reset, the
displayed percentage will exceed 100% on the second attempt. `ProgressWriter` must expose
a `Reset()` method or be reconstructed on each attempt.

**ExecutionContext addition** (in `internal/actions/action.go`, where `ExecutionContext` is defined):
```go
type ExecutionContext struct {
    // ... existing fields ...
    Reporter progress.Reporter  // set by executor before ExecutePlan; defaults to NoopReporter
}
```

Note: `ExecutionContext` is defined in `internal/actions/action.go`, not `executor.go`.
`executor.go` constructs the context; the struct definition lives in the actions package.

**TTYReporter construction**:
```go
// NewTTYReporter constructs a Reporter that auto-detects whether w is a TTY.
// If w is a TTY, Status calls animate a braille spinner in-place.
// If w is not a TTY, Status is a no-op and Log/Warn emit plain lines.
// Callers must defer reporter.Stop() immediately after construction.
func NewTTYReporter(w io.Writer) Reporter
```

**Reporter propagation to dependency installs**: `installSingleDependency` in `executor.go`
constructs its own `ExecutionContext` for each dependency (line ~736). This second construction
site must also receive the reporter — otherwise dependency installs produce no output,
breaking the CI feedback goal for multi-dependency recipes.

### Data Flow

**TTY mode (interactive terminal):**
```
install_deps.go
  → reporter.Log("Downloading kubectl 1.29.3 (40.0 MB)")  [phase entry]
  → executor.ExecutePlan() sets ctx.Reporter = reporter
    → action.Execute(ctx, params)
      → ctx.Reporter.Status("Downloading kubectl 1.29.3 (12.5 / 40.0 MB, 78%)")
         [TTYReporter: goroutine ticks every 100ms, redraws \r\033[K⠹ <msg>]
      → ctx.Reporter.Log("Downloaded kubectl 1.29.3")  [phase exit]
         [TTYReporter: stops goroutine, clears line, prints permanent line]
  → reporter.Log("kubectl 1.29.3 installed")  [final summary]
  → reporter.FlushDeferred()                  [any deferred warnings]
```

**Non-TTY mode (CI, pipe):**
```
install_deps.go
  → reporter.Log("Downloading kubectl 1.29.3 (40.0 MB)")  → plain line printed
  → action.Execute(ctx, params)
    → ctx.Reporter.Status(...)  → no-op (nothing printed)
    → ctx.Reporter.Log("Downloaded kubectl 1.29.3")  → plain line printed
  → reporter.Log("kubectl 1.29.3 installed")  → plain line printed
```

## Implementation Approach

### Phase 1: Reporter Infrastructure

Add the Reporter interface, TTYReporter concrete type, and NoopReporter to
`internal/progress/`. This phase has no behavior changes — it only adds new types.

Deliverables:
- `internal/progress/reporter.go`: Reporter interface + NoopReporter
- `internal/progress/tty_reporter.go`: TTYReporter implementation (goroutine, braille spinner)
- `internal/progress/reporter_test.go`: tests for TTY vs non-TTY behavior, goroutine lifecycle

### Phase 2: ExecutionContext Wiring

Add `Reporter Reporter` field to `ExecutionContext` in `internal/executor/executor.go`.
Set it to `NoopReporter{}` as the zero-value default. Wire `NewTTYReporter(os.Stderr)` in
the install orchestration path (`install_deps.go`). No action changes in this phase.

Deliverables:
- `internal/executor/executor.go`: ExecutionContext.Reporter field
- `cmd/tsuku/install_deps.go`: construct and assign reporter

### Phase 3: ActionDescriber Interface + High-Frequency Actions

Add the `ActionDescriber` optional interface in `internal/actions/`. Implement
`StatusMessage()` on: `download_file`, `extract`, `install_binaries`, `configure_make`,
`cargo_build`, `go_build`, `cargo_install`, `npm_install`, `pipx_install`, `gem_install`.
Update executor type-assertion callsite to use it.

Deliverables:
- `internal/actions/action.go` (or new file): ActionDescriber interface
- `internal/executor/executor.go`: type-assertion step-description logic

### Phase 4: Executor Phase-Entry Logging

Replace `fmt.Printf` step headers in `executor.go` and phase announcements in
`install_deps.go` with `reporter.Status()` (transient step detail) and `reporter.Log()`
(phase transitions: Downloading, Extracting, Building, Installing, Verifying, final success).
Remove `formatActionDescription()` from executor.go once ActionDescriber covers the
same actions.

Deliverables:
- `internal/executor/executor.go`: migrate fmt.Printf → reporter
- `cmd/tsuku/install_deps.go`: phase-entry Log calls

### Phase 5: Download Progress Unification

Replace `progress.NewWriter` calls in `download_file.go` and `download.go` with
`ProgressWriter` + reporter callback. Handle Content-Length present vs absent. Apply
100 KB threshold for suppressing counters on small files. Delete `internal/progress/progress.go`
once all callsites are migrated.

Deliverables:
- `internal/progress/progress_writer.go`: ProgressWriter (byte-counting io.Writer)
- `internal/actions/download_file.go`: reporter-based progress
- `internal/actions/download.go`: reporter-based progress

### Phase 6: Remaining Action Migration

Migrate `fmt.Printf` calls in remaining action files to `ctx.Reporter.Log()` or
`ctx.Reporter.Status()`. Permanent user-visible lines → Log; step-internal detail →
Status; debug/trace output → remove or gate behind `TSUKU_DEBUG`. Remove `"Debug:"`
printf statements from `configure_make.go`.

Note: roughly 20–30% of `fmt.Printf` calls are in context-free helper functions (e.g.,
`downloadFileHTTP`, `doDownloadFileHTTP`) that don't receive `ExecutionContext`. These
require a structural parameter addition, not a simple identifier substitution. Budget
accordingly.

Security checklist for each migrated call:
- [ ] Does this call format any value from `internal/secrets/`? If so, remove, don't migrate.
- [ ] Is the format string a variable (e.g., `fmt.Printf(someString)` → `reporter.Log(someString)`)? If so, use `reporter.Log("%s", someString)` to prevent format-string injection.

Deliverables:
- All remaining action files in `internal/actions/`: fmt.Printf → reporter

## Security Considerations

**ANSI escape code injection (High)**

Status messages include tool names, versions, URL basenames, and package names sourced from
recipe TOML files or external version providers. A crafted recipe or compromised version
provider could embed ANSI/VT100 escape sequences in these values.

Stripping only SGR color sequences (`\033[...m`) and erase-line (`\033[...K`) is
insufficient. The full threat surface includes cursor movement (`\033[H`, `\033[A/B/C/D`),
screen operations (`\033[2J`), cursor visibility (`\033[?25l`), and OSC sequences
(`\033]0;title\007`) that can modify terminal state silently. OSC title injection in
particular is invisible to users but modifies their terminal window title.

Implementation requirements:
1. Define `progress.SanitizeDisplayString(s string) string` that strips all ANSI/VT100
   escape sequences. The correct regex for complete CSI coverage is
   `\x1b\[[\x30-\x3F]*[\x20-\x2F]*[A-Za-z]` (parameter bytes span `0x30–0x3F`, which
   includes `?`, `<`, `=`, `>` in addition to digits and semicolons — necessary to catch
   hide-cursor `\x1b[?25l` and alternate-screen `\x1b[?1049h`). OSC:
   `\x1b\][^\x07]*?(\x07|\x1b\\)`. A safe single-pass alternative:
   `\x1b([@-Z\\-_]|\[[\x30-\x3F]*[\x20-\x2F]*[\x40-\x7E]|\][^\x07]*?(\x07|\x1b\\))`.
   Also strip raw `\x1b` characters not matched by the above.
2. Apply `SanitizeDisplayString` inside `TTYReporter.Status()`, `Log()`, `Warn()`, and
   `DeferWarn()` for all inputs — not in individual `StatusMessage()` implementations.
   This is defense-in-depth: callers cannot forget to sanitize.
3. For URL basenames: apply `path.Base()` first, then verify the result contains only
   printable ASCII (0x20–0x7E). Replace non-printable bytes with `?` or omit the filename.
4. The `ProgressWriter` byte counter path (formatted integers only) requires no
   sanitization.

**Format-string injection during migration**

`Reporter.Log(format, args...)` accepts a format string. A migration that converts
`fmt.Printf(recipeValue)` to `reporter.Log(recipeValue)` — using a recipe-sourced string
as the format argument — would allow format verbs in tool names or URLs to cause incorrect
output or panics. All `reporter.Log` calls must use a literal format string:
`reporter.Log("%s", recipeValue)`, not `reporter.Log(recipeValue)`. This is a named
checklist item for the Phase 6 migration issue (see Implementation Approach).

**Goroutine lifecycle and terminal state**

`TTYReporter` starts a background goroutine on the first `Status()` call. If the install
path panics or exits abnormally without calling `Stop()`, the goroutine may leak and leave
the terminal with a partial spinner line. Mitigations: `TTYReporter` must expose a `Stop()`
method; install orchestration must `defer reporter.Stop()` immediately after construction.
`Stop()` must be idempotent — calling it after `FlushDeferred()` must not panic or attempt
to restart the stopped goroutine. The goroutine should also select on a context cancellation
channel if a context is passed at construction.

**Secret leakage during migration**

Phase 6 migrates 396 `fmt.Printf` occurrences to `reporter.*` calls. Some existing calls
may format secret-bearing variables (resolved API keys, tokens, registry credentials from
`internal/secrets/`). Each migrated call must be individually reviewed. The `Reporter`
interface comment must state that callers must not pass values from `internal/secrets/` to
any Reporter method. Calls that currently expose secrets should be removed, not migrated.

## Consequences

### Positive

- **CI feedback for build recipes**: per-phase Log lines prevent 5–15 minute silence in
  CI, letting operators identify hung phases.
- **Interactive UX**: single in-place status line instead of 20–50 scrolling lines. TTY
  output is clean and professional.
- **Consistent --quiet**: once migration is complete, `--quiet` can reliably suppress all
  non-error output by injecting a NoopReporter instead of conditional quiet checks scattered
  across callers.
- **Testable output**: the Reporter interface makes action test assertions precise —
  tests can assert `reporter.Log("Downloading kubectl 1.29.3 (40.0 MB)")` rather than
  capturing stdout.
- **Download progress unified**: one rendering path replaces two (progress.Writer + fmt.Printf).

### Negative

- **Large migration surface**: 396 `fmt.Printf` occurrences across 43 action files require
  review and reclassification. High risk of inconsistency if done in multiple PRs.
- **Phase-awareness requirement**: per-phase Log calls require install_deps.go and
  executor.go to know which phase is active and emit the right Log message. This is a new
  structural concern that doesn't exist today.
- **ANSI sanitization in hot path**: stripping ANSI sequences on every Status message
  adds a small string-processing overhead. Negligible in practice (100ms tick rate)
  but worth profiling if status messages are very long.

### Mitigations

- **Large migration**: implement in phases (Reporter infrastructure → wiring → high-frequency
  actions → remaining actions) so each phase is reviewable independently. The NoopReporter
  default ensures partially migrated code still works correctly.
- **Phase awareness**: define a small set of canonical phase-entry functions in install_deps.go
  and executor.go that call `reporter.Log()`. These become the only places that emit phase
  Log lines, making phase-awareness localized rather than scattered.
- **ANSI overhead**: apply sanitization only to recipe-sourced values (tool name, version,
  URL basename), not to tsuku-generated strings. A simple regex strip on those specific
  inputs is sufficient.
