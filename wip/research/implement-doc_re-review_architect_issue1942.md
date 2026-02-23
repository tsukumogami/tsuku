# Architect Re-Review: Issue #1942

## Prior Advisory Items

### Advisory 1: Marker file names hardcoded in two places
**RESOLVED.** Constants `verifyExitMarker` and `verifyOutputMarker` extracted at `internal/sandbox/executor.go:26-29`. All four production-code usages (two in `readVerifyResults`, two in `buildSandboxScript`) reference the constants. The test file (`executor_test.go`) still uses string literals for marker names, but since the tests are in `package sandbox` and are checking script output content (not constructing paths), this is acceptable -- the tests serve as an independent check that the script contains the expected strings.

### Advisory 2: Plan cache test lacks explicit ExitCode hash differentiation test
**NOT ADDRESSED.** `internal/executor/plan_cache_test.go` still has no test verifying that plans with different `ExitCode` values produce different cache keys. Remains advisory -- the `verifyForHash` struct handles this implicitly through JSON serialization, and the risk of regression is low.

### Advisory 3: Design doc references 'v3 to v4' but actual bump is v4 to v5
**NOT ADDRESSED.** `docs/designs/DESIGN-sandbox-ci-integration.md:143` still reads "plan format version bump (v3 to v4)" while `internal/executor/plan.go:19-20` shows the actual history is v4 (removed recipe_hash) -> v5 (added ExitCode). Remains advisory -- the design doc is a historical artifact and doesn't affect runtime behavior.

## New Findings

None. The refactoring was clean:

- `internal/sandbox/verify.go` properly deleted -- no leftover wrapper
- `internal/sandbox/executor.go:384` calls `executor.CheckPlanVerification()` directly
- Tests moved to `internal/executor/plan_verify_test.go` with thorough coverage (7 table-driven cases plus 4 standalone tests)
- `internal/validate/executor.go:346` also delegates to `executor.CheckPlanVerification()`, confirming shared logic lives in one place
- No new dependency direction violations; `sandbox` imports `executor` (correct direction), `validate` imports `executor` (correct direction)
- No parallel patterns introduced

## Summary

| Metric | Count |
|--------|-------|
| blocking_count | 0 |
| advisory_count | 2 (both carried forward, unchanged) |

The fixes addressed the most impactful advisory (marker file constants). The two remaining advisories are documentation-level concerns with no structural risk.
