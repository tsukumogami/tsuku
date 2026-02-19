# Architecture Review: Pipeline Blocker Tracking

## Date: 2026-02-18

## Scope

Review of DESIGN-pipeline-blocker-tracking.md Solution Architecture and Implementation Approach sections, with reference to current code in `internal/batch/orchestrator.go`, `internal/dashboard/dashboard.go`, and `cmd/tsuku/install.go`.

---

## 1. Is the architecture clear enough to implement?

**Yes, with caveats.** The design is unusually specific -- it includes near-final code snippets for every change. This makes it straightforward for an implementer to follow. However, the specificity creates a different risk: the snippets don't account for some discrepancies with the actual codebase, which could lead to confusion during implementation.

### Discrepancies between design and code

**Two `categoryFromExitCode` functions exist.** The design mentions adding exit code 3 to "the orchestrator's `categoryFromExitCode()`" but doesn't acknowledge that a separate `categoryFromExitCode()` exists in `cmd/tsuku/install.go` (lines 326-337). The CLI version already handles `ExitRecipeNotFound` (returns `"recipe_not_found"`) and uses named constants. The orchestrator version (lines 456-471) uses literal integers, doesn't handle exit code 3, and uses different category strings (`"api_error"` vs `"network_error"` for exit code 5, `"validation_failed"` vs `"install_failed"` for the default). The design's proposed `categoryFromExitCode` snippet on line 207 uses named constants (`ExitRecipeNotFound`, etc.), but the current orchestrator code uses integer literals and doesn't import the exit code constants. This matters because the constants are defined in `cmd/tsuku/exitcodes.go`, and importing from `cmd/` into `internal/batch/` is not possible in Go. The design should specify either redefining the constants in the batch package or continuing to use literals.

**Category string mismatch.** The orchestrator's `categoryFromExitCode` returns `"api_error"` for exit 5, while the CLI's returns `"network_error"`. The design's proposed code shows `"api_error"` for exit 5, which matches the orchestrator's current behavior. This is fine, but should be noted as intentional -- the two functions serve different contexts (CLI user-facing output vs. internal pipeline recording).

**Blocker struct changes don't match current code.** The design proposes `DirectCount` and `TotalCount` fields on `Blocker`, but the current struct has a single `Count` field (dashboard.go line 122). The design's proposed JSON shows `direct_count` and `total_count`, but never specifies what happens to the existing `Count` field. Should it be removed? Kept as an alias for `TotalCount`? This needs clarification to avoid breaking existing dashboard consumers.

### Recommendation

Add a subsection to the design that explicitly lists the two `categoryFromExitCode` functions and states that only the orchestrator's version needs modification. Note the category string differences as intentional.

---

## 2. Are there missing components or interfaces?

### Missing: `BlockedBy` field on `QueueEntry`

The design's proposed `Run()` change (lines 253-267) sets `pkg.Status = StatusBlocked` for generate failures with `blocked_by`, but there's no mechanism to persist the `blocked_by` list on the queue entry itself. The `QueueEntry` struct (defined in `queue_entry.go`) doesn't have a `BlockedBy` field. The failure record gets the data, but the queue entry only gets a status flip. This means `requeue-unblocked.sh` can't check which blockers are resolved by reading the queue alone -- it must cross-reference failure records.

Looking at the existing `requeue-unblocked.sh` (lines 56-79), it does read blockers from failure JSONL files, not from the queue. So this is consistent with the current approach and isn't actually missing. The design is correct to keep `blocked_by` in failure records rather than duplicating it on queue entries.

### Missing: Deduplication in transitive computation

The design's `computeTransitiveBlockers` function (lines 297-317) has a subtle issue. The `blockers` map values are `[]string` containing package IDs (like `"homebrew:ffmpeg"`). These can contain duplicates from multiple failure files -- the same package may appear in multiple JSONL records across different batch runs. The existing `computeTopBlockers` already shows this: tests include duplicate entries (test at line 214: `"glib": {"imagemagick", "ffmpeg", "imagemagick", "ffmpeg", "gstreamer"}`). The design's proposed code counts direct dependents via `total++` inside the loop over `blockers[dep]`, which means duplicates inflate the count. The current code already handles this by using a `blocked` set (`map[string]bool`), and the design's proposed code also uses a set approach in the recursive function -- so this is actually handled. But the outer `computeTopBlockers` function needs to be updated to use the new two-count fields, and the design doesn't show that updated outer function.

### Missing: Error handling for `tsuku create` exit codes from dependency checks

In `cmd/tsuku/create.go` (around line 591), when `tsuku create` encounters missing dependencies, it calls `exitWithCode(ExitDependencyFailed)` (exit 8). But the orchestrator's `generate()` method only treats exit code 5 (`ExitNetwork`) as retryable. Exit code 8 flows to the non-retryable path and creates a `FailureRecord` with `categoryFromExitCode(8)` = `"missing_dep"`. This part works correctly today for the category. The gap is only in the `BlockedBy` field, which `generate()` never populates. The design correctly identifies this and proposes `extractBlockedByFromOutput()` to fill the gap.

However, `tsuku create` can also exit with code 3 (`ExitRecipeNotFound`) at line 617 when `builder.CanBuild()` returns false. This path has no dependency information at all -- it just means the package doesn't exist in the ecosystem source. The design adds exit code 3 to `categoryFromExitCode()` as `"recipe_not_found"`, which is correct. But the generate-phase extraction may not find any "recipe X not found in registry" pattern in this case, since the error message is "package 'X' not found in Y" (line 616). This is fine -- the category will be `"recipe_not_found"` with empty `blocked_by`, which correctly describes the situation.

### Missing: Schema migration for `Blocker`

The dashboard JSON output changes from `{count: N}` to `{direct_count: N, total_count: N}`. The `website/pipeline/index.html` JavaScript must handle both old and new formats during the transition, or the website will break between the time the code deploys and the dashboard regenerates. The design should specify whether the old `count` field is kept as a fallback.

---

## 3. Are the implementation phases correctly sequenced?

**Mostly yes.** The three phases have the right dependencies:

- Phase 1 (CLI + orchestrator) must come first because it establishes correct recording.
- Phase 2 (remediation) depends on Phase 1 being correct.
- Phase 3 (dashboard) can be developed in parallel with Phase 2 but needs remediated data to test with.

### Concern: Phase 2 timing

The design says "depends on Phase 1 being reviewed and correct, so the remediation script uses the same logic." But the remediation script (`scripts/remediate-blockers.sh`) is proposed as a bash+jq tool, not Go code. It won't literally reuse the Go extraction logic -- it'll implement equivalent regex matching in jq/bash. This creates a subtle risk: the two implementations could diverge. If the remediation runs before Phase 1 merges and the regex is slightly different, you get inconsistent data.

**Recommendation**: Ship Phases 1 and 2 in the same PR. The remediation runs once as part of the PR, so it must be in the same PR as the code fix. The design already implies this ("runs as part of the implementation PR"), but the phase separation suggests they could be separate PRs.

### Concern: Phase 3 parallel development

The design says Phase 3 "can be developed in parallel with Phase 2 but tested after remediation runs." In practice, Phase 3's transitive computation changes the `Blocker` struct (adding `DirectCount`/`TotalCount`, potentially removing `Count`). This is a breaking change for the frontend. The frontend update (Component 7) must ship simultaneously with the dashboard computation change. This is already in Phase 3, so it's fine, but the parallel development claim is misleading -- you can't deploy Phase 3 backend changes without the frontend update.

---

## 4. Are there simpler alternatives we overlooked?

### Alternative: Skip the `classifyInstallError()` reorder entirely

The design proposes reordering `classifyInstallError()` as a fix. But the orchestrator never calls `classifyInstallError()` directly -- it only sees exit codes. The exit code precedence bug only matters when the CLI is invoked directly by users (not the orchestrator). For the orchestrator path, the fix is already handled by adding exit code 3 to `categoryFromExitCode()` and by extracting `blocked_by` from output. The `classifyInstallError()` reorder is still a correct fix for the CLI's own JSON output, but it's independent of the pipeline problem. Consider whether it belongs in this design or should be a separate small fix.

**Assessment**: Keep it in. The CLI fix ensures the `--json` output (which `validate()` consumes via `parseInstallJSON`) reports the correct category. If `classifyInstallError` returns exit 3 instead of 8 for a dependency-wrapping-registry-error, then `validate()` receives exit 3, and `parseInstallJSON` falls back to `categoryFromExitCode(3)`. With the orchestrator fix (adding case 3 -> `"recipe_not_found"`), this path now produces the wrong category for what is actually a dependency failure. So the CLI fix IS needed for correctness, not just for polish.

### Alternative: Write remediation in Go instead of bash+jq

The design's Decision 2 actually mentions this: "Write a Go script (or extend the existing `cmd/batch-generate` with a `--remediate` flag)." But the architecture section shows `scripts/remediate-blockers.sh` as the chosen implementation. Go would be safer for JSONL manipulation and would share the regex pattern directly. See the detailed analysis in section 5 below.

### Alternative: Normalize blocker keys at write time, not read time

Instead of normalizing keys during dashboard generation (stripping ecosystem prefixes), ensure that `loadFailures()` always writes normalized keys. Currently, `blocked_by` values from the orchestrator are bare dependency names (like `"glib"`), but the package IDs in the blockers map values are prefixed (like `"homebrew:ffmpeg"`). The mismatch is between **keys** (dependency names, already bare) and **values** (package IDs, prefixed). For transitive lookup, you need to check whether a value (prefixed package ID) also appears as a key (bare dependency name). This means stripping the prefix from values during lookup, which is what the design proposes.

This is actually the right approach. You can't normalize at write time because the blocker keys ARE already bare names -- the problem is in the lookup direction, not the storage format.

---

## 5. Specific Implementation Questions

### 5a. Sharing the `reNotFoundInRegistry` regex pattern

The regex `reNotFoundInRegistry` is defined in `cmd/tsuku/install.go` at line 361:

```go
var reNotFoundInRegistry = regexp.MustCompile(`recipe (\S+) not found in registry`)
```

The `extractBlockedByFromOutput()` function needs this pattern in `internal/batch/orchestrator.go`. Go doesn't allow importing from `cmd/` packages into `internal/` packages.

**Options:**

1. **Copy the pattern**: Define the same regex in `internal/batch/orchestrator.go`. Simple, but creates two copies that could drift.
2. **Move to a shared package**: Create something like `internal/errpatterns/patterns.go` containing shared regex patterns.
3. **Move to `internal/registry/`**: Since the pattern matches registry-related error messages, the `registry` package is a natural home.

**Recommendation**: Option 1 (copy the pattern). The regex is a single line, tightly coupled to the error message format in the registry code. If the message format changes, both the CLI and orchestrator will need updating regardless -- the breakage is visible. A shared package adds indirection for a single pattern. Add a comment in both locations pointing to the other:

```go
// reNotFoundInRegistry matches "recipe X not found in registry" error messages.
// Keep in sync with the identical pattern in cmd/tsuku/install.go.
var reNotFoundInRegistry = regexp.MustCompile(`recipe (\S+) not found in registry`)
```

If more patterns need sharing in the future, refactor to a shared package at that point.

### 5b. Cycle detection in `computeTransitiveBlockers()`

The design proposes a memo-based approach:

```go
memo[dep] = 0  // Mark as in-progress to detect cycles
```

**Is this sufficient?**

The current codebase's `computeTransitiveBlockers` (dashboard.go lines 452-474) uses a `memo map[string][]string` where presence in the map indicates "already computed." A cycle would cause infinite recursion because the function checks `memo[dep]` before recursing, but a cycle like `A blocks B blocks A` would:

1. Call `computeTransitiveBlockers("A", ...)` -- not in memo, proceed
2. Process `B` (blocked by `A`) -- recurse into `computeTransitiveBlockers("B", ...)`
3. Process `A` (blocked by `B`) -- recurse into `computeTransitiveBlockers("A", ...)`
4. `"A"` is now in memo (set in step 1 after the check)... wait, actually the memo is set AFTER the computation, at line 472: `memo[dep] = result`. So step 1 doesn't set memo until after the recursion completes. This means the current code DOES have an infinite recursion bug for cycles.

The design's proposed fix sets `memo[dep] = 0` before recursing, which breaks the cycle. When step 3 looks up `"A"`, it finds `0` and returns immediately. This correctly handles cycles.

**Is the memo approach sufficient?** Yes, with one caveat: the `memo[dep] = 0` sentinel means that if a cycle exists, the transitive count for nodes in the cycle will be undercounted (nodes visited after the cycle detection get 0 instead of their true count). This is acceptable because cycles in dependency data are data quality issues (package A shouldn't depend on package B which depends on package A), and the correct response is to not infinitely recurse, not to compute "correct" cycle counts.

**However**, the design's proposed code has a subtle bug. It checks `bare != dep` to avoid self-loops, but this doesn't prevent the A->B->A cycle described above. The `memo[dep] = 0` sentinel handles the cycle, but the `bare != dep` check is insufficient as the sole cycle guard. Fortunately, both mechanisms exist, so it works. The `bare != dep` check prevents a different issue: a package that lists itself in `blocked_by` (data quality bug).

**Recommendation**: The memo-based approach is correct and sufficient. Consider adding a brief comment explaining why `memo[dep] = 0` is set before recursion (cycle detection).

### 5c. Remediation script: bash+jq vs. Go

The design proposes `scripts/remediate-blockers.sh` using bash+jq to modify JSONL files. The concern is about handling complex messages with special characters.

**Risks with bash+jq:**

1. **JSONL line handling**: `jq` handles JSON correctly regardless of special characters in string values. The real risk is with lines that span multiple lines (pretty-printed JSON), but the JSONL files use single-line JSON records, so this isn't an issue.
2. **Regex matching in jq**: jq's `test()` function supports PCRE-like patterns. Matching `recipe (\S+) not found in registry` is straightforward in jq.
3. **In-place modification**: Writing modified content back to the same file requires temp-file-and-rename, which bash handles fine. But error handling is weaker -- a failed `jq` command could silently corrupt a file if `set -e` doesn't catch the right failure mode.
4. **Cross-referencing with queue**: The queue file is standard JSON (not JSONL), so `jq` handles it natively.

**Risks with Go:**

1. **Shares the exact regex**: The Go code can import or duplicate `reNotFoundInRegistry`, ensuring identical matching.
2. **Stronger error handling**: Type-safe JSON parsing catches structural issues that `jq` might silently ignore.
3. **Testing**: A Go remediation tool can have unit tests.
4. **Build complexity**: Requires `go run` or a separate binary, slightly heavier than a bash script.

**Recommendation**: Go is the better choice here. The design itself acknowledges this as an option ("Write a Go script or extend the existing `cmd/batch-generate` with a `--remediate` flag"). The strongest argument for Go is consistency: the failure records are written by Go code, parsed by Go code in the dashboard, and the matching pattern is a Go regex. Adding a bash+jq layer in the middle introduces a different toolchain for one-time use. The `--remediate` flag on `cmd/batch-generate` is the cleanest approach.

However, if the team is more comfortable with bash+jq and the remediation truly runs only once, bash+jq is adequate. The `jq` JSON handling is solid, and the special-character concern is a non-issue because `jq` operates at the JSON level, not the string level.

### 5d. StatusBlocked for generate failures: semantic correctness

The design proposes that when `generate()` produces a failure with non-empty `blocked_by`, `Run()` sets `StatusBlocked`:

```go
if len(genResult.Failure.BlockedBy) > 0 {
    result.Blocked++
    pkg.Status = StatusBlocked
}
```

**The concern**: When the failure is from `generate()` (recipe creation failed), the recipe was never created. There's nothing to "unblock" in the sense that retrying the generation is what would happen once the dependency exists. Is `StatusBlocked` semantically correct here, or should it be a different status?

**Analysis**: `StatusBlocked` is defined as "Blocked by missing dependency" (queue_entry.go line 51). The generate phase failing because a dependency recipe doesn't exist IS a case of being blocked by a missing dependency. The package can't be generated until the dependency's recipe exists. When the dependency recipe is eventually created, `requeue-unblocked.sh` should flip this package back to `pending`, and the next batch run will retry the generation.

This is semantically correct. The alternative -- leaving it as `StatusFailed` with a backoff timer -- is worse because:

1. The backoff timer would retry the package even though nothing has changed (the dependency still doesn't exist).
2. Repeated retries waste API rate limit budget on requests that will definitely fail.
3. The failure count increments on each retry, pushing the backoff window out to days/weeks.

With `StatusBlocked`, the package sits idle until `requeue-unblocked.sh` detects that the dependency now exists. No wasted retries, no escalating backoff.

**One concern**: The design's proposed `Run()` code doesn't call `o.recordFailure(idx)` for blocked entries. This means `entry.FailureCount` doesn't increment and `entry.NextRetryAt` doesn't get set. This is correct behavior -- blocked entries shouldn't accumulate failure counts. But it also means that if the same package gets blocked multiple times (blocked, requeued, generates again, still blocked by a different dep), the failure count stays at whatever it was before the first block. This is fine.

**However**, there's a subtle issue the design misses: when `Run()` doesn't call `recordFailure(idx)`, it also doesn't call `recordSuccess`. So for blocked entries, only `pkg.Status = StatusBlocked` is set. The `o.queue.Entries[idx]` pointer update happens via `pkg := &o.queue.Entries[idx]` at line 116, so the status change IS persisted when the queue is saved. This is correct.

**Another issue**: The current `selectCandidates()` function only selects entries with `StatusPending` or `StatusFailed` (line 221). Blocked entries are excluded. This means once an entry is blocked, it won't be picked up again until `requeue-unblocked.sh` flips it back to `pending`. This is the intended flow and is correct.

**Recommendation**: `StatusBlocked` is the right choice for generate failures with missing dependencies. No change needed.

---

## 6. Summary of Findings

### Implementable as-is (no changes needed)

- CLI `classifyInstallError()` reorder
- Orchestrator `categoryFromExitCode()` addition (exit code 3)
- `generate()` dependency extraction approach
- `Run()` StatusBlocked for blocked generate failures
- Data flow from recording through dashboard
- Phase sequencing (1 before 2, 3 after or parallel with 2)

### Needs clarification in the design

1. **Two `categoryFromExitCode` functions**: The design should explicitly name both and note that only the orchestrator's needs the exit code 3 addition. The category string differences (`"api_error"` vs `"network_error"`, etc.) should be acknowledged as intentional.

2. **Exit code constants**: The design snippets use named constants (`ExitRecipeNotFound`) but the orchestrator uses integer literals. Clarify that the constants should be redefined in the batch package or that literals should continue to be used.

3. **`Blocker` struct migration**: Specify what happens to the existing `Count` field. Recommend replacing it with `TotalCount` (no backward-compatible alias needed since the dashboard is regenerated atomically).

4. **Remediation script language**: The Decision 2 section mentions both Go and bash options, but the architecture section only shows the bash version. Pick one. Recommend Go for consistency with the codebase.

### Risks identified

1. **Regex pattern duplication**: Low risk, manageable with cross-referencing comments. Refactor to shared package if more patterns are needed later.

2. **Dashboard schema change**: The `Blocker` struct change from `count` to `direct_count`/`total_count` must ship with the frontend update in a single deployment. Not risky if done in one PR, but would break if phased separately.

3. **Existing `requeue-unblocked.sh` compatibility**: The script references `priority-queue-$ECOSYSTEM.json` (line 18) with an ecosystem suffix, but the current unified queue file is `priority-queue.json` without a suffix. The script appears to be out of date with the unified queue migration. This is a pre-existing issue, not introduced by this design, but the remediation step (which also modifies the queue) should use the correct queue path.
