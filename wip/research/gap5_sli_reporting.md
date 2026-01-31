# Per-Environment SLI Metrics for Batch Recipe Generation

## 1. Current Metrics Infrastructure

**Nothing exists yet.** The batch pipeline is designed but not implemented. Key observations:

- `data/metrics/` directory does not exist
- `data/failures/` directory does not exist
- `batch-control.json` does not exist
- No `GITHUB_STEP_SUMMARY` usage in batch workflows
- The only batch-related workflows are `batch-operations.yml` (circuit breaker checks) and `r2-cost-monitoring.yml`

The batch generation design (DESIGN-batch-recipe-generation.md) specifies an SLI collection mechanism: each generation job appends a JSONL line to `data/metrics/batch-runs.jsonl` with fields like `batch_id`, `ecosystem`, `total`, `generated`, `failed`, `validated_linux`, `validated_macos`, `merged`, `success_rate`, `duration_seconds`, and `timestamp`.

However, this schema is **batch-level, not per-environment**. The design tracks `validated_linux` and `validated_macos` as aggregate counts but doesn't break down by specific platform (linux-glibc-x86_64 vs linux-glibc-arm64 vs linux-musl-x86_64 vs darwin-arm64 vs darwin-x86_64).

The failure record format does include an `environment` field per JSONL line, so per-environment failure data is designed to exist in `data/failures/<ecosystem>.jsonl`. But no aggregation or reporting layer is planned for it.

The registry scale strategy (DESIGN-registry-scale-strategy.md, line 1119) explicitly lists "What reports do operators need?" as a medium-priority open question, and line 1144 proposes an optional `DESIGN-operator-dashboard.md` that hasn't been written.

## 2. What Per-Environment Metrics Matter

### Per-batch (immediate feedback)
- Pass/fail count by platform for the current batch
- Which recipes failed on which platforms (and why)
- Platform-specific duration (are arm64 runners slower?)
- Cost consumed by platform (macOS minutes used vs budget)

### Rolling (trend analysis)
- Per-platform pass rate over last N batches (is arm64 getting better or worse?)
- Per-platform failure category distribution (arm64 fails on "binary_incompatible" 60% of the time vs x86_64 at 5%)
- macOS budget burn rate (are we on track to exhaust the weekly 1000 minutes?)
- Per-ecosystem per-platform matrix (Homebrew has 15% arm64 failure rate, Cargo has 2%)

### Derived insights
- "Which platform is the weakest?" (highest failure rate)
- "Which capability gap would unlock the most recipes on arm64?"
- "Should we skip musl for this ecosystem?" (if failure rate is >80%)

## 3. Options Analysis

### Option A: Extend batch-runs.jsonl with Per-Platform Counters

Modify the planned `data/metrics/batch-runs.jsonl` schema to include per-platform breakdowns:

```jsonl
{"batch_id":"2026-01-29-001","ecosystem":"cargo","total":25,
 "platforms":{
   "linux-glibc-x86_64":{"tested":25,"passed":23,"failed":2},
   "linux-glibc-arm64":{"tested":23,"passed":21,"failed":2},
   "linux-musl-x86_64":{"tested":23,"passed":18,"failed":5},
   "darwin-arm64":{"tested":21,"passed":20,"failed":1},
   "darwin-x86_64":{"tested":21,"passed":20,"failed":1}
 },
 "merged":18,"timestamp":"2026-01-29T10:30:00Z"}
```

| Criterion | Assessment |
|-----------|------------|
| Implementation complexity | Low. The merge job already aggregates per-platform results. Adding counters to the JSONL line is a few lines of Go in `internal/batch/`. |
| Time to insight | Immediate for per-batch data. Rolling trends require reading multiple JSONL lines with `jq`. |
| New infrastructure | None. Uses the already-planned `data/metrics/` file. |
| Day-one usefulness | Yes. First batch run produces per-platform data. |

**Limitation:** Raw JSONL is not operator-friendly for trend analysis. Requires `jq` pipelines or a separate script to aggregate.

### Option B: GitHub Actions Job Summary

Each batch run writes a markdown table to `$GITHUB_STEP_SUMMARY`:

```markdown
## Batch 2026-01-29-001: cargo

| Platform | Tested | Passed | Failed | Pass Rate |
|----------|--------|--------|--------|-----------|
| linux-glibc-x86_64 | 25 | 23 | 2 | 92% |
| linux-glibc-arm64 | 23 | 21 | 2 | 91% |
| linux-musl-x86_64 | 23 | 18 | 5 | 78% |
| darwin-arm64 | 21 | 20 | 1 | 95% |
| darwin-x86_64 | 21 | 20 | 1 | 95% |

**Top failure categories (arm64):** binary_incompatible (1), missing_dep (1)
**macOS budget used:** 42 min / 1000 min weekly
```

| Criterion | Assessment |
|-----------|------------|
| Implementation complexity | Low. A few `echo` statements in the workflow or a Go function that outputs markdown. |
| Time to insight | Immediate. Operators click on the workflow run and see the table. |
| New infrastructure | None. Built into GitHub Actions. |
| Day-one usefulness | Yes. Every run is self-documenting. |

**Limitation:** No cross-run trends. Each summary lives on its own workflow run page. Operators must manually compare runs. Summaries are ephemeral (tied to workflow run retention, typically 90 days).

### Option C: Reporting Script

A Go tool (`cmd/batch-report`) or shell script that reads `data/failures/<ecosystem>.jsonl` and `data/metrics/batch-runs.jsonl`, aggregates across runs, and prints a report.

```
$ go run ./cmd/batch-report --last 10

Platform Pass Rates (last 10 batches):
  linux-glibc-x86_64:  94.2%  (471/500)
  linux-glibc-arm64:   89.1%  (418/469)
  linux-musl-x86_64:   72.3%  (339/469)
  darwin-arm64:        93.8%  (394/420)
  darwin-x86_64:       93.1%  (391/420)

Top arm64 Failure Categories:
  binary_incompatible: 28 (55%)
  missing_dep:         15 (29%)
  no_platform_assets:   8 (16%)

macOS Budget (this week): 312 / 1000 min (31.2%)
```

| Criterion | Assessment |
|-----------|------------|
| Implementation complexity | Medium. Needs JSONL parsing, aggregation logic, formatted output. ~200-400 lines of Go. |
| Time to insight | On-demand. Operator runs the script when they want a report. |
| New infrastructure | None. Reads files already in the repo. |
| Day-one usefulness | Needs accumulated data from multiple runs to show trends. Single-run data is better served by Option B. |

**Advantage over A and B:** Cross-run aggregation and trend analysis without external infrastructure.

### Option D: Telemetry Worker Integration

Send per-platform metrics to the existing Cloudflare Worker telemetry service. Expose via API endpoint.

| Criterion | Assessment |
|-----------|------------|
| Implementation complexity | High. Requires telemetry worker changes, new D1 table schema, API endpoint, and CI integration to POST metrics after each run. |
| Time to insight | Real-time via API once built. |
| New infrastructure | D1 table, Worker route, authentication for CI-to-Worker calls. |
| Day-one usefulness | No. Requires the telemetry worker to be extended first. The batch pipeline itself doesn't exist yet. |

**Advantage:** Queryable API, potential for dashboards, no repo size growth. But this is essentially Phase 2 of the registry scale strategy (DESIGN-registry-scale-strategy.md, Phase 2: "Failure Analysis Backend"). Building it now front-loads infrastructure that the phasing explicitly defers.

## 4. Recommendation

**Use Option A + B together for Phase 1b. Add Option C when trend data matters (Phase 2+). Skip Option D until the failure analysis backend is built.**

Rationale:

1. **Option A (structured JSONL with per-platform fields)** is the foundation. It costs almost nothing to implement -- the merge job already collects per-platform results, so writing them as structured counters instead of discarding them is trivial. This data is the source of truth for everything else.

2. **Option B (GitHub Actions job summary)** provides immediate operator visibility with zero infrastructure. Every batch run becomes self-documenting. Operators don't need to check out the repo or run scripts -- they click on the workflow run in the GitHub UI. This is the fastest path to "operators can see per-platform breakdown."

3. **Option C (reporting script)** becomes valuable once there are 5-10+ batch runs to aggregate. It should be built when Phase 2 work starts, as that's when trend analysis matters for capacity planning. It reads the JSONL files that Option A already produces.

4. **Option D (telemetry worker)** is Phase 2 scope per the registry scale strategy. Don't build it early. The JSONL-based approach (A+B+C) carries Phase 1b without external infrastructure.

### Specific Schema Recommendation

Extend the planned `batch-runs.jsonl` schema to include a `platforms` object:

```jsonl
{"batch_id":"...","ecosystem":"...","total":N,
 "platforms":{
   "<env>":{"tested":N,"passed":N,"failed":N,"skipped":N,"duration_seconds":N}
 },
 "merged":N,"macos_minutes_used":N,"timestamp":"..."}
```

The `skipped` field captures recipes not tested on a platform due to progressive validation (failed on Linux, so never reached macOS). This distinguishes "failed on arm64" from "never tested on arm64."

The `macos_minutes_used` field at the top level enables budget tracking without parsing platform durations.

### Implementation Effort

| Item | Effort | When |
|------|--------|------|
| Per-platform fields in batch-runs.jsonl | ~1 hour | Phase 1b (merge job implementation) |
| GitHub Actions job summary table | ~2 hours | Phase 1b (merge job implementation) |
| Reporting script (cmd/batch-report) | ~1-2 days | Phase 2 or when operators request it |
| Telemetry worker integration | ~3-5 days | Phase 2 (with failure analysis backend) |

The first two items should be part of the batch pipeline implementation issues. They add negligible scope to the merge job that's already being built.
