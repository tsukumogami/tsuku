# Exploration Summary: Discovery Registry Bootstrap

## Problem (Phase 1)
The discovery registry (`recipes/discovery.json`) has 1 entry but needs ~500 to make `tsuku install <tool>` useful for the top developer tools. Without populated data, the registry lookup stage of the discovery resolver returns nothing, forcing every tool resolution through slower ecosystem probes or LLM discovery.

## Decision Drivers (Phase 1)
- Registry entries must map to working builders (github, homebrew, cargo, etc.)
- Each entry's source must exist and not be archived
- Disambiguation overrides needed for known name collisions (bat, fd, serve)
- Reuse existing infrastructure where possible (seed-queue tool, priority queue data)
- The process must be repeatable for ongoing maintenance
- Public visibility: bootstrap tooling should be contributor-friendly

## Research Findings (Phase 2)
- Upstream design (DESIGN-discovery-resolver.md) defines registry format: `{builder, source, binary?}`
- Two entry categories: GitHub-release tools not in ecosystems, and disambiguation overrides
- Existing `cmd/seed-queue/` tool fetches from Homebrew analytics API with tier assignment
- Priority queue has 204 Homebrew entries — but these map to `homebrew` builder, not `github`
- The batch pipeline and discovery registry serve different purposes: batch generates recipes, discovery maps names to builders for first-time resolution
- Shared concern: both need to verify repos exist and aren't archived
- Registry loading code (`internal/discover/registry.go`) validates builder+source fields on load

## Options (Phase 3)
1. Data sourcing: curated list from awesome-cli-apps + GitHub API scraping vs. LLM-assisted bulk generation vs. extend seed-queue tool
2. Validation: Go CLI tool with GitHub API checks vs. shell script vs. CI-only validation
3. Disambiguation: manual curation vs. automated cross-ecosystem collision detection
4. Tooling reuse: new standalone tool vs. extend cmd/seed-queue vs. one-time script

## Current Status
**Phase:** 3 - Options
**Last Updated:** 2026-02-01
