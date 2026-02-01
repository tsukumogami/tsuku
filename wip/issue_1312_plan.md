# Issue 1312 Implementation Plan

## Summary

Add a `FetchDiscoveryRegistry` method to `Registry` that downloads `discovery.json` from the remote registry and caches it locally, then call it from the `update-registry` command alongside the existing recipe refresh.

## Approach

The `Registry` struct already has the HTTP client, base URL, and cache directory needed. Adding a single method that fetches `{BaseURL}/recipes/discovery.json` and writes it to `{CacheDir}/discovery.json` keeps the change minimal and consistent with existing patterns. The command handler calls this new method before or after the recipe refresh so both registries stay in sync.

### Alternatives Considered

- **New struct/package for discovery fetching**: Unnecessary complexity since `Registry` already has all the infrastructure (client, base URL, cache dir). Would duplicate configuration.
- **Add TTL/metadata sidecar like recipe cache**: The issue explicitly says simple overwrite is fine. Discovery data changes infrequently and the file is small. No staleness tracking needed.

## Files to Modify

- `internal/registry/registry.go` - Add `FetchDiscoveryRegistry(ctx) error` method
- `internal/registry/registry_test.go` - Add tests for `FetchDiscoveryRegistry`
- `cmd/tsuku/update_registry.go` - Call `FetchDiscoveryRegistry` in the command handler

## Files to Create

None.

## Implementation Steps

- [x] Add `FetchDiscoveryRegistry(ctx context.Context) error` to `Registry` in `internal/registry/registry.go`
  - Build URL as `{BaseURL}/recipes/discovery.json`
  - Use existing `r.client` for the HTTP request
  - Create `r.CacheDir` with `os.MkdirAll` if it doesn't exist
  - Write response body to `filepath.Join(r.CacheDir, "discovery.json")`
  - Return `RegistryError` with appropriate types on failure (reuse `WrapNetworkError` for network errors)
- [x] Add unit tests for `FetchDiscoveryRegistry` in `internal/registry/registry_test.go`
  - Test successful fetch and cache write
  - Test network error produces clear error message
  - Test HTTP 404 returns appropriate error
  - Test idempotency (calling twice overwrites cleanly)
  - Test directory creation when CacheDir doesn't exist
- [x] Update `update-registry` command in `cmd/tsuku/update_registry.go`
  - In the main `Run` func, call `reg.FetchDiscoveryRegistry(ctx)` after creating the registry
  - Print status message ("Updating discovery registry...")
  - On error, print warning but don't abort (recipe refresh should still proceed)
  - Call it in all code paths (not just refresh-all; also dry-run should skip it, single-recipe should still do it)

## Testing Strategy

- Unit tests: httptest server to simulate registry responses; verify file written to temp CacheDir; verify error classification on failures
- Manual verification: `go build -o tsuku ./cmd/tsuku && ./tsuku update-registry` should create `$TSUKU_HOME/registry/discovery.json`

## Risks and Mitigations

- **Breaking existing recipe refresh**: Discovery fetch is called independently, before/after recipe refresh. An error in discovery fetch logs a warning but doesn't prevent recipe refresh.
- **URL mismatch**: URL is constructed from the same `BaseURL` used for recipes, and `recipes/discovery.json` already exists at that path in the repo.

## Success Criteria

- [ ] `tsuku update-registry` fetches and caches `discovery.json` to `$TSUKU_HOME/registry/discovery.json`
- [ ] Directory is created if it doesn't exist
- [ ] Command is idempotent (multiple runs succeed)
- [ ] Network failure produces a clear error message but doesn't block recipe refresh
- [ ] Existing recipe refresh behavior is unchanged
- [ ] All tests pass (`go test ./...`)
- [ ] Build succeeds (`go build ./cmd/tsuku`)

## Open Questions

None.
