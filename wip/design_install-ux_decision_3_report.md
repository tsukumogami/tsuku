<!-- decision:start id="non-tty-output-behavior" status="assumed" -->
### Decision: Non-TTY Output Behavior During Install

**Context**

When stdout/stderr is not a TTY — CI systems, piped output, redirected log files —
tsuku must decide how much installation progress to emit. The new Reporter design
(modeled on niwa's reporter.go) already fixes TTY detection as a structural concern:
the Reporter is constructed once with auto-detected TTY state, and `Status()` is a
no-op on non-TTY. All remaining output passes through `Reporter.Log()` or
`Reporter.Warn()`, which always emit on both TTY and non-TTY.

Today's executor.go emits 20-50 raw `fmt.Printf` lines that bypass quiet mode entirely.
A typical `tsuku install kubectl` produces 8-12 lines; a tool with two dependencies
produces 25-40. For build recipes that compile from source (5-15 minute operations),
CI operators currently see a long stream of step-level messages with no clear structure.
The Reporter migration creates a clean opportunity to recategorize these messages.

The question is which messages belong in `Reporter.Log` (shown in CI) vs
`Reporter.Status` (TTY spinner only).

**Assumptions**

- All executor `fmt.Printf` progress calls will migrate to Reporter.Log/Status
  as part of this work. If the migration is partial, non-TTY behavior will be
  inconsistent. Consequence: the decision's effect is proportional to migration completeness.
- Build recipes with 5-15 minute compile times are a real use case. The presence of
  `cargo_install`, `npm_install`, and `pipx_install` actions in the executor confirms this.
- Running in --auto mode (no user confirmation available for this decision).

**Chosen: Per-Phase Text Lines**

In non-TTY mode, emit one `Reporter.Log` line per phase transition per tool, with no
ANSI sequences, no `\r` overwrites, and no step counters. Specifically:

- "Downloading \<tool\> \<version\>" — when a download phase begins
- "Building \<tool\> \<version\>" — when a compile/build phase begins (cargo_install, npm_install, etc.)
- "Extracting \<tool\>" — when extraction begins
- "Installing \<tool\>" — when binary installation begins
- "Verifying \<tool\>" — when verification begins
- "\<tool\> \<version\> installed" — final success line (always)

Per-step messages ("Step 3/7: chmod", "Added /path/bin to ExecPaths", checksum verification
details) use `Reporter.Status` — they appear as spinner updates on TTY and are silent on
non-TTY. Warnings and errors always use `Reporter.Log`/`Reporter.Warn` regardless of mode.

The caller's responsibility is to emit one Log call at each phase entry. This requires
adding phase-awareness to the install orchestration in install_deps.go and executor.go,
but the Reporter interface itself requires no changes.

**Rationale**

Option 1 (fully silent) creates a hung-process problem for build recipes in CI. A silent
5-15 minute job gives CI operators no way to distinguish a healthy long build from a
deadlocked process. This makes CI debugging significantly harder and is likely to cause
unnecessary job cancellations. The niwa pattern works for niwa's short operations, but
tsuku's build recipes require different treatment.

Option 3 (one line per tool) solves "did it start?" but still leaves CI silent during
the body of a long build. The gap between "Installing node 20.11.0..." and completion
can be 10 minutes, which is functionally indistinguishable from Option 1 during the
compile phase — exactly where CI operators need feedback.

Option 2 (per-phase) produces 4-8 plain-text lines for a simple binary install and
10-20 lines for a multi-dependency build. This is clean enough for CI logs and
specific enough for debugging: the last log line before silence tells the operator
exactly where execution stalled. Phase lines are the unit at which most CI failures
occur (download timeouts, extraction failures, compile errors), so they provide
maximum diagnostic value at minimum verbosity.

The implementation fits the Reporter contract cleanly: phase transitions go to Log,
step details go to Status. No scattered TTY checks, no new Reporter methods.

**Alternatives Considered**

- **Fully Silent Except Summary**: Only warnings, errors, and the final "installed"
  line appear in CI. Rejected because 5-15 minute build recipes produce zero feedback
  during execution, making CI jobs look hung and forcing operators to rely solely on
  timeout settings to detect real failures.

- **One Log Line Per Tool**: One "Installing..." line at start plus completion. Rejected
  because it leaves the same 5-15 minute silence gap as Option 1 during the compile
  phase — the exact phase where CI users most need visibility. Marginally better than
  Option 1 only for confirming the job started.

**Consequences**

What changes:
- executor.go step-level `fmt.Printf` calls migrate to `Reporter.Status` (silent on non-TTY)
- New phase-entry `Reporter.Log` calls added at key phase transitions in install orchestration
- progress/progress.go's scattered `ShouldShowProgress()` TTY check is replaced by Reporter construction
- CI output for a typical binary install goes from 8-12 unstructured lines to 4-6 structured phase lines

What becomes easier:
- CI debugging: the last log line before a failure identifies the failed phase
- Pipe usage: clean plain-text lines, no ANSI pollution in `tee` logs
- CI timeout tuning: operators can set phase-level expectations rather than total-job timeouts

What becomes harder:
- Caller code needs phase awareness — install_deps.go and executor.go must distinguish
  phase-entry calls from within-phase step calls. This is a one-time structural change.
<!-- decision:end -->
