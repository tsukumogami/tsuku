# Pragmatic Review: Issue #1754

## Findings

### 1. Dead env var save/restore in TestWriteBaseline_MinimumPassRate - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/baseline_test.go:12-17` -- `TSUKU_BASELINE_DIR` is saved and restored but never read or set by any code in the codebase. This is vestigial scaffolding for an env-var-based override that was replaced by the `*ToDir` pattern. Remove the save/restore block (lines 12-17).

### 2. containsSubstring reimplements strings.Contains - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/baseline_test.go:315-326` -- `containsSubstring` and `searchSubstring` manually reimplement `strings.Contains`. The file doesn't import `strings` at all. Import `strings` and use `strings.Contains` directly. This is test-only code so it's inert, but it's a needless 12 lines.

### 3. baselineDir second candidate is speculative - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:90` -- `"testdata/llm-quality-baselines"` is a candidate path that would only match if tests run from the repo root, but Go test always sets cwd to the package directory (`internal/builders/`). This candidate never matches. Also, the fallback on line 99 silently returns the same path as the first candidate even when it doesn't exist. Minor: remove the dead candidate; consider returning an error from the fallback.

### 4. loadBaseline/writeBaseline single-caller wrappers justified by testability - NO FINDING

`loadBaseline` and `writeBaseline` each have one caller, but they exist so that unit tests can call `loadBaselineFromDir`/`writeBaselineToDir` with temp directories. This is a standard Go test pattern. No action needed.

### 5. Scope assessment - NO FINDING

The PR adds multi-provider detection, baseline loading/writing, regression reporting, the `-update-baseline` flag, the Claude baseline file, and unit tests for the baseline logic. All items are in the issue acceptance criteria. No scope creep detected.

## Summary

Blocking: 0, Advisory: 3
