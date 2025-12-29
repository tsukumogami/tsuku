# Issue 726 Summary

## What Was Implemented

Added testdata recipes for SpatiaLite and its dependencies (GEOS, PROJ, libxml2) to exercise the full build ecosystem. These recipes demonstrate CMake builds, GitLab/GNOME hosting, and complex dependency chains.

## Changes Made

- `testdata/recipes/geos-source.toml`: CMake build from GitHub, uses Homebrew for version resolution
- `testdata/recipes/proj-source.toml`: CMake build with sqlite-source dependency
- `testdata/recipes/libxml2-source.toml`: autoconf build from GNOME download server
- `testdata/recipes/spatialite-source.toml`: fossil_archive with full dependency chain (4 dependencies)
- `docs/BUILD-ESSENTIALS.md`: Added fossil_archive action documentation
- `docs/DESIGN-fossil-archive.md`: Updated status to Current, marked #726 as done

## Key Decisions

- **Homebrew version resolution**: All recipes use Homebrew formulas for version lookup since they're proven reliable and avoid rate limiting issues
- **No github_archive action**: Used download + extract pattern instead since github_archive doesn't exist in the codebase
- **Minimal configure options**: Disabled optional features (TIFF, CURL, Python, etc.) to simplify builds and reduce dependencies

## Trade-offs Accepted

- libxml2 URL hardcodes major.minor version (2.15) - acceptable for testdata recipe
- Recipes disable optional features to simplify builds - acceptable since purpose is testing fossil_archive

## Test Coverage

- Existing unit tests pass
- Version resolution verified for geos-source and spatialite-source
- Build integration not tested (testdata recipes for CI validation)

## Known Limitations

- libxml2 URL path contains hardcoded "2.15" which may need updating for future major versions
- Full integration test (tsuku install spatialite-source) requires significant build time

## Future Improvements

- Could add CI workflow job to validate spatialite dependency chain builds
- Could add version URL pattern support for libxml2 to handle major.minor changes
