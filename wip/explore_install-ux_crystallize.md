# Crystallize Decision: install-ux

## Chosen Type

Design Doc

## Rationale

What to build is clear: replace tsuku's raw `fmt.Printf()` install output with in-place
status lines modeled on niwa's Reporter pattern, and unify download progress into the same
status channel. How to build it is not: the Reporter interface contract, ExecutionContext
wiring strategy, whether actions gain a `UserLabel(params)` method or the executor constructs
messages from plan metadata, non-TTY output granularity, and download percentage vs. bytes all
require architectural decisions that must be on record before implementation begins.

The exploration made several decisions that need to survive wip/ cleanup: adopt niwa's
Reporter as the reference architecture (not invent something new), unify download progress
into Reporter.Status() calls (no separate progress widget), add Reporter to ExecutionContext
alongside the existing Logger field. These choices shape the design space and must be
documented.

## Signal Evidence

### Signals Present

- **What to build is clear, how is not**: The target UX is defined (niwa-like spinner,
  in-place updates, clean non-TTY degradation) but the interface contract (Reporter,
  UserLabel, download callback) is not yet specified.
- **Technical decisions between approaches**: UserLabel on Action interface vs.
  executor-constructed messages; non-TTY silence vs. per-phase text lines; percentage
  vs. bytes for download progress; build-action verbosity (spinner vs. compiler output).
- **Architecture and system design questions remain**: Reporter interface contract,
  ExecutionContext wiring, httputil.HTTPDownload() integration, goroutine lifecycle in
  the executor context.
- **Multiple viable implementation paths surfaced**: Two approaches for action descriptions
  (interface extension vs. metadata inference), two approaches for non-TTY (silent vs.
  text fallback), two approaches for download granularity.
- **Architectural decisions made during exploration that should be on record**: Adopt niwa
  pattern; unify download progress into Reporter.Status(); wire via ExecutionContext.
- **Core question is "how should we build this?"**: The reference model is known, the
  interface design is not.

### Anti-Signals Checked

- **What to build is still unclear**: Not present. The UX target is well-defined.
- **No meaningful technical risk or trade-offs**: Not present. 384+ call sites, new
  interface surface, goroutine infrastructure, Action interface extension.
- **Problem is operational, not architectural**: Not present. This is a structural
  refactor of the output layer.

## Alternatives Considered

- **PRD**: Ranked lower because requirements came in as input to this exploration
  (replace verbose log with in-place status like niwa), not discovered during it.
  The what-to-build question was given; the how-to-build question is what's open.
- **Plan**: Ranked lower due to two anti-signals: technical approach is still debated
  (UserLabel vs. executor-constructed, non-TTY mode) and open architectural decisions
  must be resolved before work can be sequenced.
- **No Artifact**: Ranked lower due to three anti-signals: the change touches 384+
  call sites requiring other contributors to understand the new pattern; new interface
  contracts (Reporter, UserLabel) need documentation; architectural decisions were made
  during exploration.
