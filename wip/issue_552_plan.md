# Issue 552 Implementation Plan

## Summary
Create an OpenSSL recipe using the `homebrew` action to install pre-built Homebrew bottles. OpenSSL provides TLS/crypto support for dependent tools like curl.

## Approach
Use Homebrew's `openssl@3` formula which provides pre-built bottles for all platforms. This avoids the complexity of building from source (requires Perl, complex configuration) while ensuring tested, working binaries.

### Alternatives Considered
- **Build from source**: Rejected due to complexity (Perl dependency, long build times, configuration complexity)
- **Use openssl@1.1**: Rejected due to EOL status (no security patches since Sept 2023)
- **System OpenSSL**: Rejected as it violates tsuku's self-contained philosophy and macOS ships LibreSSL

## Files to Create
- `internal/recipe/recipes/o/openssl.toml` - Recipe defining OpenSSL installation

## Implementation Steps
- [x] Create openssl.toml recipe with homebrew action
- [x] Declare zlib dependency
- [x] Fix homebrew action to support versioned formulas (@3 → /3)
- [x] Test local installation and verify binary works
- [x] Verify RPATH relocation for libraries
- [ ] Verify pkg-config integration (NOTE: pre-existing issue with .pc file relocation)
- [x] Run full test suite locally
- [x] Commit changes

## Testing Strategy
- Local testing: `./tsuku install openssl && openssl version`
- RPATH verification: Ensure libraries use relative paths ($ORIGIN/@loader_path)
- pkg-config: Verify `pkg-config --exists openssl` and paths are correct
- CI testing: All 4 platforms (Linux x86_64, macOS Intel, macOS Apple Silicon, Sandbox)

## Risks and Mitigations
- **RPATH complexity**: OpenSSL has interdependent libraries (libssl → libcrypto → libz). Mitigated by existing homebrew_relocate action which handles this pattern.
- **Homebrew changes**: Formula might change structure. Mitigated by pinning to `openssl@3` and CI catching breakage early.
- **pkg-config files**: Might be missing or malformed. Mitigated by explicit verification tests.

## Success Criteria
- [ ] `tsuku install openssl` succeeds on all platforms
- [ ] `openssl version` command works
- [ ] Recipe declares zlib dependency
- [ ] Libraries relocate correctly (no hardcoded paths)
- [ ] pkg-config reports correct openssl paths
- [ ] CI passes on all 4 platforms
- [ ] Unblocks issue #554 (curl recipe)
