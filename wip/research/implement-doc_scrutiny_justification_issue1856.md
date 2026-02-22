# Scrutiny Review: Justification Focus -- Issue #1856

**Issue**: #1856 (feat(cli): add subcategory to install error JSON output)
**Design doc**: docs/designs/DESIGN-structured-error-subcategories.md
**Scrutiny focus**: justification

## Requirements Mapping Under Review

| AC (claimed) | Status (claimed) | Evidence (claimed) |
|---|---|---|
| classifyInstallError returns (int, string) | implemented | cmd/tsuku/install.go:classifyInstallError() line 302, signature changed to (int, string) |
| Subcategory field added to installError struct | implemented | cmd/tsuku/install.go line 333: Subcategory string json:subcategory,omitempty |
| handleInstallError populates Subcategory | implemented | cmd/tsuku/install.go:handleInstallError() line 373, Subcategory: subcategory |
| omitempty behavior correct | implemented | cmd/tsuku/install_test.go TestInstallErrorJSON subtests verify presence/absence |

## Justification Analysis

### Deviation Assessment

All four mapped ACs are reported as "implemented" with no deviations. There are no `reason` or `alternative_considered` fields to evaluate. This means the justification review focuses on whether the absence of deviations is itself justified -- i.e., whether deviations exist that should have been disclosed but were not.

### Hidden Deviation Check

**1. The mapping ACs are a subset of the issue's actual ACs.**

The issue body defines five distinct AC groups with specific checkboxes:

1. **Signature change** (2 checkboxes): classifyInstallError returns (int, string); all call sites updated
2. **Subcategory mapping** (1 checkbox): Each errors.As() branch returns the correct subcategory per the table (8 error types mapped)
3. **Struct update** (2 checkboxes): Subcategory field with omitempty tag; handleInstallError populates it
4. **JSON output behavior** (3 checkboxes): Non-empty subcategory appears; empty subcategory omitted; existing fields unchanged
5. **No changes to non-JSON output** (3 checkboxes): categoryFromExitCode unchanged; stderr unchanged; exit codes unchanged

The mapping contains only 4 entries that cover a subset of these. Notably:

- **AC group 2 (subcategory mapping)** is not represented in the mapping at all. This is a core AC -- the specific error-type-to-subcategory mappings are the substance of the feature. The diff confirms all 8 mappings from the issue's table are implemented correctly. The omission from the mapping is not a deviation in implementation, but it is a gap in the mapping itself.
- **AC group 5 (no changes to non-JSON output)** is not represented. The diff confirms categoryFromExitCode is unchanged. This is a negative requirement (verify nothing changed) and is satisfied.

Since all the actual code changes match what the issue requires, this is a mapping coverage gap, not a hidden deviation. The coder summarized the ACs at a higher level than the issue specified, but the underlying work is done.

**2. No deviations that should have been disclosed.**

Reviewing the diff against all issue ACs:

- The subcategory mapping table in the issue specifies 8 error types. All 8 are implemented correctly in the switch statement (lines 311-324 of install.go).
- The tests cover all 8 error types with correct subcategory expectations.
- categoryFromExitCode is unchanged (verified in diff -- no changes to lines 349-360).
- Exit codes are unchanged (the function still returns the same exit code integers).
- The TestInstallErrorJSON test was split into "with subcategory" and "without subcategory" subtests that verify both omitempty behavior and field presence.

There are no shortcuts masked as deviations because there are no deviations to mask.

### Proportionality Check

The mapping has 4 ACs, all marked "implemented." The issue has approximately 11 discrete checkboxes across 5 groups. The mapping consolidates these into 4 high-level items. This is a condensed but not dishonest representation. The implementation covers the full scope.

### Avoidance Pattern Check

No avoidance patterns detected. There are no "too complex for this scope," "can be added later," or "out of scope" justifications because there are no deviations.

## Cross-Issue Enablement (Downstream: #1857)

Issue #1857 depends on this issue and needs:
- classifyInstallError() to return a subcategory string -- satisfied
- installError struct to include Subcategory in JSON output -- satisfied
- Subcategory values to be documented and tested -- tests exist for all values; the values themselves are hardcoded strings that #1857 can rely on

The implementation provides a sufficient foundation for #1857's parseInstallJSON() to extract subcategories from CLI JSON output.

## Findings

### Blocking: 0

None.

### Advisory: 1

**Advisory 1: Mapping ACs are a condensed subset of issue ACs**

The issue defines 11 checkboxes across 5 AC groups. The mapping contains 4 entries. While all implementation work is done (verified against the diff), the mapping omits AC group 2 (subcategory mapping -- the core feature) and AC group 5 (no changes to non-JSON output) entirely. This doesn't indicate a shortcut -- the code is correct -- but a more granular mapping would have made verification easier and reduced the gap between what was claimed and what was delivered.

Severity: advisory. The implementation is complete; only the mapping's granularity is lower than ideal.
