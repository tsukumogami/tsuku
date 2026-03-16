# Completeness Review

## Verdict: FAIL

## Issues Found

### Critical (must fix before implementation)

**1. `remove` command behavior for distributed tools is unspecified**

R8 says "removing a registry doesn't uninstall tools." But the PRD never specifies what happens when a user runs `tsuku remove <tool>` on a tool installed from a distributed source. Does it also clean up the auto-registered registry entry if no other tools come from it? Does `tsuku remove owner/repo` work, or only `tsuku remove tool-name`? The current `remove` command uses `tool@version` syntax -- how does the `owner/repo` namespace interact with the tool's installed name?

**2. Name collision between distributed sources is unaddressed**

R5 covers central-vs-distributed priority, but what happens when two different distributed registries provide a recipe with the same tool name? For example, if `alice/tools` and `bob/tools` both install a binary called `mytool`, the second install would collide in `$TSUKU_HOME/bin/`. The scope document (lead 3) flagged this explicitly. The PRD has no resolution rule.

**3. No requirement for `tsuku verify` behavior with distributed recipes**

The `verify` command is a core part of tsuku's install flow (runs post-install, can be invoked standalone). The PRD doesn't mention it. Should `verify` fetch the recipe from the distributed source to re-verify? What if the source is unreachable during verification? R12 (graceful degradation) only covers `update` and `outdated`.

**4. State schema for source tracking is insufficiently specified (R6)**

R6 says "each installed tool records its source." The current `ToolState` struct has no source field -- `RecipeSource` exists only on `Plan` (nested inside `VersionState`), and it's a free-form string ("registry", file path, "validation", "create"). The PRD doesn't specify the state schema change. An implementer would have to guess: new top-level field on `ToolState`? Structured type vs. string? What values for each source type?

**5. How `tsuku install owner/repo` resolves to a recipe name is underspecified**

R1 says the slash distinguishes distributed from central. R3 says the repo contains `.tsuku-recipes/` with TOML files. But: if `owner/repo` has `.tsuku-recipes/foo.toml` and `.tsuku-recipes/bar.toml`, which one gets installed by `tsuku install owner/repo`? The `:recipe-name` extended form exists, but the default behavior (no `:recipe-name`) for multi-recipe repos isn't defined. Does it fail? Install all? Use a heuristic (match repo name)?

### Major (likely to cause implementation rework)

**6. No requirement for `tsuku info owner/repo` or `tsuku info <tool-from-distributed>`**

The user story says `tsuku info` should show source attribution (R7). But there's no AC for `tsuku info` at all. The AC only covers `tsuku list`. What additional information should `info` show for distributed tools -- the repo URL, the git ref used, when the recipe was last fetched?

**7. Caching strategy for distributed recipes is missing**

The central registry has an entire caching subsystem (`CacheRecipe`, `GetCached`, `CacheManager`, metadata sidecars, TTL). The PRD says nothing about how distributed recipe fetches are cached. R10 says `update-registry` refreshes "all registered sources," but what does refreshing a distributed source mean? Git fetch? Re-download the TOML file via raw URL? The Known Limitations section mentions "once cached, subsequent operations work offline" but no requirement defines the caching mechanism.

**8. `tsuku registry add` semantics are ambiguous**

R8 defines `tsuku registry add <name> <source>`. But: what is `<name>`? A user-chosen alias? The `owner/repo` string? What is `<source>` -- a GitHub URL, a git clone URL, a raw content URL? The AC shows `tsuku registry add myname https://github.com/owner/repo` but doesn't clarify whether `myname` is arbitrary or must match the repo. Can you `registry add` the central registry under a different name? Can two names point to the same source?

**9. No requirement for how `tsuku outdated` shows distributed tool updates**

R10 covers `tsuku update`, and R12 covers graceful degradation for `outdated`. But there's no requirement describing what `tsuku outdated` output looks like for distributed tools. Does it show the source? Does it distinguish "recipe updated" from "tool version updated" (especially since recipe pinning is deferred)?

**10. `@ref` interaction with version providers is unclear (R11)**

R11 says `@ref` controls "which version of the recipe definition to fetch" and tool version comes from the recipe's `[version]` section. But: if a user installs `owner/repo@v1.0` and that ref's recipe has `[version] provider = "github"`, does the version provider resolve to the latest release of the tool, or v1.0 of the tool? The two dimensions (recipe version vs. tool version) are acknowledged but the expected user mental model isn't clear. A user typing `@v1.0` probably expects tool version 1.0, not recipe-at-git-tag-v1.0.

### Minor (should fix, won't block implementation)

**11. No AC for the `tsuku recipes` grouped output (R16)**

There's an AC bullet for `tsuku recipes` showing recipes from all sources, but no detail on the output format. Should it show recipe count per source? Should distributed sources that are unreachable be marked?

**12. `tsuku search` interaction is inconsistent**

Out of Scope says "cross-registry search" is deferred, meaning `tsuku search` only searches central. But if a user has auto-registered distributed sources, `tsuku search` won't find tools they've already installed. This is a UX gap that should be called out more explicitly, perhaps as a Known Limitation.

**13. No requirement for `tsuku doctor` to check distributed registry health**

`tsuku doctor` is a diagnostic command (visible in cmd/tsuku/doctor.go). It should probably validate that registered distributed sources are reachable. Not mentioned anywhere.

**14. Migration AC is too vague**

AC says "existing state.json files migrate transparently." But what does migration produce? If existing tools have `RecipeSource: "registry"` in their plans, do they get a new top-level source field set to "central"? What about tools installed from local paths -- how are they classified post-migration?

**15. No requirement covering `tsuku install --recipe <path>` interaction**

The existing `info` command already supports `--recipe <path>` for local recipe files. How does this interact with the new source model? Is a local path install tracked differently from a distributed install? The unified abstraction (Goal 3) implies these should all go through the same interface, but no requirement defines the local-path source type.

## Suggested Improvements

1. Add a "Command Impact Matrix" section that lists every tsuku command and its expected behavior change (or explicit "no change"). The scope document's lead 6 flagged this gap. Commands to cover: install, remove, list, info, update, outdated, verify, search, recipes, doctor, plan, eval, cache, config.

2. Define the state schema change for R6 explicitly. Show the before/after `ToolState` struct with the new source field and its possible values.

3. Add a disambiguation rule for multi-recipe repos when no `:recipe-name` is given. Recommend: if exactly one recipe exists, use it; if multiple exist, fail with a message listing available recipes.

4. Add a name collision rule for distributed sources. Recommend: tool names are namespaced by source in state, and `$TSUKU_HOME/bin/` symlinks use the unqualified name with a conflict error if two sources provide the same binary.

5. Clarify `@ref` semantics with a concrete example showing the difference between recipe git ref and tool version. Consider whether `@ref` is the right syntax given that `@version` already means tool version in `remove` and other commands.

6. Resolve open questions 1 and 2 before moving to design. Both affect the implementation architecture. Open question 3 (manifest schema) can remain open.

7. Add ACs for: `tsuku info <distributed-tool>`, `tsuku verify <distributed-tool>`, `tsuku remove <distributed-tool>`, and name collision error messages.

## Summary

The PRD covers the core happy path well: install from `owner/repo`, auto-register, update, list with attribution. The problem statement is clear and the goals are well-scoped. However, it has 5 critical gaps that would force an implementer to make unguided decisions: `remove` behavior, name collisions, `verify` integration, state schema details, and multi-recipe default resolution. The `@ref` syntax also risks user confusion given the existing `@version` convention elsewhere in tsuku. The PRD should also include a command impact matrix since this feature touches nearly every command. With these gaps addressed, the PRD would be ready for a design doc.

15 issues total: 5 critical, 5 major, 5 minor.
