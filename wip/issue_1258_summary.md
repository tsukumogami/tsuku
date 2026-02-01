# Issue 1258 Summary

## What Was Implemented

Replaced the grep-based platform detection in test-changed-recipes.yml with `tsuku info --json --metadata-only --recipe` calls. The workflow now uses the CLI's own platform resolution logic to determine which runners each recipe should run on, instead of pattern-matching TOML fields with grep.

## Changes Made

- `.github/workflows/test-changed-recipes.yml`:
  - Added Go setup and tsuku build steps to the matrix detection job
  - Replaced grep-based `linux_only` / `os_mapping` checks with `tsuku info` platform queries
  - Changed macOS recipe list from bare tool name strings to `{tool, path}` objects
  - Updated macOS job to consume object format (uses explicit path instead of deriving from tool name)
  - Fixed subshell variable scoping in macOS job by using a temp file for failure tracking

## Key Decisions

- Fallback to "all platforms" if `tsuku info` fails: prevents a broken recipe from blocking CI entirely; the recipe will just fail on the platform it doesn't support, which is the same behavior as before
- Unified macOS list format to objects: eliminates the path derivation logic that assumed `recipes/{letter}/{name}.toml`, which wouldn't work for embedded recipes under `internal/recipe/recipes/`

## Test Coverage

- No new Go tests (workflow-only change)
- Verified locally: `tsuku info` returns correct platform data for cross-platform (fzf) and Linux-only (btop) recipes

## Known Limitations

- The matrix detection job now requires Go setup + build, adding ~30-60s to the detection step
- Darwin-only recipes (no Linux support) will now correctly skip the Linux matrix, but this is untested in CI since no current recipes are darwin-only
