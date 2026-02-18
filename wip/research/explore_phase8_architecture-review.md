# Architecture Review: DESIGN-unified-batch-pipeline

**Reviewer:** Architecture review agent
**Date:** 2026-02-17
**Status:** Proposed design, pre-implementation

---

## 1. Is the Architecture Clear Enough to Implement?

**Verdict: Yes, with caveats.** The design is well-structured and specific about what changes where. The proposed `selectCandidates()` pseudocode, `Config` struct changes, and `BatchResult` changes are concrete enough to code against. However, several implementation details need clarification before work begins.

### Gaps in Specification

**A. How does the CLI receive breaker state?**

The design says `Config.BreakerState` replaces the workflow's inline breaker check, but `cmd/batch-generate/main.go` currently receives configuration through CLI flags. The design doesn't specify how `BreakerState` gets from `batch-control.json` to the Go binary. Options include:

- A new `-breaker-state` flag accepting JSON
- A `-control-file` flag pointing at `batch-control.json`, with the binary reading it directly
- Passing breaker state via environment variable

The workflow snippet shows reading breaker state in a shell step and passing it as `$GITHUB_OUTPUT`, but the design doesn't show the `batch-generate` binary consuming that output. The current `main.go` has no mechanism for this. Recommendation: add a `-control-file` flag to `batch-generate` and have it read `batch-control.json` directly, keeping the breaker logic in Go rather than split between shell and Go.

**B. Manual dispatch ecosystem filter.**

The design says "manual dispatch can still target a specific ecosystem for debugging" but doesn't specify how this works after `Config.Ecosystem` is removed. If someone manually dispatches with `ecosystem: cargo`, the orchestrator needs a way to filter. Options:

- Keep an optional `FilterEcosystem` field in Config (empty = all)
- Pass it as a CLI flag that re-adds the prefix filter when set

This needs to be explicit, since the design says "Remove `Config.Ecosystem` field" but then describes retaining ecosystem-specific dispatch capability.

**C. `BatchResult` output mechanism.**

The design specifies a new `BatchResult.Ecosystems map[string]int` field and says the workflow post-batch step reads `batch-results.json`. But the current `cmd/batch-generate/main.go` doesn't write a results JSON file -- it prints to stderr and writes a summary markdown. The design should specify that `main.go` writes `batch-results.json` (or that `SaveResults` does this), and what schema it uses.

**D. Failure file grouping.**

The design says "a single batch may produce multiple failure files (one per ecosystem that had failures)." The current `WriteFailures()` in `results.go` takes a single `ecosystem` string parameter. The design's code for `SaveResults()` currently calls `WriteFailures(o.cfg.FailuresDir, o.cfg.Ecosystem, result.Failures)`. The implementation needs to:

1. Group `result.Failures` by ecosystem (using the `PackageID` prefix)
2. Call `WriteFailures()` once per ecosystem group

This grouping logic isn't shown in the design and should be.

---

## 2. Are There Missing Components or Interfaces?

### Missing: `github` ecosystem rate limit

The `ecosystemRateLimits` map in `orchestrator.go` defines rate limits for `homebrew`, `cargo`, `npm`, `pypi`, `go`, `rubygems`, `cpan`, and `cask`. But many re-routed packages use `github:` as their source. There is no `github` entry in the rate limits map. When the orchestrator looks up `ecosystemRateLimits["github"]`, it gets zero, which means no rate limiting between GitHub API calls. Given GitHub's API rate limits (60 requests/hour unauthenticated, 5000/hour authenticated), this is a gap. The design should add a `github` rate limit entry (likely 1-2 seconds).

### Missing: Half-open batch size limiting in orchestrator

The design moves the circuit breaker check into the orchestrator's `selectCandidates()`, but the current workflow has half-open probe logic that limits batch size to 1. If the breaker is half-open, the workflow sets `batch_size_override=1`. After moving breaker checks into the orchestrator, this needs to be replicated: the orchestrator should limit candidates from a half-open ecosystem to 1 entry. The proposed `selectCandidates()` pseudocode skips open ecosystems but doesn't handle half-open limiting.

### Missing: Per-ecosystem result tracking in `BatchResult`

The design says the workflow's post-batch step iterates over `batch-results.json`'s `.per_ecosystem[$e].succeeded` and `.per_ecosystem[$e].total`. But `BatchResult` as proposed only has `Ecosystems map[string]int` (entry counts). It doesn't have per-ecosystem success/failure breakdowns. The struct needs something like:

```go
type EcosystemResult struct {
    Total     int `json:"total"`
    Succeeded int `json:"succeeded"`
    Failed    int `json:"failed"`
    Blocked   int `json:"blocked"`
}
// In BatchResult:
PerEcosystem map[string]EcosystemResult `json:"per_ecosystem"`
```

### Present but not called out: Dashboard hardcoded homebrew assumption

In `internal/dashboard/dashboard.go` line 431, there's a hardcoded `pkgID := "homebrew:" + record.Recipe` comment "Assume homebrew for now." Similarly, `failures.go` line 229 defaults to `eco = "homebrew"` for per-recipe format records without an ecosystem field. These are pre-existing issues but will become more visible once mixed-ecosystem batches produce failure records from multiple ecosystems. The design's Phase 3 should include fixing these hardcoded values.

### Present but not called out: Workflow queue file reference

The design correctly identifies in Phase 2, item 2 that line 1118 of the workflow references `priority-queue-${{ env.ECOSYSTEM }}.json` -- a legacy per-ecosystem queue file. The actual line in the current workflow is:

```yaml
QUEUE_FILE="data/queues/priority-queue-${{ env.ECOSYSTEM }}.json"
```

This file doesn't exist (the unified queue is at `data/queues/priority-queue.json`). This is a bug in the current workflow that likely causes silent failures in the "Create pull request" step's queue status update logic. The design should emphasize that this is both a fix for the unified pipeline and a fix for an existing bug.

---

## 3. Are the Implementation Phases Correctly Sequenced?

**Verdict: Yes, the sequencing is correct.** Phase 1 (orchestrator) is the foundation. Phase 2 (CLI/workflow) depends on Phase 1's new Config and BatchResult. Phase 3 (dashboard) depends on Phase 1's new output format.

### Minor sequencing concern

Phase 2 item 2 ("Update workflow") bundles several changes that could be split:

- Removing `ECOSYSTEM` env var and `-ecosystem` flag (depends on Phase 1)
- Fixing the `priority-queue-$ECOSYSTEM.json` reference (independent bug fix, could ship now)
- Changing concurrency group (could ship after Phase 1)
- Updating post-batch breaker updates (depends on Phase 1's per-ecosystem results)
- Updating PR creation (depends on Phase 1's batch ID format)

The `priority-queue-$ECOSYSTEM.json` fix could be an independent pre-work PR since it's a standalone bug. This reduces the Phase 2 diff size.

### Backward compatibility during rollout

The design doesn't discuss whether Phases 1-3 must ship atomically or can be rolled out incrementally. Since the workflow, CLI binary, and dashboard are all versioned together (monorepo), they'd likely ship in one PR. But if phases are separate PRs:

- Phase 1 alone would break the CLI since `main.go` still requires `-ecosystem`
- Phase 2 alone without Phase 1 would fail since `Config.Ecosystem` wouldn't exist

So Phases 1 and 2 must ship together. Phase 3 can follow separately since the dashboard is tolerant of format mismatches (the design calls for backward-compat parsing).

---

## 4. Are There Simpler Alternatives We Overlooked?

### Considered: Minimal patch without Config refactoring

The absolute smallest fix would be to change `selectCandidates()` line 158 from:

```go
prefix := o.cfg.Ecosystem + ":"
```

to:

```go
// Don't filter by ecosystem prefix
```

...without changing Config, BatchResult, or the workflow at all. Just delete the two lines (158 and 165-167). This would immediately process all pending entries because the cron still passes `-ecosystem homebrew`, but the orchestrator would ignore it for selection purposes. Rate limiting would still use `o.cfg.Ecosystem`, which means all entries would get homebrew's rate limit (1s) regardless of actual ecosystem.

**Why this doesn't work:** Rate limits would be wrong (rubygems needs 6s, not 1s). Circuit breaker state would update for "homebrew" even when failures are from cargo. Failure files would be named `homebrew-*.jsonl` even for cargo failures. The dashboard would show all runs as "homebrew."

This confirms the design's approach is the right level of change. The Config refactoring is necessary to preserve per-ecosystem safety, not over-engineering.

### Considered: "github" rate limit as a default

Rather than adding `github` to `ecosystemRateLimits`, consider adding a default rate limit for unknown ecosystems:

```go
rateLimit := ecosystemRateLimits[eco]
if rateLimit == 0 {
    rateLimit = 1 * time.Second // default for unknown ecosystems
}
```

This is more future-proof than maintaining the map for every ecosystem. The explicit map entries would serve as overrides (e.g., rubygems at 6s).

---

## 5. Accuracy of Proposed Changes Against Actual Code

### `internal/batch/orchestrator.go`

The design accurately describes the current code:

- `Config.Ecosystem` exists (line 50)
- `selectCandidates()` computes `prefix := o.cfg.Ecosystem + ":"` (line 158) and filters with `strings.HasPrefix(entry.Source, prefix)` (line 165)
- `Run()` uses `ecosystemRateLimits[o.cfg.Ecosystem]` once (line 94)
- `generateBatchID()` takes ecosystem as parameter (line 365)
- `SaveResults()` calls `WriteFailures()` with `o.cfg.Ecosystem` (line 145)

The proposed changes align with the actual code structure. No phantom references.

### `cmd/batch-generate/main.go`

The design accurately describes the current code:

- `-ecosystem` flag exists and is required (lines 13, 21-25)
- `cfg.Ecosystem` is set from the flag (line 34)
- No breaker state loading exists (would need to be added)

### `.github/workflows/batch-generate.yml`

The design accurately identifies:

- `ECOSYSTEM` env var defaulting to `homebrew` (line 33)
- Per-ecosystem concurrency group (line 38)
- Per-ecosystem circuit breaker preflight (lines 82-115)
- `-ecosystem` flag passed to batch-generate (line 140)
- `priority-queue-$ECOSYSTEM.json` legacy reference (line 1118)

### Additional accuracy notes

- The `QueueEntry.Ecosystem()` method exists at `queue_entry.go` line 116 and works as the design assumes.
- The `ecosystemRateLimits` map exists at `orchestrator.go` line 33 but is missing `github`.
- `WriteFailures()` in `results.go` does take an `ecosystem` parameter (line 125).
- The `BatchResult` struct in `results.go` does have `Ecosystem string` (line 14).
- `update_breaker.sh` already takes ecosystem as its first argument, confirming no changes are needed there.

---

## Summary of Findings

| # | Finding | Severity | Recommendation |
|---|---------|----------|----------------|
| 1 | CLI doesn't specify how breaker state reaches the binary | Medium | Add `-control-file` flag to `batch-generate` |
| 2 | Manual dispatch ecosystem filter unspecified after Config.Ecosystem removal | Medium | Add optional `FilterEcosystem` field |
| 3 | `BatchResult` missing per-ecosystem success/failure breakdown | High | Add `PerEcosystem map[string]EcosystemResult` |
| 4 | No `github` entry in `ecosystemRateLimits` | Medium | Add entry or add default rate limit for unknown ecosystems |
| 5 | Half-open batch size limiting not replicated in orchestrator | Medium | Add half-open check limiting per-ecosystem candidates to 1 |
| 6 | Dashboard hardcoded homebrew assumptions | Low | Fix in Phase 3 |
| 7 | Workflow `priority-queue-$ECOSYSTEM.json` reference is existing bug | Low | Can be fixed independently as pre-work |
| 8 | `BatchResult` output file not specified | Medium | Specify JSON output path in design |
| 9 | Failure file grouping logic not shown | Low | Add pseudocode for grouping failures by ecosystem |
| 10 | Phases 1 and 2 must ship together | Info | Note in implementation approach |
