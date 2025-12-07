# Issue #218 Summary: Homebrew Version Provider

## What Was Implemented

A version provider that queries Homebrew's public API to resolve library versions at runtime, enabling recipes for shared libraries to detect the latest available version.

## Files Changed

### New Files
- `internal/version/homebrew.go` - Core API resolution methods
- `internal/version/provider_homebrew.go` - Provider implementing VersionResolver/VersionLister
- `internal/version/homebrew_test.go` - Comprehensive test suite (16 tests)

### Modified Files
- `internal/version/resolver.go` - Added `homebrewRegistryURL` field for test injection
- `internal/version/provider_factory.go` - Registered `HomebrewSourceStrategy`
- `internal/recipe/types.go` - Added `Formula` field to VersionSection

## Key Design Decisions

1. **API Endpoint**: Uses `https://formulae.brew.sh/api/formula/{formula}.json`
2. **Security**: Formula name validation prevents injection attacks (path traversal, shell chars)
3. **Testability**: Registry URL injectable for mock HTTP servers
4. **Versioned Formulae**: `ListVersions()` extracts versions from `versioned_formulae` array (e.g., `openssl@1.1` -> `1.1`)

## Usage

```toml
[version]
source = "homebrew"
formula = "libyaml"
```

## Test Coverage

- Success cases (resolve, list versions)
- Error handling (not found, disabled, invalid JSON, network errors)
- Formula validation (path traversal, shell injection, length limits)
- Fuzzy version matching (e.g., "0.2" matches "0.2.5")
- Provider interface methods

## Acceptance Criteria Met

- [x] Provider queries Homebrew API for formula info
- [x] Extracts `versions.stable` for latest version
- [x] Integrated into version provider factory for `source = "homebrew"`
- [x] Unit tests with mocked HTTP responses
