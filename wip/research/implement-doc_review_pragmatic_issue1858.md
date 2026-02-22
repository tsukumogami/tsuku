# Pragmatic Review: Issue #1858

## Issue

fix(ci): align batch workflow category names with canonical taxonomy

## Diff Summary

Single file changed: `.github/workflows/batch-generate.yml`, line 893.

Old jq expression:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 then "network" elif .exit_code == 124 or .exit_code == 137 then "timeout" else "deterministic" end)
```

New jq expression:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end)
```

Three string renames plus a branch merge (exit codes 124/137 folded into the exit code 5 branch). No new abstractions, no new files, no scope creep.

## Findings

None.

This is the simplest correct approach. A mechanical rename of three string literals in one jq expression. The branch merge (collapsing the separate `timeout` branch into the `network` branch) is the natural consequence of the design decision that timeout is a subcategory, not a top-level category. No over-engineering, no speculative generality, no dead code introduced.

## Overall Assessment

Clean. Nothing to flag.
