# Advocate: Dual-Manifest Endpoint

## Approach Description

During a schema migration, the registry serves both old and new manifest formats at different URL paths. The URL path encodes the schema version:

- `/v1/recipes.json` -- current format (schema 1.x)
- `/v2/recipes.json` -- future format (schema 2.x)

The CLI knows which versions it supports and tries the highest first. On 404, it falls back to the next lower version. Old CLIs that only know about `/v1/` continue working until the registry decommissions that path. Deprecation signals are embedded in the old endpoint's response (e.g., a `deprecated` field or HTTP headers), warning users to upgrade before the endpoint goes away.

The URL IS the version contract. No in-document schema validation needed.

## Investigation

### Current URL Construction

The manifest URL is resolved in `internal/registry/manifest.go` at `manifestURL()`:

```go
func (r *Registry) manifestURL() string {
    if envURL := os.Getenv(EnvManifestURL); envURL != "" {
        return envURL
    }
    if r.isLocal {
        return filepath.Join(r.BaseURL, "_site", "recipes.json")
    }
    return DefaultManifestURL
}
```

`DefaultManifestURL` is `"https://tsuku.dev/recipes.json"` -- a single, unversioned path. There's no version segment in the URL today.

The env override (`TSUKU_MANIFEST_URL`) already provides one escape hatch: users and CI can point at a custom URL. But it's a single URL, not a list. The dual-manifest approach would need the CLI to construct versioned URLs itself.

### Current Schema Version Handling

The `Manifest` struct has `SchemaVersion string` but it's never validated. `parseManifest()` does `json.Unmarshal` and returns whatever it gets. The generate script (`scripts/generate-registry.py`) sets `SCHEMA_VERSION = "1.2.0"` -- a semver string.

This is the exact gap the problem statement describes. The CLI parses whatever it gets with no compatibility check.

### Caching

`FetchManifest()` caches to `$TSUKU_HOME/registry/manifest.json` -- a single file. The dual-manifest approach would need per-version cache files (e.g., `manifest.v1.json`, `manifest.v2.json`) or the CLI only caches its preferred version.

### Discovery Registry Precedent

The discovery registry (`internal/discover/registry.go`) already validates `SchemaVersion`:

```go
if reg.SchemaVersion != 1 {
    return nil, fmt.Errorf("unsupported discovery registry schema version %d (expected 1)", reg.SchemaVersion)
}
```

This uses an integer version with strict equality -- the "proven pattern" referenced in the decision drivers. Notably, this is an in-document check, not a URL-path check.

### Local/Distributed Registry Implications

For local registries, `manifestURL()` constructs a filesystem path: `filepath.Join(r.BaseURL, "_site", "recipes.json")`. Dual-manifest would require the local registry to generate two files (`_site/v1/recipes.json` and `_site/v2/recipes.json`), or the generation script to produce both formats during the transition period.

For git-based distributed registries (the common static file hosting pattern), this means maintaining two files on disk. Git repos serve static files, so versioned paths are just directories. This works naturally.

### Recipe URL Construction

Recipe fetching (`recipeURL()`) uses a different pattern entirely -- `{BaseURL}/recipes/{letter}/{name}.toml`. The manifest and recipe endpoints are independent. Versioning the manifest doesn't require versioning recipe URLs.

## Strengths

- **No silent breakage on schema change.** Old CLIs fetch `/v1/`, which keeps serving the old format. New CLIs fetch `/v2/`. The 404 fallback means a CLI that supports v2 still works against a v1-only registry. This satisfies "no silent breakage" fully.

- **Registry independence.** Each registry controls when to start serving `/v2/` and when to retire `/v1/`. No coordination needed between registries or between registry and CLI release schedule. A distributed git-based registry can add `v2/recipes.json` to its repo whenever it's ready.

- **Cache safety by construction.** If the CLI caches per-version (e.g., `manifest.v1.json`), a stale v1 cache can't be misinterpreted as v2. The version is in the filename, not in the content. Currently the single `manifest.json` cache file has no version distinction.

- **Additive deployment.** Adding `/v2/` doesn't break anything. Old CLIs don't know about it, so they never request it. The new endpoint is invisible to old clients. This satisfies the additive deployment constraint.

- **Natural deprecation lifecycle.** The registry can add deprecation signals to `/v1/` (a `deprecated: true` field, or a `Sunset` HTTP header) before removing it. Old CLIs that don't understand the field ignore it gracefully (Go's `json.Unmarshal` skips unknown fields). New CLIs can parse it and warn. When `/v1/` is finally removed, old CLIs get a 404 with an actionable error.

- **`TSUKU_MANIFEST_URL` env var remains useful.** Users who override the URL can point at any version endpoint. The env var just needs to keep overriding the resolved URL, which it already does.

## Weaknesses

- **Double the manifest files during transitions.** The registry must generate and host two manifest files for the entire transition period. For static file hosts (Cloudflare Pages, GitHub Pages), this is just disk space and CI time. For dynamic servers, it's two codepaths. The `generate-registry.py` script would need to output both formats.

- **404 fallback adds latency.** If a CLI supports v2 but the registry only has v1, the CLI makes an HTTP request that returns 404, then falls back. This is one extra round-trip on the first fetch. Mitigated by caching (the 404 result can be cached with a short TTL so fallback isn't repeated). But it's still a real cost for new-CLI-on-old-registry scenarios.

- **URL pattern diverges from recipe URLs.** Recipe URLs use `{BaseURL}/recipes/{letter}/{name}.toml` with no version segment. Manifest URLs would use `{BaseURL}/v{N}/recipes.json`. This asymmetry might confuse third-party registry implementors.

- **Semver schema version doesn't map cleanly to integer URL paths.** The current schema version is "1.2.0" (semver). URL paths work best with major version integers (`/v1/`, `/v2/`). This forces a decision: do minor/patch bumps get new endpoints? Almost certainly not -- only breaking changes should. But then the semver in the document body and the integer in the URL are redundant signals of the same thing.

- **Local registries need directory restructuring.** Currently local registries serve from `_site/recipes.json`. Versioned paths mean `_site/v1/recipes.json`, which changes the directory layout. The `manifestURL()` function for local registries would need updating.

## Deal-Breaker Risks

- **Multiplicative complexity with many registries.** If tsuku supports multiple registries (the "distributed registries" in the problem statement), each registry independently controls its version support. The CLI must probe each registry separately for version support, potentially making N*M requests (N registries * M version attempts). This could be partially mitigated by caching version discovery per registry, but it adds real complexity. Not a deal-breaker on its own, but worth tracking.

- **No deal-breakers identified.** The approach has friction (404 fallback, dual generation, URL asymmetry) but nothing that fundamentally prevents implementation or violates the decision drivers.

## Implementation Complexity

### Files to Modify

1. **`internal/registry/manifest.go`** -- Change `DefaultManifestURL` from a single URL to a function that generates versioned URLs. Modify `manifestURL()` to return a list or accept a version parameter. Update `FetchManifest()` to implement fallback logic.

2. **`internal/registry/manifest.go`** -- Update cache file naming from `manifest.json` to `manifest.v{N}.json` (or keep single file but tag with version metadata).

3. **`scripts/generate-registry.py`** -- Generate output to versioned paths (`_site/v1/recipes.json`). During transitions, generate both v1 and v2.

4. **`cmd/tsuku/update_registry.go`** -- `refreshManifest()` may need to handle version negotiation.

5. **Website/CDN deployment config** -- Ensure versioned paths are deployed and accessible (Cloudflare Pages routing).

### New Infrastructure

- Version negotiation logic (try v2, fall back to v1)
- Per-version cache management
- Deprecation signal parsing and user warning display

### Estimated Scope

Medium. The core change is in `manifestURL()` and `FetchManifest()` -- roughly 50-100 lines of new logic. The generation script needs a new output mode. Cache management needs minor adjustments. No new packages needed. Most of the complexity is in edge cases (local registries, env override, stale cache from wrong version).

## Summary

Dual-Manifest Endpoint turns the version check into a URL routing problem, which is well-understood infrastructure. Old CLIs keep working because they keep hitting their known URL; new CLIs negotiate upward. The main costs are 404-fallback latency on first contact with a registry that hasn't upgraded yet, and the obligation for registries to generate two files during transitions. These are manageable trade-offs. The approach's strongest property is that it makes "registry independence" structural rather than behavioral -- each registry controls its own migration by simply choosing which paths to serve, with no protocol-level coordination required.
