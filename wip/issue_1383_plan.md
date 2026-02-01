# Issue 1383 Implementation Plan

## Summary

Add a `Probe()` method to each of the 7 ecosystem builders that wraps the existing fetch method and returns a `discover.ProbeResult`. Each method calls the builder's existing fetch function, converts success/failure into `Exists: true/false`, and sets `Source` to match the builder's existing source string pattern.

## Approach

Each builder already has a private fetch method that queries its registry API and returns a typed response or error. `Probe()` wraps that call: success means `Exists: true`, any error means `Exists: false` (soft error handling). This avoids new HTTP requests and keeps each `Probe()` well under 20 lines. The Go builder is the only one that computes `Age` since `goProxyLatestResponse` has a `Time` field.

### Alternatives Considered

- **Add Probe() to a shared base struct**: Not viable since builders don't share a common struct, only a common interface. Each fetch method has different signatures and return types.
- **Return errors from Probe() on API failure**: Rejected per issue spec -- API failures should yield `ProbeResult{Exists: false}`, not errors.

## Files to Modify

- `internal/builders/cargo.go` - Add `Probe(ctx, name) (*discover.ProbeResult, error)` method to `CargoBuilder`
- `internal/builders/pypi.go` - Add `Probe()` method to `PyPIBuilder`
- `internal/builders/npm.go` - Add `Probe()` method to `NpmBuilder`
- `internal/builders/gem.go` - Add `Probe()` method to `GemBuilder`
- `internal/builders/go.go` - Add `Probe()` method to `GoBuilder` (with Age computation from `Time` field)
- `internal/builders/cpan.go` - Add `Probe()` method to `CPANBuilder`
- `internal/builders/cask.go` - Add `Probe()` method to `CaskBuilder`

## Files to Create

- `internal/builders/probe_test.go` - Tests for all 7 `Probe()` methods (exists, not-found, API-error cases)

## Implementation Steps

- [ ] Add `Probe()` to `CargoBuilder` -- calls `fetchCrateInfo`, returns `ProbeResult{Exists: true, Source: "crates.io:<name>"}`
- [ ] Add `Probe()` to `PyPIBuilder` -- calls `fetchPackageInfo`, returns `ProbeResult{Exists: true, Source: "pypi:<name>"}`
- [ ] Add `Probe()` to `NpmBuilder` -- calls `fetchPackageInfo`, returns `ProbeResult{Exists: true, Source: "npm:<name>"}`
- [ ] Add `Probe()` to `GemBuilder` -- calls `fetchGemInfo`, returns `ProbeResult{Exists: true, Source: "rubygems:<name>"}`
- [ ] Add `Probe()` to `GoBuilder` -- calls `fetchModuleInfo`, parses `Time` field to compute `Age` in days, returns `ProbeResult{Exists: true, Age: days, Source: "goproxy:<name>"}`
- [ ] Add `Probe()` to `CPANBuilder` -- calls `fetchDistributionInfo` (after `normalizeToDistribution`), returns `ProbeResult{Exists: true, Source: "metacpan:<dist>"}`
- [ ] Add `Probe()` to `CaskBuilder` -- calls `fetchCaskInfo`, returns `ProbeResult{Exists: true, Source: "cask:<name>"}`
- [ ] Add compile-time interface assertions: `var _ discover.EcosystemProber = (*CargoBuilder)(nil)` etc.
- [ ] Write tests in `probe_test.go` using httptest pattern (mock server, test exists/not-found/error)
- [ ] Run `go build ./...` and `go test ./...` to verify

## Testing Strategy

- **Unit tests**: For each builder, test 3 scenarios using httptest mock servers:
  1. Package exists: verify `Exists: true`, correct `Source` string, `Downloads: 0`
  2. Package not found (404): verify `Exists: false`, no error returned
  3. API error (500): verify `Exists: false`, no error returned (soft error handling)
  4. GoBuilder-specific: verify `Age` is computed correctly from `Time` field
- **Compile-time checks**: Interface satisfaction assertions ensure each builder implements `EcosystemProber`

## Method Details

| Builder | Fetch Method | Return Type | Source Format | Age |
|---------|-------------|-------------|---------------|-----|
| CargoBuilder | `fetchCrateInfo(ctx, name)` | `*cratesIOCrateResponse` | `crates.io:<name>` | 0 |
| PyPIBuilder | `fetchPackageInfo(ctx, name)` | `*pypiPackageResponse` | `pypi:<name>` | 0 |
| NpmBuilder | `fetchPackageInfo(ctx, name)` | `*npmPackageResponse` | `npm:<name>` | 0 |
| GemBuilder | `fetchGemInfo(ctx, name)` | `*rubyGemsGemResponse` | `rubygems:<name>` | 0 |
| GoBuilder | `fetchModuleInfo(ctx, name)` | `*goProxyLatestResponse` | `goproxy:<name>` | computed from `Time` |
| CPANBuilder | `fetchDistributionInfo(ctx, dist)` | `*metacpanRelease` | `metacpan:<dist>` | 0 |
| CaskBuilder | `fetchCaskInfo(ctx, name)` | `*caskAPIResponse` | `cask:<name>` | 0 |

### Probe() Template (all builders except Go)

```go
func (b *CargoBuilder) Probe(ctx context.Context, name string) (*discover.ProbeResult, error) {
    _, err := b.fetchCrateInfo(ctx, name)
    if err != nil {
        return &discover.ProbeResult{Source: fmt.Sprintf("crates.io:%s", name)}, nil
    }
    return &discover.ProbeResult{
        Exists: true,
        Source: fmt.Sprintf("crates.io:%s", name),
    }, nil
}
```

### GoBuilder Probe() (with Age)

```go
func (b *GoBuilder) Probe(ctx context.Context, name string) (*discover.ProbeResult, error) {
    info, err := b.fetchModuleInfo(ctx, name)
    if err != nil {
        return &discover.ProbeResult{Source: fmt.Sprintf("goproxy:%s", name)}, nil
    }
    result := &discover.ProbeResult{
        Exists: true,
        Source: fmt.Sprintf("goproxy:%s", name),
    }
    if t, err := time.Parse(time.RFC3339, info.Time); err == nil {
        result.Age = int(time.Since(t).Hours() / 24)
    }
    return result, nil
}
```

### CPANBuilder Note

CPAN's `Probe()` must call `normalizeToDistribution(name)` before `fetchDistributionInfo` and use the normalized name for the `Source` field, matching the pattern in `CanBuild()`.

## Risks and Mitigations

- **Import cycle**: `builders` package importing `discover` package. Mitigation: verify `discover` doesn't import `builders` (it imports `builders.SessionBuilder` interface only, so this creates a cycle). **Resolution**: The `ProbeResult` type and `EcosystemProber` interface may need to move to the `builders` package, or a new shared package. Check the import graph before implementing.
- **Name validation**: Some builders validate names before fetching (e.g., `isValidCrateName`). `Probe()` should skip validation and let the fetch method fail naturally, returning `Exists: false`.

## Success Criteria

- [ ] All 7 builders have `Probe()` methods under 20 lines each
- [ ] Compile-time assertions verify `EcosystemProber` interface satisfaction
- [ ] `Probe()` returns `ProbeResult{Exists: false}` on any error (never returns error)
- [ ] GoBuilder computes Age from the `Time` field
- [ ] Downloads is always 0
- [ ] All tests pass: `go test ./internal/builders/... ./internal/discover/...`
- [ ] No new HTTP requests beyond the existing fetch methods

## Open Questions

- **Import cycle risk**: `discover` imports `builders.SessionBuilder`, and `Probe()` returns `*discover.ProbeResult`. This creates a circular dependency. Options: (1) move `ProbeResult` to `builders` package, (2) create a shared types package, (3) have `Probe()` return a builders-local type that discover converts. Need to verify the import graph before starting implementation.
