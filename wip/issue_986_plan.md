# Issue #986 Implementation Plan

## Goal
Implement SonameIndex and ClassifyDependency for Tier 2 dependency validation.

## Files to Create

### 1. `internal/verify/index.go`
```go
// SonameIndex provides O(1) reverse lookups from soname to recipe
type SonameIndex struct {
    SonameToRecipe  map[string]string // "libssl.so.3" -> "openssl"
    SonameToVersion map[string]string // "libssl.so.3" -> "3.2.1"
}

// BuildSonameIndex creates reverse index from installed libraries
func BuildSonameIndex(state *install.State) *SonameIndex
```

### 2. `internal/verify/classify.go`
```go
// DepCategory represents dependency classification
type DepCategory int

const (
    DepPureSystem DepCategory = iota
    DepTsukuManaged
    DepExternallyManaged
    DepUnknown
)

// ClassifyDependency determines category of a dependency soname
// Classification order: soname index FIRST, then system patterns, else UNKNOWN
func ClassifyDependency(dep string, index *SonameIndex, registry *SystemLibraryRegistry, targetOS string) (DepCategory, string, string)
```

### 3. `internal/verify/types.go` (modify)
Add error category:
```go
// ErrUnknownDependency indicates dependency could not be classified
// Pre-GA, this is an error to help identify corner cases
ErrUnknownDependency ErrorCategory = 11 // Explicit value per design decision #2
```

## Implementation Details

### SonameIndex
- Iterate `state.Libs` to populate reverse lookup maps
- Handle empty state (return empty but non-nil maps)
- Soname collisions: last version wins (document this)

### Classification Order (Critical)
1. Check soname index FIRST - ensures "libssl.so.3" is TSUKU when we have recipe
2. Check system patterns via `SystemLibraryRegistry.IsSystemLibrary()`
3. Return UNKNOWN - fails pre-GA to surface corner cases

### DepCategory
- Use iota for values (0, 1, 2, 3)
- Implement `String()` method for human-readable output

## Test Plan

### index_test.go
- Empty state -> empty index
- Single library with multiple sonames
- Multiple versions of same library
- Soname collision behavior

### classify_test.go
- Known soname in index -> DepTsukuManaged
- System library -> DepPureSystem
- Unknown -> DepUnknown
- Index priority (soname in index AND matches system pattern -> use index result)

## Validation Commands
```bash
go build ./...
go test -v ./internal/verify/... -run TestSonameIndex
go test -v ./internal/verify/... -run TestClassify
go test -v ./internal/verify/... -run TestDepCategory
```
