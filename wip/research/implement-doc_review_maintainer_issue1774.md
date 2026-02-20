# Maintainer Review: #1774 feat(recipe): add gpu field to WhenClause

## Summary

No blocking findings. The implementation follows the established Libc pattern closely, which is the right call for maintainability. The GPU field was added to all the right touchpoints: `WhenClause`, `Matches()`, `IsEmpty()`, `ToMap()`, `UnmarshalTOML()`, `MergeWhenClause()`, `Constraint`, `Clone()`, `ValidateStepsAgainstPlatforms()`, `MatchTarget`, and `NewMatchTarget()`. Test coverage is thorough.

## Files Changed

- `internal/recipe/types.go` -- GPU field added to WhenClause, Constraint, MatchTarget, MergeWhenClause, Matches(), IsEmpty(), ToMap(), UnmarshalTOML()
- `internal/recipe/platform.go` -- GPU validation in ValidateStepsAgainstPlatforms()
- `internal/recipe/when_test.go` -- 27 new tests for GPU matching, unmarshaling, serialization, validation
- `internal/recipe/types_test.go` -- Tests for MergeWhenClause GPU propagation, Constraint.Clone GPU

## Findings

### 1. ADVISORY: GPU filter semantics differ from Libc filter in a non-obvious way

**File**: `internal/recipe/types.go:310-337`

The Libc check is gated on `os == "linux"`:

```go
if len(w.Libc) > 0 && os == "linux" {
```

The GPU check has no OS gate:

```go
if len(w.GPU) > 0 {
```

This is actually correct -- GPU detection returns a value on every platform (apple on macOS, none on Windows), while libc is Linux-specific. But the next person reading `Matches()` will see two array-typed fields with similar structure and wonder why one is gated and the other isn't. A one-line comment before the GPU block would clarify:

```go
// Check GPU filter (applies on all platforms, unlike libc which is Linux-only)
```

The current comment `// Check GPU filter` doesn't convey the design difference.

**Severity**: Advisory. The test suite documents this behavior (the "gpu filter with empty target gpu does not match" test at when_test.go:476 covers the empty-string case), and the design doc is explicit about GPU values on all platforms. Low misread risk.

### 2. ADVISORY: MergeWhenClause silently drops GPU for multi-value arrays

**File**: `internal/recipe/types.go:683-686`

```go
// Propagate GPU filter (single value only, no conflict detection needed)
if len(when.GPU) == 1 {
    result.GPU = when.GPU[0]
}
```

When `when.GPU` has multiple values (e.g., `["amd", "intel"]`), the constraint's GPU stays empty. This mirrors the multi-OS behavior on line 659-662:

```go
if result.OS == "" && len(when.OS) == 1 {
    result.OS = when.OS[0]
}
// Multi-OS case: result.OS stays empty (unconstrained within the listed OSes)
```

The OS case has an explicit comment explaining the multi-value behavior. The GPU case has only `"single value only, no conflict detection needed"` which doesn't tell the next person what happens when there are multiple GPU values or why that's fine.

A comment like `// Multi-GPU case: constraint stays empty (matches any GPU within the listed set)` would match the OS pattern and prevent the question "is this a bug that drops GPU constraints?"

**Severity**: Advisory. The test `TestMergeWhenClause_MultiGPULeavesEmpty` documents the behavior. The pattern matches OS handling.

### 3. ADVISORY: Test names for MatchTarget GPU are slightly less discoverable

**File**: `internal/recipe/when_test.go:550-562`

The GPU tests for `MatchTarget` are nested as sub-tests under `TestMatchTarget`:

```go
t.Run("NewMatchTarget with GPU", func(t *testing.T) { ... })
t.Run("MatchTarget with empty GPU", func(t *testing.T) { ... })
```

These follow the existing pattern for the `MatchTarget` test (libc was also tested via the original `TestMatchTarget` sub-tests). Consistent with the existing structure. No action needed.

### 4. ADVISORY: Existing `NewMatchTarget` callers pass empty GPU string

**Files**: `internal/executor/executor.go:132`, `internal/executor/plan_generator_test.go:168,988`, `internal/actions/resolver_test.go:1435,1463,1497,1505,1563`

All existing callers now pass `""` for the GPU parameter. This is the correct approach for this issue since #1775 (thread GPU through plan generation) handles wiring real values. The constructor signature change was necessary to maintain type safety.

The empty string is safe: `Matches()` with `GPU: []string{}` (no filter) returns true regardless of the target's GPU value, and steps without a GPU when clause work as before.

### 5. Overall Assessment

The implementation is clean and follows the Libc pattern closely, which is the right strategy. The next developer adding a new `WhenClause` field has a clear template to follow: check `Libc` and `GPU` for the array-field pattern, check `Arch` and `LinuxFamily` for the string-field pattern.

Key decisions are sound:
- GPU validation uses `platform.ValidGPUTypes` -- single source of truth for valid values, bridging the platform and recipe packages (previously flagged as advisory in #1773 review, now with its first production consumer)
- `MergeWhenClause` propagates single-value GPU to `Constraint` following the OS precedent
- GPU filter in `Matches()` applies on all platforms (correct given the design)
- `UnmarshalTOML` handles both array and string TOML values (matching existing fields)

Test coverage: 27 new tests covering matching semantics, TOML unmarshaling, serialization round-trip, validation of invalid values, combined filters, and edge cases (empty GPU string, empty array). The tests are readable and test behavior rather than implementation details.
