# Maintainer Review: Issue #1757 (Model Caching for Integration Tests)

## Summary

This change introduces model caching at two levels: (1) `actions/cache` in CI to persist the downloaded model across runs, and (2) a `sharedModelDir(t)` + `setupTsukuHome(t)` pattern in tests to share a single model download across test functions within a process.

The code is well-structured overall. The comments explain the "why" clearly, and the helper naming accurately describes behavior.

## Findings

### 1. `idle_test.go` does not use `setupTsukuHome` -- will re-download the model

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/idle_test.go:19`

```go
tsukuHome := t.TempDir()
os.Setenv("TSUKU_HOME", tsukuHome)
```

`TestIntegration_ActivityResetsIdleTimeout` calls `skipIfModelCDNUnavailable` and starts the daemon with model loading, but uses a bare `t.TempDir()` instead of `setupTsukuHome(t)`. This means it does not get the symlinked `models/` directory and will re-download the ~500MB model from scratch. The next developer looking at the test file will see all the other model-dependent tests using `setupTsukuHome` and assume this was an oversight. If it is intentional (testing behavior without the symlink), there's no comment explaining why.

Also uses `os.Setenv`/`os.Unsetenv` instead of `t.Setenv`, which is a separate pre-existing issue but contributes to the divergence from the pattern in `stability_test.go`.

**Severity: Blocking.** The whole point of this issue is to stop re-downloading the model per test function. This test was missed, defeating the optimization for one of the more expensive tests (it sleeps 40+ seconds while the daemon runs with a loaded model).

### 2. `os.Setenv` / `os.Unsetenv` vs `t.Setenv` -- inconsistent and unsafe for parallel tests

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/lifecycle_integration_test.go:335-336`

```go
os.Setenv("TSUKU_HOME", tsukuHome)
defer os.Unsetenv("TSUKU_HOME")
```

Four tests in `lifecycle_integration_test.go` (lines 335, 448, 483, 536) use `os.Setenv`/`defer os.Unsetenv`, while `stability_test.go` uses `t.Setenv`. A next developer copying the pattern from one file would get a different pattern than copying from the other. `t.Setenv` is strictly better: it automatically restores the previous value on cleanup and is documented by `go test` as the right approach.

This is a pre-existing issue in the lifecycle tests, but the new `stability_test.go` correctly uses `t.Setenv`, creating a visible split. The next developer won't know which to follow.

**Severity: Advisory.** The integration tests don't run in parallel (they bind to Unix sockets in the same `TSUKU_HOME`), so this won't cause a race condition bug. But the inconsistency is a copy-paste trap.

### 3. `sharedModelOnce` + `panic` -- error reporting path is opaque

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/lifecycle_integration_test.go:85-91`

```go
sharedModelOnce.Do(func() {
    dir, err := os.MkdirTemp("", "tsuku-llm-models-*")
    if err != nil {
        panic(fmt.Sprintf("failed to create shared model dir: %v", err))
    }
    sharedModelPath = dir
})
```

The `sync.Once` closure panics on error because `t.Fatalf` isn't available inside a `sync.Once` (the `t` from the first caller would be stale for later callers). This is a reasonable trade-off for a test helper, and the comment on the function explains the caching intent. A next developer seeing the `panic` might wonder why not `t.Fatal`, but the `sync.Once` context makes it clear enough. No action needed here.

**Severity: Out of scope** (pragmatic trade-off, not a misread risk).

### 4. CI cache key uses `model.rs` hash but test `modelSourceURL` is hardcoded separately

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/.github/workflows/test.yml:288`

```yaml
key: llm-model-${{ hashFiles('tsuku-llm/src/model.rs') }}
```

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/lifecycle_integration_test.go:36`

```go
const modelSourceURL = "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf"
```

The `modelSourceURL` constant in the test is used only for the `skipIfModelCDNUnavailable` availability check -- it's not the download path (the daemon resolves that from its own manifest in `model.rs`). However, if someone changes the model in `model.rs` without updating this constant, the skip check would test reachability of the *old* model URL while the daemon downloads the *new* one. In practice, both URLs are on the same HuggingFace domain, so a CDN outage would affect both equally. But the next developer might think this URL controls which model is downloaded.

The constant has a clear comment ("model source URL ... for checking availability"), which mitigates the risk.

**Severity: Advisory.** The divergence is unlikely to cause a wrong skip decision in practice, but a comment noting "this must match the 0.5B model in tsuku-llm/src/model.rs" would help the next person updating the manifest.

### 5. CI `TSUKU_LLM_MODEL_CACHE` uses `~` instead of `$HOME`

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/.github/workflows/test.yml:294`

```yaml
env:
  TSUKU_LLM_MODEL_CACHE: ~/.cache/tsuku-llm-models
```

Two things use `~/.cache/tsuku-llm-models` but resolve `~` differently:

- **`actions/cache` `path` field** (line 287): GitHub Actions' cache action explicitly expands `~` to `$HOME`. So the cached directory is `$HOME/.cache/tsuku-llm-models`.
- **`env` block** (line 294): Environment variables set via the workflow YAML `env` block are passed as literal strings to the process. Bash tilde expansion only applies to unquoted word-initial `~` in command arguments and shell-level variable assignments, not to inherited environment variables.

The Go test helper calls `os.Getenv("TSUKU_LLM_MODEL_CACHE")` and gets the literal string `~/.cache/tsuku-llm-models`. It then calls `os.MkdirAll(dir, 0755)`, which creates a directory literally named `~` under the current working directory -- not `$HOME`. The `actions/cache` restores files to `$HOME/.cache/tsuku-llm-models`, but the tests look in `./~/.cache/tsuku-llm-models`. The cache never hits.

**Severity: Blocking.** The CI model cache will never work as written. The `actions/cache` path and the `TSUKU_LLM_MODEL_CACHE` env var resolve to different locations. The env var should use `$HOME/.cache/tsuku-llm-models` or the cache path should match. This is exactly the kind of subtle difference that works in local testing (where `~` might expand in certain contexts) but fails silently in CI (cache misses just mean slower builds, not test failures).

## Verdict

The overall design -- shared model dir via symlink, process-level `sync.Once`, CI cache layer -- is clean and well-documented. The `setupTsukuHome` helper name accurately describes what it does, and the comment block explains the symlink strategy clearly.

Two issues need fixing: the `~` expansion bug in the CI workflow (the cache will silently never hit), and `idle_test.go` missing the `setupTsukuHome` conversion (re-downloads the model, defeating the purpose).
