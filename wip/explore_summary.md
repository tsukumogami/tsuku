# Exploration Summary: Probe Quality Filtering

## Problem (Phase 1)
Ecosystem probe accepts name-squatted packages as valid matches because Probe() only checks existence, not quality. This causes tools to resolve to the wrong registry.

## Decision Drivers (Phase 1)
- Correctness over speed
- No extra latency where possible
- Registry-specific signals
- Reusability with discovery registry seeding
- Fail-open for unknowns

## Research Findings (Phase 2)
- crates.io and RubyGems expose downloads in their standard API response
- npm has a separate downloads API
- PyPI has no downloads API (BigQuery only)
- Go proxy has minimal metadata but squatting is rare due to domain-based naming
- Cask is curated, no squatting concern
- All 7 Probe() implementations only check existence; none populate Downloads field

## Options (Phase 3)
- Decision 1: Where to filter → Shared QualityFilter component
- Decision 2: What metadata to collect → Downloads + version count + repository URL
- Decision 3: How to set thresholds → Per-registry minimums with conservative defaults

## Decision (Phase 5)

**Problem:**
The ecosystem probe treats any name that exists on a registry as a valid match, with no quality assessment. Name squatting is widespread on crates.io, npm, and pypi, so common tool names like prettier and httpie resolve to placeholder packages instead of their actual registries.

**Decision:**
Extend Probe() to return quality metadata (downloads, version count, repository presence) from existing API responses, and filter results through a shared QualityFilter with per-registry minimum thresholds. Packages that fail all thresholds are rejected. Cask and Go are exempt.

**Rationale:**
Most registries expose quality signals in their standard endpoint, so collecting the data adds no latency. Per-registry thresholds handle the heterogeneity across registries. A shared filter component avoids duplicating logic between the runtime probe and discovery registry seeding.

## Current Status
**Phase:** 5 - Decision (complete through Phase 7)
**Last Updated:** 2026-02-02
