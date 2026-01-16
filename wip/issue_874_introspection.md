# Issue 874 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-tap-support.md`
- Sibling issues reviewed: #862, #863, #864, #865, #866, #872, #873, #875
- Prior patterns identified:
  - `TapSourceStrategy` already implemented and registered in `provider_factory.go`
  - `TapProvider` core implementation complete in `provider_tap.go`
  - Tap cache implemented in `tap_cache.go`
  - GitHub token support implemented in provider_tap.go
  - Template variable population via `VersionInfo.Metadata` map established

## Gap Analysis

### Minor Gaps

1. **Short form parsing not implemented**: The issue specifies support for `source = "tap:hashicorp/tap/terraform"` short form syntax. The current `TapSourceStrategy.CanHandle()` only checks for `r.Version.Source == "tap"` and does not handle the `tap:` prefix parsing. This is documented as a requirement in the issue but not yet implemented.

2. **Missing short form tests**: The test file `provider_tap_test.go` has tests for the explicit form only. The validation script in the issue references `TestParseTapShortForm` which does not exist.

### Moderate Gaps

None identified. The missing short form parsing is straightforward to implement following established patterns.

### Major Gaps

None identified.

## Analysis Summary

The issue was created before sibling issues (#872, #873, #875) were implemented. Those issues have since landed the following:
- `TapProvider` implementation (from #872)
- `TapCache` implementation (from #873)
- GitHub token authentication (from #875)
- `TapSourceStrategy` registration in the factory (also from #872)

**What's already done (by prior issues):**
- [x] `TapSourceStrategy` struct implementing `ProviderStrategy` interface
- [x] Strategy registered in `NewProviderFactory()` at `PriorityKnownRegistry` (100)
- [x] `CanHandle` returns true when `r.Version.Source == "tap"` with tap and formula fields
- [x] `Create` method instantiates `TapProvider` with correct tap and formula parameters
- [x] Template variable population via `VersionInfo.Metadata` map (bottle_url, checksum, tap, formula)
- [x] Unit tests for `TapSourceStrategy.CanHandle()` with explicit form sources

**What's NOT yet done (required by issue #874):**
- [ ] `CanHandle` returns true when source starts with `"tap:"`
- [ ] Short form parsing extracts owner, repo, formula from `tap:owner/repo/formula`
- [ ] Short form parsing handles edge cases (missing parts, malformed input)
- [ ] Unit tests for short form parsing logic
- [ ] Integration test demonstrating tap provider resolves version and homebrew action installs bottle

The remaining work is limited to implementing the short form (`tap:owner/repo/formula`) parsing feature.

## Recommendation

**Proceed** - The issue can be implemented as scoped. The short form parsing is the remaining work item and is a well-defined, self-contained addition. Prior sibling issues have established all the infrastructure needed.

## Implementation Notes for Remaining Work

The short form implementation should:

1. Modify `TapSourceStrategy.CanHandle()` to also match `strings.HasPrefix(r.Version.Source, "tap:")`

2. Add a `parseTapShortForm(source string) (tap, formula string, err error)` function that:
   - Strips the `tap:` prefix
   - Splits remaining string by `/` expecting exactly 3 parts: owner, repo, formula
   - Reconstructs tap as `{owner}/{repo}`
   - Returns formula as-is
   - Returns clear error for malformed input

3. Update `TapSourceStrategy.Create()` to:
   - Check if source is short form
   - If so, parse it to extract tap and formula
   - Otherwise, use existing `r.Version.Tap` and `r.Version.Formula`

4. Add unit tests for:
   - Valid short form parsing
   - Missing parts error handling
   - Edge cases (extra slashes, empty parts)
