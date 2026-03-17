# Intent Scrutiny: Issue 3 (feat(config): add registry configuration and GetFromSource)

## Requirements Mapping Verification

### Verified: Correct

| AC | Status | Evidence | Verified |
|----|--------|----------|----------|
| RegistryEntry struct | implemented | `userconfig.go:20-28` -- `URL` and `AutoRegistered` fields, TOML-tagged | yes |
| Config StrictRegistries and Registries | implemented | `userconfig.go:47,51` -- both fields with correct TOML tags and omitempty | yes |
| Load/Save round-trip | implemented | `TestRegistrySaveAndLoadRoundTrip` covers save then load with 2 entries | yes |
| Empty registries no spurious TOML | implemented | `TestRegistryEmptyMapDoesNotProduceSpuriousTOML` checks file content for absence of keys | yes |
| Backward compat | implemented | `TestRegistryBackwardCompat_NoRegistries` loads old config without registries section | yes |
| GetFromSource routes correctly | implemented | `TestLoader_GetFromSource_Central_Registry`, `_Central_Embedded`, `_Central_PrefersRegistry`, `_Local` | yes |
| GetFromSource owner/repo | implemented | `TestLoader_GetFromSource_Distributed` with `acme/tools` mock provider | yes |
| GetFromSource error on unknown | implemented | `TestLoader_GetFromSource_UnknownSource`, `_NoMatchingDistributedProvider` | yes |
| GetFromSource bypasses cache | implemented | `TestLoader_GetFromSource_BypassesCache`, `_DoesNotWriteToCache` | yes |

All 9 AC items in the untrusted mapping are correctly verified.

## Findings

### 1. Type inconsistency in GetFromSource switch -- SourceCentral vs SourceLocal

**File:** `internal/recipe/loader.go:118-139`

`SourceCentral` is an untyped string constant (`= "central"`), while `SourceLocal`, `SourceEmbedded`, and `SourceRegistry` are typed `RecipeSource` constants. The switch statement handles this with an asymmetry:

```go
case SourceCentral:         // line 119 -- untyped string, matches directly
case string(SourceLocal):   // line 139 -- typed RecipeSource, needs cast
```

The `source` parameter is `string`, so `SourceCentral` (untyped) works without a cast, but `SourceLocal` (typed `RecipeSource`) requires `string()`. The next developer adding a new source branch will see one pattern or the other and pick whichever they saw first. If they write `case SourceEmbedded:` it won't compile (type mismatch). If they write `case string(SourceRegistry):` it works but is inconsistent with the `SourceCentral` case.

The root issue: `SourceCentral` is declared without the `RecipeSource` type while the other three have it. This is intentional (central is a user-facing alias, not a provider source), but the implicit contract is invisible to someone reading the switch.

**Severity:** Advisory. The compiler catches the wrong direction of this mistake, and the existing tests cover all branches. But the divergence between `SourceCentral` (untyped) and the others (typed) is a readability trap.

**Suggestion:** Add a one-line comment on the `SourceCentral` declaration explaining why it's untyped: it's a user-facing alias that maps to multiple provider sources (registry + embedded), not a provider identity.

### 2. GetFromSource error handling diverges between central and local/distributed

**File:** `internal/recipe/loader.go:117-172`

The `"central"` branch silently swallows errors from providers (`if err == nil && data != nil`), treating any error as "not found" and falling through to embedded. The `"local"` branch propagates errors (`return nil, err`). The distributed branch also propagates.

This means: if the registry provider returns a transient network error for `source="central"`, GetFromSource silently falls back to embedded. If the local provider returns a permission error for `source="local"`, the caller gets the error. The next developer implementing Issue 8 (source-directed loading for update/outdated) will not expect this asymmetry -- the design doc says `GetFromSource` fetches "from the same source that originally provided it." A silent fallback from registry to embedded is correct for normal resolution but surprising for source-directed operations where the caller asked for a specific source.

**Severity:** Blocking. Issue 8 will call `GetFromSource(ctx, name, "central")` to check if a newer recipe version exists. If the registry is down, the embedded (stale) recipe will be returned silently, and `outdated` will report the tool as up-to-date when it isn't. The design doc's "falls back gracefully if source is unreachable" (Issue 8 AC) suggests the caller should handle fallback, not GetFromSource.

**Suggestion:** For the central case, either (a) propagate the registry error when the registry provider is present but fails, letting the caller decide to fall back, or (b) document that central intentionally includes embedded as a fallback tier and add a test that verifies this fallback behavior with an error-returning registry mock. Currently neither the code comments nor the tests make this design choice visible.

### 3. Registry map keys: TOML table names vs owner/repo format

**File:** `internal/userconfig/userconfig_test.go:1523-1544`

The round-trip test at line 1421 uses `"acme/tools"` as a map key (matching the owner/repo format the AC specifies). But `TestRegistryLoadFromTOMLFile` at line 1530 uses `"acme_tools"` as the TOML table name because TOML doesn't allow `/` in bare keys -- it must be quoted (`[registries."acme/tools"]`).

This means the TOML file written by `Save()` (using the BurntSushi encoder) and the TOML file hand-written by a user may use different key formats. The round-trip test passes because the encoder quotes keys containing `/`. But if Issue 4's `registry add acme/tools` stores `"acme/tools"` in the map, the resulting TOML will have quoted keys that look different from the `TestRegistryLoadFromTOMLFile` fixture.

**Severity:** Advisory. The encoder handles this correctly, but the test fixture at line 1530 uses `acme_tools` (underscored) which doesn't match the owner/repo format. A developer working on Issue 4 might look at this test for the expected TOML format and use underscored keys instead of slash-separated ones.

### 4. No test for GetFromSource with "local" when tool exists in central but not local

**File:** `internal/recipe/loader_test.go:1434-1460`

`TestLoader_GetFromSource_Local` sets up only a local provider. There's no test verifying that `GetFromSource(ctx, name, "local")` does NOT consult the central provider when both are present. This is the core contract -- source-directed means "only this source." The bypass-cache tests verify the cache isn't consulted, but provider isolation for source != "central" isn't explicitly tested.

**Severity:** Advisory. The implementation clearly only iterates providers matching the source, so the behavior is correct. But for Issue 8 consumers, an explicit "doesn't leak to other providers" test would serve as documentation.

## Summary

- **1 Blocking finding:** Silent error swallowing in the central branch of GetFromSource will mislead Issue 8's update/outdated implementation into treating stale embedded data as fresh.
- **3 Advisory findings:** Type inconsistency in SourceCentral, TOML key format divergence in test fixtures, missing provider-isolation test for non-central sources.
- **Requirements mapping:** All 9 items verified correct.
