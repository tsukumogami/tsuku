# Issue 32 Baseline

## Issue Summary
Fetch recipes from external registry (https://github.com/tsuku-dev/tsuku-registry) instead of bundling them in the binary. This allows recipe updates without tsuku releases.

## Environment
- Date: 2025-11-28
- Branch: feature/32-external-registry
- Base commit: ecf8af03843bab5b63a4af4984bea43430b4b4ee

## Test Results
- All unit tests pass
- Build succeeds

## Current Architecture

### Recipe Bundling
- `bundled/bundled.go`: Uses `//go:embed recipes/*.toml` to embed all recipes
- `bundled/recipes/`: Contains 20 TOML recipe files
- Binary is self-contained with no external dependencies

### Recipe Loading
- `internal/recipe/loader.go`: `Loader` struct with `embed.FS`
- `NewLoader(recipesFS embed.FS)`: Initializes with embedded filesystem
- `Get(name)`: Retrieves recipe by name
- `List()`: Returns all available recipe names

### Initialization
- `cmd/tsuku/main.go`: `init()` creates loader with `bundled.Recipes`
- Global `loader` variable used by all commands

### Config
- `internal/config/config.go`: Defines `~/.tsuku/` directory structure
- `RecipesDir`: `~/.tsuku/recipes` (currently unused)

## Key Files to Modify
- `internal/recipe/loader.go` - Add remote fetching capability
- `internal/config/config.go` - Add registry URL configuration
- `cmd/tsuku/main.go` - Update initialization to use remote registry

## Registry Structure (Expected)
The tsuku-registry repo should have:
```
recipes/
  actionlint.toml
  golang.toml
  ...
```

## Success Criteria
- Recipes fetched from remote registry
- Bundled recipes serve as fallback
- Registry URL configurable
- `tsuku update-registry` command to refresh cache
