# Issue 718 Implementation Plan

## Summary

Create a CI workflow that validates all golden files when plan-affecting code changes. The workflow uses fine-grained path filtering based on introspection analysis to minimize false triggers.

## Approach

Follow the established workflow patterns from `validate-golden-recipes.yml` and `validate-golden-execution.yml` with fine-grained path triggers. Instead of using wildcard paths like `internal/actions/**`, explicitly list only the files that contain Decompose() methods or affect plan generation logic.

The introspection phase identified that only ~18 core files truly require full validation, rather than the ~54 files covered by the original wildcard approach. This reduces CI false positives and unnecessary validation runs.

## Files to Create

- `.github/workflows/validate-golden-code.yml` - Workflow triggered by changes to plan-affecting code

## Implementation Steps

- [x] Create `.github/workflows/validate-golden-code.yml` with:
  - Trigger on PR to main with fine-grained path filters
  - Exclude test files with paths-ignore pattern
  - Use download cache like sibling workflows
  - Run validate-all-golden.sh script
  - Output regeneration commands on failure

### Path Triggers (from introspection analysis)

**Tier 1 - Core Plan Generation (always triggers full validation):**
```yaml
paths:
  # Entry point
  - 'cmd/tsuku/eval.go'

  # Plan generation core
  - 'internal/executor/plan_generator.go'
  - 'internal/executor/plan.go'
  - 'internal/executor/plan_conversion.go'

  # Decomposition framework
  - 'internal/actions/decomposable.go'
  - 'internal/actions/action.go'

  # Composite actions (widely used)
  - 'internal/actions/composites.go'
  - 'internal/actions/download.go'

  # Recipe parsing (affects all recipes)
  - 'internal/recipe/types.go'
  - 'internal/recipe/loader.go'
  - 'internal/recipe/platform.go'
```

**Tier 2 - Ecosystem-Specific Actions (also triggers full validation for now):**
```yaml
  # Package manager decomposers
  - 'internal/actions/homebrew.go'
  - 'internal/actions/cargo_install.go'
  - 'internal/actions/npm_install.go'
  - 'internal/actions/pipx_install.go'
  - 'internal/actions/gem_install.go'
  - 'internal/actions/go_install.go'
  - 'internal/actions/nix_install.go'
  - 'internal/actions/fossil_archive.go'
  - 'internal/actions/apply_patch.go'

  # Version resolution (affects URL templates)
  - 'internal/version/*.go'
```

**Exclusions:**
```yaml
paths-ignore:
  - '**/*_test.go'
```

**Files explicitly NOT triggering validation (execution-only, no Decompose):**
- `internal/actions/extract.go`
- `internal/actions/chmod.go`
- `internal/actions/install_binaries.go`
- `internal/actions/cargo_build.go`
- `internal/actions/go_build.go`
- `internal/actions/configure_make.go`
- `internal/actions/cmake_build.go`
- `internal/actions/meson_build.go`
- `internal/actions/pip_install.go` (primitive, not PipxInstall)
- `internal/actions/nix_realize.go`
- `internal/actions/nix_portable.go`
- And ~25 other execution-only files

### Workflow Structure

The workflow should:
1. Trigger on specified path changes
2. Build tsuku binary
3. Restore download cache (if available)
4. Run `./scripts/validate-all-golden.sh`
5. Save download cache
6. On failure: output clear error message with regeneration commands

## Success Criteria

- [ ] Workflow triggers only on changes to plan-affecting files
- [ ] Changes to execution-only files (extract.go, chmod.go, etc.) do NOT trigger workflow
- [ ] Test file changes do NOT trigger workflow
- [ ] On mismatch: clear error output with recipe names and regeneration commands
- [ ] Download cache is used for efficiency

## Open Questions

None - introspection phase resolved the path filtering approach.
