# Validation Results: Issue 8 (Source-Directed Loading for Update/Outdated)

**Date**: 2026-03-17
**Scenarios validated**: scenario-24, scenario-25
**Branch**: main (commit 601f0d5a)

---

## Scenario 24: Update uses source-directed loading

**Status**: PASSED

**Tests executed**:
```
go test ./cmd/tsuku/ -run "TestLoadRecipeForTool|TestIsDistributedSource" -v -count=1
```

**Results**:
- `TestIsDistributedSource` -- PASS (6 sub-tests: empty, central, local, embedded, myorg/recipes, owner/repo)
- `TestLoadRecipeForTool_CentralSource` -- PASS (central source routes through normal chain)
- `TestLoadRecipeForTool_EmptySourceDefaultsToCentral` -- PASS (pre-migration empty source defaults to central)
- `TestLoadRecipeForTool_DistributedSource` -- PASS (distributed source routes via GetFromSource)
- `TestLoadRecipeForTool_NilState` -- PASS (nil state falls back to normal chain)
- `TestLoadRecipeForTool_ToolNotInState` -- PASS (unknown tool falls back to normal chain)
- `TestLoadRecipeForTool_EmbeddedSource` -- PASS (embedded source uses normal chain)

**Acceptance criteria verification**:
- [x] update reads ToolState.Source and uses GetFromSource
- [x] Central/embedded sources use existing chain
- [x] Empty source defaults to "central"
- [x] Unreachable source falls back gracefully (see scenario-25)

---

## Scenario 25: Outdated handles unreachable sources as warnings

**Status**: PASSED

**Tests executed**:
```
go test ./cmd/tsuku/ -run "TestLoadRecipeForTool_UnreachableDistributedFallsBack" -v -count=1
```

**Results**:
- `TestLoadRecipeForTool_UnreachableDistributedFallsBack` -- PASS

**Acceptance criteria verification**:
- [x] Unreachable distributed sources produce warnings, not fatal errors
- [x] Fallback to central succeeds when distributed source is unreachable
- [x] Central tools continue to be checked normally

---

## Full Test Suite Regression Check

**Command**: `go test ./... -count=1`

**Result**: All functional tests PASS. One lint-wrapper test (`TestGolangCILint`) fails due to pre-existing lint issues in files from earlier issues (5, 6, 7) -- not caused by issue 8.

**Lint issues** (all pre-existing, not introduced by issue 8):
- `install_distributed_test.go`: 3 unchecked `os.MkdirAll` errors (issue 7)
- `install_distributed.go`: 2 misspellings of "cancelled" (issue 7)
- `client_test.go`: 1 unclosed response body (issue 5)
- `bootstrap_test.go`, `toolchain_test.go`: 6 staticcheck warnings (pre-existing)

**Packages passing**: 40/41 (the 1 failure is lint-only, not functional)

---

## Implementation Observations

- `loadRecipeForTool` is defined in `cmd/tsuku/outdated.go:147` and shared by both update and outdated paths
- `isDistributedSource` is defined in `cmd/tsuku/update.go:106` and correctly identifies `owner/repo` format sources
- Test coverage includes 8 test functions covering central, embedded, distributed, unreachable, nil-state, empty-source, and cache scenarios
- The `ParseAndCache` helper is also tested for valid and invalid TOML inputs
