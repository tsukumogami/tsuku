# Testability Review

## Verdict: FAIL

Most acceptance criteria describe observable CLI behavior and are testable, but several requirements lack any corresponding AC, and a few criteria are too vague to verify without interpretation.

## Untestable Criteria

1. **AC: "Existing state.json files migrate transparently (no user action needed)"** -- "Transparently" is subjective. What fields are added? What happens if migration fails? Does "no user action" mean automatic on first run, or on first relevant command? -> Rewrite as: "When tsuku runs any command against a pre-existing state.json that lacks source fields, it adds `source: central` to every entry without prompting or erroring. The migrated file is valid JSON matching the new schema."

2. **AC: "koto's recipe works when moved from central registry to `tsukumogami/koto/.tsuku-recipes/koto.toml`"** -- This is a specific integration scenario, not a generalizable acceptance criterion. It's testable only if you have access to that repo with that exact path, making it fragile. It also doesn't specify what "works" means (install succeeds? update succeeds? both?). -> Rewrite as: "A recipe that previously existed in the central registry can be moved to a distributed source. After migration, `tsuku install owner/repo` installs the tool, and `tsuku update` resolves versions correctly from the new source."

3. **AC: "A distributed source being unreachable produces a warning, not a failure, for commands like `tsuku outdated`"** -- "Commands like" is ambiguous. Which commands exactly? Does `tsuku update` also warn-and-continue, or does it fail? Does `tsuku install` (which can't succeed without the source) also just warn? -> Rewrite as: "When a registered distributed source is unreachable: `tsuku outdated` and `tsuku update-registry` emit a warning per unreachable source and complete successfully for reachable sources. `tsuku update <tool>` fails with a clear error if that specific tool's source is unreachable. `tsuku install owner/repo` fails with a connection error."

## Missing Test Coverage

1. **R2 (Auto-registration) edge cases:** No AC covers what happens when auto-registering a source that's already registered, or when the same repo is installed via different refs. No AC tests that auto-registered sources persist across tsuku restarts.

2. **R3 (Registry convention) validation:** No AC for what happens when `owner/repo` exists but has no `.tsuku-recipes/` directory. No AC for malformed recipe files in the directory. No AC for repos with multiple recipes but no manifest.

3. **R4 (Recipe format compatibility):** No AC verifying that a recipe using all supported action types works identically when served from a distributed source vs. central. The koto AC is too narrow to cover this.

4. **R5 (Central registry priority):** The AC says `tsuku install ripgrep` resolves from central, but doesn't test the conflict case: what happens if a distributed source also has a recipe named `ripgrep`? The requirement says distributed sources are only consulted with qualified names, but there's no AC testing that unqualified names never hit distributed sources even when they exist.

5. **R8 (Registry management) edge cases:** No AC for `tsuku registry remove` on a non-existent registry. No AC for `tsuku registry add` with a duplicate name. No AC for `tsuku registry add` with an invalid URL.

6. **R10 (Update across registries):** No AC for `tsuku update-registry` refreshing distributed sources. The existing AC only covers `tsuku update <tool>`, not the bulk registry refresh path.

7. **R11 (Version resolution) with @ref:** The AC covers `install owner/repo:recipe@v1.0` but doesn't test what happens with an invalid ref, or whether `tsuku update` after a pinned-ref install fetches HEAD or stays pinned.

8. **R13 (No new dependencies):** This non-functional requirement has no testable criterion. -> Add: "The go.mod file after implementation contains no new external module dependencies beyond what exists today."

9. **R14 (Backward compatibility) beyond state migration:** The state.json migration AC exists, but there's no AC for backward compatibility of CLI behavior. What happens if someone has scripts calling `tsuku install <tool>` -- does stdout/stderr format change?

10. **R15 (Minimal author friction):** No measurable criterion. "One directory and one file" is stated in the requirement but not verified by any AC. -> Add: "A new distributed recipe can be created with exactly one directory (`.tsuku-recipes/`) and one file (`<name>.toml`) in any git-hosted repository. No other files, accounts, or configuration are required."

11. **R16 (Recipe listing):** The AC says "grouped by source" but doesn't specify ordering within groups, handling of empty groups, or output format. If central has 500 recipes and a distributed source has 2, how does the output look?

12. **Error messages:** R9's strict mode AC says it "tells the user to register first" but doesn't specify the exact guidance. No ACs test that error messages across all failure modes are actionable (include the command to run to fix the issue).

13. **Concurrency/race conditions:** No AC for what happens when two `tsuku install owner/repo` commands run simultaneously and both try to auto-register the same source.

14. **Security edge cases:** The PRD mentions preventing "name confusion attacks" via R5, but no AC verifies that a malicious distributed source can't shadow a central registry name through any install path.

## Summary

The acceptance criteria cover the primary happy paths well -- install, list, update, registry management, and strict mode all have corresponding ACs. However, the PRD has 16 requirements and only 13 ACs, leaving non-functional requirements (R13, R14, R15) without verification. More critically, error handling and edge cases are almost entirely absent: there are no ACs for invalid inputs, unreachable sources during install (vs. update), name conflicts, duplicate registrations, or malformed recipe directories. A tester could build a happy-path plan from these criteria but would have to invent all negative and edge-case tests from scratch.
