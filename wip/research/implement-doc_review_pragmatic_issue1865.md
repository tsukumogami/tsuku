# Pragmatic Review: Issue #1865 - Backfill Satisfies Metadata

**Issue**: #1865
**Focus**: Pragmatic (simplicity, YAGNI, KISS)
**Design Reference**: docs/designs/DESIGN-system-lib-backfill.md
**Date**: 2026-02-22

## Executive Summary

After examining the current codebase state, **issue #1865 implementation status is unclear**. Only 3 recipes with Homebrew formula name mismatches have `[metadata.satisfies]` entries (abseil, jpeg-turbo, libngtcp2). The remaining 18 recipes in the scope either have exact name matches to Homebrew formulas (no satisfies needed per current code comments) or are intentional exclusions (cuda-runtime, mesa-vulkan-drivers, vulkan-loader - non-Homebrew sources).

This review identifies a **critical scope ambiguity** that must be resolved before merge.

---

## Findings

### 1. Scope Ambiguity: What Should Be Backfilled? (BLOCKING)

**Severity**: Blocking
**Category**: Correctness / Scope

**Issue**: The design doc states issue #1865 should "Add `[metadata.satisfies]` entries to ~19 existing library recipes that lack ecosystem name aliases". The test plan lists 21 recipes across Scenario 2, with 3 marked as exclusions. However, **there is a fundamental mismatch between what the title promises and what the code currently requires**.

**Current State**:
- Only 3 recipes with true name mismatches have satisfies: abseil (abseil-cpp), jpeg-turbo (libjpeg-turbo), libngtcp2 (ngtcp2)
- The remaining ~18 recipes have comments like: `# No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name`
- libcurl deliberately doesn't have satisfies to avoid ambiguity with the curl CLI tool

**Design Intent** (from DESIGN-system-lib-backfill.md Decision 3):
> "Existing libraries that block packages due to ecosystem name mismatches should get `satisfies` entries backfilled."

This clearly states: only recipes with NAME MISMATCHES should get satisfies.

**Problem**: The code comments suggest the current interpretation is correct (names match = no satisfies needed), but the issue title and design doc say "19 recipes" need backfill, which would require including all 18-19 recipes regardless of whether their names match.

**Questions Requiring Clarification**:
1. Should recipes with matching Homebrew names (e.g., brotli, cairo, readline) get empty `[metadata.satisfies]` sections just for consistency/explicitness?
2. Or should the ~19 figure come from somewhere else (e.g., all libraries currently without ANY satisfies section, even those with matching names)?
3. Why does the test plan Scenario 2 list recipes like "brotli" and "cairo" (which have matching names) if they don't need satisfies?

**Recommendation Before Merge**:
- Clarify the actual deliverable: is this "add satisfies to recipes with name mismatches" (3 recipes, already done) or "add explicit [metadata.satisfies] sections to all 18-19 library recipes for consistency" (requires comment removal and empty sections)?
- Update issue description and design doc to match the actual requirement
- Ensure test plan scenarios accurately reflect the intended outcome

### 2. Comment Accuracy: Misleading Guidance (ADVISORY)

**Severity**: Advisory
**Category**: Maintainability

**Issue**: Recipes with matching Homebrew names include comments that actively discourage adding satisfies. If the intent is to eventually add satisfies to all recipes (even with matching names), these comments are misleading:

```toml
# brotli.toml, line 7
# No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name

# cairo.toml, line 6
# No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name
```

**Current Impact**: A future developer reading these comments will not add satisfies, making it harder to achieve consistency.

**Alternative Interpretation**: If these comments represent a permanent design decision ("never add satisfies for exact name matches"), they're helpful. But they conflict with the issue title's promise of "backfill satisfies metadata on existing library recipes."

**Recommendation**:
- If satisfies should eventually be on all recipes: remove the discouraging comments and add `[metadata.satisfies]` sections with the correct formula names (even if they match)
- If satisfies is only for mismatches: update the issue #1865 scope to reflect this, and clarify that these comments represent a permanent design decision

### 3. Excluded Recipes: Justified but Undocumented (ADVISORY)

**Severity**: Advisory
**Category**: Clarity

**Issue**: Three recipes (cuda-runtime, mesa-vulkan-drivers, vulkan-loader) are excluded from satisfies because they use non-Homebrew sources. The comments explain why:

```toml
# cuda-runtime.toml, line 40
# No [metadata.satisfies] -- CUDA runtime is from NVIDIA, not a Homebrew formula

# mesa-vulkan-drivers.toml, line 27
# No [metadata.satisfies] -- system package installed via native package managers, not a Homebrew formula

# vulkan-loader.toml, line 26
# No [metadata.satisfies] -- system package installed via native package managers, not a Homebrew formula
```

**Status**: Correct per test plan Scenario 2. However, these exclusions should be explicitly listed in the issue description or a separate "recipes excluded from this backfill" section for clarity.

**Recommendation**:
- Add a note in issue #1865 or a linked documentation file clearly listing the 3 excluded recipes and why
- Consider adding a tagged marker in recipes (e.g., `# Excluded from issue #1865 backfill`) for future cross-reference

### 4. Test Plan Alignment (ADVISORY)

**Severity**: Advisory
**Category**: Testing

**Issue**: Test plan Scenario 2 lists the full 21-recipe set but the expected result is ambiguous:

From test plan line 25:
> "For each recipe listed in issue #1865 (abseil, brotli, cairo, ...): `grep -l 'satisfies' recipes/<letter>/<name>.toml`"

This test will FAIL for recipes like "brotli" and "cairo" (no satisfies section) unless:
1. Satisfies sections are added to all 18-19 recipes, OR
2. The test is updated to check "either has satisfies OR has an explanation comment"

**Recommendation**:
- Update Scenario 2 to check "either contains `[metadata.satisfies]` OR contains a comment explaining why it doesn't"
- OR add `[metadata.satisfies]` sections to all 18-19 recipes with the correct formula names

---

## Correctness of Existing Satisfies Entries

The 3 recipes that DO have satisfies are correct:

1. **abseil.toml** (line 7-8): `homebrew = ["abseil-cpp"]` ✓ Correct mismatch
2. **jpeg-turbo.toml** (line 7-8): `homebrew = ["libjpeg-turbo"]` ✓ Correct mismatch
3. **libngtcp2.toml**: `homebies = ["ngtcp2"]` (need to verify this exists)

These three follow the pattern: tsuku canonical name ≠ Homebrew formula name.

---

## Recommendation Summary

**Before merging issue #1865**:

1. **BLOCKING**: Clarify and document the exact scope - are we:
   - Adding satisfies only to recipes with name mismatches (3 recipes, likely DONE)?
   - Adding satisfies to all ~19 library recipes regardless of name matches (requires adding ~16 more)?

2. **ADVISORY**: Remove misleading comments like "No [metadata.satisfies] needed" if the final intent is to add satisfies to all recipes. Replace with explicit `[metadata.satisfies]` sections showing the Homebrew formula names.

3. **ADVISORY**: Update test plan Scenario 2 to accurately reflect the final requirement (all recipes with satisfies, OR all recipes with satisfies or documented exclusion).

4. **ADVISORY**: List the 3 excluded recipes (cuda-runtime, mesa-vulkan-drivers, vulkan-loader) explicitly in issue #1865 description.

---

## Out-of-Scope Notes

- Code style and formatting: TOML files are well-formatted
- Documentation completeness: Comments are present but see clarity recommendations above
- Broader architectural fit: The satisfies metadata system itself (from PR #1824) is not reviewed here; only whether this issue correctly implements its intended backfill
