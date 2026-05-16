# Decision 3 Report: Library Scope in Install Restructure

**Design**: `docs/designs/DESIGN-install-state-abstraction.md`
**Tier**: 3 (standard, fast path)
**Mode**: --auto
**Status**: decided-conditionally

## Question

Does any restructure include `InstallLibrary`, or do libraries stay parallel
with separate state and no event bus integration?

## Options

- **(a) Libraries out of scope, stay parallel.** Whatever shape Decision 1
  picks covers tools only. `internal/install/library.go` and
  `cmd/tsuku/install_lib.go` untouched. Libraries keep `State.Libs` separate
  from `State.Tools` and remain invisible to the lifecycle event bus.

- **(b) Libraries join the chosen restructure.** Whatever shape Decision 1
  picks (B Service layer, C ctx-attribution, or A status quo) extends to
  `InstallLibrary`. Uniform pipeline for tool and library installs.

- **(c) Follow-up design.** Tools-only restructure now, library
  restructure deferred to a separate design doc and follow-up issue.

- **(d) Bus integration only, skip restructure for libraries.** Add
  lifecycle events to `InstallLibrary` regardless of Decision 1, but do
  not pull libraries into any Service/ctx restructure.

## Decision Criteria and Scoring

| Criterion | (a) Parallel | (b) Join | (c) Follow-up | (d) Bus only |
|---|---|---|---|---|
| 1. Respect scope (issue #2413 framing) | strong yes | violates | partial — commits later | yes |
| 2. Avoid known gaps (two pipelines for future concerns) | leaves gap | closes gap | closes gap eventually | partial — events only |
| 3. Symmetry of abstraction | asymmetric | symmetric | asymmetric in current PR | asymmetric structurally |
| 4. Migration discipline (no scope creep) | best | worst | medium | good |
| 5. Recoups PR #2412 threading cost for libraries | no | yes | eventually | partial — `Source` already threaded becomes useful |
| 6. Blast radius | smallest | largest | small now / total same | small |

Key empirical points from the exploration:

- Libraries have **meaningfully different semantics**: no symlinks, no
  `RequiredBy`, `UsedBy` instead, separate `State.Libs` map, no `Activate`,
  no rollback symmetry. Forcing a uniform Service shape onto these
  semantics is a known risk surfaced as Lead 1's "asymmetric methods —
  intentional invariants or accidents?" gap.
- The `Source` parameter is already threaded through `installLibrary`
  (per Lead 2's observation about PR #2412) but currently does nothing
  there. The threading cost has been paid; the benefit has not. This
  asymmetrically favors (d) — the cheap fix that converts paid cost into
  realized benefit.
- Issue #2413 explicitly scoped libraries out as a deliverable. Driver 7
  in the design doc records this constraint.
- The lifecycle event bus design (Current) also explicitly scoped libraries
  out. Adopting (b) reopens that scoping decision.

## Coupling to Decision 1

This question's answer is partly determined by Decision 1:

- **If Decision 1 = A (status quo)**: this question is moot. There is no
  restructure to extend or not extend. The only meaningful sub-question
  becomes whether to ship (d) — add bus events for libraries as an
  isolated improvement. Decision 1 = A combined with (d) would be a
  defensible "small targeted fix" outcome for the whole design.

- **If Decision 1 = B (installops Service)**: the natural home for
  `InstallLibrary` is on the Service alongside `Install`/`Rollback`.
  However, the differing semantics (no symlinks, different state shape,
  different "used_by" attribution model) mean either the Service grows
  conditional branches, or it grows a parallel `InstallLibraryRequest`
  alongside `InstallRequest`. Both are real costs. Recommendation under
  B: choose (c) — follow-up design. Land tools on the Service in this
  design; commit to a follow-up that brings libraries in once the
  Service shape is proven on tools.

- **If Decision 1 = C (ctx-attribution + recursion collapse)**: ctx
  threading to `InstallLibrary` is mechanically cheap (it already takes
  `src installevents.Source` — adding `ctx context.Context` follows the
  same pattern). Cancellation benefit accrues to libraries too.
  Recommendation under C: choose (b) — bring libraries along. The
  marginal cost is small and the semantic mismatch (no symlinks etc.)
  is irrelevant because C doesn't introduce a Service layer that would
  have to model those semantics. Optionally also (d) — wire library
  events onto the bus — but this is a separable follow-up.

## Chosen Option

**Conditional on Decision 1:**

- **If Decision 1 = A**: choose **(d)** — add library events to the bus
  as a targeted 2-file change. Do not restructure. This converts PR
  #2412's already-paid `Source` threading cost in `installLibrary` into
  realized benefit.

- **If Decision 1 = B**: choose **(c)** — follow-up design. Tools-only
  restructure now; commit to a library follow-up. Rationale: the Service
  shape will be informed by tool install semantics; forcing library
  semantics into the same shape simultaneously risks awkward couplings
  (Lead 1's "intentional invariants vs accidents" question is unresolved
  and the Service design is the wrong place to resolve it under time
  pressure). Pair with (d) if cheap — add bus events for libraries
  during the tools-only Service migration.

- **If Decision 1 = C**: choose **(b)** — bring libraries into the ctx
  thread. ctx threading is mechanically uniform across `Install`,
  `InstallWithOptions`, `InstallLibrary`. Cancellation benefit accrues.
  Pair with (d) for bus events if not already implied by the ctx work.

**Default answer if Decision 1 is undetermined**: (c) — follow-up design.
This is the option that respects issue #2413's scoping today while
committing to closing the gap. It is robust across Decision 1 outcomes.

## Rationale

The exploration evidence converges on three constraints. First, issue
#2413 deliberately scoped libraries out and the design drivers honor
that. Second, libraries genuinely have different state semantics that
make naive uniformity costly under Service-style shapes. Third, the
ctx-attribution shape is uniquely well-suited to extending to libraries
because it doesn't model semantics — it just threads request-scoped
metadata. The combination of these three points means the right answer
depends on Decision 1's shape, not on a standalone preference. Picking
(c) as the default keeps scope disciplined and produces a smaller current
PR; picking (b) under Decision 1 = C is a cheap win; picking (d)
unconditionally is a low-cost recoupment of PR #2412's already-paid
library threading cost and is compatible with any Decision 1 outcome.

## Rejected Options

- **(a) Libraries out of scope, stay parallel — rejected.** Leaves the
  `Source` threading cost in `installLibrary` (paid in PR #2412)
  unrecouped, and leaves bus subscribers blind to library installs. A
  future audit-log subscriber would silently miss half the install
  pipeline. This is the cost the design exists to address; accepting it
  for libraries while fixing it for tools entrenches the asymmetry.

- **(b) Libraries join the chosen restructure — rejected under Decision
  1 = A or B, accepted under C.** Under A there's nothing to join.
  Under B, the Service shape would have to model two genuinely different
  install semantics simultaneously, which is the kind of premature
  generalization the exploration warned about ("forcing uniform shape
  may force awkward couplings"). Under C the rejection reverses — ctx
  threading is shape-agnostic and the same call applies cleanly to
  libraries.

## Result

```yaml
status: decided-conditionally
chosen:
  if_decision_1_is_A: d
  if_decision_1_is_B: c
  if_decision_1_is_C: b
  default: c
confidence: 0.75
rationale: >
  Libraries have meaningfully different state semantics (no symlinks,
  UsedBy not RequiredBy, separate State.Libs) so the right scope answer
  depends on what Decision 1 picks. Under a Service layer (B), library
  semantics risk awkward couplings — defer to a follow-up. Under
  ctx-attribution (C), threading is shape-agnostic and bringing libraries
  along is cheap. Under status-quo (A), only the bus-events sub-fix (d)
  is meaningful. Across all branches, libraries should eventually become
  first-class on the event bus to recoup PR #2412's already-paid Source
  threading cost in installLibrary.
conditional_on_decision_1: true
conditional_on_decision_1_explanation: >
  The choice of library scope depends on the shape Decision 1 picks. The
  natural fit of libraries into a Service layer (B) versus a ctx thread
  (C) versus no restructure (A) differs sharply because libraries
  carry different semantics from tools (no symlinks, different state
  shape). The default (c) keeps scope disciplined and commits to closing
  the gap later regardless of Decision 1.
rejected_options:
  - option: a
    reason: >
      Leaves PR #2412's already-paid Source threading cost in
      installLibrary unrecouped and entrenches two pipelines for future
      cross-cutting concerns. Even the minimum bus-events fix (d) is
      cheaper and obviates this option.
  - option: b
    reason: >
      Rejected under Decision 1 = A (nothing to join) and under Decision
      1 = B (forces Service to model two different install semantics
      simultaneously, premature generalization). Accepted under
      Decision 1 = C where ctx threading is shape-agnostic.
report_file: wip/design_install-state-abstraction_decision_3_report.md
```
