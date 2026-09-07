---
schema: plan/v1
status: Active
execution_mode: single-pr
milestone: autoinstall-mode-resolution
issue_count: 10
upstream: docs/designs/DESIGN-autoinstall-mode-resolution.md
---

# PLAN: Autoinstall Mode Resolution

## Status

Active

Single-pr, tracking level none: no GitHub issues and no milestone are created,
so the Draft-to-Active gate auto-fires rather than waiting on approval.

## Scope Summary

Implements `docs/designs/DESIGN-autoinstall-mode-resolution.md`: a
declaration-set resolver consumed inside `Runner.candidates`, three-way
narrowing, a refusal naming the declared recipes, the terminal check moved to
where the declaration is known, gate announcements, an audit record carrying
the mode's origin, and the documentation corrections. Closes
tsukumogami/tsuku#2542, #2544, #2545, #2547 and #2550.

## Decomposition Strategy

**Horizontal.** The components have stable interfaces between them and one is a
prerequisite for every other: the fixture index has to exist before any
multi-provider behaviour can be tested, because R17 forbids obtaining a
multi-provider case any other way. A walking skeleton would have to thread an
end-to-end slice through a seam that does not exist yet, which is the thing
being built first.

**Execution mode: single-pr.** The repo declares no Delivery Preference, so the
default `consolidated` applies and no split branch fires. Nothing here is
independently useful to a reader who meets it alone — units 2 through 6 are one
behaviour change split by layer, and shipping the resolver without its
consumers would leave the tree in a state where the defect is still live and
the new surface has no caller. No `split_rationale` is recorded because no
departure was taken.

## Issue Outlines

### Issue 1: Fixture index and the multi-provider construction check

**Goal**: Build the test seam every later unit depends on, and the check that
keeps it the only route in.

Lift `newReinstallHarness` (`cmd/tsuku/install_reinstall_test.go:108-137`) and
the stubs in `internal/index/rebuild_test.go:14-131` into a shared fixture
helper rather than writing a third mechanism. The fixture must also be
reachable from the `tsuku install` path, which Issue 3's `tsuku install`
criterion needs.

**One fixture property is load-bearing and easy to miss: the declared recipe
must rank second or later in the index's own ordering** for the command under
test. If it ranks first, a narrowing that never matches anything is
indistinguishable from a narrowing that works, because index ranking and
declaration agree — and a no-op narrowing is the most plausible wrong
implementation of Issue 3.

Add the construction check: a `go/parser` pass failing on a composite literal
of two or more `BinaryMatch` elements, or a recipe map reaching `Rebuild` with
two keys yielding the same command. It must assert a **nonzero file count per
named package** — `internal/autoinstall`, `internal/project`, `internal/index`,
`cmd/tsuku` — rather than a single whole-scan matched-nothing guard, which a
check that silently walks one package of four would satisfy. The existing AST
walker skips `testdata/`; this check must opt back in, and that opt-in needs
its own assertion, because the negative control lives there.

**Acceptance Criteria**: AC48, and AC49 minus the two properties recorded below.
The three existing violations migrate — `internal/index/lookup_test.go:43-46`,
`internal/autoinstall/autoinstall_test.go:330-333`,
`internal/project/resolver_test.go:134-137` — and the roughly twenty-five
single-element literals are untouched, which is why the rule keys on element
count rather than on names.

**AC49 is partially deferred and cannot be closed in full here.** One of its
eight properties is not buildable: a prefix version cannot resolve offline
against this fixture, because `ResolveWithinBoundary` narrows a prefix only for
providers implementing `VersionLister` and `HTTPJSONProvider` implements none.
`latest` does resolve offline against a local `httptest` server, where offline
means no external network rather than no sockets.

**That does not hold AC18 back, and Issue 2 settled why.** An earlier reading
of this paragraph had AC18's prefix half needing a fixture version provider or
being cut, which contradicted R20. R20 wins: it puts resolution of a non-exact
version outside this work — it stays the installer's job, unchanged — and asks
only that carrying the recipe identity does not change what is carried
alongside it. AC18's subject is therefore which recipe was chosen, and both
halves are observable at the declaration layer, where the string is carried
verbatim and never resolved. No fixture version provider is needed, and none is
built.

The verification-pair property is **not** required. AC19 was restated to assert
which recipe the gate is invoked with rather than what it answers, so no recipe
without verification has to exist for it — which also removes the only criterion
in this plan that depended on a defect outside it. No acceptance criterion here
is unsatisfiable at merge.

**AC48 is closed by a filter over common shapes, not by total enforcement, and
the acceptance record has to say so.** `TestMultiProviderCasesUseTheFixture`
reads composite literals. A two-element match slice built by `append`, in a
loop, through a named slice type, or elided two levels down is invisible to it,
and a consumer that builds one of those could assert against its own
construction and reintroduce exactly the collapse the fixture exists to
prevent. The package comment states this for a future reader of the code; it is
restated here so that nobody reading *this* plan takes "AC48 closed" for
"enforcement is total".

That distinction is the subject of this work rather than an aside: a control
whose reported scope exceeds what it examined is one of the three defect
families the whole chain is about, and an acceptance record that let a partial
filter read as complete would be an instance of it. The residual is bounded
deliberately — reaching it takes two steps, not a slip — and the package
comment's standing instruction covers what a parser cannot: if you are checking
whether the rule catches what you are about to write, build it in the fixture.

**Complexity**: testable

**Dependencies**: None

### Issue 2: The declaration-set resolver

**Goal**: Make `internal/project` able to say which recipes a project declared
for a command, and at what versions.

`ProjectDeclaration{Recipe, Version, ConfigKey, ConfigPath}` and
`DeclarationsFor(ctx, matches)`. Dedup on the recipe a configuration key
denotes — its org-scoped source, which `SplitOrgKey` already computes — and not
on the bare name, with the org-key precedence stated per recipe rather than
falling out of iteration order. That precedence has a carve-out and the
sentence has to carry it: for a bare name the config declares with **at most
one** org-scoped source, the bare key's version if present, else the first org
key's. Written without the carve-out it is the rule AC11a names as the
dangerous wrong one.

**Two org keys whose org components differ are two declarations, not one**
(R2a). They reach the refusal rather than collapsing, so `bareToOrg`'s stderr
warning is deleted rather than kept — keeping both invites a later reader to
remove whichever one they meet first. AC11a is the criterion: the cheapest
wrong implementation dedups on the bare name, collapses the pair, and still
passes AC11, AC12 and AC13.

The warning covered slightly more than what now refuses. Its condition was two
or more org-scoped keys reducing to one bare name, which includes two naming
the *same* source — `org-a/koto` and `org-a/koto@2.0.0`, which `SplitOrgKey`
reduces alike. Those denote one recipe, stay one declaration, and are still
settled by string order on the key, with nothing printed. Accepted, because
there is one recipe to install either way, so the pick can only choose a
version of the right tool.

**A bare key alongside two org sources contributes no declaration.** AC11a says
the set is still two, so the bare key neither breaks the tie nor stands as a
third candidate. The reason is the collapse rule, not installability — a bare
key installs perfectly well, from the default loader chain. R2 identifies a
bare key with an org key reducing to it, and with two org sources that holds
against each of them separately, so the bare key adds no recipe the set does
not already carry and only fails to say which of the two it meant. The version
it declares does not reach the set, which is what a test can see: "the bare key
wins the tie" returns one declaration carrying that version.

`ProjectVersionFor` is retained in this unit so the package still compiles
against its existing caller. Its deletion, with `Tools()`, the `lookup` field
and `NewResolver`'s second parameter *and return type*, belongs to Issue 3 —
`internal/project` imports `internal/autoinstall` for exactly those, and until
they go, `autoinstall` cannot name `project.ProjectDeclaration` without a
cycle.

**Acceptance Criteria**: AC11, AC11a, AC12, AC17, AC18. Testable in
`internal/project` without a `Runner`, which is why this is the seam the split
uses.

AC17 is the declaration half only; AC17a carries the consent half and belongs
to Issue 5. The split was made during this unit, for the reason AC11a's was:
the terminal check reads `len(Tools) > 0` until Issue 5 replaces it, so an
unknown-recipe config still skips a gate an empty root does not, and no correct
declaration set can change that.

**Complexity**: testable

**Dependencies**: Issue 1

### Issue 3: Runner.candidates and the three-way narrowing

**Goal**: Consume the declaration set at the one site that produces it, so
the region above the narrowing inside `Run` holds no candidate list at all.

Extract `Runner.candidates`, moving the lookup with its `ErrIndexNotBuilt` and
`StaleIndexWarning` handling and the `ErrNoMatch` check into it. The three-way
branch sits below `ErrNoMatch`: zero declarations pass the full list through
unchanged, one narrows, more than one returns `AmbiguousDeclarationError`.
Delete `ProjectVersionFor`, `Tools()`, the `lookup` field and `NewResolver`'s
second parameter, which die together once `command` leaves the signature.

Add the package var at the lookup boundary in `cmd/tsuku` so the wiring in
`cmd_run.go` is testable for the first time. It is a wiring seam, not a fixture
source: a hand-written match slice is what Issue 1's check rejects.

**The deletion of `ProjectVersionFor` is the load-bearing half of this issue,
not tidying after it.** Issue 2 leaves a correct declaration set behind a
single-value compat shim that `run.go` still calls, so between Issue 2 and this
one the run path keeps single-picking while every test passes — the set is
produced and not consumed. Deleting the accessor is what makes the compiler
force the rewire; leaving it in place lets a correct set sit unused beside a
path that ignores it, and no behavioral test can see that, because a redundant
accessor produces no wrong answer while nothing calls it. AC11c is the
criterion, and it is satisfied by the symbol's absence rather than by an
assertion about output.

**Acceptance Criteria**: AC1, AC3, AC5 (auto half), AC6 through AC10, AC11c,
AC19, AC44, AC47. **AC4 and AC5's suggest half move to Issue 6**, because no
state in this unit reaches suggest for a declared command: the elevation is
still unconditional here and the three gates below it only lower auto to
confirm. AC1, AC3 and AC4
are run against the command whose declared recipe ranks second or later, per
Issue 1 — otherwise a narrowing that never matches passes them. AC44 is the
single-provider regression bar and belongs here because this is the unit that
could break it. **AC47 is a sign-off, not a test**: the candidate list is
narrowed at exactly one site, no positional read occurs above it, and the
region above the narrowing inside `Run` is empty by construction. Behaviour
cannot show it, which is why it is named as a criterion rather than left in a
sequencing note.

Closes #2542's identity half and #2547.

**Complexity**: critical

**Dependencies**: Issue 2

### Issue 4: The refusal

**Goal**: Make the two-declared case refuse usefully rather than pick.

`AmbiguousDeclarationError` and its formatting: each declared recipe, its
version, its config key, and one invocation that works — `tsuku install
<recipe>` then the version-specific path. Exit 10, matching the install path's
`ExitAmbiguous`, with a new case in `cmd_run.go`'s switch. Identical with and
without a terminal; no picker in either.

**Acceptance Criteria**: AC11b, AC13, AC14, AC15, AC16, AC26. AC16 exercises `tsuku
install` against the same two-provider fixture, which is why Issue 1 must make
the substitution reachable from that path.

**Complexity**: testable

**Dependencies**: Issue 3

### Issue 5: The terminal check, moved

**Goal**: Ask whether *this command* is declared, at a point where the answer
exists.

Move it inside `Runner.Run`, after all four gates, immediately before the
dispatch. The predicate is `effectiveMode == ModeConfirm && !r.IsTerminal()`
with **no declaration term**. `IsTerminal func() bool` goes on
`autoinstall.Runner` beside `Lookup` and `Exec`, nil meaning not-a-terminal.

Add `ErrNotInteractive` and a case for it in the error switch. `ExitNotInteractive`
exists and `cmd_run.go:108` exits with it directly today; what it lacks is a
case in the switch, which is the precise claim, and AC54 turns on the code not
changing.

**Acceptance Criteria**: AC2, AC17a, AC20, AC25, AC27, AC54, AC55, and one more.

AC17a lands here because it is the consent half of AC17, and consent is what
this unit changes. Issue 2 produced the empty declaration set AC17 asks for,
but an unknown-recipe config still skips the terminal check that an empty root
does not — the predicate reads `len(Tools) > 0` until this issue replaces it.

**The criterion that catches the wrong predicate**: a *declared* command, mode
raised to auto, the configuration-permission gate lowering it back to confirm,
no terminal — expect the not-interactive message and exit 12, not a prompt into
a closed stdin. AC20 does not catch a declaredness term in the predicate,
because its command is undeclared and the check fires either way; AC2 does not,
because it pins auto with no gate firing. Only this one exercises the failure
the move exists to fix. The configuration-permission gate is named because its
precondition is filesystem state, so it fires without needing a recipe
property.

**The escape-hatch enumeration the PRD obliges this plan to produce**, one list
per message that changes or appears:

- *The moved terminal-check message*: **conditional, not flat.** An earlier
  draft of this list said `--mode=auto` only, and dropped the environment
  variable because `resolveMode` refuses an env-supplied `auto` without config
  corroboration, so following it verbatim reproduces the failure. That
  reasoning is right and the draft failed to apply it to the flag.

  `--mode=auto` sets the mode to auto; the three mode-lowering gates then run
  *from* auto and can put it straight back at confirm. An unverified recipe is
  the ordinary case — every recipe in `internal/indexfixture` is one — so in
  the very state this unit exists to fix (declared, configuration-permission
  gate lowering, no terminal) following `--mode=auto` verbatim lands back on
  the same message and the same exit code. R10 requires a hatch to *complete
  the command* when followed verbatim from the state the message appeared in,
  and AC27 asks for exactness against that state. A static message naming the
  flag is false in more states than it is true.

  So the message names `--mode=auto` only where auto survives the gates from
  that state, and where a gate would lower it back it names **the gate**
  instead of a flag — the string only ever appears when following it works.
  The three gate conditions become named predicates shared by the gates and
  the hatch computation, so a fourth gate cannot quietly make the message lie.
  That is a structural guarantee rather than an invariant someone has to
  remember, which is the difference this plan keeps asking for elsewhere.
- *The new `ErrNoMatch` message*: no hatch. It reports that no recipe provides
  the command; there is nothing to escape to, and naming one would be the
  defect this enumeration exists to prevent.
- *The refusal (Issue 4)*: `tsuku install <recipe>` then the version-specific
  path. Held to AC26's bar — run one of the declared recipes to completion —
  rather than AC27's exactness, which R10 reserves for the terminal check.

Closes #2545.

**Complexity**: testable

**Dependencies**: Issue 3

### Issue 6: Bounded elevation

**Goal**: Implement the decision. This is the unit the design exists to
produce, and the earlier draft of this plan had none.

`internal/autoinstall/run.go:120-124` currently reads `if projectDeclared {
effectiveMode = ModeAuto }`, unconditionally. It becomes: raise only where the
mode's origin is `default`, and only for the declared command. An explicitly
set mode — `suggest`, `confirm` or `auto`, by flag, environment or config — is
honoured as given.

That needs an origin, which nothing carries today: `resolveMode` returns a bare
`Mode`. It returns an origin alongside it here, and `Run` takes both. Issue 6
consumes that plumbing rather than introducing it.

**Acceptance Criteria**: AC4, AC31, AC33, AC36, AC37, AC38, AC39, AC40, and the
design's D1-1 through D1-5. D1-5 is the one that distinguishes this decision
from the one it was nearly confused with: with `TSUKU_AUTO_INSTALL_MODE=auto`
and no corroborating config, the declaration must not re-raise the output of
the escalation restriction.

**AC33 is the floor, and it is this issue's rather than the documentation
issue's.** It is D1-3 restated — `suggest` set by the flag, the environment
variable and the configuration key in turn, each honoured — and the bounded
alternative is precisely the one under which it holds. An earlier draft filed
it with the never-raise criteria and so left the decision's own floor assigned
to nobody.

Closes #2544.

**Complexity**: critical

**Dependencies**: Issue 3

### Issue 7: Gate announcements and the elevation disclosure

**Goal**: Stop the mode changing silently, in either direction.

Each of the three mode-lowering gates writes one line naming itself with a
stable distinct identifier and the condition that fired it. The identifiers are
the test seam for every "no gate intervened" assertion in the criteria; a later
simplification that collapses them into one generic line removes that seam.

The disclosure fires once per install that a declaration determined — the mode
*or* the recipe, not only the mode, because R3b removes a prompt that used to
appear and no elevation occurs in that case. It names the recipe, the version,
the authorizing file's path and the recipe's source. The source is required
because the run path inherits #2552's registration exposure, so a recipe can
come from a source the user never approved and a disclosure naming only the
file would not say so.

**Acceptance Criteria**: AC21, AC30, AC35, and the design's D1-6. AC30 is the
guard on every `suggest` demonstration and applies to AC4, AC33 and AC37
wherever they are exercised.

**Complexity**: testable

**Dependencies**: Issue 6

### Issue 8: The audit record

**Goal**: Make the durable record cover the installs that did something
unexpected.

`auditEntry` gains the origin — `default`, `flag`, `environment`, `config`,
`project` — and the gate that lowered the mode where one did. Written on every
install rather than only the auto path, which is why a gate-diverted install
leaves no trace today. Consumes Issue 6's origin plumbing.

**Acceptance Criteria**: AC22, AC23, AC24.

**Complexity**: testable

**Dependencies**: Issue 6

### Issue 9: The gates-table derivation and its checks

**Goal**: Produce the record R21 obliges, and the checks that keep it honest.

The span is now bounded by two *functions* — the `r.Lookup` call inside
`Runner.candidates` and the mode dispatch in `Runner.Run` — because Issue 3
moved the lookup out of `Run`. The rule changes with it; left in its old form
the comparison fails on correct code.

Two searches over that span, in both directions: no site absent from the
recorded list, no listed site absent from the code. The check reads its expected
list out of the recorded derivation rather than out of the table, so a record
that stops matching the code fails rather than waiting for a reader.

**Acceptance Criteria**: AC45, AC46, AC50, AC52.

**Complexity**: testable

**Dependencies**: Issue 7

It waits on Issue 6 because the span is not settled until the terminal check
has moved and the announcements exist.

### Issue 10: Documentation

**Goal**: Make every document describing this path describe what it does.

Two halves with different dependencies. The **hook documentation** —
`docs/guides/GUIDE-command-not-found.md` and the `tsuku hook` long help — is
wrong today under every outcome and waits on nothing. Closes #2550. The
**consent-model documents** — `DESIGN-project-aware-exec.md`,
`docs/guides/shell-integration.md` and `tsuku run --help` — wait on Issue 7,
because the disclosure form is what they describe.

`DESIGN-project-aware-exec.md` loses the mitigations that do not work and gains
an accurate account of what remains. **Removal is the whole of the obligation.**
AC43 requires every mitigation the document *lists* to be demonstrated against
the threat; it never requires naming one that fails. So a control that does not
hold is dropped from the list without comment, leaving only mitigations that
have been followed and shown to intercept. The reasons live where the defects
are tracked, not in this diff.

This interacts with AC42, which is a separate obligation and still applies: any
document offering a *setting* as a mitigation must name at least one case that
setting does not cover.

**The `tsuku-user` skill is a third half, and it is an obligation the
repository states rather than one this plan invented.** `CLAUDE.md` requires
that a source change under `cmd/tsuku/` or `internal/shellenv/` be followed by
an assessment of that skill on two questions: whether anything it documents no
longer matches the code, and whether the change adds behavior no skill
mentions. This work changes `cmd/tsuku/` substantially.

Both answers are known already and they differ. **Nothing in the skill is
falsified**: it mentions `tsuku run` in one table row and says nothing about
non-interactive behavior, consent modes or refusals, so there is no stale claim
to correct — checked rather than assumed by the unit that moved the terminal
check. **But the second question is a yes**: a user who runs a declared command
with no terminal, or in a project declaring two providers of one command, now
meets behavior the skill does not cover.

That section is written *here*, at the end, and not earlier — its subject is
what a user encounters, and the elevation decides what a user encounters. A
section written before Issue 6 would describe a consent model that Issue 6
changes, which is the same mistake as documenting a mitigation before checking
it holds.

**Acceptance Criteria**: AC28, AC29, AC41, AC42, AC43, AC51, AC53.

AC32 and AC34 are **not** criteria of this issue and are not satisfiable under
the chosen alternative: they belong to the never-raise branch. AC32 is the
direct negation of AC36, which Issue 6 carries, and the PRD's note [c] records
AC34 as reachable only under never-raise. AC33 is not among them — it holds
under the bounded alternative and Issue 6 carries it.

**Complexity**: simple

**Dependencies**: Issue 7

## Implementation Sequence

**Critical path:** Issue 1 → 2 → 3 → 6 → 7, five units deep. Two units are
classified critical: Issue 3, where the identity fix lands, and Issue 6, where
the consent decision does.

**Parallelisable once Issue 3 lands:** Issues 4, 5 and 6 are independent of one
another. Issues 8, 9 and the consent half of 10 all wait on Issue 7. The hook
half of Issue 10 depends on nothing and can land first, since it is wrong today
under every outcome.

No dependency graph is drawn: this is a single-pr plan, the sequence is five
deep with one fan-out, and a diagram would restate this paragraph.

**What must be true before Issue 3 is considered done**, because it creates a
review instrument worth as much as the fix: the candidate list is narrowed at
exactly one site, no positional read occurs above it, and the region above the
narrowing inside `Run` is empty by construction. That is AC47, carried as one
of Issue 3's criteria rather than as a note, because a criterion in a sequencing
paragraph gates nothing.

**Recorded in the design and not planned here:** two defects outside this
work's scope, both reported through the project's security policy and tracked
there, neither described here. Neither is a prerequisite for any criterion in
this plan — AC19 was restated so that it is not. Issue 10 discharges what this
work owes them by *removing* the affected control from the mitigations
`DESIGN-project-aware-exec.md` offers, rather than by describing it: a document
must not offer a mitigation that does not hold, and is under no obligation to
explain a defect being handled elsewhere.

## A review step this plan earned twice

**Check each criterion's assignment against the assigned unit's capability, not
against the criterion's text.** Read the criterion, then ask whether the unit it
is assigned to is *able* to satisfy it. This is a different question from whether
the criterion is correct, and it is the only one that finds this failure.

It has fired three times here, and no instance would have failed any test:

- **AC33**, the consent floor — an explicitly set `suggest` being honored, which
  is what the whole bounded-elevation decision rests on — was filed among the
  criteria this plan declared unreachable, and assigned to no unit at all. The
  plan called its own load-bearing claim unreachable in one place and left it
  unowned in another.
- **AC11a** asserted that colliding org keys *refuse*, and was assigned to the
  unit that builds the declaration set. That unit cannot refuse; the refusal is
  two units later. It would have been marked closed by a unit that had delivered
  half of it, and the missing half is the half a user would notice.

- **AC17** asked that an unknown-recipe config leave every command "resolved and
  consented to" as with no file present, and was assigned to the unit that builds
  the declaration set. That unit can make the set empty; it cannot reach consent,
  because the predicate that breaks consent lives in `cmd_run.go`. Split into
  AC17 (the set) and AC17a (the consent behavior, on the unit where the predicate
  changes).

**The third one was found from the inside, and that is the more reliable seat.**
AC33 and AC11a were caught by review. AC17 was caught by the unit implementing
it, which discovered mid-build that it could not satisfy what it had been handed.
An implementer is the only reader who learns a criterion is unsatisfiable by
*trying* rather than by judging, so a unit reporting "I cannot close this one"
should be treated as a finding about the plan before it is treated as a problem
with the unit.

All three criteria were correct as written. What none carried was which unit could
satisfy it — that property lives in the assignment, not the text — so reading the
criterion, however carefully, says nothing about whether its assignment is
possible. This is the plan-level form of the defect family the work itself is
about: a control whose reported scope exceeds what it examined.

**The corollary, which is the useful half.** A multi-unit change is safe to stage
exactly when each stage has a criterion that *fails until the next stage lands*.
Staging is not the hazard; an unforced second step is. AC11c exists because
nothing forced the rewire after the set was produced, and it takes a different
shape from every other criterion here — it asserts a symbol's **absence**, because
a right answer nobody reads produces no wrong output for a behavioral test to
catch.
