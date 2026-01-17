# Architecture Analysis: Tier 2 Dependency Validation

**Date:** 2026-01-17
**Role:** Architecture Agent
**Verdict:** FEASIBLE with significant design choices

## Summary

The proposal is architecturally feasible. The existing codebase provides good foundations for cycle detection, header parsing, and recipe metadata. Key work involves adding soname → recipe mapping.

## Required Code Changes

### Core Modules

| File | Changes | Complexity |
|------|---------|------------|
| `internal/recipe/types.go` | Add `Provides`, `SystemRecipe` to MetadataSection | Low |
| `internal/recipe/loader.go` | Add `BuildDependencyMappingTable()` | Medium |
| `internal/verify/binary_deps.go` | **NEW**: Binary dep validation & soname mapping | High |
| `internal/verify/types.go` | Add `BinaryDepCheckResult` structs | Low |
| `cmd/tsuku/verify.go` | Extend `verifyLibrary()` with Tier 2 call | Medium |
| `internal/actions/resolver.go` | No changes needed (reuse cycle detection) | N/A |

## Soname → Recipe Mapping

### Recommended: Recipe Metadata

```toml
[metadata]
name = "openssl"
provides = ["libssl.so.3", "libcrypto.so.3"]
system_recipe = false
```

**Why metadata over auto-discovery:**
- Self-documenting in recipe TOML
- Versioned with recipe
- Enables CI validation
- Works with existing loader

### Implementation

```go
type DependencyMappingTable struct {
    SonameToRecipe map[string]string
}

func BuildMappingTable(loader *recipe.Loader) (*DependencyMappingTable, error) {
    table := &DependencyMappingTable{SonameToRecipe: make(map[string]string)}
    // Load all library recipes and populate from Provides field
    return table, nil
}
```

## Cycle Detection Integration

**Good news:** Existing `resolveTransitiveSet()` (resolver.go:350-446) is perfectly suitable.

The path-based tracking and `MaxTransitiveDepth = 10` can be reused:

```go
func resolveTransitiveBinary(
    ctx context.Context,
    loader RecipeLoader,
    mapping *DependencyMappingTable,
    binaryDeps []string,  // sonames from HeaderInfo.Dependencies
    path []string,        // same tracking as resolver.go
    depth int,
    visited map[string]bool,
) error {
    // Convert sonames to recipe names, then use existing cycle detection
}
```

## Key Wins

- Reuses existing cycle detection
- Header parsing already extracts `HeaderInfo.Dependencies`
- Recipe loader supports metadata extensions
- Clear `system_recipe` boundary

## Key Concerns

- **Maintenance burden**: Recipe `provides` must stay in sync with binaries
- **Cross-platform sonames**: libssl.so.3 (Linux) vs libssl.dylib (macOS)
- **Ambiguous ownership**: Can multiple recipes provide same soname?

## Estimated Effort

- Low complexity: Recipe metadata, simple mapping → 2-3 days
- Medium complexity: Validation logic, integration → 3-4 days
- High complexity: Version semantics, cross-platform → 2-3 days
- **Total: 1-2 weeks, ~1500-2000 lines including tests**
