# Exploration Summary: Automated Seeding

## Problem (Phase 1)
The batch pipeline's unified queue contains 5,275 packages but 97% come from Homebrew. Other ecosystems have popular CLI tools that aren't queued. The existing seed-queue command only fetches from Homebrew analytics, so multi-ecosystem coverage requires manual queue edits. Queue entries lack freshness tracking for disambiguation decisions.

## Decision Drivers (Phase 1)
- Reuse existing `internal/seed.Source` interface and `internal/discover` probers
- Minimize ongoing API costs (incremental over full-regeneration)
- Respect ecosystem API rate limits (1-10 req/s depending on ecosystem)
- Security: source changes for high-priority packages need manual review
- Curated overrides must never be auto-refreshed
- Bootstrap Phase B must be runnable locally

## Research Findings (Phase 2)
- `internal/seed/` has Source interface with HomebrewSource as sole implementation
- `internal/discover/` has full disambiguation: EcosystemProbe, disambiguate(), 10x threshold
- crates.io: category=command-line-utilities filter, 1 req/s
- npm: keywords:cli search, undocumented rate limits
- PyPI: no search API, use static top-15K dump + classifier filter
- RubyGems: 10 req/s load balancer, top-50 endpoint + search
- Go: no popularity data, excluded from automated seeding

## Options (Phase 3)
1. Discovery: per-ecosystem Source implementations (chosen) vs GitHub stars proxy vs libraries.io
2. Disambiguation: full probe for new, skip fresh (chosen) vs discovery ecosystem only vs re-disambiguate everything
3. Audit: per-package JSON files (chosen) vs JSONL append log vs no audit
4. Alerting: priority-based split (chosen) vs all-changes-need-review vs auto-accept-all

## Decision (Phase 5)

**Problem:**
The batch pipeline's unified queue has 5,275 entries but 97% come from Homebrew. Other ecosystems (cargo, npm, pypi, rubygems) have popular CLI tools that aren't queued. The existing seed-queue command only fetches from Homebrew analytics, and queue entries lack freshness tracking for disambiguation decisions, so stale sources can't be refreshed.

**Decision:**
Add four new Source implementations to internal/seed (CratesIO, Npm, PyPI, RubyGems), each fetching popular CLI tools from ecosystem APIs. Extend cmd/seed-queue to accept multiple sources, run disambiguation via internal/discover for new packages, and write audit logs. The weekly workflow runs all sources sequentially. Bootstrap Phase B runs locally to backfill disambiguation for existing homebrew entries.

**Rationale:**
The internal/seed.Source interface already abstracts ecosystem-specific fetching, and internal/discover handles disambiguation with the 10x threshold. Adding new Source implementations is the natural extension point. Per-ecosystem rate limiting in the seeding command avoids CI-side complexity, and audit logs use the existing DisambiguationRecord format. The 30-day freshness threshold keeps ongoing costs low while catching stale sources.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-16
