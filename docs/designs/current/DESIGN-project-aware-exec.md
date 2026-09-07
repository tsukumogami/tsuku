---
schema: design/v1
status: Current
spawned_from:
  issue: 2168
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
problem: |
  When a user types a command that isn't installed, tsuku can suggest or
  auto-install it via the command-not-found hook and tsuku run. But neither
  path checks .tsuku.toml for project-pinned versions. A developer types rg
  in a project that pins ripgrep = "14.1.0" and gets latest installed (or just
  a suggestion), not 14.1.0. The project config exists but is invisible to the
  auto-install flow. In CI and scripts without hooks, there's no path at all.
decision: |
  Wire a ProjectVersionResolver into the auto-install flow so it checks
  .tsuku.toml before falling back to latest. When a command-not-found fires
  and .tsuku.toml declares the tool, treat the project config as consent and
  install the pinned version without a prompt -- but only where the user has
  configured no consent mode; a mode they set is honored as given. tsuku run
  gains the same project awareness. Optional shims provide the same behavior
  in CI and scripts without shell hooks.
rationale: |
  The project config is explicit user intent -- the team deliberately declared
  which tools and versions they need. That declaration is consent enough to
  skip the confirmation prompt that would break the seamless experience, in the
  one state where nobody has said otherwise: the unset default. It does not
  outrank a mode the person running the command chose. The resolver is ~30-50
  lines connecting existing infrastructure (binary index, LoadProjectConfig,
  Runner.Run). Shims are static shell scripts that call tsuku run, deferring
  all logic to runtime.
---

# DESIGN: Project-Aware Exec Wrapper

## Status

Current

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Block 6 -- convergence point between Track A (auto-install) and Track B (project config). Depends on Block 3 (#1679, auto-install, implemented), Block 4 (#1680, project config, implemented), and uses the binary index from Block 1 (#1677, implemented).

## Context and Problem Statement

The vision from the parent design is: a developer types `rg .foo data.json` in a project directory. They don't have `rg` installed. The project's `.tsuku.toml` declares `ripgrep = "14.1.0"`. Tsuku intercepts the failed command, sees the project pin, installs ripgrep 14.1.0 silently, and runs the command. No `tsuku run` prefix, no confirmation prompt.

Today the pieces exist but aren't connected:

- The **command-not-found hook** (Block 2) catches `rg` and calls `tsuku suggest rg` -- but it only suggests, it doesn't auto-install with the project version
- The **binary index** (Block 1) maps `rg` -> recipe `ripgrep` -- but the auto-install flow doesn't check project config
- **`tsuku run`** (Block 3) installs and execs -- but passes `nil` for the `ProjectVersionResolver`, ignoring `.tsuku.toml`
- **`.tsuku.toml`** (Block 4) declares `ripgrep = "14.1.0"` -- but nothing reads it at command invocation time

The auto-install flow (`autoinstall.Runner.Run`) already accepts a `ProjectVersionResolver` interface designed for this integration. The command-not-found hook already calls `tsuku suggest`, which could call `tsuku run` instead when a project config declares the tool. Block 6 connects these pieces.

### The Consent Model

`.tsuku.toml` is a consent signal, and it is bounded. When a team checks in a project config declaring their tools, they're saying "these tools should be available in this project," and the confirmation prompt `tsuku run` uses for ad-hoc installs adds nothing on top of that -- for a developer who has not said anything themselves. So the declaration raises the effective consent mode to `auto` when, and only when, nobody set a mode: a `--mode` flag, a `TSUKU_AUTO_INSTALL_MODE` value and an `auto_install_mode` config key each outrank the declaration and are honored as given, `suggest` included. A file the repository ships does not overrule the person running the command.

This means:
- **Command in `.tsuku.toml`, no mode configured**: install the pinned version without a prompt, disclosing which file authorized it, then exec
- **Command in `.tsuku.toml`, a mode configured**: that mode, unchanged -- `suggest` prints an instruction and installs nothing
- **Command NOT in `.tsuku.toml`**: the configured mode, or the `confirm` default

### Scope

**In scope:**
- `ProjectVersionResolver` implementation using `LoadProjectConfig` + binary index
- Wiring the resolver into `tsuku run` and the command-not-found path
- Install without confirmation when `.tsuku.toml` declares the tool and no consent mode is configured
- Optional shim generation for CI/scripts without hooks
- CI usage patterns

**Out of scope:**
- Changes to the binary index (Block 1)
- LLM recipe discovery

## Decision Drivers

- **Seamless experience**: Users type commands, they work. No tsuku prefix, no prompts for project-declared tools.
- **Project config is consent**: `.tsuku.toml` is deliberate -- no additional confirmation needed for declared tools, where the user has expressed no preference of their own
- **Works everywhere**: Interactive shells (via hooks), scripts (via shims), CI (via shims or `tsuku run`)
- **Performance**: Cached tool lookup must complete in under 50ms
- **Composability**: Leverage existing `ProjectVersionResolver`, `LoadProjectConfig`, binary index, and consent model
- **Fallback safety**: Tools NOT in `.tsuku.toml` still go through the normal consent flow

## Considered Options

### Decision 1: How to Wire Project Awareness into the Auto-Install Flow

The `autoinstall.Runner.Run` method accepts a `ProjectVersionResolver` interface. Today `tsuku run` passes `nil`. The command-not-found hook calls `tsuku suggest` (print-only). Block 6 needs to connect `.tsuku.toml` to both paths.

A complication: `.tsuku.toml` declares tools by recipe name (`ripgrep = "14.1.0"`), but users type command names (`rg`). The binary index bridges this gap.

Key assumptions:
- `.tsuku.toml` declaring a tool is sufficient consent for auto-install
- The binary index maps command -> recipe reliably
- `tsuku run` has no established user base (Block 3 was recently built)

#### Chosen: Resolver + Bounded Mode Elevation for Project-Declared Tools

Wire a `ProjectVersionResolver` into `tsuku run` using `LoadProjectConfig` + binary index `LookupFunc`. When the resolver finds a project-pinned version, `Runner.Run` uses it. The critical addition: when the resolver returns a version (tool is in `.tsuku.toml`), the consent mode is raised to `auto` -- but only from the unset default, so a mode the user configured is kept. The project config is a consent signal, not an override.

For the command-not-found path: change the hook to call `tsuku run <command> -- [args]` instead of `tsuku suggest <command>`. The hook makes no declaration check of its own -- it cannot cheaply, and `tsuku run` has to load the project config anyway -- so every unresolvable command goes there and the runner decides. For a declared tool under an unset default that means installing the pinned version without a prompt and execing the command.

The flow for a project-declared tool (not installed, no consent mode configured):
1. User types `rg .foo data.json`
2. Shell's command-not-found hook fires
3. Hook calls `tsuku run rg -- .foo data.json`
4. `tsuku run` loads `.tsuku.toml`, constructs resolver
5. Resolver maps `rg` -> `ripgrep` (via index) -> `14.1.0` (via config)
6. Runner.Run gets the version from the resolver and raises the unset default to `auto`; a mode the user set would survive here instead, and a mode-lowering gate can put the raised one back at `confirm`
7. Discloses the authorizing file, installs ripgrep 14.1.0, execs `rg .foo data.json`

The flow for a project-declared tool (already installed, different version):
1. User types `rg .foo data.json`
2. `rg` resolves to the globally active version (e.g., 14.0.0) via `tools/current/`
3. OR: user explicitly runs `tsuku run rg .foo data.json`
4. Resolver maps `rg` -> `ripgrep` -> `14.1.0` (project pin)
5. Runner.Run checks: is the project-pinned version installed?
6. If 14.1.0 is installed: exec from `$TSUKU_HOME/tools/ripgrep-14.1.0/bin/rg`
7. If 14.1.0 is not installed: install it (auto mode), then exec from its bin dir
8. In both cases, the global `tools/current/` symlink is NOT used -- the project-specific version path is used directly

**Critical: project version pins take precedence over the installed-tool fast path.** When a resolver is present and returns a pinned version, `Runner.Run` must consult the resolver BEFORE checking whether any version is already installed. The resolver's version determines which binary to exec, not the global symlink.

The flow for a tool NOT in `.tsuku.toml`:
1. User types `jq .foo data.json`
2. Hook fires, calls `tsuku run jq .foo data.json`
3. Resolver returns `!ok` (jq not in project config)
4. Runner.Run uses the existing fast path: if any version installed, exec from `tools/current/`; if not, fall back to normal consent mode
5. Behavior identical to pre-Block-6 `tsuku run`

#### Alternatives Considered

**New `tsuku exec` command**: Separate command for project-aware execution. Rejected because `tsuku run` was built as Block 3 specifically for auto-install and has no established user base. A separate command adds cognitive overhead without protecting anyone.

**Always auto-install on command-not-found**: Skip the resolver check and auto-install any missing command. Rejected because installing arbitrary tools without consent is dangerous. The `.tsuku.toml` check provides a trust boundary.

**Project awareness without hook changes**: Only wire the resolver into `tsuku run`, don't change command-not-found behavior. Rejected because it doesn't deliver the seamless experience -- users would still need the `tsuku run` prefix.

### Decision 2: Shim Architecture

Shims provide the seamless experience in contexts without shell hooks: CI pipelines, Makefiles, shell scripts. A shim for `go` in `$TSUKU_HOME/bin/` means `go build` triggers the same project-aware flow.

#### Chosen: Explicit Per-Tool Shell Script Shims

Users create shims via `tsuku shim install <tool>`. Each shim is a static script:

```sh
#!/bin/sh
exec tsuku run "$(basename "$0")" -- "$@"
```

Commands:
- `tsuku shim install <tool>` -- creates shims for all binaries the recipe provides
- `tsuku shim uninstall <tool>` -- removes shims (content-based identification)
- `tsuku shim list` -- lists installed shims

Shims are static -- version resolution happens at runtime in `tsuku run`. No regeneration needed.

PATH precedence:
1. Project-specific tool bins (shell activation) -- real binaries win
2. `$TSUKU_HOME/bin/` (shims) -- fire when activation isn't available
3. `$TSUKU_HOME/tools/current/` (global symlinks)
4. System PATH

When shell activation is active, real binaries take precedence and shims never fire. When activation isn't available (CI), shims fire and `tsuku run` handles everything.

#### Alternatives Considered

**Auto-shim all installed tools**: Rejected -- conflicts with `tools/current/` and intercepts commands the user didn't ask to shim.

**Project-scoped auto-shims**: Rejected -- lifecycle tracking complexity, race conditions between projects, doesn't help CI.

**Compiled multi-call binary**: Rejected for now -- saves ~10ms but adds build complexity. Can optimize later.

## Decision Outcome

**Chosen: Project config as a bounded consent signal, via resolver + command-not-found hook upgrade + explicit shims**

### Summary

When a developer types `rg .foo data.json` in a project with `.tsuku.toml` declaring `ripgrep = "14.1.0"`, and that developer has configured no consent mode, the command just works. The command-not-found hook calls `tsuku run`, which uses the new `ProjectVersionResolver` to find the project pin, raises the unset default to `auto`, discloses which file authorized the install, installs ripgrep 14.1.0, and execs the command. A developer who has configured a mode gets that mode instead.

The resolver is a thin struct (~30-50 lines) connecting `LoadProjectConfig` and the binary index's `LookupFunc`. It maps command -> recipe (via index) -> version (via project config). When the tool isn't in `.tsuku.toml`, the resolver returns `!ok` and the normal consent flow applies.

The command-not-found hook changes from calling `tsuku suggest` to calling `tsuku run` for every command the shell cannot resolve, declared or not. For undeclared tools the behavior is whatever the configured auto-install mode says (suggest/confirm/auto), which for the default `confirm` means a prompt -- or, with no terminal, exit 12.

Optional shims (`tsuku shim install <tool>`) provide the same experience in CI and scripts. A shim is `exec tsuku run "$(basename "$0")" -- "$@"` -- static, no regeneration.

Three contexts, one rule -- each reaches `tsuku run`, and what happens there is
the consent mode, raised by a declaration only where nothing set one:
- **Interactive shell with hooks**: command-not-found -> `tsuku run` -> resolver -> mode
- **Interactive shell without hooks**: `tsuku run go build` -> resolver -> mode
- **CI/scripts with shims**: `go build` -> shim -> `tsuku run` -> resolver -> mode

### Rationale

The project config is a consent signal, and it is bounded. When a team checks `.tsuku.toml` into their repo declaring `ripgrep = "14.1.0"`, they're authorizing that tool at that version, and prompting the developer on top of that is friction without value -- but only where the developer has said nothing themselves. So the declaration raises the effective mode to `auto` when, and only when, the mode's origin is the unset default. A mode set by `--mode`, by `TSUKU_AUTO_INSTALL_MODE` or by `auto_install_mode` in `config.toml` is honored as given, `suggest` included; a repository-supplied file does not outrank a person. The raise can also be undone afterwards: any of the mode-lowering gates puts an elevated `auto` back at `confirm` and says which gate it was.

The hook upgrade (suggest -> run) is what delivers the seamless experience. Without it, users would need the `tsuku run` prefix, which defeats the purpose of shell integration.

Shims extend the same experience to hook-free contexts. Static shims keep the system simple -- all intelligence lives in `tsuku run` at runtime.

### Trade-offs Accepted

- **Auto-install in cloned repos**: Cloning a repo with `.tsuku.toml` and typing a declared command installs it, for a user who has configured no consent mode. This is intentional. A user who has set one keeps it: `auto_install_mode = suggest` is not overridden by a declaration, and see Security Considerations below for what that setting does and does not reach.
- **Binary index dependency**: The resolver needs the index built. Clear error message on cold start.
- **Command-not-found hook change**: Existing hook users get upgraded behavior. This is additive (run instead of suggest for project tools) not breaking.

## Solution Architecture

### Overview

Block 6 adds a `ProjectVersionResolver` implementation, modifies `tsuku run` to use it, upgrades the command-not-found hooks to call `tsuku run`, and adds a shim manager.

### Components

```
┌────────────────────────────────────────────────────────────────┐
│                       User Shell                               │
│                                                                │
│  User types: rg .foo data.json                                 │
│       │                                                        │
│       ▼                                                        │
│  command_not_found_handle                                      │
│       │                                                        │
│       └─ tsuku run rg -- .foo data.json                        │
│          (every unresolvable command; the hook does not        │
│           check .tsuku.toml, the runner does)                  │
└────────────────────────────────────────────────────────────────┘
            │
            ▼
┌────────────────────────────────────────────────────────────────┐
│  tsuku run (cmd_run.go, modified)                              │
│    │                                                           │
│    ├─ LoadProjectConfig(cwd) -> ConfigResult                   │
│    ├─ NewResolver(config, index.Lookup) -> resolver            │
│    ├─ Runner.Run(ctx, cmd, args, mode, resolver)               │
│    │    │                                                      │
│    │    ├─ resolver.ProjectVersionFor("rg")                    │
│    │    │    ├─ index.Lookup("rg") -> recipe "ripgrep"         │
│    │    │    └─ config.Tools["ripgrep"] -> "14.1.0"            │
│    │    │                                                      │
│    │    ├─ declared + mode origin is default -> mode = auto    │
│    │    ├─ gates may lower auto back to confirm                │
│    │    ├─ disclose, then install ripgrep@14.1.0 if needed     │
│    │    └─ syscall.Exec rg .foo data.json                      │
│    │                                                           │
└────────────────────────────────────────────────────────────────┘
```

### Key Interfaces

```go
// internal/project/resolver.go

type Resolver struct {
    config *ConfigResult
    lookup autoinstall.LookupFunc
}

func NewResolver(config *ConfigResult, lookup autoinstall.LookupFunc) autoinstall.ProjectVersionResolver

// ProjectVersionFor maps command -> recipe (via index) -> version (via config).
// Returns (version, true, nil) when the tool is project-declared.
// Returns ("", false, nil) when not in project config (falls through).
func (r *Resolver) ProjectVersionFor(ctx context.Context, command string) (string, bool, error)
```

```go
// internal/shim/manager.go

type Manager struct {
    binDir string
    cfg    *config.Config
}

func NewManager(cfg *config.Config) *Manager
func (m *Manager) Install(recipeName string) ([]string, error)
func (m *Manager) Uninstall(recipeName string) error
func (m *Manager) List() ([]ShimEntry, error)
func IsShim(path string) bool
```

### Changes to Existing Code

**`cmd/tsuku/cmd_run.go`**: Load project config, construct resolver, pass to `Runner.Run` where `nil` was.

**`internal/autoinstall/run.go`**: The resolver lookup must happen BEFORE the installed-tool fast path. When the resolver returns a pinned version, `Runner.Run` must exec from the version-specific bin directory (`$TSUKU_HOME/tools/{recipe}-{version}/bin/{command}`) rather than the global `tools/current/{command}` symlink. The fast path (exec from `tools/current/`) only fires when the resolver returns `!ok` or is nil.

**`autoinstall.Runner.Run`**: When the resolver returns a version (tool is project-declared) and the mode's origin is the unset default, raise the mode to `auto`. This is a small change in the existing Runner -- it reads the mode's origin as well as its value, and raises nothing that anything else set.

**Command-not-found hook scripts** (`internal/hooks/tsuku.{bash,zsh,fish}`): Add a check before calling `tsuku suggest`. If `.tsuku.toml` exists and declares the tool (via a quick `tsuku run --check <command>` or by having the hook call `tsuku run` directly and letting the Runner handle the suggest/run distinction), call `tsuku run <command> [args]` instead.

### Data Flow

**Flow 1: Interactive shell, project-declared tool**

```
1. User types: rg .foo data.json
2. Shell can't find rg -> command_not_found_handle fires
3. Hook calls: tsuku run rg .foo data.json
4. tsuku run loads .tsuku.toml: ripgrep = "14.1.0"
5. Resolver: rg -> ripgrep (index) -> 14.1.0 (config)
6. Runner: version from resolver -> mode = auto (project is consent)
7. Install ripgrep@14.1.0 if needed (no prompt)
8. exec rg .foo data.json
```

**Flow 2: Interactive shell, tool NOT in project config**

```
1. User types: jq .foo data.json
2. command_not_found_handle fires
3. Hook calls: tsuku run jq .foo data.json
4. tsuku run loads .tsuku.toml: jq not declared
5. Resolver returns (!ok)
6. Runner: normal consent mode (confirm by default)
7. Prompts: "jq is provided by recipe 'jq'. Install? [Y/n]"
```

**Flow 3: CI with shims**

```
1. CI runs: go build (shim at $TSUKU_HOME/bin/go)
2. Shim: exec tsuku run go -- build
3. Same as Flow 1 from step 4
```

**Flow 4: No .tsuku.toml at all**

```
1. User types: rg .foo data.json
2. command_not_found_handle fires
3. Hook calls: tsuku run rg .foo data.json
4. tsuku run: LoadProjectConfig -> nil
5. Resolver: NewResolver(nil, ...) -> all lookups return !ok
6. Runner: normal consent mode
7. Same behavior as today
```

## Implementation Approach

### Phase 1: Resolver (`internal/project/resolver.go`)

Build the `ProjectVersionResolver` implementation.

Deliverables:
- `internal/project/resolver.go`: `NewResolver`, `ProjectVersionFor`
- `internal/project/resolver_test.go`: Tests

### Phase 2: Wire Resolver into `tsuku run` + Bounded Mode Elevation

Modify `cmd_run.go` to construct the resolver and to resolve the mode's origin alongside its value. Modify `Runner.Run` to raise the mode to `auto` when the resolver returns a version and that origin is the unset default.

Deliverables:
- Modified `cmd/tsuku/cmd_run.go`
- Modified `internal/autoinstall/run.go` (mode elevation logic)
- Tests

### Phase 3: Upgrade Command-Not-Found Hooks

Change hooks to call `tsuku run` instead of `tsuku suggest` when `.tsuku.toml` declares the tool.

Deliverables:
- Modified `internal/hooks/tsuku.bash`, `tsuku.zsh`, `tsuku.fish`
- Tests for hook behavior

### Phase 4: Shim Manager (`internal/shim`)

Build shim creation, removal, and listing.

Deliverables:
- `internal/shim/manager.go`
- `internal/shim/manager_test.go`
- `cmd/tsuku/cmd_shim.go`

### Phase 5: Documentation

Update CLI help and CI usage examples.

## Security Considerations

### The threat

A user clones a repository they have not read, runs a command in it, and the
`.tsuku.toml` that repository ships causes a tool to be installed and executed.
That is the threat this section is about, and the mitigations below are offered
against it.

The exposure exists because a declaration is a consent signal written by
whoever wrote the repository. The rule bounds it -- a declaration raises only
the unset default, never a mode the user set -- so the exposed population is
users who have configured no consent mode, which is the common case.

Every install a declaration authorizes announces itself before it begins,
naming the recipe, the version, the path of the file that authorized it and the
recipe's source:

```
project-declaration: /home/dev/myproject/.tsuku.toml declares fd@10.2.0 (recipe source: registry)
```

That is disclosure, not prevention. It tells a user what happened; it does not
stop it.

### What a user can do about it

Two controls work against the threat as stated. Both have been followed and
then tested by typing the bare command in a repository that declares it.

**Do not install the command-not-found hook.** With no hook registered, an
unresolvable command goes to the shell's own handler and tsuku is never
invoked, so nothing installs. This is the strongest control, and it costs the
feature: `tsuku run` typed explicitly still installs, hook or no hook.

**Set `auto_install_mode = "suggest"` in `$TSUKU_HOME/config.toml`.** A mode
the user set is not raised by a declaration, so a declared tool in an untrusted
clone prints an install instruction and installs nothing. Setting it through
`TSUKU_AUTO_INSTALL_MODE` works too, and a `--mode` flag outranks both. This
control holds because the elevation is bounded; under the unconditional rule a
declaration raised `suggest` too, and the setting protected nobody.

**What `suggest` does not cover.** It governs installing, not running. A tool
already installed at the version the repository declares is executed straight
from `$TSUKU_HOME/tools`, on the fast path above, before any consent mode is
consulted -- so a repository that declares a version you happen to have already
gets that tool run for you regardless of the setting. `suggest` also says
nothing about what a tool does once it runs, and nothing about the install path
(`tsuku install`), which is a separate command with its own confirmation.

### Shim Safety

Same consent model as hooks. Shims call `tsuku run`, so the same bounded rule applies: a declaration raises an unset default, a mode the user set survives. Creating shims is an explicit user action (`tsuku shim install`), which is what makes "do not install the hook" reachable as a control without giving up shim-driven CI.

### Mitigations Summary

| Risk | Severity | Mitigation | Residual Risk |
|------|----------|------------|---------------|
| Untrusted repo installs a declared tool via the hook | Medium | Do not install the hook; or set a consent mode, which a declaration does not raise | A user who has configured no mode and kept the hook gets the install; a version already on disk runs without any mode being consulted |
| Untrusted repo installs a declared tool via a shim | Medium | Same, plus creating shims is an explicit user action | Same |
| Malicious .tsuku.toml declares many tools | Low | MaxTools cap (256) | Each declared tool is still one install |

## Consequences

### Positive

- **Seamless experience**: `rg .foo data.json` just works in a project that declares ripgrep
- **Complete vision**: The shell integration building blocks are fully connected
- **Minimal new code**: Resolver ~30-50 lines, hook change ~10 lines per shell, mode elevation ~5 lines in Runner
- **Three contexts, one experience**: Hooks, tsuku run, and shims all produce the same behavior

### Negative

- **Auto-install in untrusted repos**: Deliberate trade-off, bounded to users who have configured no consent mode.
- **Binary index dependency**: Resolver needs the index built
- **Hook behavior change**: command-not-found goes from suggest to run

### Mitigations

- **Untrusted repos**: The two controls in Security Considerations -- do not install the hook, or set a consent mode -- and the disclosure line on every install a declaration authorizes
- **Index dependency**: Clear error message directing to `tsuku update-registry`
- **Hook change**: Documented as installing and executing, in the guide and in `tsuku hook --help`, since it is not additive for an undeclared tool either
