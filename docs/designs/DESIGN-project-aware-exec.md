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
  Add tsuku exec as the project-aware execution command. It implements
  ProjectVersionResolver using LoadProjectConfig and the binary index to map
  command names to recipe names, then delegates to Runner.Run for install-if-needed
  and process replacement. Default consent mode is auto (vs confirm for tsuku run).
  Optional shims (tsuku shim install <tool>) create static shell scripts in
  $TSUKU_HOME/bin/ that call tsuku exec, enabling transparent invocation without
  the tsuku exec prefix.
rationale: |
  Composes existing infrastructure without modification: the binary index maps
  commands to recipes, LoadProjectConfig reads .tsuku.toml, Runner.Run handles
  install + exec. The only new code is a thin resolver (~30-50 lines) and the CLI
  wiring. Separate command from tsuku run avoids behavioral regressions and serves
  different audiences (CI vs interactive). Static shims avoid regeneration complexity
  since version resolution happens at runtime via tsuku exec.
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

The auto-install flow (`autoinstall.Runner.Run`) already accepts a `ProjectVersionResolver` interface, designed for exactly this integration. `tsuku run` passes `nil` for the resolver. Block 6 provides the real implementation.

### Scope

**In scope:**
- `tsuku exec <command> [args]` command semantics
- `ProjectVersionResolver` implementation using `LoadProjectConfig` + binary index
- Fallback chain: project config -> binary index -> error
- Optional shim generation: `tsuku shim install/uninstall/list`
- CI usage patterns
- Security model for shim-based auto-install

**Out of scope:**
- Changes to `tsuku run` or the auto-install flow
- Changes to the binary index
- LLM recipe discovery

## Decision Drivers

- **Works without shell hooks**: `tsuku exec` must function in CI, scripts, and non-interactive shells
- **Performance**: Cached tool lookup (already installed) must complete in under 50ms
- **Composability**: Leverage existing `ProjectVersionResolver`, `LoadProjectConfig`, and binary index
- **Compatibility**: Must coexist with `tsuku run` (Track A) and `tsuku shell` (Track B)
- **Security**: Shim-based auto-install in untrusted repos must have a clear consent model

## Considered Options

### Decision 1: `tsuku exec` Command Semantics

The `autoinstall.Runner.Run` method already accepts a `ProjectVersionResolver` interface but `tsuku run` passes nil. The question is how to implement the resolver and whether to add a new command or extend `tsuku run`.

A complication: `.tsuku.toml` declares tools by recipe name (`ripgrep = "14.1.0"`), but users type command names (`rg`). The resolver must bridge this gap.

Key assumptions:
- The binary index is available when `tsuku exec` runs (users have run `tsuku update-registry`)
- `.tsuku.toml` tool keys are always recipe names
- `tsuku exec` and `tsuku run` serve different audiences (CI/deterministic vs interactive/ad-hoc)

#### Chosen: Separate `tsuku exec` with Index-Backed Resolver

A new `tsuku exec` command constructs a `ProjectVersionResolver` backed by both `project.LoadProjectConfig` and the binary index's `LookupFunc`. The flow:

1. `tsuku exec <command> [args]` loads `.tsuku.toml` via `LoadProjectConfig(cwd)`
2. The resolver receives a command name (e.g., `rg`)
3. Resolver queries the binary index to find the recipe name (`ripgrep`)
4. Resolver checks `.tsuku.toml` for a version pin on that recipe
5. If found, returns the pinned version; if not, returns `!ok` (falls through to latest)
6. `Runner.Run` handles install-if-needed and process replacement

Implementation:
- `cmd/tsuku/cmd_exec.go`: Cobra command, wires resolver into `Runner.Run`
- `internal/project/resolver.go`: `ProjectVersionResolver` implementation
- Constructor: `NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver`
- Default consent mode: `auto` (vs `confirm` for `tsuku run`) since `tsuku exec` targets CI/script use

`tsuku run` stays unchanged -- nil resolver, `confirm` default. The two commands serve different audiences.

#### Alternatives Considered

**Enhance `tsuku run` with project awareness**: Auto-detect `.tsuku.toml` in `tsuku run`. Rejected because it changes the semantics of an existing command, risks surprising users who expect latest-version behavior, and complicates the confirm/auto mode logic. Clean separation is safer.

**Separate `tsuku exec` with name-only resolver**: Skip the binary index, only match when command name equals recipe name. Handles `go`, `node` but misses `rg` (from ripgrep), `fd` (from fd-find). Rejected because incomplete coverage undermines trust in the feature.

**Cached reverse map from recipes**: Build a parallel command-to-recipe cache by scanning recipe TOML files. Rejected because it duplicates what the binary index already provides.

### Decision 2: Shim Architecture

Shims provide transparent invocation: `go build` instead of `tsuku exec go build`. They're thin wrappers in `$TSUKU_HOME/bin/` that delegate to `tsuku exec`.

PATH precedence with shims:
1. Project-specific tool bins (from shell activation) -- highest
2. `$TSUKU_HOME/bin/` (tsuku binary + shims)
3. `$TSUKU_HOME/tools/current/` (global symlinks)
4. System PATH

When shell activation is active, real binaries win over shims. When activation isn't available (CI/scripts), shims fire.

Key assumptions:
- Shell script shim startup (~5-10ms) is acceptable on top of exec's <50ms target
- Users will shim a manageable number of tools (tens, not thousands)

#### Chosen: Explicit Per-Tool Shell Script Shims

Users create shims for specific tools via `tsuku shim install <tool>`. Each shim is a static shell script:

```sh
#!/bin/sh
exec tsuku exec "$(basename "$0")" -- "$@"
```

Commands:
- `tsuku shim install <tool>` -- creates shims for all binaries the recipe provides. Refuses to overwrite existing files (including the tsuku binary). Prints which shims were created.
- `tsuku shim uninstall <tool>` -- removes shims for the tool's binaries. Only removes files that are tsuku shims (checks content).
- `tsuku shim list` -- lists installed shims with their target tools.

Shim content is static. Version resolution happens at runtime in `tsuku exec`. No regeneration needed when `.tsuku.toml` changes, tools are installed, or projects switch.

Security: shim creation is an explicit user action. When a shimmed command runs in an untrusted repo, `tsuku exec` applies the full consent model from `Runner.Run` (mode resolution, TTY gate, verification gate). In non-interactive contexts without explicit `auto` mode opt-in, installation is blocked.

#### Alternatives Considered

**Auto-shim all installed tools (asdf pattern)**: Every install creates shims. Rejected because it conflicts with `tools/current/` (both provide the same commands with `bin/` winning) and intercepts commands the user didn't ask to shim.

**Project-scoped auto-shims**: Create/remove shims based on `.tsuku.toml` during activation. Rejected because it adds lifecycle tracking complexity, has race conditions between projects, and doesn't help CI.

**Compiled multi-call binary**: Single Go binary with argv[0]-based dispatch. Saves ~10ms per invocation. Rejected for now because the 50ms target applies to exec itself, not shim overhead. Can be adopted later without changing user-facing commands.

## Decision Outcome

**Chosen: Separate `tsuku exec` with index-backed resolver + explicit per-tool shims**

### Summary

`tsuku exec <command> [args]` reads `.tsuku.toml` from the current directory, uses the binary index to map the command name to a recipe name, checks the project config for a version pin, and delegates to `Runner.Run` for install-if-needed and process replacement via `syscall.Exec`. When the command isn't declared in `.tsuku.toml`, it falls back to the binary index for discovery (same as `tsuku run` but with `auto` consent mode).

The resolver implementation is a thin struct (~30-50 lines) that connects `LoadProjectConfig` and the binary index's `LookupFunc` -- both already exist. It implements the `ProjectVersionResolver` interface that `Runner.Run` already accepts. The only genuinely new logic is the command-to-recipe-to-version lookup chain.

Optional shims (`tsuku shim install <tool>`) create static shell scripts in `$TSUKU_HOME/bin/` that call `tsuku exec`. Shims enable transparent invocation (`go build` instead of `tsuku exec go build`) in CI and scripts. They're static -- no regeneration on config changes -- because version resolution happens at runtime in `tsuku exec`. Shell activation (Block 5) takes precedence over shims when active, since project bin paths come before `$TSUKU_HOME/bin/` on PATH.

### Rationale

The design composes four existing, tested components without modifying any of them: the binary index (command-to-recipe mapping), `LoadProjectConfig` (`.tsuku.toml` discovery), `Runner.Run` (install + exec), and the `ProjectVersionResolver` interface (the integration point designed for this purpose). The new code is a resolver struct, a CLI command, and a shim manager -- all thin wiring.

Keeping `tsuku exec` separate from `tsuku run` avoids behavioral regressions in an established command. The default `auto` mode for exec (vs `confirm` for run) reflects their different audiences: exec is for deterministic, scripted use where prompts block execution; run is for interactive exploration where consent is appropriate.

Static shims eliminate regeneration complexity. The shim is always `exec tsuku exec "$(basename "$0")" -- "$@"` regardless of which project it's in. Version resolution is deferred to runtime, keeping the shim system trivially simple.

### Trade-offs Accepted

- Users must know when to use `tsuku exec` vs `tsuku run`. Documentation must clarify the distinction.
- The resolver depends on the binary index being built. Cold-start scenarios (no `update-registry`) get a clear error message.
- Shell script shims add ~5-10ms overhead per invocation. Acceptable for CI; the compiled multi-call binary can optimize this later.
- Shims in `$TSUKU_HOME/bin/` share the directory with the tsuku binary. Content-based identification prevents accidental removal.

## Solution Architecture

### Overview

Block 6 adds a `ProjectVersionResolver` implementation in `internal/project`, a `tsuku exec` command in `cmd/tsuku`, and a shim manager in `internal/shim`. All delegate to existing infrastructure.

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI Layer                               │
│  ┌─────────────────┐     ┌──────────────────────┐              │
│  │ tsuku exec      │     │ tsuku shim            │              │
│  │  cmd_exec.go    │     │  cmd_shim.go          │              │
│  └────────┬────────┘     └──────────┬─────────────┘             │
└───────────┼──────────────────────────┼──────────────────────────┘
            │                          │
            ▼                          ▼
┌───────────────────────┐    ┌────────────────────┐
│ internal/project      │    │ internal/shim       │
│ resolver.go           │    │ manager.go          │
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
// .tsuku.toml found), all lookups return !ok.
func NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver

// ProjectVersionFor maps command -> recipe (via index) -> version (via config).
func (r *Resolver) ProjectVersionFor(ctx context.Context, command string) (string, bool, error)
```

```go
// internal/shim/manager.go

// Manager handles shim creation and removal in $TSUKU_HOME/bin/.
type Manager struct {
    binDir string  // $TSUKU_HOME/bin/
    cfg    *config.Config
}

func NewManager(cfg *config.Config) *Manager

// Install creates shims for all binaries provided by the named recipe.
// Returns the list of created shim paths. Refuses to overwrite non-shim files.
func (m *Manager) Install(recipeName string) ([]string, error)

// Uninstall removes shims for the named recipe's binaries.
// Only removes files identified as tsuku shims by content.
func (m *Manager) Uninstall(recipeName string) error

// List returns all installed shims with their target recipe names.
func (m *Manager) List() ([]ShimEntry, error)

// IsShim checks if a file at the given path is a tsuku shim by reading its content.
func IsShim(path string) bool

type ShimEntry struct {
    Command string // binary name (e.g., "rg")
    Recipe  string // recipe name (e.g., "ripgrep")
    Path    string // full path to shim
}
```

### Data Flow

**Flow 1: `tsuku exec rg .foo data.json`**

```
1. cmd_exec.go loads config and binary index
2. LoadProjectConfig(cwd) finds .tsuku.toml with ripgrep = "14.1.0"
3. NewResolver(configResult, index.Lookup) creates the resolver
4. Runner.Run(ctx, "rg", args, ModeAuto, resolver) starts
5. Runner calls resolver.ProjectVersionFor(ctx, "rg")
6. Resolver calls index.Lookup("rg") -> [{Recipe: "ripgrep", ...}]
7. Resolver checks config.Tools["ripgrep"] -> Version: "14.1.0"
8. Returns ("14.1.0", true, nil)
9. Runner installs ripgrep@14.1.0 if needed
10. Runner execs $TSUKU_HOME/tools/ripgrep-14.1.0/bin/rg with args
```

**Flow 2: `tsuku exec python3` (not in .tsuku.toml)**

```
1-4. Same as above
5. Resolver calls index.Lookup("python3") -> [{Recipe: "python", ...}]
6. Resolver checks config.Tools["python"] -> not found
7. Returns ("", false, nil)
8. Runner falls back to binary index discovery (standard Track A flow)
9. Runner installs python@latest if needed (with consent model)
10. Runner execs the binary
```

**Flow 3: Shim invocation (`go build` with shim installed)**

```
1. Shell resolves `go` to $TSUKU_HOME/bin/go (shim)
2. Shim runs: exec tsuku exec "go" -- "build"
3. tsuku exec flow (same as Flow 1) starts
4. LoadProjectConfig finds go = "1.22" in .tsuku.toml
5. Resolver returns "1.22"
6. Runner ensures go-1.22.x is installed, execs go build
```

## Implementation Approach

### Phase 1: Resolver (`internal/project/resolver.go`)

Build the `ProjectVersionResolver` implementation. Pure logic, no CLI.

Deliverables:
- `internal/project/resolver.go`: `NewResolver`, `ProjectVersionFor`
- `internal/project/resolver_test.go`: Tests for command-to-recipe mapping, version pin found, version pin not found, no config (nil), index error handling

### Phase 2: `tsuku exec` Command

Wire the resolver into `Runner.Run` via a new CLI command.

Deliverables:
- `cmd/tsuku/cmd_exec.go`: Cobra command, mode resolution, resolver wiring
- Register in main.go
- Tests for exec command

### Phase 3: Shim Manager (`internal/shim`)

Build the shim creation, removal, and listing logic.

Deliverables:
- `internal/shim/manager.go`: `Manager`, `Install`, `Uninstall`, `List`, `IsShim`
- `internal/shim/manager_test.go`: Tests for create, overwrite protection, content-based identification, uninstall, list

### Phase 4: `tsuku shim` Commands

Wire the shim manager into CLI commands.

Deliverables:
- `cmd/tsuku/cmd_shim.go`: Cobra commands for install/uninstall/list
- Register in main.go
- Tests

### Phase 5: Documentation

Update CLI help text and add CI usage examples.

## Security Considerations

### Auto-Install via Shims in Untrusted Repos

When a shimmed command runs in a cloned repo with `.tsuku.toml`, `tsuku exec` reads the repo's config and may trigger installation. The consent model from `Runner.Run` provides layered protection:

1. **Mode resolution chain**: `--mode` flag -> `TSUKU_AUTO_INSTALL_MODE` env -> `auto_install_mode` config -> `confirm` (default)
2. **TTY gate**: In `confirm` mode without a TTY (common in CI), installation is blocked
3. **Verification gate**: In `auto` mode, recipes must have checksums for automatic install
4. **Binary index gate**: Only recipes in the curated index can be resolved

A malicious `.tsuku.toml` can declare any recipe name, but installation only proceeds if the recipe exists in the curated registry and passes verification.

**Mitigations:**
- Default consent mode for `tsuku exec` is `auto`, but this is blocked in non-interactive contexts without explicit opt-in via config or env var
- Shim creation is an explicit user action -- not triggered by cloning a repo
- The binary index limits resolution to curated recipes only
- Checksum verification applies to all installs regardless of trigger

### Shim File Safety

Shims live in `$TSUKU_HOME/bin/` alongside the tsuku binary. The shim manager:
- Refuses to overwrite existing files that aren't tsuku shims (content-based check)
- Never overwrites the `tsuku` binary itself
- Uses content-based identification for uninstall (only removes files it created)

### PATH Precedence

Shell activation (Block 5) prepends project-specific paths before `$TSUKU_HOME/bin/`. This means:
- With activation: real binaries used directly, shims never fire
- Without activation: shims fire and call `tsuku exec` with full consent model

This layering is safe: the more trusted path (real installed binaries) always takes precedence over the less trusted path (shim -> exec -> install).

### Mitigations Summary

| Risk | Severity | Mitigation | Residual Risk |
|------|----------|------------|---------------|
| Untrusted repo triggers install via shim | Medium | Consent model (mode chain, TTY gate, verification) | User with auto mode in config gets auto-install in any repo |
| Shim overwrites system binary | Low | Content-based check, explicit refusal for tsuku binary | None |
| Malicious .tsuku.toml declares fake recipe | Low | Binary index limits to curated recipes, checksum verification | Registry compromise would allow it |
| Shim startup overhead | Low | ~5-10ms acceptable; multi-call binary can optimize later | Noticeable on very fast commands |

## Consequences

### Positive

- **CI works without hooks**: `tsuku exec go build` in CI gets the project-pinned Go version with zero shell setup
- **Complete Track A + B convergence**: The vision from the parent design is fully realized
- **Minimal new code**: Resolver is ~30-50 lines connecting existing components
- **Static shims**: No regeneration, no staleness, no lifecycle management
- **Future-proof**: Multi-call binary can replace shell shims later without changing commands

### Negative

- **Two exec commands**: Users must learn when to use `tsuku exec` vs `tsuku run`
- **Binary index dependency**: `tsuku exec` needs the index built (`tsuku update-registry`)
- **Shim overhead**: ~5-10ms per invocation on top of exec's <50ms
- **Dual-purpose `$TSUKU_HOME/bin/`**: Directory now holds the tsuku binary and optional shims

### Mitigations

- **Two commands**: Document clearly. `tsuku exec` = project-aware, deterministic. `tsuku run` = ad-hoc, interactive.
- **Index dependency**: Clear error message directing to `tsuku update-registry`. Consider auto-building on first `tsuku exec` if no index exists.
- **Shim overhead**: Acceptable for CI. Multi-call binary optimization path documented.
- **Dual-purpose bin/**: Content-based shim identification prevents confusion.
