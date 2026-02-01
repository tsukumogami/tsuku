# Issue 1383 Summary

## What Was Implemented

Added `Probe()` method to all 7 ecosystem builders (cargo, pypi, npm, gem, go, cpan, cask) implementing the `EcosystemProber` interface. Each method wraps the builder's existing registry fetch call and returns a `ProbeResult` with existence, source, and optional age metadata.

## Changes Made

- `internal/builders/probe.go`: New file with `ProbeResult` struct and `EcosystemProber` interface (moved from discover package)
- `internal/discover/ecosystem_probe.go`: Removed `ProbeResult` and `EcosystemProber` (moved to builders to avoid import cycle)
- `internal/builders/cargo.go`: Added `CargoBuilder.Probe()` wrapping `fetchCrateInfo`
- `internal/builders/pypi.go`: Added `PyPIBuilder.Probe()` wrapping `fetchPackageInfo`
- `internal/builders/npm.go`: Added `NpmBuilder.Probe()` wrapping `fetchPackageInfo`
- `internal/builders/gem.go`: Added `GemBuilder.Probe()` wrapping `fetchGemInfo`
- `internal/builders/go.go`: Added `GoBuilder.Probe()` wrapping `fetchModuleInfo` with Age calculation
- `internal/builders/cpan.go`: Added `CPANBuilder.Probe()` with `normalizeToDistribution` before fetch
- `internal/builders/cask.go`: Added `CaskBuilder.Probe()` wrapping `fetchCaskInfo`
- `internal/builders/probe_test.go`: Tests for all 7 builders plus API error handling

## Key Decisions

- **Moved types to builders package**: `ProbeResult` and `EcosystemProber` moved from `discover` to `builders` to avoid import cycle (discover imports builders, so builders can't import discover).
- **Source field uses raw name**: `Source` is the tool name as-is (or normalized for CPAN), not a prefixed format. This matches the design's intent for the resolver to pass the source to the builder's `Build()` method.

## Test Coverage

- New tests added: 22 test cases across 8 test functions
- Compile-time interface assertions for all 7 builders
- Covers: exists, not found, API error (500), CPAN module notation normalization, Go invalid time handling
