# Decision Report: Resolver Key Matching

## Question

How should the resolver reconcile org-prefixed config keys (tsukumogami/koto) with bare binary-index recipe names (koto)?

## Status

COMPLETE

## Chosen Alternative

Dual-key lookup in the resolver

## Confidence

high

## Rationale

The resolver is the only place where config keys meet binary index keys. By teaching `ProjectVersionFor` to try both the bare recipe name and a reverse-mapped org-scoped name, we keep the binary index unchanged, keep the config format unchanged, and isolate the reconciliation to a single function. The reverse mapping comes from data already available: config keys that contain a `/` can be split into `org/repo` prefix and bare name, building a small map at resolver construction time. This avoids polluting the binary index with org-scoped names (which would break `isValidRecipeName` and the registry fetch path) and avoids normalizing config keys at parse time (which would lose the org identity needed for auto-registration).

## Considered Alternatives

### Alternative A: Dual-key lookup in the resolver

**Description:** At `NewResolver` construction, build a reverse map from bare recipe names to their org-scoped config keys by scanning `config.Tools`. For each key containing `/`, extract the bare suffix (everything after the last `/` for `owner/repo:name` qualified names, or everything after `/` for `owner/name` shorthand). In `ProjectVersionFor`, after the binary index returns matches with bare recipe names, first try `config.Tools[m.Recipe]` (handles bare keys for backward compatibility), then fall back to `config.Tools[reverseMap[m.Recipe]]` (handles org-scoped keys).

For the name collision case (two orgs ship a tool with the same binary name), the reverse map naturally handles it: both `orgA/tool` and `orgB/tool` map bare name `tool` to two config keys. The resolver can check all matches and return the first that has a config entry. Since the binary index already returns multiple matches sorted by preference, iterating the cross product of (matches x config keys) handles disambiguation correctly -- if the user pins `orgA/tool` in their config, only that entry matches.

**Pros:**
- Single-point change: only `resolver.go` is modified
- Binary index stays bare-name-only, preserving `isValidRecipeName` and all existing paths
- Config parsing is untouched -- `LoadProjectConfig` works as-is with TOML quoted keys
- Backward compatible: bare keys hit the fast path (`config.Tools[m.Recipe]`), no reverse map needed
- Name collisions resolved naturally by cross-product iteration
- No new data structures leak into other packages

**Cons:**
- Adds ~15 lines to resolver.go
- Reverse map must be rebuilt if config changes (not an issue since resolver is constructed per-invocation)

**Verdict:** Chosen

### Alternative B: Normalize config keys at parse time

**Description:** In `LoadProjectConfig` / `parseConfigFile`, detect org-scoped keys (containing `/`) and add a second entry under the bare suffix. The config `Tools` map would contain both `"tsukumogami/koto"` and `"koto"` pointing to the same `ToolRequirement`. The resolver then works unchanged since `m.Recipe` ("koto") finds a match.

**Pros:**
- Zero changes to resolver.go
- Simple to implement in parseConfigFile

**Cons:**
- Loses information about which org a bare name belongs to -- if two orgs provide `tool`, both normalize to the same bare key, causing a silent last-write-wins conflict at parse time
- Caller code that iterates `config.Tools` (for display, auto-install, or `runProjectInstall`) sees duplicate entries -- needs filtering
- The org prefix is needed downstream for auto-registering distributed sources; normalizing it away too early forces re-parsing or a separate data structure
- Violates the "config map keys match what the user wrote" principle, making error messages confusing

**Verdict:** Rejected -- loses org identity and creates ambiguity for name collisions

### Alternative C: Store org-scoped names in the binary index

**Description:** When a distributed recipe is installed, insert binary index rows with recipe = "tsukumogami/koto" instead of (or in addition to) the bare "koto". The resolver then matches directly because both the index and config use org-scoped keys.

**Pros:**
- Direct match, no reverse mapping needed in resolver
- Explicit provenance in every index row

**Cons:**
- Breaks `isValidRecipeName` which rejects names containing `/` -- that guard exists specifically to prevent path traversal in registry fetch paths
- The central registry rebuild path (`reg.ListAll` -> `reg.FetchRecipe`) deals exclusively in bare names; inserting org-scoped names creates a parallel namespace that every consumer of the index must handle
- The `SetInstalled` path (called by install/remove) would need to know whether to use the bare or org-scoped name, coupling install state naming to config format
- State.json uses bare names as keys; now state and index would disagree on naming, creating a new mismatch to reconcile

**Verdict:** Rejected -- too many downstream consumers would break, and it creates a new mismatch with state.json

### Alternative D: Add an org-to-recipe mapping table to the binary index DB

**Description:** Add a new SQLite table `org_recipes(org_key TEXT, bare_name TEXT)` populated during distributed install. The resolver queries this table when a bare-name lookup fails against config.

**Pros:**
- Persistent mapping survives across resolver instances
- Could be useful for other features (e.g., `tsuku list` showing org provenance)

**Cons:**
- Overkill for the problem -- the resolver is constructed fresh each invocation and already has access to the config (which contains the mapping implicitly)
- Adds schema migration complexity to the index DB
- Distributed installs would need to write to this table, coupling the install flow to the index schema
- The mapping is already derivable from the config keys at zero cost

**Verdict:** Rejected -- unnecessary persistence for a derivable mapping

## Assumptions

- Config keys for org-scoped tools will always use the `org/recipe` format (as chosen in Decision 1 and the design doc's "quoted key" approach)
- The binary index will continue to store bare recipe names only (no plan to change this for central registry recipes)
- State.json will continue to use bare recipe names as keys for installed tools
- A single `.tsuku.toml` is unlikely to have name collisions (two orgs providing the same bare tool name), but the design must handle it correctly when it does occur

## Implications

- `resolver.go` needs a `bareName -> []orgScopedKey` reverse map, built in `NewResolver` or lazily on first call
- The resolver's `ProjectVersionFor` loop becomes a nested check: for each binary match, try bare key first, then any org-scoped keys that map to that bare name
- `runProjectInstall` (the auto-install path) will need a similar awareness when iterating config tools to determine which are missing -- it must recognize that `"tsukumogami/koto"` maps to the already-installed bare-name `"koto"` in state.json. This is a separate concern from the resolver but uses the same name-splitting logic, so a shared utility function (`splitOrgKey(key) (org, bare string, isOrgScoped bool)`) should be extracted
- Error messages from the resolver don't need to change since they don't reference recipe names directly
