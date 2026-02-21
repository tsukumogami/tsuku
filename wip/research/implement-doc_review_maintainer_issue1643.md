# Maintainer Review: Issue #1643

**Issue**: feat(llm): implement tsuku llm download command
**Focus**: maintainability (clarity, readability, duplication)
**Files reviewed**: `cmd/tsuku/llm.go`, `cmd/tsuku/llm_test.go`, `cmd/tsuku/main.go`, `internal/llm/local.go` (TriggerModelDownload)

---

## Finding 1: `RunE` returns nil on every path -- mixed convention

**File**: `cmd/tsuku/llm.go:43,58-188`
**Severity**: Advisory

The `llmDownloadCmd` uses `RunE: runLLMDownload`, but `runLLMDownload` never returns a non-nil error. Every error path calls `exitWithCode(ExitGeneral)` then `return nil`. This makes `RunE` misleading: the next developer will expect the function to return errors to cobra for display, but it never does.

The rest of the codebase uses `Run:` (not `RunE:`) with `exitWithCode`. See `create.go:119`, `install.go:55`, `search.go:18`, `config.go:43`, etc. Using `RunE` here creates an inconsistency. The next developer adding a new subcommand under `llm` will look at this file as the pattern and may either (a) also use `RunE` with the same `return nil` antipattern, or (b) actually return errors assuming cobra will handle them, leading to different error formatting.

**Suggestion**: Switch to `Run:` to match the codebase convention, or if `RunE` is preferred for new code, actually return errors instead of calling `exitWithCode`.

---

## Finding 2: `--force` flag description promises re-download, but behavior is uncertain

**File**: `cmd/tsuku/llm.go:47,130`
**Severity**: Advisory

The `--force` flag is documented as "Re-download even if files already exist" and the Long description says "Re-download even if files exist." When `--force` is set, the code skips the early return at line 130 and proceeds to prompt + `TriggerModelDownload`.

For the addon binary, `EnsureAddon` checks for an existing binary and returns early if found. There is no force parameter passed to `EnsureAddon`, so `--force` does not re-download the addon binary.

For the model, `TriggerModelDownload` sends a lightweight Complete request. Whether the addon server re-downloads an already-loaded model depends on the addon's implementation. If the addon sees the model is already loaded, it will likely just respond immediately -- no re-download occurs.

The next developer debugging a "model seems corrupted" scenario will reach for `--force` expecting it to re-download both artifacts. It won't. The flag's actual behavior is "re-prompt and re-verify" rather than "re-download."

**Suggestion**: Either update the flag description to "Verify and re-trigger model download even if files appear present" or wire force semantics through to `EnsureAddon` and the addon server. A comment explaining the limitation would also help.

---

## Finding 3: `TriggerModelDownload` name suggests a download operation but actually sends an inference request

**File**: `internal/llm/local.go:240-266`
**Severity**: Advisory

`TriggerModelDownload` sends a minimal Complete request ("Respond with OK." / "OK") to force the addon to download the model as a side effect. The function name suggests it's a download operation, but it's actually an inference call that happens to trigger a download.

The godoc comment at line 240-243 explains this clearly, which partially mitigates the risk. But the name alone will mislead. A developer searching for "how does model download work" will find this function and think there's a dedicated download path. A developer debugging slow downloads will look at this function and wonder why it's sending a Complete request.

The comment on line 251 ("This is the only way to trigger model download since the gRPC API has no dedicated DownloadModel RPC") is the key piece of context. Without it, the code looks like a bug.

**Suggestion**: The name is acceptable given the clear comment, but consider naming it `EnsureModelDownloaded` or `WarmModel` to better convey that it's using inference as a download trigger. Or add a `// WARNING:` prefix to make the workaround nature more visible.

---

## Finding 4: Duplicated model-prompt logic between `runLLMDownload` and `ensureModelReady`

**File**: `cmd/tsuku/llm.go:137-159` vs `internal/llm/local.go:205-238`
**Severity**: Advisory

Both `runLLMDownload` (in the CLI command) and `ensureModelReady` (in LocalProvider) build a model description string using the same pattern:

`cmd/tsuku/llm.go:138-141`:
```go
modelDesc := "LLM model"
if status.ModelName != "" {
    modelDesc = fmt.Sprintf("LLM model (%s)", status.ModelName)
}
```

`internal/llm/local.go:223-226`:
```go
description := "LLM model"
if status.ModelName != "" {
    description = fmt.Sprintf("LLM model (%s)", status.ModelName)
}
```

Both then call `prompter.ConfirmDownload(ctx, description, modelSize)`. The download command also manually calls `TriggerModelDownload` after prompting, while `ensureModelReady` just prompts and lets the subsequent `Complete` call handle the download.

This divergence means changes to the prompt wording or flow need to be made in two places. It's not dangerous today because the paths are clearly different (CLI command vs provider), but the duplication is a maintenance trap.

**Suggestion**: Extract the model description formatting into a shared helper, or add a comment in each location referencing the other so the next developer knows to update both.

---

## Finding 5: Test names accurately describe behavior but tests are shallow

**File**: `cmd/tsuku/llm_test.go`
**Severity**: Advisory

The tests verify command structure (subcommand registration, flag existence, descriptions) but don't test the actual `runLLMDownload` function's behavior. There are no tests for:
- The prompt flow (approve/decline)
- The error paths (addon not found, server start failure, model download failure)
- The `--force` bypass of the cache check
- The `--yes` auto-approve path

Test names are accurate ("model flag removed", "force flag exists") -- they don't lie. But the test file as a whole gives the impression of coverage that doesn't extend to the command's runtime behavior. A next developer looking at test coverage might assume the prompt/download flow is tested somewhere and skip writing tests for changes in that area.

The `TriggerModelDownload` tests in `internal/llm/local_test.go:452-538` are well-structured and test both success and error paths with proper mock servers. The gap is only in the CLI command layer.

**Suggestion**: Consider adding a test that exercises the `runLLMDownload` function with a mock prompter and mock gRPC server to verify the end-to-end flow. At minimum, add a comment noting that CLI-level integration tests are not present.

---

## Finding 6: `NewAddonManager("", nil, "")` passes empty strings intentionally but reads like incomplete code

**File**: `cmd/tsuku/llm.go:74`
**Severity**: Advisory

```go
addonManager := addon.NewAddonManager("", nil, "")
```

The three empty/nil arguments look like placeholder code that someone forgot to fill in. Looking at `NewAddonManager`, empty `homeDir` is handled (falls back to env/default), `nil` installer is valid (means "no recipe installation"), and empty `backendOverride` is fine. But a next developer seeing `("", nil, "")` in the download command will question whether this is correct -- the download command should be able to download the addon, but `nil` installer means `installViaRecipe` will fail with "no installer configured."

The same pattern appears in `local.go:54` (`NewLocalProvider`) with the same reasoning. But in `local.go:68` (`NewLocalProviderWithInstaller`), a real installer is passed. The download command creates its own `AddonManager` without an installer, meaning addon download depends on the binary already being installed or on `TSUKU_LLM_BINARY` being set. This might be intentional (the recipe system is used elsewhere), but it's not obvious.

**Suggestion**: Add a brief comment on line 74 explaining why nil installer is correct here, e.g., `// nil installer: EnsureAddon finds existing binary; recipe install is handled by 'tsuku install tsuku-llm'`.

---

## Overall Assessment

The code is well-structured and follows a clear step-by-step pattern with good error messages. The `printDownloadSummary` helper is clean. The verification step after `TriggerModelDownload` (re-checking `GetStatus`) is a good defensive practice.

No blocking findings. The main maintenance risks are the `--force` flag not doing what its name promises, and the duplicated prompt-formatting logic. Both are low-risk since the command is a leaf (no downstream code depends on it) and the workaround nature of `TriggerModelDownload` is well-documented in comments.
