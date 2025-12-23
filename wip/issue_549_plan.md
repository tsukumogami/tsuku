# Issue 549 Implementation Plan

## Summary

Create a cmake recipe using the `homebrew` action to install CMake from Homebrew bottles across all 4 platforms (Linux x86_64, Linux arm64, macOS Intel, macOS Apple Silicon), enabling tsuku to provide CMake as a build essential.

## Approach

Following the established pattern from other build essential recipes (make, pkg-config, zlib, openssl), this recipe will:

1. Use the `homebrew` action to download and install pre-built CMake bottles from Homebrew's GitHub Container Registry
2. Follow the same structure as other homebrew-based recipes (zlib.toml, openssl.toml, make.toml)
3. Install CMake binaries using `install_binaries` with `install_mode = "directory"` to preserve the full CMake installation structure
4. Add platform-specific validation through the existing CI build-essentials workflow
5. Update the design doc's mermaid diagrams to mark issues #552 (openssl) and #554 (curl) as done

The `cmake_build` action already exists in the codebase and declares implicit dependencies on cmake, so once the recipe is added, tools using `cmake_build` will automatically provision cmake.

### Alternatives Considered

**Alternative 1: Build CMake from source**
- Why not chosen: CMake is a complex build system with many dependencies. Using Homebrew bottles provides pre-built, tested binaries that are known to relocate properly.

**Alternative 2: Download from cmake.org**
- Why not chosen: Would require manual handling of platform-specific downloads, extraction, and relocation. Homebrew bottles provide a unified interface across platforms.

## Files to Modify

- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/DESIGN-dependency-provisioning.md` - Update mermaid diagrams to mark #552 and #554 as done (green), update #549 status from ready to done

## Files to Create

- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/c/cmake.toml` - CMake recipe using homebrew action

## Implementation Steps

- [ ] Create cmake.toml recipe following the pattern from make.toml and pkg-config.toml
  - Use `homebrew` action with formula = "cmake"
  - Use homebrew version provider
  - Install binaries with `install_mode = "directory"` to preserve full cmake installation
  - Add verify section with `cmake --version` command
- [ ] Update DESIGN-dependency-provisioning.md mermaid diagrams
  - In Milestone 2 (Build Environment) diagram: Change class for I552 and I554 from ready to done
  - In Milestone 3 (Full Integration) diagram: Change class for I554 from ready to done
  - Update the status markers in table if needed
- [ ] Add cmake verification function to test/scripts/verify-tool.sh
  - Test `cmake --version` output
  - Test configuring a simple CMakeLists.txt project
- [ ] Validate recipe locally by installing cmake and running verification script
- [ ] Commit changes and push to feature branch

## Testing Strategy

**Manual testing:**
1. Build tsuku locally
2. Install cmake: `./tsuku install cmake`
3. Verify installation: `./test/scripts/verify-tool.sh cmake`
4. Verify relocation: `./test/scripts/verify-relocation.sh cmake`
5. Test on a simple CMake project to ensure it can configure and build

**CI testing:**
The existing `.github/workflows/build-essentials.yml` workflow has infrastructure for testing homebrew-based tools across platforms. The workflow will need to be updated to include cmake in the test matrix once the recipe is merged.

**Platform coverage:**
- Linux x86_64 (ubuntu-latest)
- macOS Intel (macos-15-intel)
- macOS Apple Silicon (macos-14)
- Note: arm64_linux excluded per existing workflow pattern (Homebrew doesn't publish bottles for this platform)

## Risks and Mitigations

**Risk 1: CMake binaries may not relocate properly**
- Mitigation: Homebrew bottles for cmake are widely used and tested. The homebrew action includes placeholder replacement and RPATH fixup. Verification scripts will catch relocation issues before merge.

**Risk 2: CMake installation may be large**
- Mitigation: CMake is a build essential that will be used by multiple recipes. The disk space cost is justified by enabling cmake-based builds.

**Risk 3: Platform-specific CMake behavior differences**
- Mitigation: Use the same CMake version across all platforms via Homebrew bottles. CI testing on all 3 platforms will catch platform-specific issues.

## Success Criteria

- [ ] cmake.toml recipe exists and follows established patterns
- [ ] `tsuku install cmake` succeeds on all supported platforms
- [ ] `cmake --version` works from relocated path
- [ ] cmake can configure a simple CMakeLists.txt project
- [ ] Verification scripts pass (verify-tool.sh, verify-relocation.sh, verify-no-system-deps.sh)
- [ ] Mermaid diagrams in DESIGN-dependency-provisioning.md correctly show #552 and #554 as done

## Open Questions

None - the pattern is well-established from previous homebrew recipes.
