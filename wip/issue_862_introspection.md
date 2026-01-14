# Issue 862 Introspection

## Context Reviewed

- Design doc: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/docs/designs/DESIGN-cask-support.md`
- Sibling issues reviewed: #863, #864, #865, #866 (all OPEN, none closed)
- Prior patterns identified:
  - Version provider pattern: `VersionResolver` interface in `internal/version/provider.go`
  - Provider factory registration: `NewProviderFactory()` in `internal/version/provider_factory.go`
  - Action registration: `init()` function in `internal/actions/action.go`
  - ExecutionContext pattern: `internal/actions/action.go` (existing fields: `ToolsDir`, `LibsDir`, etc.)
  - Template substitution: `expandVarsInString()` uses `{var}` syntax in `internal/executor/plan_generator.go`

## Gap Analysis

### Minor Gaps

1. **Template syntax mismatch**: The issue specifies `{{version.url}}` syntax but the codebase uses `{version}` (single braces). The plan generator's `expandVarsInString()` function in `internal/executor/plan_generator.go:523` uses `{k}` pattern, not `{{k}}` or `{{k.subfield}}`. Either:
   - The template substitution system needs to be extended for dotted paths
   - Or the design document uses different syntax than the actual implementation

2. **Recipe parameter evaluation location**: The issue mentions "Template Substitution Location: Look for existing `{{version}}` substitution code as a pattern." The actual location is `internal/executor/plan_generator.go` in the `expandParams()` and `expandVarsInString()` functions. The existing substitution only supports flat keys (e.g., `{version}`, `{os}`), not nested/dotted paths (e.g., `{version.url}`).

3. **CaskVersionInfo type relationship**: The issue specifies `CaskVersionInfo` as "extending base `VersionInfo`", but `VersionInfo` is a concrete struct with only `Tag` and `Version` fields. Go doesn't have struct inheritance - this will require either:
   - Embedding `VersionInfo` in `CaskVersionInfo`
   - Creating a separate type with additional URL/Checksum fields
   - Modifying the version provider interface contract

4. **Version source naming**: The issue uses `source = "cask"` in the recipe, matching the existing pattern in `provider_factory.go`. This is consistent with other providers like `"homebrew"`, `"pypi"`, etc.

5. **Cask field in version section**: The issue shows `cask = "iterm2"` in the `[version]` section, but the existing `Recipe.Version` struct in `internal/recipe/types.go` likely doesn't have a `Cask` field. This field needs to be added.

### Moderate Gaps

None identified. The issue spec is detailed and aligns with existing patterns.

### Major Gaps

None identified. This is the walking skeleton (first issue in milestone), so there's no prior work that could conflict.

## Recommendation

**Proceed** with implementation.

This is the first issue in the milestone (no siblings closed) and serves as the foundation for downstream issues. The issue spec is comprehensive and well-aligned with existing codebase patterns.

## Implementation Notes

Based on code review, the implementer should note:

1. **Template substitution extension required**: The `{version.url}` dotted-path syntax is new - will need to extend `expandVarsInString()` or add a pre-processing step. Current system only handles flat variable names.

2. **VersionInfo extension approach**: Consider returning extended metadata through the version provider's `ResolveLatest()` method. Look at how `HomebrewProvider` works in `internal/version/provider_homebrew.go` for patterns.

3. **Recipe types extension**: Add `Cask` field to `recipe.Version` struct to support `cask = "iterm2"` syntax.

4. **Action registration**: Follow existing pattern in `internal/actions/action.go` `init()` function to register `AppBundleAction`.

5. **ExecutionContext**: Add `AppsDir` field following the pattern of existing fields like `ToolsDir` and `LibsDir`.

## Proposed Amendments

No amendments needed - the issue spec is complete and implementation-ready.
