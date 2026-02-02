# Issue 1385 Implementation Plan

## Summary

Wire the ecosystem probe into the chain resolver by constructing builders and passing them to `NewEcosystemProbe`.

## Approach

In `runDiscovery()`, construct all 7 ecosystem builders with nil HTTP client (uses defaults), type-assert each to `builders.EcosystemProber`, and pass the list to `discover.NewEcosystemProbe` with 3-second timeout.

## Files to Modify

- `cmd/tsuku/create.go` — replace stub with wired construction

## Implementation Steps

- [ ] Replace `&discover.EcosystemProbe{}` with builder construction and `NewEcosystemProbe` call
- [ ] Add `"time"` import if not present
- [ ] Run go vet, go test, go build
