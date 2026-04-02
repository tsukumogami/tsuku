# Decision 2: Project Config Injection into MaybeAutoApply

## Question

Where and how is project config injected into MaybeAutoApply? Covers integration point, loading strategy, and any ToolRequirement changes.

## Analysis

### Option A: Pass project config as parameter

Add `*project.ConfigResult` to MaybeAutoApply's signature. The caller (main.go PersistentPreRun) loads it via `LoadProjectConfig(cwd)` and passes it in.

**Pros:**
- Explicit dependency -- clear from the signature what MaybeAutoApply uses.
- Testable without filesystem -- tests construct a ConfigResult directly.
- Matches the pattern in `cmd_run.go` where the caller loads project config and passes it to downstream consumers (NewResolver, Runner).

**Cons:**
- Adds a parameter to an already 4-parameter function.
- Introduces `internal/project` as a transitive type dependency for anyone calling MaybeAutoApply (though only main.go calls it).
- Every future consumer of MaybeAutoApply must know to load project config.

### Option B: Load project config inside MaybeAutoApply

MaybeAutoApply calls `LoadProjectConfig(os.Getwd())` internally. No signature change.

**Pros:**
- No caller changes. Encapsulated.
- Only one call site exists (main.go), so "protecting future callers" has limited value.

**Cons:**
- Hidden dependency on CWD and filesystem. Tests must set up real `.tsuku.toml` files or mock `os.Getwd()`.
- `internal/updates` gains a direct import on `internal/project`. Currently the updates package depends on config, install, log, notices, telemetry, and userconfig. Adding project is another edge in the dependency graph.
- Breaks the pattern established by cmd_run.go and shellenv/activate.go, where the *caller* loads project config and passes it to the consumer. Every existing usage of LoadProjectConfig follows caller-loads-and-passes.

### Option C: Inject a filter function

Add `filterFn func(tool string, entry UpdateCheckEntry) bool`. The caller builds it from project config.

**Pros:**
- Maximum decoupling -- updates package has zero knowledge of project config.
- Easy to test: pass a lambda.

**Cons:**
- Indirection obscures intent. A reader of MaybeAutoApply won't understand *why* entries are filtered without reading the caller.
- The filter needs access to `LatestWithinPin` and the project version constraint to decide whether the update is within the project's pin. This means the filter callback signature gets complex or leaks update internals.
- Over-engineered for a single use case. There's one caller and one filtering concern.

### ToolRequirement Extension

The current `ToolRequirement` has only `Version string`. Should it gain an `AutoUpdate bool`?

**No.** The Version field already contains sufficient information:
- Exact versions (e.g., "20.16.0") suppress auto-update: `PinLevelFromRequested` returns `PinExact`, and `LatestWithinPin` will equal `ActiveVersion`, so no update is pending.
- Prefix versions (e.g., "20") allow auto-update within the pin boundary.

An `AutoUpdate` bool would let users pin `node = "20.16.0"` but still auto-update, which contradicts the mental model: if you pin an exact version, you want that version. The design doc's scope section explicitly excludes per-tool auto-update config in `.tsuku.toml`. The existing pin semantics via `PinLevelFromRequested` and `VersionMatchesPin` handle everything needed.

## Recommendation

**Option A: Pass project config as parameter.**

Rationale:

1. **Consistency.** Every existing call to `LoadProjectConfig` follows the same pattern: the command-level code calls it from CWD and passes the result to internal packages. `cmd_run.go` (line 94), `cmd_shim.go` (line 65), `shellenv/activate.go` (line 48), and `install_project.go` (line 42) all do this. Option B would be the only case where an internal package loads project config itself.

2. **Testability.** The existing MaybeAutoApply tests construct configs in memory. Adding a `*project.ConfigResult` parameter (nil-safe, like `userCfg`) keeps that pattern. Option B forces test setup to write `.tsuku.toml` files to temp dirs and control CWD.

3. **Simplicity over cleverness.** Option C's filter function is more abstract than needed. There's one caller and one filtering concern. The filter would need enough context about pin semantics that it essentially reimplements the comparison logic outside the updates package.

4. **Nil-safe convention.** The function already handles `userCfg == nil` gracefully. A nil `*project.ConfigResult` means "no project context" -- MaybeAutoApply applies global behavior unchanged. This matches how cmd_run.go treats a nil project config.

The signature becomes:
```go
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config,
    projectCfg *project.ConfigResult, installFn InstallFunc,
    tc *telemetry.Client) []ApplyResult
```

The caller change in main.go is minimal -- add three lines to load project config (matching the pattern already used in cmd_run.go) and pass it through.

**ToolRequirement: no changes needed.** The Version field combined with existing pin-level logic is sufficient.

## Confidence

High. The codebase has a clear, consistent pattern for project config loading. Option A follows it exactly. The alternatives either break consistency (B) or add unnecessary abstraction (C).
