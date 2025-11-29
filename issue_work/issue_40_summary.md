# Issue 40 Implementation Summary

## Changes Made

### New Files

1. **`internal/builders/builder.go`**
   - Defines `Builder` interface with `Name()`, `CanBuild()`, `Build()` methods
   - Defines `BuildResult` struct with `Recipe`, `Warnings`, and `Source` fields

2. **`internal/builders/registry.go`**
   - Implements builder registry with thread-safe registration and lookup
   - Methods: `Register()`, `Get()`, `List()`

3. **`internal/builders/cargo.go`**
   - `CargoBuilder` implementation for crates.io packages
   - Queries crates.io API for package metadata
   - Fetches Cargo.toml from GitHub to discover executables
   - Falls back to crate name if Cargo.toml unavailable
   - Security: validates crate names and executable names against injection

4. **`internal/builders/cargo_test.go`**
   - Unit tests with mocked HTTP responses
   - Tests for name validation, build flow, fallback behavior, URL construction

5. **`cmd/tsuku/create.go`**
   - New `tsuku create <tool> --from <ecosystem>` command
   - Supports `--force` flag to overwrite existing recipes
   - Writes generated recipe to `~/.tsuku/recipes/<tool>.toml`

### Modified Files

1. **`internal/recipe/loader.go`**
   - Added `recipesDir` field for local recipes directory
   - Added `NewWithLocalRecipes()` constructor
   - Added `SetRecipesDir()` method
   - Modified `GetWithContext()` to check local recipes before registry
   - Added `loadLocalRecipe()` and `warnIfShadowsRegistry()` helper methods

2. **`internal/recipe/loader_test.go`**
   - Added tests for local recipe loading
   - Tests for priority (local > registry), fallback, and parse error handling

3. **`cmd/tsuku/main.go`**
   - Changed loader initialization to use `NewWithLocalRecipes()` with `cfg.RecipesDir`
   - Registered `createCmd` command

## Usage Example

```bash
# Create a recipe for a Rust crate
tsuku create ripgrep --from crates_io

# The recipe is now available locally
cat ~/.tsuku/recipes/ripgrep.toml

# Install the tool using the local recipe
tsuku install ripgrep
```

## Generated Recipe Example

```toml
[metadata]
name = "ripgrep"
description = "ripgrep recursively searches directories for a regex pattern"
homepage = "https://github.com/BurntSushi/ripgrep"

[version]
source = "crates_io"

[[steps]]
action = "cargo_install"
crate = "ripgrep"
executables = ["rg"]

[verify]
command = "rg --version"
```

## Test Results

All tests pass:
- `internal/builders` - 12 tests covering CargoBuilder and Registry
- `internal/recipe` - 31 tests including 6 new local recipe tests
- Full suite: 14 packages tested

## Architecture Notes

- **Priority order**: In-memory cache > Local recipes (`~/.tsuku/recipes/`) > Registry cache > Remote registry
- **Transparency**: Users can inspect and edit recipes before installation
- **Fallback chain**: Cargo.toml parsing > crate name as executable
- **Security**: Input validation prevents shell injection in crate/executable names
