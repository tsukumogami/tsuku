# Design: Selective Recipe Embedding

**Status**: Proposed
**Author**: Claude
**Created**: 2024-12-04

## Problem Statement

Currently, all 126 recipes in `internal/recipe/recipes/` are embedded into the tsuku binary via `go:embed`. This creates several issues:

1. **Binary bloat**: Every recipe adds to the binary size, even though most users only need a small subset of tools
2. **Tight coupling**: All recipes must be validated and released together with the CLI
3. **Unclear dependencies**: The CLI code depends on specific recipes (nodejs, python-standalone, rust, pipx, ruby) for bootstrap operations, but this dependency is implicit and not enforced
4. **Missing validation**: There's no compile-time or test-time guarantee that CLI-required recipes are present

### CLI-Required Recipes

The CLI code in `internal/install/bootstrap.go` and `internal/toolchain/toolchain.go` depends on these recipes:

| Recipe | Used By | Purpose |
|--------|---------|---------|
| `nodejs` | `EnsureNpm()`, npm toolchain | npm_install action bootstrap |
| `python-standalone` | `EnsurePython()` | pipx_install action bootstrap |
| `rust` | `EnsureCargo()`, crates.io toolchain | cargo_install action bootstrap |
| `pipx` | `EnsurePipx()`, pypi toolchain | pipx_install action (depends on python-standalone) |
| `ruby` | rubygems toolchain | gem_install action bootstrap |

Additionally, `go` and `golang` recipes are in the test matrix, suggesting they may be important for testing.

### Current State

- **126 recipes** embedded in binary
- **5-7 recipes** actually required by CLI code
- **~120 recipes** could be fetched from registry instead
- No validation ensures required recipes are present

## Research

### How Other Package Managers Handle This

**Homebrew**:
- Core formulae live in a separate tap (`homebrew-core`)
- Only the `brew` CLI is distributed; formulae are fetched on demand
- Built-in commands don't depend on specific formulae

**asdf**:
- Plugins are separate repositories
- Core CLI has no embedded tool definitions
- Everything is fetched on demand

**mise (formerly rtx)**:
- Tool definitions are fetched from registry
- CLI includes built-in support for common tools (node, python, etc.)
- Plugin system for extensibility

**proto**:
- Tool manifests are fetched from registry
- Core tools (node, npm, etc.) have built-in support
- WASM plugins for extensibility

### Key Insight

Most modern version managers separate:
1. **Core runtime dependencies** - embedded or built-in
2. **Tool catalog** - fetched from registry on demand

## Considered Options

### Option 1: Separate Embedded vs Registry Directories

Split recipes into two directories:
- `internal/recipe/core/` - CLI-required recipes (embedded)
- `recipes/` - All other recipes (registry only, not embedded)

**Pros**:
- Clear separation of concerns
- Smaller binary size
- CLI dependencies are explicit
- Easy to add compile-time validation

**Cons**:
- Two places to look for recipes
- Need to update registry URL or deployment workflow
- Migration effort required

### Option 2: Embed Manifest with Selective Loading

Keep all recipes in one directory but use a manifest to control embedding:
- `internal/recipe/core-recipes.txt` - list of recipes to embed
- Build process only embeds listed recipes

**Pros**:
- Single source of truth for recipes
- Flexible control over what's embedded
- Minimal code changes

**Cons**:
- More complex build process
- Requires custom embed logic (can't use simple `//go:embed`)
- Manifest could get out of sync

### Option 3: Tag-Based Embedding

Add metadata to recipes indicating if they should be embedded:
```toml
[metadata]
name = "nodejs"
embedded = true  # or core = true
```

**Pros**:
- Self-documenting
- No separate manifest needed
- Recipe is single source of truth

**Cons**:
- Requires parsing all recipes at build time
- Complex build process
- Can't use simple `//go:embed`

### Option 4: Hardcode Core Dependencies

Remove embedding entirely except for hardcoded core recipes:
- Hardcode the ~5 bootstrap recipes in Go code
- Everything else comes from registry

**Pros**:
- Simplest implementation
- Maximum binary size reduction
- Clear what's "special"

**Cons**:
- Core recipes can't be updated without CLI release
- Loses benefits of TOML-based configuration
- Harder to test

## Decision

**Chosen: Option 1 - Separate Embedded vs Registry Directories**

This provides the clearest separation and enables:
1. Compile-time validation that core recipes exist
2. Simple `//go:embed` usage
3. Clear mental model for contributors
4. Independent evolution of registry recipes

## Solution Architecture

### Directory Structure

```
tsuku/
├── internal/recipe/
│   ├── core/           # Embedded recipes (CLI dependencies)
│   │   ├── go.toml
│   │   ├── golang.toml
│   │   ├── nodejs.toml
│   │   ├── pipx.toml
│   │   ├── python-standalone.toml
│   │   ├── ruby.toml
│   │   └── rust.toml
│   ├── embedded.go     # Loads from core/
│   ├── loader.go
│   └── ...
├── recipes/            # Registry recipes (not embedded)
│   ├── a/
│   │   ├── actionlint.toml
│   │   └── ...
│   ├── b/
│   │   └── ...
│   └── ...
└── scripts/
    └── generate-registry.py  # Generates from recipes/
```

### Resolution Chain Update

Current: `local → embedded → registry`

New: `local → core (embedded) → registry`

The resolution chain stays the same conceptually, but "embedded" now only contains core recipes.

### Registry Changes

The `DefaultRegistryURL` currently points to:
```
https://raw.githubusercontent.com/tsuku-dev/tsuku/main/internal/recipe
```

This should change to:
```
https://raw.githubusercontent.com/tsuku-dev/tsuku/main/recipes
```

Or use GitHub Pages at `registry.tsuku.dev` which already deploys `recipes.json`.

### Validation

Add a test that verifies all CLI-required recipes are present in `core/`:

```go
// internal/recipe/core_test.go
func TestCoreRecipesExist(t *testing.T) {
    required := []string{
        "nodejs",
        "python-standalone",
        "rust",
        "pipx",
        "ruby",
        "go",      // for test matrix
        "golang",  // alias
    }

    for _, name := range required {
        if !embedded.Has(name) {
            t.Errorf("required core recipe %q is missing", name)
        }
    }
}
```

### Website Integration

The website at `tsuku.dev/recipes/` should show all recipes from both sources:
- Core recipes marked with a badge (e.g., "[core]" or "[built-in]")
- Registry recipes shown normally

The `recipes.json` generated by `generate-registry.py` should:
1. Include recipes from `recipes/` directory
2. Optionally include core recipes with a flag

### Future: Minimum CLI Version

For recipes that require specific CLI features, add optional metadata:

```toml
[metadata]
name = "some-tool"
min_cli_version = "0.5.0"  # Requires features from v0.5.0+
```

This is out of scope for this design but noted for future consideration.

## Implementation Plan

1. **Create `core/` directory** with CLI-required recipes
2. **Move remaining recipes** to `recipes/` (top-level)
3. **Update `embedded.go`** to load from `internal/recipe/core/`
4. **Update registry URL** to point to `recipes/`
5. **Update `generate-registry.py`** to read from `recipes/`
6. **Update deploy workflows** for new paths
7. **Add validation test** for core recipes
8. **Update CI/CD testing infrastructure** (see below)
9. **Update documentation**

## Testing Infrastructure Changes

The separation of core vs registry recipes enables differentiated testing strategies:

### Core Recipes: Comprehensive Testing

Core recipes are critical CLI dependencies and should be tested on every CI run:

**`.github/workflows/test.yml`** (existing, modify):
```yaml
# Add to existing test.yml - runs on every push/PR
integration-core:
  name: "Core: ${{ matrix.recipe.tool }} (${{ matrix.os }})"
  runs-on: ${{ matrix.os }}
  strategy:
    fail-fast: false
    matrix:
      os: [ubuntu-latest, macos-latest]
      recipe:
        - { tool: nodejs }
        - { tool: python-standalone }
        - { tool: rust }
        - { tool: pipx }
        - { tool: ruby }
        - { tool: go }
        - { tool: golang }
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version-file: 'go.mod'
    - run: go build -o tsuku ./cmd/tsuku
    - run: echo "$HOME/.tsuku/bin" >> $GITHUB_PATH
    - name: "Install ${{ matrix.recipe.tool }}"
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      run: ./tsuku install --force ${{ matrix.recipe.tool }}
```

This ensures:
- Core recipes are always functional
- Bootstrap operations work on all platforms
- Regressions are caught immediately

### Registry Recipes: Change-Based Testing

Registry recipes should only be tested when they change:

**`.github/workflows/test-changed-recipes.yml`** (existing, keep as-is):
- Triggers on `pull_request` when `recipes/**/*.toml` changes
- Uses matrix strategy to test each changed recipe individually
- Runs on both Linux and macOS

### Scheduled Comprehensive Testing

Add a scheduled workflow to catch upstream breakage (e.g., tool releases that break recipes):

**`.github/workflows/scheduled-recipe-tests.yml`** (new):
```yaml
name: Scheduled Recipe Tests

on:
  schedule:
    - cron: '0 6 * * 1'  # Weekly on Monday at 6 AM UTC
  workflow_dispatch:

jobs:
  # Test all core recipes
  test-core:
    name: "Core: ${{ matrix.recipe }}"
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        recipe: [nodejs, python-standalone, rust, pipx, ruby, go, golang]
    runs-on: ${{ matrix.os }}
    steps:
      # ... same as above

  # Test a rotating subset of registry recipes
  test-registry-sample:
    name: "Registry Sample: ${{ matrix.recipe }}"
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        recipe:
          # Popular/important recipes - rotate this list periodically
          - terraform
          - kubectl
          - k9s
          - trivy
          - lazygit
          # ... 10-20 high-value recipes
    runs-on: ${{ matrix.os }}
    steps:
      # ... same pattern
```

### Testing Summary

| Recipe Type | Trigger | Scope | Purpose |
|-------------|---------|-------|---------|
| Core | Every push/PR | All core recipes | Ensure CLI dependencies work |
| Registry | PR with changes | Changed recipes only | Validate recipe modifications |
| All | Weekly schedule | Core + sample registry | Catch upstream breakage |

### Path Filters Update

Update `.github/workflows/test.yml` path filters:

```yaml
# Detect if code files changed
- uses: dorny/paths-filter@v3
  id: filter
  with:
    filters: |
      code:
        - '**'
        - '!**/*.md'
        - '!docs/**'
        - '!recipes/**'        # Registry recipes don't trigger code tests
        - '!website/**'
      core_recipes:
        - 'internal/recipe/core/**/*.toml'  # Core recipe changes trigger full tests
```

## Security Considerations

### Supply Chain

- Core recipes are embedded, providing integrity guarantees
- Registry recipes are fetched over HTTPS from GitHub
- No change to existing security model

### Binary Integrity

- Smaller binary = smaller attack surface
- Core recipes are signed as part of the binary
- Registry recipes rely on HTTPS + GitHub's security

### Validation

- Core recipe validation at test time prevents accidental removal
- Registry recipes are validated by CI before merge

## Migration

### For Users

- No user-facing changes required
- CLI behavior remains the same
- Recipes that were embedded will now be fetched from registry (transparent)

### For Contributors

- Core recipes go in `internal/recipe/core/`
- Other recipes go in `recipes/`
- PR validation will catch misplacement

## Open Questions

1. Should we include the test matrix recipes (go, golang, etc.) in core?
2. Should `recipes.json` include core recipes or keep them separate?
3. Should we add `embedded_since` version metadata to core recipes?
