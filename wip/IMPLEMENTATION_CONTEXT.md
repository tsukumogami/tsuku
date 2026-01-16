# Implementation Context: Issue #926

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Information

- **Title**: feat(eval): add cpan_install constraint support
- **Tier**: simple
- **Dependencies**: #921 (closed - implemented constrained evaluation skeleton)

## Pattern to Follow

This follows the same pattern as:
- #921 (pip) - PipConstraints extraction
- #922 (go_build) - GoSum extraction  
- #923 (cargo_install) - CargoLock extraction
- #924 (npm_install) - NpmLock extraction
- #925 (gem_install) - GemLock extraction

## Implementation Requirements

1. **Constraint Extraction** (`internal/executor/constraints.go`):
   - Add `extractCpanConstraintsFromSteps()` function
   - Look for `cpan_exec` steps with `meta_data` parameter (cpanfile.snapshot content)
   - Add helper function `HasCpanMetaConstraint()`

2. **Tests** (`internal/executor/constraints_test.go`):
   - TestExtractConstraints_CpanExec
   - TestExtractConstraints_CpanExecInDependency
   - TestExtractConstraints_CpanExecFirstWins
   - TestExtractConstraints_CpanExecEmptyMetaData
   - TestHasCpanMetaConstraint

## EvalConstraints Struct

The `CpanMeta` field already exists in `EvalConstraints` struct (`internal/actions/decomposable.go`):

```go
// CpanMeta contains cpanfile.snapshot content for cpan_install steps.
// Future: will be populated by issue #926.
CpanMeta string
```

## Key Insight

cpan_exec steps (in golden files) contain a `meta_data` parameter with the cpanfile.snapshot content. The constraint extraction needs to extract this and populate `constraints.CpanMeta`.
