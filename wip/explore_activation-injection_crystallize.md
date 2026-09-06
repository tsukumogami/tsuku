# Crystallize: activation-injection

## Stage 1 — what this exploration is

**A chain.**

The four terminal categories all fail:

- **Competitive analysis** — precondition fails. `## Visibility` is `Public`, so
  the category is off the board.
- **Spike report** — the core question was never "can we do this?". Feasibility
  was never in doubt; the two defects were reproduced before the exploration
  started. The question was the *extent of a class*, and the answer is
  actionable work, not an assessment.
- **Rejection record** — no rejection conclusion. The opposite: the exploration
  found more to fix than it started with.
- **Decision record** — the contested points (validator placement, quoter
  placement, charset, scope boundary) were each settled during convergence with
  the tsukumogami seat, and none needed `shirabe:decision` because none ended in
  disagreement. They are several connected choices attached to real
  implementation work, which is a chain, not a standalone decision.

## Stage 2 — which entry point

**`/scope`.**

`/execute` is not a candidate: no `docs/plans/PLAN-*.md` exists and no file
carries `schema: plan/v1`, so the precondition fails and the arm is absent.

`/scope` over a direct `/plan`: the requirements are close to settled (the
issue's acceptance criteria plus this exploration's findings), and the technical
approach is settled in outline, which would argue for entering at `/plan`. But
three things want a hop that `/plan` does not provide:

1. The issue's acceptance criteria are **partly wrong**, not merely incomplete.
   Criterion 1 says rejects must be "reported (not silently skipped)" for both
   `tsuku shell` and `tsuku hook-env` — but `ActivationResult.Skipped` is
   write-only in production, and the placement decision moved rejection to parse
   time, where the reporting shape is an error rather than a per-tool report.
   Criterion 3 says "a name containing `..` cannot produce a PATH entry outside
   `$TSUKU_HOME/tools`" — true and insufficient, since the version component
   reaches the same composition unguarded at this sink. Requirements need a hop.
2. The exploration changed what the fix *is*. It went from two defects in one
   file to three defects at the activation sink (the `Dir` value being fixable
   only at the emitter), a second unquoted emitter in `cmd/tsuku`, a validator
   that must be extracted rather than written, and a placement that moved out of
   the contested file entirely.
3. `/scope` decides per hop which documents survive, which is exactly what this
   work needs: BRIEF and PRD are almost certainly not warranted for a bug with a
   filed issue, and whether a DESIGN survives is a real question given the
   correction work already owed to `DESIGN-shell-env-activation.md`.

## The document question, flagged early

An unusual constraint: this work must **correct** an existing design document as
part of the fix. `DESIGN-shell-env-activation.md` carries at least seven false
control claims, and correcting them is the deliverable that stops the next
person concluding the control exists. So the chain should not author a *new*
design that duplicates or competes with the one being corrected. If a DESIGN
survives at all it should be a small amendment, not a second record.

## Handoff

Next: `/shirabe:scope`, gated by `host_env_tsuku` per the dispatch brief.

Carry forward: `wip/explore_activation-injection_findings.md` (the settled scope
recommendation and the out-of-scope owners), and the six research files under
`wip/research/`.
