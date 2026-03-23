---
status: Accepted
problem: |
  Tsuku's shell integration features (command-not-found suggestions, auto-install)
  require knowing which recipe provides a given command name. No such reverse lookup
  exists today. Every candidate lookup would require scanning all recipes, which
  violates the 50ms shell-integration budget. The binary index fills this gap.
decision: |
  Build a SQLite-backed binary index at $TSUKU_HOME/cache/binary-index.db, rebuilt
  fully on every `update-registry` invocation. The index maps command names to recipes
  using data from Recipe.ExtractBinaries() (registry path) and VersionState.Binaries
  (installed path). When multiple recipes provide the same command, conflicts are
  resolved by preferring installed recipes over uninstalled, then using lexicographic
  order as a stable tiebreaker. The BinaryIndex interface returns all matches ranked
  by preference, with the caller responsible for acting on the top result.
rationale: |
  SQLite with a single-column index on command name achieves sub-millisecond lookups
  against a registry of hundreds of recipes, well within the 50ms budget. Full rebuild
  on update-registry is simpler than incremental sync and correct because registry
  updates are infrequent. Returning all ranked matches (rather than one winner) lets
  callers like tsuku suggest show all options, while callers like auto-install can
  take the top result. The installed-first ranking matches user intent: if they've
  already installed a recipe that provides a command, it should win over an uninstalled
  alternative.
spawned_from:
  issue: 1677
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
---

# DESIGN: Binary Index

## Status

Accepted

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
`ExtractBinaries()` on every recipe in the registry -- hundreds of files for a
scan that must complete in under 50ms for interactive shell use.

The binary index solves this by pre-computing the command-to-recipe mapping at
registry-update time and storing it in a queryable format.

### Scope

**In scope:**
- SQLite index at `$TSUKU_HOME/cache/binary-index.db`
- Index populated from `Recipe.ExtractBinaries()` for all registry recipes
- Hybrid index: installed tools supplement the registry for custom/distributed recipes
- `tsuku which <command>` CLI command
- `BinaryIndex` Go interface and `BinaryMatch` struct
- Conflict resolution (multiple recipes providing same command)
- Error handling: missing, stale, and corrupt index

**Out of scope:**
- LLM recipe discovery (future Block 6+)
- Project config version pinning (Block 4)
- The command-not-found shell handler itself (Block 2, issue #1678)
- The auto-install flow (Block 3, issue #1679)

## Decision Drivers

- **50ms lookup budget**: shell-integrated operations must complete under 50ms;
  index lookups must be well within this to leave room for surrounding logic
- **Accuracy depends on recipe metadata**: recipes without `metadata.binaries`
  or explicit action binary declarations won't appear in lookups
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
```

Rebuild procedure:
1. Begin transaction
2. DELETE all rows with `source = 'registry'`
3. For each recipe in registry: call `ExtractBinaries()`, insert rows
4. UPDATE `installed = 1` for recipes with any installed version in state.json
5. Upsert rows with `source = 'installed'` from `VersionState.Binaries` for
   tools whose source is "local" or distributed (not in registry)
6. Commit

The `installed` flag is updated on every install/remove operation (not just
registry rebuilds) to keep it current.

#### Alternatives Considered

**JSON file at `$TSUKU_HOME/cache/binary-index.json`:**
A JSON object mapping command name to `[]BinaryMatch`. Simple to write, no
dependencies. Rejected because JSON doesn't support indexed lookup -- reading
the file and deserializing for every lookup would add 5-20ms per query and
consume memory proportional to index size. For lookup performance, SQLite's
indexed access is clearly superior.

**Incremental rebuild (diff registry, update changed recipes):**
Track recipe content hashes and only re-extract binaries when a recipe file
changes. Rejected because the implementation complexity isn't justified -- full
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

### Decision 2: Conflict Resolution Policy

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

### Decision 3: BinaryIndex Interface and Error Handling

The Go interface must be stable enough for downstream designs (#1678, #1679) to
proceed. It must also handle the index being absent (first run), stale (registry
updated but index not rebuilt), or corrupt.

**Key assumptions:**
- Decision 2 chose ranked `[]BinaryMatch` return type (factored into interface)
- Decision 1 chose SQLite full rebuild (affects error surface: rebuild can fail)
- Callers are CLI commands and shell-integrated code paths; both need clear errors

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

// Registry is satisfied by *registry.Registry. It uses the actual available
// methods (ListCached, GetCached) rather than a hypothetical LoadRecipe method.
// Rebuild calls recipe.Parse() on the raw bytes from GetCached internally.
type Registry interface {
    ListCached() ([]string, error)
    GetCached(name string) ([]byte, error)
}

// StateReader provides read access to installed tool state during Rebuild.
// Keeping this as a narrow interface avoids coupling internal/index to
// internal/install internals.
// Satisfied by *install.StateManager.
type StateReader interface {
    AllTools() (map[string]install.ToolState, error)
}

// Open opens or creates the binary index at the given path.
// If the index file does not exist, it is created empty (not rebuilt).
// Returns ErrIndexNotBuilt if the index exists but has never been populated.
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
`built_at` timestamp stored in a `meta` table:

```sql
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Populated by Rebuild(): INSERT OR REPLACE INTO meta VALUES ('built_at', '<unix_ts>')
```

#### Alternatives Considered

**Block Lookup during rebuild:**
Make `Lookup()` wait until any in-progress rebuild completes before returning.
Rejected because rebuild can take seconds and shell-integrated callers must not
block. Returning slightly stale results is preferable to a noticeable delay.

**Panic on corrupt database:**
Treat a corrupt index as a fatal error. Rejected because the index is a cache --
it can always be rebuilt. Corrupt databases should log a warning and trigger a
non-blocking background rebuild, not crash the process.

**Include version info in BinaryMatch:**
Add `LatestVersion string` or `InstalledVersions []string` to `BinaryMatch`.
Rejected for this block's scope -- version resolution is the responsibility of
Block 3 (auto-install), which has the version resolution infrastructure. Adding
version data to the index creates duplication and coupling.

## Decision Outcome

**Chosen: SQLite index with full rebuild, installed-first ranking, and lazy-fail error handling**

### Summary

The binary index lives at `$TSUKU_HOME/cache/binary-index.db` (SQLite). It maps
command names to `[]BinaryMatch` with a single-column index on `command` for
sub-millisecond lookups. The index is rebuilt in full whenever `update-registry`
runs -- this is fast enough (~1s) that incremental logic isn't worth the complexity.

Conflict resolution ranks installed recipes first, then uses lexicographic order
as a stable tiebreaker. `Lookup()` returns all ranked matches, letting callers
choose whether to take the top result (auto-install) or list all (suggest).

The index degrades gracefully: missing files are created empty, missing data
returns a typed `ErrIndexNotBuilt` error that callers can act on, and stale
data returns results with a non-blocking warning rather than blocking or failing.

### Rationale

Each decision reinforces the others. SQLite gives fast indexed lookups (Decision 1)
that support the 50ms budget without blocking. Returning all ranked matches
(Decision 2) avoids hardcoding a policy that callers like Block 2 and Block 3 may
want to override. The lazy-fail error handling (Decision 3) means first-run and
stale-index cases produce actionable errors rather than silent failures or panics.

### Trade-offs Accepted

- **Recipe metadata completeness**: recipes without binary declarations are invisible
  to the index. A CI validation pass and a developer warning (`tsuku doctor`) can
  surface gaps, but the index can never recover data that recipes don't declare.
- **Full rebuild cost**: rebuilding the entire index on every `update-registry` takes
  ~1s for a large registry. This is acceptable because `update-registry` is a
  network-bound operation where local processing time is dominated by download time.
- **Installed-first ranking can surprise**: a user with `neovim` installed asking
  for `vi` suggestions gets `neovim` ranked first, not `vim`. This is intentional --
  their installed recipe wins -- but may not always match expectations.

## Solution Architecture

### Overview

The binary index is a local SQLite database that sits between the recipe registry
and shell-facing commands. It's built by the registry updater and queried by
command-not-found, auto-install, and `tsuku which`.

```
  tsuku update-registry
        │
        ▼
  BinaryIndex.Rebuild()
        │
        ├─── Recipe.ExtractBinaries()  (for each recipe in registry)
        │
        └─── VersionState.Binaries     (from state.json, for installed tools)
                         │
                         ▼
              binary-index.db (SQLite)
              ┌───────────────────────────────────────────────┐
              │ binaries: command, recipe, binary_path,        │
              │           source, installed                    │
              │ meta: built_at                                 │
              └───────────────────────────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        ▼                ▼                ▼
  tsuku which    command-not-found   tsuku run/exec
  (Block 1 CLI)  handler (Block 2)   (Block 3/6)
```

### Components

**`internal/index/binary_index.go`** -- Core implementation

- `sqliteBinaryIndex` struct: holds `*sql.DB`
- Implements `BinaryIndex` interface
- `Open(dbPath string) (BinaryIndex, error)`: opens/creates DB, runs migrations
- `Rebuild(ctx, registry Registry, state StateReader)`: transaction-wrapped full rebuild;
  calls `registry.ListCached()` + `registry.GetCached()` + `recipe.Parse()` internally
- `Lookup(ctx, command)`: parameterized SELECT with ORDER BY ranking
- `SetInstalled(ctx, recipe, installed)`: single-row UPDATE for incremental flag management
- Dependency inversion: `Registry` and `StateReader` interfaces defined in this package;
  `*registry.Registry` and `*install.StateManager` satisfy them without importing
  `internal/index` from those packages

**`internal/index/schema.go`** -- SQL schema and migrations

- `initSchema(db *sql.DB) error`: CREATE TABLE IF NOT EXISTS statements
- Schema version tracked in `meta` table for future migrations

**`internal/index/stale.go`** -- Staleness detection

- `CheckStaleness(db *sql.DB, registryDir string) (bool, error)`
- Compares `built_at` from meta table against `mtime` of registry directory

**`cmd/tsuku/cmd_which.go`** -- `tsuku which <command>` CLI

- Opens binary index, calls `Lookup()`
- Formats output:
  - Single match: `<command> is provided by recipe '<recipe>'`
  - Multiple matches: table showing recipe, path, installed status
  - No match: `<command> not found in binary index`
  - Not built: `Binary index not built. Run 'tsuku update-registry' first.`

### Key Interfaces

```go
// Package index provides command-to-recipe reverse lookup.
package index

import (
    "context"
    "errors"
)

// BinaryIndex provides command-to-recipe lookup.
type BinaryIndex interface {
    Lookup(ctx context.Context, command string) ([]BinaryMatch, error)
    Rebuild(ctx context.Context, registry Registry) error
    Close() error
}

// BinaryMatch is a single result from BinaryIndex.Lookup.
type BinaryMatch struct {
    Recipe     string // Recipe name (e.g., "jq")
    Command    string // Command name (e.g., "jq")
    BinaryPath string // Relative path in tool dir (e.g., "bin/jq")
    Installed  bool   // True if any version is currently installed
    Source     string // "registry" or "installed" (for local/distributed)
}

// Registry provides access to recipe metadata during rebuild.
// The registry package implements this interface.
type Registry interface {
    ListRecipes(ctx context.Context) ([]string, error)
    LoadRecipe(ctx context.Context, name string) (*recipe.Recipe, error)
}

// ErrIndexNotBuilt signals that the index has never been populated.
var ErrIndexNotBuilt = errors.New("binary index not built; run `tsuku update-registry`")

// ErrIndexCorrupt signals an unrecoverable database error.
// Callers should delete the file and rebuild.
var ErrIndexCorrupt = errors.New("binary index corrupt; rebuild with `tsuku update-registry --rebuild-index`")

// StaleIndexWarning is returned (wrapped) when the index may be out of date.
// Results are still returned; this is advisory only.
type StaleIndexWarning struct {
    BuiltAt    time.Time
    RegistryAt time.Time
}
func (s *StaleIndexWarning) Error() string { ... }
```

### Data Flow

**Rebuild flow** (on `update-registry`):

```
1. registry.Update() fetches latest recipes to $TSUKU_HOME/registry/
2. registry.Update() calls index.Rebuild(ctx, registry, stateReader)
3. Rebuild begins a write transaction
4. DELETE FROM binaries WHERE source = 'registry'
5. For each name in registry.ListCached():
   a. bytes = registry.GetCached(name) → []byte
   b. r = recipe.Parse(bytes) → *Recipe
   c. r.ExtractBinaries() → []string (e.g., ["bin/jq"])
   d. For each binary path: derive command = filepath.Base(binaryPath)
   e. INSERT OR REPLACE INTO binaries(command, recipe, binary_path, source, installed)
6. Load state via stateReader.AllTools() → for each ToolState with ActiveVersion != "":
   a. UPDATE binaries SET installed=1 WHERE recipe=<toolName>
   b. For tools with source "local" or "owner/repo":
      i. For each binary in VersionState.Binaries for the ActiveVersion:
         INSERT OR REPLACE with source='installed'
7. INSERT OR REPLACE INTO meta('built_at', unix_ts)
8. Commit
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

Deliverables: `internal/index/binary_index.go`, `internal/index/schema.go`

### Phase 2: Rebuild

- `Rebuild()` implementation with transaction, recipe scan, state.json integration
- Wire into `cmd/tsuku/cmd_update_registry.go` to call rebuild after fetch
- Integration test: rebuild against fixture recipes, verify row counts and content

Deliverables: `Rebuild()` in `binary_index.go`, update-registry wiring

### Phase 3: Lookup and Staleness

- `Lookup()` implementation with parameterized query, ranking, stale check
- `internal/index/stale.go`: `CheckStaleness()`
- Unit tests: lookup returns correct ranking, installed-first ordering verified

Deliverables: `Lookup()`, `stale.go`

### Phase 4: CLI and install/remove integration

- `cmd/tsuku/cmd_which.go`: `tsuku which <command>` command
- Wire `SetInstalled()` into `install.Manager` for incremental flag updates:
  - **On install**: after successful install, call `index.SetInstalled(ctx, toolName, true)`
  - **On `RemoveVersion`**: if `ActiveVersion` is still set after removal, no index update needed; if `ActiveVersion` becomes empty (last version removed), call `index.SetInstalled(ctx, toolName, false)`
  - **On `RemoveAllVersions`**: always call `index.SetInstalled(ctx, toolName, false)`
- End-to-end test: install recipe → `tsuku which` shows `installed: true`; remove all versions → `tsuku which` shows `installed: false`
- Note: after Phase 2 and before Phase 4, the `installed` flag is accurate only at rebuild time. This is acceptable as an intermediate state during development.

Deliverables: `cmd_which.go`, install/remove wiring

## Security Considerations

### Index Integrity

The binary index is derived from the recipe registry and installed state. It does
not fetch data from any external source beyond what `update-registry` already
fetches. No new network surface is introduced.

**Risk: Index manipulation** -- a local attacker with write access to
`$TSUKU_HOME/cache/` could modify the index to redirect command lookups to
malicious recipes.

**Mitigation:** The index is a cache derived from signed/verified sources. The
index file is not integrity-checked on read because the threat model for local
disk modification already covers the recipes and state.json files themselves.
If those are trusted, the derived index is trusted. If they're not, the index
is the least of the concerns.

**Risk: Registry poisoning via index** -- an attacker who compromises the recipe
registry could add a recipe mapping a common command (e.g., `git`) to a malicious
recipe.

**Mitigation:** Lookups from the binary index lead to suggestions or prompts, not
silent installs. Block 2 (command-not-found) calls `tsuku suggest` which only
prints a message. Block 3 (auto-install) defaults to `confirm` mode requiring
user consent. The index itself does not execute anything.

### SQLite File Handling

**Risk: SQLite injection** -- command names passed to `Lookup()` could contain
SQL metacharacters.

**Mitigation:** All queries use parameterized statements (`WHERE command = ?`).
No string interpolation is used in SQL construction.

**Risk: Corrupt database** -- filesystem issues or interrupted writes could leave
the SQLite file in a corrupt state.

**Mitigation:** Rebuild uses a transaction; partial writes are rolled back. If
`Open()` detects corruption, `ErrIndexCorrupt` is returned with a message
directing the user to run `tsuku update-registry --rebuild-index`. The corrupt
file is not deleted automatically to avoid data loss from filesystem errors.

**Risk: Concurrent access -- multiple tsuku processes rebuilding or reading simultaneously.**

**Mitigation:** Implementers must enable SQLite WAL mode (`PRAGMA journal_mode = WAL`)
at DB initialization, which allows concurrent readers during a write transaction. Set
`PRAGMA busy_timeout = 5000` (5 seconds) to handle transient write locks gracefully
rather than returning immediate errors. `Lookup()` may return slightly stale results
during a concurrent rebuild; this is acceptable per the non-blocking stale-warning
design.

**Note on `dbPath`:** The `Open(dbPath string)` function accepts a path parameter
for testability. In production it is always called with `config.BinaryIndexPath()`
which returns an absolute path under `$TSUKU_HOME`. Implementations must not accept
user-supplied `dbPath` values without validation.

### No Sensitive Data

The binary index contains only recipe names, command names, and binary paths --
all public information from the recipe registry. It contains no credentials,
tokens, or user-specific data beyond the `installed` flag (which reflects local
state already present in state.json).

## Consequences

### Positive

- `tsuku which <command>` gives users instant feedback on what provides a command
- Sub-millisecond lookups in shell-integrated paths stay well within the 50ms budget
- Installed-first ranking matches user intent without requiring configuration
- Stale-index warning (non-blocking) helps users discover when to re-run update-registry
- Custom and distributed recipe binaries are discoverable via the installed supplement

### Negative

- **Recipe metadata dependency**: recipes without binary declarations are invisible.
  The index can't compensate for missing `metadata.binaries` or action parameters.
- **Full rebuild cost**: ~1s for a large registry on every `update-registry`. Not
  perceptible in practice (dominated by network time), but adds to test time.
- **SQLite dependency**: adds `database/sql` + a SQLite driver to the binary.
  The driver is a CGo dependency or requires a pure-Go fallback (e.g., `modernc.org/sqlite`).

### Mitigations

- **Recipe metadata**: add a `tsuku doctor` check listing recipes missing binary
  declarations; add CI validation that new recipes declare at least one binary
- **Rebuild cost**: benchmark in CI; if it exceeds 2s, switch to incremental strategy
- **SQLite driver**: use `modernc.org/sqlite` (pure Go, no CGo) to avoid cross-compilation
  issues on platforms tsuku supports; evaluate binary size impact before merging
