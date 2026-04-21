# Explore Scope: install-ux

## Visibility

Public

## Core Question

How should tsuku replace its current verbose per-step log output during install,
update, and similar commands with in-place status lines that animate in a TTY and
degrade gracefully in pipes/CI? This includes unifying download progress into the
same status channel so there's one consistent output mechanism for everything that
happens during an install.

## Context

Today `tsuku install` and `tsuku update` emit a long scrolling log: every step name,
every download start, every sub-directory listing. Most of this is only useful as
transient status. The executor uses raw `fmt.Printf()` with no TTY awareness. A
`Spinner` exists in `internal/progress/` but is never used during installs. Download
progress uses a separate `progress.Writer` widget. niwa's `Reporter` (braille spinner,
TTY-aware, deferred summaries, output to stderr) is the stated reference for the
target UX.

Key existing code:
- `cmd/tsuku/install_deps.go` — install orchestration, `installWithDependencies()`
- `internal/executor/executor.go` — step execution loop, raw `fmt.Printf()` at lines 462, 536, 543
- `internal/progress/spinner.go` — existing Spinner (unused during install)
- `internal/progress/progress.go` — download progress Writer (separate mechanism to unify)
- niwa `internal/workspace/reporter.go` — reference implementation

## In Scope

- `tsuku install`, `tsuku update`, and any command that emits per-step log noise
- TTY detection, quiet mode, non-TTY fallback behavior
- Download progress unified into the same status line (no separate progress bar widget)
- Step status (resolving, downloading, extracting, installing, verifying)
- Dependency resolution status

## Out of Scope

- Error output formatting (separate concern)
- `tsuku list`, `tsuku outdated`, and read-only commands
- Self-update output (different flow)

## Research Leads

1. **What does niwa's output UX look like in practice during a real multi-step operation?**
   Understand the exact sequence of what a user sees — spinner text at each stage,
   how completion is signaled, what gets permanently printed vs. cleared. Need concrete
   examples, not just API surface.

2. **How should a reporter abstraction propagate through tsuku's executor call chain?**
   The executor is called many levels deep. Does it thread via parameter, interface in
   context, or package-level state? What's the least-invasive wiring that still gives
   step runners access to the status channel?

3. **What should non-TTY and quiet-mode output look like?**
   Pipes, CI, and shell scripts need clean output. Does each step get one log line?
   Does it go mostly silent? Is structured output (JSON) relevant here?

4. **What information density is right for the happy path?**
   Currently every step name prints. Should the new UX show step names, or just a
   single "Installing kubectl 1.29..." line that updates in place? What about dependency
   resolution — shown or hidden?

5. **How do peer CLI tools handle unified install progress (step + download in one line)?**
   cargo, brew, npm each have a different approach. What patterns do users already
   understand, and which handle the "downloading 40MB then extracting" sequence well?

6. **Is there evidence of real demand for this, and what do users do today instead?** (lead-adversarial-demand)
   You are a demand-validation researcher. Investigate whether evidence supports
   pursuing this topic. Report what you found. Cite only what you found in durable
   artifacts. The verdict belongs to convergence and the user.

   ## Visibility

   Public

   Respect this visibility level. Do not include private-repo content in output
   that will appear in public-repo artifacts.

   ## Six Demand-Validation Questions

   Investigate each question. For each, report what you found and assign a
   confidence level (High/Medium/Low/Absent).

   1. Is demand real? Look for distinct issue reporters, explicit requests,
      maintainer acknowledgment in tsuku's issue tracker.
   2. What do people do today instead? Look for workarounds in issues, docs,
      or code comments (e.g., --quiet usage, piping output).
   3. Who specifically asked? Cite issue numbers, comment authors — not paraphrases.
   4. What behavior change counts as success? Look for acceptance criteria or
      stated outcomes in issues or linked docs.
   5. Is it already built? Search the codebase for prior implementations or
      partial work toward in-place/spinner output during installs.
   6. Is it already planned? Check open issues, design docs, or roadmap items.
