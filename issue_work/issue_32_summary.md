# Issue 32 Implementation Summary

## Changes Made

### New Files
- `internal/registry/registry.go` - Registry client for fetching recipes from remote registry
- `internal/registry/registry_test.go` - Comprehensive tests for registry functionality

### Deleted Files
- `bundled/bundled.go` - Removed bundled recipes embedding
- `bundled/recipes/*.toml` - Removed all bundled recipe files

### Modified Files
- `internal/config/config.go` - Added `RegistryDir` field for caching remote recipes
- `internal/recipe/loader.go` - Simplified to use registry only (no bundled fallback)
- `internal/install/bootstrap.go` - Updated to use registry for package manager bootstrapping
- `cmd/tsuku/main.go` - Updated to use registry-only loader, added `update-registry` command

## Key Implementation Details

### Registry Client (`internal/registry/registry.go`)
- Fetches recipes from `https://raw.githubusercontent.com/tsuku-dev/tsuku-registry/main`
- Registry URL configurable via `TSUKU_REGISTRY_URL` environment variable
- Caches fetched recipes locally at `~/.tsuku/registry/{letter}/{name}.toml`
- 30-second timeout for fetch operations

### Recipe Resolution Priority
1. **In-memory cache** - Already loaded recipes
2. **Disk cache** - Previously fetched recipes in `~/.tsuku/registry/`
3. **Remote registry** - Fetch from GitHub and cache

No bundled fallback - all recipes come from the external registry.

### New Command: `tsuku update-registry`
Clears the local recipe cache, forcing fresh downloads on next use.

## Testing
- Unit tests with mock HTTP server
- Tests for cache operations (read, write, clear)
- Tests for environment variable override
- Tests for context cancellation

## Build Verification
- All unit tests pass
- Build succeeds
- gofmt formatting verified
- go vet passes
