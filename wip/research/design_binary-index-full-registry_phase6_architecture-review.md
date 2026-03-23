# Architecture Review: DESIGN-binary-index-full-registry

Reviewer: tsukumogami-architect-reviewer
Date: 2026-03-23

---

## 1. Is the architecture clear enough to implement?

Yes, with one gap.

The design is implementable as written. The data flow diagrams, interface snippets, and the
`Rebuild()` pseudocode in the Solution Architecture section are specific enough to guide a
single-pass implementation. The phasing is a single phase, so there is no sequencing ambiguity.

**Gap — FetchRecipe does not cache.** The design states that fetched TOMLs "caches result for
future runs" in the clean-machine data flow diagram, and the Consequences section lists "Recipe
TOML files are cached as a side effect of the first rebuild" as a positive. However, the
proposed `Rebuild()` pseudocode calls `reg.FetchRecipe(ctx, n)` and uses the returned bytes
directly — it never calls `reg.CacheRecipe(name, data)`. The current `fetchRemoteRecipe`
implementation in `registry.go` also does not write to the cache; only `CacheRecipe()` does
that. The design claims caching as a benefit but the proposed code does not deliver it.

**Impact:** Subsequent warm-machine runs will continue to fetch every recipe that was fetched in
the first run, rather than serving from cache. The warm-machine data flow diagram ("cache hit for
most recipes") is incorrect for any recipe whose TOML was not cached before this feature shipped.

**Required fix:** After a successful `FetchRecipe()` call, `Rebuild()` (or `FetchRecipe` itself)
must call `reg.CacheRecipe(name, data)`. The `index.Registry` interface needs either a
`CacheRecipe` method or `FetchRecipe` must be documented to cache as a side effect. The simplest
fix is to have `Rebuild()` call `CacheRecipe` after each successful fetch; this requires adding
`CacheRecipe(name string, data []byte) error` to the `index.Registry` interface.

---

## 2. Are there missing components or interfaces?

**Missing: `CacheRecipe` on `index.Registry`** (see above — blocking for correctness of the
warm-machine path).

**Missing: `ListAll` context threading.** The proposed `ListAll` reads `GetCachedManifest()`,
which takes no `context.Context`. The context is passed to `ListAll` but silently dropped. For
the current implementation this is fine (manifest reads are local disk), but the signature
advertises context support it doesn't use. This is advisory, not blocking.

**Present but undocumented: `ListCached` is retained on `*registry.Registry`.** The design
removes `ListCached` from the `index.Registry` interface (the updated interface in the Solution
Architecture section does not include it). `ListCached` still exists on `*registry.Registry` and
is the fallback inside `ListAll`. This is correct — `ListCached` is still used internally — but
the design should note it explicitly to prevent a future reviewer from removing it as dead code.

**Correct: `FetchRecipe` already exists.** The design accurately states that `*registry.Registry`
already implements `FetchRecipe`. The only new method needed on `*registry.Registry` is `ListAll`.

**Correct: transaction boundary.** The design proposes fetching all recipes concurrently
(outside the transaction) and then inserting within the existing transaction. This matches the
current `Rebuild()` structure, where `BeginTx` is called after the `ListCached` call. The
concurrent fetch block in the pseudocode runs before `BeginTx`, so the goroutines collect raw
TOML bytes into `results`, and the sequential insert loop runs inside the transaction. This
preserves the atomicity guarantee: a mid-run failure still rolls back cleanly.

---

## 3. Are the implementation phases correctly sequenced?

There is only one phase, and it is self-contained:

1. Extend `index.Registry` interface (add `ListAll`, `FetchRecipe`, and — if the caching gap
   is addressed — `CacheRecipe`).
2. Implement `ListAll` on `*registry.Registry`.
3. Update `Rebuild()` to call `ListAll`, `GetCached`, `FetchRecipe`, and `CacheRecipe`.
4. Update `stubRegistry` in `rebuild_test.go` to implement the new interface methods.

Steps 1–4 are a single atomic change with no hidden dependency on any other in-flight issue.

One ordering note: `TestRebuild_Transactional` currently expects `Rebuild()` to return an error
when `GetCached` fails. Under the new design, a `FetchRecipe` failure is non-fatal (skip with
warning), but the test is testing a `GetCached` failure path that no longer applies — names come
from `ListAll`, and the cache-miss path goes to `FetchRecipe` rather than erroring. The
acceptance criteria in the design do not mention updating or replacing this test. The implementer
must either delete it or replace it with a test that verifies that `Rebuild()` still returns an
error only when the initial `ListAll()` call fails.

---

## 4. Are there simpler alternatives overlooked?

The design correctly rejected Option A (eager seeding) and Option C (signature change). The
chosen design is structurally minimal. However, one simplification is available:

**Simpler: move fetch-and-cache into `FetchRecipe` rather than splitting across `Rebuild`.**
If `FetchRecipe` is defined to always cache on success (matching the pattern of
`FetchDiscoveryEntry`, which does cache), the `index.Registry` interface needs no `CacheRecipe`
method. `Rebuild()` calls `FetchRecipe`, gets bytes back, and the caching is an internal detail
of `*registry.Registry`. This avoids widening the `index.Registry` interface by one more method.
The tradeoff is that test stubs must decide whether to simulate caching side effects, which they
currently don't need to. On balance, caching inside `FetchRecipe` is the cleaner factoring and
matches the existing `FetchDiscoveryEntry` precedent in the same file.

---

## Structural Verdict

**One blocking finding**: The cache-miss path after `FetchRecipe` does not write to the local
cache, contradicting the design's stated "cached as a side effect" benefit. Fix before
implementation by adding cache-write either inside `FetchRecipe` (preferred, matches
`FetchDiscoveryEntry`) or in `Rebuild()` after each successful fetch.

**One test gap**: `TestRebuild_Transactional` tests a behavior the new design changes. The
acceptance criteria must explicitly address what replaces it.

All other structural elements — interface growth, dependency boundaries, transaction placement,
concurrency pattern, fallback to `ListCached` on no-manifest — are correct and consistent with
the patterns established in Issues 1–3.

---

## Interface Summary (as the design intends it after fix)

```go
// index.Registry — updated
type Registry interface {
    ListAll(ctx context.Context) ([]string, error)       // NEW
    GetCached(name string) ([]byte, error)               // existing
    FetchRecipe(ctx context.Context, name string) ([]byte, error)  // NEW on interface; already on *registry.Registry
    // Option A: add CacheRecipe here
    // Option B (preferred): FetchRecipe caches internally; no CacheRecipe on interface
}
```

```go
// registry.Registry — new method only
func (r *Registry) ListAll(ctx context.Context) ([]string, error)
// FetchRecipe: already exists; add CacheRecipe call at end of fetchRemoteRecipe (if Option B)
```
