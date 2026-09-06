---
schema: prd/v1
status: Accepted
problem: |
  Documented `.tsuku.toml` forms resolve to directory names tsuku never
  creates, so they put nothing on PATH: the "latest", "" and prefix version
  pins, and org-scoped keys, whose bare name is never derived. Every unhonored
  declaration is dropped with exit 0 and empty stderr. A developer following
  tsuku's own guide writes a declaration that does nothing and gets no signal.
goals: |
  Activation resolves every documented form against what is actually
  installed -- version pins by version order rather than string order, and
  org-scoped keys by their derived bare name. Any declaration it cannot honor
  is reported on stderr, with the reason distinguishable, at a frequency a
  developer will tolerate on a shell prompt. Nothing about it can stall or
  break a shell, and the documents that specify it stop contradicting it.
upstream: docs/briefs/BRIEF-shellenv-activation-pins.md
source_issue: 2543
---

# PRD: Documented `.tsuku.toml` forms that silently do not activate

## Status

Accepted

## Problem Statement

`.tsuku.toml` lets a project declare the tools it needs. `tsuku shell` prints
shell code that puts those versions on PATH, and `tsuku hook-env` does the same
from a shell prompt hook so that entering a directory activates its tools
automatically. Both are consumed as `eval "$(...)"`. Four version forms are
documented: an exact version, a prefix such as `"1.22"`, `"latest"`, and `""`.
Only the exact form works.

Activation resolves a declaration by interpolating the declared strings into
`$TSUKU_HOME/tools/<name>-<version>/bin` and skipping the entry when that
directory is absent — the name interpolated as raw as the version. So `latest`
looks for `tools/nodejs-latest/bin` and a
prefix looks for `tools/nodejs-26/bin`, neither of which tsuku creates. The
empty form never reaches that lookup at all: an explicit branch above it skips
the entry, with a comment recording the resolution as deferred. Reproduced
against a build of `main`: `nodejs = "26.8.1"` puts one entry on PATH, while
`""`, `"latest"`, `"26"` and `">=26"` each put zero, all four exiting 0 with
zero bytes on stderr.

The name half fails the same way. An org-scoped key such as
`"tsukumogami/koto" = "1.0"` is the documented form for a tool from a
distributed registry, and activation looks for `tools/tsukumogami/koto-1.0/bin`
while the installer wrote `tools/koto-1.0`. `SplitOrgKey` exists for this and is
called from `internal/project`'s own resolver; the activation loop iterates raw
map keys and never calls it.

Every one of those skips is silent in every channel the developer has.
`ActivationResult.Skipped` is computed correctly and read by no non-test caller,
so the information exists and reaches nobody. `tsuku install` reads the same
file and resolves the same forms correctly, which means someone can install
every tool their project declares and still get an empty PATH.

The full framing, including how the design that specifies activation
contradicts itself about prefix pins, is in the upstream brief.

### Terms

**Installation state** is the record tsuku keeps of what is installed, at
`$TSUKU_HOME/state.json`. This document uses that phrase throughout and never a
synonym.

**Channel pin** is a fifth version form, written with a leading `@`. `"@lts"`
is the shape. It names a release channel on the provider side, so resolving it
needs a provider lookup rather than a comparison against versions. It is not
one of the four documented forms, `tsuku install` handles it separately, and
activation does not resolve it. It is named here only because a developer can
write one, and because today it fails the same silent way everything else does.

`"latest"` is **not** a channel pin. It's one of the four documented forms and
it resolves, per R3.

## Goals

1. Every documented form activates the tool it names, resolved against what is
   installed on the machine — the four version forms, and org-scoped keys.
2. A developer can tell why a declaration wasn't honored, and can tell the
   possible reasons apart from one another.
3. Reporting is proportionate enough that nobody removes the prompt hook to
   escape it, because a removed hook means the feature is off entirely.
4. Activation can't break a shell or stall a prompt, under any input or any
   concurrent tsuku activity.
5. Activation and installation agree about what a version string means, and keep
   agreeing.
6. The design and the guide describe what activation actually does, so the next
   reader isn't misled the way this one was.

## User Stories

Use cases, since the actor is a developer using a CLI in a shell.

**Joining a project pinned to the newest installed version.** Someone clones a
repository they didn't configure whose `.tsuku.toml` declares `jq = "latest"`,
and `cd`s into it. jq is installed. The newest installed jq is on PATH by the
time the prompt returns, ahead of any globally active jq, and nothing is
printed.

**Writing a prefix pin from the guide.** Someone writes `go = "1.22"` for a
project needing Go 1.22.x, with Go 1.22.5 installed, and runs
`eval "$(tsuku shell)"`. `go version` reports 1.22.5.

**Hitting a pin nothing satisfies.** Someone enters a project pinned to a
version they've never installed. Every other tool in the file still activates,
and stderr names the tool that didn't and says nothing installed matches. They
run `tsuku install`.

**Writing a constraint from another ecosystem.** Someone writes
`nodejs = ">=26"`. stderr tells them that isn't a valid version string, in a
different sentence from the one about nothing being installed, so they know
installing won't help.

**Navigating inside a project whose pin is unsatisfiable.** Someone works in a
project with one bad declaration, moving between its subdirectories for an hour.
They're told once, on the way in. They aren't told again on every `cd`.

## Requirements

Most requirements serve a goal above. Two — the PATH ordering rule and
`tsuku shell`'s unchanged no-project-file behavior — are no-regression
requirements instead: they exist to pin behavior this work must not disturb, and
have no new outcome above them by design.

### Resolution

- **R1.** An exact version declaration resolves to that version when
  installation state records it as installed.
- **R2.** A prefix declaration resolves to the newest installed version inside
  that prefix boundary, using tsuku's existing dot-boundary rule, under which
  `"1"` matches `1.29.3` and does not match `10.0.0`. This holds for a
  major-only prefix (`"1"`) and a major-minor prefix (`"1.22"`) alike.
- **R3.** `"latest"` resolves to the newest installed version of the tool.
  Which version is globally active has no bearing on it: resolution is over the
  installed set, not over the `tools/current` selection.
- **R4.** `""` resolves identically to `"latest"`, as the guide states the two
  forms are equivalent.
- **R5.** Candidacy is settled before "newest" is chosen. Activation filters to
  the candidates R10 defines and picks the newest of those, rather than picking
  the newest and then testing it. So with `1.7` recorded but its files gone and
  `1.6` recorded and intact, `jq = "latest"` activates `1.6` silently rather
  than reporting on `1.7`. A developer asked for the newest usable version and
  got it.
- **R6.** "Newest" is decided by tsuku's version comparison, not by string
  comparison. Given `9.0.0` and `10.0.0` installed, `latest` resolves to
  `10.0.0`; given `0.9.0` and `0.10.0`, to `0.10.0`. A stable release outranks a
  prerelease of the same version, so with `1.0.0` and `1.0.0-rc.1` installed,
  `latest` resolves to `1.0.0`. When every installed version is a prerelease,
  `latest` resolves to the newest of them rather than reporting no match.
  Resolution is against what is present, and excluding prereleases would leave
  a tool that is installed unable to activate.
- **R7.** An org-scoped key resolves to the tool the installer wrote. Activation
  derives the bare name from the declared key, so `"owner/repo" = "1.0"` and
  `"owner/repo:tool" = "1.0"` activate `$TSUKU_HOME/tools/<bare>-1.0/bin`, not a
  path built from the raw key. Today the loop iterates raw map keys and never
  derives, so a supported form of the file silently activates nothing — the same
  defect as the version forms, arriving through the name half of the same path.
- **R8.** Activation decides whether an installed version satisfies a
  declaration, which pin level a declaration expresses, and whether a version
  string is well formed, by calling the same routines `tsuku install` and
  `tsuku outdated` call. It carries no copy of any of the three, so activation
  and installation can't drift apart.
- **R9.** Activation's notion of what is installed matches what tsuku actually
  installed. A version is a candidate only when installation state records it,
  so a directory left behind by a partial removal is not a candidate, and a
  declaration of one tool never resolves to a version of a differently-named
  tool. (The mechanism, and why directory names can't answer this, is under
  Decisions.)
- **R10.** A version is a **candidate** for a declaration when installation state
  records it, it satisfies the declaration, and its bin directory exists. The
  file test is directory existence and nothing more: an existing but empty bin
  directory counts as present and activates, because stat-ing every binary on
  every prompt isn't a cost this path can carry. This preserves today's
  guarantee that activation never puts a nonexistent directory on PATH.
- **R11.** When installation state was read, the declaration is a resolvable
  form, and it has no candidates, the reason is `missing-files`
  if at least one version *satisfying that declaration* was excluded because its
  directory was absent, and `no-match` otherwise. The quantifier is over
  satisfying versions only: `jq = "2"` on a machine with no jq 2.x installed
  reports `no-match` even if some unrelated jq 1.6 has lost its files, because
  telling the developer that jq 2 is recorded but missing would be false.
- **R12.** A declaration for a tool with no installed versions at all — nothing
  recorded in installation state under that name — reports reason `no-match`.
  This is also what a misspelled tool name produces, which follows from name
  validation being out of scope.
- **R13.** A declaration that can't be honored doesn't prevent any other
  declaration in the same file from activating.
- **R14.** Activated tools appear on PATH in lexical order by tool name, ahead
  of the pre-activation PATH, so `go` precedes `node`. This is the order
  activation produces today. When two declared tools ship a binary of the same
  name the alphabetically earlier tool therefore wins, and that holds for the
  exact pins that activate today and for the forms this work adds alike.
- **R15.** Moving from one project to another replaces the first project's PATH
  entries with the second's rather than stacking them, so walking between
  projects can't grow PATH without bound. The pre-activation PATH carried in
  `_TSUKU_PREV_PATH` is the base each activation composes against.
- **R16.** An activated version takes precedence over the globally active
  version of the same tool, which tsuku exposes through symlinks under
  `$TSUKU_HOME/tools/current`. This is the shadowing behavior the guide already
  describes.

### Reporting

- **R17.** When a declaration can't be honored, activation writes a message to
  stderr naming the tool and the reason.
- **R18.** Five reasons are distinguishable as distinct sentences, not one
  sentence with a substituted noun. They're named rather than numbered so the
  names survive renumbering, and each must contain the quoted substring:
  - **no-match** — no installed version satisfies this declaration. Contains
    `nothing installed matches`.
  - **bad-form** — the version string was rejected outright as malformed.
    Contains `not a valid version string`.
  - **channel** — the declaration is a channel pin, which activation doesn't
    resolve. Contains `channel pin`.
  - **unreadable** — installation state exists but could not be read or
    decoded. Contains `could not read`. An *absent* state file is not this
    reason: the state loader returns an empty state and no error for a missing
    file, so a machine that has deleted its state is indistinguishable from one
    that has never installed anything, and both report `no-match`. This
    reason exists so a read that didn't produce an answer is reported as such,
    rather than as the different and false claim that nothing is installed.
  - **missing-files** — installation state records this version as installed,
    but its files are gone. Contains `recorded as installed but its files are
    missing`. It's separate from no-match because collapsing the two would tell
    a developer to install something tsuku already believes is installed, and
    `tsuku install` would likely no-op; the actionable advice is to reinstall.
- **R19.** `bad-form` fires when the version string is rejected by the same
  string validation `tsuku install` applies, which rejects on disallowed
  characters, so `">=26"` is `bad-form`. A string of letters, digits, dots and
  hyphens passes that validation and is classified as a prefix, so
  `nodejs = "twenty-six"` is a well-formed prefix that matches nothing and
  reports `no-match`. Activation must not add a second, stricter validator to
  narrow this gap, because that would violate R8.
- **R20.** R17 applies to every version form, including an exact pin naming a
  version that isn't installed. No form is exempt from reporting.
- **R21.** Messages go to stderr only. Standard output carries exclusively the
  shell code both entry points are `eval`'d for, so
  `eval "$(tsuku hook-env bash)"` and `eval "$(tsuku shell)"` stay correct in
  the presence of any number of unhonorable declarations.
- **R22.** Activation exits 0 for all five reasons. A prompt hook that exits
  non-zero on entering a directory gets wrapped in `|| true` by users, which
  would discard the reporting this work adds.
- **R23.** `--quiet` suppresses the five reason messages, and also the R35
  parse diagnostic, on both `tsuku shell` and
  `tsuku hook-env`, leaving PATH behavior unchanged. The flag already exists as
  a persistent root flag and this work adds nothing to it. It is not, however,
  the answer for the prompt-hook path: the shipped hook fragments invoke
  `tsuku hook-env` with no flags, so a developer can only pass `--quiet` there
  by editing a generated file. R24 is the whole mitigation for hook frequency,
  and R23 is for a developer invoking either command by hand.
- **R24.** *Budget.* `tsuku hook-env` emits at most one message per unhonorable
  declaration per entry into a given project, so a file with two bad
  declarations produces two messages on entry rather than one. A file that won't
  parse has no declarations to count, so its R35 diagnostic gets the same
  once-per-entry budget as a single unhonorable declaration. The `unreadable`
  reason is the exception: it reports once per activation, naming the tools it
  couldn't resolve, because the failure is a property of the read rather than of
  any declaration, and ten near-identical lines for one cause is the noise this
  section exists to prevent.
- **R25.** *Suppression rule.* `tsuku hook-env` reports only when the project
  directory it resolved differs from `_TSUKU_DIR`. The project is the directory
  containing the `.tsuku.toml` activation resolved, parseable or not. So moving
  between subdirectories of an already-activated project produces nothing,
  moving to a different project reports for that project, and returning to the
  first reports again. Where nested project files exist, the project is
  whichever one activation actually resolved, since that's the one whose
  declarations were read.
- **R26.** *Inheritance.* A shell that doesn't inherit `_TSUKU_DIR` reports
  again; one that does, such as a subshell of an activated shell, doesn't.
- **R27.** *Remediation takes effect.* `tsuku hook-env` re-resolves an
  unchanged project when installation state has changed since that project was
  last activated, rather than short-circuiting on the unchanged directory. Once
  a developer runs the `tsuku install` a message told them to run, the tool is
  on PATH at the next prompt. Without this, the message this work adds names a
  remediation that appears to do nothing, which is worse than the silence it
  replaced: it turns "I can't tell what's wrong" into "I did what it said and
  it's still broken", and sends someone hunting a bug in the tool.

  It's also intermittent, which is worse than being merely broken. The early
  exit compares the working directory against `_TSUKU_DIR` exactly, so *any*
  movement re-resolves — `cd sub` inside the project does it, not only leaving
  and returning. A developer who happens to change directory after installing
  sees the tool appear; one who stays put sees it never work. Behavior that
  depends on whether someone incidentally moved is what gets filed as
  intermittent and never reproduced.
- **R28.** *The re-resolve is silent.* A re-resolve triggered by R27 reports
  nothing. Not when it finds the tool now present, which needs no announcement,
  and not when it finds the declaration still unhonorable, which the developer
  was already told about on entry. Reporting there would build a nag keyed on
  "anyone installed anything", firing in one terminal because an unrelated tool
  was installed in another. The entry-based budget in R24 is unaffected: a
  re-resolve is not an entry.
- **R29.** *The trigger is deliberately coarse.* Any change to installation
  state re-resolves, not only a change touching a declared tool, because
  installing anything rewrites the whole file and a per-tool check would need
  the very read the trigger exists to avoid. This is intentional over-triggering
  and must not be narrowed into a per-tool condition later: doing so
  reintroduces the trap R27 closes. The cost that makes it affordable is that
  detecting the change is a stat rather than a read: roughly 36 microseconds
  against the 5 ms unchanged-directory budget the design already sets, about
  0.7% of it. The argument is deliberately made against that budget rather than
  against today's measured `hook-env` cost. tsukumogami/tsuku#2548 is a defect,
  and a broken performance property is not a licence to spend against it — when
  it's fixed, the 5 ms budget becomes real again and this still fits.
- **R30.** *Sessions that predate this change.* A shell whose environment
  carries the older tracking variables but nothing recording the last
  activation's installation state must re-resolve once and record, rather than
  reading the absent record as "never re-resolve" or as "always re-resolve". The
  first leaves every pre-existing session broken until the developer restarts
  their shell; the second removes the early exit for those sessions entirely.
  Both failures are silent, so the case is settled here rather than left to an
  implementer.
- **R31.** *Edits in place, accepted divergence.* Editing `.tsuku.toml` while
  standing inside an already-activated project produces no report until the
  project is left and re-entered, because neither the directory nor the project
  changed. Whether the edit takes effect before then depends on where the
  developer is standing, and that's accepted rather than fixed here: at the
  project root the working directory still matches `_TSUKU_DIR`, so the next
  prompt short-circuits and PATH is unchanged; in a subdirectory it doesn't
  match, so the next prompt re-activates and PATH picks the edit up silently.
  The same applies to repairing a file that wouldn't parse. Both self-correct on
  re-entry.
- **R32.** `tsuku shell` reports on every invocation. It's one-shot and
  explicitly asked for, so there's no repetition to suppress.

### Failure behavior

- **R33.** Activation reads installation state without acquiring a lock, and so
  never waits on another tsuku process. A declaration still resolves correctly
  while an install is writing installation state; reason `unreadable` is
  reserved for a read that fails outright, not for one that finds another
  process writing.
- **R34.** Activation reads installation state at most once per activation,
  regardless of how many tools the file declares.
- **R35.** A `.tsuku.toml` that can't be parsed produces one diagnostic line on
  stderr, with no command usage block, and exit 0.
- **R36.** On a parse failure, `tsuku hook-env` treats the directory containing
  the unparseable file as the resolved project and records it, exactly as it
  would for a project that parsed. Where that directory differs from
  `_TSUKU_DIR`, stdout carries shell code setting `_TSUKU_DIR` to it and setting
  PATH and `_TSUKU_PREV_PATH` to the pre-activation base — the shape of an
  activation in which nothing activated — and nothing else. Where it already
  equals `_TSUKU_DIR`, stdout is empty and the `eval` is a no-op.

  Recording it is what makes R24's budget apply: the tracking variable has to
  point at the project the developer is standing in, both so later movement
  compares against the right project and so there is something to suppress the
  repeat diagnostic against. Unsetting it instead would restore PATH correctly
  and then re-report on every prompt for as long as the developer stayed in the
  directory, with no way to silence it from the shipped hook — which is the
  outcome Goal 3 forbids.

  `tsuku shell` records nothing on a parse failure. It's one-shot and doesn't
  own the prompt hook's tracking variables, so it prints the R35 diagnostic and
  exits without emitting shell code.
- **R37.** Leaving a project, meaning moving to a directory with no
  `.tsuku.toml` above it, restores the pre-activation PATH, prints nothing on
  stderr, and exits 0. There's nothing to report on the way out, and no
  reporting state survives the exit. The exit status is stated because leaving
  is the most common transition after entering, and sharing a code path with
  `tsuku shell`'s non-zero no-project exit would make every departure from a
  project return non-zero.
- **R38.** `tsuku shell` invoked where no `.tsuku.toml` is found anywhere above
  the working directory keeps its current behavior: one line on stderr saying no
  project file was found, nothing on stdout, and a non-zero exit. This work
  doesn't change it. The non-zero exit is specific to there being no project at
  all and doesn't conflict with R22, which governs a project that exists and has
  declarations that can't be honored.
- **R39.** `tsuku hook-env` does not share that behavior. Where no project file
  is found and none was previously activated, it exits 0 with empty stdout and
  empty stderr. It runs on every prompt, including every prompt outside any
  project, so a non-zero exit there is the outcome R22 exists to prevent.

### Documentation

- **R40.** `docs/designs/current/DESIGN-shell-env-activation.md` is amended so
  its specified algorithm, its worked PATH example, and the implementation
  agree, and so it states what activation does when a declaration can't be
  honored, including all five reasons and the non-blocking read.
- **R41.** `docs/guides/shell-integration.md` states what a developer sees when
  a declaration can't be honored, and its statement of the four version forms
  becomes true rather than aspirational. It also mentions channel pins, since a
  developer can now receive a message naming one and would otherwise find no
  trace of the concept in the guide.

## Acceptance Criteria

Message-content criteria hold for both `tsuku hook-env` and `tsuku shell`; only
frequency differs, per R24 and R32.

**Resolution**

- [ ] With a tool installed at version X, a `.tsuku.toml` declaring that tool as
      `"latest"` puts `$TSUKU_HOME/tools/<tool>-X/bin` on PATH via both
      `tsuku hook-env` and `tsuku shell`.
- [ ] The same holds for `""`.
- [ ] The same holds for a major-only prefix matching X, and for a major-minor
      prefix matching X.
- [ ] The same holds for an exact declaration of X.
- [ ] With `9.0.0` and `10.0.0` installed and `9.0.0` recorded as the active
      version, `"latest"` selects `10.0.0`. With `0.9.0` and `0.10.0` installed,
      it selects `0.10.0`. A prefix of `"1"` selects neither.
- [ ] With `1.0.0` and `1.0.0-rc.1` installed, `"latest"` selects `1.0.0`. With
      only `1.0.0-rc.1` installed, it selects `1.0.0-rc.1`.
- [ ] The activated version appears on PATH ahead of
      `$TSUKU_HOME/tools/current`.
- [ ] A project declaring two tools that both ship a binary of the same name
      resolves the collision in favour of the alphabetically earlier tool name,
      for exact pins and for the forms this work adds alike.

**Resolution source (R8, R9, R10, R12)**

- [ ] A change to the shared matching, pin-level or version-validation rule
      that activation doesn't follow in step turns an automated check **of
      activation's own behavior** red. The failing check must implicate
      activation: install's tests going red when install's rule changes doesn't
      satisfy this, because that happens under an implementation carrying a
      private copy, which is the case this criterion exists to catch. How the
      check is built is the design's call; whose behavior it exercises is not.
- [ ] Activation classifies a declaration the same way `tsuku install` does, for
      every case in the test corpus that already exists for the matching and
      pin-level routines.
- [ ] With `git-lfs` installed and `git` not installed, a declaration of
      `git = "latest"` reports `no-match` and puts no `git-lfs` directory on
      PATH.
- [ ] A `$TSUKU_HOME/tools/<tool>-<version>` directory with no corresponding
      installation-state entry is never put on PATH.
- [ ] With one version recorded whose bin directory has been removed and no
      other candidate, the declaration reports `missing-files`, not `no-match`,
      and puts no path on PATH.
- [ ] With `2.0.0` recorded but its directory removed and `1.0.0` recorded and
      intact, `"latest"` activates `1.0.0` and prints nothing.
- [ ] `jq = "2"` with only `1.6` recorded and its files gone reports `no-match`,
      not `missing-files`. This separates filtering by match-then-files from
      files-then-match; an implementation doing the latter reports
      `missing-files` and tells the developer to reinstall a version that was
      never recorded.
- [ ] A declaration for a tool with no installation-state entry at all reports
      `no-match`.

**Reporting**

- [ ] Capturing stdout and stderr separately for a declaration that can't be
      honored: stdout contains only the shell code for the requested shell and
      nothing else; the diagnostic appears on stderr and does not appear on
      stdout. Checked for `fish` as well as `bash`, because the fish hook pipes
      stdout straight to `source` with no command-substitution boundary, so a
      leak there executes rather than merely printing.
- [ ] Each reason's message contains the substring R18 quotes for it and the
      declared tool's name, and that substring is absent from the other four
      reasons' messages.
- [ ] `nodejs = ">=26"` produces `bad-form`. `nodejs = "@lts"` produces
      `channel`.
- [ ] With installation state present but undecodable, a file declaring ten
      tools produces exactly one `unreadable` message naming the tools it
      couldn't resolve, PATH is left unchanged, and no message claims nothing is
      installed.
- [ ] Each of the five reasons exits 0.
- [ ] An exact pin naming an uninstalled version reports rather than staying
      silent.
- [ ] A file mixing one satisfiable and one unsatisfiable declaration activates
      the satisfiable one and reports only the other.
- [ ] `--quiet` suppresses all five messages on both commands while leaving PATH
      behavior unchanged.

**Frequency (R24, R32)**

Tested at the command level rather than against the resolution function, because
the behavior lives in how the command exports and re-reads `_TSUKU_DIR`.

- [ ] Running `tsuku hook-env` in a project root with an unsatisfiable
      declaration reports; feeding the resulting `_TSUKU_DIR` and
      `_TSUKU_PREV_PATH` into a second invocation from a subdirectory produces
      empty stderr.
- [ ] A project A to project B to project A sequence reports on each of the
      three entries.
- [ ] A file with two unsatisfiable declarations produces two messages on entry,
      each naming its own tool.
- [ ] Feeding the variables emitted by a first invocation into a second
      invocation from the *same* directory produces empty stderr, including when
      every declaration in the file was unhonorable. This catches an
      implementation that omits `_TSUKU_DIR` when nothing activated, which would
      re-report on every prompt forever.
- [ ] A shell inheriting `_TSUKU_DIR` from an activated parent produces no
      report; one that doesn't inherit it reports.
- [ ] Editing `.tsuku.toml` to add an unsatisfiable declaration while standing
      at the project root produces no report and no PATH change until the
      project is left and re-entered. Making the same edit while standing in a
      subdirectory produces no report either, and PATH picks the edit up on the
      next prompt.
- [ ] Repairing an unparseable `.tsuku.toml` while standing at the project root
      produces no message and no PATH change until the project is left and
      re-entered.
- [ ] Entering a project with an unsatisfiable declaration reports; installing a
      matching version without leaving the directory puts it on PATH at the next
      invocation, with empty stderr.
- [ ] Installing an unrelated tool, leaving the declaration still unsatisfiable,
      produces empty stderr on the next invocation rather than re-reporting.
- [ ] With installation state unchanged, a second invocation from the same
      directory short-circuits: no decode of installation state, empty stdout,
      empty stderr.
- [ ] A shell environment carrying `_TSUKU_DIR` and `_TSUKU_PREV_PATH` but no
      record of the last activation's installation state re-resolves once and
      records, rather than re-resolving on every subsequent prompt or never
      re-resolving at all.
- [ ] `tsuku shell` reports on each invocation regardless of the above.

**Failure behavior**

- [ ] With another process holding `$TSUKU_HOME/state.json.lock` exclusively for
      the whole duration and never releasing it, activation still resolves
      declarations correctly from the last committed state and returns without
      waiting for that lock. Measured at the activation entry point rather than
      end to end, because the command's total time is dominated by the unrelated
      startup cost in tsukumogami/tsuku#2548.
- [ ] Resolving a `.tsuku.toml` declaring ten tools decodes installation state
      exactly once, and the decode count doesn't grow with the number of
      declarations.
- [ ] A malformed `.tsuku.toml` produces a single stderr diagnostic, no usage
      block, and exit 0.
- [ ] A malformed `.tsuku.toml` whose directory already equals `_TSUKU_DIR`
      leaves stdout empty, so the `eval` is a no-op.
- [ ] A malformed `.tsuku.toml` whose directory differs from `_TSUKU_DIR` puts
      on stdout, and only on stdout, the code setting `_TSUKU_DIR` to that
      directory and PATH and `_TSUKU_PREV_PATH` to the pre-activation base.
- [ ] Entering a project whose file won't parse reports once; feeding the
      emitted variables into a second invocation from the same directory, and
      into a third from a subdirectory, produces empty stderr both times. The
      second environment must be built only from what the first invocation
      actually emitted, never synthesized: under an implementation that records
      nothing that set is empty, and its emptiness is the whole signal.
- [ ] `tsuku hook-env` where no project file is found and none was previously
      activated exits 0 with empty stdout and empty stderr.
- [ ] `tsuku shell` on an unparseable file prints the diagnostic on stderr,
      prints nothing on stdout, and exits 0, whether or not `_TSUKU_PREV_PATH`
      is set. It must not emit a `_TSUKU_DIR` recording: that branch belongs to
      `tsuku hook-env`, and a shared code path is the way `tsuku shell` would
      inherit it.
- [ ] `--quiet` suppresses the parse diagnostic as well as the five reason
      messages.
- [ ] Leaving a project restores the pre-activation PATH, prints nothing, and
      exits 0.
- [ ] `tsuku shell` with no `.tsuku.toml` anywhere above the working directory
      prints one line on stderr saying no project file was found, prints nothing
      on stdout, and exits non-zero.

**Documentation**

- [ ] `DESIGN-shell-env-activation.md`'s worked PATH example, its
      resolution-algorithm step, and its trade-off entry about uninstalled
      versions each state what the implementation does, and it carries a section
      naming the five reporting reasons and the non-blocking read.
- [ ] `shell-integration.md`'s version-strings section describes behavior that
      the acceptance criteria above verify, and it tells a developer what they
      see when a declaration can't be honored.

## Decisions and Trade-offs

The brief deferred two questions. Both are closed here.

**Whether `hook-env` and `tsuku shell` report identically.** They report the
same messages and differ in frequency, and the difference is forced by the
trigger rather than chosen: `tsuku shell` is one-shot and asked for, while
`hook-env` fires unbidden. What makes it a real decision is that the obvious
reading of "warn on activation" nags badly. `hook-env` short-circuits only when
the working directory equals `_TSUKU_DIR`, and `_TSUKU_DIR` holds the project
root. So `cd src/` inside an activated project is a full re-activation, and a
warning attached naively to activation fires again — once per directory change
anywhere under the project, in both directions, for as long as someone works
there. Ten subdirectories, ten warnings about one missing tool. R24 is written
against the project rather than the directory for that reason, and it needs no
new state because `_TSUKU_DIR` already carries exactly the fact. The alternative
considered was a time-based throttle file of the kind tsuku already keeps per
tool for out-of-channel notices. Rejected: on-disk state plus an interval nobody
can pick correctly, to answer a question an existing environment variable
answers exactly.

**What activation does when it can't read installation state.** It doesn't
wait, and R33 states the rule rather than only the prohibition, because
"never blocks" alone admits two implementations that behave oppositely — one
that attempts a non-blocking lock and activates nothing on contention, and one
that reads without a lock and activates normally. The second is correct.
Installation state is saved by writing a temporary file and renaming it over the
target, and rename is atomic, so a reader taking no lock always sees a complete
file: the old contents or the new, never half of either. Worst case is a read
landing just before a rename and returning one-version-stale data, which
self-corrects on the next prompt. The lock therefore buys a reader nothing the
rename doesn't already give it, and costs it an unbounded wait — the shared lock
is taken without a non-blocking flag and with no timeout, no non-blocking shared
variant exists in the codebase, and flock makes no fairness guarantee between a
queued shared lock and a stream of exclusive ones. A shell that hangs on `cd` is
a worse failure than the one this work fixes.

`unreadable` exists because a read that fails should say so. Silently treating
a failed read as "nothing is installed" would report a default instead of
admitting the
check didn't run, which is the same disease as `Skipped` being computed and
never read.

**Why installation state rather than directory names.** Directory names are
`<name>-<version>` with a hyphen that both halves may contain, so scanning for
the prefix `git-` collects `git-lfs-3.5.1`. The hazard is asymmetric — a longer
name is safe against a shorter one — and can't be resolved soundly from the
filesystem, only heuristically, by assuming version strings start with a digit,
which no validator enforces. Directory scanning is also about a thousand times
cheaper than reading installation state, so there's real pressure toward it;
that's why R9 and R10 are stated as requirements and given falsifying criteria
rather than left as advice.

**Why there's no millisecond budget.** Reading installation state isn't free:
it carries a full installation plan per installed version, so on a real machine
it's megabytes and decoding dominates. But the command it would be measured
against currently costs roughly 1.3 seconds per invocation for an unrelated
reason described under Known Limitations — two orders of magnitude larger. A
latency budget written against today's baseline would mostly be measuring that
defect. R33 and R34 constrain blocking and read counts instead, which is the
durable form of the same concern and stays meaningful once #2548 is fixed.

## Known Limitations

Resolution is against what's installed, so a correct declaration for a version
the developer hasn't installed still doesn't activate. It now says so, which is
the change; installing it stays a separate action, deliberately.

Channel pins are reported rather than resolved.

A declaration can be honored differently by `tsuku install` and by activation at
the same moment, because install resolves against everything published and
activation against what's present. That's inherent to activation not fetching,
and the reporting is what makes it visible rather than confusing.

R5's fallback buys a working tool at the cost of one new silence. When the
newest version satisfying a declaration has lost its files and an older one
hasn't, activation uses the older one and says nothing, so a corrupt install of
the newer version stays invisible: installation state still records it, and
`tsuku install` will likely no-op on it. The trade is deliberate. Reporting a
successful activation is the wrong thing to put on a shell prompt, and the
alternative is worse: refusing to activate a tool the developer has a usable
copy of. `tsuku doctor` is the natural place for a check that surfaces it, and
this work doesn't add one.

Because name validation is out of scope, a misspelled tool name is
indistinguishable from a tool that simply isn't installed. Both report
`no-match`.

Measured during discovery and tracked separately as tsukumogami/tsuku#2548:
`tsuku hook-env` currently costs roughly 1.3 seconds per invocation on a machine
with three distributed registries configured, against a design budget of 5 ms.
That cost is paid on every prompt by every user who follows the guide's
instruction to install the hook, so it keeps this feature unaffordable even once
it's correct. It doesn't weaken R24: a slow hook and a noisy hook get deleted
for different reasons, and fixing one doesn't excuse the other.

## Out of Scope

- Installing anything. Activation resolves against what's present and doesn't
  fetch; install-on-demand is separate work.
- Resolving declarations against the recipe registry. Registry lookups are
  cache-backed and activation runs on every prompt, so a cold cache would let
  tsuku reject a valid `.tsuku.toml` — trading a silent skip for a loud wrong
  answer.
- Validating that a declared tool name exists, which needs the registry and is
  out for the same reason.
- Resolving channel pins.
- The `tsuku install` reporting defects tracked as tsukumogami/tsuku#2546.
- The `hook-env` startup cost described under Known Limitations.
- Changing how installation state is stored, including the installation plans
  that dominate its size.
