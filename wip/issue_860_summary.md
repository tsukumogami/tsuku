# Issue 860 Summary

## What Was Implemented

Enabled static linking for tsuku binary builds across all CI workflows by adding `CGO_ENABLED=0` to Go build commands. This produces a fully static binary that works on any Linux distribution regardless of libc implementation (glibc vs musl).

## Changes Made

- `.github/workflows/sandbox-tests.yml`: Added CGO_ENABLED=0 to build command
- `.github/workflows/build-essentials.yml`: Added CGO_ENABLED=0 to 9 build commands
- `.github/workflows/validate-golden-execution.yml`: Added CGO_ENABLED=0 to 2 build commands
- `.github/workflows/validate-golden-recipes.yml`: Added CGO_ENABLED=0 to build command
- `.github/workflows/generate-golden-files.yml`: Added CGO_ENABLED=0 to build command
- `.github/workflows/test-changed-recipes.yml`: Added CGO_ENABLED=0 to 2 build commands
- `.github/workflows/test.yml`: Added CGO_ENABLED=0 to 3 build commands
- `.github/workflows/npm-builder-tests.yml`: Added CGO_ENABLED=0 to 2 build commands
- `.github/workflows/cargo-builder-tests.yml`: Added CGO_ENABLED=0 to 2 build commands
- `.github/workflows/pypi-builder-tests.yml`: Added CGO_ENABLED=0 to 2 build commands
- `.github/workflows/homebrew-builder-tests.yml`: Added CGO_ENABLED=0 to build command
- `.github/workflows/gem-builder-tests.yml`: Added CGO_ENABLED=0 to 2 build commands
- `.github/workflows/validate-golden-code.yml`: Added CGO_ENABLED=0 to build command
- `.github/workflows/scheduled-tests.yml`: Added CGO_ENABLED=0 to 2 build commands

## Key Decisions

- **Static linking via CGO_ENABLED=0**: Chosen because it's already used in `.goreleaser.yaml` for production releases and `integration_test.go` for container testing, proving the approach works without issues.

- **Updated all workflows**: Rather than just sandbox-related workflows, updated all 14 workflows with Go build commands to ensure consistency and avoid future issues if any workflow adds container testing.

## Trade-offs Accepted

- **DNS resolution uses pure Go resolver**: With CGO disabled, Go uses its pure Go DNS resolver instead of the system resolver. This is acceptable because the production releases already use this approach without issues.

- **No new tests added**: The existing `test-sandbox-multifamily` job in `build-essentials.yml` already tests Alpine family containers. Once the fix is deployed, this job will validate the fix works.

## Test Coverage

- New tests added: 0 (existing CI covers this)
- Coverage change: No change (only workflow YAML files modified)

## Known Limitations

None - the solution is comprehensive and aligns with existing production build configuration.

## Verification

Binary built with `CGO_ENABLED=0` shows as "statically linked" via `file` command:
```
tsuku: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, ...
```
