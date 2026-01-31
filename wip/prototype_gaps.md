# Batch Pipeline Prototype Gap Report

Findings from the first successful batch-generate run (PR #1265: 14 recipes generated, 11 failures).

## Gap 1: Existing recipes cause false failures

**Source:** 3 packages failed with "recipe already exists" (git, docker, mise)

The queue was seeded with packages that already had recipes. `tsuku create` correctly refuses to overwrite without `--force`, but the orchestrator classifies these as `validation_failed` — polluting the failure log with false failures that aren't capability gaps.

**Three layers, three fixes:**

1. **Seed tool** (`cmd/seed-queue`): should skip packages that already have recipes when populating the queue. Prevents junk entries from entering the queue in the first place. Future work — update the `Merge()` function in `internal/seed/queue.go` to accept a recipes directory and filter against it.

2. **Orchestrator** (`internal/batch/orchestrator.go`): should trust the queue. If something is `pending`, generate it. This enables intentional re-runs (operator puts a package back on the queue to regenerate). No change needed.

3. **Failure classification**: the orchestrator should recognize "recipe already exists" as a distinct outcome from real generation failures. Either classify as a separate category (e.g., `already_exists`) or mark the queue entry as `skipped` instead of `failed`. This keeps gap analysis data clean.

**CLI safety net**: `tsuku create` already refuses to overwrite without `--force`. Since the batch orchestrator doesn't pass `--force`, accidental overwrites in auto-merged PRs are already prevented.

**Resolution:**

1. **Safety net (future)**: Add an optional `force_override` field to the queue package schema. Add a CI check that prevents merging a queue file containing a duplicate of an existing recipe unless `force_override: true` is set on that entry. This prevents accidental overwrites in auto-merged PRs while still allowing intentional regeneration.

2. **Immediate fix**: Manually remove every queue entry that duplicates an existing recipe in the registry. This unblocks the current PR.

3. **Seed tool (future)**: Filter out existing recipes during queue population so duplicates don't enter the queue in the first place.

**Affected issues:** #1267 (seed tool filtering), #1268 (CI queue validation with force_override)

## Gap 2: Homebrew deterministic fallback incomplete

**Source:** 8 packages failed with "no LLM providers available" after "falling back to LLM"

Bottle inspection runs but isn't sufficient for all packages. The builder falls back to LLM even when running without API keys, producing a hard failure instead of a structured deterministic-failed error. The design acknowledges this ("Homebrew builder requires refactoring") and points to DESIGN-homebrew-deterministic-mode.md (#1188, marked done).

**Resolution:** No action for M-BatchPipeline. Filed needs-design issue for the Homebrew builder to produce structured `DeterministicFailedError` when bottle inspection is insufficient and no LLM keys are present, and to integrate any interface changes with the batch pipeline. The 32% LLM-fallback rate (8/25) is useful validation spike data — worse than the estimated 10-15%.

**Affected issue:** #1266 (needs-design, outside M-BatchPipeline scope)

## Gap 3: Workflow builds from source vs install.sh

**Source:** We changed the workflow to `go build` from source because the released binary lacked `--output`. The design says "install released tsuku via install.sh."

Building from source caught the `--output` bug immediately. install.sh would have masked it until the next release. Issue #1253 adds version pinning to install.sh, but the workflow no longer uses install.sh.

**Resolution:** Rescoped #1253 to "pinned release with build-from-source fallback." Moved out of critical path — now depends on #1258 (last issue in pipeline). Build from source remains the default during development. Updated design doc table and dependency graph.

**Affected issue:** #1253 (rescoped and deferred)

## Gap 4: No rate limiting between packages

**Source:** The orchestrator processes packages sequentially with no delay. The 25-package prototype run worked, but the design requires per-ecosystem rate limiting (1 req/sec for most ecosystems).

**Resolution:** No action needed. Already covered in #1252 acceptance criteria (per-ecosystem sleep intervals). The 25-package prototype didn't hit any limits due to small batch size and authenticated GitHub API access.

**Affected issue:** #1252 (already tracked)
