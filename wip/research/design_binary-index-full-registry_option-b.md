# Option B: Manifest-Aware Interface

## How it works
Extend `index.Registry` with `ListAll(ctx context.Context) ([]string, error)` that reads recipe names from the locally-cached manifest (already downloaded by `refreshManifest()`). `Rebuild()` calls `ListAll()` for enumeration and `FetchRecipe(ctx, name)` for content when a recipe isn't in cache. The manifest is already a free side effect of `update-registry`.

## Manifest data available
`Manifest.Recipes []ManifestRecipe` — each entry has `Name string`. The full list of all 1,400 recipe names is available from the cached manifest without any additional network requests.

## registry.Registry methods available
- `GetCached(name) ([]byte, error)` — returns cached TOML if present
- `FetchRecipe(ctx, name) ([]byte, error)` — fetches individual TOML from registry URL
- `GetCachedManifest() (*Manifest, error)` — reads locally cached manifest without network
- `IsCached(name) bool` — check before fetching

## Current index.Registry interface
```go
type Registry interface {
    ListCached() ([]string, error)
    GetCached(name string) ([]byte, error)
}
```

## Interface changes required
```go
type Registry interface {
    ListAll(ctx context.Context) ([]string, error)  // NEW: enumerate from manifest
    GetCached(name string) ([]byte, error)           // unchanged
    FetchRecipe(ctx context.Context, name string) ([]byte, error)  // NEW: fetch uncached
}
```
`*registry.Registry` already implements `GetCached` and `FetchRecipe`. Add `ListAll()` reading from `GetCachedManifest()`.

## Adapter changes required
None. `*registry.Registry` passed directly into `Rebuild()` at the cmd/ layer.

## Pros
- Zero extra network requests for name enumeration (manifest already downloaded)
- On-demand fetch amortized: each recipe fetched once, then cached for future runs
- Fallback: `ListAll()` can fall back to `ListCached()` when manifest unavailable (offline)
- Minimal surface change: two new methods on an internal interface

## Cons
- First `update-registry` on a clean machine fetches all ~1,400 TOMLs (one-time cost)
- Rate limiting risk on first run (mitigated with bounded concurrency)
- `index.Registry` interface grows from 2 to 3 methods; test stubs need updating
