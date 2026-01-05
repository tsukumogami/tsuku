# Implementation Context: Issue #806

**Source**: docs/DESIGN-sandbox-dependencies.md

## Summary

Issue #806 enables multi-family sandbox CI testing for complex tools. It was blocked by:
- #805 - Implicit action dependencies (CLOSED - fixed in PR #808)
- #703 - Declared recipe dependencies (CLOSED - fixed by design doc)

Both blockers are now resolved. This issue is a simple CI configuration change:

## Current State

`.github/workflows/build-essentials.yml` has limited matrix:
```yaml
test-sandbox-multifamily:
  matrix:
    family: [debian]  # Limited - should be all 5
    tool: [make, pkg-config]  # Simple tools only
```

## Acceptance Criteria

- Expand matrix to all 5 families: `[debian, rhel, arch, alpine, suse]`
- Test complex tools with dependencies: `[cmake, ninja]` or `[sqlite]`
- All combinations pass (5 families × 2 tools = 10 test runs)
- CI passes with expanded matrix

## Implementation

This is a CI configuration change only - modify the matrix in the workflow file to expand testing coverage.
