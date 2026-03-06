# Phase 3 Research: Deprecation Warning UX

## Questions Investigated

1. How does the CLI currently surface warnings?
2. Does `--quiet` suppress warnings?
3. Does `--json` output have a warnings field?
4. How does `buildinfo.Version()` work? Can we compare it against a semver `min_cli_version`?
5. Is there any existing once-per-session warning mechanism?
6. Where would the warning be triggered?
7. How should multi-registry warnings work?

## Findings

### 1. Warning Conventions

The codebase uses two distinct warning patterns:

**Direct `fmt.Fprintf(os.Stderr, "Warning: ...")`** - The dominant pattern. Used in ~40 locations across the codebase. Examples:
- `internal/registry/cached_registry.go:156` - stale cache fallback
- `internal/registry/cache_manager.go:230` - cache near capacity
- `internal/registry/manifest.go:133` - manifest cache write failure
- `internal/config/config.go` - invalid config values (12+ occurrences)
- `cmd/tsuku/create.go` - recipe creation warnings
- Many action implementations (`download.go`, `cargo_build.go`, etc.)

**`fmt.Printf("Warning: ...")`** (stdout) - Used in a few places:
- `internal/recipe/loader.go:268,274` - local recipe shadows embedded/registry
- Various action implementations during installation output

**`slog.Warn()`** - Almost unused. Only one occurrence in `internal/llm/addon/manager.go:219`. The structured logging system exists (`internal/log/handler.go`) but warnings overwhelmingly bypass it.

**Key observation:** There is no centralized `printWarning()` helper. The `printInfo()`/`printInfof()` helpers in `cmd/tsuku/helpers.go` exist for informational output (respecting `--quiet`), but there is no equivalent for warnings. Warnings are ad-hoc `fmt.Fprintf` calls scattered throughout the codebase.

### 2. `--quiet` Flag and Warning Suppression

The `--quiet` flag (`cmd/tsuku/main.go:49`) sets `quietFlag = true` and configures the slog logger to `slog.LevelError` (line 159).

However, since nearly all warnings use `fmt.Fprintf(os.Stderr, "Warning: ...")` directly instead of `slog.Warn()`, **`--quiet` does NOT suppress most warnings**. It only suppresses output through `printInfo()`/`printInfof()` (which check `quietFlag`) and slog-based output.

The stale cache warning, cache capacity warning, config warnings, and action warnings all fire regardless of `--quiet`. This is a pre-existing inconsistency, not something introduced by the deprecation design.

**Implication for deprecation warnings:** If we add them via `fmt.Fprintf(os.Stderr)` they'd match the existing convention. But ideally the deprecation warning should be suppressible with `--quiet`. A `printWarning()` helper that checks `quietFlag` would be the right approach, and could later be adopted by existing warnings too.

### 3. `--json` Output and Warnings

`--json` is a **per-command flag**, not a global flag. It's registered on individual commands: `install`, `validate`, `search`, `list`, `info`, `versions`, `outdated`, `plan show`, `config`, `cache info`, `check-deps`, `verify-deps`.

No JSON output structure currently includes a `"warnings"` field. The `installError` struct (`cmd/tsuku/install.go:334-341`) has: `status`, `category`, `subcategory`, `message`, `missing_recipes`, `exit_code`. No warnings array.

Since `--json` is not a global flag, deprecation warnings written to stderr would still appear alongside JSON stdout output. This is actually fine for machine consumers -- they parse stdout JSON and can ignore stderr. But it's not clean.

**Implication:** Adding a `"warnings"` array to JSON output structures would be ideal for machine consumers. Since `--json` is per-command, this would need to be added to each command's JSON struct. Alternatively, deprecation warnings could just go to stderr (consistent with current behavior) and JSON consumers could ignore them.

### 4. `buildinfo.Version()` and Semver Comparison

`buildinfo.Version()` (`internal/buildinfo/version.go`) returns:
- **Release builds** (goreleaser): Injected version string, e.g., `"v0.1.0"` (semver with `v` prefix)
- **Dev builds with VCS**: `"dev-<12-char-hash>"` or `"dev-<hash>-dirty"`
- **Dev builds without VCS**: `"dev"`
- **Broken builds**: `"unknown"`
- **go install from tagged release**: Version from `info.Main.Version`

There is **no semver parsing or comparison function** anywhere in `internal/buildinfo/`. The package only provides `Version()` and the internal `devVersion()` helper.

**Implication for `min_cli_version`:** Comparing `buildinfo.Version()` against a semver string like `"0.5.0"` requires:
1. A semver parsing library or manual parser (strip `v` prefix, split on `.`, compare integers)
2. Handling non-semver versions: `"dev-*"`, `"dev"`, `"unknown"` should either skip the check or always warn
3. The `v` prefix needs to be handled (goreleaser injects `v0.1.0`, but `min_cli_version` in the manifest might omit it)

Go's `semver` package from `golang.org/x/mod/semver` would work, but adding a dependency for this is a decision to weigh. A minimal hand-rolled parser (just major.minor.patch comparison) might suffice since tsuku doesn't use pre-release semver tags for releases.

Dev builds should probably always pass the version check (assume latest) or always warn (assume nothing). Skipping the check for dev builds seems most practical -- developers running from source are likely on the latest code.

### 5. Once-per-Session Warning Mechanism

There is **no existing once-per-session warning mechanism** in the CLI. `sync.Once` is used in:
- `internal/recipe/loader.go` - `satisfiesOnce` for lazy satisfies index building
- `internal/secrets/secrets.go` - (not related to warnings)
- `internal/llm/` - protobuf generated code

The Loader has no flag or mechanism to track "already warned this session." The `CachedRegistry` stale warning (`cached_registry.go:156`) fires on each stale recipe access -- if you install a tool with 3 stale dependencies, you'd get 3 warnings.

**Implication:** For deprecation warnings, a once-per-session approach makes sense to avoid spamming. The natural place for this would be either:
- A `sync.Once` on the Loader or a new WarningCollector
- A flag on the registry/manifest fetch path that tracks "already warned about deprecation"

### 6. Where Should the Warning Be Triggered?

The manifest is fetched in two places:
- `internal/registry/manifest.go:77` - `FetchManifest()` fetches and caches it
- `cmd/tsuku/update_registry.go` - the `update-registry` command calls `FetchManifest`

The manifest is currently fetched on-demand by `update-registry`. It's also read from cache by the Loader's `buildSatisfiesIndex()` via `GetCachedManifest()`.

**Best trigger points:**

**Option A: During manifest fetch** (`FetchManifest` or a wrapper). This is the natural place since the manifest already has `schema_version`. After parsing the manifest, check for a `deprecation` object and emit a warning. Pros: single trigger point, fires during `update-registry`. Cons: not all commands fetch the manifest.

**Option B: During recipe load** (in the Loader). Check the deprecation state when the Loader initializes or on first recipe load. Pros: covers all commands that use recipes. Cons: requires manifest access at Loader init time.

**Option C: At CLI startup** (in `main.go init()`). Check cached manifest deprecation state during initialization. Pros: covers every command. Cons: adds latency to every command, even `--help`.

**Recommended:** Option A + B hybrid. The deprecation info lives in the manifest. When `FetchManifest()` returns a manifest with a `deprecation` object, it stores it. The Loader (or a new middleware) checks for cached deprecation state on first recipe access and emits the warning once via `sync.Once`. This ensures:
- `update-registry` surfaces warnings immediately
- Any recipe-using command surfaces the warning from cached state
- Commands like `--help` and `--version` don't pay the cost

### 7. Multi-Registry Warnings

The current system **does not support multiple registries**. There is a single `Registry` in `internal/registry/`, a single `RegistryDir` in config (`$TSUKU_HOME/registry`), and a single manifest URL (`tsuku.dev/recipes.json` or `TSUKU_MANIFEST_URL` override).

The Loader (`internal/recipe/loader.go`) has a single `registry *registry.Registry` field. There is no registry list, no registry priority chain, and no mechanism for multiple registry sources beyond the embedded/local/registry three-tier system (which is source type priority, not multiple remote registries).

**Implication:** Multi-registry deprecation warnings are not applicable to the current architecture. If multi-registry support is added later, the deprecation system would need to track which registry each warning came from. But for now, this is a non-concern -- there's exactly one registry, so at most one deprecation notice.

## Implications for Design

1. **Add a `printWarning()` helper** to `cmd/tsuku/helpers.go` that writes `"Warning: ..."` to stderr and respects `quietFlag`. This benefits both deprecation warnings and existing ad-hoc warnings over time.

2. **Semver comparison needs implementation.** Either bring in `golang.org/x/mod/semver` or write a minimal parser. Dev builds (`dev-*`, `dev`, `unknown`) should skip the `min_cli_version` check -- they're assumed to be current.

3. **Use `sync.Once` for per-session dedup.** The deprecation warning should fire at most once per CLI invocation, even if multiple recipe loads touch the manifest.

4. **Trigger on manifest read, not CLI startup.** Check cached deprecation state when the Loader first needs the manifest (via `buildSatisfiesIndex` or a new check path). This avoids adding latency to commands that don't touch recipes.

5. **JSON output: stderr is sufficient for now.** No existing JSON structures have a `warnings` field. Deprecation warnings going to stderr is consistent with all other warnings. A `warnings` array in JSON output could be added later as an enhancement.

6. **Multi-registry is N/A.** Single registry today, no dedup needed.

## Surprises

1. **`--quiet` doesn't suppress most warnings.** The vast majority of warnings bypass both `quietFlag` and `slog`, going directly to stderr via `fmt.Fprintf`. This is a pre-existing inconsistency. A `printWarning()` helper would start to fix this, but full cleanup is out of scope for the deprecation feature.

2. **`slog.Warn()` is essentially unused.** Despite having a full structured logging system with level-based filtering, only one call to `slog.Warn` exists in the entire codebase. All other warnings use raw `fmt.Fprintf`.

3. **No semver tooling exists.** `buildinfo.Version()` returns a string and nothing in the codebase parses or compares version strings. The `min_cli_version` feature will be the first consumer of version comparison logic.

4. **Warnings go to both stdout and stderr.** The `loader.go` warnings use `fmt.Printf` (stdout) while registry warnings use `fmt.Fprintf(os.Stderr)`. This inconsistency means some warnings could corrupt piped output. New warnings should always go to stderr.

## Summary

The CLI has no centralized warning system -- warnings are ad-hoc `fmt.Fprintf` calls to stderr (and sometimes stdout) that bypass both `--quiet` and `slog`. Deprecation warnings should introduce a `printWarning()` helper that respects `--quiet`, fire once per session via `sync.Once` during first manifest/recipe access, and go to stderr. Semver comparison is needed for `min_cli_version` but doesn't exist yet, and dev builds should skip the check. Multi-registry is not a concern since the architecture supports exactly one registry today.
