---
focus: maintainer
issue: 7
blocking_count: 2
advisory_count: 4
---

# Maintainer Review: Issue 7 (Distributed Sources in Install Flow)

## Overall Assessment

The separation between `install.go` (routing) and `install_distributed.go` (distributed-specific logic) is clean. A new developer reading `install.go` can follow the branch at line 188 -- "if it has a slash, it's distributed" -- and jump into the dedicated file for the details. The `distributedInstallArgs` struct and `parseDistributedName` function are well-documented with format examples. Test coverage is solid for the pure functions.

## Blocking

**1. Double fetch from the distributed source is invisible and fragile**
`cmd/tsuku/install.go:220-223` -- `fetchRecipeBytes` and `loader.GetWithContext` both hit the distributed provider for the same recipe in the same install. The first call fetches raw bytes for hashing; the second fetches, parses, and caches the recipe. The next developer will see `fetchRecipeBytes` and assume it's the only fetch, or will see `GetWithContext` and assume it covers everything. If the source is rate-limited or the recipe is large, this doubles the cost with no visible reason.

Worse, the two calls can return different content if the upstream recipe changes between them (unlikely but possible for a mutable branch-based source). The hash would then describe content different from what was actually installed.

Fix: Either have `GetWithContext` return the raw bytes alongside the parsed recipe (so one fetch serves both needs), or compute the hash from the bytes the loader already fetched internally. The current code works but the next person who touches either call won't realize the coupling.

**2. `recordDistributedSource` creates a fresh `install.New(cfg)` that doesn't share state with the install that just ran**
`cmd/tsuku/install_distributed.go:217-228` -- `recordDistributedSource` calls `config.DefaultConfig()` and `install.New(cfg)` to get a fresh `StateManager`, then calls `UpdateTool`. Meanwhile, the install that just completed via `runInstallWithTelemetry` used its own `StateManager` instance. Both do load-modify-save cycles with file locking, so they won't corrupt, but the next developer will think `recordDistributedSource` is updating the same in-memory state as the install. If someone adds an in-memory cache to `StateManager`, this becomes a real bug -- the two instances would diverge. The same pattern appears in `checkSourceCollision` (line 179).

Fix: Either pass the state manager through from the install flow, or add a comment at each `install.New(cfg)` call explaining why a fresh instance is intentional (file-lock-based atomicity, no shared in-memory state).

## Advisory

**3. `CacheRecipe` comment says "useful for testing" but it's now load-bearing for distributed installs**
`internal/recipe/loader.go:436` -- The comment reads "This is useful for testing or loading recipes from non-standard sources." In the distributed flow, `CacheRecipe` is called on line 230 of `install.go` to make the recipe findable under its bare name during dependency resolution. This is a critical step, not a test convenience. The next developer reading the loader might think `CacheRecipe` is safe to restrict or remove since it's "for testing."

**4. Implicit ordering contract: ensureDistributedSource must run before GetWithContext**
`cmd/tsuku/install.go:197-223` -- `ensureDistributedSource` dynamically adds a provider to the global `loader` via `addDistributedProvider`. If someone reorders these calls (e.g., tries to load the recipe first to check if it exists), `GetWithContext` will fail with "no provider registered" -- a confusing error with no hint about the missing setup step. A brief comment at line 197 noting "this must precede GetWithContext because it registers the provider" would prevent the trap.

**5. `versionConstraint` is set but never used in the distributed path**
`cmd/tsuku/install.go:190` -- `versionConstraint = dArgs.Version` is assigned but the distributed path never references `versionConstraint` again. It uses `resolveVersion` for the install and `distributedTelemetryTag()` (a hardcoded string) for telemetry. In the non-distributed path (line 278), `versionConstraint` is passed as the telemetry tag. The next developer might think the distributed path intentionally skips version-based telemetry, or might think it's a bug. A comment explaining why the distributed path uses an opaque tag would clarify.

**6. `isDistributedName` is dead code**
`cmd/tsuku/install_distributed.go:73` -- Defined and tested but never called outside its test. `parseDistributedName` already handles detection (returns nil for non-distributed names). The pragmatic reviewer already flagged this; confirming from the maintainability angle: the next developer will wonder if removing it breaks something. Delete it.
