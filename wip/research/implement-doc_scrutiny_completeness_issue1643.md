# Scrutiny Review: Completeness -- Issue #1643

**Issue**: feat(llm): implement tsuku llm download command
**Focus**: completeness
**Files changed**: cmd/tsuku/llm.go, cmd/tsuku/llm_test.go, cmd/tsuku/main.go

---

## AC-by-AC Evaluation

### AC: "downloads addon binary"
**Mapping claim**: implemented
**Verdict**: confirmed
**Evidence**: `cmd/tsuku/llm.go:75-90` -- `addonManager.EnsureAddon(ctx)` downloads the addon binary via the recipe system. This reuses infrastructure from #1629.

### AC: "hardware detection selects model"
**Mapping claim**: implemented
**Verdict**: partially confirmed
**Evidence**: `cmd/tsuku/llm.go:94-131` -- The command starts the addon server (`lifecycle.EnsureRunning`) and queries `GetStatus` which reports `backend`, `available_vram_bytes`, and `model_name`. The hardware detection and model selection happen inside the addon server. The Go command displays the results. However, the command only reports the selection -- it does not actually download the selected model (see "model downloads with progress" below).

### AC: "model downloads with progress"
**Mapping claim**: implemented
**Verdict**: **NOT CONFIRMED -- BLOCKING**
**Evidence**: `cmd/tsuku/llm.go:169-170` contains the comment: "The addon server handles model downloading on first inference. The download command ensures the addon is running and reports status." The command prompts the user for download confirmation (lines 142-167) then prints "Download complete." (line 172) without ever triggering a model download. The gRPC API (`proto/llm.proto`) has only `Complete`, `Shutdown`, and `GetStatus` RPCs -- there is no `DownloadModel` or equivalent RPC. The model is only downloaded when `Complete` is first called during actual inference. This means `tsuku llm download` does NOT pre-download the model, which is the command's core purpose ("pre-download addon and models for CI/offline use").

### AC: "SHA256 verification"
**Mapping claim**: implemented
**Verdict**: confirmed (transitive)
**Evidence**: SHA256 verification happens in `AddonManager.EnsureAddon` for the addon binary (via the recipe system). Model verification happens in the Rust addon at model load time. The download command itself doesn't do SHA256 work, but the claim is reasonable since verification is built into the underlying systems.

### AC: "--model flag overrides"
**Mapping claim**: implemented
**Verdict**: **NOT CONFIRMED -- BLOCKING**
**Evidence**: `cmd/tsuku/llm.go:126-132` -- The `--model` flag sets `effectiveModel` which is used only for display strings (stderr messages and `printDownloadSummary`). It is never sent to the addon server. There is no gRPC mechanism to tell the server to use a different model. The flag is cosmetic; it changes what the user sees but not what model would actually be used. Even if the model download were functional, it would download whatever the hardware detection selected, ignoring `--model`.

### AC: "--force re-downloads"
**Mapping claim**: implemented
**Verdict**: partially confirmed
**Evidence**: `cmd/tsuku/llm.go:135` -- `llmDownloadForce` bypasses the "already present" early exit so the prompt is shown again. However, since the model download doesn't actually occur (see above), `--force` only re-prompts the user and re-displays the summary. For the addon binary, there is no force re-download mechanism -- `EnsureAddon` checks if the binary exists and returns early if found.

### AC: "exit 0 when cached"
**Mapping claim**: implemented
**Verdict**: confirmed
**Evidence**: `cmd/tsuku/llm.go:135-139` -- When `status.Ready && status.ModelName != "" && !llmDownloadForce`, the function returns nil (exit 0) with "Addon and model already present." message.

### AC: "exit 1 on failure"
**Mapping claim**: implemented
**Verdict**: confirmed
**Evidence**: Multiple `exitWithCode(ExitGeneral)` calls at lines 83, 88, 101, 113, 155, 159, 164. `ExitGeneral` is defined as 1.

### AC: "exit 2 invalid model"
**Mapping claim**: deviated, reason: "gRPC API lacks model validation endpoint"
**Verdict**: deviation acknowledged
**Evidence**: No model validation exists in the gRPC API. The `--model` flag doesn't send the model name to the server at all, so even if a validation endpoint existed, it wouldn't be called. The deviation reason is accurate but understates the problem -- it's not just that validation is missing, but that the `--model` flag has no functional effect.

### AC: "works in CI"
**Mapping claim**: implemented
**Verdict**: partially confirmed
**Evidence**: `cmd/tsuku/llm.go:67-68` -- `--yes` flag uses `AutoApprovePrompter` which always returns true. This handles the non-interactive requirement. However, since the command doesn't actually download the model, "works in CI" is only true for the addon binary download. The stated CI use case (pre-download for offline use) is not achieved.

---

## Missing/Phantom ACs

No phantom ACs detected. All mapping entries correspond to behaviors described in the design doc and test plan scenarios.

No missing ACs from the issue description detected beyond what's in the mapping.

---

## Summary

Two blocking findings:

1. **Model download is not implemented.** The command's core purpose is "pre-download addon and models for CI/offline use" but it only downloads the addon binary. The model download is deferred to first inference. The command prints "Download complete." after prompting, without having downloaded anything. The gRPC API lacks a `DownloadModel` RPC, so there's no mechanism to trigger model download from the Go side.

2. **`--model` flag is display-only.** The flag changes the text shown to the user but is never sent to the addon server. Even if model download were implemented, the override wouldn't take effect.
