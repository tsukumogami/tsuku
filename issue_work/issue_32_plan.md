# Issue 32 Implementation Plan

## Summary

Add support for fetching recipes from the external tsuku-registry GitHub repository, with bundled recipes as fallback and configurable registry URL.

## Registry Structure

The tsuku-registry (https://github.com/tsuku-dev/tsuku-registry) is organized as:
```
recipes/
├── a/
│   ├── actionlint.toml
│   ├── age.toml
│   └── ...
├── b/
│   └── ...
└── ...
```

Recipes are accessed via raw GitHub URLs:
`https://raw.githubusercontent.com/tsuku-dev/tsuku-registry/main/recipes/{first-letter}/{name}.toml`

## Approach

### Design Decisions

1. **Fetch on demand**: Don't download all recipes upfront. Fetch individual recipes when needed.
2. **Local cache**: Cache downloaded recipes in `~/.tsuku/registry/`
3. **Bundled fallback**: If network fails and no cache, fall back to bundled recipes
4. **Configurable URL**: Allow custom registry URL via environment variable or config

### Recipe Resolution Priority
1. Local cache (`~/.tsuku/registry/{letter}/{name}.toml`)
2. Remote registry (fetch and cache)
3. Bundled recipes (fallback)

### Cache Management
- `tsuku update-registry` - Clear cache (next fetch gets fresh recipes)
- Cache has no TTL - explicit refresh only
- Individual recipe refresh on `tsuku update <tool>`

## Files to Create
- `internal/registry/registry.go` - Registry client for fetching recipes

## Files to Modify
- `internal/config/config.go` - Add registry URL config
- `internal/recipe/loader.go` - Add remote fetching to Loader
- `cmd/tsuku/main.go` - Add `update-registry` command

## Implementation Steps
- [ ] Add registry URL to config
- [ ] Create registry client to fetch recipes from GitHub
- [ ] Update Loader to check cache, fetch remote, fallback to bundled
- [ ] Add `update-registry` command to clear/refresh cache
- [ ] Update tests
- [ ] Test with actual registry

## API Design

### Registry Client
```go
type Registry struct {
    BaseURL   string // https://raw.githubusercontent.com/tsuku-dev/tsuku-registry/main
    CacheDir  string // ~/.tsuku/registry
}

func (r *Registry) FetchRecipe(name string) ([]byte, error)
func (r *Registry) ClearCache() error
```

### Updated Loader
```go
type Loader struct {
    bundled  embed.FS           // Fallback recipes
    registry *Registry          // Remote registry client
    cache    map[string]*Recipe // In-memory cache
}

func (l *Loader) Get(name string) (*Recipe, error)
// 1. Check in-memory cache
// 2. Check disk cache
// 3. Fetch from registry (and cache)
// 4. Fall back to bundled
```

## Testing Strategy
- Unit tests with mock HTTP server
- Integration test fetching real recipe from registry
- Test fallback to bundled when offline

## Environment Variables
- `TSUKU_REGISTRY_URL` - Override default registry URL

## Success Criteria
- [ ] `tsuku install age` fetches from registry (not bundled)
- [ ] Recipes cached locally after first fetch
- [ ] Works offline with cached recipes
- [ ] Falls back to bundled when offline and no cache
- [ ] `tsuku update-registry` clears cache
- [ ] Custom registry URL works

## Risks and Mitigations
- **Network failures**: Graceful fallback to bundled
- **Registry unavailable**: Same fallback behavior
- **Slow fetches**: Single recipe fetch is fast (~100ms)

## Open Questions
None - design is straightforward.
