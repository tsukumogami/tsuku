---
status: Proposed
problem: |
  Org-scoped recipes (like tsukumogami/koto) have no working syntax in
  .tsuku.toml. The TOML format forbids slash in bare keys, the project install
  path lacks distributed-name detection, and the resolver's binary index uses
  bare recipe names that don't match org-prefixed config keys. This makes
  .tsuku.toml impractical for CI environments that need self-contained config.
decision: |
  Use TOML quoted keys ("tsukumogami/koto" = "latest") with a pre-scan phase
  in runProjectInstall that batch-bootstraps distributed providers, and a
  dual-key lookup in the resolver that maps bare binary-index names back to
  org-scoped config keys via a reverse map.
rationale: |
  Quoted keys require zero struct changes and are fully backward compatible.
  The pre-scan reuses existing ensureDistributedSource without modification.
  The dual-key resolver keeps the binary index bare-name-only (preserving
  path traversal guards) while isolating the reconciliation to a single point.
  Alternatives (dotted keys, explicit registries, index schema changes) were
  rejected for parsing ambiguity, excessive surface area, or downstream breakage.
---

# DESIGN: Org-Scoped Project Config

## Status

Proposed

## Context and Problem Statement

Issue #2230 documents five different syntax attempts for org-scoped recipes in `.tsuku.toml`, none of which work reliably. The only combination that works (`"tsukumogami/koto" = ""` with a pre-registered registry) requires prior manual `tsuku install` -- defeating the purpose of declarative project config.

The problem has three layers. First, TOML syntax: bare keys can't contain `/`, so `tsukumogami/koto = "latest"` is a parse error. Second, runtime: `runProjectInstall` passes tool names directly to `runInstallWithTelemetry` without distributed-name detection, so even quoted keys like `"tsukumogami/koto"` fail at recipe lookup. Third, the resolver: the binary index stores bare recipe names (`koto`), but the config map is keyed by the full org-scoped name (`tsukumogami/koto`), breaking shell integration version pinning.

## Decision Drivers

- CI-friendliness: `.tsuku.toml` must be self-contained -- no prior `tsuku install` or manual registry setup
- Backward compatibility: existing `.tsuku.toml` files with bare keys must continue working
- Minimal surface area: prefer reusing existing distributed-source machinery over new abstractions
- Consistency with CLI: `tsuku install tsukumogami/koto` works; the config should feel similar
- TOML ergonomics: quoted keys are a minor friction but widely precedented (mise, devcontainer.json)

## Decisions Already Made

During exploration, the following options were evaluated and eliminated:

- **Dotted keys** (`[tools.tsukumogami]` / `koto = "latest"`): parsing ambiguity between org scope and nested config table. Issue #2230 already flagged this as "misinterpreted as nested config."
- **Array of tables** (`[[tools]]` with name/version fields): completely breaking migration, excessive verbosity.
- **Value-side encoding** (`koto = "tsukumogami/koto@latest"`): stringly-typed, fragile, less self-documenting.
- **Explicit `[registries]` section in `.tsuku.toml`**: too verbose for the common case. The org prefix in the tool key already identifies the distributed source, making auto-registration sufficient.
- **Quoted-key approach chosen**: `"tsukumogami/koto" = "latest"` requires zero struct changes, full backward compatibility, and follows mise and devcontainer.json precedent.

## Considered Options

### Decision 1: Project Install Integration

When a user runs `tsuku install` (no args) in a directory with a `.tsuku.toml` containing org-scoped tools, `runProjectInstall` needs to bootstrap distributed providers before attempting recipe lookups. The CLI install path already handles this through `parseDistributedName` and `ensureDistributedSource`, but the project install path bypasses all distributed-source logic. The question is how to bridge this gap without duplicating the CLI path's complexity or breaking batch install performance.

#### Chosen: Pre-scan and batch-bootstrap

Before the per-tool install loop, scan all tool keys for `/` using `parseDistributedName`. Collect unique sources, load user config once, and call `ensureDistributedSource` once per unique source. This bootstraps all distributed providers upfront, so the existing per-tool loop can build qualified names (`source:recipe`) and pass them through the loader's `getFromDistributed` path.

For a `.tsuku.toml` with 5 tools from `myorg/recipes`, the source is registered and its provider bootstrapped once rather than five times. The `installYes` flag (from `--yes`/`-y`) propagates to `ensureDistributedSource` for non-interactive auto-approval, making CI work without manual intervention. After each distributed tool installs, `recordDistributedSource` records the recipe hash and `checkSourceCollision` validates consistency, same as the CLI path.

#### Alternatives Considered

**Inline per-tool detection**: Check each tool name inside the install loop and call `ensureDistributedSource` per tool. Rejected because the batch nature of project install makes per-tool source bootstrapping wasteful -- it loads user config N times and checks registration N times for the same source.

**Dedicated `[sources]` section in `.tsuku.toml`**: Add a new TOML section for explicit source declarations. Rejected because it adds a new config schema, breaks consistency with the CLI's `org/tool` syntax, and makes bare tool keys ambiguous across sources.

**Extract shared helper for both CLI and project paths**: Refactor the CLI's distributed handling into a reusable function. Rejected because the two paths have different error handling (project install is lenient and continues on failure; CLI exits immediately), making a unified flow awkward. The shared surface is small enough (calling existing functions directly) that a new abstraction isn't warranted.

### Decision 2: Resolver Key Matching

The resolver maps binary names to project-pinned versions for shell integration. It uses a binary index to find which recipe provides a command, then looks up that recipe in `config.Tools`. The binary index stores bare recipe names (`koto`), but org-scoped config keys are `tsukumogami/koto`. This mismatch breaks shell integration version pinning for distributed tools. The fix must handle the common case (no collision) efficiently while correctly resolving the uncommon case (two orgs providing a tool with the same binary name).

#### Chosen: Dual-key lookup in the resolver

At resolver construction, build a reverse map from bare recipe names to their org-scoped config keys by scanning `config.Tools`. For each key containing `/`, extract the bare suffix (everything after the last `/`). In `ProjectVersionFor`, after the binary index returns matches, try `config.Tools[m.Recipe]` first (fast path for bare keys), then fall back to checking the reverse map for any org-scoped keys that match.

For name collisions (two orgs ship a tool with the same binary name), the reverse map stores multiple org-scoped keys per bare name. The resolver checks all matches against config entries, returning the first hit. Since the user's `.tsuku.toml` pins a specific org-scoped tool, only that entry matches.

A shared utility `splitOrgKey(key) (source, bare string, isOrgScoped bool)` is extracted since both `runProjectInstall` and the resolver need the same name-splitting logic.

#### Alternatives Considered

**Normalize config keys at parse time**: Detect org-scoped keys in `LoadProjectConfig` and add duplicate entries under bare names. Rejected because it loses org identity (needed for auto-registration), creates silent last-write-wins conflicts for name collisions, and makes `config.Tools` iteration confusing with phantom entries.

**Store org-scoped names in the binary index**: Insert index rows with `recipe = "tsukumogami/koto"` instead of bare `koto`. Rejected because it breaks `isValidRecipeName` (which guards against path traversal by rejecting `/` in names), creates a parallel namespace in the index that every consumer must handle, and diverges from state.json's bare-name convention.

**Persistent org-to-recipe mapping table in index DB**: Add a SQLite table mapping org keys to bare names. Rejected because the mapping is already derivable from config keys at zero cost, making persistence unnecessary overhead.

## Decision Outcome

**Chosen: Pre-scan batch-bootstrap + dual-key resolver lookup**

### Summary

The fix touches two code paths. In `runProjectInstall` (`install_project.go`), a new pre-scan phase iterates all tool keys before the install loop. Keys containing `/` are parsed via `parseDistributedName` to extract source and recipe names. Unique sources are collected and bootstrapped once each via `ensureDistributedSource`, which auto-registers unregistered sources (respecting `--yes` for CI) and creates distributed providers for the session. The per-tool install loop then builds qualified names (`source:recipe`) for distributed tools and passes them through the existing loader path. Bare-key tools are unaffected.

In the resolver (`resolver.go`), construction builds a reverse map from bare recipe names to org-scoped config keys. `ProjectVersionFor` tries the bare key first (backward-compatible fast path), then checks the reverse map for org-scoped matches. A shared `splitOrgKey` utility extracts the source prefix and bare name from org-scoped keys, used by both `runProjectInstall` and the resolver.

The `.tsuku.toml` syntax uses TOML quoted keys: `"tsukumogami/koto" = "latest"`. This requires no changes to `ProjectConfig`, `ToolRequirement`, or config parsing -- the TOML library handles quoted keys natively, and the map key arrives as the string `tsukumogami/koto`.

### Rationale

The two decisions reinforce each other. The pre-scan ensures distributed providers exist before any tool installation or resolver lookup. The dual-key resolver handles the naming mismatch that the pre-scan creates (org-scoped config keys vs. bare binary-index names). Both use the same `splitOrgKey` utility and the same convention: keys with `/` are org-scoped, keys without are bare.

The approach reuses every existing function (`parseDistributedName`, `ensureDistributedSource`, `getFromDistributed`, `checkSourceCollision`, `recordDistributedSource`) without modification. Changes are isolated to `install_project.go` (pre-scan) and `resolver.go` (reverse map). The `ProjectConfig` struct, config parser, binary index schema, and state.json format are all unchanged.

## Solution Architecture

### Overview

The solution adds org-scoped tool support to `.tsuku.toml` by teaching two existing code paths about the `owner/repo` naming convention. No new packages, config sections, or data structures are introduced. The TOML quoted-key syntax (`"tsukumogami/koto" = "latest"`) already parses correctly; the work is in making the runtime handle what the parser produces.

### Components

**1. `splitOrgKey` utility** (`internal/project/orgkey.go`, new file)

```go
// splitOrgKey splits an org-scoped tool key into source and bare recipe name.
// For "tsukumogami/koto", returns ("tsukumogami/koto", "koto", true).
// For "tsukumogami/registry:mytool", returns ("tsukumogami/registry", "mytool", true).
// For "node", returns ("", "node", false).
// Returns an error for malformed sources (path traversal, invalid format).
func splitOrgKey(key string) (source, bare string, isOrgScoped bool, err error)
```

Used by both `runProjectInstall` and the resolver. Delegates to the existing `parseDistributedName` parsing logic for the `owner/repo:recipe` format but lives in `internal/project/` to avoid a dependency from `internal/project` on `cmd/tsuku`. Validates the source component via the same rules as `validateRegistrySource` (must be exactly `owner/repo`, no path traversal).

**2. Pre-scan in `runProjectInstall`** (`cmd/tsuku/install_project.go`, modified)

After loading `.tsuku.toml` and building the sorted tool list, but before the confirmation prompt:

1. Iterate tool keys, call `parseDistributedName` on each
2. Collect unique sources into a `map[string]bool`
3. Load system config once via `config.DefaultConfig()`
4. For each unique source, call `ensureDistributedSource(source, installYes, sysCfg)`
5. If any source fails to bootstrap, mark all tools from that source as failed and continue with remaining tools

In the per-tool install loop, for org-scoped tools:
- Build the qualified name: `dArgs.Source + ":" + dArgs.RecipeName`
- Call `checkSourceCollision(dArgs.RecipeName, dArgs.Source, installForce, sysCfg)` before install
- Pass `dArgs.RecipeName` as the tool name to `runInstallWithTelemetry`
- After success, call `fetchRecipeBytes` + `computeRecipeHash` to compute the recipe hash, then `recordDistributedSource` to record the source and hash in state.json (parity with CLI path)

**3. Dual-key lookup in resolver** (`internal/project/resolver.go`, modified)

```go
type Resolver struct {
    config     *ConfigResult
    lookup     autoinstall.LookupFunc
    bareToOrg  map[string][]string // bare recipe name -> org-scoped config keys
}
```

`NewResolver` scans `config.Tools` keys, calling `splitOrgKey` on each. Keys with `isOrgScoped == true` are added to `bareToOrg[bare] = append(bareToOrg[bare], key)`. Values are sorted alphabetically for deterministic resolution order when multiple org-scoped keys map to the same bare name.

`ProjectVersionFor` changes from:

```go
for _, m := range matches {
    if req, ok := r.config.Config.Tools[m.Recipe]; ok {
        return req.Version, true, nil
    }
}
```

To:

```go
for _, m := range matches {
    // Fast path: bare key match (existing behavior)
    if req, ok := r.config.Config.Tools[m.Recipe]; ok {
        return req.Version, true, nil
    }
    // Org-scoped key match via reverse map
    if orgKeys, ok := r.bareToOrg[m.Recipe]; ok {
        for _, orgKey := range orgKeys {
            if req, ok := r.config.Config.Tools[orgKey]; ok {
                return req.Version, true, nil
            }
        }
    }
}
```

### Data Flow

```
.tsuku.toml                    tsuku install (no args)
    |                                |
    v                                v
ProjectConfig.Tools ──────> runProjectInstall
  {"tsukumogami/koto": "1.0"}       |
  {"node": "20"}                    |
                              Pre-scan phase:
                              ├── parseDistributedName("tsukumogami/koto") -> source, recipe
                              ├── ensureDistributedSource("tsukumogami/koto")
                              │   ├── auto-register in config.toml (if new)
                              │   └── create DistributedRegistryProvider
                              └── parseDistributedName("node") -> nil (skip)
                                    |
                              Install loop:
                              ├── "tsukumogami/koto" -> qualified: "tsukumogami/koto:koto"
                              │   └── loader.GetWithContext("tsukumogami/koto:koto") -> getFromDistributed
                              └── "node" -> runInstallWithTelemetry("node") (unchanged)
```

```
Shell integration (shim invocation):
    koto --version
        |
        v
    Resolver.ProjectVersionFor("koto")
        |
        v
    binary index: koto -> [{Recipe: "koto", ...}]
        |
        v
    config.Tools["koto"] -> miss
    bareToOrg["koto"] -> ["tsukumogami/koto"]
    config.Tools["tsukumogami/koto"] -> {Version: "1.0"} -> hit
```

## Implementation Approach

### Phase 1: Shared utility

Add `splitOrgKey` to `internal/project/orgkey.go` with tests. This is a pure function with no dependencies.

Deliverables:
- `internal/project/orgkey.go`
- `internal/project/orgkey_test.go`

### Phase 2: Resolver dual-key lookup

Add `bareToOrg` map to `Resolver`, build it in `NewResolver`, and update `ProjectVersionFor` to check both paths. Add tests covering bare keys (unchanged), org-scoped keys, and the name collision case.

Deliverables:
- `internal/project/resolver.go` (modified)
- `internal/project/resolver_test.go` (new or extended)

### Phase 3: Project install pre-scan

Add the pre-scan phase to `runProjectInstall`. Detect org-scoped keys, batch-bootstrap sources, and route qualified names through the distributed install path. Handle source bootstrap failures gracefully (continue with remaining tools).

Deliverables:
- `cmd/tsuku/install_project.go` (modified)
- `cmd/tsuku/install_project_test.go` (new or extended)

### Phase 4: Functional tests

Add end-to-end tests covering the full `.tsuku.toml` workflow with org-scoped tools: config parsing, project install, and shell integration resolution.

Deliverables:
- `test/functional/features/project_config.feature` (new or extended)

## Security Considerations

**Trust boundary shift -- config-driven source registration**: This design moves source trust from "user explicitly types `org/repo` at the CLI" to "config file declares it." This is a meaningful change. A malicious `.tsuku.toml` checked into a cloned repository could trigger auto-registration of attacker-controlled GitHub orgs as recipe sources. The `--yes` flag, commonly used in CI, bypasses the interactive confirmation prompt, so the attacker's recipes would install automatically.

Mitigations:
- Without `--yes`, `ensureDistributedSource` prompts for confirmation before registering each new source. The confirmation prompt explicitly names the source (`Install from unregistered source "evil/recipes"?`), giving the user a chance to reject it.
- With `strict_registries` enabled, `ensureDistributedSource` blocks all unregistered sources regardless of flags. CI environments that use `--yes` should also enable `strict_registries` and pre-register trusted sources via `tsuku registry add`.
- The pre-scan phase passes `installYes` (from the `--yes`/`-y` flag) directly to `ensureDistributedSource`. This is the same flag propagation as the CLI path -- no new approval bypass mechanism is introduced.

**Path traversal via org-scoped keys**: The `parseDistributedName` function already rejects keys containing `..`. The `splitOrgKey` utility adds its own validation, rejecting malformed source strings (must be exactly `owner/repo` format). The binary index's `isValidRecipeName` continues to reject `/` in recipe names, so org-scoped names never reach the registry fetch path.

**Resolver name collision as shadowing vector**: If two org-scoped keys in `.tsuku.toml` map to the same bare recipe name (e.g., `acme/koto` and `evil/koto`), the resolver's `bareToOrg` map stores both. Values are sorted alphabetically for deterministic resolution, but this still means one org shadows the other. This is a configuration error on the user's part (pinning two different sources for the same binary). The resolver should log a warning at construction time when duplicate bare names are detected, so the user can disambiguate using the `owner/repo:recipe` qualified syntax.

**Source collision detection**: The existing `checkSourceCollision` function verifies that a tool isn't being replaced from a different source without user consent. This applies to distributed tools installed via project config, called per-tool inside the install loop.

## Consequences

### Positive

- `.tsuku.toml` becomes self-contained for CI: org-scoped tools work on fresh machines with `tsuku install -y`
- Zero breaking changes: existing configs with bare keys are completely unaffected
- No new config schema or struct fields: quoted keys work with existing `map[string]ToolRequirement`
- Consistent with CLI: `tsuku install tsukumogami/koto` and `"tsukumogami/koto" = "latest"` feel natural together

### Negative

- Quoted keys are a minor UX friction: users must write `"tsukumogami/koto"` with quotes, which is less ergonomic than bare keys
- Trust boundary shift: config-driven source registration means a malicious `.tsuku.toml` in a cloned repo could trigger auto-registration of attacker-controlled sources, especially with `--yes` in CI
- Name collision resolution in the resolver is alphabetically deterministic but still picks one org over another: if two orgs provide a tool with the same binary name, users must disambiguate manually

### Mitigations

- For quoted-key friction: `tsuku init` can generate correct quoted syntax when it detects org-scoped tools
- For trust boundary shift: `strict_registries` blocks unregistered sources regardless of flags; CI environments should enable it alongside `--yes` and pre-register trusted sources. Without `--yes`, the interactive confirmation prompt names each source explicitly. Documentation should recommend `strict_registries = true` for CI
- For name collision: the resolver logs a warning at construction time when duplicate bare names are detected; users can use the `owner/repo:recipe` qualified syntax to disambiguate
