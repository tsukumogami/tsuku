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

## Considered Options

Five decisions were evaluated independently. Decisions 1, 2 and 5 are
user-visible and are recorded here once confirmed; decisions 3 and 4 follow.

### Decision 3: How project install gates, defers and records a source registration

Today one function validates a source, checks whether it is registered, applies
`strict_registries`, prompts, writes `config.toml` and builds a session provider,
and both callers run it before their `--dry-run` branch
(`cmd/tsuku/install.go:243`, `cmd/tsuku/install_project.go:107`). A missing
terminal falls through to the write. The project install's "Proceed?" gate comes
after that write, so declining it cannot undo the registration.

Key assumptions: the registration write is the only `config.toml` write on any
install path, so gating it is sufficient for #2552's two validation scripts; a
project install reads one `.tsuku.toml`, so every project-named source in a run
shares a declaring file.

#### Chosen: split primitives, with the write deferred to a commit step after "Proceed?"

The function is rebuilt from three primitives: classify a source (validate, look
it up, apply the strict refusal; no network, no write), add a session-only
provider, and write the entry. The command-line path composes them in today's
order, so its behavior, including its non-terminal registration, is unchanged.

The project install gets a plan object that classifies each unique source, asks
about each unregistered one after the tool list and before "Proceed?", and writes
every approved source in a single save immediately after "Proceed?" and before
any recipe fetch. Declining at either prompt leaves `config.toml` untouched. A
source without consent has its tools skipped while the rest of the install
proceeds, and the command exits with a new code, 16, that outranks the
install-failure codes. A dry run classifies, notes each unregistered source on
stderr and writes nothing, whatever the terminal and flags.

Each project-caused entry records how it was approved and the resolved absolute
path of the declaring config, as two optional string fields alongside the
existing auto-registration flag, written once and never updated.

Deferring the write is the only change the feature actually requires, and
separating the write from the check turns "write after the user proceeds" into an
ordering in the caller rather than a flag inside a shared function. A new exit
code is used because every existing one is either claimed or misleading: one
advertises a flag this command does not have, one already means a complete abort,
one is a security block taken by decision 4, and one cannot be told apart from an
install failure.

#### Alternatives Considered

- **A mode or origin parameter on the existing function.** One call cannot
  express "decide now, write after a later prompt" without returning a pending
  token, and the gap spans the tool list, the dry-run branch and the "Proceed?"
  gate. It also folds a five-way flag matrix into the one function whose
  non-terminal branch an existing test pins.
- **A separate project-mode entry point.** Duplicates validation, the
  registration lookup and the strict refusal whose exact message a functional
  scenario asserts, and the command-line path has to change anyway to stop
  writing under `--dry-run`.
- **Register during the pre-scan and roll back on decline or dry run.** Saving
  re-encodes the whole file from a residue-free decode, so a restore cannot be
  byte-identical and drops unknown keys; it races concurrent writers; and a dry
  run that writes and then undoes has still written.
- **Gate only the write with a boolean.** The write is not the only thing that
  moves: the question relocates in the output, and per-source bookkeeping needs
  more than the existing per-tool failure flag can carry.
- **A real terminal pair in tests, with no production change.** The requirement
  is that the terminal check itself be substitutable; and because each prompt
  builds its own buffered reader, scripted input loses its second answer whatever
  the fixture does.
- **A nested table or a coarse origin enum for provenance, or a side file.** The
  nested table writes a bare empty header into every pre-existing entry on the
  first save; an enum records neither fact the requirement asks for; a side file
  is a second source of truth that `tsuku registry remove` lets drift.

### Decision 4: How a refused config is represented and reported

Five callers have to report a refusal, and their existing error paths differ:
`tsuku run` discards the load error entirely (`cmd/tsuku/cmd_run.go:209`), while
the shell hook and `tsuku shell` already give the existing parse error exactly
the treatment the requirements ask for here.

#### Chosen: a typed refusal error, carried on activation's existing unusable-config path

`internal/project` gains a `RefusedError` alongside `ParseError`, carrying the
refused path, the directory holding it, the reason and the remedy. Discovery
returns it instead of a config, having read no bytes. The shared loader helper
prints the single stderr line, because `tsuku run` discards the error and would
otherwise lose it silently; `tsuku install` and `tsuku shim install` then only
choose an exit code, 14, so the line appears exactly once and never alongside
"no `.tsuku.toml` found".

Activation treats a refusal and a parse failure as one case: stdout adds nothing
to `PATH`, the refused file's directory goes into the existing tracking variable,
and the report fires when the refused file differs from the previous prompt's or
when the refusal changes `PATH`. That second term covers a config that becomes
refused in place while its tools are active, which directory comparison alone
cannot see. No new tracking variable is introduced.

A typed error is the only representation that fails closed: every caller already
treats a non-nil load error as "no usable config", so none can act on a refused
file, and neither can one added later.

#### Alternatives Considered

- **A variant of the existing parse error.** Every renderer would still have to
  tell the two apart to avoid reporting a file as unparseable when it was never
  read, and `tsuku install` would exit as it does for a syntax error.
- **A refused state on the result type.** Two callers dereference the config
  immediately after the nil check, so this panics; with an empty config instead
  they print "No tools declared" on stdout and exit 0.
- **The existing diagnostics channel.** It is written straight to stderr with no
  quiet-flag check and sits outside the entry gate, so a refusal would reprint on
  every prompt anywhere below the refused file.
- **A second discovery entry point returning found, absent or refused.** The lint
  that keeps loading centralized matches the existing function's name, so a new
  entry point is invisible to it while existing callers keep compiling against
  the old one.
- **A new tracking variable for the refused path.** It records what the existing
  directory variable already holds, at the cost of an export on every prompt.
- **Printing at each call site, or from inside the library.** The first reverses
  the property the shared helper exists for; the second puts gating decisions
  (the quiet flag, the entry check) in the wrong layer.

## Security Considerations

A `.tsuku.toml` is attacker-controlled by construction in this threat model, and
a recipe served by a source a project named becomes attacker-controlled once that
source is registered. Four dimensions apply.

**Handling the file itself.** The read discipline is the load-bearing part:
`lstat` the entry, open with `O_NOFOLLOW|O_NONBLOCK` (following the link only
after checking the link's own owner), compare device and inode against the
`lstat`, `fstat` the descriptor, require a regular file, decide, and only then
read the bytes from that same descriptor. The decision and the bytes therefore
concern one object, which a stat-then-open pair cannot guarantee; the same idiom
and reasoning already exist in `internal/actions/install_program_files.go`. Two
live denial-of-service paths close as a side effect. A named pipe at
`.tsuku.toml` currently hangs every shell prompt for anyone who can create a file
in an ancestor directory, because the stat succeeds and the read blocks; the
regular-file requirement and the non-blocking open end that. And a planted config
is currently read and decoded on every prompt, where a refusal now reads nothing.

One pre-existing exposure is not closed: nothing caps the byte size of a config
before it is read and decoded, and inside `$HOME` the rule returns early without
any check, so an oversized file in a cloned repository is read on every prompt.
`MaxTools` is a post-decode count and documented as not being a defense against a
large file. A byte cap is cheap and independent of the trust rule; it is recorded
here rather than folded in, because it belongs to the parse path.

**What the rule may read, and what it trusts.** Discovery reads metadata only:
the entry, the opened descriptor, the config's directory, the owners and modes of
that directory's ancestors when a third party's claim is in question, the kernel's
hardlink-protection setting when the directory is writable by others and not
sticky, and the resolution of `$HOME` and the ceilings. It writes nothing. The
trusted set is the invoking user and root, plus the config directory's owner when
no group- or other-writable ancestor above it belongs to anyone else. Three limits
are stated rather than defended: a symlink pointing out of the tree is judged on
its target's owner but not on its target's directory; the in-tree test is a path
comparison the descriptor cannot confirm, which is exploitable only by someone
already trusted; and a bind mount can present a parent chain that differs from the
real one, which is unreachable because such a mount is not visible in the victim's
namespace rather than because it cannot be created.

**Source trust and consent.** No consent input comes from the project file: the
approval decision takes flags, a terminal check and a prompt function, reads no
environment, and the file's unknown keys are dropped by the decoder with a
diagnostic. A detected CI environment grants nothing. The escalation rule takes
one input derived from the file — whether the declaration's key names a source —
and that input can only withhold the raise. The sufficient half comes from the
loader, which reflects the user's own configuration. Stated as an invariant,
because it is what keeps the rule honest under later edits: **a `.tsuku.toml` can
narrow consent and can never widen it.**

Two consent gaps remain open by decision and are recorded in the PRD's Known
Limitations: a blanket approval flag in CI approves whatever source a fork's pull
request adds, and a key with no source component still installs from an
already-registered source with no consent step. A third is the user's own choice:
setting auto mode globally bypasses the escalation narrowing entirely, which
matters because that setting is the remedy offered to anyone who relied on silent
installs from a source they registered. A per-source allow list is the recorded
follow-up for both the first and the third.

A dry run still contacts the network: resolving a preview builds a session
provider for each named source, which probes that source's repository. Because a
dry run deliberately asks nothing, that contact precedes any consent. A real run
is ordered differently: at a terminal an unregistered source is asked about
before anything is fetched, while an already-registered source, and any source on
a run with no terminal, is probed without a new question. The destination is
constrained to a known host by the source-name validation, so this is a signal to
the attacker that a specific machine ran the command rather than a general request
forgery.

**What the new output discloses.** The one stream a shell evaluates carries only
the exports, every value quoted per dialect by the existing shell-quoting helper,
including the tracking variable that now records an attacker-chosen directory
under a refusal. Refusals go to stderr as a single line with the path quoted, so
control characters in a hostile directory name render visibly instead of reaching
the terminal; owners are reported as numeric ids rather than resolved through the
name service, which would be a network call on the prompt path. Nothing from a
refused file's contents can appear anywhere, because the file is never read.

**Residual risks, in one place.** A config the invoking user owns but leaves
writable by a shared group can be rewritten in place with no change any ownership
check can see; the file-mode clause covers the world-writable case only, because a
group check would refuse ordinary repositories on distributions that use
user-private groups. A checkout owned by another user is applied when the user
works in it, and with root invoking, that means an unprivileged user chooses what
root installs. Ownership means nothing where the filesystem does not implement it
or where uids are supplied by a mount option. And a project file can still pin an
old version of a default-registry tool, which `tsuku run` installs and executes
without asking, because default-registry declarations deliberately stay silent.
