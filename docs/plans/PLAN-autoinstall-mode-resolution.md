---
schema: plan/v1
status: Active
execution_mode: single-pr
milestone: autoinstall-mode-resolution
issue_count: 8
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

**Goal**: Build the test seam every later unit depends on, and the static check
that keeps it the only route in.

Lift `newReinstallHarness` (`cmd/tsuku/install_reinstall_test.go:108-137`) and
the stubs in `internal/index/rebuild_test.go:14-131` into a shared fixture
helper rather than writing a third mechanism. Add the AST check: a `go/parser`
pass over `_test.go` files in `internal/autoinstall`, `internal/project`,
`internal/index` and `cmd/tsuku`, failing on a composite literal of two or more
`BinaryMatch` elements, or a recipe map reaching `Rebuild` with two keys
yielding the same command. Copy the matched-nothing guard from
`cmd/tsuku/install_reinstall_test.go:644-684` so it cannot pass by scanning
zero files.

**Acceptance Criteria**: AC48, AC49. The three existing violations migrate —
`internal/index/lookup_test.go:43-46`, `internal/autoinstall/autoinstall_test.go:330-333`,
`internal/project/resolver_test.go:134-137`. The roughly twenty-five
single-element literals are untouched, which is why the rule keys on element
count rather than on recipe names. A deliberately non-conforming fixture under
`testdata/` is reported by the check.

**Not deliverable here, and recorded rather than attempted.** Two of R19's
eight properties: a recipe with no checksum verification cannot be constructed
at all, and a prefix version cannot resolve offline. AC19 depends on a
prerequisite fix outside this plan; AC18's prefix half needs a fixture provider
or is cut. `latest` does resolve offline, against a local `httptest` server —
"offline" meaning no external network rather than no sockets, which AC49 says.

**Complexity**: testable

**Dependencies**: none

### Issue 2: The declaration-set resolver and Runner.candidates

**Goal**: Replace the version-only lookup and consume it where the un-narrowed
list cannot reach the consumers.

`ProjectDeclaration{Recipe, Version, ConfigKey, ConfigPath}` and
`DeclarationsFor(ctx, matches)`. Dedup on bare recipe name with the org-key
precedence stated per recipe rather than falling out of iteration order. Drop
`Resolver.Tools()`, the `lookup` field and `NewResolver`'s second parameter,
which die together once `command` leaves the signature. Keep `bareToOrg`'s
stderr warning: it is the only signal for two org keys reducing to one bare
name, which R6's refusal does not cover.

Then `Runner.candidates`, with the three-way branch below the `ErrNoMatch`
check, and `AmbiguousDeclarationError` for the many case.

**These land together.** `run.go` and `cmd_run.go` call the old surface, so the
resolver cannot land alone. The only sanctioned split is additive — add
`DeclarationsFor` beside `ProjectVersionFor` and delete the old one in a later
commit — and it is available if the combined change proves too large to review.

**Acceptance Criteria**: AC1 through AC12, AC17, AC18, AC44. Closes #2542's
identity half and #2547. AC44 is the regression bar for single-provider
commands and belongs here rather than at the end: this is the unit that could
break it, and the existing single-provider tests passing unmodified is the
cheapest signal that it did not.

**Complexity**: critical

**Dependencies**: Issue 1

### Issue 3: The refusal

**Goal**: Make the two-declared case refuse usefully rather than pick.

Exit 10, matching the install path's `ExitAmbiguous`, with a new case in
`cmd_run.go`'s switch. The message carries each declared recipe, its version
and its config key, and one invocation that works: `tsuku install <recipe>`
then the version-specific path. No new CLI surface — a `.tsuku.toml`
disambiguation key is out of scope, and AC26 requires the named invocation to
complete rather than to be a single token.

Identical with and without a terminal. No picker in either, because a file that
was supposed to settle the question should not be answered per invocation.

**Acceptance Criteria**: AC13, AC14, AC15, AC16, AC26.

**Complexity**: testable

**Dependencies**: Issue 2

### Issue 4: The terminal check, moved

**Goal**: Ask whether *this command* is declared, at a point where the answer
exists.

Move it inside `Runner.Run`, after all four gates, immediately before the
dispatch. The predicate is `effectiveMode == ModeConfirm && !r.IsTerminal()`
with **no declaration term** — by that point the mode already encodes
declaredness, and a literal reading of R9 that adds one reintroduces the
exit-13 defect the move exists to fix. `IsTerminal func() bool` goes on
`autoinstall.Runner` beside `Lookup` and `Exec`, nil meaning not-a-terminal.

Add `ErrNotInteractive` and its switch case; `ExitNotInteractive` exists but is
mapped nowhere. Fix the message's escape hatches: it currently names
`TSUKU_AUTO_INSTALL_MODE=auto`, which the escalation restriction refuses
without config corroboration, so following it verbatim reproduces the failure.
Give the `ErrNoMatch` path a message, since moving the check makes that path
reachable where the guard used to speak.

**Acceptance Criteria**: AC20, AC25, AC27, AC54, AC55. Closes #2545.

**Complexity**: testable

**Dependencies**: Issue 2

### Issue 5: Gate announcements and the elevation disclosure

**Goal**: Stop the mode changing silently, in either direction.

Each of the three mode-lowering gates writes one line naming itself with a
stable distinct identifier and the condition that fired it. The identifiers are
the test seam for every "no gate intervened" assertion in the criteria — a
later simplification that collapses them into one generic line removes that
seam, and should not.

The elevation is disclosed once per install it enables, naming the recipe, the
version and the path of the authorizing file. Not per invocation, where most
runs install nothing; not per project, where the second declared tool goes
silent. Also disclosed where a declaration determined the *recipe* without
raising the mode, since R3b removes a prompt that used to appear.

**Acceptance Criteria**: AC21, AC35, AC40. Elevation raises only an unset
default, so a flag-, environment- or config-set mode is a floor exactly as
`suggest` is.

**Complexity**: testable

**Dependencies**: Issue 2

### Issue 6: The audit record

**Goal**: Make the durable record cover the installs that did something
unexpected.

`auditEntry` gains the mode's origin from the closed set `default`, `flag`,
`environment`, `config`, `project`, and the gate that lowered the mode where
one did. `resolveMode` returns the origin alongside the mode, which it does not
today. Written on every install rather than only the auto path — the current
condition means a gate-diverted install leaves no trace at all.

**Acceptance Criteria**: AC22, AC23, AC24.

**Complexity**: testable

**Dependencies**: Issue 5

### Issue 7: Documentation

**Goal**: Make every document that describes this path describe what it does.

Two halves that do not share a dependency. The **hook documentation** —
`docs/guides/GUIDE-command-not-found.md` and the `tsuku hook` long help — is
wrong today under every possible outcome and waits on nothing; it can land
first. Closes #2550. The **consent-model documents** —
`DESIGN-project-aware-exec.md`, `docs/guides/shell-integration.md` and
`tsuku run --help` — wait on unit 5, because the disclosure form is what they
describe.

`DESIGN-project-aware-exec.md` also loses two mitigations that do not work and
gains an accurate account of what remains: not installing the hook works but is
undiscoverable until #2550 lands; `TSUKU_CEILING_PATHS` protects only if each
untrusted repository is named before it is cloned.

**Acceptance Criteria**: AC28, AC29, AC30, AC31 through AC34, AC36 through
AC43, AC51, AC53. Closes #2544 and #2550. AC30 is the guard on any
demonstration that `suggest` is honoured: assert the install instruction was
printed, that no elevation disclosure appeared, that none of the three gate
identifiers appeared, and that the declared recipe was not already installed —
without all four the demonstration can pass while measuring something else.

**Complexity**: simple

**Dependencies**: Issue 5

The hook half of this outline depends on nothing and can land first; the
consent-model half is what waits on Issue 5. The declaration names the
stricter of the two so nothing is sequenced too early.

### Issue 8: The gates-table derivation and its checks

**Goal**: Produce the record R21 obliges the design to leave behind, and the
two mechanical checks that keep it honest.

The design's derivation section describes the post-change table. This unit
produces it against the code as it then stands, and wires the checks. The span
is now bounded by two *functions* rather than two line markers — the `r.Lookup`
call inside `Runner.candidates` and the mode dispatch in `Runner.Run` — because
the extraction in Issue 2 moved the lookup out of `Run`. The derivation rule
changes with it, and a rule left in its old form makes the comparison fail on
correct code.

Two searches over that span, in both directions: no site absent from the
recorded list, and no listed site absent from the code. The check reads its
expected list out of the recorded derivation rather than out of the table, so a
record that stops matching the code fails rather than waiting for a reader.

**Acceptance Criteria**: AC45, AC46, AC50, AC52.

**Complexity**: testable

**Dependencies**: Issue 5

It waits on Issue 5 rather than Issue 2 because the span is not settled until
the terminal check has moved and the gate announcements exist. Deriving it
earlier would record a table that the next unit invalidates.

## Implementation Sequence

**Critical path:** Issue 1 → 2 → 5 → 6, four units deep, with Issue 8
branching off 5 alongside 6. Issue 2 is the bulk of
the work and the only one classified critical.

**Parallelisable:** Issues 3, 4 and 5 are independent of one another once 2
lands. The hook half of Issue 7 can be done at any point, including first,
since it is wrong today under every outcome.

No dependency graph is drawn: this is a single-pr plan, the sequence is four
units deep with one fan-out, and a diagram of it would restate this paragraph.

**What must be true before unit 2 is considered done**, because it is where
every requirement about identity lands and the review instrument it creates is
worth as much as the fix: the candidate list is narrowed at exactly one site,
no positional read occurs above it, and the region above the narrowing inside
`Run` is empty by construction. That is AC47, and it is a sign-off rather than
a test because behaviour cannot show it.

**Recorded in the design and not planned here:** the unvalidated declared
version that reaches `ToolBinDir` and lets a cloned repository's config name an
arbitrary path for the fast path to exec, and the verification gate that never
fires. Both are prerequisite defects with blast radius outside this work, both
are held pending a disclosure decision, and AC19 depends on the second.
