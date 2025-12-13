# Issue 446 Implementation Plan

## Summary

Implement `pip_install` as a Tier 2 Ecosystem Primitive that executes Python/pip installs with deterministic configuration using hash-checking and isolated venvs.

## Approach

Create a new primitive action that:
1. Creates an isolated venv for the package
2. Installs with deterministic flags (`--require-hashes`, `--no-deps`, `--only-binary :all:`)
3. Sets environment variables for reproducibility (`SOURCE_DATE_EPOCH`, `PYTHONDONTWRITEBYTECODE=1`)
4. Validates Python version before installation
5. Registers as a primitive (non-decomposable)

The action follows the existing `pipx_install` pattern but executes pip directly with venv isolation instead of relying on pipx, giving full control over deterministic flags.

### Alternatives Considered
- **Extend pipx_install**: pipx doesn't expose `--require-hashes` and `--no-deps` flags cleanly, making deterministic execution difficult.
- **Use pip-tools at exec time**: Would require pip-tools as a dependency; better to accept pre-computed locked requirements.

## Files to Modify
- `internal/actions/action.go` - Add Register call for PipInstallAction
- `internal/actions/decomposable.go` - Add "pip_install" to primitives map

## Files to Create
- `internal/actions/pip_install.go` - Main action implementation
- `internal/actions/pip_install_test.go` - Unit tests

## Implementation Steps
- [ ] Create `pip_install.go` with action struct and Execute method
- [ ] Register as primitive in `decomposable.go`
- [ ] Register in `action.go` init function
- [ ] Add unit tests for parameter validation
- [ ] Add unit tests for deterministic flag behavior
- [ ] Verify build and all tests pass

## Testing Strategy
- Unit tests: Test parameter validation (required fields, version format)
- Unit tests: Verify environment variables are set correctly
- Unit tests: Test `--require-hashes` flag is applied when use_hashes=true
- Unit tests: Test Python version validation
- Manual test: Defer integration test to future issue (requires pip environment)

## Risks and Mitigations
- **Risk**: pip not available in execution environment
  - **Mitigation**: Create venv which provides pip automatically; document requirement for Python
- **Risk**: Hash checking fails due to wheel availability
  - **Mitigation**: Use `--only-binary :all:` to enforce wheels; document limitation

## Success Criteria
- [ ] `pip_install.go` created with full Execute() implementation
- [ ] Action registered in registry
- [ ] Action registered as primitive (non-decomposable)
- [ ] Parameters match issue spec: source_dir, requirements, constraints, python_version, use_hashes, output_dir
- [ ] Environment variables set: SOURCE_DATE_EPOCH, PYTHONDONTWRITEBYTECODE=1
- [ ] Uses --require-hashes when use_hashes: true
- [ ] Uses --no-build-isolation for reproducibility
- [ ] Unit tests pass
- [ ] Build and vet pass

## Open Questions
None - implementation follows ecosystem_pip.md research document.
