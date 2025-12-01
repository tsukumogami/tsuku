# Issue 161 Summary

## Problem
cpan_install action passed distribution names (e.g., `Perl-Critic`) directly to cpanm, but cpanm only understands module names (e.g., `Perl::Critic`) or full tarball paths.

## Solution
Added `distributionToModule()` helper function that converts distribution format (hyphens) to module format (double colons) before calling cpanm.

## Changes
- `internal/actions/cpan_install.go`:
  - Added `distributionToModule()` function
  - Applied conversion when building cpanm target
- `internal/actions/cpan_install_test.go`:
  - Added `TestDistributionToModule` with 6 test cases

## Testing
- All unit tests pass (17 packages)
- Build succeeds
- New function tested with various distribution name formats

## Limitations
- Assumes standard CPAN naming convention (distribution hyphens map to module double-colons)
- Non-standard distributions that don't follow this convention won't work (documented limitation)
