# Issue 872 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-tap-support.md` (Status: Planned)
- Sibling issues reviewed:
  - #862 (feat(cask): add walking skeleton for cask support) - CLOSED
  - #873, #874, #875 - OPEN, depend on #872
  - #863, #864, #865, #866 - OPEN, cask feature issues
- Prior patterns identified:
  - CaskProvider implementation in `internal/version/provider_cask.go`
  - CaskSourceStrategy in `internal/version/provider_factory.go`
  - VersionInfo.Metadata map pattern for provider-specific data
  - recipe.VersionSection fields (Source, Cask, Formula, etc.)

## Gap Analysis

### Minor Gaps

1. **Recipe field for tap specification**: The issue specifies recipe syntax like `source = "tap"` with `tap = "hashicorp/tap"` and `formula = "terraform"`, but the `VersionSection` struct currently lacks a `Tap` field. The `Formula` field already exists (added for Homebrew support).

   **Resolution**: Add `Tap string` field to `recipe.VersionSection` in `internal/recipe/types.go`. This follows the pattern used for `Cask`, `Formula`, and other provider-specific fields.

2. **TapVersionInfo vs VersionInfo.Metadata**: The design doc shows a `TapVersionInfo` struct with explicit fields (Version, Formula, BottleURL, Checksum, Extra). However, the cask implementation (which #862 established) uses the existing `VersionInfo` struct with metadata stored in the `Metadata map[string]string` field.

   **Resolution**: Follow the cask pattern - use `VersionInfo` with Metadata map keys for `bottle_url`, `checksum`, `formula`, `tap`. This provides consistency and avoids type proliferation. The design doc's `TapVersionInfo` can be an internal parsing struct, but the provider should return standard `VersionInfo`.

3. **TapSourceStrategy registration**: The factory needs a `TapSourceStrategy` registered at `PriorityKnownRegistry`. This follows the exact pattern from `CaskSourceStrategy`.

   **Resolution**: Create `TapSourceStrategy` in `provider_factory.go` following the `CaskSourceStrategy` pattern.

4. **Test patterns**: The cask provider tests (`provider_cask_test.go`) establish a pattern for testing:
   - Direct provider tests (ResolveLatest, ResolveVersion, SourceDescription)
   - Strategy tests (CanHandle)
   - Interface compliance test

   **Resolution**: Follow the same test structure for `tap_test.go`.

### Moderate Gaps

None identified. The issue spec is well-aligned with established patterns.

### Major Gaps

None identified. The cask walking skeleton (#862) provides clear implementation patterns that directly apply.

## Recommendation

**Proceed**

## Implementation Notes

The following patterns from #862 should be followed:

1. **File structure**: Create `internal/version/tap.go`, `internal/version/tap_parser.go`, `internal/version/tap_test.go`

2. **Provider signature**:
   ```go
   type TapProvider struct {
       resolver *Resolver
       tap      string   // e.g., "hashicorp/tap"
       formula  string   // e.g., "terraform"
   }

   func NewTapProvider(resolver *Resolver, tap, formula string) *TapProvider
   ```

3. **Metadata map keys** (matching template variables):
   - `bottle_url` - Platform-specific bottle URL
   - `checksum` - SHA256 checksum
   - `formula` - Full formula name
   - `tap` - Tap identifier

4. **VersionSection field** (needs to be added):
   ```go
   Tap string `toml:"tap"` // Homebrew tap for third-party formula resolution (e.g., "hashicorp/tap")
   ```

5. **Strategy pattern**:
   ```go
   type TapSourceStrategy struct{}
   func (s *TapSourceStrategy) Priority() int { return PriorityKnownRegistry }
   func (s *TapSourceStrategy) CanHandle(r *recipe.Recipe) bool {
       return r.Version.Source == "tap" && r.Version.Tap != "" && r.Version.Formula != ""
   }
   ```

## Proposed Amendments

None required. The issue spec is complete and aligns with codebase patterns.
