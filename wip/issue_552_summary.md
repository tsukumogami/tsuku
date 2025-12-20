# Issue 552 Summary

## What Was Implemented
Added OpenSSL 3.x recipe to enable TLS/crypto support for dependent tools like curl. Fixed homebrew action to support versioned formulas using @ symbol (e.g., openssl@3).

## Changes Made
- `internal/recipe/recipes/o/openssl.toml`: Created new recipe using homebrew action
  - Uses openssl@3 formula from Homebrew
  - Declares zlib as dependency
  - Installs shared libraries (libssl, libcrypto), CLI binary, and headers
  - Configured to install on all 4 platforms via GHCR bottles

- `internal/actions/homebrew.go`: Fixed support for versioned formulas
  - Added formulaToGHCRPath() helper function
  - Converts @ to / for GHCR repository paths (openssl@3 → openssl/3)
  - Updated all GHCR URL construction to use this conversion
  - Fixes token requests, manifest queries, and blob downloads

- `docs/DESIGN-dependency-provisioning.md`: Updated milestone progress
  - Marked issues 540, 541, 542, 553 as done (were incorrectly marked as external)
  - Updated second chart to mark 553 as done and 554 as ready

## Key Decisions
- **Use openssl@3 instead of openssl@1.1**: OpenSSL 1.1.x reached end-of-life in September 2023 and no longer receives security patches. OpenSSL 3.x is the current stable branch.

- **Fix homebrew action for all versioned formulas**: Rather than working around the @ symbol limitation, implemented a proper fix that benefits all future versioned formulas (python@3.12, node@20, etc.).

- **Use homebrew bottles instead of source build**: Homebrew provides pre-built, tested bottles for all platforms. Building from source would require Perl, complex configuration, and 5-10 minutes vs <30 seconds for bottles.

## Trade-offs Accepted
- **pkg-config relocation incomplete**: The .pc files in `lib/pkgconfig/` currently contain hardcoded temporary paths instead of final installation paths. This is a pre-existing issue affecting all homebrew bottles with pkg-config files (zlib, libpng, etc.), not specific to OpenSSL. The issue should be addressed by enhancing homebrew_relocate action separately rather than blocking this PR.

## Test Coverage
- All existing tests pass (22 packages)
- OpenSSL installs successfully and `openssl version` works
- Libraries and binary properly relocated
- zlib dependency automatically installed
- No new test files added (recipe testing happens in CI)

## Known Limitations
- pkg-config files (.pc) contain hardcoded temp directory paths
- This affects all Homebrew bottles with .pc files, not just OpenSSL
- Workaround: Tools using pkg-config need explicit PKG_CONFIG_PATH
- Proper fix: Enhance homebrew_relocate to handle .pc file path rewriting

## Future Improvements
- Fix pkg-config file relocation in homebrew_relocate action
- Add automated tests for versioned formula GHCR path conversion
- Consider adding openssl@1.1 recipe if legacy tools require it

## Progress Update
This PR completes the work needed to unblock issue #554 (curl recipe):
- ✅ #551: setup_build_env action (completed in previous PR)
- ✅ #552: openssl recipe (this PR)
- ⏩ #554: curl recipe now ready to proceed
