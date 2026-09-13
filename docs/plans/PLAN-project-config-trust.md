---
schema: plan/v1
status: Active
execution_mode: single-pr
tracking_level: none
upstream: docs/designs/DESIGN-project-config-trust.md
milestone: "Project Config Trust"
issue_count: 9
---

# PLAN: Project Config Trust

## Status

Active

## Scope Summary

Close three defects that shipped in v0.14.0 and share one threat — a
`.tsuku.toml` the invoking user did not author. Discovery gains a trust rule
outside the user's home and refuses loudly, with a typed error every caller
renders once. Registering a recipe source that only a project config named
requires explicit consent, is written after the install proceeds rather than
before, and records how it was approved and which file asked. `tsuku run` stops
waiving its install prompt for a declaration whose key names a source the user
has not registered.

The work lands as one pull request closing tsukumogami/tsuku#2555, #2559 and
#2552. Nine units, sequenced so the seams each acceptance criterion needs exist
before the criterion is written.

## Decomposition Strategy

**Hybrid: vertical by defect, with a walking skeleton inside the discovery
slice.** Discovery comes first because the other two slices sit on it and it is
the only one with a new decision procedure to get right. Within it the seams and
the read discipline land before the rule that uses them, and the rule lands
before anything renders its refusal — so the rule is testable before a caller
exists to print it.

The grouping rule is one unit per seam boundary: a unit ends where the next one
would have to change a different package's interface. That is why the consent
refactor (issue 4) is separate from the project-install plan that composes it
(issue 5), and why the provenance fields (issue 6) are separate from the write
that populates them.

Two chains run independently — discovery (1, 2, 3) and consent (4, 5, 6, 7) —
plus the escalation predicate (8), which depends on neither because the command
layer already loads the user configuration before it builds the runner.
Documentation (9) is the only unit with cross-chain dependencies.

## Issue Outlines

### Issue 1: feat(project): discovery seams, the read discipline, and resolved ceilings

**Complexity**: complex

**Goal**: Give `internal/project` the filesystem and identity seams the design's
Key Interfaces section describes, and rebuild `LoadProjectConfig`'s walk on top
of them: the `os.Stat` + `os.ReadFile` pair in `config.go` becomes one
open-without-following-and-without-blocking followed by a device/inode
comparison against the entry's link metadata, a regular-file check on the
descriptor, and a read from that same descriptor; `buildCeilings` resolves
`$HOME` and each `TSUKU_CEILING_PATHS` entry through symlinks before comparing
them against the walk (R5); and a directory the walk cannot examine ends the walk
with no config and no message rather than being treated as holding no config. A
second exported form of `LoadProjectConfig` takes the seam environment, with
today's one-argument form calling it with the production one, and the
centralization lint in `cmd/tsuku/project_config_test.go` plus its canary
`TestOnlyCallerCheckCanFail` are extended to match both forms. The design names
neither the environment type nor the second entry point and does not say whether
the seams are one interface or a struct of function values, so those names are
the implementer's to pick; the criteria below are written against behavior. No
trust clause, no home early return and no `RefusedError` land here — those are
issue 2 — so on an entry that is a symlink this issue lands only the mechanical
half of the design's symlink sequence, enough that a symlinked `.tsuku.toml`
keeps loading as it does today.

**Acceptance Criteria**:
- `internal/project` exports a discovery environment carrying the operations the
  design lists: resolve a path through symlinks, read link metadata exposing
  owner uid, mode, file type, device and inode, read a link's target, enumerate a
  directory's ancestors, and open a path without following symlinks and without
  blocking, yielding a handle that reports its own metadata, can be read, and can
  be closed. It also carries the invoking user's id as a plain value, passed
  rather than read from the process, following the `configPermissionCondition`
  precedent.
- A production constructor returns the environment backed by `os` and `syscall`,
  and is what the existing one-argument `LoadProjectConfig(startDir string)`
  passes to the new form.
- `LoadProjectConfig(startDir string) (*ConfigResult, error)` keeps its signature
  and calls a second exported form that takes the environment explicitly; every
  filesystem access the walk makes goes through it.
- A unit test drives the second form with a fake environment whose ancestor
  enumerator ends at a synthetic root under `t.TempDir()`, and asserts the walk
  does not climb into the real filesystem above the temporary directory.
- A unit test supplies an environment that fails the test on any filesystem
  access not routed through the seams, and asserts discovery reads no config
  bytes before it decides to read the file (R25).
- `parseConfigFile` no longer calls `os.ReadFile` on a path. The bytes it decodes
  come from a handle opened with
  `os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK`, matching the idiom and its
  reasoning in `copyProgramFile` (`internal/actions/install_program_files.go`).
- After opening, the walk stats the handle and compares its device and inode
  against the link metadata read before the open; a mismatch does not parse the
  bytes. A unit test swaps the file between the metadata read and the open
  through the seam and asserts the swapped file is not parsed (R4).
- The regular-file check is made on the handle whose bytes are parsed: a FIFO, a
  directory, a socket or a device node at `.tsuku.toml` is not parsed, and a
  symlink to a regular file still loads. A unit test creates a FIFO at the config
  path and asserts the load returns without parsing it and without blocking.
- `buildCeilings` resolves `$HOME` and every `TSUKU_CEILING_PATHS` entry through
  the environment's path resolver before they enter the ceiling set, and the walk
  compares resolved paths. A unit test with `HOME` set to a symlinked path, and a
  second with a symlinked `TSUKU_CEILING_PATHS` entry, places a config above the
  real directory and asserts it is not found (R5).
- A `TSUKU_CEILING_PATHS` entry that cannot be resolved is kept in the ceiling set
  as written after `filepath.Clean`, and a unit test asserts an entry naming a
  directory that does not exist is ignored without error (R5).
- A directory the walk cannot examine — the entry's metadata read fails for a
  reason other than "does not exist" — ends the walk immediately, returning
  `nil, nil`, and writes nothing to any stream. A unit test builds a directory the
  seam reports as unexaminable and asserts no config is returned and nothing is
  printed (R5). An entry that simply does not exist still continues the walk to
  the parent.
- `TestLoadProjectConfigReportingIsTheOnlyCaller` flags a direct call to either
  exported form, so a caller routing around `loadProjectConfigReporting` via the
  environment-taking form is caught.
- `TestOnlyCallerCheckCanFail` asserts `cmd/tsuku/project_config.go` literally
  contains a direct call to whichever form the helper now uses, so the exclusion
  in the lint above still hides a real occurrence.
- Existing `internal/project` discovery tests pass unchanged, including the
  symlink-resolution, parent-traversal and ceiling-path tests and the `ParseError`
  tests; a `.tsuku.toml` that is itself a symlink to a regular file still loads.
- `go test ./...` passes

**Dependencies**: None

**Type**: code

**Files**: internal/project/config.go, internal/project/config_test.go, internal/project/env.go, internal/project/env_test.go, cmd/tsuku/project_config.go, cmd/tsuku/project_config_test.go

---

### Issue 2: feat(project): decide which configs discovery applies outside the resolved home

**Complexity**: complex

**Goal**: Add the trust decision and the `RefusedError` type to
`internal/project`, built on the seams, read discipline and resolved ceilings
from issue 1. Outside a resolved home, three clauses judge the file the walk
actually opens; below a qualifying home an early return skips the decision
entirely so R1 behavior is unchanged. A failed clause returns a typed refusal
naming the file instead of a config, with nothing parsed, and the walk stops
there. Nothing renders the refusal yet — that is issue 3.

**Acceptance Criteria**:
- The home early return fires only when all four preconditions hold — `HOME` is
  set, resolves through the seam, is not the filesystem root, and is owned by the
  invoking uid — and only for a config whose resolved path is strictly below it.
  When it fires, none of the three clauses runs and the outcome matches today's
  behavior (R1).
- With `HOME` unset, with `HOME=/`, and with `HOME` owned by another uid through
  the seams, there is no resolved home: the clauses apply to every path, and the
  L6 layout is refused in each of the three cases.
- Ownership clause: a config owned by the invoking uid or by uid 0 is accepted on
  ownership alone, and the ancestor chain is not enumerated at all in that case
  (L1, L2, L4, L11). Otherwise the config is accepted only when every group- or
  other-writable directory from the config's own directory up to the filesystem
  root is owned by *that config's owner* (L3, L12). A directory whose metadata
  cannot be read counts as writable, and no ancestor is exempted for carrying the
  sticky bit.
- A test pins the asymmetry that separates the two ownership-identical rows: with
  a root-owned, world-writable, sticky `/tmp` stand-in as the parent, a config
  owned by a third uid is refused (L8), while the same config under a root-owned
  0755 parent is found (L3). A second assertion covers the trap directly — a
  root-owned writable ancestor does not satisfy the chain even though root is
  trusted as an owner.
- Directory clause: a config whose directory is world-writable without the sticky
  bit is refused (L13), unconditionally and reading the world bit only, so a
  config at mode 0664 in a 0775 directory the invoking user owns is found (L11).
- Mode clause: a config that is itself world-writable is refused, and its
  `RefusedError` remedy names mount options as well as `chmod`.
- All three clauses must pass and none short-circuits another; the ownership
  clause's shortcut is a shortcut past the ancestor chain only. A test asserts a
  config owned by the invoking user inside a world-writable non-sticky directory
  is refused, and a test asserts a config owned by the invoking user directly in
  the sticky world-writable `/tmp` stand-in is found.
- Symlink sequence, on the open-without-following failure a link produces: read
  the link, apply the ownership and directory clauses at the link's own location,
  resolve the target, apply all three clauses at the target's location, then open
  the target without following and compare device and inode against the target's
  own metadata before reading. Tests cover a foreign-owned link, a foreign-owned
  target, and a link from an acceptable location to a target in a world-writable
  directory — all refused (L9) — and a link and its target both inside an accepted
  checkout, found (L10).
- The mode clause runs at the target only, so a plain `ln -s` inside an accepted
  checkout is found on both Linux (where a link's `lstat` mode is 0777) and macOS
  (where it is umask-derived); a test asserts the found outcome under both mode
  values injected through the metadata seam, so the same layout cannot diverge by
  platform.
- A `.tsuku.toml` that is a symlink to another symlink is refused rather than
  walked; a symlinked *directory* component of the target's path is resolved
  rather than refused, and a target reached through such a component is found when
  the clauses pass at the resolved location.
- `RefusedError` carries the refused path, the directory holding it, the reason
  and the remedy, and the load returns it in place of a `*ConfigResult` with
  nothing parsed. A refused file containing invalid TOML produces the
  `*RefusedError` and no `*ParseError` and no parse diagnostic (R3); `errors.As`
  distinguishes it from `*ParseError`.
- A refusal stops the walk: with a refused config below a second, acceptable
  config in a directory above it, neither config is applied and the error is the
  refusal from the lower file (R3).
- A table-driven unit test covers every row L1 through L13, built under
  `t.TempDir()` through issue 1's seams and running as a non-root user with no
  second account: foreign and root owners and the invoking uid are injected rather
  than created, every "found" row loads its config, and every "refused" row
  returns a `*RefusedError`. The ancestor enumerator is bounded at the fixture's
  synthetic root.
- The `/tmp` stand-in fixture sets the sticky bit with the mode constant, since
  `os.Mkdir(path, 0o1777)` does not set it in Go; a test asserts the fixture
  directory actually reports sticky, so a fixture that silently lost the bit
  cannot make the L8 row pass for the wrong reason.
- On every refused row, no bytes are read from the config: the decision path
  performs no content read through the seam before returning the refusal (R25).
- `go test ./...` passes

**Dependencies**: <<ISSUE:1>>

**Type**: code

**Files**: internal/project/trust.go, internal/project/trust_test.go, internal/project/refused_error.go, internal/project/refused_error_test.go, internal/project/config.go, internal/project/config_test.go

---

### Issue 3: feat(cmd): report a refused .tsuku.toml across the five callers

**Complexity**: complex

**Goal**: Render `project.RefusedError` everywhere a config is loaded, so a
refusal reaches stderr exactly once and never reaches the stream the shell
evaluates. The three existing parse-failure renderers generalize to carry both
errors rather than gaining parallel refusal-only twins, `tsuku install` and
`tsuku shim install` fail on a refusal with the existing security-block exit
code, and `internal/activation` gains a refusal branch beside its `ParseError`
one. `loadProjectConfigReporting` and `ComputeActivation` each gain a second form
taking discovery's environment, which is what lets the layout table be exercised
in process through the commands and the shell hook.

**Acceptance Criteria**:
- The parse diagnostic function generalizes to return one line for
  `*project.RefusedError` as well as `*project.ParseError`, naming the refused
  file, the reason and the remedy the type carries; the report helper generalizes
  with it. No second, refusal-only diagnostic function or report helper is added.
- `loadProjectConfigReporting` prints the refusal line to stderr itself,
  unconditionally rather than through the quiet-aware helper, so `tsuku run` still
  reports it even though it discards the load error; a test asserts `tsuku run`
  under a refused config prints the line on stderr on every invocation and
  resolves the command exactly as it does with no config present (R6, R9).
- Under a refused config, `tsuku install` with no arguments and `tsuku shim
  install` with no arguments exit with `ExitForbidden` (14) rather than
  `ExitGeneral` or a usage code, and neither prints its "no `.tsuku.toml` found"
  message. The refusal line appears exactly once (R6, R10).
- `ComputeActivation` gains a `*project.RefusedError` branch beside the existing
  `*project.ParseError` one, returning `PATH` and `PrevPath` from the previous
  path so stdout adds nothing to `PATH`, `Dir` set to the refused file's directory
  (the existing tracking variable; no new tracked variable is added), `Active`
  true and the stamp recorded (R7).
- On that branch `Entered` is true when the refused file's directory differs from
  the current one **or** when the refusal changes `PATH`. The widening is confined
  to the refusal branch: the `ParseError` branch and the found-config return keep
  their present meaning, because `Entered` is also read by the activation
  reporter.
- `tsuku hook-env` under a refused config exits 0, emits no refusal text on
  stdout, and reports through the quiet-aware path: a test that feeds each call's
  exported tracking variables into the next call's environment asserts the line
  prints on the first call, not on a second call in the same directory, not on a
  third call from a subdirectory, and again after a call from an unrelated
  directory; with `--quiet`, no call prints it (R7).
- Starting from an activated project, a `tsuku hook-env` call from a refused
  location restores the previous `PATH` — the only `PATH` change — prints the
  refusal, and the next call from the same location prints nothing (R7).
- `tsuku shell`'s own branch generalizes from the parse-failure test so a refusal
  returns after printing instead of falling through to its "no `.tsuku.toml`
  found" block and a non-zero exit: `tsuku shell` exits 0, prints the refusal on
  each of two invocations regardless of `--quiet`, and its stdout adds nothing to
  `PATH` (R8).
- A test with a directory name containing a newline, `$`, a backtick and an escape
  sequence asserts the refusal is exactly one stderr line with the path quoted and
  no raw control characters, that the line states both a reason and an action, and
  that `tsuku run`, `tsuku install`, `tsuku shell` and `tsuku hook-env` write no
  refusal text to stdout (R6).
- `loadProjectConfigReporting` gains a second form taking discovery's environment,
  with the existing one-argument form delegating to it with the production
  environment; `ComputeActivation` gains the same second form, with the existing
  signature delegating. No call site outside the new tests changes shape beyond
  this (R26).
- In-process tests drive a foreign-owned refused layout through `tsuku install`,
  `tsuku shim install`, `tsuku run`, `tsuku shell` and `tsuku hook-env` via those
  second forms, passing in `go test -short` as a non-root user with no second
  account (R26).
- The centralization lint and its canary still pass with both helper forms
  present, the canary still matching a literal direct call.
- `go test ./...` passes

**Dependencies**: <<ISSUE:2>>

**Type**: code

**Files**: cmd/tsuku/project_config.go, cmd/tsuku/activation_report.go, cmd/tsuku/hook_env.go, cmd/tsuku/shell.go, cmd/tsuku/cmd_run.go, cmd/tsuku/install_project.go, cmd/tsuku/cmd_shim.go, cmd/tsuku/exitcodes.go, internal/activation/activate.go, cmd/tsuku/project_config_test.go, cmd/tsuku/shell_test.go

---

### Issue 4: refactor(install): split distributed source handling into consent primitives

**Complexity**: testable

**Goal**: Rebuild `ensureDistributedSource` from three primitives — classify a
source (validate, look it up, apply the `strict_registries` refusal, with no
network call and no write), add a session-only provider, and write the registry
entry — so a later caller can decide about a source long before anything is
written. The command-line install path composes them in today's order for a real
install, leaving its behavior unchanged including the registration it performs
with no terminal, and composes the first two without the write under `--dry-run`.
Classification answers "is this source registered?" from the configured
registries in `config.toml` rather than from the loader's live provider list, so
a briefly unreachable repository cannot make a registered source read as
unregistered. The terminal check becomes a substitutable package-level value and
the prompts share one line reader, which is what makes every consent criterion in
this feature exercisable in process.

**Acceptance Criteria**:
- `cmd/tsuku/install_distributed.go` exposes three separate primitives: one that
  classifies a source, one that adds a session-only provider for it, and one that
  writes its registry entry. Each is callable independently and none of them calls
  the others.
- The classify primitive validates the source, loads the user config, and reports
  whether the source is present in the configured registries. It returns the
  existing `strict_registries` error, with its current message text and its
  `tsuku registry add <source>` remedy line, unchanged.
- The classify primitive performs no network call and never saves the user config;
  a unit test asserts that calling it against a fresh `config.toml` leaves that
  file byte-for-byte unchanged.
- Classification does not read the loader's provider list: a unit test with
  `owner/repo` present in the configured registries and a loader holding no
  provider for it classifies the source as registered.
- The write primitive is the only place on any install path that saves
  `config.toml`. It writes the source URL and `AutoRegistered: true` and prints
  the "Auto-registered source" line to stderr, matching today's output.
- The session-provider primitive keeps `addDistributedProvider`'s current
  behavior, including its no-op when a provider for the source already exists.
- `ensureDistributedSource` keeps its signature and composes the primitives in
  today's order: classify, then for an already-registered source add the provider
  and return, then the non-auto-approve interactive prompt, then write, then add
  the provider.
- `TestEnsureDistributedSource_NonTTY_AutoApproves`,
  `TestEnsureDistributedSource_AutoApprove_SkipsPrompt`,
  `TestEnsureDistributedSource_AlreadyRegistered` and
  `TestEnsureDistributedSource_InvalidSource` pass without modification to their
  assertions (R17).
- A non-dry-run `tsuku install owner/repo:tool` with stdin not a terminal still
  registers the source with `auto_registered = true` and prints "Auto-registered
  source" on stderr (R17).
- In the distributed branch of `cmd/tsuku/install.go`, when `--dry-run` is set the
  source handling composes classify plus the session provider and never reaches
  the write primitive, whatever the terminal state and whatever `--yes` / `--force`
  are set to (R11).
- Under `--dry-run` the command-line path asks no registration question, and an
  unregistered source is named on stderr as not registered (R11).
- With no `config.toml` present, `tsuku install --dry-run owner/repo:tool` with
  stdin closed does not create one; with one present it is byte-for-byte unchanged
  afterwards (R11).
- The dry-run branch still sits below the source handling, so `runDryRun`
  resolves the qualified name through the session provider the source handling
  just added.
- The terminal check becomes a package-level substitutable value in `cmd/tsuku`,
  in the shape of the existing `stdinIsTerminal` precedent, and every current call
  site resolves through it (R26).
- A test substitutes that value to drive both a terminal and a missing terminal in
  process, with no real tty and no root.
- One shared line reader serves the prompts, replacing the per-prompt
  `bufio.NewReader(os.Stdin)` at each site.
- A test that scripts two answers into the substituted stdin as two lines has both
  lines consumed in order by two successive prompts, which the per-prompt readers
  lose today.
- `go test ./...` passes

**Dependencies**: None

**Type**: code

**Files**: cmd/tsuku/install_distributed.go, cmd/tsuku/install_distributed_test.go, cmd/tsuku/install.go, cmd/tsuku/install_project.go, cmd/tsuku/create.go, cmd/tsuku/config.go, cmd/tsuku/install_sandbox.go, cmd/tsuku/install_test.go

---

### Issue 5: feat(install): defer project source registration behind consent

**Complexity**: complex

**Goal**: Replace the project install's source pre-scan with a plan object that
classifies each unique source, asks about each unregistered one after the tool
list and before the existing "Proceed?" confirmation, and writes every approved
source in a single save after the user proceeds and before any recipe fetch. A
source left unapproved skips only its own tools while the rest of the declared
tools still install, and the command exits with a new needs-approval code that
outranks the install-failure codes. A dry run classifies, lists each unregistered
source with its declaring file on stderr, and writes nothing whatever the
terminal and flags.

**Acceptance Criteria**:
- A project source plan type in `cmd/tsuku` holds, per unique source, its
  classification state, the names of the tools declared from it, its approval and
  its error, and exposes classify, consent, commit and activate steps. The project
  install uses it in place of the current pre-scan loop; a source declared by
  several tools is classified once.
- Classify calls issue 4's classify primitive for each unique source, makes no
  network call and writes nothing. An invalid source or a `strict_registries`
  refusal keeps today's warning text and marks that source's tools failed, so the
  `strict_registries` functional scenario passes unchanged and its one refused
  tool still produces the install-failed code.
- The consent step takes its inputs as an explicit struct (the `--yes` flag, a
  terminal-check func, and an ask func), reads no environment, and is invoked
  after the tool-list output and before the "Proceed?" gate. A test asserts
  `CI=true` in the environment does not register a source on a run with no
  terminal and no `--yes` (R22).
- The source prompt goes to stderr, names the source and the resolved absolute
  path of the declaring `.tsuku.toml`, lists the tools declared from that source,
  defaults to no, and treats an empty answer as a decline (R13).
- A test driving issue 4's substitutable terminal check and shared line reader
  with scripted input asserts the source prompt is asked before "Proceed?" and
  that a scripted two-line answer reaches both prompts.
- Commit writes every approved source in one save of the user config, called after
  the "Proceed?" gate passes and before any recipe fetch in the install loop.
  Declining the source prompt, and a yes at the source prompt followed by a no at
  "Proceed?", each leave `config.toml` byte-for-byte unchanged; the "Proceed?"
  decline still exits with the user-declined code (R14).
- Activate builds the session providers for approved and already-registered
  sources; a save error or a provider error marks that source's tools failed with
  a warning, as a registration error does today.
- The per-tool result type gains a needs-approval status, distinct from installed
  and from failed. Tools whose source was left unapproved — declined at the
  prompt, or no consent and no terminal — are given that status and never
  attempted, while every other declared tool is still installed. A test with a
  default-registry tool, a registered source's tool and an unregistered source's
  tool, run with no terminal and no `--yes`, asserts exactly that split (R15).
- The summary buckets needs-approval explicitly instead of folding it into the
  installed bucket, and reports the skipped count on its own line.
- The structured output reports a skipped tool with a needs-approval status and
  carries `exit_code: 16`. In a run that both skips a source and fails another
  tool, the JSON carries the skipped tool and the failed tool as distinct statuses
  and the failed tool keeps its error string (R15).
- `ExitNeedsApproval = 16` is added to `cmd/tsuku/exitcodes.go` beside the existing
  constants, with a doc comment saying a project-named source needs the user's
  approval before it is registered and its tools were skipped. A test asserts the
  constant's value alongside the existing partial-failure test.
- Exit-code precedence: whenever any source was left unapproved the command
  returns 16, including when other tools failed and the run would otherwise have
  returned the install-failed or partial-failure code. The failure is still named
  on stderr and in the structured output (R15).
- When every declared tool belongs to a source that needs approval, "Proceed?" is
  not asked, nothing is written, and the command exits 16.
- One stderr block per skipped source is printed after the summary, naming the
  source, the declaring file, the skipped tools, and both ways to approve:
  `tsuku install --yes` and `tsuku registry add <source>`. The wording
  distinguishes "no terminal to ask on" from "was not approved" at the prompt. The
  declaring file and the tool names are quoted the way the refusal line is, and a
  test using a directory name containing a newline, `$`, a backtick and an escape
  sequence asserts no raw control characters reach stderr and that nothing of this
  block is written to stdout (R15).
- The project dry run classifies, never runs the consent or commit steps, never
  prompts for registration, and prints one stderr note per unregistered
  project-named source naming the source and the declaring file. It still
  activates session providers so previews resolve, and its exit codes keep
  describing resolution only. Tests cover: with an injected terminal, with
  `--yes`, and with `--force`, no prompt is shown and `config.toml` is unchanged;
  with no `config.toml` present, none is created (R11).
- `go test ./...` passes

**Dependencies**: <<ISSUE:4>>

**Type**: code

**Files**: cmd/tsuku/install_project.go, cmd/tsuku/install_project_test.go, cmd/tsuku/install_distributed.go, cmd/tsuku/exitcodes.go

---

### Issue 6: feat(userconfig): record how a project-caused source registration was approved

**Complexity**: testable

**Goal**: Give the registry entry type two optional string fields recording how a
project-caused registration was approved and the resolved absolute path of the
`.tsuku.toml` that declared the source, and render them in `tsuku registry list`.
Issue 5's deferred write supplies both values and populates them once at
registration; nothing later updates them. Entries written before this change keep
loading, keep `auto_registered = true` and its "(auto-registered)" annotation, and
list exactly as they do today.

**Acceptance Criteria**:
- The registry entry type gains two flat `omitempty` string fields, one for how
  the registration was approved and one for the declaring config's path, alongside
  the existing URL and auto-registration fields. Not a nested table: a nested table
  without `omitempty` writes a bare empty header into every pre-existing entry on
  the first save, which the design rejects.
- The approval field's doc comment states the closed vocabulary — a yes at the
  interactive source prompt, and `--yes` — and that it is empty for entries added
  by `tsuku registry add`, by a command-line-named install, or before the field
  existed. The path field's states that it is the absolute, symlink-resolved path
  of the declaring `.tsuku.toml`, written once at registration and never updated.
- The project install's deferred save (issue 5) writes both fields on each
  approved source, with `AutoRegistered: true` unchanged. A source already present
  in the configured registries when the write runs is left as it is, so a second
  project naming the same source does not rewrite its record (R16).
- `autoRegisterSource` and `tsuku registry add` leave both fields zero-valued, so
  `omitempty` emits neither key and the bytes those two paths write to
  `config.toml` are unchanged (R17).
- A round-trip test saves entries with the fields set and with them unset,
  reloads, and asserts the values survive, that an entry with them unset emits
  neither key, and that a sibling entry's keys are unaffected.
- A backward-compatibility test decodes a `config.toml` in the shape the previous
  release wrote — a registry table with only the URL and the auto-registration
  flag — and asserts the decoder reports zero undecoded keys, both old fields are
  intact, and both new fields are empty. Saving that config back does not add
  either key to the entry (R16).
- The registry listing keeps its per-entry first line byte-for-byte: the same
  format, the same default URL fallback, and the same "(auto-registered)" suffix
  driven by the auto-registration flag alone. The existing registry-list tests
  pass unchanged.
- An entry carrying provenance gets one additional indented continuation line
  after its first line; an entry with both fields empty gets no second line. The
  approval renders in words rather than as the stored token, and a value matching
  neither vocabulary entry (a hand edit) is rendered quoted rather than dropped or
  assumed.
- The declaring path is rendered quoted, because it is a string an
  attacker-controlled repository chose. A test with a path containing a newline,
  an ANSI escape and a double quote asserts the rendered line contains no raw
  control characters and that the listing is still one line per entry plus one
  provenance line.
- An end-to-end assertion over `tsuku install` in a project whose `.tsuku.toml`
  names an unregistered source: after an interactive yes the entry records the
  prompt, after `--yes` it records the flag, both record the resolved absolute path
  of that `.tsuku.toml`, both keep `auto_registered = true`, and `tsuku registry
  list` shows the approval, the path and the "(auto-registered)" annotation. A
  second install from a different project declaring the same source leaves the
  record byte-identical (R16).
- `go test ./...` passes

**Dependencies**: <<ISSUE:5>>

**Type**: code

**Files**: internal/userconfig/userconfig.go, internal/userconfig/userconfig_test.go, cmd/tsuku/registry.go, cmd/tsuku/registry_test.go, cmd/tsuku/install_distributed.go, cmd/tsuku/install_project.go, cmd/tsuku/install_project_test.go

---

### Issue 7: fix(install): stop `--force` consenting to a project-named source registration

**Complexity**: simple

**Goal**: Narrow `--force` so it no longer approves registering a recipe source
that only a project `.tsuku.toml` named. The project install path currently
passes `installYes || installForce` as the auto-approve argument; after issue 5
that path asks through the plan, so `--force` stops feeding the decision. The
command-line path is untouched: R17 keeps a source the user typed registering
exactly as today, `--force` included. The flag keeps its other meanings on both
paths — suppressing security warnings, and replacing a tool already installed from
a different source.

**Acceptance Criteria**:
- The project-install path's consent input is `installYes` alone; `installForce`
  no longer reaches it.
- `tsuku install --force` in a project whose `.tsuku.toml` names an unregistered
  source writes nothing to `config.toml`, skips that source's tools, installs the
  rest, and exits 16 with the message naming both approval routes (R12).
- `tsuku install --yes` in the same project registers the source, as does an
  interactive yes.
- `tsuku install --force owner/repo:tool` registers the source with
  `auto_registered = true` and prints "Auto-registered source" with no terminal
  attached, exactly as today, and
  `TestEnsureDistributedSource_NonTTY_AutoApproves` passes unchanged (R17).
- `tsuku install --force` still suppresses the security warning path and still
  replaces a tool installed from a different source, including during a project
  install.
- The `--force` help string no longer claims it proceeds without prompts: it did
  not skip the install confirmation before this change and does not now.
- `go test ./...` passes

**Dependencies**: <<ISSUE:5>>

**Type**: code

**Files**: cmd/tsuku/install_project.go, cmd/tsuku/install.go, cmd/tsuku/install_project_test.go, cmd/tsuku/install_test.go

---

### Issue 8: feat(autoinstall): withhold the run escalation for an unregistered source

**Complexity**: testable

**Goal**: Narrow the declaration-caused consent raise in `tsuku run` so it no
longer waives the prompt for a declaration whose winning configuration key names
a recipe source the user has not registered. `elevate` gains one input — whether
that key's source component is absent from the user's configured registries —
supplied by the command layer as a predicate on the `Runner` struct so
`internal/autoinstall` keeps taking no dependency on user configuration.
Everything else in the consent chain stays exactly as it is, and the prompt and
no-terminal messages gain the fact the reader needs to answer the question.

**Acceptance Criteria**:
- `Runner` gains an exported predicate field reporting whether a source is present
  in the user's configured registries. Its doc comment follows the convention
  `IsTerminal` sets on the same struct: it states that a nil function means no
  source is registered, and says why withholding the raise is the fail-closed
  outcome.
- `elevate` takes the new input alongside the mode, the origin and the declared
  flag, and raises the unset default to auto only when the declaration's key
  qualifies. It raises nothing for any other origin, exactly as today (R18, R21).
- `Runner.Run` derives the input from the declaration's configuration key: a key
  that is not org-scoped always qualifies; an org-scoped key qualifies only when
  the predicate reports its source registered (R18).
- `cmd/tsuku/cmd_run.go` wires the predicate from the user config it already
  loads, testing membership by exact string comparison of the configured key. It
  does not read the loader's live provider list, and it adds no provider and
  writes nothing (R23).
- A table test on `elevate` asserts the returned mode and origin — not the
  presence of a prompt — over: declared with an unregistered source and the unset
  default (not raised); declared with a registered source and the unset default
  (raised); declared with no source component (raised); and each non-default
  origin unchanged whichever way the source input falls.
- A test asserts the nil-predicate default: with the predicate unwired, a
  declaration carrying an org-scoped key is not raised, while one carrying a plain
  key still is.
- A run test asserts that a declared command whose key names an unregistered
  source reaches the unraised confirm path: with no terminal it prints the message
  and returns the not-interactive error, installing and executing nothing.
- A test asserts a registered source is honored whichever route registered it —
  the predicate is the only input, so a source present in the configured
  registries raises regardless of the auto-registration flag or any provenance
  field on the entry (R18).
- A test pins the collapse limit rather than leaving it to be discovered: a config
  declaring the same tool both plainly and with an unregistered source produces
  one declaration carrying the plain key, so the run is raised and the
  unregistered-source message is absent from stdout and stderr.
- When the raise is withheld at a terminal, the prompt names the source the key
  names, says it is not registered, and states that `tsuku run` resolves the
  recipe through the user's own configured sources rather than the named one. A
  test asserts all three parts, and asserts the third unconditionally rather than
  only for a key whose bare name the default registry also carries (R19).
- The no-terminal message carries the same three facts when the withheld raise
  meets a closed stdin, and keeps naming only the escape hatches that work from
  that state (R19).
- A test asserts that a `tsuku run` which takes the auto raise leaves the
  configured registries exactly as it found them: no provider is added to the
  loader during the run, and `config.toml` is byte-identical afterwards (R23).
- Existing behavior is unchanged and covered by the tests already in the
  elevation, bounded-elevation and terminal-check suites: explicitly set modes,
  the `TSUKU_AUTO_INSTALL_MODE` restriction, the mode-lowering gates including the
  config-permission gate, the already-installed fast path, and the disclosure line
  for a qualifying declaration, whose text matches the current output exactly
  (R20, R21).
- `go test ./...` passes

**Dependencies**: None

**Type**: code

**Files**: internal/autoinstall/autoinstall.go, internal/autoinstall/run.go, internal/autoinstall/elevation_test.go, cmd/tsuku/cmd_run.go, cmd/tsuku/run_bounded_elevation_test.go

---

### Issue 9: docs: correct discovery, consent and exit-code documentation for the new trust rules

**Complexity**: simple

**Goal**: Every statement in the tree about how discovery finds a config, what
`--force` does, and which exit codes an install can return is now wrong or
incomplete. This issue fixes the anchors the design's documentation phase names
and carries the release-note text the requirements ask for (R24, R24a).

**Acceptance Criteria**:
- `docs/designs/current/DESIGN-shell-env-activation.md`: the duplicated mitigation
  line claiming `TSUKU_CEILING_PATHS` prevents traversal into untrusted parent
  directories is removed. The corrected statement above it, which names
  tsukumogami/tsuku#2555, stays and is the only one (R24).
- `plugins/tsuku-user/skills/tsuku-user/SKILL.md`, the discovery sentence:
  rewritten to say what the walk does outside `$HOME`, in terms a user can act on
  — which configs are applied, what a refusal looks like, and the remedies the
  refusal names (R24).
- Same file, the consent-mode list entry describing the raise to auto for a
  declared tool: narrowed to say the raise is withheld when the declaration's key
  names a source the user has not registered (R24).
- Same file, the exit-code table: entry 14 covers a refused config as well as
  running as root, and a row for 16 is added in numeric order.
- `docs/guides/shell-integration.md`, the project-install exit-code table
  (0, 6, 15): gains a row for 16.
- `docs/guides/shell-integration.md`, the ceiling-variable section: its example
  does not work on macOS today because entries are compared unresolved while the
  walk is resolved. With ceilings resolved (issue 1) the example is true; the
  prose claiming exact matching is checked against the new behavior and corrected
  if it no longer holds.
- The `--force` help string carries no claim that the flag proceeds without
  prompts. (Landed with issue 7; verified here.)
- The release-note text R24a requires is carried in Part 1 of the pull request
  body, marked so it can be lifted into the draft release: every source already
  present in the user's configured registries is trusted for silent installs from
  the upgrade onward, including any a project config caused to be registered
  before the change, and the reader is pointed at `tsuku registry list` and
  specifically at the entries it marks "(auto-registered)". This repository keeps
  no release-note file — `changelog` and `release` are both disabled in
  `.goreleaser.yaml`, and the release-prepare workflow pulls the tag's notes from
  the draft release — so the PR body is where the text has to live for whoever
  cuts the release.
- No committed file references a `wip/` path.

**Dependencies**: <<ISSUE:3>>, <<ISSUE:5>>, <<ISSUE:6>>, <<ISSUE:7>>, <<ISSUE:8>>

**Type**: docs

**Files**: docs/designs/current/DESIGN-shell-env-activation.md, plugins/tsuku-user/skills/tsuku-user/SKILL.md, docs/guides/shell-integration.md, cmd/tsuku/install.go

## Dependency Graph

## Implementation Sequence

`execution_mode` is `single-pr`, so no merge-order diagram is drawn: the work
lands as one pull request and the ordering lives in each outline's
**Dependencies** declaration.

**Three units can start at once:** issue 1 (discovery seams), issue 4 (consent
primitives) and issue 8 (the escalation predicate). Each is the head of an
independent line of work.

**Critical path:** two chains of equal length run in parallel —
1 → 2 → 3 → 9 and 4 → 5 → 6 → 9. Issue 7 hangs off issue 5 alongside issue 6 and
can be done in either order relative to it. Issue 9 is the only unit with
cross-chain dependencies and is therefore last.

**Why the seams come first in both chains.** Issue 1 and issue 4 carry the test
infrastructure every acceptance criterion downstream of them needs — the
filesystem and ownership seams with a bounded ancestor enumerator, the
substitutable terminal check, one shared line reader, and the in-process command
drivers issue 3 adds on top. None of the layout cases, and none of the consent
cases, can be exercised as a non-root user with no second account until they
exist, so they are units of their own rather than work smuggled into the units
that need them.

**Commit shape.** Nine units, one pull request (the design's D9). The
intermediate states are visible in the branch and never in a shipped version: the
window between issue 2 and issue 3, where a refusal has a type but no renderer, is
the one worth knowing about.

## References

- `docs/designs/DESIGN-project-config-trust.md` — the accepted design these units
  implement.
- `docs/prds/PRD-project-config-trust.md` — the requirements and acceptance
  criteria each unit cites by number.
- Issues closed: tsukumogami/tsuku#2555, tsukumogami/tsuku#2559,
  tsukumogami/tsuku#2552.
- Related: tsukumogami/tsuku#2571, which is why a non-exact declaration reaches
  the consent decision on every invocation rather than once.
