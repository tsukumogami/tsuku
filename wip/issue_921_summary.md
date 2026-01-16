# Issue 921 Summary

## What Was Implemented

Implemented the foundational constrained evaluation infrastructure for golden file validation. Added `--pin-from` flag to `tsuku eval` that extracts version constraints from existing golden files and uses them during evaluation to produce deterministic output matching the golden file.

## Changes Made

- `internal/actions/decomposable.go`: Added `EvalConstraints` struct with fields for pip, go, cargo, npm, gem, and cpan constraints. Added `Constraints *EvalConstraints` field to `EvalContext`.

- `internal/executor/constraints.go` (new): Implemented `ExtractConstraints()` to parse golden files and extract pip constraints from `locked_requirements` in `pip_exec` steps. Added `ParsePipRequirements()` for parsing the requirements format with hash lines.

- `internal/executor/constraints_test.go` (new): Comprehensive tests for constraint extraction including pip_exec parsing, empty plans, invalid files, dependency extraction, and package name normalization.

- `internal/executor/plan_generator.go`: Added `Constraints *actions.EvalConstraints` field to `PlanConfig` and propagation to `EvalContext`.

- `cmd/tsuku/eval.go`: Added `--pin-from` flag and constraint loading logic. Constraints are extracted from the specified golden file and passed to plan generation.

- `internal/actions/pipx_install.go`: Added `decomposeWithConstraints()` method to generate pip_exec steps using pinned versions from constraints instead of live PyPI resolution.

## Key Decisions

- **EvalConstraints in decomposable.go**: Placed alongside `EvalContext` for co-location since constraints are evaluation context that flows through decomposition.

- **ExtractConstraints in executor package**: Operates on plan structures which are executor domain, not action domain.

- **Package name normalization**: Normalize to lowercase with hyphens (PEP 503) for consistent lookups regardless of input format.

- **Placeholder hashes in constrained output**: When using constraints, generate `--hash=sha256:0` as a placeholder since the exact hash doesn't matter for validation - only package versions need to match.

## Trade-offs Accepted

- **No hash preservation**: Constraints only preserve package versions, not hashes. This is acceptable because the purpose is deterministic version resolution, not security verification (which happens during actual installation).

- **Integration test deferred**: Full end-to-end integration test is deferred as it requires python-standalone to be installed for pip resolution.

## Test Coverage

- New tests added: 12 test functions in constraints_test.go
- Coverage: Constraint extraction, parsing, and helper functions fully tested
- All existing tests pass (5645 tests)

## Known Limitations

- Only pip constraints implemented in this issue. Go, Cargo, npm, gem, and cpan constraints will be implemented in subsequent issues (#922-#926).

- Constrained evaluation requires the golden file to have been generated for the same platform. Cross-platform constrained evaluation is not supported.

## Future Improvements

- Issue #927 will update `validate-golden.sh` to use `--pin-from` for validation
- Issue #928 will re-enable the CI validation workflow
- Issue #929 will add documentation for the constrained evaluation feature
