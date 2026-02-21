# Pragmatic Review: Issue #1642

## Issue: feat(llm): add download permission prompts and progress UX

## Review Focus: pragmatic (simplicity, YAGNI, KISS)

---

## Finding 1: Spinner.SetMessage is dead code

**File**: `internal/progress/spinner.go:61`
**Severity**: Advisory

`SetMessage` is exported, tested, but has zero production callers. Only the test `TestSpinner_TTY_SetMessage` exercises it. The spinner is always created, started with a message, and stopped -- no mid-flight message changes occur anywhere in the codebase.

**Suggestion**: Remove `SetMessage` and its test. If a future issue needs it, it's trivial to add back. The method is small enough that this isn't blocking.

---

## Finding 2: NilPrompter could be inlined

**File**: `internal/llm/addon/prompter.go:86-93`
**Severity**: Advisory

`NilPrompter` is only used in tests (`manager_test.go:411`, `factory_test.go:697`). It's a named struct with a single method that returns a constant. Tests could use an inline anonymous function or a `mockPrompter{approved: false, err: ErrDownloadDeclined}` (which already exists in manager_test.go). However, it's 7 lines and documents the "always decline" semantic clearly. Not worth blocking.

---

## Finding 3: Scrutiny review's blocking finding is incorrect

The prior scrutiny review flagged that `WithPrompter` is "never called from production code." This is wrong. The factory defaults to `InteractivePrompter` at `factory.go:158-159` when no explicit prompter is passed. The `--yes` flag wiring exists at `cmd/tsuku/create.go:572-576` and `cmd/tsuku/create.go:1041-1045`. Both the interactive and auto-approve paths are fully wired.

This is not a finding against the implementation -- noting it for the record to correct the scrutiny review's assessment.

---

## Summary

No blocking findings. The implementation is straightforward: a Prompter interface with two concrete implementations (interactive and auto-approve), a spinner for inference feedback, and factory-level default wiring. The pieces connect end-to-end through the existing factory options pattern.

Two advisory items: `Spinner.SetMessage` has no production callers (dead method), and `NilPrompter` is test-only code in the production package. Neither compounds or creates maintenance burden.
