# Issue 529 Implementation Plan

## Summary

Split validation into two phases: run `tsuku eval` on host (generates plan + caches downloads), then run `tsuku install --plan` in container with cached assets mounted read-only and no network access.

## Approach

The current validation runs `tsuku install` entirely inside a container with network access. This causes rate limit issues (no GITHUB_TOKEN) and redundant downloads. The new approach:

1. **Host phase**: Generate plan via executor.GeneratePlan(), persist downloads to cache
2. **Container phase**: Execute plan with pre-cached assets, no network needed

### Alternatives Considered
- **Mount GITHUB_TOKEN into container**: Would expose credentials in container environment
- **Keep current approach + fix test binary detection**: Already implemented in #528, but doesn't solve rate limits or redundant downloads

## Files to Modify

- `internal/validate/executor.go` - Refactor Validate() to use eval+plan approach
- `internal/validate/source_build.go` - Same refactor for ValidateSourceBuild()
- `internal/executor/plan_generator.go` - Add option to persist downloads to cache
- `internal/validate/predownload.go` - Add method to save to download cache

## Files to Create

None - reusing existing components

## Implementation Steps

- [ ] Step 1: Add CacheDownloads option to PlanConfig
  - Modify GeneratePlan to optionally save downloads to cache via DownloadCache
  - PreDownloader already downloads for checksum; add path to persist before cleanup

- [ ] Step 2: Refactor Validate() to use eval+plan pattern
  - Call GeneratePlan on host with CacheDownloads=true
  - Write plan JSON to workspace
  - Add cache directory mount (read-only)
  - Change validation script to run `tsuku install --plan`
  - Set network to "none" instead of "host"

- [ ] Step 3: Refactor ValidateSourceBuild() similarly
  - Same pattern as Validate() but with source build config

- [ ] Step 4: Update tests
  - Update executor_test.go mocks to expect new behavior
  - Verify cache mount is read-only in tests

## Testing Strategy

- Unit tests: Update existing tests to verify new mount patterns
- Integration tests: LLM ground truth tests should pass without rate limits
- Manual verification: Run `go test -run TestLLMGroundTruth` with GITHUB_TOKEN on host

## Risks and Mitigations

- **Risk**: Container can't find cached assets
  - **Mitigation**: Mount cache at same path as host, set TSUKU_HOME correctly

- **Risk**: Checksum mismatch between cached and expected
  - **Mitigation**: install --plan already validates checksums

## Success Criteria

- [ ] Validation runs with --network=none (no network inside container)
- [ ] Cache directory mounted read-only in container
- [ ] LLM tests pass without rate limit errors
- [ ] All existing validate tests pass

## Open Questions

None - all resolved during research.
