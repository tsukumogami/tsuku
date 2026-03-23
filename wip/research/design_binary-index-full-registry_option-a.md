# Option A: Eager Seeding

## How it works

Before calling `rebuildBinaryIndex()`, `update-registry` reads the manifest (recipes.json), extracts all recipe names from the manifest's `recipes[]` array, and downloads any recipe TOML that isn't already in the local cache. Then `Rebuild()` calls `ListCached()` to retrieve cached recipes—no interface change needed.

Flow:
1. `refreshManifest()` fetches recipes.json and caches it
2. New: Extract recipe names from manifest.Recipes array
3. New: For each name, check if cached locally; if not, call `FetchRecipe()` to download
4. Existing: `Rebuild()` calls `reg.ListCached()` (no change)
5. `rebuildBinaryIndex()` proceeds normally

## Manifest contents (what we get for free)

The manifest provides **recipe metadata without requiring individual TOML downloads**:

```go
type Manifest struct {
	SchemaVersion int                `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	Deprecation   *DeprecationNotice `json:"deprecation,omitempty"`
	Recipes       []ManifestRecipe   `json:"recipes"`
}

type ManifestRecipe struct {
	Name                string              `json:"name"`
	Description         string              `json:"description"`
	Homepage            string              `json:"homepage"`
	Dependencies        []string            `json:"dependencies"`
	RuntimeDependencies []string            `json:"runtime_dependencies"`
	Satisfies           map[string][]string `json:"satisfies,omitempty"`
}
```

From the manifest alone, we get:
- Recipe name (e.g., "jq")
- Description and homepage
- Runtime dependencies
- Ecosystem name mappings (satisfies)

What the manifest **does not provide**:
- Binary names (e.g., which recipe provides the `jq` command)
- Installation steps
- Version providers
- Platform-specific details

Therefore, eager seeding trades **wider network cost for guaranteed cache coverage**: we must download all ~1,400 individual TOMLs to extract binary information for the index.

## Recipe fetch mechanism

The registry URL pattern is straightforward:
```
https://raw.githubusercontent.com/tsukumogami/tsuku/main/recipes/{first-letter}/{name}.toml
```

Method signature:
```go
// FetchRecipe fetches a recipe from the registry (remote URL or local directory).
// Returns the recipe content as bytes, or an error if not found.
func (r *Registry) FetchRecipe(ctx context.Context, name string) ([]byte, error)
```

For remote registries, it:
1. Constructs URL from recipe name's first letter
2. Makes HTTP GET request to raw GitHub or custom registry
3. Handles 404 (not found), 429 (rate limit), and other errors
4. Returns raw TOML bytes on success

For local registries (development/testing), it reads the TOML file directly from the filesystem.

## Registry size (recipe count)

Current state:
- **Total recipes**: 1,400 TOML files
- **Organized**: Bucketed by first letter (a/ through z/) for parallel downloads
- **Verification**: Confirmed by counting files in `/recipes/` directory

## Network cost estimate

**Total data to download for all recipes:**
- Total size: ~1.16 MB (1,157,648 bytes)
- Average per recipe: ~827 bytes
- Count: 1,400 recipes

**Sequential download**: 1,400 requests × ~100ms RTT (GitHub + DNS) = ~140 seconds (worst case, no concurrency)

**Parallel download** (by first letter, 26 parallel streams): ~6-10 seconds for network I/O

**Disk footprint after download**: ~1.2 MB (already cached, so no additional cost on subsequent builds)

## Interface changes required

**None.** Option A requires no changes to the Registry or BinaryIndex interfaces.

Current `Rebuild()` signature:
```go
func (idx *sqliteBinaryIndex) Rebuild(ctx context.Context, reg Registry, state StateReader) error
```

Required Registry interface (from binary_index.go):
```go
type Registry interface {
	ListCached() ([]string, error)
	GetCached(name string) ([]byte, error)
}
```

With Option A, `Rebuild()` continues to call `ListCached()` identically. The new seeding logic runs **before** `Rebuild()` is called, in the `update-registry` command itself.

## Pros

1. **No interface changes**: Lowest migration cost; Rebuild() remains unchanged
2. **Complete cache coverage**: After seeding, all registry recipes are guaranteed in cache
3. **Deterministic binary index**: No recipes are silently skipped due to missing cache entries
4. **Works with staleness detection**: Respects cache TTL; already-fresh recipes are not re-downloaded
5. **Concurrent downloads possible**: Can parallelize by first letter for fast seeding
6. **Simple implementation**: ~50 lines of code in `update-registry` command to seed from manifest
7. **User experience**: Single `tsuku update-registry` call handles both cache and index; no surprises

## Cons

1. **Always downloads all recipes**: No opt-out for users who only care about specific tools
2. **Network cost**: 1.2 MB every time full refresh is run (even if most recipes are already fresh)
3. **Rate limiting risk**: 1,400 sequential or parallel HTTP requests may hit GitHub rate limits (1,000 req/hr for unauthenticated)
4. **Slow on first run**: ~10s+ for parallel downloads on first setup (even if only one tool is needed)
5. **Wasted bandwidth**: Downloads recipes user may never install
6. **Backward compatibility**: Older CLI versions (before Option A) still have stale index problem; not retroactive

## Implementation notes

**Where to add seeding:**

In `cmd/tsuku/update_registry.go`, after `refreshManifest()` completes:

```go
// Seed cache from manifest before rebuild
if err := seedCacheFromManifest(ctx, cachedReg.Registry(), manifest); err != nil {
	// Log warning but continue; rebuild will use whatever is cached
	fmt.Fprintf(os.Stderr, "Warning: failed to seed cache from manifest: %v\n", err)
}
```

**Helper function signature:**

```go
func seedCacheFromManifest(ctx context.Context, reg *registry.Registry, manifest *registry.Manifest) error {
	// Extract recipe names from manifest
	// For each name, check if cached; if not, call FetchRecipe and cache
	// Can parallelize by first letter or run sequentially with rate limiting
	// Return error only on critical failures (network down); skip individual recipe failures
}
```

**Seeding strategy:**

- Use manifest `recipes[].Name` to get full list upfront
- Check each recipe against `reg.IsCached(name)` to skip already-cached ones
- For cache misses, call `reg.FetchRecipe(ctx, name)` and `reg.CacheRecipe(name, data)`
- Parallelize by grouping recipes by first letter (26 goroutines max) to respect GitHub rate limits
- Collect errors per-recipe and report summary; continue on individual failures
