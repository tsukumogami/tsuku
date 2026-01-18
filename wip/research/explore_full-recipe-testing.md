# Full Recipe Testing Research

## Summary

Full recipe testing (all recipes, not just changed ones) is triggered by **critical code changes** or **scheduled nightly runs**.

## Triggers for Full Testing

### 1. Critical Code Changes (`validate-golden-code.yml`)

When these files change, ALL recipes with golden files are validated:

**Plan generation core:**
- `cmd/tsuku/eval.go`
- `internal/executor/plan_generator.go`
- `internal/executor/plan.go`
- `internal/executor/plan_conversion.go`

**Action framework:**
- `internal/actions/decomposable.go`
- `internal/actions/action.go`
- `internal/actions/composites.go`
- `internal/actions/download.go`

**Package manager decomposers (triggers full testing):**
- `internal/actions/homebrew.go`
- `internal/actions/cargo_install.go`
- `internal/actions/npm_install.go`
- `internal/actions/pipx_install.go`
- `internal/actions/gem_install.go`
- `internal/actions/go_install.go`
- `internal/actions/nix_install.go`
- `internal/actions/fossil_archive.go`
- `internal/actions/apply_patch.go`

**Recipe parsing:**
- `internal/recipe/types.go`
- `internal/recipe/loader.go`
- `internal/recipe/platform.go`

**Version resolution:**
- `internal/version/*.go`

### 2. Files That Do NOT Trigger Full Testing

Execution-only code:
- `extract.go`, `chmod.go`, `install_binaries.go`
- `cargo_build.go`, `go_build.go`, `configure_make.go`, `cmake_build.go`, `meson_build.go`
- `pip_install.go`, `nix_realize.go`, `nix_portable.go`
- `executor.go`, `recipe/validate.go`, `recipe/writer.go`

### 3. Scheduled Nightly Tests (`scheduled-tests.yml`)

- Cron: `0 2 * * *` (2 AM UTC)
- Tests both `ci.linux` and `ci.scheduled` test suites
- Tests both `ci.macos` and `ci.scheduled` test suites
- Includes slow/expensive tests not run on every PR

### 4. Recipe Validation (`test.yml`)

- Triggers on: recipe changes, validator code changes, or scheduled runs
- Validates all recipes with `tsuku validate --strict`

## Detection Mechanism

GitHub Actions path filters detect critical changes:

```yaml
paths:
  - 'cmd/tsuku/eval.go'
  - 'internal/executor/plan_generator.go'
  - 'internal/executor/plan.go'
  # ... (35 total files)
```

When ANY of these files change, the entire golden file suite is re-validated.

## Key Insight

The distinction between "plan generation code" and "execution code" is deliberate:
- Plan generation affects what recipes produce as outputs
- Execution code only affects how those plans are run
- Only plan generation changes warrant testing all recipes
