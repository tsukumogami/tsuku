# Lead: How does tsuku's existing version provider system relate to update checking?

## Findings

### 1. Version Provider Architecture (Highly Reusable)

Tsuku has a mature, pluggable version provider system in `internal/version/`. The core interfaces are:

- **`VersionResolver`** (`provider.go:10-21`): Minimal interface with `ResolveLatest(ctx)` and `ResolveVersion(ctx, version)`. Every provider implements this.
- **`VersionLister`** (`provider.go:28-34`): Extends `VersionResolver` with `ListVersions(ctx)` for providers that can enumerate all versions.
- **`ProviderFactory`** (`provider_factory.go:48-104`): Strategy-pattern factory that routes recipes to the correct provider. 20+ strategies registered, covering GitHub, PyPI, npm, crates.io, RubyGems, Go proxy, Homebrew, Cask, Tap, nixpkgs, MetaCPAN, Fossil, and custom sources.

This entire system is **directly reusable** for auto-update checks. The `ResolveLatest()` method on any provider already answers "what is the newest version?" -- which is the fundamental question an update checker needs to answer.

### 2. Version Comparison Infrastructure (Fully Reusable)

`internal/version/version_utils.go` provides:

- `CompareVersions(v1, v2)` -- handles semver, calver, Go toolchain, custom formats, and prerelease ordering.
- `normalizeVersion()` -- strips v-prefixes, handles `go1.x`, `Release_X_Y_Z`, `kustomize/v5.7.1` formats.
- `splitPrerelease()` -- separates core version from prerelease identifiers.

`internal/version/version_sort.go` provides `SortVersionsDescending()`.

This is fully reusable for determining whether an installed version is behind the latest.

### 3. Existing Version List Caching (Partially Reusable)

`internal/version/cache.go` implements `CachedVersionLister`:

- File-based cache in `$TSUKU_HOME/cache/versions/`
- Configurable TTL
- SHA256-based cache keys from source descriptions
- Atomic writes
- `Refresh()` method to bypass cache
- **Critically: `ResolveLatest` and `ResolveVersion` are NOT cached** (lines 87-94 delegate directly to underlying). Only `ListVersions` is cached.

This means auto-update checks calling `ResolveLatest()` would hit the network every time. The cache infrastructure exists but doesn't cover the resolution path. A new caching layer for "latest version" results would need to be built, or `ResolveLatest` would need to be wrapped similarly.

### 4. The `outdated` Command (Significant Gaps)

`cmd/tsuku/outdated.go` is the closest existing feature to update checking. Current behavior:

- Iterates all installed tools
- Loads each tool's recipe via `loadRecipeForTool()` (source-aware loading)
- **Only checks GitHub-based tools** (lines 77-88 scan for `github_archive`/`github_file` actions, skip everything else)
- Calls `res.ResolveGitHub(ctx, repo)` directly instead of using `ProviderFactory`
- Does simple string inequality (`latest.Version != tool.Version`) with no semantic comparison
- No caching at all -- every run makes network requests for every installed tool

This means PyPI, npm, crates.io, RubyGems, Go, Homebrew, and custom-source tools are **invisible** to the outdated command. This is a significant gap that the auto-update system would need to fix by using the `ProviderFactory` properly.

### 5. The `update` Command (Delegates to Install)

`cmd/tsuku/update.go` is thin: it verifies the tool is installed, then calls `runInstallWithTelemetry()` with the tool name and no version constraint (meaning "latest"). There's:

- No version channel awareness
- No rollback on failure
- No caching of update check results
- A `--dry-run` flag that calls `runDryRun()`
- Source-directed recipe loading (distributed registries work)

### 6. State Model and Pinning (Foundation Exists, Needs Extension)

`internal/install/state.go` defines the state model:

- **`ToolState.ActiveVersion`**: Currently symlinked version
- **`ToolState.Versions`**: Map of all installed versions (multi-version support already exists)
- **`VersionState.Requested`**: Records what the user originally asked for (`"17"`, `"@lts"`, `""`)

The `Requested` field is interesting -- it already captures user intent. However:

- No fields exist for **version channel pinning** (major/minor/patch ceiling)
- No fields for **last update check timestamp** or **cached latest version**
- No fields for **update policy** (auto, notify, disabled)
- No fields for **failed update tracking** (for deferred error reporting)

### 7. User Configuration (Needs New Section)

`internal/userconfig/userconfig.go` manages `$TSUKU_HOME/config.toml`. Currently has sections for telemetry, LLM, secrets, auto-install mode, and registries. There is:

- No `[updates]` section for global update preferences
- No per-tool update configuration
- The `Get`/`Set` pattern with `AvailableKeys()` is extensible -- adding new config keys is straightforward

### 8. Multi-Version Support (Helps Rollback)

The state model already supports multiple installed versions per tool (`ToolState.Versions` map). The install manager has atomic staging-based installation with rollback on symlink failure (`internal/install/manager.go:117-150`). This multi-version infrastructure provides a natural rollback path: keep the previous version installed, only switch `ActiveVersion` after successful update, and switch back on failure.

### 9. No Background/Async Infrastructure

There is no existing infrastructure for:

- Background update checks
- Scheduled tasks
- Inter-process notification (e.g., "an update check found a new version, show notification on next command")
- File-based signaling between processes

The shim system (`internal/shim/`) exists for PATH delegation but doesn't do update checking.

## Implications

1. **The version provider system is the right foundation for update checking.** The `ProviderFactory.ProviderFromRecipe()` method already routes any recipe to the correct version source. Auto-update should use this, not the hard-coded GitHub path that `outdated` uses today.

2. **The `outdated` command should be rewritten** to use `ProviderFactory` before or as part of auto-update work. Currently it only covers GitHub tools, which means it misses a large portion of the recipe ecosystem.

3. **A new "latest version" cache is needed.** The existing `CachedVersionLister` caches version lists but not `ResolveLatest` results. Auto-update checks need a separate, lightweight cache that stores `{tool: latestVersion, checkedAt: timestamp}` with a configurable TTL. This could live in state.json or in a dedicated cache file.

4. **State model extension is modest but necessary.** Adding fields to `ToolState` for pinning preferences and update tracking is straightforward given the existing JSON-based state with migration support (see `migrateToMultiVersion`, `migrateSourceTracking`).

5. **Multi-version support simplifies rollback.** Because tsuku already keeps multiple versions installed and switches via `ActiveVersion`, rollback after a failed update can revert `ActiveVersion` and re-symlink the previous version without re-downloading.

6. **The biggest new work is the update check scheduling and notification layer.** No async or background infrastructure exists. This would need to be built from scratch -- likely as lightweight file-based signaling (write "update available" markers, read them on next command invocation).

## Surprises

1. **The `outdated` command ignores non-GitHub tools entirely.** Despite having 15+ version providers covering PyPI, npm, crates.io, etc., the outdated command hard-codes GitHub-only checking. This is a much larger gap than expected.

2. **`ResolveLatest` is not cached** even though `ListVersions` is. The cache infrastructure exists but was designed for version enumeration (the `versions` command), not for update checking.

3. **The `Requested` field on `VersionState`** already captures user intent ("17", "@lts", ""), which is a partial foundation for channel pinning. If a user installed with `tsuku install node@17`, the system could infer "pin to major version 17" from this field.

4. **Multi-version is already implemented.** The state model tracks all installed versions per tool and has an `ActiveVersion` pointer. This was built for side-by-side version coexistence, but it also means rollback infrastructure is largely in place.

## Open Questions

1. **Should "latest version" cache live in state.json or a separate file?** State.json is atomically written with file locking. Adding per-tool cache timestamps there avoids a new file, but it means every tool invocation (via shim) could trigger a state.json write when updating the "last checked" timestamp. A separate cache file might have less contention.

2. **How should channel pinning interact with `Requested`?** If a user ran `tsuku install node@17`, should the system automatically interpret this as "pin to major 17, allow minor/patch updates"? Or should pinning be an explicit separate concept?

3. **What is the desired behavior when the outdated command is fixed to use ProviderFactory?** Should this be done as prerequisite work, or can it be deferred and the auto-update system directly use ProviderFactory, with outdated remaining GitHub-only until a separate fix?

4. **Should update checks happen inline (during command execution) or via a background process?** Inline is simpler but adds latency. Background avoids latency but needs process management. Claude Code uses background checks with file-based signaling -- is that the target model?

## Summary

Tsuku's version provider system (15+ providers, factory pattern, version comparison, and version list caching) provides a strong foundation for auto-update checks -- the core "what's the latest version?" question is already answered by `ProviderFactory.ProviderFromRecipe()` combined with `ResolveLatest()`. The main gaps are: (a) the `outdated` command bypasses this infrastructure and only checks GitHub tools, (b) `ResolveLatest` results are not cached (only `ListVersions` is), and (c) the state model and user config have no fields for pinning, update frequency, or deferred failure reporting. The biggest open question is whether update checks should happen inline during command execution or via a background/file-signaling mechanism, since no async infrastructure exists today.
