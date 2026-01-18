---
status: Proposed
problem: All 171 recipes are embedded in the CLI binary, causing unnecessary bloat and coupling recipe updates to CLI releases.
decision: Separate recipes into critical (embedded) and community (registry-fetched) based on directory location, with split golden file directories.
rationale: Location-based categorization is simplest. Split golden files let code changes validate only critical recipes; community recipes are fully tested on their own PRs plus nightly runs for drift detection.
---

# Recipe Registry Separation

## Status

**Proposed**

## Context and Problem Statement

Tsuku currently embeds all 171 recipes directly in the CLI binary via Go's `//go:embed` directive. This was a practical choice for early development: recipes are always available, installation works offline, and there's no registry infrastructure to maintain.

As tsuku matures, this approach creates problems:

1. **Binary bloat**: Every recipe adds to binary size, regardless of whether users need it
2. **Update coupling**: Recipe improvements require a new CLI release
3. **CI burden**: All recipes receive the same testing rigor, even rarely-used ones
4. **Maintenance friction**: Contributors must rebuild the CLI to test recipe changes

However, the CLI depends on certain recipes to function. Actions like `go_install`, `cargo_build`, and `homebrew` require tools (Go, Rust, patchelf) that tsuku itself must install. These "critical" recipes must remain embedded to ensure tsuku can bootstrap its own dependencies.

### Scope

**In scope:**
- Defining criteria for critical vs community recipes
- Moving community recipes to `recipes/` directory at repo root
- Updating the loader to fetch community recipes from registry
- Restructuring CI to differentiate testing levels
- Updating golden file testing strategy for community recipes

**Out of scope:**
- Version pinning or lockfile features
- Recipe signing or verification (future enhancement)
- Multiple registry support
- Recipe deprecation workflows

## Decision Drivers

- **Bootstrap reliability**: CLI must always install action dependencies without network access to recipe registry
- **Binary size**: Smaller binaries mean faster downloads and less disk usage
- **Recipe agility**: Community recipes should update without waiting for CLI releases
- **CI efficiency**: Critical recipes warrant exhaustive testing; community recipes need lighter validation
- **Backwards compatibility**: Existing workflows must continue working
- **Contributor experience**: Recipe development shouldn't require rebuilding the CLI

## Implementation Context

### Current Architecture

**Embedding mechanism** (`internal/recipe/embedded.go:13`):
```go
//go:embed recipes/*/*.toml
var embeddedRecipes embed.FS
```

**Loader priority chain** (`internal/recipe/loader.go:73-115`):
1. In-memory cache
2. Local recipes (`$TSUKU_HOME/recipes/`)
3. Embedded recipes
4. Registry (GitHub raw fetch, cached to `$TSUKU_HOME/registry/`)

The loader already supports fetching non-embedded recipes from the registry. The infrastructure exists; it just isn't used because all recipes are embedded.

### Critical Recipe Analysis

Actions that depend on tsuku-managed tools:

| Action | Required Tool |
|--------|--------------|
| `go_install`, `go_build` | go |
| `cargo_install`, `cargo_build` | rust |
| `npm_install`, `npm_exec` | nodejs |
| `pip_install`, `pipx_install` | python-standalone |
| `gem_install`, `gem_exec` | ruby |
| `cpan_install` | perl |
| `homebrew_relocate`, `meson_build` | patchelf (Linux) |
| `configure_make`, `cmake_build`, `meson_build` | make, zig, pkg-config |
| `cmake_build` | cmake |
| `meson_build` | meson, ninja |

Transitive dependencies (libraries):
- `ruby` depends on `libyaml`
- `curl` depends on `openssl`, `zlib`

**Note:** `nix-portable` is auto-bootstrapped by the CLI (not a recipe). The nix_install and nix_realize actions download it directly with hardcoded SHA256 checksums. This is a special case that doesn't follow normal recipe patterns.

**Important:** The current `Dependencies()` infrastructure has known gaps. For example, `homebrew` has a TODO (#644) noting that composite actions don't automatically aggregate primitive dependencies. The build-time analysis must account for these gaps.

Estimated critical recipes: 15-20 (language toolchains + build tools + their dependencies). This estimate needs validation via actual dependency graph analysis during implementation.

### Current Testing Architecture

Tsuku uses four complementary workflows that trigger under different conditions:

**Functional testing (`test-changed-recipes.yml`):**
- Triggers when: Recipe files change (`internal/recipe/recipes/**/*.toml`)
- Tests: Only the changed recipes (not all recipes)
- Actions: Installs recipes on Linux (per-recipe parallel) and macOS (aggregated)
- Filters: Skips library recipes, system dependencies, and execution-excluded recipes

**Plan validation (`validate-golden-recipes.yml`):**
- Triggers when: Recipe files change
- Tests: Only the changed recipes
- Actions: Regenerates plans with `--pin-from` and compares to golden files
- Validates: Recipe has golden files for all supported platforms (or exclusions)

**Code validation (`validate-golden-code.yml`):**
- Triggers when: Critical plan generation code changes (35 files)
- Tests: ALL recipes with golden files (full suite)
- Actions: Compares all golden files against regenerated plans
- Critical files include: `eval.go`, `plan_generator.go`, action decomposers, version providers, recipe parser

**Execution validation (`validate-golden-execution.yml`):**
- Triggers when: Golden plan files change (`testdata/golden/plans/**/*.json`)
- Tests: Changed golden files only
- Actions: Runs `tsuku install --plan <golden-file>` to verify installation

**Key insight:** Only code changes trigger full recipe validation. Recipe changes only test the changed recipe. This means a community recipe can break without CI catching it if the breakage comes from external factors (download URL changes, version drift).

**Three exclusion files:**
- `exclusions.json`: Platform-specific exclusions (~50 entries) - "can't generate golden file for this platform"
- `execution-exclusions.json`: Recipe-wide execution exclusions (10 entries) - "can't reliably execute in CI"
- `code-validation-exclusions.json`: Code validation exclusions (7 entries) - "golden file is stale"

## Considered Options

### Decision 1: Recipe Categorization Method

How do we identify which recipes are critical?

#### Option 1A: Explicit Metadata Flag

Add a `critical = true` field to recipe metadata:

```toml
[metadata]
name = "go"
critical = true
```

**Pros:**
- Simple, explicit, easy to understand
- Contributors can see and reason about criticality
- No magic or implicit behavior

**Cons:**
- Manual maintenance burden
- Risk of forgetting to mark a transitive dependency as critical
- Doesn't automatically update when action dependencies change

#### Option 1B: Computed from Action Dependencies

Build-time script analyzes action implementations, extracts `Dependencies()` returns, and computes transitive closure automatically.

**Pros:**
- Always accurate: derived from actual code
- No manual maintenance
- Updates automatically when action dependencies change

**Cons:**
- More complex build process
- Requires parsing Go code or maintaining a separate registry
- Harder for contributors to predict what's critical

#### Option 1C: Hybrid Approach

Compute the set automatically, but allow explicit overrides via metadata:
- `critical = true` forces a recipe to be critical
- `critical = false` forces a recipe to be community (override computed status)

**Pros:**
- Best of both approaches
- Automatic detection with escape hatch
- Can handle edge cases the automation misses

**Cons:**
- More complex to understand
- Two sources of truth (computed + overrides)

### Decision 2: Directory Structure

Where do critical and community recipes live?

#### Option 2A: Current Location Split

- Critical: `internal/recipe/recipes/` (unchanged, embedded)
- Community: `recipes/` at repo root (new, not embedded)

**Pros:**
- Minimal changes to existing embed directive
- Clear separation in directory structure
- `recipes/` matches monorepo documentation

**Cons:**
- Moving a recipe from community to critical requires moving files
- Creates a boundary that needs clear criteria for when recipes should move

#### Option 2B: Single Directory with Build Filter

All recipes in `recipes/`. Build process filters which to embed based on computed criticality.

**Pros:**
- Single location for all recipes
- No file moves when criticality changes
- Simpler directory structure

**Cons:**
- More complex build process
- Harder to see at a glance what's embedded
- Embed directive can't use dynamic filtering directly

### Decision 3: Testing Strategy

How should testing differ between critical and community recipes?

**Current behavior reminder:**
- Recipe changes: Only that recipe is tested (plan + execution)
- Code changes (35 files): ALL recipes' golden files are validated (plan comparison only)
- Golden file changes: Changed files are executed

The question is: what should change when we split recipes into categories?

#### Option 3A: Broader Triggers for Critical, Unchanged for Community

Critical recipes get tested whenever ANY critical recipe OR critical code changes. Community recipes keep current behavior (only tested when that recipe changes).

**Pros:**
- Critical recipes are always validated together as a unit
- Community recipe PRs stay fast
- No change to community recipe testing on their own PRs

**Cons:**
- Defining "critical code" vs "community code" adds complexity
- Doesn't address community recipe drift (broken by external factors)

#### Option 3B: Current Triggers, Split Execution Scope

Keep current triggers, but when code changes occur:
- Critical recipes: Full execution validation (plan + install)
- Community recipes: Plan validation only (no execution)

**Pros:**
- Same trigger logic, just different execution scope
- Critical recipes always fully validated
- Community recipes still get plan validation on code changes

**Cons:**
- Community recipes may have broken downloads undetected
- Still runs plan validation for all 150+ community recipes on code changes

#### Option 3C: Split Golden Files with Nightly Community Execution

Split golden file directories by category. On code changes, only validate critical recipes. Community recipes validated only when changed, with nightly full execution run.

**Pros:**
- Code changes run much faster (15-20 critical vs 150+ community)
- Community recipe changes still tested (plan + execution)
- Nightly catches external breakage within 24 hours

**Cons:**
- Community recipe breakage from code changes not caught until nightly
- Requires splitting golden file directory structure
- More complex CI workflow logic

### Evaluation Against Decision Drivers

| Driver | 1A (Explicit) | 1B (Computed) | 1C (Hybrid) |
|--------|--------------|--------------|-------------|
| Bootstrap reliability | Fair | Good | Good |
| Maintenance burden | Poor | Good | Good |
| Contributor clarity | Good | Poor | Fair |

| Driver | 2A (Split Dirs) | 2B (Build Filter) |
|--------|-----------------|-------------------|
| Contributor clarity | Good | Fair |
| Build complexity | Good | Poor |
| Migration ease | Good | Fair |

| Driver | 3A (Broader Triggers) | 3B (Split Execution) | 3C (Split Golden + Nightly) |
|--------|----------------------|---------------------|------------------------------|
| CI efficiency | Fair | Fair | Good |
| Regression detection | Good (critical) / Poor (community) | Good (critical) / Fair (community) | Good (critical) / Fair (community) |
| Simplicity | Poor | Good | Fair |

### Uncertainties

- **Binary size impact**: The 15-20 estimate needs validation. Implementation should measure baseline binary size and compare after separation.
- **Registry availability**: When GitHub is unavailable, what should happen?
  - Stale cache fallback (use previously fetched version)?
  - Hard failure with clear error message?
  - Graceful degradation showing which recipes are unavailable?
- **Version drift**: How do we handle community recipe updates that conflict with installed versions?
- **Cache invalidation**: How long should cached community recipes remain valid? Indefinite caching risks stale recipes; aggressive invalidation increases network dependency.

## Decision Outcome

**Chosen: Location-based categorization + Split golden files with nightly community execution (3C)**

### Summary

Recipe criticality is determined by directory location: `internal/recipe/recipes/` = critical (embedded), `recipes/` = community (fetched from registry). Golden files are split into `critical/` and `community/` subdirectories. Code changes only validate critical recipes. Community recipes are fully tested when changed, with nightly runs catching external breakage.

### Rationale

**Location-based categorization chosen because:**
- **Maximum simplicity**: No metadata field to maintain, no computed analysis to debug
- **Explicit by action**: Moving a recipe file IS the act of changing its criticality
- **Clear contributor understanding**: Directory location is unambiguous
- **Aligns with existing loader priority**: The embed directive already uses directory paths

**Split golden files + nightly testing (3C) chosen because:**
- **CI efficiency**: Code changes only validate 15-20 critical recipes instead of 170+
- **Community recipes still fully tested when changed**: Plan validation AND execution on that recipe's PR
- **Nightly catches external drift**: Download URL changes, version drift detected within 24 hours
- **Clear trigger logic**: Critical = always validated on code changes; Community = validated on their own changes + nightly

**Alternatives rejected:**

- **1A (Explicit metadata)**: Adds manual maintenance burden with risk of forgetting transitive dependencies
- **1B/1C (Computed from dependencies)**: Complex build process, harder to predict results, requires fixing Dependencies() infrastructure gaps (#644)
- **3A (Broader triggers)**: Doesn't address community recipe drift; adds complexity defining "critical code"
- **3B (Split execution)**: Still validates all 150+ community recipes on code changes, slow CI

### Trade-offs Accepted

By choosing location-based categorization:
- Moving a recipe between categories requires moving files (accepted: this is an intentional friction)
- Critical recipe list isn't automatically updated when action dependencies change (accepted: critical recipes change rarely, manual review is appropriate)

By choosing split golden files + nightly:
- Community recipe breakage from code changes not caught until nightly (accepted: code changes rarely break community recipes, and nightly catches it)
- Community recipe issues from external factors (URL changes) may go undetected for up to 24 hours (accepted: faster contributor feedback is worth this tradeoff)
- Requires splitting golden file directories and updating workflow triggers (accepted: one-time migration cost)

## Solution Architecture

### Overview

The solution separates recipes into two directory locations:
- **Critical recipes**: `internal/recipe/recipes/` (embedded via `//go:embed`)
- **Community recipes**: `recipes/` at repo root (fetched from GitHub registry)

The loader's existing priority chain handles this naturally. No code changes are needed for the basic fetch mechanism - the separation is purely organizational.

### Directory Structure

```
tsuku/
├── internal/
│   └── recipe/
│       ├── embedded.go          # //go:embed recipes/*/*.toml
│       └── recipes/             # Critical recipes (15-20)
│           ├── g/go.toml
│           ├── r/rust.toml
│           ├── n/nodejs.toml
│           └── ...
├── recipes/                     # Community recipes (~150)
│   ├── a/actionlint.toml
│   ├── f/fzf.toml
│   └── ...
└── testdata/
    └── golden/
        ├── plans/
        │   ├── critical/        # Golden files for critical recipes
        │   └── community/       # Golden files for community recipes
        └── exclusions.json      # Updated with category awareness
```

### Loader Behavior

The existing loader (`internal/recipe/loader.go`) already supports this flow:

```
User requests recipe "fzf"
    ↓
1. Check in-memory cache → miss
    ↓
2. Check local recipes ($TSUKU_HOME/recipes/fzf.toml) → miss
    ↓
3. Check embedded recipes (internal/recipe/recipes/) → miss (fzf is community)
    ↓
4. Fetch from registry (GitHub raw URL) → found
    ↓
5. Cache to $TSUKU_HOME/registry/f/fzf.toml
    ↓
6. Return recipe
```

For critical recipes like "go":
```
User requests recipe "go"
    ↓
1-2. Cache/local checks → miss
    ↓
3. Check embedded recipes → found (go is critical)
    ↓
4. Return recipe (no network needed)
```

### Registry URL Structure

Community recipes are fetched from GitHub raw URLs:
```
https://raw.githubusercontent.com/tsukumogami/tsuku/main/recipes/{letter}/{name}.toml
```

This URL pattern needs updating from the current:
```
https://raw.githubusercontent.com/tsukumogami/tsuku/main/internal/recipe/recipes/{letter}/{name}.toml
```

### Golden File Organization

Golden files mirror the recipe structure:
```
testdata/golden/plans/
├── critical/           # Full validation (plan + execution)
│   ├── g/go/
│   ├── r/rust/
│   └── ...
└── community/          # Plan-only validation (nightly execution)
    ├── a/actionlint/
    ├── f/fzf/
    └── ...
```

### CI Workflow Changes

**Current workflows:**
| Workflow | Trigger | Scope |
|----------|---------|-------|
| `test-changed-recipes.yml` | Recipe files change | Changed recipes only |
| `validate-golden-recipes.yml` | Recipe files change | Changed recipes only |
| `validate-golden-code.yml` | 35 critical code files change | ALL recipes |
| `validate-golden-execution.yml` | Golden files change | Changed golden files |

**Changes needed:**

1. **test-changed-recipes.yml** - Update path triggers:
   - Currently: `internal/recipe/recipes/**/*.toml`
   - Add: `recipes/**/*.toml` (community recipes)
   - Behavior unchanged: tests changed recipes on their PRs

2. **validate-golden-recipes.yml** - Update path triggers:
   - Currently: `internal/recipe/recipes/**/*.toml`
   - Add: `recipes/**/*.toml`
   - Behavior unchanged: validates changed recipes have golden files

3. **validate-golden-code.yml** - Scope reduction (key change):
   - Currently: Validates ALL golden files when code changes
   - Change to: Only validate `testdata/golden/plans/critical/**`
   - Rationale: Code changes rarely break community recipes; nightly catches any drift

4. **validate-golden-execution.yml** - No change needed:
   - Already only executes changed golden files
   - Will naturally work with split directory structure

5. **nightly-community-validation.yml** (new):
   - Cron: Daily at 2 AM UTC
   - Scope: All community recipes (`testdata/golden/plans/community/**`)
   - Actions: Full plan validation + execution
   - Reporting: Creates GitHub issue on failure

**Testing behavior by scenario:**

| Scenario | Critical Recipes | Community Recipes |
|----------|------------------|-------------------|
| Recipe file changes | Plan + Execution | Plan + Execution |
| Critical code changes (35 files) | Plan validation | Not tested (wait for nightly) |
| Golden file changes | Execution | Execution |
| Nightly run | Not included | Full validation + Execution |

**Note:** Community recipes are still FULLY tested when they change. The only difference is they're excluded from the "code changes" trigger, which catches potential side effects from plan generation code changes.

## Implementation Approach

### Stage 1: Recipe Migration

**Goal:** Move community recipes to `recipes/` directory.

**Steps:**
1. Identify critical recipes (action dependencies + transitive deps)
2. Move all other recipes from `internal/recipe/recipes/` to `recipes/`
3. Update embed directive if needed (should work unchanged)
4. Update registry URL in `internal/registry/registry.go`

**Validation:** All existing tests pass. `tsuku install <community-recipe>` works via registry fetch.

### Stage 2: Golden File Reorganization

**Goal:** Separate golden files by category.

**Steps:**
1. Create `testdata/golden/plans/critical/` and `testdata/golden/plans/community/`
2. Move golden files to appropriate directories
3. Update regeneration scripts to use new paths
4. Update validation scripts to use new paths

**Validation:** Golden file scripts work with new structure.

### Stage 3: CI Workflow Updates

**Goal:** Adjust workflow triggers and scope for the split structure.

**Steps:**
1. **test-changed-recipes.yml**: Add `recipes/**/*.toml` to path triggers (alongside existing `internal/recipe/recipes/**/*.toml`)

2. **validate-golden-recipes.yml**: Add `recipes/**/*.toml` to path triggers. Update script to detect recipe category from path and look in appropriate golden file directory.

3. **validate-golden-code.yml**: Change scope from all golden files to `testdata/golden/plans/critical/**` only. This is the key optimization - code changes no longer validate 150+ community recipes.

4. **validate-golden-execution.yml**: Update to handle both `critical/` and `community/` subdirectories in golden file detection.

5. **Create nightly-community-validation.yml**:
   - Cron trigger: `0 2 * * *`
   - Runs `validate-all-golden.sh` for `testdata/golden/plans/community/`
   - Executes all community golden files
   - Creates GitHub issue on failure with list of broken recipes

6. Update exclusions.json: Add `category` field or split into `critical-exclusions.json` and `community-exclusions.json`

**Validation:**
- Code change PRs complete faster (only critical recipes)
- Community recipe change PRs still run full validation
- Nightly workflow runs and reports failures

### Stage 4: Cache Policy Implementation

**Goal:** Implement cache TTL and invalidation.

**Steps:**
1. Add fetch timestamp metadata alongside cached recipes
2. Implement 24-hour default TTL (configurable via `TSUKU_RECIPE_CACHE_TTL`)
3. Add `tsuku update-registry` command to force refresh all cached recipes
4. Add `--force` flag to `tsuku install` to bypass cache

**Validation:** Cache expires after TTL, fresh recipes fetched.

### Stage 5: Documentation and Migration Guide

**Goal:** Document the new structure for contributors.

**Steps:**
1. Update CONTRIBUTING.md with recipe category guidance
2. Add authoritative list of critical recipes with dependency rationale
3. Document the nightly validation process and failure notification channels
4. Update troubleshooting for "recipe not found" errors (network issues)
5. Create incident response playbook for repository compromise

## Security Considerations

### Download Verification

**Critical recipes** (embedded): Binary signature verification for downloaded artifacts remains unchanged. These recipes undergo full execution testing in CI.

**Community recipes** (fetched): Recipe files themselves are fetched over HTTPS from GitHub. No additional signing is implemented in this design. The fetched recipe content is subject to GitHub's repository integrity guarantees.

**Future enhancement**: Recipe signing could add an integrity layer for community recipes, verifying that fetched TOML matches a signed manifest.

### Execution Isolation

No change. All recipe steps execute with the same isolation model regardless of source. Users already trust recipe content when running `tsuku install`.

### Supply Chain Risks

**Embedded recipes**: Reviewed at PR time, compiled into binary. Attack surface is the PR review process. Changes are visible in git history and require PR approval.

**Community recipes**: Fetched at runtime from GitHub. Attack surface expands to:
- GitHub account compromise
- Repository compromise
- Network MITM (mitigated by HTTPS)

**Mitigations:**
- The loader caches fetched recipes. Once cached, the same content is used until cache expires or user clears it.
- Users can pin specific recipe versions via local overrides in `$TSUKU_HOME/recipes/`.
- All recipe changes go through the same PR review process before reaching main branch.

**Cache poisoning risk**: If a cached recipe is malicious, it persists until cache invalidation. Stage 4 addresses this with:
- 24-hour default cache TTL with periodic refresh checks
- `tsuku update-registry` command to refresh all cached recipes
- `tsuku install --force` to bypass cache for specific recipe

**Account compromise recovery**: If the GitHub repository is compromised:
- Embedded recipes in released binaries are unaffected
- Community recipes could be replaced with malicious versions
- Recovery requires: reverting malicious commits, notifying users to clear cache, potential emergency CLI release if critical recipes affected

### User Data Exposure

No change. This design doesn't affect what data tsuku collects or transmits.

### Security Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious community recipe | PR review, GitHub HTTPS, cache persistence | Compromised GitHub account could push malicious recipe |
| Cache poisoning | Cache-until-clear semantics, local override option | Stale malicious cache persists until explicit clear |
| Network unavailable | Critical recipes embedded, community cached | First-time installs of community recipes fail offline |
| Download tampering | HTTPS to GitHub, binary checksums in recipes | Recipe file itself has no signature |

## Consequences

### Positive

- Smaller CLI binary (estimated 30-50% recipe content reduction)
- Recipe updates ship independently of CLI releases
- CI runs faster for community recipe changes
- Clear mental model: "critical = CLI needs it to work"

### Negative

- Two categories to understand instead of one
- Community recipes may be unavailable during network issues
- Additional infrastructure complexity (registry cache management)
- Split testing strategy is more complex

### Neutral

- Migration requires moving files and updating embed directive
- Documentation needs updating to explain the distinction
- Contributors need to understand when a recipe should be critical
