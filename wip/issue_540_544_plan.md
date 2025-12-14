# Issue 540 & 544 Implementation Plan

## Summary

Add zlib and expat recipes following the established pattern: homebrew_bottle recipes for the official registry, and source build recipes in testdata for testing infrastructure.

## Approach

Follow the pattern from PR #565 (make/gdbm):
- Registry recipes use `homebrew_bottle` for fast, reliable installation
- testdata recipes use `homebrew_source` + `configure_make` for testing build infrastructure
- CI tests both approaches via build-essentials.yml

### Alternatives Considered
- Source-only recipes: Would test more but be slower and less reliable for users
- Bottle-only: Wouldn't validate dependency linking (expat needs zlib)

## Files to Create

### Registry (homebrew_bottle)
- `internal/recipe/recipes/z/zlib.toml` - zlib library using homebrew_bottle
- `internal/recipe/recipes/e/expat.toml` - expat using homebrew_bottle

### testdata (source builds for testing)
- `testdata/recipes/expat-source.toml` - expat built from source with zlib dependency

## Files to Modify
- `.github/workflows/build-essentials.yml` - Add zlib and expat to test matrix

## Implementation Steps

- [ ] Create zlib.toml in registry (type=library, homebrew_bottle)
- [ ] Create expat.toml in registry (homebrew_bottle with xmlwf binary)
- [ ] Create expat-source.toml in testdata (homebrew_source + configure_make)
- [ ] Update build-essentials.yml to test zlib and expat bottles
- [ ] Update build-essentials.yml to test expat-source build
- [ ] Update design doc to mark issues as done
- [ ] Run tests and validate locally

## Testing Strategy

- Unit tests: Existing recipe validation tests will cover new recipes
- CI: build-essentials.yml tests on Linux x86_64, macOS Intel, macOS ARM
- Manual: `./tsuku install zlib` and `./tsuku install expat` locally

## Recipe Details

### zlib.toml (library)
- type = "library"
- homebrew_bottle for zlib
- Libraries: lib/libz.a, lib/libz.dylib (or .so on Linux)
- Verify: ls lib/libz.a

### expat.toml (tool)
- homebrew_bottle for expat
- Binary: bin/xmlwf
- Verify: xmlwf --version (or --help)

### expat-source.toml (test recipe)
- dependencies = ["make", "zlib"]
- homebrew_source + configure_make
- Tests that source builds can link against tsuku-provided zlib

## Success Criteria

- [ ] zlib recipe installs successfully on all 3 platforms
- [ ] expat recipe installs successfully on all 3 platforms
- [ ] expat-source builds and links against zlib on all 3 platforms
- [ ] CI passes for all new tests
- [ ] Design doc updated with done status

## Open Questions

None - following established pattern from PR #565.
