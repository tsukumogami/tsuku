# Issue 1252 Implementation Plan

## Summary

Add preflight circuit breaker check to batch-generate.yml workflow and per-ecosystem rate limiting to the Go orchestrator (internal/batch/orchestrator.go).

## Approach

Based on the introspection analysis, the original issue spec is outdated. The implementation should focus on two specific features:

1. **Preflight circuit breaker check**: Add a workflow step in batch-generate.yml that reads batch-control.json BEFORE the generation job starts. If the ecosystem's circuit breaker is open, fail early with a clear error message. This prevents wasting CI resources on ecosystems with known issues.

2. **Rate limiting**: Add per-ecosystem sleep between package generations in the Go orchestrator's Run() method. Sleep durations: 1s for most ecosystems, 6s for rubygems (10 req/min limit).

The circuit breaker state is already being written by scripts/update_breaker.sh (#1352), but it's never read. Similarly, batch ID generation is already done (#1349). We don't need to create Go structs for batch-control.json because it's shell-managed, and we don't need a separate preflight job—a preflight step within the existing workflow is sufficient.

### Alternatives Considered

**Alternative 1: Go-based preflight tool**
Create a Go CLI tool (`cmd/batch-preflight`) that reads batch-control.json, checks circuit breaker state, validates budget, and outputs package lists. The workflow would call this tool in a preflight step.

Why not chosen: Over-engineered for the current need. The circuit breaker check can be done in 5 lines of shell using jq. A Go tool adds complexity (parsing JSON, error handling, testing) without significant benefit. The shell approach matches the existing pattern (scripts/update_breaker.sh is shell-based).

**Alternative 2: Rate limiting via workflow sleep**
Add `sleep 1` in the batch-generate.yml shell script between package generation calls instead of implementing it in Go.

Why not chosen: The workflow doesn't loop over packages—the orchestrator does. The workflow calls `./batch-generate` once, which processes all packages internally. Adding sleep to the Go code is more natural and allows for per-ecosystem configuration (rubygems needs 6s, others need 1s).

**Alternative 3: Circuit breaker check in Go orchestrator**
Read batch-control.json in the Go orchestrator's Run() method and return early if circuit is open.

Why not chosen: While technically cleaner (keeps all logic in Go), it wastes CI time building binaries and setting up the environment before discovering the circuit is open. The workflow-level check fails fast (before go build) and provides better UX with a GitHub Actions error annotation.

## Files to Modify

- `.github/workflows/batch-generate.yml` - Add preflight step (lines 39-66) to check circuit breaker state before generation
- `internal/batch/orchestrator.go` - Add rate limiting sleep in Run() method's package processing loop (after line 76)

## Implementation Steps

- [ ] Add preflight step to batch-generate.yml
  - Insert new step after checkout (line 44) and before "Build binaries" (line 50)
  - Read batch-control.json with jq
  - Check if `.circuit_breaker[$ecosystem].state == "open"`
  - If open, fail with `::error::` annotation containing ecosystem, opened_at, and reason
  - Exit with non-zero status to prevent downstream jobs from running
- [ ] Add rate limit configuration to orchestrator
  - Define rate limit map at package level: `var ecosystemRateLimits = map[string]time.Duration{...}`
  - Map entries: homebrew=1s, cargo=1s, npm=1s, pypi=1s, go=1s, rubygems=6s, cpan=1s, cask=1s
- [ ] Add rate limiting sleep to Run() method
  - After line 76 (`o.setStatus(pkg.ID, "in_progress")`), check if this is not the first package
  - If not first, get sleep duration from ecosystemRateLimits map using o.cfg.Ecosystem
  - Sleep for that duration
  - Add comment explaining rate limit purpose (ecosystem API limits)
- [ ] Add unit test for rate limiting
  - Create test in orchestrator_test.go that verifies sleep duration
  - Use time.Since() to measure elapsed time for multi-package batch
  - Assert that duration >= (packageCount - 1) * expectedSleep

## Testing Strategy

- Unit tests:
  - `internal/batch/orchestrator_test.go`: Add test for rate limiting (verify sleep duration for 3-package batch)
  - Test with rubygems ecosystem (6s sleep) and cargo ecosystem (1s sleep)
  - Mock time.Sleep if necessary to avoid slow tests, or use small package counts (2-3)

- Integration tests:
  - Manual workflow run with open circuit breaker (should fail in preflight step)
  - Manual workflow run with closed circuit breaker (should proceed to generation)
  - Check workflow logs for error annotations with correct format

- Manual verification:
  - Trigger batch-generate workflow for an ecosystem
  - Watch generation logs to confirm 1s (or 6s for rubygems) delay between packages
  - Artificially set circuit breaker to open in batch-control.json and verify workflow fails at preflight

## Risks and Mitigations

- **Risk**: Preflight check fails to read batch-control.json (file not found, invalid JSON)
  - **Mitigation**: Use `jq -e` for existence check; on parse error, fail with clear message. The file is already in the repo and has a valid schema.

- **Risk**: Rate limiting delays are too aggressive and slow down batch runs unnecessarily
  - **Mitigation**: Conservative defaults (1s) match design doc table. RubyGems 6s aligns with documented 10 req/min limit. Limits can be tuned later based on real API responses.

- **Risk**: Sleep doesn't prevent rate limit errors if tsuku CLI makes multiple API calls per package
  - **Mitigation**: Rate limiting is best-effort at this phase. The retry logic (MaxRetries=3 for ExitNetwork) handles transient rate limit errors. Future work can add token bucket or per-request throttling if needed.

## Success Criteria

- [ ] Preflight step in workflow checks circuit breaker state and fails if open
- [ ] Error annotation includes ecosystem name, opened_at timestamp, and reason
- [ ] Orchestrator sleeps between package generations (not before first package)
- [ ] Sleep duration matches ecosystem: 6s for rubygems, 1s for others
- [ ] Unit test validates rate limiting sleep duration
- [ ] Manual workflow run with open circuit breaker fails at preflight
- [ ] go test ./internal/batch/... passes
- [ ] go build ./cmd/batch-generate succeeds

## Open Questions

None. The amended scope clarifies that we only need workflow-level preflight checks and Go-level rate limiting, not a full preflight job or Go structs for batch-control.json.
