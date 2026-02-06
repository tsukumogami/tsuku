# Disambiguation Builder Audit Findings

## Summary

Audited 20 tools in `data/discovery-seeds/disambiguations.json` against existing recipes.

- **Tools with recipes**: 8
- **Builder mismatches**: 4
- **Tools without recipes**: 12 (github builder is appropriate)

## Mismatches Found

The following tools have recipes with `homebrew` action but are marked with `github` builder in disambiguations.json:

| Tool | Recipe Path | Action | Current Builder | Correct Builder |
|------|-------------|--------|-----------------|-----------------|
| dust | recipes/d/dust.toml | homebrew | github | homebrew |
| fd | recipes/f/fd.toml | homebrew | github | homebrew |
| task | recipes/g/go-task.toml | homebrew | github | homebrew |
| yq | recipes/y/yq.toml | homebrew | github | homebrew |

## Tools with Correct Builders

These tools have recipes and correct builders:

| Tool | Recipe Path | Action | Builder | Status |
|------|-------------|--------|---------|--------|
| age | recipes/a/age.toml | github_archive | github | OK |
| fzf | recipes/f/fzf.toml | github_archive | github | OK |
| gum | recipes/g/gum.toml | github_archive | github | OK |
| just | recipes/j/just.toml | github_archive | github | OK |

## Tools Without Recipes

The following 12 tools don't have existing recipes, so `github` builder is appropriate:

- bat, buf, delta, dive, gh, hub, jq, ko, rg, sd, sk, step

## Impact

The 4 mismatched tools will fail when users try to install them with `--deterministic-only` mode because:
- They are routed through the `github` builder
- The `github` builder requires LLM for recipe generation
- But deterministic-only mode disables LLM

These tools should use the `homebrew` builder since they have existing recipes with the homebrew action.

## Recommendation

Update `data/discovery-seeds/disambiguations.json` to change the builder from `github` to `homebrew` for:
- dust
- fd
- task
- yq
