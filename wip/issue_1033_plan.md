# Issue 1033 Implementation Plan

## Summary

Move 155 non-embedded recipes from `internal/recipe/recipes/` to `recipes/` at the repo root and update `DefaultRegistryURL` in `internal/registry/registry.go` to remove the `internal/recipe` prefix, enabling remote fetching from the new location.

## Approach

Use a script-based migration to move all non-embedded recipes while keeping embedded recipes in place. The key insight is that `recipeURL()` already appends `/recipes/{letter}/{name}.toml` to the base URL, so only the base URL needs updating from `https://raw.githubusercontent.com/tsukumogami/tsuku/main/internal/recipe` to `https://raw.githubusercontent.com/tsukumogami/tsuku/main`.

### Alternatives Considered

- **Manual file moves**: Rejected - tedious and error-prone with 155 files
- **Full directory restructure**: Rejected - issue scope is limited to recipe migration, other references (CI workflows, docs) are addressed in subsequent issues

## Files to Modify

- `internal/registry/registry.go` - Update `DefaultRegistryURL` constant (line 21)
- `internal/recipe/loader.go` - Update error message path reference (line 159)

## Files to Create

- `recipes/a/` through `recipes/z/` - Letter subdirectories (as needed)
- Move 155 recipe TOML files from `internal/recipe/recipes/` to `recipes/`

## Implementation Steps

- [ ] Create letter subdirectories in `recipes/` (a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p, r, s, t, v, w, z)
- [ ] Move non-embedded recipes from `internal/recipe/recipes/` to `recipes/`, preserving letter structure
- [ ] Verify all 16 embedded recipes remain in `internal/recipe/recipes/`
- [ ] Update `DefaultRegistryURL` in `internal/registry/registry.go` to remove `internal/recipe` prefix
- [ ] Update error message in `internal/recipe/loader.go` to reference correct path
- [ ] Run `go build -o tsuku ./cmd/tsuku` to verify embed directive still works
- [ ] Run `go test ./...` to verify all tests pass

## Testing Strategy

- **Unit tests**: Run `go test ./...` to ensure existing tests pass
- **Build verification**: Run `go build -o tsuku ./cmd/tsuku` to confirm the embed directive works
- **Manual verification**: Confirm embedded recipes are accessible without network

## Risks and Mitigations

- **Breaking embed directive**: Mitigated by verifying build succeeds and embedded recipes remain in `internal/recipe/recipes/`
- **Missing embedded recipe during migration**: Mitigated by using explicit list from EMBEDDED_RECIPES.md (16 recipes)
- **Test failures from path references**: Mitigated by running full test suite; pre-existing failures in `internal/actions` are unrelated (documented in baseline)

## Success Criteria

- [ ] `recipes/` directory exists at repo root with letter subdirectories
- [ ] All 155 non-embedded recipes moved to `recipes/`
- [ ] All 16 embedded recipes remain in `internal/recipe/recipes/`
- [ ] `DefaultRegistryURL` no longer contains `internal/recipe`
- [ ] `go build -o tsuku ./cmd/tsuku` succeeds
- [ ] `go test ./...` passes (excluding pre-existing failures)

## Open Questions

None - requirements are clear from issue and design context.

## Reference: Embedded Recipes (16 total)

Recipes that MUST stay in `internal/recipe/recipes/`:

**Toolchains**: go, rust, nodejs, python-standalone, ruby, perl

**Build tools**: make, cmake, meson, ninja, zig, pkg-config, patchelf

**Libraries**: libyaml, openssl, zlib
