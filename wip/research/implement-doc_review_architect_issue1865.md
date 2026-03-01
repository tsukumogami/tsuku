# Architecture Review: Issue #1865 (Backfill satisfies metadata)

**Issue**: #1865 - fix(recipes): backfill satisfies metadata on existing library recipes
**Review Focus**: Architect (design patterns, separation of concerns, structural fit)
**Status**: Scrutinized (pending CI)

## Overview

This review assesses whether the backfill of `[metadata.satisfies]` entries to 20 existing library recipes respects the codebase's architecture and patterns. The changes are purely data (recipe TOML files) with no code changes, so structural analysis focuses on consistency with:
- Recipe format conventions
- The satisfies metadata contract
- Dependency resolution patterns
- Registry validation expectations

## Findings

### 1. SATISFIES METADATA SEMANTICS (Advisory)

**Severity**: Advisory
**Category**: Design clarity
**Scope**: All modified recipes

The backfill adds `[metadata.satisfies]` entries mapping recipe names to their ecosystem package aliases. Three recipes get *new* satisfies entries (abseil, jpeg-turbo, libngtcp2), while 17 receive *explanatory comments* indicating why they don't need satisfies entries.

**Observation**: The distinction is clear and intentional:
- **abseil → abseil-cpp** (Homebrew names differ)
- **jpeg-turbo → libjpeg-turbo** (Homebrew names differ)
- **libngtcp2 → ngtcp2** (Homebrew names differ)
- **gmp, brotli, gettext, etc.** (Recipe name matches Homebrew formula name, so satisfies is redundant)

**Finding**: The comments are helpful for reviewers, but the semantics need to align with how the loader uses this data. Let me verify the intended behavior.

The loader's `buildSatisfiesIndex()` (internal/recipe/loader.go:370-418) scans all satisfies entries across all recipes and builds a `pkgName -> recipeName` index. This index is used for fallback lookup when a dependency is requested that doesn't match any recipe's canonical name.

**No architectural concern**: The comments clarify intent ("No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name") which documents the decision but doesn't introduce ambiguity in the index structure. When generate-registry.py runs, it validates satisfies entries and will catch any duplicates (line 329-345 of scripts/generate-registry.py).

### 2. RECIPE STRUCTURE CONSISTENCY (No Issues)

**Scope**: All modified recipes

All 20 recipes maintain the standard library recipe structure:
- `[metadata]` section with name, description, homepage, type = "library"
- `[metadata.satisfies]` when needed (3 recipes)
- Platform-conditional `[[steps]]` sections (homebrew for linux/darwin, apk_install for alpine)
- `[[steps]]` with install_binaries action declaring outputs

**No issues found**: The structure follows existing library recipe patterns (e.g., recipes/o/openssl.toml in embedded recipes already uses this format).

### 3. SATISFIES INDEX BUILDING AND LOOKUP (No Architectural Issues)

**Scope**: Data integration point

The modified recipes will be indexed by the loader's lazy-built satisfies index when recipes are loaded:

1. **Embedded recipes** (internal/recipe/recipes/*.toml): Already contain openssl with satisfies metadata
2. **Registry recipes** (recipes/*/*.toml): Will include the 20 backfilled recipes
3. **Index build**: Happens on first fallback lookup call (loader.go:424, sync.Once ensures single execution)

**How it works**:
- When `tsuku create` processes a dependency, it first tries a direct recipe lookup
- If that fails, it calls `lookupSatisfies()` which triggers index build
- The index maps package names (ecosystem-agnostic) → recipe names
- Example: "abseil-cpp" → "abseil", allowing the pipeline to resolve Homebrew deps to tsuku recipes

**Finding**: The satisfies metadata is correctly positioned in the MetadataSection (types.go:162) and is already validated by generate-registry.py. The backfill doesn't introduce any new lookup patterns or change how the index is built.

### 4. VALIDATION AND CI CONTRACT (No Blocking Issues)

**Scope**: Registry validation (scripts/generate-registry.py)

The generate-registry.py script (lines 186-225) validates satisfies metadata at CI time:
- Checks ecosystem names match pattern `^[a-z][a-z0-9-]*$`
- Validates package names match pattern `^[a-z0-9@.-]+$`
- Detects duplicate satisfies entries across recipes (lines 321-345)
- Detects conflicts with canonical recipe names (lines 340-345)

**Cross-recipe validation** (lines 321-345):
- Builds a `satisfies_claims` map tracking which recipe claims each package name
- Fails CI if two recipes claim the same package name
- Prevents conflicts with existing recipe canonical names

**Finding**: The three satisfies entries (abseil-cpp, libjpeg-turbo, ngtcp2) must pass this validation. No other recipe should already claim these names. This is enforced at CI time, so the implementation is sound.

### 5. DEPENDENCY RESOLUTION INTEGRATION (No Architectural Violations)

**Scope**: Pipeline use of satisfies metadata

The satisfies metadata is consumed by:
- **Recipe loader** (loader.go): Builds index for fallback lookup
- **Registry manifest** (registry/manifest.go): Includes satisfies in recipes.json
- **Pipeline discovery** (referenced in design doc): When batch orchestrator finds missing deps, the loader resolves them via satisfies

**How the pipeline uses it** (from design doc):
1. Batch orchestrator finds "openssl@3" dep needed
2. `loader.GetWithContext()` tries direct lookup → fails
3. Falls back to `lookupSatisfies("openssl@3")` → finds "openssl" recipe
4. Pipeline uses "openssl" recipe to resolve "openssl@3"

**Finding**: The backfill enables this resolution path for three new package name aliases. The code path is unchanged; only the data available for lookup expands.

### 6. COMMENTS AND DOCUMENTATION (No Issues)

**Scope**: Inline documentation in modified recipes

Recipes without satisfies entries include comments:
```toml
# No [metadata.satisfies] needed -- Homebrew formula name matches tsuku canonical name
```

This is helpful for future maintainers. It documents the *decision* not to add satisfies, which is more valuable than silence because it:
- Shows the decision was intentional (not forgotten)
- Explains the rule ("formula name matches canonical name")
- Guides future additions to this library

**Finding**: This comment pattern is lightweight and helpful. It does not introduce confusion because the generate-registry.py validator doesn't reference inline comments.

## Pattern Consistency Check

### 1. Library Recipe Pattern
✓ All recipes use `type = "library"` (matches design doc requirement)
✓ Platform-conditional steps (when clauses for glibc, musl, darwin)
✓ Install_binaries action with proper outputs list

### 2. Satisfies Entry Pattern
✓ Entries map `homebrew = ["package-name"]` (only homebrew ecosystem is used, consistent with discovery phase focus)
✓ Names use kebab-case (matching recipe naming convention)
✓ Entries are ecosystem-keyed (multipl ecosystems possible, though only homebrew used here)

### 3. Metadata Validation Contract
✓ All recipes have required metadata fields (name, description, homepage)
✓ Types are consistent (name: string, satisfies: map[string][]string)
✓ No schema drift from types.go MetadataSection

## Integration Points

### Code Flow (No architectural issues found)

1. **Recipe Loading**: `loader.Get()` loads recipes from embedded/registry → parses TOML → metadata.Satisfies populated
2. **Index Building**: `buildSatisfiesIndex()` scans satisfies entries from all recipes → lazy-loaded into `satisfiesIndex` map
3. **Dependency Resolution**: `lookupSatisfies(name)` queries index → returns canonical recipe name
4. **Registry Manifest**: `registry/manifest.go` includes satisfies in recipes.json output

The backfilled recipes integrate seamlessly because:
- No new code is needed
- The loader already handles satisfies metadata
- The index mechanism is already present
- Registry validation is already in place

## Summary

The backfill is architecturally sound and follows established patterns. The 20 library recipes are properly formatted, the three satisfies entries follow the existing mapping convention, and the explanatory comments for recipes without satisfies are helpful for maintainability. The implementation leverages the existing satisfies infrastructure without introducing new patterns or bypassing established contracts.

**Cross-architectural check**: No violations of dependency direction, no state contract changes, no parallel pattern introduction, no action dispatch bypasses.

### Architectural Fitness

- ✓ Uses existing satisfies metadata field (defined in MetadataSection)
- ✓ Follows library recipe structure (type = "library", platform-conditional steps)
- ✓ Respects registry validation contract (generate-registry.py will validate)
- ✓ Integrates with loader's lazy-built satisfies index
- ✓ No new code paths or special cases needed
- ✓ Data-only change (no code changes = minimal risk)

### Readiness for Merge

The implementation is ready for merge pending:
1. CI validation (generate-registry.py must pass)
2. No cross-recipe satisfies conflicts detected
3. Platform testing via test-recipe.yml workflow (designs specified this for library recipes)

The three satisfies entries should be cross-checked against the current recipe registry to ensure no name collisions, but this is CI-validated.
