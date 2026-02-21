# Scrutiny Review: Intent - Issue #1642

## Issue: feat(llm): add download permission prompts and progress UX

## Scrutiny Focus: intent

---

## Sub-check 1: Design Intent Alignment

### Design doc expectations (Phase 6 / #1642 scope)

The design document describes issue #1642 as:

> _Prompt user before addon/model downloads. Show progress bars during downloads and spinner during inference._

Phase 6 ("First-Use Experience Polish") in the Solution Architecture says:

> - Permission prompt before addon and model downloads
> - Progress bars during downloads
> - Status messages during inference
> - Error messages when hardware isn't sufficient

The Data Flow section describes the decline behavior:

> LocalProvider checks for addon, prompts user, triggers download

And the "Download Considerations" section:

> tsuku prompts for confirmation before the initial download.

The Responsibility Split section says tsuku CLI handles:

> - User prompts ("Download addon? Y/n")

### Requirements mapping (untrusted)

```
--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
[
  {"ac":"addon download prompt","status":"implemented"},
  {"ac":"model download prompt","status":"implemented"},
  {"ac":"decline with cloud provider message","status":"implemented"},
  {"ac":"download progress bars","status":"deviated","reason":"addon downloads already use progress.Writer; model downloads inside Rust addon would need proto streaming changes"},
  {"ac":"inference spinner","status":"implemented"},
  {"ac":"non-TTY suppression","status":"implemented"},
  {"ac":"--yes flag skips prompts","status":"implemented"}
]
--- END UNTRUSTED REQUIREMENTS MAPPING ---
```

### Assessment

#### AC: addon download prompt -- VERIFIED

`internal/llm/addon/manager.go` at lines 121-131: `EnsureAddon` checks if the addon is installed and, when not, calls `m.prompter.ConfirmDownload(ctx, "tsuku-llm inference addon", estimatedAddonSize)` before proceeding to install. This matches the design doc's intent.

`internal/llm/addon/prompter.go` at lines 38-75: `InteractivePrompter.ConfirmDownload` displays the artifact description and size, checks for TTY, defaults to yes on empty input. This matches the design doc's user prompt flow.

Tests in `internal/llm/addon/manager_test.go` (TestEnsureAddon_WithPrompter_Approved, TestEnsureAddon_WithPrompter_Declined, TestEnsureAddon_WithPrompter_AlreadyInstalled) verify the prompt flows.

#### AC: model download prompt -- VERIFIED

`internal/llm/local.go` at lines 205-238: `ensureModelReady` calls `GetStatus` on the addon server, and if the model isn't loaded, prompts the user via `p.prompter.ConfirmDownload(ctx, description, modelSize)`. The `modelPrompted` field prevents re-prompting on subsequent Complete calls within a session.

The design doc describes model downloads as a separate consent step from the addon download. The implementation correctly separates these: addon prompt in `AddonManager.EnsureAddon`, model prompt in `LocalProvider.ensureModelReady`.

#### AC: decline with cloud provider message -- VERIFIED

`internal/llm/local.go` at lines 96-99: When addon download is declined, returns `"local LLM addon download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead"`.

At lines 117-120: When model download is declined, returns `"model download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead"`.

This matches the design doc's intent that declining local inference should guide users toward cloud providers.

#### AC: download progress bars -- DEVIATION

The mapping claims addon downloads already use `progress.Writer` and model downloads would need proto streaming. This is a plausible explanation: the addon binary is downloaded via the recipe system (which uses `progress.Writer` for HTTP downloads), while model downloads happen inside the Rust addon server where the Go side has no visibility into download progress without adding proto streaming. This is a genuine architectural constraint, not a shortcut.

#### AC: inference spinner -- VERIFIED

`internal/llm/local.go` at lines 130-140: The `Complete` method creates a `progress.Spinner` and calls `Start("Generating...")` before sending the request, then `Stop()` on success or `StopWithMessage("Generation failed.")` on error.

`internal/progress/spinner.go`: The Spinner implementation handles both TTY (animated) and non-TTY (single print) modes.

#### AC: non-TTY suppression -- VERIFIED

`internal/llm/addon/prompter.go` at line 40: `InteractivePrompter.ConfirmDownload` calls `progress.ShouldShowProgress()` and declines with `ErrDownloadDeclined` in non-TTY environments.

`internal/progress/spinner.go` at lines 51-55: In non-TTY mode, the spinner prints the message once without animation.

`internal/progress/progress.go` at lines 167-169: `ShouldShowProgress` checks if stdout is a terminal.

#### AC: --yes flag skips prompts -- FINDING

The `AutoApprovePrompter` struct exists in `internal/llm/addon/prompter.go` (lines 77-84) and is tested.

The `WithPrompter` factory option exists in `internal/llm/factory.go` (lines 106-110) and is tested.

**However, the --yes flag is NOT wired to the prompter in production code.** The `createAutoApprove` flag in `cmd/tsuku/create.go` is used for recipe preview confirmation and LLM discovery approval, but the LLM factory is created inside the builders (`internal/builders/github_release.go:219`, `internal/builders/homebrew.go:529`) via `llm.NewFactory(ctx)` without passing `WithPrompter`. The `WithPrompter` option exists but is never called from any production code path.

This means `LocalProvider.prompter` is always nil in production. Looking at `internal/llm/addon/manager.go` line 123-124:

```go
if m.prompter != nil {
    ok, err := m.prompter.ConfirmDownload(...)
```

When `prompter` is nil, the download proceeds without prompting -- the comment on `AddonManager.prompter` says "If nil, downloads proceed without prompting (legacy behavior)." Similarly in `local.go` line 219:

```go
if p.prompter != nil && !p.modelPrompted {
```

When `prompter` is nil, model downloads also proceed without prompting.

So the plumbing is complete (Prompter interface, InteractivePrompter, AutoApprovePrompter, NilPrompter, WithPrompter factory option, SetPrompter methods) but the wiring from `cmd/tsuku/create.go` to the factory is missing. The --yes flag and interactive prompting both require production callsite wiring that doesn't exist. **The infrastructure is built but the feature is not activated.**

### Finding severity: Blocking

The design doc's intent is clear: "tsuku prompts for confirmation before the initial download." The implementation builds all the pieces but doesn't connect them to the CLI. A user running `tsuku create` will never see a download prompt -- the addon and model will download silently. The --yes flag has no effect on download prompts because the prompts never appear.

This is a blocking finding because it affects two ACs (addon download prompt and --yes flag) at the integration level. The individual components work correctly in isolation (tests pass), but the end-to-end flow doesn't match the design's described behavior.

## Sub-check 2: Cross-issue Enablement

Downstream issues: none. Skipped.

## Backward Coherence

Previous summary: "Extended existing TestLLMGroundTruth rather than creating new test files."

The implementation is consistent with the existing codebase patterns:
- Uses the same `addon` package for addon management
- The Prompter interface follows the existing Provider interface pattern
- Tests use the same testify/require style
- Spinner uses the existing progress package conventions

No contradictions with prior work.

---

## Summary of Findings

| # | AC | Severity | Finding |
|---|---|----------|---------|
| 1 | --yes flag skips prompts / addon download prompt | Blocking | `WithPrompter` is defined and tested but never called from production code. The factory is created via `llm.NewFactory(ctx)` in the builders without a prompter. The prompter nil-check in both `AddonManager.EnsureAddon` and `LocalProvider.ensureModelReady` causes silent download (no prompt) when prompter is nil. The design doc requires prompting; the implementation has all the infrastructure but no wiring. |
| 2 | download progress bars | Advisory | Deviation is genuine -- model downloads happen inside the Rust addon where Go has no visibility. The addon installation uses the recipe system which already has progress bars. |
