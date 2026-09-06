---
schema: design/v1
status: Planned
upstream: docs/prds/PRD-shellenv-activation-pins.md
problem: |
  Project activation resolves a declared version by interpolating the declared
  string into a directory name, so only exact pins work and every unhonored
  declaration is dropped silently. Fixing it needs the pin-matching rules, the
  version ordering, and installation state, and `internal/shellenv` can reach
  none of the three: `internal/install` already imports it, so the edge that
  would close the loop is a cycle.
decision: |
  Move `activate.go` out of `internal/shellenv` into a new `internal/activation`
  package, which breaks the cycle because activation and the shell.d cache never
  shared a symbol. Activation then calls the existing pin routines and version
  ordering directly, and reads installation state through a one-decode,
  lock-free accessor behind a narrow interface. Skip data becomes typed reasons
  with differing payloads, rendered by `cmd/tsuku`. A third tracking variable
  records installation state's stat so a remediation takes effect at the next
  prompt without re-reading state on every prompt.
rationale: |
  The cycle is a filing accident rather than a dependency: nothing in
  `internal/install` touches activation, and nothing in activation touches the
  rest of `shellenv`. Splitting at that existing seam costs six lines and
  unlocks all three needs at once, where extracting the pin routines alone
  unlocks two and leaves the state read needing a second mechanism. Lock-free is
  safe because state is published by atomic rename, and it is necessary because
  the shared lock has no non-blocking variant. Typed reasons with different
  payloads make a collapse into one templated message a visible deletion of
  information rather than a tidy-up.
---

# DESIGN: Project activation for non-exact version pins

## Status

Planned

## Upstream Design Reference

Parent: [DESIGN: Shell Environment Activation](current/DESIGN-shell-env-activation.md)

That document specifies the behavior this design changes, and contradicts itself
about it: line 196 states a project declaring `go = "1.22"` gets
`$TSUKU_HOME/tools/go-1.22.5/bin` on PATH, while the algorithm at line 133
specifies `{name}-{version}` interpolation, which can only produce
`tools/go-1.22/bin`. Line 212 reasons about the same declaration as though the
string named an exact release. Its Mitigations section records "print a warning
to stderr when skipping uninstalled tools", never built.

Amending it is in scope here (PRD R39) and the amendment is specified under
Implementation Approach.

## Context and Problem Statement

`internal/shellenv/activate.go` resolves a declared version by building
`$TSUKU_HOME/tools/<name>-<version>/bin` from the declared string and skipping
the entry when that directory is absent. So `"latest"` looks for
`tools/nodejs-latest/bin`, a prefix looks for `tools/nodejs-26/bin`, and neither
is ever created. The empty form doesn't reach the lookup: an explicit branch
skips it, commenting that resolution is "out of scope for the activation
skeleton". `ActivationResult.Skipped` is populated and read by no non-test
caller.

The PRD settles what should happen. This document settles how, and the technical
problem is narrower than the behavior change suggests: **activation needs three
things that all live on the far side of one import edge.**

1. **Pin matching, pin level and version-string validation** —
   `internal/install/pin.go`, which PRD R7 requires activation to call rather
   than copy.
2. **Version ordering** — `CompareVersions` and `SortVersionsDescending` in
   `internal/version`, which PRD R6 requires so `latest` doesn't pick `9.0.0`
   over `10.0.0`.
3. **The installed set** — `internal/install`'s state manager, which PRD R8
   requires as the authority on what is installed.

`internal/shellenv` can reach none of them. `internal/install` already imports
`internal/shellenv` from four non-test files, so the reverse edge is a cycle,
confirmed by writing the import and building:

```
package github.com/tsukumogami/tsuku/internal/shellenv
	imports github.com/tsukumogami/tsuku/internal/install from activate.go
	imports github.com/tsukumogami/tsuku/internal/shellenv from precedence.go: import cycle not allowed
```

`internal/version` is closed for the same reason, since `resolve.go` imports
`internal/install`.

A second problem sits behind the first. Activation runs from a shell prompt hook,
so how it reads installation state matters as much as that it does: the existing
accessor decodes the whole state file per tool queried, and takes a shared lock
that blocks indefinitely with no non-blocking variant in the codebase.

## Decision Drivers

- **PRD R7** — activation must call the shared routines, carrying no copy, so
  activation and installation can't disagree about what a version string means.
- **PRD R32** — activation must never wait on another tsuku process. A shell
  that hangs on `cd` is a worse failure than the one being fixed.
- **PRD R33** — installation state is decoded at most once per activation,
  regardless of how many tools a project declares.
- **PRD R17** — five reporting reasons stay distinguishable as different
  sentences, not one sentence with a substituted noun.
- **The 5 ms budget** the parent design sets for the unchanged-directory path.
  Arguments here are made against that budget, not against today's measured
  `hook-env` cost, which is dominated by tsukumogami/tsuku#2548 and will be fixed.
- **Blast radius.** This is a bug fix on a path that runs on every shell prompt.
  Options that move a lot of code are worse than options that move a little,
  independent of their other merits.

## Considered Options

Four decisions were investigated independently, each against the code, with
every option trialled by compiling and then reverted. The options and their
outcomes are below.

### Decision 1: where the shared routines live

- **A. Extract `internal/install/pin.go` to a stdlib-only leaf package.**
  Trialled and reverted: 7 files, builds and tests clean, and it removes the
  `internal/version → internal/install` edge as a side effect, since those four
  symbols are the only reason that edge exists. Genuinely worthwhile — nothing
  inside `internal/install` uses `pin.go`, so it's a file filed in the wrong
  package. **Rejected for this work because it solves two of three needs.**
  Activation still couldn't read installation state, so A needs pairing with
  dependency inversion or with option E anyway. It belongs in its own change.
- **B. Dependency inversion for all three.** Rejected. `internal/install` can't
  supply the ordering (it doesn't import `internal/version` and can't), so this
  is two interfaces or a struct of function values, threaded through
  `ComputeActivation` and wired in `cmd/tsuku`. R7 is then satisfied by a wiring
  line rather than by a call, which is a weaker guarantee for more machinery.
- **C. Move resolution out of `shellenv` entirely**, leaving it to consume a
  resolved result. Right direction, but it splits activation's logic across two
  packages that must be read together.
- **D. Duplicate the matcher in `shellenv`.** Ruled out by R7's text, which
  forbids carrying a copy. Worth recording that it is *not* ruled out by the
  acceptance criterion — see Decision Outcome — so the rejection stands on the
  right reason.
- **E. Move `activate.go` into a new `internal/activation` package.**
  **Chosen.** Trialled and reverted: excluding the file move, 4 files changed,
  +6/-6.

### Decision 2: how the installed set is read

- **A. A new lock-free bulk accessor** returning versions for a set of tool names
  from one decode. **Chosen.**
- **B. Add `TryLockShared` and report `unreadable` on contention.** Dead on
  arrival: PRD R32 reserves `unreadable` for a read that fails outright, "not for
  one that finds another process writing".
- **C. Keep the per-tool accessor, cache within one activation.** Satisfies R33,
  but the once-ness lives in a convention any future caller can bypass rather
  than in the shape of the call.
- **D. A plan-free read-side projection.** In scope — it writes nothing and
  changes no stored byte — and measured at a 44% cut rather than the 23x the
  research summary implied, because `encoding/json` still lexes every byte of the
  3.0 MB file. Rejected because it costs a second hand-maintained schema for the
  same file with no compiler check that the two agree, and one of the two
  migrations it would have to reimplement is load-bearing: omitting
  `migrateToMultiVersion` makes old-format state files resolve to zero candidates
  and report `no-match` for everything, silently and wrongly. Recorded as the
  available lever if the changed-path cost becomes binding.

Measured, same real 3.0 MB state file, 100 iterations:

| decode shape | 3.0 MB | 6.3 MB synthetic |
|---|---|---|
| stat + read only | 1.2 ms | 2.5 ms |
| full decode + migrations | 39.7 ms | 70.0 ms |
| projection, `Plan` omitted | 22.2 ms | 44.2 ms |
| projection, `Plan` as `json.RawMessage` | 32.0 ms | 58.1 ms |

### Decision 3: the reporting data shape

- **A. Structured facts returned; `cmd/tsuku` renders.** **Chosen.**
- **B. Render to an injected `io.Writer`.** Fewer moving parts, and it puts the
  message next to the code that knows the reason. Rejected on three costs: the
  `--quiet` gate is a package-level flag in `package main` and would have to
  travel; the two entry points have different frequency rules that the computing
  package can't see; and the substring cross-product tests become
  capture-buffer tests needing a whole activation per reason.
- **C. Reuse an existing tsuku diagnostic type.** Searched; nothing fits.
  `internal/errmsg` formats errors and would prefix `Error:` on conditions that
  must exit 0. `internal/notices` is keyed by tool with a once-ever latch and is
  rendered by a path `hook-env` is explicitly skipped from.
  `verify.IntegrityResult`'s free-text `Reason string` is the shape that would
  violate R17 on day one.

### Decision 4: how per-prompt state is carried

- **A. A third exported variable holding a stat of installation state.**
  **Chosen.**
- **B. Fold it into `_TSUKU_DIR`.** Rejected on more than taste: R24's
  suppression rule is defined as the resolved directory differing from
  `_TSUKU_DIR`, so folding a stamp in makes any state change report, which is
  exactly the "anyone installed anything" nag R27 forbids. It also breaks
  `cd $_TSUKU_DIR`, and that variable is documented to users.
- **C. A per-shell file.** Rejected: no stable key (PIDs are reused), no
  shell-exit hook to clean up, a write on the prompt path, and the PRD already
  rejected an on-disk throttle file for the other half of the same mechanism.
- **D. Re-resolve unconditionally.** Ruled out by the PRD's own falsification
  criterion, which requires no decode on an unchanged second invocation. Cost is
  37.4 ms per prompt against a 5 ms budget, and it grows with usage because
  install plans are 97.5% of the file and every install adds one.

## Decision Outcome

**The cycle is a filing accident, and splitting at the seam that already exists
solves all three needs at once.**

`internal/shellenv` holds two unrelated concerns. Every symbol `internal/install`
reaches for is shell.d cache and PATH-precedence machinery —
`ShellDSelection`, `RebuildShellCache`, `ManagedBinaries`, `DisplayName`. Nothing
in `internal/install` touches `ComputeActivation`, `ActivationResult` or
`FormatExports`. Symmetrically, `activate.go` references no symbol defined
elsewhere in its own package; it uses `internal/config` and `internal/project`
and nothing else. The package doc comment describes only the half that leaves:
"Package shellenv computes per-directory PATH activation for tsuku projects."

So the dependency was never "activation depends on install and install depends on
activation". It was "activation is filed next to the shell.d cache, and install
depends on the shell.d cache". Moving `activate.go` to `internal/activation` is
+6/-6 outside the file move, and afterwards activation imports `internal/install`
and `internal/version` directly.

Extracting `pin.go` (Decision 1 option A) remains a good change on its own terms
and should be done separately. The reason is stronger than keeping the diff
small: after the move, `internal/activation → internal/install` is a legal edge,
so the matcher reuse this issue needs no longer requires the leaf package at
all. The extraction stops being a blocker and becomes what it always was on its
merits — a question about where shared version logic lives, with its own
consumers and its own argument. Bundling them would tie a file move to a
shared-utility boundary change, and a revert would take both.

So it isn't deferred to "someday", the trigger for revisiting: when a third
consumer needs the matcher, or when `internal/activation`'s import of
`internal/install` starts dragging in things activation has no business
touching. Today the edge is consistent with the existing layout —
`internal/version` already imports `internal/install` — so it is not a new sin,
just one worth not deepening.

**A note on the acceptance criterion, because it surprised the investigation.**
PRD R7's criterion requires that a change to the shared rule which activation
doesn't follow turns an automated check of activation's own behavior red. That
criterion does not discriminate between the options, because a Go external test
package (`package shellenv_test` inside `internal/shellenv/`) may import
`internal/install` even when `internal/install` imports `internal/shellenv` — the
external test package is compiled separately and nothing imports it. Verified by
building. So the check is constructible under every option including D, and the
decision rests on R7's text and on ongoing cost instead.

The check must **derive** its expectations by calling the shared routines at test
time rather than hardcoding a table. A hardcoded table goes red on both sides
when the shared rule changes, which proves nothing about whether activation
followed. The inputs it drives are currently unexported test data in
`internal/install/pin_test.go`; sharing the *inputs* is not a copy of the
routines and doesn't violate R7.

The other three decisions follow from the PRD with less drama: read once and
lock-free because atomic rename makes it safe and the absent non-blocking shared
lock makes it necessary; typed reasons rendered by `cmd/tsuku` because that is
where `--quiet` already lives and where the two entry points' differing frequency
rules are already known; and a third tracking variable because the mechanism's
correct lifetime is exactly an environment variable's, which is the same argument
the PRD used to reject an on-disk throttle.

## Solution Architecture

### Packages

```
internal/activation/         NEW - activate.go, activate_test.go moved verbatim
  imports internal/config, internal/project,
          internal/install (pin routines), internal/version (ordering)

internal/shellenv/           keeps cache.go, doctor.go, precedence.go,
                             selection.go, filelock_*.go. Doc comment becomes:
                             shell.d init-cache construction, its doctor checks,
                             and PATH-precedence shadowing detection.

internal/install/            gains InstalledVersionsFor on StateManager
internal/project/            gains a typed ParseError carrying the project dir
cmd/tsuku/                   hook_env.go, shell.go: wiring and rendering
```

### The installed-set accessor

On `StateManager`, not `Manager`: `NewStateManager` is one struct literal, where
building a `Manager` drags in registry construction the prompt path must not pay
for.

```go
// InstalledVersionsFor returns the versions installation state records for each
// named tool, from a single decode, without taking the state lock.
//
// Versions come back in unspecified order. Callers that need "newest" must sort
// with version comparison, not string comparison.
//
// A tool with no state entry yields no map entry and no error. A missing state
// file yields an empty map and no error: absent and empty are the same thing
// here, and both mean nothing is installed.
//
// Hidden tools are included. A hidden execution dependency is still installed.
func (sm *StateManager) InstalledVersionsFor(names []string) (map[string][]string, error)
```

Four properties are deliberate. It takes names, which bounds the result and
states the contract in the type. It returns **unsorted**, so it can't inherit the
`sort.Strings` bug in `Manager.InstalledVersions` and so the caller is forced to
reach for version comparison. It doesn't filter hidden tools, because PRD R9
defines a candidate as a version state records. And it's lock-free, calling the
existing unexported `loadWithoutLock` under the in-process read mutex.

`LoadWithoutLock` — exported, no production caller, with two adjacent doc
comments that disagree about what the caller must hold — is not built on. Its
stated contract is true of every internal caller and repurposing the name would
make one symbol carry two contradictory contracts. It should be deleted; it is
`internal/`, so nothing outside can depend on it.

The unexported `loadWithoutLock` this builds on carries the same claim — "caller
must already hold both `sm.mu` and the file lock" — and its six existing callers
all do. After this change one caller deliberately does not, so that comment must
be amended rather than left to become the next contradiction: the file lock is
optional for readers, because `Save` publishes by atomic rename.

**Invariant:** `internal/activation` must never be imported by `internal/install`
or by anything in `internal/install`'s dependency graph. The bug this design
fixes exists because activation logic was filed in a package `internal/install`
depends on. `internal/install` transitively depends on `internal/project`, which
is the other natural home someone might later move resolution to, and that would
cycle for the identical reason.

### The contract

```go
// in internal/activation

// InstalledSet reads what installation state records. Consumer-declared and one
// method wide, so activation imports internal/install only for the three pin
// functions -- which is what lets Decision 1A's later extraction remove that
// edge without changing this API.
type InstalledSet interface {
    InstalledVersionsFor(names []string) (map[string][]string, error)
}

func ComputeActivation(
    cwd, prevPath, curDir, stamp string,
    cfg *config.Config,
    installed InstalledSet,
) (*ActivationResult, error)
```

**The stat and the stamp comparison both happen inside `ComputeActivation`**, not
in `cmd/tsuku`. It already holds `cfg`, so it can reach `state.json`, and putting
the comparison there keeps the whole short-circuit in one testable place.
`Entered` is `result.Dir != curDir`.

**On a parse failure `ComputeActivation` returns a non-nil result *and* a non-nil
error.** That is unusual in Go and is therefore stated rather than left to
taste: it makes `FormatExports` produce the required shape for free, because
`Active=true` with zero bin directories already emits exactly `PATH=base`,
`_TSUKU_DIR=dir`, `_TSUKU_PREV_PATH=base`. The alternative — unwrapping
`project.ParseError` in `cmd/tsuku` and synthesising the exports there —
duplicates `FormatExports` and walks into the shared-code-path hazard the PRD's
falsification criteria name. The parse-failure result carries `Dir`, the base
PATH, the stamp, and `Entered`, so the once-per-entry budget applies to the
diagnostic exactly as it does to the reason messages.

### Resolution

Activation takes the installed-set read behind the interface above, satisfied by
`*install.StateManager`. After the package
split this is not a cycle workaround — the direct import is legal — it is what
makes the decode-counting acceptance criterion testable with a fake.

Per declaration, in order:

1. The **tool name** must be a safe single path segment — charset check plus
   rejection of `..`, `/` and `\`. Failure is `bad-form`. This runs first
   because the name reaches `filepath.Join` whatever the version says; see
   Security Considerations.
2. `install.ValidateRequested` on the version — failure is `bad-form`.
3. `install.PinLevelFromRequested` — `PinChannel` is `channel`.
4. Filter the tool's recorded versions to those satisfying the declaration via
   `install.VersionMatchesPin`, then to those whose bin directory exists.
5. `version.SortVersionsDescending` over what survives; take the first. Where
   `CompareVersions` returns 0, fall back to comparing the raw strings, so the
   order is total.

   This tie-break is **deterministic, not semantically correct**, and the
   distinction should survive into the code comment. Nothing can be correct here
   — the comparator has already said the two versions are equal. What matters is
   that the answer is the same on every prompt and on every machine. Nobody
   should later mistake this for a version-ordering improvement and try to make
   it clever.

   It is needed because `CompareVersions` returns 0 for distinct strings by
   three independent routes: `compareCoreParts` discards its `Sscanf` error, so
   *any* non-numeric component compares as zero; missing components pad to zero,
   so `1.0` ties `1.0.0`; and `splitPrerelease` strips build metadata, so
   `1.0.0+a` ties `1.0.0+b`. Feed that into `sort.Slice`, which Go documents as
   not stable, over a slice built by ranging a map, and the winner among tied
   versions is drawn fresh every prompt with nothing having changed — the same
   command, the same state, a different answer, and no way for a user to
   reproduce their own bug report.
6. No survivors: `missing-files` if at least one *satisfying* version was
   excluded by the directory check, otherwise `no-match`.

Candidacy is filtered before "newest" is chosen, per PRD R5, so a declaration
falls back to an older intact version rather than reporting on a newer broken
one.

The lexical `sort.Strings` over *tool names* that gives PRD R13 its PATH
ordering lives in the loop these steps replace. It survives the move verbatim
and must not be dropped: the five steps above are the per-declaration body, not
a replacement for the loop around it.

The explicit empty-version skip branch above the directory lookup is deleted.
That branch, not the interpolation, is why `""` is one of the four broken forms.

### Reporting

```go
type Reason int

const (
    ReasonNoMatch Reason = iota
    ReasonBadForm
    ReasonChannel
    ReasonMissingFiles
)

type Unhonorable struct {
    Tool     string
    Declared string
    Reason   Reason
    Version  string // ReasonMissingFiles only: the version whose files are gone
}

type StateUnreadable struct {
    Tools []string // declarations that needed the read, in PATH order
    Err   error    // for the debug log, never for the message
}
```

`Tools` holds only the declarations that would have needed the installed set —
a `bad-form` or `channel` declaration is classified without ever consulting
state, so naming it in a message about an unreadable state file would be wrong.

`ActivationResult.Skipped []string` is replaced by `Unhonorable []Unhonorable`
and a nil-able `*StateUnreadable`, plus an `Entered bool` so the caller knows
whether this activation entered a project not already recorded rather than
inferring it.

**`unreadable` is a separate field rather than an entry in the slice**, and this
is load-bearing. It is a property of the read that would have classified every
declaration, not of any one of them; when it fires nothing was classified at all.
Putting it in the slice would mean writing N identical entries and
de-duplicating at render time, and a de-duplication step is a line someone can
delete, after which the acceptance criterion requiring exactly one message goes
red for a reason nobody predicted. With a separate field the wrong output is
unrepresentable.

**What stops the five reasons collapsing into one templated message.** There is
no airtight structural answer in Go, and the design says so rather than
pretending otherwise. What raises the cost of that edit past looking like a
simplification is that **the reasons carry different data, not different words**:
`missing-files` needs the version to reinstall, `bad-form` and `channel` need the
declared string the developer is about to edit, `no-match` is about the installed
set, and `unreadable` isn't in the slice at all. Collapsing to one format string
would first have to delete information from three of the five — a visible
regression in a diff rather than a refactor. This is also why `Reason` gets no
prose-producing `String()`: that would make `fmt.Sprintf("%s: %s", tool, reason)`
the natural call site, which *is* the templated message R17 forbids. The renderer
is a switch with four whole sentences and no generic `default` arm.

Rendering lives in `cmd/tsuku` through the existing `printWarning`, so `--quiet`
travels nowhere and both commands are gated by construction. These messages are
not log output and must not be routed through `log.Default()`: a level prefix
would disturb the substring assertions PRD R17 pins, and `printWarning` is
gated on `quietFlag` directly, which is the gate R22 actually asks for.

### The state stamp

`_TSUKU_STATE_STAMP`, holding `<mtime-nanos>-<size>` from one `os.Stat` of
`$TSUKU_HOME/state.json`, with a distinguished token when the stat fails for any
reason. The short-circuit becomes:

```
cwd != "" && curDir != "" && cwd == curDir && stamp != "" && stamp == currentStamp()
```

Five rules, each closing a specific silent failure:

- **Inequality only, never ordering.** Any difference re-resolves. An ordering
  comparison gets a state file restored from backup exactly backwards: the file
  is older than the stamp, so "not newer" holds, so the shell never re-resolves.
  Clock steps and `TSUKU_HOME` switches fall out for free.
- **Stat, never hash.** A content hash is the 37 ms read the stamp exists to
  avoid. Measured: `os.Stat` on the real file is **2.6 µs**, 0.05% of the 5 ms
  budget.
- **mtime and size, from the one stat.** Size is load-bearing, not
  belt-and-braces, and the original justification for it understated the case.

  It was written as a coarse-filesystem edge case: mtime alone loses where
  granularity is one second, so activate at T, install at T, next prompt at
  T+0.1 leaves the mtime unchanged. Measured on ordinary Linux ext4, **two
  back-to-back writes carry the identical nanosecond mtime 185 times out of
  200**. This is not a coarse-filesystem problem. Linux caches the timestamp
  per timer tick rather than reading the clock per write, so at the speed a
  prompt hook and an install actually run, matching mtimes are the common case
  rather than the exception.

  That reframes the choice. On the original reading, dropping size costs
  correctness on unusual filesystems, which a future reader might accept. On the
  measurement, dropping it breaks the feature on the machine it was developed
  on. Size closes it free, since installing a version always adds a
  `VersionState` and so changes the length. Not the inode: tsuku builds for
  Windows and there's no portable equivalent.
- **Stat before reading state, and record that same value.** The wrong order is
  stable-looking and broken: read at T1, install commits at T2, stat at T3, and
  the shell resolved against old state while recording the new stamp, so it never
  re-resolves. The right order fails safe.
- **The stat-failure token is a fixed non-empty literal**, `no-state`. Never the
  empty string: the short-circuit tests `stamp != ""`, so an empty token means
  the early exit never fires on a machine with no `state.json` at all — a fresh
  install, or `TSUKU_HOME` pointed somewhere new — and every prompt re-parses
  and re-resolves forever, silently, on exactly the machines with nothing
  installed. The token also carries no error text and no path, or it would embed
  `$TSUKU_HOME` in an emitted value.
- **The re-resolve branch always emits the full export block**, even when the
  computed PATH is byte-identical. This is what makes the upgrade case work, and
  it is the likelier failure because "nothing changed, so print nothing" looks
  like a sensible optimisation. An implementation that skips the emission never
  records the stamp, so a pre-existing shell re-resolves on every prompt forever.

**Upgrade case (PRD R29):** an absent stamp never compares equal, so the
short-circuit doesn't fire, the first prompt re-resolves silently and emits the
stamp, and the second prompt short-circuits. Re-resolve once and record.

**The variable contract has exactly two ends, both in Go.** All three hook
fragments were read in full: none mentions any tsuku variable. They `eval` or
`source` whatever `hook-env` prints. `internal/hook/install.go` is likewise
untouched. The contract lives between `FormatExports` and `os.Getenv`, so the
production changes are three files.

`tsuku shell` emits all three on its success path. It already emits two through
the same `FormatExports` and has already taken the stat. Emitting two of three
would need a fork inside `FormatExports`, which is the shared-code-path hazard
the PRD's own falsification criteria name.

The `curDir != ""` conjunct must stay ahead of the stamp comparison, or
`tsuku shell` loses the `curDir=""` mechanism it uses to defeat the early exit.

**Exit codes are the callers' decision, not `ComputeActivation`'s**, and the two
callers diverge deliberately. `hook-env` exits 0 always — for the five reasons,
for a parse failure, and where no project file is found — because a prompt hook
that exits non-zero gets wrapped in `|| true`. `tsuku shell` exits non-zero only
where no project file is found anywhere above the working directory.

One wrinkle in that last sentence, which PRD R37 states as a no-regression
requirement: today's behavior is conditional. `runShell` passes `curDir=""`, so
with no `.tsuku.toml` found *and* `_TSUKU_PREV_PATH` set, `ComputeActivation`
returns a deactivation result and `tsuku shell` prints an export block and exits
0. R37's criterion carries no such precondition. The behavior is preserved as-is
— a shell that was activated and is now outside any project gets its PATH
restored, which is right — and the divergence is recorded here so the criterion
is read as "unchanged", which it is, rather than as "always non-zero", which it
never was.

### Behavior changes beyond the PRD's list

Two fall out of the restructure and should be in release notes:

- A tool directory that exists but can't be stat-ed (permissions) currently falls
  through and reaches PATH anyway, because the branch tests only
  `os.IsNotExist`. It will now report `missing-files` and stay off PATH. The
  substring is mildly imprecise for `EACCES` — the files may be there — but the
  remediation is the same and `unreadable` is scoped by the PRD to installation
  state, not to a tool's directory.
- The `filepath.Abs` skip branch is dead: `ToolBinDir` is built with
  `filepath.Join` from `TSUKU_HOME`, so the input is already absolute. Dropped
  rather than given a reason of its own.

## Implementation Approach

1. **Move `activate.go` and its test to `internal/activation`**, update the two
   call sites, correct `shellenv`'s package doc comment. No behavior change; this
   lands and passes on its own.
2. **Add `InstalledVersionsFor`** and delete `LoadWithoutLock`.
3. **Add `project.ParseError`** carrying the directory, path and cause.
4. **Replace `Skipped`** with the typed reasons and `StateUnreadable`; implement
   resolution; fix the stat branch; drop the dead `Abs` branch. Carry the
   security chain's tool-name check through the rewrite, reclassifying it as
   `bad-form`, and validate state-derived versions before they become path
   components.
4a. *(Moved.)* The `%q` quoter swap and the tool-name check belong to the
   security chain and are not steps of this design. This chain's resolution
   sequence consumes the tool-name check and must not drop it while rewriting
   the loop it sits in.
5. **Add the stamp** through `ComputeActivation`, `FormatExports` and both
   callers, including the deactivation unset and the parse-failure recording.
6. **Render in `cmd/tsuku`**, set `SilenceUsage`/`SilenceErrors` on `hook-env`,
   and catch the parse failure so it returns nil rather than a non-zero exit.
   Note the flags alone are insufficient: the command still returns a non-nil
   error, which `main.go` prints and exits non-zero on.
7. **Amend the documents.** In `DESIGN-shell-env-activation.md`:
   - the algorithm at line 133, the worked example at 196 (already correct, now
     true), the trade-off at 212, the Negative bullet at 436 and the never-built
     mitigation at 442 — all four state or imply the silent skip;
   - the security mitigation at 412, "Tools that aren't installed are silently
     skipped, not fetched";
   - the risk row at 419, whose claimed "All paths constrained to
     $TSUKU_HOME/tools/, name validation" mitigation does not exist;
   - the two-variables statement at 198 and the variable table at 145, which
     become three;
   - the fast-path claims at 104 and 200, which say the unchanged-directory path
     does "no filesystem I/O" — the stamp adds one `os.Stat`, so that becomes
     false and it is a performance claim a reader would rely on;
   - every package and file name that moves: line 218 ("adds a new
     `internal/shellenv` package"), the component diagram box at 239, the
     `package shellenv` header of the Key Interfaces block at 261, and the
     Phase 1 deliverables at 352-353;
   - the Key Interfaces block itself — `Skipped []string` at 269 is the field
     being replaced, and the `ComputeActivation` signature at 280 is already
     stale and changes again here;
   - a new section on the five reasons and the non-blocking read.

   Outside that document, `cmd/tsuku/shell.go`'s `Long` help text at 23-27
   enumerates the two tracking variables and needs the third.

   In `docs/guides/shell-integration.md`: the version-forms section, what a
   developer sees when a declaration can't be honored, channel pins, and the
   variable list at 167.

Steps 1 through 3 are independently landable and reviewable. Step 1 in particular
is a pure move that should not be reviewed alongside behavior changes.

## Security Considerations

The review found two serious defects in the code this design rewrites. Both are
inherited rather than introduced.

**Neither is closed by this design.** They were folded in when the review found
them, on the argument that they are cheap and sit in the functions this work
already rewrites. A separate chain now owns both, with its own analysis and its
own PR, so that argument no longer applies: the fixes have an owner, and
carrying work another chain is scoping in parallel would leave this document
overstating what it delivers.

The analysis stays here because it is where the defects were found and because
this design's own resolution sequence consumes one of the controls. What
follows describes the defect class and the fix the other chain implements, and
the "This design" paragraphs below should be read as "the fix", not as a
commitment by this document.

The two chains land in a fixed order: the security fixes go first, against
`internal/shellenv/activate.go` in its current location, and this chain's
package move rebases and carries them through the rename.

### Tool names from `.tsuku.toml` are never validated

`cfg.ToolBinDir(name, version)` is `filepath.Join(ToolsDir, name+"-"+version,
"bin")`, and `filepath.Join` calls `Clean`. Tool names are validated nowhere:
`internal/project` checks only `MaxTools`, and no tool-name validator exists in
the tree. `ValidateRequested` and `ValidateVersionString` both guard the
version, which is the other half of the same path.

So a declared name that is not a single path segment can escape
`$TSUKU_HOME/tools` and put a directory of the repo author's choosing at the
front of PATH. Confirmed against the real `ComputeActivation` using an empty
directory rather than a payload: the escape reaches PATH and the entry does not
appear in `Skipped`. The reproduction is deliberately not written down here —
this document is committed to a public repository, and the class and the fix are
what a reviewer needs.

Two things make it worse than a missing check. It needs no privilege: a repo is
content, and a developer who runs `eval "$(tsuku shell)"` in a clone is exposed
with no hook installed at all. And the parent design already claims the control.
Its risk row at line 419 gives the mitigation as "All paths constrained to
$TSUKU_HOME/tools/, name validation", while the residual-risk cell *in the same
row* reads "Tool names with unusual characters could construct unexpected
paths". The document claims the control and records its absence side by side,
and rates the row Low. Whoever wrote it saw the gap and filed it as residual
rather than as missing.

**The fix:** activation rejects a declared tool name that is not a safe
single path segment — a charset check plus rejection of `..`, `/` and `\` — and
reports it as `bad-form`.

This does not collide with the PRD. "Validating that a declared tool name
exists" is out of scope because it needs the registry; this is syntactic and
needs nothing. R17's prohibition on a second, stricter validator is about
version strings narrowing the `bad-form`/`no-match` boundary, and this validates
a name, not a version.

### `%q` is not a shell quoter

`FormatExports` quotes with `fmt.Sprintf("%q", …)`, which produces a Go string
literal. It escapes `"`, `\` and non-printables, and does **not** escape `$` or
the backtick — the two characters that matter inside shell double quotes.

This is not fish-specific, which is how it was first reported. bash and zsh
perform command substitution inside double quotes as a matter of POSIX, and
their hook fragments are `eval "$(tsuku hook-env …)"`, so `eval` re-parses the
emitted text. Confirmed by executing what `FormatExports` actually emits under
bash; the fish half is documentation-based, as fish was not installed on the
machine used.

Severity splits by feed and should not be averaged. Fed by a tool name, per the
finding above, it is code execution from cloning a repo — high. Fed by
`_TSUKU_PREV_PATH` alone it is same-user with no privilege boundary crossed —
low. Validating tool names removes the repo-content feed but not the defect.

**The fix:** replace `%q` with per-dialect single-quoting. POSIX shells wrap
in `'…'` with each `'` rewritten as `'\''`; fish wraps in `'…'` and
backslash-escapes `\` and `'`. Single quotes suppress expansion in all three.
No future emitted value may be interpolated into shell text without going
through that quoter.

**The fish quoter ships unverified.** fish was not installed on the machine the
review ran on, so the bash half was executed and the fish half rests on
documentation. That is not a reason to hold the fix — the POSIX half is proven
and the fish rule is the same rule — but whoever has fish should exercise it
before this is called done, and it should not be assumed covered because the
bash case passed.

### Smaller items this design also closes

- **State-derived versions become path components unvalidated.** Introduced
  here, because reading state is the new input. `VersionMatchesPin` returns true
  unconditionally for `""` and `latest`, so for the commonest declaration
  whatever string state holds passes into `ToolBinDir`. State is written by
  tsuku and is same-user, so this is a trust assumption rather than a live
  vector — but the design elevates state to the authority on what is installed,
  and closing it costs one call. State-derived candidates pass
  `install.ValidateVersionString` before becoming a path component; a failure
  drops the candidate silently rather than reporting, because it is a corrupt
  state entry rather than a property of the declaration.
- **Two stamp rules that were in Decision 4 and not in this document.** The
  stamp is never echoed back from the environment — always freshly computed —
  and its stat-failure token is a fixed literal containing no error text and no
  path. `fmt.Sprintf("unreadable-%v", err)` would embed `$TSUKU_HOME` in an
  emitted value and re-inherit the quoting defect through a different door.

### Recorded, not fixed here

- **`.tsuku.toml` discovery walks past world-writable directories.** A repo
  cloned outside `$HOME` walks to `/`, so a `.tsuku.toml` placed at `/tmp` by
  any local user applies to every hooked shell working beneath `/tmp`; combined
  with the tool-name defect that is cross-user code execution on a shared host.
  `buildCeilings` also adds no ceiling at all when `os.UserHomeDir()` fails.
  Medium, and the fix changes `internal/project` discovery semantics this design
  does not otherwise touch. It needs its own owner.
- **`MaxTools` is enforced after the whole file is decoded**, so the constant's
  "prevents resource exhaustion" comment does not describe the code. A
  pathological `.tsuku.toml` is read and parsed on every prompt. Low, and worth
  one line in the amended documents because the design's own budget argument is
  about per-prompt cost.

### Checked and clear

The lock-free read has no integrity consequence: `Save` publishes by atomic
rename, so a reader sees the old or the new file and never a torn one, and
`state.json` is same-user with no privilege boundary for the lock to have been
defending. Reading state follows no attacker-supplied path, and the stat is only
ever compared for equality, never used to decide access.

The usage-block noise fixed in step 6 is a UX defect and not an injection route:
cobra sends both the error and the usage text to stderr, and the hooks eval only
stdout. Worth stating because "usage text reaching `eval`" is the
plausible-sounding version of it.

## Consequences

**Positive.** All four documented version forms work. Every unhonorable
declaration is reported with a distinguishable reason. Activation can't hang a
prompt. Activation and installation share one rule with no second copy to drift.
`internal/shellenv`'s doc comment stops misdescribing the package. A permissions
failure stops silently putting a directory on PATH.

**Negative.** One more package. Activation gains a dependency on all of
`internal/install` for three pure string functions — heavier than needed, and
what Decision 1 option A would fix later. A changed-directory activation now
costs a ~37 ms state decode where it previously cost a few `os.Stat` calls; this
is the price of R8, and the plan-free projection is the recorded lever if it
becomes binding. A third environment variable widens a contract that had two
names.

**Mitigations.** The decode is on the changed-directory path only; the
unchanged-directory path gains one 2.6 µs stat and no decode. The state-storage
lever is named rather than lost. The variable's contract has two ends and both
are in this change's blast radius.
