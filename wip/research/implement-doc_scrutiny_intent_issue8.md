# Scrutiny Review: Intent Focus - Issue 8

**Issue:** #8 feat(cli): add source-directed loading to update, outdated, and verify
**Focus:** intent
**Reviewer:** scrutiny-intent

## Sub-check 1: Design Intent Alignment

### AC: "update reads ToolState.Source, calls GetFromSource"

**Design doc (lines 621-627) specifies:**
> **Subsequent update** (`tsuku update <tool>`):
> 1. Read `ToolState.Source` for the tool
> 2. Call `loader.GetFromSource(ctx, name, source)` to fetch fresh recipe from the recorded source

**Implementation (update.go):**

The `update` command calls `ensureSourceProvider(toolName, cfg)` which registers a distributed provider in the loader chain, then delegates to `runInstallWithTelemetry` which uses the normal `loader.Get()` chain. It does NOT call `GetFromSource`.

**Assessment: BLOCKING**

The design doc's intent for `GetFromSource` is to bypass the priority chain and fetch directly from the recorded source. This matters because:

1. **Shadowing risk:** If a local recipe or embedded recipe exists with the same name as a distributed recipe, the chain-based approach (`loader.Get`) will return the local/embedded recipe instead of the distributed one. The whole point of source-directed loading is to ensure `update` fetches from the SAME source the tool was originally installed from.

2. **Freshness guarantee:** `GetFromSource` is documented to "not consult or populate the in-memory recipes cache" (design doc line 586-587). The chain approach goes through the cache and may return stale data.

By contrast, `outdated.go` and `verify.go` correctly use `loadRecipeForTool()` which calls `GetFromSource` for distributed sources. The `update` command is the odd one out.

The `update` command's approach is functionally different: it registers the provider and hopes the chain resolves to it, rather than explicitly fetching from the recorded source. For tools with unique names across sources this works by accident, but it violates the design's intent for source isolation.

### AC: "outdated iterates installed tools checking each against recorded source"

**Implementation (outdated.go):**

Uses `loadRecipeForTool()` which reads `ToolState.Source`, calls `GetFromSource` for distributed sources, and falls back to the normal chain for central/local/embedded. This matches the design intent.

**Assessment: PASS**

### AC: "verify uses cached recipe from recorded source"

**Implementation (verify.go):**

Changed from `loader.Get(name, recipe.LoaderOptions{})` to `loadRecipeForTool(context.Background(), name, state, cfg)`. This correctly uses `GetFromSource` for distributed sources.

**Assessment: PASS**

### AC: "Unit tests for each command covering central, embedded, and distributed source paths"

**Implementation (source_directed_test.go):**

12 test functions covering:
- `TestIsDistributedSource` - source string classification
- `TestLoadRecipeForTool_CentralSource` - central path
- `TestLoadRecipeForTool_EmptySourceDefaultsToCentral` - migration fallback
- `TestLoadRecipeForTool_DistributedSource` - distributed path via GetFromSource
- `TestLoadRecipeForTool_UnreachableDistributedFallsBack` - graceful degradation
- `TestLoadRecipeForTool_NilState` - nil state handling
- `TestLoadRecipeForTool_ToolNotInState` - missing tool
- `TestEnsureSourceProvider_EmptySource` - update path, empty source
- `TestEnsureSourceProvider_CentralSource` - update path, central
- `TestEnsureSourceProvider_NotInstalled` - update path, not installed
- `TestParseAndCache` / `TestParseAndCache_InvalidTOML` - new Loader method

**Assessment: ADVISORY**

The tests cover `loadRecipeForTool` (used by outdated/verify) and `ensureSourceProvider` (used by update) well. However, there are no tests for the embedded source path -- all "non-distributed" tests use central/registry providers. The AC says "central, embedded, and distributed source paths" and embedded is absent. This is minor since the embedded path goes through the same `loader.Get` chain, but it's a literal gap against the AC text.

### AC: "No changes to CLI output format or exit codes"

**Assessment: PASS**

All changes are internal routing. The output-producing code is unchanged. No new exit codes introduced.

## Sub-check 2: Cross-issue Enablement

`context.downstream_issues` is empty. Skipping this sub-check.

## Backward Coherence

**Previous summary (Issue 7):**
> Files changed: cmd/tsuku/install.go, cmd/tsuku/install_distributed.go, install_distributed_test.go, internal/install/state.go, internal/recipe/loader.go. Key decisions: parseDistributedName handles 4 formats. ensureDistributedSource auto-registers with dynamic AddProvider. checkSourceCollision uses ToolState.Source. RecipeHash added to ToolState.

Issue 7 established two patterns for distributed source handling:
1. `ensureDistributedSource` - for the install flow (includes validation, prompting, auto-registration)
2. `addDistributedProvider` - lower-level provider registration

Issue 8 introduces two new patterns:
1. `ensureSourceProvider` - lighter version of `ensureDistributedSource` for update (reads state, registers provider)
2. `loadRecipeForTool` - shared helper for outdated/verify (reads state, calls GetFromSource)

The inconsistency between `update` (chain-based via `ensureSourceProvider`) and `outdated`/`verify` (source-directed via `loadRecipeForTool`) introduces a split in the codebase's approach. This is a coherence concern: two different patterns for the same conceptual operation ("load a recipe for an already-installed tool, respecting its recorded source").

The `update` command could use `loadRecipeForTool` to fetch the recipe and then pass it to the install flow, achieving consistency with the other commands and matching the design intent.

## Summary of Findings

| # | AC | Severity | Finding |
|---|-----|----------|---------|
| 1 | update reads ToolState.Source, calls GetFromSource | **BLOCKING** | update.go uses ensureSourceProvider + chain, not GetFromSource. Design intent requires source-directed fetch to prevent shadowing. |
| 2 | Unit tests: embedded source path | **ADVISORY** | No test covers embedded source path explicitly, though it's functionally equivalent to the central path through the chain. |
| 3 | Coherence: split patterns | **ADVISORY** | update uses a different approach (register-then-chain) than outdated/verify (GetFromSource). This creates two patterns for the same operation. |
