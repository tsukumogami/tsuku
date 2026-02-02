# Issue 1405 Summary

## What Changed

Added quality filtering to the ecosystem probe as a walking skeleton. The Cargo builder now extracts full quality metadata from the crates.io API (recent downloads, version count, repository presence). A new QualityFilter rejects low-quality matches using per-registry thresholds with OR logic.

## Key Decisions

- **ProbeResult extended instead of returning RegistryEntry**: The plan called for Probe() to return `*discover.RegistryEntry`, but this creates an import cycle (builders -> discover -> builders). Instead, ProbeResult was extended with the same quality fields.
- **Nil return for not-found**: Removed the `Exists` bool field. Nil ProbeResult means not found, simplifying all callers.
- **No logging for rejections**: The project uses `internal/log` (not stdlib log). Since the discover package doesn't have a logger wired in, rejections are silent. The reason string is still computed and available for future use.
- **Stub builders**: Six non-Cargo builders return minimal ProbeResult (Source only). Full metadata extraction is covered by issues #1406-#1410.

## Files Changed

- `internal/builders/probe.go` - Extended ProbeResult, removed Exists/Age fields
- `internal/builders/cargo.go` - Full quality metadata from crates.io API
- `internal/builders/{npm,pypi,gem,go,cpan,cask}.go` - Nil for not-found, stub metadata
- `internal/discover/registry.go` - Extended RegistryEntry with quality fields
- `internal/discover/quality_filter.go` - New: per-registry threshold filter
- `internal/discover/quality_filter_test.go` - New: filter unit tests
- `internal/discover/ecosystem_probe.go` - Wired filter into resolver
- `internal/discover/ecosystem_probe_test.go` - Quality filter integration tests
- `internal/builders/probe_test.go` - Updated for new ProbeResult shape

## Verification

- `go build ./...` passes
- `go vet ./...` passes
- `go test ./...` passes (all packages)
