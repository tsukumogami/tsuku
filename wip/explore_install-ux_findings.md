# Exploration Findings: install-ux

## Core Question

How should tsuku replace its current verbose per-step log output during install,
update, and similar commands with in-place status lines that animate in a TTY and
degrade gracefully in pipes/CI? This includes unifying download progress into the
same status channel so there's one consistent output mechanism for everything that
happens during an install.

## Round 1

### Key Insights

- **niwa's Reporter is directly adoptable** (niwa-ux-in-practice): Background goroutine at 100ms
  ticks, `\r\033[K` in-place redraws, TTY auto-detected once at construction, Status is no-op
  on non-TTY, DeferWarn/FlushDeferred for clean summaries. Well-tested. No invention needed
  for the core pattern.

- **Wiring point is already laid** (executor-wiring): ExecutionContext has an unused `Logger`
  field (line 429) added with the same pattern in mind. Adding a `Reporter` interface field
  alongside it propagates to all 384+ action callsites with zero signature changes — actions
  already receive `execCtx` as their first parameter.

- **Download progress unification is structurally feasible** (information-density): Replace
  `progress.Writer` instantiation in `httputil.HTTPDownload()` with a progress callback that
  calls `Reporter.Status(...)`. Three separate messages ("Step 1/6: download_file" →
  "Downloading: url" → progress bar widget) collapse into one unified status line.

- **Step names should be eliminated on the happy path** (information-density, peer-cli-patterns):
  cargo, brew, npm all show semantic phases + progress, not action names. Dropping step names
  and step counters is ~70% of the output reduction. "Step 2/6: extract" has zero user value.

- **Actions lack a description interface** (information-density — surprising): Actions have
  `Name()` returning `"download_file"` but no `UserLabel(params)` method. Showing "Downloading
  kubectl 1.29.3" requires either adding `UserLabel(params) string` to the Action interface, or
  constructing messages from plan metadata in the executor. This is the central new design surface.

- **Demand is internal but the gap is real** (adversarial-demand): `internal/progress/spinner.go`
  exists, is functional, is TTY-aware, and is never called during install. `--quiet` (issue #16)
  is a workaround symptom. The unused-but-built Spinner and the workaround flag signal a known
  gap, not a speculative feature request.

- **Non-TTY quiet mode is currently broken** (non-tty-quiet): `--quiet` suppresses `printInfof()`
  but not executor `fmt.Printf()` calls. Piping `tsuku install foo` still dumps step-by-step
  noise. The niwa pattern fixes this structurally — Status becomes a no-op on non-TTY, so
  transient output disappears without needing `--quiet` at all.

### Tensions

- **Silent in non-TTY vs. something for CI**: niwa's non-TTY is "Status is no-op, only Log
  lines survive." CI gets zero progress during a 5-minute build recipe. Cargo uses the same
  pattern. Open design call: is zero-except-summary acceptable for CI?

- **Percentage vs. "downloading..."**: Percentage requires Content-Length (not always present).
  Bytes-transferred works universally. npm/pnpm skip percentages entirely. Right unit is a
  design decision.

- **UserLabel on Action vs. executor-constructed messages**: Adding `UserLabel(params) string`
  to the Action interface is clean but touches 30+ implementations. Inferring from plan metadata
  is simpler but loses action-specific specificity. Central architectural decision.

### Gaps

- Build actions (`cargo_build`, `go_build`, `cmake_build`, 10–300 seconds): whether to pipe
  compiler output as transient Status lines or just show a spinner — no peer-CLI consensus.
- Existing `internal/progress/spinner.go` interface fit with Reporter pattern not verified.
- No external user demand evidence — internally identified gap, not user-reported pain.

### Decisions

- Adopt niwa Reporter pattern as the architecture reference (not invent a new abstraction).
- Unify download progress into the Reporter status channel (no separate progress bar widget).
- Open questions (non-TTY granularity, percentage vs. bytes, UserLabel approach, build verbosity)
  are design decisions to resolve in the design doc, not research gaps.

### User Focus

Ready to decide. Enough is known about the reference architecture, wiring path, information
density goal, and download unification approach to write a design doc. Remaining questions
are design decisions, not unknowns.

## Accumulated Understanding

The current install output problem is well-understood: tsuku emits 20–50+ lines per install
via raw `fmt.Printf()` scattered across the executor and 30+ action implementations. The
output bypasses `--quiet` partially, is not TTY-aware, and uses a separate `progress.Writer`
widget for downloads that doesn't coordinate with step output.

The solution shape is clear: adopt niwa's Reporter pattern (background goroutine, `\r\033[K`
in-place updates, TTY auto-detection, Status/Log/Defer abstraction). Wire it through
ExecutionContext (the existing Logger field proves this was anticipated). Replace the separate
download `progress.Writer` with Reporter.Status callbacks in `httputil.HTTPDownload()`. Eliminate
step names and step counters from happy-path output; show semantic phases instead.

Key design decisions still open: (1) what to show in non-TTY/CI (fully silent or per-phase
text lines), (2) whether to show download percentage vs. bytes-only, (3) whether the Action
interface gains a `UserLabel(params)` method or the executor constructs descriptions from plan
metadata, (4) how to handle long-running build actions (compiler output or spinner only).

The implementation is significant (~384 fmt.Printf call sites, new Reporter infrastructure,
Action interface extension) but well-scoped. A design doc should make the architectural
decisions and specify the interface contract before implementation begins.

## Decision: Crystallize
