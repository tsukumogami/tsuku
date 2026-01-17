# Implementation Context for Issue #961

## Summary

**Problem**: Constrained evaluation for pipx_install loses hashes because PipConstraints only stores package→version mappings.

**Root Cause**: `constraints.go:ParsePipRequirements` extracts versions but not hashes. When `pipx_install.go:decomposeWithConstraints` reconstructs requirements, it generates placeholder `--hash=sha256:0`.

**Fix Pattern**: Follow Go/Cargo/npm/gem/cpan approach - store the full `locked_requirements` string rather than parsing it.

## Key Files

- `internal/executor/constraints.go` - Constraint extraction (add `PipRequirements` field handling)
- `internal/actions/decomposable.go` - EvalConstraints struct (add `PipRequirements string` field)
- `internal/actions/pipx_install.go` - Decomposition (use stored requirements directly)

## Implementation Approach

1. Add `PipRequirements string` field to `EvalConstraints` (like `GoSum`, `CargoLock`)
2. Store full `locked_requirements` string during extraction (not just parsed versions)
3. Use stored requirements directly in `decomposeWithConstraints`
4. Keep `PipConstraints map[string]string` for version lookups (used by `GetPipConstraint`)

## Dependencies

None - this is a self-contained fix within the constraint pinning system.

## Testing

- Unit tests for constraint extraction with hashes
- Verify golden file validation passes for pipx recipes (black, httpie, meson, poetry, ruff)
