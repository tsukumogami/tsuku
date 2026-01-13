# Issue 708 Introspection

## Context Reviewed
- Design doc: none (issue has no milestone)
- Sibling issues reviewed: #709 (dynamic patch discovery - related, open)
- Prior patterns identified: Recipe modifications that intentionally use `pin` instead of dynamic `source`

## Current State Analysis

### Files Referenced in Issue

The issue identifies 4 recipes as "affected":
1. `testdata/recipes/bash-source.toml` - **Modified since issue creation**
2. `testdata/recipes/readline-source.toml` - **Modified since issue creation**
3. `testdata/recipes/python-source.toml` - **Modified since issue creation**
4. `testdata/recipes/sqlite-source.toml` - **Modified since issue creation**

### Critical Finding: Recipes Already Fixed

All 4 referenced recipes have been **intentionally modified** to use pinned versions:

| Recipe | Version Config | Reason Documented |
|--------|---------------|-------------------|
| bash-source | `pin = "5.3"` | "patches are version-specific" (#709 dependency) |
| readline-source | `pin = "8.3"` | "patches are version-specific" (#709 dependency) |
| python-source | `pin = "3.13.11"` | "patch is version-specific" (#709 dependency) |
| sqlite-source | `fossil_repo` | Uses fossil-based version resolution (not homebrew source) |

Each recipe now includes comments explaining:
- The dynamic version configuration is **commented out**
- The recipe uses `[version] pin = "X.Y.Z"` instead
- Resolution depends on #709 (dynamic patch discovery)

### Remaining Case: gdbm-source.toml

One recipe (`gdbm-source.toml`) still exhibits the pattern described in issue #708:
- Has `source = "homebrew"` with `formula = "gdbm"`
- Uses `download_file` with hardcoded URL containing `gdbm-1.26.tar.gz`

However, this recipe has **no patches**, so the inconsistency is straightforward to fix (replace `download_file` with `download` using `{version}` placeholder).

## Gap Analysis

### Minor Gaps
- Issue examples are now invalid (recipes have been fixed with pins)
- Issue should note that gdbm-source.toml is the cleaner example case
- Validation should distinguish between:
  - `source = X` (dynamic) + hardcoded URL = warning (original intent)
  - `pin = X` + hardcoded URL = acceptable (intentional)

### Moderate Gaps
- **Scope reduced**: 3 of 4 "affected" recipes are now intentionally pinned due to #709 dependency
- **Implementation target narrowed**: Only patched recipes without `pin` trigger this validation
- Issue text needs amendment to reflect current state

### Major Gaps
None. The core validation concept remains valid, just with narrower scope.

## Recommendation

**Amend**

The issue intent is still valid (detect inconsistent version config + hardcoded URLs), but the examples and affected recipes list is now outdated. The implementation should:

1. Only warn when `[version]` has a dynamic `source` (not `pin`)
2. Use `gdbm-source.toml` as the exemplar instead of bash/readline/python
3. Acknowledge that many "inconsistent" recipes are intentionally pinned pending #709

## Proposed Amendments

Add comment to GitHub issue with updated scope:

```markdown
## Update (Implementation Analysis)

The original "affected recipes" (bash-source, readline-source, python-source, sqlite-source)
have been modified to use `[version] pin = "X.Y.Z"` instead of dynamic version sources.
This was an intentional change because their patches are version-specific (see #709).

**Current exemplar**: `testdata/recipes/gdbm-source.toml` still exhibits this pattern
(homebrew source + hardcoded download_file URL).

**Implementation note**: The validation should only warn when:
1. `[version]` uses a dynamic source (homebrew, github, etc.) - NOT `pin`
2. Recipe uses `download_file` (not `download`)
3. The URL contains version-like patterns

Recipes using `[version] pin = "X.Y.Z"` with hardcoded URLs are intentionally consistent.
```
