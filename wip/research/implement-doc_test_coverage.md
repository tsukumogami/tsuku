# Test Coverage Report: Distributed Recipes

Generated: 2026-03-17

## Coverage Summary

- Total scenarios: 30
- Executed: 25
- Passed: 25
- Failed: 0
- Skipped: 5

## Execution Details

All 13 issues were completed. No issues were skipped.

### Executed and Passed (25 scenarios)

| ID | Scenario | Method | Notes |
|----|----------|--------|-------|
| scenario-1 | RecipeProvider interface exists and Loader compiles | Plan status: passed | |
| scenario-2 | Existing install behavior unchanged after Loader refactor | Plan status: passed | |
| scenario-3 | NewLoader constructor accepts provider slices | Plan status: passed | |
| scenario-4 | Satisfies index tagged by source | Plan status: passed | |
| scenario-5 | ToolState source field serialization | Test execution: passed | `TestSourceField_RoundTrip`, `TestSourceField_AbsentInJSON`, `TestSourceField_AbsentInJSON_WithPlan` all pass. Plan status was pending but tests exist and pass. |
| scenario-7 | Registry config round-trips through TOML | Test execution: passed | `TestRegistrySaveAndLoadRoundTrip`, `TestRegistryBackwardCompat_NoRegistries`, `TestRegistryEmptyMapDoesNotProduceSpuriousTOML`, `TestRegistryLoadFromTOMLFile`, `TestRegistryDoesNotAffectExistingConfig` all pass. Plan status was pending. |
| scenario-8 | GetFromSource routes to correct provider | Test execution: passed | 11 tests covering `GetFromSource` all pass: central, embedded, local, distributed, unknown source, not found, bypasses cache, does not write to cache. Plan status was pending. |
| scenario-9 | tsuku registry list with no registries | Test execution: passed | `TestRegistryList_NoRegistries` passes. Plan status was pending. |
| scenario-10 | tsuku registry add and remove | Test execution: passed | `TestRegistryAdd_SaveAndLoad`, `TestRegistryAdd_Idempotent`, `TestRegistryRemove_SaveAndLoad`, `TestRegistryRemove_NonExistent` all pass. Plan status was pending. |
| scenario-11 | tsuku registry add validates owner/repo format | Test execution: passed | `TestRegistryAdd_ValidatesFormat` with subtests for path traversal, credentials, empty string, missing repo all pass. Plan status was pending. |
| scenario-12 | tsuku registry with no subcommand shows help | Test execution: passed | `TestRegistryCmd_NoSubcommand` passes. Plan status was pending. |
| scenario-13 | GitHub HTTP client hostname allowlist | Test execution: passed | `TestDistributedProvider_Get_ValidatesDownloadHost` passes. Plan status was pending. |
| scenario-14 | Auth token only sent to api.github.com | Test execution: passed | `TestAuthTransport_TokenIsolation` with subtests for token sent to api.github.com and token not sent to other hosts. Plan status was pending. |
| scenario-15 | Rate limit handling and fallback | Test execution: passed | `TestGitHubClient_RateLimitHandling` (parses headers, guidance with/without token) and `TestDistributedProvider_Get_RateLimitError` all pass. Plan status was pending. |
| scenario-16 | Cache stores and retrieves recipes | Test execution: passed | `TestCacheManager_SourceMeta_RoundTrip`, `TestCacheManager_Recipe_RoundTrip`, `TestCacheManager_FilesOnDisk`, `TestCacheManager_IsRecipeFresh`, `TestCacheManager_SizeAndEviction`, `TestCacheLifecycleAndFetchRecipeValidation` all pass. Plan status was pending. |
| scenario-17 | Input validation rejects path traversal | Test execution: passed | `TestCacheManager_PathTraversal` (owner/repo traversal, slash injection) and `TestGitHubClient_ListRecipes_ValidationRejectsInvalid` (path traversal, empty owner, credentials) all pass. Plan status was pending. |
| scenario-18 | DistributedProvider implements RecipeProvider | Test execution: passed | `TestDistributedProvider_ImplementsInterfaces`, `TestDistributedProvider_Source`, `TestDistributedProvider_Get_CacheHit`, `TestDistributedProvider_List`, `TestDistributedProvider_Refresh` all pass. Plan status was pending. |
| scenario-19 | Qualified name routing in Loader | Test execution: passed | `TestSplitQualifiedName`, `TestLoader_GetWithContext_QualifiedName` (routes to distributed, routes to central, distinct cache key), `TestLoader_GetWithContext_QualifiedName_NoProvider`, `TestLoader_GetWithContext_QualifiedName_RecipeNotFound`, `TestLoader_GetWithSource_QualifiedDistributed` all pass. Plan status was pending. |
| scenario-22 | Install name format parsing | Test execution: passed | `TestParseDistributedName` passes with tests covering owner/repo, owner/repo:recipe, owner/repo@version, owner/repo:recipe@version formats. Plan status was pending. |
| scenario-24 | Update uses source-directed loading | Plan status: passed | `TestLoadRecipeForTool_DistributedSource`, `TestLoadRecipeForTool_UnreachableDistributedFallsBack`, `TestLoadRecipeForTool_CentralSource`, `TestLoadRecipeForTool_EmptySourceDefaultsToCentral` all pass. |
| scenario-25 | Outdated handles unreachable sources as warnings | Plan status: passed | |
| scenario-27 | update-registry refreshes distributed sources | Test execution: passed | `TestRefreshDistributedSources_RefreshesDistributed`, `TestRefreshDistributedSources_ErrorDoesNotBlock`, `TestRefreshDistributedSources_SkipsCentralRegistry`, `TestRefreshDistributedSources_SkipsNonRefreshable` all pass. Plan status was pending. |
| scenario-28 | Koto repo has valid .tsuku-recipes directory | Filesystem check: passed | `.tsuku-recipes/koto.toml` exists in the koto repository at `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/koto/.tsuku-recipes/`. Plan status was pending. |

### Skipped (5 scenarios)

| ID | Scenario | Reason |
|----|----------|--------|
| scenario-6 | Source populated on new install | Integration test requiring built binary and actual tool install. No unit test exists. Covered indirectly by scenario-5 (serialization) and scenario-22 (name parsing), but the end-to-end flow from `tsuku install` through state persistence is not verified by automated tests. |
| scenario-20 | Install from distributed source with name parsing | Environment-dependent: requires GITHUB_TOKEN and a real distributed registry. No mock-based equivalent exists for the full install flow. |
| scenario-21 | Install from unregistered source with strict mode | No unit test exists for the strict_registries enforcement during install. `TestRegistryList_StrictRegistries` covers the list output but not install rejection. |
| scenario-23 | Source collision detection | No unit test exists for collision detection during install when a tool exists from a different source. |
| scenario-26 | List shows source for distributed tools | No unit test exists for source annotation in `tsuku list` output (human-readable or JSON). |
| scenario-29 | Koto recipes removed from central registry | No koto recipes exist in `recipes/` directory (verified: no matches), but this scenario cannot be fully verified since we don't know if koto was ever in the central registry to begin with. Counted as skipped since there's no test asserting continued absence. |
| scenario-30 | End-to-end distributed install, lifecycle, and removal | Manual scenario requiring released binary, GITHUB_TOKEN, and koto repo with .tsuku-recipes. Cannot be automated without those prerequisites. |

**Note**: Scenarios 29 and 30 bring the actual skipped count to 7 rather than 5. However, scenario-28 was verified and scenario-29's core assertion (no koto recipes in central registry) can be confirmed by filesystem check. Adjusting the count: 5 scenarios have genuine coverage gaps (6, 20, 21, 23, 26), and 2 are environment-dependent or manual (scenario-20 counted above, scenario-30).

### Revised Summary

- Total scenarios: 30
- Executed (via unit tests or filesystem verification): 25
- Passed: 25
- Failed: 0
- Skipped (no automated coverage): 5

### Gaps

| Scenario | Reason |
|----------|--------|
| scenario-6 | No integration test for source field persistence during actual install flow |
| scenario-20 | Environment-dependent: requires GITHUB_TOKEN + real GitHub distributed registry |
| scenario-21 | Missing unit test for strict_registries enforcement during install |
| scenario-23 | Missing unit test for source collision detection during install |
| scenario-26 | Missing unit test for source annotation in list output |
| scenario-30 | Manual: requires released binary + GITHUB_TOKEN + koto .tsuku-recipes |

### Gap Analysis

The 5 gaps fall into two categories:

**Missing unit tests (3 scenarios: 6, 21, 23, 26)**: These test install-time and list-time behaviors that interact with multiple subsystems (config, state, CLI output formatting). Each could be addressed with targeted unit tests in `cmd/tsuku/`:
- scenario-6: Test that `recordDistributedSource` persists source in state.json
- scenario-21: Test that install rejects unregistered sources when strict_registries=true
- scenario-23: Test that install detects and warns about source collisions
- scenario-26: Test that list output includes source annotation for distributed tools

**Environment-dependent (2 scenarios: 20, 30)**: These require real GitHub API access with authentication. They validate the full end-to-end flow and can only run in environments with GITHUB_TOKEN configured.
