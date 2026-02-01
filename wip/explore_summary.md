# Exploration Summary: Ecosystem Probe

## Problem (Phase 1)
The discovery resolver chain has a stub ecosystem probe that always returns nil. When a tool isn't in the curated registry, discovery skips straight to LLM search. The ecosystem probe should check package registries (npm, PyPI, crates.io, etc.) first since these are deterministic, fast, and free.

## Decision Drivers (Phase 1)
- Must work within existing SessionBuilder interface without breaking changes
- Parent design specifies download/age filtering, but APIs don't expose this data
- 3-second timeout budget shared across all ecosystem queries
- Must handle partial results (some APIs respond, others timeout)
- Disambiguation needed when multiple ecosystems match

## Research Findings (Phase 2)
- None of the 7 ecosystem APIs expose download counts in their standard response
- Only Go proxy provides a publish date (Time field)
- All builders already have CanBuild() that queries the registry API
- The parent design's filtering thresholds (>90 days, >1000 downloads/month) can't be applied as specified
- Cask should be excluded (requires formula name, same as homebrew)

## Options (Phase 3)
- Decision 1 (Metadata): Existence-only vs additional API calls vs enriched probing
- Decision 2 (Filtering): Skip filtering vs heuristic filtering vs static priority
- Decision 3 (Interface): Reuse CanBuild vs new Probe method vs combined approach

## Decision (Phase 5)

**Problem:**
The ecosystem probe is the second stage of tsuku's discovery resolver chain, but it's currently a stub. Without it, tools not in the curated registry (~500 entries) fall through to LLM-based discovery, which requires an API key and costs money. Most popular developer tools exist in at least one package registry, so a deterministic probe could resolve them for free in under 3 seconds.

**Decision:**
Use the existing CanBuild() method with a lightweight Probe() extension for metadata where available. Query all ecosystem builders in parallel with a 3-second shared timeout. Since registry APIs don't expose download counts, replace popularity-based filtering with a static ecosystem priority list for disambiguation. Single matches are auto-selected; multiple matches use the priority order or prompt the user interactively.

**Rationale:**
The parent design assumed registry APIs would provide download counts and publish dates, but none of the seven ecosystem APIs expose downloads in their standard endpoints. Adding secondary API calls for popularity data would double latency and add fragile dependencies on third-party stats services. The static priority approach is simpler, faster, and deterministic. It can be upgraded to popularity-based ranking later if the metadata becomes available.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-01
