# Issue #986 Summary

## Implementation

Created SonameIndex and ClassifyDependency for Tier 2 dependency validation.

### Files Created/Modified

| File | Change |
|------|--------|
| `internal/verify/index.go` | NEW: SonameIndex struct, BuildSonameIndex, Lookup, Contains |
| `internal/verify/classify.go` | NEW: DepCategory type, ClassifyDependency, ClassifyResult |
| `internal/verify/types.go` | ADD: ErrUnknownDependency constant (explicit value 11) |
| `internal/verify/index_test.go` | NEW: Comprehensive index tests |
| `internal/verify/classify_test.go` | NEW: Comprehensive classification tests |

### SonameIndex API

```go
// Build index from state
index := verify.BuildSonameIndex(state)

// Lookup a soname
recipe, version, found := index.Lookup("libssl.so.3")

// Check if soname exists
if index.Contains("libyaml-0.so.2") { ... }

// Get index size
count := index.Size()
```

### Classification API

```go
// Classify a dependency
category, recipe, version := verify.ClassifyDependency(
    "libssl.so.3",
    index,
    verify.DefaultRegistry,
    "linux",
)

// Or use result struct
result := verify.ClassifyDependencyResult(dep, index, registry, targetOS)
```

### Dependency Categories

| Category | Meaning |
|----------|---------|
| `DepPureSystem` | OS-provided (libc, libpthread) - verify accessible, skip recursion |
| `DepTsukuManaged` | Built/managed by tsuku - verify sonames, recurse |
| `DepExternallyManaged` | Tsuku recipe via pkg manager - verify sonames, skip recursion |
| `DepUnknown` | Unclassified - FAIL pre-GA |

### Key Design Decisions

1. **Classification Order**: Soname index FIRST, then system patterns, else UNKNOWN
2. **Index Priority**: If libz.so.1 is in index AND matches system pattern, index wins
3. **Error Constant**: ErrUnknownDependency = 11 (explicit value per design decision #2)

## Test Coverage

- 10 SonameIndex tests covering nil state, empty state, single/multiple libraries, multiple versions
- 12 ClassifyDependency tests covering all categories, nil index/registry, path variables
- 2 error category tests verifying explicit value and string output
