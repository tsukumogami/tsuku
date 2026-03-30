# Phase 2 Research: Architecture Perspective

## Lead 2: Priority ordering and phasing

### Findings

After reading all eight exploration research files and validating against the source code, I identified 15 distinct capabilities in the auto-update design space. The codebase has significant existing infrastructure that reduces scope for some capabilities but also reveals prerequisite bugs that must be fixed before any auto-update work is safe.

**Key codebase observations:**

1. **The `Requested` field bug is confirmed.** `update.go:82` calls `runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient)` -- both `reqVersion` and `versionConstraint` are empty strings. This means `tsuku update node` after `tsuku install node@18` will resolve to the absolute latest (e.g., Node 22), ignoring the user's original `@18` constraint. The `Requested` field is stored in state but never consulted during updates.

2. **The `outdated` command bypasses ProviderFactory.** `outdated.go:77-88` scans recipe steps for `github_archive` / `github_file` actions and calls `res.ResolveGitHub()` directly. Any tool sourced from PyPI, npm, crates.io, Homebrew, or custom sources is invisible to outdated checks. Meanwhile, `ProviderFactory` (used by `executor.go` and `versions.go`) supports 20+ strategies and correctly routes any recipe.

3. **`ResolveLatest` is not cached.** `cache.go:87-89` delegates `ResolveLatest` and `ResolveVersion` directly to the underlying provider. Only `ListVersions` is cached. Auto-update checks calling `ResolveLatest` hit the network every time.

4. **Multi-version and atomic installation already work.** `state.go` tracks `ActiveVersion` + `Versions` map. `manager.go` uses staging-then-rename for atomic installs and `AtomicSymlink` for symlink switching. Rollback = keep old version directory, switch `ActiveVersion` back.

5. **No self-update infrastructure exists.** The tsuku binary lives at `$TSUKU_HOME/bin/tsuku` outside the managed tool system. No `self-update` subcommand exists.

6. **No background/async or notification infrastructure exists.** No file-based signaling, no deferred error queue, no update check cache. The telemetry notice marker-file pattern (`internal/telemetry/notice.go`) is the closest existing analog.

7. **The userconfig system is extensible.** Adding new config keys (e.g., `[updates]` section) follows the existing `Get`/`Set` pattern in `internal/userconfig/userconfig.go`.

### Dependency Graph

```
                        +--------------------------+
                        | C1: Fix Requested field  |
                        |     bug in update.go     |
                        +----------+---+-----------+
                                   |   |
                      +------------+   +-----------+
                      v                            v
         +-------------------------+    +-------------------------+
         | C2: Fix outdated to use |    | C5: Version channel     |
         |     ProviderFactory     |    |     pinning (pin level  |
         +----------+--------------+    |     from Requested)     |
                    |                   +----------+--------------+
                    v                              |
         +-------------------------+               |
         | C3: Cache ResolveLatest |               |
         |     results (new cache) |               |
         +----------+--------------+               |
                    |                              |
                    v                              v
         +-------------------------+    +-------------------------+
         | C4: Time-cached update  |    | C6: Pin-aware outdated  |
         |     checks (background  |    |     (show in-pin vs     |
         |     goroutine + cache   |    |     out-of-pin updates) |
         |     file)               |    +-------------------------+
         +----------+--------------+
                    |
                    v
         +-------------------------+
         | C7: Update notification |
         |     UX (stderr hints,   |
         |     CI suppression)     |
         +-------------------------+


         +-------------------------+       +-------------------------+
         | C8: Self-update binary  |       | C10: tsuku update --all |
         |     (rename-in-place)   |       |      (batch updates)    |
         +----------+--------------+       +-------------------------+
                    |                              ^
                    v                              |
         +-------------------------+               |
         | C9: Self-update check   |    +----------+
         |     (version cache +    |    |
         |     notification)       |    |
         +-------------------------+    |
                                        |
         +-------------------------+    |
         | C11: Rollback on update |----+
         |      failure (keep old  |
         |      version, revert    |
         |      ActiveVersion)     |
         +-------------------------+

         +-------------------------+
         | C12: Deferred error     |
         |      reporting (notices |
         |      directory)         |
         +-------------------------+

         +-------------------------+
         | C13: Update config in   |
         |      config.toml        |
         |      (interval, disable)|
         +-------------------------+

         +-------------------------+
         | C14: Graceful offline   |
         |      degradation        |
         +-------------------------+

         +-------------------------+
         | C15: Update outcome     |
         |      telemetry          |
         +-------------------------+
```

**Dependency edges (A depends on B):**

| Capability | Depends on |
|---|---|
| C2 (Fix outdated) | None (independent fix) |
| C1 (Fix Requested bug) | None (independent fix) |
| C3 (Cache ResolveLatest) | C2 (needs ProviderFactory for all providers) |
| C4 (Time-cached update checks) | C3 (needs cached resolution to avoid network on every check) |
| C5 (Version channel pinning) | C1 (needs Requested to be respected first) |
| C6 (Pin-aware outdated) | C2, C5 (needs both ProviderFactory and pinning) |
| C7 (Notification UX) | C4 (needs cached check results to display) |
| C8 (Self-update binary) | None (fully independent) |
| C9 (Self-update check) | C8 (needs self-update mechanism), C4 (shares cache infrastructure) |
| C10 (Batch update --all) | C1 (must respect Requested per tool) |
| C11 (Rollback) | None (leverages existing multi-version) |
| C12 (Deferred error reporting) | None (new subsystem, but most valuable alongside C4 or C11) |
| C13 (Update config) | None (independent, but gates C4 and C7 for user control) |
| C14 (Offline degradation) | C3 or C4 (needs cache to fall back to) |
| C15 (Update telemetry) | C10 or C11 (needs update events to track) |

### Proposed Phasing

#### Phase 0: Prerequisite Fixes (no new features, fix existing bugs)

| ID | Capability | Rationale | Effort |
|---|---|---|---|
| C1 | Fix Requested field bug in `update.go` | `tsuku update` must respect the original version constraint. Without this, any auto-update will break pinned tools. | Small |
| C2 | Fix `outdated` to use `ProviderFactory` | The outdated command only checks GitHub tools. This is both a standalone bug and a prerequisite for reliable update checking across all providers. | Medium |

**Why these must ship first:** C1 is a correctness bug that makes any form of automatic updating dangerous. C2 is a coverage gap that makes update checking unreliable. Both are independently valuable as bug fixes, even without auto-update.

#### Phase 1: MVP Auto-Update (smallest set that delivers core value)

| ID | Capability | Rationale | Effort |
|---|---|---|---|
| C3 | Cache `ResolveLatest` results | Prevents network requests on every check. Follows existing cache patterns (TTL + atomic write + corruption-as-miss). | Medium |
| C4 | Time-cached update checks | Background goroutine during command execution, writes to `$TSUKU_HOME/cache/update-check.json`. The core mechanism. | Medium |
| C7 | Basic notification UX | Stderr hint after command completion. Suppressed by `TSUKU_NO_UPDATE_CHECK`, `--quiet`, non-TTY, and `CI=true`. | Small |
| C13 | Update config in config.toml | `[updates]` section with `enabled` (bool) and `interval` (duration). Needed for user control of C4. | Small |
| C8 | Self-update binary mechanism | Rename-in-place pattern. `tsuku self-update` command. High user value, independent of tool auto-update. | Medium |

**MVP delivers:** Users get notified about available updates (for all tools, not just GitHub), can self-update the CLI, and can configure/disable the behavior. The check is non-blocking and time-gated.

**MVP does NOT include:** Version channel pinning (updates show absolute latest), rollback, deferred error reporting, batch update. These are valuable but not required for a useful first release.

#### Phase 2: Polish and Channels

| ID | Capability | Rationale | Effort |
|---|---|---|---|
| C5 | Version channel pinning | Derive pin level from Requested field component count. `""` = track latest, `"20"` = major pin, `"1.29"` = minor pin, `"1.29.3"` = exact pin (no auto-update). | Medium |
| C6 | Pin-aware outdated display | Show "available within pin" vs "available overall" columns in `tsuku outdated`. | Small |
| C9 | Self-update check integration | Fold tsuku's own version into the cached update check. Show "A new version of tsuku is available" alongside tool notifications. | Small |
| C10 | `tsuku update --all` | Batch update all outdated tools (respecting pins). Trivial once single-tool update is correct. | Small |
| C11 | Rollback on update failure | Keep old version directory during update. On failure, revert `ActiveVersion` and re-symlink. Leverages existing multi-version. | Medium |
| C14 | Graceful offline degradation | When update check fails (network error), use last cached result with a stale-data notice. Follow recipe cache's stale-if-error pattern. | Small |

**Phase 2 delivers:** Pin-aware updates that respect user intent, batch operations, resilience against failures and offline environments, and a unified self-update experience.

#### Future (independent, ship when needed)

| ID | Capability | Rationale | Effort |
|---|---|---|---|
| C12 | Deferred error reporting | File-based notices in `$TSUKU_HOME/notices/`. Valuable for unattended updates, but auto-update in Phase 1 is interactive (user sees output). Becomes important when/if background scheduled updates are added. | Medium |
| C15 | Update outcome telemetry | Extend existing telemetry events with success/failure/rollback outcomes. Low priority until there's enough update volume to analyze. | Small |
| -- | Security advisory integration | Cross-reference installed versions against vulnerability databases. Different data pipeline, different UX. Reuses notification infrastructure. | Large |
| -- | Pre/post-update hooks | User-defined scripts in config. Niche use case. | Medium |
| -- | Organization policy files | Shared version constraints across teams. Enterprise feature. | Large |
| -- | Scheduled/background updates | Cron-like unattended updates. Users can `crontab` themselves; tsuku doesn't need this initially. | Medium |

### Implications for Requirements

1. **The PRD should treat Phase 0 fixes as hard prerequisites, not optional.** The Requested field bug and the outdated/ProviderFactory gap are not "nice to haves" -- they are correctness issues that make auto-update unsafe or incomplete. The PRD should either include them in scope or explicitly require them to be resolved before auto-update work begins.

2. **Self-update and tool auto-update should be specced together but can be implemented independently.** They share notification UX and configuration, but the binary replacement mechanism (self-update) and the version-check-and-notify mechanism (tool auto-update) have no code-level dependency. The PRD can cover both in a single document with clear separation.

3. **Version channel pinning is Phase 2, not MVP.** The explore research suggests channels are important, but the existing `Requested` field already stores the data. MVP can show updates against absolute latest and still be useful. Phase 2 adds the filtering layer. The PRD should define the pin semantics now (so the data model is right) but mark the enforcement as Phase 2.

4. **The notification UX should be defined minimally.** MVP = stderr line. Phase 2 = configurable levels (`off`/`pinned`/`all`). The PRD should spec the final state but note which parts are MVP vs later.

5. **Rollback is Phase 2 because the existing architecture already handles the happy path.** Atomic staging + multi-version means a failed download or extraction already leaves the system clean. Rollback only matters when the new version installs successfully but turns out broken at runtime -- a harder problem that can wait.

6. **The cache file format should be designed for extensibility.** MVP stores `{tool: latestVersion, checkedAt}`. Phase 2 adds `{pinnedLatest, absoluteLatest}`. The PRD should define the final schema even if MVP only populates a subset.

7. **`tsuku update --all` depends on C1 (Requested fix).** Without it, `update --all` would break every pinned tool. This is why C10 is in Phase 2 alongside C5, even though it's trivial to implement.

### Open Questions

1. **Should Phase 0 fixes be separate issues/PRs that ship before the auto-update work begins, or part of the auto-update implementation?** Separate is cleaner (they're independently valuable bug fixes), but it means three phases of delivery instead of two.

2. **How should the MVP notification handle pinned tools?** MVP doesn't have pin awareness. If a user installed `node@18` and Node 22 is out, the MVP notification would say "node: 18.20.0 -> 22.3.0 available." This could be confusing for users who intentionally pinned. Should MVP suppress notifications for tools with a non-empty `Requested` field, or show them anyway with a note?

3. **Should the PRD mandate a specific cache file format, or leave it to the design doc?** The explore research recommends `$TSUKU_HOME/cache/update-check.json` with a specific schema. The PRD could define requirements (TTL, invalidation triggers) and leave format to design.

4. **Is the `tsuku self-update` command name settled?** Alternatives: `tsuku upgrade` (like proto), `tsuku self update` (like rustup, two words), `tsuku update --self`. The command name affects the notification message text.

5. **Should Phase 1 include `TSUKU_NO_UPDATE_CHECK` as an env var, a config.toml key, or both?** The explore research recommends both. The dual-config pattern is established in the codebase but adding it to config.toml for the first time for a TTL setting would be new.

## Summary

The auto-update feature decomposes into 15 capabilities with a clear dependency chain. Two prerequisite bug fixes (the Requested field being ignored by `tsuku update`, and `tsuku outdated` only checking GitHub tools) must ship first since they make any form of automatic updating either unsafe or incomplete. The MVP (Phase 1) is five capabilities: ResolveLatest caching, time-gated background checks, basic stderr notifications, update config, and self-update -- delivering the core "know when updates exist and be able to act on them" experience. Version channel pinning, pin-aware display, rollback, and batch update follow in Phase 2 once the foundation is solid.
