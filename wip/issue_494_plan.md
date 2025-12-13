# Issue 494 Implementation Plan

## Summary

Extend the validation executor to support source builds by creating a specialized build container image with required build tools, and integrate validation into the source build generation flow with appropriate timeouts.

## Approach

The approach extends the existing `Executor.Validate()` method with a source-build-aware validation mode. Key differences from bottle validation:

1. **Container image**: Use a larger image with build tools pre-installed instead of `debian:bookworm-slim`
2. **Longer timeouts**: Source builds take much longer than bottle extraction
3. **Build script generation**: Generate a script that runs the actual build steps instead of just `tsuku install`
4. **Network isolation**: Keep `--network=host` during download phase, but build phase runs with network access (required for go get, cargo fetch during builds)

### Alternatives Considered

- **Pre-download all sources then run with --network=none**: More secure but complex; source builds may need network for dependency resolution (cargo, go mod). Not chosen for initial implementation.
- **Build custom Docker image with all possible build tools**: Would bloat image significantly. Not chosen; better to install tools on-demand based on recipe's build system.
- **Use nix-portable for hermetic builds**: Already exists for nix recipes but adds complexity. Not chosen for Homebrew source builds.

## Files to Modify

- `internal/validate/executor.go` - Add source build validation method and build-tool image support
- `internal/validate/executor_test.go` - Add tests for source build validation
- `internal/builders/homebrew.go` - Enable validation in buildFromSource() flow

## Files to Create

- `internal/validate/source_build.go` - Source build validation logic (separated for clarity)
- `internal/validate/source_build_test.go` - Tests for source build validation

## Implementation Steps

- [ ] Add SourceBuildValidationImage constant for build-tool image
- [ ] Create ValidateSourceBuild method on Executor with source-specific options
- [ ] Implement buildSourceBuildScript() to generate build validation script
- [ ] Add SourceBuildLimits with longer timeout (15 min vs 5 min)
- [ ] Update buildFromSource() to call validation
- [ ] Add unit tests for source build validation script generation
- [ ] Add integration test with a simple autotools formula
- [ ] Run full test suite and verify build

Mark each step [x] after it is implemented and committed.

## Testing Strategy

- **Unit tests**: Test script generation for different build systems (autotools, cmake, cargo, go)
- **Unit tests**: Test build-tool installation logic for different build systems
- **Integration tests**: Validate a simple autotools formula (jq) end-to-end
- **Manual verification**: Generate and validate neovim (cmake + resources) on Linux

## Risks and Mitigations

- **Long validation times**: Source builds are slow. Mitigation: Use 15-minute timeout, make validation optional with flag.
- **Missing build dependencies in container**: Some formulas need additional libraries. Mitigation: Install common build dependencies in base script; LLM repair loop can add missing deps.
- **Platform mismatch**: Linux container can't validate macOS-specific steps. Mitigation: Only validate Linux steps; document limitation.
- **Network-dependent builds**: Some build systems fetch deps during build. Mitigation: Keep network enabled during validation.

## Success Criteria

- [ ] Source builds are validated in isolated container
- [ ] Build tools (make, cmake, cargo, go) are available based on build system
- [ ] Patches and resources are applied before build
- [ ] Expected binaries are verified after build
- [ ] Verify command runs on built binaries
- [ ] Validation failures feed into LLM repair loop
- [ ] Unit tests pass for all new code
- [ ] Integration test with jq source build passes
- [ ] golangci-lint passes

## Open Questions

None blocking - the design is clear from the existing bottle validation infrastructure.
