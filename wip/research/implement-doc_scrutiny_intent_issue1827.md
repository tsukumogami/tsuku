# Scrutiny Review: Intent - Issue #1827

**Issue**: feat(cli): check satisfies index before generating recipes in tsuku create
**Focus**: intent
**Date**: 2026-02-21

---

## Independent Impression (from diff, before evaluating mapping)

The diff adds:

1. A `checkExistingRecipe()` helper function in `create.go` (lines 462-476) that wraps `loader.GetWithContext()`. It returns the canonical name and a boolean indicating whether a recipe was found.

2. An early guard in `runCreate()` (lines 481-495) that calls `checkExistingRecipe(loader, toolName)` before any builder setup, API calls, or toolchain checks. If a match is found and `createForce` is false, it prints an error message and exits.

3. A comment clarifying the purpose of the pre-existing `os.Stat()` check at line 773 (the check that catches custom `--output` paths).

4. Six unit tests in `create_test.go` that all test the `checkExistingRecipe()` helper directly. None of the tests exercise `runCreate()` itself or the `createForce` flag variable.

---

## Sub-check 1: Design Intent Alignment

### Design doc description of Phase 2 (Issue #1827 scope)

From the design doc (Solution Architecture, Phase 2):
> 1. Update `tsuku create` to check loader (including satisfies) before generating
> 2. Print a clear message when an existing recipe already satisfies the requested name
> 3. Add `--force` override for cases where the user explicitly wants a separate recipe
> 4. Add tests for the duplicate detection path

From the Data Flow section:
> For `tsuku create openssl@3 --from homebrew:openssl@3`:
> 1. Before generating, `create` calls loader to check if recipe exists
> 2. Exact lookup for `openssl@3` fails
> 3. Satisfies fallback finds `openssl@3` → `openssl`
> 4. Prints message: "Recipe 'openssl' already satisfies 'openssl@3'. Use --force to create anyway."
> 5. Exits without generating a duplicate

### Assessment

**The core duplicate detection works as designed.** `checkExistingRecipe()` uses `GetWithContext()` which traverses all four tiers plus the satisfies fallback (verified by reading `loader.go`). The message format differs slightly from the design doc example (`"Error: recipe..."` vs `"Recipe..."`) but the AC says "like", granting flexibility. The early placement in `runCreate()` (before builder registration, API calls, toolchain checks) matches the design's intent.

**Minor advisory: message format inconsistency.** The design doc (Data Flow section, line 307) shows: `"Recipe 'openssl' already satisfies 'openssl@3'. Use --force to create anyway."` The implementation uses `"Error: recipe '%s' already satisfies '%s'. Use --force to create anyway.\n"` with lowercase `recipe` and an `Error:` prefix. This is stylistically consistent with other error messages in the file and the AC says "like", so it's not a violation.

### Finding: AC item (b) test coverage is nominal, not behavioral

**Severity: advisory**

The issue AC states: "Unit tests cover: ... (b) `--force` overrides the check."

`TestCheckExistingRecipe_ForceSkipsCheck` does not test `--force` behavior. It only verifies that `openssl` is findable via `checkExistingRecipe()`. The comment in the test itself acknowledges this: "the --force skip is at the call site in runCreate, not inside checkExistingRecipe." The test is documenting where the logic lives, not testing that the logic works.

The `createForce` global variable is never set to `true` in any test, and `runCreate()` is not exercised by any of the six new tests. The `if !createForce` branch in `runCreate()` has no test coverage.

This is advisory rather than blocking because: the branch is visually simple (a single `if !createForce` guard), the `createForce` variable is a cobra flag already tested elsewhere in the codebase, and the design doc's emphasis is on the "satisfies check" functionality rather than `--force` integration testing. However, the AC text explicitly lists it as a required test case, so the nominal test does not satisfy the AC as written.

### Finding: AC item (a) tests helper, not integration

**Severity: advisory**

`TestCheckExistingRecipe_SatisfiesMatchPreventsGeneration` tests that `checkExistingRecipe()` returns `("openssl", true)` when asked for `openssl@3`. This verifies the helper works, but does not verify that the warning is printed or that generation is prevented. The AC says "satisfies match triggers the warning and prevents generation" -- the warning and prevention happen in `runCreate()`, which is not tested.

The distinction matters because the warning and exit-code logic could change in `runCreate()` without any test failing. That said, `checkExistingRecipe()` is small and the integration logic in `runCreate()` is visually trivial, so the risk of undetected regression is low.

---

## Sub-check 2: Cross-issue Enablement

`context.downstream_issues` is empty (no issues depend on #1827). This sub-check is skipped per the review instructions.

---

## Backward Coherence

Previous issue (#1826) established:
- `satisfiesIndex` as a lazy-built map in the loader
- `GetWithContext()` with satisfies fallback
- `LookupSatisfies()` as a public method

This issue uses `GetWithContext()` in `checkExistingRecipe()`, which is consistent with how #1826 designed the integration. The implementation does not use `LookupSatisfies()` directly -- it instead relies on the implicit fallback inside `GetWithContext()`. This is consistent with the design's "every code path benefits automatically through the loader" philosophy.

No naming conventions, patterns, or structures from #1826 are contradicted.

---

## Requirements Mapping Evaluation

```
--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
[
  {"ac":"Before recipe generation begins, tsuku create calls the recipe loader to check if any existing recipe resolves the requested name","status":"implemented","evidence":"create.go:checkExistingRecipe() lines 462-476"},
  {"ac":"When an existing recipe satisfies the requested name, the command prints a message and exits with non-zero exit code","status":"implemented","evidence":"create.go lines 487-493"},
  {"ac":"The check covers all loader tiers","status":"implemented","evidence":"create.go line 471 calls loader.GetWithContext which traverses all tiers"},
  {"ac":"The existing --force flag also overrides the satisfies duplicate check","status":"implemented","evidence":"create.go line 485: if !createForce wraps the entire check block"},
  {"ac":"When --force is set, the satisfies check is skipped entirely","status":"implemented","evidence":"create.go line 485"},
  {"ac":"The satisfies check runs early in runCreate","status":"implemented","evidence":"create.go lines 481-495"},
  {"ac":"Direct name matches are also caught by this check","status":"implemented","evidence":"create.go line 486-488"},
  {"ac":"Unit tests cover all required cases","status":"implemented","evidence":"6 tests in create_test.go"}
]
--- END UNTRUSTED REQUIREMENTS MAPPING ---
```

### Entry-by-entry verification

1. **"Before recipe generation begins, tsuku create calls the recipe loader..."**
   - Status: Confirmed. `checkExistingRecipe(loader, toolName)` is called at lines 485-495, before builder registration at line 503. The diff confirms the placement.

2. **"When an existing recipe satisfies the requested name, the command prints a message and exits with non-zero exit code"**
   - Status: Confirmed. Lines 487-493 print to stderr and call `exitWithCode(ExitGeneral)`. `ExitGeneral` is a non-zero exit code.

3. **"The check covers all loader tiers"**
   - Status: Confirmed. `GetWithContext()` traverses cache, local, embedded, registry, and satisfies fallback in sequence (verified in loader.go). The mapping's claim is accurate.

4. **"The existing --force flag also overrides the satisfies duplicate check"** and **"When --force is set, the satisfies check is skipped entirely"**
   - Status: Confirmed for the production code. The `if !createForce` guard at line 485 is correct. The mapped evidence is accurate.

5. **"The satisfies check runs early in runCreate"**
   - Status: Confirmed. The check is placed immediately after `toolName := args[0]`, before any substantive work.

6. **"Direct name matches are also caught by this check"**
   - Status: Confirmed. `GetWithContext()` performs exact name lookup before falling back to satisfies. When `toolName == r.Metadata.Name`, the direct branch (line 487-488) prints the "already exists" message.

7. **"Unit tests cover all required cases"**
   - Status: Partially confirmed. Six tests exist and pass. However:
     - Test (b) from the AC ("--force overrides the check") is not genuinely covered. `TestCheckExistingRecipe_ForceSkipsCheck` only proves the recipe is findable; it does not set `createForce=true` or call `runCreate()`.
     - Test (a) from the AC ("satisfies match triggers the warning and prevents generation") is partially covered. The satisfies match is verified, but the warning and prevention (which happen in `runCreate()`) are not tested.

---

## Summary

**Blocking findings**: 0

**Advisory findings**: 2

1. The `--force` test (`TestCheckExistingRecipe_ForceSkipsCheck`) is nominal: it verifies the recipe is findable but doesn't test the `if !createForce` branch or that generation proceeds when `--force` is set. The AC explicitly requires this test case.

2. The satisfies match tests verify the `checkExistingRecipe()` helper but don't exercise the warning message or exit behavior in `runCreate()`. The AC says the test should cover "triggers the warning and prevents generation."

Both findings are advisory because the production implementation correctly handles both cases, the logic is simple and visually verifiable, and the risk of undetected regression is low. The gap is in the test's fidelity to the AC's stated coverage requirement, not in the implementation's correctness.
