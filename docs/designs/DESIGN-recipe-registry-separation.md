---
status: Proposed
problem: All 171 recipes are embedded in the CLI binary, causing unnecessary bloat and coupling recipe updates to CLI releases.
decision: Separate recipes into critical (embedded in internal/recipe/recipes/) and community (fetched from recipes/ at repo root) based on directory location.
rationale: Location-based categorization is simpler than computed or explicit metadata. Plan-only PR testing with nightly execution for community recipes balances CI speed with regression detection.
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

Three-layer golden file validation:
1. **Recipe validation** (`validate-golden-recipes.yml`): Recipe changes trigger golden file checks
2. **Code validation** (`validate-golden-code.yml`): Plan generation code changes validate ALL golden files
3. **Execution validation** (`validate-golden-execution.yml`): Golden files execute on platform matrix

Exclusion mechanisms exist:
- `testdata/golden/exclusions.json`: Platform-specific exclusions (265 entries)
- `testdata/golden/execution-exclusions.json`: Recipe-level execution exclusions (10 entries)
- `testdata/golden/code-validation-exclusions.json`: Code bypass exclusions (7 entries)

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

#### Option 3A: Full Testing for Critical, Hash-Only for Community

Critical recipes get full golden file validation (plan generation + execution). Community recipes only verify their golden file hash matches (no regeneration check).

**Pros:**
- Much faster CI for community recipes
- Critical recipes still exhaustively tested
- Clear quality difference reflects criticality difference

**Cons:**
- Community recipe regressions harder to catch
- Hash-only testing misses functional issues
- Two different testing systems to maintain

#### Option 3B: Full Testing for All, Different Failure Handling

All recipes get full golden file testing, but:
- Critical failures block CI
- Community failures are warnings (non-blocking) or nightly-only

**Pros:**
- Same test infrastructure for all recipes
- Community issues still detected, just not blocking
- Simpler mental model

**Cons:**
- CI still slow for community recipes
- Warning fatigue risk
- Doesn't reduce CI time

#### Option 3C: Execution Testing for Critical, Plan-Only for Community (Enhanced)

Critical recipes: full plan generation + execution on platform matrix. Community recipes: plan generation only during PRs, with nightly full execution runs to catch issues.

**Pros:**
- Catches most issues without slow PR checks
- Significant CI time savings for contributors
- Still validates recipe syntax and plan structure
- Nightly runs catch download failures before users report

**Cons:**
- Issues discovered nightly, not immediately
- Platform-specific execution bugs may slip through for up to 24 hours
- Requires nightly workflow maintenance

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

| Driver | 3A (Hash-Only) | 3B (Warn-Only) | 3C (Plan-Only) |
|--------|----------------|----------------|----------------|
| CI efficiency | Good | Poor | Good |
| Regression detection | Poor | Good | Fair |
| Simplicity | Poor | Good | Good |

### Uncertainties

- **Binary size impact**: The 15-20 estimate needs validation. Implementation should measure baseline binary size and compare after separation.
- **Registry availability**: When GitHub is unavailable, what should happen?
  - Stale cache fallback (use previously fetched version)?
  - Hard failure with clear error message?
  - Graceful degradation showing which recipes are unavailable?
- **Version drift**: How do we handle community recipe updates that conflict with installed versions?
- **Cache invalidation**: How long should cached community recipes remain valid? Indefinite caching risks stale recipes; aggressive invalidation increases network dependency.

## Decision Outcome

**Chosen: Location-based categorization + Plan-only testing with nightly runs**

### Summary

Recipe criticality is determined by directory location: `internal/recipe/recipes/` = critical (embedded), `recipes/` = community (fetched from registry). This eliminates the need for computed dependency analysis or explicit metadata flags. Testing uses plan-only validation for community recipes in PRs, with nightly full execution runs to catch issues.

### Rationale

**Location-based categorization chosen because:**
- **Maximum simplicity**: No metadata field to maintain, no computed analysis to debug
- **Explicit by action**: Moving a recipe file IS the act of changing its criticality
- **Clear contributor understanding**: Directory location is unambiguous
- **Aligns with existing loader priority**: The embed directive already uses directory paths

**Plan-only + nightly testing chosen because:**
- **CI efficiency**: Community recipe PRs run quickly (plan validation only)
- **Regression detection**: Nightly runs catch download failures and platform issues within 24 hours
- **Same infrastructure**: Uses existing golden file validation, just at different trigger points

**Alternatives rejected:**

- **1A (Explicit metadata)**: Adds manual maintenance burden with risk of forgetting transitive dependencies
- **1B/1C (Computed from dependencies)**: Complex build process, harder to predict results, requires fixing Dependencies() infrastructure gaps (#644)
- **3A (Hash-only)**: Catches zero functional issues - not acceptable for quality
- **3B (Warn-only)**: CI still slow; warning fatigue is a real problem

### Trade-offs Accepted

By choosing location-based categorization:
- Moving a recipe between categories requires moving files (accepted: this is an intentional friction)
- Critical recipe list isn't automatically updated when action dependencies change (accepted: critical recipes change rarely, manual review is appropriate)

By choosing plan-only + nightly testing:
- Community recipe issues may go undetected for up to 24 hours (accepted: faster contributor feedback is worth this tradeoff)
- Requires maintaining a nightly workflow (accepted: incremental complexity)

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
- `validate-golden-recipes.yml` - Validates changed recipes
- `validate-golden-code.yml` - Validates all when code changes
- `validate-golden-execution.yml` - Executes golden files

**New workflow structure:**

1. **Critical recipe changes** (internal/recipe/recipes/**):
   - Full plan validation
   - Full execution validation
   - Blocks merge on failure

2. **Community recipe changes** (recipes/**):
   - Plan validation only
   - Does NOT block on execution
   - Faster PR feedback

3. **Nightly community validation** (new):
   - Full execution of all community recipes
   - Reports failures via issue/notification
   - Doesn't block anything (already merged)

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

**Goal:** Differentiate testing based on recipe category.

**Steps:**
1. Update `validate-golden-recipes.yml` to detect recipe category from path
2. Update `validate-golden-execution.yml` to only run for critical recipes in PRs
3. Create `nightly-community-validation.yml` for full community testing
4. Update exclusions.json schema if needed

**Validation:** PRs with community recipes run faster. Nightly catches issues.

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
