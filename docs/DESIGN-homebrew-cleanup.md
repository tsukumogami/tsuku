# Design: Homebrew and Legacy Action Cleanup

**Status**: Proposed

## Context and Problem Statement

Research into Homebrew's actual behavior revealed that our design for Homebrew source builds was based on incorrect assumptions. Key findings:

1. **99.94% of Homebrew formulas have bottles** (8,096 out of 8,101)
2. **The 5 formulas without bottles** are internal bootstrapping formulas (`portable-libffi`, `portable-libxcrypt`, `portable-libyaml`, `portable-openssl`, `portable-zlib`) that users should never install
3. **Homebrew's default behavior** no longer falls back to source builds - it fails with "no bottle available!" and requires explicit `--build-from-source`
4. **The "source fallback" scenario is rare and transient** - mainly affects `brew upgrade` immediately after a version bump, before bottles are built

This means:
- The `homebrew_source` action provides no practical value for production recipes
- The HomebrewBuilder's Phase 2 (source builds) is unnecessary
- The distinction between `homebrew_bottle` and `homebrew_source` is confusing when bottles are the only viable option

Additionally, the codebase has accumulated legacy actions from early experimentation:
- `hashicorp_release`: A convenience wrapper that should be explicit primitive actions
- Source build test fixtures in the registry that should be in testdata/

### Why Now

1. **Avoid confusion**: New contributors may think `homebrew_source` is a valid production choice
2. **Reduce maintenance**: The source build code in HomebrewBuilder is complex and unused
3. **Cleaner API**: `homebrew` is clearer than `homebrew_bottle` when there's only one option
4. **Test hygiene**: Test fixtures don't belong in the production registry

### Scope

**In scope:**
- Deprecate and remove `homebrew_source` action
- Rename `homebrew_bottle` action to `homebrew`
- Remove source build code from HomebrewBuilder
- Replace `hashicorp_release` with explicit primitive actions in recipes
- Move source build test fixtures from registry to testdata/
- Update documentation and design docs
- Close/update related GitHub issues

**Out of scope:**
- Changes to bottle-based functionality (working correctly)
- Changes to build actions (`configure_make`, `cmake_build`, etc.) - these remain valuable for non-Homebrew use cases
- The `meson_build` action (Issue #521) - should be re-scoped for non-Homebrew use cases

## Decision Drivers

- **Simplicity**: Fewer actions and clearer naming reduces cognitive load
- **Accuracy**: Code should reflect reality (bottles are the only practical option)
- **Maintainability**: Less unused code means less maintenance burden
- **Test isolation**: Test fixtures should not pollute the production registry
- **Pre-GA freedom**: tsuku has no users yet, so breaking changes have no migration cost

## Considered Options

### Decision 1: homebrew_source Action

#### Option 1A: Keep homebrew_source

Keep the action for potential future use or edge cases.

**Pros:**
- No breaking changes
- Available if edge cases emerge

**Cons:**
- Misleading - suggests source builds are a viable production option
- Maintenance burden for unused code
- Confuses the Homebrew story

#### Option 1B: Deprecate and Remove homebrew_source

Remove the action entirely since it provides no practical value.

**Pros:**
- Cleaner codebase
- Clear message that bottles are the only option
- No maintenance burden

**Cons:**
- Breaking change for any recipes using it (only test fixtures)
- Removes option for hypothetical edge cases

### Decision 2: homebrew_bottle Naming

#### Option 2A: Keep homebrew_bottle Name

Maintain the current `homebrew_bottle` action name.

**Pros:**
- No breaking changes
- Explicit about what it does

**Cons:**
- Implies there's an alternative (homebrew_source) when there isn't
- Verbose when "bottle" is the only option

#### Option 2B: Rename to homebrew

Rename `homebrew_bottle` to `homebrew` since bottles are the only option.

**Pros:**
- Cleaner, shorter name
- No false implication of alternatives
- Matches user mental model ("install from Homebrew")

**Cons:**
- Would be a breaking change if tsuku had users (it doesn't - pre-GA)

### Decision 3: hashicorp_release Action

#### Option 3A: Keep hashicorp_release

Maintain the convenience wrapper action.

**Pros:**
- Shorter recipes for HashiCorp tools
- No migration needed

**Cons:**
- Hides what's actually happening
- Inconsistent with other recipes that use explicit primitives
- Another action to maintain

#### Option 3B: Replace with Explicit Primitives

Convert `hashicorp_release` recipes to use `download` + `extract` + `chmod` + `install_binaries`.

**Pros:**
- Transparent - users see exactly what happens
- Consistent with other recipes
- One less action to maintain
- Decomposable by default (already primitives)

**Cons:**
- Slightly longer recipes
- Migration work for 6 recipes

### Decision 4: Source Build Test Fixtures

#### Option 4A: Keep Test Fixtures in Registry

Keep bash.toml, python.toml, readline.toml in `internal/recipe/recipes/`.

**Pros:**
- No file moves needed

**Cons:**
- Pollutes production registry with non-functional recipes
- Confusing for users browsing available recipes
- These recipes can't actually be installed (they're source builds)

#### Option 4B: Move to testdata/

Move source build fixtures to `testdata/homebrew-source-fixtures/`.

**Pros:**
- Clear separation of test fixtures from production recipes
- Registry only contains installable recipes
- Test fixtures clearly labeled as such

**Cons:**
- Need to update test references
- File reorganization

## Decision Outcome

**Chosen: 1B + 2B + 3B + 4B**

### Summary

Remove all Homebrew source build infrastructure and rename `homebrew_bottle` to `homebrew`. Replace the `hashicorp_release` convenience action with explicit primitives. Move source build test fixtures out of the production registry into testdata.

### Rationale

The research conclusively showed that Homebrew source builds have no practical use case - 99.94% of formulas have bottles, and the remaining 5 are internal bootstrapping formulas. Keeping source build infrastructure creates confusion and maintenance burden for zero benefit.

Renaming `homebrew_bottle` to `homebrew` reflects reality: there's only one way to install from Homebrew, so the qualifier is unnecessary. Since tsuku is pre-GA with no external users, we can make this change directly without a deprecation period.

Replacing `hashicorp_release` with explicit primitives aligns with tsuku's transparency philosophy - users should understand what actions do. The 6 affected recipes are a small migration cost for a cleaner, more consistent codebase.

Moving test fixtures to testdata/ is standard practice - test artifacts shouldn't appear in production systems.

## Solution Architecture

### Component Changes

```
Before:                              After:

internal/actions/                   internal/actions/
├── homebrew_bottle.go         →   ├── homebrew.go (renamed)
├── homebrew_source.go         →   │   (deleted)
└── composites.go                  └── composites.go
    └── HashiCorpReleaseAction →       (hashicorp_release removed)

internal/builders/                  internal/builders/
└── homebrew.go                    └── homebrew.go
    ├── bottle code            →       ├── bottle code (kept)
    └── source code            →       └── (source code removed)

internal/recipe/recipes/            internal/recipe/recipes/
├── b/bash.toml               →    │   (moved to testdata)
├── p/python.toml             →    │   (moved to testdata)
├── r/readline.toml           →    │   (moved to testdata)
├── l/libyaml.toml                 ├── l/libyaml.toml (updated: homebrew_bottle → homebrew)
├── t/terraform.toml               ├── t/terraform.toml (updated: explicit primitives)
└── ...                            └── ...

testdata/                          testdata/
                              →    └── homebrew-source-fixtures/
                                       ├── bash.toml
                                       ├── python.toml
                                       └── readline.toml
```

### Rename Strategy

Since tsuku is pre-GA with no external users, we rename `homebrew_bottle` to `homebrew` directly:

1. Rename the action in code
2. Update all recipes to use the new name
3. Update all tests and documentation

No alias or deprecation period is needed.

### HashiCorp Recipe Migration

Current terraform.toml:
```toml
[[steps]]
action = "hashicorp_release"
product = "terraform"
binary = "terraform"
```

Migrated terraform.toml:
```toml
[[steps]]
action = "download"
url = "https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip"

[[steps]]
action = "extract"
format = "zip"

[[steps]]
action = "chmod"
files = ["terraform"]

[[steps]]
action = "install_binaries"
binaries = ["terraform"]
```

## Implementation Approach

Each phase is self-contained - tsuku should build and pass tests after each phase is complete.

### Phase 1: Rename homebrew_bottle to homebrew

1. Rename `homebrew_bottle.go` → `homebrew.go`
2. Rename `HomebrewBottleAction` → `HomebrewAction`
3. Update action name from `"homebrew_bottle"` to `"homebrew"`
4. Update all code references to the action
5. Update all recipes using `homebrew_bottle` to use `homebrew`
6. Update all tests

### Phase 2: Migrate HashiCorp Recipes

Replace `hashicorp_release` action usage with explicit primitives in all 6 recipes:
- terraform.toml
- vault.toml
- nomad.toml
- packer.toml
- boundary.toml
- waypoint.toml

### Phase 3: Remove hashicorp_release Action

1. Remove `HashiCorpReleaseAction` from composites.go
2. Remove related tests
3. Update validator to reject `hashicorp_release`

### Phase 4: Move Source Build Test Fixtures

1. Create `testdata/homebrew-source-fixtures/` directory
2. Move test fixtures from registry:
   - `internal/recipe/recipes/b/bash.toml` → `testdata/homebrew-source-fixtures/bash.toml`
   - `internal/recipe/recipes/p/python.toml` → `testdata/homebrew-source-fixtures/python.toml`
   - `internal/recipe/recipes/r/readline.toml` → `testdata/homebrew-source-fixtures/readline.toml`
3. Update `llm-test-matrix.json` to reference new locations
4. Update any test code referencing these fixtures

### Phase 5: Remove homebrew_source Action

1. Remove `homebrew_source.go` and `homebrew_source_test.go`
2. Update validator to reject `homebrew_source`
3. Update action registry

### Phase 6: Remove HomebrewBuilder Source Build Code

Remove all source build code from `internal/builders/homebrew.go`:
- `buildFromSource()` method
- `generateSourceRecipe()` method
- `sourceRecipeData` struct
- `validateSourceRecipeData()` method
- `buildSourceToolDefs()` method
- `buildSourceSystemPrompt()` method
- `generateSourceRecipeOutput()` method
- `buildSourceSteps()` method
- `ToolFetchFormulaRuby` constant
- `ToolExtractSourceRecipe` constant
- Related helper methods and types

### Phase 7: Issue Cleanup

1. Close issue #521 (meson_build) with note to re-scope for non-Homebrew use cases
2. Update Homebrew Builder milestone to reflect reduced scope
3. Close any open issues related to source builds

### Phase 8: Consolidate Homebrew Design Documentation

Consolidate all Homebrew-related design docs into a single authoritative document:

1. Create new `docs/DESIGN-homebrew.md` describing the current state:
   - The `homebrew` action (bottles only)
   - The HomebrewBuilder (LLM-based bottle recipe generation)
   - How Homebrew integration works in tsuku

2. Delete superseded design docs:
   - `docs/DESIGN-homebrew-builder.md` (superseded by consolidated doc)
   - `docs/DESIGN-homebrew-cleanup.md` (this doc - work complete)

3. Update any cross-references in other design docs

## Security Considerations

### Download Verification

**No change**: The `homebrew` action (renamed from `homebrew_bottle`) continues to verify downloads via SHA256 checksums from GHCR manifest annotations. This is unchanged.

### Execution Isolation

**No change**: Recipe validation continues to run in containers with `--network=none`. The removal of source build code doesn't affect isolation.

### Supply Chain Risks

**Improved**: Removing source build capability eliminates a potential attack vector where malicious Ruby formula code could be analyzed by the LLM. With bottles only, we trust Homebrew's CI-built binaries exclusively.

### User Data Exposure

**No change**: The cleanup is internal refactoring. No new data is collected or exposed.

## Consequences

### Positive

1. **Cleaner codebase**: Removing ~1,500 lines of unused source build code
2. **Clearer API**: `homebrew` is simpler than `homebrew_bottle`
3. **Accurate mental model**: Code reflects reality (bottles only)
4. **Reduced maintenance**: Fewer actions and code paths to maintain
5. **Better test hygiene**: Test fixtures isolated from production registry
6. **Improved security posture**: No LLM analysis of Ruby formula code

### Negative

1. **Longer HashiCorp recipes**: Explicit primitives are more verbose
2. **Lost optionality**: If source builds ever become needed, code must be rewritten

### Risks

1. **Edge case discovery**: A legitimate source build use case might emerge
   - Mitigation: Build actions remain available; only Homebrew-specific source code is removed
   - Mitigation: The removed code is in git history if needed

## References

- Research findings: Homebrew API analysis showing 99.94% bottle coverage
- [Homebrew Bottles Documentation](https://docs.brew.sh/Bottles)
- [Homebrew Discussion #305](https://github.com/Homebrew/discussions/discussions/305) - Source fallback behavior change
- DESIGN-homebrew-builder.md - Original design being superseded
