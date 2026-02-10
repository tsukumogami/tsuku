---
summary:
  constraints:
    - Must replace 'tsuku deps' with 'tsuku info --deps-only --system --family alpine'
    - Workflow syntax must be valid YAML
    - CI must pass for all matrix libraries (zlib, libyaml, gcc-libs)
  integration_points:
    - .github/workflows/integration-tests.yml (library-dlopen-musl job)
    - tsuku info --deps-only --system --family command (from #1573)
  risks:
    - Syntax change could break if not exact
  approach_notes: |
    Simple find-and-replace in the workflow file.
    Change: tsuku deps --system --family alpine
    To: tsuku info --deps-only --system --family alpine
---

# Implementation Context: Issue #1575

This issue migrates the `library-dlopen-musl` job from the prototype `tsuku deps` command to the production `tsuku info --deps-only --system` interface.

## Key Change

```yaml
# Before
DEPS=$(./tsuku deps --system --family alpine ${{ matrix.library }})

# After
DEPS=$(./tsuku info --deps-only --system --family alpine ${{ matrix.library }})
```

## Validation

The issue provides a validation script to verify:
1. 'tsuku deps' is NOT present in the workflow
2. 'tsuku info --deps-only --system --family alpine' IS present
