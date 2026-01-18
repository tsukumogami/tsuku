# Exclusion Mechanisms Research

## Summary

Tsuku uses three separate exclusion files to control different aspects of golden file testing. Each has a distinct purpose and schema.

## Three Exclusion Files

| File | Purpose | Scope |
|------|---------|-------|
| `exclusions.json` | Platform-specific golden file generation | Recipe + Platform |
| `execution-exclusions.json` | Execution validation skips | Recipe-wide |
| `code-validation-exclusions.json` | Code validation skips | Recipe-wide |

## 1. Platform-Specific Exclusions (`exclusions.json`)

**Purpose:** Controls whether golden files can be generated for a recipe/platform combo

**Schema:**
```json
{
  "exclusions": [
    {
      "recipe": "brotli",
      "platform": {
        "os": "linux",
        "arch": "amd64",
        "linux_family": "debian"
      },
      "issue": "https://github.com/tsukumogami/tsuku/issues/123",
      "reason": "library recipes lack meaningful verification"
    }
  ]
}
```

**Fields:**
- `recipe`: kebab-case recipe name
- `platform.os`: "linux" | "darwin"
- `platform.arch`: "amd64" | "arm64"
- `platform.linux_family`: optional (debian, rhel, arch, alpine, suse)
- `issue`: GitHub issue URL (required)
- `reason`: explanation

**Current entries:** ~50 (mainly libraries and toolchain-drift recipes)

## 2. Execution Exclusions (`execution-exclusions.json`)

**Purpose:** Excludes entire recipes from installation testing in CI

**Schema:**
```json
{
  "exclusions": [
    {
      "recipe": "amplify",
      "issue": "https://github.com/tsukumogami/tsuku/issues/456",
      "reason": "npm_install toolchain version drift"
    }
  ]
}
```

**Note:** No platform field - entire recipe is excluded

**Current entries:** 10 (npm, cargo, compile_from_source issues)

**Examples:** amplify, cdk, serve, netlify-cli, curl, git

## 3. Code Validation Exclusions (`code-validation-exclusions.json`)

**Purpose:** Excludes recipes from golden file content comparison (not execution)

**Schema:** Same as execution-exclusions.json

**Use case:** Stale golden files needing regeneration

**Current entries:** 7 (missing dependencies, needs regeneration)

**Examples:** curl, ninja, sqlite

## Workflow Integration

### validate-golden-execution.yml

```bash
# Loads both exclusion files
exclusions=$(cat testdata/golden/exclusions.json)
execution_exclusions=$(cat testdata/golden/execution-exclusions.json)

# Two filter functions
is_excluded() -> checks exclusions.json (platform-aware)
is_execution_excluded() -> checks execution-exclusions.json (recipe-wide)
```

### validate-golden-recipes.yml

```bash
# Recipe with no golden files is OK if it has exclusions
if [ ! -d "testdata/golden/plans/$letter/$recipe" ]; then
  exclusion_count=$(jq '...' exclusions.json)
  if exclusion_count > 0; skip; else error;
fi
```

### validate-golden-code.yml

```bash
# Skips recipes in code-validation-exclusions.json
validate-golden.sh checks exclusion before content comparison
```

## Validation Script (`validate-golden-exclusions.sh`)

- Validates JSON format
- Checks issue URLs match GitHub format
- With `--check-issues`: verifies linked issues are still OPEN
- Exit codes: 0=valid, 1=stale (closed issues), 2=invalid

## Issue Tracking Integration

Every exclusion MUST link to an open GitHub issue:
- URI format: `https://github.com/{owner}/{repo}/issues/{number}`
- Closed issues trigger CI failures (stale exclusions)
- Prevents orphaned exclusions

## Reason Categories

Standardized patterns:
- "library recipes lack meaningful verification"
- "system-required recipe - only validates presence of X"
- "configure auto-detects homebrew X but linker cannot find"
- "golden files not yet generated"
- "npm_install toolchain version drift"
- "stale golden file (missing dependencies, needs regeneration)"

## Key Insight

The three-tier exclusion system allows fine-grained control:
1. **Platform exclusions:** "Can't generate golden file for this platform"
2. **Execution exclusions:** "Can't reliably execute this recipe in CI"
3. **Code validation exclusions:** "Golden file is stale, skip comparison"

Each serves a different purpose in the testing pipeline.
