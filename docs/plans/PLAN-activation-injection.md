---
schema: plan/v1
status: Draft
execution_mode: single-pr
upstream: docs/designs/DESIGN-activation-injection.md
milestone: "Guard externally-supplied values at the config boundary"
issue_count: 11
---

# PLAN: Guarding externally-supplied values at the config boundary

## Status

Draft

## Scope Summary

Implements `DESIGN-activation-injection` against `PRD-activation-injection`,
closing tsukumogami/tsuku#2553. Eleven work items in four batches: a leaf shell
quoter and the two emitters that need it, one extracted name predicate shared
with its existing consumer, validation at `parseConfigFile` covering both
components of every declaration, a sink-level backstop on the registry name
path, and correction of ten false claims across three design documents.

Everything lands in **one pull request**. The batches are a review and
sequencing device, not separate deliveries.

One ordering constraint comes from outside this plan. A sibling change moves
`internal/shellenv/activate.go` into a new package immediately after this
merges, so this work is written against the **pre-move** location and must stay
cherry-pickable against `main`. Nothing here may anticipate that move.

## Decomposition Strategy

**Hybrid, batched by control rather than by layer.** Each batch delivers one
complete control, so a reviewer can evaluate a security property in isolation
rather than reconstructing it from fragments spread across layers.

Grouping rules:

- One issue per new package or extracted function, so the extraction is
  reviewable separately from its wiring.
- One issue per emitter, because the two emitters have different current states
  — one quotes wrongly, the other not at all — and conflating them hides that
  the second exists.
- The boundary is one issue, not one per validated component. Splitting it
  would leave an intermediate commit where the name is checked and the version
  is not, which is a state the PRD explicitly says fails (a name-only fix
  leaves the identical hole through the other component).
- Test work is not a separate issue. Each issue carries its own criteria; a
  testing issue at the end is how test strategy becomes negotiable under
  schedule pressure.

**Batch 1 lands first because of which residual each ordering leaves**, not
because it is independently correct. Compare them.

Boundary-first leaves `result.Dir` — the directory holding the `.tsuku.toml`,
named by whoever authored the cloned repository — reaching
`export _TSUKU_DIR=%q` on every activation. No traversal, nothing planted, no
path guessed: name the repository `$(…)` and it runs on `eval`. Validation
cannot touch it, because the path is legitimate.

Quoting-first leaves the traversal, which needs `<traversed>/bin/<cmd>` to
already exist — a planted file at a guessed location.

So Batch 1 closes the more severe residual with the lower precondition. That is
the reason, and it is stated because "leaves a coherent partial state" invites
reordering on scheduling grounds and this argument does not.

Batch 3 depends on Batch 2 because it calls the predicate that batch extracts.

## Issue Outlines

### Issue 1: Add `internal/shellquote`

**Goal**: A dependency-free leaf package exporting `POSIX(string) string` and
`Fish(string) string`.

**Acceptance Criteria**:
- `POSIX` wraps in single quotes with each `'` rewritten as `'\''`. Body lifted
  from `internal/actions/set_env.go:252`, which is already correct.
- `Fish` wraps in single quotes escaping `\` first, then `'`. Order is
  load-bearing — escaping quotes first would re-escape the backslash it just
  introduced.
- Both return `''` for the empty string.
- Neither escapes a newline; it is literal inside single quotes in both
  dialects.
- The package imports only the standard library.
- Table-driven tests cover: `$(id)`, a backtick, `$HOME`, an embedded single
  quote, an embedded newline, two adjacent backslashes, and a plain value.

**Dependencies**: None.
**Type**: feat. **Complexity**: testable.
**Files**: `internal/shellquote/shellquote.go`, `internal/shellquote/shellquote_test.go`.

### Issue 2: Provision fish in the Go test job

**Goal**: Make a real fish evaluation test possible, so the POSIX/fish
divergence is caught by execution rather than by hand-derivation.

**Acceptance Criteria**:
- The Go test job installs fish.
- `exec.LookPath("fish")` succeeds in CI, so a fish-guarded test runs rather
  than skips.
- The bash pattern already in `internal/actions/set_env_test.go:317` is
  unchanged and still runs.

**Dependencies**: None.
**Type**: ci. **Complexity**: trivial.
**Files**: `.github/workflows/test.yml`.

*Rationale for it being its own issue: without it, every fish assertion is
either hand-derived — which is where the backslash divergence gets missed — or
permanently skipped, which reads as coverage while testing nothing.*

### Issue 3: Route `FormatExports` through the quoter

**Goal**: Replace all eight `%q` sites in `internal/shellenv/activate.go` with
dialect-correct quoting, and shape emission so a future value cannot bypass it.

**Acceptance Criteria**:
- No `%q` remains in `FormatExports`, in either the activation or the
  deactivation branch.
- Emission goes through a helper taking the *value*, not a format string, so
  adding a variable without quoting requires deliberately bypassing the helper.
- Under bash: output built from a `Dir` containing `$(touch marker)` is
  evaluated in `bash --norc --noprofile`; the marker must not exist and the
  value must read back byte-identical. Same for a backtick, `$HOME`, an
  embedded single quote, a newline, and a plain value asserted on the
  round-trip rather than the emitted literal.
- Under fish: the same set, plus a value containing two adjacent backslashes,
  which is the only fixture separating the two dialects.
- Both branches covered; the deactivation branch is a separate `switch`.
- The three existing assertions at `activate_test.go:221`, `:378` and `:398`
  are rewritten. They currently pin the broken quoter's exact output, so a
  correct fix makes them fail — this is the fix, not a regression, and the PR
  body must say so.

**Dependencies**: Issue 1, Issue 2.
**Type**: fix. **Complexity**: testable.
**Files**: `internal/shellenv/activate.go`, `internal/shellenv/activate_test.go`.

### Issue 4: Route `cmd/tsuku/shellenv.go` through the quoter

**Goal**: Close the emitter that quotes nothing at all.

**Acceptance Criteria**:
- Lines 40 and 47 no longer interpolate into hand-written double quotes.
- Output survives the same bash evaluation test as Issue 3.
- **The trailing `$PATH` still expands.** The line emits
  `export PATH="<binDir>:<currentDir>:$PATH"`; quoting the whole statement
  would round-trip both components perfectly, satisfy every "no substitution"
  criterion, and silently discard the user's `PATH`. Correct emission is
  `export PATH='<binDir>':'<currentDir>':"$PATH"`. Asserted by evaluating the
  output and checking the pre-existing `PATH` is still present — the only
  criterion in this plan that catches over-quoting.
- Behaviour is otherwise unchanged: this issue fixes quoting only and does not
  give the command a fish dialect. That is #2556 and stays out.

**Dependencies**: Issue 1.
**Type**: fix. **Complexity**: simple.
**Files**: `cmd/tsuku/shellenv.go`, `cmd/tsuku/shellenv_test.go`.

### Issue 5: Extract one strict name predicate

**Goal**: One exported definition of a well-formed recipe name in
`internal/recipe`, layering the strict character rule over
`IsValidRecipeName`, with the existing consumer routed through it.

**Acceptance Criteria**:
- An exported predicate accepts only lowercase letters, digits, `.`, `_` and
  `-`; rejects empty, a leading `-`, a leading `.`, and a name that *is* `..`.
- `..` is rejected as a whole path segment, not as a substring: `foo..bar` is
  accepted. This differs from both rules being consolidated, which test `..`
  with `strings.Contains`, so the extraction is not mechanical and must not
  silently adopt the looser form.
- Rejects `x$(id)y`, `` a`id`b ``, `tool name`, `a;b`, `-rf`, `.hidden`, a
  newline, and a non-ASCII letter.
- **Rejects `a:b`, and the error names the colon.** This is the criterion the
  rule exists for, not one more entry in the list. Every other fixture above is
  also caught by a denylist of shell metacharacters plus a case check — which
  is this codebase's shape in three places, so it is what an implementer will
  reach for. A colon is in no such denylist. It is also the only name-level
  attack that neither the quoter nor a containment assertion reaches, so this
  rule is its sole control rather than defence in depth over an existing one.
- Accepts `llama.cpp` and `hdrhistogram_c`, the only registry names with a dot
  and an underscore respectively.
- **The predicate does not call `IsValidRecipeName`.** That function rejects
  `..` by substring, so delegating to it would reject `foo..bar`, which R2
  requires accepted. `IsStrictRecipeName` applies the charset allowlist and its
  own segment rule instead. `IsValidRecipeName` is left untouched, so
  `internal/recipe/name_test.go:24` still passes and Issue 8's backstop is
  unaffected.
- `validateRuntimeDependencyNames` calls the strict predicate alone, replacing
  both its pattern check and its belt-and-suspenders `IsValidRecipeName` call.
- **Exactly one existing fixture changes**:
  `internal/recipe/validator_runtime_deps_names_test.go:59`, which pins
  `foo..bar` as rejected. It flips to accepted, with a comment saying why.
  Every other existing test in both files passes unchanged.
- **The predicate returns an error, not a bool**, so per-rule messages survive.
  The existing rejects assert `"must match"`, `"path traversal"` and
  `"must not start with '-'"`; a boolean collapses them and weakens exactly the
  diagnostics this change is strengthening elsewhere.

  *An earlier draft claimed no fixture changed at all, from grepping
  `validator_test.go` — the wrong file. A later draft said two changed, which
  over-corrected: `name_test.go:24` only changes if `IsValidRecipeName` itself
  changes, and it does not.*
- A test sweeps all 1449 names in `recipes/` and accepts every one.

- A **separate, wider** rule for an org source half: each of the two segments
  non-empty, not `.` or `..`, no separator, **case unrestricted**. Accepts
  `BurntSushi/toml`, which parses `.tsuku.toml` in this repository. Reusing the
  bare-name rule here would refuse it, and R9's one-definition principle
  actively encourages that reuse, so this is called out rather than left to
  judgement.

**Dependencies**: None.
**Type**: refactor. **Complexity**: testable.
**Files**: `internal/recipe/name.go`, `internal/recipe/name_test.go`, `internal/recipe/validator.go`, `internal/recipe/validator_runtime_deps_names_test.go`.

*The `a:b` case is the one that decides the rule's shape: a colon is neither a
shell metacharacter nor traversal, so no denylist catches it, and it is the
`PATH` separator — the composed entry splits in two, the second half relative
to the working directory. A containment check passes it and the quoter cannot
reach it. Only an allowlist stops it.*

### Issue 10: Extract the pin rule into `internal/pinsafe`

**Goal**: Make the existing version rule callable from `internal/project`.

**Acceptance Criteria**:
- A leaf package `internal/pinsafe` holds `ValidateRequested` and **only** that
  symbol, importing only `fmt`, `strings` and `unicode` — everything the
  current function uses. `pin.go`'s other three symbols stay where they are;
  moving them costs the same later and would have a caller to justify it.
- `internal/install.ValidateRequested` delegates to it, and `internal/install`'s
  existing tests pass unchanged.
- No new rule: the accept/reject set is identical to today's. `1.0:evil` is
  refused (verified: `invalid character ":"`), `1.2.3-RC1` and `@lts` accepted.
- The package name says which question the function answers. Two same-named
  `ValidateVersionString`s exist in this tree — `internal/version`'s accepts
  `../../evil`, `internal/install`'s rejects it — so a package called `pin`
  holding a path-safety check is the shape that invites the wrong import.

**Dependencies**: None.
**Type**: refactor. **Complexity**: simple.
**Files**: `internal/pinsafe/pinsyntax.go`, `internal/pinsafe/pinsyntax_test.go`, `internal/install/pin.go`.

*Why this issue exists: `internal/project` **cannot** import `internal/install`.
The cycle is `project -> install -> shellenv -> project`
(`internal/install/precedence.go:7`, `internal/shellenv/activate.go:15`), and an
earlier draft of this plan wired `install.ValidateRequested` into
`parseConfigFile` directly, which does not compile. Note that `install` depends
on `project`, so refiling cannot break this cycle the way it can elsewhere —
the extraction is forced.*

### Issue 6: Validate at `parseConfigFile`

**Goal**: Every declaration checked once, where the file becomes a config.

**Acceptance Criteria**:
- Each key is split with `SplitOrgKey`; **its error is propagated, not
  discarded**, and a key that yields no bare name is refused.
- The derived bare name is checked with Issue 5's predicate.
- The org source half is checked with Issue 5's **wider** source rule.
  `ow$(id)ner/repo:jq` is refused despite an impeccable bare name;
  `BurntSushi/toml` is accepted, because uppercase is legal in a GitHub owner.
- Org-scoped **accepts** are pinned, not just the refusals:
  `tsukumogami/koto` resolves to bare `koto`, and
  `tsukumogami/registry:mytool@2.0.0` to `mytool`. Without these, a validator
  that rejects every org key passes the whole issue.
- `jq@2.0.0` is refused, with an error saying the version belongs in the value
  or in the org-scoped form. This is the deliberate half of a recorded
  asymmetry, not an oversight.
- A name containing a null byte is refused. `a:b` is refused with an error
  naming the colon.
- Each version is checked with `pinsafe.ValidateRequested` (Issue 10 — not
  `install`, which `internal/project` cannot import), which rejects and never
  normalises. Accepts `latest`, an exact pin, a prefix pin, **the empty
  string**, `1.2.3-rc1` and `@lts`; a version containing a colon is refused;
  `v1.2.3` reaches the resolver unchanged. The prerelease and channel forms are
  named because an over-strict version rule loses exactly those and passes
  every other criterion here.
- A name that passes validation but whose directory does not exist is skipped
  without an error, and that outcome is distinguishable from a refusal. This is
  the criterion that stops an over-rejecting validator hiding behind tsuku's
  existing silent-skip behaviour.
- Refusal is **per declaration**: other declarations in the same file are
  honoured, asserted by their bin directories reaching the emitted PATH rather
  than by absence of an error. A file that cannot be parsed as TOML is refused
  whole.
- Traversal fixtures require the escaped bin directory to exist on disk;
  activation stats before adding to PATH, so without it an unfixed binary also
  produces no entry and the test passes against doing nothing. The assertion is
  anchored at the tools directory, not `$TSUKU_HOME`: with `jq-` consuming a
  segment, `../..` still lands inside the tools tree.
- `../tools-evil/x` is refused. The PRD asks for this to be caught by a
  separator-appended containment comparison; the DESIGN declines containment as
  a control because it does not catch `a:b`. Both hold: the **allowlist**
  refuses this name, and the criterion asserts the outcome rather than the
  mechanism, so neither document is contradicted.

**Dependencies**: Issue 5, Issue 10.
**Type**: feat. **Complexity**: complex.
**Files**: `internal/project/config.go`, `internal/project/config_test.go`, `internal/project/orgkey.go`.

### Issue 11: Carry refusals to the user

**Goal**: A refused declaration is visible, on stderr, from every command that
reads the config.

**Acceptance Criteria**:
- `ConfigResult` gains a diagnostics slice, populated at parse. The refusal
  cannot travel on `parseConfigFile`'s `error` return: that return aborts the
  whole load, which Issue 6 reserves for a TOML parse failure.
- Five consumers print it: `internal/shellenv/activate.go:48`,
  `cmd/tsuku/install_project.go:55`, `cmd/tsuku/cmd_shim.go:65`,
  `cmd/tsuku/cmd_run.go:95` and `internal/updates/apply.go`. `cmd_run.go:95`
  discards the load error today (`projectCfg, _ :=`) and needs the most change.
- Output goes to **stderr**, asserted per consumer by capturing the two streams
  separately. `cmd/tsuku/hook_env.go:51` prints activation output to stdout and
  the shell hook evaluates it, so a diagnostic on stdout would be executed
  rather than read.
- The message names the offending key and states the expected shape, and says
  "lowercase" where case is the fault. Appending to a field no production
  caller reads does not satisfy this — `ActivationResult.Skipped` is exactly
  such a field and every silent skip today goes into it.

**Dependencies**: Issue 6.
**Type**: feat. **Complexity**: testable.
**Files**: `internal/project/config.go`, `internal/shellenv/activate.go`, `cmd/tsuku/install_project.go`, `cmd/tsuku/cmd_shim.go`, `cmd/tsuku/cmd_run.go`, `internal/updates/apply.go`, plus tests.

*Split from Issue 6 on the validation-versus-diagnostics axis, which is free.
The axis that must not be cut is name-versus-version: no intermediate commit
may validate one component and leave the other unvalidated, and this split does
not. Issue 6 alone regresses nothing — a silently dropped declaration is what a
missing tool directory already does today.*

### Issue 7: Delete the dead `effectivePin` fallback

**Goal**: Remove the branch that logs at debug and falls back to a cached
global pin when version validation fails.

**Acceptance Criteria**:
- The fallback branch is gone.
- Lands in the same change as Issue 6, never before it. Ahead of the boundary an
  invalid pin would still reach this code and the branch would be doing live
  work.

**Dependencies**: Issue 11.
**Type**: refactor. **Complexity**: trivial.
**Files**: `internal/updates/apply.go`.

*Already dead in production today for a second reason — the real caller passes
a nil project config — so this is hygiene rather than an observable change.
Touches a file owned by another open issue; flag the hunk in the PR body.*

### Issue 8: Backstop the registry name path

**Goal**: Reject a traversing recipe name at the sink as well as the boundary.

**Acceptance Criteria**:
- `IsValidRecipeName` is called at `recipePath` and at `Registry.cachePath`.
- A traversing name passed directly to those functions is refused, exercised by
  calling them directly — no config-driven route reaches them once Issue 6 holds,
  which is what a backstop is.
- Removing either the boundary check or this one leaves a failing test, so the
  backstop is testable *as* a backstop.
- Documented in code comments as defence in depth beneath the boundary, not as
  the remedy, so nobody later removes the boundary on the grounds the sink is
  guarded.
- Uses `IsValidRecipeName` unchanged. Issue 5 deliberately leaves that function
  alone, so this backstop keeps its current substring `..` rejection — stricter
  than the boundary, which is acceptable for a backstop and is why the two
  rules are allowed to differ here.

**Dependencies**: None.
**Type**: feat. **Complexity**: simple.
**Files**: `internal/recipe/provider_unified.go`, `internal/registry/registry.go`, plus tests.

### Issue 9: Correct the design record

**Goal**: Ten enumerated false claims across three documents state what the
code does.

**Acceptance Criteria**:
- `DESIGN-shell-env-activation.md`: the claims at lines 388, 391, 393, 401, 418
  and 420. The risk row is at **418** — 419 is a different row and is accurate,
  so a reviewer working from the wrong line edits the wrong thing. The
  mitigation cell must describe the control this PR actually adds — a
  syntactic single-segment check on the derived name plus dialect-correct
  quoting — rather than being deleted or softened.
- `DESIGN-org-scoped-project-config.md`: the three claims at lines 270 and 131.
- `DESIGN-notification-routing.md`: the claim at line 418.
- The `TSUKU_CEILING_PATHS` claim states that it is opt-in and unset by
  default. This is an eleventh claim, outside R11's declared closed set of ten
  — corrected here because it is in the same section and is false in the same
  way, and named as an addition rather than smuggled in under the count.
- No claim is added that this PR does not implement. Writing a new false record
  of a control into the document that already carries one is the specific
  failure this issue exists to avoid.

**Dependencies**: Issue 3, Issue 4, Issue 6, Issue 11, Issue 8

*The record can only describe controls that exist, so this lands after the four
issues that build them.*

**Type**: docs. **Complexity**: simple.
**Files**: `docs/designs/current/DESIGN-shell-env-activation.md`, `docs/designs/current/DESIGN-org-scoped-project-config.md`, `docs/designs/current/DESIGN-notification-routing.md`.

## Implementation Sequence

**Batch 1 — quoting.** Issue 2 first (CI, so fish is available to everything after),
then Issue 1, then Issue 3 and Issue 4 in either order. Closes the emitter defects without
touching validation, and is independently correct if everything after it were
dropped.

**Batch 2 — the predicates.** Issue 5 and Issue 10, both independent of Batch 1
and of each other; all three can run in parallel.

**Batch 3 — the boundary.** Issue 6, then Issue 11, then Issue 7, all in the
same change. This is the
critical path item and the largest single review surface.

**Batch 4 — backstop and record.** Issue 8 any time after Issue 5. Issue 9 last, because it
describes what the others built.

Critical path: Issue 2 → Issue 1 → Issue 3, and separately
(Issue 5, Issue 10) → Issue 6 → Issue 11 → Issue 7 → Issue 9. Issue 4 and Issue 8 are off
the path.

## References

- `docs/designs/DESIGN-activation-injection.md` — the approach and the
  alternatives it was chosen over.
- `docs/prds/PRD-activation-injection.md` — requirements and the fifty
  acceptance criteria these outlines draw from.
- `internal/actions/set_env.go` — the correct POSIX quoter being extracted.
- `internal/actions/set_env_test.go` — the evaluate-and-assert-no-side-effect
  test shape to copy.
- `internal/install/symlink_test.go` — the `tools-malicious` sibling-prefix
  fixture.
