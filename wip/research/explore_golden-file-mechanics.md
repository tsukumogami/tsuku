# Golden File Testing Mechanics Research

## Summary

Golden file testing ensures deterministic, reproducible installation plans across all supported platforms. The system uses 415 golden plan files with three validation workflows.

## Golden File Structure

**Location:** `testdata/golden/plans/` organized by first letter
```
testdata/golden/plans/
├── a/age/v1.3.1-linux-amd64.json
├── f/fzf/v0.60.0-linux-amd64.json
└── ...
```

**Format:** JSON serialization of `InstallationPlan` (Plan Format Version 3)

**Coverage:** 415 files across multiple platform combinations

**Determinism:** Plans strip non-deterministic fields (`generated_at`, `recipe_source`) before comparison

## Platform Support

**Platforms with CI runners:**
- `linux-amd64` (ubuntu-latest)
- `darwin-amd64` (macos-14-intel, macos-15-intel)
- `darwin-arm64` (macos-latest)

**Platforms without CI runners (golden files only):**
- `linux-arm64`

**Linux family variants:**
- `linux-debian-amd64`
- `linux-rhel-amd64`
- `linux-arch-amd64`
- `linux-alpine-amd64`
- `linux-suse-amd64`

## Family-Aware Golden Files

**Family-agnostic recipes:** Single Linux file per version
- Used for: download, github_archive, go_install
- Example: `v0.60.0-linux-amd64.json`

**Family-aware recipes:** Five Linux files per version
- Used for: recipes with apt_install, dnf_install, etc.
- Example: `v1.0.0-linux-debian-amd64.json`, `v1.0.0-linux-rhel-amd64.json`

Detection: `tsuku info --metadata-only` reveals if recipe is family-aware

## Validation Workflows

### 1. validate-golden-recipes.yml (Recipe Changes)

- **Trigger:** Recipe files change
- **Action:** Validate golden files exist for all platforms
- **Validates:** Plan generation produces matching output

### 2. validate-golden-code.yml (Code Changes)

- **Trigger:** Plan generation code changes (35 key files)
- **Action:** Validate ALL 415 golden files
- **Validates:** Code changes don't affect plan outputs

### 3. validate-golden-execution.yml (Golden File Changes)

- **Trigger:** Golden files themselves change
- **Action:** Execute plans via `tsuku install --plan`
- **Validates:** Plans actually install successfully

## Validation Process

**Plan comparison (`validate-golden.sh`):**
1. Generate plan with `--pin-from` (constrained version resolution)
2. SHA256 hash comparison for fast matching
3. `diff` for detailed inspection on mismatch

**Execution (`validate-golden-execution.yml`):**
1. Route to appropriate runner (Linux per-recipe, macOS aggregated)
2. Run `tsuku install --plan <golden-file>`
3. Verify installation succeeds

## Key Technical Details

- **Plan Format Version 3:** Nested dependencies for self-contained plans
- **Determinism flag:** Indicates if entire plan is reproducible
- **URL/Checksum Storage:** Each ResolvedStep includes computed URL, sha256, size
- **Recipe Hash:** SHA256 of recipe content for cache invalidation
- **`--pin-from` flag:** Constrains version resolution to match golden files

## Execution Strategy

**Linux:** Per-recipe matrix (each recipe in separate job)
**macOS:** Aggregated (all recipes in one job to reduce runner pressure)

## Key Insight

The three workflows form a defense-in-depth strategy:
- Recipe changes -> validate that recipe's golden files
- Code changes -> validate ALL golden files
- Golden file changes -> actually execute the plans
