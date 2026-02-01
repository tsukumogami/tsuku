# Exploration Summary: Discovery Registry Bootstrap

## Problem (Phase 1)
The discovery registry has 1 entry but needs ~500 for the resolver to deliver value. The current schema conflates tool identity with install instructions, blocking future evolution.

## Decision Drivers (Phase 1)
- Serve today's resolver (builder+source required)
- Enable future evolution (optional metadata fields)
- Separate identity from install path
- Entry accuracy and contributor-friendliness

## Research Findings (Phase 2)
- Upstream design defines registry format, resolver code at internal/discover/registry.go
- Recipe metadata already includes description, homepage, tier, dependencies
- Seed-queue tool provides reusable patterns for API clients and JSON I/O
- Batch pipeline shares validation concerns but serves different purpose

## Options (Phase 3)
1. Schema: required install + optional metadata (chosen) vs nested objects vs optional install now
2. Sourcing: Go CLI with seed lists (chosen) vs LLM generation vs awesome-list scraping
3. Validation: builder-aware + enrichment + CI freshness (chosen) vs validation-only vs PR-time CI
4. Disambiguation: automated detection + manual resolution (chosen) vs fully automated

## Decision (Phase 5)

**Problem:**
The discovery registry has 1 entry but needs ~500 for the resolver to deliver value. The current schema conflates tool identity (where it's maintained, what it does) with install instructions (which builder to use). This blocks future evolution: an LLM builder could infer install paths from metadata, and richer data improves disambiguation UX. The bootstrap must populate entries that serve today's resolver while collecting metadata for future builders and features.

**Decision:**
Evolve the registry schema to v2: keep builder+source as required fields (today's resolver works unchanged), add optional metadata fields (repo, homepage, description, disambiguation flag). A Go CLI tool reads curated seed lists, validates via builder-specific API checks, enriches with metadata from the same API responses, detects ecosystem name collisions, and outputs discovery.json. CI runs weekly freshness checks. Schema v3 (optional install fields) is deferred until an LLM builder exists to consume metadata-only entries.

**Rationale:**
Separating tool identity from install path is a low-cost, backward-compatible schema change that collects useful data now without requiring consumers. Keeping builder+source required avoids premature abstraction. Curated seed lists focus manual effort on the high-value decision (correct source for each tool) while automating mechanical work (description, homepage, validation). The Go tool reuses patterns from existing seed-queue infrastructure.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-01
