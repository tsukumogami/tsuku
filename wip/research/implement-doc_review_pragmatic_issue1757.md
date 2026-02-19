# Pragmatic Review: Issue #1757 (ci(llm): add model caching for integration tests)

## Findings

### 1. BLOCKING: Tilde not expanded in env var -- cache path mismatch

`.github/workflows/test.yml:294` -- `TSUKU_LLM_MODEL_CACHE: ~/.cache/tsuku-llm-models` passes a literal `~` to Go via `os.Getenv`. Go does not expand tilde. The test code at `lifecycle_integration_test.go:79` calls `os.MkdirAll(dir, 0755)` with the literal `~/.cache/tsuku-llm-models`, creating a directory named `~` in the working directory instead of under `$HOME`. Meanwhile `actions/cache` at line 287 expands `~` correctly to `/home/runner/.cache/tsuku-llm-models`. The cached models and the test's model directory point to different locations, so the cache has no effect.

Fix: Use `${{ env.HOME }}/.cache/tsuku-llm-models` or `$HOME/.cache/tsuku-llm-models` in the `run:` step instead of `~`. Example:

```yaml
      - name: Run LLM integration tests
        run: |
          export TSUKU_LLM_MODEL_CACHE="$HOME/.cache/tsuku-llm-models"
          go test -tags=integration -v ./internal/llm/...
```

Or keep it in `env:` but use the GitHub Actions context:

```yaml
        env:
          TSUKU_LLM_MODEL_CACHE: /home/runner/.cache/tsuku-llm-models
```

### 2. ADVISORY: `sharedModelDir` is single-caller but justified

`lifecycle_integration_test.go:75` -- `sharedModelDir` is called only from `setupTsukuHome`. Normally a single-caller helper is inline-worthy, but here it encapsulates `sync.Once` state and env var branching. The separation is clean and readable. No action needed.

## Summary

| Level | Count |
|-------|-------|
| Blocking | 1 |
| Advisory | 1 |

The core design is sound: shared model directory via symlink, CI cache keyed on model.rs hash, `setupTsukuHome` helper used by 8 callers across two test files. The only real issue is the tilde expansion bug that silently defeats the caching.
