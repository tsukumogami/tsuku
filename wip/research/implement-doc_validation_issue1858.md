# Validation Results: Issue #1858

## Scenario 10: CI workflow jq uses canonical category names

**ID**: scenario-10
**Status**: passed

### Checks Performed

1. **`"network_error"` appears at least once in batch-generate.yml**
   - Result: PASS
   - Found on line 893 in the jq category-mapping expression

2. **`"generation_failed"` appears at least once in batch-generate.yml**
   - Result: PASS
   - Found on line 893 in the jq category-mapping expression

3. **`"network"` as a standalone category value no longer appears**
   - Result: PASS
   - No matches found. The string "network" only appears as part of "network_error".

4. **`"timeout"` as a standalone category value no longer appears**
   - Result: PASS
   - No matches found.

5. **`"deterministic"` as a standalone category value no longer appears**
   - Result: PASS
   - No matches found.

### Category Mapping Expression

The single jq category-mapping expression in the workflow (line 893):

```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end),
```

This confirms:
- Exit code 8 maps to `"missing_dep"` (unchanged)
- Exit codes 5, 124, and 137 all map to `"network_error"` (124 and 137 were previously mapped to `"timeout"`)
- All other exit codes map to `"generation_failed"` (was previously `"deterministic"` or similar)
- No legacy category names (`"api_error"`, `"validation_failed"`, `"deterministic_insufficient"`, `"deterministic"`, `"timeout"`, `"network"`) appear anywhere in the file

### Additional Verification

A search for all occurrences of the word "category" in the workflow file found only the single jq expression above. There are no other category-mapping expressions that could use legacy names.
