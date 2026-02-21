# Scrutiny Review: Completeness - Issue #1642

## Issue: feat(llm): add download permission prompts and progress UX

## Files Changed
- internal/llm/addon/prompter.go (new)
- internal/llm/addon/prompter_test.go (new)
- internal/progress/spinner.go (new)
- internal/progress/spinner_test.go (new)
- internal/llm/addon/manager.go (modified)
- internal/llm/addon/manager_test.go (modified)
- internal/llm/factory.go (modified)
- internal/llm/local.go (modified)

## AC-by-AC Evaluation

### AC: "addon download prompt" -- claimed: implemented
**Verdict: PASS**

Evidence confirmed. `AddonManager.EnsureAddon()` (manager.go:122-131) checks for a non-nil prompter and calls `ConfirmDownload` with description "tsuku-llm inference addon" and `estimatedAddonSize` (50 MB). `InteractivePrompter.ConfirmDownload` (prompter.go:39-75) displays size, asks "Continue? [Y/n]", parses response. Tests cover approve, decline, EOF, and zero-size cases.

### AC: "model download prompt" -- claimed: implemented
**Verdict: PASS**

Evidence confirmed. `LocalProvider.ensureModelReady()` (local.go:205-238) calls `GetStatus` RPC to check model readiness. If model not loaded and prompter is set, prompts with model name and size from server status. `modelPrompted` field prevents re-prompting within a session. The model name is displayed when available from the status response.

### AC: "decline with cloud provider message" -- claimed: implemented
**Verdict: PASS**

Evidence confirmed. Two locations:
- local.go:98: `"local LLM addon download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead"`
- local.go:119: `"model download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead"`

Both check `errors.Is(err, addon.ErrDownloadDeclined)`.

### AC: "download progress bars" -- claimed: deviated
**Verdict: PASS (deviation acknowledged)**

The reason states addon downloads already use `progress.Writer` and model downloads are inside the Rust addon. This is accurate: `progress.Writer` exists in `progress/progress.go` and is used by the download action pipeline. Model downloads happen server-side in the Rust addon, which would require streaming proto changes to relay progress back to Go. Reasonable deviation.

### AC: "inference spinner" -- claimed: implemented
**Verdict: PASS**

Evidence confirmed. `LocalProvider.Complete()` (local.go:130-140) creates a `Spinner`, starts it with "Generating...", stops it after `sendRequest` returns. `StopWithMessage("Generation failed.")` on error. The spinner implementation in `progress/spinner.go` animates with |/-\ frames at 100ms intervals, clears the line on stop.

### AC: "non-TTY suppression" -- claimed: implemented
**Verdict: PASS**

Evidence confirmed at two levels:
1. `InteractivePrompter.ConfirmDownload` (prompter.go:40-43) checks `progress.ShouldShowProgress()` and returns `ErrDownloadDeclined` in non-TTY.
2. `Spinner.Start` (spinner.go:51-53) prints message once without animation in non-TTY.
3. `ShouldShowProgress()` (progress.go:167-169) checks `IsTerminalFunc(int(os.Stdout.Fd()))`.
4. Tests override `IsTerminalFunc` to verify both paths.

### AC: "--yes flag skips prompts" -- claimed: implemented
**Verdict: BLOCKING -- wiring incomplete**

The `AutoApprovePrompter` exists (prompter.go:77-84) and always returns `true`. The `WithPrompter` factory option exists (factory.go:102-110) and correctly wires the prompter to `LocalProvider` via `SetPrompter` (factory.go:184-186).

However, **no caller passes `WithPrompter` to `NewFactory`**. The builders call `llm.NewFactory(ctx)` with zero options:
- `internal/builders/github_release.go:219`: `factory, err = llm.NewFactory(ctx)`
- `internal/builders/homebrew.go:529`: `factory, err = llm.NewFactory(ctx)`
- `internal/discover/llm_discovery.go:154`: `factory, err = llm.NewFactory(ctx)`

The `cmd/tsuku/create.go` command has `createAutoApprove` (`--yes` flag) but never creates or passes a prompter to the LLM factory. The plumbing exists (types, option, setter) but the CLI integration that actually makes `--yes` flow through to `WithPrompter(&addon.AutoApprovePrompter{})` is missing.

As a result, `m.prompter` is always nil in production, so `EnsureAddon` skips the prompt entirely (downloads without asking). The `--yes` flag doesn't skip prompts -- prompts never happen at all. The interactive prompter is also never wired, so the addon download prompt AC is also technically incomplete in production (it works only if someone manually calls `SetPrompter`).

This means all three prompter-dependent ACs (addon download prompt, model download prompt, --yes flag) are implemented as library code but not wired to the CLI. They pass unit tests but would not function end-to-end.

## Summary

| AC | Claimed | Verified | Severity |
|----|---------|----------|----------|
| addon download prompt | implemented | library only, not CLI-wired | blocking |
| model download prompt | implemented | library only, not CLI-wired | blocking |
| decline with cloud provider message | implemented | confirmed | -- |
| download progress bars | deviated | valid deviation | -- |
| inference spinner | implemented | confirmed | -- |
| non-TTY suppression | implemented | confirmed | -- |
| --yes flag skips prompts | implemented | plumbing exists, no CLI wiring | blocking |

## Phantom ACs
None. All mapping entries correspond to requirements from the design doc's Phase 6 description.

## Missing ACs
None detected beyond what the mapping covers.
