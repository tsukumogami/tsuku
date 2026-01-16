# Implementation Context: Issue #921

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Summary

**Title**: feat(eval): add constrained evaluation skeleton with pip support
**Tier**: testable
**Dependencies**: None (this is the first issue in the milestone)

## What This Issue Accomplishes

This issue implements the foundation for constrained evaluation - the ability to pass version constraints from existing golden files to `tsuku eval` so all evaluation code runs but dependency resolution produces deterministic versions.

### Key Components to Implement

1. **EvalConstraints struct**: Data structure to hold constraints extracted from golden files
2. **ExtractConstraints function**: Parse golden files to extract pip constraints (locked_requirements)
3. **EvalContext extension**: Add Constraints field to EvalContext
4. **--pin-from CLI flag**: Add flag to tsuku eval command
5. **pip_exec constraint support**: Modify pipx_install.Decompose() to use constraints when available

### Design References

From the design document:

```go
// EvalConstraints holds version constraints extracted from golden files
type EvalConstraints struct {
    // PipConstraints maps package names to pinned versions
    // Extracted from locked_requirements in pip_exec steps
    PipConstraints map[string]string

    // GoSum contains the full go.sum content for go_build steps
    GoSum string

    // CargoLock contains the full Cargo.lock content for cargo_install steps
    CargoLock string

    // NpmLock contains package-lock.json content for npm_install steps
    NpmLock string
}
```

### CLI Interface

```bash
# Constrained evaluation for validation
tsuku eval httpie@3.2.4 --pin-from golden.json --os darwin --arch arm64

# Normal evaluation (unconstrained, for generating new golden files)
tsuku eval httpie@3.2.4 --os darwin --arch arm64
```

### What Gets Exercised

- Recipe TOML parsing
- Version provider logic
- pipx_install.Decompose()
- pip constraint handling (new code path)
- Template expansion
- Platform filtering
- Step ordering

### Files to Modify (from design)

- `internal/actions/decomposable.go` - Add EvalConstraints struct, extend EvalContext
- `internal/executor/constraints.go` (new) - ExtractConstraints implementation
- `cmd/tsuku/eval.go` - Add --pin-from flag definition and constraint loading
- `internal/actions/pipx_install.go` - Modify Decompose() for constraint support

### Exit Criteria

1. `--pin-from` flag is available on `tsuku eval`
2. Constraints can be extracted from golden files with pip_exec steps
3. When constraints are provided, pip_exec produces deterministic output matching the golden file
4. Tests cover constraint extraction and constrained evaluation
5. Existing unconstrained evaluation continues to work
