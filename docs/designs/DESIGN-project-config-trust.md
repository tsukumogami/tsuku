---
schema: design/v1
status: Proposed
upstream: docs/prds/PRD-project-config-trust.md
problem: |
  (filled at Phase 6)
decision: |
  (filled at Phase 6)
rationale: |
  (filled at Phase 6)
---

# DESIGN: Project Config Trust

## Status

Proposed

## Context and Problem Statement

A `.tsuku.toml` reaches three code paths that each treat it as the invoking
user's own file. The PRD (`docs/prds/PRD-project-config-trust.md`) states the
outcomes; this section describes the technical problem in each path.

**Discovery.** `project.LoadProjectConfig` (`internal/project/config.go:113`)
resolves the start directory with `filepath.EvalSymlinks`, then walks parents.
At each directory it returns nil if the directory is a ceiling, otherwise
`os.Stat`s `<dir>/.tsuku.toml` and, if present, reads and parses it with
`os.ReadFile`. Ceilings are `filepath.Clean($HOME)` plus each
`TSUKU_CEILING_PATHS` entry, compared as strings. Nothing looks at who owns a
file or a directory, so outside `$HOME` the walk reaches `/`, and because the
ceilings are not resolved, a symlinked `$HOME` or ceiling never matches either
(both reproduced). The stat and the read are separate calls, so any check made
on the stat result is separated from the bytes parsed. There are two ways in:
`loadProjectConfigReporting` (`cmd/tsuku/project_config.go:22`), which serves
`tsuku install`, `tsuku run` and `tsuku shim install` and prints content
diagnostics to stderr, and `activation.ComputeActivation`
(`internal/activation/activate.go:139`), which serves `tsuku hook-env` and
`tsuku shell` and whose stdout the shell evaluates. `tsuku run` discards the load
error (`cmd/tsuku/cmd_run.go:209`).

The fix has to refuse a file the invoking user didn't choose (a `/tmp/.tsuku.toml`
another account planted above `/tmp/shared-work`, a directory another user
pre-created under a world-writable parent, a symlink owned by or pointing at a
foreign file) while still finding a checkout's own config when the checkout
lives outside `$HOME`: owned by the user, owned by root on a container volume,
owned by another non-root uid (host uid 1000 used as 1001), or owned by a
non-root uid with tsuku running as root in a Dockerfile `RUN`. Inside a resolved,
user-owned `$HOME` other than `/`, behavior must not change. A refused file stops
the walk and is never parsed. A refusal is one stderr line naming the file
(quoted), the reason and the remedy; hook-env reports it once per refused file,
not per prompt, and never writes it to stdout; `tsuku run` reports it on each
invocation and continues as if there were no config; `tsuku install` and
`tsuku shim install` fail on it.

**Source registration.** Two callers reach `ensureDistributedSource`
(`cmd/tsuku/install_distributed.go:85`): the command-line argument path
(`cmd/tsuku/install.go:243`) and the project pre-scan
(`cmd/tsuku/install_project.go:107`). Both run before their `--dry-run` branch,
and inside, `!autoApprove && isInteractive()` means a missing terminal falls
through to `autoRegisterSource`, which saves `config.toml`. `--yes` and `--force`
are both passed as `autoApprove`. The project install's `Proceed? [Y/n]` comes
after the pre-scan, so declining it doesn't undo a registration. The fix has to
make a dry run write nothing for any source (a session-only provider from
`addDistributedProvider` is enough to resolve distributed recipes); make a
project-named source need an interactive yes, `--yes` or `--force` (or an existing
entry), never a missing terminal or `CI=true`; write the entry only once the
install proceeds; skip the unapproved source's tools while installing the rest,
exit with a code that means "needs approval", and name the source, the declaring
file, `tsuku install --yes` and `tsuku registry add`; and record, for a
project-caused registration, the approval mechanism and the declaring path,
shown by `tsuku registry list`. Command-line-named sources outside dry-run,
`strict_registries` and `tsuku registry add` keep their current behavior.

**Run escalation.** `autoinstall.Runner.Run` (`internal/autoinstall/run.go:175`)
calls `elevate(mode, origin, declaration != nil)`, which turns an unset default
into auto for any declared command. The declaration reduces every key to a bare
recipe name (`internal/project/declaration.go`), and the recipe is matched
against the binary index, whose `Source` is `"registry"` or `"installed"` keyed
by name rather than by where the recipe came from. The install then resolves the
bare name through the loader chain (local, embedded, central, then registered
distributed sources). The fix has to raise only when the key is bare and the
recipe the run would install comes from the default registry (central or
embedded) or the local recipes; not raise for any registered distributed source;
name the real source in the prompt and the no-terminal message, and say when the
key named a different source; and leave explicit modes, the mode-lowering gates,
the environment-variable restriction and the already-installed fast path as they
are.

**Cross-cutting.** No consent or trust input may come from `.tsuku.toml`. The
decisions under discovery and escalation must be testable as a non-root user,
so the invoking uid, path owners and modes, discovery's filesystem access and the
terminal check need seams. The documentation that describes discovery and
consent (`DESIGN-shell-env-activation.md` and the `tsuku-user` skill) has to match.

## Decision Drivers

- **Refuse what the user didn't choose, without breaking outside-`$HOME`
  checkouts.** The PRD's layout table (L1-L10) is the acceptance target for
  discovery: planted parents and squatted shared directories are refused for
  root and non-root users; user-, root- and foreign-uid-owned checkouts under a
  non-world-writable parent are found.
- **No change inside the user's home.** Configs strictly below a resolved,
  user-owned `$HOME` other than `/` behave exactly as today, including the
  existing tests that fence discovery with `HOME` or `TSUKU_CEILING_PATHS`.
- **Never silent, never on hook-env stdout.** Every refusal reaches stderr once
  in a form the user can act on; hook-env's stdout is evaluated by the shell.
- **A missing terminal, a dry run or a CI flag is never consent** for a change
  that outlives the command, while `--yes`, `--force` and prior registration
  still work for scripts that rely on them.
- **Default-registry declarations see no new prompt**, and the existing
  disclosure line for them is unchanged.
- **Hot-path cost.** `tsuku hook-env` runs on every prompt; discovery may add
  metadata reads only on paths it already considers, no content reads before the
  decision and no network calls.
- **Testable without root or a second account**, including in-process tests of
  `cmd/tsuku` commands, following the existing `configPermissionCondition`
  pattern (`internal/autoinstall/run.go`) of passing the uid as a parameter.
- **Supported platforms are Linux and macOS.** `syscall.Stat_t` is available on
  both; the binary does not build for Windows today.
- **One reviewable PR** covering #2555, #2552 and #2559.
