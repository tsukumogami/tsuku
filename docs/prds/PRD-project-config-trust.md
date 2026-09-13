---
schema: prd/v1
status: Draft
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
  rather than acted on or dropped silently, and nothing that outlives a command
  happens without an explicit yes.
upstream: docs/briefs/BRIEF-project-config-trust.md
---

# PRD: Project Config Trust

## Status

Draft

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
  written. The design record describes the ceiling variable as a mitigation
  in force when it is unset by default. The `$HOME` stop is also weaker than it
  looks: the start directory is resolved through symlinks but `$HOME` and the
  ceiling entries are not, so when `$HOME` or a ceiling is given as a symlinked
  path, the walk passes it and a config above it is applied (reproduced in a
  unit test).
- **A project file registers a permanent source with no consent (#2552).**
  With no terminal attached, `tsuku install` and `tsuku install --dry-run` in a
  directory whose config names `"owner/repo:tool"` both wrote
  `[registries."owner/repo"]` into `config.toml` and exited 6. The same
  happens for `tsuku install --dry-run owner/repo:tool` with the source named
  on the command line. The entry is consulted by every later resolution for
  every project, and nothing records which file asked for it.
- **`tsuku run` skips its prompt whatever the declared source (#2559).** A key
  naming `evil-owner/evil-repo:<tool>` raised the unset default consent mode to
  auto, and with no terminal the run installed and executed a recipe with no
  prompt. The recipe it installed came from the default registry, so the key's
  source was ignored rather than honoured, but the declaration still decided
  that nobody would be asked. Once a source has been registered and a recipe
  installed from it, a plain declaration can install from that source the same
  way.

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
  named, the user is told on stderr which file and which source, and why.
- Nothing that outlives the command, such as a new entry in `config.toml`,
  happens because a terminal was missing or because the command was a dry run.

## User Stories

- As a **developer on a shared Linux host** working in `/tmp/shared-work`, where
  another account left `/tmp/.tsuku.toml`, I want tsuku to ignore that file and
  tell me it did, so that another user can't reshape my `PATH`.
- As a **developer evaluating an unfamiliar repository** whose `.tsuku.toml`
  declares a tool from `someorg/recipes`, I want `tsuku install --dry-run` to
  tell me the source would need approval without writing anything, and `tsuku
  install` to ask me about `someorg/recipes` by name, so that I decide which
  sources my machine trusts.
- As a **maintainer whose CI runs `tsuku install` with no terminal** in a repo
  that declares an org-scoped source, I want the job to fail and name the source,
  the declaring file and the command that approves it, so that approval is an
  explicit, visible step in the pipeline rather than a side effect.
- As a **developer running a declared command** through `tsuku run` or the
  command-not-found hook, I want default-registry tools to install as they do
  today and any other source to be put to me as a question naming where the
  install comes from, so that a repository can't waive the prompt for a source
  I never approved.
- As a **developer building in a container** whose checkout is a volume at
  `/srv/app`, possibly owned by another uid or by root, I want the project's own
  `.tsuku.toml` found as it is today, so that the fix doesn't break the ordinary
  container layout.
- As a **user auditing my configuration**, I want `tsuku registry list` to show
  which sources a project caused to be registered and which file asked, so that
  I can tell a source I added from one a repository added.

## Requirements

### Functional: discovery (#2555)

- **R1.** A `.tsuku.toml` whose resolved directory is at or below the resolved
  `$HOME` is found and applied exactly as today.
- **R2.** Outside the resolved `$HOME`, discovery applies a config only when the
  invoking user can be held to have chosen it. The rule that decides this is
  chosen in the design, and must produce these outcomes:
  - **Must be found:** a checkout owned by the invoking user (`/srv/app`); a
    root-owned volume used by a non-root user in a container; a volume owned by a
    different non-root uid (for example host uid 1000 used as uid 1001); a
    checkout owned by a non-root uid used by root in a Dockerfile `RUN`; a config
    at a repository root with the working directory deep inside it.
  - **Must be refused:** a config in a parent directory planted by another user
    (`/tmp/.tsuku.toml` above `/tmp/shared-work`), for a non-root and a root
    invoking user; a config in a directory another user pre-created in a shared
    world-writable location (`/tmp/build`, `mkdir -p` by the victim then `cd`);
    a `.tsuku.toml` that is a symlink owned by, or pointing at a file owned by,
    another user.
- **R3.** When discovery refuses a config, the walk stops there. It does not
  continue to directories above the refused file.
- **R4.** The decision covers both a symlinked `.tsuku.toml` entry and the file
  whose bytes are actually parsed, and is made on the file that is read, so a
  file swapped between the check and the read cannot be parsed unchecked.
- **R5.** `$HOME` and every `TSUKU_CEILING_PATHS` entry are compared against the
  walk on symlink-resolved paths, as the start directory already is.

### Functional: refusal reporting

- **R6.** A refused config produces one line on stderr naming the file (quoted,
  since the path may be attacker-chosen) and the reason. Nothing about the
  refusal is written to stdout.
- **R7.** `tsuku shell` and the prompt hook (`tsuku hook-env`) exit 0 with their
  normal stdout, and print the refusal once per entry into the directory rather
  than on every prompt.
- **R8.** `tsuku run` prints the refusal line and then behaves as though no
  config were present.
- **R9.** `tsuku install` with no arguments fails with a non-zero exit and the
  refusal reason, rather than reporting that no `.tsuku.toml` exists.
  `tsuku shim install` with no arguments behaves the same way.

### Functional: source registration (#2552)

- **R10.** `tsuku install --dry-run` writes no configuration under any terminal
  condition, whether the source was named by a project config or on the command
  line. Its output lists each source a real run would need approved and, for
  project-named sources, the declaring file.
- **R11.** A source that is not already registered and is named only by a
  project config is registered only with explicit consent: a yes at an
  interactive prompt, `--yes`, or `--force`. A source already present in
  `config.toml` (added with `tsuku registry add` or registered earlier) needs no
  new consent. The absence of a terminal is never consent.
- **R12.** The interactive prompt for such a source names the source and the
  declaring file.
- **R13.** With no terminal and no explicit consent, `tsuku install` reports each
  source that would need registering and the declaring file, names
  `tsuku registry add <source>` as the way to approve it, and exits non-zero
  without writing `config.toml`.
- **R14.** A registration caused by a project config records how it was approved
  and the path of the declaring config. `tsuku registry list` shows both.
  Entries written before this change keep loading and are listed without that
  record.
- **R15.** `strict_registries`, `tsuku registry add`, and non-dry-run installs of
  a source named on the command line behave exactly as today.

### Functional: `tsuku run` escalation (#2559)

- **R16.** `tsuku run` raises the unset default consent mode to auto for a
  declared command only when both hold: the declaration's key names no source
  other than the default registry, and the recipe the run would install comes
  from the default registries (central or embedded) or the user's local recipes
  under `$TSUKU_HOME/recipes`. A recipe from any registered distributed source,
  whether added with `tsuku registry add` or registered by a project, does not
  qualify. In every other case the command gets the mode it would have had if
  undeclared.
- **R17.** When a declared command is not raised because of its source, the
  prompt, and the message shown when no terminal is attached, name the source the
  install would actually come from. When the declaration's key names a different
  source from that one, the output says the named source is not what `tsuku run`
  installs.
- **R18.** A consent mode set by `--mode`, `TSUKU_AUTO_INSTALL_MODE`, or
  `auto_install_mode` in `config.toml` reaches the install as set; only the raise
  a declaration causes changes. Running an already-installed declared version
  without consent (the existing fast path) is unchanged.
- **R19.** A declared command whose recipe comes from the default registry, under
  a key that names no other source, behaves exactly as today: no new prompt, and
  the same disclosure line.

### Functional: cross-cutting

- **R20.** No consent or trust input is read from `.tsuku.toml`: not the set of
  approved sources, not the consent mode, and not any future discovery opt-out.
  A detected CI environment (for example `CI=true`) never grants consent.
- **R21.** The documentation describes the new behaviour:
  `docs/designs/current/DESIGN-shell-env-activation.md` states that
  `TSUKU_CEILING_PATHS` is opt-in and unset by default and no longer claims it
  prevents traversal; the `tsuku-user` skill under `plugins/` describes the
  discovery rule, the refusal message, source consent for project installs, and
  the narrowed escalation.

### Non-functional

- **R22.** Discovery's added checks read only metadata of files and directories
  the walk already visits, and make no network calls, since `tsuku hook-env`
  runs on every shell prompt.
- **R23.** The ownership information the discovery and escalation decisions read
  can be substituted in tests, so the other-user cases are testable without root
  or a second account.

## Acceptance Criteria

Discovery and reporting:

- [ ] The #2555 reproduction no longer applies `/tmp/.tsuku.toml` from
  `/tmp/shared-work`, and stderr names the refused file (R2, R6).
- [ ] A test with `HOME` pointed at a directory that is not an ancestor of the
  project asserts that a config at the root of a checkout outside `$HOME` is
  found (R2).
- [ ] Tests assert the discovery outcome, not only a message, for each R2 layout:
  every "must be found" layout loads its config and every "must be refused"
  layout returns a refusal, including the root-invoker planted-parent case and
  the pre-created `/tmp/<name>` case (R2, R23).
- [ ] A test places a refused config below a second, acceptable config and
  asserts that neither is applied (R3).
- [ ] Tests cover a symlinked `.tsuku.toml` whose link is foreign-owned and one
  whose target is foreign-owned, and both are refused (R4).
- [ ] A test with a symlinked `$HOME` and a config above the real home asserts the
  config is not found; the same for a symlinked `TSUKU_CEILING_PATHS` entry (R5).
- [ ] Existing in-`$HOME` discovery tests and the `project-config.feature`
  scenarios pass unchanged (R1).
- [ ] Under a refused config, `tsuku hook-env` exits 0, its stdout contains only
  what it emits with no config, the refusal appears on stderr on the first prompt
  in the directory and not on a second prompt in the same directory (R6, R7).
- [ ] Under a refused config, `tsuku run` prints the refusal on stderr and
  resolves the command as it would with no config (R8).
- [ ] Under a refused config, `tsuku install` and `tsuku shim install` with no
  arguments exit non-zero and name the file and the reason (R9).

Source registration:

- [ ] Both validation scripts in #2552 pass: after `tsuku install --dry-run
  </dev/null` and after `tsuku install </dev/null`, `config.toml` is byte-for-byte
  unchanged (R10, R13).
- [ ] `tsuku install --dry-run owner/repo:tool </dev/null` leaves `config.toml`
  unchanged (R10).
- [ ] Dry-run output for a project naming an unregistered source lists that
  source and the declaring file (R10).
- [ ] With no terminal and no `--yes`/`--force`, project `tsuku install` exits
  non-zero, and stderr names the source, the declaring file and
  `tsuku registry add <source>` (R13).
- [ ] With `--yes`, with `--force`, and with the source already registered,
  project `tsuku install` proceeds with no terminal (R11).
- [ ] At a terminal, the prompt names the source and the declaring file, and
  declining leaves `config.toml` unchanged (R11, R12).
- [ ] After a project-caused registration, `config.toml` records the approval
  mechanism and the declaring path, and `tsuku registry list` shows them; a
  `config.toml` containing an entry written by v0.14.0 still loads and lists
  (R14).
- [ ] The `strict_registries` scenario in `project-config.feature` and the
  existing `tsuku registry add` tests pass unchanged, and a non-dry-run
  `tsuku install owner/repo:tool` with no terminal behaves as on v0.14.0 (R15).

`tsuku run`:

- [ ] A unit test asserts the escalation decision directly, not the presence of a
  prompt: an org-scoped key naming a non-default source does not raise the unset
  default to auto (R16).
- [ ] A unit test asserts that a bare key whose recipe would come from a
  registered distributed source does not raise the unset default (R16).
- [ ] A unit test asserts that a bare key whose recipe comes from the default
  registry is raised exactly as today, and the end-to-end headless test for that
  case still installs with no prompt (R16, R19).
- [ ] When not raised, the prompt names the source the install comes from, and
  for an org key whose bare name the default registry also carries, the output
  states that the named source is not what `tsuku run` installs (R17).
- [ ] With `--mode=auto`, and with `auto_install_mode = "auto"` in `config.toml`,
  a declared command naming a non-default source runs in auto as set (R18).

Cross-cutting:

- [ ] A test asserts that a `.tsuku.toml` containing keys named like consent or
  trust settings (for example `auto_install_mode`, `registries`,
  `strict_registries`) changes no consent decision, and that `CI=true` in the
  environment does not let a non-TTY project install register a source (R20).
- [ ] `DESIGN-shell-env-activation.md` no longer contains the claim that
  `TSUKU_CEILING_PATHS` prevents traversal, and states it is opt-in and unset by
  default (R21).
- [ ] The `tsuku-user` skill no longer states that discovery simply stops at
  `$HOME`, and documents the refusal message, source consent for project
  installs, and the narrowed escalation (R21).

## Out of Scope

- Validation of tool names and versions in `.tsuku.toml` (#2553, fixed by #2563).
- New prompts for tools declared from the default registry.
- Changes to `strict_registries`, to `tsuku registry add`, or to non-dry-run
  installs of sources named on the command line.
- Other dry-run side effects from steps that run before any command, including
  writes to the recipe caches (#2549).
- Startup cost proportional to the number of registries (#2548).
- Cleaning up sources that earlier versions auto-registered.
- A flag that approves a named source for one project install (for example
  `--approve-source owner/repo`); see Known Limitations.
- Org-scoped declarations that reduce to the same tool name (#2561), fish shell
  activation (#2556), the stale binary-index check (#2530), and #2546.

## Known Limitations

- **`--yes` in CI approves whatever a pull request's config names.** A job that
  runs `tsuku install --yes` on a fork's pull request registers any source that
  pull request adds to `.tsuku.toml`. #2552 accepts `--yes` as consent, so this
  stays. A flag that approves only named sources would close it and is a
  candidate follow-up.
- **Dry-run still populates the distributed recipe cache** under
  `$TSUKU_HOME/cache/distributed/`. It grants no trust, since sources are loaded
  from `config.toml` only, and the broader dry-run write problem is #2549.
- **Entries registered before this change carry no provenance.** They keep
  working, and `tsuku registry list` shows them without a record of what added
  them.
- **Registered sources no longer get silent installs from `tsuku run`.** A user
  who registered a source with `tsuku registry add` and relied on a declared
  command installing from it without a prompt now gets a prompt, or sets
  `--mode=auto` or `auto_install_mode`. This follows from R16 and is confirmed
  with the user at the design hop.
- **A config inside a checkout owned by another user is applied when the user
  works in it.** Discovery bounds which files are reachable; what a reachable
  file may do is bounded by the consent rules above. The exact layouts are in R2.

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
  sources named on the command line, which are otherwise unchanged. A dry run
  that writes global configuration breaks the flag's one promise, and #2552's
  criterion is not limited to project installs.
- **`--yes` and `--force` both count as consent.** #2552 names `--yes`; `--force`
  is documented as proceeding without prompts and is typed by the user, and
  narrowing it would break scripts. The residual CI exposure is listed above.
- **An explicit command fails on a refused config.** Considered: treating a
  refused config as absent. For `tsuku install` that would say "no
  `.tsuku.toml` found" about a file that exists, which is the silent disappearance
  the problem statement warns about.
- **The discovery rule and the escalation predicate are chosen in the design.**
  This PRD fixes the outcomes (R2, R16) and leaves the mechanism to the design,
  where each is settled as a recorded decision and confirmed with the user: which
  ownership rule produces R2's layouts, and how the run path learns where a recipe
  would come from.
- **Ceilings are compared on resolved paths.** The rule in R1 depends on knowing
  whether a directory is under `$HOME`, which is only reliable when both sides
  are resolved the same way.
