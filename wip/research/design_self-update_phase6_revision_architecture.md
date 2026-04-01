# Architecture Review: DESIGN-self-update.md (Revised -- Auto-Apply Default)

**Reviewer**: architect-reviewer
**Focus**: Structural fit of auto-apply self-update during background check
**Revision**: Second pass after design revision (auto-apply now default, not opt-in)

## Summary

The design is architecturally sound. It correctly keeps self-update out of the recipe pipeline (D5), creates a contained parallel path with clear boundaries, and reuses the existing cache infrastructure via a well-known constant rather than a separate schema. The `checkAndApplySelf`/`applySelfUpdate` split, the `MaybeAutoApply` skip guard, and the placement within `RunUpdateCheck` all fit the existing codebase structure.

One finding downgraded from blocking to advisory on closer inspection. Four advisory findings total, zero blocking.

---

## Finding 1: Notification mechanism introduces a second one-shot display path (Advisory)

**Location**: Design Decision 3 -- `.self-update-applied` file in `$TSUKU_HOME/cache/updates/`

**What exists**: `internal/notices/` provides structured one-shot notifications: JSON files in `$TSUKU_HOME/notices/`, with `WriteNotice`/`ReadUnshownNotices`/`MarkShown`. `MaybeAutoApply` uses this for failure notifications (`apply.go:93-100`), and `displayUnshownNotices` reads them during the `PersistentPreRun` flow (`apply.go:127-138`).

**What the design adds**: A plain-text `.self-update-applied` file in the cache directory, read and removed by custom code in `PersistentPreRun`. This is a second mechanism for the same concern (deferred user notification).

**Why advisory, not blocking**: The `.self-update-applied` file has exactly one writer (`checkAndApplySelf`) and one reader (`PersistentPreRun`). It represents a structurally different event -- a *success* notification for a binary replacement, not a failure notice for a managed tool. The existing `Notice` struct is failure-specific (`Error` field is semantically required). No other code will need to produce or consume `.self-update-applied`, so the pattern won't be copied.

**Recommendation**: Consider extending the `notices` package with a `Type` field (`"info"` vs `"error"`) and routing self-update success through it. This keeps one notification pipeline. But the current approach is contained and won't cause drift.

---

## Finding 2: checkAndApplySelf signature doesn't match described config gating (Advisory)

**Location**: Design "Key Interfaces" section -- `checkAndApplySelf(ctx, cacheDir, resolver, exePath)`

The design says `checkAndApplySelf` checks `userCfg.UpdatesSelfUpdate()` and returns early if disabled (Decision 3, point 1). But the signature doesn't accept `userCfg`. Either:

- The config check happens in `RunUpdateCheck` before calling `checkAndApplySelf` (cleaner -- matches how `RunUpdateCheck` already gates on `userCfg.UpdatesEnabled()`), or
- The function signature needs a `userCfg` parameter.

The data flow diagram shows the check inside `checkAndApplySelf`. Align the signature with whichever approach is chosen.

---

## Finding 3: MaybeAutoApply skip guard is defense-in-depth, not load-bearing (Advisory)

**Location**: Design "Components" -- `apply.go` modification adding `if entry.Tool == SelfToolName { continue }`

The current `MaybeAutoApply` filter (`apply.go:46`) requires `LatestWithinPin != ""` for an entry to be actionable. The tsuku cache entry won't populate `LatestWithinPin` (no pin concept), so it would already be filtered out. The explicit `SelfToolName` guard is correct as defense-in-depth -- if someone later adds `LatestWithinPin` to the tsuku entry (e.g., for update channels), the guard prevents `MaybeAutoApply` from routing tsuku through the recipe pipeline.

No change needed. Worth noting in the design that the guard is defensive, not the primary filter.

---

## Finding 4: Self-update download extends the check lock hold time (Advisory)

**Location**: Interaction between `RunUpdateCheck` lock (`checker.go:36-40`) and `checkAndApplySelf` placement

`RunUpdateCheck` holds an exclusive flock on `.lock` for the entire duration of the tool loop *and* the `checkAndApplySelf` call. If the self-update download is slow (large binary, slow network), it extends how long the check lock is held. This blocks concurrent `tsuku check-updates` spawns from proceeding -- they hit the non-blocking `TryLockExclusive` in `trigger.go:40` and silently skip.

No deadlock or user-visible impact. The practical effect is that a slow self-update download extends the window during which other terminals' check-update attempts are suppressed. Since the sentinel is only touched after `checkAndApplySelf` completes, those skipped checks will retry on the next invocation.

**Recommendation**: Consider calling `TouchSentinel` before `checkAndApplySelf` so that tool-check freshness isn't blocked by self-update I/O. The sentinel's purpose is "have we checked tool versions recently" -- the self-update is a separate concern.

---

## Structural Assessment

| Concern | Verdict |
|---------|---------|
| Action dispatch | Not applicable -- self-update correctly avoids the recipe/action pipeline per D5 |
| Provider usage | Clean -- uses `GitHubProvider` directly, correct since there's no recipe |
| State contract | No `state.json` changes -- tsuku stays out of managed tool state, cache layer only |
| CLI surface | New `self-update` subcommand, no overlap. Skip-list addition correct |
| Dependency direction | `internal/updates/self.go` -> `internal/version`, `internal/httputil`, `internal/buildinfo` -- all same-level or lower |
| Cache integration | Reuses `UpdateCheckEntry` and `WriteEntry`/`ReadAllEntries` with `SelfToolName` constant |
| Parallel patterns | Finding 1 (notification) is the only concern; contained, won't compound |

## Verdict

No blocking findings. Four advisory findings -- the most actionable are the notification mechanism (Finding 1) and the lock hold time (Finding 4). The design fits the existing architecture.
