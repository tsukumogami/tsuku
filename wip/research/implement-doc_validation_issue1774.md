# Validation Results: Issue #1774

**Issue**: feat(recipe): add gpu field to WhenClause
**Date**: 2026-02-19
**Scenarios tested**: scenario-5, scenario-6, scenario-7, scenario-8
**Result**: 4 passed, 0 failed

---

## scenario-5: WhenClause GPU matching logic

**Command**: `go test ./internal/recipe/ -run 'TestWhenClause.*GPU' -v -count=1`
**Status**: PASSED

Test `TestWhenClause_Matches_GPU` ran 16 sub-tests covering:
- Empty GPU matches any/no GPU target (2 cases)
- Single GPU value match/no-match (nvidia matches nvidia, not amd, not none)
- Multi-value GPU match (["amd","intel"] matches amd, matches intel, not nvidia)
- Special values: "none" matches no-GPU target, "apple" matches apple target
- Empty target GPU does not match non-empty clause GPU
- AND semantics with OS (3 cases: both match, OS match/GPU mismatch, OS mismatch)
- AND semantics with OS+arch+GPU (2 cases: all match, arch mismatch)

All 16 sub-tests passed in 0.00s.

---

## scenario-6: WhenClause IsEmpty includes GPU check

**Command**: `go test ./internal/recipe/ -run 'TestWhenClause.*IsEmpty' -v -count=1`
**Status**: PASSED

Test `TestWhenClause_IsEmpty` ran 10 sub-tests including GPU-specific cases:
- "clause with gpu is not empty": `WhenClause{GPU: ["nvidia"]}` returns `false` for `IsEmpty()` -- PASS
- "clause with empty gpu array is empty": `WhenClause{GPU: []}` returns `true` for `IsEmpty()` -- PASS

Also verified that other existing IsEmpty tests continue passing (nil, zero-value, platform, OS, package_manager, libc).

---

## scenario-7: TOML unmarshal parses gpu field from recipe

**Command**: `go test ./internal/recipe/ -run 'TestWhenClause_UnmarshalTOML_GPU' -v -count=1`
**Status**: PASSED

Test `TestWhenClause_UnmarshalTOML_GPU` ran 4 sub-tests:
- "single value array": `gpu = ["nvidia"]` parses to `GPU: ["nvidia"]` -- PASS
- "multiple values": `gpu = ["amd", "intel"]` parses to `GPU: ["amd", "intel"]` -- PASS
- "single string converted to array": `gpu = "nvidia"` parses to `GPU: ["nvidia"]` -- PASS
- "combined with os and arch filters": `os = ["linux"], arch = "amd64", gpu = ["nvidia"]` all parse correctly -- PASS

---

## scenario-8: ToMap round-trips GPU field

**Command**: `go test ./internal/recipe/ -run 'TestWhenClause_ToMap_GPU' -v -count=1`
**Status**: PASSED

Two tests ran:
- `TestWhenClause_ToMap_GPU`: Step with `GPU: ["nvidia", "amd"]` serializes to `whenMap["gpu"] = ["nvidia", "amd"]` -- PASS
- `TestWhenClause_ToMap_GPU_Empty`: Step with empty GPU omits "gpu" key from when map -- PASS
