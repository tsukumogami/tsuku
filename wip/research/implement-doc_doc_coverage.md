# Documentation Coverage Report: sandbox-ci-integration

## Summary

- Total entries: 3
- Updated: 0
- Skipped: 0 (no prerequisite issues were skipped)
- Gaps: 3

All 6 prerequisite issues (#1942, #1943, #1944, #1945, #1946, #1947) completed successfully. No issues were skipped. All 3 doc entries had their prerequisites met but were not updated during implementation.

## Gaps

| Entry | Doc Path | Reason |
|-------|----------|--------|
| doc-1 | README.md (Sandbox Testing section) | Prerequisites #1942, #1943, #1944 all completed, but docs not updated. Missing: (1) recipe verification details, (2) `--env` flag examples, (3) `--json` flag examples. |
| doc-2 | docs/ENVIRONMENT.md (new Sandbox section) | Prerequisite #1943 completed, but docs not updated. Missing: sandbox-hardcoded environment variables (`TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, `PATH`), override behavior, and summary table entry for `TSUKU_SANDBOX`. |
| doc-3 | docs/GUIDE-plan-based-installation.md (Plan Format section) | Prerequisite #1942 completed, but docs not updated. The guide shows `format_version: 2` in both the example JSON (line 214) and the key fields table (line 246), but the actual version is now 5. The guide doesn't cover verify fields, so only the version number needs updating. Note: the version was already stale before this design (stuck at 2 while the code was at 4); this design bumped it from 4 to 5. |

## Detail by Entry

### doc-1: README.md -- Sandbox Testing

**Status:** Not updated
**Prerequisites met:** Yes (#1942, #1943, #1944 all completed)

Three changes were expected:
1. Update the "Verifies the tool installs and runs correctly" bullet to mention recipe `[verification]` section usage (from #1942)
2. Add `--env KEY=VALUE` flag usage examples for environment variable passthrough, including the `--env KEY` host-read form (from #1943)
3. Add `--json` flag usage example with JSON schema field descriptions (`passed`, `verified`, `install_exit_code`, `verify_exit_code`, `duration_ms`, `error`) (from #1944)

### doc-2: docs/ENVIRONMENT.md -- Sandbox section (new)

**Status:** Not updated
**Prerequisites met:** Yes (#1943 completed)

Expected a new "Sandbox" section documenting:
- Five hardcoded container variables: `TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, `PATH`
- Behavior: user-provided `--env` keys matching these are silently dropped
- `TSUKU_SANDBOX` added to the summary table
- Note that `TSUKU_REGISTRY_URL` is consumed on the host during plan generation, not inside the container

### doc-3: docs/GUIDE-plan-based-installation.md -- Plan Format

**Status:** Not updated
**Prerequisites met:** Yes (#1942 completed)

The guide references `format_version` in two places:
- Line 214: Example JSON shows `"format_version": 2`
- Line 246: Table describes it as "Plan schema version (currently 2)"

The actual format version is now 5. The guide doesn't cover verify fields or the `PlanVerify` struct, so only the version number is relevant. The version discrepancy predates this design (was at 2 while code moved through 3, 4, and now 5).
