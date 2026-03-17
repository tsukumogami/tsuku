---
review: pragmatic
issue: 2
title: "feat(state): add source tracking to ToolState"
---

# Pragmatic Review: Issue 2

## Finding 1: `Plan.RecipeSource` value inconsistency between install paths

**Severity: Advisory**

`cmd/tsuku/helpers.go:205` normalizes `RecipeSource` to `"central"` via `recipeSourceFromProvider()` before passing it to `PlanConfig`. But `cmd/tsuku/install_deps.go:125` hardcodes `RecipeSource: "registry"` (the raw provider value). This means:

- Tools installed via `tsuku install` get `Plan.RecipeSource = "central"`
- Dependencies installed via `installDeps` get `Plan.RecipeSource = "registry"`

The migration handles both correctly (anything not `"local"` defaults to `"central"`), so there's no functional bug. But if code ever inspects `Plan.RecipeSource` directly (not via the migration), it'll see inconsistent values for the same source.

**Fix:** Change `install_deps.go:125` to `RecipeSource: "central"` for consistency with the normalized vocabulary.

## Finding 2: `ToolState.Source` never set explicitly during install

**Severity: Advisory**

Acceptance criteria says "New installs populate `Source` during plan generation in `cmd/tsuku/helpers.go`." The implementation relies entirely on the lazy migration in `migrateSourceTracking()` running inside `loadWithoutLock()` during the `UpdateTool` call. This works because the install flow does `UpdateTool` -> `loadWithoutLock()` -> `migrateSourceTracking()` -> save. But it's an indirect path that requires understanding the full call chain to verify correctness.

No fix needed -- the current approach is correct. Flagging for documentation accuracy.

## Finding 3: `install_lib.go:116` also hardcodes `"registry"`

**Severity: Advisory**

`cmd/tsuku/install_lib.go:116` passes `RecipeSource: "registry"` directly to `PlanConfig`, same inconsistency as Finding 1.

**Fix:** Same as Finding 1 -- normalize to `"central"`.

## Summary

No blocking issues. The migration logic is correct, idempotent, and well-tested (7 test cases covering default, inference, idempotency, skip-existing, round-trip, absent-in-JSON, and absent-with-plan). The `recipeSourceFromProvider` function and its test cover all current and future source types.

The main gap is the inconsistent `RecipeSource` values between `helpers.go` (normalized) and `install_deps.go`/`install_lib.go` (raw). This doesn't break anything today but could confuse future readers or code that inspects Plan.RecipeSource directly.
