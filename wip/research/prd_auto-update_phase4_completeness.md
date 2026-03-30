# Completeness Review

## Verdict: PASS
The PRD is thorough and implementable with minor gaps that should be addressed before finalizing.

## Issues Found

1. **No AC for R21 (Atomic operations)**: R21 specifies temp-file-then-rename for all file writes, but no acceptance criterion verifies this. An implementer might skip atomic writes for state.json or cache files. Add AC: "All file writes (state, cache, notices) use temp-file-then-rename; a crash during write never corrupts existing files."

2. **No AC for R19 (Zero added latency / 10s timeout)**: R19 specifies a 10-second absolute timeout for background checks, but the AC only says "the primary command's output is not delayed." Add AC: "Background update check goroutine is terminated after 10 seconds regardless of completion state."

3. **`auto_apply` config key referenced in D1 but absent from R-list and AC**: D1 mentions `updates.auto_apply = false` as the way to get notification-only mode. But the config AC (Configuration section) lists `enabled`, `check_interval`, `notify_out_of_channel`, and `self_update` -- no `auto_apply`. Either add `auto_apply` to the config AC or clarify that `enabled = false` covers this case.

4. **Rollback after rollback is unspecified**: R9 says rollback switches to the "previously active version." What if the user rolls back, then auto-update applies a new version, and the user rolls back again -- do they get the version before the auto-update, or the version they manually rolled back to last time? The PRD should clarify that rollback history is one level deep (the last active version before the current one).

5. **Self-update version pinning is unspecified**: R8 says tsuku's version is checked, but the PRD doesn't say whether tsuku self-update respects any pin boundary. Can the user pin tsuku to a major version? If not, state that self-update always tracks latest. If yes, describe how the pin is set.

6. **`TSUKU_AUTO_UPDATE=1` in R16 has no AC**: R16 says explicit opt-in via `TSUKU_AUTO_UPDATE=1` overrides CI detection, but no acceptance criterion covers this. Add one under the Notification and suppression section.

7. **Scope doc research lead #4 (edge cases) partially addressed**: The scope document flagged "recipe changes format between versions" as needing AC. The PRD's known limitations cover concurrent updates and state-directory gaps, but recipe format incompatibility during auto-update isn't mentioned. If a recipe TOML schema changes between tsuku versions and auto-update applies old recipes, what happens?

8. **No AC for old version garbage collection (R18)**: R18 specifies a configurable retention period (default 7 days) and a GC mechanism, but no AC verifies that old versions are actually cleaned up after the retention period, or that GC doesn't delete a version that's still the rollback target.

## Suggested Improvements

1. **Add a configuration reference table**: The config surface is spread across R4, R12, R13, R16, R17, D1, and the Configuration AC section. A single table listing every config key, its location (config.toml / env var / CLI flag / .tsuku.toml), default value, and description would help implementers and prevent inconsistencies.

2. **Clarify background auto-apply timing**: R3 says "the update happens during a tsuku command invocation as a non-blocking background operation." It's unclear whether the update is applied mid-command (symlinks swapped while the user's command is running) or staged and applied on the next invocation. Spell out the exact lifecycle: check -> download -> stage -> apply (when?).

3. **Add US for notification-only users**: D1 mentions `auto_apply = false` for notification-only mode, but there's no user story for this persona. A developer who wants awareness without automatic changes is a common pattern. Adding a user story would anchor the `auto_apply` config key.

4. **Specify notice file format and cleanup**: R11 describes the notice system ($TSUKU_HOME/notices/) but doesn't specify the file format, naming convention, or cleanup policy. An implementer would have to guess whether notices are one-file-per-event, a single append log, JSON, or plain text.

5. **Clarify "latest" vs empty-string pin behavior for pre-release filtering**: R1 says empty string or "latest" tracks the latest stable version. The out-of-scope section mentions pre-release filtering. Worth explicitly stating that "latest" means "latest stable, excluding pre-releases" to avoid ambiguity.

## Summary

The PRD covers the problem space well, with clear phasing, well-reasoned trade-offs, and strong alignment to the scope document's research leads. The main gaps are missing acceptance criteria for several non-functional requirements (atomicity, timeouts, GC), an undocumented config key (`auto_apply`), and some underspecified behaviors (rollback depth, self-update pinning, notice file format). These are fixable without restructuring.
