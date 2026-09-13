---
schema: design/v1
status: Proposed
problem: |
  A `.tsuku.toml` reaches three code paths that each treat it as a file the
  invoking user wrote. Discovery walks past the checkout and applies whatever it
  finds, so a config planted above an outside-home checkout applies to other
  users; project install writes a source the file names into the user's global
  configuration with no consent when no terminal is attached, and even under
  --dry-run; and tsuku run waives its install prompt for a declared tool whatever
  source the declaration names.
decision: |
  Discovery applies a config outside the resolved home only when the file it
  actually opens is owned by the invoking user, by root, or by the owner of the
  config's own directory with no group- or other-writable ancestor belonging to
  anyone else, refusing loudly with a typed error rather than skipping silently.
  Registering a source a project named requires an interactive yes or --yes, is
  written only after the install proceeds, and records how it was approved and
  which file asked. tsuku run then trusts what the registries hold, asking only
  about a source the user has not registered.
rationale: |
  Each layer's output is the next layer's input, and none of them reads the
  project file for permission, so a file can narrow what happens and never widen
  it. The discovery rule is the only mechanism that separates a foreign-owned
  checkout that must keep working from a directory another user pre-created in a
  shared space, because those differ not in ownership but in who could have
  created the path. Trusting registered sources keeps the fix proportionate: the
  alternative made the organization's own configurations prompt on every run
  forever, whose only remedy was to switch the protection off globally.
upstream: docs/prds/PRD-project-config-trust.md
user_visible_surface: true
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

The acceptance target is R1-R5 for which configs apply, R6-R10 for how a refusal
is reported, and the thirteen-row layout table in R2 for the cases that decide
the rule. The hard part of that table is that its two deciding rows are
ownership-identical: a checkout owned by a third party under an ordinary parent
must be found, and a directory another user pre-created under a world-writable
parent must be refused.

**Source registration.** Two callers reach `ensureDistributedSource`
(`cmd/tsuku/install_distributed.go:85`): the command-line argument path
(`cmd/tsuku/install.go:243`) and the project pre-scan
(`cmd/tsuku/install_project.go:107`). Both run before their `--dry-run` branch,
and inside, `!autoApprove && isInteractive()` means a missing terminal falls
through to `autoRegisterSource`, which saves `config.toml`. `--yes` and `--force`
are both passed as `autoApprove`. The project install's `Proceed? [Y/n]` comes
after the pre-scan, so declining it doesn't undo a registration. The acceptance
target is R11-R17: what a dry run may do, what counts as consent, when the entry
is written, what happens to the other tools when one source is unapproved, and
what the registration records. One fact from the tree shapes the answer: this
write is the only one any install path makes to `config.toml`, so gating it is
sufficient rather than merely necessary.

**Run escalation.** `autoinstall.Runner.Run` (`internal/autoinstall/run.go:175`)
calls `elevate(mode, origin, declaration != nil)`, which turns an unset default
into auto for any declared command. The declaration reduces every key to a bare
recipe name (`internal/project/declaration.go`), and the recipe is matched
against the binary index, whose `Source` is `"registry"` or `"installed"` keyed
by name rather than by where the recipe came from. The install then resolves the
bare name through the loader chain (local, embedded, central, then registered
distributed sources), so a source named in a key has never been able to supply a
recipe here: what the key controls is only whether the prompt is waived. The
acceptance target is R18-R21, which narrow that waiver to sources the user has
registered while leaving explicit modes, the mode-lowering gates, the
environment-variable restriction and the already-installed fast path alone.

**Cross-cutting.** R22 forbids any consent or trust input from `.tsuku.toml`;
R25 bounds what discovery may read on the shell-prompt path; R26 requires the
awkward cases — another user's files, root, a terminal — to be reachable in
tests without root or a second account, including from in-process tests of the
command package; R24 and R24a cover the documentation and the release note that
existing statements about discovery and consent now contradict.

## Decision Drivers

- **D1 — Refuse what the user didn't choose, without breaking outside-`$HOME`
  checkouts.** The layout table in R2 is the acceptance target for discovery:
  planted parents and squatted shared directories are refused for root and
  non-root users; user-, root- and foreign-uid-owned checkouts under an ordinary
  parent are found.
- **D2 — No change inside the user's home, and none for ordinary permissions.**
  Configs strictly below a resolved, user-owned home behave exactly as today
  (R1), and no unconditional clause may read the group bit, since a umask of 002
  makes group-writable files and directories ordinary.
- **D3 — Never silent, never on the evaluated stream.** Every refusal reaches
  stderr once in a form the user can act on (R6); the shell hook's stdout is
  evaluated by the shell.
- **D4 — A missing terminal, a dry run or a CI environment is never consent**
  for a change that outlives the command (R11, R12), while a deliberate flag and
  a prior registration still work for the scripts that rely on them.
- **D5 — Declarations the user has already accepted see no new prompt** (R18,
  R21), and the existing disclosure line is unchanged.
- **D6 — Hot-path cost.** The shell hook runs on every prompt; discovery may add
  metadata reads only on paths it already considers, no content reads before the
  decision, and no network calls (R25).
- **D7 — Testable without root or a second account** (R26), including
  in-process tests of the command package, following the existing
  `configPermissionCondition` pattern of passing the uid as a parameter.
- **D8 — Supported platforms are Linux and macOS.** `syscall.Stat_t` is
  available on both; the binary does not build for Windows today, so no clause
  may depend on a facility only one platform has.
- **D9 — One reviewable pull request** covering #2555, #2552 and #2559, built as
  a sequence of commits rather than separate releases.

## Considered Options

Five decisions, each evaluated against the drivers above.

### Decision 1: Which configs discovery applies outside the user's home

Two layouts do the work, and no rule that reads ownership alone can tell them
apart. A checkout at `/srv/app` owned by another non-root user, under a
root-owned parent, must be found: that is the container volume and the
copied-in-checkout case. A directory another user pre-created under a
world-writable parent, which the invoking user then works in, must be refused:
that is the reported attack reached one level lower. In both, the config and its
directory are owned by someone who is neither the invoking user nor root. What
separates them sits above the config and is a permission bit: only an
administrator could have created `/srv/app`, while anyone could have created
`/tmp/build`, so ownership of a name is evidence about who placed it only when
the namespace was not open to everyone.

#### Chosen: judge the owner of the file actually read, and ask who could have created the path

Below a resolved home directory nothing changes, as an early return before any of
this runs (D2). Outside it, three clauses decide, all judged on the file the walk
actually opens rather than on a path.

**Ownership.** Accept when the config's owner is the invoking user or root.
Otherwise accept only when every group- or other-writable directory on the chain
from the config's own directory up to the filesystem root is owned by that same
owner. A directory whose metadata cannot be read counts as writable, following
the existing treatment of "cannot determine" as failing in
`configPermissionCondition`. The chain is walked only when the config's owner is
a third party, which is what keeps the ordinary path free of it (D6) and what
lets this clause read the group bit where the two below cannot.

That single condition is what separates the two deciding layouts, which are
ownership-identical (D1): a checkout under an ordinary parent has nothing
writable above it belonging to anyone else, while a directory pre-created in a
shared space sits under a parent that is world-writable and owned by root rather
than by the squatter. An earlier form of this rule also required the config's
owner to *be* the owner of its own directory. That was redundant — the chain test
refuses both attack layouts on its own — and it refused a layout that occurs in
practice: files copied into a container image with an explicit owner, landing in
a directory the build created as root. Removing it also removes the asymmetry
this design otherwise had to warn about.

**The config's directory.** Refuse when it is world-writable and does not carry
the sticky bit. In such a directory an attacker can unlink the user's config and
put a hard link to another of the user's own files in its place, which every
ownership check then accepts; the sticky bit is what forbids the unlink. This
clause applies identically on both supported platforms. An earlier form gated it
on the Linux kernel setting that forbids the link half of that attack, which
would have had to define what an absent reading means: macOS has no such setting,
so failing open would disable the clause on the one platform with no protection
at all, and failing closed is the same as not gating. Ungated, it must read the
world bit only, because a group-writable directory is the ordinary result of a
umask of 002 (D2).

**The config's own mode.** Refuse when the file itself is world-writable, which
closes an in-place rewrite that leaves no trace in any metadata an ownership
check reads. This is the clause that refuses a checkout on a Windows drive
mounted without the metadata option, where the reported mode is translated from
Windows permissions; the refusal names the mount options that fix it as well as
`chmod`, because `chmod` alone cannot (D3).

The walk never stops on ownership, so every refusal names a file (D3). The file
is opened without following symlinks and without blocking, checked on the
descriptor, and read from that same descriptor, so the bytes parsed belong to the
object the decision was made on (R4).

#### Alternatives Considered

- **Strict ownership, as git does it.** Accept only what the invoking user owns,
  with root additionally accepting root-owned paths and the sudo invoker, plus an
  opt-out list. Rejected because it refuses the container volume, the
  foreign-owned checkout and the build running as root over a copied-in checkout,
  all of which must keep working; adopting it would have required promoting the
  opt-out from a constraint to a feature. Its open-and-check discipline was
  adopted wholesale.
- **Stop the walk where directory ownership changes.** Rejected because the
  squatted directory *is* the starting directory, so its owner is the attacker
  and the rule accepts the planted config; and because stopping early means the
  planted file is never examined, so nothing is reported, which the requirements
  forbid.
- **The same, plus refusing any writable directory between the start and the
  root.** The writability idea is right and survives as the qualifier above.
  Rejected as stated because an unconditional test refuses every temporary
  directory fixture and the functional suite, since the system temporary
  directory is world-writable.
- **Mode and structure only: refuse world-writable directories, stop at a
  repository root or a filesystem boundary.** Rejected because all three checks
  are silent on the squatted directory, and because the repository-root stop
  changes behavior inside the home directory for submodules and for a config
  placed above several checkouts.
- **Trust on first use, as direnv and mise do.** Rejected because the shell hook
  cannot prompt, so the first use in a container or a server checkout would refuse
  with no way to approve, and because it would prompt in every repository
  including the majority that name only default-registry tools.

### Decision 2: How a refused config is represented and reported

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

- **A variant of the existing parse error.** Rejected because every renderer
  would still have to tell the two apart to avoid reporting a file as unparseable
  when it was never read, and `tsuku install` would exit as it does for a syntax
  error.
- **A refused state on the result type.** Rejected because two callers dereference
  the config immediately after the nil check, so this panics; with an empty config
  instead they print "No tools declared" on stdout and exit 0.
- **The existing diagnostics channel.** Rejected because it is written straight to
  stderr with no quiet-flag check and sits outside the entry gate, so a refusal
  would reprint on every prompt anywhere below the refused file.
- **A second discovery entry point returning found, absent or refused.** Rejected
  because the lint that keeps loading centralized matches the existing function's
  name, so a new entry point is invisible to it while existing callers keep
  compiling against the old one.
- **A new tracking variable for the refused path.** Rejected because it records
  what the existing directory variable already holds, at the cost of an export on
  every prompt.
- **Printing at each call site, or from inside the library.** Rejected because the
  first reverses the property the shared helper exists for, and the second puts
  gating decisions (the quiet flag, the entry check) in the wrong layer.
- **Naming the resolved source in the existing disclosure line.** Considered as a
  lighter-weight substitute for changing the escalation rule at all: tell the user
  where the recipe came from instead of withholding the raise. Rejected because
  that line is printed whatever the consent mode, while the provenance read behind
  it was guarded on the unset default, so the same command would have described its
  source differently depending on a flag that has nothing to do with provenance.

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
one is the security block decision 2 assigns to a refused config, and one cannot
be told apart from an install failure.

#### Alternatives Considered

- **A mode or origin parameter on the existing function.** Rejected because one
  call cannot express "decide now, write after a later prompt" without returning a
  pending token, and the gap spans the tool list, the dry-run branch and the
  "Proceed?" gate. It also folds a five-way flag matrix into the one function whose
  non-terminal branch an existing test pins.
- **A separate project-mode entry point.** Rejected because it duplicates
  validation, the registration lookup and the strict refusal whose exact message a
  functional scenario asserts, and because the command-line path has to change
  anyway to stop writing under `--dry-run`.
- **Register during the pre-scan and roll back on decline or dry run.** Rejected
  because saving re-encodes the whole file from a residue-free decode, so a restore
  cannot be byte-identical and drops unknown keys; it races concurrent writers; and
  a dry run that writes and then undoes has still written.
- **Gate only the write with a boolean.** Rejected because the write is not the
  only thing that moves: the question relocates in the output, and per-source
  bookkeeping needs more than the existing per-tool failure flag can carry.
- **A real terminal pair in tests, with no production change.** Rejected because
  the requirement is that the terminal check itself be substitutable; and because
  each prompt builds its own buffered reader, scripted input loses its second
  answer whatever the fixture does.
- **A nested table or a coarse origin enum for provenance, or a side file.**
  Rejected because the nested table writes a bare empty header into every
  pre-existing entry on the first save; an enum records neither fact the
  requirement asks for; and a side file is a second source of truth that
  `tsuku registry remove` lets drift.

### Decision 4: Which flags consent to registering a source a project named

Two flags currently approve registering a source that only a project config
names. The question is whether both should.

#### Chosen: only `--yes`

`--force` keeps its other meanings: suppressing security warnings, and replacing
a tool already installed from a different source, including during a project
install. It no longer approves a registration. The reasoning is what the flags
mean rather than distrust of the person typing them. `--yes` answers consent
questions; `--force` forces an operation through; a permanent new entry in the
user's global configuration is not what someone forcing an install asked for.
Three facts supported it: `--force`'s own help text already claims it proceeds
without prompts, which is untrue on this path because it does not skip the
install confirmation; the design that introduced project-driven registration
propagates `--yes` alone; and nothing in the repository pairs `--force` with a
project-named registration.

#### Alternatives Considered

- **Keep both.** The consistency argument is real and was put to the user at full
  strength: a deliberate `--force` is as deliberate as a deliberate `--yes`, and
  the user had already decided that a deliberate `--yes` confers trust. Rejected
  because the two grounds originally offered for keeping it — the documented
  meaning, and script breakage — are both false here, and because it leaves two
  flags granting the same permanent write with only one of them documented for it.
- **Make `--force` imply `--yes` everywhere.** Rejected because it resolves the
  inconsistency by widening at the moment this work exists to narrow, and changes
  `--force` for every current user in the quiet direction: more things proceed
  without asking.

### Decision 5: When `tsuku run` installs a declared tool without asking

Today a declaration raises the consent mode on one input: that a declaration
exists. That input says nothing about where the install comes from, so a key
naming a source the user never approved waives the prompt exactly as a plain key
does.

#### Chosen: ask only about a source the user has not registered

`tsuku run` raises the unset default for a declared command unless the key names
a source absent from the user's configured registries. A key with no source
component always qualifies. A key naming a registered source qualifies, whichever
route registered it.

What makes the rule sufficient is that a recipe can only be resolved from the
default registry, the user's local recipes, or a registered source, so a
declaration can never cause an install from somewhere the user has not already
accepted. What it could do, and what the defect did, is name a source the user
never approved and get the tool installed anyway without being asked.

**What the rule does not do.** The predicate reads the declaration's winning
configuration key, and which key wins is a precedence decision: a config
declaring the same tool both plainly and with a source collapses to one
declaration carrying the plain key. So a file that names an unapproved source
*and nothing else* is caught, and a file that adds one plain line is not. The
attacker gains nothing by that — a plain key already installs silently, and the
source they named could never supply the recipe — but the protection should be
described accurately: it surfaces an unapproved source to someone reading an
honest repository, and it is not a barrier against a file written to evade it. It
does not need to be, because the named source cannot serve the install either
way.

**The output.** When the rule withholds the raise, the message names the source
the key names, says it is not registered, and states that recipes are resolved
through the user's own configured sources rather than the named one. That last
statement needs no lookup: the branch is only reached for a source outside the
registries, and the resolution chain is built from the registries plus the default
and local sources, so the named source is categorically not the origin. Naming the
specific registry that will supply the recipe would require reading provenance out
of the loader, which this rule deliberately does not do.

Two consequences are worth stating where an implementer will see them. The rule
needs no provenance input at all: what it evaluates is a membership test against
the configured registries, which the command layer already holds, so no loader
hook and no source accessor are needed to serve it. And because the registries
are the whole trust boundary, a run must not be able to enlarge them mid-flight.
That is a property to pin with a test rather than to assume — a `tsuku run` that
takes the auto raise must leave the configured registries exactly as it found
them.

#### Alternatives Considered

- **The conjunction: a plain key AND a recipe resolving from the default registry
  or local recipes.** Rejected because it breaks the ordinary case it was meant
  to leave alone. Every repository in this organization declares tools from the
  organization's own distributed source, one of them as `latest`, and a non-exact
  declaration never matches the already-installed shortcut
  (tsukumogami/tsuku#2571). Those declarations would prompt on every invocation
  forever and fail on every headless run, with no remedy short of enabling auto
  mode globally — a larger hammer that switches the protection off everywhere,
  and the one a user reaches for when the smaller control has no proportionate
  answer. Consenting to have a source available is then treated as consenting to
  unattended installs from it, which is the trade this decision accepts.
- **Admit sources added with `tsuku registry add` but not those approved with
  `--yes`.** A middle position. Rejected on the same evidence and for a second
  reason: the flag that would distinguish them is written only when true, so the
  check would read an absence and fall open, and making it reliable would have
  changed what `tsuku registry add` writes.
- **The binary index or the installed state as the provenance input.** Rejected
  because both misreport. The index's source column records which rebuild pass
  inserted a row, so a local recipe and a distributed one land in the same bucket;
  the installed state is never written on the run path and its migration records a
  distributed install as central.

## Decision Outcome

**Chosen:** an ownership-plus-writability trust decision on the file discovery
actually opens; a typed `RefusedError` printed once by the shared loader helper;
a project install that classifies sources up front and defers every
`config.toml` write to a single save after "Proceed?"; `--yes` as the only flag
that consents to that write; and an escalation predicate that is a membership
test against the user's configured registries.

### Summary

The five decisions answer one question at four different points, and they compose
in one direction: **a project file may narrow what happens, never widen it.**

Discovery decides which file is read at all. Outside the user's home it applies a
config only when the file it actually opens is owned by someone the user can be
held to have chosen, and it refuses loudly rather than skipping quietly, because a
config that vanishes without explanation is its own bug report. Inside a resolved
home directory an early return skips the whole decision.

Refusal reporting is the shared channel for all of it. `internal/project` gains a
`RefusedError` carrying the path, its directory, the reason and the remedy. The
shared loader helper prints one stderr line; `tsuku install` and `tsuku shim
install` exit 14; activation puts nothing on the stream the shell evaluates and
reports once per refused file rather than on every prompt.

Consent decides whether a source a file names may enter the user's configuration.
The single registration function splits into three primitives — classify, add a
session-only provider, write the entry — and the project install composes them
through a plan object: classify every unique source, ask about each unregistered
one after the tool list and before "Proceed?", then write all approved sources in
one save immediately after "Proceed?" and before any recipe fetch. Declining at
either prompt leaves `config.toml` untouched; a dry run classifies and writes
nothing whatever the terminal and flags; a source left unapproved has its tools
skipped and the command exits 16. Each project-caused entry records how it was
approved and the absolute path of the declaring config, as two optional string
fields beside the existing auto-registration flag. `--yes` is the flag that
answers the consent question; `--force` forces an operation through and no longer
acquires a registry entry on the way.

Escalation then trusts what consent produced. `tsuku run` asks only about a source
the user has not registered, so the configured registries are the trust boundary,
and the two fixes meet: getting a source registered requires agreement, and
anything registered is honored without asking again.

### Rationale

What makes the set coherent is that each layer's output is the next layer's input
and none of them reads the project file for permission. The file can say which
tool, and can withhold a silent install by naming an unapproved source. It cannot
say that a source is trusted, that a mode is auto, or that a directory is safe.

## Solution Architecture

### Overview

Three packages change, each at one entry point, plus two small surfaces for
recording and reporting. Discovery gains a decision and a typed refusal; the
install command gains a plan that separates checking a source from writing it;
the run path gains a membership test; the user configuration gains two fields; the
registry listing renders them.

### Components

**`internal/project` — discovery and the refusal type.** `LoadProjectConfig`
keeps its signature and its production behavior, with a second exported form
taking the environment described under Key Interfaces. Inside, the walk gains the
trust decision and the open-then-check read discipline. A new `RefusedError`
carries the refused path, its directory, the reason and the remedy, and is
returned in place of a config, with nothing parsed. `FindProjectDir`, which
collapses every error to an empty string, has no callers today; either it goes,
or it is documented as discarding a refusal without reporting it and unsuitable
for any path a user sees.

**`cmd/tsuku` — consent, reporting and exit codes.** `ensureDistributedSource` is
rebuilt from three primitives: classify a source (validate, look up, apply the
strict-registries refusal; no network, no write), add a session-only provider, and
write the entry. For a real install the command-line path composes them in
today's order, so its behavior is unchanged, including the registration it
performs with no terminal. Under a dry run it composes the first two and skips
the write, which means the dry-run branch moves above the source handling in that
command: today it sits below, which is why a dry run currently registers.

Project install gets a plan object holding, per source, its state, its declared
tools, its approval and its error; it classifies, asks after printing the tool
list and before the existing confirmation, and writes every approved source in one
save after the user proceeds and before any fetch. A source left unapproved gives
its tools a state of their own, distinct from installed and from failed, so the
summary can name them and the structured output can report them rather than
defaulting a skipped tool to success.

`loadProjectConfigReporting` prints the refusal line itself, because `tsuku run`
discards the load error. It prints unconditionally rather than through the
quiet-aware helper, because `tsuku install`'s exit code is meaningless without
the line; the shell hook and `tsuku shell` report through activation's own path,
where the quiet flag applies.

One exit code is new, for a source needing approval; the refused-config case
reuses the existing "blocked for security reasons" code rather than adding a
second name for the same number.

**`internal/autoinstall` — the escalation predicate.** The elevation gains one
input: whether the declaration's key names a source outside the configured
registries. The declaration already carries the configuration key, and the
membership test is supplied by the command layer as a predicate, so the package
takes no dependency on user configuration. Everything else in the consent chain
is untouched: explicitly set modes, the environment-variable restriction, the
mode-lowering gates and the already-installed fast path.

**`internal/userconfig` and the registry listing.** A registry entry gains two
optional fields recording how a project-caused registration was approved and the
resolved path of the declaring config, written once and never updated. Entries
written before this change load unchanged and list as they do today. The listing
keeps its first line byte-for-byte and adds an indented line where provenance
exists.

### Key Interfaces

The seams are the design, not an afterthought, because the cases that matter are
another user's files, root, and a terminal. Discovery's filesystem access is one
interface rather than a metadata accessor, because three acceptance criteria
assert things about the *open* and the *read* rather than about owners and modes:
that a file swapped between the check and the read is not parsed, that no config
content is read before the decision, and that nothing bypasses the seams. Its
shape:

- resolve a path through symlinks; read link metadata (owner, mode, type, device
  and inode); read a link's target; enumerate a directory's ancestors;
- open a path without following symlinks and without blocking, yielding a handle
  that can report its own metadata, be read, and be closed.

The ancestor enumerator matters more than it looks: without it a fixture built
under a temporary directory climbs into the real filesystem above it, which
disqualifies an injected third-party owner and refuses a "must be found" layout
for a reason that has nothing to do with the rule. A failure to read a path is
distinct from reaching the top: the first fails closed, the second is ordinary
termination.

Alongside it, three plain values: the invoking user's id, passed rather than read
from the process; the terminal check; and the registry-membership predicate the
command layer hands to the run path.

**How tests supply them.** The exported discovery entry point keeps its signature
and its production defaults, and a second exported form takes the environment
explicitly. Package-level variables would be reachable but would make the tests
order-dependent; an unexported parameter would be clean but unreachable from the
command package, which R26 requires, since the layouts involving another user's
files have to be driven through `tsuku install` and the shell hook as well as
through discovery directly. The lint that keeps loading centralized matches the
exported name, so it must be extended to match both forms, or the second one is
invisible to it.

The terminal check is not yet substitutable: it is a plain function with several
call sites, and the replaceable variable this design first pointed at belongs to
a different package. Converting it to a package-level variable in the command
package is part of the work, and doing it once covers every existing prompt as
well as the new one.

### Data Flow

Discovery walks from the working directory. At each directory it tests the
resolved ceilings, then the entry: absent, continue; present, decide. The decision
reads the link, opens the file without following symlinks and without blocking,
compares device and inode, checks the descriptor, and either reads the bytes from
that same descriptor or returns a refusal that stops the walk. Callers receive a
config, nothing, or a refusal; the refusal reaches stderr once, through the shared
helper for commands and through activation's existing unusable-config path for the
shell hook.

Project install parses the file, classifies each named source, prints the tool
list, asks about each unregistered source, asks the existing confirmation, then
writes the approved entries in one save and installs. A source without approval
skips its own tools and the rest install; the command exits with the
needs-approval code.

`tsuku run` resolves the command, narrows to what the project declared, and
decides the mode: raise unless the key names a source outside the registries.
From there the existing gates, the terminal check and the dispatch are unchanged.

## Implementation Approach

The decomposition is vertical: one slice per defect, discovery first, because it
is the layer the other two sit on and the only one with a new decision procedure
to get right. Within the discovery slice, phases 1 and 2 are a walking skeleton —
the decision and its type, then the reporting across five callers — so the rule
can be tested before anything renders it.

The unlock edges are worth stating, because two of them are not obvious. Phase 2's
reporting needs phase 1's refusal type, so the type ships in phase 1 even though
nothing renders it yet. Phase 4's provenance fields need phase 3's write
primitive, since that is the only place an entry is written. Phase 5's membership
predicate needs phase 3's classification, which is where a source's registration
state is already computed.

These are six commits in one pull request (D9), not six releases. Between phase 1
and phase 2 a refusal has a type but no renderer, so activation would surface it
through its generic error path; that window exists inside the branch and never in
a shipped version.

### Phase 1: discovery seams, the trust decision, and the refusal type

The decision unit, the filesystem and identity seams, the open-then-check read
discipline, and the `RefusedError` type itself. The ceiling and home resolution
comes here too: the rule turns on "below a resolved home", which is the same
computation the ceiling set performs unresolved today, and splitting them would
leave two notions of home in one file for a phase. Deliverables: the decision and
its unit tests over the layout table, including the cases needing a foreign owner
and a root invoker; the read discipline, which also refuses anything that is not a
regular file; the type and its message construction.

### Phase 2: refusal reporting across the five callers

The shared helper printing the line, the two commands that fail on it, and
activation's unusable-config branch. Three renderers generalize from the
parse-failure case rather than gaining parallel ones: the diagnostic function, the
report helper, and the two branch conditions in the shell hook and `tsuku shell`.
`tsuku shell` is the one most easily missed — it has its own branch, and without
the generalization a refusal falls through to a usage block and a non-zero exit,
breaking its requirement to exit 0. Deliverables: one stderr line with the path
quoted; the shell hook reporting once per refused file; `tsuku shell` reporting on
every invocation; `tsuku run` reporting and continuing; the two commands exiting
on it.

### Phase 3: consent primitives and the project source plan

The split of the registration path, the plan object, the deferred write, the
needs-approval exit code, and the dry-run behavior for both callers. Deliverables:
the three primitives with the command-line path recomposed in today's order for a
real install and the dry-run branch moved above the source handling; the plan; the
messages; the shared line reader; the terminal check converted to a substitutable
value in the command package, which covers the existing prompts as well as the new
one; the skipped-source state in both the summary and the structured output.

### Phase 4: the provenance record

The two configuration fields, written once at registration, and the listing that
shows them. Deliverables: the fields; the writer; the rendering; round-trip tests
including a configuration file written by the previous version.

### Phase 5: the escalation predicate

The membership test, its wiring from the command layer, and the prompt and
no-terminal messages that name the unregistered source. Deliverables: the
predicate and its unit tests asserting the decision rather than the prompt; the
test pinning that a run adds no provider; the messages.

### Phase 6: documentation

The design record's stale mitigation claim, the user-facing skill, the two
exit-code tables, the release note recording that registered sources are trusted
from the upgrade onward, and the `--force` flag description, which claims it
proceeds without prompts — untrue before this change, since it never skipped the
install confirmation, and further untrue after it.

## Security Considerations

A `.tsuku.toml` is attacker-controlled by construction in this threat model, and
a recipe served by a source a project named becomes attacker-controlled once that
source is registered. Four dimensions apply.

**Handling the file itself.** The read discipline is the load-bearing part, and
its security claim is that the decision and the bytes concern one object, which a
stat-then-open pair cannot guarantee. The sequence is in Data Flow; the same idiom
and its reasoning already exist in `internal/actions/install_program_files.go`.
Two live denial-of-service paths close as a side effect, described under
Consequences.

A symlinked config needs the sequence spelled out, because opening without
following fails on a link rather than succeeding. On that failure: read the link,
judge the link's own owner, resolve the target, judge the target, then open the
*target* without following and compare device and inode against the target's own
metadata before reading. A chain of more than one link is refused rather than
walked. "In the tree" means the resolved target lies under the directory holding
the link: for a target inside it the directory clauses apply to the link's
directory, and for a target outside it only the ownership test applies, so a
target's own directory is never examined. That is a real limit, stated rather
than implied.

One pre-existing exposure is not closed: nothing caps the byte size of a config
before it is read and decoded, and inside `$HOME` the rule returns early without
any check, so an oversized file in a cloned repository is read on every prompt.
`MaxTools` is a post-decode count and documented as not being a defense against a
large file. A byte cap is cheap and independent of the trust rule; it is recorded
here rather than folded in, because it belongs to the parse path.

**What the rule may read, and what it trusts.** Discovery reads metadata only:
the entry, the opened descriptor, the config's directory, the owners and modes of
that directory's ancestors when a third party's claim is in question, and the
resolution of `$HOME` and the ceilings. It reads no kernel settings, which is what
lets the sticky-bit clause behave identically on both supported platforms. It
writes nothing. The
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
and that input can only withhold the raise. The sufficient half is membership in
the user's configured registries, which is user state and cannot be written by a
project file. Stated as an invariant, because it is what keeps the rule honest
under later edits: **a `.tsuku.toml` can narrow consent and can never widen it.**

That membership test reads the configured registries, not the loader's list of
live providers. The two look interchangeable and are not: providers are built at
startup under a shared timeout and a failure only warns, so the provider list
would make a registered source read as unregistered whenever its repository is
briefly unreachable, and consent would depend on network conditions. The
comparison is against the configured string exactly; a spelling variant reads as
unregistered and prompts, which fails safe and is recorded as a usability
limitation rather than a security one.

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
dry run deliberately asks nothing, that contact precedes any consent, and it is
the one unconsented outbound request the feature leaves. A real run is ordered
differently: a registered source is probed without a new question, because the
provider chain is built from the user's configuration on every command; an
unregistered source is probed only once it has been approved. On a run with no
terminal an unapproved source is neither probed nor written — its tools are
skipped and the command exits. The destination is constrained to a known host by
the source-name validation, so this is a signal to the attacker that a specific
machine ran the command rather than a general request forgery.

**What the new output discloses.** The one stream a shell evaluates carries only
the exports, every value quoted per dialect by the existing shell-quoting helper,
including the tracking variable that now records an attacker-chosen directory
under a refusal. Refusals go to stderr as a single line with the path quoted, so
control characters in a hostile directory name render visibly instead of reaching
the terminal; owners are reported as numeric ids rather than resolved through the
name service, which would be a network call on the prompt path. Nothing from a
refused file's contents can appear anywhere, because the file is never read.

**The largest residual is the standing grant**, and it belongs here rather than
only under Consequences: every source already in the user's registries is trusted
from the upgrade onward, including any the registration defect placed there with
nobody asked. It is the one residual where the thing being trusted was put there
by the defect this work closes. It is nonetheless the right call, for a reason
stronger than convenience: before this change *every* declaration raised the
consent mode, so a silently registered source was already serving silent installs
for any plain name it carried. The upgrade extends no reach; it declines to
retract one. The release note names the annotated entries so the set that can be
wrong is reviewable in one command.

**The approval record is advisory.** It lives in the same file as the entries it
describes, so anyone who can write that file can fabricate it. Nothing reads it to
make a decision; it exists to audit an honest history. There is defense in depth
here by accident rather than design: the permission gate on that file lowers a
raised consent mode whenever it is writable beyond its owner, so the common way to
acquire that write also disarms the raise.

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

## Consequences

### Positive

- A file the user did not write can no longer choose where their tools come from.
  Discovery refuses what they cannot be held to have chosen, registration needs
  their agreement, and the run path asks about any source they have not approved.
- Two denial-of-service paths on the shell-prompt path close as a side effect: a
  named pipe at the config path currently hangs every prompt, and a planted config
  is currently read and decoded on every prompt where it will now be refused
  unread.
- A refusal is legible. One line naming the file, the reason and a remedy that
  works, including on mounts where the obvious remedy does not.
- Registrations become auditable: how a source was approved and which file asked
  for it, visible in the listing.
- Roughly thirty activation tests currently fence the walk with an environment
  variable so a real config above the temporary directory cannot be found. They
  become hermetic without the fence.
- Dry run gains the property its name promises: it writes no configuration at all.

### Negative

- The discovery rule is intricate for its size, and its central asymmetry is
  invisible without a comment: the ancestor test must compare against the narrow
  trusted set, never the one including the config's own owner, and it must never
  exempt a sticky ancestor. Both are easy mistakes — each looks like a
  simplification and each re-admits the squatting layout.
- A Windows-drive checkout mounted without metadata is refused until the user sets
  a mount option, because its reported permissions are translated from Windows
  and grant write to others.
- Scripts gain one new exit code to know about, 16 for a source left unapproved,
  and see the existing security-block code, 14, on a refused config. A project
  install that relied on `--force` alone to register a source now fails, loudly,
  naming the one-flag fix.
- Registering a source is a standing decision: any project may then declare tools
  from it without being asked again.
- Every source already registered is trusted from the upgrade onward, including
  any the defect registered silently. This is deliberate and stated in the release
  notes.
- Prompt-path cost rises by one metadata read on the ordinary path, and by an
  ancestor scan only when a third party's claim is in question.

### Mitigations

- The comment at the decision site names the squatting layout as the regression to
  re-derive before either clause is changed, and the test table covers every
  layout in the requirement rather than a sample.
- The refusal message names the mount options as well as `chmod`, so the
  instruction works wherever the user reads it.
- The needs-approval message names both ways to approve, and the refused-config
  message names a remedy rather than only a reason.
- A per-source allow list scoped to a project is the recorded follow-up for
  anyone who wants registration to be narrower than a standing decision.
- The release note points at the registry listing so the standing grant can be
  reviewed in one command.
- The seams that make the rule testable also make its cost measurable: the
  counting test asserts which paths are examined, and pins that no config content
  is read before the decision.
