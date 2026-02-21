# Scrutiny Review: Justification Focus
# Issue: #1827 — feat(cli): check satisfies index before generating recipes in tsuku create

## Overview

The requirements mapping reports all 8 ACs as "implemented" with no deviations. The justification focus therefore has no deviation explanations to evaluate directly. Instead, this review examines whether any "implemented" claims mask shortcuts that function as implicit deviations — specifically the --force test coverage.

## Independent Assessment of the Diff

The diff adds:

1. `checkExistingRecipe(l *recipe.Loader, toolName string) (string, bool)` — a helper that calls `l.GetWithContext()` and returns the canonical recipe name when found.
2. An early guard in `runCreate` that calls `checkExistingRecipe(loader, toolName)` when `!createForce`, prints a differentiated message (direct match vs. satisfies match), and calls `exitWithCode(ExitGeneral)`.
3. An updated comment on the pre-existing `os.Stat()` check explaining why both checks coexist.
4. Six new tests in `create_test.go` covering nil loader, direct name match (embedded and local), satisfies fallback match, no-match case, and a test named `TestCheckExistingRecipe_ForceSkipsCheck`.

The implementation is structurally sound. The issue is in how one of the six tests relates to the AC it purports to cover.

---

## AC-by-AC Justification Assessment

Since no ACs are marked as deviations, the justification lens applies to whether the "implemented" claims conceal shortcuts.

### AC 1: Before recipe generation begins, tsuku create calls the recipe loader to check if any existing recipe resolves the requested name

**Mapping evidence**: `create.go:checkExistingRecipe() lines 462-476`

**Assessment**: The helper exists at lines 467-476. The call in `runCreate` at line 486 precedes all builder-registry setup and API calls. No concern.

**Verdict**: Clean.

---

### AC 2: When an existing recipe satisfies the requested name, the command prints a message and exits with non-zero exit code

**Mapping evidence**: `create.go lines 487-493`

**Assessment**: Lines 487-493 print a differentiated message (canonical == requested name vs. canonical != requested name) and call `exitWithCode(ExitGeneral)`. The issue body specifies a message format of `Recipe '<canonical-name>' already satisfies '<requested-name>'. Use --force to create anyway.` The actual message at line 490-491 reads: `Error: recipe '%s' already satisfies '%s'. Use --force to create anyway.` — the word "Error:" prefixes the message and "Recipe" is replaced by "Error: recipe". This is close but not identical to the specified format. This is a minor stylistic divergence, not a functional shortcut. The non-zero exit code via `exitWithCode(ExitGeneral)` is confirmed.

**Verdict**: Clean. (The message format difference is within acceptable implementation latitude; it is advisory at most and not a justification concern.)

---

### AC 3: The check covers all loader tiers

**Mapping evidence**: `create.go line 471 calls loader.GetWithContext which traverses all tiers`

**Assessment**: `GetWithContext` was enhanced in #1826 to cover all tiers including the satisfies fallback. The delegation to `loader.GetWithContext` means coverage is inherited from the loader. No independent tier-walking logic is needed here. This is the correct approach and not a shortcut.

**Verdict**: Clean.

---

### AC 4: The existing --force flag also overrides the satisfies duplicate check

**Mapping evidence**: `create.go line 485: if !createForce wraps the entire check block`

**Assessment**: Line 485 `if !createForce {` is confirmed in the diff. The check block (lines 486-494) is entirely inside this guard. When `createForce` is true, `checkExistingRecipe` is never called.

**Verdict**: Clean.

---

### AC 5: When --force is set, the satisfies check is skipped entirely

**Mapping evidence**: `create.go line 485`

Same as AC 4. Confirmed.

**Verdict**: Clean.

---

### AC 6: The satisfies check runs early in runCreate

**Mapping evidence**: `create.go lines 481-495`

**Assessment**: The guard block is placed immediately after `toolName := args[0]`, before `builderRegistry.NewRegistry()`, before any API calls. This is confirmed in the diff.

**Verdict**: Clean.

---

### AC 7: Direct name matches are also caught by this check

**Mapping evidence**: `create.go line 486-488`

**Assessment**: The differentiated message at lines 487-488 handles the `canonicalName == toolName` case explicitly. The test `TestCheckExistingRecipe_DirectNameMatchEmbedded` (for embedded) and `TestCheckExistingRecipe_DirectNameMatchLocal` (for local) confirm this path. The `os.Stat()` check that used to be the only direct-name check still exists but is now supplemented.

**Verdict**: Clean.

---

### AC 8: Unit tests cover all required cases

**Mapping evidence**: "6 tests in create_test.go"

**Assessment (main finding):**

The issue body specifies four test cases:

> (a) satisfies match triggers the warning and prevents generation
> (b) --force overrides the check
> (c) direct name match via loader also triggers the check
> (d) no match allows generation to proceed normally

The six tests are:

1. `TestCheckExistingRecipe_SatisfiesMatchPreventsGeneration` — covers (a)
2. `TestCheckExistingRecipe_DirectNameMatchLocal` — covers part of (c)
3. `TestCheckExistingRecipe_DirectNameMatchEmbedded` — covers part of (c)
4. `TestCheckExistingRecipe_NoMatchAllowsGeneration` — covers (d)
5. `TestCheckExistingRecipe_NilLoader` — defensive edge case (not explicitly listed)
6. `TestCheckExistingRecipe_ForceSkipsCheck` — claimed to cover (b)

**Test 6 does not cover (b).** The test body is:

```go
func TestCheckExistingRecipe_ForceSkipsCheck(t *testing.T) {
    // Verify that the recipe WOULD be found if checked, documenting that
    // the --force skip is at the call site in runCreate, not inside
    // checkExistingRecipe.
    l := newTestLoader(t)
    _, found := checkExistingRecipe(l, "openssl")
    if !found {
        t.Fatal("expected recipe to exist (test setup verification)")
    }
}
```

This test calls `checkExistingRecipe` directly without setting `createForce = true` and without invoking `runCreate`. It verifies only that the recipe loader would find `openssl` — that is, it is a setup verification, not a behavioral assertion about the `--force` code path.

The test comment is transparent about this: "documenting that the --force skip is at the call site in runCreate, not inside checkExistingRecipe." But this transparency does not substitute for a test that actually exercises the `if !createForce` guard. The `createForce` global is package-level and settable in tests; a proper test would set `createForce = true`, call `runCreate` (or the guard directly), and assert the check is not invoked.

The mapping marks AC 8 as "implemented" without flagging this gap. Because the test exists and appears plausible from the name, this is a case where the evidence looks stronger than it is. The implementing agent appears to have accepted "the check is documented" as equivalent to "the behavior is tested."

**Severity: Blocking.** AC (b) — `--force` overrides the check — is a required test case per the issue body. The test `TestCheckExistingRecipe_ForceSkipsCheck` does not test this behavior. It tests that a recipe exists, which is a precondition already tested by other tests in the file. Fixing this requires a test that sets `createForce = true` before the check runs and asserts that generation proceeds (or at minimum that `checkExistingRecipe` is not reached when `!createForce` is false).

---

## Proportionality Assessment

Seven of eight ACs have genuinely clean implementations with no shortcuts. The single gap is in AC 8's --force test coverage, which is a structural issue: the test is named to suggest coverage but tests a different thing. This is not a pattern of selective effort — the rest of the work is solid.

---

## Summary

No deviation explanations exist to evaluate; all ACs are claimed as "implemented." Six of eight ACs are correctly marked with legitimate evidence. One AC (3) relies on upstream implementation from #1826, which is the appropriate design.

The one blocking finding: AC 8 requires a test for `--force` overriding the check. `TestCheckExistingRecipe_ForceSkipsCheck` does not test this — it tests that the recipe loader finds `openssl`, which is a precondition already covered by `TestCheckExistingRecipe_DirectNameMatchEmbedded`. Setting `createForce = true` and asserting the guard is skipped is the missing assertion.

One minor advisory: the printed error message format (`Error: recipe '%s' already satisfies '%s'`) differs slightly from the issue body's specified format (`Recipe '<canonical-name>' already satisfies '<requested-name>'`). This is not a functional gap.
