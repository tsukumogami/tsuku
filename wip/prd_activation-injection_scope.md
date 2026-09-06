# PRD Scope: activation-injection

Upstream: `docs/briefs/BRIEF-activation-injection.md`.

## Phase 1 and Phase 2 are already discharged

Phases 1 (scope) and 2 (discover) are not re-run. This chain's own
`/shirabe:explore` fanned out six research leads over exactly this question
and its findings are at `wip/explore_activation-injection_findings.md`, with
the per-lead reports under `wip/research/explore_activation-injection_r1_*`.
Re-running a discovery fan-out against a question already answered by six
agents would produce a second, drifting copy of the same findings.

What the explore covered, mapped onto what Phase 2 would have asked:

| Phase 2 would ask | Answered by |
| --- | --- |
| Where do externally-supplied values reach path construction? | `lead-path-sinks` |
| Where does tsuku emit shell-evaluated text, and how is it quoted? | `lead-shell-sinks` |
| What can a hostile repository actually set? | `lead-input-inventory` |
| What guards exist and what do they cover? | `lead-guard-coverage` |
| Does the class extend into process execution? | `lead-exec-sinks` (bounded negative) |
| What do the design and test records claim? | `lead-record-claims` |

Three findings arrived after the explore closed and are recorded in the
findings file: the version component escaping at a third sink that execs, the
registry fetch-URL and cache-write path, and the searched negative on the
misapplied version validator.

## What the PRD has to settle that the explore did not

1. **The blast radius of a rejection.** Unbundled during the BRIEF hop and
   deferred here as the brief's only substantive Open Question.
2. **Which requirements are acceptance-testable, and by what fixture.** The
   explore produced discriminating fixtures; the PRD has to turn them into
   criteria that a wrong implementation fails.
3. **Where the boundary between this feature and its five deferred siblings
   falls in requirement terms**, not just in prose.

## Constraint inherited from a sibling chain

A sibling chain's requirement that the declared version be reported verbatim
by the resolver means nothing may normalise or rewrite versions on the read
path. Rejecting at load is compatible; sanitising at the resolver is not. Any
requirement here must reject, never repair.
