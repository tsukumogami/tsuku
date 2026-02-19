# Pragmatic Review: Issue #1758 - LLM Quality Gate CI Job

## Finding 1: Near-complete duplication between llm-integration and llm-quality jobs

**Severity: Blocking**

`.github/workflows/test.yml:302-359` -- The `llm-quality` job duplicates 7 of 8 steps verbatim from `llm-integration` (lines 245-299). The only differences are:

1. The final `go test` command targets `./internal/builders/...` instead of `./internal/llm/...`
2. The quality job adds `TSUKU_LLM_BINARY` env var and `-timeout 30m`

Both jobs: checkout with submodules, setup Go, setup Rust 1.80, cache Cargo registry (identical key), install protobuf, build tsuku-llm, cache LLM model (identical key). This is ~50 lines of duplicated YAML that will drift.

**Fix:** Merge the two test commands into the `llm-integration` job. When `llm_quality` is true, run the builders tests as an additional step. Alternatively, if they must stay separate jobs, extract the build steps into a reusable workflow or composite action. But given both need the same binary on the same runner, a single job with two test steps is simplest.

## Finding 2: llm_quality filter references nonexistent path

**Severity: Advisory**

`.github/workflows/test.yml:406` -- `internal/builders/prompts/**` matches zero files. This is speculative generality: adding a trigger path for a directory that doesn't exist yet. When the directory is created, the developer who creates it should add the filter at that time.

However, this is minor since an unused glob pattern in paths-filter is inert (it won't cause false positives or negatives for existing files). Non-blocking because it doesn't create dead code that compiles.

## Finding 3: llm_quality filter misses the test file itself

**Severity: Advisory**

`.github/workflows/test.yml:405-409` -- The filter triggers on `internal/builders/prompts/**`, `tsuku-llm/src/main.rs`, `internal/builders/llm-test-matrix.json`, and `testdata/llm-quality-baselines/**`. But changes to `internal/builders/llm_integration_test.go` or `internal/builders/baseline_test.go` (which contain the quality test logic: `TestLLMGroundTruth`, `TestWriteBaseline_MinimumPassRate`, etc.) won't trigger the quality gate.

If someone changes the test assertions or ground-truth evaluation logic, the quality gate won't run. The `code` filter covers `**/*.go` but `llm_quality` specifically does not. This means the quality job only fires on data/config changes, not on logic changes to the quality tests themselves.

This may be intentional (the `llm` filter catches `internal/llm/**` changes, and Go code changes trigger unit tests). But `internal/builders/*.go` changes are covered by neither `llm` nor `llm_quality` -- they fall through to the generic `code` filter which runs unit/lint/functional tests but not the LLM quality gate.

**Fix:** Either add `internal/builders/llm_integration_test.go` and `internal/builders/baseline_test.go` to the `llm_quality` filter, or document that quality gate logic changes are validated by the standard unit test pipeline.

## Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | 50 lines of duplicated YAML between llm-integration and llm-quality | Blocking |
| 2 | Filter references nonexistent `internal/builders/prompts/**` | Advisory |
| 3 | Filter misses quality test source files | Advisory |
