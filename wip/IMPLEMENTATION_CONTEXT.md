---
summary:
  constraints:
    - Schema v2 must be backward compatible with v1 (ParseRegistry accepts both)
    - RegistryLookup only reads builder+source; metadata fields are optional and unused by resolver
    - Priority queue entries are all homebrew source; builder is always "homebrew"
    - GitHub API calls need GITHUB_TOKEN for rate limits; Homebrew API is public
    - This is a skeleton issue — stubs for enrichment, graduation, collision detection
  integration_points:
    - internal/discover/registry.go — add optional metadata fields to RegistryEntry, update ParseRegistry for v2
    - internal/discover/registry_test.go — tests for v2 parsing
    - cmd/seed-discovery/main.go — new CLI tool
    - data/priority-queue.json — input (204 homebrew entries)
    - data/discovery-seeds/*.json — input seed lists (new)
    - recipes/discovery.json — output (currently has 2 entries, will grow)
  risks:
    - GitHub API rate limits during validation (need token and caching)
    - Priority queue has entries with status "blocked"/"failed" — need to decide whether to include all or filter
    - discovery.json is committed to repo — large diffs on regeneration
  approach_notes: |
    Follow the existing cmd/seed-queue pattern for CLI structure.
    Builder names: "github" and "homebrew" (match internal/builders names).
    Priority queue format: {id, source, name, tier, status, added_at}.
    Current discovery.json has schema_version 1 with 2 entries (jq, fd).
    Seed list format: {category, entries: [{name, builder, source, ...}]}.
---

# Implementation Context: Issue #1364

Create cmd/seed-discovery tool that reads seed lists + priority queue, validates entries,
and outputs schema v2 discovery.json. Skeleton issue with stubs for enrichment (#1366),
graduation (#1367), and full seed data (#1368).
