## Summary

After merging PR #1824 (ecosystem name resolution), `tsuku create` incorrectly blocks when the requested tool name matches an existing recipe exactly. The check was intended to prevent duplicate recipe creation for ecosystem aliases (e.g., blocking `tsuku create openssl@3` because the `openssl` recipe already satisfies that name), but it also blocked exact name matches where regeneration is valid and expected.

This caused 8 functional test failures in the post-merge CI run on commit 378cdb53 (Tests workflow run 22266075744, job 64412760941).

## Root cause

PR #1824 added a `checkExistingRecipe` call at the top of `runCreate()` in `cmd/tsuku/create.go`. The function uses the recipe loader to resolve the tool name across all tiers (embedded, registry, cache) plus the new satisfies fallback. When a match is found, `runCreate` blocked creation for **all** matches, including exact name matches where `canonicalName == toolName`.

The problematic code path:

```go
if canonicalName, found := checkExistingRecipe(loader, toolName); found {
    if canonicalName == toolName {
        // This branch incorrectly blocked regeneration of existing recipes
        fmt.Fprintf(os.Stderr, "Error: recipe '%s' already exists. Use --force to create anyway.\n", toolName)
    } else {
        fmt.Fprintf(os.Stderr, "Error: recipe '%s' already satisfies '%s'.\n", canonicalName, toolName)
    }
    exitWithCode(ExitGeneral)
}
```

Both the exact-match branch and the satisfies-match branch called `exitWithCode`, but only the satisfies-match case should block. Exact name matches represent a valid `tsuku create` use case: regenerating an existing recipe with different builder flags.

## All affected functional test scenarios

All 8 failures are in `test/functional/features/create.feature`. Each scenario runs `tsuku create <tool>` for a tool that has an existing recipe, and the overly aggressive check blocks before any builder, API call, or toolchain check runs.

| # | Scenario | Tool | Builder | Feature file line | Expected behavior | Actual |
|---|----------|------|---------|-------------------|-------------------|--------|
| 1 | Create recipe to default location | `prettier` | npm | L7 | exit 0, recipe created | exit 1, "recipe 'prettier' already exists" |
| 2 | Create recipe with --output flag | `prettier` | npm | L13 | exit 0, recipe created at custom path | exit 1, "recipe 'prettier' already exists" |
| 3 | Create recipe from rubygems | `jekyll` | rubygems | L26 | exit 0, recipe created | exit 1, "recipe 'jekyll' already exists" |
| 4 | Create with --yes auto-install missing gem toolchain | `jekyll` | rubygems | L42 | stderr contains "requires gem" | stderr contains "recipe 'jekyll' already exists" |
| 5 | Create recipe from pypi | `ruff` | pypi | L53 | exit 0, recipe created | exit 1, "recipe 'ruff' already exists" |
| 6 | Create recipe from cpan | `ack` | cpan | L64 | exit 0, recipe created | exit 1, "recipe 'ack' already exists" |
| 7 | Create recipe from cask | `iterm2` | cask | L78 | exit 0, recipe created | exit 1, "recipe 'iterm2' already exists" |
| 8 | Deterministic-only with discovery GitHub builder fails with actionable message | `fd` | discovery | L90 | exit 9, "requires LLM" message | exit 1, "recipe 'fd' already exists" |

## Fix

PR #1840 changes the condition to only block when the canonical name differs from the requested name (satisfies-alias match), allowing exact name matches to proceed:

```go
if canonicalName, found := checkExistingRecipe(loader, toolName); found && canonicalName != toolName {
    // Only block satisfies matches (e.g., "openssl@3" -> "openssl")
    fmt.Fprintf(os.Stderr, "Error: recipe '%s' already satisfies '%s'.\n", canonicalName, toolName)
    exitWithCode(ExitGeneral)
}
```

## References

- Source: #1824 (ecosystem name resolution)
- Fix: #1840
- Failed CI run: [Tests workflow run 22266075744](https://github.com/tsukumogami/tsuku/actions/runs/22266075744/jobs/64412760941)
- Failing commit: 378cdb53ccc22b4fab30e208396036a735a64ab6
