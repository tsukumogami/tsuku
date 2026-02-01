# Issue 1384 Implementation Plan

## Summary

Replace the stub `EcosystemProbe` in `internal/discover/ecosystem_probe.go` with a parallel resolver. One file to modify, one test file to create.

## Approach

Replace the empty struct and stub method with:
- `NewEcosystemProbe()` constructor with probers, timeout, and static priority map
- `Resolve()` that fans out goroutines, collects via buffered channel, filters, ranks, returns

## Files to Modify

- `internal/discover/ecosystem_probe.go` — replace stub with full implementation

## Files to Create

- `internal/discover/ecosystem_probe_test.go` — tests for all acceptance criteria

## Implementation Steps

- [ ] Add imports, struct fields, constructor, priority map
- [ ] Implement Resolve() with goroutine fan-out and channel collection
- [ ] Add filtering (Exists check, name match) and priority ranking
- [ ] Handle 0/1/N result cases and all-failures error case
- [ ] Write unit tests: timeout, priority, zero/single/multi results, name mismatch, all failures, soft errors
- [ ] Run go vet, go test, go build
