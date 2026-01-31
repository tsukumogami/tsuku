# Exploration Summary: Structured JSON Output

## Problem (Phase 1)
The CLI uses exit code 6 for all install failures and communicates error details only through human-readable stderr text. Programmatic consumers (batch orchestrator, CI) resort to regex parsing to classify failures.

## Decision Drivers (Phase 1)
- Batch orchestrator already has a TODO requesting JSON output (#1273)
- Several commands already support --json (validate, plan, verify-deps, config)
- Exit code 8 (ExitDependencyFailed) exists but install never uses it
- printJSON() helper already exists and is reusable

## Research Findings (Phase 2)
- install.go exits with ExitInstallFailed (6) for all errors including missing deps
- install_deps.go wraps errors with dep name but no structured metadata
- Batch orchestrator's classifyValidationFailure uses regex on "recipe X not found in registry"
- eval command already outputs JSON unconditionally
- validate command uses --json flag with conditional output pattern

## Options (Phase 3)
- Option A: Fix exit codes only (no JSON)
- Option B: Add --json flag to install + fix exit codes
- Option C: Add --json to install + create, fix exit codes, update all consumers

## Decision (Phase 5)
**Problem:** The CLI lumps all install failures into exit code 6 and provides no machine-readable output, forcing programmatic consumers to parse error text with regex.
**Decision:** Fix exit code conflation in tsuku install (use code 8 for dependency failures, code 3 for missing recipes) and add a --json flag that emits structured error details. Update the batch orchestrator to use the new exit codes, removing regex parsing.
**Rationale:** Correct exit codes are the highest-value change since the orchestrator already uses categoryFromExitCode(). Adding --json provides richer detail for consumers that need it, following the existing pattern from validate and plan commands.

## Current Status
**Phase:** 7 - Security
**Last Updated:** 2026-01-31
