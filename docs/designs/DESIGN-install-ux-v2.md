---
status: Planned
problem: |
  tsuku install produces 40+ sequential lines instead of a single updating status
  line. The Reporter/TTYReporter wired in PR #2280 only reached the action
  execution layer. The command-entry layer, install orchestration, library install,
  and verification paths all still use raw fmt.Printf, leaving the spinner with
  nowhere to live for ~95% of execution time.
decision: |
  Create the Reporter once in runInstallWithTelemetry and thread it as a new
  parameter to the recursive installWithDependencies function. Wire Reporter into
  internal/install/Manager via SetReporter() (mirroring the executor pattern).
  Convert all intermediate orchestration output to Status() calls, convert action
  sub-step detail lines to Status() or silence, suppress verify sub-steps via
  Verbose:false, and keep only the install start, verify, and completion lines as
  permanent Log() output.
rationale: |
  SetReporter() on Manager is the only injection strategy that satisfies minimal
  interface churn — no existing callers change signatures. Status() conversion
  achieves the single-line goal on TTY while remaining a no-op on non-TTY/CI,
  so CI still receives the Log() lines that matter. The conflict between
  silencing all orchestration labels and CI legibility is resolved by keeping
  the install-start and install-done lines as Log().
---

# DESIGN: Install UX v2

## Status

Planned

## Context and Problem Statement

`tsuku install serve` produces 40+ sequential lines. PR #2280 added a `Reporter`
interface with a TTY spinner, but only wired it into the action execution layer
(`internal/executor/`, `internal/actions/`). The remaining output comes from three
unwired layers:

- **Command orchestration** (`cmd/tsuku/install_deps.go`, `install_lib.go`): plan
  generation messages, dependency skip notices, completion lines — 12 fmt.Printf calls
- **Install internals** (`internal/install/manager.go`, `library.go`, `bootstrap.go`):
  runtime dependency resolution, library install notices — 12 fmt.Printf calls
- **Verify output** (`cmd/tsuku/verify.go`): step-by-step verification sub-steps
  printed during post-install check — 15 fmt.Printf calls
- **Action verbosity**: actions use `reporter.Log()` (permanent lines) for every
  sub-step (extracting, linking, running commands), producing many lines where only
  a status update was wanted

The result: the spinner appears once at the very end, for one action (`npm_exec`).
Everything else scrolls past as plain text.

The goal is a single status line that updates in place for the entire install
execution. Permanent lines appear only for the install start, verification, completion,
errors, and actionable notices (PATH guidance). Everything else — plan generation,
dependency resolution, action execution sub-steps — updates the same status line.

## Decision Drivers

- **Single visible line on TTY**: the spinner must own the terminal for the full
  duration of `tsuku install` and `tsuku update`
- **Clean CI output**: non-TTY mode must still produce useful sequential lines
  (start, verify, done per tool), not silence
- **Shared reporter instance**: recursive dependency installs must use the same
  reporter, not create independent spinners
- **Minimal interface churn**: changes to `internal/install/` should be additive,
  not require touching every call site simultaneously
- **No regression on failure**: when an install fails, all relevant diagnostic
  output must still reach the user

## Considered Options

### Decision 1: Reporter injection into internal/install/

`internal/install/Manager` is the central coordinator for tool installation state.
Its `InstallWithOptions`, `InstallLibrary`, and related methods currently call
`fmt.Printf` directly for progress output. To produce single-line output on TTY,
these methods need access to the same `progress.Reporter` instance created at command
entry. Three injection strategies were evaluated.

#### Chosen: Struct field + SetReporter()

Add a `reporter progress.Reporter` field to `Manager` and a `SetReporter(r progress.Reporter)`
method. Internally, methods call `m.getReporter()` which returns the stored reporter or
`progress.NoopReporter{}` if nil. This mirrors exactly how `internal/executor/Executor`
already handles the reporter (set via `exec.SetReporter(reporter)` in `install_deps.go`
at line 358).

The command layer currently creates the reporter inside `installWithDependencies` (at the
bottom of the function, just before creating the executor). Since `installWithDependencies`
is recursive, each call today creates its own TTYReporter, which prevents a single shared
spinner. The correct wiring requires creating the reporter once at the outermost entry
point (`runInstallWithTelemetry`) and threading it as a new parameter into
`installWithDependencies`. Each invocation then calls `mgr.SetReporter(reporter)` and
`exec.SetReporter(reporter)` with the shared instance. `defer reporter.Stop()` moves to
the top level so it fires once after the entire recursive install tree completes.

#### Alternatives Considered

**Method parameters**: Add `reporter progress.Reporter` as an explicit parameter to each
Manager method that produces output (`InstallWithOptions`, `InstallLibrary`, etc.).
Rejected because it requires simultaneous signature changes across 5+ files and 8+ call
sites, violating the minimal-churn constraint. The recursion cascade also spreads the
change into outer `cmd/tsuku` functions.

**context.Context**: Store the Reporter in a context value and thread `ctx` through
Manager method signatures. Rejected because Go explicitly discourages `context.WithValue`
for service injection (per the context package documentation), and Manager has no
`context.Context` parameters today — adopting context is a larger churn than method
parameters.

---

### Decision 2: Output classification

With the Reporter available at all output sites, each existing output call must be
mapped to the right channel. The `progress.Reporter` interface provides: `Status(msg)`
(ephemeral, overwrites on TTY, no-op on non-TTY), `Log(format, args)` (permanent on
both TTY and non-TTY), `DeferWarn(format, args)` (permanent, deferred to after `Stop()`),
and `Warn` (immediate permanent). On non-TTY CI runs, `Status()` is a complete no-op,
so anything classified as Status() becomes invisible in CI.

The cross-validation conflict: D2 originally classified all orchestration labels
as `Status()`, but D4 assumed CI would see Log() lines from the orchestrator. Resolved
by keeping the install-start ("Installing X@Y...") and install-done ("X@Y installed")
lines as `Log()` and classifying only the intermediate labels as `Status()`.

#### Chosen: Start/done Log(); intermediates Status(); notices DeferWarn(); detail silence

| Category | Channel | Rationale |
|----------|---------|-----------|
| "Installing X@Y..." (start) | `Log()` | CI signal: user needs to know what started |
| "X@Y installed" (done) | `Log()` | CI signal: completion confirmation |
| Plan generation, dep-checking, dep-resolving | `Status()` | Ephemeral activity; CI doesn't need it |
| "Installing dep Y..." (recursive) | `Status()` | Intermediate; covered by start/done at each level |
| Library install notice ("Installed library to:") | silence | Internal sub-step; duplicates state output |
| Verify top-level ("Verifying X@Y") | `Log()` | Takes time; CI signal |
| Verify sub-steps (Tier 1/2, step-by-step) | silence (Verbose:false) | Diagnostic traces, not user output |
| Action sub-steps (extracting, running, linking) | `Status()` | In-progress activity; CI-invisible is fine |
| Bulk operation counts ("Linking N files from Y") | `Log()` | Milestone with user value |
| Per-file lines ("+ Linked: libgcc_s.so.1") | silence | Tight-loop detail; flickers spinner on TTY |
| "📍 Installed to:", "🔗 Wrapped N binaries:" | silence | Emoji violation; covered by "installed" Log() |
| PATH guidance ("To use the installed tool...") | `DeferWarn()` | Actionable; correct position is after Stop() |
| Command output ("Output: ...") | `Log()` | Diagnostic signal, especially on failure |
| Retry notices ("Retry N/M after...") | `Log()` | Anomaly worth permanent record |
| Errors | stderr (unchanged) | Always permanent |

#### Alternatives Considered

**Keep all as Log()**: Status quo. Produces 40+ lines per install on non-TTY.
Unacceptable — this is the exact problem being solved.

**Convert all to Status()**: Silences CI entirely for successful installs. Operators
running in CI pipelines get no confirmation that tools were installed. Rejected.

---

### Decision 3: Verify output during install

After installing a tool, `install_deps.go` calls `RunToolVerification` with
`opts.Verbose = true` today. This causes `cmd/tsuku/verify.go` to print 8-12
sub-step lines per tool (`Step 1: Verifying...`, `Running: node --version`,
`Output: v25.9.0`, `Integrity: OK`, etc.), multiplied by the number of dependencies.

#### Chosen: Verbose:false from install path

Change the single call site in `install_deps.go` to pass `Verbose: false`.
The existing `RunToolVerification` respects this flag and suppresses sub-step
`printInfo` calls. The `reporter.Log("Verifying %s@%s")` line at line 569 of
`install_deps.go` is not controlled by Verbose and remains as the single CI-visible
verification signal. Failure detail comes from the error returned by
`RunToolVerification` itself — the error message includes the command output,
expected/actual version patterns, and binary integrity failures. Sub-steps don't
need to have been printed for the failure to be diagnosable.

The `tsuku verify` command continues to default to `Verbose: true`, giving users
full diagnostic output on demand.

#### Alternatives Considered

**Route sub-steps through Status()**: Requires threading `Reporter` into
`RunToolVerification` and its call chain (a larger refactor) and still provides
zero CI context since `Status()` is a no-op on non-TTY. The error chain already
contains the failure detail. Strictly more work for no additional user value.

**Buffer sub-steps, flush on failure**: Accumulate verify sub-steps in memory and
print them only on failure. Adds buffer management complexity for information already
present in the structured error return. Rejected.

---

### Decision 4: Action sub-step verbosity

After PR #2280, actions in `internal/actions/` call `reporter.Log()` for every
sub-step. On TTY, each `Log()` call clears the spinner, prints a permanent line,
and resumes the spinner. A typical `tsuku install` with one dependency runs through
~8 action steps, each with 3-8 sub-steps — producing the 40+ lines seen in the
failure report.

#### Chosen: Extraction/command-in-progress → Status(); per-file loops → silence; milestones → Log()

Specific changes by category:

**Convert to Status():**
- `extract.go`: "Extracting: %s" → `Status()`
- `run_command.go`: "Running: %s" → `Status()`
- `install_binaries.go`: "Installing directory tree to: %s", "Copying directory tree..." → `Status()`

**Remove entirely (silence):**
- `extract.go`: "Format: %s", "Strip dirs: %d" — recipe parameters, not news
- `link_dependencies.go`: individual "Linked: %s", "Linked (symlink): %s", "Already linked: %s" per-file lines
- `install_binaries.go`: per-file "Installed (executable): %s", "Installed: %s"
- `install_libraries.go`: per-file "Installed symlink: %s", "Installed: %s"
- `run_command.go`: "Description: %s", "Working dir: %s", "Command executed successfully"

**Keep as Log():**
- `download_file.go`: "Using cached: %s", "Retry N/M after...", "Downloading %s"
- `link_dependencies.go`: "Linking N library file(s) from Y" (bulk count, milestone)
- `install_binaries.go`: consolidated completion line with output count
- `run_command.go`: "Output: %s" (command stdout has diagnostic value on failure)
- `run_command.go`: "Skipping (requires sudo): %s" (anomaly worth recording)

**Note on Status() call sites**: `reporter.Status()` takes a `string`, not a format
string. Callers that need formatting must use `reporter.Status(fmt.Sprintf(...))`.

#### Alternatives Considered

**Convert all Log() to Status()**: Silences CI output from actions entirely.
Retry notices, cache hits, and command output all carry diagnostic value that CI
operators need. Rejected.

**Keep all as Log()**: Status quo — the problem being solved. Rejected.

**Convert per-file lines to Status()**: A tight loop calling Status() on each of
6-20 files causes rapid spinner flicker on TTY with no readable content. Silence is
strictly better. Rejected.

## Decision Outcome

**Chosen: 1A + 2-resolved + 3B + 4-classified**

### Summary

`internal/install/Manager` gains a `reporter progress.Reporter` field and a
`SetReporter()` method, mirroring the existing executor pattern. The reporter is
created once in `runInstallWithTelemetry` and passed as a new parameter to
`installWithDependencies`. Each invocation of `installWithDependencies` calls
`mgr.SetReporter(reporter)` and `exec.SetReporter(reporter)` with the shared instance,
so the single spinner owns the terminal for the entire recursive install tree.
`defer reporter.Stop()` moves to `runInstallWithTelemetry` so it fires once after the
full tree completes, not after each recursive call. Manager methods (`InstallWithOptions`,
`InstallLibrary`, `bootstrap`) replace their `fmt.Printf` calls with
`m.getReporter().Status(...)` for in-progress activity and silence for per-item
detail.

The output shape for a successful `tsuku install serve` with one dependency becomes:

**TTY**: one spinner line updating throughout — `⣾ Downloading nodejs@25.9.0...`,
`⣾ Extracting node-v25.9.0-linux-x64.tar.gz`, `⣾ Verifying nodejs@25.9.0`,
`⣾ Verifying serve@14.2.6` — replaced by two permanent Log lines at the end:
`Installing serve@14.2.6` and `serve@14.2.6 installed`.

**Non-TTY/CI**: four permanent lines: `Installing serve@14.2.6`, `Verifying nodejs@25.9.0`,
`Verifying serve@14.2.6`, `serve@14.2.6 installed`. Errors, retries, and command output
also appear as permanent lines.

The verify path change is a one-line change at the `install_deps.go` call site
(`Verbose: false`). The action verbosity reduction touches ~25 `reporter.Log()` calls
across 6 action files, converting them to `Status()` or removing them.

Failure behavior is unchanged: errors bubble up through return values and print via the
existing error-reporting path. The DeferWarn for PATH guidance appears after `Stop()`
clears the spinner, so it always prints below the completion line.

### Rationale

The decisions reinforce each other at the seams. SetReporter() on Manager allows the
same reporter instance to reach library installs and runtime dependency resolution
without touching call sites that don't need output (state management, symlink creation).
The Status()/Log() split means non-TTY CI output is controlled entirely by which calls
use Log() — adding a new Log() call is the explicit choice to make something CI-visible.
The Verbose:false change is safe precisely because the verification error messages are
self-contained — they don't depend on the sub-steps having been printed. And silencing
per-file loops rather than converting them to Status() avoids spinner flicker while
keeping the bulk-count milestone lines that tell CI operators how many files were
processed.

## Solution Architecture

### Overview

A single `progress.Reporter` instance is created once at the outermost entry point and
flows through every layer of the install execution. All intermediate output becomes
ephemeral `Status()` updates that overwrite the same terminal line on TTY. Permanent
output is limited to install-start, verification, install-done, errors, and
deferred notices.

### Components

```
cmd/tsuku/install_deps.go
  └── runInstallWithTelemetry()
        ├── reporter := progress.NewTTYReporter(os.Stderr)   — MOVED: created here, once
        ├── defer reporter.Stop()                             — MOVED: fires after full tree
        └── installWithDependencies(..., reporter)            — NEW: reporter as parameter

      installWithDependencies(..., reporter progress.Reporter)   — NEW parameter
        ├── mgr.SetReporter(reporter)    — NEW: wires reporter into install orchestration
        ├── [recursive dep calls pass reporter through]
        │
        ├── internal/install/Manager
        │     ├── SetReporter(r) / getReporter()    — NEW: stored on struct
        │     ├── InstallWithOptions()               — replaces fmt.Printf with Status()/silence
        │     ├── InstallLibrary()                   — replaces fmt.Printf with Status()/silence
        │     └── bootstrap.go                       — replaces fmt.Printf with Status()/silence
        │
        ├── exec.SetReporter(reporter)               — UNCHANGED pattern, shared instance
        ├── internal/executor/Executor               — already wired
        │     └── internal/actions/*                 — Log()→Status() + silence per Decision 4
        │
        ├── reporter.Log("Installing %s@%s")         — permanent start line
        ├── reporter.Status(fmt.Sprintf(...))         — intermediate activity
        ├── RunToolVerification(Verbose: false)        — CHANGED: false instead of true
        │     └── suppress sub-step printInfo calls
        │
        └── reporter.Log("%s@%s installed")          — permanent done line
              reporter.DeferWarn("PATH guidance")    — after Stop() fires at top level
```

### Key Interfaces

```go
// cmd/tsuku/install_deps.go — signature change to thread reporter
func installWithDependencies(toolName, reqVersion, versionConstraint string,
    isExplicit bool, parent string, visited map[string]bool,
    telemetryClient *telemetry.Client, reporter progress.Reporter) error

// internal/install/manager.go (additions)
func (m *Manager) SetReporter(r progress.Reporter)
func (m *Manager) getReporter() progress.Reporter  // returns NoopReporter{} if nil

// internal/install/library.go — no signature changes; uses m.getReporter() internally
// internal/install/bootstrap.go — no signature changes; uses m.getReporter() internally

// progress.Reporter — unchanged from PR #2280
// Status(msg string)                      -- ephemeral
// Log(format string, args ...any)         -- permanent
// DeferWarn(format string, args ...any)   -- permanent, after Stop()
// Stop()                                  -- clears spinner
```

### Data Flow

```
install command invoked
        ↓
runInstallWithTelemetry()
  reporter := progress.NewTTYReporter(os.Stderr)   ← created once
  defer reporter.Stop()                              ← fires after full tree
        ↓
installWithDependencies(..., reporter)
        ↓
[recursive dep loop — each call receives same reporter]
  installWithDependencies("dep-Y", ..., reporter)
    mgr.SetReporter(reporter)
    exec.SetReporter(reporter)
    reporter.Log("Installing dep-Y@V")        ← permanent
    dep install runs (executor + actions)     ← spinner updates (Status calls)
    reporter.Log("Verifying dep-Y@V")         ← permanent
    reporter.Log("dep-Y@V installed")         ← permanent
        ↓
main tool: installWithDependencies("X", ..., reporter)
  mgr.SetReporter(reporter)
  exec.SetReporter(reporter)
  reporter.Log("Installing X@Y")             ← permanent line 1
  main tool install runs                     ← spinner updates
  reporter.Log("Verifying X@Y")             ← permanent
  reporter.Log("X@Y installed")             ← permanent line N
        ↓
[runInstallWithTelemetry defer fires]
reporter.Stop()
reporter.FlushDeferred()                     ← PATH guidance if needed
```

## Implementation Approach

### Phase 1: SetReporter on Manager + reporter parameter threading

Add `reporter progress.Reporter` field and `SetReporter()` / `getReporter()` to
`internal/install/manager.go`. Replace the fmt.Printf calls in `manager.go`,
`library.go`, and `bootstrap.go` with `m.getReporter().Status(...)` or silence per
the Decision 2 classification table.

Move reporter creation from `installWithDependencies` to `runInstallWithTelemetry` and
add `reporter progress.Reporter` as the last parameter of `installWithDependencies`.
Each invocation calls `mgr.SetReporter(reporter)` and `exec.SetReporter(reporter)`.
`defer reporter.Stop()` moves to `runInstallWithTelemetry`.

Deliverables:
- `internal/install/manager.go` — SetReporter, getReporter, fmt.Printf → Status/silence
- `internal/install/library.go` — fmt.Printf → silence
- `internal/install/bootstrap.go` — fmt.Printf → silence
- `cmd/tsuku/install_deps.go` — reporter moved to top level, added as parameter, mgr.SetReporter() call

### Phase 2: Install start/done Log lines

In `install_deps.go` and `install_lib.go`, ensure the install-start and install-done
lines use `reporter.Log()` (not `printInfof` which goes to stdout), replace intermediate
orchestration labels with `reporter.Status()`, remove the emoji completion lines
(`📍 Installed to:`, `🔗 Wrapped N binaries:`), and move PATH guidance to
`reporter.DeferWarn()`.

Deliverables:
- `cmd/tsuku/install_deps.go` — reclassify all remaining fmt.Printf / printInfof calls
- `cmd/tsuku/install_lib.go` — same

### Phase 3: Verify sub-step suppression

Change the `RunToolVerification` call in `install_deps.go` from `Verbose: true` to
`Verbose: false`.

Deliverables:
- `cmd/tsuku/install_deps.go` — one-line Verbose flag change

### Phase 4: Action verbosity reduction

Convert or remove ~25 `reporter.Log()` calls across 6 action files per the Decision 4
classification table.

Deliverables:
- `internal/actions/extract.go`
- `internal/actions/link_dependencies.go`
- `internal/actions/run_command.go`
- `internal/actions/install_binaries.go`
- `internal/actions/install_libraries.go`
- Any additional action files with Log() calls producing per-file-loop output

### Phase 5: Tests and validation

Update functional tests (`internal/executor/install_output_test.go`) to reflect
the reduced Log() count. Validate non-TTY output shape against the expected 3-5 lines
for a binary install. Add a test asserting that a recursive dependency install
produces Log() lines for each dep's verify and completion, not for sub-steps.

Deliverables:
- `internal/executor/install_output_test.go` — updated assertions

## Security Considerations

This design introduces no new security dimensions. It is a refactoring of output routing — replacing direct `fmt.Printf` calls with `Reporter` method calls across the install orchestration and action layers. All artifact handling (downloads, extraction, checksum verification), permission scope, supply chain trust, and data exposure are identical before and after implementation. The changes are confined to output formatting, a layer already below authentication, download verification, and secret handling. No new external dependencies, filesystem permissions, network calls, or sensitive data flows are introduced.

## Consequences

### Positive

- Single status line on TTY for the entire install duration, including dependency
  resolution, library installs, and verification
- CI output reduced from 40+ lines to 4-6 lines for a typical binary recipe with
  one dependency
- Failure output unchanged: errors print immediately via the existing error path;
  command output (kept as Log()) is visible in CI logs
- `tsuku verify --verbose` still produces full diagnostic output on demand
- No changes to `progress.Reporter` interface or `TTYReporter` implementation

### Negative

- Per-file operation detail (which files were linked, which binaries were installed)
  is no longer visible in normal output. Users debugging a failed install must run
  `tsuku verify --verbose` or set a debug flag to see it.
- Action authors must now know the Status()/Log()/silence classification — a new
  convention that isn't enforced by the compiler.
- `Status()` takes a plain string, not a format string, so callers use
  `fmt.Sprintf` rather than the Log-style variadic API.

### Mitigations

- Document the Status()/Log()/silence classification in a code comment at the
  `progress.Reporter` interface declaration
- Error messages from failing actions continue to include the operation detail
  (e.g., which file failed to link), so the silence of per-file loop lines doesn't
  hide failure context
- A future `--verbose` flag on `tsuku install` could unconditionally route Status()
  to Log() for power users who want the full output
