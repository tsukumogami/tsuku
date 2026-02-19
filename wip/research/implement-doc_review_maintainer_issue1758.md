# Maintainer Review: Issue #1758 - LLM Quality Gate CI Job

## Summary

The new `llm-quality` job and `llm_quality` filter in `.github/workflows/test.yml` follow existing CI conventions well. The job structure mirrors `llm-integration` closely, the env-var comment explaining the `$HOME` expansion pattern is carried over correctly, and the `TSUKU_LLM_BINARY` usage via `${{ github.workspace }}` (a GitHub Actions expression, not shell) avoids the tilde-expansion trap documented for this codebase.

The test separation is clear in intent: `llm-integration` tests the LLM addon lifecycle (`internal/llm/...`), while `llm-quality` tests recipe generation quality (`internal/builders/...`). The `timeout-minutes: 45` and the `-timeout 30m` flag on the Go test command are consistent.

## Findings

### 1. Phantom path in `llm_quality` filter

**File:** `.github/workflows/test.yml`, line 406
**Severity:** Blocking

The `llm_quality` filter includes `internal/builders/prompts/**`, but this directory does not exist. The actual prompt text lives inline in Go source files:
- `internal/builders/github_release.go:992` (`buildSystemPrompt()`)
- `internal/builders/homebrew.go:1650` (`buildSystemPrompt()`)

This means changing a prompt -- the primary thing this quality gate is designed to catch -- will not trigger the quality gate. The next developer who modifies a prompt will reasonably assume the quality gate is running, but it won't be. The filter silently matches nothing.

Either:
- (a) Add `internal/builders/github_release.go` and `internal/builders/homebrew.go` to the filter (targeting the actual prompt-containing files), or
- (b) Extract prompts to `internal/builders/prompts/` directory and embed them (matching the filter's expectation), or
- (c) Broaden to `internal/builders/*.go` (catches prompt changes but also triggers on non-prompt builder changes)

Option (a) is the simplest fix. Option (c) may over-trigger but won't miss anything.

### 2. `llm_quality` filter does not catch test matrix code changes

**File:** `.github/workflows/test.yml`, lines 405-409
**Severity:** Advisory

The filter watches `internal/builders/llm-test-matrix.json` and `testdata/llm-quality-baselines/**`, which is good. But it doesn't watch changes to the test runner itself (`internal/builders/llm_integration_test.go`) or the baseline comparison logic (`internal/builders/baseline_test.go`). If someone changes validation logic in `validateGitHubRecipe()` or `validateHomebrewSourceRecipe()`, the quality gate won't run.

This is advisory because test runner changes tend to accompany prompt or matrix changes (and the unit test suite covers the baseline helpers). But the next developer might not realize that editing `llm_integration_test.go` alone doesn't trigger a quality run.

### 3. `actions/cache` not pinned to SHA

**File:** `.github/workflows/test.yml`, lines 267, 285, 325, 343
**Severity:** Advisory

All four `actions/cache` usages (two in `llm-integration`, two in `llm-quality`) use `actions/cache@v5` (a mutable tag). The rest of the workflow pins actions to commit SHAs with version comments (e.g., `actions/checkout@de0fac2e...  # v6.0.2`). This pre-dates this PR (lines 267 and 285 are in the existing `llm-integration` job), but the new `llm-quality` job copies the pattern, widening the inconsistency.

This is advisory because the `actions/cache` entries were already present before this change. Worth noting for a follow-up pin, but not a blocking concern for this PR.

### 4. Near-duplicate job structure between `llm-integration` and `llm-quality`

**File:** `.github/workflows/test.yml`, lines 245-299 vs 302-359
**Severity:** Advisory

The two jobs share identical steps for: checkout with submodules, Go setup, Rust setup, Cargo cache, protobuf install, tsuku-llm build, and model cache. The only differences are:
- The `if` condition (different filter)
- `timeout-minutes: 45` (only on quality)
- The `TSUKU_LLM_BINARY` env var (only on quality)
- The final `go test` command (different package path and timeout flag)

This is a common CI pattern and YAML doesn't support DRY well without composite actions or reusable workflows. The duplication is acceptable for now, but the next developer who updates the Rust toolchain version, cargo cache key, or protobuf install will need to remember to update both jobs. A comment like `# Keep in sync with llm-integration` (and vice versa) would help.

### 5. `llm-integration` does not set `TSUKU_LLM_BINARY`

**File:** `.github/workflows/test.yml`, lines 292-299 vs 350-359
**Severity:** Advisory

`llm-integration` builds tsuku-llm but doesn't set `TSUKU_LLM_BINARY`. The tests in `internal/llm/...` must find the binary through a different mechanism (likely `lifecycle_integration_test.go:172` which falls back to looking in `tsuku-llm/target/release/`). Meanwhile `llm-quality` explicitly sets `TSUKU_LLM_BINARY`.

This difference is intentional (the LLM lifecycle tests have their own binary discovery), but the divergence could confuse the next developer who sees the quality job set the env var and wonders why the integration job doesn't. A one-line comment in the integration job like `# LLM lifecycle tests locate the binary via setupTsukuHome()` would prevent the question.

## Score

| Metric | Count |
|--------|-------|
| Blocking | 1 |
| Advisory | 4 |
