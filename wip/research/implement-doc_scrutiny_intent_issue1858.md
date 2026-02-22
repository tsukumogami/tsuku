# Scrutiny Review: Intent Focus -- Issue #1858

## Issue

fix(ci): align batch workflow category names with canonical taxonomy

## Design Document

`docs/designs/DESIGN-structured-error-subcategories.md`

## Requirements Mapping (Untrusted)

```
--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
[
  {"ac": "jq uses canonical category names", "status": "implemented"},
  {"ac": "exit codes 124/137 map to network_error", "status": "implemented"},
  {"ac": "old standalone category names removed", "status": "implemented"}
]
--- END UNTRUSTED REQUIREMENTS MAPPING ---
```

## Sub-check 1: Design Intent Alignment

### Design Doc Description of This Issue's Scope

The design document (Phase 3: CI Workflow Alignment) states:

> Update the inline `jq` in `batch-generate.yml` to use canonical category names. The current mapping (`deterministic`, `network`, `timeout`) becomes (`generation_failed`, `network_error`, `network_error`).

The canonical taxonomy table in the design doc defines:
- Exit code 5 -> `network_error`
- Exit codes 124, 137 -> These are not listed in the canonical table (which maps exit codes 3, 5, 6, 7, 8, 9). The CI workflow section states `"timeout"` is folded into `"network_error"`.
- Default/else -> `generation_failed`
- Exit code 8 -> `missing_dep` (unchanged)

The design doc also says (Changes from current CI workflow taxonomy):
- `deterministic` renamed to `generation_failed`
- `timeout` folded into `network_error` (timeout is a subcategory)
- `network` renamed to `network_error`

### Diff Analysis

The diff shows exactly one line changed in `.github/workflows/batch-generate.yml`:

Old:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 then "network" elif .exit_code == 124 or .exit_code == 137 then "timeout" else "deterministic" end)
```

New:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end)
```

Changes:
1. `"network"` (exit 5) -> `"network_error"` -- matches design intent.
2. `"timeout"` (exit 124, 137) -> folded into the same elif branch as exit 5, producing `"network_error"` -- matches design intent. The timeout branch is eliminated and its exit codes merged with the network branch.
3. `"deterministic"` (else) -> `"generation_failed"` -- matches design intent.
4. `"missing_dep"` (exit 8) -> unchanged -- matches design intent.

### Assessment

The implementation precisely captures the design's intent. The design doc describes a three-name change for the CI workflow (`deterministic`->`generation_failed`, `timeout`->fold into `network_error`, `network`->`network_error`), and all three are present in the diff. The structural simplification (merging the timeout and network branches into one condition) is the natural way to express "timeout is a subcategory, not a top-level category." The `jq` expression remains syntactically valid with balanced parentheses and correct if/elif/else/end structure.

The design doc's implementation issues table marks this as a "simple" tier issue and the implementation is proportionately simple -- a single-line YAML change.

The design document status markers (Mermaid diagram) were also updated from `blocked` to `ready` for #1858 and #1859, which is consistent with #1857 being completed.

**Finding: None.** Design intent is fully captured.

## Sub-check 2: Cross-Issue Enablement

The issue body states "Downstream Dependencies: None. This is a leaf node." The design doc's dependency graph confirms #1858 has no downstream issues.

**Skipped.** No downstream issues to evaluate.

## Backward Coherence Check

Previous summary: "Files changed: .github/workflows/batch-generate.yml. Key decisions: YAML-only change: updated jq category mapping to use canonical taxonomy names (deterministic->generation_failed, network->network_error, timeout->network_error)."

This is actually the summary for the current issue (#1858), not a prior issue. However, reviewing against what #1857 established: the orchestrator's `categoryFromExitCode()` was normalized to use canonical taxonomy names. This issue (#1858) adopts exactly the same names in the CI workflow's jq, which is the whole point -- aligning the second producer with the first. No contradictions with prior work. The category names (`network_error`, `generation_failed`, `missing_dep`) are consistent with what #1857 established in the orchestrator.

**Finding: None.** No backward coherence issues.

## Summary of Findings

| # | AC | Severity | Assessment |
|---|-----|----------|------------|
| -- | -- | -- | No findings. Implementation matches design intent precisely. |

**Blocking findings: 0**
**Advisory findings: 0**
