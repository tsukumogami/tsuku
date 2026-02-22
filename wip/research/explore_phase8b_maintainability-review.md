# Maintainability Review: DESIGN-requeue-on-recipe-merge.md

Reviewer role: maintainer-reviewer (can the next developer understand and change this with confidence?)

## 1. Scope Creep Assessment

**Verdict: The scope is justified, but the design should acknowledge the risk more explicitly.**

Issue #1825 asks for "run requeue when recipes are merged." The design takes on three additional things:

1. **Renaming `cmd/reorder-queue/` to `cmd/queue-maintain/`** -- This is motivated by the name-behavior mismatch that would exist if requeue were added to a binary called "reorder-queue." Reasonable. But the rename itself is a separate unit of work with its own blast radius (workflow references, documentation, CI steps).

2. **Single-owner pattern for queue writes** -- This fixes a real concurrent-writer problem (two workflows with different concurrency groups both writing `priority-queue.json`). The design correctly identifies that the batch-generate.yml code at lines 1109-1124 uses `.packages[]` on a file that uses `.entries[]`, meaning it silently does nothing today. Removing dead code is fine. But the design couples this cleanup with the requeue feature, making the PR larger and harder to revert.

3. **Satisfies-aware dependency resolution** -- This is the most significant addition. Without it, the requeue logic would miss cases where `blocked_by` names use ecosystem package names (e.g., `openssl@3`) that differ from recipe names (`openssl`). The design explains this well.

**Recommendation**: Phase the delivery as suggested in question 5 below. The satisfies resolution is necessary for correctness, but the rename and single-owner pattern could be separate PRs.

## 2. Simplification Opportunities

### Phase 1 (extract shared failure loading) may be premature

The design proposes extracting `loadBlockerMap()` and `loadBlockersFromFile()` from `internal/reorder/` to `internal/blocker/` before implementing `internal/requeue/`. But `internal/requeue/` needs a *different* data structure from reorder:

- **Reorder** needs: `dep -> [blocked_pkg_ids]` (the forward blocker map, which is what `loadBlockerMap` returns).
- **Requeue** needs: `pkg_id -> [blocking_deps]` (the reverse index: which deps block this package).

The design says requeue will "build a reverse index: package-to-blockers (inverting the blocker-to-packages map from failure data)." So requeue calls `loadBlockerMap()` then inverts it. This is correct but means requeue doesn't actually share the *computation* from `internal/blocker/`, just the file-loading code. Whether the extraction is worth a separate phase depends on how much code `loadBlockerMap` and `loadBlockersFromFile` represent. Looking at the current code, they're ~60 lines. Extracting them is fine but calling it "Phase 1" suggests it's load-bearing. It's really a minor refactor.

### The `reorder.Run()` interface change is under-specified

The current `reorder.Run()` takes `Options` (file paths) and handles its own I/O:

```go
func Run(opts Options) (*Result, error) {
    queue, err := batch.LoadUnifiedQueue(opts.QueueFile)
    // ...
    batch.SaveUnifiedQueue(outputPath, queue)
}
```

The design proposes changing it to accept `*batch.UnifiedQueue` directly. This is a breaking change to `reorder.Run()`'s signature and semantics. The design's pseudocode in the CLI section shows `reorder.Run(queue, *failuresDir)`, but the current caller (`cmd/reorder-queue/main.go`) passes `Options{QueueFile, FailuresDir, OutputFile, DryRun}`. The design doesn't specify what happens to `DryRun` and `OutputFile` in the new interface. These flags move to the CLI level, but this needs to be explicit.

**Advisory**: Specify the new `reorder.Run()` signature in the same detail as `requeue.Run()`. The next developer will look at the pseudocode, see `reorder.Run(queue, *failuresDir)`, and not know what happened to the other options.

## 3. Satisfies Resolution Clarity

**This is the design's biggest maintainability risk.**

The connection between three concepts is critical and currently spread across the document:

1. **`blocked_by` arrays in failure JSONL** use ecosystem package names (e.g., `"openssl@3"`, `"sqlite3"`)
2. **Queue entries and recipe names** use canonical recipe names (e.g., `"openssl"`, `"sqlite"`)
3. **`satisfies` metadata in recipes** maps from recipe name to ecosystem names (e.g., `openssl.satisfies.homebrew = ["openssl@3"]`)

The design explains this in Decision 3 (lines 125-139), but the explanation is in the "Chosen" subsection of a decision record. The next developer maintaining `internal/requeue/` won't necessarily read the design doc. The code itself needs to make this clear.

**Blocking concern**: The `Run()` signature says it takes `*batch.UnifiedQueue` and `failuresDir string`. But where do the satisfies mappings come from? The design says "Uses the existing `internal/recipe` loader's satisfies index for this" but also says "The satisfies index can be built from the registry manifest (already available to the Go code) or from recipe TOML files on disk." These are two different data sources with different availability guarantees. The design doesn't commit to one.

Looking at the existing code:
- `internal/recipe/loader.go` builds the satisfies index lazily from embedded recipes and the registry manifest (`buildSatisfiesIndex`). It needs a `*registry.Registry` and optionally embedded recipes.
- The registry manifest is cached at `$TSUKU_HOME/registry/manifest.json`.

In CI (where `queue-maintain` runs), the registry is the local checkout, not `$TSUKU_HOME`. The satisfies index from `internal/recipe/loader.go` depends on `registry.GetCachedManifest()`, which reads from the registry's cache directory. In CI, this cache may or may not exist.

**The design should specify exactly which data source `queue-maintain` uses for satisfies mappings in CI.** Options:
- Read recipe TOML files from the checkout's `recipes/` directory (scan for `satisfies` fields).
- Read the registry manifest from `data/` or wherever it's cached in CI.
- Use `internal/recipe.Loader.LookupSatisfies()` (but this requires constructing a Loader with registry access).

This is not an academic concern. If the satisfies source is wrong or unavailable, requeue silently fails to unblock packages -- exactly the problem the design is trying to fix.

## 4. Naming and Structural Decisions

### `queue-maintain` name: adequate but not great

The name "queue-maintain" is better than "reorder-queue" for a tool that does both requeue and reorder. But "maintain" is vague. A developer looking at `cmd/queue-maintain/` won't know what maintenance it performs without reading the code. Consider `queue-maintain` vs `queue-unblock-reorder` or similar. However, this is **advisory** -- the name doesn't create misread risk, just a minor discovery cost.

### `--skip-requeue` and `--skip-reorder` flags

These are presented as "for testing and debugging." In practice, workflows will always run both. The flags add testing surface but also add two code paths that need to be correct (running reorder without requeue, running requeue without reorder). If the only consumers are tests, consider making the `internal/requeue` and `internal/reorder` packages independently testable (they already are via their `Run()` functions) rather than adding CLI flags.

**Advisory**: The flags don't cause misread risk, but they create an implicit contract: if you skip requeue, the reorder results may be based on stale blocked/pending statuses. The design should document this dependency, even if the flags are debug-only.

### The `requeue.Run()` function modifies queue in place

The design says "Modifies the queue in place, returns the result." This is fine for the immediate use case (load once, run both, write once). But the function signature `Run(queue *batch.UnifiedQueue, failuresDir string) (*Result, error)` takes a pointer and mutates it. The name `Run` doesn't convey mutation. Future callers might assume `Run` is read-only (computes what would change) and `Result` contains the changes to apply.

**Advisory**: The in-place mutation is documented in the design but should be reflected in code comments. The `Result` struct having `Requeued int` alongside the queue being already modified is a mild invisible-side-effect pattern. Consider naming it `Apply` or adding a clear doc comment.

### Stale comment in orchestrator.go

The design correctly identifies the stale comment at `internal/batch/orchestrator.go:543`:

```go
// Downstream consumers like requeue-unblocked.sh use these names
// to construct file paths, so we reject /, \, .., <, and >.
```

After this change, the path-traversal rationale no longer applies (queue-status check replaces filesystem check). The validation itself should remain (defense in depth), but the comment's justification will be wrong. The design puts this in Phase 5 (cleanup). Good.

## 5. Phasing Strategy

The design proposes 5 sequential phases. The question is whether a more incremental delivery would be safer.

**Current phasing:**
1. Extract shared failure loading (refactor)
2. Implement `internal/requeue/`
3. Create `cmd/queue-maintain/` (rename + wire)
4. Update workflows
5. Cleanup

**Alternative phasing (fix the bug first, consolidate later):**

1. **Fix the immediate bug**: Add `queue-maintain` build/run steps to `update-queue-status.yml` using a minimal requeue implementation that checks queue status (no satisfies resolution yet). This unblocks packages whose recipe names match `blocked_by` names exactly. Ship it.
2. **Add satisfies resolution**: Extend the requeue logic with satisfies-aware checking. This unblocks the `openssl@3` -> `openssl` cases. Requires the satisfies data source decision from section 3.
3. **Consolidate CLI**: Rename `cmd/reorder-queue/` to `cmd/queue-maintain/`, wire both operations together.
4. **Fix single-owner**: Remove dead queue-write code from `batch-generate.yml`.
5. **Cleanup**: Delete bash script, update comments.

**The alternative is safer** because step 1 is a small, testable change that fixes the most common case (exact name matches). If the satisfies data source turns out to be complicated (section 3), the exact-match fix is already in production.

However, the design's phasing is also reasonable if the team is confident about the satisfies data source and wants to avoid shipping a known-incomplete fix. The design's phases are logically sequential and each builds on the previous one.

**Advisory**: Either phasing works. The key risk is shipping all 5 phases as one PR vs. separate PRs. A single PR makes the rename, the new package, the workflow changes, and the bash deletion all-or-nothing. If any phase has a bug, the whole thing needs to revert.

## 6. Verified Design Claims

### Claim: batch-generate.yml queue writes are dead code

**Verified.** Lines 1116 and 1122 use `.packages[]` on `priority-queue.json`, which uses `.entries[]`. The jq select finds nothing, the queue is written unchanged. Removing this code is safe.

### Claim: `requeue-unblocked.sh` reads legacy format

**Verified.** Line 93: `jq -r '.packages[] | select(.status == "blocked") | .id' "$QUEUE"` reads from `priority-queue-$ECOSYSTEM.json` (line 18) with `.packages[]` and `.id`. The unified queue uses `.entries[]` and `.name`.

### Claim: embedded recipes don't appear in `blocked_by`

**Not independently verifiable from the code.** The design states this based on analysis of failure data. The code in `internal/batch/orchestrator.go` generates `missing_dep` failures when `tsuku install` can't find a recipe. Embedded recipes are always found (they're compiled into the binary), so it's logically correct that they won't appear in `blocked_by`. But this is a runtime property, not a code-level guarantee. If a future change makes embedded recipe loading fallible, this assumption breaks silently.

**Advisory**: Add a comment in the requeue code explaining why embedded recipes are excluded from the satisfies check, so the next developer doesn't add redundant handling.

## 7. Summary of Findings

### Blocking

1. **Unspecified satisfies data source for CI** (Section 3). The design says the satisfies index "can be built from the registry manifest or from recipe TOML files on disk" but doesn't commit to one. In CI, the availability of these sources differs. The implementation will need to pick one, and the wrong choice will silently fail to unblock packages. The design should specify which source `queue-maintain` uses and how it's accessed in the CI environment.

### Advisory

2. **`reorder.Run()` signature change is under-specified** (Section 2). The design shows pseudocode but doesn't specify what happens to `DryRun`, `OutputFile`, and other `Options` fields. The next developer implementing Phase 3 will need to guess.

3. **`requeue.Run()` modifies queue in place without name signaling** (Section 4). The `Run` name doesn't convey mutation. A doc comment is sufficient; no rename needed.

4. **Consider fix-first phasing** (Section 5). Shipping exact-match requeue first (no satisfies) and adding satisfies resolution as a follow-up reduces risk if the satisfies data source proves complicated.

5. **`--skip-requeue` and `--skip-reorder` create an undocumented dependency** (Section 4). Running reorder without requeue operates on stale statuses. Document this or remove the flags.

6. **Phase 1 extraction is load-bearing in name only** (Section 2). Moving ~60 lines of file-loading code to a shared package is fine but doesn't warrant a separate "phase" label. Fold it into Phase 2.
