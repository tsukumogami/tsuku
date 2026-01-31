# Retry Mechanism for Transient Platform Validation Failures

Research output for DESIGN-batch-failure-analysis.md, addressing the open question: "How to distinguish structural vs transient failures?"

## 1. Failure Category Analysis

The failure-record schema defines six categories. Here's the transient/structural classification:

| Category | Type | Rationale |
|----------|------|-----------|
| `missing_dep` | **Structural** | Won't resolve until the dependency recipe exists. Already has re-queue trigger (dependency merge hook). |
| `no_bottles` | **Structural** | Homebrew doesn't provide pre-built bottles. Won't change between runs. |
| `build_from_source` | **Structural** | Tool requires compilation, which tsuku's deterministic path can't handle. |
| `complex_archive` | **Structural** | Archive structure too unusual for automated extraction. |
| `api_error` | **Transient** | Network timeout, rate limit, 5xx from ecosystem API. Will likely succeed on retry. |
| `validation_failed` | **Ambiguous** | Could be structural (binary genuinely doesn't work on platform) or transient (flaky runner, network issue during validation). Needs sub-classification. |

### The `validation_failed` problem

This is where transient/structural distinction matters most. The merge job sees "arm64 validation failed" but can't tell if the binary is genuinely arm64-incompatible or if the runner had a transient issue. Sub-signals that indicate transient failure:

- Exit code from `tsuku install` indicates network/download error (not execution error)
- Failure message contains "timeout", "connection refused", "rate limit", "502", "503"
- The same recipe passed on the same platform in a previous batch run
- Multiple unrelated recipes fail on the same platform in the same run (runner issue)

Sub-signals that indicate structural failure:

- Binary executes but segfaults or returns "exec format error"
- Download succeeds but binary fails `--version` check
- Consistent failure across multiple batch runs

## 2. Industry Patterns

### CI flaky test handling

Most CI systems use a simple pattern: retry N times, then mark as flaky. Specific approaches:

- **GitHub Actions**: `retry-action` retries a step up to N times with configurable backoff. No built-in classification.
- **Bazel**: Marks tests as `flaky` after inconsistent results across runs. Flaky tests are retried up to 3 times within the same invocation.
- **GitLab CI**: `retry` keyword on jobs with `when` filter (e.g., `runner_system_failure`, `stuck_or_timeout_failure`). This is the closest parallel -- classifying the failure type to decide retry eligibility.

### Package registry patterns

- **Homebrew**: Bottle builds that fail get retried once by the CI system. If still failing, the formula is marked as `pour_bottle? false` and falls back to source build. No sophisticated classification.
- **Nixpkgs Hydra**: Failed builds are retried on a schedule. If a build fails 3 times consecutively, it's marked as broken. When a dependency is rebuilt, dependents are automatically re-queued.
- **crates.io / PyPI**: These don't build packages -- they accept pre-built artifacts. Not directly applicable.

### Common pattern

The industry consensus is: **retry a small fixed number of times within the same run, then treat persistent failures as real**. Classification-based retry (analyzing error messages) is rare because it's fragile. The simpler signal is: "did it work on retry?"

## 3. Options Analysis

### Option A: In-job retry with backoff

Each platform validation job retries failed `tsuku install` up to 2 times with 30s/60s backoff before reporting failure.

| Dimension | Assessment |
|-----------|------------|
| CI cost | Low overhead: 2 retries x 60s = 2 min max per transient failure. Structural failures burn this too, but install failures are fast (seconds, not minutes). |
| Complexity | Minimal. A retry loop around the install command in the orchestrator. ~20 lines of Go. |
| Merge job interaction | None. Merge job sees final pass/fail. No changes needed. |
| Classification accuracy | Implicit: if it passes on retry, it was transient. No need to parse error messages. |
| Queue lifecycle | No impact. Recipe either passes or fails within the same run. |

**Pros**: Dead simple. Catches ~90% of transient failures (network blips, brief rate limits). No schema changes. No new workflows.

**Cons**: Burns a few minutes of CI time on structural failures that retry uselessly. Can't catch longer transient issues (rate limit that lasts 15 min, runner outage spanning the whole job).

### Option B: Failure classification + selective re-queue

The merge job parses failure messages to classify as transient/structural. Transient failures get a `retry_count` field and are re-queued for the next batch run.

| Dimension | Assessment |
|-----------|------------|
| CI cost | Good. Only transient failures are retried. Structural failures are never re-run. |
| Complexity | High. Requires: (1) error message parsing heuristics, (2) new `retry_count` field in failure schema, (3) re-queue logic in merge job, (4) max retry cap, (5) queue status for "pending_retry" vs "failed". |
| Merge job interaction | Heavy. Merge job becomes responsible for classification and re-queue decisions. |
| Classification accuracy | Fragile. Error message parsing is heuristic. Misclassification in either direction: transient marked structural (permanent false constraint) or structural marked transient (infinite retry loop without cap). |
| Queue lifecycle | Adds complexity. Need `retry_count` and `last_failed_environments` fields. Queue entries can cycle between failed and pending. |

**Pros**: Minimal wasted CI time. Creates structured data about failure types.

**Cons**: Error message parsing is the weakest part. Every new failure mode requires updating the classifier. The existing `api_error` category already captures the clearest transient case -- the hard cases (ambiguous `validation_failed`) are exactly where heuristic parsing is least reliable.

### Option C: Separate retry workflow

A dedicated workflow reads `data/failures/*.jsonl`, filters for `api_error` and recent `validation_failed` entries, and re-runs validation on just those recipes on just the failed platforms.

| Dimension | Assessment |
|-----------|------------|
| CI cost | Efficient. Only failed recipe+platform combos are retried. But a separate workflow run has fixed overhead (checkout, setup, runner spin-up). |
| Complexity | Medium-high. New workflow file, filtering logic, result reconciliation back into failure records and recipe constraints. |
| Merge job interaction | The retry workflow must update recipe TOML constraints if a retry passes. This duplicates merge job logic or requires the merge job to be invocable standalone. |
| Classification accuracy | Same problem as Option B -- needs to decide which failures are worth retrying. |
| Queue lifecycle | Recipes stay in `failed` state between the main run and the retry run. Time window where constraints are wrong. |

**Pros**: Clean separation. Main workflow is unaffected. Retry budget is independently controllable.

**Cons**: Result reconciliation is the hard part. If a retry passes on arm64, the recipe's `supported_arch` constraint must be updated. This means either re-running the full merge logic or building a separate "constraint update" tool. The time window between initial failure and retry completion leaves recipes with incorrect constraints.

### Option D: Two-pass validation

First pass validates all recipes. Second pass (same workflow) retries failures once. Merge job waits for both passes.

| Dimension | Assessment |
|-----------|------------|
| CI cost | Moderate. Second pass only runs failed recipes, but the whole workflow is held open waiting. Runner idle time between passes. |
| Complexity | Medium. Need to collect first-pass failures, launch targeted second-pass jobs, then merge all results. GitHub Actions `needs` + matrix strategy can express this but it's not trivial. |
| Merge job interaction | Clean. Merge job sees consolidated results from both passes. No changes to merge logic. |
| Classification accuracy | Same implicit approach as Option A: if it passes on retry, it was transient. |
| Queue lifecycle | No impact. Single workflow run produces final results. |

**Pros**: Catches transient failures without classification heuristics. Merge job stays simple. No time window with incorrect constraints.

**Cons**: Workflow complexity (three-stage: validate -> collect failures -> retry). Workflow duration increases. Second pass runner spin-up overhead for potentially zero retries.

## 4. Recommendation

**Use Option A (in-job retry) as the primary mechanism, with a lightweight version of Option B for `api_error` only.**

### Rationale

1. **Option A handles the common case.** Most transient failures are brief: a network blip, a momentary 429, a slow DNS resolution. Retrying 2 times with 30s/60s backoff within the same job catches these with near-zero complexity. The orchestrator already runs `tsuku install` as a subprocess -- wrapping it in a retry loop is trivial.

2. **`api_error` is the only category that's unambiguously transient.** Rather than building a general classifier, treat `api_error` failures specially: when the merge job records a failure with category `api_error`, also set a `retryable: true` field. The next batch run can pick these up from the failure log and re-validate on the specific failed platforms. This is a narrow, well-defined re-queue path that doesn't require error message parsing.

3. **`validation_failed` that persists through in-job retries is almost certainly structural.** If `tsuku install` fails 3 times in a row on the same platform, the binary probably doesn't work there. Recording this as a platform constraint is correct. The rare false positive (extended outage during the batch run) self-corrects: the next batch run for new recipes validates on all platforms, and if the same tool appears again (e.g., version update), it gets a fresh chance.

4. **Avoid the complexity trap.** Options B, C, and D all introduce significant complexity (classification heuristics, separate workflows, multi-stage pipelines) to handle an edge case. The design docs already note that partial platform coverage is acceptable -- a recipe constrained to fewer platforms is better than no recipe. The cost of a rare false constraint is low.

### Implementation sketch

In `cmd/batch-generate` orchestrator, the per-package validation loop:

```
for each package:
    for attempt in [1, 2, 3]:
        result = run("tsuku install --sandbox ...")
        if result.success:
            break
        if attempt < 3:
            sleep(30s * attempt)
    record result (pass or fail after 3 attempts)
```

In the failure record schema, add optional field:

```json
"retryable": {
    "type": "boolean",
    "description": "True if failure is likely transient (api_error category). Next batch run should re-validate."
}
```

In the merge job, when processing `api_error` failures: set `retryable: true`. The batch orchestrator's preflight step checks failure records and includes retryable failures in the validation set for their specific failed platforms.

### Cost estimate

- In-job retry: adds ~2 min worst case per structural failure per platform. With 100 recipes and ~10% failure rate, that's 10 recipes x 2 min = 20 min across all platforms. Acceptable.
- `api_error` re-queue: near-zero cost (only fires when ecosystem APIs were down).
- No new workflows, no new schema beyond one optional boolean field.
