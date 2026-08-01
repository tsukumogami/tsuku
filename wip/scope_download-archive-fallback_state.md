```yaml
topic: download-archive-fallback
chain_started: 2026-08-01T21:34:06Z
last_updated: 2026-08-01T21:34:06Z
phase_pointer: phase-2
exit: UNSET
exit_artifacts: []
planned_chain:
  - brief
  - prd
  - design
  - plan
chain_skipped: []
chain_ran:
  - brief
child_snapshots:
  brief:
    status: Accepted
    content_hash: 5f5d7f86ebb143457680495c1be9501f99408748
    captured_at: 2026-08-01T21:55:00Z
    validator: clean
execution_mode: auto
max_rounds: 5
visibility: Public
source_issue: 2443
branch: feat/2443-download-archive-fallback
```

## Notes

- `$ARGUMENTS` was `2443 --auto`. `2443` is a GitHub issue reference, not a
  topic slug in the repo's prevailing style (`docs/` uses descriptive
  kebab-case: `PLAN-curated-recipes.md`, `DESIGN-checksum-pinning.md`).
  Topic slug resolved to `download-archive-fallback`, which matches
  `^[a-z0-9-]+$`; issue 2443 is recorded in `source_issue:` instead.
- `## Repo Visibility:` header is absent from `CLAUDE.md`. The skill default
  is Private ("Default to Private if unknown"), but the workspace CLAUDE.md
  states tsuku is a Public repo and the GitHub repo is public. Recorded
  `visibility: Public` — the stricter content-governance setting — rather
  than letting private-only content classes past the validator in a public
  repo.
- `shirabe slug-prefix-detect download-archive-fallback --docs-root docs`
  returned `no-prevailing-prefix`, so no prefix recommendation applies.

## Phase 1 — chain proposal (auto-confirmed: Proceed)

Discovery found no artifact at any canonical path for this topic
(`docs/briefs/`, `docs/prds/`, `docs/designs/`, `docs/designs/current/`,
`docs/plans/`), so `child_snapshots:` is empty and this is a cold start.
Framing-shift question is answered by the dispatch brief: the framing has
in fact shifted since #2384 and #2391 — the problem is no longer "zig has a
bad mirror" but "one recipe naming one host turns one operator's outage into
a repo-wide plan-generation failure".

Gate verdicts:

- `/brief` — fires (R4 EITHER-signal: no upstream BRIEF at canonical path;
  framing-shift also positive).
- `/prd` — fires (R5 Mandatory-with-auto-skip: no PRD at canonical path).
- `/design` — fires (R7 shape-dependent).
  - P1 architectural-alternatives: **fires**. Three live alternatives the
    PRD leaves open — per-recipe source list vs. learning Zig's
    community-mirror protocol; plan records the full ordered list vs. the
    single URL that served; source list carried in `Params` vs. beside the
    hoisted `step.URL`.
  - P2 new-component references: **does-not-fire**. Every touched component
    exists (`internal/actions/`, `internal/recipe/`, `internal/executor/`);
    no new binary, service, or substrate.
  - P3 Complex classification: **fires**. Six layers named in the issue, a
    shared four-consumer funnel (`decomposeDownload`), a published-artifact
    format change, and a cache-keying change. Architectural complexity is
    explicit in the issue's own text.
- `/plan` — fires (ALWAYS). `execution_mode: single-pr` is required by the
  dispatch brief.

`--auto` selects **Proceed** without prompting.
