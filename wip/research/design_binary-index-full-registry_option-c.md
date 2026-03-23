# Option C: Explicit Name List

## How it works
`update-registry` reads the manifest (already fetched by `refreshManifest()`), extracts all recipe names, and passes `[]string` to `rebuildBinaryIndex()`. `Rebuild()` receives the full name list directly — no `ListAll()` method needed on the interface. For content, `Rebuild()` uses `GetCached()` first, `FetchRecipe()` as fallback.

## Manifest data available
Same as Option B: `Manifest.Recipes[].Name` gives all recipe names from the cached manifest.

## Rebuild signature change
```go
// Current
func (idx *sqliteBinaryIndex) Rebuild(ctx context.Context, reg Registry, state StateReader) error

// Option C
func (idx *sqliteBinaryIndex) Rebuild(ctx context.Context, names []string, reg Registry, state StateReader) error
```
Or keep the current signature and add `ListAll()` to the interface (collapses into Option B).

## update-registry wiring change
`refreshManifest()` needs to return the manifest, and `rebuildBinaryIndex()` needs to accept the name list. Currently both discard the manifest after use.

## Fetch-vs-cache logic
Same as Option B: `GetCached()` → fallback to `FetchRecipe()` → cache result.

## Error handling for unfetchable recipes
Skip with `slog.Warn` (consistent with current malformed-TOML handling in Rebuild).

## Interface changes required
```go
type Registry interface {
    ListCached() ([]string, error)                                   // kept for fallback
    GetCached(name string) ([]byte, error)                          // unchanged
    FetchRecipe(ctx context.Context, name string) ([]byte, error)   // NEW
}
```
`ListAll()` moves to cmd/ layer (reads manifest, passes names directly).

## Pros
- `index.Registry` interface stays closer to current form (no `ListAll`)
- Manifest-reading logic stays in cmd/ where manifest is already handled
- Explicit: Rebuild gets the name list as a parameter, no hidden manifest dependency

## Cons
- `BinaryIndex.Rebuild()` signature changes — breaks the public interface
- `refreshManifest()` needs a return value (currently void-ish)
- Manifest coupling moves to cmd/ layer, which is fine but adds param threading
- Same rate limit exposure as Option B on first run
