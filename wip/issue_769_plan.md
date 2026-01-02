# Issue 769 Implementation Plan

## Summary

Implement `ContainerImageName()` function in `internal/sandbox/container_spec.go` to generate deterministic container image names based on package set hashing. The function takes a `ContainerSpec` and returns a cache-friendly image name like `tsuku/sandbox-cache:debian-a1b2c3d4e5f6g7h8`.

## Approach

Add the `ContainerImageName()` function directly to the existing `container_spec.go` file where `ContainerSpec` is defined. This co-locates related functionality and follows the pattern established by `DeriveContainerSpec()`.

The hash algorithm follows the design doc specification:
1. Extract all `<pm>:<package>` pairs from the spec's Packages map
2. Sort the pairs deterministically (alphabetically)
3. Compute SHA256 hash of the joined pairs (newline-separated)
4. Return image name with family prefix and first 16 hex chars of hash

Family prefix (`debian-`, `rhel-`, etc.) is included in the tag for human readability and easier debugging, even though the hash itself is unique.

### Alternatives Considered

- **Alternative 1: Separate `image_cache.go` file** - Rejected because this is a single function tightly coupled to `ContainerSpec`. Creating a new file would be premature abstraction.

- **Alternative 2: Hash only packages, omit PM names** - Rejected because different package managers can have packages with the same name (e.g., `apt:git` vs `pacman:git`). Including the PM name ensures uniqueness.

- **Alternative 3: Omit family prefix from tag** - Rejected for UX reasons. While technically redundant (hash is unique), the prefix makes image names human-readable during debugging and manual image management (e.g., `docker images | grep debian`).

## Files to Modify

- `internal/sandbox/container_spec.go` - Add `ContainerImageName()` function with SHA256 hashing logic
- `internal/sandbox/container_spec_test.go` - Add comprehensive test coverage for hash stability and uniqueness

## Files to Create

None - all code goes in existing files.

## Implementation Steps

- [x] Add `ContainerImageName()` function to `container_spec.go`
  - Import `crypto/sha256`, `encoding/hex`, and `strings` packages
  - Extract and sort `<pm>:<package>` pairs from `spec.Packages`
  - Compute SHA256 hash of newline-joined pairs
  - Return formatted string: `tsuku/sandbox-cache:<family>-<hash>` where hash is first 16 hex chars
  - Add godoc comment explaining deterministic hashing and cache benefits

- [x] Add unit tests to `container_spec_test.go`
  - Test hash stability: same packages → same hash (run twice to verify)
  - Test hash uniqueness: different packages → different hash
  - Test deterministic ordering: package order in map doesn't affect hash
  - Test PM inclusion in hash: same package name but different PM → different hash
  - Test all linux families: verify family prefix is included correctly
  - Test edge cases: empty packages (should still work with nil spec handling), single package

- [x] Run tests and verify passing
  - `go test -v ./internal/sandbox/...`
  - Verify all new tests pass
  - Check existing tests still pass (no regressions)

- [x] Run linters
  - `go vet ./internal/sandbox/...`
  - `golangci-lint run --timeout=5m ./internal/sandbox/...`

## Testing Strategy

**Unit tests** (in `container_spec_test.go`):

1. **Hash Stability Test**
   - Create identical `ContainerSpec` instances
   - Call `ContainerImageName()` multiple times
   - Assert all results are identical
   - Verifies deterministic hashing

2. **Hash Uniqueness Test**
   - Create specs with different package sets
   - Verify each produces a unique image name
   - Test cases: different packages, different PMs, different families

3. **Deterministic Ordering Test**
   - Create spec with packages added in random order
   - Call function multiple times
   - Verify hash is identical regardless of map iteration order
   - Critical for cache reliability

4. **Package Manager Inclusion Test**
   - Create two specs with same package name but different PMs (e.g., `apt:git` vs `pacman:git`)
   - Verify they produce different hashes
   - Ensures PM is part of hash input

5. **Family Prefix Test**
   - Create specs for each linux family (debian, rhel, arch, alpine, suse)
   - Verify image name includes correct family prefix
   - Example: `debian` → `tsuku/sandbox-cache:debian-<hash>`

6. **Edge Cases**
   - Single package: verify hash is computed correctly
   - Multiple PMs (if applicable to same family): verify all packages are hashed
   - Verify hash length is exactly 16 hex characters

**Manual verification**:
- Print sample image names for each family to visually confirm format
- Verify hash output matches expectations from design doc example

## Risks and Mitigations

**Risk 1: Hash collisions**
- **Likelihood**: Extremely low (SHA256 with 16 chars = 64 bits)
- **Impact**: Different package sets would share a cache image (incorrect)
- **Mitigation**: Using SHA256 (industry standard) and 16 hex chars (64 bits) provides sufficient uniqueness for package sets in practice. Birthday paradox applies, but with realistic package combinations (<1000s), collision probability is negligible.

**Risk 2: Hash instability due to map iteration**
- **Likelihood**: Medium (Go maps have random iteration order)
- **Impact**: Same packages would produce different hashes → cache misses
- **Mitigation**: Explicit sorting of packages and PMs before hashing (already in design). Test coverage includes determinism tests.

**Risk 3: Breaking changes to hash algorithm**
- **Likelihood**: Low (design is stable)
- **Impact**: Old cached images would be ignored, requiring rebuilds
- **Mitigation**: Document hash format in godoc. If algorithm changes in future, we can version the cache key (e.g., `v2-<hash>`). For this issue, follow design doc spec exactly.

## Success Criteria

- [ ] `ContainerImageName()` function exists and is exported
- [ ] Function takes `*ContainerSpec` as parameter (not separate args)
- [ ] Returns format: `tsuku/sandbox-cache:<family>-<hash>` where hash is 16 hex chars
- [ ] Hash includes all `<pm>:<package>` pairs in sorted order
- [ ] Hash uses SHA256 algorithm
- [ ] Unit tests verify hash stability (same input → same output)
- [ ] Unit tests verify hash uniqueness (different input → different output)
- [ ] Unit tests verify deterministic ordering (map order doesn't affect hash)
- [ ] All tests pass: `go test ./internal/sandbox/...`
- [ ] Linters pass: `go vet` and `golangci-lint`
- [ ] No test coverage regression (new function is well-tested)

## Open Questions

None - the introspection clarified all spec ambiguities. Implementation is ready to proceed.
