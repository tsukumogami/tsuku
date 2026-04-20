# Crystallize Decision: background-updates

## Chosen Type

Design Doc

## Rationale

What to build is clear: move `MaybeAutoApply` to a background subprocess so it no
longer blocks the command the user asked for. How to build it involves real design
decisions that need to be on record before the wip/ artifacts are cleaned:

- Where in the command lifecycle should the background spawn be placed — as an async
  detached call from `PersistentPreRun`, moved to `PersistentPostRun`, or restructured
  as part of the `check-updates` subprocess (combine check + apply in one background run)?
- How should the notice schema be extended? The `Notice` struct needs a `Kind` field
  (or equivalent) to distinguish update results from registry-refresh status. A schema
  change to a file format stored on user disk requires backward-compatible deserialization.
- How should the explicit-install-vs-auto-apply race be handled? If `tsuku install foo`
  is run while a background auto-apply for `foo` is in flight (or enqueued), which wins?
  The existing `state.json.lock` probably prevents conflicts, but this needs explicit design.
- Should the distributed registry initialization timeout be fixed in the same change?
  `main.go init()` uses `context.Background()` (no timeout) for each distributed source.
  It's a separate blocking path but closely related to the overall startup-latency story.
- Should the process group isolation gap (`SysProcAttr{Setpgid: true}`) be fixed as part
  of this work, or separately?

Architectural decisions made during exploration also need a permanent home:
- OS schedulers, systemd timers, and persistent daemons are ruled out.
- The detached-subprocess pattern in `trigger.go` is the confirmed mechanism.
- The notice system (file-backed, pull-per-command) is the correct delivery channel.
- Notices should appear after command output, not before.

## Signal Evidence

### Signals Present

- What to build is clear, how to build it is not: exploration confirmed the root cause
  (`MaybeAutoApply` in `PersistentPreRun`) and the mechanism direction (detached subprocess)
  but left several concrete design decisions open (lifecycle placement, schema change,
  race condition handling, scope of distributed registry fix).
- Technical decisions need to be made between approaches: at least three open design
  questions remain, each with multiple viable answers and different tradeoffs.
- Architecture and integration questions remain: the notice schema is an on-disk format;
  changes need backward-compatible design. The lock strategy for concurrent installs needs
  explicit handling.
- Exploration surfaced multiple viable implementation paths within the confirmed mechanism:
  `PersistentPreRun` async spawn vs `PersistentPostRun` vs merged into `check-updates`.
- Architectural decisions were made during exploration that should be on record: OS
  scheduler options eliminated; detached subprocess confirmed; notice system as delivery
  channel confirmed; peer tool patterns surveyed and documented.
- Core question is "how should we build this?" — confirmed. The "what" and "why" were
  known before exploration began.

### Anti-Signals Checked

- "What to build is still unclear (route to PRD first)": not present. The requirement —
  don't block foreground commands — was clear from the start.
- "No meaningful technical risk or trade-offs": not present. Schema backward compatibility,
  lock contention, and process lifecycle are all meaningful design risks.
- "Problem is operational, not architectural": not present. The fix requires restructuring
  how the command lifecycle works.

## Alternatives Considered

- **Decision Record**: Ranked second. Several decisions were made during exploration that
  future contributors need to understand. However, the decisions are interconnected — they
  form a design, not a single isolated choice — making a Design Doc the more appropriate
  container.
- **Plan**: Would be appropriate after the Design Doc is written. The approach is confirmed
  enough to decompose into issues, but open design decisions (schema change, lifecycle
  placement) need to be resolved first so the Plan isn't sequencing undefined work.
- **PRD**: Requirements were given as input (don't block foreground commands), not
  discovered by exploration. The tiebreaker between PRD and Design Doc favors Design Doc
  when requirements were given. The core open question is architectural.
- **No Artifact**: Rejected because architectural decisions were made during exploration
  that need to survive the wip/ cleanup. Direct implementation without a design doc risks
  making the wrong choices on schema, lifecycle, and concurrency.
