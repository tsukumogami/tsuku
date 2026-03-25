---
status: Proposed
upstream: docs/designs/DESIGN-shell-integration-building-blocks.md
spawned_from:
  issue: 1679
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
problem: |
  Users must explicitly install every tool before using it. `tsuku install jq` works, but
  the friction compounds in scripting and project contexts where teams share tool lists.
  The missing piece is a first-use provisioning command: run a command, let tsuku handle
  installation transparently, then execute it. Three consent modes (suggest, confirm, auto)
  accommodate different safety postures without a one-size-fits-all default.
decision: |
  Add `tsuku run <command> [args...]` that looks up the command in the binary index, handles
  the missing-tool case according to the configured mode (suggest/confirm/auto), installs if
  consented, then execs the command. Mode is resolved as: `--mode` flag > `TSUKU_AUTO_INSTALL`
  env var > `auto_install_mode` config key > default `confirm`. A `ProjectVersionResolver`
  interface accepts a version override from project config (#1680) without coupling the
  auto-install core to that package. Audit events are appended to `$TSUKU_HOME/audit.log`.
rationale: |
  `tsuku run` mirrors how container runtimes expose run-without-install semantics, making the
  UX intuitive. Choosing `confirm` as the default balances discoverability against supply-chain
  risk: users who want silent installs must opt in explicitly. The `ProjectVersionResolver`
  interface keeps the auto-install core free of direct #1680 dependencies while allowing
  version pinning to plug in at the call site. Exiting with the command's real exit code
  (via `syscall.Exec` on Unix) preserves script correctness.
---

# DESIGN: Auto-Install Flow

## Status

Proposed

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md), Issue #1679.

Relevant sections: Track A (Command Interception), Auto-Install block.

## Context and Problem Statement

Tsuku's current workflow is explicit: `tsuku install jq`, then `jq`. For ad-hoc use and
scripted environments this friction adds up. Teams sharing a project must synchronize tool
lists manually. New contributors run unknown commands and get unhelpful "command not found"
errors even when tsuku has a recipe.

The command-not-found hook (#1678) solved the discovery problem: it intercepts unknown
commands and suggests `tsuku install`. Auto-install goes further — it actually performs the
install when the user consents, then runs the command without requiring a second invocation.

Three distinct use cases need different behavior:

- **Interactive developer**: wants a prompt before anything installs
- **Power user / automation**: wants silent installs after opting in explicitly
- **CI / scripts**: wants explicit failure, never silent installs

The design must serve all three without requiring each group to modify shell hooks or
wrapper scripts.

## Decision Drivers

- **Safety default**: `confirm` mode requires explicit user consent before installing; `auto`
  must be a deliberate opt-in, not something users stumble into
- **Exit code fidelity**: `tsuku run jq` exits with the same code that `jq` would have; shell
  scripts relying on exit codes must not break
- **Binary index dependency**: all tool discovery is offline, using the existing SQLite index;
  no network calls during lookup
- **Interface stability for #1680**: project config version pinning must plug in without
  changing auto-install internals
- **Non-TTY safety**: CI and scripts that accidentally set `auto_install_mode = confirm` must
  not hang waiting for input; they must fail fast with a clear error

## Considered Options

### CLI surface: `tsuku run` vs transparent exec shim

**Option A — `tsuku run <command> [args...]`:** Explicit prefix. Users type
`tsuku run jq .foo file.json`. No shell modification required.

**Option B — Transparent shim in PATH:** Generate a thin wrapper script per managed command,
placed in `$TSUKU_HOME/bin/`. Scripts intercept unknown commands transparently.

**Option C — Shell alias injection:** Modify shell config to define `command_not_found_handle`
that calls `tsuku run` instead of `tsuku suggest`.

Option A was chosen for this design. Option B is the subject of #2168 (project-aware exec
wrapper), which builds on this design rather than replacing it. Option C couples auto-install
tightly to the shell hook and conflates suggest with install; #1678 deliberately kept suggest
separate. `tsuku run` provides the clean, composable primitive.

---

### Mode resolution order: flag > env > config > default

**Option A — flag > env > config > default:** Allows scripts to override via env
(`TSUKU_AUTO_INSTALL=auto`), permanent change via config, and one-off override via flag.

**Option B — flag > config > default (no env):** Simpler, but env var override is important
for CI pipelines that can't modify config files.

**Option C — config only (no flag, no env):** Lowest flexibility; breaks scripting use cases.

Option A chosen. The env var name `TSUKU_AUTO_INSTALL` follows existing patterns in the
codebase (`TSUKU_HOME`, `TSUKU_TELEMETRY`).

---

### Non-TTY fallback: error vs suggest

**Option A — Error with message:** When `confirm` mode is active and stdin is not a TTY,
exit non-zero with an explanation: "stdin is not a TTY; use --mode=auto to install without
a prompt, or run interactively."

**Option B — Silently fall back to suggest:** Print install instructions and exit, mimicking
the hook behavior.

**Option C — Silently fall back to auto:** Install without prompting.

Option A chosen. Options B and C both hide the mode mismatch. A CI pipeline that accidentally
runs with `confirm` mode should fail loudly, not silently succeed (B) or silently install (C).
The error message gives the user a direct fix.

---

### Project version interface: concrete dependency vs interface

**Option A — `ProjectVersionResolver` interface with a nil default:** Define
`type ProjectVersionResolver interface { VersionFor(ctx, command) (version string, ok bool, err error) }`.
Pass `nil` when no project config is loaded; auto-install treats nil as "no override."

**Option B — Direct import of the project config package (#1680):** Call
`projectconfig.Load()` inline.

**Option C — Config file lookup at auto-install time (inline):** Read `tsuku.toml` directly
in `cmd_run.go` without an interface.

Option A chosen. Option B creates a circular dependency risk and couples the auto-install
command to a package that doesn't exist yet. Option C embeds policy in the wrong place.
The interface keeps auto-install testable in isolation and allows #1680 to inject a real
resolver without modifying the `run` command.

---

### Audit logging: structured vs human-readable

**Option A — Append-only human-readable log at `$TSUKU_HOME/audit.log`:**
`2026-03-25T12:00:00Z auto-install jq@1.7.1 (mode=auto, requested_by=tsuku run)`

**Option B — Structured JSON log:** One JSON object per line.

**Option C — No audit log; rely on shell history.**

Option A chosen for initial design. JSON (Option B) is more parseable but adds complexity
before there's a clear consumer. Shell history (Option C) doesn't capture mode or version.
The human-readable format is readable with `cat` and can be upgraded to JSON later.

## Decision Outcome

`tsuku run <command> [args...]` is the auto-install primitive. It:

1. Looks up `<command>` in the binary index (offline, no network)
2. If already installed, execs the command directly (no install, no prompt)
3. If not installed, applies mode logic:
   - **suggest**: prints `tsuku install <recipe>` and exits 1
   - **confirm**: prompts interactively (requires TTY); installs on "y", exits 1 on "n" or Ctrl-C
   - **auto**: installs silently, appends to audit log
4. After install, execs the command with the original arguments
5. Exit code is always the command's exit code (via `syscall.Exec` on Unix)

## Solution Architecture

### New command: `cmd/tsuku/cmd_run.go`

```
tsuku run <command> [args...]

Flags:
  --mode string   Override auto_install_mode config (suggest|confirm|auto)
```

### Mode resolution

```
resolveMode(cmd *cobra.Command, cfg *userconfig.Config) (Mode, error)

1. If --mode flag set: parse and use it
2. Elif TSUKU_AUTO_INSTALL env var set: parse and use it
3. Else: use cfg.AutoInstallMode (default "confirm")
```

### Run flow

```
runAutoInstall(ctx, stdout, stderr, cfg, resolver, command, args, mode):
  matches, err := lookupBinaryCommand(ctx, cfg, command)
  if err is ErrIndexNotBuilt:
    print guidance, exit ExitIndexNotBuilt (11)
  if no matches:
    print "No recipe provides <command>", exit ExitGeneral (1)
  if any match is Installed:
    exec command [args]  // already installed, skip install logic
  pick best match (single match, or installed match if multiple)
  version := resolver.VersionFor(ctx, command)  // nil resolver -> latest
  switch mode:
    suggest: print "Install with: tsuku install <recipe>", exit 1
    confirm: prompt user; if no TTY, exit with error
    auto:    install silently, audit log
  if install fails: print error, exit ExitInstallFailed (6)
  exec command [args]
```

### ProjectVersionResolver interface

Defined in `cmd/tsuku/cmd_run.go` (or a shared internal package if #2168 also uses it):

```go
// ProjectVersionResolver optionally overrides the version used when
// auto-installing a tool. Implementations are provided by the project
// config package (#1680). A nil resolver means "use latest."
type ProjectVersionResolver interface {
    // VersionFor returns the project-pinned version for the given command.
    // ok=false means no pin; the caller uses latest.
    VersionFor(ctx context.Context, command string) (version string, ok bool, err error)
}
```

The `tsuku run` command passes `nil` at construction time. #1680 will wire in a real
resolver when project config is loaded.

### UserConfig extension

Add to `internal/userconfig/userconfig.go`:

```go
// AutoInstallMode controls how tsuku run handles missing tools.
// Valid values: "suggest", "confirm", "auto". Default: "confirm".
AutoInstallMode string `toml:"auto_install_mode,omitempty"`
```

Register the key in `cmd/tsuku/config.go` alongside existing keys (`telemetry`, `llm.*`).

### Audit log

Appended by the `auto` mode install path only:

```
$TSUKU_HOME/audit.log
```

Format (one line per install):

```
2026-03-25T12:00:00Z  auto-install  jq@1.7.1  mode=auto
```

Fields: ISO-8601 timestamp, action, `recipe@version`, `mode=<mode>`. Tab-separated.
File is created on first write with mode 0600. No rotation in v1.

### Exec semantics

On Unix: use `syscall.Exec(path, args, env)` to replace the tsuku process. The command
inherits tsuku's file descriptors and exit code is the command's exit code.

On Windows (if supported): `os/exec.Cmd.Run()` with the command's exit code forwarded.

## Implementation Approach

Single issue (#1679) implementing:

1. `AutoInstallMode` field in `userconfig.Config` + config command registration
2. `cmd/tsuku/cmd_run.go`: full run command with mode resolution, flow, exec
3. `ProjectVersionResolver` interface (nil default — #1680 plugs in later)
4. Audit log writer
5. TTY detection (using `golang.org/x/term.IsTerminal`)
6. Unit tests for mode resolution, non-TTY error, suggest mode, confirm mode (y/n), auto mode
7. Registration in `cmd/tsuku/main.go`

The implementation does not require Docker container tests — all code paths are unit-testable
with injectable readers/writers and a fake `ProjectVersionResolver`.

## Security Considerations

### Threat model

**Malicious recipe execution via `auto` mode:** If a recipe is compromised (e.g., supply
chain attack on the upstream binary), `auto` mode installs and execs it without user
confirmation. Mitigations:

- `auto` mode is not the default; users must set `auto_install_mode = auto` explicitly
- The audit log provides a record of every auto-installed tool and version
- Recipe integrity verification (checksums) is the responsibility of the install pipeline,
  not the auto-install flow; this design does not weaken existing verification
- The config command documentation should warn: "Setting auto to auto_install_mode disables
  confirmation prompts. Only use this in controlled environments where recipe integrity
  is verified upstream."

**Command injection via `<command>` argument:** The command name is looked up in the index;
only exact matches trigger install. The value is passed to `syscall.Exec` as an argument
array, not interpolated into a shell string. No injection risk.

**`confirm` mode TOCTOU:** User confirms at prompt, then the network request happens. An
attacker who can modify the recipe registry between confirmation and download could swap
content. Mitigation: the install pipeline verifies checksums against the recipe manifest;
this is out of scope for this design.

**Non-TTY `confirm` bypass:** A script that pipes "y\n" to stdin could bypass the confirm
prompt. Mitigation: TTY detection uses `term.IsTerminal(int(os.Stdin.Fd()))` which returns
false for pipes. The design specifies that `confirm` mode in a non-TTY context exits with an
error rather than reading from stdin.

**Audit log tampering:** The audit log is append-only with mode 0600, but it is not
cryptographically signed. It provides a convenience record, not a tamper-proof audit trail.
This is acceptable for v1; cryptographic signing can be added later.

### Production and CI recommendation

Document in user-facing help: for CI and production environments, set
`auto_install_mode = suggest` (or omit configuration and rely on the `confirm` default,
which fails fast in non-TTY contexts). Never set `auto_install_mode = auto` in shared CI
environments where the runner is not fully trusted.

## Consequences

**Positive:**
- Users can run `tsuku run jq .foo data.json` without a prior install step
- Exit code fidelity means existing scripts are safe to use the command in
- `confirm` default protects users who haven't thought about auto-install policy
- `ProjectVersionResolver` interface lets #1680 plug in without changes to this code

**Negative:**
- `auto` mode creates a risk surface if users enable it without understanding implications;
  mitigated by documentation and the audit log
- `syscall.Exec` on Unix means deferred cleanup (e.g., `defer` blocks) doesn't run after
  exec; callers must flush state before calling exec

**Neutral:**
- The `tsuku run` prefix is slightly more verbose than transparent shim approaches (#2168);
  users who want transparent invocation should use #2168 when available
