# Issue 445 Implementation Plan

## Summary

Implement the `npm_exec` primitive action that executes npm/Node.js builds with deterministic configuration, using `npm ci` with lockfile enforcement and security hardening.

## Approach

Create a new `npm_exec.go` following the existing action patterns (similar to `npm_install.go` and `cargo_install.go`). The action will:
1. Accept parameters for deterministic npm execution
2. Use `npm ci` instead of `npm install` when `use_lockfile: true`
3. Set `SOURCE_DATE_EPOCH` for build timestamp reproducibility
4. Validate Node.js version if specified
5. Use isolated npm cache directory

This is an ecosystem primitive - it cannot be decomposed further within tsuku but achieves determinism through npm-specific configuration.

### Alternatives Considered

- **Extend npm_install**: Rejected - npm_exec has fundamentally different semantics (build execution vs package installation)
- **Make npm_exec decomposable**: Rejected - ecosystem primitives are terminal actions per design doc

## Files to Create

- `internal/actions/npm_exec.go` - Main action implementation
- `internal/actions/npm_exec_test.go` - Unit tests

## Files to Modify

- `internal/actions/action.go` - Register `NpmExecAction` in init()
- `internal/actions/decomposable.go` - Register `npm_exec` as primitive (if ecosystem primitives belong there)

## Implementation Steps

- [x] Create `npm_exec.go` with `NpmExecAction` struct and `Name()` method
- [x] Implement parameter parsing and validation
- [x] Implement Node.js version detection and validation
- [x] Implement `Execute()` with `npm ci` and deterministic flags
- [x] Register action in `action.go`
- [x] Add unit tests for parameter validation
- [x] Add unit tests for Node.js version validation
- [x] Add unit tests for deterministic flag behavior
- [x] Run go vet, go test, go build

## Testing Strategy

- Unit tests: Parameter validation, Node.js version parsing
- Mock tests: Test command construction without actual execution
- Test error cases: missing params, invalid node version, npm not found

## Risks and Mitigations

- **Node.js/npm not installed**: Action will fail with clear error message
- **Lockfile format incompatibility**: Document npm version requirements

## Success Criteria

- [x] `NpmExecAction` implements `Action` interface
- [x] Action registered in action registry
- [x] `npm_exec` registered as primitive
- [x] All parameter validation working
- [x] Node.js version validation working
- [x] Tests pass, no lint errors

## Open Questions

None - the ecosystem research document provides comprehensive guidance.
