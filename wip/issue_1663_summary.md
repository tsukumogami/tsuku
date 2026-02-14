# Issue 1663 Summary

## What Was Implemented

Removed 19 recipe exclusions from `execution-exclusions.json` that were blocking CI due to glibc version mismatch segfaults. The underlying issue was resolved when GitHub updated `ubuntu-latest` to Ubuntu 24.04 (glibc 2.39) in January 2025.

## Changes Made

- `testdata/golden/execution-exclusions.json`: Removed 19 recipes that were excluded due to #1663
  - 18 recipes with "Segfault (exit 139) - glibc version mismatch with Homebrew bottles"
  - 1 recipe (sqlite) with "Crash (abort trap) - Homebrew bottle issue on macOS"

## Key Decisions

- **Simplified approach**: Rather than implementing a `minimum_glibc` field as originally planned, relied on GitHub's infrastructure update that already resolved the underlying issue.
- **Defer minimum_glibc feature**: The user-facing glibc version constraint feature remains a good idea for better error messages but was deferred as it's a larger undertaking.

## Trade-offs Accepted

- **No minimum_glibc field**: Users on older systems will still see cryptic segfaults instead of clear "requires glibc 2.35" messages. This is acceptable because:
  1. The immediate CI issue is resolved
  2. The feature can be added in a follow-up issue
  3. Most users are on recent systems with compatible glibc

## Test Coverage

- No new tests added (configuration-only change)
- Existing tests pass

## Known Limitations

- Recipes may still fail for users on older systems (glibc < 2.35)
- No programmatic detection of glibc version requirements

## Future Improvements

- Add `minimum_glibc` field to recipes for better user-facing error messages
- Implement glibc version detection in platform checks
