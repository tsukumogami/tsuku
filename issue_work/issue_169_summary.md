# Issue 169 Summary

## What Was Implemented

Added support for specifying a separate Go module path for version resolution in recipes. This enables recipes for Go tools where the install path (subpackage) differs from the version module (parent module).

## Changes Made

- `internal/recipe/types.go`: Added `Module` field to `VersionSection` struct
- `internal/version/provider_factory.go`: Updated `GoProxySourceStrategy.Create()` to check `r.Version.Module` first before falling back to step params
- `internal/version/provider_goproxy_test.go`: Added test case for the new behavior

## New Recipes

7 new Go tool recipes created:
- `staticcheck` - Go static analysis (version module: honnef.co/go/tools)
- `goimports` - import formatter (version module: golang.org/x/tools)
- `godoc` - documentation server (version module: golang.org/x/tools)
- `gore` - Go REPL (version module: github.com/x-motemen/gore)
- `mockgen` - mock generator (version module: go.uber.org/mock)
- `dlv` - Go debugger (version module: github.com/go-delve/delve)
- `cobra-cli` - CLI scaffolding (no separate module needed)

## Key Decisions

- **Field location**: Added to `[version]` section rather than step params to keep version resolution concerns separate from installation concerns
- **Fallback behavior**: When `Version.Module` is empty, falls back to existing behavior of extracting module from step params for backward compatibility

## Test Coverage

- Added `TestGoProxySourceStrategy_Create_WithVersionModule` to verify the new behavior
- All existing tests continue to pass

## Known Limitations

- The `module` field is specific to `goproxy` source; other version providers don't use it
