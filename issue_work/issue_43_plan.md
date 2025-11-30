# Issue #43: Add npm Builder - Implementation Plan

## Overview
Implement NpmBuilder following the established pattern from CargoBuilder, GemBuilder, and PyPIBuilder.

## npm Registry API Analysis
- Endpoint: `https://registry.npmjs.org/<package>`
- The `bin` field is directly available in the package metadata
- Example: `"bin":{"prettier":"./bin/prettier.js"}`
- This is simpler than PyPI/Gem since no separate file fetch is needed

## Implementation Steps

### 1. Create `internal/builders/npm.go`
- Follow PyPIBuilder pattern
- `NpmBuilder` struct with httpClient and registryBaseURL
- `Name()` returns "npm"
- `CanBuild()` validates package name and checks existence
- `Build()` generates recipe with npm_install action

### 2. npm API Response Structure
```go
type npmPackageResponse struct {
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Homepage    string            `json:"homepage"`
    Repository  interface{}       `json:"repository"` // can be string or object
    Versions    map[string]struct {
        Bin map[string]string `json:"bin"`
    } `json:"versions"`
    DistTags struct {
        Latest string `json:"latest"`
    } `json:"dist-tags"`
}
```

### 3. Executable Discovery
- Get latest version from `dist-tags.latest`
- Extract `bin` field from `versions[latest]`
- `bin` can be:
  - `string`: single executable (package name is the command)
  - `map[string]string`: multiple executables with names as keys
- Fall back to package name with warning if no bin field

### 4. Recipe Generation
- Action: `npm_install`
- Version source: `npm`
- Params: `package`, `executables`

### 5. Create `internal/builders/npm_test.go`
- Mock npm API responses
- Test CanBuild, Build, package name validation
- Test bin field parsing (string vs map)
- Test fallback behavior

### 6. Update `cmd/tsuku/create.go`
- Add "npm" to normalizeEcosystem()
- Register NpmBuilder

### 7. Create `.github/workflows/npm-builder-tests.yml`
- Integration tests on Linux/macOS
- Test with `prettier` package
- Daily schedule + path triggers

## Package Name Validation
Use existing `isValidNpmPackageName` from `internal/version/resolver.go` or create similar validation:
- Max 214 characters
- Lowercase
- Can be scoped (@scope/package) or unscoped
- No consecutive dots

## Test Package
Use `prettier` for integration tests:
- Well-known package
- Has `bin` field with executable
- Supports --version flag
