## Goal

Fix the libsixel-source recipe test failure on macOS Apple Silicon.

## Root Cause

The failure is in dependency chain resolution, not in the meson build itself:

1. `libsixel-source` uses `meson_build` action
2. `MesonBuildAction.Dependencies()` declares `["meson", "ninja", "zig"]` as install-time dependencies
3. `meson` recipe uses `pipx_install` action, which requires `python-standalone`
4. The plan generator installs `python-standalone` as an eval-time dependency for meson
5. After installing python-standalone, the plan generator for meson still can't find it

### Error Message

```
Failed to generate plan: failed to generate dependency plans:
  failed to generate plan for dependency meson:
    failed to resolve step pipx_install:
      failed to decompose pipx_install:
        python-standalone not found: install it first (tsuku install python-standalone)
```

### Why It Works Locally But Fails in CI

- **Local**: Dependencies (meson, python-standalone) already installed from previous runs
- **CI**: Fresh TSUKU_HOME per test, so freshly installed python-standalone isn't picked up by meson plan generator

## Technical Details

### Key Files

- `internal/actions/meson_build.go`: Declares install-time dependencies via `Dependencies()` method
- `internal/actions/resolver.go`: `ResolveDependencies()` aggregates dependencies from all recipe steps
- `internal/executor/plan_generator.go`: `generateDependencyPlans()` generates plans for dependencies
- `internal/executor/executor.go`: `installDependencies()` installs deps and populates `execPaths`

### Dependency Types

| Type | Declared In | When Resolved | Purpose |
|------|-------------|---------------|---------|
| Recipe-level | `[metadata].dependencies` in TOML | Before plan execution | Explicit user-facing deps |
| Action install-time | Action's `Dependencies().InstallTime` | During plan generation | Implicit deps required by actions |
| Action eval-time | Action's `Dependencies().EvalTime` | During `tsuku eval` | Deps needed to decompose composite actions |

### The Bug

In `plan_generator.go`, when generating dependency plans:

1. `generateDependencyPlans()` calls `generateSingleDependencyPlan()` for each dependency
2. For `meson`, this triggers plan generation which needs to decompose `pipx_install`
3. `pipx_install` decomposition requires `python-standalone` to be installed
4. The system installs `python-standalone` as an eval-time dep (line ~188-197 in plan_generator.go)
5. **But**: The freshly installed python-standalone isn't visible to the meson plan generator

The issue is likely that after installing an eval-time dependency, the registry/loader doesn't refresh to see the newly installed tool's binaries in `ExecPaths`.

### Potential Fix Areas

1. After installing eval-time deps in `generateDependencyPlans()`, update the execution context or paths
2. Ensure `pipx_install` decomposition can find freshly installed python-standalone
3. Consider whether the meson recipe should explicitly declare python-standalone as a dependency

## Acceptance Criteria

- [ ] Fix dependency resolution so eval-time dependencies are visible to subsequent plan generation
- [ ] Re-enable libsixel-source in macOS CI tests
- [ ] Verify the fix passes on macOS Apple Silicon
