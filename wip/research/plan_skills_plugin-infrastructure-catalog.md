# Plugin Infrastructure Catalog

Patterns from koto and shirabe for implementing Claude Code plugins.

## marketplace.json Schema

Located at `.claude-plugin/marketplace.json`:

```json
{
  "$schema": "https://anthropic.com/claude-code/marketplace.schema.json",
  "name": "tsuku",
  "description": "AI skills for the tsuku package manager",
  "owner": { "name": "tsukumogami" },
  "plugins": [
    {
      "name": "tsuku-recipes",
      "description": "Recipe authoring and testing skills",
      "version": "0.1.0",
      "author": { "name": "tsukumogami" },
      "source": "./plugins/tsuku-recipes"
    },
    {
      "name": "tsuku-user",
      "description": "End-user CLI and project configuration skills",
      "version": "0.1.0",
      "author": { "name": "tsukumogami" },
      "source": "./plugins/tsuku-user"
    }
  ]
}
```

## plugin.json Schema

Located at `plugins/<name>/.claude-plugin/plugin.json`:

Pattern A (koto - explicit skill list):
```json
{
  "name": "tsuku-recipes",
  "version": "0.1.0",
  "description": "Recipe authoring and testing skills",
  "author": { "name": "tsukumogami" },
  "skills": [
    "./skills/recipe-author",
    "./skills/recipe-test"
  ]
}
```

Pattern B (shirabe - glob pattern):
```json
{
  "skills": "./skills/"
}
```

For tsuku: use Pattern A (explicit list) for clarity.

## SKILL.md Frontmatter

```yaml
---
name: skill-name
description: |
  Multi-line description explaining:
  - What the skill does
  - When to use it
  - Trigger phrases
argument-hint: '<expected argument format>'
---
```

## settings.json (committed)

```json
{
  "enabledPlugins": {
    "tsuku-recipes@tsuku": true,
    "tsuku-user@tsuku": true,
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

## External Consumer settings.json Snippet

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

## Directory Structure for tsuku

```
tsuku/
  .claude-plugin/
    marketplace.json
  .claude/
    settings.json          # committed
    settings.local.json    # gitignored (personal: tsukumogami, env, hooks, permissions)
  plugins/
    tsuku-recipes/
      .claude-plugin/
        plugin.json
      skills/
        recipe-author/
          SKILL.md
          references/
            action-reference.md
            platform-reference.md
            verification-reference.md
            dependencies-reference.md
            distributed-reference.md
            exemplar-recipes.md
        recipe-test/
          SKILL.md
      AGENTS.md
    tsuku-user/
      .claude-plugin/
        plugin.json
      skills/
        tsuku-user/
          SKILL.md
```

## Koto's CLAUDE.md Plugin Maintenance Section (Template)

```markdown
## tsuku-recipes Plugin Maintenance

Skills in `plugins/tsuku-recipes/skills/` guide agents authoring and testing
tsuku recipes. They drift silently when tsuku changes without a corresponding
skill update.

| Skill | Path | Scope |
|-------|------|-------|
| recipe-author | plugins/tsuku-recipes/skills/recipe-author/ | Recipe TOML writing |
| recipe-test | plugins/tsuku-recipes/skills/recipe-test/ | Recipe testing workflow |

**After completing any source change in the areas below, assess both skills:**

1. **Broken contracts** -- read the diff and each skill: does anything documented
   no longer match the code?
2. **New surface** -- does this change add behavior that neither skill mentions?

| Area | Relevant skill |
|------|---------------|
| internal/actions/ -- action names, params, Dependencies() | recipe-author |
| internal/version/ -- provider types, source values | recipe-author |
| internal/recipe/ -- TOML structure, when clauses, validation | recipe-author |
| internal/executor/ -- plan generation, decomposition | recipe-test |
| cmd/tsuku/validate.go -- validation rules, exit codes | recipe-test |
```
