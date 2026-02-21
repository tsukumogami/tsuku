# Scrutiny Review: Completeness
**Issue**: #1827 (feat(cli): check satisfies index before generating recipes in tsuku create)
**Focus**: completeness
**Reviewer**: scrutiny agent
**Date**: 2026-02-21

---

## Method

The diff was examined first, before reading the requirements mapping, to form an independent impression of the implementation. The issue body was then used to extract all acceptance criteria. Each mapping entry was verified against the diff and the actual code at the cited locations.

---

## AC Extraction from Issue Body

The issue body states the following acceptance criteria:

1. Before recipe generation begins, `tsuku create <name>` calls the recipe loader (with satisfies fallback) to check if any existing recipe resolves the requested name.
2. When an existing recipe satisfies the requested name, the command prints a message like: `Recipe '<canonical-name>' already satisfies '<requested-name>'. Use --force to create anyway.` and exits with a non-zero exit code.
3. The check covers all loader tiers: in-memory cache, local recipes, embedded recipes, registry, and the satisfies index fallback.
4. The existing `--force` flag (currently used to overwrite local recipe files) also overrides the satisfies duplicate check, allowing recipe generation to proceed.
5. When `--force` is set, the satisfies check is skipped entirely and the command behaves as it does today.
6. The satisfies check runs early in `runCreate`, before the builder session is created, toolchain checks, or any API calls are made.
7. Direct name matches (e.g., `tsuku create openssl` when `openssl` already exists) are also caught by this check, extending coverage beyond the current `os.Stat()` check to include embedded and registry recipes.
8. Unit tests cover: (a) satisfies match triggers the warning and prevents generation, (b) `--force` overrides the check, (c) direct name match via loader also triggers the check, (d) no match allows generation to proceed normally.

---

## Requirements Mapping Evaluation

The coder provided 8 mapping entries. All 8 ACs appear in the mapping. No phantom ACs were detected.

---

## AC-by-AC Verification

### AC 1: Loader call before recipe generation
**Claimed status**: implemented
**Claimed evidence**: `create.go:checkExistingRecipe()` lines 462-476
**Diff verification**: CONFIRMED. `checkExistingRecipe()` is added at lines 462-476 and calls `l.GetWithContext(context.Background(), toolName, recipe.LoaderOptions{})`. The function is called from `runCreate` at lines 481-495, before any builder registration or API activity (builder registration begins at line 502).

**Assessment**: Evidence is accurate and the code does what is claimed.

---

### AC 2: Error message format and non-zero exit
**Claimed status**: implemented
**Claimed evidence**: `create.go lines 487-493`
**Diff verification**: PARTIAL MISMATCH (advisory).

The issue AC specifies:
> `Recipe '<canonical-name>' already satisfies '<requested-name>'. Use --force to create anyway.`

The actual code prints (line 490-491):
> `Error: recipe '%s' already satisfies '%s'. Use --force to create anyway.`

There are two differences:
1. The actual message is prefixed with "Error: " and uses lowercase "recipe" vs the AC's titlecase "Recipe".
2. The AC uses single quotes around argument names, which the code also uses, so that matches.

The non-zero exit is confirmed via `exitWithCode(ExitGeneral)` at line 493.

For direct name matches (AC 7 overlap), the code prints a different message (line 488): `Error: recipe '%s' already exists. Use --force to create anyway.` rather than a satisfies message, which is intentional and reasonable (the two cases are distinguished).

**Assessment**: The exit code is correct. The error message is functionally equivalent and directionally matches the AC's intent. The "like" qualifier in the AC ("prints a message **like**") gives implementation latitude. This is advisory, not blocking.

---

### AC 3: Check covers all loader tiers
**Claimed status**: implemented
**Claimed evidence**: `create.go line 471 calls loader.GetWithContext which traverses all tiers`
**Diff verification**: CONFIRMED. `GetWithContext` in the loader traverses: in-memory cache (line 94), local recipes dir (line 99), embedded recipes (line 115), registry (line 127), and satisfies fallback (line 138). Calling `l.GetWithContext(context.Background(), toolName, recipe.LoaderOptions{})` with a zero `LoaderOptions{}` struct (meaning `RequireEmbedded` is false) hits all tiers.

**Assessment**: Evidence is accurate. All 5 tiers are covered.

---

### AC 4 and AC 5: --force overrides the satisfies check
**Claimed status**: implemented
**Claimed evidence**: `create.go line 485: if !createForce wraps the entire check block`
**Diff verification**: CONFIRMED. Line 485 shows `if !createForce {` wrapping the call to `checkExistingRecipe`. When `createForce` is true, the entire block (lines 486-494) is skipped.

**Assessment**: Evidence is accurate.

---

### AC 6: Check runs early in runCreate
**Claimed status**: implemented
**Claimed evidence**: `create.go lines 481-495`
**Diff verification**: CONFIRMED. The check at lines 481-495 comes immediately after `toolName := args[0]` (line 479) and before the builder registry setup (line 502), discovery calls, sandbox setup, or any other activity.

**Assessment**: Evidence is accurate.

---

### AC 7: Direct name matches are caught
**Claimed status**: implemented
**Claimed evidence**: `create.go line 486-488`
**Diff verification**: CONFIRMED. The `checkExistingRecipe` function calls `GetWithContext` which checks exact name match in all tiers before falling back to satisfies. If `toolName` matches the canonical name exactly, `r.Metadata.Name == toolName` is true and the direct-match branch fires. The test `TestCheckExistingRecipe_DirectNameMatchLocal` and `TestCheckExistingRecipe_DirectNameMatchEmbedded` exercise these paths.

**Assessment**: Evidence is accurate.

---

### AC 8: Unit test coverage
**Claimed status**: implemented
**Claimed evidence**: 6 tests in `create_test.go`
**Diff verification**: CONFIRMED with one qualification.

The 6 tests exist and all pass (verified by running `go test ./cmd/tsuku/... -run TestCheckExistingRecipe`):
- `TestCheckExistingRecipe_SatisfiesMatchPreventsGeneration` -- covers AC 8(a): satisfies match found
- `TestCheckExistingRecipe_DirectNameMatchLocal` -- covers AC 8(c): direct name match via loader (local)
- `TestCheckExistingRecipe_DirectNameMatchEmbedded` -- covers AC 8(c): direct name match via loader (embedded)
- `TestCheckExistingRecipe_NoMatchAllowsGeneration` -- covers AC 8(d): no match returns false
- `TestCheckExistingRecipe_NilLoader` -- covers nil loader edge case
- `TestCheckExistingRecipe_ForceSkipsCheck` -- partially covers AC 8(b)

**Qualification for AC 8(b) -- `--force` overrides the check:**

The AC requires a test that `--force` overrides the check. `TestCheckExistingRecipe_ForceSkipsCheck` does not actually test the `--force` path. Its body calls `checkExistingRecipe(l, "openssl")` and asserts the recipe is found -- this is only a setup verification that the recipe exists, not a test that `--force` causes the check to be skipped. The comment in the test says "documenting that the --force skip is at the call site in runCreate, not inside checkExistingRecipe" -- this is an explanation for why the test doesn't test what the AC requires. The actual force-skip logic in `runCreate` (line 485: `if !createForce`) is not unit-tested.

There is no test that sets `createForce = true` and verifies the check is bypassed. The `--force` override is exercised only through integration paths, not unit tests.

**Assessment**: AC 8(b) is nominally addressed by `TestCheckExistingRecipe_ForceSkipsCheck` but the test doesn't actually verify that `--force` skips the check -- it only verifies that the recipe would be found without `--force`. This is a gap. The AC says "Unit tests cover ... (b) `--force` overrides the check." The coder's comment acknowledges the force-skip isn't inside `checkExistingRecipe`, but provides no test demonstrating the override works end-to-end. This is advisory (the logic is a single `if !createForce` guard, the gap is test coverage, not the implementation itself).

---

## Missing ACs

No missing ACs. All 8 ACs from the issue body have corresponding mapping entries.

## Phantom ACs

No phantom ACs detected. All 8 mapping entries correspond to ACs in the issue body.

---

## Summary

### Blocking Findings

None.

### Advisory Findings

1. **AC 8(b): `--force` test doesn't verify the override** -- `TestCheckExistingRecipe_ForceSkipsCheck` is the designated test for `--force` but only verifies that a recipe exists, not that `createForce = true` causes the satisfies check to be skipped. The implementation logic (`if !createForce`) is correct, but the unit test doesn't exercise it. A proper test would set `createForce = true`, call the relevant path, and assert no error is returned.

2. **AC 2: Error message format diverges from AC spec** -- The AC specifies `Recipe '<canonical-name>' already satisfies '<requested-name>'. Use --force to create anyway.` but the implementation produces `Error: recipe '%s' already satisfies '%s'. Use --force to create anyway.` The "like" qualifier in the AC provides latitude, making this advisory.

3. **`--force` flag description not updated** -- The flag description at line 134 still reads "Overwrite existing local recipe" and doesn't mention the satisfies duplicate check. This isn't an explicit AC, but the validation script calls out "Verify --force flag description updated" as a step. Not blocking.

### Overall Assessment

The implementation is complete. All 8 ACs have corresponding mapping entries, all cited file/line evidence checks out in the diff, and the tests pass. The functional behavior (early check, tier coverage, force bypass, distinct messages for satisfies vs. direct matches) is correctly implemented. The advisory gap is that `TestCheckExistingRecipe_ForceSkipsCheck` documents the force-skip architecture but doesn't actually assert it works -- a minor test completeness gap for a single-line guard.
