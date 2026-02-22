# Scrutiny Review: Justification Focus -- Issue #1858

## Issue

fix(ci): align batch workflow category names with canonical taxonomy

## Scrutiny Focus

**justification** -- Evaluate the quality of deviation explanations.

## Diff Summary

Single file changed: `.github/workflows/batch-generate.yml`

The jq category mapping at line 893 was updated:
- `"network"` (exit code 5) -> `"network_error"`
- `"timeout"` (exit codes 124, 137) -> folded into `"network_error"` branch
- `"deterministic"` (default) -> `"generation_failed"`
- `"missing_dep"` (exit code 8) -> unchanged

The conditional was also simplified: exit codes 5, 124, and 137 are now handled in a single branch rather than two separate branches.

Additional changes: design doc status markers updated (blocked -> ready), state file metadata updated. These are not substantive implementation changes.

## Requirements Mapping Analysis

### Mapping Entries vs Issue ACs

The issue defines 5 acceptance criteria:

1. jq uses `"network_error"` instead of `"network"` for exit code 5
2. Exit codes 124/137 produce `"network_error"` instead of `"timeout"`
3. Default/else branch produces `"generation_failed"` instead of `"deterministic"`
4. `"missing_dep"` for exit code 8 remains unchanged
5. jq expression still produces valid JSONL output (no syntax errors)

The mapping consolidates these into 3 entries:

1. "jq uses canonical category names" -> covers ACs 1, 3, 4 (partially)
2. "exit codes 124/137 map to network_error" -> covers AC 2
3. "old standalone category names removed" -> covers AC 1, 2, 3 (partially)

AC 5 (valid JSONL output / no syntax errors) has no explicit mapping entry.

### Justification Assessment

**No deviations reported.** All three mapping entries are marked "implemented" with no `reason`, `alternative_considered`, or deviation explanation fields. Since the justification focus evaluates the quality of deviation explanations, and there are no deviations to evaluate, the primary analysis is whether the absence of deviations is honest.

**Verification against diff:** The diff confirms that all three claimed implementations are accurate. The jq expression was updated exactly as described in the issue requirements. The old category names (`network`, `timeout`, `deterministic`) are gone, replaced by canonical names (`network_error`, `generation_failed`). Exit codes 124 and 137 now map to `network_error`. The implementation matches all five issue ACs.

**AC consolidation assessment:** The coder consolidated 5 ACs into 3 mapping entries. This consolidation is reasonable for this issue -- the ACs are fine-grained descriptions of what is essentially one jq expression change. The consolidation does not hide any gaps: every AC is verifiably satisfied by the single-line diff. The unmapped AC 5 (valid JSONL syntax) is implicitly satisfied by the fact that the jq expression follows the same structural pattern as the original.

**Proportionality:** Zero deviations for a simple, mechanical issue is appropriate. This is a single-line YAML change that directly implements what the issue asks for. There is nothing to deviate from.

**Avoidance patterns:** None detected. No "too complex," "out of scope," or "can be added later" language.

## Findings

### Blocking

None.

### Advisory

1. **AC 5 not explicitly mapped** (severity: advisory). The issue's AC "The jq expression still produces valid JSONL output (no syntax errors)" has no corresponding mapping entry. The diff shows the expression is syntactically valid (it follows the same `if/elif/else/end` pattern as the original), so this is satisfied in practice. The coder could have included it as a mapping entry for completeness. This is not blocking because the implementation is correct.

## Overall Assessment

The mapping has no deviations to justify, which is appropriate for this issue. The implementation is a direct, mechanical translation of old category names to canonical names in a single jq expression. The coder's three mapping entries cover all substantive requirements, with one minor AC (syntax validity) left implicit. No blocking findings.
