# Decision: Post-Install Verify Output Level

**Decision ID**: design_install-ux-v2_decision_3
**Date**: 2026-04-21
**Status**: Decided

## Question

During a post-install verify pass, should output show all sub-steps, be suppressed entirely (status-only), or show only failure detail?

## Context

`RunToolVerification` in `cmd/tsuku/verify.go` is called in two contexts:

1. **`tsuku verify <tool>`** (interactive command) — passes `Verbose: true`, expects full sub-step output.
2. **Post-install pass** in `cmd/tsuku/install_deps.go:583` — currently passes `Verbose: true` as well, which produces 8-12 lines per tool via `printInfo`/`printInfof` (direct `fmt.Printf` bypassing the `Reporter` interface).

The post-install call at line 583 is:
```go
opts := ToolVerifyOptions{Verbose: true, SkipPATHChecks: true, SkipDependencyValidation: true}
```

The key asymmetry: `RunToolVerification` emits output through `printInfo`/`printInfof` (global `fmt.Printf` wrappers), not through the `Reporter` interface. The `reporter` variable at the call site has `Status()`, `Log()`, `DeferWarn()`, etc., but `RunToolVerification` never receives it.

## Options Evaluated

### Option A: Sub-steps as Status() calls

Route each sub-step string through `reporter.Status()`. On TTY, the spinner shows the current step and replaces it; on non-TTY, `Status()` is a no-op, so CI gets zero output on success.

On failure, the error surfaces through `return fmt.Errorf(...)`, which the caller at line 584 wraps and passes to `reporter.Log` or `fmt.Fprintf(os.Stderr, ...)`.

**Problem**: The current failure path for sub-steps bundles detail into the error string itself (e.g., `"pattern mismatch\n  Expected: %s\n  Got: %s"`). That detail reaches the user. But intermediate step context — which step failed, what command ran before the failure — is only visible if those steps were emitted as `Status()` and the spinner happened to show them. On non-TTY, that context is invisible.

More importantly: `RunToolVerification` accepts `ToolVerifyOptions`, not a `Reporter`. Routing through `Status()` requires threading `Reporter` into the function signature, which is a larger refactor.

### Option B: Suppress entirely (no output, no Status)

Pass `Verbose: false` to `RunToolVerification`. Sub-steps emit nothing. On success, the caller's `reporter.Log("Verifying %s@%s", ...)` already prints one line (line 569). On failure, the error string from `RunToolVerification` contains enough detail (it embeds the failing command output, expected/actual patterns, mismatch descriptions).

**Assessment**: Meets all constraints with zero interface changes. The single `reporter.Log("Verifying %s@%s", ...)` call already exists and provides the status line. The spinner at the call site (`reporter.Status(...)` in the surrounding install loop) covers the "in-progress" signal on TTY. On failure, `fmt.Errorf` chains embed the diagnostic detail. The `Verbose: true` path is preserved for the `tsuku verify` command.

The downside is that CI gets exactly one line on success ("Verifying nodejs@25.9.0") with no sub-step confirmation. That's the correct behavior for CI: less noise, full error on failure.

### Option C: Show only failure detail (emit nothing on success, Log() lines on failure)

This is Option B but with the addition of buffering sub-steps and flushing them on failure. Requires either buffering inside `RunToolVerification` (needs `Reporter`) or a retry-read approach. Adds complexity for marginal benefit — the error strings already encode failure detail.

The key insight: the failure path in `RunToolVerification` already returns structured errors. Step 1 failure returns `"installation verification failed: %w\nOutput: %s"`. Integrity failure returns the mismatch table. These reach the user through the `fmt.Errorf` wrapping in the caller. There's nothing to buffer that isn't already in the error.

## Decision: Option B

**Change**: Pass `Verbose: false` in the post-install `ToolVerifyOptions`.

**Rationale**:

1. The sub-step lines in `RunToolVerification` are diagnostic traces, not user-facing status. The `reporter.Log("Verifying %s@%s", ...)` line already fulfills the "user knows verification is running" requirement.

2. On TTY, the spinner driven by earlier `reporter.Status(...)` calls in the install loop covers the in-progress signal. There's no need for 8-12 sub-step lines to scroll past.

3. On non-TTY (CI), `Status()` is already a no-op. The one `reporter.Log` call gives a single confirmation line. This is better than 8-12 lines per dependency.

4. Failure detail is not lost. All failure paths in `RunToolVerification` return `fmt.Errorf` with structured messages that include command output, expected vs. actual patterns, and mismatch tables. The caller wraps this at line 584: `return fmt.Errorf("installation verification failed: %w", err)`. That full error reaches the terminal.

5. This requires one character change (`true` → `false`) at the call site. No interface changes, no `Reporter` threading, no buffering logic.

6. The `Verbose: true` path is preserved for `tsuku verify <tool>` (line 1052), where detailed output is appropriate and expected.

## What Changes

File: `cmd/tsuku/install_deps.go`, line 583.

```go
// Before
opts := ToolVerifyOptions{Verbose: true, SkipPATHChecks: true, SkipDependencyValidation: true}

// After
opts := ToolVerifyOptions{Verbose: false, SkipPATHChecks: true, SkipDependencyValidation: true}
```

The surrounding `reporter.Log("Verifying %s@%s", toolName, version)` at line 569 remains — it's the correct single-line confirmation.

## Assumptions

- The design already passes a `Reporter` to the install loop and uses `reporter.Log` at lines 569 and 604. The verify sub-steps are the only remaining `printInfo`/`printInfof` calls in that code path.
- Failure messages returned by `RunToolVerification` are self-contained. They include command output and pattern details without requiring the sub-steps to have been printed first.
- The `tsuku verify` command is the appropriate place for verbose output. Post-install verification is a sanity check, not a diagnostic session.

## Rejected Options

**Option A** — routing sub-steps through `Status()` — adds implementation complexity (threading `Reporter` into `RunToolVerification`) and still doesn't solve the non-TTY case without further work. On non-TTY, `Status()` is a no-op, so CI would see zero output even for the "which step failed" context. The failure error strings already provide that context, making the added complexity unjustified.

**Option C** — buffering sub-steps and flushing on failure — adds buffering logic for information already present in the error chain. It's strictly more work than Option B for no additional user value.
