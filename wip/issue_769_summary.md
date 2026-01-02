# Issue 769 Summary

## What Was Implemented

Implemented `ContainerImageName()` function that generates deterministic cache-friendly image names from container specifications. The function computes SHA256 hashes of package sets to enable container image caching, allowing test runs with identical dependencies to reuse previously built containers.

## Changes Made

- `internal/sandbox/container_spec.go`: Added `ContainerImageName()` function
  - Imports: Added `crypto/sha256` and `encoding/hex` for hashing
  - Function takes `*ContainerSpec` and returns image name like `tsuku/sandbox-cache:debian-a1b2c3d4e5f6g7h8`
  - Hash algorithm: Sorts all `pm:package` pairs, joins with newlines, computes SHA256, takes first 16 hex chars
  - Family prefix included in tag for human readability during debugging

- `internal/sandbox/container_spec_test.go`: Comprehensive test suite with 6 test functions
  - `TestContainerImageName`: Format validation for all 5 families (debian, rhel, arch, alpine, suse)
  - `TestContainerImageName_HashStability`: Verifies same spec produces same hash across multiple calls
  - `TestContainerImageName_HashUniqueness`: Verifies different specs produce different hashes
  - `TestContainerImageName_DeterministicOrdering`: Ensures map iteration order doesn't affect hash
  - `TestContainerImageName_PackageManagerInHash`: Confirms PM name is part of hash (apt:git ≠ pacman:git)
  - `TestContainerImageName_MultipleManagers`: Edge case handling for mixed PMs

## Key Decisions

- **Keep family prefix for readability**: Issue spec originally included `<family>-<hash>` format. During introspection, we considered removing the family (since hash already encodes it via PM name) but decided to keep it for human debugging and image management ease. The redundancy is acceptable for UX benefits.

- **16 hex characters for hash**: Uses first 16 hex chars (64 bits) of SHA256. This provides sufficient uniqueness for realistic package combinations while keeping image names reasonably short. Risk of collision is negligible given package set diversity.

- **Deterministic sorting at two levels**: First sorts packages within each PM, then sorts all `pm:package` pairs globally. This double-sorting ensures consistent hashing regardless of Go map iteration order.

- **Newline-separated hash input**: Joins `pm:package` strings with newlines rather than other separators. Follows design doc specification and provides clear delimiter that won't appear in package names.

## Trade-offs Accepted

- **Family prefix redundancy**: The family is technically redundant (hash already includes PM which determines family), but we kept it for readability. This makes image names slightly longer but significantly more user-friendly.

- **Fixed hash length (16 chars)**: Could use full SHA256 (64 chars) for guaranteed uniqueness, but 16 chars (64 bits) is sufficient for package sets. Birthday paradox applies, but with realistic combinations (<thousands), collision probability is negligible.

- **No cache checking in this issue**: The acceptance criteria originally included "Check if image exists before building" but introspection determined this belongs in #770 (executor integration) when Runtime interface gains `ImageExists()` method. This issue focuses solely on name generation.

## Test Coverage

- New tests added: 6 test functions with 17 total test cases
- All tests pass
- Coverage areas:
  - Format validation (repository:tag structure, hex chars, length)
  - Hash stability (determinism across calls)
  - Hash uniqueness (different inputs yield different outputs)
  - Deterministic ordering (map order independence)
  - PM inclusion (validates PM is part of hash)
  - All 5 linux families (debian, rhel, arch, alpine, suse)
  - Edge cases (multiple PMs, single package, various counts)

## Known Limitations

- **No validation of ContainerSpec contents**: Function assumes spec.LinuxFamily and spec.Packages are valid. If LinuxFamily is empty or Packages is nil, the function will produce unusual (but deterministic) names. Validation is expected to happen during `DeriveContainerSpec()`.

- **Hash collisions theoretically possible**: With 64 bits, collisions could occur but are extremely unlikely for realistic package sets. If needed in future, could increase to 32 hex chars (128 bits) or version the cache key.

- **No versioning of hash algorithm**: If hash algorithm changes in future, old cached images become orphaned. Could add version prefix (e.g., `v2-<hash>`) if algorithm needs to evolve, but current design is stable.

## Future Improvements

- Add telemetry to track cache hit rates in production
- Consider optional full-length hash mode for paranoid users
- Support custom image repositories (currently hardcoded to `tsuku/sandbox-cache`)
- Add cache cleanup utilities for removing unused images
