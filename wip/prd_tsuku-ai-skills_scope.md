# /prd Scope: tsuku-ai-skills

## Problem Statement

The tsuku repo has no committed CLAUDE.md and no AI skills for recipe authoring -- its most complex and frequent contributor workflow. External recipe authors who don't clone the full repo get zero AI-assisted guidance for the 50+ action system, 13+ version providers, platform-conditional logic, or distributed recipe folders. Contributors who do clone the repo get only a workspace-managed CLAUDE.local.md covering build commands but nothing about internal architecture, recipe patterns, or plugin maintenance. The koto repo (PR #126) establishes the organizational pattern that tsuku should follow.

## Initial Scope

### In Scope

- Committed CLAUDE.md: promote universally-useful content from CLAUDE.local.md, add contributor internals (key packages, action/provider interfaces), add recipe-skill maintenance protocol (following koto's CLAUDE.md pattern)
- tsuku-recipes plugin (single plugin, 2 skills): recipe-author (TOML structure, actions, providers, platform conditionals, exemplar recipes, distributed recipe authoring via .tsuku-recipes/) and recipe-test (validate -> eval -> sandbox -> golden workflow)
- Plugin infrastructure: marketplace.json, plugin.json, committed settings.json
- External distribution: settings.json snippet for external recipe authors, AGENTS.md for non-Claude-Code agents
- CLAUDE.local.md cleanup: keep only workspace-specific content (Repo Visibility, Default Scope, QA Configuration, Environment)

### Out of Scope

- tsuku-dev plugin (contributor content goes in CLAUDE.md instead)
- End-user skills (tsuku install/update workflows)
- Changes to the action system or recipe format
- LLM eval harness for skill freshness (start with simpler CI checks)
- Migrating .claude/shirabe-extensions/ (separate effort)

## Research Leads

1. CLAUDE.md content audit: What from CLAUDE.local.md is universally useful vs workspace-specific? What contributor-facing content should be added? How does koto's CLAUDE.md structure compare?
2. Recipe skill content needs: What are the gaps between existing recipe documentation (recipes/CLAUDE.local.md, docs/ guides) and what recipe authors need? Which actions/providers are most commonly needed? What does the .tsuku-recipes distributed recipe folder mechanism look like and what docs exist for it?
3. Plugin infrastructure requirements: What's the exact marketplace.json and settings.json format? How does sparsePaths work for external consumers? What needs to move from settings.local.json to committed settings.json?
4. Skill maintenance protocol: What source areas in tsuku map to recipe skill content? What CI check is appropriate for exemplar freshness?

## Coverage Notes

- Distributed recipe authoring (.tsuku-recipes/) added after initial scoping -- needs investigation in lead #2
- PLAN-distributed-recipes.md exists on this branch and may contain relevant context
- The existing design doc (DESIGN-tsuku-ai-skills.md) has useful architectural decisions but was scoped for a 2-plugin model that we're now simplifying to 1 plugin + CLAUDE.md
