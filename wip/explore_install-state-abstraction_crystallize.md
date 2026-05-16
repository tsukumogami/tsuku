# Crystallize Decision: install-state-abstraction

## Chosen Type

**Design Doc**

## Rationale

The exploration surfaced two viable architectural shapes (Candidate C —
context.Context-based attribution with a recursion-collapse refactor;
Candidate B — a new `internal/install/installops` Service layer) and a
defensible status-quo position (Candidate A). The remaining questions are
exactly the kind a design document's structured decomposition is built
to resolve:

1. Which shape (B, C, both, neither) to pursue.
2. How to weigh the 12-month forecast for new cross-cutting concerns
   against the upfront restructure cost.
3. Whether `state.json` exposure via `mgr.GetState().UpdateTool(...)`
   is a smell the restructure must heal or a pragmatic convenience to
   preserve.
4. How any restructure composes with the just-shipped lifecycle event
   bus and whether libraries (`InstallLibrary`) come along.

The issue (#2413) was filed with the `needs-design` label and explicit
acceptance criteria requiring a design doc that evaluates the proposal,
weighs trade-offs with concrete examples, considers `context.Context`-
based attribution threading as an alternative, and reaches an Accepted
or Rejected status. Convergence findings produced the evidence base
that design doc needs.

## Signal Evidence

### Signals Present (Design Doc)

- **Multiple competing technical approaches identified, requirements clear**:
  Candidate B and Candidate C are both technically viable; the trade-off
  between them is the central design question. (Lead 3, Lead 6)
- **The "what" is settled, the "how" is the open question**: requirements
  are clear from the issue body (reduce blast radius of cross-cutting
  threading; preserve event bus composition; consider ctx as alternative).
  The architecture is the gap. (Issue #2413, Lead 4, Lead 6)
- **Integration risk and cross-cutting concerns**: Lead 2 documented a
  recurring class of integration churn (one new cross-cutting concern
  every 4-6 weeks across 5 PRs); the architecture choice has direct
  downstream consequences. The bus interaction, library scope question,
  and `state.json` exposure question all want explicit weighing.
- **Architectural choice between known options**: 5 patterns surveyed
  (Lead 3), 3 candidate shapes carried forward (Lead 6), 2 surviving
  candidates (B and C) + status-quo. Concrete enough that a structured
  decision-decomposition can evaluate them.
- **Decisions need to live somewhere permanent**: Eliminating literal
  repository pattern, command/middleware/decorator chains, and
  OperationOptions consolidation as options is a real architectural
  choice future contributors need to find. The design doc captures
  the alternatives considered without leaving them as tribal knowledge.

### Anti-Signals Checked

- **Single obviously-right approach**: not present. Two shapes remain
  viable and the choice depends on judgment calls (forecast, taste).
- **No cross-component implications**: not present. Touches
  `internal/install/`, `cmd/tsuku/`, and composes with
  `internal/installevents/`, `internal/notices/`, `internal/telemetry/`.
- **Self-contained one-file change**: not present. Either viable
  shape touches 10-20+ files.

## Alternatives Considered

- **PRD**: Not present as a fit. Requirements are already clear from
  the issue and from Lead 2's empirical evidence. "What to build" isn't
  the gap — "how to build it (and whether to build it at all)" is.
- **Decision Record**: Considered briefly. A standalone decision record
  could work IF the choice were between two pre-defined options. But
  the exploration surfaced that the choice has internal sub-decisions
  (state.json exposure, library scope, sequencing of B and C), each of
  which is itself worth structured evaluation. A design doc holds
  multiple decisions; a decision record holds one. Design doc is
  better-fit.
- **No Artifact**: Strongly considered as the "don't do anything" answer
  the user's mandate explicitly allowed. Rejected because Lead 2's
  evidence (3 in-tree workarounds, parameter explosion, cadence of one
  concern every 4-6 weeks) shows the cumulative cost is real. The
  status-quo position is defensible inside a design doc as one of the
  alternatives weighed, but "no artifact" understates the structural
  pain enough to mislead a future contributor evaluating a similar
  refactor.
- **Spike Report**: Not present as a fit. Feasibility isn't the open
  question — both candidates B and C are clearly feasible. The question
  is which is the right shape, which is a design decision.
- **Plan**: Premature. /plan operates on an Accepted design doc; the
  design has not been written yet.

## Deferred Types

None of the deferred types (Prototype) scored well. The exploration
produced enough concrete sketches (Lead 6's before/after code in
`install_deps.go`) that a prototype is unnecessary — the design phase
can work from the sketches.
