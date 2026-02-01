# Exploration Summary: Discovery Resolver

## Problem (Phase 1)
tsuku requires --from flags on every create invocation and users must distinguish between install and create. Users expect name-only tool installation.

## Decision Drivers (Phase 1)
- First-impression reliability for top ~500 tools
- No API key requirement for common tools
- Latency budget: registry <100ms, ecosystem <3s, LLM <15s
- Disambiguation correctness
- Build on existing builder infrastructure
- Graceful degradation

## Research Findings (Phase 2)
- Recipe loader already implements priority chain pattern
- Builder registry has CanBuild() for existence checking
- parseFromFlag() produces {builder, source} pairs - same as DiscoveryResult
- internal/discover/ package follows existing conventions

## Options (Phase 3)
- Resolver architecture: sequential chain vs parallel vs registry-then-parallel (chose C)
- Registry storage: embedded vs embedded+sync vs remote-only (chose B)
- Ecosystem filtering: global thresholds vs per-ecosystem vs no filtering (chose A)

## Decision (Phase 5)
**Problem:** tsuku requires --from flags on every create invocation, but users expect tsuku install <tool> to just work without knowing the source.
**Decision:** Three-stage resolver (embedded registry, parallel ecosystem probe, LLM fallback) behind a unified tsuku install entry point, with disambiguation via registry overrides and popularity ranking.
**Rationale:** A registry handles the top ~500 tools instantly without API keys, ecosystem probes cover the middle ground under 3 seconds, and LLM discovery handles the long tail. This layered approach degrades gracefully when keys are missing or APIs are down.

## Current Status
**Phase:** 7 - Security (complete, ready for Phase 8 review)
**Last Updated:** 2026-01-31
