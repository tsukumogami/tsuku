# Phase 4 Review: DESIGN-requeue-on-recipe-merge.md

Reviewer: architect-reviewer
Date: 2026-02-21

## 1. Problem Statement Assessment

The problem statement is specific and grounded in real data:

- **Concrete example**: PR #1801 adding `gmp` and `openssl@3` recipes, with named packages (`aarch64-elf-gcc`, `aarch64-elf-gdb`, `afflib`) staying blocked for hours. This is verifiable -- the queue currently shows `gmp` with status "success" at line 16836 and `aarch64-elf-gdb` still "blocked" at line 569 of `priority-queue.json`.
- **Format mismatch is real**: `requeue-unblocked.sh` reads `priority-queue-$ECOSYSTEM.json` with `.packages[]` and `.id` fields (lines 18, 93, 102-103). The live unified queue uses `priority-queue.json` with `.entries[]` and `.name` fields. This is documented as a known gap in `DESIGN-registry-scale-strategy.md` at lines 213, 754, 995, and 1218.
- **Shared data observation is accurate**: `internal/reorder/reorder.go` does load the same failure JSONL data (`loadBlockerMap`, line 147) and unified queue (`batch.LoadUnifiedQueue`, line 72) that the requeue logic needs.

One minor gap: the problem statement says "after a PR adds the missing recipe, affected packages stay blocked until the next batch cycle" but doesn't quantify the current hourly cadence. Line 4 of `batch-generate.yml` (`cron: '0 * * * *'`) confirms hourly. Worth making explicit in the doc since "roughly hourly" is softer than what the code says.

**Verdict**: Specific enough to evaluate solutions. No significant gaps.

## 2. Missing Alternatives

The alternatives analysis is reasonably complete. Two additions worth considering:

### 2a. Webhook/event-driven requeue without consolidation

The design conflates two independent improvements: (1) fixing the format mismatch and (2) reducing latency by triggering on recipe merge. You could fix the bash script to read the unified format AND add it as a step in `update-queue-status.yml` without consolidating it into the Go tool. This alternative isn't a strong contender (it still duplicates failure loading), but its absence makes the consolidation seem like the only path when in fact the latency fix alone could be done in bash.

This doesn't change the recommendation -- the Go consolidation is clearly better. But excluding this middle option makes the "fix the bash script" alternative weaker than it needs to be, because it's evaluated only as "fix format, stay in batch-generate.yml" rather than "fix format AND trigger on merge."

### 2b. Separate requeue step in update-queue-status.yml calling the existing bash script

Similar to above: add a step after the status-update commit in `update-queue-status.yml` that calls a fixed `requeue-unblocked.sh`. Lightweight, solves the latency problem, but still two tools reading the same data.

Neither of these are missing in a way that changes the outcome, but the design would be more rigorous if it acknowledged that the latency fix and the consolidation are separable concerns and explained why doing both at once is preferred.

## 3. Rejection Rationale Assessment

### Decision 1 alternatives

- **"Fix the bash script"**: Rejected as "quick fix but duplicates failure-loading logic that exists in Go." This is fair. The bash script's `blocked_pairs` function (lines 57-79 of `requeue-unblocked.sh`) reimplements what `loadBlockerMap` does in Go. However, the rationale also says "we'd still need a separate workflow step to run reorder after requeue." This conflates the language decision with the CLI structure decision. The language could be bash while still solving the sequencing problem by calling the Go reorder tool afterwards. Minor conflation, not a strawman.

- **"New standalone Go CLI"**: Rejected as "adds coordination complexity without benefit." Fair. The two operations share inputs and should always run together. No strawman here.

### Decision 2 alternatives

- **"Subcommands"**: Rejected as "adds complexity for operations that should always happen together." Fair. The example of running `requeue` without `reorder` leaving stale ordering is a real risk.

- **"Keep name, add flag"**: Rejected as "binary name becomes misleading." This is the weakest rejection -- a misleading name is a cosmetic concern, not a structural one. But it's also the weakest alternative, so the calibration is appropriate.

### Decision 3 alternatives

- **"Filesystem check"**: Rejected as "requires recipe files in the checkout and couples the tool to the directory layout." Fair, though slightly overstated. The recipe directory layout is stable.

- **"Both checks"**: Rejected as "adds complexity for a case that doesn't matter." This rejection hinges on the claim that recipes outside the queue don't need consideration. See section 5 below for where this breaks down.

**Verdict**: No strawmen. The "fix the bash script" alternative is slightly under-explored (it could include triggering on merge), but this doesn't change the recommendation.

## 4. Unstated Assumptions

### 4a. `update-queue-status.yml` runs before queue-maintain in the same workflow

Decision 3 states: "`update-queue-status.yml` marks recipes as 'success' in the same workflow run, before the queue-maintain step." This is the core sequencing assumption. Currently, `update-queue-status.yml` doesn't run queue-maintain at all -- that's the planned addition. The assumption is that the new step will be added after the existing commit/push step (lines 176-203 of `update-queue-status.yml`).

But there's a subtlety: the commit/push step pushes the status change to `main`. If queue-maintain runs in the same job after the push, it needs to operate on the just-pushed state. This works if the checkout already has the updated queue file in the working tree (it does, since jq modifies it in place before commit). But the design should be explicit about this: queue-maintain runs on the local working tree, not on a fresh checkout. If it ran on a fresh checkout, there'd be a race where the push hasn't propagated yet.

### 4b. All blockers are identified by name, and names match queue entry names

The blocker map from failure JSONL uses names like `"gmp"`, `"bdw-gc"`, `"aarch64-elf-binutils"`. These match queue entry names exactly. If a blocker were identified by source (e.g., `"homebrew:gmp"`) instead of bare name, the queue-status-check lookup would fail. The current data confirms bare names are used, but this assumption should be documented.

### 4c. The legacy per-ecosystem queue files are not needed for the requeue logic

The design explicitly marks "retiring the per-ecosystem queue format" as out of scope. But `batch-generate.yml` still calls `requeue-unblocked.sh` at lines 125 and 1006, which reads the per-ecosystem format. The migration path needs to be: (1) add queue-maintain to `update-queue-status.yml`, (2) replace the two `requeue-unblocked.sh` calls in `batch-generate.yml` with queue-maintain calls, (3) then retire the script. The design should make this migration sequence explicit.

### 4d. Requeue + reorder in update-queue-status.yml doesn't conflict with the commit/push already there

The workflow already commits and pushes the status change. Adding queue-maintain after that means a second commit and push (or modifying the workflow to do a single commit after both status update and queue-maintain). The design doesn't address this sequencing within the workflow.

## 5. Technical Concerns

### 5a. Decision 3 edge case: embedded recipes not in the queue

**This is a real gap, but not a current practical concern.**

The embedded recipes in `internal/recipe/recipes/` include: `ca-certificates`, `openssl`, `ruby`, `patchelf`, `zig`, `nodejs`, `ninja`, `pkg-config`, `make`, `rust`, `zlib`, `go`, `cmake`, `libyaml`, `perl`, `gcc-libs`, `meson`, `python-standalone`.

Of these, only `ca-certificates` appears in the unified queue (line 5586, status "success"). The remaining 17 embedded recipes have no queue entry. If any of these were blockers for a "blocked" queue entry, the queue-status-check approach would miss them because there's no queue entry to find with status "success."

However, searching the current failure JSONL files shows that none of these embedded recipe names appear in `blocked_by` arrays. This makes sense: embedded recipes are the build toolchain (compilers, build systems, crypto libs). The batch pipeline's `missing_dep` detection fires when `tsuku install` fails because a recipe isn't found in the registry. Since embedded recipes ARE found (they're compiled into the binary), they never trigger `missing_dep` failures.

**The design's dismissal of the "both checks" alternative ("if a recipe isn't in the queue, it was never a candidate for the batch pipeline, so the belt-and-suspenders approach adds complexity for a case that doesn't matter") is technically correct for the current data but the reasoning is wrong.** The reason embedded recipes don't appear as blockers isn't because they aren't batch candidates -- it's because they're already resolved at install time. The design should state this more precisely. As-is, a reader might think: "what about a recipe that exists in `recipes/` but isn't in the queue?" That case CAN'T produce a `missing_dep` failure either, because the recipe file exists and would be found by `tsuku install`.

**Verdict: Advisory, not blocking.** The queue-status-check is sound because the only way a package becomes "blocked" is via a `missing_dep` failure, and the only way that failure resolves is when the missing recipe is added -- which means it gets added to the queue (via PR, then `update-queue-status.yml` marks it "success") or was already resolving at install time (embedded). The edge case doesn't exist in practice, but the reasoning in the doc should be tightened.

### 5b. Concurrency between update-queue-status.yml and batch-generate.yml

**This is a real concern that the design must address.**

The two workflows use different concurrency groups:
- `update-queue-status.yml`: `queue-operations-status-update`
- `batch-generate.yml`: `queue-operations`

These groups DON'T serialize against each other. Both workflows modify `data/queues/priority-queue.json` and push to `main`. If a recipe PR merges while a batch-generate run is in its merge job, the following race is possible:

1. `batch-generate.yml` merge job reads `priority-queue.json`, modifies statuses
2. `update-queue-status.yml` reads `priority-queue.json`, marks new recipe as "success"
3. `update-queue-status.yml` pushes first
4. `batch-generate.yml` pushes, overwriting the status update (even with rebase, the concurrent jq modifications are lost because both sides modified the same file)

Adding queue-maintain to `update-queue-status.yml` makes this worse: now the status update workflow also reorders entries, making the diff larger and merge conflicts more likely.

The design should either:
- Acknowledge this as a pre-existing issue (it exists today since `update-queue-status.yml` already writes the queue file)
- Propose a mitigation (e.g., unify the concurrency groups, or have queue-maintain use `git pull --rebase` before committing)
- Note that the retry-with-rebase logic (which both workflows already have) handles the push conflict, though it can still lose concurrent jq modifications

The retry-with-rebase approach handles the push failure but NOT the data conflict: if both workflows modify the same JSON file, `git pull --rebase` will either auto-merge (JSON files rarely merge cleanly) or conflict. The queue file is a single JSON blob, so rebase will likely produce a conflict that fails the workflow.

**Verdict: Blocking (for the design).** The design proposes adding more mutations to `update-queue-status.yml` without addressing the existing concurrent-write risk. This should be called out even if the mitigation is "accept the risk; retry logic handles most cases and the hourly batch run will catch anything missed."

### 5c. Renaming cmd/reorder-queue/ to cmd/queue-maintain/ and migration from requeue-unblocked.sh

**The design is clear on the rename but unclear on the migration path for batch-generate.yml.**

Currently:
- `batch-generate.yml` calls `./scripts/requeue-unblocked.sh` at lines 125 and 1006
- `batch-generate.yml` does NOT call `cmd/reorder-queue/` at all (confirmed: no matches for "reorder-queue" in the workflow)

The design says `batch-generate.yml` changes are out of scope ("Changes to how `batch-generate.yml` processes its generate/merge jobs"). But replacing `requeue-unblocked.sh` IS in scope ("Retiring `scripts/requeue-unblocked.sh`"). These two statements conflict: you can't retire the script without updating the two callsites in `batch-generate.yml`.

The migration should be:
1. Create `cmd/queue-maintain/` (renamed from `cmd/reorder-queue/`) with consolidated requeue+reorder logic
2. Add a queue-maintain step to `update-queue-status.yml` (after the status update)
3. Replace both `./scripts/requeue-unblocked.sh` calls in `batch-generate.yml` with `go run ./cmd/queue-maintain/` (or a pre-built binary)
4. Delete `scripts/requeue-unblocked.sh`
5. (Optional, separate PR) Delete the old `cmd/reorder-queue/` directory if not done in step 1

Step 3 is a change to `batch-generate.yml`, which the design says is out of scope. This is a scope inconsistency.

**Verdict: Advisory.** The implementation will naturally discover this. But the design should either expand scope to include the `batch-generate.yml` callsite updates, or narrow the "retire" claim to "deprecate" (keeping the script until a follow-up removes it).

## 6. Summary of Findings

### Blocking

1. **Concurrent queue writes**: The design adds queue mutations to `update-queue-status.yml` (concurrency group `queue-operations-status-update`) without addressing the concurrent-write risk with `batch-generate.yml` (concurrency group `queue-operations`). Both push modifications to the same JSON file on `main`. The design should document this risk and either mitigate it (unify concurrency groups, add file-level locking, or accept the risk with explicit rationale).

### Advisory

2. **Scope inconsistency on batch-generate.yml**: The design puts "changes to batch-generate.yml" out of scope but puts "retiring requeue-unblocked.sh" in scope. The script is called at lines 125 and 1006 of `batch-generate.yml`. Either include the callsite migration or narrow the retirement claim.

3. **Decision 3 reasoning could be more precise**: The dismissal of embedded-recipe edge cases is correct in outcome but imprecise in reasoning. The actual invariant is: a package can only be "blocked" via a `missing_dep` failure, and `missing_dep` only fires for recipes not found at install time. Embedded and registered recipes are found at install time, so they never appear as unresolved blockers. State this directly instead of "if a recipe isn't in the queue, it was never a candidate for the batch pipeline."

4. **Unstated assumption about workflow step sequencing**: Queue-maintain depends on the status-update step having already modified the queue file in the working tree. The design should make explicit that queue-maintain runs in the same job, after status updates, on the same working tree (not a fresh checkout).

### Not a concern

5. **Embedded recipes not in queue**: Not a practical edge case. Embedded recipes are resolved at install time and never produce `missing_dep` failures. Current data confirms zero overlap between embedded recipe names and `blocked_by` entries.

6. **Rename migration for cmd/reorder-queue/**: Clean rename since `batch-generate.yml` doesn't currently reference `cmd/reorder-queue/` at all. Only `requeue-unblocked.sh` needs replacement (covered by advisory #2 above).
