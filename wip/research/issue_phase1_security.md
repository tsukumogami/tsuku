# Security Review: update-registry TTL bypass / stale recipe after fix

## Issue Description

After PR #2255 was merged (replacing the broken `homebrew` action with a `github_archive`
action in `recipes/i/infisical.toml`), a user ran `tsuku update-registry` and received
the message "infisical: already fresh". A subsequent `tsuku install infisical` still used
the old (broken) recipe.

---

## Findings

### 1. Root cause: TTL-based freshness skips explicitly-updated recipes

`RefreshAll()` in `internal/registry/cached_registry.go` (lines 334-342) skips any recipe
whose metadata shows `CachedAt` within the configured TTL (default 24 hours):

```go
// Check if fresh - skip refresh for fresh entries
if meta != nil && c.isFresh(meta) {
    stats.Fresh++
    stats.Details = append(stats.Details, RefreshDetail{
        Name:   name,
        Status: "already fresh",
        Age:    age,
    })
    continue
}
```

`isFresh()` is purely time-based: it compares `meta.CachedAt + ttl` against the current
clock. There is no content-hash comparison against the remote registry and no awareness
that the upstream source has changed.

A user who had fetched the infisical recipe within the last 24 hours (which is likely for
anyone who just tried and failed to install it) will always receive "already fresh" unless
they know to pass `--all` or `--recipe infisical`.

### 2. The workaround exists but is not surfaced

`cmd/tsuku/update_registry.go` implements `forceRegistryRefreshAll()` triggered by
`--all`, and `runSingleRecipeRefresh()` triggered by `--recipe`. Either would bypass the
TTL and fetch the fixed recipe. However:

- The default `update-registry` output gives no indication that a "--all" flag exists
  or that individual recipes can be force-refreshed.
- The "already fresh" message implies correctness, not that content may have changed
  upstream.
- A user who ran `update-registry` as a troubleshooting step would reasonably conclude
  the cache is correct and look elsewhere for the cause.

### 3. Security implications of stale recipe propagation

#### Supply-chain severity: MEDIUM-HIGH

The infisical fix in PR #2255 specifically addresses a broken `homebrew` action that
caused a segfault (`exit status 139`). In this case, the original bug was a functional
failure, not a checksum mismatch or tampered binary. However, the same caching mechanism
applies to any recipe fix, including security patches that:

- Correct a wrong or tampered download URL
- Fix a missing or incorrect checksum/verification pattern
- Replace a supply-chain-compromised download source

If a recipe were silently serving a malicious URL or wrong checksum, a user who ran
`update-registry` after a fix was merged would still install the compromised version
for up to 24 hours (or 7 days via stale-fallback). The user's cache, not the registry,
determines what gets installed.

#### The metadata hash is not used for invalidation

`CacheMetadata.ContentHash` stores a SHA-256 of the cached TOML content, but this hash
is never compared against the remote registry during `RefreshAll`. It exists only in the
metadata sidecar file. There is no mechanism for the registry server to advertise a
"minimum version" or "this recipe was updated" signal that would force a client refresh.

#### Stale-fallback window extends exposure

The 7-day `maxStale` window in `CachedRegistry` means a user who has been offline or
experiencing network issues could continue using a compromised recipe for up to a week
without any blocking error.

### 4. No differentiation between "fresh enough for normal use" and "may have security fix"

The TTL applies uniformly to all recipes. There is no mechanism to:
- Tag a recipe update as security-critical, requiring immediate propagation
- Publish a "minimum acceptable cache timestamp" so old cached copies are invalidated
- Notify users that a recipe they have cached was recently patched

### 5. The user-facing message is misleading

"infisical: already fresh" tells the user the local cache matches what they'd get from
the registry. It doesn't. It only means the cache is younger than the TTL. This is a
correctness problem and a security communication problem: users reasonably trust this
message and stop investigating.

---

## Answers to the Evaluation Questions

### 1. Is the problem clearly defined?

Yes. The scenario is unambiguous: a recipe fix is merged, `update-registry` silently
skips the recipe because the cached version is within TTL, and `install` continues using
the broken version. The mechanism (TTL-based freshness with no remote comparison) is
clear from the code.

### 2. Type

Bug. The existing `update-registry` command's TTL-only freshness check produces
incorrect behavior when upstream content changes within the TTL window.

### 3. Is the scope appropriate for a single issue?

Yes, with a note. The core bug (update-registry should not skip already-cached recipes
when the cache predates a registry change) is a single, tractable fix. A broader
"cache invalidation" design (e.g., content-hash comparison, registry-side cache-busting
tokens, or security-critical update channels) would be a separate design effort.

A minimal fix scoped to this issue could be:
- Make `update-registry` always fetch all cached recipes regardless of TTL (current `--all`
  behavior as the default, or at minimum clearly documenting and surfacing `--all` in the
  normal output).
- Or: compare the remote content hash against the stored `ContentHash` in metadata and
  refresh whenever they differ, regardless of TTL.

### 4. Gaps and ambiguities

- **Reproduction steps are implicit.** The issue should note that the user must have
  cached infisical within the last 24 hours to hit this (which is very likely for someone
  who just attempted a broken install).
- **"Same bug as before" needs clarification.** The issue says "showed the same bug as
  before the PR." This means the broken recipe is still installed. It should be explicit
  that the old cached `.toml` file is still being used rather than the updated one.
- **Expected behavior is not stated.** The issue should say: after `update-registry`,
  `install infisical` should use the version from the registry at the time of the
  `update-registry` run.
- **The `--all` workaround is not mentioned.** The issue would be strengthened by noting
  that `tsuku update-registry --all` or `tsuku update-registry --recipe infisical` works
  around the problem today.

### 5. Security assessment

MEDIUM-HIGH supply-chain risk. The mechanism that should propagate a recipe security
fix is neutralized by the TTL cache for users who fetched the recipe recently. This is
the population most likely to need the fix (they just tried to use the broken recipe).
The "already fresh" message actively misleads them. For fixes that correct a malicious
download source or wrong checksum, this cache behavior leaves users vulnerable for up
to 24 hours after a fix lands, despite running the update command explicitly.

---

## Recommended Title

```
fix(registry): update-registry --all should be default behavior, not opt-in
```

Or, if scoped more narrowly to the misleading output and the immediate fix:

```
fix(registry): update-registry skips recipe refresh when cache is within TTL
```

The second is more precise for a single-issue bug report. The first is better if the
fix is to make force-refresh the default for `update-registry`.

---

## Relevant Code Paths

- `internal/registry/cached_registry.go:334` -- TTL freshness check in `RefreshAll`
- `internal/registry/cached_registry.go:196` -- `isFresh()` function (time-only check)
- `cmd/tsuku/update_registry.go:163` -- `--all` flag triggers `forceRegistryRefreshAll`
- `cmd/tsuku/update_registry.go:168` -- default path calls `cachedReg.RefreshAll(ctx)` (TTL-gated)
- `internal/registry/cache.go:18` -- `DefaultCacheTTL = 24 * time.Hour`
- `internal/registry/cached_registry.go:54` -- `maxStale = 7 * 24 * time.Hour` (stale fallback window)
