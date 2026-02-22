# Scrutiny Review: Completeness - Issue #1858

## Issue

fix(ci): align batch workflow category names with canonical taxonomy

## Diff Summary

Single file changed: `.github/workflows/batch-generate.yml`

The jq category mapping on line 893 was updated from:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 then "network" elif .exit_code == 124 or .exit_code == 137 then "timeout" else "deterministic" end)
```
to:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end)
```

Additionally, the design doc Mermaid diagram was updated to change #1858 and #1859 from `blocked` to `ready`, and the `wip/implement-doc-state.json` was updated with implementation metadata.

## Issue Acceptance Criteria (extracted from issue body)

1. The `jq` expression uses `"network_error"` instead of `"network"` for exit code 5
2. Exit codes 124 and 137 produce `"network_error"` instead of `"timeout"`
3. The default/else branch produces `"generation_failed"` instead of `"deterministic"`
4. `"missing_dep"` for exit code 8 remains unchanged
5. The `jq` expression still produces valid JSONL output (no syntax errors)

## Requirements Mapping (untrusted, from coder)

--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
1. ac: "jq uses canonical category names", status: "implemented"
2. ac: "exit codes 124/137 map to network_error", status: "implemented"
3. ac: "old standalone category names removed", status: "implemented"
--- END UNTRUSTED REQUIREMENTS MAPPING ---

## Findings

### Finding 1: AC consolidation obscures coverage (advisory)

The issue defines 5 distinct acceptance criteria. The coder's mapping contains only 3 entries, which are paraphrased consolidations rather than 1:1 mappings to the issue's ACs. Specifically:

- Mapping entry "jq uses canonical category names" loosely covers issue ACs #1 (network_error for exit 5), #3 (generation_failed for else), and #4 (missing_dep unchanged), but none are called out individually.
- Mapping entry "exit codes 124/137 map to network_error" maps to issue AC #2 directly.
- Mapping entry "old standalone category names removed" is a summary statement, not a direct AC from the issue. It partially overlaps with ACs #1 and #3.

No individual AC from the issue body is explicitly missing from coverage when the mapping is interpreted generously. However, two specific ACs lack explicit mapping entries:

**AC #4: `"missing_dep"` for exit code 8 remains unchanged** -- not explicitly called out in any mapping entry. The diff confirms `missing_dep` is preserved (the condition `if .exit_code == 8 then "missing_dep"` is unchanged), so this is satisfied in the code but the mapping doesn't call it out.

**AC #5: The jq expression still produces valid JSONL output (no syntax errors)** -- not mentioned in any mapping entry. The diff shows syntactically valid jq. This is inherently difficult to "evidence" without running the expression, but the coder should have at minimum acknowledged this AC.

### Finding 2: Phantom AC -- "old standalone category names removed" (advisory)

The mapping entry "old standalone category names removed" does not correspond to any specific acceptance criterion in the issue body. It's a valid observation about the change, but it's a coder-invented summary rather than a traced AC. This is not blocking since it doesn't substitute for a harder AC; it's additive paraphrasing.

### Finding 3: All "implemented" claims verified against diff (no finding)

For the substance of each claim:

- **"jq uses canonical category names"**: Confirmed. The diff shows the jq expression now uses `"missing_dep"`, `"network_error"`, and `"generation_failed"` -- all canonical taxonomy names per the design doc.
- **"exit codes 124/137 map to network_error"**: Confirmed. The old expression had a separate branch `elif .exit_code == 124 or .exit_code == 137 then "timeout"`. The new expression folds these into the exit code 5 branch: `elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error"`.
- **"old standalone category names removed"**: Confirmed. `"network"`, `"timeout"`, and `"deterministic"` no longer appear in the jq expression.

### Finding 4: Design doc change appropriate (no finding)

The design doc Mermaid diagram change (blocked -> ready for #1858 and #1859) is a reasonable status update reflecting that #1857 was completed.

## Overall Assessment

The implementation is correct and complete against the issue requirements. All 5 issue ACs are satisfied by the code change. The mapping is imprecise in form -- it consolidates 5 ACs into 3 paraphrased entries and introduces one phantom entry -- but this is cosmetic for a single-line YAML change where correctness is straightforward to verify by inspection. No blocking findings.
