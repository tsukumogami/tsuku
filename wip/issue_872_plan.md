# Issue 872 Implementation Plan

## Summary

Implement a dedicated `tap` version provider that fetches formula metadata from third-party Homebrew taps via GitHub raw content, parses Ruby formula files with regex, and returns version info with bottle URLs and checksums using the existing `VersionInfo.Metadata` map pattern.

## Approach

Follow the established cask provider pattern from #862: create a dedicated `TapProvider` implementing `VersionResolver`, add a `TapSourceStrategy` to the factory, and add a `Tap` field to `recipe.VersionSection`. The provider uses GitHub raw content fetching (`raw.githubusercontent.com`) to avoid API rate limits, parses formula files with targeted regex patterns, and maps platform tags to construct bottle URLs.

### Alternatives Considered

- **Extend HomebrewProvider**: Rejected because tap metadata access is fundamentally different from `formulae.brew.sh` API access. The cask design established the pattern of dedicated providers for different data sources.
- **Custom TapVersionInfo struct for return values**: Rejected per introspection guidance. Use existing `VersionInfo.Metadata` map pattern for consistency with cask provider, avoiding type proliferation.

## Files to Modify

- `internal/recipe/types.go` - Add `Tap string` field to `VersionSection` struct
- `internal/version/provider_factory.go` - Add `TapSourceStrategy` struct and register in factory

## Files to Create

- `internal/version/provider_tap.go` - TapProvider implementing VersionResolver
- `internal/version/tap_parser.go` - Ruby formula parsing via regex
- `internal/version/provider_tap_test.go` - Unit tests with mocked HTTP responses

## Implementation Steps

- [ ] 1. Add `Tap` field to `recipe.VersionSection` in `internal/recipe/types.go`
- [ ] 2. Create `internal/version/tap_parser.go` with Ruby formula parsing:
  - [ ] Version regex: `version "x.y.z"`
  - [ ] Bottle block regex: `bottle do ... end`
  - [ ] Root URL regex: `root_url "https://..."`
  - [ ] SHA256 checksum regex (both patterns): `sha256 "hash" => :platform` and `sha256 platform: "hash"`
  - [ ] Error handling for no version, no bottle block, no checksums
- [ ] 3. Create `internal/version/provider_tap.go`:
  - [ ] TapProvider struct with resolver, tap, formula fields
  - [ ] NewTapProvider constructor
  - [ ] Formula file discovery logic (Formula/, HomebrewFormula/, root)
  - [ ] GitHub raw content fetching from `raw.githubusercontent.com/{owner}/homebrew-{repo}/HEAD/{path}`
  - [ ] Platform tag mapping (darwin/arm64 -> arm64_sonoma, etc.)
  - [ ] Bottle URL construction: `{root_url}/{formula}--{version}.{platform}.bottle.tar.gz`
  - [ ] ResolveLatest: fetch formula, parse, return VersionInfo with Metadata
  - [ ] ResolveVersion: resolve specific version (validate against parsed version)
  - [ ] SourceDescription: return "Tap:{tap}/{formula}"
  - [ ] Metadata map keys: `bottle_url`, `checksum`, `formula`, `tap`
- [ ] 4. Add `TapSourceStrategy` to `internal/version/provider_factory.go`:
  - [ ] Priority: `PriorityKnownRegistry` (100)
  - [ ] CanHandle: `source == "tap" && Tap != "" && Formula != ""`
  - [ ] Create: return `NewTapProvider(resolver, tap, formula)`
  - [ ] Register in `NewProviderFactory()`
- [ ] 5. Create `internal/version/provider_tap_test.go`:
  - [ ] Test formula parsing with fixture data
  - [ ] Test ResolveLatest with mocked HTTP
  - [ ] Test ResolveVersion with mocked HTTP
  - [ ] Test SourceDescription
  - [ ] Test interface compliance
  - [ ] Test TapSourceStrategy.CanHandle
  - [ ] Test error cases: formula not found, no bottle block, no matching platform
- [ ] 6. Run `go test ./...` to verify all tests pass
- [ ] 7. Run `go vet ./...` and `golangci-lint run --timeout=5m ./...`

## Testing Strategy

- **Unit tests**: Focus on formula parsing with various real-world formula patterns (hashicorp/tap formulas), provider methods with mocked HTTP responses, strategy CanHandle logic
- **Integration tests**: Not required for this slice; factory integration is tested in #874
- **Manual verification**: Build and test parsing against real formula files from hashicorp/tap

### Test Fixtures

Create test fixtures based on real Homebrew formula patterns:

```ruby
# Simple formula with explicit version and standard bottle block
class Terraform < Formula
  version "1.7.0"

  bottle do
    root_url "https://github.com/hashicorp/homebrew-tap/releases/download/v1.7.0"
    sha256 "abc123..." => :arm64_sonoma
    sha256 "def456..." => :sonoma
    sha256 "ghi789..." => :x86_64_linux
  end
end
```

## Risks and Mitigations

- **Ruby parsing fragility**: Regex-based parsing may fail on complex formulas. Mitigation: target only stable patterns (`version "x"`, `bottle do...end`, `sha256`), fail clearly with descriptive errors, comprehensive test fixtures.
- **Platform tag mapping**: macOS version detection may be needed for accurate platform tags. Mitigation: for initial implementation, use a simplified mapping that covers common platforms; full macOS version detection can be added in a follow-up.
- **Formula file location variability**: Different taps use different directory structures. Mitigation: implement discovery logic that checks `Formula/`, `HomebrewFormula/`, and repository root.

## Success Criteria

- [ ] TapProvider implements VersionResolver interface
- [ ] Formula parsing extracts version, root_url, and platform-specific checksums
- [ ] Platform tag mapping works for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
- [ ] Bottle URL construction follows Homebrew convention
- [ ] Clear error messages for: formula not found, no bottle block, no matching platform
- [ ] All tests pass with `go test ./...`
- [ ] Code passes `go vet` and `golangci-lint`
- [ ] `Tap` field added to `recipe.VersionSection`
- [ ] `TapSourceStrategy` registered in provider factory

## Open Questions

None. The introspection document resolved the key design decisions:
1. Use `VersionInfo.Metadata` map pattern (not custom return struct)
2. Add `Tap` field to `recipe.VersionSection`
3. Follow CaskSourceStrategy pattern for factory integration
