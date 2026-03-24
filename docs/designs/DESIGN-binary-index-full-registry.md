---
status: Accepted
problem: |
  The binary index only covers locally-cached recipes. On a clean machine,
  tsuku which jq returns "not found" immediately after tsuku update-registry
  because no recipes are in the local cache yet. The index can't serve its
  stated purpose -- reverse command lookup for shell integration -- until the
  user has already installed every tool they might want to discover.
decision: |
  Extend the index.Registry interface with ListAll(ctx) and FetchRecipe(ctx, name)
  methods. Rebuild() calls ListAll() to enumerate all known recipes from the
  manifest (already downloaded by update-registry), then fetches uncached recipe
  TOMLs on demand using a bounded worker pool (10 concurrent fetches). Recipes
  that can't be fetched are skipped with a warning rather than aborting the rebuild.
rationale: |
  The manifest is downloaded as a side effect of every update-registry run, so
  ListAll() adds zero extra network requests for name enumeration. Fetching
  uncached recipe TOMLs is a one-time cost per recipe amortized across runs.
  Bounded concurrency prevents connection exhaustion without requiring a full
  sequential fallback. Skipping unfetchable recipes keeps the rebuild non-fatal
  while still indexing everything reachable.
upstream: docs/designs/DESIGN-binary-index.md
---

# DESIGN: Binary Index — Full Registry Coverage

## Status

Accepted

## Upstream Design Reference

Parent: [DESIGN: Binary Index](DESIGN-binary-index.md)
This design addresses a gap in the parent: the Registry interface in the parent
doc was internally inconsistent (interface declared `ListRecipes`, data flow used
`ListCached`), and the implementation used the narrower `ListCached` path.

## Context and Problem Statement

The binary index maps command names to recipes so shell integration features can
answer "which recipe provides `jq`?" without scanning hundreds of TOML files at
runtime. The index is rebuilt on every `tsuku update-registry` invocation.

The current `Rebuild()` implementation calls `reg.ListCached()`, which walks
`$TSUKU_HOME/registry/` and returns only locally-cached recipe TOMLs. On a clean
machine, that directory is empty. `update-registry` in this state prints "No
cached recipes to refresh" and calls `Rebuild()` with an empty source. The
resulting index is empty, so `tsuku which jq` returns "not found" — even though
tsuku can install jq.

This makes the command useless for its primary case: discovering what provides
an unknown command before installing it.

The registry manifest (`recipes.json`) is already downloaded by `update-registry`
as a side effect of `refreshManifest()`. It lists all recipe names. This
information is available but not used by `Rebuild()`.

## Decision Drivers

- **Correctness**: `tsuku which <command>` must return results for any installable
  command after `tsuku update-registry`, not just commands the user has already
  installed
- **No extra manifest network request**: the manifest is already downloaded; the
  fix should reuse it rather than adding a new download step
- **Interface stability**: `BinaryIndex.Rebuild()` signature must not change;
  the interface is already consumed by downstream designs
- **Rate limit safety**: 1,400+ recipe fetches on a clean machine must not hit
  GitHub raw content rate limits under typical concurrency
- **Non-fatal on fetch failure**: individual recipe fetch errors must not abort
  the entire rebuild; partial coverage is better than no coverage

## Considered Options

### Decision 1: How should Rebuild() learn about all registry recipes?

The core choice is where the full recipe list comes from and when individual
recipe TOMLs are fetched for recipes not yet in the local cache.

#### Chosen: Manifest-driven enumeration with on-demand fetch (Option B)

Extend `index.Registry` with two new methods:

```go
// ListAll returns all recipe names known to the registry.
// Reads from the locally-cached manifest; falls back to ListCached
// if no manifest is available (e.g., offline mode).
ListAll(ctx context.Context) ([]string, error)

// FetchRecipe fetches a recipe's raw TOML from the registry source.
// Used by Rebuild() on cache miss. Callers should try GetCached first.
FetchRecipe(ctx context.Context, name string) ([]byte, error)
```

`Rebuild()` calls `ListAll()` for the name list, `GetCached()` for cached TOMLs,
and `FetchRecipe()` (bounded to 10 concurrent workers) for uncached ones.

`*registry.Registry` already implements `GetCached` and `FetchRecipe`. A new
`ListAll()` method is added that reads `GetCachedManifest()` and returns
`manifest.Recipes[i].Name` for each entry.

#### Rejected: Eager seeding before Rebuild (Option A)

Download all 1,400+ recipe TOMLs in `update-registry` before calling `Rebuild()`,
keeping `ListCached()` as the enumeration source. Rejected because it front-loads
all fetch cost even if the user only needs a subset of the index, adds 10+ seconds
to every `update-registry` on a warm machine with no new recipes, and makes the
seeding step invisible to the user with no natural progress reporting hook.

#### Rejected: Explicit name list parameter on Rebuild (Option C)

`update-registry` reads the manifest, extracts all recipe names, and passes
`[]string` to `Rebuild()` directly. Rejected because it requires changing
`BinaryIndex.Rebuild()`'s signature, which is a public interface consumed by
downstream designs. Option B achieves the same behavior without touching the
signature.

---

### Decision 2: What happens when a recipe can't be fetched during Rebuild?

Some recipes in the manifest may be temporarily unavailable (network error, 404,
rate limit). The rebuild must decide whether to abort or continue.

#### Chosen: Skip with slog.Warn, continue rebuild

Log a warning per unfetchable recipe and continue. The index will be partial but
functional. Consistent with the existing handling of malformed TOML recipes in
`Rebuild()`.

#### Rejected: Abort on any fetch failure

Returning an error from `Rebuild()` when any recipe is unavailable causes
`update-registry` to exit non-zero. Rejected because transient network errors
during a ~1,400 recipe fetch are expected. Aborting the entire rebuild on one
failure means the index might never get built in flaky network environments.

---

### Decision 3: Concurrency bound for on-demand fetches

Fetching potentially hundreds of uncached recipes requires concurrency control
to avoid connection exhaustion and rate limit pressure.

#### Chosen: Semaphore of 10 concurrent fetches

Use a buffered channel as a semaphore with capacity 10. Provides meaningful
parallelism while keeping open connection count bounded. No retry logic added
(see Consequences — Negative).

#### Rejected: Unbounded goroutine-per-recipe

Spawning one goroutine per recipe for 1,400 items could open 1,400 simultaneous
TCP connections. Rejected because raw.githubusercontent.com rate limits are
per-IP; flooding with parallel requests risks triggering 429 responses across
the entire batch.

#### Rejected: Sequential (no concurrency)

Fetching 1,400 recipes at 100ms average RTT takes ~140 seconds. Rejected because
`update-registry` is already a network-bound operation, but a 2-minute freeze
with no output is unacceptable UX.

## Decision Outcome

**Chosen: Manifest-driven enumeration, on-demand fetch, bounded concurrency**

`Rebuild()` calls `ListAll()` to get all recipe names from the cached manifest,
then processes each name: serve from cache if available, fetch and cache if not,
skip with warning on error. Ten concurrent fetch workers limit connection pressure.

## Solution Architecture

### Overview

`Rebuild()` gains two new code paths: manifest enumeration (via `ListAll()`) and
on-demand recipe fetch (via `FetchRecipe()`). The `index.Registry` interface grows
by two methods. `*registry.Registry` implements both. No other components change.

### Components

**`internal/index/binary_index.go`** — `Registry` interface updated:

```go
type Registry interface {
    // ListAll returns all recipe names known to this registry source.
    // Reads from the manifest when available; falls back to ListCached.
    // ListCached is retained on *registry.Registry for this fallback path
    // and must not be removed even if it is no longer referenced directly.
    ListAll(ctx context.Context) ([]string, error)

    // GetCached returns raw TOML for a recipe if locally cached.
    // Returns nil, nil on cache miss.
    GetCached(name string) ([]byte, error)

    // FetchRecipe fetches a recipe TOML from the registry source.
    // Callers should try GetCached first and use FetchRecipe on cache miss.
    FetchRecipe(ctx context.Context, name string) ([]byte, error)

    // CacheRecipe writes raw TOML to the local recipe cache.
    // Called by Rebuild() after a successful FetchRecipe to avoid re-fetching
    // on subsequent update-registry runs.
    CacheRecipe(name string, data []byte) error
}
```

**`internal/registry/registry.go`** — new `ListAll` method:

```go
// ListAll returns all recipe names from the cached manifest.
// Falls back to ListCached if no manifest is cached.
func (r *Registry) ListAll(ctx context.Context) ([]string, error) {
    manifest, err := r.GetCachedManifest()
    if err != nil || manifest == nil {
        return r.ListCached()
    }
    names := make([]string, len(manifest.Recipes))
    for i, recipe := range manifest.Recipes {
        names[i] = recipe.Name
    }
    return names, nil
}
```

**`internal/index/rebuild.go`** — `Rebuild()` updated:

```go
// 1. Enumerate all recipe names
names, err := reg.ListAll(ctx)
if err != nil {
    return fmt.Errorf("index: list recipes: %w", err)
}

// 2. Fetch/cache content for each recipe (bounded concurrency)
sem := make(chan struct{}, 10)
var mu sync.Mutex
type result struct{ name string; data []byte }
results := make([]result, 0, len(names))

var wg sync.WaitGroup
for _, name := range names {
    wg.Add(1)
    go func(n string) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()

        data, err := reg.GetCached(n)
        if err != nil || len(data) == 0 {
            data, err = reg.FetchRecipe(ctx, n)
            if err != nil {
                slog.Warn("index rebuild: fetch recipe", "recipe", n, "err", err)
                return
            }
            // Cache the fetched TOML so subsequent runs are cache hits.
            if cacheErr := reg.CacheRecipe(n, data); cacheErr != nil {
                slog.Warn("index rebuild: cache recipe", "recipe", n, "err", cacheErr)
            }
        }
        mu.Lock()
        results = append(results, result{n, data})
        mu.Unlock()
    }(name)
}
wg.Wait()

// 3. Insert rows within existing transaction (unchanged from current)
```

### Data Flow

**Rebuild on clean machine (first run)**:
```
update-registry
  └─ refreshManifest()          → downloads recipes.json, caches it
  └─ rebuildBinaryIndex()
       └─ reg.ListAll(ctx)      → reads cached manifest → 1,400 names
       └─ reg.GetCached(name)   → cache miss for all (clean machine)
       └─ reg.FetchRecipe(ctx)  → fetches TOML, 10 concurrent workers
                                  → caches result for future runs
       └─ extractBinaries()     → existing logic, unchanged
       └─ INSERT INTO binaries  → existing logic, unchanged
```

**Rebuild on warm machine (subsequent runs)**:
```
update-registry
  └─ refreshManifest()          → refreshes recipes.json if stale
  └─ rebuildBinaryIndex()
       └─ reg.ListAll(ctx)      → reads cached manifest → 1,400 names
       └─ reg.GetCached(name)   → cache hit for most recipes
       └─ reg.FetchRecipe(ctx)  → only for newly-added recipes since last run
```

## Implementation Approach

### Phase 1: Full registry coverage in Rebuild

- Add `ListAll(ctx context.Context) ([]string, error)` to `index.Registry` interface
- Add `FetchRecipe(ctx context.Context, name string) ([]byte, error)` to `index.Registry` interface
- Implement `ListAll()` on `*registry.Registry` using `GetCachedManifest()`
- Update `Rebuild()` to call `ListAll()` instead of `ListCached()`, with bounded-concurrency on-demand fetch
- Update test stubs in `rebuild_test.go` to implement the two new interface methods
- Acceptance criteria:
  - `Rebuild()` against a mock registry with an empty cache but a populated manifest produces rows for all manifest recipes
  - `Rebuild()` against a mock where some recipes return fetch errors skips those recipes and continues
  - `Rebuild()` on a fully-cached registry (warm machine) uses only `GetCached()`, never `FetchRecipe()`
  - `Rebuild()` calls `CacheRecipe()` after each successful `FetchRecipe()` call; a subsequent `Rebuild()` call with the same mock produces no `FetchRecipe()` calls
  - `TestRebuild_Transactional` is updated or replaced: cache-miss no longer aborts `Rebuild()`; a fetch error on one recipe must not roll back rows already inserted for other recipes
  - Recipe names from `ListAll()` are validated before passing to `recipeURL()`: names containing `/`, `..`, or null bytes are rejected with `slog.Warn` and skipped
  - `fetchRemoteRecipe` wraps `resp.Body` with `io.LimitReader(resp.Body, 1<<20)` before `io.ReadAll`
  - `go test ./internal/index/... ./internal/registry/...` passes
  - `go build ./cmd/tsuku` passes

## Security Considerations

The new `FetchRecipe()` call path downloads recipe TOML files from the registry
URL (`https://raw.githubusercontent.com/tsukumogami/tsuku/main` by default, or
`TSUKU_REGISTRY_URL` if set). This is the same trust boundary as the existing
`update-registry` network operations. Two new concerns arise from the `ListAll()`
path: recipe name validation (path traversal risk in local-registry deployments)
and the unbounded `io.ReadAll` call in `fetchRemoteRecipe` (memory exhaustion
risk during cold-start burst fetches). Both are addressed as acceptance criteria
in the implementation phase.

**Recipe name validation**: `ListAll()` returns names directly from the manifest.
Before any name is passed to `recipeURL()` (which constructs the fetch path or
local filesystem path), it must be validated to reject names containing `/`, `..`,
or null bytes. Without this check, a local-registry deployment (`TSUKU_REGISTRY_URL`
pointing to a local path) would compose an `os.ReadFile` argument from an
attacker-supplied manifest entry, enabling path traversal outside
`$TSUKU_HOME/registry/`. Validation is an acceptance criterion for the implementation.

**Recipe content trust**: recipe TOMLs are parsed by `extractBinariesFromMinimal`,
which only reads specific TOML keys and does not execute any recipe logic. A
malicious TOML in the registry cannot cause code execution during index rebuild.
The binary index stores recipe names and binary paths — both are public registry
data — no credentials, tokens, or user data.

**TLS verification**: `FetchRecipe()` uses the existing `registry.Registry` HTTP
client, which uses Go's default TLS stack with full certificate chain and hostname
verification. Transport security is inherited from existing recipe fetch paths.

**Response size cap**: the implementation must wrap `resp.Body` with
`io.LimitReader(resp.Body, 1<<20)` (1 MiB) before `io.ReadAll` in
`fetchRemoteRecipe`. Without this, a malicious or misconfigured server could
return an arbitrarily large body; with 10 concurrent fetches during cold-start,
this is a memory exhaustion risk. This is an acceptance criterion for the
implementation, not a design-level concern.

**Redirect behavior**: Go's `http.Client` follows cross-origin redirects by
default. The default registry URL (`raw.githubusercontent.com`) is unaffected
in practice, but deployments using `TSUKU_REGISTRY_URL` with an HTTP endpoint
should use HTTPS to avoid redirect-based downgrade attacks.

**Rate limiting gap**: no retry or backoff is implemented for 429 responses
during bulk fetch. Hitting a rate limit skips affected recipes (non-fatal);
a subsequent `update-registry` retries them. A future improvement would add
`Retry-After` backoff to `FetchRecipe()`.

**`TSUKU_REGISTRY_URL` trust**: if a user points `TSUKU_REGISTRY_URL` at an
attacker-controlled server, `FetchRecipe()` downloads attacker-supplied TOML.
This is an existing concern for all recipe fetch operations — the new fetch
volume amplifies it. Treat `TSUKU_REGISTRY_URL` as a sensitive configuration
value in shared environments.

## Consequences

### Positive

- `tsuku which <command>` returns results for any installable command after a
  single `tsuku update-registry`, even on a clean machine
- Recipe TOML files are cached as a side effect of the first rebuild, making
  subsequent installs faster (cache hit) and subsequent rebuilds cheap
- The fix is contained to two packages (`internal/index`, `internal/registry`)
  with no cmd/ changes beyond inheriting the extended interface

### Negative

- **First-run latency**: a clean machine fetches ~1,400 recipe TOMLs during the
  first `update-registry`. With 10 concurrent workers at ~100ms RTT, this adds
  roughly 15-20 seconds to the first run
- **No retry on 429**: if the registry rate-limits the bulk fetch, affected
  recipes are silently skipped. The index will be partial until the next
  `update-registry` succeeds for those recipes
- **Interface growth**: `index.Registry` grows from 2 to 5 methods (`ListAll`,
  `FetchRecipe`, `CacheRecipe` are new); test stubs need updating
- **Test churn**: `TestRebuild_Transactional` must be replaced; the new
  non-fatal error model changes what "transactional" means for `Rebuild()`

### Mitigations

- **First-run latency**: add a progress message ("Indexing registry recipes...
  N/1400") so users know the command is working. This is a UX improvement that
  can land in the same issue
- **No retry**: document as a known gap; add a follow-up issue for retry/backoff
  in `FetchRecipe()`
- **Interface growth**: `ListAll` is new; `FetchRecipe` and `CacheRecipe` already
  exist on `*registry.Registry`. Test stubs need three new method stubs, each
  trivially implemented with fixed return values
- **Test churn**: replace `TestRebuild_Transactional` with two targeted tests:
  one verifying partial-failure non-fatality, one verifying atomicity of the
  insert phase (all-or-nothing on DB write errors)

