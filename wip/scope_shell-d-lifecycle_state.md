```yaml
topic: shell-d-lifecycle
last_updated: 2026-08-02T13:58:01Z
phase_pointer: phase-2
chain_started: 2026-08-02T13:58:01Z
execution_mode: auto
max_rounds: 5
visibility: Public
visibility_source: CLAUDE.local.md ## Repo Visibility header (main checkout; the
  file is gitignored and therefore absent from this worktree, but the repo path
  under public/ corroborates)
coordinated: false
planned_chain:
  - design
  - plan
chain_skipped:
  - name: brief
    reason: >-
      upstream-exploration-supplied-framing. The dispatch brief and
      wip/explore_shell-d-lifecycle_findings.md already carry the problem shape,
      scope boundary, and success criterion a BRIEF would author. R4's
      EITHER-signal fires on "no upstream BRIEF at canonical path", but the
      framing-shift signal is negative and the framing exists durably upstream,
      so the author selected Adjust to drop it.
  - name: prd
    reason: >-
      requirements-supplied-as-input. The dispatch brief states seven explicit
      acceptance criteria. wip/explore_shell-d-lifecycle_crystallize.md scored
      PRD as demoted on the "requirements were provided as input to the
      exploration" anti-signal, and scored Design Doc 6 with zero anti-signals.
      Writing a PRD would restate the criteria verbatim. Adjust selected.
chain_ran: []
r6_predicates:
  p1_architectural_alternatives:
    verdict: fires
    reason: >-
      Six candidate re-render mechanisms were characterized in round 1 with a
      strongest-objection argument against each and no dominant winner: replay
      the stored plan, a render capability separate from Execute, regenerate
      from the recipe, store rendered content in state, a stable per-tool
      indirection path, delete-plus-lazy-regenerate.
  p2_new_components:
    verdict: fires
    reason: >-
      Every candidate introduces new surface. The leading shapes need either a
      new capability interface in internal/actions, a new leaf package to hold
      install.ValidateRequested so the actions -> version -> install cycle can
      be severed, or a new stable per-tool directory under $TSUKU_HOME.
  p3_complex_classification:
    verdict: fires
    reason: >-
      The change spans internal/install, internal/actions, internal/shellenv,
      and cmd/tsuku, obliges a same-PR update of three plugin skills per the
      repo's plugin-maintenance table, and must not disturb an import cycle or
      arm the golden-plan CI workflow.
upstream_inputs:
  - wip/explore_shell-d-lifecycle_findings.md
  - wip/explore_shell-d-lifecycle_crystallize.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-shelld-write-path.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-lifecycle-events.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-state-plan-replayability.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-rerender-mechanisms.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-adjacent-gaps.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-test-infrastructure.md
  - wip/research/explore_shell-d-lifecycle_r1_lead-prior-art.md
child_snapshots: {}
```

## Chain Proposal

Emitted at Phase 1, resolved **Adjust** then **Proceed**.

```
Planned chain:
  /brief   — skipped (Adjust: framing supplied durably by upstream exploration)
  /prd     — skipped (Adjust: acceptance criteria supplied as input)
  /design  — fires (R7 shape-dependent: P1 fires, P2 fires, P3 fires)
  /plan    — fires (ALWAYS)
```

All three R6 predicates fire, so `/design` runs its full decision-roster shape.
The mechanism question routes through the `shirabe:decision` framework, which the
dispatch brief designates for contested choices and whose input shape — several
characterized candidates with no dominant winner — this matches exactly.
