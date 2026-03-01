# Maintainability Review: Issue #1865 (Backfill satisfies metadata on existing library recipes)

**Issue**: #1865
**Review Focus**: maintainer (clarity, readability, duplication)
**Design Doc**: docs/designs/DESIGN-system-lib-backfill.md
**Date**: 2026-02-22

---

## Summary

Issue #1865 adds `[metadata.satisfies]` entries to 19 existing library recipes that lack ecosystem name aliases. The implementation is straightforward and follows a clear pattern established by recipes like `abseil.toml` and `jpeg-turbo.toml`.

**Finding**: One maintainability concern is identified—misleading comments in recipes that _don't_ have satisfies metadata but claim none is needed. This is blocking because the next developer reading the code will form the wrong mental model about when satisfies metadata is required.

---

## Finding 1: Misleading Comments on Recipes Without Satisfies (BLOCKING)

**Severity**: Blocking

**Files and Line References**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/r/readline.toml:7`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/e/expat.toml:6`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/b/brotli.toml:7`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/g/giflib.toml:6`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/g/gmp.toml:6`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/g/gettext.toml:6`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/l/libxml2.toml:6`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/l/libpng.toml:7`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/l/libssh2.toml:7`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/g/geos.toml:6`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/l/libnghttp3.toml:7`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/p/proj.toml:7`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/z/zstd.toml:7`

**What the code says:**
```toml
[metadata]
name = "gmp"
type = "library"
# No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name
```

**What the next person will think**: "The canonical name matches the Homebrew formula name, so satisfies metadata is never needed here. If I add a new recipe with a matching name, I don't need satisfies."

**What actually happens**: Issue #1865 specifically requires adding `[metadata.satisfies]` entries to these recipes. The comment is now false. Example: `gmp` will receive `satisfies = { homebrew = ["gmp"] }` even though the formula name already matches the canonical name.

**Why this is blocking**: The next developer maintaining this repo will see these comments and conclude the logic for when satisfies is needed is "formula name matches canonical name". This is incorrect. The actual rule (from the design doc Decision 3) is: "All library recipes should include satisfies metadata from the start. Existing libraries that lack it should get it backfilled." The distinction between "needed" and "should have" matters—the former is optional, the latter is mandatory.

**Impact on future changes**: If a new library recipe is added and someone reads these comments, they'll skip satisfies metadata thinking it's not required when the formula name matches. This will be caught in code review but wastes time and creates friction.

**Suggestion**: Replace comments on recipes being updated with `[metadata.satisfies]` entries to reflect the actual requirement:
```toml
# Adding satisfies metadata to enable ecosystem name resolution by the pipeline
[metadata.satisfies]
homebrew = ["gmp"]
```

Or, if the recipe's satisfies entry maps to the same canonical name, use a comment that explains the pattern:
```toml
[metadata.satisfies]
# Even though the tsuku name matches the formula name, including satisfies
# ensures the pipeline recognizes all ecosystem aliases for this library.
homebrew = ["gmp"]
```

For recipes that DON'T receive satisfies (cuda-runtime, mesa-vulkan-drivers, vulkan-loader), update the comment to explain _why_ no satisfies applies:
```toml
# No [metadata.satisfies] -- system package from NVIDIA (not Homebrew formula)
```
vs. the current comment which implies formula-name matching is the decision criterion.

---

## Finding 2: Consistent Pattern Across Affected Recipes (ADVISORY)

**Severity**: Advisory

**Context**: Recipes receiving satisfies metadata follow one of two patterns:

**Pattern A** (formula name matches canonical name):
- `readline`, `expat`, `brotli`, `giflib`, `gmp`, `gettext`, `libxml2`, `libpng`, `libssh2`, `geos`, `libnghttp3`, `proj`, `zstd`
- Will receive: `satisfies = { homebrew = ["gmp"] }` (name matches name)

**Pattern B** (formula name differs from canonical name):
- `abseil` (canonical) → `abseil-cpp` (formula) — already has satisfies
- `jpeg-turbo` (canonical) → `libjpeg-turbo` (formula) — already has satisfies
- `libngtcp2` (canonical) → `ngtcp2` (formula) — already has satisfies
- `libnghttp2` (canonical) → `nghttp2` (formula) — already has satisfies

**Readability note**: Pattern A recipes (name-matches-name) are less immediately obvious about _why_ they need satisfies. A reader seeing `homebrew = ["gmp"]` in the gmp recipe might not immediately understand it's for ecosystem name resolution when the names already match. Consider documenting this more explicitly.

**Suggestion (optional)**: Add a brief explanatory comment in recipes where formula name matches canonical name:
```toml
[metadata.satisfies]
# Enables pipeline dependency resolution (resolves "gmp" dependency to this recipe)
homebrew = ["gmp"]
```

This is a minor readability improvement and is not blocking.

---

## Finding 3: Three Recipes Correctly Exempted (CLEAR)

**Files**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/c/cuda-runtime.toml:40`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/m/mesa-vulkan-drivers.toml:27`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/v/vulkan-loader.toml:26`

**What's correct**: These three recipes have comments explaining _why_ no satisfies applies (they come from system package managers or NVIDIA, not Homebrew). The test plan scenario-2 explicitly expects these three to have documented exclusions, and they do.

**Clarity note**: The wording could be more precise. Current comment:
```
# No [metadata.satisfies] -- system package installed via native package managers, not a Homebrew formula
```

This clearly explains the reason (not a Homebrew formula), which is good. These three are correctly handled.

---

## Finding 4: Consistency with Registry Validation (CLEAR)

**Context**: The design doc Decision 3 mentions that `generate-registry.py` validates satisfies entries for:
- Correct format (lowercase alphanumeric)
- No duplicate claims across recipes
- No conflicts with canonical recipe names

The test plan scenario-1 confirms this is already implemented. Issue #1865 will trigger these validations on the new satisfies entries.

**Clarity for maintainers**: The recipes being updated all follow standard naming (kebab-case, lowercase), so they should pass validation automatically. No concern here.

---

## Finding 5: Recipes Already With Satisfies (CLEAR)

**Files**:
- `recipes/a/abseil.toml:7-8`
- `recipes/j/jpeg-turbo.toml:7-8`
- `recipes/l/libngtcp2.toml:8-9`
- `recipes/l/libnghttp2.toml:8-9`
- `recipes/s/sqlite.toml:9-10` (different tier: supports tool + library)

**Status**: These recipes already have satisfies metadata and don't need updating. The code structure is clear and consistent across all five. The pattern is well-established.

---

## Summary of Findings

| Finding | Severity | Action | Impact |
|---------|----------|--------|--------|
| Misleading comments claiming no satisfies needed | Blocking | Update comments to match new requirement or explain exemption rationale | Prevents future misunderstanding of satisfies policy |
| Pattern A (name-matches-name) could use explicit comment | Advisory | Add brief explanatory comment to clarify "why" | Minor improvement to code clarity |
| System package recipes correctly exempted | Clear | No action needed | Good |
| Registry validation consistency | Clear | No action needed | Good |
| Existing satisfies recipes | Clear | No action needed | Good |

---

## Detailed Recommendation for #1865 Implementation

**Before merging, update the comments on the 13 recipes in Pattern A**:

**Pattern A recipes** (name matches formula):
- readline, expat, brotli, giflib, gmp, gettext, libxml2, libpng, libssh2, geos, libnghttp3, proj, zstd

Replace:
```toml
# No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name
```

With (choose one):

**Option 1 (conservative)**: Just remove the false claim and add the actual metadata:
```toml
[metadata.satisfies]
homebrew = ["gmp"]
```

**Option 2 (more explicit)**: Add a comment explaining the pattern:
```toml
[metadata.satisfies]
# Enables pipeline resolution of Homebrew gmp dependency to this recipe
homebrew = ["gmp"]
```

**Pattern B recipes and system packages** are already correctly handled and don't need changes beyond what the issue specifies.

---

## Overall Assessment

The core implementation is straightforward and correct. The blocking concern is the misleading comments that contradict the actual requirement. Once those comments are updated to either:
1. Reflect the new satisfies entries being added, or
2. Explain the rationale for recipes that don't have satisfies

...the implementation will be clear and maintainable. The next developer reading the code will understand that:
- Library recipes should have satisfies metadata (even when name matches formula)
- System-only packages (cuda-runtime, mesa, vulkan) have documented exemptions
- The pipeline uses satisfies for ecosystem name resolution

This is a solid first pass; the comment updates will make it production-ready.
