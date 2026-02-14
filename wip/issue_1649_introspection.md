# Issue 1649 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-disambiguation.md`
- Sibling issues reviewed: #1648, #1650, #1651, #1652, #1653, #1654, #1655 (all closed)
- Prior patterns identified:
  - `disambiguate.go` with ranking algorithm and selection logic
  - `ConfirmDisambiguationFunc` callback type for interactive disambiguation
  - `AmbiguousMatchError` type with `--from` formatting
  - `probeOutcome` struct used for internal representation
  - Table-driven tests with comprehensive edge cases
  - `SelectionReason` constants for batch tracking

## Gap Analysis

### Minor Gaps

1. **Registry iteration method**: The issue spec and design doc show `registry.Entries()` in the pseudo-code, but `DiscoveryRegistry` does not expose an `Entries()` method. Implementation should iterate over `registry.Tools` map directly. This is a minor API design detail.

2. **Integration point clarified**: Issue says "Chain resolver calls typosquat check before probe stage" but design doc clarifies the check can happen "in the chain resolver or at the start of disambiguation." Given the chain architecture (`chain.go`), the check should occur in `ChainResolver.Resolve()` after name normalization but before iterating through stages. The design explicitly shows this in Phase 2 files to modify: "chain.go - Add typosquat check before probe".

3. **Test naming convention**: Existing tests follow `TestFunctionName` pattern with table-driven subtests. The issue's validation script runs `go test -v ./internal/discover -run TestTyposquat`, so test functions should be named `TestTyposquat*` to match.

### Moderate Gaps

None identified. The issue spec is self-contained and does not conflict with completed work.

### Major Gaps

None identified. Issue #1649 has no dependencies and its function signature is explicitly defined. The sibling issues did not modify the integration point (chain resolver iteration loop).

## Recommendation

**Proceed**

The issue specification is complete and actionable. Minor gaps are implementation details that can be resolved during development:
- Use `registry.Tools` for iteration instead of a non-existent `Entries()` method
- Add typosquat check in `ChainResolver.Resolve()` after normalization, before stage iteration

## Implementation Notes

From sibling issues, follow these established patterns:

1. **File naming**: Create `internal/discover/typosquat.go` and `internal/discover/typosquat_test.go` per design doc Phase 2.

2. **Type definitions**: Use existing types from `resolver.go`:
   - The issue spec defines `TyposquatWarning` struct - this is new and should be added
   - Use `DiscoveryRegistry` from `registry.go` for registry access

3. **Test patterns**: Follow table-driven test style from `disambiguate_test.go`:
   ```go
   tests := []struct {
       name     string
       toolName string
       // ... fields
       expected *TyposquatWarning
   }{...}
   ```

4. **Integration**: The chain resolver does not currently have access to the registry. The integration will need to:
   - Pass registry reference to `ChainResolver` or
   - Have the first stage (registry lookup) perform the check

   Given that typosquat checking is against registry entries, and the registry lookup stage already has access to the registry, the cleanest integration may be in `RegistryLookup.Resolve()` before returning a miss, rather than in `ChainResolver.Resolve()`. This is a judgment call for implementation.
