---
status: Proposed
spawned_from:
  issue: 2168
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
problem: |
  Track A (auto-install via tsuku run) and Track B (project config via .tsuku.toml)
  don't converge at command invocation time in non-interactive contexts. CI pipelines,
  shell scripts, and users without prompt hooks can't get project-declared tool
  versions installed on first use. The autoinstall.Runner already accepts a
  ProjectVersionResolver interface but no implementation exists.
decision: |
  Augment tsuku run with project awareness by implementing ProjectVersionResolver
  using LoadProjectConfig and the binary index to map command names to recipe names.
  When .tsuku.toml declares the tool, the project-pinned version is used; otherwise,
  the existing fallback-to-latest behavior kicks in. Optional shims (tsuku shim
  install <tool>) create static shell scripts in $TSUKU_HOME/bin/ that call tsuku
  run, enabling transparent invocation without the tsuku run prefix.
rationale: |
  tsuku run was built as Block 3 specifically as the auto-install execution path.
  It has no established user base to protect from behavioral changes. Adding a
  separate tsuku exec command would create unnecessary cognitive overhead when tsuku
  run can gain project awareness naturally by passing a real ProjectVersionResolver
  instead of nil. Composes existing infrastructure: binary index maps commands to
  recipes, LoadProjectConfig reads .tsuku.toml, Runner.Run already accepts the
  resolver interface. Static shims avoid regeneration complexity since version
  resolution happens at runtime.
---

# DESIGN: Project-Aware Exec Wrapper

## Status

Proposed

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Block 6 -- convergence point between Track A (auto-install) and Track B (project config). Depends on Block 3 (#1679, auto-install, implemented), Block 4 (#1680, project config, implemented), and uses the binary index from Block 1 (#1677, implemented).

## Context and Problem Statement

Track A (Blocks 1-3) handles command discovery and auto-install: when a user types an unknown command, the binary index identifies the recipe, and `tsuku run` installs it. Track B (Blocks 4-5) handles project environments: `.tsuku.toml` declares tool requirements, and shell activation puts the right versions on PATH.

These tracks converge in interactive shells with hooks. But they don't converge in three important contexts:

1. **CI pipelines**: No shell hooks. A CI script needs project-declared tool versions installed on first use.
2. **Shell scripts**: A Makefile calls `go build` without tsuku shell activation.
3. **Users without hooks**: Developers who don't want prompt hooks still want project-pinned versions.

The auto-install flow (`autoinstall.Runner.Run`) already accepts a `ProjectVersionResolver` interface, designed for exactly this integration. `tsuku run` currently passes `nil` for the resolver. Block 6 provides the real implementation and wires it in.

### Scope

**In scope:**
- `ProjectVersionResolver` implementation using `LoadProjectConfig` + binary index
- Wiring the resolver into `tsuku run` (replacing the nil)
- Fallback chain: project config -> binary index -> error
- Optional shim generation: `tsuku shim install/uninstall/list`
- CI usage patterns
- Security model for shim-based auto-install

**Out of scope:**
- Changes to the auto-install flow itself (Block 3, already implemented)
- Changes to the binary index (Block 1, already implemented)
- LLM recipe discovery

## Decision Drivers

- **Works without shell hooks**: `tsuku run` with resolver must function in CI, scripts, and non-interactive shells
- **Performance**: Cached tool lookup (already installed) must complete in under 50ms
- **Composability**: Leverage existing `ProjectVersionResolver`, `LoadProjectConfig`, and binary index
- **No unnecessary commands**: Don't add a new CLI entry point when an existing one can be augmented
- **Security**: Shim-based auto-install in untrusted repos must have a clear consent model

## Considered Options

### Decision 1: How to Add Project Awareness

The `autoinstall.Runner.Run` method already accepts a `ProjectVersionResolver` interface but `tsuku run` passes nil. The question is how to implement the resolver and where to wire it in.

A complication: `.tsuku.toml` declares tools by recipe name (`ripgrep = "14.1.0"`), but users type command names (`rg`). The resolver must bridge this gap using the binary index.

Key assumptions:
- The binary index is available when `tsuku run` executes (users have run `tsuku update-registry`)
- `.tsuku.toml` tool keys are always recipe names
- `tsuku run` was built recently as Block 3 and has no established user base to protect from behavioral changes

#### Chosen: Augment `tsuku run` with Index-Backed Resolver

Wire a `ProjectVersionResolver` into `tsuku run` by implementing it using `LoadProjectConfig` + binary index `LookupFunc`. The flow:

1. `tsuku run <command> [args]` loads `.tsuku.toml` via `LoadProjectConfig(cwd)`
2. The resolver receives a command name (e.g., `rg`)
3. Resolver queries the binary index to find the recipe name (`ripgrep`)
4. Resolver checks `.tsuku.toml` for a version pin on that recipe
5. If found, returns the pinned version; if not, returns `!ok` (falls through to latest via binary index)
6. `Runner.Run` handles install-if-needed and process replacement as usual

Implementation:
- `internal/project/resolver.go`: `ProjectVersionResolver` implementation
- Constructor: `NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver`
- Modified `cmd/tsuku/cmd_run.go`: constructs resolver and passes it to `Runner.Run` instead of nil

The consent mode logic stays exactly as it is. The resolver adds project-version awareness transparently -- if `.tsuku.toml` pins a version, that version is used. If not (or if no config exists), the existing behavior is unchanged.

#### Alternatives Considered

**New `tsuku exec` command**: Add a separate command with different defaults (auto mode vs confirm). Rejected because `tsuku run` was built as Block 3 specifically for auto-install execution and has no established user base. A separate command creates unnecessary cognitive overhead ("when do I use exec vs run?") without protecting anyone from behavioral changes. The consent mode is configurable via `--mode`, env var, and config -- there's no need for a command-level split.

**Name-only resolver (skip binary index)**: Only match when command name equals recipe name. Handles `go`, `node` but misses `rg` (from ripgrep), `fd` (from fd-find). Rejected because incomplete coverage undermines trust in the feature.

**Cached reverse map from recipes**: Build a parallel command-to-recipe cache. Rejected because it duplicates what the binary index already provides.

### Decision 2: Shim Architecture

Shims provide transparent invocation: `go build` instead of `tsuku run go build`. They're thin wrappers in `$TSUKU_HOME/bin/` that delegate to `tsuku run`, which handles project-config lookup + auto-install.

PATH precedence with shims:
1. Project-specific tool bins (from shell activation) -- highest
2. `$TSUKU_HOME/bin/` (tsuku binary + shims)
3. `$TSUKU_HOME/tools/current/` (global symlinks)
4. System PATH

When shell activation is active, real binaries win over shims. When activation isn't available (CI/scripts), shims fire.

Key assumptions:
- Shell script shim startup (~5-10ms) is acceptable on top of run's <50ms target
- Users will shim a manageable number of tools (tens, not thousands)

#### Chosen: Explicit Per-Tool Shell Script Shims

Users create shims for specific tools via `tsuku shim install <tool>`. Each shim is a static shell script:

```sh
#!/bin/sh
exec tsuku run "$(basename "$0")" -- "$@"
```

Commands:
- `tsuku shim install <tool>` -- creates shims for all binaries the recipe provides. Refuses to overwrite existing files (including the tsuku binary). Prints which shims were created.
- `tsuku shim uninstall <tool>` -- removes shims for the tool's binaries. Only removes files that are tsuku shims (checks content).
- `tsuku shim list` -- lists installed shims with their target tools.

Shim content is static. Version resolution happens at runtime in `tsuku run` via the resolver. No regeneration needed when `.tsuku.toml` changes, tools are installed, or projects switch.

Security: shim creation is an explicit user action. When a shimmed command runs in an untrusted repo, `tsuku run` applies the full consent model from `Runner.Run` (mode resolution chain, TTY gate, verification gate).

#### Alternatives Considered

**Auto-shim all installed tools (asdf pattern)**: Every install creates shims. Rejected because it conflicts with `tools/current/` (both provide the same commands with `bin/` winning) and intercepts commands the user didn't ask to shim.

**Project-scoped auto-shims**: Create/remove shims based on `.tsuku.toml` during activation. Rejected because it adds lifecycle tracking complexity, has race conditions between projects, and doesn't help CI.

**Compiled multi-call binary**: Single Go binary with argv[0]-based dispatch. Saves ~10ms. Rejected for now -- can be adopted later without changing user-facing commands.

## Decision Outcome

**Chosen: Augmented `tsuku run` with index-backed resolver + explicit per-tool shims**

### Summary

`tsuku run` gains project awareness by wiring a real `ProjectVersionResolver` implementation where it currently passes `nil`. The resolver uses `LoadProjectConfig` to read `.tsuku.toml` and the binary index to map command names to recipe names. When `tsuku run rg .foo data.json` executes, the resolver maps `rg` -> `ripgrep` via the index, finds `ripgrep = "14.1.0"` in `.tsuku.toml`, and returns that version. `Runner.Run` installs if needed and execs the binary. When no `.tsuku.toml` exists or the tool isn't declared, the existing behavior is unchanged -- the binary index provides discovery and the latest version is used.

Optional shims (`tsuku shim install <tool>`) create static shell scripts in `$TSUKU_HOME/bin/` that call `tsuku run`. This enables transparent invocation: `go build` instead of `tsuku run go build`. Shims are static -- no regeneration on config changes -- because version resolution happens at runtime. Shell activation (Block 5) takes precedence over shims when active.

The resolver is ~30-50 lines of Go connecting `LoadProjectConfig`, the binary index `LookupFunc`, and the `ProjectVersionResolver` interface that `Runner.Run` already accepts. The change to `cmd_run.go` is minimal: construct the resolver and pass it where `nil` was.

### Rationale

`tsuku run` was built as Block 3 to be the auto-install execution entry point. It has no established user base, no scripts depending on its current nil-resolver behavior. Adding project awareness is the natural evolution of the command, not a behavioral regression. A separate `tsuku exec` would fragment the CLI surface for no benefit.

The design composes four existing components: binary index (command-to-recipe), `LoadProjectConfig` (`.tsuku.toml` discovery), `Runner.Run` (install + exec), and `ProjectVersionResolver` (the integration point designed for this). Static shims keep the system trivially simple -- version resolution is always deferred to runtime.

### Trade-offs Accepted

- The resolver depends on the binary index being built. Cold-start scenarios need clear error messages.
- Shell script shims add ~5-10ms overhead per invocation. The multi-call binary can optimize this later.
- Shims in `$TSUKU_HOME/bin/` share the directory with the tsuku binary. Content-based identification prevents confusion.

## Solution Architecture

### Overview

Block 6 adds a `ProjectVersionResolver` implementation in `internal/project`, wires it into the existing `tsuku run` command, and adds a shim manager in `internal/shim`.

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI Layer                               │
│  ┌─────────────────┐     ┌──────────────────────┐              │
│  │ tsuku run       │     │ tsuku shim            │              │
│  │  cmd_run.go     │     │  cmd_shim.go          │              │
│  │  (modified)     │     │  (new)                │              │
│  └────────┬────────┘     └──────────┬─────────────┘             │
└───────────┼──────────────────────────┼──────────────────────────┘
            │                          │
            ▼                          ▼
┌───────────────────────┐    ┌────────────────────┐
│ internal/project      │    │ internal/shim       │
│ resolver.go (new)     │    │ manager.go (new)    │
│  NewResolver()        │    │  Install/Uninstall  │
│  ProjectVersionFor()  │    │  List/IsShim        │
└───────────┬───────────┘    └────────────────────┘
            │
            ▼
┌───────────────────────────────────────────────────────┐
│               Existing Infrastructure                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐│
│  │ LoadProject  │  │ BinaryIndex  │  │ autoinstall   ││
│  │ Config       │  │ .Lookup      │  │ .Runner.Run   ││
│  │ (Block 4)    │  │ (Block 1)    │  │ (Block 3)     ││
│  └──────────────┘  └──────────────┘  └──────────────┘│
└───────────────────────────────────────────────────────┘
```

### Key Interfaces

```go
// internal/project/resolver.go

// Resolver implements autoinstall.ProjectVersionResolver by combining
// project config (recipe name -> version) with the binary index
// (command name -> recipe name).
type Resolver struct {
    config *ConfigResult
    lookup autoinstall.LookupFunc
}

// NewResolver creates a ProjectVersionResolver. If config is nil (no
// .tsuku.toml found), all lookups return !ok (falls through to default).
func NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver

// ProjectVersionFor maps command -> recipe (via index) -> version (via config).
func (r *Resolver) ProjectVersionFor(ctx context.Context, command string) (string, bool, error)
```

```go
// internal/shim/manager.go

// Manager handles shim creation and removal in $TSUKU_HOME/bin/.
type Manager struct {
    binDir string
    cfg    *config.Config
}

func NewManager(cfg *config.Config) *Manager

// Install creates shims for all binaries provided by the named recipe.
func (m *Manager) Install(recipeName string) ([]string, error)

// Uninstall removes shims for the named recipe's binaries.
func (m *Manager) Uninstall(recipeName string) error

// List returns all installed shims.
func (m *Manager) List() ([]ShimEntry, error)

// IsShim checks if a file is a tsuku shim by reading its content.
func IsShim(path string) bool

type ShimEntry struct {
    Command string
    Recipe  string
    Path    string
}
```

### Data Flow

**Flow 1: `tsuku run rg .foo data.json` (with .tsuku.toml)**

```
1. cmd_run.go loads config, binary index, and project config
2. NewResolver(projectConfig, index.Lookup) creates the resolver
3. Runner.Run(ctx, "rg", args, mode, resolver)
4. Runner calls resolver.ProjectVersionFor(ctx, "rg")
5. Resolver calls index.Lookup("rg") -> [{Recipe: "ripgrep", ...}]
6. Resolver checks config.Tools["ripgrep"] -> Version: "14.1.0"
7. Returns ("14.1.0", true, nil)
8. Runner installs ripgrep@14.1.0 if needed
9. Runner execs $TSUKU_HOME/tools/ripgrep-14.1.0/bin/rg with args
```

**Flow 2: `tsuku run python3` (not in .tsuku.toml)**

```
1-4. Same as above
5. Resolver calls index.Lookup("python3") -> [{Recipe: "python", ...}]
6. Resolver checks config.Tools["python"] -> not found
7. Returns ("", false, nil)
8. Runner falls back to binary index discovery (existing Track A flow)
9. Runner installs python@latest (with consent model)
```

**Flow 3: `tsuku run go build` (no .tsuku.toml at all)**

```
1. cmd_run.go loads project config -> nil
2. NewResolver(nil, index.Lookup) -> resolver where all lookups return !ok
3. Runner.Run proceeds with nil-equivalent resolver
4. Behavior identical to current tsuku run (no project awareness)
```

**Flow 4: Shim invocation (`go build` with shim installed)**

```
1. Shell resolves `go` to $TSUKU_HOME/bin/go (shim)
2. Shim runs: exec tsuku run "go" -- "build"
3. Same as Flow 1 from there
```

## Implementation Approach

### Phase 1: Resolver (`internal/project/resolver.go`)

Build the `ProjectVersionResolver` implementation.

Deliverables:
- `internal/project/resolver.go`: `NewResolver`, `ProjectVersionFor`
- `internal/project/resolver_test.go`: Tests for command-to-recipe mapping, version pin found/not found, nil config, index error

### Phase 2: Wire Resolver into `tsuku run`

Modify `cmd_run.go` to construct the resolver and pass it to `Runner.Run`.

Deliverables:
- Modified `cmd/tsuku/cmd_run.go`: load project config, construct resolver, pass to Runner.Run
- Tests verifying resolver is wired in

### Phase 3: Shim Manager (`internal/shim`)

Build shim creation, removal, and listing.

Deliverables:
- `internal/shim/manager.go`: `Manager`, `Install`, `Uninstall`, `List`, `IsShim`
- `internal/shim/manager_test.go`: Tests

### Phase 4: `tsuku shim` Commands

Wire the shim manager into CLI.

Deliverables:
- `cmd/tsuku/cmd_shim.go`: Cobra commands for install/uninstall/list
- Register in main.go

### Phase 5: Documentation

Update CLI help and CI usage examples.

## Security Considerations

### Auto-Install via Shims in Untrusted Repos

When a shimmed command runs in a cloned repo with `.tsuku.toml`, `tsuku run` reads the repo's config and may trigger installation. The consent model from `Runner.Run` provides layered protection:

1. **Mode resolution chain**: `--mode` flag -> `TSUKU_AUTO_INSTALL_MODE` env -> `auto_install_mode` config -> `confirm` (default)
2. **TTY gate**: In `confirm` mode without a TTY, installation is blocked
3. **Verification gate**: In `auto` mode, recipes must have checksums
4. **Binary index gate**: Only recipes in the curated index can be resolved

**Mitigations:**
- Default consent mode remains `confirm` -- project awareness doesn't change the consent model
- Shim creation is an explicit user action
- The binary index limits resolution to curated recipes
- Checksum verification applies to all installs

### Shim File Safety

- Refuses to overwrite existing non-shim files (content-based check)
- Never overwrites the `tsuku` binary
- Content-based identification for uninstall

### PATH Precedence

Shell activation prepends project paths before `$TSUKU_HOME/bin/`. With activation: real binaries win. Without: shims fire with full consent model.

### Mitigations Summary

| Risk | Severity | Mitigation | Residual Risk |
|------|----------|------------|---------------|
| Untrusted repo triggers install via shim | Medium | Consent model (mode chain, TTY gate, verification) | User with auto mode gets auto-install in any repo |
| Shim overwrites system binary | Low | Content-based check, tsuku binary protection | None |
| Malicious .tsuku.toml declares fake recipe | Low | Binary index limits to curated recipes, checksums | Registry compromise |
| Shim startup overhead | Low | ~5-10ms acceptable; multi-call binary can optimize | Noticeable on fast commands |

## Consequences

### Positive

- **CI works without hooks**: `tsuku run go build` in CI gets the project-pinned Go version
- **Complete Track A + B convergence**: The shell integration vision is realized
- **No new CLI commands for exec**: `tsuku run` gains project awareness naturally
- **Minimal new code**: Resolver is ~30-50 lines connecting existing components
- **Static shims**: No regeneration, no staleness

### Negative

- **Binary index dependency**: `tsuku run` with resolver needs the index built
- **Shim overhead**: ~5-10ms per invocation on top of run's <50ms
- **Dual-purpose `$TSUKU_HOME/bin/`**: Holds tsuku binary and optional shims

### Mitigations

- **Index dependency**: Clear error message. Consider auto-building on first run if no index exists.
- **Shim overhead**: Multi-call binary optimization path documented.
- **Dual-purpose bin/**: Content-based shim identification.
