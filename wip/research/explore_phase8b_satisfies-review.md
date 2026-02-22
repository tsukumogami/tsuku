# Architect Review: Decision 3 (Queue Status Check with Satisfies Resolution)

## Scope

Review of `docs/designs/DESIGN-requeue-on-recipe-merge.md`, focusing on the integration of `satisfies` metadata (PR #1824 / `DESIGN-ecosystem-name-resolution.md`) into the requeue tool's recipe existence check.

## Question 1: Does building the satisfies index in the requeue tool duplicate the recipe loader's work?

### Finding: Ambiguity in the design text, but no structural violation if resolved correctly

The design text is ambiguous about where the satisfies index comes from. Two statements create tension:

**Line 131**: "The satisfies index can be built from the registry manifest (already available to the Go code) or from recipe TOML files on disk."

**Line 204**: "Uses the existing `internal/recipe` loader's satisfies index for this."

These point in different directions. Line 131 suggests building a new index. Line 204 suggests reusing the loader's index. The difference matters architecturally:

**Option A (build a new index)**: `internal/requeue/` would scan recipe TOML files or the manifest and build its own `map[string]string` from satisfies entries. This duplicates `Loader.buildSatisfiesIndex()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/recipe/loader.go:370-418`. Two implementations of the same index with the same data sources would diverge over time. **This would be a blocking pattern introduction.**

**Option B (use the recipe loader's index)**: `internal/requeue/` calls `loader.LookupSatisfies(name)` (exposed at `loader.go:449`). This reuses the existing index. But it requires the requeue tool to instantiate a recipe `Loader` with access to embedded recipes and the registry manifest. The `Loader` needs a `*registry.Registry` with a `CacheDir` pointing at the cached manifest.

**Option C (extract the index)**: Export `buildSatisfiesIndex` as a standalone function that takes the data sources as parameters. Both the loader and the requeue tool call it. This is the cleanest separation but may be premature -- the loader's lazy `sync.Once` pattern works well, and the requeue tool is the only second consumer.

**Recommendation**: Option B is the right fit. The `LookupSatisfies` method already exists as a public API (`loader.go:449`). The design's line 204 says "uses the existing internal/recipe loader's satisfies index" -- this should be the authoritative statement. Line 131 should be tightened to remove the ambiguity about building from scratch. The CLI (`cmd/queue-maintain/`) can instantiate a `Loader` configured with the embedded recipes and a registry pointed at the local checkout's cached manifest.

**Severity**: Advisory. The design's intent on line 204 is correct, but the contradicting language on line 131 could mislead an implementer into building a parallel index.

## Question 2: Does `update-queue-status.yml` give queue-maintain enough information?

### Finding: Critical gap in the data flow

The design says: "When `update-queue-status.yml` marks `openssl` as 'success', does the requeue tool need access to the recipe files to know that `openssl` satisfies `openssl@3`?"

**Yes, it does.** Here's the data flow problem:

1. `update-queue-status.yml` detects that `recipes/o/openssl.toml` changed.
2. It finds the queue entry named `"openssl"` and marks it `"success"`.
3. `queue-maintain` runs. It scans the blocker map and finds that `afflib` is blocked by `"openssl@3"` (from the failure JSONL at `data/failures/homebrew-2026-02-18T15-40-59Z.jsonl`).
4. To know that `"openssl@3"` is resolved, it needs to know that the recipe `openssl` declares `satisfies.homebrew = ["openssl@3"]`.

The queue itself has no `satisfies` data. The queue entry for `openssl` has `name`, `source`, `priority`, `status`, `confidence` -- no satisfies mappings. The satisfies data lives in recipe TOML files (embedded at `internal/recipe/recipes/openssl.toml:8-9`) and in the registry manifest.

So queue-maintain needs access to recipe metadata. The design acknowledges this at line 204 ("loads its recipe metadata") but doesn't specify the mechanics clearly. In the CI environment (`update-queue-status.yml`), the tool runs in a full checkout of the repo, so:

- Embedded recipes are compiled into the binary (available via the `Loader`'s `EmbeddedRegistry`).
- Registry recipes are on disk at `recipes/` (available if the `Loader` is configured with `recipesDir`).
- The registry manifest may or may not be cached in `$TSUKU_HOME/registry/manifest.json`, but in CI there's no `$TSUKU_HOME`.

**The implementation needs to ensure the `Loader` can find satisfies data in CI.** The embedded recipes cover the current cases (`openssl`, `gcc-libs`, `python-standalone` at `internal/recipe/recipes/`). For registry-only recipes with `satisfies` entries, the loader would need either a `recipesDir` pointing at the checkout's `recipes/` directory, or a cached manifest.

**Severity**: Advisory. The current `satisfies` entries are all on embedded recipes, so this works today. But the design should specify that `cmd/queue-maintain/` needs to configure the recipe loader with access to the recipe files (e.g., `--recipes-dir` flag or auto-detection from the checkout root). Without this, future `satisfies` entries on registry-only recipes won't be picked up.

## Question 3: Is the implementation phase ordering correct?

### Finding: Phase ordering is correct

The phases are:

1. Extract shared failure loading (`internal/blocker/`)
2. Implement `internal/requeue/` (depends on Phase 1 for blocker map)
3. Create `cmd/queue-maintain/` (depends on Phase 2)
4. Update workflows (depends on Phase 3)
5. Cleanup and documentation

The satisfies integration fits naturally into Phase 2: `internal/requeue/Run()` needs the satisfies index to build the resolved-names set. The recipe loader is already available as a dependency -- `internal/requeue/` would import `internal/recipe/` (same level, both internal packages, dependency flows correctly).

**No dependency inversion**: `internal/requeue/` importing `internal/recipe/` is fine. Both are internal packages at the same depth. The recipe loader doesn't import batch or requeue packages.

**Severity**: No issue.

## Question 4: Edge cases in satisfies resolution

### Finding: Three edge cases partially addressed, one unaddressed

**Edge case 1: Multiple `blocked_by` entries, some resolved, some not.**
The design handles this at step 6: "If ALL blockers are resolved, flips status." Correct -- partial resolution shouldn't unblock.

**Edge case 2: A `blocked_by` name that is both a recipe name AND a satisfies target of a different recipe.**
Example: recipe `sqlite` exists, and some other recipe declares `satisfies.homebrew = ["sqlite"]`. The ecosystem name resolution design handles this at `DESIGN-ecosystem-name-resolution.md:169`: "A satisfies entry that matches another recipe's canonical name should be rejected by validation." The requeue tool doesn't need to handle this because validation prevents it from occurring.

**Edge case 3: A `blocked_by` name that matches a queue entry name but that entry is not "success".**
Example: `openssl@3` appears in the queue as a separate entry with status "failed" (before the duplicate is cleaned up). The recipe `openssl` is "success" and satisfies `openssl@3`. Should the blocked package be unblocked?

The design says a dependency is resolved if "its name (or a name it's satisfied by) appears in the queue with status 'success'". But what if `openssl@3` appears in the queue as "failed"? The design checks the resolved set (success entries) and their satisfies expansions. If `openssl` is "success" and satisfies `openssl@3`, then `openssl@3` would be in the resolved set, regardless of whether a separate `openssl@3` entry exists with a different status. This seems correct -- the dependency is met because a recipe that provides `openssl@3` exists and is merged.

**Edge case 4 (unaddressed): Stale failure data.**
Failure JSONL files accumulate over time. A package might have `blocked_by: ["openssl@3"]` in an old failure file but have been re-processed successfully in a newer batch run. The requeue tool builds the blocker map from ALL failure files, not just the latest. If the package was successfully processed, its queue status should already be "success" or "pending", not "blocked". But if it was re-processed and failed for a different reason (no longer missing_dep), it might be "failed" with a new failure record that has no `blocked_by`.

The design's requeue logic only acts on entries with status "blocked". If the entry is "failed" (different reason), requeue correctly ignores it. The stale failure data would add the package to the reverse blocker map, but since the entry isn't "blocked", the requeue step skips it. **This is handled implicitly.**

**Edge case 5 (partially addressed): `blocked_by` names from non-Homebrew ecosystems.**
The `extractBlockedByFromOutput` function at `orchestrator.go:519` extracts names from "recipe X not found in registry" messages. These are recipe names (the name passed to `tsuku install`), not ecosystem package names. So for Homebrew, if the generated recipe has `runtime_dependencies = ["openssl@3"]` and the dependency resolver's `parseDependency` splits on `@` to get `openssl`, then the "not found" error would say "recipe openssl not found" (not `openssl@3`).

Wait -- this contradicts what the actual failure data shows. The failure record at `data/failures/homebrew-2026-02-18T15-40-59Z.jsonl` has `"blocked_by":["openssl@3"]` for `afflib`. So the raw Homebrew formula name IS making it into `blocked_by`.

Looking more carefully at the code path: `extractBlockedByFromOutput` matches "recipe X not found in registry". The question is whether the dependency resolution produces `openssl@3` or `openssl` in the error message. From `DESIGN-ecosystem-name-resolution.md:82`: "parseDependency() splits on first @ to separate name from version. openssl@3 -> name=openssl, version=3. This means the satisfies fallback is never triggered for metadata.runtime_dependencies entries."

So for `runtime_dependencies = ["openssl@3"]`, `parseDependency` produces `openssl` (which exists), so no error. The `blocked_by: ["openssl@3"]` in the failure data must come from a different code path -- likely step-level `dependencies` (not metadata `runtime_dependencies`), or from when the `openssl` embedded recipe didn't exist yet.

Looking at the actual error message in the failure data: "Error: registry: recipe openssl@3 not found in registry". This tells us the name `openssl@3` was passed to the registry lookup without `@`-splitting. This happens in step-level dependency resolution, where names are used as-is (no `parseDependency` call).

**Bottom line**: The satisfies resolution is necessary for `blocked_by` entries that contain ecosystem names like `openssl@3`. The design correctly identifies this need. The edge case is well-understood.

**Severity**: Advisory for edge case 4 (stale data). The implicit handling is correct but not documented in the design.

## Question 5: Does the overall architecture hold together?

### Finding: Architecture is sound, with one advisory clarification needed

**Dependency direction**: Correct. The proposed dependency graph is:
```
cmd/queue-maintain/ -> internal/requeue/ -> internal/recipe/ (for satisfies)
                    -> internal/reorder/ -> internal/blocker/
                    -> internal/batch/   (for queue I/O)
```
All arrows point downward/sideways at the same level. No circular dependencies.

**Pattern consistency**: The design follows the existing pattern of consolidating shared logic into internal packages. Moving `loadBlockerMap` to `internal/blocker/` parallels how `ComputeTransitiveBlockers` is already shared there. The requeue package follows the same signature pattern as reorder (`Run()` with queue + failures dir).

**Single writer pattern**: Sound. `update-queue-status.yml` becomes the sole writer of `priority-queue.json` on main. `batch-generate.yml` runs `queue-maintain` in its working tree but doesn't push queue changes. This eliminates the concurrent writer problem.

**Interface alignment**: The design proposes `reorder.Run()` to accept `*batch.UnifiedQueue` directly instead of loading from disk. This is a good change -- it lets `cmd/queue-maintain/` load once and pass the same queue object to both requeue and reorder. But the current `reorder.Run()` at `internal/reorder/reorder.go:71` takes `Options` and loads the queue itself. The design mentions this change at line 211 but the Phase 1 description doesn't include it. Phase 1 says "extract loadBlockerMap" but the reorder interface change is a separate concern. This is minor -- the implementer will discover the interface mismatch during Phase 3 integration.

**Scanner buffer size note**: The design at line 213 correctly identifies the 64KB default scanner buffer limit as a risk. The current `loadBlockersFromFile` at `reorder.go:175` uses `bufio.NewScanner` without adjusting the buffer. Good catch; should be fixed during the extraction to `internal/blocker/`.

**Overall verdict**: The architecture is sound. The satisfies integration is the right approach -- use the recipe loader's existing index rather than building a parallel one. The main risk is the ambiguous language on line 131 that could lead an implementer to build a duplicate index.

## Summary of Findings

| # | Finding | Severity | Location |
|---|---------|----------|----------|
| 1 | Ambiguous language on satisfies index source (line 131 vs line 204) | Advisory | `DESIGN-requeue-on-recipe-merge.md:131` |
| 2 | `cmd/queue-maintain/` needs recipe loader configuration for satisfies | Advisory | `DESIGN-requeue-on-recipe-merge.md:204` |
| 3 | Phase ordering is correct | No issue | -- |
| 4 | Stale failure data edge case handled implicitly but not documented | Advisory | `DESIGN-requeue-on-recipe-merge.md:197-207` |
| 5 | `reorder.Run()` interface change needed but not in Phase 1 scope | Advisory | `DESIGN-requeue-on-recipe-merge.md:210-211` |
| 6 | Overall architecture is sound, no blocking issues | No issue | -- |

No blocking findings. The design integrates `satisfies` correctly and follows existing patterns.
