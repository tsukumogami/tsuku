---
schema: prd/v1
status: Accepted
problem: |
  Since v0.14.0 a `.tsuku.toml` decides whether `tsuku run` installs a declared
  tool without asking and which recipe sources `tsuku install` adds to the
  user's global configuration, yet tsuku applies any file it finds as though
  the invoking user wrote it. A file planted above a checkout outside `$HOME`
  applies to other users, a project-named source is registered with no consent
  in CI and even under `--dry-run`, and `tsuku run` skips its prompt whatever
  source a declaration names (tsukumogami/tsuku#2555, #2552, #2559).
goals: |
  A project file changes a user's tools only in ways that user would agree to.
  Tools declared from the default registry behave exactly as today. A config
  tsuku refuses, or a source it needs approval for, is reported on stderr
  rather than acted on or dropped silently, and neither a missing terminal nor
  a dry run lets a project file make a lasting change.
absorbed:
  - docs/briefs/BRIEF-project-config-trust.md
---

# PRD: Project Config Trust

## Status

Accepted

Absorbed [BRIEF: Project Config Trust](docs/briefs/BRIEF-project-config-trust.md); carried in Absorbed Brief.

## Absorbed Brief

**Problem.** Since v0.14.0 a `.tsuku.toml` decides whether `tsuku run` installs a
declared tool without asking and which recipe sources `tsuku install` registers
globally, and tsuku applies whatever file it finds as though the user wrote it. A
file planted above a checkout outside `$HOME`, or shipped in a cloned repository,
can choose where a user's tools come from, and the user is never asked. Refusing
such files quietly is only half a fix, because a config tsuku ignores without
saying so looks like tools that stopped working for no reason.

**Outcome.** A developer working in a cloned repository, a shared host's scratch
directory, or a container volume can trust that a project file changes their
tools only in ways they would agree to. Default-registry tools behave as today
with no new question, and checkouts outside `$HOME` still find their own config.
Anything tsuku declines or needs approval for is reported on stderr, naming the
file and the source, and neither a missing terminal nor a dry run lets a project
file make a lasting change.

**Journeys and boundary.** Five journeys framed the feature and are carried as
User Stories below: a developer on a shared host with a config planted above
them; a developer evaluating an unfamiliar repository; a CI pipeline with nobody
to ask; a developer running a declared command; and a container user with a
checkout outside `$HOME`. The last pulls against the first, since a file the
developer didn't create can be legitimate on a volume and hostile in `/tmp`,
which is why R2 is stated as a table of layouts. The scope boundary is carried in
Requirements and Out of Scope. The framing builds on
`docs/designs/current/DESIGN-org-scoped-project-config.md` (config-driven source
registration and the trust boundary it shifted),
`docs/designs/current/DESIGN-shell-env-activation.md` (the activation design
carrying the ceiling-variable claim), and
`docs/designs/current/DESIGN-autoinstall-mode-resolution.md` (the consent-mode
model `tsuku run` uses).

## Problem Statement

Before v0.14.0 a `.tsuku.toml` could only activate tools the user had already
installed. Since v0.14.0 it also decides whether `tsuku run` installs a declared
tool without prompting, and it names org-scoped recipe sources that `tsuku
install` writes into the user's global `config.toml`. tsuku applies whichever
file it finds as though the person running the command had written it, and
nothing checks that assumption.

Three defects follow, each reproduced against v0.14.0 (commit 32f937e4):

- **Discovery reaches files the user never chose (#2555).** Discovery walks up
  from the working directory and stops only at `$HOME`, an opt-in
  `TSUKU_CEILING_PATHS` entry, or `/`. A checkout outside `$HOME` never passes
  through `$HOME`, so the walk continues to `/`. From `/tmp/shared-work`,
  `tsuku shell` applied a `/tmp/.tsuku.toml` another account could have
  written. The design record describes the ceiling variable as a mitigation in
  force when it is unset by default. The `$HOME` stop is also weaker than it
  looks: the start directory is resolved through symlinks but `$HOME` and the
  ceiling entries are not, so when `$HOME` or a ceiling is given as a symlinked
  path, the walk passes it and a config above it is applied (reproduced in a
  unit test).
- **A project file registers a permanent source with no consent (#2552).**
  With no terminal attached, `tsuku install` and `tsuku install --dry-run` in a
  directory whose config names `"owner/repo:tool"` both wrote
  `[registries."owner/repo"]` into `config.toml` and exited 6. `tsuku install
  --dry-run owner/repo:tool`, with the source named on the command line, did the
  same. The entry is consulted by every later resolution for every project, and
  nothing records which file asked for it.
- **`tsuku run` skips its prompt whatever the declared source (#2559).** A key
  naming `evil-owner/evil-repo:<tool>` raised the unset default consent mode to
  auto, and with no terminal the run installed and executed a recipe with no
  prompt. The recipe came from the default registry, so the key's source was
  ignored rather than honored, but the declaration still decided that nobody
  would be asked. Once a source has been registered and a recipe installed from
  it, a plain declaration can install from that source the same way.

The shared cause is that a file's presence stands in for the user's consent: a
config found above the checkout is taken as the user's own, and a missing
terminal is taken as a yes. Refusing files quietly is not a fix on its own,
because a config that tsuku ignores without saying so looks like tools that
stopped working for no reason.

## Goals

- A developer can work in a cloned repository, a shared host's scratch space, or
  a container volume and trust that a project file changes their tools only in
  ways they would agree to.
- Repositories that declare tools from the default registry, and checkouts that
  live outside `$HOME` such as `/srv/app` or a mounted volume, keep working with
  no new question.
- Whenever tsuku declines a file or needs a yes before using a source a file
  named, the user is told on stderr which file and which source, why, and what
  they can do.
- A project file cannot cause a lasting change, such as a new entry in
  `config.toml`, because a terminal was missing or because the command was a dry
  run.

## User Stories

- As a **developer on a shared Linux host** working in `/tmp/shared-work`, where
  another account left `/tmp/.tsuku.toml`, I want tsuku to ignore that file and
  tell me it did, so that another user can't reshape my `PATH`.
- As a **developer evaluating an unfamiliar repository** whose `.tsuku.toml`
  declares a tool from `someorg/recipes`, I want `tsuku install --dry-run` to
  tell me the source would need approval without writing anything, and `tsuku
  install` to ask me about `someorg/recipes` by name, so that I decide which
  sources my machine trusts.
- As a **maintainer whose CI job, script or agent runs `tsuku install` with no
  terminal** in a repo that declares an org-scoped source, I want the run to
  install what it can, fail naming the source, the declaring file and how to
  approve it, and leave `config.toml` alone, so that approval is an explicit,
  visible step rather than a side effect.
- As a **developer running a declared command** through `tsuku run` or the
  command-not-found hook, I want default-registry tools to install as they do
  today and any other source to be put to me as a question naming where the
  install comes from, so that a repository can't waive the prompt for a source
  I never approved.
- As a **developer building in a container** whose checkout is a volume at
  `/srv/app`, possibly owned by another uid or by root, I want the project's own
  `.tsuku.toml` found as it is today, so that the fix doesn't break the ordinary
  container layout.
- As an **image author running `tsuku install` as root in a Dockerfile `RUN`**
  over a checkout copied in with a non-root owner, I want the checkout's config
  found, so that builds keep working.
- As a **developer whose layout worked on v0.14.0 and is now refused**, I want
  the refusal to say which file, why, and what I can change, so that I'm not
  left guessing why my tools stopped activating.
- As a **user auditing my configuration**, I want `tsuku registry list` to show
  which sources a project caused to be registered and which file asked, so that
  I can tell a source I added from one a repository added.

## Requirements

Terms used below:

- **Default registry:** the central recipe registry and the recipes embedded in
  the tsuku binary.
- **Local recipes:** recipes the user placed under `$TSUKU_HOME/recipes`.
- **Bare key:** a `[tools]` key with no `owner/repo` source component
  (`jq`, not `owner/repo:jq`).
- **Project-named source:** an `owner/repo` source that reaches `tsuku install`
  only through a `.tsuku.toml` key, that is, `tsuku install` with no tool
  arguments. A source given as a command-line argument is **command-line-named**.
- **Resolved `$HOME`:** `$HOME` resolved through symlinks, when it resolves, is
  not `/`, and is owned by the invoking user. When `$HOME` fails any of these,
  there is no resolved `$HOME`: it does not stop the walk, and every path counts
  as outside it.

### Functional: discovery (#2555)

- **R1.** A `.tsuku.toml` in a directory strictly below the resolved `$HOME` is
  found and applied exactly as today, including when it is a symlink. As today,
  the walk stops at the resolved `$HOME` itself without reading a config there.
  The checks in R2 through R4 apply only outside the resolved `$HOME`.
- **R2.** Outside the resolved `$HOME`, discovery applies a config only when the
  invoking user can be held to have chosen it. The rule that decides this is
  chosen in the design, and must produce the outcomes in this table. "Owner" is
  the file's owner; the parent column describes the directory the checkout sits
  in.

  | # | Layout | Invoking user | Outcome |
  |---|--------|---------------|---------|
  | L1 | `/srv/app` owned by the invoking user, config at its root, parent `/srv` root-owned 0755 | non-root | found |
  | L2 | `/srv/app` owned by root, parent root-owned 0755 (for example a volume in a container) | non-root | found |
  | L3 | `/srv/app` owned by a different non-root uid, parent root-owned 0755, on a host or in a container | non-root | found |
  | L4 | `/srv/app` owned by a non-root uid, parent root-owned 0755 (for example Dockerfile `RUN` over a `--chown` copy) | root | found |
  | L5 | Any of L1-L4 with the config at the checkout root and the working directory several levels below it | as in L1-L4 | found |
  | L6 | `/tmp/.tsuku.toml` owned by another user, working directory `/tmp/shared-work`, `/tmp` root-owned 1777 | non-root | refused |
  | L7 | As L6 | root | refused |
  | L8 | `/tmp/build` pre-created by another user with a config inside, then entered by the invoking user, `/tmp` root-owned 1777 | non-root or root | refused |
  | L9 | A `.tsuku.toml` symlink, or its target, owned by a uid the rule would not accept for a regular file at the link's location | any | refused |
  | L10 | A `.tsuku.toml` symlink and its target both owned by a uid the rule accepts for that location (for example inside L3's checkout) | any | found |

  The table is the minimum. For a layout it doesn't list, the design states the
  outcome. If the design finds that no rule can give two listed layouts their
  required outcomes, the refusal wins, the design records the conflict and the
  changed row, and this PRD is amended before the plan.
- **R3.** When discovery refuses a config, the walk stops there. It does not
  continue to directories above the refused file, and the refused file's
  contents are not parsed, so a refused file that would also fail to parse
  produces only the refusal.
- **R4.** For a symlinked `.tsuku.toml`, the decision covers both the link and
  the file whose bytes are parsed, and is made on the file that is actually read,
  so a file swapped between the check and the read cannot be parsed unchecked.
- **R5.** `$HOME` and every `TSUKU_CEILING_PATHS` entry are compared against the
  walk on symlink-resolved paths, as the start directory already is. A ceiling
  entry that cannot be resolved is compared as written, as today. A directory the
  walk cannot examine ends the walk with no config found and no message, as
  today; nothing was found, so there is nothing to refuse.

### Functional: refusal reporting

- **R6.** A refused config produces exactly one line on stderr naming the file
  (quoted, since the path may be attacker-chosen), the reason, and what the user
  can do about it. Nothing about the refusal is written to stdout by any command.
- **R7.** `tsuku hook-env` exits 0 under a refused config and handles it the way
  it handles a config that fails to parse today: its stdout adds nothing to
  `PATH` and records the refused file's directory in the tracking variables, so
  later prompts know it was reported. Entering a refused location from an active
  project deactivates that project, restoring the `PATH` it had before; that is
  the only `PATH` change. The refusal prints on the first prompt after the
  refused file changes from the previous prompt's (including after leaving and
  re-entering, and on the prompt that deactivates a project), and not on later
  prompts while the same file is refused, including after moving to a
  subdirectory. `--quiet` suppresses the line for `tsuku hook-env` only.
- **R8.** `tsuku shell` exits 0 under a refused config, prints the refusal on
  every invocation, and its stdout adds nothing to `PATH`.
- **R9.** `tsuku run` prints the refusal on every invocation and then behaves as
  though no config were present.
- **R10.** `tsuku install` and `tsuku shim install` with no arguments fail with a
  non-zero exit and the refusal, and do not report that no `.tsuku.toml` exists.

### Functional: source registration (#2552)

- **R11.** `tsuku install --dry-run` writes no configuration and does not create
  `config.toml`, regardless of terminal, `--yes`, or `--force`, and for project-
  and command-line-named sources alike. It never prompts for source
  registration. On stderr it lists each source that is not already registered,
  with the declaring file for project-named sources.
- **R12.** A project-named source that is not already registered is registered
  only with explicit consent: a yes at an interactive prompt, `--yes`, or
  `--force`. A source already present in `config.toml`, however it got there,
  needs no new consent. The absence of a terminal is never consent, and neither
  is a detected CI environment.
- **R13.** The interactive prompt for a project-named source names the source and
  the declaring file, and is asked before the existing "Proceed?" confirmation.
- **R14.** Registration is written only once the install proceeds. Declining at
  any prompt in a project install, whether the source prompt or the "Proceed?"
  confirmation, leaves `config.toml` unchanged.
- **R15.** When a project-named source is declined, or has no consent and no
  terminal, the tools declared from that source are skipped and every other
  declared tool still installs. The command then exits non-zero and, on stderr,
  names each skipped source, its declaring file, the skipped tools, and the two
  ways to approve it: `tsuku install --yes` and `tsuku registry add <source>`.
  The exit code distinguishes "needs approval" from an install failure; the
  design names it. Nothing is written to `config.toml` for a skipped source.
- **R16.** A project-caused registration records how it was approved (one of an
  interactive yes, `--yes`, or `--force`) and the resolved absolute path of the
  declaring config, written once at registration and never updated by later
  installs. It keeps `auto_registered = true` and the "(auto-registered)"
  annotation. `tsuku registry list` shows the approval and the path. Entries
  written before this change keep loading and keep their annotation, and are
  listed without an approval record.
- **R17.** `strict_registries` and `tsuku registry add` behave exactly as today.
  A non-dry-run `tsuku install owner/repo:tool` with a command-line-named source
  behaves exactly as today, including registering the source with
  `auto_registered = true` and printing "Auto-registered source" when no
  terminal is attached.

### Functional: `tsuku run` escalation (#2559)

- **R18.** `tsuku run` raises the unset default consent mode to auto for a
  declared command only when both hold: the declaration's key is bare, and the
  recipe the run would install comes from the default registry or the local
  recipes. A recipe from any registered distributed source does not qualify,
  whether the source was added with `tsuku registry add` or registered by a
  project. In every other case the command gets the mode it would have had if
  undeclared.
- **R19.** When a declared command is not raised because of its source, the
  prompt, and the message shown when no terminal is attached, name the source the
  install would actually come from. When the declaration's key names a different
  source from that one, the output says the named source is not what `tsuku run`
  installs.
- **R20.** The consent mode reaches the install as today's resolution and
  mode-lowering gates produce it, including the rule that
  `TSUKU_AUTO_INSTALL_MODE=auto` counts only when `config.toml` also says auto;
  this change removes only the declaration-caused raise for commands that don't
  meet R18. Running an already-installed declared version without consent (the
  existing fast path) is unchanged.
- **R21.** A declared command with a bare key whose recipe comes from the default
  registry or the local recipes behaves exactly as today: no new prompt, and the
  same disclosure line.

### Functional: cross-cutting

- **R22.** No consent or trust input is read from `.tsuku.toml`: not the set of
  approved sources and not the consent mode. A detected CI environment (for
  example `CI=true`) never grants consent or raises the consent mode.
- **R23.** `tsuku run`, the command-not-found path, and shell activation never
  register a source.
- **R24.** The documentation describes the new behavior:
  `docs/designs/current/DESIGN-shell-env-activation.md` states that
  `TSUKU_CEILING_PATHS` is opt-in and unset by default and no longer claims it
  prevents traversal; the `tsuku-user` skill under `plugins/` describes the
  discovery rule, the refusal message, source consent for project installs, and
  the narrowed escalation.

### Non-functional

- **R25.** Discovery's added checks read only file metadata (the walk's
  directories, a config's link and target, and the resolution of `$HOME` and
  ceiling entries), read no config contents before the decision, and make no
  network calls, since `tsuku hook-env` runs on every shell prompt.
- **R26.** Tests can substitute the invoking uid, each path's owner and mode, the
  filesystem access discovery performs, and the terminal check. Every acceptance
  criterion below that involves another user's files, root, or a terminal runs
  through these seams in `go test -short` as a non-root user with no second
  account, including in-process tests of `cmd/tsuku` commands, since a
  functional scenario can only create files owned by the invoking user.

### Design constraint

- **R27.** If a user-side opt-out from the discovery rule is ever added, it is
  read only from the user's own configuration or environment, never from
  `.tsuku.toml`.

## Acceptance Criteria

Discovery and reporting:

- [ ] A unit test builds each R2 layout under `t.TempDir()`, reproducing the
  parent's owner and mode (a 1777 stand-in for `/tmp`, a root-owned 0755 stand-in
  for `/srv`) through the R26 seams, and asserts the discovery outcome, not only a
  message: every "found" row loads its config and every "refused" row returns a
  refusal, with any row changed under R2's conflict clause asserted as amended
  (R2, R26).
- [ ] A test with `HOME` pointed at a directory that is not an ancestor of the
  project asserts that a config at the root of a checkout outside `$HOME` is
  found (R2).
- [ ] Manual check, recorded in the PR: the #2555 reproduction with a second
  account no longer applies `/tmp/.tsuku.toml` from `/tmp/shared-work`, and
  stderr names the refused file (R2, R6).
- [ ] A test places a refused config below a second, acceptable config and
  asserts that neither is applied; a refused config with invalid TOML produces
  the refusal and no parse diagnostic (R3).
- [ ] Tests cover the L9 symlink cases (foreign-owned link, foreign-owned target)
  and the L10 case, and a test swaps the file between the check and the read
  through a seam and asserts the swapped file is not parsed (R4).
- [ ] Tests with `HOME` set to a symlinked path, and with a symlinked
  `TSUKU_CEILING_PATHS` entry, place a config above the real directory and assert
  it is not found. With `HOME=/`, with `HOME` unset, and with `HOME` owned by
  another uid through the R26 seams, the L6 layout is refused (R1, R5).
- [ ] A `TSUKU_CEILING_PATHS` entry naming a directory that doesn't exist is
  ignored without error, and a walk that reaches a directory it cannot examine
  (mode 000) returns no config and prints nothing (R5).
- [ ] A config directly in the resolved `$HOME` is not applied, and a config at
  `$HOME/projects/x` is, as today (R1).
- [ ] Existing in-`$HOME` discovery tests and the `project-config.feature`
  scenarios pass unchanged (R1).
- [ ] A test with a directory name containing a newline, `$`, a backtick and an
  escape sequence asserts the refusal is exactly one stderr line with the path
  quoted and no raw control characters, that the line states a reason and an
  action the user can take, and that `tsuku run`, `tsuku install`, `tsuku shell`
  and `tsuku hook-env` write no refusal text to stdout (R6).
- [ ] Under a refused config, `tsuku hook-env` exits 0 and its stdout adds
  nothing to `PATH`. The test feeds each call's exported tracking variables into
  the next call's environment and asserts: the refusal prints on the first call,
  not on a second call in the same directory, not on a third call from a
  subdirectory, and again after a call from an unrelated directory. With
  `--quiet`, no call prints it (R7).
- [ ] Starting from an activated project, a `tsuku hook-env` call from a refused
  location restores the previous `PATH` and prints the refusal, and the next call
  from the same location prints nothing (R7).
- [ ] Under a refused config, `tsuku shell` exits 0, prints the refusal on each
  of two invocations, and its stdout adds nothing to `PATH` (R8).
- [ ] Under a refused config, `tsuku run` prints the refusal on stderr and
  resolves the command as it would with no config (R9).
- [ ] Under a refused config, `tsuku install` and `tsuku shim install` with no
  arguments exit non-zero, name the file and the reason, and do not print the "no
  `.tsuku.toml` found" message (R10).

Source registration:

- [ ] Both validation scripts in #2552 pass: after `tsuku install --dry-run
  </dev/null` and after `tsuku install </dev/null`, `config.toml` is byte-for-byte
  unchanged (R11, R15).
- [ ] `tsuku install --dry-run owner/repo:tool </dev/null` leaves `config.toml`
  unchanged and lists the source as not registered (R11).
- [ ] With an injected terminal, and separately with `--yes` and with `--force`,
  project and command-line `tsuku install --dry-run` show no registration prompt
  and leave `config.toml` unchanged (R11).
- [ ] With no `config.toml` present, `tsuku install --dry-run` does not create
  one (R11).
- [ ] Dry-run output on stderr lists each unregistered project-named source with
  its declaring file (R11).
- [ ] With a config declaring a default-registry tool, two unregistered sources
  and one registered source, run with no terminal and no `--yes`/`--force`: the
  default-registry tool and the registered source's tool are attempted, the two
  unregistered sources and their tools are named on stderr with the declaring
  file, `tsuku install --yes` and `tsuku registry add <source>`, the exit is the
  needs-approval code, and `config.toml` is byte-for-byte unchanged (R12, R15).
- [ ] With `--yes`, with `--force`, and with the source already registered, a
  project install with no terminal produces no consent error and its exit is not
  the needs-approval code. With `--yes` and `--force`, `config.toml` gains the
  entry; with the source already registered, `config.toml` is unchanged. None of
  these depends on the recipe fetch succeeding (R12).
- [ ] `CI=true` in the environment does not let a project install with no
  terminal register a source (R12, R22).
- [ ] Through an injected terminal and scripted input: the source prompt names
  the source and the declaring file and comes before "Proceed?"; declining it
  leaves `config.toml` unchanged, the other declared tools are still attempted,
  and the exit is the needs-approval code; a yes to the source followed by a no
  at "Proceed?" leaves `config.toml` unchanged (R13, R14, R15).
- [ ] After a project-caused registration via an interactive yes, via `--yes`,
  and via `--force`, `config.toml` records the matching approval and the resolved
  absolute declaring path, keeps `auto_registered = true`, and `tsuku registry
  list` shows the approval, the path and the "(auto-registered)" annotation. A
  later install from a second project naming the same source doesn't change the
  record. A `config.toml` with an entry written by v0.14.0 still loads and lists
  with its annotation (R16).
- [ ] The `strict_registries` scenario in `project-config.feature` and the
  existing `tsuku registry add` tests pass unchanged. A non-dry-run
  `tsuku install owner/repo:tool </dev/null` writes the source with
  `auto_registered = true` and prints "Auto-registered source" on stderr (R17).

`tsuku run`:

- [ ] A unit test asserts the escalation decision directly, not the presence of a
  prompt, for each case: an org-scoped key naming a non-default source is not
  raised; a bare key whose recipe comes from a registry-added source, and one from
  a project-registered source, are not raised; a bare key whose recipe comes from
  the central registry, the embedded recipes, or the local recipes is raised
  (R18, R26).
- [ ] With no terminal, the #2559 reproduction (an `evil-owner/evil-repo:<tool>`
  key, mode unset) installs and executes nothing, exits non-zero, and prints a
  message naming the source the install would come from (R18, R19).
- [ ] When not raised at a terminal, the prompt names the source the install
  comes from; for an org key whose bare name the default registry also carries,
  the output states that the named source is not what `tsuku run` installs (R19).
- [ ] With `--mode=auto`, and with `auto_install_mode = "auto"` in `config.toml`,
  a declared command naming a non-default source runs in auto; with
  `TSUKU_AUTO_INSTALL_MODE=auto` and no config key, the mode is confirm, as today;
  with `--mode=suggest`, a default-registry declared command stays at suggest;
  an already-installed declared version executes with no prompt (R20).
- [ ] The existing headless test for a default-registry declared command still
  installs with no prompt, and its disclosure line matches the current text
  exactly (R21).
- [ ] `CI=true` does not raise the consent mode for a declared command that
  doesn't meet R18 (R22).

Cross-cutting:

- [ ] A test asserts that a `.tsuku.toml` containing keys named like consent
  settings (`auto_install_mode`, `registries`, `strict_registries`) changes no
  consent decision, and that those keys produce the existing
  "ignoring unrecognized key" diagnostic, so the test can't pass merely because
  they weren't decoded (R22).
- [ ] A test drives `tsuku run`, the command-not-found path, and activation with
  an unregistered org-scoped key and asserts `config.toml` is unchanged (R23).
- [ ] `DESIGN-shell-env-activation.md` no longer contains "prevents traversal"
  about `TSUKU_CEILING_PATHS` and states it is opt-in and unset by default (R24).
- [ ] The `tsuku-user` skill no longer contains the sentence saying discovery
  stops at `$HOME`, and contains the refusal message text and
  `tsuku registry add` in its project-install section. Review item: its
  description of the discovery rule and of the narrowed escalation matches the
  implementation (R24).
- [ ] A unit test routes all of discovery's filesystem access (open, stat, lstat,
  readlink, directory reads and path resolution) through the R26 seam, fails on
  any access that bypasses it, and asserts that discovery examines only the
  walk's directories, the config's link and target, and `$HOME` and ceiling
  resolution, and reads no config contents before the decision (R25, R26).
- [ ] `tsuku hook-env` under a refused config and under an accepted config exits
  0 with the expected output when `HTTP_PROXY` and `HTTPS_PROXY` point at an
  unreachable address (R25).

## Out of Scope

- Validation of tool names and versions in `.tsuku.toml` (#2553, fixed by #2563).
- New prompts for tools declared from the default registry.
- Changes to `strict_registries`, to `tsuku registry add`, or to non-dry-run
  installs of command-line-named sources.
- Making `tsuku run` install from the source an org-scoped key names. `tsuku run`
  resolves recipes as it does today; R19 only reports the mismatch.
- A user-side opt-out from the discovery rule. R27 constrains one if it is ever
  added.
- Other dry-run side effects from steps that run before any command, including
  writes to the recipe caches (#2549).
- Startup cost proportional to the number of registries (#2548).
- Cleaning up sources that earlier versions auto-registered.
- A flag that approves a named source for one project install (for example
  `--approve-source owner/repo`); see Known Limitations.
- Org-scoped declarations that reduce to the same tool name (#2561), fish shell
  activation (#2556), the stale binary-index check (#2530), and #2546.

## Known Limitations

- **Command-line-named sources still auto-register with no terminal.** `tsuku
  install owner/repo:tool` with no terminal registers the source as it does
  today (R17). The user typed the source, which is why it is treated differently
  from a project-named one.
- **`--yes` in CI approves whatever a pull request's config names.** A job that
  runs `tsuku install --yes` on a fork's pull request registers any source that
  pull request adds to `.tsuku.toml`. #2552 accepts `--yes` as consent, so this
  stays. A flag that approves only named sources would close it and is a
  candidate follow-up.
- **Dry-run still populates the distributed recipe cache** under
  `$TSUKU_HOME/cache/distributed/`. It grants no trust, since sources are loaded
  from `config.toml` only, and the broader dry-run write problem is #2549.
- **Entries registered before this change carry no approval record.** They keep
  working and are listed without one.
- **Sources approved with `tsuku registry add` record no declaring file.** That
  command is the user's own action and is unchanged (R17); `tsuku install --yes`
  is the approval path that records which file asked.
- **Registered sources no longer get silent installs from `tsuku run`.** A user
  who registered a source and relied on a declared command installing from it
  without a prompt now gets a prompt, or sets `--mode=auto` or
  `auto_install_mode`.
- **A config inside a checkout owned by another user is applied when the user
  works in it** (L3). Discovery bounds which files are reachable; what a
  reachable file may do is bounded by the consent rules above.
- **`tsuku run` repeats the refusal on every invocation.** With the
  command-not-found hook, each missing command under a refused config prints the
  line once.

## Decisions and Trade-offs

- **Consent is per source, not per file.** Considered: one "trust this config"
  approval covering everything a file does, and a trust prompt only for files
  that name a non-default source. Whole-file trust would prompt in every
  repository with a `.tsuku.toml`, including the many that declare only
  default-registry tools, re-prompt on every edit or never, and in CI approve any
  source a pull request adds. Prompting per file for non-default sources asks the
  same question once per repository instead of once per source. Per-source
  consent asks the question a user can answer ("do you trust `owner/repo`?")
  exactly when a new source appears, and leaves default-registry repositories
  alone. Discovery gets its own rule, since per-source consent cannot stop a
  planted file from choosing versions.
- **`--dry-run` writes no configuration for any source.** Considered: exempting
  command-line-named sources, which are otherwise unchanged. A dry run that
  writes global configuration breaks the flag's one promise, and #2552's
  criterion is not limited to project installs.
- **`--yes` and `--force` both count as consent.** #2552 names `--yes`; `--force`
  is documented as proceeding without prompts and is typed by the user, and
  narrowing it would break scripts. The residual CI exposure is listed above.
- **A skipped source doesn't stop the rest of the install.** Considered: failing
  the whole install when any source lacks consent. Installing what can be
  installed matches today's per-source failure handling and the unfamiliar-repository journey in Absorbed Brief,
  where declining one source still installs the other tools; the non-zero exit
  keeps CI honest.
- **An explicit command fails on a refused config.** Considered: treating a
  refused config as absent. For `tsuku install` that would say "no
  `.tsuku.toml` found" about a file that exists, which is the silent
  disappearance the problem statement warns about.
- **Registered sources don't qualify for `tsuku run`'s raise.** Considered:
  treating sources added with `tsuku registry add` as approved for silent
  installs. #2559 requires a prompt for any org-scoped key, so that treatment
  would help only bare keys and split the rule; org-scoped run elevation shipped
  only in v0.14.0; and qualifying registered sources can be added later. The
  design records this as a decision and confirms it with the user; if the user
  chooses otherwise, this PRD is amended before the plan.
- **The discovery rule and the escalation predicate are chosen in the design.**
  This PRD fixes the outcomes (R2's table, R18) and leaves the mechanisms to the
  design, where each is settled as a recorded decision and confirmed with the
  user: which ownership rule produces R2's outcomes, and how the run path learns
  where a recipe would come from. Exit codes for the new failure paths are also
  the design's.
- **Ceilings are compared on resolved paths, and a degenerate `$HOME` doesn't
  count.** R1's guarantee depends on knowing whether a directory is under the
  user's home, which is only reliable when both sides are resolved and the home
  is a real, user-owned directory other than `/`.
