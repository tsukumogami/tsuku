# Issue 1071 Implementation Plan

## Summary

Flatten the embedded recipes directory by moving 17 TOML files from `internal/recipe/recipes/<letter>/<name>.toml` to `internal/recipe/recipes/<name>.toml`, then updating the Go embed directive and all scripts/docs that reference the letter-based path structure.

## Approach

Straightforward file moves followed by pattern updates in Go code, Python scripts, shell scripts, GitHub workflows, and documentation. The Go embed directive change from `recipes/*/*.toml` to `recipes/*.toml` automatically handles the flattened structure.

## Files to Modify

- `internal/recipe/embedded.go` - Update embed directive from `recipes/*/*.toml` to `recipes/*.toml`; update comment on line 43
- `scripts/generate-registry.py` - Update PATH_PATTERN regex and embedded glob pattern
- `scripts/validate-all-golden.sh` - Remove first_letter logic for embedded recipe path lookup
- `.github/workflows/validate-embedded-deps.yml` - Remove first_letter logic for embedded recipe paths
- `docs/EMBEDDED_RECIPES.md` - Update recipe path example (line ~65)

Note: CONTRIBUTING.md and docs/BUILD-ESSENTIALS.md were listed in the issue but don't exist or don't contain the patterns mentioned.

## Files to Create

None

## Implementation Steps

- [x] Move all 17 TOML files from `internal/recipe/recipes/<letter>/` to `internal/recipe/recipes/`
- [x] Remove empty letter directories (c, g, l, m, n, o, p, r, z)
- [x] Update `internal/recipe/embedded.go`: change embed directive to `recipes/*.toml` and update path comment
- [x] Update `scripts/generate-registry.py`: modify PATH_PATTERN and discover_recipes() glob
- [x] Update `scripts/validate-all-golden.sh`: simplify embedded recipe path lookup
- [x] Update `.github/workflows/validate-embedded-deps.yml`: simplify embedded recipe path lookup
- [x] Update `docs/EMBEDDED_RECIPES.md`: fix recipe path example
- [x] Run `go test ./...` to verify changes (passes, pre-existing lint issues in unrelated files)
- [x] Run `python3 scripts/generate-registry.py` to verify website generation (171 recipes found)

## Success Criteria

- [x] All 17 recipe files exist at `internal/recipe/recipes/<name>.toml`
- [x] No letter directories remain under `internal/recipe/recipes/`
- [x] `go test ./...` passes (recipe tests pass; pre-existing lint issues in unrelated files)
- [x] `python3 scripts/generate-registry.py` succeeds
- [x] All path references in scripts and docs point to flat structure

## Open Questions

None
