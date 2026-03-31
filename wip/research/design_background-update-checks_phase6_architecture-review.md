# Architecture Review: DESIGN-background-update-checks.md

Reviewer: Architect Reviewer
Focus: Structural fit with existing codebase patterns

---

## 1. Is the architecture clear enough to implement?

**Yes, with two gaps that need resolution before implementation.**

The design specifies file paths, function signatures, data structures, and the stat-flock-spawn protocol in enough detail for an implementer to proceed. The data flow diagram is unambiguous. The `UpdateCheckEntry` struct schema is fully defined.

### Gap A: Missing TryLockExclusive in FileLock

The design says the trigger calls a non-blocking flock (`LOCK_EX|LOCK_NB`). The existing `FileLock` in `internal/install/filelock.go` only exposes blocking `LockShared()` and `LockExclusive()`. The Unix implementation (`filelock_unix.go`) directly calls `syscall.Flock` with `LOCK_EX` (no `LOCK_NB` flag). The design acknowledges this ("will gain `TryLockExclusive()` or the function uses raw `syscall.Flock` directly") but doesn't decide which path.

**Recommendation:** Add `TryLockExclusive() (bool, error)` to `FileLock`. This keeps the lock abstraction in one place and avoids raw syscall usage in `internal/updates/`. The Windows implementation needs a corresponding `LockFileEx` with `LOCKFILE_FAIL_IMMEDIATELY`. This is a real implementation dependency that Phase 3 should account for -- it touches `filelock.go`, `filelock_unix.go`, and `filelock_windows.go`.

### Gap B: How checker obtains recipe and state

The data flow shows the checker calling `loadRecipeForTool` and loading `state.json`, but the design doesn't specify which packages provide these. The checker needs:
- Installed tool list with active version and requested constraint (from `internal/install.State`)
- Recipe loading per tool (from `internal/recipe` or through `config.Config.RegistryDir`)
- Provider construction (from `internal/version.ProviderFactory`)

These are all existing types, but the `checker.go` file's import graph matters. An explicit list of dependencies in the design would prevent the implementer from accidentally pulling in `cmd/` packages or creating circular imports.

---

## 2. Are there missing components or interfaces?

### No new interfaces needed -- confirmed correct

The design explicitly states "No interface changes" and composes existing `VersionResolver`, `VersionLister`, and `ProviderFactory`. This is the right call. The `UpdateCheckEntry` is a data struct, not a contract. The checker is a concrete function, not a pluggable abstraction. For a background process with a single implementation, this is appropriate.

### Missing: cache directory cleanup / migration

The design places update cache files at `$TSUKU_HOME/cache/updates/`. The existing version cache lives at `$TSUKU_HOME/cache/versions/`. Both use atomic temp+rename writes. Both are JSON files. But they have different naming conventions: version cache uses SHA256-hashed filenames (`hex[:16].json`), update cache uses tool names (`<toolname>.json`). The design correctly notes tool names are filesystem-safe, so the divergence is justified.

However, neither the design nor Feature 7 (referenced for cleanup) is implemented yet. The cache directory will accumulate stale entries for uninstalled tools. This isn't a blocking structural issue -- it's bounded and documented in Consequences -- but Phase 1 should include a `RemoveEntry(cacheDir, toolName)` function so that `tsuku remove` can clean up eagerly. Without it, the cache becomes a second source of truth for "what's installed" that drifts from `state.json`.

### Missing: error propagation strategy for trigger

The trigger function signature returns `error`, but all three call sites discard it (`_ = updates.CheckAndSpawnUpdateCheck(...)`). This is fine for the trigger (best-effort), but the design should state this explicitly so implementers don't add error logging that spams every prompt. The `hook-env` path especially must be silent -- any stderr output corrupts the shell eval.

---

## 3. Are the implementation phases correctly sequenced?

**Yes. The sequencing is sound.**

- Phase 1 (cache + config) has zero dependencies on new code. It extends `userconfig.go` following the established LLMConfig pattern and creates the new `internal/updates/` package with pure data read/write functions. Independently testable.
- Phase 2 (checker) depends on Phase 1's cache write functions and on existing `version.ProviderFactory` + `version.ResolveWithinBoundary`. No circular dependency risk.
- Phase 3 (trigger) depends on Phase 1's `isCheckStale` and Phase 2's existence (to spawn the subcommand). The trigger doesn't import `checker.go` -- it spawns a process. Clean boundary.

One ordering refinement: Phase 2 should include the `TryLockExclusive` addition to `FileLock` (Gap A above), since the checker also acquires the flock (blocking mode). Phase 3 needs the non-blocking variant. If `TryLockExclusive` ships in Phase 3, the checker in Phase 2 can't be fully integration-tested with realistic lock contention. Moving it to Phase 2 is cleaner.

---

## 4. Are there simpler alternatives overlooked?

### The sentinel model is the right choice

The O(1) stat on `.last-check` vs O(N) per-tool stat is a genuine win. For 50+ installed tools, the difference matters on every prompt. The alternatives (directory mtime, metadata JSON) are correctly rejected.

### Consider: reuse CachedVersionLister TTL instead of a separate sentinel

The existing `CachedVersionLister` already has per-entry `ExpiresAt` fields and TTL-based freshness checks. The update cache could piggyback on this: if any version cache entry is expired, a check is needed. This would eliminate the sentinel file entirely.

**Why this doesn't work:** The version cache uses SHA256-hashed filenames, so you'd need to glob and parse each file to check expiry -- back to O(N). The sentinel is simpler. The design made the right call.

### Consider: single background process vs per-trigger spawn

The design spawns a new `tsuku check-updates` process each time the sentinel is stale. An alternative is a long-running daemon (like `brew autoupdate`). The design correctly avoids this -- a daemon is a much larger maintenance surface, and the flock-based dedup means at most one check process runs at a time.

---

## 5. Does the design compose well with existing patterns?

### Version cache pattern: consistent

The `WriteEntry` function uses atomic temp+rename, matching `CachedVersionLister.writeCache()` exactly. Cache directory is under `$TSUKU_HOME/cache/`, consistent with the version cache at `$TSUKU_HOME/cache/versions/`. The `UpdateCheckEntry` struct mirrors `cacheEntry` (both have `CachedAt`, `ExpiresAt`, `Source` fields). Good pattern reuse.

### FileLock pattern: needs extension, not replacement

The design correctly identifies `internal/install/filelock.go` as the lock primitive. It needs `TryLockExclusive` but doesn't propose a parallel lock implementation. This is the right approach -- extend the existing type.

**Structural note:** `FileLock` lives in `internal/install/`, which is a higher-level package than a pure locking primitive warrants. The new `internal/updates/` package will import `internal/install` for `FileLock`, `State`, `ToolInfo`, and `ValidateRequested`. This is fine directionally (updates depends on install, not the reverse), but it means `internal/updates/` inherits all of `internal/install`'s transitive dependencies. If this becomes a problem, extracting `FileLock` to `internal/filelock/` would be a clean refactor -- but not needed now.

### UserConfig pattern: precise match

The design's `UpdatesConfig` with `*bool` pointer fields and getter methods checking env vars first is an exact structural copy of `LLMConfig`. The five getter methods follow the same pattern as `LLMEnabled()`, `LLMIdleTimeout()`, etc. The `Get`/`Set`/`AvailableKeys` extensions are mechanical additions to the existing switch statements. No new abstractions introduced.

### Hook-env integration: minimal and correct

The `hook-env.go` modification is a single line after `ComputeActivation()`. The current `hook-env` is 47 lines, loads `config.DefaultConfig()` but not `userconfig.Config`. The trigger function signature takes both `*config.Config` and `*userconfig.Config`, meaning the hook-env will need to add a `userconfig.Load()` call.

**Potential concern:** `userconfig.Load()` reads and parses a TOML file. The design's 5ms budget assumes this is fast. Looking at the implementation, `Load()` calls `os.ReadFile` + `toml.Decode`. For a typical config file (<1KB), this is <0.5ms. But it's not cached -- every prompt invocation re-parses. If this is already loaded elsewhere in the command lifecycle, it should be shared. Since `hook-env` is a standalone hidden command with no `PersistentPreRun` config loading, adding `userconfig.Load()` here is the right approach.

### CLI surface: clean

The `check-updates` hidden subcommand doesn't overlap with existing commands. `tsuku outdated` shows outdated tools interactively; `check-updates` silently populates the cache. Different purpose, no duplication.

### PersistentPreRun integration: requires care

The current `PersistentPreRun` is just `initLogger`. The design adds a skip-list-gated call. Since cobra's `PersistentPreRun` doesn't chain (child overrides parent), the implementer needs to verify no subcommand sets its own `PersistentPreRun` or `PersistentPreRunE`. If any do, the trigger call won't fire for those commands.

---

## Findings Summary

### Blocking

1. **FileLock missing TryLockExclusive** -- The trigger's non-blocking flock probe requires `LOCK_NB` support that doesn't exist in the current `FileLock` API. Without it, the implementer will either (a) use raw `syscall.Flock` in `internal/updates/`, bypassing the lock abstraction (parallel pattern), or (b) call blocking `LockExclusive()` which defeats the <2ms budget by potentially blocking indefinitely. The design must specify that `TryLockExclusive() (bool, error)` is added to `FileLock` as a prerequisite, with platform implementations for both Unix and Windows.

### Advisory

2. **hook-env needs userconfig.Load() not mentioned in design** -- The design says the trigger takes `*userconfig.Config` but hook-env currently doesn't load it. The modification is trivial but should be listed in the Phase 3 deliverables to avoid an implementer passing `nil`.

3. **Add RemoveEntry to Phase 1 cache API** -- Without it, `tsuku remove` can't clean up stale update cache entries, creating drift between `state.json` and the cache directory. Small addition, prevents a class of staleness bugs.

4. **PersistentPreRun chaining** -- Cobra doesn't chain `PersistentPreRun` from parent to child. If any subcommand defines its own, the trigger is silently skipped. The design should note this constraint and recommend `PersistentPreRunE` with explicit parent chain calls, or verify no subcommands override it.

5. **Checker import graph unspecified** -- `internal/updates/checker.go` will need `internal/install`, `internal/version`, `internal/recipe`, and `internal/config`. Listing these explicitly prevents accidental `cmd/` imports.

### Out of Scope

- The 10-second timeout adequacy (correctness concern, not structural)
- Bounded concurrency strategy for parallel tool checks (implementation detail)
- Whether `ResolveWithinBoundary` returning `*VersionInfo` when the checker only needs a version string is wasteful (pragmatic concern)
