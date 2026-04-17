# Lead: Recipe placement options

## Findings

### Provider chain and lookup order

The CLI builds its provider chain in `cmd/tsuku/main.go` (lines 122–153):

1. **Local** (`SourceLocal`): `$TSUKU_HOME/recipes/`, highest priority, set by `cfg.RecipesDir`
2. **Embedded** (`SourceEmbedded`): Go `embed.FS` compiled into the binary, `internal/recipe/recipes/*.toml`
3. **Central registry** (`SourceRegistry`): HTTP fetch of `recipes/{letter}/{name}.toml` from `github.com/tsukumogami/tsuku/main`, with a 24-hour TTL disk cache at `$TSUKU_HOME/registry/`
4. **Distributed** (lowest priority): user-configured third-party repos, added at startup from `userconfig.Registries`

The `Loader.resolveFromChain` method walks this list in order, returning on the first hit. The `satisfies` fallback index (for ecosystem package-name aliases) is also consulted if a direct name lookup misses. There is no separate "curated" slot in the chain — embedded and registry are both treated as the "central" source (`SourceCentral`) for update/outdated/verify source-tracking purposes.

### What embedded recipes actually are

The 19 files in `internal/recipe/recipes/` are build toolchain dependencies: `go`, `rust`, `python-standalone`, `nodejs`, `ruby`, `zig`, `cmake`, `ninja`, `meson`, `make`, `openssl`, `zlib`, `libyaml`, `gcc-libs`, `patchelf`, `ca-certificates`, `bash`, `perl`, `pkg-config`. None of these are developer end-user tools; they exist because action executors (`cargo_build`, `go_build`, `meson_build`, etc.) validate their dependencies against `RequireEmbedded=true` to ensure they work without network access. Adding a tool recipe here makes it available even before any cache is populated, but costs binary size and requires a new release for every update.

### What the external registry contains

The `recipes/` directory has 1,405 TOML files across 26 letter subdirectories. Of these:
- **1,221** carry `tier = 0` and `llm_validation = "skipped"`, the fingerprint of pipeline-generated output.
- **184** have neither field — these are handcrafted recipes. Examples: `gh`, `minikube`, `mise`, `sqlite`, `staticcheck`, `stern`, `skaffold`, `syft`, `maturin`, `miller`, `scaleway-cli`.

So a handcrafted tier already exists implicitly: the absence of `tier` and `llm_validation` fields de facto marks a recipe as curated. The distinction is invisible in the file system layout (all live in `recipes/{letter}/`) and there is no CI path that treats these differently from generated ones.

### The `tier` field in MetadataSection

`types.go` line 163 shows `Tier int` in `MetadataSection` with the comment "Installation tier: 1=binary, 2=package manager, 3=nix". This field was originally meant to classify the *installation method*, not recipe quality. Pipeline-generated recipes populate it with `0` (the zero value, probably as a sentinel for "auto-generated" or "unclassified"). Handcrafted recipes omit it entirely. There are only 2 recipes in the registry with `tier = 3` (both nix-based). The field is not consumed by any install or display logic observed in the code — it is a metadata label with no runtime effect.

### How caching works

The registry provider uses an `HTTPStore` with a `DiskCache` at `$TSUKU_HOME/registry/{letter}/{name}.toml` with:
- 24-hour TTL (fresh reads from disk, no network)
- 7-day stale-if-error window
- 50 MB max cache size with LRU eviction

This means all `recipes/` content — handcrafted or generated — is served over the same HTTP fetch-and-cache path. A user on a clean machine who requests `gh` will fetch it from GitHub raw content, cache it locally, and consult it for up to 24 hours without a round-trip. Embedded recipes bypass this entirely; they are always available without network.

### CI treatment

The `recipe-validation-core.yml` workflow collects `internal/recipe/recipes/*.toml` and `recipes/*/*.toml` together, applies the same sandbox test matrix (11 platform combinations), and uses the same pass/fail logic. There is no separate CI tier or gate for handcrafted vs generated recipes.

### Placement option analysis

**Option A — Embedded (`internal/recipe/recipes/`)**

- Pro: always available offline, no TTL staleness, zero network dependency for dependency graph resolution
- Pro: already used for build deps; pattern is established
- Con: every recipe update requires a binary release — unacceptable for frequently-updated tools like `gh`, `kubectl`, `terraform`
- Con: binary size grows proportionally; with ~20 embedded recipes today the cost is small, but scaling to 50–100 curated tools is non-trivial
- Con: no mechanism to mark a recipe "curated" at the file level; only implied by location
- Verdict: only appropriate for build-time action dependencies, not end-user tool recipes

**Option B — Regular `recipes/{letter}/` (status quo)**

- Pro: zero infrastructure change; handcrafted recipes already live here and work
- Pro: same CI, same update pipeline, same TTL cache
- Con: no signal in the file system or TOML that a recipe is handcrafted — operators can't distinguish
- Con: no separate CI gate means a low-quality generated recipe can sit next to a carefully-written handcrafted one with no differentiation
- Verdict: workable today but lacks explicit identity, making it harder to enforce quality contracts over time

**Option C — Dedicated directory (`recipes/core/` or `recipes/curated/`)**

- Pro: file system signal is unambiguous; `ls recipes/core/` gives the curated set
- Pro: CI can add a separate job (faster lint, extra validation, mandatory golden files) targeting only `recipes/core/*.toml`
- Pro: future tooling (registry manifest, `tsuku recipes --curated`) has a concrete source of truth
- Con: the registry fetch path in `registry.go` currently constructs URLs as `{BaseURL}/recipes/{letter}/{name}.toml`; a flat `recipes/core/` would require a new layout variant (either the existing `grouped` layout logic is modified, or a new `CoreProvider` is added to the chain between embedded and central)
- Con: breaking alphabetical grouping; the provider would need to know to try `recipes/core/gh.toml` before `recipes/g/gh.toml`
- Verdict: cleanest long-term signal but requires meaningful provider and URL construction changes

**Option D — Metadata flag in existing `recipes/` (`curated = true` or `tier = 0` reassignment)**

- Pro: zero structural change; adding a field to existing TOML is backward-compatible
- Pro: CI can gate on the field (run stricter validation only when `curated = true`)
- Pro: the `tier` field already has in-code support (though unused at runtime); repurposing `tier` is tempting but its comment says "1=binary, 2=package manager, 3=nix", meaning it describes install *method*, not quality
- Con: discoverability is poor — users can't browse curated recipes without tooling support
- Con: the field is advisory; nothing enforces it for new handcrafted recipes unless CI blocks PRs that lack it
- Verdict: lowest friction, best suited as an interim measure or complement to another approach

### What distinguishes a recipe that warrants curation

Based on the existing 184 handcrafted recipes, these patterns recur:
- Tools with non-trivial multi-OS/multi-arch logic (platform-specific archive formats, checksum verification, binary aliasing)
- Tools with cross-ecosystem satisfies mappings (e.g., `homebrew = ["sqlite3"]`)
- Tools that are themselves dependency providers for other recipes (e.g., `rust`, `go`, `nodejs`)
- High-traffic tools where a broken recipe has broad user impact (`gh`, `kubectl`, `terraform`, `minikube`)
- Tools whose official release artifacts do not follow the patterns the LLM pipeline handles well (fossil archives, non-GitHub hosting, installer scripts)

## Implications

1. The "embedded" slot is a sealed contract for build-time action deps, not a general curated store. Expanding it for user-facing tools would introduce release coupling that would slow iteration on popular recipes.

2. The implicit handcrafted tier (recipes without `tier` and `llm_validation`) already functions but is undiscoverable and unenforced. Any explicit approach needs to preserve backward compatibility with these 184 files.

3. A metadata flag (`curated = true`) is the lowest-cost way to formalize the distinction within the existing `recipes/` layout. It can be adopted incrementally and enforced by CI once coverage is high enough.

4. A dedicated directory (`recipes/core/`) provides the strongest structural signal and enables separate CI gates, but requires changes to the URL construction in `registry.go` and a new provider priority slot, since the loader today has no way to distinguish `recipes/core/gh.toml` from `recipes/g/gh.toml` — they'd both be central registry content.

5. The `tier` field should not be repurposed for recipe quality; its meaning is already defined as installation method. A separate `curated` boolean (or a `quality` string) would avoid conflation.

## Surprises

1. The `llm_validation = "skipped"` field appears in 1,218 of 1,221 generated recipes — it is a de facto provenance marker, not an opt-out flag. This means the absence of `llm_validation` is already a reliable signal for handcrafted status, though it was never designed to carry that semantic.

2. `tier = 0` appears in 1,219 recipes. Given the field's documented meaning (1=binary, 2=package manager, 3=nix), `0` is clearly the default zero value emitted by the pipeline, not a meaningful tier designation. The field is present in `MetadataSection` but has no observable runtime effect anywhere in the codebase.

3. The `SourceCentral` constant maps both `SourceRegistry` and `SourceEmbedded` to the same user-facing string. This means if a tool had both an embedded recipe and a central registry recipe, update/verify operations would prefer the registry version (as coded in `providersForSource`), and the embedded version acts as a guaranteed offline fallback. This is the exact behavior that a "always-available curated" set would need — but it currently only works for build deps because only those are embedded.

4. The CI workflow at `recipe-validation-core.yml` line 59 explicitly loops over both `internal/recipe/recipes/*.toml` and `recipes/*/*.toml` in the same test matrix. There is no lighter or heavier CI path for handcrafted recipes today.

## Open Questions

1. Should the curated signal be a boolean `curated = true` field, or a richer value like `quality = "curated"` that could later support intermediate levels (e.g., `quality = "verified"`)?

2. If a metadata flag is used, should the existing 184 handcrafted recipes be retroactively tagged in a single PR, or should tagging be opportunistic (on next edit)? Retroactive tagging is a high-noise diff but gives CI immediate enforcement coverage.

3. For high-traffic tools that need offline reliability (e.g., `gh`, `terraform`, `kubectl`), is the embedded slot the right answer despite the release coupling? Or is the 7-day stale-if-error window on the central registry cache sufficient?

4. If a dedicated `recipes/core/` directory is chosen, how should the provider chain order it? Options: (a) insert a new `SourceCurated` provider between embedded and central, giving core recipes higher priority than generated ones; (b) keep it as a subset of the central registry but with a separate index entry; (c) treat it as a separate registry URL entirely.

5. What CI gate should curated recipes satisfy that generated ones do not? Candidates: mandatory golden files for all supported platforms, full sandbox test on every PR (not just nightly), stricter hardcoded-version detection, required `homepage` and `description`.

## Summary (3 sentences)

The embedded slot (`internal/recipe/recipes/`) is a sealed contract for build-time action dependencies and is unsuitable for end-user tool recipes because every update would require a binary release; the right placement for curated recipes is in `recipes/` where the existing 184 handcrafted recipes already live. A metadata flag (`curated = true`) is the lowest-friction way to formalize the distinction within the current layout, since `llm_validation = "skipped"` and `tier = 0` already function as reliable provenance markers but were never designed to carry that semantic. A dedicated subdirectory (`recipes/core/`) would provide a stronger structural signal and enable separate CI gates, but requires changes to the URL construction and provider chain, making it a medium-term option rather than an immediate one.
