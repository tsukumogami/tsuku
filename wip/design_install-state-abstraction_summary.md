# Design Summary: install-state-abstraction

## Input Context (Phase 0)

**Source:** /explore handoff (Round 1, six leads, all completed)
**Topic:** install-state-abstraction
**Branch:** explore/install-state-abstraction
**Visibility:** Public
**Scope:** Tactical
**Issue:** #2413 (https://github.com/tsukumogami/tsuku/issues/2413)
**Issue Label:** needs-design

**Problem (1-2 sentences):** Threading the new `Source` attribution parameter through the
install pipeline in PR #2412 drove the file count higher than predicted, exposing a
recurring pattern: cross-cutting concerns thread through 10 wrapper signatures and
authors increasingly route around the cost (9 CLI flag globals, parallel `runDryRun`,
stranded `telemetryClient`). The design must decide between status-quo, a context.Context-
based attribution refactor, or a new `installops` Service layer that also hides
`state.json` semantics behind named operations.

**Constraints (from exploration):**

- The lifecycle event bus (DESIGN-notices-install-event-bus.md, Current) must compose
  unchanged. Verb-per-event vocabulary, sync-with-recover delivery, publish-after-state
  invariant, subscriber-locality contract — all preserved.
- Java-style literal repository pattern, command/middleware/decorator chains, and
  aggressive `OperationOptions` consolidation are eliminated by the exploration
  (recorded in `wip/explore_install-state-abstraction_decisions.md`).
- Two viable shapes remain: Candidate B (`installops` Service layer) and Candidate C
  (ctx-attribution + recursion-collapse). Status-quo (Candidate A) is a defensible
  third option.
- `Manager` is in `internal/` — no external API compatibility obligation.
- The user explicitly authorized this design to reach "Rejected" status if the
  evidence supports doing nothing.

**Key empirical findings (from exploration findings file):**

- 5 install-touching PRs in 3 months each added one cross-cutting concern (one every 4-6 weeks).
- `installWithDependencies` grew from 7 to 10 positional parameters.
- 3 in-tree workarounds prove the cost is producing maintenance debt.
- PR #2412's actual `Source`-threading cost was 11 files / ~120 LOC, not the 36-file headline.
- Manager doesn't take `ctx` anywhere — Ctrl-C during atomic-rename is a silent hazard.
- The structural smell is `mgr.GetState().UpdateTool(name, func(ts){...})` leaking state
  semantics into `cmd/tsuku/install_deps.go` at lines 223, 475, 584.
- `Source` behaves like a process-level attribute (every call site passes a literal
  constant), not a per-call argument — strongly favors ctx-based attribution.

## Current Status

**Phase:** 0 - Setup (Explore Handoff Complete)
**Last Updated:** 2026-05-16
**Next:** Phase 1 (Decomposition into decision questions)

## Pointers for /design

- The exploration findings file is the authoritative input:
  `wip/explore_install-state-abstraction_findings.md` (see especially the
  `## Accumulated Understanding` section).
- Detailed research is in `wip/research/explore_install-state-abstraction_r1_lead-*.md`
  if a decision question needs to drill deeper.
- The crystallize decision is at `wip/explore_install-state-abstraction_crystallize.md`.
- The exploration decisions file is at `wip/explore_install-state-abstraction_decisions.md`
  (treat its entries as constraints).
