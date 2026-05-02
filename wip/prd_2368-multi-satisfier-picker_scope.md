# PRD Scope: 2368-multi-satisfier-picker

## Problem statement

`tsuku install <alias>` should be able to handle aliases that map to multiple recipes — present the user with a picker on a TTY, fail with a clear error under `-y`. Today the satisfies index is 1:1 at the type level, so the four-vendor OpenJDK family (`openjdk`, `temurin`, `corretto`, `microsoft-openjdk`) ships in PR #2362 with no way for `tsuku install java` to actually offer the four-way choice the milestone exists to provide.

## In scope

- New schema field for multi-satisfier aliases.
- Multi-eligible satisfies index (per-alias many-to-one).
- Arrow-driven picker on TTY.
- Helpful error format under `-y` listing candidates and `--from <recipe>` syntax.
- `--from <recipe-name>` flag form (no colon) for explicit selection.
- All four JDK recipes declare they satisfy `java`.
- Tests covering the truth table cases A–E.

## Out of scope

- Picker engagement during dependency resolution (deps must resolve deterministically; multi-satisfier alias as a runtime dep is a recipe authoring error and validator should reject it).
- Other commands (`tsuku run`, `tsuku info`) gaining picker behavior — install only.
- Generalizing to other "virtual package" concepts (apt-style provides) — only the satisfies-aliases shape ships here.
- Fuzzy search / multi-select picker UX — single-select arrow + Enter only.

## Research leads

All discovery work was completed in `/explore 2368 --auto` (Round 1, six leads). Findings consolidated in:

- `wip/explore_2368-multi-satisfier-picker_findings.md` — synthesis
- `wip/explore_2368-multi-satisfier-picker_decisions.md` — decisions log
- `wip/research/explore_2368-multi-satisfier-picker_r1_lead-tui-library.md`
- `wip/research/explore_2368-multi-satisfier-picker_r1_lead-schema-shape.md`
- `wip/research/explore_2368-multi-satisfier-picker_r1_lead-resolution-semantics.md`
- `wip/research/explore_2368-multi-satisfier-picker_r1_lead-discovery-interaction.md`
- `wip/research/explore_2368-multi-satisfier-picker_r1_lead-tty-and-error.md`
- `wip/research/explore_2368-multi-satisfier-picker_r1_lead-cache-and-edges.md`

No new research leads needed for the PRD.

## Coverage notes

The user's pre-decided constraints (1–4 in the orchestration prompt) plus the six lead picks form a complete spec. Phase 2 (Discover) is satisfied by the explore output and is being skipped.
