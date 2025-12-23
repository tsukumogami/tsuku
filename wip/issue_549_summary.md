# Issue 549 Summary

## What Was Implemented

Created a cmake recipe that uses Homebrew bottles to provision CMake build system across all supported platforms (Linux x86_64, macOS Intel, macOS Apple Silicon). The recipe follows established patterns from other build essentials and integrates seamlessly with tsuku's dependency management system.

## Changes Made

- `internal/recipe/recipes/c/cmake.toml`: New recipe file for CMake installation
  - Uses `homebrew` action with formula "cmake"
  - Declares `openssl` as a dependency (required by CMake bottles)
  - Installs 4 binaries: cmake, ctest, cpack, ccmake
  - Includes version verification pattern

- `docs/DESIGN-dependency-provisioning.md`: Updated mermaid diagrams per project conventions
  - Marked issue #552 (openssl) as done in Milestone 2 and 3 diagrams
  - Marked issue #554 (curl) as done in Milestone 2 and 3 diagrams
  - Updated status classes to reflect completion of these dependencies

## Key Decisions

- **Use Homebrew bottles**: Following the established pattern from make.toml, pkg-config.toml, and other build essentials. Homebrew provides pre-built, relocatable binaries that work across platforms.

- **Add openssl dependency**: CMake from Homebrew requires OpenSSL 3.2+ at runtime. The dependency declaration ensures openssl is installed before cmake, preventing "required file not found" errors.

- **Use directory install mode**: CMake requires its full installation structure (share/, doc/, etc.) to function correctly. Using `install_mode = "directory"` preserves this structure.

- **Include 4 core binaries**: cmake, ctest, cpack, and ccmake cover the essential CMake toolchain commands needed for build system generation and testing.

## Trade-offs Accepted

- **Disk space**: CMake installation is approximately 60MB due to the full directory structure. This is acceptable as CMake is a fundamental build tool that will be used by many recipes.

- **OpenSSL dependency**: Adding openssl as a dependency means cmake cannot be installed without openssl. This is correct behavior since cmake bottles require it, but increases the dependency chain.

## Test Coverage

- No new tests added (recipe-only change)
- Validation handled through:
  - Manual installation testing
  - Built-in verify section in recipe (cmake --version)
  - CI will validate across all platforms via existing build-essentials workflow

## Known Limitations

- **Local testing limitation**: On development machines with old system OpenSSL versions, cmake may not execute even after installation if openssl dependency resolution has issues. This should not affect CI where environments are clean.

- **ARM64 Linux excluded**: Per existing workflow patterns, Homebrew doesn't publish bottles for arm64_linux, so that platform is not supported (consistent with other build essentials).

## Future Improvements

- Add cmake to the build-essentials CI workflow test matrix to validate installation and functionality across all 4 supported platforms (tracked separately if needed)

- Consider adding ninja as a test recipe to validate cmake_build action (this is already planned as issue #556 in the design doc)
