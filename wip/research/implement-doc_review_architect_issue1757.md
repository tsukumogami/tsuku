# Architect Review: Issue #1757 (ci(llm): add model caching for integration tests)

## Summary

The change adds model caching to LLM integration tests via three mechanisms: (1) a CI workflow cache step for the model file, (2) a `sharedModelDir` helper that reuses a single model directory across tests in the same process, and (3) a `setupTsukuHome` helper that symlinks each test's `models/` to the shared directory. The pattern is clean and well-integrated with the existing test infrastructure.

## Findings

### 1. BLOCKING: `TSUKU_LLM_MODEL_CACHE` receives unexpanded tilde in CI

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/.github/workflows/test.yml`, line 294

```yaml
      - name: Run LLM integration tests
        env:
          TSUKU_LLM_MODEL_CACHE: ~/.cache/tsuku-llm-models
        run: go test -tags=integration -v ./internal/llm/...
```

The `env:` block sets environment variables as literal strings -- no shell expansion occurs. The Go test receives the literal string `~/.cache/tsuku-llm-models` from `os.Getenv("TSUKU_LLM_MODEL_CACHE")`. When `sharedModelDir` calls `os.MkdirAll(dir, 0755)`, Go's `os` package does not expand `~`. It will create `./~/.cache/tsuku-llm-models` relative to the working directory.

Meanwhile, `actions/cache` on line 287 _does_ expand `~` in its `path` parameter (documented behavior), so the cache is stored at `/home/runner/.cache/tsuku-llm-models`.

Result: the cache stores files at one path, the tests look at a different path. The model is re-downloaded every CI run, defeating the cache entirely.

**Fix:** Use `${{ github.workspace }}/../.cache/tsuku-llm-models` or the runner's home explicitly:

```yaml
        env:
          TSUKU_LLM_MODEL_CACHE: /home/runner/.cache/tsuku-llm-models
```

Or align both to use the same resolvable path. The simplest fix is to change the cache `path` and the env var to both use an absolute path or `${{ runner.temp }}/tsuku-llm-models`.

### 2. ADVISORY: Inconsistent `os.Setenv` vs `t.Setenv` across integration tests

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/lifecycle_integration_test.go`, lines 335, 448, 483, 536

Several tests in `lifecycle_integration_test.go` use `os.Setenv`/`defer os.Unsetenv`:

```go
os.Setenv("TSUKU_HOME", tsukuHome)
defer os.Unsetenv("TSUKU_HOME")
```

While `stability_test.go` (line 27, 101) correctly uses the safer `t.Setenv`:

```go
t.Setenv("TSUKU_HOME", tsukuHome)
```

`t.Setenv` automatically restores the previous value on cleanup and is the idiomatic approach since Go 1.17. The `os.Setenv` pattern can leak state between tests if a test panics before the defer runs, or if tests run in parallel (integration tests currently don't, but it's a latent hazard). Since `stability_test.go` already uses the correct pattern, the older file should be updated for consistency, but this doesn't compound -- the tests run sequentially in CI and there are no current callers copying the old pattern.

### 3. ADVISORY: Cache key couples to model selection file, not model identity

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/.github/workflows/test.yml`, line 288

```yaml
key: llm-model-${{ hashFiles('tsuku-llm/src/model.rs') }}
```

The cache key hashes `model.rs`, which contains all three model entries (0.5B, 1.5B, 3B). Any change to _any_ model entry invalidates the cache, even if CI only uses one model (likely the 0.5B on CI runners with limited RAM). This is overly broad but safe -- it causes unnecessary cache misses rather than stale hits. The `restore-keys: llm-model-` fallback mitigates this by allowing partial matches.

This is a pragmatic tradeoff. A more precise key would hash only the specific model URL/checksum, but that would require extracting model metadata into a separate file. Not worth the complexity for CI caching.

## Structural Assessment

The symlink approach (`setupTsukuHome`) is well-designed:

- Each test gets an isolated `TSUKU_HOME` (sockets, locks, state are per-test)
- The `models/` subdirectory is symlinked to a shared location, so the ~500MB model download happens once per test process
- The `sharedModelDir` function uses `sync.Once` for process-level sharing with a `TSUKU_LLM_MODEL_CACHE` override for CI-level persistence
- This aligns with how `tsuku-llm` resolves models: `$TSUKU_HOME/models/{name}.gguf` (see `tsuku-llm/src/main.rs:704-712`, `tsuku-llm/src/models.rs:85-86`)

The CI workflow change follows the existing pattern: the Cargo registry cache (lines 266-275) uses `actions/cache@v5` with a hash-based key and restore-keys fallback. The model cache step mirrors this structure.

No new patterns are introduced. The `setupTsukuHome` helper consolidates what was previously ad-hoc `t.TempDir()` + `os.Setenv` across multiple tests. Tests that don't need model inference (`TestIntegration_LockFilePreventsduplicates`, `TestIntegration_StaleSocketCleanup`) correctly continue using bare `t.TempDir()`.
