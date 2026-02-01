# Issue 1252 Summary

## What Was Implemented

Added preflight circuit breaker check and per-ecosystem rate limiting to the batch generation pipeline. The issue spec was amended during introspection since ~80% of the original ACs were already complete from #1349, #1350, and #1352.

## Changes Made
- `.github/workflows/batch-generate.yml`: Added preflight step after checkout that reads batch-control.json and fails early if the ecosystem's circuit breaker is open
- `internal/batch/orchestrator.go`: Added `ecosystemRateLimits` map and sleep between package generations in Run()
- `internal/batch/orchestrator_test.go`: Added TestEcosystemRateLimits (map validation) and TestRun_rateLimiting (timing test)
- `docs/designs/DESIGN-batch-recipe-generation.md`: Marked #1252 done, M63 done, promoted #1255 to ready

## Key Decisions
- Shell-based preflight (not Go): Matches existing pattern, fails before Go setup saving CI time
- Rate limit map as package-level var: Simple, testable, overridable in tests
- Timing test with 100ms override: Avoids slow tests while still validating sleep behavior

## Test Coverage
- New tests added: 2 (TestEcosystemRateLimits, TestRun_rateLimiting)
- All existing tests continue to pass

## Known Limitations
- Rate limits are hardcoded; changing them requires a code change
- Preflight check only covers circuit breaker, not budget (advisory only per spec)
