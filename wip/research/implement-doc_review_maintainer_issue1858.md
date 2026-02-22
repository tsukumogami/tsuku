# Maintainer Review: Issue #1858

## Issue

fix(ci): align batch workflow category names with canonical taxonomy

## Review Focus

Maintainability: clarity, readability, duplication, naming, consistency.

## Scope

Single file changed: `.github/workflows/batch-generate.yml`, line 893.

Old:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 then "network" elif .exit_code == 124 or .exit_code == 137 then "timeout" else "deterministic" end)
```

New:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end)
```

Design doc Mermaid diagram updated (status change from blocked to ready for #1858 and #1859).

## Findings

### Finding 1: Divergent category mapping between jq and Go -- intentional but undocumented (advisory)

**File**: `.github/workflows/batch-generate.yml:893` vs `internal/batch/orchestrator.go:491-508`

The jq expression maps exit codes 124 and 137 to `"network_error"`. The Go `categoryFromExitCode()` has no case for 124 or 137 -- they'd fall to `default` and produce `"generation_failed"`.

This divergence is intentional: 124/137 are shell-level timeout codes from the `timeout`/`gtimeout` command wrapping the tsuku binary in CI (lines 277, 434, 574), so the Go function never sees them. But the next developer looking at the jq expression and the Go function side by side will wonder why they handle different exit codes and may try to "fix" the Go function to match, or vice versa.

A brief inline comment on line 893 would prevent that misread:

```yaml
# Exit codes 124/137 come from the shell timeout wrapper, not from tsuku itself
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end),
```

**Severity**: Advisory. The divergence won't cause a bug since these code paths can't produce each other's exit codes. But the trap is real for someone maintaining both files.

### Finding 2: Stale comment in internal/builders/errors.go (out of scope)

`internal/builders/errors.go:135` says "Values match the category enum in failure-record.schema.json." After #1857 changed the schema to use the canonical taxonomy, `api_error` and `validation_failed` in that file no longer match the schema enum. This is a pre-existing issue from #1857, not introduced by #1858, so it's out of scope for this review. Noting it for awareness.

### No other findings

The change is minimal and correct:
- Category string names (`network_error`, `generation_failed`, `missing_dep`) are consistent with the orchestrator's `categoryFromExitCode()` and the schema enum.
- The jq syntax is valid (balanced if/elif/else/end).
- No old category names (`"network"`, `"timeout"`, `"deterministic"`) remain anywhere in the workflow file.
- No other scripts or workflow files reference the old pipeline category names.
- The structural simplification (merging timeout exit codes into the network branch) is the natural way to express "timeout is a subcategory of network_error."

## Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | Divergent exit code handling between jq and Go function -- intentional but a comment would prevent misreads | advisory |

**Blocking findings: 0**
**Advisory findings: 1**
