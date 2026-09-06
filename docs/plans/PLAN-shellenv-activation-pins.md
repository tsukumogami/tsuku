---
schema: plan/v1
status: Active
execution_mode: single-pr
upstream: docs/designs/DESIGN-shellenv-activation-pins.md
milestone: "Project activation for non-exact version pins"
issue_count: 8
---

# PLAN: Project activation for non-exact version pins

## Status

Active

## Scope Summary

Implements `docs/designs/DESIGN-shellenv-activation-pins.md`: activation
resolves all four documented version forms against the installed set, reports any
declaration it cannot honor with a distinguishable reason, and never blocks a
prompt.

## Decomposition Strategy

Horizontal. The design describes layers with stable interfaces and a clear
prerequisite chain. A walking skeleton was considered and rejected: the
end-to-end path already exists and works for exact pins, so there is no
integration risk to surface early. This work replaces the resolution step inside
a pipeline that already runs on every prompt, and the risk is per-layer behavior
under hostile input or stale state rather than whether the pieces connect.

Execution mode is single-pr. No split-trigger branch fires: there is no hard
constraint, the repo states no preference, and no unit is independently useful
to a reader who meets it alone — a package move with no behavior change, an
unused accessor and a typed error are invisible until the resolution work
consumes them. `split_rationale` is therefore empty, with no exception.

### The two security fixes are no longer in this plan

The design closes two inherited defects in the functions this work rewrites: an
unvalidated declared tool name reaching `filepath.Join` and escaping
`$TSUKU_HOME/tools` onto PATH, and `FormatExports` quoting with `%q`, which is a
Go string literal rather than a shell quoter. A separate chain now owns both,
with its own analysis and its own PR.

That resolves a scope question this plan had answered the other way, and it also
removes an inconsistency an earlier draft carried: the plan gave the
lower-severity fix its own issue sequenced first with a paragraph of
justification, while burying the higher-severity one as one criterion among
thirteen inside the largest issue, behind two dependencies. That inverted the
design's own severity ranking. It is moot now, and worth recording because the
same shape — a security control filed as a line item inside a functional change
— is how the control the parent design claims got lost in the first place.

**Cross-chain ordering.** The security fixes edit
`internal/shellenv/activate.go` and Issue 1 below moves that file. They land
first, in the file's current
location; this chain rebases and carries them through the rename, which git
tracks. A security commit that lands and is then moved by the next PR is harder
to cherry-pick, not easier, which is the reason for that order rather than the
reverse.

Issue 4 depends on the tool-name check existing but does not implement it. Issue
8 amends the parent design's claim about that control, which becomes true once
the other chain lands.

## Issue Outlines

### Issue 1: Move activation out of `internal/shellenv` into `internal/activation`

**Goal**: Activation lives where it can reach the pin routines, the version
ordering and installation state, with no behavior change.

**Complexity**: simple

**Dependencies**: None

No dependency inside this chain. Across chains it rebases onto the security
chain's PR, which lands first — see Cross-chain ordering above.

`activate.go` references no symbol from the rest of its package, and the four
`shellenv.` symbols `internal/install` imports are all shell.d cache or
PATH-precedence machinery. Move `activate.go` and `activate_test.go` verbatim,
update the two call sites in `cmd/tsuku`, and rewrite `internal/shellenv`'s
package doc comment, which currently describes only the half that leaves.

**Acceptance Criteria**:

- [ ] `activate.go` and its test live in `internal/activation` with no content
      change beyond the package clause. Git records it as a rename.
- [ ] No shim, alias or forwarding declaration is left in `internal/shellenv`,
      and nothing else in that package is edited beyond the doc comment.
- [ ] `cmd/tsuku/hook_env.go` and `cmd/tsuku/shell.go` compile against the new
      location; no other call site exists.
- [ ] `internal/activation` can import `internal/install` and `internal/version`
      without a cycle, demonstrated by doing so rather than asserted.
- [ ] `internal/shellenv`'s doc comment names what stays: shell.d init-cache
      construction, its doctor checks, and PATH-precedence shadowing.
- [ ] `go build ./...` and the full test suite pass with no behavior change.
- [ ] The security chain's changes to `activate.go` survive the move intact.

### Issue 2: Add a lock-free, one-decode installed-set accessor

**Goal**: Activation can learn what is installed without blocking on another
tsuku process and without paying a decode per declared tool.

**Complexity**: testable

**Dependencies**: None

`GetToolState` decodes the whole state file per tool queried and takes a shared
lock that blocks with no timeout and no non-blocking variant. Add
`InstalledVersionsFor(names []string) (map[string][]string, error)` on
`StateManager`, calling the existing unexported `loadWithoutLock` under the
in-process read mutex.

**Acceptance Criteria**:

- [ ] Returns each named tool's recorded versions from exactly one decode,
      counted, and the count does not grow with the number of names passed.
- [ ] Takes no file lock, and completes while another process holds
      `state.json.lock` exclusively for the whole duration and never releases
      it.
- [ ] Does **not** sort. The returned order is whatever the decode produced, and
      the doc comment says callers needing "newest" must sort by version
      comparison. An accessor that sorts, by any comparator, fails.
- [ ] A tool with no state entry yields no map entry and no error; a missing
      state file yields an empty map and no error.
- [ ] Hidden tools are included, and the doc comment says so and why.
- [ ] The exported `LoadWithoutLock`, which has no production caller, is
      deleted.
- [ ] `loadWithoutLock`'s doc comment is corrected: the file lock is optional
      for readers, because `Save` publishes by atomic rename.

### Issue 3: Give `LoadProjectConfig` a typed parse error carrying the project directory

**Goal**: A caller can tell a `.tsuku.toml` parse failure from any other error,
and can recover the directory the bad file was in.

**Complexity**: simple

**Dependencies**: None

`LoadProjectConfig` returns the parse error bare, at the point where the loop
variable holds the directory the caller needs. Add `project.ParseError{Dir,
Path, Err}`, matched with `errors.As`.

**Acceptance Criteria**:

- [ ] A `.tsuku.toml` that will not parse produces a `*project.ParseError` whose
      `Dir` is the directory containing it, for a file at the working directory
      and for one found by walking up.
- [ ] `errors.As` and `errors.Is` behave, and the wrapped cause is reachable.
- [ ] Existing callers that only check for a non-nil error are unaffected.

### Issue 4: Resolve all four documented version forms against the installed set

**Goal**: Every version form the guide documents activates the tool it names,
resolved against what is installed and chosen by version order.

**Complexity**: critical

**Dependencies**: Issue 1, Issue 2

Replace the string interpolation with the design's resolution sequence: the
tool-name check the security chain adds, `ValidateRequested`,
`PinLevelFromRequested`, filter by `VersionMatchesPin` **then** by directory
existence, `SortVersionsDescending` with a raw-string tie-break, then classify.
Delete the explicit empty-version skip branch and the dead `filepath.Abs`
branch, and stop the `os.Stat` fall-through that currently puts a directory on
PATH when the stat fails for any reason other than not-exists.

**Acceptance Criteria**:

- [ ] `latest`, `""`, a major-only prefix and a major-minor prefix each activate
      the expected installed version, via both entry points.
- [ ] An exact declaration activates the same version it does today.
- [ ] **With `1.0.0` and `2.0.0` installed, `latest` selects `2.0.0`.** This is
      the criterion that discriminates, and it is deliberately the dullest case
      in the set: every digit-boundary fixture below is *also* satisfied by
      `sort.Strings` ascending taking the first element, because `"10.0.0"`
      happens to sort before `"9.0.0"`. Only a pair where the newer version is
      lexicographically later separates version ordering from string ordering.
- [ ] With `9.0.0` and `10.0.0` installed and `9.0.0` recorded active, `latest`
      selects `10.0.0`; with `0.9.0` and `0.10.0`, it selects `0.10.0`; a prefix
      of `"1"` selects neither.
- [ ] Selection is unchanged when the accessor returns its candidates in reverse
      order, so an implementation that takes the first element without sorting,
      or that relies on the accessor's ordering, fails.
- [ ] With `1.0.0` and `1.0.0-rc.1` installed, `latest` selects `1.0.0`; with
      only `1.0.0-rc.1`, it selects that.
- [ ] Two versions that `CompareVersions` reports equal — `1.0` against `1.0.0`,
      or `1.0.0+a` against `1.0.0+b` — select the same one across repeated
      invocations in separate processes against identical state.
- [ ] With `2.0.0` recorded but its directory removed and `1.0.0` intact,
      `latest` activates `1.0.0` and prints nothing.
- [ ] **`jq = "2"` with only `1.6` recorded and its files gone selects the
      no-match reason, not the missing-files reason.** This separates
      match-then-files filtering from files-then-match; the latter tells the
      developer to reinstall a version that was never recorded.
- [ ] **The tool-name check survives this rewrite.** A declared name containing
      `..`, `/` or `\` still activates nothing, and no path outside
      `$TSUKU_HOME/tools` reaches PATH — asserted against the rewritten
      resolution, not inherited from the security chain's own tests. This
      criterion exists because Issue 4 rewrites the loop that check lives in,
      deleting branches around it, and Issue 8 then amends the parent design to
      claim the control exists. A rewrite that quietly drops it would make that
      amendment false, which is the exact failure this whole chain was opened
      to fix.
- [ ] A rejected tool name reports the `bad-form` reason once Issue 5 lands, so
      the check is not merely silent.
- [ ] A state-recorded version that fails `ValidateVersionString` is dropped
      from the candidate set rather than becoming a path component.
- [ ] With `git-lfs` installed and `git` not, `git = "latest"` puts no `git-lfs`
      directory on PATH.
- [ ] A directory with no state entry is never activated; a state entry whose
      directory cannot be stat-ed for any reason, including a permissions
      failure rather than absence, is never activated.
- [ ] Changing the shared matching, pin-level or version-validation rule without
      activation following in step turns an automated check of activation's own
      behavior red. The check derives its expectations by calling the shared
      routines, not from a hardcoded table, and the failing check implicates
      activation rather than only the shared routine's own tests.
- [ ] Resolving a ten-tool file decodes installation state exactly once.
- [ ] PATH ordering among activated tools is lexical by tool name, unchanged.
- [ ] **Moving between projects replaces PATH entries rather than stacking
      them.** Walking A → B → A → B leaves PATH the same length it was after the
      first activation. Each activation composes against the pre-activation base
      carried in `_TSUKU_PREV_PATH`, not against the current `PATH`. An
      implementation reading `os.Getenv("PATH")` as its base when the tracking
      variable is set grows PATH without bound and lets each project's entries
      shadow the next.

### Issue 5: Replace `Skipped` with typed reasons and render them on stderr

**Goal**: A developer can tell why a declaration was not honored, and can tell
the five possible reasons apart.

**Complexity**: critical

**Dependencies**: Issue 1, Issue 4

`ActivationResult.Skipped []string` becomes `Unhonorable []Unhonorable` plus a
nil-able `*StateUnreadable` and an `Entered bool`. `cmd/tsuku` renders through
the existing `printWarning`.

**Acceptance Criteria**:

- [ ] Five reasons exist as distinct sentences with the substrings the PRD pins,
      each also naming the declared tool, and each substring absent from the
      other four messages.
- [ ] **Reason selection is pinned, not only reason existence.** Each of these
      inputs produces its named reason: a pin nothing installed satisfies →
      no-match; `">=26"` → bad-form; `"@lts"` → channel; undecodable
      installation state → unreadable; a satisfying version whose directory is
      gone → missing-files. And the case that is easy to get backwards: an
      *absent* state file is `no-match`, not `unreadable`, because the loader
      cannot distinguish it from a machine that has installed nothing. Without
these, five correct sentences can be wired
      to the wrong conditions and every other criterion still passes.
- [ ] The reasons carry different payloads, and the renderer is a switch with no
      generic `default` arm that formats an unrecognised reason.
- [ ] The five messages are not one format string with a substituted phrase.
      Five phrase constants reached through a switch and interpolated into a
      shared template satisfies every substring assertion while being exactly
      what the PRD forbids. The discriminator is that the messages differ in
      what they *contain*, not only in one clause: `missing-files` names the
      version to reinstall, `bad-form` and `channel` name the declared string,
      `no-match` names neither. A single template cannot carry all three
      without dead fields.
- [ ] `unreadable` is a separate field, not an entry in the per-declaration
      slice: a ten-tool file with undecodable state produces exactly one message
      naming the tools it could not resolve. A design where the slice holds N
      identical entries de-duplicated at render time fails.
- [ ] `StateUnreadable.Tools` lists only declarations that needed the read.
- [ ] Capturing the streams separately: stdout contains only the shell code for
      the requested shell and nothing else, and the diagnostic appears on stderr
      and not on stdout. Asserted by string comparison rather than by the shell
      surviving the `eval`, and verified for fish as well as bash.
- [ ] All five reasons exit 0.
- [ ] An exact pin naming an uninstalled version reports rather than staying
      silent.
- [ ] A file mixing one satisfiable and one unsatisfiable declaration activates
      the first and reports only the second.
- [ ] A file with two unsatisfiable declarations produces two messages on entry,
      each naming its own tool.
- [ ] `--quiet` suppresses all five on both commands, PATH behavior unchanged.
- [ ] `internal/activation` writes to neither stream.
- [ ] `Entered` expresses the property it exists for: it is true exactly when
      this activation entered a project not already recorded, and the caller
      decides to report from it rather than from the `curDir` comparison
      directly.

### Issue 6: Record installation state's stat so remediation takes effect

**Goal**: A remediation the developer performs takes effect at the next prompt,
without a re-read on prompts where nothing changed.

**Complexity**: critical

**Dependencies**: Issue 1, Issue 4, Issue 5

Add `_TSUKU_STATE_STAMP` through `ComputeActivation`, `FormatExports` and both
callers, plus the deactivation unset. The short-circuit gains
`stamp != "" && stamp == currentStamp()`, after the existing `curDir` conjuncts.

**Acceptance Criteria**:

Every multi-invocation criterion below builds the second environment **only**
from what the first invocation actually emitted. Synthesizing it by hand
manufactures the state a broken implementation failed to produce, and passes a
build that should fail. This applies to all of them, not only where it is
repeated.

- [ ] An activation where zero bin directories result still returns a result
      carrying `Dir`, the base PATH and the stamp, and still emits them. An
      implementation returning nil, or a result with an empty `Dir`, when
      nothing activated makes every once-per-entry guarantee unenforceable.
- [ ] The stamp is composed from a single `os.Stat` of installation state and
      carries both mtime and size. A stamp built from mtime alone fails: on a
      filesystem with one-second granularity, activate at T and install at T
      then leaves the stamp unchanged, which is the exact case this exists to
      prevent.
- [ ] Entering a project with an unsatisfiable declaration reports; installing a
      matching version without leaving the directory puts it on PATH at the next
      invocation, with empty stderr.
- [ ] Installing an unrelated tool, leaving the declaration unsatisfiable,
      produces empty stderr rather than re-reporting.
- [ ] With state unchanged, a second invocation from the same directory
      short-circuits: no decode, empty stdout, empty stderr. The second
      environment is built only from what the first invocation emitted, never
      synthesized.
- [ ] An environment carrying the two old variables and no stamp re-resolves
      once and records, then short-circuits — not never, and not every prompt.
- [ ] The stat is taken before state is read, and the recorded value is that
      same stat. An implementation that stats after reading fails, because it
      records a stamp describing state it did not resolve against.
- [ ] The comparison is inequality only; a state file whose mtime moves
      backwards still re-resolves.
- [ ] The stat-failure token is a fixed non-empty literal carrying no error text
      and no path. A machine with no installation state short-circuits on the
      second prompt rather than re-resolving forever.
- [ ] The emitted stamp is always freshly computed; a value placed in the
      environment is never echoed back into stdout.
- [ ] The stamp is emitted through the same shell quoter the other two values
      use, not through `%q`. It adds a third emitted value to the exact function
      the security chain re-quotes, and a new `%q` call added after that chain
      lands would silently reopen what it closed.
- [ ] `tsuku shell` emits all three variables on its success path; deactivation
      unsets all three.
- [ ] **`tsuku shell` never short-circuits, whatever the environment holds.**
      With `_TSUKU_DIR` set to the current directory and `_TSUKU_STATE_STAMP`
      set to the matching stamp, it still resolves and still emits. It defeats
      the early exit by passing an empty `curDir`, and the `curDir != ""`
      conjunct must stay ahead of the stamp comparison for that to keep working.
      An implementation that reads the stamp from the environment in
      `tsuku shell` the way `hook-env` does makes `ComputeActivation` return
      nil, which `runShell` turns into an empty string, which makes the command
      print "no .tsuku.toml found" and exit non-zero for a project that exists
      and is perfectly valid.
- [ ] The re-resolve branch emits the full export block even when PATH is
      byte-identical, so the stamp is always recorded.
- [ ] Moving from project A to project B and back to A reports on each of the
      three entries.
- [ ] Moving between subdirectories of an already-activated project reports
      nothing.
- [ ] A shell inheriting the tracking variables from an activated parent
      produces no report; one that does not inherit them reports.
- [ ] Editing `.tsuku.toml` to add an unsatisfiable declaration while standing
      at the project root produces no report and no PATH change until the
      project is left and re-entered.

### Issue 7: Handle the parse-failure path without a usage block or a non-zero exit

**Goal**: A `.tsuku.toml` that will not parse produces one line and a working
shell, not a usage block and a failing eval.

**Complexity**: testable

**Dependencies**: Issue 3, Issue 5, Issue 6

`hook-env` sets neither `SilenceUsage` nor `SilenceErrors`, so a malformed
`.tsuku.toml` prints a duplicated error and a full usage block every prompt. The
flags alone are not sufficient: the command still returns a non-nil error, which
`main.go` prints and exits non-zero on. It must catch the parse failure, render
one line, and return nil.

**Acceptance Criteria**:

- [ ] On a parse failure `ComputeActivation` returns a non-nil result **and** a
      non-nil error, and the result carries the project directory, the base
      PATH, the stamp and `Entered`. The design flags this return convention as
      deliberately unusual for Go; an implementation returning a nil result and
      synthesising the exports in `cmd/tsuku` duplicates `FormatExports` and
      fails this criterion.
- [ ] A malformed `.tsuku.toml` produces one stderr diagnostic, no usage block,
      and exit status 0 — asserted on the exit code, not only on the absence of
      usage text.
- [ ] Its directory becomes the recorded project and stdout carries that
      recording, with the stamp alongside. An implementation that prints the
      diagnostic and emits nothing fails.
- [ ] Feeding only what that invocation emitted into a second invocation from
      the same directory, and a third from a subdirectory, produces empty stderr
      both times. The environment is never synthesized by the test.
- [ ] Where the recorded directory already matches, stdout is empty and the
      `eval` is a no-op.
- [ ] Repairing the file while standing at the project root produces no message
      and no PATH change until the project is left and re-entered.
- [ ] `tsuku shell` on an unparseable file prints the diagnostic on stderr,
      nothing on stdout, emits no recording, and its exit status is asserted —
      whether or not `_TSUKU_PREV_PATH` is set. Leaving the status unstated lets
      an implementation return the parse error and exit non-zero, or emit empty
      output and hit the "no project file" branch, and both pass a criterion
      that only checks the streams.
- [ ] `tsuku hook-env` where no project file is found and none was previously
      activated exits 0 with empty stdout and empty stderr.
- [ ] Leaving a project restores the pre-activation PATH, prints nothing, and
      exits 0.
- [ ] `tsuku shell` with no project file anywhere above keeps today's behavior,
      exit status included.
- [ ] `--quiet` suppresses the parse diagnostic.

### Issue 8: Amend the design and the guide to match the implementation

**Goal**: The design and the guide describe what activation actually does, so
the next reader is not misled the way this one was.

**Complexity**: simple

**Dependencies**: Issue 4, Issue 5, Issue 6, Issue 7

`DESIGN-shell-env-activation.md` specifies the algorithm being replaced,
contradicts itself about prefix pins, records a mitigation that was never built,
and claims a path-constraint control that did not exist until the security
chain landed.

Each criterion names a specific location, so the check is a diff review rather
than an unbounded search for anything still false.

**Acceptance Criteria**:

- [ ] The algorithm step, the worked PATH example, the trade-off entry about
      uninstalled versions, the Negative bullet and the never-built stderr
      mitigation all describe what the code does.
- [ ] **All three statements of the absent name-validation control are
      corrected**, not one: the prose claiming path traversal in tool names is
      already guarded, the risk row's mitigation cell, and the security
      mitigation stating uninstalled tools are silently skipped. The risk row's
      residual-risk cell no longer contradicts its own mitigation cell.
- [ ] The two-variable statements and the variable table become three.
- [ ] **The fast-path claims are corrected in both places.** The parent design
      states the unchanged-directory path does no filesystem I/O; the stamp adds
      one stat, and Issue 6's always-emit rule means that path can now also
      write output. Both are performance claims a reader would rely on.
- [ ] Every package and file name that moved is updated, along with the
      `Skipped` field and the stale `ComputeActivation` signature in the Key
      Interfaces block.
- [ ] A new section states the five reasons and the non-blocking read.
- [ ] `docs/guides/shell-integration.md` describes what a developer sees when a
      declaration cannot be honored, mentions channel pins, and lists three
      tracking variables. Its statement of the four version forms is now true.
- [ ] `cmd/tsuku/shell.go`'s `Long` help text names the third variable.
- [ ] One line records that `MaxTools` is a post-decode count, not an input
      bound.

## Dependency Graph

## Implementation Sequence

**Dependencies.** Issues 1, 2 and 3 have none within this chain. Issue 4 needs 1
and 2; issue 5 needs 1 and 4; issue 6 needs 1, 4 and 5; issue 7 needs 3, 5 and
6; issue 8 needs 4, 5, 6 and 7.

**Cross-chain.** Issue 1 rebases onto the security chain's PR, which lands
first. Nothing else here waits on it.

**Critical path:** 1 → 4 → 5 → 6 → 7 → 8. Six of the eight issues.

**Do 1 alone and early.** It is a verbatim move, and a move reviewed alongside
behavior changes is a move nobody can verify. Everything after it is written
against the new location, so doing it late means writing code twice.

**Issues 2 and 3 parallelize with 1** — they touch `internal/install/state*.go`
and `internal/project/config.go`, disjoint from `activate.go`. In a single-PR
flow that buys ordering freedom rather than wall-clock, so take them whenever
convenient before Issue 4.

**4 and 5 do not merge.** Resolution decides which reason applies; the reason
vocabulary is only meaningful once something produces each one. Landing them
together makes the diff hard to read and hides which half a failing test is
about.

**8 goes last and is not optional.** It is documentation, and documentation
sequenced last is what gets dropped when time runs out — but this entire chain
exists because a design said one thing and the code did another, and Issue 8 is
where that stops being true. Its criteria name specific locations so the check
is a diff review; the work is not done while any of them is outstanding.
