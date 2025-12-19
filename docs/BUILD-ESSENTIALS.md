# Build Essentials

tsuku provides build tools and libraries needed for source compilation. These are automatically installed as implicit dependencies when you install a tool that requires compilation.

## Available Build Essentials

### Compilers

**zig**
- C/C++ compiler via `zig cc` and `zig c++`
- Automatically used as fallback when system compiler unavailable
- Cross-platform support (Linux x86_64, macOS Intel, macOS ARM)
- Recipe: `internal/recipe/recipes/z/zig.toml`

### Build Tools

**make**
- GNU Make build automation
- Required by configure_make action
- Installed from Homebrew bottles
- Recipe: `internal/recipe/recipes/m/make.toml`

### Libraries

**zlib**
- Compression library (libz)
- Common dependency for many tools
- Installed from Homebrew bottles
- Recipe: `internal/recipe/recipes/z/zlib.toml`

**gdbm**
- GNU database manager (key-value store)
- Available as both bottle and source build
- Recipes:
  - Bottle: `internal/recipe/recipes/g/gdbm.toml`
  - Source: `testdata/recipes/gdbm-source.toml` (validation only)

**libpng**
- PNG image library
- Depends on zlib
- Installed from Homebrew bottles
- Recipe: `internal/recipe/recipes/l/libpng.toml`

**pngcrush**
- PNG optimization tool
- Example consumer of libpng dependency chain
- Recipe: `internal/recipe/recipes/p/pngcrush.toml`

## Installation

Build essentials are installed automatically when needed. You can also install them explicitly:

```bash
# Explicit installation
tsuku install zig
tsuku install make
tsuku install zlib
```

All build essentials are installed to `$TSUKU_HOME/tools/` and managed using the same dependency tracking as regular tools.

## Platform Support

Build essentials are validated on:
- Linux x86_64 (ubuntu-latest)
- macOS Intel (macos-13)
- macOS Apple Silicon (macos-14)

Note: arm64 Linux is not currently supported for Homebrew bottles due to upstream availability limitations.

## Validation

Build essentials undergo additional validation beyond standard tests:

- **Functional tests**: `test/scripts/verify-tool.sh`
- **Relocation tests**: `test/scripts/verify-relocation.sh`
- **Dependency isolation**: `test/scripts/verify-no-system-deps.sh`

See the [Build Essentials CI workflow](../.github/workflows/build-essentials.yml) for the complete validation matrix.

## See Also

- [Actions and Primitives Guide](GUIDE-actions-and-primitives.md#build-environment-configuration) - How tsuku configures build environments
- [Contributing Guide](../CONTRIBUTING.md#testing-build-essentials) - How to test build essential recipes
