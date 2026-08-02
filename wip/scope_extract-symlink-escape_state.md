```yaml
topic: extract-symlink-escape
chain_started: 2026-08-02T21:40:00Z
last_updated: 2026-08-02T22:15:00Z
phase_pointer: phase-2
visibility: Public
execution_mode: auto
max_rounds: 5
coordinated: false
upstream_input: wip/explore_extract-symlink-escape_crystallize.md
framing_shift: false
exit: UNSET
exit_artifacts: []
planned_chain:
  - design
  - plan
chain_skipped:
  - name: brief
    reason: >-
      R4 signal 1 holds (no BRIEF at canonical path), but adjusted out at the
      Phase 1 Adjust branch: the framing this altitude persists -- problem,
      intended outcome, scope boundary -- is already durable in
      tsukumogami/tsuku#2473 and in the exploration crystallize artifact.
  - name: prd
    reason: >-
      R5 gate would fire (no PRD at canonical path), adjusted out at the Phase 1
      Adjust branch: the requirements are the issue's four acceptance criteria,
      authored and agreed by the maintainer who filed the bug.
design_predicates:
  P1: >-
    fires -- five enforcement mechanisms were live (os.Root, EvalSymlinks
    resolve-then-check, securejoin, two-pass header validation,
    staging-then-verify). The choice is settled by measurement but must be on
    record before wip/ is cleaned.
  P2: >-
    does-not-fire -- no new binary, service, library, or runtime substrate.
    os.Root is stdlib at the Go version already required.
  P3: >-
    does-not-fire -- issue is labeled `bug`; implementation is mechanical once
    the mechanism is chosen.
child_snapshots:
  design:
    status: Proposed
    path: docs/designs/DESIGN-extract-symlink-escape.md
    captured_at: 2026-08-02T22:15:00Z
chain_ran:
  - design
worktree_rebases: []
worktree_divergences: []
```
