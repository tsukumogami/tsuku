# Plan Analysis: tsuku-ai-skills

## Source Document
Path: docs/prds/PRD-tsuku-ai-skills.md
Status: Accepted
Input Type: prd

## Scope Summary

Add two Claude Code plugins (tsuku-recipes and tsuku-user) to the tsuku monorepo with 4 skills total, a committed CLAUDE.md for repo orientation and plugin maintenance, reorganized public documentation under docs/guides/, and CI freshness checks for skill content.

## Components Identified

- **CLAUDE.md** (Workstream A): Committed repo-level CLAUDE.md with promoted content from CLAUDE.local.md, key internal packages table, and plugin maintenance protocol. CLAUDE.local.md reduced to workspace-specific sections.
- **Plugin Infrastructure** (Workstream B): marketplace.json, plugin.json for both plugins, committed settings.json. Follows koto pattern.
- **recipe-author Skill** (Workstream C): SKILL.md with action names table, version providers, platform conditionals, verification. Bundled agent-shaped reference files. Exemplar recipes. Distributed recipe coverage.
- **recipe-test Skill** (Workstream D): SKILL.md covering validate -> eval -> sandbox -> golden workflow. Exit codes, test infrastructure pointers.
- **tsuku-user Skill** (Workstream E): SKILL.md covering .tsuku.toml project config, CLI commands, shell integration, troubleshooting, auto-update.
- **External Distribution** (Workstream F): settings.json snippet in GUIDE-distributed-recipe-authoring.md. AGENTS.md for non-Claude-Code agents.
- **Documentation Organization** (Workstream G): Move GUIDE-*.md from docs/ root to docs/guides/. Update all cross-references.
- **Skill Freshness** (Workstream H): CI workflow validating exemplar recipe paths exist and pass tsuku validate. No hooks.json in either plugin.

## Research Catalogs

Detailed surface catalogs produced by 10 research agents are available at wip/research/plan_skills_*.md:

| Catalog | Content |
|---------|---------|
| actions-catalog.md | 52 actions across 8 categories with params and deps |
| version-providers-catalog.md | 14 providers with config fields and auto-detection |
| guides-catalog.md | 11 guides assessed for bundle vs pointer vs skip |
| recipe-format-catalog.md | Full TOML schema with validation rules |
| user-surface-catalog.md | 23 CLI commands, .tsuku.toml, shell integration, exit codes |
| distributed-catalog.md | .tsuku-recipes/ discovery, trust model, caching |
| testing-catalog.md | validate/eval/sandbox/golden workflow, CI patterns |
| plugin-infrastructure-catalog.md | marketplace/plugin JSON schemas from koto/shirabe |
| claude-md-catalog.md | Content promotion plan and new sections |
| exemplar-recipes-catalog.md | Best teaching recipe per pattern category |

## Success Metrics (from PRD)

- Recipe authors can install tsuku-recipes and get contextual guidance for writing and testing recipes
- End users can install tsuku-user and get guidance for .tsuku.toml, CLI, shell integration
- Committed CLAUDE.md gives all contributors repo orientation and plugin maintenance protocol
- Skill content stays fresh through CI exemplar validation and contributor protocol

## External Dependencies

- koto PR #126 pattern: CLAUDE.md + plugin maintenance protocol (already merged as reference)
- shirabe plugin infrastructure: marketplace.json schema, settings.json format (already proven)
- Existing docs/GUIDE-*.md files: must exist at current paths before being moved to docs/guides/
