# Issue 397 Implementation Plan

## Summary

Split `internal/version/resolver.go` (1,269 lines) into focused modules following Go best practices. Each step will be implemented and verified independently.

## Approach

Extract code incrementally, ensuring tests pass after each step:

1. Move independent utilities first (no cross-dependencies)
2. Introduce functional options pattern (allows clean constructor replacement)
3. Move domain-specific resolvers (npm, nodejs, etc.)
4. Clean up resolver.go as facade

### Order Rationale
- Start with utilities and HTTP client (no method dependencies)
- Options pattern before removing old constructors (allows tests to update incrementally)
- npm/nodejs last (they use utilities we're extracting)

## Implementation Steps

### Step 1: Extract HTTP client factory to httpclient.go

- [x] Create `internal/version/httpclient.go`
- [x] Move `NewHTTPClient()` function (lines 46-110)
- [x] Move `validateIP()` function (lines 112-147)
- [x] Verify tests pass

### Step 2: Extract version utilities to version_utils.go

- [ ] Create `internal/version/version_utils.go`
- [ ] Move `normalizeVersion()` function (lines 744-765)
- [ ] Move `isValidVersion()` function (lines 767-783)
- [ ] Move `compareVersions()` function (lines 785-820)
- [ ] Verify tests pass

### Step 3: Introduce functional options pattern

- [ ] Create `internal/version/options.go` with:
  - `Option` type: `type Option func(*Resolver)`
  - `WithNpmRegistry(url string) Option`
  - `WithPyPIRegistry(url string) Option`
  - `WithCratesIORegistry(url string) Option`
  - `WithRubyGemsRegistry(url string) Option`
  - `WithMetaCPANRegistry(url string) Option`
  - `WithGoDevURL(url string) Option`
  - `WithGoProxyURL(url string) Option`
  - `WithHomebrewRegistry(url string) Option`
- [ ] Modify `New()` to accept variadic options: `New(opts ...Option) *Resolver`
- [ ] Update tests to use new pattern
- [ ] Verify tests pass

### Step 4: Remove duplicate constructors

- [ ] Delete `NewWithNpmRegistry()` - replaced by `New(WithNpmRegistry(url))`
- [ ] Delete `NewWithPyPIRegistry()` - replaced by `New(WithPyPIRegistry(url))`
- [ ] Delete `NewWithCratesIORegistry()` - replaced by `New(WithCratesIORegistry(url))`
- [ ] Delete `NewWithRubyGemsRegistry()` - replaced by `New(WithRubyGemsRegistry(url))`
- [ ] Delete `NewWithMetaCPANRegistry()` - replaced by `New(WithMetaCPANRegistry(url))`
- [ ] Delete `NewWithGoDevURL()` - replaced by `New(WithGoDevURL(url))`
- [ ] Delete `NewWithGoProxyURL()` - replaced by `New(WithGoProxyURL(url))`
- [ ] Update all call sites using old constructors
- [ ] Verify tests pass

### Step 5: Move npm resolution to npm.go

- [ ] Create `internal/version/npm.go`
- [ ] Move `npmPackageNameRegex` variable (line 580)
- [ ] Move `isValidNpmPackageName()` function (lines 582-614)
- [ ] Move `ListNpmVersions()` method (lines 616-719)
- [ ] Move `ResolveNpm()` method (lines 822-867)
- [ ] Verify tests pass (npm_test.go should work as-is since same package)

### Step 6: Move Node.js resolution to nodejs.go

- [ ] Create `internal/version/nodejs.go`
- [ ] Move `ResolveNodeJS()` method (lines 869-925)
- [ ] Verify tests pass

### Step 7: Final cleanup and verification

- [ ] Review resolver.go - should be ~400 lines (Resolver struct, GitHub methods, Go methods, custom resolution)
- [ ] Run full test suite: `go test ./internal/version/...`
- [ ] Run golangci-lint
- [ ] Verify no circular dependencies

## File Changes Summary

| New File | Contents |
|----------|----------|
| `httpclient.go` | HTTP client factory, SSRF protection |
| `version_utils.go` | Version normalization and comparison |
| `options.go` | Functional options for Resolver |
| `npm.go` | npm registry resolution |
| `nodejs.go` | Node.js distribution resolution |

| Modified File | Changes |
|---------------|---------|
| `resolver.go` | Remove moved code, update New() |
| `resolver_test.go` | Update constructor calls |
| `npm_test.go` | No changes (same package) |

## Success Criteria

- [ ] All existing tests pass
- [ ] No new circular dependencies
- [ ] resolver.go reduced to ~400 lines
- [ ] Each new file is focused and cohesive
- [ ] golangci-lint passes

## Risks and Mitigations

- **Risk:** Breaking existing callers of `NewWith*` functions
  - **Mitigation:** These are internal functions, only tests use them

- **Risk:** Import cycles
  - **Mitigation:** All new files are in same package, no imports needed
