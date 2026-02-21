# Scrutiny Review: Justification Focus
## Issue 1816: docs(ci): document batch size configuration and tuning
## Scrutiny Focus: justification

---

## Independent Assessment of the Diff

Files changed: `CONTRIBUTING.md`, `docs/workflow-validation-guide.md`, `wip/implement-doc-state.json`

The diff adds a new `## CI Batch Configuration` section to `workflow-validation-guide.md` covering:
- How batching works (ceiling division mechanics)
- The configuration file format and values
- Manual override via `workflow_dispatch` (batch_size_override, 1-50 range)
- Tuning guidelines (when to increase/decrease, thresholds of 10 min / 3 min)
- How to find per-recipe results in the UI
- A follow-up note about `validate-golden-execution.yml`

`CONTRIBUTING.md` gains a single paragraph after the CI workflow table mentioning batched jobs, referencing the config file location and linking to the guide.

The `ci-batch-config.json` file itself was NOT modified in this commit (it was created in prior issues #1814/#1815). It contains no inline comments (JSON does not support them) and no accompanying comment document.

---

## Requirements Mapping (Untrusted Input)

--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---

| AC | Status | Evidence |
|----|--------|----------|
| ci-batch-config.json includes docs explaining batch sizes | implemented | workflow-validation-guide.md Configuration File subsection |
| Section covering what batch sizes control | implemented | workflow-validation-guide.md intro and config file subsections |
| How to use batch_size_override | implemented | Manual Override via workflow_dispatch subsection |
| Guidelines for increasing/decreasing | implemented | Tuning Guidelines subsection |
| Valid range 1-50 | implemented | workflow-validation-guide.md lines 186,189 |
| CONTRIBUTING.md mentions batched jobs | implemented | CONTRIBUTING.md paragraph after CI table |
| Follow-up note for validate-golden-execution.yml | implemented | Follow-Up subsection |
| Verify batch sizes produce under 10 min | deviated | No CI timing data available; tuning guidelines document when to adjust |

--- END UNTRUSTED REQUIREMENTS MAPPING ---

---

## Justification Analysis

### Deviation 1: "Verify batch sizes produce under 10 min"

**Issue AC (verbatim):** "Verify that batch sizes of 15 (for `test-changed-recipes` Linux) and 20 (for `validate-golden-recipes`) produce job durations under 10 minutes for typical PRs (5-20 changed recipes). If timing data shows these values need adjustment, update the config file and document the rationale for the new values."

**Claimed deviation reason:** "No CI timing data available; tuning guidelines document when to adjust"

**Assessment: Advisory**

The reason is factually honest and not evasive. The AC requires active CI verification, which requires actual pipeline runs against a live repository with real recipe installs. In an offline doc-only branch with no new code changes, there is no mechanism to generate that timing data. The claim "no CI timing data available" is plausible given that:

1. This is a documentation issue with no workflow changes that would trigger new CI runs.
2. The prior implementation issues (#1814, #1815) changed the workflows, but the timing data from those runs would be in GitHub Actions history, not in the local branch.

The deviation is proportionate: this is one of eight ACs, and it is explicitly the most operationally-dependent one (requires a live CI environment with changed recipes). The other seven ACs (all documentation) are claimed as implemented, and the diff supports those claims.

**However, the alternative depth is thin.** The mapping entry for this deviation provides no `alternative_considered` field, and the narrative only states what was omitted, not what was examined or reasoned about. A stronger deviation would note:
- Whether the implementer checked CI run history for timing evidence from the prior issues
- Whether the batch sizes were validated as part of prior issue merges (#1814, #1815)
- Why documenting "when to adjust" is an acceptable substitute rather than just explaining the constraint

The deviation is not a disguised shortcut -- verifying real CI timing genuinely requires a live environment -- but the justification is thinner than ideal. This is advisory rather than blocking.

### AC: "ci-batch-config.json includes inline comments or an accompanying doc explaining each workflow's batch size values and why they differ"

**Status claimed:** implemented
**Evidence claimed:** workflow-validation-guide.md Configuration File subsection

**Assessment: Advisory**

The issue AC reads: "`.github/ci-batch-config.json` includes inline comments **or** an accompanying doc explaining each workflow's batch size values and why they differ (e.g., alpine uses 5 because source builds are 3-5x slower than ubuntu)."

The diff confirms `ci-batch-config.json` has no changes and contains no inline comments (JSON does not support them). The "accompanying doc" path is satisfied by the `workflow-validation-guide.md` Configuration File subsection, which includes the sentence: "**Why the sizes differ:** `test-changed-recipes` installs and runs each tool, which takes longer per recipe. Golden file validation just regenerates plans and compares them, so it can handle more recipes per batch."

This satisfies the "or" branch of the AC. The coder's evidence claim is valid.

One observation: the AC's alpine example ("alpine uses 5 because source builds are 3-5x slower") has no counterpart in the documentation. The config file in this branch does not include `validate-golden-execution` platform-specific sizes (alpine: 5, rhel: 10, etc.) -- those are present in the design doc but not in the actual implemented config. The explanation in the guide covers the distinction between the two implemented workflows, not the full design-doc rationale. This is fine given what was actually implemented, but it means the "why they differ" explanation is somewhat narrower than the AC's example implied. This is advisory only.

### No Avoidance Patterns Found in Non-Deviation ACs

The remaining six ACs are all documentation deliverables with clear evidence in the diff:

- **"Section covering what batch sizes control"**: The guide intro and "How Batching Works" subsection directly explain this. Evidence is supported.
- **"How to use batch_size_override"**: The "Manual Override via workflow_dispatch" subsection provides step-by-step instructions. Evidence is supported.
- **"Guidelines for increasing/decreasing"**: The "Tuning Guidelines" subsection provides explicit thresholds (10 min decrease, 3 min increase). Evidence is supported.
- **"Valid range 1-50"**: Line 186 says "1-50, or 0 to use the config default" and line 189 says "Values outside the 1-50 range are clamped automatically". Evidence is supported.
- **"CONTRIBUTING.md mentions batched jobs"**: The paragraph at line 614 of CONTRIBUTING.md directly addresses this. Evidence is supported.
- **"Follow-up note for validate-golden-execution.yml"**: The "Follow-Up: validate-golden-execution.yml" subsection names all three per-recipe steps. Evidence is supported.

### Proportionality Check

One deviation out of eight ACs, on the one AC that requires live CI infrastructure rather than document authorship. The implemented ACs are the ones that a documentation issue can realistically address. The deviation is on a verification AC that was always going to be awkward to fulfill in a docs-only change. Proportionality is reasonable.

---

## Summary

**Blocking findings:** 0

**Advisory findings:** 2

1. The deviation for "Verify batch sizes produce under 10 min" is honest but thin. The reason states the constraint without examining alternatives (e.g., checking CI history from prior issue merges, confirming timing evidence was reviewed before approving those issues). The deviation is not a shortcut disguise -- the AC genuinely requires live CI -- but the justification would be stronger with a note on what was looked at.

2. The "ci-batch-config.json includes docs explaining batch sizes" AC is satisfied via the "or" path (accompanying doc), which the diff supports. The explanation covers why the two implemented workflows differ in batch size. The AC's alpine example (3-5x slower) is absent, but that is because the implemented config does not include the platform-specific validate-golden-execution sizes. This is narrow rather than wrong.

Both findings are advisory. The overall justification quality is adequate: the one deviation is on a legitimately environment-dependent AC, the implemented ACs have evidence confirmed in the diff, and there are no avoidance-pattern deviations or selective-effort signs.
