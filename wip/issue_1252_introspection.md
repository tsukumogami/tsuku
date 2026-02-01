# Issue #1252 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-batch-recipe-generation.md`
- Sibling issues reviewed: #1349, #1350, #1352 (closed 2026-02-01), #1254, #1256, #1257, #1273 (all closed recently)
- Prior patterns identified:
  - Batch ID generation implemented in shell (`.github/workflows/batch-generate.yml` lines 696-709)
  - Circuit breaker updates implemented in shell (lines 711-719)
  - Queue status updates implemented in shell (lines 752-761)
  - Recipe list tracking in shell variables (lines 599-600, 615, 646, 665)
  - All batch orchestration happens in GitHub Actions YAML, not Go

## Current State Analysis

### What Actually Exists

The batch-generate workflow (`.github/workflows/batch-generate.yml`) is fully operational:
- **Generation job** (lines 39-99): Runs `cmd/batch-generate` Go tool, builds multi-platform tsuku binaries
- **Validation jobs** (lines 101-520): Four parallel jobs validating on Linux x86_64/arm64 and macOS arm64/x86_64
- **Merge job** (lines 522-837): Shell-based aggregation, constraint derivation, batch_id generation, circuit breaker updates, queue status updates, PR creation

The Go orchestrator (`internal/batch/orchestrator.go`) exists but is narrowly scoped:
- Reads priority queue, selects candidates
- Invokes `tsuku create` via subprocess for each package
- Tracks generation success/failure
- Writes failure records
- Updates queue package statuses

### What Issue #1252 Specifies

1. **Preflight job**: Circuit breaker check, budget check, package list output
2. **Rate limiting**: Per-ecosystem sleep between package generations
3. **Go structs for batch-control.json**
4. **Orchestrator integration**: Check circuit breaker before selecting candidates

## Gap Analysis

### Major Gaps

#### 1. Architectural Mismatch: Go vs Shell Implementation

The issue assumes a Go-centric architecture with a preflight job that:
- Reads `batch-control.json` and outputs package lists as JSON artifacts
- Is a separate GitHub Actions job that passes data to downstream jobs

**Reality:** The workflow evolved differently:
- The generation job directly runs `cmd/batch-generate`, which reads the queue internally
- No preflight job exists or is needed — `cmd/batch-generate` already handles candidate selection
- Batch ID generation, circuit breaker updates, and queue status updates all happen in shell within the merge job (implemented by #1349, #1350, #1352)

**Why this matters:** Implementing AC#1-4 (preflight job, circuit breaker check, package list output, batch ID generation) would create duplicate logic. The batch ID generation in AC#4 is already done (lines 696-709). The circuit breaker integration in AC#1 would need to happen in `cmd/batch-generate`, not a separate preflight job.

#### 2. Batch ID Generation Already Implemented

AC#4 says "Batch ID generation" is needed. But #1349 (closed 2026-02-01) already implemented this in the merge job (lines 696-709). The format matches the spec: `YYYY-MM-DD-<ecosystem>` with sequence numbers for same-day batches.

**Why this matters:** AC#4 is already complete. Implementing it again would duplicate logic.

#### 3. Go Structs for batch-control.json Have No Consumer

AC#8 specifies Go struct definitions for `batch-control.json`. But the actual circuit breaker logic lives in `scripts/update_breaker.sh` (a shell script called by the merge job). The Go orchestrator doesn't read `batch-control.json` — it only reads the priority queue.

**Why this matters:** Without a Go consumer for these structs, they'd be unused code. The circuit breaker integration would need to be refactored to use Go instead of shell, which conflicts with the established pattern from #1352.

### Moderate Gaps

#### 4. Rate Limiting Location Unclear

AC#5-7 specify rate limiting with per-ecosystem sleep durations. The design says this should happen in the orchestrator, but doesn't specify where.

**Current state:** `internal/batch/orchestrator.go` processes packages sequentially but has no sleep between them.

**Required clarification:** Should rate limiting be:
- Added to `cmd/batch-generate/main.go` (the CLI tool)?
- Added to `internal/batch/orchestrator.go` as a configurable field?
- Both (config in Orchestrator, sleep call in CLI tool)?

The design says "rate-limiting mechanism to the orchestrator" but doesn't specify if this means the Go struct or the CLI binary.

### Minor Gaps

#### 5. Batch Control JSON Schema Exists But Isn't Used

The file `batch-control.json` exists at the repo root with the structure specified in AC#8, but:
- No Go code reads it
- The shell script `scripts/update_breaker.sh` writes to it via `jq`
- The merge job reads it via `jq` (line 711-719)

**Resolution:** If we add Go structs, they should have actual readers. Otherwise, document that `batch-control.json` is shell-managed.

## Recommendation

**Re-plan**

### Reasoning

The issue spec was written before the actual implementation evolved. Three major implementation decisions invalidate large parts of the spec:

1. **No preflight job is needed.** The generation job already runs `cmd/batch-generate`, which internally reads the queue and selects candidates. Adding a separate preflight job would create unnecessary job-to-job artifact passing.

2. **Batch ID generation is complete.** Issue #1349 delivered this in the merge job. AC#4 is already done.

3. **Circuit breaker and queue updates are shell-based.** Issues #1350 and #1352 implemented these in the merge job using `jq` and shell scripts. Refactoring to Go would undo that work.

### What Actually Needs to Be Done

The only unimplemented requirement is **rate limiting** (AC#5-7). This should be a focused issue:

**New scope:**
- Add `RateLimitSeconds` field to `batch.Config` struct
- Add sleep call in `internal/batch/orchestrator.go` between package generations
- Pass rate limit value from `cmd/batch-generate` CLI flags (with per-ecosystem defaults)
- Add test coverage for rate limiting behavior

**Out of scope (already done or not needed):**
- Preflight job (not needed — `cmd/batch-generate` handles this)
- Batch ID generation (done in #1349)
- Circuit breaker check in preflight (not applicable — no preflight job)
- Package list output (done — `cmd/batch-generate` reads queue internally)
- Go structs for batch-control.json (not needed — shell-based implementation is working)

### Proposed Action

Close #1252 and create a new, focused issue for rate limiting:

**Title:** `feat(batch): add rate limiting to package generation loop`

**Scope:**
- Add `RateLimitSeconds` to `batch.Config`
- Sleep between packages in `orchestrator.Run()`
- Wire CLI flag with per-ecosystem defaults
- Test coverage

This removes the 80% of AC items that are either already done or based on an architecture that doesn't exist, and focuses on the one missing piece.

## Blocking Concerns

**None for a new focused issue.** The rate limiting work is straightforward and well-scoped.

**For the current issue spec:** Attempting to implement AC#1-4 and AC#8-9 as written would:
- Create duplicate batch ID logic (conflicts with #1349)
- Add unused Go structs (no consumer)
- Require refactoring shell-based circuit breaker to Go (conflicts with #1352)
- Add a preflight job that duplicates `cmd/batch-generate` logic
