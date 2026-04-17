# Phase 3 Maintainer Assessment: update-registry TTL skips upstream-changed recipes

## 1. Does the research change your understanding of the problem?

**Yes, meaningfully.**

Phase 1 characterized this as "TTL-only check with no content comparison." That's accurate at a high level, but the code reading adds important texture:

- The `--all` flag *already exists* in the CLI (line 81 of `update_registry.go`) and does exactly what the user needs. It routes through `forceRegistryRefreshAll()` which calls `cachedReg.Refresh()` per recipe, bypassing the `isFresh` guard entirely. The fix isn't missing functionality -- it's a missing hint.
- The `--recipe infisical` path also bypasses TTL because `runSingleRecipeRefresh` calls `cachedReg.Refresh()` directly, not `RefreshAll()`. So users have two existing workarounds; neither is surfaced when they see "already fresh."
- `isFresh` is time-only: `meta.CachedAt.Add(c.ttl) > time.Now()`. No ETag, no content hash, no upstream timestamp. The registry server's content can change at any moment and the client won't know until TTL expires. This is the structural root cause, but the *symptom* in the bug report is the missing hint.

The problem is now best framed as two related issues:
- **Immediate (UX)**: "already fresh" output gives users no path forward when the upstream recipe has changed during TTL. The `--all` workaround exists but is invisible.
- **Structural (correctness)**: Even with `--all` awareness, the default behavior still makes stale-within-TTL recipes inaccessible without manual override.

The title direction from Phase 1 ("update-registry skips recipes that changed upstream within TTL window") is correct. The fix is most actionable as: when `update-registry` is run explicitly (not as a background refresh), it should surface the `--all` hint in the "already fresh" message, or better, bypass TTL entirely since an explicit user invocation signals intent to fetch fresh data.

## 2. Is this a duplicate of an existing issue?

**No.** Phase 1 confirmed no open duplicates. The code confirms no existing issue or TODO tracks this behavior.

## 3. Is the scope still appropriate for a single issue?

**Yes, with one clarification.**

The scope is tight enough for one issue. There are two overlapping fixes, but both touch the same code path and the same user interaction surface:

1. Make plain `update-registry` (without `--all`) force-refresh all cached recipes rather than deferring to TTL. This is the primary fix -- it aligns behavior with user intent.
2. If we keep the TTL-respecting default (debatable), at minimum append a hint to the "already fresh" summary line: "To force-refresh all recipes regardless of freshness, run `tsuku update-registry --all`."

These are variants of the same fix, not separate issues. A single issue can specify both the preferred fix (force-refresh on explicit invocation) and the fallback (hint text) if the preferred approach is rejected.

## 4. Are we ready to draft?

**Yes.**

No blockers. The root cause is confirmed, no duplicate exists, the scope is clean, and both the fix path and workaround are identified. The issue can be drafted with a concrete reproduction, root cause explanation, and a clear proposed fix with the alternative.

## 5. What context from research should be included in the issue?

Include:

- **Reproduction steps**: `tsuku update-registry` → `infisical: already fresh` → `tsuku install infisical` gets stale recipe. Concrete, reproducible with any recipe updated upstream within its TTL window.
- **Root cause (brief)**: `RefreshAll()` in `internal/registry/cached_registry.go` guards refresh behind a time-only `isFresh` check; no content comparison. When a recipe changes upstream before TTL expires, the local cache doesn't know.
- **Existing workarounds**: `tsuku update-registry --all` or `tsuku update-registry --recipe <name>` both bypass TTL and fetch fresh content. Neither is mentioned when users see "already fresh."
- **Proposed fix**: Change the default behavior of `update-registry` (without flags) to bypass TTL and force-refresh, since explicit user invocation implies intent to pull fresh data. Background/automatic refreshes (not user-invoked) should keep TTL semantics to avoid unnecessary network traffic.
- **Alternative (lower-effort)**: If TTL-respecting default is intentional, add a hint to the "all fresh" summary output pointing users to `--all`.
- **Affected files**: `internal/registry/cached_registry.go` (`RefreshAll`, `isFresh`), `cmd/tsuku/update_registry.go` (`runRegistryRefreshAll`, output formatting).

Omit: internal workflow references, ETag/content-hash enhancement ideas (out of scope for this bug), performance considerations for large registries (separate concern).
