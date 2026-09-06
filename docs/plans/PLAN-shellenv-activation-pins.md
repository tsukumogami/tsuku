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

### On choosing acceptance-criteria fixtures

One finding from this plan's review generalises past this feature, and it is
recorded here because the fixed criterion alone does not carry it.

Five criteria were written to prove `latest` selects by version order rather
than string order: `9.0.0` against `10.0.0`, `0.9.0` against `0.10.0`, the
stable-versus-prerelease pair, and two more. The obvious wrong implementation —
`sort.Strings` ascending, take the first — passes every one of them. The reason
is that those cases were chosen *because* string comparison is known to be wrong
there, which is what made them illustrative; but ascending string order puts
`"10.0.0"` before `"9.0.0"`, so take-first yields `10.0.0`, which is the right
answer. The wrongness cancels. Every case selected for being interesting was
selected for the property that makes the bug invisible.

The only fixture that discriminates is `1.0.0` against `2.0.0`, where ascending
string order yields `1.0.0` and the requirement wants `2.0.0`. The dullest
available pair is the only one that tests the requirement rather than the
anecdote.

**The general rule: a case chosen for being interesting is chosen for a property
that may interact with the wrong implementation.** Illustrative cases demonstrate
a mechanism to a reader. Discriminating cases separate a right implementation
from a wrong one. They are not the same set, and the overlap is not reliable.
When writing criteria, ask of each fixture which of the two jobs it is doing —
and if every fixture in a set is doing the first, the set proves nothing.

**And do not settle it by inspection.** Asking of each fixture whether it
discriminates is the same act of judgment that produced the bad set, so it
reaches the same answer. Issue 4's suite was written under this very heading,
by an author who had just written it, and still shipped two criteria nothing
pinned: the containment guard, whose call site could be deleted with the suite
staying green, and `ValidateRequested`, likewise. Ten deliberate mutations —
each the plausible wrong implementation of one documented rule — found both in
about a minute. Eight went red as intended, which is the part that makes the
two silences mean something.

So: break the code on purpose, one rule at a time, and watch which criterion
goes red. A criterion no mutation can break is either untested or unreachable,
and those are different problems with different fixes. Untested wants a test.
Unreachable wants the honest note saying so, and a test at the level where the
thing can still fail.

One practical trap, since the harness is the thing being trusted: **a mutation
that does not compile is not a passing mutation.** Go reports an unused import
as a build error, so a mutation that deletes the last use of one produces
`FAIL <pkg> [build failed]` rather than `--- FAIL: TestName`. A harness counting
`--- FAIL` lines reads that as "nothing broke" and reports a gap that is not
there. One of the mutations here did exactly that, and the false gap was only
caught by re-running it by hand. Check the build separately from the tests, and
treat a build failure as an invalid mutation to be rewritten, never as a
result.

### On approvals whose premises move

A second finding generalises past this feature, and like the one above it is
recorded because no single fixed criterion carries it.

The reviewers approved this design's security section at class-and-fix detail,
with no reproduction. That was right — **on the premise that the fix landed in
the same pull request.** The premise held when they said it. It stopped holding
when the security work moved to its own chain, at which point the same document
became a public description of a defect with no fix anywhere, which nobody had
approved. The approval never changed; the world under it did.

The same mechanism nearly cost this plan its tool-name criterion: the check was
guarded by an acceptance criterion that lived in the security issue, so when
that issue left, the criterion left with it and the control it guarded was
orphaned inside an issue that rewrites the loop it sits in.

**The general rule: an approval carries premises, and a scope change can
invalidate an approval without anyone revisiting it.** The decision stays on the
page looking settled while the thing it depended on moves. Nobody re-checks it,
because re-checking settled decisions is not what a scope change prompts anyone
to do. When work moves between chains, the question to ask is not only "who owns
this now" but "which decisions were made on the assumption it lived here", and
the criteria that guarded it are the first place to look.

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

Also consumes the security chain's tool-name check, which this issue must carry
through the rewrite rather than reimplement.

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
      or `1.0.0+a` against `1.0.0+b` — select the same one **for every ordering
      of the tied group**.

      This was first written as repeated invocations in separate processes.
      Enumerating the permutations is strictly stronger and is what the test
      does: a separate process samples one of the orderings map iteration might
      produce, and enumeration covers all of them, including the ones a sample
      would be lucky to miss. Ordering is the right axis because it is the
      whole cause — `sort.Slice` is not stable and the candidate slice is built
      by ranging a map.
- [ ] With `2.0.0` recorded but its directory removed and `1.0.0` intact,
      `latest` activates `1.0.0` and prints nothing.
- [ ] **`jq = "2"` with only `1.6` recorded and its files gone selects the
      no-match reason, not the missing-files reason.** This separates
      match-then-files filtering from files-then-match; the latter tells the
      developer to reinstall a version that was never recorded.
- [ ] **A declared key whose derived bare name (after `SplitOrgKey`) is not a
      single safe path segment is rejected, names the offending key, and
      contributes no PATH entry.**
- [ ] **A legitimate org-scoped key — `owner/repo` or `owner/repo:tool` — is
      accepted, resolves to its bare name, and activates the installed version
      at `$TSUKU_HOME/tools/<bare>-<version>/bin`.** This is false today:
      activation iterates raw map keys and never calls `SplitOrgKey`, so
      `"tsukumogami/koto" = "1.0"` looks for `tools/tsukumogami/koto-1.0/bin`
      while the installer wrote `tools/koto-1.0`. The criterion cannot be
      satisfied by leaving that in place.
- [ ] **Every PATH entry activation produces lies within `$TSUKU_HOME/tools`,
      asserted by containment of the composed path** — separator-appended prefix
      check, as `internal/install/symlink.go` already does so that
      `tools-malicious` does not match `tools` — and **not** by inspecting the
      key for characters. Keying the property on the composed path rather than
      on "contains `/`" is what lets the previous criterion be true at all, and
      it does not require anyone to have enumerated every bad character.
- [ ] **The containment guard is pinned by a direct test of the guard, because
      the end-to-end assertion above cannot fail.** Found by mutation: deleting
      the guard's call site leaves the whole suite green. With the tool-name and
      version checks in place, no `.tsuku.toml` can produce an input that
      reaches it — the guard is unreachable defense in depth, and an end-to-end
      test of an unreachable guard passes whether the guard is there or not.
      That is the fixture/oracle collapse this plan warns about twice already,
      arriving in the plan's own acceptance criteria.

      Both stay, with the split made explicit in the test names and comments:
      the guard keeps the property true if a later change adds a candidate
      source or relaxes one of the two checks above it, and the end-to-end
      assertion catches an escaping entry arriving by some route neither check
      covers. Neither is redundant; only the claim that the end-to-end test
      verified the guard was wrong.
- [ ] A rejected tool name reports the `bad-form` reason once Issue 5 lands, so
      the check is not merely silent.
- [ ] **A malformed declared version — `../evil`, `1.0 0` — reports `bad-form`,
      not `no-match`.** Also found by mutation: dropping `ValidateRequested`
      entirely left the suite green, because a malformed declaration matches no
      recorded version anyway and the PATH result is identical. The reason is
      the only observable difference, and it is the one that matters —
      `no-match` sends the developer to look at what is installed when the
      problem is the line they wrote.
- [ ] A state-recorded version that fails `ValidateVersionString` is dropped
      from the candidate set rather than becoming a path component. **This does
      not collapse into the security chain's parse-time check, and the two must
      both exist.** There are two version values: the *declared* one (`latest`,
      `26`) which parse-time validation sees, and the *resolved* one (`26.8.1`,
      chosen from installation state) which is what actually becomes the path
      component. Parse-time validation never sees the second. Exact pins are
      covered by theirs, non-exact by this one, and dropping either leaves its
      half unvalidated. `internal/updates/gc.go` already validates a
      state-derived version before building a path for the same reason.
- [ ] **The validator at this sink answers the question the sink asks: "is this
      safe to compose into a path?" — which in this tree is
      `install.ValidateVersionString`, not `internal/version`'s function of the
      same name.** Two functions share that name and they answer different
      questions: `internal/version`'s asks "is this a plausible version token",
      `install`'s asks "is this safe to compose into a path". A path sink asks
      the second.

      Do not reach for this by asking which is *stricter*. Neither is: the
      version one is stricter on charset (it rejects a space, and `~`), the
      install one is stricter on path-safety (it rejects `..`, `/`, `\`). The
      word silently resolves to whichever axis the reader was already thinking
      about, and a reviewer who reads "charset-stricter, therefore safer" picks
      the one that **accepts `../../evil`** — in-charset from end to end, since
      `/` is permitted for scoped npm names such as `@biomejs/biome@2.3.8` and
      `.` is a legal version character so `..` is never special. That is a check
      reporting green on the one input it was added to stop.

      The second reason is the ordinary one: `internal/version`'s rejects
      `1.0.0 beta` and `1.0.0~rc`, which `install`'s accepts and which tsuku may
      therefore have installed, so it would refuse to activate versions tsuku
      itself created — the silent-skip disease reappearing inside its own fix.
      `install`'s is the gate that let the directory exist, so it is the correct
      oracle for "could tsuku have made this path".

      Stated on the question rather than the function because a criterion naming
      the function protects this sink, and a criterion naming the question
      protects the next one.
- [ ] A version tsuku installed — one `install.ValidateVersionString` accepts,
      with a directory present — always activates. The validation at the sink is
      never stricter than the one that created the directory.
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
- [ ] **Every PATH assertion compares against a literal expected path, never
      against a value computed by calling `cfg.ToolBinDir`, and fixtures create
      their directories from literals too.** The existing tests do both — the
      oracle at `activate_test.go:74` and the fixture at `:29` are the same call
      as the code under test — so a `ToolBinDir` that traversed out of
      `$TSUKU_HOME/tools` would have its escape created by the fixture and
      matched by the assertion, and every one of them would pass. A test whose
      expectation is derived from the function it is testing cannot fail for the
      reason it exists.
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
- [ ] Each payload is **used in the message**, not merely carried on the struct.
      The `missing-files` message contains the version whose files are gone, and
      the `bad-form` and `channel` messages contain the declared string. A
      write-only field that no message reads satisfies "different payloads" on
      inspection while the output is still one template.
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

The name-validation correction below is only true once the security chain lands.

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
- [ ] **All three statements of the name-validation control are corrected**,
      not one: the prose claiming path traversal in tool names is already
      guarded, the risk row's mitigation cell, and the security mitigation
      stating uninstalled tools are silently skipped. Each is corrected to
      describe the control the security chain actually added — a syntactic
      single-path-segment check on the declared name — rather than being deleted
      or softened. The risk row's residual-risk cell no longer contradicts its
      own mitigation cell.
- [ ] The two-variable statements and the variable table become three.
- [ ] **The fast-path claims are corrected in both places.** The parent design
      states the unchanged-directory path does no filesystem I/O; the stamp adds
      one stat, and Issue 6's always-emit rule means that path can now also
      write output. Both are performance claims a reader would rely on.
- [ ] **The emission claim is corrected everywhere it appears**, not only in the
      fast-path sentences: the statement that output is emitted only when PATH
      needs to change, the corresponding step in the first flow, and the
      `ComputeActivation` doc comment. Issue 6's always-emit rule falsifies all
      of them, and this criterion exists because the previous draft named two
      locations and left three.
- [ ] The stale `ComputeActivation` call signatures in both flow descriptions
      and the document's frontmatter are updated.
- [ ] **This chain's own design is amended too.** `DESIGN-shellenv-activation-pins.md`
      no longer claims to close the two security defects, and its implementation
      steps do not list work the security chain owns. A design that overstates
      what its PR delivers is the same defect as a parent design claiming a
      control it lacks.
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
first. Two issues here consume what that chain delivers and are recorded on
their own Dependencies lines: Issue 4 carries the tool-name check through its
rewrite, and Issue 8 amends the parent design's claim about that control, which
only becomes true once the other chain lands.

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
