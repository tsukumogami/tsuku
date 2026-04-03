---
design_doc: docs/prds/PRD-tsuku-ai-skills.md
input_type: prd
decomposition_strategy: horizontal
strategy_rationale: "Components are primarily documentation and configuration with clear boundaries and no runtime interaction. Infrastructure is prerequisite for skills, skills are prerequisite for distribution docs."
confirmed_by_user: false
issue_count: 10
execution_mode: single-pr
---

# Plan Decomposition: tsuku-ai-skills

## Strategy: Horizontal

Layer-by-layer: infrastructure first, then skills in parallel, then docs/CI last.
Components have clear boundaries with no runtime coupling. The docs reorg (GUIDE-*.md
move) should happen early since skill references will point to new paths.

## Issue Outlines

### Issue 1: docs: reorganize public guides into docs/guides/

- **Type**: standard
- **Complexity**: simple
- **Goal**: Move all GUIDE-*.md files from docs/ root to docs/guides/ and update all cross-references
- **Section**: PRD Workstream G (R23-R24)
- **Milestone**: tsuku AI Skills
- **Dependencies**: None
- **Research**: wip/research/plan_skills_guides-catalog.md (lists all 11 guides and their current paths)
- **Notes**: Must happen before skill content is written, since skills will reference docs/guides/ paths. Update CONTRIBUTING.md, design docs, and any other internal references.

### Issue 2: docs: commit CLAUDE.md and reduce CLAUDE.local.md

- **Type**: standard
- **Complexity**: simple
- **Goal**: Promote universally-useful content from CLAUDE.local.md to committed CLAUDE.md, add key internal packages table and plugin maintenance protocol
- **Section**: PRD Workstream A (R1-R4)
- **Milestone**: tsuku AI Skills
- **Dependencies**: None
- **Research**: wip/research/plan_skills_claude-md-catalog.md (content split plan, 24+ internal packages, draft maintenance protocol)

### Issue 3: feat(plugins): add plugin infrastructure and committed settings.json

- **Type**: standard
- **Complexity**: simple
- **Goal**: Create marketplace.json, plugin.json for both plugins, committed settings.json, and empty SKILL.md stubs
- **Section**: PRD Workstream B (R5-R7, R6a)
- **Milestone**: tsuku AI Skills
- **Dependencies**: None
- **Research**: wip/research/plan_skills_plugin-infrastructure-catalog.md (JSON schemas from koto/shirabe, directory structure, settings split)

### Issue 4: feat(plugins): add recipe-author skill with bundled references

- **Type**: standard
- **Complexity**: testable
- **Goal**: Write recipe-author SKILL.md with action table, version providers, platform conditionals, verification. Create bundled reference files and exemplar recipes list.
- **Section**: PRD Workstream C (R8-R14)
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:1>>, <<ISSUE:3>>
- **Research**:
  - wip/research/plan_skills_actions-catalog.md (52 actions with params, categories, deps)
  - wip/research/plan_skills_version-providers-catalog.md (14 providers with config fields)
  - wip/research/plan_skills_recipe-format-catalog.md (full TOML schema, when clause syntax, validation rules)
  - wip/research/plan_skills_exemplar-recipes-catalog.md (curated recipes per pattern category)
  - wip/research/plan_skills_distributed-catalog.md (.tsuku-recipes/ setup, install syntax, trust model)
  - wip/research/plan_skills_guides-catalog.md (bundle vs pointer recommendations per guide)

### Issue 5: feat(plugins): add recipe-test skill

- **Type**: standard
- **Complexity**: testable
- **Goal**: Write recipe-test SKILL.md covering validate -> eval -> sandbox -> golden workflow with test infrastructure pointers and common failure patterns
- **Section**: PRD Workstream D (R15-R18)
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:1>>, <<ISSUE:3>>
- **Research**:
  - wip/research/plan_skills_testing-catalog.md (full testing workflow, commands, exit codes, CI patterns, container testing, golden files)

### Issue 6: feat(plugins): add tsuku-user skill

- **Type**: standard
- **Complexity**: testable
- **Goal**: Write tsuku-user SKILL.md covering .tsuku.toml project config, CLI commands, shell integration, troubleshooting, and auto-update
- **Section**: PRD Workstream E (R15a-R15f)
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:3>>
- **Research**:
  - wip/research/plan_skills_user-surface-catalog.md (23 CLI commands, .tsuku.toml schema, shell integration, exit codes, config.toml, update workflow)

### Issue 7: docs: add external distribution documentation

- **Type**: standard
- **Complexity**: simple
- **Goal**: Add Claude Code Integration section to GUIDE-distributed-recipe-authoring.md with settings.json snippet. Create AGENTS.md for tsuku-recipes plugin.
- **Section**: PRD Workstream F (R19-R20)
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:1>>, <<ISSUE:4>>, <<ISSUE:5>>
- **Research**: wip/research/plan_skills_plugin-infrastructure-catalog.md (external consumer snippet), wip/research/plan_skills_distributed-catalog.md

### Issue 8: ci: add skill exemplar validation workflow

- **Type**: standard
- **Complexity**: simple
- **Goal**: Create CI workflow that validates exemplar recipe paths exist and pass tsuku validate. Verify no hooks.json in either plugin.
- **Section**: PRD Workstream H (R25-R26)
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:4>>
- **Research**: wip/research/plan_skills_testing-catalog.md (CI patterns, existing workflow structure)

### Issue 9: docs: update CLAUDE.md plugin maintenance section with final skill paths

- **Type**: standard
- **Complexity**: simple
- **Goal**: After all skills are written, update the CLAUDE.md maintenance protocol with final skill paths and source-area mappings verified against actual content
- **Section**: PRD R3 (finalization)
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:2>>, <<ISSUE:4>>, <<ISSUE:5>>, <<ISSUE:6>>
- **Notes**: The initial CLAUDE.md (Issue 2) has a draft maintenance section. This issue verifies and finalizes it after skills exist.

### Issue 10: chore: clean up research catalogs and update design doc

- **Type**: standard
- **Complexity**: simple
- **Goal**: Remove wip/research/plan_skills_*.md catalogs, update existing design doc on the branch to reflect PRD-driven scope changes (drop tsuku-dev, add tsuku-user, bundled references)
- **Section**: Cleanup
- **Milestone**: tsuku AI Skills
- **Dependencies**: <<ISSUE:7>>, <<ISSUE:8>>, <<ISSUE:9>>
