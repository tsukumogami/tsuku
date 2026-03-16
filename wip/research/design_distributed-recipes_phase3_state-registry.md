# Phase 3 Research: State & Registry Management

## Questions Investigated
1. Current ToolState structure and Plan definition; how state is persisted and loaded
2. How RecipeSource flows through plan generation
3. How to add a source field to ToolState with lazy migration
4. Where to store registered distributed sources
5. Which commands need source awareness and what changes are needed
6. How `tsuku update <tool>` knows which source to check
7. Name collision handling between distributed sources

## Findings

### 1. Current State Structure

**File: `internal/install/state.go`**

The top-level `State` struct (line 115-119):
```go
type State struct {
    Installed map[string]ToolState                      `json:"installed"`
    Libs      map[string]map[string]LibraryVersionState `json:"libs,omitempty"`
    LLMUsage  *LLMUsage                                 `json:"llm_usage,omitempty"`
}
```

`ToolState` (line 74-91) contains:
- `ActiveVersion string` -- currently symlinked version
- `Versions map[string]VersionState` -- all installed versions
- `Version string` -- deprecated, kept for migration
- `IsExplicit bool` -- user-requested tool
- `RequiredBy []string` -- reverse dependency tracking
- `IsHidden bool` -- hidden from PATH/list
- `IsExecutionDependency bool` -- installed by tsuku internally
- `InstalledVia string` -- package manager used (npm, pip, cargo)
- `Binaries []string` -- deprecated, use Versions[v].Binaries
- `InstallDependencies []string`
- `RuntimeDependencies []string`

`VersionState` (line 16-24) contains a `Plan *Plan` which stores `RecipeSource string` (line 35).

**Key observation:** RecipeSource is already stored per-version inside the cached Plan. This means for any installed tool, we can already determine its recipe source from `ToolState.Versions[v].Plan.RecipeSource`. Current values are `"registry"` or `"local"` (set in `cmd/tsuku/helpers.go:177-179`).

**Persistence:** `StateManager` (line 122-125) uses `$TSUKU_HOME/state.json` with file locking (shared for reads, exclusive for writes) and atomic writes (write to `.tmp`, rename). In-process concurrency is protected by `sync.RWMutex`.

**Migration pattern:** The `migrateToMultiVersion()` method (line 125-144 in `state_tool.go`) runs on every `Load()` call. It detects old format entries (have `Version` but no `ActiveVersion`) and upgrades them in memory. This is the lazy migration pattern to follow.

### 2. RecipeSource Flow Through Plan Generation

**File: `internal/executor/plan_generator.go`**

`PlanConfig.RecipeSource` (line 30) is a string passed into plan generation. It flows into the `InstallationPlan` struct and is stored in state when the plan is cached in `VersionState.Plan`.

**File: `cmd/tsuku/helpers.go:172-179`**

RecipeSource is set at the CLI level during plan generation:
```go
if recipePath != "" {
    r, err = recipe.ParseFile(recipePath, constraintLookup)
    recipeSource = "local"
} else {
    r, err = loader.Get(toolName, recipe.LoaderOptions{})
    recipeSource = "registry"
}
```

This is currently binary: `"local"` (file path) or `"registry"` (loader chain). With distributed recipes, this would need to carry the owner/repo identifier for distributed sources.

### 3. Adding Source Field to ToolState

**Recommendation: Add `Source string` to ToolState (top-level)**

```go
type ToolState struct {
    ActiveVersion string `json:"active_version,omitempty"`
    Versions      map[string]VersionState `json:"versions,omitempty"`
    Source        string `json:"source,omitempty"` // "central", "embedded", file path, or "owner/repo"
    // ... existing fields ...
}
```

**Rationale for top-level rather than per-version:**
- A tool comes from one source. You don't install v1 from `alice/tools` and v2 from `bob/tools` under the same name.
- The source determines where to check for updates, which is a tool-level concern.
- `Plan.RecipeSource` already exists per-version and records the recipe origin at install time. The new `Source` field is the *authoritative* source for future operations (update, outdated).

**When is it set?**
- On first install: derived from how the recipe was resolved. Central registry -> `"central"`, embedded -> `"embedded"`, local file -> file path, distributed -> `"owner/repo"`.
- On `tsuku install --recipe ./foo.toml`: `Source` = absolute path or `"local"`.
- On update: source doesn't change (the tool's source is sticky).

**Lazy migration for existing entries:**
```go
func (s *State) migrateToolSource() {
    for name, tool := range s.Installed {
        if tool.Source == "" {
            // Infer from cached plan if available
            if tool.ActiveVersion != "" {
                if vs, ok := tool.Versions[tool.ActiveVersion]; ok && vs.Plan != nil {
                    switch vs.Plan.RecipeSource {
                    case "registry":
                        tool.Source = "central"
                    case "local":
                        tool.Source = "local"
                    default:
                        tool.Source = "central" // safe default
                    }
                } else {
                    tool.Source = "central" // pre-plan installations
                }
            } else {
                tool.Source = "central"
            }
            s.Installed[name] = tool
        }
    }
}
```

This runs in the `Load()` path alongside `migrateToMultiVersion()`. Existing tools get `"central"` as the default, which is correct -- they all came from the central registry.

### 4. Where to Store Registered Distributed Sources

**File: `internal/config/config.go`** -- `Config` struct defines directory paths. No user-editable config file parsing here; that's in `internal/userconfig/userconfig.go` which manages `$TSUKU_HOME/config.toml` (TOML format, loaded by `userconfig.Load()`).

**File: `internal/userconfig/userconfig.go`** -- `Config` struct has `Telemetry`, `LLM`, and `Secrets` fields. Saved atomically with 0600 permissions.

**Three options evaluated:**

**Option A: In state.json (new top-level field)**
```json
{
  "installed": { ... },
  "libs": { ... },
  "registries": {
    "alice/tools": {
      "url": "https://github.com/alice/tools",
      "added_at": "2026-03-15T10:00:00Z",
      "auto_registered": true
    }
  }
}
```
- Pro: Single file, atomic with tool state, no new persistence layer.
- Con: Mixes operational state (what's installed) with configuration (what sources are trusted). State is managed with exclusive locks -- registry edits would contend with install operations.

**Option B: In config.toml (new section)**
```toml
[registries]
[registries."alice/tools"]
url = "https://github.com/alice/tools"
auto_registered = true

[registries."bob/recipes"]
url = "https://github.com/bob/recipes"
```
- Pro: Config.toml already exists and handles user preferences. Registries are a user preference (which sources to trust). Editable by hand. No contention with state operations.
- Con: config.toml currently has 0600 permissions and stores secrets. Registries aren't secret, but they'd inherit this model.

**Option C: Separate `$TSUKU_HOME/registries.json`**
```json
{
  "registries": {
    "alice/tools": {
      "url": "https://github.com/alice/tools",
      "added_at": "2026-03-15T10:00:00Z",
      "auto_registered": true
    }
  }
}
```
- Pro: Clean separation, no contention, simple to reason about. Can have its own locking.
- Con: Yet another file, another persistence layer to maintain.

**Recommendation: Option B (config.toml)**

Registries are user configuration -- "which sources do I trust" -- not runtime state. They belong alongside telemetry and LLM preferences. The `userconfig` package already has `Load()`, `Save()`, `Get()`, `Set()` patterns that registry management can follow. The `tsuku config set` / `tsuku config get` infrastructure can be extended, and `tsuku registry add/remove/list` commands would use the same `userconfig.Config` struct.

Add to `userconfig.Config`:
```go
type Config struct {
    Telemetry  bool              `toml:"telemetry"`
    LLM        LLMConfig         `toml:"llm"`
    Secrets    map[string]string `toml:"secrets,omitempty"`
    Registries map[string]RegistryEntry `toml:"registries,omitempty"`
}

type RegistryEntry struct {
    URL            string `toml:"url"`
    AutoRegistered bool   `toml:"auto_registered,omitempty"`
}
```

**Strict mode** (`strict_registries`) also belongs in config.toml:
```toml
strict_registries = false  # default: allow auto-registration

[registries."alice/tools"]
url = "https://github.com/alice/tools"
```

### 5. Command Source Awareness

**`cmd/tsuku/list.go`** -- Lists installed tools via `mgr.List()` which returns `InstalledTool{Name, Version, Path, IsActive}`. Currently no source info displayed. Changes needed: optionally show source in output (e.g., `[alice/tools]` suffix), expose source in `--json` output. This requires `InstalledTool` to grow a `Source` field, populated from `ToolState.Source`.

**`cmd/tsuku/info.go`** -- Loads recipe via `loader.Get(toolName, ...)`. For distributed tools, the loader chain would need to check registered distributed sources. The info output would show recipe source. Moderate change: extend `infoOutput` JSON struct.

**`cmd/tsuku/outdated.go`** -- Currently loads each recipe via `loader.Get()` and extracts the GitHub repo from step params to check latest version. This is fragile (it reverse-engineers the repo from the recipe). With source tracking, `outdated` could use `ToolState.Source` to determine where to check. For distributed sources, it would fetch the recipe from the distributed registry and use its version provider. Moderate change.

**`cmd/tsuku/verify.go`** -- Loads recipe via `loader.Get(name, ...)` for verification commands. For distributed tools, the loader needs to find the recipe from the right source. The verification logic itself doesn't change. Small change: loader needs to consult the tool's recorded source.

**`cmd/tsuku/recipes.go`** -- Lists recipes via `loader.ListAllWithSource()`. Returns `RecipeInfo{Name, Description, Source}` where `Source` is currently `SourceLocal`, `SourceEmbedded`, or `SourceRegistry`. New source type needed: `SourceDistributed` with owner/repo metadata. `RecipeInfo` struct needs extension.

**`cmd/tsuku/update.go`** -- Calls `runInstallWithTelemetry()` which goes through the normal install flow. Currently doesn't consult state for source. Needs to pass source info so the recipe loader resolves from the correct distributed registry.

**`cmd/tsuku/update_registry.go`** -- Currently refreshes the central registry cache. Would need to also refresh distributed registry caches. New command `tsuku registry list/add/remove` would be separate files.

### 6. How `tsuku update <tool>` Knows Which Source to Check

Currently, `update.go` calls `runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient)` which goes through the normal recipe loading chain (local > embedded > registry). There's no source-awareness at all.

**With source tracking:**
1. Before resolving the recipe, `update` reads `ToolState.Source` for the tool.
2. If source is `"central"` or `"embedded"`, use the existing loader chain (no change).
3. If source is `"owner/repo"`, pass that info to the loader so it fetches from the distributed registry.
4. If source is a file path, use `recipe.ParseFile(path)`.

The key architectural point: the recipe `Loader` needs a new method or option to resolve from a specific distributed source. Something like:
```go
loader.GetFromSource(ctx, toolName, "alice/tools", recipe.LoaderOptions{})
```

Or `LoaderOptions` gains a `Source string` field that overrides the normal priority chain.

### 7. Name Collisions Between Distributed Sources

**Current key: `state.Installed` is `map[string]ToolState`**, keyed by tool name (e.g., `"kubectl"`, `"gh"`). This is a flat namespace.

If `alice/tools` and `bob/tools` both define `mytool`, there's a collision. Two approaches:

**Approach A: Last-install-wins (simple, matches current behavior)**
- `state.Installed["mytool"]` points to whichever was installed last.
- Source is tracked, so `tsuku info mytool` shows which source it came from.
- Installing from a different source replaces the existing installation.
- This is how most package managers work (Homebrew taps, npm scoped packages).

**Approach B: Namespaced keys (e.g., `alice/tools/mytool`)**
- `state.Installed["alice/tools/mytool"]` and `state.Installed["bob/tools/mytool"]` coexist.
- Both could install different binaries.
- Massive blast radius: every command that reads state by tool name needs updating.
- PATH conflicts for binaries with the same name.

**Recommendation: Approach A (last-install-wins)** with collision detection.

When a user runs `tsuku install mytool` and it resolves from `alice/tools`, but `state.Installed["mytool"]` already exists with `Source: "bob/tools"`:
1. Warn the user: "mytool is currently installed from bob/tools. Replace with alice/tools? [y/N]"
2. If confirmed, update source and proceed with install.
3. `--force` flag skips the prompt.

For auto-registration (R2), the installed tool's source is recorded. If a tool with the same name exists from a different source, auto-registration still happens for the source, but the install requires explicit user consent for the replacement.

## Implications for Design

### RecipeSource Values Need Standardization

Currently `Plan.RecipeSource` is `"registry"` or `"local"`. The new `ToolState.Source` needs richer values:
- `"central"` -- from the central registry (what `"registry"` means today)
- `"embedded"` -- from the embedded binary registry
- `"local"` or absolute file path -- from a local recipe file
- `"owner/repo"` -- from a distributed GitHub repository

The plan's `RecipeSource` should align with these values. This means updating `cmd/tsuku/helpers.go:177-179` where `recipeSource` is set.

### Loader Architecture Impact

The recipe `Loader` currently has a fixed priority chain: cache > local > embedded > registry. Distributed sources need to be inserted. Two options:
1. **Extend the existing chain:** Add distributed sources between local and registry.
2. **Source-directed loading:** When `ToolState.Source` is known (update, verify, outdated), bypass the chain and load directly from that source.

Option 2 is cleaner. The priority chain is for *initial* resolution (when the user types `tsuku install foo` without specifying a source). For subsequent operations, the recorded source is authoritative.

### Config vs State Separation

This design splits registry information across two files:
- **config.toml**: Which distributed registries are known (user configuration, R8/R9)
- **state.json**: Which source each tool came from (operational state, R6)

This is the right separation. Config answers "where can I look?" State answers "where did this come from?"

### Migration Path (R18)

Lazy migration covers existing state.json entries. Since all currently installed tools come from the central registry or embedded, defaulting `Source` to `"central"` is safe. The `config.toml` file doesn't need migration -- it simply gains a new `[registries]` section when the user first adds a distributed source.

### Registry Management Commands (R8)

New commands map naturally to `userconfig` operations:
- `tsuku registry list` -- reads `config.Registries`, displays registered sources
- `tsuku registry add <name> <source>` -- validates source, adds to `config.Registries`, saves
- `tsuku registry remove <name>` -- removes from `config.Registries`, saves. Per R13, does NOT remove tools installed from that source.

These are lightweight commands that don't touch state.json at all.

## Surprises

1. **RecipeSource already exists in state.** The `Plan.RecipeSource` field is already stored per version in state.json. This means we're not starting from zero on source tracking -- we're promoting an existing per-version-plan field to a first-class top-level ToolState field. This significantly reduces risk.

2. **The `outdated` command is fragile.** It reverse-engineers the GitHub repo by scanning recipe steps for `github_archive`/`github_file` actions and extracting the `repo` param. This won't work for distributed sources that use different version providers. The `outdated` command needs a more principled approach: load the recipe from the tool's recorded source and use its version provider.

3. **No config.toml parsing in the install path.** Currently `userconfig.Load()` is only used by `cmd/tsuku/config.go` and LLM/secrets features. The install/update/outdated paths never read config.toml. Adding registry configuration to config.toml means the install path now needs to load it to discover registered distributed sources. This is a new dependency for performance-sensitive paths.

4. **`InstalledVia` field exists but serves a different purpose.** It tracks the package manager used (npm, pip, cargo), not the recipe source. These are orthogonal: a tool from `alice/tools` might still be `InstalledVia: "cargo"`. No conflict, but the naming proximity could cause confusion.

5. **The Loader has no concept of a "source registry."** It has local, embedded, and one central registry. Adding multiple distributed registries means the Loader needs to manage a set of registry clients, each with their own cache directory. The `$TSUKU_HOME/registry/` directory currently caches the central registry. Distributed sources would need separate cache directories, e.g., `$TSUKU_HOME/cache/registries/alice-tools/`.

## Summary

State.json already stores RecipeSource per-version inside cached plans, so adding a top-level `Source` field to ToolState is a natural promotion with clean lazy migration (default everything to `"central"`). Registered distributed sources belong in config.toml alongside other user preferences, keeping a clean separation from operational state. The biggest architectural gap is in the recipe Loader, which has no concept of multiple registry sources and will need source-directed loading for update/verify/outdated operations on distributed tools.
