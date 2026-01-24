## Goal

Remove the letter-based subdirectory structure from embedded recipes, placing all 17 recipe files directly in `internal/recipe/recipes/`.

## Context

After PR #1069 migrated 155 registry recipes to `recipes/` at the repo root, only 17 critical embedded recipes remain in `internal/recipe/recipes/`. The letter-based subdirectory structure (e.g., `g/go.toml`, `m/make.toml`) was useful when managing hundreds of recipes but adds unnecessary complexity for this small set.

Current embedded recipes: cmake, gcc-libs, go, libyaml, make, meson, ninja, nodejs, openssl, patchelf, perl, pkg-config, python-standalone, ruby, rust, zig, zlib.

## Acceptance Criteria

- [ ] All 17 recipe files moved from `internal/recipe/recipes/<letter>/<name>.toml` to `internal/recipe/recipes/<name>.toml`
- [ ] Empty letter directories removed
- [ ] Go embed directive updated: `//go:embed recipes/*/*.toml` → `//go:embed recipes/*.toml`
- [ ] `scripts/generate-registry.py` updated:
  - Glob pattern for embedded recipes changed to find flat `.toml` files
  - `PATH_PATTERN` regex updated to accept flat embedded recipe paths
- [ ] `scripts/validate-all-golden.sh` updated to remove `first_letter` path logic
- [ ] `.github/workflows/validate-embedded-deps.yml` updated to remove `first_letter` path logic
- [ ] Documentation updated:
  - CONTRIBUTING.md (lines ~397, ~439)
  - docs/EMBEDDED_RECIPES.md (line ~65)
  - docs/BUILD-ESSENTIALS.md (recipe path examples)
- [ ] `go test ./...` passes
- [ ] Website generation works: `python3 scripts/generate-registry.py` succeeds
- [ ] CI passes

## Dependencies

None
