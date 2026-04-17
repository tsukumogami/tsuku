# Decision Report: Registry Refresh TTL Semantics — Decision 1

**Question:** Should plain `tsuku update-registry` (no flags) force-fetch all cached recipes regardless of TTL, or should it keep the TTL skip and surface a hint ("run --all to force refresh") in the "already fresh" output?

**Prefix:** design_registry-refresh-ttl-semantics_decision_1
**Date:** 2026-04-16
**Status:** Decided

---

## Context

`tsuku update-registry` is documented as "Refresh the recipe cache." The command exists precisely so users can pull down updated recipes after upstream fixes. Yet `RefreshAll()` in `internal/registry/cached_registry.go` applies a 24-hour TTL: any recipe cached within the last 24 hours is skipped and reported "already fresh." No network request is made and no content comparison occurs.

This creates a trap: users most likely to need a refresh — those who cached a broken recipe earlier the same day — are exactly the ones who get no update. The `--all` flag (`registryRefreshAll` bool) already bypasses TTL via `forceRegistryRefreshAll()`, but it is never surfaced to users who see "already fresh" and assume they're up to date.

The TTL exists for a valid reason: implicit cache access during `tsuku install`, `tsuku info`, and `tsuku search` should not hammer the registry on every invocation. That constraint must be preserved. The background `check-updates` process (`cmd_check_updates.go`) does not call registry refresh at all, so it is unaffected by any option.

The invocation site inventory (from the design doc) is short:

| Call site | Type | Current TTL behavior |
|---|---|---|
| `cmd/tsuku/update_registry.go:168` — `cachedReg.RefreshAll()` | explicit user command | skips fresh recipes (bug) |
| `internal/registry/cached_registry.go:GetRecipe()` | implicit during install/info/etc. | respects TTL (correct) |
| `cmd/tsuku/cmd_check_updates.go` — background check | automated | does not call registry refresh |

The fix must differentiate explicit user invocations from implicit/automated ones.

---

## Key Assumptions

- Users who type `tsuku update-registry` intend to fetch current recipes from the registry; they are not asking "tell me if my cache is still fresh."
- The TTL protects against implicit cache hammering, not against deliberate refresh requests. The constraint "TTL preserved for implicit/automated calls" does not apply to an explicit refresh command.
- The `--all` flag is not discoverable from the "already fresh" summary line; users who see it have no signal to try again with a flag.
- The `forceRegistryRefreshAll()` function already exists and works correctly. No new logic is required to implement Option A — only routing changes.
- The design constraint "minimal user-facing complexity" means adding flags or hints increases surface area, not reduces it.
- There is no automated caller of plain `tsuku update-registry`; the background process uses a separate code path (`updates.RunUpdateCheck`).

---

## Chosen Option

**Option A: Force-refresh by default.**

Plain `tsuku update-registry` always fetches all cached recipes from the network, regardless of TTL. This is identical to the current `--all` behavior. The `--all` flag can be retained as a no-op alias or deprecated quietly; either way it does not change the default.

TTL remains in `GetRecipe()` to protect implicit cache access during install and info operations.

### Detailed rationale

The command name is the contract. `update-registry` says "update." Users who run it after seeing a broken recipe are not asking the system to check whether the cache is still within a freshness window — they are asking for fresh data. Returning "already fresh" after a network-skipping TTL check violates that contract without telling the user what happened.

The existing `--all` bypass already proves the TTL skip is wrong for explicit invocations: the developers who added it recognized the need to escape the TTL. The question is only whether that escape should be the default or require a hidden flag. Given that the documented purpose of the command is refreshing, the default must deliver a refresh.

Option A requires the smallest code change. `runRegistryRefreshAll` currently branches on `registryRefreshAll` to choose between `forceRegistryRefreshAll` and `cachedReg.RefreshAll`. Removing that branch — or making `registryRefreshAll` default to true — routes all plain invocations to the force path. No new API, no new public surface.

The constraint "minimal user-facing complexity" also favors A. Option B adds a hint line that users must notice and then re-run with a flag. This is more steps, more surface, and still doesn't fix the underlying broken default. Complexity is not measured by line count in the implementation — it is measured by steps the user must take to achieve a stated goal.

Network cost is acceptable. A user who runs `tsuku update-registry` is explicitly accepting a network round-trip. The 24-hour TTL protects against unasked-for network traffic during `tsuku install`; it was never intended to throttle explicit refresh. With ~150 registry recipes at ~3KB each, a full refresh is a few hundred HTTPS requests to GitHub's raw content CDN — fast and cheap on any reasonable connection.

---

## Rejected Alternatives

### Option B: Keep TTL + surface a hint

**Rejection reason:** Does not fix the bug. The core failure mode is that users run `tsuku update-registry` after a recipe fix is merged and receive stale content without knowing it. Adding a hint line ("run --all to force refresh") shifts responsibility to the user without correcting the misleading default. Users reading "already fresh" have no reason to believe the cache might contain the old broken recipe — the hint must be understood, noticed, and acted upon at exactly the moment when the user is already confused. This adds discoverability friction without removing the root cause. The hint also adds ongoing display noise for every user who runs the command when their cache genuinely is fresh, which is most of the time.

Additionally, the hint implies that `--all` is unusual (a "force" option), when in practice it should be the only behavior for an explicit refresh command. Naming the escape hatch `--all` and the broken default "normal" inverts the expected semantic.

### Option C: Remove TTL from RefreshAll entirely

**Rejection reason:** Violates the constraint that TTL must be preserved for implicit/automated callers. `RefreshAll` is a method on `CachedRegistry`, which is a shared type used by multiple call sites. If `RefreshAll` always fetches regardless of TTL, any future automated caller that uses it will bypass the intended rate-limiting behavior. Option A preserves the TTL in `GetRecipe()` (the implicit path) while routing the command layer to always force-refresh. This separation is cleaner and safer than removing TTL from the shared method.

---

## Consequences

### Positive

- `tsuku update-registry` reliably delivers updated recipes after upstream fixes, including for users who ran the command within the previous 24 hours.
- No new flags, no new hint lines, no new user mental model. The command does what its name says.
- The `--all` flag becomes a no-op alias (or can be deprecated in a later cleanup), reducing confusion about why two paths exist.
- The TTL still protects implicit cache access. `GetRecipe()` in `CachedRegistry` is unchanged, so `tsuku install`, `tsuku info`, and `tsuku search` continue to serve from cache within the freshness window.
- The `forceRegistryRefreshAll()` function is already implemented and tested; this change primarily reroutes the default call path.

### Negative

- Plain `tsuku update-registry` now always makes network requests, even if the cache was refreshed a few minutes ago. Users who run the command as a no-op health check (to verify the registry is reachable) will get a real refresh every time. This is acceptable: the command's documented purpose is refreshing, not connectivity checking.
- If a future automated caller is added that uses `runRegistryRefreshAll` internally, it will incur full network cost. That caller must use `cachedReg.RefreshAll()` directly (with TTL) rather than invoking the command-level function. This is a documentation and convention concern, not a code concern.

### Implementation note

The change is localized to `cmd/tsuku/update_registry.go`. The `registryRefreshAll` flag and the `if registryRefreshAll { ... } else { ... }` branch in `runRegistryRefreshAll` are removed (or the else branch is removed and the force path becomes unconditional). `CachedRegistry.RefreshAll()` and its TTL logic remain unchanged for implicit callers.
