---
focus: pragmatic
issue: 6
blocking_count: 2
advisory_count: 2
---

# Pragmatic Review: Issue 6 (DistributedProvider)

## Blocking

**1. `Owner()` and `Repo()` accessors have zero callers outside tests.**
`internal/distributed/provider.go:85-92` -- `Owner()` and `Repo()` are public getters called only from `TestDistributedProvider_OwnerRepo`. No production code uses them. The fields are already accessible internally via `p.owner`/`p.repo`. Remove both methods and the test.

**2. `ForceListRecipes` duplicates ~90% of `ListRecipes` logic.**
`internal/distributed/client.go:147-174` -- The only difference is skipping the cache freshness check (lines 113-116 in `ListRecipes`). Add a `force bool` parameter to `ListRecipes` (or an internal `listRecipes(ctx, owner, repo, skipCache bool)`) and delete `ForceListRecipes`. The current approach means every bug fix to the rate-limit fallback path must be applied in two places.

## Advisory

**3. `splitQualifiedName` uses `LastIndex` for colon, creating ambiguous "multiple colons" behavior.**
`internal/recipe/loader.go:138` -- The test case `"acme/tools:sub:recipe"` parses qualifier as `"acme/tools:sub"` which is not a valid owner/repo. The validation on lines 150-151 would reject it (three parts after split on `/`), so it's inert, but `LastIndex` is misleading -- `Index` would be simpler and match the documented `owner/repo:recipe` format without the dead edge case.

**4. Test `TestDistributedProvider_Get_FetchesFromServer` doesn't actually test fetching.**
`internal/distributed/provider_test.go:112-166` -- The test pre-populates cache, hits the allowlist rejection, and asserts the error message. The `apiHandler` is never called. The test name promises server fetching but validates URL validation. Rename to `TestDistributedProvider_Get_RejectsNonAllowlistedHost` or restructure to test the actual fetch path.
