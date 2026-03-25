---
status: Planned
upstream: docs/designs/DESIGN-shell-integration-building-blocks.md
spawned_from:
  issue: 1679
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
problem: |
  Developers using tsuku must run `tsuku install <tool>` explicitly before using any tool,
  even when tsuku already has a recipe for it. This friction accumulates in ad-hoc use and
  shared project environments where contributors must manually synchronize tool lists. The
  existing command-not-found hook surfaces the install suggestion but requires a second
  invocation; there is no path from "command not found" to "command runs" in a single step.
decision: |
  Introduce `tsuku run <command> [args...]` backed by a new `internal/autoinstall/` library
  that owns the install-then-exec flow. Three consent modes cover the three use cases: `confirm`
  (interactive prompt, default), `suggest` (print instructions, exit 1), and `auto` (silent
  install with audit log). Mode is resolved from a four-step chain: `--mode` flag, then
  `TSUKU_AUTO_INSTALL_MODE` env var, then `auto_install_mode` config key, then `confirm`.
  On Unix, `syscall.Exec` replaces the tsuku process after install so exit codes propagate
  directly. The library surface is stable from day one so `tsuku exec` (#2168) can import it.
rationale: |
  Placing the core in `internal/autoinstall/` follows every precedent in this codebase for
  shared cross-command logic and gives `tsuku exec` a clean import target without a later
  extraction refactor. The four-step mode resolution chain matches the existing `TSUKU_TELEMETRY`
  pattern and provides a CI escape hatch (env var) without sacrificing the safety default of
  `confirm`. Error-out on non-TTY confirm is the only non-TTY behaviour that satisfies the
  "fail fast with a clear error" constraint. The env var escalation restriction prevents
  repository-supplied `.envrc` files from silently enabling auto mode.
---

# DESIGN: Auto-Install Flow

## Status

Planned

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md), Issue #1679.

Relevant sections: Track A (Command Interception), Auto-Install block.

## Context and Problem Statement

Tsuku's current workflow is explicit: `tsuku install jq`, then `jq`. For ad-hoc use and
scripted environments, this friction adds up. Teams sharing a project must synchronize tool
lists manually. New contributors run unfamiliar commands and get generic "command not found"
errors even when tsuku has a recipe.

The command-not-found hook (#1678) solved the discovery problem — it intercepts unknown
commands and suggests `tsuku install`. Auto-install goes one step further: it installs the
tool when the user consents, then runs the command without a second invocation.

Three distinct use cases need different behavior:

- **Interactive developer**: wants a prompt before anything installs
- **Power user / automation author**: wants silent installs after opting in explicitly
- **CI / scripts**: wants explicit failure, never silent installs

The design must serve all three without requiring each group to modify shell hooks or
wrapper scripts.

### Scope

**In scope:**
- `tsuku run <command> [args...]` CLI command
- Three consent modes: suggest, confirm, auto
- Mode resolution order: flag, env var, config, default
- TTY detection for confirm mode and non-TTY fallback behavior
- Audit logging for auto-mode installs
- Integration interface for project config version pinning (#1680)
- Error handling: install failure, exec failure, partial install cleanup

**Out of scope:**
- Transparent shim generation (Block 6, #2168, builds on this design)
- Project configuration file format (Block 4, #1680)
- Shell environment activation (Block 5, #1681)
- LLM-based recipe discovery (future work)
- Windows shell support

## Decision Drivers

- **Safety default**: `confirm` mode requires explicit user consent; `auto` must be a
  deliberate opt-in, not something users stumble into
- **Exit code fidelity**: `tsuku run jq` exits with the same code `jq` would have returned;
  shell scripts relying on exit codes must not break
- **Offline lookup**: all tool discovery uses the local SQLite binary index; no network
  calls happen during command lookup
- **Non-TTY safety**: CI and scripts running with `confirm` mode must fail fast with a clear
  error, never hang waiting for input
- **Interface stability for #1680**: project config version pinning must plug in without
  changing auto-install internals
- **Audit trail for auto mode**: silent installs must leave a record the user can inspect

## Considered Options

### Decision 1: CLI Entry Point and Exec Handoff

The auto-install feature needs a user-facing invocation surface and a mechanism for handing
off to the installed command after installation. These two aspects are coupled: the choice of
surface determines what exec primitive is available, and whether the install logic needs to be
a reusable library or can stay embedded in a single command handler.

A downstream design, `tsuku exec` (#2168, project-aware exec wrapper), will need to replicate
the same install-then-exec flow for project-pinned tools. This is not a hypothetical future
concern — #2168 is already scoped and depends on this design. The install logic must therefore
be accessible as a library function from day one; building it first as a private command handler
and extracting later would require a refactor that could introduce regressions and delay #2168.

The exec handoff primitive on Unix is `syscall.Exec`, which replaces the tsuku process image
with the target command. Because tsuku's process is replaced — not forked — the tool's exit
code becomes the process exit code with no wrapping overhead. On Windows, `os/exec.Cmd.Run()`
with an explicit `os.Exit` is the fallback, following the pattern already used in `verify.go`.

**Key assumptions:**
- `#2168` will be a cobra subcommand in the same binary (`cmd/tsuku/cmd_exec.go`), not a separate binary
- `syscall.Exec` is the correct handoff primitive on Unix; cleanup must complete before calling it since deferred functions will not run after the replacement
- The binary index must already be built; if not, `tsuku run` exits `ExitIndexNotBuilt` (11), matching `tsuku suggest`

#### Chosen: `tsuku run` cobra subcommand backed by `internal/autoinstall/`

`tsuku run <command> [args...]` is a new cobra subcommand registered in `cmd/tsuku/main.go`.
`cmd/tsuku/cmd_run.go` is a thin wrapper: it resolves the mode, constructs the runner, and
delegates. All install-then-exec logic lives in a new `internal/autoinstall/` package that
exports a stable `Runner` type:

```go
// internal/autoinstall/run.go
type Runner struct { /* cfg, index, writers */ }

func (r *Runner) Run(ctx context.Context, command string, args []string,
    mode Mode, resolver ProjectVersionResolver) error
```

`cmd_run.go` passes a consent function appropriate for interactive use. `cmd_exec.go` (#2168)
passes its own consent policy. The exec handoff calls `syscall.Exec` on Unix after all cleanup
completes, so the tool's exit code propagates directly. Users should use `--` to separate tsuku
flags from the target command's flags: `tsuku run jq -- --arg foo bar`.

#### Alternatives Considered

**Embedded in `cmd_run.go` only (no shared library):** Implements the feature correctly for
#1679 but violates the interface-stability constraint. When #2168 arrives, all install-then-exec
logic must be extracted from `package main` to `internal/` — a refactor that creates unnecessary
risk and delays #2168's development. Rejected because the extraction cost is identical whether
done now or later, but doing it later guarantees disruption.

**Shell function / alias injection:** Provides transparent invocation (no `tsuku run` prefix)
but is a UX layer, not a library interface. It requires maintaining hook code in three shell
dialects, cannot provide reliable consent UX in non-interactive contexts, and still requires a
Go library surface for #2168. It could be layered on top of `tsuku run` as a future enhancement
— a shell wrapper that calls `tsuku run` — but does not answer the library interface question.
Rejected as a standalone answer; transparent invocation is deferred to #2168's shim system.

---

### Decision 2: Mode Resolution Order and Non-Interactive Fallback

Three consent modes drive the install decision: `suggest` prints install instructions and exits
without installing; `confirm` prompts the user interactively and installs only on explicit
affirmation; `auto` installs silently. The spec requires `confirm` as the default. Beyond the
mode definitions themselves, two technical choices remain: the priority chain for resolving the
active mode across flag, environment, config, and default; and what `confirm` mode does when
stdin is not a TTY.

The non-interactive fallback has direct security implications. If `confirm` degrades silently
to `auto` in a CI pipeline, software is installed without human consent — creating a supply
chain attack surface. If it degrades silently to `suggest`, the mode mismatch is hidden from
the operator, who sees suggestion output without realising their config setting was ignored.
An explicit error is the only behavior that satisfies the design driver "non-TTY safety: CI
and scripts must fail fast with a clear error."

Two codebase precedents shape the resolution chain. First, `TSUKU_TELEMETRY` and
`TSUKU_LLM_IDLE_TIMEOUT` already override config for behavioral preferences, establishing that
env vars are a legitimate override layer — not just for transient runtime tuning. Second,
`internal/userconfig.LLMIdleTimeout()` shows the canonical pattern: check env var first, fall
through to config, return the default.

**Key assumptions:**
- `TSUKU_AUTO_INSTALL_MODE` follows the established `TSUKU_` prefix naming convention
- The "fail fast with a clear error" non-TTY safety constraint is binding; relaxing it to allow silent degradation would make Option B (fallback to suggest) viable
- CI pipelines are sometimes generated or templated in ways that make per-command flag injection impractical, justifying the env var layer

#### Chosen: Flag > `TSUKU_AUTO_INSTALL_MODE` > config > default, error-out on non-TTY confirm

Resolution order (highest to lowest priority):

1. `--mode=<value>` cobra flag on `tsuku run`
2. `TSUKU_AUTO_INSTALL_MODE` environment variable (`suggest`, `confirm`, or `auto`) — **with the restriction below**
3. `auto_install_mode` key in `$TSUKU_HOME/config.toml`
4. Default: `confirm`

**Env var escalation restriction:** `TSUKU_AUTO_INSTALL_MODE=auto` via environment variable is
only honoured when `auto_install_mode = "auto"` is also set in the persistent `$TSUKU_HOME/config.toml`,
or when `--mode=auto` is passed explicitly as a flag. The environment variable alone cannot
escalate from `confirm` to `auto`. This prevents a malicious `.envrc` in a cloned repository
(loaded by `direnv` or similar) from silently bypassing the user's consent configuration.
The env var *can* downgrade mode (e.g., `auto` → `confirm`, `auto` → `suggest`) without
corroboration.

`cmd/tsuku/cmd_run.go` implements `resolveMode(flagMode string, cfg *userconfig.Config) (Mode, error)`
following this chain. `internal/userconfig.Config` gains `AutoInstallMode string` with TOML key
`auto_install_mode`.

When the resolved mode is `confirm` and `term.IsTerminal(int(os.Stdin.Fd()))` returns false,
`tsuku run` prints a clear error and exits with `ExitNotInteractive`:

```
tsuku: confirm mode requires a TTY; set TSUKU_AUTO_INSTALL_MODE=auto or use --mode=auto for non-interactive use
```

No installation is attempted. The error message names both escape hatches so operators know
immediately what to change. `ExitNotInteractive` is a new distinct exit code added to
`exitcodes.go`, allowing scripts to distinguish this condition from other failures.

#### Alternatives Considered

**Flag > config > default, error-out (no env var layer):** Functionally sound but inconsistent
with `TSUKU_TELEMETRY` and `TSUKU_LLM_IDLE_TIMEOUT` precedents. Generated or templated CI
configs cannot inject per-command flags; the env var exists precisely for this use case.
Rejected on consistency grounds.

**Flag > env var > config > default, silent fallback to `suggest`:** Violates the explicit
design driver "fail fast with a clear error." Silent degradation makes the mode mismatch
invisible to operators — they see suggestion output and cannot tell their `confirm` config
was ignored. Exit code 1 from `suggest` and exit code 1 from "confirm silently degraded to
suggest" are indistinguishable. Rejected.

**Flag > env var > config > default, silent fallback to `auto`:** Directly violates the core
security constraint. Silent escalation from `confirm` to `auto` installs software without
human consent, defeats the purpose of confirm mode, and creates a supply chain attack vector.
Rejected without qualification.

**Config only (no flag or env var):** Removes per-invocation control. Unusable in ephemeral
containers or read-only home directories where config file modification is impractical.
Rejected.

---

### Decision 3: Auto-Install Core API and Shared Interface Boundary

Both `tsuku run` (#1679) and `tsuku exec` (#2168) share the same install-then-exec flow and
need to accept version overrides from project config (#1680). The question is where the shared
logic and the shared `ProjectVersionResolver` interface live — and what the package boundary
looks like for both current and future consumers.

Go's convention for this pattern is an interface defined in the consuming package (or a shared
`internal/` package), with the implementing package importing the interface's type. The project
config package (#1680) does not exist yet; the interface must be defined now in a location where
both #1680 and #2168 can import it without creating import cycles.

The existing codebase already applies this pattern: `internal/hook/` is used by hook commands,
`internal/index/` is used by lookup and suggest, `internal/userconfig/` is used by config
commands. Shared logic that crosses command boundaries goes in `internal/`. The auto-install
core is exactly this kind of shared logic.

**Key assumptions:**
- `tsuku exec` (#2168) will be `cmd/tsuku/cmd_exec.go` in `package main`, making same-package access technically valid today — but not relied upon as a justification for keeping the core in `cmd/`
- The project config package (#1680) will implement `ProjectVersionResolver` from this package, plugging in without modifying the auto-install core
- `lookupBinaryCommand` in `cmd/tsuku/lookup.go` will either move into this package or remain as a thin adapter; `cmd_suggest.go` continues to work either way

#### Chosen: Extract to `internal/autoinstall/`

A new package at `internal/autoinstall/` exports:

- **`ProjectVersionResolver` interface** — one method:
  `ProjectVersionFor(ctx context.Context, command string) (version string, ok bool, err error)`.
  Returns the project-pinned version, or `ok=false` if no pin exists. Defined here so both
  `cmd_run.go` and `cmd_exec.go` share the same type without duplication.

- **`Mode` type and constants** — `ModeAuto`, `ModeConfirm`, `ModeSuggest`. Shared by both
  commands; resolved in `cmd_run.go`/`cmd_exec.go` via their respective `resolveMode` functions.

- **`Runner` struct and constructor** — `NewRunner(cfg *config.Config, stdout, stderr io.Writer) *Runner`.
  Holds the config and I/O writers; all methods are unit-testable without cobra or `os.Exit`.

- **`Runner.Run(ctx, command, args, mode, resolver)`** — the full install-then-exec flow.

`cmd_suggest.go` either imports lookup from `internal/autoinstall/` directly or keeps a thin
adapter in `cmd/tsuku/lookup.go`. Either approach avoids breaking the suggest command.

#### Alternatives Considered

**Keep in `cmd/tsuku/` (define interface and core in `cmd/tsuku/autoinstall.go`):** Technically
valid since both callers are in `package main`. Rejected because it buries testable library
logic in `package main`, discourages isolation testing, and sets a precedent that future
commands or consumers would need to break when reusing the logic.

**`internal/run/` or `internal/exec/` instead of `internal/autoinstall/`:** Functionally
identical to the chosen option. Rejected on naming: `internal/run/` creates a confusing overlap
with the `tsuku run` command's conceptual space; `internal/exec/` implies OS-level exec
semantics rather than the higher-level install-gate-exec orchestration this package performs.
`internal/autoinstall/` names the domain precisely.

---

### Decision 4: Audit Log Format

The `auto` mode installs tools silently without user confirmation. A record of these installs
is required so users can audit what was installed on their behalf. The question is whether that
record should be human-readable text or structured data.

There is no existing consumer of this log file — no dashboard, no `tsuku audit` command, no
log aggregation pipeline. The decision is whether to optimise for immediate human readability
(open the file, read it) or for future tooling (parse with `jq`, ingest into a log system).
The choice is low-stakes and reversible: migrating from text to NDJSON or vice versa is a
one-line format change with no external API contract.

**Key assumptions:**
- No existing tooling consumes `$TSUKU_HOME/audit.log` at design time
- The file is user-local; it is not shipped to a central log aggregator

#### Chosen: NDJSON (one JSON object per line)

Each auto-install appends one line:

```json
{"ts":"2026-03-25T12:00:00Z","action":"auto-install","recipe":"jq","version":"1.7.1","mode":"auto"}
```

Fields: `ts` (RFC-3339), `action`, `recipe`, `version`, `mode`. The file is parseable with
`jq` from day one, making future tooling (a `tsuku audit` command, log ingest, grep by recipe)
straightforward without a migration step.

#### Alternatives Considered

**Tab-separated text:** Human-readable with `cat` or `tail`, no parser needed. Rejected in
favour of NDJSON because structured data is more useful for tooling and the implementation
cost difference is negligible (`json.Marshal` vs `fmt.Fprintf`).

## Decision Outcome

**Chosen: 1B + 2A + 3B + 4A**

### Summary

The implementation centres on a new `internal/autoinstall/` package that owns the
install-then-exec flow, shared by both `tsuku run` (this design) and `tsuku exec` (#2168).
`cmd/tsuku/cmd_run.go` is a thin cobra wrapper: it resolves the consent mode through the
four-step priority chain (`--mode` flag → `TSUKU_AUTO_INSTALL_MODE` env → `auto_install_mode`
config → default `confirm`), then calls `autoinstall.Runner.Run(ctx, command, args, mode,
resolver)`. The `resolver` argument is `nil` for `tsuku run` (meaning "use latest"), and will
be a project config implementation for `tsuku exec`.

Inside `Runner.Run`, the flow is: look up `command` in the binary index offline; if already
installed, call `syscall.Exec` immediately (no prompt, no install); if not installed, apply
mode logic — `suggest` prints the install command and exits 1, `confirm` checks for a TTY
(returning `ExitNotInteractive` with an actionable error if stdin is not a TTY, then prompts
and installs on 'y'), `auto` installs silently and appends a timestamped NDJSON line to
`$TSUKU_HOME/audit.log`. After any successful install, `syscall.Exec` replaces the tsuku
process with the installed command, so the tool's exit code becomes the process exit code
with no wrapping.

Four security gates apply before any install proceeds: (1) `Runner.Run` checks
`os.Geteuid() == 0` and refuses to install or exec as root; (2) `$TSUKU_HOME/config.toml`
must be mode 0600 owned by the current user before its `auto_install_mode` value is
honoured — otherwise the mode falls back to `confirm`; (3) in `auto` mode, recipes without
upstream verification (`checksum_url` or `signature_url`) are ineligible for silent install
and fall back to `confirm`; (4) in `auto` mode, a command that resolves to more than one
recipe in the index falls back to `confirm` (conflict resolution from Block 1 is not yet
defined).

Three additions to `exitcodes.go` cover the new failure modes: `ExitNotInteractive` for
non-TTY confirm, `ExitUserDeclined` for an explicit 'n' at the prompt, and no new code for
install failure (existing `ExitInstallFailed = 6` applies). The `ProjectVersionResolver`
interface lives in `internal/autoinstall/`, giving #1680 a clear import target when it lands.

### Rationale

All three decisions converge on `internal/autoinstall/` as the shared boundary — this is not
incidental. The interface-stability constraint from #2168 is what drives the library extraction
decision (1B), and the same extraction naturally makes `ProjectVersionResolver` a stable import
point for #1680 (3B). Placing shared logic in `internal/` follows every precedent in this
codebase (`internal/hook/`, `internal/index/`, `internal/userconfig/`), so the approach fits
without introducing new conventions.

The mode resolution chain (2A) accepts a small complexity cost — four priority levels instead
of two — to satisfy two constraints simultaneously: the env var layer matches existing codebase
patterns (`TSUKU_TELEMETRY`, `TSUKU_LLM_IDLE_TIMEOUT`) and serves CI pipelines that can't
inject per-command flags. The error-out on non-TTY confirm is the only behavior consistent with
"fail fast with a clear error" — the two silent alternatives either hide the mode mismatch or
silently install software, both of which are worse outcomes than a clear diagnostic.

## Solution Architecture

### Overview

`tsuku run <command> [args...]` is a cobra subcommand backed by a new `internal/autoinstall/`
package. The package owns the full install-then-exec flow and is the stable library surface
that `tsuku exec` (#2168) imports. The command layer (`cmd_run.go`) handles flag parsing, mode
resolution, and TTY detection before delegating to the library.

### Components

**`internal/autoinstall/` — core library**

The install-then-exec flow. Exported types:

```go
// Mode is the installation consent mode.
type Mode int

const (
    ModeConfirm Mode = iota // default: prompt interactively
    ModeSuggest             // print instructions, exit 1
    ModeAuto                // install silently, audit log
)

// ProjectVersionResolver provides an optional version pin from project config.
// A nil resolver means "use latest". #1680 will implement this interface.
type ProjectVersionResolver interface {
    ProjectVersionFor(ctx context.Context, command string) (version string, ok bool, err error)
}

// Runner performs the install-then-exec flow.
type Runner struct { /* cfg *config.Config, stdout/stderr io.Writer */ }

func NewRunner(cfg *config.Config, stdout, stderr io.Writer) *Runner

// Run looks up command, applies mode logic, optionally installs, then execs.
// On Unix, successful exec replaces the process (syscall.Exec); Run never returns
// in that case. On install failure or user decline, Run returns an error wrapping
// the appropriate exit code.
func (r *Runner) Run(ctx context.Context, command string, args []string,
    mode Mode, resolver ProjectVersionResolver) error
```

**`cmd/tsuku/cmd_run.go` — thin cobra wrapper**

Handles flag binding (`--mode`), mode resolution, and TTY gating:

```go
func resolveMode(flagMode string, cfg *userconfig.Config) (autoinstall.Mode, error)
// Priority: --mode flag → TSUKU_AUTO_INSTALL_MODE env → cfg.AutoInstallMode → ModeConfirm
```

After resolving mode, `cmd_run.go` checks TTY when mode is `ModeConfirm`:
if `!term.IsTerminal(int(os.Stdin.Fd()))`, it prints the non-TTY error and exits
`ExitNotInteractive` before calling `Runner.Run`. This keeps TTY awareness out of the library
and makes the library unit-testable without mocking terminal state.

**`internal/userconfig/userconfig.go` — config extension**

New field: `AutoInstallMode string \`toml:"auto_install_mode,omitempty"\``

Unset or empty resolves to `confirm` in `resolveMode`.

**`cmd/tsuku/exitcodes.go` — new codes**

- `ExitNotInteractive = 12` — `confirm` mode with no TTY
- `ExitUserDeclined = 13` — user typed 'n' at the confirm prompt
- `ExitForbidden = 14` — operation refused (e.g., running as root)

**`$TSUKU_HOME/audit.log` — auto-mode audit trail**

Append-only NDJSON (one JSON object per line), created on first write with mode 0600:

```json
{"ts":"2026-03-25T12:00:00Z","action":"auto-install","recipe":"jq","version":"1.7.1","mode":"auto"}
```

Fields: `ts` (RFC-3339), `action` (always `"auto-install"`), `recipe`, `version`, `mode`.

### Key Interfaces

The `ProjectVersionResolver` interface is the primary integration point for downstream designs.
`tsuku run` passes `nil` (latest). `tsuku exec` (#2168) will pass a resolver backed by
`tsuku.toml` from the current directory. `internal/autoinstall/` never imports the project
config package; #1680 imports `internal/autoinstall/` and implements the interface.

The `Runner.Run` signature is the contract #2168 depends on. Its parameters — `command`,
`args`, `mode`, `resolver` — are the full public surface. The `--` separator is recommended in
documentation to prevent flag collision between tsuku flags and the target command's flags.

### Security Gates

Before any install or exec, `Runner.Run` enforces four guards in order:

1. **Root guard**: if `os.Geteuid() == 0`, return `ExitForbidden` — tsuku never execs as root
2. **Config permission check**: if `$TSUKU_HOME/config.toml` is not mode 0600 or not owned by the current user, log a warning and treat `auto_install_mode` as unset (effective mode falls back to `confirm`)
3. **Verification gate** (auto mode only): if the matched recipe has no `checksum_url` and no `signature_url`, fall back to `confirm` mode for this install
4. **Conflict gate** (auto mode only): if the binary index returns more than one recipe for the command, fall back to `confirm` mode for this install

Gates 3 and 4 apply only when the resolved mode is `auto`. They do not block `confirm` or `suggest` mode.

### Data Flow

```
tsuku run jq .foo data.json
  │
  ├─ cmd_run.go: resolveMode() → ModeConfirm
  │   (env var TSUKU_AUTO_INSTALL_MODE=auto only honoured if config also has auto)
  ├─ cmd_run.go: TTY check (pass — interactive shell)
  │
  ├─ autoinstall.Runner.Run(ctx, "jq", [".foo", "data.json"], ModeConfirm, nil)
  │   ├─ Root guard: os.Geteuid() != 0 → ok
  │   ├─ Config permission check → ok
  │   ├─ lookupBinaryCommand("jq") → []BinaryMatch
  │   │
  │   ├─ [if installed] syscall.Exec("/home/user/.tsuku/bin/jq", ...) ← process replaced
  │   │
  │   ├─ [if not installed, ModeConfirm]
  │   │   ├─ print "Install jq? [y/N]: "
  │   │   ├─ read stdin → 'y'
  │   │   ├─ tsuku install jq@<version from resolver, or latest>
  │   │   └─ syscall.Exec("/home/user/.tsuku/bin/jq", ...) ← process replaced
  │   │
  │   ├─ [if not installed, ModeSuggest]
  │   │   └─ print "Install with: tsuku install jq" → exit 1
  │   │
  │   └─ [if not installed, ModeAuto]
  │       ├─ Verification gate: recipe has checksum_url or signature_url? else → ModeConfirm
  │       ├─ Conflict gate: single match? else → ModeConfirm
  │       ├─ tsuku install jq@<version>
  │       ├─ append NDJSON line to $TSUKU_HOME/audit.log
  │       └─ syscall.Exec("/home/user/.tsuku/bin/jq", ...) ← process replaced
  │
  └─ [if ExitIndexNotBuilt] print guidance → exit 11
```

## Implementation Approach

### Phase 1: `internal/autoinstall/` skeleton

Deliverables:
- `internal/autoinstall/autoinstall.go`: `Mode` type, `ProjectVersionResolver` interface, `Runner` struct, `NewRunner` constructor
- `internal/autoinstall/run.go`: `Runner.Run` stub (returns `ErrNotImplemented`)
- `internal/autoinstall/autoinstall_test.go`: test infrastructure — mock resolver, injectable writers

### Phase 2: Userconfig and exit codes

Deliverables:
- `internal/userconfig/userconfig.go`: `AutoInstallMode string` field
- `cmd/tsuku/exitcodes.go`: `ExitNotInteractive = 12`, `ExitUserDeclined = 13`
- `cmd/tsuku/cmd_run.go`: `resolveMode` function with tests
- `tsuku config` command: register `auto_install_mode` key with description

### Phase 3: Core flow in `Runner.Run`

Deliverables:
- Full `Runner.Run` implementation: lookup → mode dispatch → suggest/confirm/auto → install → exec
- Confirm prompt with consent reader (injectable for tests)
- Audit log writer for auto mode
- Unit tests covering all mode paths, install failure, index-not-built

### Phase 4: `tsuku run` cobra command

Deliverables:
- `cmd/tsuku/cmd_run.go`: cobra command registration, `--mode` flag, help text
- TTY gating with `ExitNotInteractive` path
- Integration test: `go test ./cmd/tsuku/ -run TestRun*` using subprocess

## Security Considerations

### External artifact handling

The auto-install flow triggers the existing tsuku install pipeline, which downloads and
executes external binaries. Mitigations already in place: HTTPS enforcement, checksum URL
verification, PGP signature support with fingerprint pinning, SSRF-protected HTTP client.

**Auto-mode verification gate:** In `auto` mode, the pipeline runs without human review. To
maintain the integrity guarantee that manual installs provide, `Runner.Run` treats a recipe
without `checksum_url` or `signature_url` as ineligible for silent install. It falls back to
`confirm` mode for that recipe, presenting a prompt that cites the missing verification.
This makes the verification requirement mechanical rather than opt-in for recipe authors.

### Privilege escalation prevention

`Runner.Run` checks `os.Geteuid() == 0` before any install or exec. If running as root,
it returns `ExitForbidden (14)` with an error message directing the user to run as a
non-root user. This prevents `syscall.Exec` from launching an arbitrary binary with
root privileges.

### Consent bypass via environment injection

`TSUKU_AUTO_INSTALL_MODE=auto` set in a repository's `.envrc` (loaded automatically by
`direnv` or similar) would otherwise silently override a user's configured `confirm` mode.
The env var escalation restriction prevents this: the environment variable can only activate
`auto` mode when `auto_install_mode = "auto"` is also set in `$TSUKU_HOME/config.toml` or
`--mode=auto` is passed explicitly. The env var alone can downgrade mode (e.g., `auto` →
`confirm`) but cannot escalate.

`$TSUKU_HOME/config.toml` must be mode 0600 and owned by the current user before its
`auto_install_mode` value is honoured. If permissions are incorrect, `resolveMode` logs a
warning and treats the config value as absent (effective mode falls back to `confirm`).

### Supply chain trust

The binary index maps command names to recipes. In `auto` mode, a command name that resolves
to multiple recipes would make the choice of which to install non-deterministic from the user's
perspective. The conflict gate (Security Gate 4) prevents silent install when the index returns
more than one match, falling back to `confirm` so the user makes an explicit choice.

### Audit trail

Silent installs in `auto` mode append a timestamped NDJSON line to `$TSUKU_HOME/audit.log`
(mode 0600). Command arguments are not logged — arguments may contain secrets. The log has no
built-in rotation policy; the user is responsible for managing its size. A `tsuku audit`
command or rotation support is deferred to a follow-on issue.

### Environment variable inheritance

`syscall.Exec` passes the full process environment to the installed tool. Users who run
`tsuku run` with secrets in their environment (e.g., `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`)
should be aware that the target tool inherits those values. This is standard UNIX process
behaviour and is not unique to tsuku, but is worth noting for users using `auto` mode with
tools they haven't explicitly reviewed.

## Consequences

### Positive

- Users run `tsuku run jq .foo data.json` without a prior `tsuku install` step; the tool is
  provisioned on first use
- Exit code fidelity via `syscall.Exec`: shell scripts that test `$?` after `tsuku run` see the
  tool's own exit code, not tsuku's
- `internal/autoinstall/` gives `tsuku exec` (#2168) a clean, stable import on day one,
  avoiding a later extraction refactor
- `confirm` default protects users from accidental silent installs; `auto` requires deliberate
  opt-in via config or env
- `ExitNotInteractive` lets CI pipelines detect the "wrong mode for this context" condition
  and fail with a clear diagnostic rather than a generic exit 1

### Negative

- `tsuku run <command>` prefix is more verbose than transparent invocation; users who want
  `jq` instead of `tsuku run jq` must wait for #2168's shim system
- `syscall.Exec` on Unix means deferred cleanup (temp files, signal handlers) does not run
  after the handoff; callers must complete all cleanup before invoking `Run`
- The four-step mode resolution chain is slightly complex to document and test; env var names
  are another piece of global state users must know about

### Mitigations

- Transparent invocation (no prefix) is deferred to #2168's shim system, which builds on
  this design; `tsuku run` is the canonical primitive that shims call
- `Runner.Run` documentation must state explicitly that no deferred functions run after
  `syscall.Exec`; the implementation must complete all cleanup before the call
- `tsuku run --help` documents all three modes, the env var name, and the config key in one
  place; `tsuku config set auto_install_mode auto` provides a discoverable way to change the default
