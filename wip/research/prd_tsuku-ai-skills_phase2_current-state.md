# Phase 2 Research: Current-State Analyst

## Lead 1: CLAUDE.md Content Audit

### Findings

**tsuku CLAUDE.local.md (root) - 162 lines:**
- Monorepo structure overview (universally useful)
- Component table with tech stacks (universally useful)
- Quick Reference for CLI build/test (universally useful)
- Commands table (universally useful)
- Development section - Docker + integration tests (universally useful)
- Release Process - automated GoReleaser (universally useful)
- Environment setup - .local.env sourcing (universally useful)
- Key Points - gofmt, golangci-lint, CI (universally useful)
- QA Configuration (workspace-specific)
- Conventions - $TSUKU_HOME usage (universally useful)

**Component-specific CLAUDE.local.md files:**
- `recipes/CLAUDE.local.md` (81 lines): Recipe format, validation, actions reference
- `telemetry/CLAUDE.local.md` (62 lines): API endpoints, event schema, deployment
- `website/CLAUDE.local.md` (59 lines): Dev commands, file purposes, conventions

**koto CLAUDE.md pattern (for comparison):**
- Repo description, directory structure, quick reference, key commands, key points
- Architecture-specific section: Plugin Maintenance Protocol
- Responsibility matrix: maps source areas to affected skills

### Key Gaps vs koto Pattern

1. No architecture overview of internal packages
2. No key internal packages listing
3. No plugin/skill maintenance protocol
4. No responsibility matrix (source area -> skill mapping)

### Workspace-Specific Sections (stay in CLAUDE.local.md)

- Repo Visibility, Default Scope (tsukumogami workflow config)
- QA Configuration (workspace-specific testing overrides)
- Environment variables requiring .local.env

### Implications for Requirements

- ~90% of CLAUDE.local.md content is universally useful and should move to committed CLAUDE.md
- Need to add: key internal packages table, skill maintenance protocol with source-area mapping
- Component CLAUDE.local.md files (recipes/, telemetry/, website/) are separate concerns and should stay as-is

### Open Questions

1. Should recipes/CLAUDE.local.md content be promoted to committed too, or left as workspace-managed?
2. How detailed should the internal packages section be? (koto lists ~6 directories; tsuku has 20+ internal packages)

## Summary

tsuku's CLAUDE.local.md is 70% ready for commitment. Covers build/test/CLI well but lacks architecture pointers and skill maintenance protocol (the koto pattern). Key additions: internal packages table, skill maintenance section with source-area-to-skill mapping.
