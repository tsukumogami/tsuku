```yaml
topic: nvm-data-root
chain_started: 2026-08-02T16:45:00Z
last_updated: 2026-08-02T17:06:00Z
phase_pointer: phase-2
exit: UNSET
exit_artifacts: []
visibility: Public
execution_mode: auto
max_rounds: 5
plan_execution_mode: single-pr
planned_chain:
  - design
  - plan
chain_skipped:
  - name: brief
    reason: requirements-given-upstream-no-framing-shift
  - name: prd
    reason: requirements-given-upstream-repo-precedent-design-direct
chain_ran: []
child_snapshots:
  design:
    status: Proposed
    content_hash: pending
    captured_at: 2026-08-02T17:06:00Z
```

# Scope State: nvm-data-root

Slug validated against `^[a-z0-9-]+$` as provided. `shirabe
slug-prefix-detect nvm-data-root --docs-root docs` returned
`no-prevailing-prefix`, so no prefix recommendation applies.

Visibility resolved to `Public` from the `## Repo Visibility: Public` header in
the repo's `CLAUDE.local.md`. No stale `parent_orchestration:` block was present
at session start.

Execution mode is `auto`: the dispatch brief carries an explicit autonomy mandate,
so decision points follow the recommended default and the run does not block on
author input. `--auto` does not suppress the R9 hard-finalization check.

## Phase 1: Discovery

**Framing shift (R4).** The framing did shift during exploration — the problem is
not primarily a garbage-collection bug — but that shift is already captured in the
DESIGN skeleton the `/explore` handoff wrote, and there is no accepted BRIEF or PRD
on disk for it to invalidate. No re-framing artifact is needed.

**On-disk topic artifacts.**

| Path | Present | Status |
|------|---------|--------|
| `docs/briefs/BRIEF-nvm-data-root.md` | no | — |
| `docs/prds/PRD-nvm-data-root.md` | no | — |
| `docs/designs/DESIGN-nvm-data-root.md` | yes | Proposed |
| `docs/designs/current/DESIGN-nvm-data-root.md` | no | — |
| `docs/plans/PLAN-nvm-data-root.md` | no | — |

## Chain proposal, and the Adjust applied to it

The mechanical gates fire for all four children: `/brief` on R4's first
EITHER-signal (no BRIEF at the canonical path) and `/prd` on R5 (no Accepted PRD
at the canonical path). Both were adjusted out, for a reason that is about this
repo rather than about the gates.

Requirements were not produced by this exploration — they were given. Issue #2464
carries six acceptance criteria and they survived exploration intact, which is the
exact anti-signal (`requirements were provided as input`) that demoted PRD during
crystallize. And repo precedent is unambiguous for this class of work:
`DESIGN-shell-d-lifecycle.md`, the immediate upstream for this issue and a product
of the same recipe review, went straight to a design doc with no BRIEF and no PRD.
The two BRIEFs and seven PRDs on disk cover feature work (auto-update,
distributed-recipes, tool-lifecycle-hooks), not labelled bugs with written
acceptance criteria.

One requirements-shaped question does exist — *should `tsuku remove nvm` be able to
delete the user's Node installs?* — and it is carried into the design as the gating
decision rather than split into a separate PRD, matching how
`DESIGN-shell-d-lifecycle.md` carried its own product questions in
`decision:`/`rationale:` frontmatter and a Considered Options section.

**Planned chain: `/design` → `/plan`.**

### R6 shape predicates for `/design`

Evaluated against issue #2464 and the `/explore` handoff, there being no PRD.

- **P1 — architectural alternatives: fires.** Two viable data-root locations
  survive elimination (a tsuku-owned stable path versus nvm's own `$HOME/.nvm`),
  and the placement mechanism for `nvm.sh`/`nvm-exec` is open between a narrow new
  action and `run_command`. The template-variable shape (`{tsuku_home}` vs
  `{share_dir}` vs a per-tool `{data_dir}`) is a third open alternative.
- **P2 — new component references: fires.** No existing action can place a file or
  symlink outside the staging directory, so the fix introduces one; `set_env` gains
  a placeholder it does not have; a user-facing notice may need a new `Kind` because
  `renderBackgroundSuccess` drops `Messages`.
- **P3 — Complex classification: fires.** The choice is gated on a product question
  with consequences for what `tsuku remove` means, and the exploration eliminated
  ten of twelve candidate shapes on concrete evidence that has to be recorded.

All three fire, so `/design` runs with a full decision roster. `/plan` fires ALWAYS.

`plan_execution_mode: single-pr` — the dispatch brief's deliverable is one
review-ready PR with green CI, so the PLAN is a self-contained document rather than
a GitHub milestone with per-issue PRs.

## Phase 2: Child invocation — design

`parent_orchestration:` sentinel active for the `/design` child.

```yaml
parent_orchestration:
  parent: scope
  child: design
  pre_invocation_sha: 82c88b5fdcd9c7e3ce94be9aeacd825556db9122
  invoked_at: 2026-08-02T17:10:00Z
```
