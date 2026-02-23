# Re-review: Issue #1942 (Maintainer Reviewer)

## Status of Previously Flagged Items

### Advisory 1: Stale "Currently 3" comment in plan.go -- RESOLVED

The `FormatVersion` field's godoc at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/executor/plan.go:27` now reads:

```go
// FormatVersion enables future evolution of the plan format.
// See PlanFormatVersion for the current version.
FormatVersion int `json:"format_version"`
```

This points the reader to the `PlanFormatVersion` constant (line 20), which has the full version history including the new Version 5 entry. The next developer looking at the struct field will follow the reference instead of trusting a possibly-stale inline number.

### Advisory 2: Overlapping Sandbox() godoc paragraphs -- RESOLVED

The `Sandbox()` method at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/sandbox/executor.go:112-122` now has a single clean doc block:

```go
// Sandbox runs an installation plan in an isolated container.
// It uses the provided SandboxRequirements to configure the container
// with appropriate image, network access, and resource limits.
//
// The sandbox process:
// 1. Detect available container runtime
// 2. Write plan JSON to workspace
// 3. Generate sandbox script based on requirements
// 4. Mount tsuku binary, plan, and cache into container
// 5. Run container with configured limits
// 6. Check verification output
```

The redundant second paragraph that restated the same information is gone.

### Advisory 3: Marker file names as bare strings -- RESOLVED

Constants are now defined at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/sandbox/executor.go:26-29`:

```go
const (
	verifyExitMarker   = ".sandbox-verify-exit"
	verifyOutputMarker = ".sandbox-verify-output"
)
```

All four usage sites in the production code use the constants:
- `readVerifyResults()` (lines 356-357): `filepath.Join(workspaceDir, verifyExitMarker)` / `filepath.Join(workspaceDir, verifyOutputMarker)`
- `buildSandboxScript()` (lines 516-517): `fmt.Sprintf` with `verifyOutputMarker` / `verifyExitMarker`

The test file (`executor_test.go`) still uses literal strings like `.sandbox-verify-exit` in `strings.Contains` checks. This is acceptable -- tests are asserting the script's rendered output as a string, and the constants are unexported. If someone renames a constant without updating its value, the test will catch the mismatch.

## Additional Fixes Applied

### verify.go wrapper deleted -- CONFIRMED

`internal/sandbox/verify.go` no longer exists. The single call site at `executor.go:384` now calls `executor.CheckPlanVerification()` directly. The import was already present, so no new dependency was introduced.

### Tests moved to plan_verify_test.go -- CONFIRMED

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/executor/plan_verify_test.go` is in `package executor`, co-located with `plan_verify.go`. The tests cover:
- Exit code match/mismatch with empty pattern
- Pattern match/mismatch
- Non-default expected exit codes
- Table-driven suite with 7 cases including multiline output

The tests are well-named (test names match the behavior they assert) and use `t.Parallel()` throughout.

## New Findings

None. The fixes are clean and no new maintainability issues were introduced.

## Summary

| Metric | Count |
|--------|-------|
| blocking_count | 0 |
| advisory_count | 0 |

All three previously flagged advisory items are resolved. The additional pragmatic-review fix (deleting verify.go wrapper) was applied correctly. The code is clear for the next developer.
