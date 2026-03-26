---
status: Current
problem: |
  Tsuku's shell integration features (command-not-found suggestions, auto-install)
  require knowing which recipe provides a given command name. No such reverse lookup
  exists today. Every candidate lookup would require scanning all recipes, which
  violates the 50ms shell-integration budget. Additionally, a cache-only approach
  leaves the index empty on a clean machine until recipes happen to be cached,
  making the feature useless for its primary purpose: discovering installable commands
  before you've installed them.
decision: |
  Build a SQLite-backed binary index at $TSUKU_HOME/cache/binary-index.db, rebuilt
  on every `update-registry` invocation. The index.Registry interface provides
  ListAll() (reads the cached manifest for all recipe names), GetCached() (returns
  locally-cached TOML), FetchRecipe() (fetches uncached TOMLs on demand), and
  CacheRecipe() (writes fetched TOMLs to the local cache). Rebuild() uses ListAll()
  to enumerate all known recipes, serves cached ones immediately, and fetches the
  rest using 10 concurrent workers. Fetch errors skip the affected recipe with a
  warning rather than aborting the rebuild. The installed flag is updated
  incrementally on install and remove without requiring a full rebuild.
rationale: |
  SQLite with a single-column index on command name achieves sub-millisecond lookups
  well within the 50ms budget. The manifest is already downloaded by update-registry,
  so ListAll() adds zero extra network requests for name enumeration. Fetching
  uncached TOMLs is a one-time amortized cost; subsequent runs are cache hits. Ten
  concurrent fetch workers provide meaningful parallelism without exhausting
  connections or triggering rate limits. Skipping unfetchable recipes keeps the
  rebuild non-fatal while still indexing everything reachable.
spawned_from:
  issue: 1677
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
---

# DESIGN: Binary Index

## Status

Current

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Relevant sections: Block 1 (Binary Index), Decision 3 (Binary Index Strategy), Key Interfaces.

## Context and Problem Statement

Tsuku's shell integration vision requires a reverse lookup from command name to
recipe. When a user types an unknown command like `jq`, the command-not-found
handler needs to know that recipe "jq" provides it. When the auto-install flow
receives `tsuku run jq .foo data.json`, it needs to locate the recipe without
requiring the user to know the recipe name.

Today, tsuku has the raw data needed for this index:

- `Recipe.ExtractBinaries()` scans a recipe and returns the binary names it
  provides, checking `metadata.binaries` first then action parameters
- `VersionState.Binaries` tracks which binary paths each installed version
  provides, populated at install time

But there's no index. Looking up which recipe provides "jq" would require calling
`ExtractBinaries()` on every recipe in the registry — hundreds of files for a
scan that must complete in under 50ms for interactive shell use.

A cache-only approach makes the problem worse: `$TSUKU_HOME/registry/` is empty
on a clean machine, so an index built from `ListCached()` would be empty too.
`tsuku which jq` would return "not found" even though tsuku can install jq. The
registry manifest (`recipes.json`) is already downloaded by `update-registry` and
lists all recipe names; using it to drive the rebuild closes this gap.

The binary index solves both problems by pre-computing the command-to-recipe
mapping at registry-update time from all known recipes (not just locally-cached
ones) and storing it in a queryable format.

### Scope

**In scope:**
- SQLite index at `$TSUKU_HOME/cache/binary-index.db`
- Index populated from all recipes in the registry manifest
- Hybrid index: installed tools supplement the registry for custom/distributed recipes
- `tsuku which <command>` CLI command
- `BinaryIndex` Go interface and `BinaryMatch` struct
- Conflict resolution (multiple recipes providing the same command)
- Error handling: missing, stale, and corrupt index

**Out of scope:**
- LLM recipe discovery (future Block 6+)
- Project config version pinning (Block 4)
- The command-not-found shell handler itself (Block 2, issue #1678)
- The auto-install flow (Block 3, issue #1679)

## Decision Drivers

- **50ms lookup budget**: shell-integrated operations must complete under 50ms;
  index lookups must be well within this to leave room for surrounding logic
- **Correctness on clean machine**: `tsuku which <command>` must return results
  for any installable command after `tsuku update-registry`, not just commands
  the user has previously installed or cached
- **No extra manifest network request**: the manifest is already downloaded;
  registry enumeration should reuse it rather than adding a separate fetch
- **Interface stability**: `BinaryIndex.Rebuild()` signature must not change;
  the interface is consumed by downstream designs
- **Rate limit safety**: fetching 1,400+ recipe TOMLs on a clean machine must
  not hit GitHub raw content rate limits under typical concurrency
- **Non-fatal on fetch failure**: individual recipe fetch errors must not abort
  the entire rebuild; partial coverage is better than no coverage
- **Conflict correctness**: multiple recipes may provide the same command name;
  the resolution policy must be deterministic and intuitive
- **Offline operation**: the index must work without network access after build
- **Freshness**: the index must stay in sync after `update-registry`; stale
  lookups that suggest the wrong recipe erode user trust
- **Custom recipe support**: locally-installed or distributed recipes not in the
  central registry must also be discoverable

## Considered Options

### Decision 1: Storage Format and Rebuild Strategy

The index needs to be built from recipe data, stored persistently, and queried
at runtime in under 50ms. The key choices are storage format and whether rebuilds
are incremental or full.

**Key assumptions:**
- The registry contains hundreds of recipes, not tens of thousands
- `update-registry` is an infrequent operation (minutes between calls at most)
- The index file will be under 10MB for a typical registry

#### Chosen: SQLite with Full Rebuild on update-registry

Store the index in a SQLite database at `$TSUKU_HOME/cache/binary-index.db`.
Rebuild the entire index on every `update-registry` call.

Schema:

```sql
CREATE TABLE IF NOT EXISTS binaries (
    command    TEXT NOT NULL,
    recipe     TEXT NOT NULL,
    binary_path TEXT NOT NULL,  -- e.g., "bin/jq" (relative to tool dir)
    source     TEXT NOT NULL,   -- "registry", "installed"
    installed  INTEGER NOT NULL DEFAULT 0,  -- 1 if any version installed
    PRIMARY KEY (command, recipe)
);
CREATE INDEX IF NOT EXISTS idx_command ON binaries(command);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Populated by Rebuild(): INSERT OR REPLACE INTO meta VALUES ('built_at', '<RFC3339>')
```

Rebuild procedure:
1. Begin transaction
2. DELETE all rows from binaries
3. For each recipe name from `ListAll()`: fetch TOML (from cache or network), extract
   binary paths, insert rows with `source = 'registry'`
4. For installed tools not covered by the registry pass: insert rows with
   `source = 'installed'` from `VersionState.Binaries`
5. INSERT OR REPLACE `built_at` into meta
6. Commit

The `installed` flag is updated on every install/remove operation (not just
registry rebuilds) to keep it current.

#### Alternatives Considered

**JSON file at `$TSUKU_HOME/cache/binary-index.json`:**
A JSON object mapping command name to `[]BinaryMatch`. Simple to write, no
dependencies. Rejected because JSON doesn't support indexed lookup — reading
the file and deserializing for every lookup would add 5-20ms per query and
consume memory proportional to index size. For lookup performance, SQLite's
indexed access is clearly superior.

**Incremental rebuild (diff registry, update changed recipes):**
Track recipe content hashes and only re-extract binaries when a recipe file
changes. Rejected because the implementation complexity isn't justified — full
rebuild of hundreds of recipes takes under 1 second and `update-registry` is
already a network-fetching operation where a second of local processing is
imperceptible. Incremental logic would require cache invalidation for renamed
or deleted recipes and adds surface area for subtle bugs.

**In-memory index (no persistence):**
Build the index in memory at process startup, skip persistence. Rejected because
it reintroduces the startup scan cost for every tsuku invocation, including
shell-integrated ones. The whole point of the index is to amortize the scan
cost across infrequent rebuilds.

---

### Decision 2: How Rebuild() Learns About All Registry Recipes

The core choice is where the full recipe list comes from and when individual
recipe TOMLs are fetched for recipes not yet in the local cache.

#### Chosen: Manifest-Driven Enumeration with On-Demand Fetch

Extend `index.Registry` with two new methods beyond `GetCached`:

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

`*registry.Registry` implements `ListAll()` by reading `GetCachedManifest()` and
returning `manifest.Recipes[i].Name` for each entry, falling back to `ListCached()`
if no manifest is cached.

#### Rejected: Eager Seeding Before Rebuild

Download all 1,400+ recipe TOMLs in `update-registry` before calling `Rebuild()`,
keeping `ListCached()` as the enumeration source. Rejected because it front-loads
all fetch cost even if the user only needs a subset of the index, adds time to
every `update-registry` on a warm machine with no new recipes, and makes the
seeding step invisible to the user with no natural progress reporting hook.

#### Rejected: Explicit Name List Parameter on Rebuild

Pass `[]string` recipe names directly to `Rebuild()`. Rejected because it requires
changing `BinaryIndex.Rebuild()`'s signature, which is a public interface consumed
by downstream designs. The chosen approach achieves the same behavior without
touching the signature.

---

### Decision 3: Conflict Resolution Policy

Multiple recipes may provide the same command name. For example, both `vim` and
`neovim` may provide a `vi` binary. The index must define how to rank and select
among them.

**Key assumptions:**
- Most command names map to exactly one recipe (no conflict is the common case)
- Users who have installed a recipe that provides a command care most about that
  recipe, not alternatives
- Stable, deterministic ordering matters for reproducibility across machines

#### Chosen: Installed-First, Lexicographic Tiebreaker, All Matches Returned

Return all matching `BinaryMatch` results, ranked by:

1. **Installed recipes first**: if the user has any version installed, that recipe
   ranks above uninstalled ones
2. **Lexicographic tiebreaker**: among recipes in the same installation tier,
   sort by recipe name alphabetically for stable ordering

The `BinaryIndex.Lookup()` method returns a ranked `[]BinaryMatch` slice. The
first element is the "preferred" match. Callers that want exactly one result take
`[0]`; callers that want to show all options (like `tsuku suggest`) iterate the
full slice.

This approach defers the "which one to install" decision to callers that have
more context (e.g., project config, user preference flags).

#### Alternatives Considered

**Return only the top-ranked single match:**
`Lookup()` returns `(*BinaryMatch, error)` with one winner. Simpler caller API.
Rejected because it prevents `tsuku suggest` from showing all candidates and
prevents future callers from implementing their own ranking without bypassing
the index. The cost of returning a slice is minimal.

**Prompt the user at lookup time:**
When conflicts exist, ask the user to choose interactively. Rejected because the
binary index is a library component used by shell hooks where interactive prompts
are inappropriate. Conflict resolution via prompt belongs in the caller (Block 2
or Block 3), not the index.

**Recipe metadata "canonical" flag:**
Add a `canonical = true` field to recipe TOML marking the preferred provider for
a command. Rejected because it requires recipe authors to coordinate, creates
maintenance burden, and is brittle when new recipes are added. The installed-first
heuristic captures the most important signal (what the user has already chosen)
without requiring coordination.

---

### Decision 4: Behavior When a Recipe Can't Be Fetched During Rebuild

Some recipes in the manifest may be temporarily unavailable (network error, 404,
rate limit). The rebuild must decide whether to abort or continue.

#### Chosen: Skip with slog.Warn, Continue Rebuild

Log a warning per unfetchable recipe and continue. The index will be partial but
functional. Consistent with the handling of malformed TOML recipes in `Rebuild()`.

#### Rejected: Abort on Any Fetch Failure

Returning an error from `Rebuild()` when any recipe is unavailable causes
`update-registry` to exit non-zero. Rejected because transient network errors
during a ~1,400 recipe fetch are expected. Aborting the entire rebuild on one
failure means the index might never get built in flaky network environments.

---

### Decision 5: Concurrency Bound for On-Demand Fetches

Fetching potentially hundreds of uncached recipes requires concurrency control
to avoid connection exhaustion and rate limit pressure.

#### Chosen: Semaphore of 10 Concurrent Fetches

Use a buffered channel as a semaphore with capacity 10. Provides meaningful
parallelism while keeping open connection count bounded. No retry logic added
(see Consequences — Negative).

#### Rejected: Unbounded Goroutine-Per-Recipe

Spawning one goroutine per recipe for 1,400 items could open 1,400 simultaneous
TCP connections. Rejected because raw.githubusercontent.com rate limits are
per-IP; flooding with parallel requests risks triggering 429 responses across
the entire batch.

#### Rejected: Sequential (No Concurrency)

Fetching 1,400 recipes at 100ms average RTT takes ~140 seconds. Rejected because
`update-registry` is already a network-bound operation, but a 2-minute freeze
with no output is unacceptable UX.

---

### Decision 6: BinaryIndex Interface and Error Handling

The Go interface must be stable enough for downstream designs (#1678, #1679) to
proceed. It must also handle the index being absent (first run), stale (registry
updated but index not rebuilt), or corrupt.

#### Chosen: Interface with Lazy Fallback on Missing Index

```go
// BinaryIndex provides command-to-recipe lookup.
type BinaryIndex interface {
    // Lookup returns recipes that provide the given command, ranked by preference
    // (installed recipes first, then lexicographic). Returns empty slice if no
    // match found. Never returns an error for "not found" -- use len(result) == 0.
    Lookup(ctx context.Context, command string) ([]BinaryMatch, error)

    // Rebuild regenerates the index from the recipe registry and installed state.
    // Safe to call concurrently -- uses a write lock internally.
    Rebuild(ctx context.Context, registry Registry, state StateReader) error

    // SetInstalled updates the installed flag for a single recipe without a
    // full rebuild. Called by install.Manager on install and remove.
    // installed=true: set on successful install.
    // installed=false: set only when ActiveVersion becomes empty (RemoveAllVersions),
    //   not on RemoveVersion when other versions remain.
    SetInstalled(ctx context.Context, recipe string, installed bool) error

    // Close releases resources held by the index (database connection).
    Close() error
}

// BinaryMatch is a result from BinaryIndex.Lookup.
type BinaryMatch struct {
    Recipe     string // Recipe name (e.g., "jq")
    Command    string // Command name as typed (e.g., "jq")
    BinaryPath string // Path within tool dir (e.g., "bin/jq")
    Installed  bool   // True if any version of Recipe is currently installed
    Source     string // "registry" or "installed" (for custom/local recipes)
}

// Registry is satisfied by *registry.Registry.
type Registry interface {
    // ListAll returns all recipe names known to this registry source.
    // Reads from the manifest when available; falls back to ListCached.
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

// StateReader provides read access to installed tool state during Rebuild.
// Satisfied by *install.StateManager.
type StateReader interface {
    AllTools() (map[string]install.ToolState, error)
}

// Open opens or creates the binary index at the given path.
// If the index file does not exist, it is created empty (not rebuilt).
// dbPath must be an absolute path under $TSUKU_HOME (use config.BinaryIndexPath()).
func Open(dbPath string) (BinaryIndex, error)

// ErrIndexNotBuilt is returned by Lookup when the index exists but has
// no data. Callers should trigger a rebuild.
var ErrIndexNotBuilt = errors.New("binary index not built; run `tsuku update-registry`")
```

**Error handling policy:**

| Condition | Behavior |
|-----------|----------|
| Index file missing | `Open()` creates empty DB; `Lookup()` returns `ErrIndexNotBuilt` |
| Index never built (empty) | `Lookup()` returns `ErrIndexNotBuilt` with hint message |
| Index stale (registry newer) | Return results with a `StaleIndex` warning in the error chain; do not block |
| Corrupt database | Return wrapped `sqlite3.ErrCorrupt`; caller triggers rebuild |
| Concurrent rebuild in progress | `Lookup()` uses read lock, proceeds normally |

Stale detection: compare `mtime` of the registry directory against a
`built_at` timestamp stored in the `meta` table.

#### Alternatives Considered

**Block Lookup during rebuild:**
Make `Lookup()` wait until any in-progress rebuild completes before returning.
Rejected because rebuild can take seconds and shell-integrated callers must not
block. Returning slightly stale results is preferable to a noticeable delay.

**Panic on corrupt database:**
Treat a corrupt index as a fatal error. Rejected because the index is a cache —
it can always be rebuilt. Corrupt databases should log a warning and trigger a
non-blocking background rebuild, not crash the process.

**Include version info in BinaryMatch:**
Add `LatestVersion string` or `InstalledVersions []string` to `BinaryMatch`.
Rejected for this scope — version resolution is the responsibility of Block 3
(auto-install), which has the version resolution infrastructure. Adding version
data to the index creates duplication and coupling.

## Decision Outcome

**Chosen: SQLite index with full rebuild, manifest-driven enumeration, bounded
on-demand fetch, installed-first ranking, and lazy-fail error handling**

### Summary

The binary index lives at `$TSUKU_HOME/cache/binary-index.db` (SQLite). It maps
command names to `[]BinaryMatch` with a single-column index on `command` for
sub-millisecond lookups. The index is rebuilt in full whenever `update-registry`
runs.

`Rebuild()` enumerates all recipes from the cached manifest via `ListAll()`, serves
locally-cached TOMLs immediately, and fetches the rest using 10 concurrent workers.
Fetch failures skip the affected recipe with a warning — the index is partial but
never broken. Fetched TOMLs are cached for future runs, so the first-run fetch cost
is a one-time expense.

Conflict resolution ranks installed recipes first, then uses lexicographic order
as a stable tiebreaker. `Lookup()` returns all ranked matches, letting callers
choose whether to take the top result (auto-install) or list all (suggest).

The index degrades gracefully: missing files are created empty, missing data
returns a typed `ErrIndexNotBuilt` error that callers can act on, and stale
data returns results with a non-blocking warning rather than blocking or failing.

### Rationale

Each decision reinforces the others. Manifest-driven enumeration (Decision 2)
ensures the index covers the full registry, not just the local cache. SQLite's
indexed access (Decision 1) keeps lookups within the 50ms budget regardless of
index size. Bounded concurrency (Decision 5) makes first-run fetch practical
without flooding the upstream server. The lazy-fail error handling (Decision 6)
means first-run and stale-index cases produce actionable errors rather than silent
failures or panics.

### Trade-offs Accepted

- **Recipe metadata completeness**: recipes without binary declarations are invisible
  to the index. CI validation and `tsuku doctor` can surface gaps, but the index
  can never recover data that recipes don't declare.
- **First-run latency**: ~15-20 seconds on a clean machine while TOMLs are fetched.
  This is acceptable because it's a one-time cost and is dominated by network RTT,
  not CPU.
- **No retry on 429**: rate-limited recipes are skipped; the index is partial until
  the next `update-registry`. A follow-up can add `Retry-After` backoff.
- **Installed-first ranking can surprise**: a user with `neovim` installed asking
  for `vi` suggestions gets `neovim` ranked first, not `vim`. This is intentional —
  their installed recipe wins — but may not always match expectations.

## Solution Architecture

### Overview

The binary index sits between the recipe registry and shell-facing commands. It's
built by the registry updater and queried by command-not-found, auto-install, and
`tsuku which`.

```
  tsuku update-registry
        │
        ├─ refreshManifest()          → downloads recipes.json → list of all names
        │
        └─ rebuildBinaryIndex()
               │
               ├─ reg.ListAll(ctx)    → all recipe names from manifest
               ├─ reg.GetCached(n)    → local cache hit (fast path)
               ├─ reg.FetchRecipe()   → on-demand fetch, 10 concurrent workers
               │                        fetched TOMLs cached via reg.CacheRecipe()
               ├─ extractBinaries()   → binary path extraction (no recipe package import)
               └─ VersionState.Binaries (from state.json, for installed-only tools)
                              │
                              ▼
                   binary-index.db (SQLite)
                   ┌───────────────────────────────────────────────┐
                   │ binaries: command, recipe, binary_path,        │
                   │           source, installed                    │
                   │ meta: built_at                                 │
                   └───────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
  tsuku which         command-not-found        tsuku run/exec
  (Block 1 CLI)       handler (Block 2)        (Block 3/6)
```

### Components

**`internal/index/binary_index.go`** — Core implementation

- `sqliteBinaryIndex` struct: holds `*sql.DB`
- Implements `BinaryIndex` interface
- `Open(dbPath string) (BinaryIndex, error)`: opens/creates DB, runs migrations
- `Lookup(ctx, command)`: parameterized SELECT with ORDER BY ranking
- `SetInstalled(ctx, recipe, installed)`: single-row UPDATE for incremental flag management
- Dependency inversion: `Registry` and `StateReader` interfaces defined in this package

**`internal/index/rebuild.go`** — `Rebuild()` implementation

- Calls `ListAll()` to enumerate all recipes
- Bounded-concurrency fetch loop (10-worker semaphore)
- Transaction wraps only the insert phase; fetch errors never roll back partial inserts
- DB write errors during insert abort the entire transaction atomically

**`internal/index/schema.go`** — SQL schema and migrations

- `initSchema(db *sql.DB) error`: CREATE TABLE IF NOT EXISTS statements
- Schema version tracked in `meta` table for future migrations

**`internal/registry/registry.go`** — `ListAll` implementation

```go
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

**`cmd/tsuku/cmd_which.go`** — `tsuku which <command>` CLI

- Opens binary index, calls `Lookup()`
- Formats output:
  - Single match: `<command> is provided by recipe '<recipe>'`
  - Multiple matches: table showing recipe, path, installed status
  - No match: `<command> not found in binary index`
  - Not built: `Binary index not built. Run 'tsuku update-registry' first.`

### Data Flow

**Rebuild on clean machine (first run)**:

```
update-registry
  └─ refreshManifest()          → downloads recipes.json, caches it
  └─ rebuildBinaryIndex()
       └─ reg.ListAll(ctx)      → reads cached manifest → ~1,400 names
       └─ reg.GetCached(name)   → cache miss for all (clean machine)
       └─ reg.FetchRecipe(ctx)  → fetches TOML, 10 concurrent workers
                                  → caches result for future runs
       └─ extractBinaries()     → binary path extraction
       └─ INSERT INTO binaries  → inside transaction
```

**Rebuild on warm machine (subsequent runs)**:

```
update-registry
  └─ refreshManifest()          → refreshes recipes.json if stale
  └─ rebuildBinaryIndex()
       └─ reg.ListAll(ctx)      → reads cached manifest
       └─ reg.GetCached(name)   → cache hit for most recipes
       └─ reg.FetchRecipe(ctx)  → only for newly-added recipes since last run
```

**Lookup flow** (during command-not-found or `tsuku which`):

```
1. index.Open($TSUKU_HOME/cache/binary-index.db)
2. Check staleness (non-blocking, advisory)
3. SELECT command, recipe, binary_path, installed, source
   FROM binaries WHERE command = ?
   ORDER BY installed DESC, recipe ASC
4. Scan rows into []BinaryMatch
5. Return (results, staleWarning or nil)
```

## Implementation Approach

### Phase 1: Schema and Core

- `internal/index/schema.go`: CREATE TABLE statements, `initSchema()`
- `internal/index/binary_index.go`: `Open()`, `Close()`, `sqliteBinaryIndex` struct
- Unit tests: Open creates DB, schema migrates cleanly

### Phase 2: Rebuild with Full Registry Coverage

- `Rebuild()` in `internal/index/rebuild.go`: `ListAll()` enumeration, bounded-concurrency
  fetch loop, transaction-wrapped insert phase, state.json integration
- `ListAll()` on `*registry.Registry` using `GetCachedManifest()`
- Wire into `cmd/tsuku/cmd_update_registry.go` to call rebuild after fetch
- Integration tests: full-manifest rebuild, fetch-error skip, warm-machine cache-hit path

### Phase 3: Lookup and Staleness

- `Lookup()` implementation with parameterized query, ranking, stale check
- Unit tests: lookup returns correct ranking, installed-first ordering verified

### Phase 4: CLI and install/remove Integration

- `cmd/tsuku/cmd_which.go`: `tsuku which <command>` command
- Wire `SetInstalled()` into `install.Manager` for incremental flag updates:
  - **On install**: call `index.SetInstalled(ctx, toolName, true)`
  - **On `RemoveAllVersions`**: call `index.SetInstalled(ctx, toolName, false)`
  - **On `RemoveVersion`**: update only if `ActiveVersion` becomes empty
- End-to-end test: install recipe → `tsuku which` shows `installed: true`;
  remove all versions → `tsuku which` shows `installed: false`

## Security Considerations

### Recipe Name Validation

`ListAll()` returns names directly from the manifest. Before any name is passed
to URL or path construction, it is validated to reject names containing `/`, `..`,
or null bytes. Without this check, a local-registry deployment (`TSUKU_REGISTRY_URL`
pointing to a local path) could compose a filesystem path from a manifest entry,
enabling path traversal outside `$TSUKU_HOME/registry/`.

### Index Integrity

The binary index is derived from the recipe registry and installed state. It does
not fetch data from any external source beyond what `update-registry` already
fetches. No new trust boundary is introduced.

**Risk: Index manipulation** — a local attacker with write access to
`$TSUKU_HOME/cache/` could modify the index to redirect command lookups to
malicious recipes.

**Mitigation:** The index is a cache derived from signed/verified sources. If
the recipes and state.json are trusted, the derived index is trusted. If they're
not, the index is the least of the concerns.

**Risk: Registry poisoning via index** — an attacker who compromises the recipe
registry could add a recipe mapping a common command (e.g., `git`) to a malicious
recipe.

**Mitigation:** Lookups from the binary index lead to suggestions or prompts, not
silent installs. Block 2 (command-not-found) calls `tsuku suggest` which only
prints a message. Block 3 (auto-install) defaults to `confirm` mode requiring
user consent. The index itself does not execute anything.

### SQLite File Handling

**Risk: SQLite injection** — command names passed to `Lookup()` could contain
SQL metacharacters.

**Mitigation:** All queries use parameterized statements (`WHERE command = ?`).
No string interpolation is used in SQL construction.

**Risk: Corrupt database** — filesystem issues or interrupted writes could leave
the SQLite file in a corrupt state.

**Mitigation:** Rebuild uses a transaction; partial writes are rolled back. If
`Open()` detects corruption, `ErrIndexCorrupt` is returned with a message
directing the user to run `tsuku update-registry`. The corrupt file is not
deleted automatically to avoid data loss from filesystem errors.

**Risk: Concurrent access** — multiple tsuku processes rebuilding or reading simultaneously.

**Mitigation:** SQLite WAL mode (`PRAGMA journal_mode = WAL`) allows concurrent
readers during a write transaction. `PRAGMA busy_timeout = 5000` handles
transient write locks gracefully. `Lookup()` may return slightly stale results
during a concurrent rebuild; this is acceptable per the non-blocking stale-warning
design.

### FetchRecipe Network Surface

`FetchRecipe()` downloads recipe TOML files from the registry URL
(`https://raw.githubusercontent.com/tsukumogami/tsuku/main` by default, or
`TSUKU_REGISTRY_URL` if set). This is the same trust boundary as existing
`update-registry` network operations. Additional considerations from the bulk
fetch path:

- **Response size cap**: `resp.Body` is wrapped with `io.LimitReader(resp.Body, 1<<20)`
  (1 MiB) before `io.ReadAll`. Without this, a malicious server could return an
  arbitrarily large body; with 10 concurrent fetches during cold-start, this is a
  memory exhaustion risk.
- **TLS verification**: `FetchRecipe()` uses the existing `registry.Registry` HTTP
  client with Go's default TLS stack. Transport security is inherited from existing
  recipe fetch paths.
- **Redirect behavior**: Go's `http.Client` follows cross-origin redirects by default.
  Deployments using `TSUKU_REGISTRY_URL` with an HTTP endpoint should use HTTPS to
  avoid redirect-based downgrade attacks.
- **Rate limiting**: no retry or backoff is implemented for 429 responses during bulk
  fetch. Hitting a rate limit skips affected recipes (non-fatal); a subsequent
  `update-registry` retries them.
- **`TSUKU_REGISTRY_URL` trust**: if a user points `TSUKU_REGISTRY_URL` at an
  attacker-controlled server, `FetchRecipe()` downloads attacker-supplied TOML. The
  bulk fetch volume amplifies this existing concern. Treat `TSUKU_REGISTRY_URL` as
  a sensitive configuration value in shared environments.

### No Sensitive Data

The binary index contains only recipe names, command names, and binary paths —
all public information from the recipe registry. It contains no credentials,
tokens, or user-specific data beyond the `installed` flag (which reflects local
state already present in state.json).

## Consequences

### Positive

- `tsuku which <command>` gives users instant feedback on what provides a command,
  including on a clean machine after the first `update-registry`
- Sub-millisecond lookups in shell-integrated paths stay well within the 50ms budget
- Recipe TOML files are cached as a side effect of the first rebuild, making
  subsequent installs faster (cache hit) and subsequent rebuilds cheap
- Installed-first ranking matches user intent without requiring configuration
- Stale-index warning (non-blocking) helps users discover when to re-run update-registry
- Custom and distributed recipe binaries are discoverable via the installed supplement

### Negative

- **Recipe metadata dependency**: recipes without binary declarations are invisible.
  The index can't compensate for missing `metadata.binaries` or action parameters.
- **First-run latency**: a clean machine fetches ~1,400 recipe TOMLs during the
  first `update-registry`. With 10 concurrent workers at ~100ms RTT, this adds
  roughly 15-20 seconds to the first run.
- **No retry on 429**: if the registry rate-limits the bulk fetch, affected recipes
  are silently skipped. The index will be partial until the next `update-registry`
  succeeds for those recipes.
- **SQLite dependency**: adds `database/sql` + a SQLite driver to the binary.
  The driver is a pure-Go fallback (`modernc.org/sqlite`) to avoid CGo cross-compilation
  issues.

### Mitigations

- **Recipe metadata**: add a `tsuku doctor` check listing recipes missing binary
  declarations; add CI validation that new recipes declare at least one binary
- **First-run latency**: a progress message ("Indexing registry recipes... N/1400")
  lets users know the command is working
- **No retry**: document as a known gap; add a follow-up issue for retry/backoff
  in `FetchRecipe()`
- **Full rebuild cost**: benchmark in CI; if it exceeds 2s on warm cache, switch
  to incremental strategy
