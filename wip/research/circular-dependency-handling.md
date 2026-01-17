# Circular Dependency Handling in tsuku

**Date:** 2026-01-17
**Purpose:** Document existing circular dependency detection for reuse in Tier 2 verification

## Summary

Tsuku has **two-level circular dependency protection**:
1. **Recipe-level**: Path-based cycle detection during dependency resolution
2. **Installation-level**: Visited map with special handling for already-installed tools

## Key Code Locations

### Level 1: Transitive Dependency Resolution

**File:** `internal/actions/resolver.go`

**Constants (lines 15-20):**
```go
MaxTransitiveDepth = 10  // Maximum recursion depth
ErrCyclicDependency      // Error for cycles
ErrMaxDepthExceeded      // Error for depth limit
```

**Cycle Detection Algorithm (`resolveTransitiveSet`, lines 343-446):**
- Uses `visited` map to track already-processed dependencies
- Maintains `path` slice for current resolution chain (for error reporting)
- Checks at multiple points:
  - Lines 373-378: Checks if depName is in ancestor path
  - Lines 410-420: Checks for transitive cycles

**Error format:**
```
cyclic dependency detected: A -> B -> C -> A
maximum dependency depth exceeded: exceeded depth 10 at path root -> D0 -> D1 -> ...
```

### Level 2: Installation-Level Detection

**File:** `cmd/tsuku/install_deps.go` (lines 148-217)

**Critical logic (Issue #732 fix):**
```go
// Check if already installed BEFORE checking for circular dependencies
// This prevents false positives when multiple tools share a dependency
if visited[toolName] {
    return fmt.Errorf("circular dependency detected: %s", toolName)
}
visited[toolName] = true
```

## Data Structures

### Recipe Dependencies (`internal/recipe/types.go:154-157`)
```go
Dependencies             []string  // Install-time dependencies
RuntimeDependencies      []string  // Runtime dependencies
ExtraDependencies        []string  // Additional install-time
ExtraRuntimeDependencies []string  // Additional runtime
```

### State Tracking (`internal/install/state.go`)

**Tool state (lines 76-93):**
```go
RequiredBy            []string  // Tools that depend on this tool
InstallDependencies   []string  // Install-time deps
RuntimeDependencies   []string  // Runtime deps
```

**Library state (lines 95-99):**
```go
UsedBy []string  // Tools using this library version
```

## Dependency Resolution Phases

1. **Direct Resolution** (`ResolveDependencies`, lines 56-179)
   - Extracts from recipe steps
   - Filters self-dependencies (lines 98-102)

2. **Transitive Expansion** (`ResolveTransitive`, lines 288-342)
   - Recursive expansion with cycle detection
   - Separate visited maps for install-time vs runtime

3. **Platform Filtering** (lines 181-205)
   - Filters by target OS

## Existing Commands

- `tsuku check-deps <recipe>` - Lists and validates all dependencies (direct + transitive)
  - Calls `ResolveTransitive()` for full expansion

## Tests

- `TestResolveTransitive_Cycle` (resolver_test.go:640-685)
- `TestResolveTransitive_SelfCycle` (resolver_test.go:687-713)
- `TestResolveTransitive_MaxDepthExceeded` (resolver_test.go:715-747)
- `TestDependencyResolution_SharedDependency` (dependency_test.go:101-184)

## Reuse Opportunities for Tier 2

1. **Path-based cycle detection**: Same pattern can track binary dependency resolution path
2. **Visited map**: Prevent redundant validation of shared dependencies
3. **Depth limiting**: Prevent runaway recursion in deep dependency trees
4. **Error formatting**: Include full path in cycle error messages
