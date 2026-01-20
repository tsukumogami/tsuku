# Issue 1033 Summary

## What Was Implemented

Migrated 155 non-embedded recipes from `internal/recipe/recipes/` to `recipes/` at the repository root, establishing a clear separation between embedded recipes (action dependencies required for CLI bootstrap) and registry recipes (fetched at runtime). Updated the registry URL to point to the new location.

## Changes Made

- `internal/registry/registry.go`: Updated `DefaultRegistryURL` from `.../internal/recipe` to `.../main` (line 20)
- `internal/recipe/loader_test.go`: Fixed test to use "go" instead of "golang" for embedded recipe check (line 1127)
- `internal/recipe/recipes/`: Kept 16 embedded recipes (go, rust, nodejs, python-standalone, ruby, perl, make, cmake, meson, ninja, zig, pkg-config, patchelf, libyaml, openssl, zlib)
- `recipes/`: Created with 22 letter subdirectories containing 155 migrated recipes

## Key Decisions

- **Keep loader error message unchanged**: The error in `loader.go` correctly references `internal/recipe/recipes/` because that's where embedded recipes must be added
- **Use git mv for migration**: Preserves file history and makes review cleaner

## Trade-offs Accepted

- **CI workflow updates deferred**: Per milestone structure, workflow path updates are handled by subsequent issues (#1034-#1038)
- **Documentation updates deferred**: References to old paths in docs will be updated by downstream issues

## Test Coverage

- Existing tests pass (except pre-existing failures in `internal/actions`)
- Fixed `TestLoader_Get_RequireEmbedded` to use actual embedded recipe ("go" instead of "golang")
- No new tests needed - this is a file relocation with URL update

## Known Limitations

- Registry recipes won't be fetchable until this PR is merged (the URL points to `main` branch)
- CI workflows still trigger on old paths until updated by #1034

## Future Improvements

- Subsequent issues will update CI workflows and documentation to reference new paths
