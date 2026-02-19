# Architect Review: Issue #1758 - LLM Quality Gate CI Job

## Summary

The change adds an `llm-quality` job and `llm_quality` filter to `.github/workflows/test.yml`. The job runs `TestLLMGroundTruth` against the `internal/builders/...` package using the local LLM provider when prompt-adjacent files change.

## Structural Fit

The new job follows the established CI pipeline design: a filter in the `matrix` job gates a downstream job via `needs.matrix.outputs.<filter>`. This matches the pattern used by `llm`, `rust`, `code`, `recipes`, and `functional` filters. The job structure (checkout, Go setup, Rust setup, Cargo cache, protobuf, build tsuku-llm, model cache, run tests) mirrors `llm-integration` exactly. No parallel pattern introduced.

Cache keys for both Cargo (`${{ runner.os }}-cargo-llm-${{ hashFiles('tsuku-llm/Cargo.lock') }}`) and LLM model (`llm-model-${{ hashFiles('tsuku-llm/src/model.rs') }}`) are identical between `llm-integration` and `llm-quality`. These jobs won't run concurrently for the same PR (different triggers), and actions/cache handles shared keys correctly across jobs in the same workflow run. No issue here.

## Findings

### 1. Ghost filter path: `internal/builders/prompts/**` does not exist -- BLOCKING

**File:** `.github/workflows/test.yml:406`

```yaml
llm_quality:
  - 'internal/builders/prompts/**'
  - 'tsuku-llm/src/main.rs'
  - 'internal/builders/llm-test-matrix.json'
  - 'testdata/llm-quality-baselines/**'
```

The path `internal/builders/prompts/**` matches no files in the repository. No directory `internal/builders/prompts/` exists. Prompts are embedded as string literals in Go source files (`github_release.go:992` via `buildSystemPrompt()`, `homebrew.go:1650` via `buildSystemPrompt()`).

This means the core trigger for the quality gate -- "prompts changed" -- is inert. Modifying `buildSystemPrompt()` in `github_release.go` or `homebrew.go` will not trigger this job. The `code` filter will fire (matching `**/*.go`), running unit tests with `-short` which skips `TestLLMGroundTruth`. The quality gate silently misses the changes it was designed to catch.

This is a structural problem because the filter path creates a false sense of coverage. Future prompt changes will pass CI without quality validation.

**Fix:** Replace the ghost path with the Go files that contain the prompts:

```yaml
llm_quality:
  - 'internal/builders/github_release.go'
  - 'internal/builders/homebrew.go'
  - 'tsuku-llm/src/main.rs'
  - 'internal/builders/llm-test-matrix.json'
  - 'testdata/llm-quality-baselines/**'
```

Or, if prompts are expected to be extracted to files in the future, document that intention and add the current Go files alongside the future path.

### 2. Filter misses `tsuku-llm/src/` changes beyond `main.rs` -- ADVISORY

**File:** `.github/workflows/test.yml:407`

```yaml
- 'tsuku-llm/src/main.rs'
```

The `tsuku-llm` binary has 11 Rust source files across `tsuku-llm/src/` (hardware.rs, model.rs, models.rs, llama/*.rs). The filter only watches `main.rs`. Changes to inference parameters in `llama/sampler.rs`, model loading in `model.rs`, or hardware detection in `hardware.rs` could affect quality output without triggering this gate.

The existing `llm` filter covers `tsuku-llm/**` broadly for the integration test job. The quality gate's narrower scope may be intentional (main.rs is where the gRPC server and prompt routing live), but quality-affecting changes in other modules won't trigger the gate.

Not blocking because the `llm` filter triggers `llm-integration` for any `tsuku-llm/**` change, providing partial coverage. However, `llm-integration` tests `internal/llm/...`, not `internal/builders/...`, so it doesn't run the ground truth quality suite either.

### 3. `actions/cache@v5` uses unpinned tag -- ADVISORY

**File:** `.github/workflows/test.yml:325, 343`

Both cache steps use `actions/cache@v5` without a pinned commit hash, while the rest of the workflow consistently pins actions to commit SHAs (e.g., `actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd`). The existing `llm-integration` job (lines 267, 285) has the same pattern, so this is inherited, not introduced. Flagging because the `lint-workflows` job may eventually enforce pinned hashes, and both jobs would need updating together.

## Non-Issues

- **Job dependency structure:** Correctly depends on `matrix` job, follows the established `needs: matrix` + `if: ${{ needs.matrix.outputs.<filter> == 'true' }}` pattern.
- **Model cache sharing:** Both `llm-integration` and `llm-quality` use the same cache key (`llm-model-${{ hashFiles('tsuku-llm/src/model.rs') }}`). This is correct -- the model is the same regardless of which test suite runs.
- **Cargo cache sharing:** Same key structure, same correctness argument.
- **Test command:** `go test -tags=integration -v -timeout 30m ./internal/builders/...` will run all builders tests (not just LLM ones), since no build tags gate files in that package. This is fine -- the non-LLM tests are fast Go unit tests. `TestLLMGroundTruth` skips via `testing.Short()` guard when run by `unit-tests` (which uses `-short`), and runs here because `-short` is absent. The `-tags=integration` flag is technically unnecessary for this package (no integration-tagged files), but harmless and consistent with the `llm-integration` job.
- **timeout-minutes: 45:** Reasonable given the 30m test timeout plus build time.

## Verdict

One blocking finding. The ghost filter path means the quality gate won't fire when prompts actually change, which defeats the purpose of the job.
