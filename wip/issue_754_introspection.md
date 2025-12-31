# Issue 754 Introspection

## Context Reviewed

- **Design doc**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/DESIGN-system-dependency-actions.md`
- **Sibling issues reviewed**: None closed yet (all Phase 1 infrastructure issues are ready/pending)
- **Prior patterns identified**:
  - Existing `Platform` struct in `internal/executor/plan.go` (OS/Arch only)
  - Existing platform utilities in `internal/recipe/platform.go`
  - Runtime detection already uses `runtime.GOOS` and `runtime.GOARCH` throughout codebase
  - No `internal/platform/` package yet exists

## Gap Analysis

### Minor Gaps Identified

1. **Package location pattern**: Issue specifies `internal/platform/target.go`, which is consistent with existing package structure (not a gap, just confirming location)

2. **Existing Platform struct difference**: The existing `executor.Platform` struct (lines 87-91 in plan.go) only has `OS` and `Arch` fields. The new `Target` struct in issue #754 needs both this AND `LinuxFamily`. This is NOT a conflict - they serve different purposes:
   - `executor.Platform`: Describes what a *plan* was built for (static, binary data in JSON)
   - `platform.Target`: Describes what we're *targeting for plan generation* (dynamic, includes family detection)

   The design doc (lines 553-564) explicitly separates these concerns, showing `Target` as a parameter to plan generation while `WhenClause` remains unchanged.

3. **LinuxFamily validation**: The issue accepts criteria specify valid values as "debian, rhel, arch, alpine, suse" but doesn't mention validation. The design doc (lines 208-244) provides the exact mapping via `distroToFamily` variable. Implementation should include field validation.

4. **Helper methods pattern**: Issue specifies `OS() string` and `Arch() string` methods for parsing `Platform` field. The design doc (lines 622-632) shows these being used in `DetectTarget()`. These parse the "linux/amd64" format string into components. This is a reasonable pattern for encapsulation.

5. **LinuxFamily initialization rule**: Issue states "LinuxFamily is only set when OS is Linux (empty for darwin, windows)" - this is a critical invariant that should be enforced/documented in the struct or constructor.

### Moderate Gaps

1. **Constructor vs bare struct**: Issue doesn't specify how `Target` is constructed. Should there be a helper function like `DetectTarget()` that ensures the invariant? The design doc shows this as necessary (lines 622-632: `func DetectTarget()`).

   **Gap**: Should issue #754 include implementation of `DetectTarget()` alongside the struct, or is that deferred to #759 (linux_family detection)?

   **Design intent** (lines 604-633): The `DetectTarget()` function lives in `family.go`, not `target.go`, and is implemented in Phase 1. However, issue #754 creates `target.go` and #759 creates `family.go`. This is a sequencing issue - should #754 include a stub or full `DetectTarget()`?

2. **Parser implementation detail**: The helper methods `OS()` and `Arch()` must parse strings like "linux/amd64". Issue doesn't specify behavior for invalid formats. Should there be error handling or validation?

3. **Test fixtures**: Issue specifies "Unit tests for Target struct" but doesn't mention what scenarios to test:
   - Valid platform/family combinations?
   - Invalid LinuxFamily values?
   - Invariant violations (LinuxFamily set on non-Linux)?
   - Helper method edge cases?

### Major Gaps

None identified. The issue spec is complete for the struct definition work.

## Recommendation

**Proceed with clarifications incorporated as minor gaps**

## Key Findings

1. **Spec aligns well with design**: The Target struct spec in issue #754 matches the design doc's targeting model (lines 553-564). The separation from existing `executor.Platform` is intentional and correct.

2. **No conflicting prior work**: The `internal/platform/` package doesn't exist, so this is greenfield work. The existing `Platform` struct serves a different purpose (describing generated plans) and won't conflict.

3. **Sibling issues independent**: Issue #754 has no dependencies and can be implemented independently. Siblings (#755, #756) are similarly ready. This is a well-decomposed task.

4. **Implementation is straightforward**: The Target struct, helper methods, and tests are all clearly scoped and can be completed without external dependencies or design conflicts.

## Proposed Amendments

None required, but clarifications for implementation:

1. For the `OS()` and `Arch()` helper methods: Decide on error handling for malformed platform strings (currently undocumented in issue). Recommend parsing "linux/amd64" format with clear error messages for invalid input.

2. For LinuxFamily validation: Implement as a validation function or in test assertions to enforce that LinuxFamily is only set when OS=="linux" and is one of {debian, rhel, arch, alpine, suse}.

3. For test coverage: Cover at minimum:
   - Valid Target creation for each linux_family value
   - Invalid LinuxFamily rejection
   - Non-Linux platforms with empty LinuxFamily
   - Helper method parsing (valid and invalid formats)
   - Invariant enforcement (no LinuxFamily on darwin/windows)

4. For future #759 integration: Keep implementation of `DetectFamily()` and `DetectTarget()` functions ready for #759 PR, but they can live in separate `family.go` file created in that issue.
