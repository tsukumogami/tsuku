# Lead: What should the CLI do at each mismatch severity level?

## Findings

### Current error handling patterns

The CLI has a layered error handling system built around typed errors and actionable suggestions:

1. **`registry.RegistryError`** (`internal/registry/errors.go`) -- Typed errors with `ErrorType` enum (Network, NotFound, Parsing, Validation, RateLimit, Timeout, DNS, Connection, TLS, CacheRead, CacheWrite, CacheTooStale, CacheStaleUsed). Each type has a `Suggestion()` method returning user-facing advice.

2. **`errmsg.Fprint`** (`internal/errmsg/errmsg.go`) -- Walks the error chain looking for `Suggester` interface implementors. Appends "Suggestion: ..." to error output. Used by `printError()` in `cmd/tsuku/helpers.go`.

3. **`classifyInstallError`** (`cmd/tsuku/install.go:306`) -- Maps errors to exit codes (3=RecipeNotFound, 5=Network, 6=InstallFailed, 8=DependencyFailed). Supports JSON output (`--json` flag) with category/subcategory strings.

4. **Warning pattern** -- Warnings are printed to stderr via `fmt.Fprintf(os.Stderr, "Warning: ...")`. Examples:
   - Stale cache fallback: `cached_registry.go:156` -- "Warning: Using cached recipe 'X' (last updated Y ago). Run 'tsuku update-registry' to refresh."
   - Cache write failure: `loader.go:299` -- "Warning: failed to cache recipe X: ..."
   - Shadow detection: `loader.go:269` -- "Warning: local recipe 'X' shadows embedded recipe"
   - Manifest fetch failure: `update_registry.go:232` -- "Warning: failed to fetch registry manifest: ..."
   - Cache size: `cache_manager.go:230` -- "Warning: Recipe cache is X% full..."

### The 4-tier resolution chain

Defined in `internal/recipe/loader.go`, method `GetWithContext` (line 87):

1. **In-memory cache** -- Previously loaded recipes (no version check possible here, already parsed)
2. **Local recipes** (`$TSUKU_HOME/recipes/*.toml`) -- Loaded via `loadLocalRecipe`, parsed with `toml.Unmarshal` + `validate()`. Parse errors are fatal (returned immediately). Missing files fall through.
3. **Embedded recipes** -- Frozen at build time in the binary via `EmbeddedRegistry`. Parse errors are fatal.
4. **Registry** (disk cache or remote) -- `fetchFromRegistry` checks disk cache first (`GetCached`), then fetches remotely (`FetchRecipe`). Cache write failures are warnings. Parse errors are fatal.

After all 4 tiers, a **satisfies fallback** (`lookupSatisfies`) checks if another recipe provides the requested name. This uses both embedded recipes and the cached manifest's satisfies entries.

### Schema version: completely unused

The `Manifest` struct in `internal/registry/manifest.go:28` declares `SchemaVersion string`, and `scripts/generate-registry.py` writes `"1.2.0"`. But `parseManifest()` (line 158) does a plain `json.Unmarshal` with no version check. The field is never read after parsing.

Individual recipe TOML files have no schema version field at all. The `Recipe` struct in the recipe package has no version field.

### Decision points where version checks could be inserted

**Point A: Manifest fetch/parse** (`registry/manifest.go:158`, `parseManifest`)
- Best place to check manifest schema version
- Called by both `FetchManifest` and `GetCachedManifest`
- Could compare `manifest.SchemaVersion` against a built-in compatibility range

**Point B: Recipe parse** (`recipe/loader.go:308`, `parseBytes`)
- Central point where all recipe bytes become structs
- TOML unknown-field handling is relevant: `BurntSushi/toml` silently ignores unknown fields by default
- A recipe format version field in the TOML (e.g., `format_version = "1"`) could be checked here

**Point C: Recipe validation** (`recipe/loader.go:664`, `validate`)
- Currently checks metadata.name, steps, verify
- Could add format version validation here

**Point D: Pre-install plan generation** (`cmd/tsuku/helpers.go:122`, `generateInstallPlan`)
- Higher-level check before committing to execution
- Could warn about schema mismatches before downloading anything

### Warning frequency considerations

Current warnings are **per-occurrence, unbounded** -- every stale cache hit produces a warning, every shadow produces a warning. There is no session tracking or rate limiting.

The `--quiet` flag (`main.go:49`) suppresses `printInfo` output but does NOT suppress `fmt.Fprintf(os.Stderr, ...)` warnings. Warnings always show.

For version mismatch warnings, the question is: how often does the CLI encounter a manifest or recipe? During `install`, exactly once per recipe (plus dependencies). During `recipes` or `search`, the manifest is checked once. A "warn once per session" approach would need state within the process lifetime, which the current architecture supports (the `Loader` is a singleton in `cmd/tsuku/helpers.go:19`).

### Stale manifest + new remote scenario

When `update-registry` runs, it calls `refreshManifest` (line 229) which fetches the remote manifest and caches it. If the cached manifest has schema_version "1.2.0" and the remote has "2.0.0", the current code would overwrite the cache with the new data and parse it. Since `parseManifest` does a simple `json.Unmarshal`, any new fields would be silently dropped, and any removed fields would be zero-valued. There is no mechanism to detect or handle this.

### How `CachedRegistry` handles staleness

`CachedRegistry.GetRecipe` (`cached_registry.go:92`) implements stale-if-error:
- Fresh cache: return immediately
- Expired cache + network success: refresh cache, return fresh
- Expired cache + network failure + within maxStale (7 days): return stale with warning
- Expired cache + network failure + beyond maxStale: return `ErrTypeCacheTooStale` error

This pattern is relevant because a version-incompatible cached manifest should NOT be used as a stale fallback -- it's worse than no manifest.

## Implications

1. **Schema version check belongs in `parseManifest`** -- This is the single chokepoint for all manifest parsing. A version check here would catch both cached and freshly-fetched manifests. The check should happen AFTER successful JSON parse but BEFORE returning the struct.

2. **Three severity levels make sense given the existing error patterns:**
   - **Compatible (minor bump, e.g., 1.2 -> 1.3)**: Log at INFO level (only shown with `--verbose`). New fields silently ignored by Go's `json.Unmarshal`. No action needed.
   - **Deprecated (approaching major bump, e.g., 1.x with deprecation signal)**: Warn on stderr (like the stale-cache pattern). Show once per CLI invocation using a flag on the Loader or a sync.Once. Include "Run 'tsuku update' to get the latest CLI version."
   - **Breaking (major bump, e.g., 1.x CLI reads 2.x manifest)**: Block with a clear error using the existing `RegistryError` type (could add `ErrTypeSchemaIncompatible`). Include suggestion: "This registry requires tsuku vX.Y or later. Run 'tsuku update' or visit tsuku.dev."

3. **Recipe TOML format versioning is separate from manifest versioning.** Recipes use TOML with unknown-field tolerance. A `format_version` field in recipes would be forward-looking but currently unnecessary since the TOML schema has been additive only.

4. **The stale fallback must be version-aware.** `CachedRegistry.handleStaleFallback` should NOT return a stale manifest whose schema version is incompatible, even if it's within the 7-day window. A version-incompatible stale manifest is worse than no manifest -- it could produce wrong satisfies mappings or missing fields.

5. **Warning deduplication per invocation is easy.** The `Loader` is a process-lifetime singleton. Adding a `versionWarned sync.Once` field would ensure a deprecation warning shows at most once per CLI run.

6. **The 4-tier chain does not need version checks at every tier.** Only the manifest (which is per-registry, not per-recipe) needs a schema version check. Individual recipe parsing already handles unknown fields gracefully through TOML's default behavior. If a recipe introduces a new required field, the `validate()` function catches it at parse time.

## Surprises

1. **No recipe-level format version exists.** Unlike the manifest, individual recipe TOML files have no version or format field. This means recipe format changes rely entirely on TOML's unknown-field tolerance and the `validate()` function. This works for additive changes but provides no mechanism for detecting incompatible recipe formats.

2. **Warnings bypass the quiet flag.** `fmt.Fprintf(os.Stderr, ...)` calls are not gated on `quietFlag`. This means schema version warnings would always display, which may be desirable for breaking changes but noisy for deprecation notices.

3. **The satisfies index silently uses whatever manifest is cached.** `buildSatisfiesIndex` (loader.go:370) reads the cached manifest but never checks its freshness or version. If the cached manifest has an incompatible schema, the satisfies index could silently produce wrong results (missing entries rather than errors, since new fields would be zero-valued).

4. **`BurntSushi/toml` silently ignores unknown TOML keys.** This is actually good for forward compatibility -- a newer recipe format with extra fields won't break older CLIs. But it also means a CLI can't detect that it's reading a recipe with features it doesn't understand.

## Open Questions

1. **Should the CLI embed its supported schema version range, or just the major version?** SemVer ranges (e.g., "supports 1.x") are more flexible but add complexity. A simple major-version check covers the critical case (breaking changes).

2. **How should distributed registries signal their schema version?** The manifest already has a `schema_version` field, so any registry that produces a manifest automatically signals its version. But should there be a separate lightweight endpoint (like `version.json`) that the CLI can check without downloading the full manifest?

3. **What happens to the embedded manifest?** Embedded recipes don't go through the manifest at all -- they're parsed directly from TOML. If recipe format versioning is added later, the embedded registry needs its own version check. Currently the embedded registry has no manifest.

4. **Should `--quiet` suppress deprecation warnings?** Current behavior: warnings always show. This is arguably correct for deprecation notices (users need to know). But it breaks the expectation that `--quiet` means "errors only."

5. **What is the upgrade path when a breaking change occurs?** The CLI currently has no self-update mechanism. If the manifest moves to schema 2.0 and the user's CLI only supports 1.x, the error message needs to point to a specific upgrade path (GitHub releases, reinstall instructions).

## Summary

The CLI has no schema version checking -- the `Manifest.SchemaVersion` field is populated by the generation script ("1.2.0") but never read by any CLI code path, and individual recipe TOML files have no format version at all. The best insertion point for version checks is `parseManifest()` in `internal/registry/manifest.go`, which is the single chokepoint for all manifest parsing and can leverage existing `RegistryError` types and the `Suggester` pattern for actionable error messages. The biggest open question is how the version check interacts with stale-if-error fallback -- a cached manifest that is version-incompatible should be treated as unusable rather than "better than nothing," which inverts the current staleness assumption.
