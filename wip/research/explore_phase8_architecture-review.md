# Architecture Review: DESIGN-requeue-on-recipe-merge.md

Phase 8 final review by architect-reviewer.

Ref: `docs/designs/DESIGN-requeue-on-recipe-merge.md`

## 1. Is the architecture clear enough to implement?

**Yes, with one interface gap to close.** The design specifies the new package (`internal/requeue/`), the modified package (`internal/reorder/`), the CLI rename (`cmd/reorder-queue/` -> `cmd/queue-maintain/`), and the workflow changes with enough precision to implement. The `Options`/`Result`/`Change` structs are defined, the function contract is spelled out (returns modified queue, does NOT save), and the workflow YAML snippets are concrete.

The gap: the design shows `requeue.Run(queue, *failuresDir)` in the CLI pseudocode (line 215), but the `internal/requeue/` interface section defines `Options` with `QueueFile`, `FailuresDir`, and `DryRun` fields. The `Run()` function description says it "Loads the unified queue via `batch.LoadUnifiedQueue()`" (line 184), which means it loads internally. But the CLI pseudocode loads the queue once at the top (`queue := batch.LoadUnifiedQueue(...)`) and passes it to both `requeue.Run()` and `reorder.Run()`. These two descriptions contradict each other. See finding #1 below.

## 2. Are there missing components or interfaces?

### 2a. `internal/reorder.Run()` signature needs to change

Currently `reorder.Run(opts Options)` loads the queue from disk internally via `batch.LoadUnifiedQueue(opts.QueueFile)` (reorder.go line 72). The design's CLI pseudocode shows the caller loading the queue once and passing it to both steps. If `reorder.Run()` keeps its current signature, `cmd/queue-maintain/` either:
- Passes a path and lets reorder reload from disk (duplicating the load, and losing requeue's in-memory changes unless requeue writes first), or
- Needs `reorder.Run()` to accept the queue object directly.

The design mentions "The reorder package's `Run()` function signature stays the same" (line 193). This conflicts with the single-load goal. Either `reorder.Run()` needs a new overload that accepts `*batch.UnifiedQueue`, or the CLI writes the queue between steps and reorder reloads it. The former is cleaner.

### 2b. Shared failure loading extraction

The design says to extract `loadBlockerMap()` and `loadBlockersFromFile()` from `internal/reorder/` to `internal/blocker/` (Phase 1). This is the right call. `internal/blocker/` already has `ComputeTransitiveBlockers` and `BuildPkgToBare`. Adding the JSONL loading there makes it a complete blocker analysis package.

However, `loadBlockerMap()` currently returns `map[string][]string` (dependency name -> list of blocked package IDs). The requeue logic needs a different view: for each blocked entry, what are its `blocked_by` dependencies? The current map is inverted relative to what requeue needs. The design says requeue checks "its `blocked_by` list (from failure records)" (line 188), which means requeue needs per-package blocker lists, not the inverted dependency-to-packages map. The implementer will need to build this reverse index from the JSONL data. This should be called out explicitly -- it's not the same data structure `loadBlockerMap()` returns.

### 2c. No `batch/` changes needed

The `UnifiedQueue` struct (`internal/batch/bootstrap.go:32-36`) uses `.entries[]` in JSON. `LoadUnifiedQueue`/`SaveUnifiedQueue` are already available. No changes needed in the batch package.

## 3. Are the implementation phases correctly sequenced?

**Yes.** The phases form a valid dependency chain:

1. Phase 1 (extract shared failure loading) -- pure refactor, no behavior change. Must come first since Phase 2 depends on it.
2. Phase 2 (implement `internal/requeue/`) -- uses Phase 1's shared loading. Can be tested independently.
3. Phase 3 (create `cmd/queue-maintain/`) -- wires Phases 1+2 together with existing reorder.
4. Phase 4 (update workflows) -- deploys Phase 3 into CI. Must come last since it removes the old script.
5. Phase 5 (docs) -- no code dependency.

One sequencing consideration: Phase 3 renames `cmd/reorder-queue/` to `cmd/queue-maintain/`. If any active branch references the old binary path, the rename creates a merge conflict. Check whether `batch-generate.yml` or any other workflow currently builds `cmd/reorder-queue/`. I don't see it in the current `batch-generate.yml`, so the rename should be safe.

## 4. Are there simpler alternatives we overlooked?

**The design already evaluated the reasonable alternatives.** The "fix the bash script" option was considered and rejected for good reasons (duplicates Go code, two-tool coordination). The "new standalone CLI" option was rejected (coordination cost without benefit since the operations always pair).

One simplification the design could consider: **skip the `internal/requeue/` package entirely and put the requeue logic directly in `cmd/queue-maintain/main.go`**. The requeue operation is ~30 lines: iterate blocked entries, check if all their blockers have status "success" in the queue, flip to "pending". It doesn't need its own package with Options/Result/Change types unless it has independent callers. If the answer is "only queue-maintain calls it," then the package boundary adds overhead without value.

However, this is advisory, not blocking. The separate package makes testing cleaner and follows the existing pattern where `internal/reorder/` is a package with its own types.

## 5. Does the "single owner" pattern for queue writes actually work?

### What the design proposes

`update-queue-status.yml` is the sole writer of `priority-queue.json` to main. `batch-generate.yml` runs `queue-maintain` locally in its working tree but does NOT commit queue changes to the batch PR. Queue updates happen after PR merge via `update-queue-status.yml`.

### What happens with batch-generate's working tree

`batch-generate.yml` currently does `git add recipes/ data/ batch-control.json` at line 1132. The `data/` glob captures everything under `data/`, including `data/queues/priority-queue.json`. The design proposes changing this to:

```
git add recipes/ data/failures/ data/metrics/ batch-control.json
```

This is correct -- it explicitly lists the `data/` subdirectories that should go into the PR, excluding `data/queues/`. The working tree will have queue modifications (from queue-maintain running in the generate and merge jobs), but they won't be staged because the `git add` doesn't include them. This works.

**One concern**: the merge job at line 828 does `cp -r artifacts/data/* data/` which copies the generate job's `data/` artifacts (including potentially `data/queues/`) into the working tree. Then line 1132 does `git add ... data/`. If we change line 1132 to exclude `data/queues/`, the queue modifications from both the generate job's requeue-unblocked and the merge job's requeue-unblocked will remain unstaged. This is the desired behavior.

But: the generate job uploads `data/` as an artifact at line 170-174:
```yaml
- name: Upload passing recipes
  path: |
    recipes/
    data/
```

This uploads ALL of `data/`, including `data/queues/`. The merge job downloads this into `artifacts/data/` and copies it to `data/`. So the merge job's working tree gets the generate job's queue state. Then the merge job runs its own `requeue-unblocked.sh` (line 1006), which writes to `data/queues/priority-queue-homebrew.json` (the legacy queue -- note: NOT `priority-queue.json`). The current `requeue-unblocked.sh` writes to the legacy per-ecosystem file, not the unified queue. So currently there's no conflict on the unified queue file from the script.

After the proposed changes, `queue-maintain` will write to `priority-queue.json` in the merge job's working tree. The `git add` at line 1132 must exclude it. The design's proposed change handles this correctly.

**Also note**: `data/queues/batch-results.json` is also under `data/queues/`. The current PR creation step reads this file (lines 1110-1118) to mark entries in the queue. If we change `git add` to `data/failures/ data/metrics/`, we lose `batch-results.json` from the PR commit. But `batch-results.json` is a transient file (used within the workflow run), not something that needs to persist on main. However, this should be verified: does anything on main read `batch-results.json`? The circuit breaker update step (lines 970-1003) reads it, but that runs before the commit. So excluding `data/queues/` from `git add` should be fine.

**Wait -- there's another file**: `data/queues/priority-queue-homebrew.json` (the legacy per-ecosystem queue). The current `requeue-unblocked.sh` reads/writes this file. The design says this is out of scope ("Retiring the per-ecosystem queue format entirely (separate cleanup)"). If `queue-maintain` replaces `requeue-unblocked.sh` and operates on the unified queue, the legacy per-ecosystem queue files won't be updated anymore. Is anything still reading the per-ecosystem files? The dashboard and `update-queue-status.yml` use the unified queue. If the legacy files are truly dead, this is fine. If something still reads them, this is a silent regression. The design acknowledges this is out of scope but should note the risk.

### Timing and race condition analysis

The single-owner pattern works because:
1. `update-queue-status.yml` triggers on push to main with `recipes/**` path filter
2. It runs after the batch PR merges
3. It has its own concurrency group (`queue-operations-status-update`)
4. `batch-generate.yml` has a separate concurrency group (`queue-operations`)

These two workflows can still run concurrently -- a batch run could be in progress when a recipe merge triggers `update-queue-status.yml`. Since only `update-queue-status.yml` writes to the queue on main, and `batch-generate.yml` only writes to a branch, there's no conflict on the main branch. The concurrency within `update-queue-status.yml` is handled by its own group (cancel-in-progress: false, so runs queue up).

**One edge case**: if `update-queue-status.yml` is mid-push and another recipe merge arrives, the second run waits (concurrency group queues it). The second run's `git pull --rebase` at line 195 picks up the first run's changes. This is handled.

### Verdict

The single-owner pattern works correctly. The `git add` change is the critical piece and the design specifies it correctly.

## 6. Is the `internal/requeue/` package interface right?

### Should `Run()` take the queue object directly or load it internally?

**It should take the queue object directly.** The design's `Run()` description (line 184) says it loads the queue internally, but the CLI pseudocode (line 215) passes the queue in. The CLI pseudocode is right: loading once and passing to both requeue and reorder avoids double-loading and ensures reorder sees requeue's changes without a disk round-trip.

The `Options` struct should drop `QueueFile` and `Run()` should accept `*batch.UnifiedQueue` as a parameter:

```go
func Run(queue *batch.UnifiedQueue, opts Options) (*Result, error)
```

Where `Options` has only `FailuresDir` and `DryRun`.

Similarly, `reorder.Run()` should gain an overload or be modified to accept the queue directly. Currently it loads from disk (reorder.go line 72). For the single-load pattern to work, either:
- Both `Run()` functions accept the queue directly (clean)
- The CLI writes the queue between steps (ugly but works)

**Recommendation**: modify both packages to accept the queue object. The CLI handles load and save. The packages handle logic only. This follows the dependency direction principle: packages are lower-level than the CLI.

## Specific Code Path Questions

### Q: Does queue-maintain need to re-read the file after the status update step?

**No.** The `update-queue-status.yml` workflow runs the status update step first (marks recipes as "success" in the working tree), then the design adds queue-maintain as a subsequent step. Since both operate on the same working tree files, queue-maintain reads the already-modified `priority-queue.json` from disk. No re-read needed -- it's reading the file for the first time, and that file already has the status updates applied by the previous step.

If queue-maintain loaded the queue in the previous step and cached it, that would be a problem. But since the status update is bash/jq and queue-maintain is a separate process invocation, the file on disk is the source of truth.

**Should the status update step be moved into queue-maintain?** No. The status update logic involves recipe file parsing (extracting sources from TOML), source matching, and confidence adjustment. This is specific to the "recipe just merged" event and doesn't belong in a general-purpose queue maintenance tool. Keeping them separate maintains single responsibility: the status update step knows about recipe-to-queue mapping; queue-maintain knows about requeue+reorder.

### Q: If we exclude `data/queues/priority-queue.json` from the batch PR, are there other files in `data/` that should still be included?

Looking at what's under `data/`:
- `data/queues/priority-queue.json` -- EXCLUDE (single owner is update-queue-status.yml)
- `data/queues/priority-queue-homebrew.json` -- legacy per-ecosystem queue, probably exclude (out of scope for this design)
- `data/queues/batch-results.json` -- transient, used within the workflow only, safe to exclude
- `data/failures/*.jsonl` -- INCLUDE (batch generates these during validation)
- `data/metrics/*.jsonl` -- INCLUDE (batch generates SLI metrics)
- `data/disambiguations/` -- possibly included if batch modifies them, but typically not
- `data/dep-mapping.json` -- static dependency mapping, shouldn't change during batch
- `data/README.md` -- static

The design's proposed `git add recipes/ data/failures/ data/metrics/ batch-control.json` is correct. It captures the three categories that batch legitimately produces while excluding the queue files. The `data/disambiguations/` directory is not modified by the batch merge job and can stay out.

## Findings

### Finding 1: Contradictory `Run()` interface (Advisory)

`docs/designs/DESIGN-requeue-on-recipe-merge.md` lines 184 vs 215 -- The `internal/requeue/` section says `Run()` "Loads the unified queue via `batch.LoadUnifiedQueue()`" but the CLI pseudocode passes the queue as a parameter: `requeue.Run(queue, *failuresDir)`. The pseudocode is correct (supports single-load). Resolve the contradiction: `Run()` should accept `*batch.UnifiedQueue` and the `Options` struct should drop `QueueFile`. Same applies to `reorder.Run()` if the single-load goal is preserved.

**Advisory** because this is a design document inconsistency, not a code pattern violation. The implementer will discover it immediately.

### Finding 2: Existing `.packages[]` bug in batch-generate (Blocking -- pre-existing)

`batch-generate.yml` lines 1116 and 1122 use `.packages[]` to update queue entries:
```bash
'(.packages[] | select(.name == $name)).status = "success"'
```

But the unified queue uses `.entries[]`, not `.packages[]`. The `update-queue-status.yml` workflow correctly uses `.entries[$idx]` (lines 142, 157, 162). This means the queue status updates in the batch PR creation step are already silently failing (jq selects nothing, writes the queue unchanged). This is a pre-existing bug that the design's proposal to remove these lines (lines 1109-1124) would fix as a side effect.

**Blocking** in the sense that it confirms the design's decision to remove this code path is correct. The code is already broken and should be removed, not fixed.

### Finding 3: Reverse index needed for requeue (Advisory)

The design says requeue checks "its `blocked_by` list (from failure records)" for each blocked entry (line 188). But `loadBlockerMap()` returns `map[string][]string` keyed by dependency name (blocker -> blocked packages). Requeue needs the inverse: for a given package, what are its blockers? The implementer needs to build this reverse index from JSONL data or from the blocker map. The design should note this data structure difference explicitly.

**Advisory** because the implementer will realize this during Phase 2 and the JSONL files contain the raw data needed to build either direction.

### Finding 4: Legacy per-ecosystem queue orphaning (Advisory)

`scripts/requeue-unblocked.sh` currently writes to `data/queues/priority-queue-homebrew.json` (the legacy per-ecosystem format). The design deletes this script and replaces it with `queue-maintain`, which operates on the unified `priority-queue.json`. After this change, nothing updates the legacy per-ecosystem queue files. If any downstream consumer reads them (dashboards, reports, other scripts), they'll show stale data.

The design marks legacy queue retirement as "out of scope" which is fine, but should explicitly note that `priority-queue-homebrew.json` will stop receiving requeue updates. If it's already dead, no risk. If anything reads it, there's a silent regression.

**Advisory** because the design acknowledges the scope boundary.

### Finding 5: `reorder.Run()` signature change needed (Advisory)

`internal/reorder/reorder.go` line 71 -- `Run(opts Options)` loads the queue from disk internally. For the proposed single-load pattern in `cmd/queue-maintain/`, either `Run()` needs to accept the queue object or the CLI must write between steps. The design says "The reorder package's `Run()` function signature stays the same" (line 193), which conflicts with the single-load CLI pseudocode. The signature should change to accept `*batch.UnifiedQueue`.

**Advisory** because the two-load alternative (CLI writes, reorder reloads) works correctly, just wastes a disk round-trip.

### Finding 6: Artifact upload includes queue files (Advisory)

`batch-generate.yml` line 170-174 uploads all of `data/` as an artifact, including `data/queues/`. The merge job downloads and copies these to its working tree (line 828). After the proposed changes, `queue-maintain` runs in both the generate job and the merge job, both modifying `priority-queue.json` in their working trees. The generate job's queue modifications get uploaded as artifacts and copied into the merge job's tree, then the merge job's `queue-maintain` overwrites them. This is wasteful but not incorrect since the `git add` excludes queue files.

To be cleaner, the artifact upload could exclude `data/queues/` by listing `data/failures/` and `data/metrics/` instead. But this isn't required for correctness.

**Advisory** because the behavior is correct with the proposed `git add` change.

## Summary

The design is sound and ready for implementation. The core decisions (Go consolidation, single command, queue-status check, single owner) are well-motivated and the workflow integration is correct. The single-owner pattern eliminates the concurrent-writer problem cleanly.

Key items to address before or during implementation:

1. Resolve the `Run()` interface contradiction (accept queue object directly vs load internally)
2. Note that `loadBlockerMap()` returns the wrong orientation for requeue's needs
3. Confirm nothing reads the legacy `priority-queue-homebrew.json` before proceeding (or note the risk explicitly)

The pre-existing `.packages[]` bug in batch-generate lines 1116-1122 validates the design's decision to remove that code path.
