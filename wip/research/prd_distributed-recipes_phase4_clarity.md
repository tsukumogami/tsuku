# Clarity Review

## Verdict: FAIL

The PRD is well-structured and covers the problem space thoroughly, but contains enough ambiguities in key areas (fetching mechanism, multi-recipe resolution, manifest format, caching behavior) that two developers could build materially different implementations.

## Ambiguities Found

1. **R1 (Install syntax):** "Extended forms: `owner/repo:recipe-name` (when repo has multiple recipes)" -> What happens when a user runs `tsuku install owner/repo` on a repo with multiple recipes and no recipe name is specified? Does it fail? Install all? Prompt? Pick a default? -> Clarify: define the exact behavior for multi-recipe repos when no recipe name is given (e.g., "fails with an error listing available recipes").

2. **R2 (Auto-registration):** "tsuku automatically registers that source as a known registry" -> What is registered as the name? The `owner/repo` string? A generated slug? Does the user control it? -> Clarify: specify the naming scheme for auto-registered sources (e.g., "registered under the name `owner/repo`").

3. **R3 (Registry convention):** "An optional manifest enables richer metadata for multi-recipe registries" -> The manifest format is completely undefined. This is also listed as Open Question #3, but R3 states it as a requirement. If the manifest is required for multi-recipe repos to work properly, its absence makes multi-recipe support untestable. -> Clarify: either remove multi-recipe manifest from v1 scope or define a minimal schema.

4. **R3 (Registry convention):** "No manifest file is required for single-recipe repos" -> How does tsuku identify which TOML file to use? By filename matching the repo name? By being the only file? What if there are two TOML files and no manifest? -> Clarify: specify the single-recipe selection heuristic (e.g., "if exactly one `.toml` file exists in `.tsuku-recipes/`, use it; if multiple exist without a manifest, fail with an error").

5. **R5 (Central registry priority):** "Unqualified names always resolve from the central registry first, then embedded recipes" -> What about auto-registered distributed sources? If I previously installed `owner/ripgrep` and later run `tsuku install ripgrep`, is the central registry version installed (as a separate tool), or does it recognize I already have ripgrep from a distributed source? -> Clarify: define whether the same tool name from different sources can coexist, and how name collisions are handled.

6. **R10 (Update across registries):** "`tsuku update-registry` refreshes metadata from all registered sources" -> What does "refreshes metadata" mean for a git-based distributed source? Clone/fetch the repo? Download just the `.tsuku-recipes/` directory? Cache the recipe TOML locally? How much of the repo is fetched? -> Clarify: define the fetch mechanism (sparse checkout, archive download, raw file fetch) and what gets cached.

7. **R11 (Version resolution):** "For v1, `tsuku update` always fetches the latest recipe (HEAD or latest tag)" -> "HEAD or latest tag" are two different things. Which one? Is it HEAD of the default branch? The most recent semver tag? The most recent tag of any kind? -> Clarify: pick one strategy and define it precisely (e.g., "fetches the recipe from HEAD of the repo's default branch").

8. **R12 (Graceful degradation):** "commands that check it (update, outdated) show a warning but don't fail entirely" -> What about `tsuku update <specific-tool>` where that specific tool's source is unreachable? That's different from `tsuku outdated` which checks everything. Does a targeted update also just warn, or does it fail since the user explicitly requested it? -> Clarify: distinguish between batch operations (warn and continue) and targeted operations (fail with error).

9. **R9 (Strict mode):** "A system configuration option (`strict_registries`, off by default)" -> Where does this config live? In `$TSUKU_HOME/config.toml`? In state.json? In environment variables? This interacts with Open Question #2 but the requirement doesn't acknowledge that dependency. -> Clarify: at minimum, specify that the storage location is TBD pending Open Question #2, or resolve the open question.

10. **R8 (Registry management):** "`tsuku registry add <name> <source>`" -> What is `<source>`? A GitHub `owner/repo` string? A full URL? A local path? All three? The acceptance criteria shows `tsuku registry add myname https://github.com/owner/repo` (a URL) but R1 uses `owner/repo` shorthand. Are both accepted? -> Clarify: enumerate the accepted source formats.

11. **R13 (No new dependencies):** "Use git operations or HTTP fetching with existing stdlib" -> "Git operations" could mean shelling out to `git` (a system dependency) or reimplementing git protocol in Go. If it means shelling out, this contradicts the project philosophy of "no system dependencies." -> Clarify: specify whether `git` binary is expected on the system or whether HTTP-only fetching (e.g., GitHub archive/raw API) is required.

12. **R15 (Minimal author friction):** "A tool author should go from 'no tsuku support' to 'installable via tsuku' by creating one directory and one file" -> The word "should" signals aspiration, not requirement. Is this a hard requirement or a goal? What if some edge cases (multi-binary tools, complex build steps) require more? -> Clarify: restate as "A tool author MUST be able to..." for the simple case, and specify what's acceptable for complex cases.

13. **Acceptance Criteria #1:** "`tsuku install owner/repo` fetches `.tsuku-recipes/` from the repo and installs the tool" -> "Installs the tool" is not binary pass/fail. What constitutes a successful install? Binary appears in `$TSUKU_HOME/bin/`? State.json is updated? Both? -> Clarify: define the observable postconditions (binary in PATH, state.json entry with source field, etc.).

14. **Known Limitations (Git hosting assumption):** "Non-GitHub sources need a full URL or host override in config" -> The "host override in config" mechanism is completely unspecified. Is this in scope for v1 or not? If it is, it needs a requirement. If not, it should be in Out of Scope. -> Clarify: either add a requirement for host configuration or explicitly defer it.

15. **R16 (Recipe listing) is numbered out of sequence** (appears after R11, before R12). This suggests requirements were added incrementally. While not an ambiguity per se, misnumbering can cause confusion when referencing requirements in design docs and issues. -> Renumber sequentially.

16. **Out of Scope (Content-hash pinning):** "Content-hash pinning is sufficient for now" appears in the cryptographic signing out-of-scope item, but content-hash pinning is itself listed as deferred in Known Limitations. So is content-hash pinning in v1 or not? -> Clarify: remove the "is sufficient for now" phrasing if content-hash pinning is deferred, or add it as a requirement if it's in scope.

## Suggested Improvements

1. **Add a "Fetching Mechanism" section** that specifies how distributed recipe content is retrieved. The PRD is silent on whether tsuku uses `git clone`, GitHub's archive API, raw file downloads, or something else. This is the single largest implementation ambiguity.

2. **Resolve Open Question #1 (directory name) before leaving Draft.** This affects every tool author's experience and the acceptance criteria reference `.tsuku-recipes/` as if it's decided.

3. **Define the caching model.** When a distributed recipe is fetched, where is it cached? For how long? How does the cache interact with `update-registry`? The central registry has `$TSUKU_HOME/registry/`; does the distributed model extend this?

4. **Add error scenario acceptance criteria.** The current criteria are all happy-path. Add criteria for: invalid `owner/repo` (repo doesn't exist), repo exists but has no `.tsuku-recipes/`, recipe TOML is malformed, network failure mid-install.

5. **Specify state.json schema changes.** R6 says tools record their source, and R14 says existing state.json files migrate transparently. Define the new field name and migration behavior (e.g., "existing entries without a `source` field default to `central`").

6. **Clarify the relationship between "registries" and "sources."** The PRD uses both terms. R8 introduces `tsuku registry` commands, but tools are described as having a "source." Are these the same concept? Can a single registry contain multiple tools with different sources?

## Summary

The PRD does a good job framing the problem, defining goals, and scoping what's out. However, it has 16 ambiguities that range from terminology inconsistencies to gaps that would produce different implementations. The most critical gaps are the unspecified fetching mechanism, undefined multi-recipe resolution behavior, and the conflicting statements about content-hash pinning scope. Resolving the open questions and adding error-path acceptance criteria would bring this to a passable state.
