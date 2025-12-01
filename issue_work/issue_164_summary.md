# Issue 164 Summary

## Problem
Some CPAN distributions have names that don't follow the standard convention. For example, the `ack` distribution contains `App::Ack` module. The existing `distributionToModule()` conversion would give `ack` instead of `App::Ack`, causing cpanm to fail.

## Solution
Added an optional `module` parameter to cpan_install action. When provided, this module name is used directly with cpanm instead of converting the distribution name.

## Changes
- `internal/actions/cpan_install.go`:
  - Added optional `module` parameter handling
  - Added `isValidModuleName()` validation function
  - Updated function documentation
- `internal/actions/cpan_install_test.go`:
  - Added `TestIsValidModuleName` with 25 test cases
  - Added `TestCpanInstallAction_Execute_ModuleParameter` for validation
  - Added `TestCpanInstallAction_Execute_WithModuleParameter` for integration

## Usage Example
```toml
[[steps]]
action = "cpan_install"
distribution = "ack"        # Used by metacpan version provider
module = "App::Ack"         # Used when calling cpanm
executables = ["ack"]
```

## Testing
- All unit tests pass (17 packages)
- Build succeeds
- New module validation tested with valid/invalid inputs including security cases
