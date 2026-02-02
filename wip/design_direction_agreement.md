# Design Direction Agreement: Probe Quality Filtering

**Date:** 2026-02-01

## Agreement

Instead of building quality filtering as a separate inline step in the ecosystem probe (adding fields to `ProbeResult`, filtering in the resolver), the quality metadata should be part of the discovery record format.

### Key Points

1. Discovery JSON files (e.g., `recipes/discovery/b/ba/bash-completion.json`) already represent pre-computed resolutions of "tool name -> registry + metadata." The seeding pipeline that produces these already queries registry APIs.

2. When LLM discovery comes online, it will dynamically produce the same structure. The discovery record becomes the universal interface regardless of source: batch seeding, LLM discovery, or real-time ecosystem probe.

3. Each ecosystem probe should produce a discovery-compatible record enriched with quality signals. The quality filter operates on that common shape, not on raw `ProbeResult` structs with registry-specific field paths.

4. The `QualityFilter` should operate on discovery records, not on `ProbeResult`. This makes reuse between the probe and seeding pipeline natural rather than requiring a separate shared component.

### What This Changes

- The `ProbeResult` extensions proposed in the current design draft (adding `VersionCount`, `HasRepository`, `Downloads`) are reinventing fields that should live in the discovery record format.
- The design's architecture needs to shift from "add fields to ProbeResult and filter in the resolver" to "have each ecosystem produce a discovery-compatible record, then filter on that."
- The two related designs (`DESIGN-discovery-resolver.md` and `DESIGN-registry-scale-strategy.md`) define the context for how this fits together.

### Next Steps

- Read `DESIGN-discovery-resolver.md` and `DESIGN-registry-scale-strategy.md`
- Revise the probe quality filtering design to align with the discovery record as the universal interface
