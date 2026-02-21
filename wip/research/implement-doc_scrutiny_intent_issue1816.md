# Scrutiny Review: Intent Focus — Issue #1816

**Issue**: #1816 docs(ci): document batch size configuration and tuning
**Design doc**: docs/designs/DESIGN-recipe-ci-batching.md
**Scrutiny focus**: intent
**Date**: 2026-02-21

---

## Independent Impression from the Diff

Files changed in the issue commit:
- `CONTRIBUTING.md` — added one paragraph after the CI workflow table
- `docs/workflow-validation-guide.md` — added a "CI Batch Configuration" section (~78 lines)
- `wip/implement-doc-state.json` — state tracking file (not substantive)

The `docs/workflow-validation-guide.md` addition covers:
1. An introductory paragraph explaining why batching exists
2. "How Batching Works" subsection with ceiling-division explanation and numeric example
3. "Configuration File" subsection with JSON snippet and explanation of why test-changed-recipes (15) vs validate-golden-recipes (20) sizes differ
4. "Manual Override via workflow_dispatch" subsection with numbered steps
5. "Tuning Guidelines" subsection with "when to decrease / when to increase" guidance and explicit thresholds (10 min, 3 min)
6. "Finding Per-Recipe Results" subsection explaining how to navigate batched job output
7. "Follow-Up: validate-golden-execution.yml" subsection noting the three unbatched job types

The `CONTRIBUTING.md` addition is a single paragraph after the CI workflow table directing contributors to `ci-batch-config.json` and the guide's anchor link.

---

## Sub-check 1: Design Intent Alignment

### AC: ".github/ci-batch-config.json includes inline comments or an accompanying doc"

**Issue text**: "explaining each workflow's batch size values and why they differ (e.g., alpine uses 5 because source builds are 3-5x slower than ubuntu)"

**What the design says**: The Solution Architecture section specifies the config file format with per-workflow, per-platform batch sizes including `validate-golden-execution` with alpine=5, rhel=10, suse=10, arch=8. The example in the issue AC references alpine specifically.

**What the implementation does**: The Configuration File subsection in the guide includes a JSON snippet showing only the two workflows that were batched (test-changed-recipes and validate-golden-recipes). It explains why *those two* differ ("test-changed-recipes installs and runs each tool... Golden file validation just regenerates plans"). It does not explain alpine vs. ubuntu differences because alpine batching is out of scope for this milestone.

**Assessment**: The "why they differ" requirement is satisfied for the workflows that were actually batched. The alpine example in the AC was illustrative, not prescriptive — it described an expected future state when validate-golden-execution is also batched. The implemented explanation is accurate to the current state of `ci-batch-config.json`. The mapping claim is accurate.

**Severity**: Advisory. The doc is accurate but a reader of the full design doc may wonder why the validate-golden-execution per-platform sizes (present in the design's config example) are absent from the actual file and the documentation. The guide's Follow-Up section addresses this gap in coverage, but not explicitly in the Configuration File subsection.

---

### AC: "Section covering what batch sizes control and where they're configured"

**Design doc context**: Phase 3 specifies "Document the batch size parameter and how to tune it." The design overview describes the config file and workflow_dispatch override.

**Implementation**: The guide's introductory paragraph plus the "Configuration File" and "How Batching Works" subsections cover both what batch sizes control (number of jobs, recipes-per-job tradeoff) and where they're configured (ci-batch-config.json). Evidence is solid.

**Severity**: None. Satisfied.

---

### AC: "How to use batch_size_override input on workflow_dispatch"

**Design doc context**: "For `workflow_dispatch`, an optional `batch_size_override` input takes precedence over the config file when provided. This lets contributors experiment with different sizes on manual runs without committing config changes."

**Implementation**: The "Manual Override via workflow_dispatch" subsection provides 4 numbered steps and a narrative explanation. The actual workflow YAML confirms this input exists with exactly this behavior. The doc correctly describes the 0-means-default behavior.

**Severity**: None. Satisfied with strong evidence.

---

### AC: "Guidelines for when to increase or decrease batch sizes"

**Design doc context**: The rationale section mentions "If CI timing data reveals that 15 recipes regularly approach the timeout, the batch size can be reduced without code changes." The design emphasizes the 10-minute bound as the trigger for concern.

**Implementation**: The "Tuning Guidelines" subsection explicitly uses the 10-minute threshold for decreasing and 3-minute threshold for increasing, matching the issue body's examples verbatim. Consistent with design intent.

**Severity**: None. Satisfied.

---

### AC: "Valid range for batch sizes (1-50, enforced by the guard clause)"

**Design doc context**: "A guard clause clamps all values to the range 1-50."

**Implementation**: Line 186 in the guide: "Enter a value in the `batch_size_override` field (1-50, or 0 to use the config default)". Line 189: "Values outside the 1-50 range are clamped automatically with a warning in the workflow log." The workflow YAML confirms clamping with `::warning::` annotations. The doc uses "clamped" rather than "guard clause" — this is appropriate user-facing language for the same behavior.

**Severity**: None. Satisfied.

---

### AC: "CONTRIBUTING.md mentions that recipe CI uses batched jobs"

**Design doc context**: No explicit CONTRIBUTING.md guidance in the design doc. The issue AC specifies the purpose: "so contributors understand why a single check covers multiple recipes."

**Implementation**: The CONTRIBUTING.md paragraph reads: "Recipe CI workflows use **batched jobs**: each check may test multiple recipes rather than one per job. If a batch fails, expand the job's log in the GitHub Actions UI and look for the `::group::` section of the specific recipe to find per-recipe results."

This directly addresses the stated purpose (why a single check covers multiple recipes, how to find per-recipe results). Well placed after the CI workflow table.

**Severity**: None. Satisfied.

---

### AC: "A note records the follow-up: validate-golden-execution.yml has three per-recipe matrix jobs that should be batched"

**Issue text**: "A note (either in the docs or as a tracked GitHub issue) records the follow-up... with per-platform batch sizes already defined in the config file"

**Design doc context**: The scope section lists validate-golden-execution as explicitly out of scope: "Per-recipe jobs in `validate-golden-execution.yml` (`validate-coverage`, `execute-registry-linux`, `validate-linux`) -- these have the same problem but different matrix shapes and should be batched in a follow-up."

**Implementation**: The "Follow-Up: validate-golden-execution.yml" subsection says: "Batching these jobs is tracked as follow-up work." The text implies an active tracking artifact exists ("is tracked"), but no link to a GitHub issue is provided and no such issue was found in the repository (gh issue list search returned no dedicated follow-up issue). The note also does not mention that "per-platform batch sizes already defined in the config file" — the actual ci-batch-config.json contains no validate-golden-execution entries.

**Finding — ADVISORY**: The AC allows a doc note as sufficient and the note exists. However, the text "is tracked as follow-up work" overstates the situation since no tracking issue exists and the per-platform batch sizes mentioned in the AC's description are not in the config file. The second omission (per-platform sizes already defined) is more significant as a design-intent gap: the design doc's config example shows alpine=5, rhel=10, etc., and the AC specifically says "with per-platform batch sizes already defined in the config file." The implementation did not pre-populate these values, so this detail cannot be accurately noted.

**Severity**: Advisory. The note exists and the AC's "either in the docs or as a tracked GitHub issue" means the doc note satisfies the literal requirement. The language "is tracked" is slightly misleading given no issue exists, but it's not a blocking gap. The omission of "per-platform batch sizes already defined" is an inaccuracy — the design expected these values to be pre-seeded so future work could reference them, but they weren't added.

---

### AC: "Verify that batch sizes of 15 and 20 produce job durations under 10 minutes"

**Issue text**: "If timing data shows these values need adjustment, update the config file and document the rationale for the new values."

**Design doc context**: Phase 3 explicitly says "Verify batch size of 15 produces reasonable job durations for typical PRs." The design rationale states: "The batch size of 15 is a conservative starting point: it caps a 300-recipe PR at 20 jobs per workflow while keeping individual jobs under 10 minutes (assuming ~30s average per recipe)."

**Implementation**: Marked as "deviated" with evidence "No CI timing data available; tuning guidelines document when to adjust."

**Assessment**: The deviation reason is factually accurate — the branch doesn't have CI run history showing actual job durations. The design doc's rationale itself provides a theoretical basis (30s average × 15 recipes = ~7.5 minutes), so the 10-minute bound is already justified analytically in the design. The AC says "verify" which implies empirical confirmation, not analytical reasoning. The deviation is legitimate but the mapping could have cited the design doc's own timing analysis as the basis for confidence in the current values.

**Finding — ADVISORY**: The deviation is honest and the reason is valid. The design doc provides analytical justification for the current batch sizes that the doc could have cited. Not blocking since CI timing data genuinely wasn't available and the AC itself says "if timing data shows adjustment is needed" — implying the action is conditional on finding a problem, not mandatory regardless.

---

## Sub-check 2: Cross-Issue Enablement

The downstream issues list is empty — this is the terminal issue in the sequence. Cross-issue enablement check is skipped per the review protocol.

---

## Backward Coherence

**Previous summary**: "Files changed: .github/ci-batch-config.json, .github/workflows/validate-golden-recipes.yml. Key decisions: Removed per-recipe actions/cache entirely rather than switching to batch-level cache keys. Golden validation is CPU-bound so download caching adds no benefit."

**Coherence check**: The documentation does not contradict the previous issue's decisions. The guide's "Configuration File" section accurately shows ci-batch-config.json content matching what #1815 established. The guide does not mention download caching, which is appropriate since that optimization was explicitly removed. No contradictions in naming conventions, pattern descriptions, or configuration format.

**Severity**: None. Consistent with prior work.

---

## Summary of Findings

| Finding | Severity | AC |
|---------|----------|----|
| Follow-up section says "is tracked" but no issue exists; also omits that per-platform sizes are not yet in the config file | Advisory | Follow-up note AC |
| Batch size verification deviated with valid reason, but could have cited design doc's analytical justification | Advisory | Verify timing AC |
| Config file explanation omits that design expected alpine/rhel/suse/arch sizes to be pre-seeded | Advisory | ci-batch-config.json AC |

**Blocking findings**: 0
**Advisory findings**: 3
