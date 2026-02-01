# Issue 1312 Summary

## What Was Implemented

Extended `tsuku update-registry` to fetch and cache `discovery.json` from the remote registry alongside existing recipe refresh. The discovery registry maps tool names to their installation sources, enabling the discovery resolver to look up tools by name without network calls.

## Changes Made

- `internal/registry/registry.go`: Added `FetchDiscoveryRegistry` method that downloads `{BaseURL}/recipes/discovery.json` and writes it to `{CacheDir}/discovery.json`, reusing the existing HTTP client and error types
- `internal/registry/registry_test.go`: Added 4 tests covering success, 404, directory creation, and network error cases
- `cmd/tsuku/update_registry.go`: Added `refreshDiscoveryRegistry` call in the command handler; runs before recipe refresh, logs warnings on failure without blocking

## Key Decisions

- Discovery fetch errors are warnings, not fatal: recipe refresh continues even if discovery registry fetch fails
- No TTL/metadata sidecar: simple overwrite per the design spec
- Fetch runs in all non-dry-run paths (including single-recipe refresh)

## Test Coverage

- 4 new tests added for `FetchDiscoveryRegistry`
- All registry tests pass

## Known Limitations

- Pre-existing `TestNoStdlibLog` failure from `internal/discover/chain.go` (merged in #1304, not caused by this change)
