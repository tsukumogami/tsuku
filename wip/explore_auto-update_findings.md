# Exploration Findings: auto-update

## Round 1

### Key Insights

1. **Tsuku already has ~80% of the infrastructure.** The `Requested` field stores user intent, version providers do fuzzy resolution, multi-version directories enable rollback, atomic staging+rename handles safe installation, and five caching subsystems establish patterns to follow. The gap is a thin policy layer, not a ground-up build.

2. **The `Requested` field is stored but ignored during updates.** `tsuku install node@18` records `Requested: "18"`, but `tsuku update node` resolves to absolute latest (possibly Node 22). This is a latent bug that auto-update would amplify into silent channel-jumping. **Decision: fix as part of auto-update design, not separately.**

3. **Self-update on Unix is a solved problem.** Rename-in-place (download temp, rename old aside, rename new in) works because Unix processes hold inodes, not directory entries. The open question is whether tsuku should treat itself as a managed tool or keep a separate code path.

4. **Prefix-level pinning is the natural fit.** The number of version components the user typed ("" = latest, "20" = major, "1.29" = minor, "1.29.3" = exact) determines the update boundary. No new syntax needed.

5. **No CLI tool does configurable out-of-channel notifications.** Every tool either shows all updates or nothing. A `notifications.updates` config with `off`/`pinned`/`all` levels would be a differentiator.

6. **The `outdated` command only checks GitHub-sourced tools.** PyPI, npm, crates.io, RubyGems, and all other providers are invisible. Auto-update needs the `ProviderFactory` path.

7. **Rollback is nearly free.** Multi-version directories mean the old version stays on disk. `Activate()` switches symlinks back. The new subsystem needed is a file-based notice queue (`$TSUKU_HOME/notices/`) for deferred error reporting.

8. **Background goroutine + file-based cache is the recommended check model.** No async infrastructure exists in tsuku today, but a non-blocking goroutine that writes results to `$TSUKU_HOME/cache/update-check.json` fits existing patterns.

### Tensions

- **Unified vs. separate self-update**: Treating tsuku as its own managed tool is cleaner but creates bootstrap risk. A separate rename-in-place code path is more resilient but means two update mechanisms.
- **Implicit vs. explicit pinning**: Infer pin level from `Requested` component count (zero new fields, surprising for calver) or store as explicit `PinLevel` field (more fields, unambiguous).

### Gaps

- No background/async infrastructure in tsuku
- `ResolveLatest` isn't cached (only `ListVersions`)
- No deferred error reporting mechanism
- Bug #2103: existing `update` command doesn't switch active version when target is already installed
- `outdated` only checks GitHub tools

### Demand Signal

Demand not validated -- no external requests. Single-maintainer project, so this is expected. No evidence the feature was considered and rejected. Manual `update` and `outdated` commands show the maintainer already values the workflow; auto-update is a natural evolution.

### Natural Extensions (should be in scope or immediately follow)

1. **Channel-aware updates** -- fix the Requested field being ignored during update (designed together with auto-update)
2. **`tsuku update --all`** -- trivial once single-tool update is reliable
3. **Update outcome telemetry** -- small addition to existing telemetry
4. **Graceful offline degradation** -- required for cached checks to be useful

### Separate Initiatives (track independently)

- Pre-release channel opt-in
- Version range constraints in `.tsuku.toml`
- Pre/post-update hooks
- Security advisory integration
- Organization policy files

## Decision: Crystallize
