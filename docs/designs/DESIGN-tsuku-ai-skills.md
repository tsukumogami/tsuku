---
status: Superseded
superseded_by: docs/prds/PRD-tsuku-ai-skills.md
note: |
  The PRD revised the scope to: single tsuku-recipes plugin with bundled
  references, separate tsuku-user plugin, committed CLAUDE.md instead of
  tsuku-dev plugin.
problem: |
  The tsuku package manager has no AI skills for Claude Code, despite having
  three distinct personas (recipe authors, CLI contributors, maintainers)
  with complex workflows. Recipe authoring alone involves 50+ actions, 13+
  version providers, and platform-conditional logic. The exploration identified
  a two-plugin architecture following koto's directory pattern, but the
  technical approach for skill content structure, reference loading strategy,
  and external distribution needs to be designed.
decision: |
  Two plugins (tsuku-recipes and tsuku-dev) in a flat plugins/ directory following
  koto's pattern. tsuku-recipes has 2 skills (recipe-author and recipe-test),
  tsuku-dev has 1 skill (tsuku-contributor). Skills use a hybrid content
  architecture: ~150-line quick-reference for common lookups embedded in SKILL.md,
  with pointers to full guide files and 5-8 exemplar recipes for deep dives.
rationale: |
  The two-plugin split lets external recipe authors install only tsuku-recipes via
  sparsePaths without downloading dev skills, Go source, or the 1,400-recipe
  registry. The hybrid content approach handles the 80% case (action name lookups,
  syntax checks) without file reads while preserving authoritative source docs for
  complex topics. The asymmetric skill count (2+1) reflects that recipe authoring
  is the highest-frequency task with a natural activity-based split between writing
  and testing.
---

# DESIGN: tsuku AI Skills

## Status

Superseded by [`docs/prds/PRD-tsuku-ai-skills.md`](../prds/PRD-tsuku-ai-skills.md). The PRD revised the scope to a single `tsuku-recipes` plugin with bundled references, a separate `tsuku-user` plugin, and committed CLAUDE.md instead of a `tsuku-dev` plugin.

## Context and Problem Statement

Tsuku is a package manager for developer tools with a substantial codebase (55K+ lines in the action system alone, 1,400 recipes, 20+ version providers). Three distinct personas interact with it: recipe authors who write TOML recipe files, CLI contributors who work on the Go codebase, and maintainers who manage the batch pipeline and CI.

Currently, tsuku has zero committed Claude Code plugin infrastructure. All AI assistance comes from workspace-level plugins (shirabe, tsukumogami) installed via niwa, which are generic workflow skills not tailored to tsuku's domain. The existing CLAUDE.md covers build/test commands and conventions but doesn't address recipe authoring complexity, action parameter schemas, version provider configuration, or the fragmented recipe testing workflow.

An exploration identified that recipe authoring is the highest-value skill target -- it's the most frequent contributor task, has the largest gap between documentation and actual complexity, and serves both internal and external authors. The recipe testing workflow is particularly fragmented across 6+ commands with no unified experience.

The key open questions are architectural: how should skills reference tsuku's extensive documentation (300+ line action guides), how should the 1,400 recipe corpus be used for example-driven guidance, what is the exact skill inventory per plugin, and how should skills be structured internally.

## Decision Drivers

- Context bloat: each skill adds ~100 tokens to every conversation's system prompt. Plugin-level enablement is the granularity for context control.
- External recipe authors don't clone tsuku -- they need a lightweight distribution path (settings.json snippet + GitHub source with sparsePaths).
- Recipe complexity spectrum: 18-line binary downloads to 40+ line source builds with platform conditionals and dependency templating.
- `tsuku create` already handles easy recipe generation. Skills should focus on the cases that fall outside automated generation: debugging, platform conditionals, source builds, recipe tweaking.
- The recipe testing workflow (validate -> eval -> sandbox -> golden) is documented but not unified into a single guided experience.

## Decisions Already Made

From the exploration convergence:

- **Two-plugin split**: `tsuku-recipes` (recipe authoring, lightweight) and `tsuku-dev` (contributor/maintainer). End-user skills are out of scope.
- **Koto's directory pattern**: marketplace.json at `.claude-plugin/`, plugins in `plugins/<name>/` subdirectories within the tsuku monorepo.
- **Recipe authoring is P0**: highest frequency, largest documentation gap.
- **Committed settings.json**: only reference publicly-available plugins (local tsuku plugins + shirabe via GitHub source). Workspace-managed plugins (tsukumogami) stay in settings.local.json.
- **Targeted lookups over full reference embedding**: skills should pull from existing guides on demand rather than embedding the full 300+ line action parameter reference.
- **External distribution via documentation**: add a settings.json snippet to the distributed recipe authoring guide.

## Considered Options

### Decision 1: Skill Content Architecture

How should recipe-authoring skills be structured internally and reference tsuku's documentation?

Key assumptions:
- Action names and basic syntax change infrequently enough for manual sync
- 5-8 exemplar recipes cover the major pattern categories across the 1,400 recipe corpus
- The model can reliably classify tasks as quick-reference-sufficient vs. needing a full guide read
- Guide files remain at current paths in `docs/`

#### Chosen: Hybrid Quick-Reference with Guide Pointers

SKILL.md embeds a compact quick-reference (~120-150 lines) covering the most commonly needed information: action names with one-line descriptions, version provider types, platform conditional syntax, and verification quick-start. For deeper topics (source builds, libc handling, library dependencies, troubleshooting), it points to specific guide files with clear "read this when..." instructions. It names 5-8 exemplar recipes by category so the model knows which to read for different patterns.

This handles the 80% case (action lookup, syntax check, simple patterns) without file reads, while deep dives use authoritative source docs. At ~150 lines, the skill stays well under comparable skill sizes and avoids the drift problem of full embedding. The exemplar recipe list converts the 1,400-recipe corpus from an unsearchable haystack into a curated pattern set.

#### Alternatives Considered

- **Full Embedding**: Condense all 1,757 lines of guides into ~400-500 lines in SKILL.md. Rejected: creates drift risk, pays full context cost even for simple tasks, contradicts the exploration's targeted-lookup finding.
- **Pure Router**: Slim ~60-line SKILL.md that reads guide files on demand. Rejected: too thin for common lookups, requires file reads for basic syntax, lacks exemplar recipe curation for the large corpus.
- **Layered Sub-Files**: SKILL.md + co-located reference files in `references/` subdirectory. Rejected: adds file management complexity without proportional benefit -- typical tasks need 2-3 sub-files, matching the hybrid approach's cost with more indirection.

### Decision 2: Skill Inventory

What specific skills go in each plugin and what are their boundaries?

Key assumptions:
- Recipe authoring will be invoked significantly more frequently than dev skills
- The existing `recipes/CLAUDE.local.md` actions table is too sparse for real authoring
- Contributors already read Go interfaces directly; dev skills supplement code reading
- Maintainer-specific operations are infrequent enough for CLAUDE.md rather than a dedicated skill

#### Chosen: Asymmetric (2 + 1)

**tsuku-recipes (2 skills):**
- `recipe-author` -- TOML structure, action parameters, version providers, platform conditionals, os/arch mappings. Invoked when writing or editing recipe TOML files.
- `recipe-test` -- Testing workflow (validate -> eval -> sandbox -> golden), common failure patterns, debugging recipe issues, CI validation behavior. Invoked when testing or debugging a recipe.

**tsuku-dev (1 skill):**
- `tsuku-contributor` -- Action development (interfaces, registration, testing), version provider development (strategy pattern, priorities), debugging, CI patterns. Maintainer ops folded in since contributors and maintainers overlap heavily.

The split follows user activity boundaries, not topic boundaries. Recipe authors switch between "what parameters does this action take?" and "why did my recipe fail validation?" -- these are different modes. Contributors work on actions and providers in the same coding session, so a single reference skill is sufficient.

#### Alternatives Considered

- **Minimal (1+1)**: One monolithic skill per plugin. Rejected: recipe authors always load testing content even when just writing TOML; the ~800+ token monolithic skill mixes concerns.
- **Focused Split (2+2)**: Separate action-dev and provider-dev skills. Rejected: small audience with overlapping registration/interface/testing patterns doesn't justify the extra ~100 tokens.
- **Granular (3+3)**: Six total skills including separate recipe-actions, maintainer-ops. Rejected: over-segmented. recipe-schema vs recipe-actions is artificial. 6 skills at ~600 tokens burns context for minimal gain.

### Decision 3: Plugin Infrastructure and Distribution

What is the exact plugin infrastructure layout, settings.json configuration, and external distribution experience?

Key assumptions:
- Claude Code resolves marketplace.json source paths relative to the marketplace file's parent directory
- sparsePaths in extraKnownMarketplaces filters sparse checkout to only listed paths
- External consumers can enable a single plugin from a multi-plugin marketplace
- Existing `.claude/shirabe-extensions/` will be migrated separately

#### Chosen: Flat plugins directory (koto pattern)

All plugin directories live under `plugins/` at the repo root, with `marketplace.json` at `.claude-plugin/`. Each plugin is self-contained with its own `plugin.json`, `AGENTS.md`, and optional `hooks.json`.

This is the only layout that preserves two-plugin separation, allows external consumers to pull only tsuku-recipes via targeted sparsePaths (`[".claude-plugin", "plugins/tsuku-recipes"]`), and follows the proven koto pattern. The committed `settings.json` enables both local tsuku plugins plus shirabe as a remote marketplace.

#### Alternatives Considered

- **Subdirectory plugins with cross-referencing**: Plugin metadata in `.claude-plugin/plugins/` with skills in a shared `skills/` directory. Rejected: fragile relative paths reaching outside plugin directories, no precedent, cannot selectively pull one plugin's skills.
- **Single root plugin (shirabe pattern)**: All skills in one plugin. Rejected: contradicts the two-plugin decision, forces external recipe authors to load dev-workflow skills and hooks they don't need.

## Decision Outcome

The three decisions compose cleanly:

1. **Infrastructure** (D3) provides the file layout: `plugins/tsuku-recipes/` and `plugins/tsuku-dev/` under a single marketplace.
2. **Inventory** (D2) fills those plugins: 2 recipe skills in tsuku-recipes, 1 contributor skill in tsuku-dev.
3. **Content architecture** (D1) defines how each skill works internally: hybrid quick-reference with guide pointers and curated exemplar recipes.

External recipe authors get a focused experience: they install only `tsuku-recipes@tsuku` via a settings.json snippet, downloading just the `.claude-plugin/` and `plugins/tsuku-recipes/` directories (~150-200 lines of skill content). Tsuku contributors get the full set -- both plugins auto-loaded from the committed settings.json.

## Solution Architecture

### Overview

Two Claude Code plugins hosted in the tsuku monorepo, distributed via a single marketplace. Each plugin contains self-contained skills that use a hybrid reference architecture to balance context cost against usability.

### Components

```
tsuku/
  .claude-plugin/
    marketplace.json                    # Declares tsuku marketplace with both plugins
  .claude/
    settings.json                       # Committed: enables tsuku plugins + shirabe
    settings.local.json                 # Personal: tsukumogami, hooks, permissions
  plugins/
    tsuku-recipes/
      .claude-plugin/
        plugin.json                     # Lists recipe-author and recipe-test skills
      skills/
        recipe-author/
          SKILL.md                      # ~150 lines: hybrid quick-ref + guide pointers
          references/
            exemplar-recipes.md         # Curated 5-8 recipe paths by pattern category
        recipe-test/
          SKILL.md                      # Testing workflow: validate -> eval -> sandbox -> golden
      AGENTS.md                         # Cross-platform agent guidance (Codex, Windsurf)
    tsuku-dev/
      .claude-plugin/
        plugin.json                     # Lists tsuku-contributor skill
      skills/
        tsuku-contributor/
          SKILL.md                      # Action dev, provider dev, CI patterns
      AGENTS.md
```

### Key Interfaces

**marketplace.json:**
```json
{
  "name": "tsuku",
  "description": "AI skills for the tsuku package manager",
  "plugins": [
    { "name": "tsuku-recipes", "source": "./plugins/tsuku-recipes" },
    { "name": "tsuku-dev", "source": "./plugins/tsuku-dev" }
  ]
}
```

**Committed settings.json:**
```json
{
  "enabledPlugins": {
    "tsuku-recipes@tsuku": true,
    "tsuku-dev@tsuku": true,
    "shirabe@shirabe": true
  },
  "extraKnownMarketplaces": {
    "tsuku": {
      "source": { "source": "file", "path": ".claude-plugin/marketplace.json" }
    },
    "shirabe": {
      "source": {
        "source": "github",
        "repo": "tsukumogami/shirabe",
        "sparsePaths": [".claude-plugin", "skills"]
      },
      "autoUpdate": true
    }
  }
}
```

**settings.json key ownership:**

| Key | settings.json (committed) | settings.local.json (gitignored) |
|-----|---------------------------|----------------------------------|
| enabledPlugins (tsuku, shirabe) | Yes | No |
| enabledPlugins (tsukumogami) | No | Yes |
| extraKnownMarketplaces (tsuku, shirabe) | Yes | No |
| extraKnownMarketplaces (tsukumogami, directory-based) | No | Yes |
| env (GH_TOKEN, etc.) | No | Yes |
| bypassPermissions | No | Yes |
| hooks | No | Yes |

Only publicly-available plugins belong in committed settings.json. Personal configuration, workspace-managed plugins, and credentials stay in settings.local.json.

**External consumer settings.json snippet:**
```json
{
  "enabledPlugins": {
    "tsuku-recipes@tsuku": true
  },
  "extraKnownMarketplaces": {
    "tsuku": {
      "source": {
        "source": "github",
        "repo": "tsukumogami/tsuku",
        "sparsePaths": [".claude-plugin", "plugins/tsuku-recipes"]
      }
    }
  }
}
```

Note: `autoUpdate` is omitted (defaults to false) so external consumers explicitly control when plugin content updates. They can add `"autoUpdate": true` if they want automatic updates.

### Skill Content Structure

Each skill's SKILL.md follows the hybrid pattern:

**recipe-author** (~150 lines):
- Frontmatter (name, description, triggers)
- Quick-reference: action names table (name + one-line purpose, ~30 lines)
- Quick-reference: version provider types (~10 lines)
- Quick-reference: platform conditional syntax with examples (~15 lines)
- Quick-reference: verification patterns (~10 lines)
- Deep-dive pointers: "Read docs/guides/GUIDE-actions-and-primitives.md for full action parameters", "Read docs/guides/GUIDE-hybrid-libc-recipes.md for libc-conditional patterns", etc.
- Exemplar recipes: curated by pattern category (~8 entries, see selection criteria below)
- Workflow note: "If the user says `tsuku create` failed or they need to fix a generated recipe, start by reading the generated recipe and comparing against the exemplars."

**Exemplar recipe selection criteria.** Each exemplar must cover a distinct pattern category. Target coverage:
- Binary download + extract (the most common pattern)
- Homebrew-backed installation
- Source build with configure/make and dependency declarations
- Platform-conditional recipe with `[steps.when]` clauses
- Ecosystem-delegated install (cargo/npm/pip/go)
- Library recipe with runtime dependencies and rpath
- Recipe with custom verification (output mode, exit code checks)
- Recipe using version provider inference (no explicit `[version]` section)

Select recipes that are human-authored (not LLM-generated with `llm_validation = "skipped"`) and representative of their category. The exemplar list lives in `references/exemplar-recipes.md` and should be verified by CI (check that listed files exist).

**recipe-test** (~80-100 lines):
- Frontmatter
- Testing workflow steps: validate -> eval -> sandbox -> golden (with exact commands)
- Test infrastructure pointers: `docker-dev.sh` for container setup, `make build-test` for local builds, `tsuku doctor` for environment checks, `TSUKU_HOME` isolation for safe local testing
- Cross-family testing script (condensed from CONTRIBUTING.md)
- Common failure patterns and exit codes (exit code 6 = container failure, 8 = verification failure)
- Known issues (e.g., --recipe post-install bug #2218)
- Pointer to CONTRIBUTING.md for full testing documentation

**tsuku-contributor** (~120-150 lines):
- Frontmatter
- Action development: interface requirements (Name, Execute, IsDeterministic, Dependencies), registration in init(), Decomposable for composites, testing patterns
- Version provider development: VersionResolver/VersionLister interfaces, strategy pattern, priority levels, NewProviderFactory registration
- Quick reference: key file locations (internal/actions/, internal/version/, internal/executor/)
- Pointer to Go source for full interface details

### Data Flow

```
External recipe author                    Tsuku contributor
        |                                        |
        v                                        v
settings.json snippet                   committed settings.json
        |                                        |
        v                                        v
GitHub source (sparse)                  Local file source
pulls .claude-plugin/ +                 reads .claude-plugin/
plugins/tsuku-recipes/                  marketplace.json
        |                                        |
        v                                        v
recipe-author skill                     recipe-author + recipe-test +
recipe-test skill                       tsuku-contributor skills
```

## Implementation Approach

### Phase 1: Plugin Infrastructure

Create the marketplace and plugin scaffolding with empty skills.

Deliverables:
- `.claude-plugin/marketplace.json`
- `plugins/tsuku-recipes/.claude-plugin/plugin.json`
- `plugins/tsuku-dev/.claude-plugin/plugin.json`
- `.claude/settings.json` (committed, replacing config from settings.local.json)
- Empty SKILL.md stubs for all 3 skills

### Phase 2: recipe-author Skill

Write the P0 skill with the hybrid quick-reference architecture.

Deliverables:
- `plugins/tsuku-recipes/skills/recipe-author/SKILL.md` (~150 lines)
- `plugins/tsuku-recipes/skills/recipe-author/references/exemplar-recipes.md`
- Verification: invoke the skill and confirm it provides useful recipe guidance

### Phase 3: recipe-test Skill

Write the testing workflow skill.

Deliverables:
- `plugins/tsuku-recipes/skills/recipe-test/SKILL.md` (~80-100 lines)
- Verification: walk through the validate -> eval -> sandbox flow using the skill

### Phase 4: tsuku-contributor Skill

Write the contributor development skill.

Deliverables:
- `plugins/tsuku-dev/skills/tsuku-contributor/SKILL.md` (~120-150 lines)
- Verification: invoke the skill and confirm it provides action/provider development guidance

### Phase 5: External Distribution

Update documentation for external recipe authors.

Deliverables:
- Update `docs/guides/GUIDE-distributed-recipe-authoring.md` with Claude Code integration section containing the settings.json snippet
- `plugins/tsuku-recipes/AGENTS.md` for non-Claude-Code agents
- `plugins/tsuku-dev/AGENTS.md`

## Security Considerations

The skills are system prompt content that shapes AI model behavior, not passive documentation. The practical risk is equivalent to CLAUDE.md files -- anyone who trusts the tsuku repo to run Claude Code already trusts its prompt content. The plugin distribution uses the same marketplace mechanism already proven in koto and shirabe.

The committed settings.json references only public resources (tsuku's own marketplace and shirabe's public repo). No credentials, tokens, or private URLs are included. The settings.local.json (gitignored) continues to hold personal configuration.

**Hooks escalation path.** Plugin directories can contain `hooks.json` that execute arbitrary commands on consumers' machines. The tsuku-recipes plugin must not include hooks, since external recipe authors install it with `autoUpdate` potentially enabled. A CI check should verify that `plugins/tsuku-recipes/hooks.json` does not exist. The tsuku-dev plugin may include hooks since it targets repo contributors who already trust the codebase.

**External consumer update policy.** The external consumer settings.json snippet omits `autoUpdate` (defaults to false) so consumers explicitly control when plugin content changes. This limits blast radius if the repo is compromised. Consumers who want automatic updates can opt in.

## Consequences

### Positive

- Recipe authors get focused, contextual help for the most complex contributor task without loading irrelevant dev skills
- External recipe authors can install skills from their own repo with a 12-line settings.json snippet
- Tsuku contributors get plugin infrastructure that auto-loads on clone, matching the koto and shirabe developer experience
- The hybrid content architecture avoids the maintenance burden of full reference embedding while handling common lookups instantly

### Negative

- The quick-reference tables in SKILL.md need manual updates when actions or version providers change
- Exemplar recipe list may become stale as better examples appear in the registry
- Adding `plugins/` to the already-busy tsuku monorepo root adds another top-level directory
- External recipe authors must manually add the settings.json snippet -- there's no automated discovery

### Mitigations

- Quick-reference tables only contain action names and one-line descriptions, not full parameter docs -- changes are infrequent and low-risk
- Exemplar recipes can be validated with a CI check that verifies listed recipe files still exist
- The plugins directory follows an established pattern (koto has the same) and is self-contained
- The settings.json snippet is documented in the distributed recipe authoring guide, the primary entry point for external authors
