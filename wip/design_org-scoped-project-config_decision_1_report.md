# Decision Report: Project Install Integration

## Question

How should runProjectInstall detect org-scoped tool keys, bootstrap distributed providers, and handle auto-registration from project config?

## Status

COMPLETE

## Chosen Alternative

Pre-scan and batch-bootstrap: scan all tool keys for `/` before the install loop, call ensureDistributedSource once per unique source, then let the existing per-tool loop pass qualified names through unchanged.

## Confidence

high

## Rationale

The pre-scan approach reuses every existing function (parseDistributedName, ensureDistributedSource, addDistributedProvider) without modification. It keeps the change isolated to runProjectInstall, avoids N redundant config loads/saves for N tools from the same source, and naturally handles the provider lifecycle problem (providers must exist before runInstallWithTelemetry calls the loader). CI-friendliness is preserved because ensureDistributedSource already handles auto-registration when not interactive, and the --yes flag propagates through installYes which is already in scope.

## Considered Alternatives

### Alternative A: Pre-scan and batch-bootstrap

**Description:** Before the install loop, iterate all tool keys, call parseDistributedName on each, collect unique sources, and call ensureDistributedSource once per source. Then in the per-tool loop, build the qualified name (`source:recipe`) for distributed tools and pass the recipe name to runInstallWithTelemetry, mirroring what the CLI path does. The system config is loaded once and reused.

**Pros:**
- Reuses parseDistributedName and ensureDistributedSource exactly as-is -- zero changes to install_distributed.go
- Deduplicates source registration: if .tsuku.toml has 5 tools from `myorg/recipes`, the source is registered and its provider bootstrapped once
- Config load (config.DefaultConfig) happens once, not per-tool
- Clear separation of concerns: source setup is a distinct phase before tool installation
- Backward compatible: bare keys don't contain `/`, so parseDistributedName returns nil and they skip the distributed path entirely
- The loader already handles qualified names via getFromDistributed, so no loader changes needed

**Cons:**
- Adds ~20 lines to runProjectInstall for the pre-scan phase
- Source collision checking (checkSourceCollision) needs to happen per-tool, not per-source, adding a small amount of logic inside the loop

**Verdict:** Chosen

### Alternative B: Inline detection per tool in the install loop

**Description:** Inside the existing `for _, t := range tools` loop, check each tool name with parseDistributedName. If it matches, call ensureDistributedSource, build the qualified name, and proceed. This mirrors the CLI path's inline approach from install.go lines 213-283.

**Pros:**
- Mirrors the CLI path structure closely -- easy to understand by analogy
- No pre-scan phase; all logic is in one place

**Cons:**
- Calls ensureDistributedSource N times for N tools from the same source. While hasDistributedProvider short-circuits the provider creation, it still loads user config and checks registration each time
- config.DefaultConfig() would be called inside the loop unless hoisted, adding either redundancy or an awkward conditional hoist
- Mixes source lifecycle management with per-tool installation logic, making the function harder to follow
- The CLI path can afford this because users typically install one tool at a time; project install is inherently batch

**Verdict:** Rejected -- the batch nature of project install makes per-tool source bootstrapping wasteful and harder to reason about.

### Alternative C: Dedicated [sources] section in .tsuku.toml

**Description:** Add a new `[sources]` TOML section to ProjectConfig where users explicitly declare distributed sources. runProjectInstall reads this section, bootstraps providers from it, and tool keys reference recipes by bare name (with source resolved from the declared sources). Example:

```toml
[sources]
myorg = "myorg/recipes"

[tools]
mytool = "1.0.0"
```

**Pros:**
- Clean separation between source declaration and tool requirements
- No need to parse tool keys for `/` -- source context is explicit
- Could support source-level configuration (branch, tag, etc.)

**Cons:**
- New abstraction: requires changes to ProjectConfig, project.LoadProjectConfig, and TOML schema
- Breaks the constraint "consistency with CLI" -- `tsuku install myorg/recipes:mytool` works with `/` syntax, but the config would use a different model
- Bare tool keys become ambiguous: is `mytool` from central or from `myorg`? Needs resolution rules
- Larger surface area: new config section, new validation, new documentation
- Not self-contained for CI: the mapping between source alias and tool name requires understanding the indirection

**Verdict:** Rejected -- too much new surface area for the initial implementation. Could be revisited as an optimization if users find the `org/repo:tool` key syntax too verbose.

### Alternative D: Extract shared helper used by both CLI and project paths

**Description:** Refactor the distributed handling from install.go's Run function into a shared helper (e.g., `prepareDistributedInstall(name, autoApprove) (recipeName, version, error)`), then call it from both the CLI path and runProjectInstall.

**Pros:**
- Eliminates duplication between the two paths
- Future changes to distributed handling automatically apply to both

**Cons:**
- The CLI path does more than just preparation: it handles dry-run, recipe hash computation, source collision checks, and recordDistributedSource as post-install steps. Extracting a clean shared helper means either a very narrow helper (just source bootstrapping, which Alternative A already achieves by calling existing functions) or a wide helper that bundles unrelated concerns
- Refactoring the CLI path risks regressions in the already-working single-tool flow
- The two paths have different error handling (project install is lenient, CLI exits immediately) making a shared flow awkward

**Verdict:** Rejected -- the shared surface is small enough (parseDistributedName + ensureDistributedSource) that direct calls are simpler than a new abstraction layer. The error handling divergence makes a unified flow impractical.

## Assumptions

- Tool keys in .tsuku.toml will use the same `owner/repo:recipe` syntax as the CLI. The map key itself carries the distributed source information.
- The installYes flag (from `--yes` / `-y`) is sufficient for non-interactive auto-approval of unregistered sources in CI. No new flag is needed.
- Source collision checking can use the same checkSourceCollision function, called per-tool inside the install loop after providers are bootstrapped.
- Recording distributed source metadata (recordDistributedSource) should happen per-tool after install, same as the CLI path.

## Implications

- ProjectConfig.Tools map keys will contain `/` characters for org-scoped tools (e.g., `"myorg/recipes:mytool"` as a TOML key). TOML supports this with quoted keys.
- The display logic in runProjectInstall needs to handle qualified names gracefully -- showing `myorg/recipes:mytool@1.0.0` in the tool list.
- The pre-scan phase creates a natural extension point for future batch optimizations (e.g., prefetching recipe indexes from distributed sources).
- Unpinned version warnings should apply equally to distributed tools.
- Error messages should distinguish between source bootstrap failures (all tools from that source will fail) and individual tool failures.
