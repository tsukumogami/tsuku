# Issue 169 Implementation Plan

## Summary

Add a `Module` field to `VersionSection` that allows recipes to specify a different Go module path for version resolution than the install path used in `go_install` steps.

## Approach

The `GoProxySourceStrategy` currently extracts the module path from `[[steps]]` params. We'll add a `Module` field to `[version]` section and update the strategy to prefer this field when present.

### Alternatives Considered
- Use `github_repo` workaround: Works but loses the benefit of goproxy for Go tools
- Modify go_install action to accept separate version module: More invasive change

## Files to Modify

- `internal/recipe/types.go` - Add `Module` field to `VersionSection`
- `internal/version/provider_factory.go` - Update `GoProxySourceStrategy.Create()` to prefer `r.Version.Module`

## Files to Create

Recipes for affected tools:
- `internal/recipe/recipes/s/staticcheck.toml`
- `internal/recipe/recipes/g/goimports.toml`
- `internal/recipe/recipes/g/godoc.toml`
- `internal/recipe/recipes/g/gore.toml`
- `internal/recipe/recipes/m/mockgen.toml`
- `internal/recipe/recipes/d/dlv.toml`
- `internal/recipe/recipes/c/cobra-cli.toml`

## Implementation Steps

- [x] Add `Module` field to `VersionSection` struct in types.go
- [x] Update `GoProxySourceStrategy.Create()` to check `r.Version.Module` first
- [x] Add test for new behavior in provider_goproxy_test.go
- [x] Create staticcheck recipe with module field
- [x] Create goimports recipe with module field
- [x] Create godoc recipe with module field
- [x] Create gore recipe with module field
- [x] Create mockgen recipe with module field
- [x] Create dlv recipe with module field
- [x] Create cobra-cli recipe (needs verify command fix)
- [x] Run tests

## Testing Strategy

- Unit tests: Add test case for `GoProxySourceStrategy` with `r.Version.Module` set
- Validation: Existing recipe validation tests cover new recipes
- CI integration tests will verify the recipes work

## Recipe Template

```toml
[metadata]
name = "<tool-name>"
description = "<description>"
homepage = "<homepage>"
version_format = "semver"
dependencies = ["go"]

[version]
source = "goproxy"
module = "<parent-module-for-version-lookup>"

[[steps]]
action = "go_install"
module = "<full-install-path>"
executables = ["<binary-name>"]

[verify]
command = "<binary> --version"
pattern = "<pattern>"
```

## Success Criteria

- [x] `Module` field added to `VersionSection`
- [x] `GoProxySourceStrategy` prefers `r.Version.Module` over step params
- [x] 7 new Go tool recipes created
- [x] All tests pass
