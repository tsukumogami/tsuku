---
status: Draft
problem: |
  The tsuku repo has no committed CLAUDE.md and no AI skills for recipe authoring,
  despite recipe authoring being the most complex and frequent contributor workflow.
  External recipe authors who don't clone the full repo have zero AI-assisted guidance
  for the 60+ action system, 15 version providers, platform-conditional logic, or
  distributed recipe folders. Contributors who clone the repo get only a workspace-managed
  CLAUDE.local.md that covers build commands but nothing about internal architecture
  or plugin maintenance.
goals: |
  Recipe authors (both central registry contributors and distributed recipe maintainers)
  can install a focused AI skill that accurately guides them through authoring and testing.
  Tsuku contributors get a committed CLAUDE.md with repo orientation, key internal packages,
  and a skill maintenance protocol. Skill content stays fresh through CI checks and a
  contributor-facing assessment protocol.
---

# PRD: tsuku AI Skills

## Status

Draft

## Problem Statement

Tsuku has a substantial recipe authoring surface: 60+ actions across composites
and primitives, 15 version providers, platform-conditional when clauses (os, libc,
linux_family, gpu), library dependency propagation, and a distributed recipe system
via .tsuku-recipes/ folders. The existing documentation is thorough -- 11 GUIDE-*.md
files cover specific topics in depth -- but there's no AI skill that surfaces this
knowledge contextually when recipe authors need it.

External recipe authors face the steepest cliff. They maintain .tsuku-recipes/
folders in their own repos without access to the central registry's documentation
or contributor tooling. Their only entry point is GUIDE-distributed-recipe-authoring.md,
which covers directory structure but not the underlying recipe TOML format, action
parameters, or testing workflow.

The tsuku repo itself has no committed CLAUDE.md. The workspace-managed
CLAUDE.local.md covers build commands and CLI usage but nothing about the internal
package structure (internal/actions/, internal/version/, internal/executor/), recipe
patterns, or how source changes affect downstream AI skills. The koto repo (PR #126)
establishes the organizational pattern: committed CLAUDE.md for repo orientation with
a skill maintenance protocol, plugin skills for external-facing domain workflows.

The recipe testing workflow is particularly fragmented. Authors must discover and
chain 4+ commands (validate, eval, sandbox install, golden file comparison) across
separate docs, with platform-specific variations and container requirements that
aren't unified into a single guided experience.

## Goals

- Recipe authors can install the tsuku-recipes plugin and get contextual guidance
  for writing and testing TOML recipes, including distributed .tsuku-recipes/ folders.
- A committed CLAUDE.md gives every tsuku contributor (including external ones)
  repo orientation, key internal package pointers, and a skill maintenance protocol.
- Skill content stays accurate through CI exemplar validation and a contributor
  protocol that prompts skill assessment on source changes.

## User Stories

**As an external recipe author** maintaining a .tsuku-recipes/ folder in my project,
I want a skill that explains the TOML recipe format, action parameters, and platform
conditionals so I can write correct recipes without cloning the tsuku repo or reading
its Go source code.

**As a central registry contributor** writing a recipe for a new tool, I want
curated exemplar recipes by pattern category (binary download, source build,
homebrew-backed, ecosystem-delegated) so I can start from a working example instead
of guessing the right action and parameter combination.

**As a recipe author debugging a failed installation**, I want a testing workflow
skill that walks me through validate, eval, sandbox install, and verification so I
can diagnose issues without piecing together commands from 4 separate guide files.

**As a tsuku contributor** modifying action code or version providers, I want
CLAUDE.md to tell me which source areas affect recipe skills so I can assess whether
my changes require a skill update in the same PR.

**As an AI agent in a tsuku repo session**, I want a committed CLAUDE.md that
orients me on the repo structure, key packages, and conventions so I can assist
effectively without reading workspace-specific local configuration.

## Requirements

### Committed CLAUDE.md (Workstream A)

**R1** -- Commit a CLAUDE.md at the tsuku repo root. Content must include: repo
description, monorepo structure, build/test/lint commands, CLI command table,
development workflow (Docker, integration tests), release process, and conventions
(gofmt, golangci-lint, $TSUKU_HOME usage). This content is promoted from the
existing CLAUDE.local.md.

**R2** -- CLAUDE.md must include a "Key Internal Packages" section listing the
major internal packages with one-line descriptions. At minimum: actions, config,
distributed, executor, install, recipe, registry, version, updates, project,
shellenv, telemetry.

**R3** -- CLAUDE.md must include a "tsuku-recipes Plugin Maintenance" section
following the koto pattern. The section must instruct contributors to assess
recipe skills after changes to internal/actions/, internal/version/, and
internal/recipe/ by: (1) reading the diff and each skill, then checking whether
anything the skill documents no longer matches the code (broken contracts), and
(2) checking whether the change adds action names, parameter schemas, version
source types, or when clause filters that neither skill mentions (new surface).
If either check surfaces gaps, the contributor updates the skill in the same PR.

**R4** -- CLAUDE.local.md must be reduced to workspace-specific content only:
Repo Visibility, Default Scope, QA Configuration, and Environment sections.
Content promoted to CLAUDE.md must not be duplicated.

### tsuku-recipes Plugin Infrastructure (Workstream B)

**R5** -- Create .claude-plugin/marketplace.json declaring the tsuku marketplace
with the tsuku-recipes plugin.

**R6** -- Create plugins/tsuku-recipes/.claude-plugin/plugin.json listing
recipe-author and recipe-test as skills.

**R7** -- Create .claude/settings.json (committed) enabling tsuku-recipes@tsuku
and shirabe@shirabe. The committed settings must declare the local tsuku
marketplace via file source and shirabe via GitHub source with sparsePaths.
Personal configuration (tsukumogami, env vars, hooks, permissions) must remain
in settings.local.json.

### recipe-author Skill (Workstream C)

**R8** -- Create plugins/tsuku-recipes/skills/recipe-author/SKILL.md with a
hybrid quick-reference architecture. The skill must embed an action names table
with columns: action name, category, and one-line description. The table must
cover all action categories (composites and primitives).

**R9** -- recipe-author SKILL.md must list all version provider types with the
corresponding [version] source value.

**R10** -- recipe-author SKILL.md must include a platform conditional cheat sheet
covering when clause syntax with examples for os, libc, linux_family, and gpu
filters.

**R11** -- recipe-author SKILL.md must include verification quick-start covering
version mode, output mode, and common format transforms (semver, strip_v, calver).

**R12** -- recipe-author must include a references/exemplar-recipes.md file with
curated recipe paths covering these pattern categories (one exemplar per
category, minimum 7): binary download, homebrew-backed, source build with
dependencies, platform-conditional with when clauses, ecosystem-delegated
(cargo/npm/pip/go), library with outputs and rpath, and custom verification.
Exemplar recipes must be human-authored (not llm_validation = "skipped").

**R13** -- recipe-author SKILL.md must include deep-dive pointers to existing
guide files. At minimum: GUIDE-actions-and-primitives.md,
GUIDE-hybrid-libc-recipes.md, GUIDE-library-dependencies.md,
GUIDE-recipe-verification.md, GUIDE-troubleshooting-verification.md,
GUIDE-distributed-recipe-authoring.md. The skill must also include a pointer
to the public documentation root (docs/guides/) so agents running older skill
versions can discover new guides independently.

**R14** -- recipe-author SKILL.md must cover distributed recipe authoring:
.tsuku-recipes/ directory setup, file naming convention (kebab-case TOML),
install syntax (owner/repo, owner/repo:recipe, owner/repo@version), and a
pointer to GUIDE-distributed-recipe-authoring.md for the full trust model
and caching behavior.

### recipe-test Skill (Workstream D)

**R15** -- Create plugins/tsuku-recipes/skills/recipe-test/SKILL.md covering the
full testing workflow with exact commands: tsuku validate, tsuku eval,
tsuku install --sandbox, and golden file validation.

**R16** -- recipe-test SKILL.md must include test infrastructure pointers:
docker-dev.sh for container setup, make build-test for local builds, tsuku doctor
for environment checks, TSUKU_HOME isolation for safe local testing.

**R17** -- recipe-test SKILL.md must document at least exit codes 6 (container
failure) and 8 (verification failure) with their failure scenarios.

**R18** -- recipe-test SKILL.md must include a pointer to CONTRIBUTING.md for full
testing documentation, including cross-family testing instructions.

### External Distribution (Workstream E)

**R19** -- Update GUIDE-distributed-recipe-authoring.md with a "Claude Code
Integration" section containing a settings.json snippet for external consumers.
The snippet must use sparsePaths to limit downloads to .claude-plugin/ and
plugins/tsuku-recipes/ only. autoUpdate must be omitted (defaults to false) so
external consumers explicitly control update timing.

**R20** -- Create plugins/tsuku-recipes/AGENTS.md providing recipe authoring
guidance for non-Claude-Code agents. Content must cover: what tsuku recipes are,
recipe TOML format overview, pointer to the action names table in recipe-author
SKILL.md, the testing workflow (validate -> eval -> sandbox), and links to
GUIDE-*.md files. Length must not exceed 120 lines.

### Documentation Organization (Workstream F)

**R23** -- Move public-facing guide files (GUIDE-*.md) from docs/ root into
docs/guides/. Public guides are documentation intended for recipe authors and
tool users. Internal artifacts (designs, PRDs, roadmaps, plans) stay at their
current paths. This separation prevents agents browsing the guide directory
from loading internal planning artifacts into context.

**R24** -- Update all internal cross-references to moved guide files. This
includes SKILL.md pointers, CONTRIBUTING.md links, and any design docs that
reference GUIDE-*.md paths.

### Skill Freshness (Workstream G)

**R25** -- Create a CI check that validates all recipe file paths referenced in
exemplar-recipes.md still exist and pass tsuku validate. The check must run when
skill files or referenced recipes change.

**R26** -- The tsuku-recipes plugin must not contain a hooks.json file. External
recipe authors install the plugin with potentially-enabled autoUpdate; executing
arbitrary commands on their machines is not acceptable.

## Acceptance Criteria

### CLAUDE.md

- [ ] CLAUDE.md exists at the tsuku repo root and is committed to git
- [ ] CLAUDE.md contains: repo description, monorepo structure diagram, build/test/lint commands, CLI command table, development workflow, release process, and conventions sections
- [ ] CLAUDE.md contains a "Key Internal Packages" section listing at least 12 packages
- [ ] CLAUDE.md contains a "tsuku-recipes Plugin Maintenance" section that names internal/actions/, internal/version/, and internal/recipe/ as trigger areas
- [ ] The maintenance section distinguishes between "broken contracts" and "new surface" and instructs contributors to update skills in the same PR
- [ ] CLAUDE.local.md contains only: Repo Visibility, Default Scope, QA Configuration, and Environment sections
- [ ] No section headings or multi-line blocks from CLAUDE.local.md appear verbatim in CLAUDE.md

### Plugin Infrastructure

- [ ] .claude-plugin/marketplace.json exists and declares the tsuku marketplace
- [ ] plugins/tsuku-recipes/.claude-plugin/plugin.json exists and lists recipe-author and recipe-test
- [ ] .claude/settings.json exists and is committed
- [ ] settings.json enables tsuku-recipes@tsuku and shirabe@shirabe
- [ ] settings.json does not contain env, hooks, permissions, or mcpServers keys
- [ ] settings.json declares tsuku marketplace via file source and shirabe via GitHub source with sparsePaths
- [ ] plugins/tsuku-recipes/hooks.json does not exist

### recipe-author Skill

- [ ] plugins/tsuku-recipes/skills/recipe-author/SKILL.md exists
- [ ] SKILL.md contains an action names table covering composites and primitives
- [ ] SKILL.md lists all version provider types with source values
- [ ] SKILL.md includes when clause syntax with os, libc, linux_family, and gpu examples
- [ ] SKILL.md includes verification quick-start (version mode, output mode, format transforms)
- [ ] references/exemplar-recipes.md exists with at least 7 recipe paths (one per required category)
- [ ] All listed exemplar recipe files exist in the recipes/ directory
- [ ] No listed exemplar has llm_validation = "skipped" in its metadata
- [ ] SKILL.md includes pointers to at least 6 GUIDE-*.md files
- [ ] SKILL.md covers .tsuku-recipes/ directory setup and documents all three install syntax forms: owner/repo, owner/repo:recipe, owner/repo@version

### recipe-test Skill

- [ ] plugins/tsuku-recipes/skills/recipe-test/SKILL.md exists
- [ ] SKILL.md contains exact commands for validate, eval, sandbox install workflow
- [ ] SKILL.md references docker-dev.sh, make build-test, tsuku doctor, TSUKU_HOME
- [ ] SKILL.md documents exit codes 6 and 8
- [ ] SKILL.md contains a pointer to CONTRIBUTING.md

### External Distribution

- [ ] GUIDE-distributed-recipe-authoring.md contains a "Claude Code Integration" section
- [ ] The section contains a settings.json snippet with sparsePaths
- [ ] The snippet does not include autoUpdate: true
- [ ] plugins/tsuku-recipes/AGENTS.md exists and is 120 lines or fewer
- [ ] AGENTS.md covers recipe format overview, action reference pointer, testing workflow, and GUIDE-*.md links

### Documentation Organization

- [ ] All GUIDE-*.md files have been moved from docs/ to docs/guides/
- [ ] No GUIDE-*.md files remain at docs/ root
- [ ] All cross-references to moved guide files are updated (SKILL.md, CONTRIBUTING.md, design docs)
- [ ] recipe-author SKILL.md includes a pointer to docs/guides/ as the public documentation root

### Skill Freshness

- [ ] A CI workflow validates that exemplar recipe paths exist and pass tsuku validate
- [ ] The workflow triggers on changes to plugins/tsuku-recipes/ or referenced recipe files

## Out of Scope

- tsuku-dev plugin or contributor skill (contributor content goes in CLAUDE.md)
- End-user skills (tsuku install/update workflows for tool consumers)
- Changes to the action system, recipe format, or version providers
- LLM eval harness for skill correctness (start with file-existence CI checks)
- Migrating .claude/shirabe-extensions/ to the new plugin structure
- Golden file snapshot validation for exemplar recipes (future enhancement)
- Non-GitHub distributed recipe hosting (GitLab, Gitea)

## Known Limitations

- The CLAUDE.md maintenance protocol relies on contributors following it. No
  automated enforcement blocks PRs that skip skill assessment.
- The quick-reference tables in SKILL.md need manual updates when actions or
  version providers change. Frequency is low (actions change ~monthly) but
  there's no automated detection of drift in the table content itself.
- External recipe authors must manually add the settings.json snippet. There's
  no automated discovery or installation prompt.

## Decisions and Trade-offs

**CLAUDE.md over tsuku-dev plugin**: Contributor-facing content (internal
packages, architecture pointers, CI patterns) belongs in CLAUDE.md, not a
separate plugin skill. A plugin adds ~100 tokens per conversation for content
that's standard repo orientation. The koto pattern confirms this: CLAUDE.md for
contributor context, plugin skills only for external domain workflows. This
simplifies the plan from 2 plugins to 1.

**Hybrid quick-reference over full embedding**: The 11 GUIDE-*.md files total
1,700+ lines. Embedding them in SKILL.md creates drift risk and context bloat.
The hybrid approach (action names table + guide pointers) handles 80% of lookups
(action name, syntax check) without file reads, while deep dives use the
authoritative source docs.

**File-existence CI over eval harness**: A CI check that verifies exemplar recipe
paths exist and pass `tsuku validate` catches the most common drift (deleted or
renamed recipes) with ~50 lines of bash. An LLM eval harness (koto's approach)
is more thorough but disproportionate for recipe skills where the content is
primarily reference tables and pointers, not behavioral guidance.

**Single plugin for both recipe skills**: recipe-author and recipe-test serve the
same persona (recipe authors) in different modes (writing vs testing). Splitting
into two plugins would force external authors to install both separately. A single
plugin with two skills keeps installation simple.

**No hooks.json in tsuku-recipes**: External recipe authors install the plugin
from a GitHub source, potentially with autoUpdate enabled. Hooks execute arbitrary
commands on consumers' machines. The blast radius of a compromised hook is
unacceptable for a lightweight documentation plugin.
