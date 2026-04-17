# Phase 3 Review: User Perspective

## 1. Does the research change your understanding of the problem?

**Yes, in two meaningful ways.**

**Clearer root cause.** Phase 1 identified that `RefreshAll` uses a time-based TTL and skips in-window entries. The research confirms this precisely: `isFresh()` at `internal/registry/cached_registry.go:195` is pure clock math -- `CachedAt + TTL > now`. No content comparison. No remote signal. The `ContentHash` field in `CacheMetadata` exists but is never compared against the upstream TOML during `RefreshAll`. The fix merged in PR #2255 was live on `main` immediately; nothing on the client side could have known.

**Security dimension is broader than the functional failure.** The infisical case was a segfault, not a supply-chain issue. But the research makes clear that the same TTL bypass applies to recipe fixes that correct tampered download URLs or wrong checksums. If a recipe fix corrects a malicious source, users who cached the recipe within the last 24 hours will continue installing from the bad source even after running `update-registry` -- and the "already fresh" output will tell them nothing is wrong. The stale-fallback window (`maxStale = 7 * 24 * time.Hour`) extends exposure for users with intermittent connectivity. This is worth a sentence in the issue -- not to frame this as a security bug per se, but to communicate why the behavior matters beyond user frustration.

Understanding is otherwise unchanged: the bug is in `RefreshAll`, the workaround is `--all` or `--recipe <name>`, and neither is surfaced to users who hit "already fresh."

---

## 2. Is this a duplicate of an existing issue?

**No.** The research found no existing open issues covering this scenario. The TTL behavior has not been filed before.

---

## 3. Is the scope still appropriate for a single issue?

**Yes.** The bug is in one function (`RefreshAll`) with a clear behavioral contract mismatch: an explicit `update-registry` invocation should not silently skip recipes because they were recently cached. Both the minimal fix (surface `--all` in the "already fresh" output) and the proper fix (bypass TTL for explicit user invocations) fit a single issue without requiring architectural changes.

The security dimension (content-hash comparison to detect upstream changes) would be a second, follow-on issue. It should not be merged into this one -- the risk is expanding scope such that neither fix ships quickly. A note in the issue that links that opportunity as a "see also" is appropriate.

---

## 4. Are we ready to draft?

**Yes.** No blockers. The following are confirmed and can be stated in the issue:

- Exact code location of the bug (`cached_registry.go:334`, `isFresh` at line 195)
- The workaround (`--all` or `--recipe <name>`)
- Why the workaround is invisible to users
- Two fix directions (minimal output hint vs. behavioral change to make force-refresh the default for explicit invocations)
- The `ContentHash` field that already exists in metadata but is unused for invalidation

The only minor gap: we don't know the exact tsuku version the user was running. That's not blocking -- the root cause is version-independent.

---

## 5. Context from research to include in the issue

**Include:**

- The TTL mechanism: `isFresh()` is time-only; it does not compare local cached content against the upstream registry. A recipe merged 10 minutes ago looks identical to one merged 6 months ago as far as the cache is concerned.
- The window: default TTL is 24 hours (`config.DefaultRecipeCacheTTL`). Users who installed or refreshed infisical recently (precisely the users most likely to be affected by a bug fix) are the ones most likely to hit this.
- The workaround: `tsuku update-registry --all` forces a full refresh regardless of TTL. `tsuku update-registry --recipe infisical` refreshes a single recipe. Either would have fetched the fixed TOML.
- Why the workaround is not reached: the "already fresh" output implies correctness, not staleness. No hint of `--all` is emitted. A user who just ran `update-registry` and saw a clean result has no reason to try again with a flag.
- The `ContentHash` field: `CacheMetadata` already stores a SHA-256 of the cached TOML. A future improvement could compare this against a hash published in the manifest (or fetched live) to detect content changes independent of TTL. This is noted as a follow-on; it does not block this fix.
- Security note (brief): for recipe fixes that correct download URLs or verification data, not just functional bugs, this cache behavior means users may continue installing from an incorrect source after the fix lands. "Already fresh" is a misleading signal in that context.

**Omit:**

- Internal command names (`/tsukumogami:explore`, etc.)
- The stale-fallback 7-day window as a headline -- it is real but is a separate concern from the explicit `update-registry` call the issue is about. Mention only in context.
- Implementation details of the content-hash manifest approach -- that belongs in a follow-on issue or design.
