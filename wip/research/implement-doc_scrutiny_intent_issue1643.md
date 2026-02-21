# Scrutiny Review: Intent - Issue #1643

**Issue**: #1643 (feat(llm): implement tsuku llm download command)
**Focus**: intent (design alignment + cross-issue enablement)

## Design Intent Description

The design doc (DESIGN-local-llm-runtime.md) describes `tsuku llm download` in several places:

1. **Scope section** (line 194): "Users in bandwidth-constrained environments can pre-download via `tsuku llm download` or configure cloud providers instead."

2. **Responsibility Split** (line 417): "The `tsuku llm download` command for pre-downloading the addon" is listed under tsuku CLI responsibilities.

3. **Phase 6** (line 712): "tsuku llm download command for manual pre-download"

4. **Issue table** (line 71-72): "CLI command to pre-download addon and models for CI/offline use. Hardware detection selects appropriate model."

5. **Batch pipeline data flow** (lines 606-609): Shows CI use case where the idle timeout naturally handles batch. The `tsuku llm download` command is the intentional pre-download mechanism for CI environments where on-demand download during first inference would be disruptive.

6. **Consequences - Download size** (line 385): "`tsuku llm download` for pre-download in CI" is listed as a mitigation for the first-use download cost.

**Core intent**: The command exists so users can proactively download both the addon AND the model before they ever run `tsuku create`. This is especially important for CI, offline environments, and bandwidth-constrained setups. The command should complete with both artifacts present on disk, ready for offline use.

## Sub-check 1: Design Intent Alignment

### BLOCKING: Model download not actually performed

The most significant gap is that `runLLMDownload` does NOT actually trigger model download. Looking at `cmd/tsuku/llm.go` lines 169-173:

```go
// The addon server handles model downloading on first inference.
// The download command ensures the addon is running and reports status.
fmt.Fprintln(os.Stderr, "")
fmt.Fprintln(os.Stderr, "Download complete.")
```

The comment on line 169 explicitly acknowledges this: "The addon server handles model downloading on first inference." This means the command:

1. Downloads the addon binary (via `addonManager.EnsureAddon`)
2. Starts the addon server
3. Queries status via `GetStatus`
4. Prompts the user about the model
5. Prints "Download complete" -- WITHOUT actually downloading the model

The design doc's intent is clear: pre-download means the artifacts are on disk after the command completes. The phrase "pre-download addon and models for CI/offline use" (the command's own `Short` description) promises that after running `tsuku llm download`, you can go offline and `tsuku create` will work. But if the model isn't downloaded until first inference, a CI pipeline that runs `tsuku llm download` in a setup step and then `tsuku create` in an offline step will fail.

The `--model` flag override is also not communicated to the addon. The code sets `effectiveModel` locally but never passes it to the addon server or triggers a download for that specific model. The addon will select its own model based on hardware detection regardless of `--model`.

### ADVISORY: "Download complete" message is misleading

Even ignoring the model gap, the "Download complete" message at line 172 appears unconditionally after the prompt acceptance, regardless of whether any actual download occurred. If the addon was already present (only model was missing), the user sees "Download complete" without any download happening.

### ADVISORY: --force flag only affects the cache-check path, not actual re-download

The `--force` flag at line 135 bypasses the "already present" early return, but the code that follows doesn't actually invoke a re-download. It prompts the user and then prints "Download complete." For `--force` to match its intent (re-download even if files exist), it would need to communicate a force/re-download instruction to the addon.

### ADVISORY: --model override is display-only

At lines 127-132, `llmDownloadModel` is assigned to `effectiveModel` and displayed to the user, but this value is never passed to the addon server. The gRPC proto has no mechanism to request a specific model download. The `StatusResponse` just reports what the addon selected. So `--model qwen2.5-0.5b-instruct-q4` prints "Selected model: qwen2.5-0.5b-instruct-q4 (override via --model)" but the addon will still select whatever its hardware detection chooses. This is cosmetic, not functional.

## Sub-check 2: Cross-Issue Enablement

Downstream issues are empty (leaf node). Skipping.

## Backward Coherence

Previous summary indicates files changed: `internal/llm/addon/prompter.go`, `internal/progress/spinner.go`, `internal/llm/addon/manager.go`, `internal/llm/factory.go`, `internal/llm/local.go`, `cmd/tsuku/create.go`.

The prompter infrastructure (`Prompter` interface, `InteractivePrompter`, `AutoApprovePrompter`, `NilPrompter`) and its integration into the factory and create command follow a consistent pattern. The download command reuses this infrastructure correctly. No backward coherence issues.

## Deviation Assessment

The mapping reports one deviation:

- **AC "exit 2 invalid model"** deviated because "gRPC API lacks model validation endpoint."

This deviation is moot given the blocking finding: the `--model` flag is never communicated to the addon, so there's no code path where an invalid model name would be detected, validated, or rejected. The deviation reason is technically accurate (the gRPC API indeed lacks this), but the underlying issue is broader -- model override is not functional at all.

## Summary

| Finding | Severity | Description |
|---------|----------|-------------|
| Model not actually downloaded | Blocking | Command prints "Download complete" but defers model download to first inference, defeating the pre-download purpose |
| --model override is display-only | Advisory | Value is shown to user but never sent to addon server |
| --force doesn't trigger re-download | Advisory | Bypasses cache check but no actual re-download occurs |
| "Download complete" message unconditional | Advisory | Message appears even when no download happened |
