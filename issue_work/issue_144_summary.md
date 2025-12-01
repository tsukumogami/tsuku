# Issue 144 Summary

## What Was Implemented

Added Perl integration tests to the test matrix, enabling automated testing of Perl runtime and CPAN tool installations in CI.

## Changes Made
- `test-matrix.json`: Added T50 (perl) and T51 (ack) test cases
- `internal/actions/cpan_install_test.go`: Fixed test isolation for environments with perl installed

## Test Cases Added

### T50: perl (in CI linux)
- Tier 2 (runtime category with golang, nodejs, rust)
- Tests relocatable-perl installation from tsuku-registry
- Features: download_archive, directory install mode

### T51: ack (in scheduled)
- Tier 5 (package manager install category)
- Tests cpan_install action with App-Ack distribution
- Features: cpan_install, metacpan version detection, perl dependency
- In scheduled array pending ack recipe addition to tsuku-registry (#14)

## Key Decisions
- **T50 in CI linux only**: Perl downloads are ~50MB; add to macOS later if needed
- **T51 in scheduled**: ack recipe not yet in registry; moved to scheduled to avoid CI failure

## Trade-offs Accepted
- **Partial coverage**: Only perl runtime tested in CI until ack recipe is added
- **Linux-only for now**: Perl tests run on Linux; macOS support can be added later

## Test Coverage
- Perl runtime installation verified in CI
- cpan_install ready for testing once ack recipe is added

## Known Limitations
- T51 (ack) requires tsuku-registry#14 (popular Perl tool recipes) to be completed
- macOS Perl tests not added yet

## Future Improvements
- Add ack to CI linux once tsuku-registry#14 merges
- Consider adding macOS Perl tests
