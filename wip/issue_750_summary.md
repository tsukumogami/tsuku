# Issue 750 Summary

## What Was Done

1. **Added homebrew redundancy detection** (`internal/version/redundancy.go`)
   - Added formula-matching logic to detect when recipes declare `source = "homebrew"` while also using the `homebrew` action with the same formula
   - This extends the existing redundancy detection that covers 8 other action types

2. **Added unit tests** (`internal/version/redundancy_test.go`)
   - Redundant case: same formula in version and action
   - Redundant case: empty formulas (both default to metadata.name)
   - Non-redundant case: homebrew version with different action (e.g., cmake_build)
   - Non-redundant case: different formulas between version and action

3. **Fixed 19 recipes** with redundant homebrew version blocks:
   - bash, cmake, gcc-libs, gdbm, geos, git, libpng, libxml2, libyaml, make
   - openssl, patchelf, pkg-config, pngcrush, proj, readline, spatialite, sqlite, zlib

4. **Preserved 3 recipes** that correctly use homebrew for version resolution but build from source:
   - curl, ninja, ncurses

## Acceptance Criteria

- [x] Add homebrew to `actionInference` map in `redundancy.go`
- [x] Include formula matching: only flag as redundant when `[version].formula` matches step `formula`
- [x] Add unit tests covering all cases
- [x] `tsuku validate --strict` on affected recipes shows warning (now fixed)

## Note

After removing redundant `[version]` blocks, a separate validation warning appears about `github_repo`. This is a pre-existing issue in validation logic (false positive for homebrew recipes) and should be tracked separately.
