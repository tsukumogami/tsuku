# Issue 765 Summary

## What Was Implemented

Implemented `ExtractPackages()` function in the sandbox package that extracts package requirements from a filtered installation plan, grouped by package manager. This enables the sandbox container building feature to know which system packages need to be pre-installed in containers.

## Changes Made
- `internal/sandbox/packages.go`: New file implementing ExtractPackages function
- `internal/sandbox/packages_test.go`: Comprehensive unit tests covering all package managers

## Key Decisions
- Placed in sandbox package: The function is specifically for sandbox container building, so it belongs with the sandbox executor code
- Returns nil for no system deps: Allows callers to distinguish "no packages needed" (nil) from "empty package list" (empty map)
- Separate keys for each PM: Uses "apt", "brew", "dnf", etc. as keys rather than grouping by linux family, matching the design doc specification

## Trade-offs Accepted
- brew_install and brew_cask both aggregate to "brew" key: This is intentional per the design doc; the container builder will use `brew install` for both

## Test Coverage
- New tests added: 19 test functions covering all package managers and edge cases
- Coverage includes: nil plans, empty plans, all PM types (apt, brew, dnf, pacman, apk, zypper), aggregation across multiple steps, mixed actions

## Known Limitations
- Configuration actions (apt_repo, apt_ppa, dnf_repo) are tracked as "has system deps" but don't contribute packages to the map - this is correct behavior since they prepare for subsequent installs

## Future Improvements
Container building integration (issue #768) will use this function to build minimal containers with the required packages pre-installed.
