# Pragmatic Review: DESIGN-requeue-on-recipe-merge.md

## Finding 1: Satisfies resolution in requeue is solving a self-healing problem

**Severity: Blocking (scope creep)**

The design adds satisfies resolution to `queue-maintain` (Decision 3, lines 125-139) so the requeue logic can map ecosystem names like `openssl@3` to recipe names like `openssl`.

But this problem fixes itself:

1. The `satisfies` fallback already exists in `internal/recipe/loader.go` (line 133-139). After PR #1824, `tsuku install openssl@3` resolves to the `openssl` recipe via the satisfies index. The install succeeds.
2. Future batch runs won't produce `missing_dep` failures for satisfied names -- the loader handles them at install time.
3. Old failure records with stale ecosystem names (`blocked_by: ["openssl@3"]`) become irrelevant as new batch runs succeed and overwrite failure JSONL. The queue-status approach already handles the common case: if `openssl` is marked "success" in the queue, the requeue check could simply look for the bare recipe name in the resolved set.

The satisfies resolution in the requeue tool is reimplementing logic that already lives in the loader, for a transient data mismatch that ages out on its own.

**Fix:** Drop satisfies resolution from `queue-maintain`. Check blockers against the queue's "success" entries by exact name match. Stale ecosystem-name blockers clear themselves within one batch cycle. If that latency is unacceptable, add satisfies resolution later when there's evidence it matters -- right now there's no data showing how many blocked entries would remain stuck.

## Finding 2: Single-owner pattern is naming dead code removal

**Severity: Advisory**

Decision 4 (lines 141-157) frames removing the batch-generate queue writes as adopting a "single-owner pattern." In reality, the code at `batch-generate.yml:1109-1124` uses `.packages[]` while the unified queue uses `.entries[]`. The jq expressions select nothing and write the queue unchanged. This is dead code.

The design correctly identifies this (line 83: "the jq expressions silently select nothing and the queue is written unchanged") but then wraps the deletion in a pattern name with alternatives-considered analysis. The fix is: delete the dead code. No pattern needed.

**Fix:** Frame Phase 4 step 3 as "remove dead queue-update code from batch-generate.yml" rather than a design decision with alternatives. Cut Decision 4 entirely -- the rationale is "this code doesn't work, so remove it." The concurrent-writer concern disappears because there was never a concurrent writer.

## Finding 3: Rename from reorder-queue to queue-maintain adds churn for no consumer

**Severity: Advisory**

Nothing external references `cmd/reorder-queue/` (grep confirms: no workflow references, no scripts call it). The binary name exists only in `cmd/reorder-queue/main.go` and design docs. Renaming it forces updating 6+ design doc references across the repo for a binary that has zero external callers.

The design's own rationale (line 119): "the binary name becomes misleading since it now does more than reorder." But a pipeline CLI called by one workflow step doesn't need a marketing-friendly name.

**Fix:** Keep `cmd/reorder-queue/`, add the requeue step to it. If the name bothers someone later, renaming a zero-caller binary is trivial.

## Finding 4: 5-phase plan is 3 phases of real work and 2 phases of ceremony

**Severity: Advisory**

- Phase 1 (extract shared failure loading): Justified -- `loadBlockerMap` is duplicated.
- Phase 2 (implement `internal/requeue/`): Justified -- this is the core deliverable.
- Phase 3 (create `cmd/queue-maintain/`): Wire-up. Collapses with Phase 2 since the CLI is a thin main().
- Phase 4 (update workflows): Justified -- this is the integration work.
- Phase 5 (cleanup and documentation): Updating design doc cross-references and checking for stale comments. This is PR hygiene, not a phase.

**Fix:** 3 phases: (1) Extract blocker loading + implement requeue logic, (2) Add requeue to the CLI + update workflows, (3) Delete dead code (`requeue-unblocked.sh`, dead batch-generate queue writes). Phase 3 could be a follow-up PR.

## Finding 5: --skip-requeue and --skip-reorder flags are speculative generality

**Severity: Advisory**

The design adds `--skip-requeue` and `--skip-reorder` flags (line 111) "for testing and debugging." No workflow caller would use these. The default is both. Tests should test the internal packages directly, not via CLI flags.

**Fix:** Drop the flags. The two internal packages (`requeue.Run`, `reorder.Run`) are independently testable via unit tests. If a debugging need arises, add the flag then.

## Finding 6: New `internal/requeue/` package Result/Change types are over-specified

**Severity: Advisory**

The `Result` struct (lines 184-188) and `Change` struct (lines 190-193) return structured data about what was requeued. The only consumer is the CLI main(), which prints a count. `Requeued int` and `Remaining int` are sufficient; `Details []Change` with per-entry `ResolvedBy []string` has no consumer.

**Fix:** Return `(requeued int, err error)`. Add structure when a consumer needs it.

## Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | Satisfies resolution solves a self-healing problem | Blocking |
| 2 | "Single-owner pattern" is just deleting dead code | Advisory |
| 3 | Rename adds churn for zero-caller binary | Advisory |
| 4 | 5 phases -> 3 phases | Advisory |
| 5 | --skip-requeue/--skip-reorder flags are speculative | Advisory |
| 6 | Result/Change types over-specified for one consumer | Advisory |

The core value of this design -- replacing the broken bash script with Go that uses the unified queue format, and wiring requeue into `update-queue-status.yml` for fast unblocking -- is correct and necessary. The blocking concern is satisfies resolution: it reimplements loader logic to handle a transient data problem. The advisory items are modest over-engineering that won't compound but could be trimmed.
