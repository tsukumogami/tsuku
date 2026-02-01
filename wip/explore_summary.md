# Exploration Summary: Discovery Registry Bootstrap

## Problem (Phase 1)
The discovery registry has 1 entry but needs ~500 for the resolver to deliver value. The registry is the only resolver stage implemented today. The current schema conflates tool identity with install instructions, blocking future evolution.

## Decision Drivers (Phase 1)
- Serve today's resolver (builder+source required)
- Align with batch pipeline (don't duplicate curation work)
- Disambiguation correctness (durable registry value)
- Graduation path (entries become redundant as recipes appear)
- Enable future evolution (optional metadata fields)

## Research Findings (Phase 2)
- Upstream design defines registry format, resolver code at internal/discover/registry.go
- Recipe metadata already includes description, homepage, tier, dependencies
- Seed-queue tool provides reusable patterns for API clients and JSON I/O
- Batch pipeline shares the priority queue as data source
- Registry scale strategy defines deterministic-only batch pipeline; GitHub Release tools are manual/user-facing

## First Principles (from DESIGN-registry-scale-strategy.md)
- The registry is a pre-computed cache of name-to-builder mappings
- Once a recipe exists, the registry entry is redundant (except disambiguation)
- The batch pipeline generates recipes for ecosystem tools (homebrew, cargo, npm, etc.)
- GitHub Release builder is LLM-only, excluded from automation
- The registry's durable value: disambiguation overrides + GitHub Release tools
- Transitional value: bridge until batch pipeline generates recipes

## Options (Phase 3)
1. Schema: required install + optional metadata (chosen) vs nested objects vs optional install now
2. Sourcing: priority queue + curated seeds (chosen) vs LLM generation vs awesome-list scraping
3. Graduation: exclude entries with recipes except disambiguation (chosen) vs keep all vs remove from seeds
4. Validation: builder-aware + enrichment (chosen) vs validation-only

## Decision (Phase 5)

**Problem:**
The discovery registry has 1 entry but needs ~500. It's the only resolver stage that exists today. The batch pipeline generates recipes for ecosystem tools, but until those recipes exist, the registry bridges the gap. Its durable value is disambiguation overrides and GitHub Release tools the batch pipeline can't automate.

**Decision:**
Evolve schema to v2 with optional metadata. Populate by deriving entries from the priority queue (transitional ecosystem entries) and curated seed lists (durable GitHub Release + disambiguation entries). A Go CLI tool validates, enriches, and cross-references recipes. Entries graduate out as the batch pipeline generates recipes. Disambiguation entries persist regardless.

**Rationale:**
The priority queue already has 204 entries curated for the batch pipeline. Converting those to registry entries is mechanical and avoids duplicate curation. GitHub Release tools need separate curation (~200-300 entries) because they're outside the batch pipeline's scope. The graduation model keeps the registry lean — it converges toward disambiguation + GitHub Release mappings over time.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-01
