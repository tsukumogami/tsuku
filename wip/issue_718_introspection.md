# Issue 718 Introspection

## Context Reviewed

- Design doc: `docs/DESIGN-golden-plan-testing.md`
- Sibling issues reviewed: #717 (recipe change validation), #719 (execution validation)
- Prior patterns identified:
  - PR #743 established workflow pattern in `validate-golden-recipes.yml`
  - Recipe change workflow uses fine-grained filtering (only validates recipes with changed `.toml` files)
  - Both #717 and #719 are now closed, establishing the CI workflow patterns

## User Question

The user asked: "please identify where there is opportunity to refactor and break up files for more fine tuned filtering to prevent validating every recipe for most PRs."

The concern is that the issue spec defines broad path triggers:
- `internal/executor/**`
- `internal/actions/**`
- `internal/recipe/**`
- `cmd/tsuku/eval.go`

Any change in these paths would trigger full golden file validation for ALL recipes.

## Gap Analysis

### Minor Gaps

1. **Path triggers are over-broad**: The issue spec uses wildcard paths that would trigger validation even for changes that don't affect plan generation.

2. **Prior pattern from #717 establishes fine-grained approach**: The recipe change workflow (PR #743) only validates recipes whose `.toml` files changed. The code change workflow should follow a similar principle of minimal blast radius.

### Moderate Gaps

**Opportunity for refined path filtering**: Analysis of the codebase shows clear separation of concerns:

| Category | Files | Affects Plan Generation |
|----------|-------|------------------------|
| **Plan generation** | `executor/plan_generator.go`, `executor/plan.go` | YES - always |
| **Decomposition** | `actions/decomposable.go`, `actions/composites.go`, `actions/download.go`, specific `*_install.go` files | YES - when action is used |
| **Execution only** | `executor/executor.go`, most action `Execute()` methods | NO |
| **Recipe parsing** | `recipe/loader.go`, `recipe/types.go` | YES - structure changes |
| **Recipe validation** | `recipe/validate.go`, `recipe/validator.go` | NO - post-parse only |
| **Recipe writing** | `recipe/writer.go` | NO - output only |
| **Version resolution** | `internal/version/*` | YES - affects resolved versions |

**Key insight**: Many files in `internal/actions/**` contain ONLY execution logic (the `Execute()` method) and NOT decomposition logic (the `Decompose()` method). Changes to execution logic don't affect plan generation and shouldn't trigger golden file validation.

### Files That Actually Affect Plan Generation

**High Impact - Core Plan Logic**:
- `cmd/tsuku/eval.go` - entry point
- `internal/executor/plan_generator.go` - main plan generation
- `internal/executor/plan.go` - plan types and validation

**Medium Impact - Action Decomposition**:
- `internal/actions/decomposable.go` - decomposition framework
- `internal/actions/composites.go` - download_archive, github_archive, github_file
- `internal/actions/download.go` - download action decomposition
- `internal/actions/homebrew.go` - homebrew decomposition (affects bottle recipes)
- `internal/actions/fossil_archive.go` - fossil decomposition

**Medium Impact - Ecosystem Decomposition** (affects specific recipe types):
- `internal/actions/cargo_install.go` (affects Rust recipes)
- `internal/actions/npm_install.go` (affects Node recipes)
- `internal/actions/pipx_install.go` (affects Python recipes)
- `internal/actions/gem_install.go` (affects Ruby recipes)
- `internal/actions/go_install.go` (affects Go recipes)
- `internal/actions/nix_install.go` (affects Nix recipes)
- `internal/actions/apply_patch.go` (affects patched recipes)

**Medium Impact - Recipe Parsing**:
- `internal/recipe/types.go` - recipe structure
- `internal/recipe/loader.go` - recipe loading
- `internal/recipe/platform.go` - platform detection

**Low/No Impact** (execution only, no plan generation):
- `internal/actions/extract.go` - only Execute(), no Decompose()
- `internal/actions/chmod.go` - only Execute()
- `internal/actions/install_binaries.go` - only Execute()
- `internal/actions/cargo_build.go` - ecosystem primitive, only Execute()
- `internal/actions/go_build.go` - ecosystem primitive, only Execute()
- `internal/actions/configure_make.go` - ecosystem primitive, only Execute()
- All `*_test.go` files
- `internal/recipe/validate.go` - validation only
- `internal/recipe/version_validator.go` - version validation only
- `internal/recipe/writer.go` - TOML serialization only

### Major Gaps

None identified. The issue spec is achievable with amendments.

## Recommendation

**Amend** - The issue should be amended to use more fine-grained path filtering.

## Proposed Amendments

### Amendment 1: Replace broad path triggers with precise file lists

Instead of:
```yaml
paths:
  - 'internal/executor/**'
  - 'internal/actions/**'
  - 'internal/recipe/**'
  - 'cmd/tsuku/eval.go'
```

Use a two-tier approach:

**Tier 1 - Core plan generation (always validate ALL golden files)**:
```yaml
paths:
  # Core plan generation - affects all plans
  - 'cmd/tsuku/eval.go'
  - 'internal/executor/plan_generator.go'
  - 'internal/executor/plan.go'
  - 'internal/executor/plan_conversion.go'

  # Decomposition framework - affects all decomposable actions
  - 'internal/actions/decomposable.go'
  - 'internal/actions/action.go'

  # Common composites - most recipes use these
  - 'internal/actions/composites.go'
  - 'internal/actions/download.go'

  # Recipe parsing - affects all recipes
  - 'internal/recipe/types.go'
  - 'internal/recipe/loader.go'
  - 'internal/recipe/platform.go'

  # Version resolution
  - 'internal/version/*.go'
```

**Tier 2 - Ecosystem-specific (future enhancement: validate only affected recipes)**:
```yaml
# These could potentially only validate recipes that use the specific action
# For now, include in full validation but consider future optimization
  - 'internal/actions/homebrew.go'        # Homebrew bottle recipes
  - 'internal/actions/cargo_install.go'   # Rust recipes
  - 'internal/actions/npm_install.go'     # Node recipes
  - 'internal/actions/pipx_install.go'    # Python recipes
  - 'internal/actions/gem_install.go'     # Ruby recipes
  - 'internal/actions/go_install.go'      # Go recipes
  - 'internal/actions/nix_install.go'     # Nix recipes
  - 'internal/actions/fossil_archive.go'  # Fossil recipes
  - 'internal/actions/apply_patch.go'     # Patched recipes
```

### Amendment 2: Add explicit exclusions for test files

```yaml
paths-ignore:
  - '**/*_test.go'
```

### Amendment 3: Future optimization note

Add to acceptance criteria:
- [ ] Document opportunity for Tier 2 optimization in the workflow file comments

The Tier 2 optimization would map ecosystem-specific action files to recipe types, allowing:
- Change to `cargo_install.go` -> only validate recipes with `cargo_install` action
- Change to `homebrew.go` -> only validate recipes with `homebrew` action

This optimization is non-blocking for MVP but could reduce CI time significantly.

## Files Analysis Summary

| Path Pattern | Files | Plan Impact | Validation Scope |
|--------------|-------|-------------|------------------|
| `internal/executor/*.go` | 5 non-test | 3 affect plans | Tier 1 (specific files only) |
| `internal/actions/*.go` | ~40 non-test | ~12 have Decompose | Tier 1 + Tier 2 split |
| `internal/recipe/*.go` | 9 non-test | 3 affect plans | Tier 1 (specific files only) |
| `cmd/tsuku/eval.go` | 1 | YES | Tier 1 |

This refines the original ~54 files (plus tests) down to ~18 core files that truly require full validation, with ~12 more that could potentially be optimized further.
