# Maintainer Review: Issue #2131

## Issue

chore: update Codecov configuration for 75% target

## Files Changed

- `codecov.yml`

## Findings

No blocking or advisory findings.

## Assessment

The change is a two-line config edit: project target moved from 60% to 75%, and a `range: "70...90"` was added for badge coloring. Both values are self-documenting in the context of a Codecov config file. The patch target remains at 50%, which is consistent with the plan doc's scope (only the project target changes).

The ignore list is unchanged and well-organized -- it excludes generated code, test utilities, and entry points, which makes sense for coverage measurement.

No maintainability concerns. The next developer can read this file and understand the intent immediately.
