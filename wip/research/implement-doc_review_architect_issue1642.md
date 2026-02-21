# Architect Review: Issue #1642

## Issue: feat(llm): add download permission prompts and progress UX

## Review Focus: architecture (design patterns, separation of concerns)

---

## Files Changed

- `internal/llm/addon/prompter.go` (new)
- `internal/llm/addon/prompter_test.go` (new)
- `internal/progress/spinner.go` (new)
- `internal/progress/spinner_test.go` (new)
- `internal/llm/addon/manager.go` (modified)
- `internal/llm/addon/manager_test.go` (modified)
- `internal/llm/factory.go` (modified)
- `internal/llm/local.go` (modified)

---

## Finding 1: Duplicate byte formatting function

**Severity: Advisory**

**Location:** `internal/llm/addon/prompter.go:96` (`FormatSize`) vs `internal/progress/progress.go:134` (`formatBytes`)

Both functions do the same thing: format a byte count into a human-readable string (KB, MB, GB). The implementations are nearly identical -- same constants, same switch structure, same precision. The only differences are naming (`FormatSize` vs `formatBytes`), export visibility (exported vs unexported), and minor formatting (space before unit vs no space: `"50.0 MB"` vs `"50.0MB"`).

This is a parallel pattern. The `progress` package already has this utility. The new code in the `addon` package introduces a second one. Two callers -- `progress.Writer` and `addon.InteractivePrompter` -- now independently format byte counts.

**Impact:** Low. `FormatSize` has exactly one caller (the prompter). It won't spread because other code already uses `formatBytes`. The formatting difference (space before unit) is intentional for the user-facing prompt context. If `formatBytes` were exported from `progress` and reused, the output format would need to stay consistent across both contexts, which may not be desirable.

**Suggestion:** Export `formatBytes` from `progress` as `FormatBytes` and use it in the prompter, or accept the duplication since the formatting needs differ slightly and the function is trivial.

---

## Finding 2: Prompter wiring through factory default

**Severity: Advisory (positive observation)**

The factory defaults to `InteractivePrompter` when no explicit `WithPrompter` option is passed (`factory.go:154-159`). This means all production callers get prompting by default without needing to change. The `--yes` flag wiring in `cmd/tsuku/create.go:572-577` correctly passes `WithPrompter(&addon.AutoApprovePrompter{})` through `LLMFactoryOptions`, which flows through the builders (`github_release.go:221`, `homebrew.go:413`) and discovery (`llm_discovery.go:155`).

The design doc says "tsuku prompts for confirmation before the initial download." The architecture accomplishes this with a safe-by-default approach: the factory provides the prompter, not the caller. Callers only need to override when they want non-interactive behavior. This is a sound pattern.

The `prompterExplicit` flag on `factoryOptions` (`factory.go:42`) correctly distinguishes "no one passed `WithPrompter`" from "someone explicitly passed `WithPrompter(nil)`", ensuring tests can bypass prompting without getting the default.

---

## Finding 3: Spinner placement in progress package

**Severity: Advisory (positive observation)**

`Spinner` lives in `internal/progress/`, next to the existing `Writer`. Both handle the same concern: giving the user visual feedback during long operations. `Writer` tracks download progress (bytes transferred), `Spinner` shows indeterminate activity (inference waiting). They share the same TTY detection mechanism (`ShouldShowProgress()`), the same output target convention (stderr), and the same line-clearing approach (`\r` + space padding).

The dependency direction is correct: `internal/llm/local.go` imports `internal/progress`, not the other way around. `Spinner` has no knowledge of LLM concepts -- it's a general-purpose UI primitive that happens to be used by the LLM provider first.

---

## Finding 4: Model readiness check via GetStatus RPC

**Severity: Advisory**

`local.go:205-238` (`ensureModelReady`) uses the `GetStatus` RPC to check whether the model is loaded, and if not, prompts for download consent. If `GetStatus` fails (line 207-211), the method silently returns nil and lets the `Complete` RPC proceed, which will trigger model download inside the Rust addon without user consent.

The comment says "the server may still be starting up" as justification for swallowing the error. This is plausible during the preemptive startup window. However, there's an architectural tension: the design doc says the Go side handles "User prompts" while the Rust addon handles "Model download." If `GetStatus` fails consistently (e.g., proto version mismatch), the consent mechanism is bypassed entirely.

This is contained to the `ensureModelReady` method and doesn't affect other components. The worst case is a download without prompting, which was the pre-#1642 behavior (so no regression). But it does mean the prompting guarantee has a known gap.

**Suggestion:** Consider logging a warning when `GetStatus` fails, so the gap is visible in debug output. Not blocking because the fallback behavior is the prior default.

---

## Finding 5: No state contract changes

No new fields added to state structs. No template variable changes. No CLI surface changes (the `--yes` flag already existed for other purposes; this issue reuses it for download prompts). No new subcommands. The implementation fits within existing boundaries.

---

## Overall Assessment

The implementation fits the existing architecture cleanly. The Prompter interface follows the established pattern of injectable behaviors with safe defaults. The Spinner is placed in the right package. The factory's default-to-interactive approach means existing callers get prompting without changes. The `--yes` flag flows through the existing `LLMFactoryOptions` plumbing in builders and discovery.

The only structural imperfection is the `FormatSize`/`formatBytes` duplication, which is minor and contained. No blocking findings.
