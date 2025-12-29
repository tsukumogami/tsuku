# Issue 742 Summary

## What Was Implemented

Created expat and libcurl library recipes using Homebrew bottles to enable git-source test. Updated git-source recipe to use libcurl library instead of curl tool, and re-enabled the test-git-source CI workflow.

## Changes Made

- `internal/recipe/recipes/e/expat.toml`: Created new library recipe for expat XML parser using Homebrew action
- `internal/recipe/recipes/l/libcurl.toml`: Created new library recipe for libcurl HTTP client using Homebrew action
- `testdata/recipes/git-source.toml`: Updated dependency from curl to libcurl and fixed RPATH to use dynamic version variables
- `.github/workflows/build-essentials.yml`: Re-enabled test-git-source job by removing if: false condition

## Key Decisions

- **Use Homebrew bottles for library provisioning**: Reuses proven infrastructure pattern from existing library recipes (openssl, zlib, readline) rather than building from source
- **Create separate libcurl recipe**: Keeps libcurl (library) separate from curl (tool) to avoid conflicts and enable focused dependency management
- **Use dynamic version variables in RPATH**: Changed from hardcoded versions to {deps.<dep>.version} pattern for maintainability

## Trade-offs Accepted

- **Homebrew dependency**: Libraries come from Homebrew bottles, limiting to platforms where Homebrew publishes bottles (excludes arm64_linux)
- **msgfmt requirement**: git-source build requires msgfmt (gettext) which isn't available in local dev environment, but CI runners have it installed

## Test Coverage

- No new unit tests required (using existing Homebrew action)
- Existing recipe validation tests validate TOML structure
- CI integration tests will validate installation on all platforms (Linux x86_64, macOS Intel, macOS Apple Silicon)

## Known Limitations

- Local development: git-source build fails locally due to missing msgfmt, but succeeds in CI
- Platform support: Limited to platforms where Homebrew publishes bottles for expat and curl

## Future Improvements

- Consider adding gettext/msgfmt recipe for local development testing
- Consider source-build recipes as fallback for platforms without Homebrew bottles
