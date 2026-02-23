# Pragmatic Review: Issue #1947

## Blocking Findings

None.

## Advisory Findings

**1. Dual JSON format detection could be a single jq expression**

`batch-generate.yml:277-283` (x86_64) and `batch-generate.yml:429-435` (arm64) -- The two-step `install_exit_code` / `exit_code` fallback is correct (sandbox JSON vs error JSON emit different field names), but the three-branch if/elif/else could be a single jq expression: `jq -r '.install_exit_code // .exit_code // 1'`. Reduces 7 lines to 1 per occurrence (4 occurrences across x86_64 and arm64 jobs). Minor -- the explicit form is clear and the `jq -e` check is marginally safer against `null` vs missing key ambiguity. **Advisory.**

**2. Pre-existing: LIBCS parallel array vs if/then derivation inconsistency**

`batch-generate.yml:232` uses `LIBCS=("glibc" "glibc" "glibc" "glibc" "musl")` parallel to `NAMES`. The migrated `recipe-validation-core.yml:134-137` uses the simpler `if alpine then musl else glibc` pattern. The parallel array is fragile (adding a family requires updating two arrays in sync) but was pre-existing and not introduced by this commit. Noting for awareness, not actionable here. **Advisory.**
