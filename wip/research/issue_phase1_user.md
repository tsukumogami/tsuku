# Issue Assessment: User Perspective

## Scenario

PR #2255 fixed a bug in the infisical recipe. A user then ran `tsuku update-registry` hoping to get the corrected recipe, but saw "infisical: already fresh". Subsequent `tsuku install infisical` still exhibited the old bug.

---

## 1. Is the problem clearly defined?

**Yes**, with one detail left implicit.

The bug is reproducible and the failure mode is clear: `tsuku update-registry` says "already fresh" for a recipe that was updated upstream, so the fix never reaches the user. The implicit detail is *why* this happens -- the 24-hour TTL in `RefreshAll` skips entries that aren't yet expired -- but a user doesn't need to understand that to file a valid report. The symptom is unambiguous.

The problem statement is complete enough to reproduce:
1. Have infisical cached (e.g., from a recent `tsuku install infisical`).
2. Registry merges a fix for the infisical recipe.
3. Run `tsuku update-registry`.
4. See "infisical: already fresh" -- no network fetch performed.
5. `tsuku install infisical` uses the stale, broken recipe.

---

## 2. Type

**Bug.**

`tsuku update-registry` is explicitly documented as "Refresh the recipe cache." A user running it after a known upstream fix reasonably expects the cache to be updated. The command silently skips fresh-by-TTL entries without warning the user that their local copy may be stale relative to the upstream registry.

The root cause: `RefreshAll` in `internal/registry/cached_registry.go` checks TTL freshness and continues (skips) entries still within the 24-hour window. The `--all` flag bypasses this, but it is not surfaced in the default `update-registry` output or suggested when entries are skipped.

---

## 3. Scope -- appropriate for a single issue?

**Yes.**

The fix is contained to the `update-registry` UX. Two complementary approaches could be combined in one issue or split:

- **Option A (minimal):** When `update-registry` skips a recipe as "already fresh," note that `--all` forces a refresh regardless of TTL. This is a documentation/output change only.
- **Option B (proper fix):** Make `update-registry` (without `--all`) always fetch from the network -- i.e., treat the command as an explicit user intent to refresh, bypassing TTL. TTL should only gate background/implicit refreshes, not an explicit command invocation.

Option B is the correct user-expectation fix and is bounded to `RefreshAll` / `runRegistryRefreshAll`. Both fit a single issue.

---

## 4. Gaps and ambiguities

- **Expected behavior not stated explicitly.** The report says the user "got 'already fresh'" but doesn't state what they expected instead (presumably: the recipe is fetched from the network and the fixed version is cached). This should be spelled out.
- **Workaround not mentioned.** `tsuku update-registry --all` would have worked. Noting the workaround helps users unblock themselves and helps triagers confirm the bug.
- **No version or OS info.** Not strictly needed to reproduce, but useful for priority.
- **The `--recipe infisical` path is untested in the report.** `runSingleRecipeRefresh` calls `Refresh()` which always fetches from the network (no TTL check). So `tsuku update-registry --recipe infisical` would have worked. This matters for scope: the bug is specific to the default (no-flag) path, not to `--recipe`.

---

## 5. Recommended title

```
fix(registry): update-registry skips TTL-fresh recipes even when invoked explicitly
```

Alternative if the team prefers to frame it as a UX/output improvement:

```
fix(update-registry): always fetch from network when update-registry is run without --all
```

---

## Summary

Clear bug, well-scoped, with a straightforward fix: `tsuku update-registry` (with no flags) should treat explicit invocation as a signal to refresh regardless of TTL. TTL-based skipping belongs in background/implicit refresh paths, not in a command the user runs specifically to pull upstream changes. The workaround (`--all`) exists but is invisible to users who encounter "already fresh" output.
