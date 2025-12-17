# Issue 612 Summary

## What Was Implemented

The go_install action was already fully evaluable when this issue was assigned. The implementation includes a complete Decompose method that captures go.sum at eval time and produces a go_build step for deterministic execution, matching the patterns established by cargo_install, npm_install, pip_install, and gem_install.

## Changes Made

No code changes were necessary. The implementation was complete:
- `internal/actions/go_install.go`: Already has Decompose method (lines 264-402)
- `internal/actions/go_build.go`: Primitive action already exists with full implementation
- `internal/actions/util.go`: Helper functions ResolveGo, ResolveGoVersion, GetGoVersion already present
- `internal/actions/action.go`: Both actions already registered (lines 142, 146)

## Key Decisions

### Decision: Use existing implementation
**Rationale**: After thorough code review and testing, confirmed that all acceptance criteria were already met:
- ✅ go_install.Decompose() captures go.sum content at eval time
- ✅ Dependency resolution via `go get` with MVS algorithm
- ✅ Exec phase uses isolated GOMODCACHE with CGO_ENABLED=0
- ✅ Build flags include `-trimpath -buildvcs=false`
- ✅ Go toolchain version captured in plan
- ✅ Ecosystem primitives correctly marked as non-deterministic (residual non-determinism per design doc)

### Decision: No modifications to IsDeterministic
**Rationale**: The ecosystem documentation (ecosystem_go.md) explicitly documents residual non-determinism from compiler versions, CGO, and timestamps. Following the pattern from pip_exec and gem_exec, go_build correctly returns false for IsDeterministic, as it's an ecosystem primitive with residual non-determinism rather than a core primitive.

## Trade-offs Accepted

None - implementation was already optimal.

## Test Coverage

- Existing tests: 8 decomposition tests in go_install_test.go
- Existing tests: 18 build/integration tests in go_build_test.go
- All tests passing: `go test -test.short ./internal/actions/` - PASS
- No new tests needed: comprehensive coverage already exists

## Known Limitations

From the design documentation (ecosystem_go.md):

1. **Compiler version differences**: Different Go versions may produce different binaries
   - Mitigation: Go version is captured in the plan for reproducibility

2. **CGO-enabled builds**: Introduce C toolchain non-determinism
   - Mitigation: CGO disabled by default (CGO_ENABLED=0)

3. **Platform-specific code**: Different GOOS/GOARCH use different source files
   - Expected behavior: Platform is captured in execution context

These are documented characteristics of the Go ecosystem, not bugs in the implementation.

## Future Improvements

The following features are tracked in separate milestones of DESIGN-deterministic-resolution.md:

1. **CLI integration** (Milestone 1): `tsuku eval` command to generate plans
2. **Plan execution** (Milestone 3): `tsuku install --plan` to install from exported plans
3. **Offline execution**: Support for air-gapped environments with pre-downloaded modules

These are separate from making go_install evaluable (this issue), which is now complete.

## Baseline Test Failures

The two pre-existing test failures noted in the baseline are unrelated to go_install:

1. `TestSandboxIntegration`: Expects `--plan` flag (Milestone 3 feature)
2. `TestEvalPlanCacheFlow`: External resource 404 error

Neither blocks the completion of this issue.
