# Scrutiny Review: Completeness - Issue #1816

**Issue**: #1816 - docs(ci): document batch size configuration and tuning
**Scrutiny Focus**: completeness
**Date**: 2026-02-21

## Files Changed (from diff)

- `CONTRIBUTING.md` - one paragraph added after the CI workflow table
- `docs/workflow-validation-guide.md` - new "CI Batch Configuration" section appended (lines 149-219)
- `wip/implement-doc-state.json` - state tracking update (not relevant to AC verification)

## Acceptance Criteria Extraction

From the issue body, the ACs are:

**AC-1**: `.github/ci-batch-config.json` includes inline comments or an accompanying doc explaining each workflow's batch size values and why they differ (e.g., alpine uses 5 because source builds are 3-5x slower than ubuntu)

**AC-2a**: A section covering what batch sizes control and where they're configured

**AC-2b**: How to use the `batch_size_override` input on `workflow_dispatch` to experiment with different sizes

**AC-2c**: Guidelines for when to increase or decrease batch sizes (e.g., if batched jobs regularly exceed 10 minutes, reduce; if most finish in under 3 minutes, consider increasing)

**AC-2d**: The valid range for batch sizes (1-50, enforced by the guard clause)

**AC-3**: CONTRIBUTING.md mentions that recipe CI uses batched jobs, so contributors understand why a single check covers multiple recipes

**AC-4**: A note (in docs or as tracked GitHub issue) records the follow-up: `validate-golden-execution.yml` has three per-recipe matrix jobs that should be batched

**AC-5**: Verify batch sizes of 15 (test-changed-recipes Linux) and 20 (validate-golden-recipes) produce job durations under 10 minutes for typical PRs (5-20 changed recipes). If timing shows adjustment needed, update config and document rationale.

## Requirements Mapping (Untrusted - Subject to Verification)

```
--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
1. ac: "ci-batch-config.json includes docs explaining batch sizes"
   status: implemented
   evidence: "workflow-validation-guide.md Configuration File subsection"

2. ac: "Section covering what batch sizes control"
   status: implemented
   evidence: "workflow-validation-guide.md intro and config file subsections"

3. ac: "How to use batch_size_override"
   status: implemented
   evidence: "Manual Override via workflow_dispatch subsection"

4. ac: "Guidelines for increasing/decreasing"
   status: implemented
   evidence: "Tuning Guidelines subsection"

5. ac: "Valid range 1-50"
   status: implemented
   evidence: "workflow-validation-guide.md lines 186,189"

6. ac: "CONTRIBUTING.md mentions batched jobs"
   status: implemented
   evidence: "CONTRIBUTING.md paragraph after CI table"

7. ac: "Follow-up note for validate-golden-execution.yml"
   status: implemented
   evidence: "Follow-Up subsection"

8. ac: "Verify batch sizes produce under 10 min"
   status: deviated
   evidence: "No CI timing data available; tuning guidelines document when to adjust"
--- END UNTRUSTED REQUIREMENTS MAPPING ---
```

## Findings

### AC Coverage Check

All 5 issue ACs (treating AC-2 as a bundle of sub-items) are represented in the mapping. No ACs are missing entirely. There are no phantom ACs.

However, AC-1 from the mapping is narrowed relative to the issue text. The issue says the config should explain "why they differ (e.g., alpine uses 5 because source builds are 3-5x slower than ubuntu)". The mapping reduces this to the simpler claim "docs explaining batch sizes". These are different: one is about per-platform rationale, the other is about general documentation.

### Evidence Verification

**AC-1 (config explains batch sizes and why they differ)**

The diff shows `ci-batch-config.json` was NOT changed in this commit. The file contains only:
```json
{
  "batch_sizes": {
    "test-changed-recipes": { "linux": 15 },
    "validate-golden-recipes": { "default": 20 }
  }
}
```

The workflow-validation-guide.md "Configuration File" subsection explains why the two workflow sizes differ ("test-changed-recipes installs and runs each tool, which takes longer per recipe...so it can handle more recipes per batch"). This is accurate for the two workflows actually implemented.

**Important nuance**: The issue AC mentions alpine as an example of per-platform rationale (alpine=5 because source builds are 3-5x slower). In the design doc, `validate-golden-execution.yml` had per-platform sizes (alpine: 5, rhel: 10, etc.), but that workflow was explicitly out of scope for #1814 and #1815. The actual config only has two entries, neither of which is platform-specific in the alpine sense. The workflow-validation-guide.md explanation correctly addresses the rationale for the two entries that exist. The "alpine=5" example from the AC was aspirational/illustrative in the issue; the actually-implemented config doesn't need alpine-specific rationale because the workflows with alpine variants weren't batched in this series. This is not a gap - the evidence matches what exists.

**Verdict**: The evidence correctly covers the rationale for the existing config entries. The mapping's narrowed AC text ("docs explaining batch sizes") misses the intent of the original ("why they differ"), but the actual implementation does explain the difference. The evidence holds up, but the AC description in the mapping is imprecise.

**AC-2a (what batch sizes control and where configured)**

The "CI Batch Configuration" intro paragraph (line 151) and the "Configuration File" subsection (line 161) both address this. The Configuration File subsection names `.github/ci-batch-config.json` and explains what the values control. Evidence confirmed in diff. **Verified.**

**AC-2b (how to use batch_size_override)**

The "Manual Override via workflow_dispatch" subsection (lines 181-189) provides a 4-step walkthrough, names both workflows that accept it, and notes the value range. Evidence confirmed in diff. **Verified.**

**AC-2c (guidelines for when to increase/decrease)**

The "Tuning Guidelines" subsection (lines 192-205) gives specific thresholds: decrease when batched jobs exceed 10 minutes, increase when most finish in under 3 minutes. Both conditions match the language in the AC exactly. **Verified.**

**AC-2d (valid range 1-50)**

Line 186: "Enter a value in the `batch_size_override` field (1-50, or 0 to use the config default)". The range is stated. Line 189: "Values outside the 1-50 range are clamped automatically with a warning in the workflow log." **Verified.** Both lines cited in the mapping exist and contain the claimed content.

**AC-3 (CONTRIBUTING.md mentions batched jobs)**

The diff adds one paragraph to CONTRIBUTING.md after the CI table. The paragraph says "Recipe CI workflows use **batched jobs**: each check may test multiple recipes rather than one per job." It also explains how to find per-recipe results within a batch job's log, and links to the guide. This satisfies the AC's purpose: a contributor seeing a batch failure will understand why one check covers multiple recipes. **Verified.**

**AC-4 (follow-up note for validate-golden-execution.yml)**

The "Follow-Up: validate-golden-execution.yml" subsection (lines 217-219) names the three specific jobs (`validate-coverage`, `execute-registry-linux`, `validate-linux`) and explains they weren't included due to different matrix shapes. It says "Batching these jobs is tracked as follow-up work." However, the AC says "A note (either in the docs or as a tracked GitHub issue)". The note says "tracked as follow-up work" but does not provide a link to an actual GitHub issue or confirm one was created. The design doc AC text allows either docs or a tracked issue, so the docs note alone satisfies it. **Verified**, though the note is thin on specifics.

**AC-5 (verify batch sizes produce under 10 minutes) - deviated**

The coder marks this as deviated because no CI timing data is available. The deviation is reported as "tuning guidelines document when to adjust." This is a deviation rather than evidence of verification. Evaluation of the deviation's acceptability is a justification concern, not a completeness concern. For completeness purposes: the AC explicitly states "if timing data shows these values need adjustment, update the config file and document the rationale." The coder did not run CI to check timing. The mapping correctly reports this as deviated rather than falsely claiming it implemented.

### Missing ACs

None detected. All five issue-level ACs map to at least one entry in the requirements mapping.

### Phantom ACs

None detected. All eight mapping entries correspond to actual requirements from the issue.

### AC-1 Imprecision: Advisory Finding

The mapping entry `ac: "ci-batch-config.json includes docs explaining batch sizes"` omits the "and why they differ" qualifier. The actual AC explicitly includes this. The implementation does explain why the sizes differ (the Configuration File subsection answers this), so the evidence holds - but the mapping's ac field is a simplified restatement that drops a meaningful constraint. This is a minor mapping hygiene issue: the coder correctly implemented the requirement but wrote a shrunken AC in the mapping. Advisory level only since the implementation itself satisfies the full AC.

## Summary

**Blocking findings**: 0

**Advisory findings**: 1 - The mapping's AC-1 text omits the "why they differ" qualifier from the issue's original AC. The implementation does explain the rationale for different batch sizes, so this doesn't indicate a gap in the implementation itself, but the mapping's evidence description is imprecise and could cause confusion in a future review.

**Overall assessment**: All ACs are covered. Evidence cited in the mapping is present in the diff and plausible for the stated claims. The deviation on AC-5 (timing verification) is correctly reported as a deviation rather than a false "implemented" claim. The implementation is complete for a documentation issue with no timing data available.
