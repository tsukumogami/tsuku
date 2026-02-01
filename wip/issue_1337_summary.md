# Issue 1337 Summary

## What Was Implemented

Added `--from` and `--deterministic-only` flags to `tsuku install`, enabling single-command recipe generation and installation (e.g., `tsuku install shfmt --from homebrew:shfmt`).

## Changes Made

- `cmd/tsuku/install.go`: Added `installFrom` and `installDeterministicOnly` flags. When `--from` is set, forwards to the create pipeline by setting create's package-level vars and calling `runCreate()`, then installs the generated recipe via `runInstallWithTelemetry()`. Updated Long help text with `--from` examples.
- `test/functional/features/install.feature`: Added two scenarios: successful `--from` create+install with shfmt, and error case for `--from` without a tool name.

## Key Decisions

- Reuse create's package-level flag vars directly rather than extracting a shared function. This keeps the change minimal and avoids a larger refactor of create.go's globals. The coupling is acceptable since both commands live in the same package.
- Use `shfmt` for the functional test instead of `jq` or `actionlint`, since those have homebrew runtime dependencies (oniguruma, shellcheck) that don't exist in the test registry.
- Map `--force` to `createAutoApprove` (skips recipe review prompt) and set `createForce = true` (overwrites existing recipe) unconditionally, since `--from` always generates a fresh recipe.

## Trade-offs Accepted

- Direct mutation of create.go's package-level vars is a code smell, but avoids a refactoring PR that would touch create.go's entire flag handling.
- `--deterministic-only` is forwarded but `--skip-sandbox` is not (create defaults apply). This is intentional since the homebrew builder already skips sandbox testing.

## Test Coverage

- 2 new functional test scenarios (end-to-end --from flow and error case)
- Unit tests in cmd/tsuku pass (existing coverage of install command routing)

## Known Limitations

- Only works with builders that the create pipeline supports (homebrew, npm, pypi, etc.)
- Runtime dependencies of the generated recipe must already exist as recipes in the registry
