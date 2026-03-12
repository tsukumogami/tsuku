# Test Plan: Code Coverage 75%

Generated from: docs/plans/PLAN-code-coverage-75.md
Issues covered: 5

## Infrastructure Scenarios

## Scenario 1: Codecov config has correct project target
**ID**: scenario-1
**Testable after**: #2131
**Commands**:
- `grep 'target: 75%' codecov.yml`
- `grep -A1 'range:' codecov.yml`
**Expected**: codecov.yml contains `target: 75%` under `project.default`, and a `range` key set to `"70...90"`. The `patch.default.target` remains at `50%`.
**Status**: passed

## Scenario 2: Codecov patch target unchanged
**ID**: scenario-2
**Testable after**: #2131
**Commands**:
- `grep -A3 'patch:' codecov.yml`
**Expected**: The patch section still contains `target: 50%` -- no unintended changes to patch coverage requirements.
**Status**: passed

## Scenario 3: executor package tests pass and meet 75%
**ID**: scenario-3
**Testable after**: #2132
**Commands**:
- `go test -coverprofile=cover_executor.out ./internal/executor/...`
- `go tool cover -func=cover_executor.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/executor` shows >= 75.0%.
**Status**: passed (76.4%)

## Scenario 4: validate package tests pass and meet 75%
**ID**: scenario-4
**Testable after**: #2132
**Commands**:
- `go test -coverprofile=cover_validate.out ./internal/validate/...`
- `go tool cover -func=cover_validate.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/validate` shows >= 75.0%.
**Status**: passed (75.2%)

## Scenario 5: builders package tests pass and meet 75%
**ID**: scenario-5
**Testable after**: #2132
**Commands**:
- `go test -coverprofile=cover_builders.out ./internal/builders/...`
- `go tool cover -func=cover_builders.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/builders` shows >= 75.0%.
**Status**: passed (75.5%)

## Scenario 6: userconfig package tests pass and meet 75%
**ID**: scenario-6
**Testable after**: #2132
**Commands**:
- `go test -coverprofile=cover_userconfig.out ./internal/userconfig/...`
- `go tool cover -func=cover_userconfig.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/userconfig` shows >= 75.0%.
**Status**: passed (94.0%)

## Scenario 7: discover package tests pass and meet 75%
**ID**: scenario-7
**Testable after**: #2133
**Commands**:
- `go test -coverprofile=cover_discover.out ./internal/discover/...`
- `go tool cover -func=cover_discover.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/discover` shows >= 75.0%.
**Status**: passed (75.3%)

## Scenario 8: verify package tests pass and meet 75%
**ID**: scenario-8
**Testable after**: #2133
**Commands**:
- `go test -coverprofile=cover_verify.out ./internal/verify/...`
- `go tool cover -func=cover_verify.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/verify` shows >= 75.0%.
**Status**: passed (75.1%)

## Scenario 9: actions package tests pass and meet 66%
**ID**: scenario-9
**Testable after**: #2134
**Commands**:
- `go test -coverprofile=cover_actions.out ./internal/actions/...`
- `go tool cover -func=cover_actions.out | tail -1`
**Expected**: All tests pass. The total coverage line for `internal/actions` shows >= 66.0%.
**Status**: passed (66.8%)

## Scenario 10: actions tests make no network calls
**ID**: scenario-10
**Testable after**: #2134
**Commands**:
- `go test -count=1 ./internal/actions/... 2>&1`
**Expected**: All tests pass without network access. No test should require downloading remote resources or connecting to external services. Tests that previously required network should use stubs or fixtures.
**Status**: passed (uses .invalid domains)

## Scenario 11: existing tests still pass (full suite regression)
**ID**: scenario-11
**Testable after**: #2132, #2133, #2134
**Commands**:
- `go test ./...`
**Expected**: All existing tests across the entire codebase continue to pass. No regressions introduced by new test files.
**Status**: passed

## Use-Case Scenarios

## Scenario 12: full coverage report exceeds 75% aggregate
**ID**: scenario-12
**Testable after**: #2131, #2132, #2133, #2134, #2135
**Commands**:
- `go test -coverprofile=cover_all.out ./...`
- `go tool cover -func=cover_all.out | tail -1`
**Expected**: The aggregate coverage across all Codecov-relevant packages (excluding ignored paths: bundled, cmd, internal/install, internal/testutil, test files) is >= 75.0%.
**Status**: pending
**Environment**: automatable

## Scenario 13: CI pipeline passes with updated Codecov config
**ID**: scenario-13
**Testable after**: #2131, #2132, #2133, #2134, #2135
**Commands**:
- Push branch, open PR, observe CI checks
- `gh pr checks <PR_NUMBER> --repo tsukumogami/tsuku`
**Expected**: CI passes all checks including Codecov status check. The Codecov project status check reports >= 75% and shows green. The patch check continues to use the 50% target.
**Status**: pending
**Environment**: manual (requires PR with all issues merged, Codecov integration active)

## Scenario 14: Codecov badge reflects new target
**ID**: scenario-14
**Testable after**: #2135
**Commands**:
- After merge, check Codecov dashboard for tsukumogami/tsuku
**Expected**: The Codecov badge and dashboard show the coverage percentage in the green range (70-90 as defined by the new range setting) once coverage exceeds 75%.
**Status**: pending
**Environment**: manual (requires merge to main, Codecov dashboard access)
