# Issue 1649 Implementation Plan

## Summary

Add a `CheckTyposquat` function that computes Levenshtein distance between a requested tool name and all registry entries, returning a warning when distance is 1 or 2 (exact matches excluded). Integration happens in `ChainResolver.Resolve()` after name normalization but before stage iteration.

## Approach

Create a new `typosquat.go` file with a pure Go implementation of Levenshtein distance and the `CheckTyposquat` function. The chain resolver gains a `WithRegistry` method to inject the registry reference needed for typosquat checking.

### Alternatives Considered

- **Integration in RegistryLookup.Resolve()**: The introspection noted this might be cleaner since RegistryLookup already accesses the registry. However, the design doc and issue spec both explicitly state integration should be in `ChainResolver.Resolve()` before stage iteration. This also makes the check run once per resolution rather than being tied to a specific stage implementation.

- **External Levenshtein library**: Could import a third-party library for edit distance. Rejected because the algorithm is simple (~20 lines), and tsuku follows a "no system dependencies" philosophy that extends to minimizing external Go dependencies for simple functionality.

## Files to Modify

- `internal/discover/chain.go` - Add `registry` field, `WithRegistry()` method, and typosquat check call in `Resolve()` after normalization

## Files to Create

- `internal/discover/typosquat.go` - `TyposquatWarning` struct, `levenshtein()` function, and `CheckTyposquat()` function
- `internal/discover/typosquat_test.go` - Table-driven tests following `disambiguate_test.go` patterns

## Implementation Steps

- [ ] Create `internal/discover/typosquat.go` with `TyposquatWarning` struct definition
- [ ] Implement `levenshtein(a, b string) int` function using dynamic programming
- [ ] Implement `CheckTyposquat(toolName string, registry *DiscoveryRegistry) *TyposquatWarning`
- [ ] Add `registry *DiscoveryRegistry` field to `ChainResolver` struct
- [ ] Add `WithRegistry(reg *DiscoveryRegistry) *ChainResolver` builder method
- [ ] Call `CheckTyposquat` in `Resolve()` after normalization, before stage iteration
- [ ] Store typosquat warning in result or log it (determination needed during implementation)
- [ ] Create `internal/discover/typosquat_test.go` with test cases for:
  - No matches (distance > 2)
  - Distance 1 match
  - Distance 2 match
  - Distance 3 (no warning)
  - Exact match (distance 0, no warning)
  - Empty registry (no warning)
  - Case sensitivity (normalized comparison)
- [ ] Run `go test ./internal/discover -run TestTyposquat` to verify tests pass
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` before commit

## Testing Strategy

### Unit Tests

- **TestTyposquatLevenshtein**: Verify distance calculations for known string pairs
  - `("bat", "bat")` = 0
  - `("bat", "bta")` = 2 (two substitutions)
  - `("ripgrep", "rgiprep")` = distance calculation
  - `("a", "ab")` = 1 (insertion)
  - `("ab", "a")` = 1 (deletion)

- **TestTyposquatCheckTyposquat**: Table-driven tests for the main function
  - No similar names in registry -> nil
  - Exact match in registry -> nil (distance 0 excluded)
  - Distance 1 match -> TyposquatWarning with correct fields
  - Distance 2 match -> TyposquatWarning with correct fields
  - Distance 3 match -> nil (threshold exceeded)
  - Empty registry -> nil

### Integration Testing

- Manual verification: Build tsuku, add test registry entry, request similar name
- Verify warning struct is populated correctly

## Risks and Mitigations

- **False positives for short names**: Short tool names like "go", "rg", "fd" have many edit-distance neighbors. Mitigation: The design doc acknowledges this and accepts distance <= 2 as research-backed. Users can ignore warnings.

- **Performance with large registry**: Currently ~500 entries. At O(n*m) per comparison where n and m are string lengths, checking all entries is negligible. Mitigation: If registry grows significantly (10K+), consider indexing by first character or using BK-trees. Not needed for current scale.

- **TyposquatWarning field naming**: Issue spec defines specific fields (`requested`, `similar`, `distance`, `source`). Must match exactly for downstream consumers. Mitigation: Use exact field names from issue spec.

- **Registry access pattern**: ChainResolver doesn't currently hold a registry reference. Need to wire it through `WithRegistry()` builder method similar to existing `WithTelemetry()` and `WithLogger()` patterns.

## Success Criteria

- [ ] `go test ./internal/discover -run TestTyposquat` passes all tests
- [ ] `CheckTyposquat("bta", registry)` returns warning when registry contains "bat"
- [ ] `CheckTyposquat("bat", registry)` returns nil when registry contains "bat" (exact match)
- [ ] `CheckTyposquat("xyz", registry)` returns nil when no similar names exist
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run --timeout=5m ./...` passes
- [ ] `go build ./cmd/tsuku` succeeds

## Open Questions

None. The issue spec and design doc provide sufficient detail for implementation. Minor implementation decisions (like whether to log the warning vs return it) will be resolved during development based on how the chain resolver currently handles metadata.
