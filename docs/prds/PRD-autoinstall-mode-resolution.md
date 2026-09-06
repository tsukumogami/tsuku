---
schema: prd/v1
status: Accepted
problem: |
  `tsuku run` is keyed by command while `.tsuku.toml` declares recipes, so it
  has to invert a many-to-one mapping to find out what a project asked for. The
  component that does that inversion correctly reports only a version and
  discards which recipe it matched, so every later decision — which recipe to
  execute, which to install, whether consent was already given, whether a
  terminal is required — is made from the binary index's own ranking instead.
  The result ranges from a prompt the documentation promises will not appear to
  silently executing a tool the project never declared.
goals: |
  One rule for how `tsuku run` decides what a project asked for and what it may
  do about it unattended, applied at every site that decides. A declared tool
  resolves to the declared recipe or to an error naming the declared
  candidates, never to something else. The consent question is settled on the
  record and every document describing it agrees with the code.
absorbed: docs/briefs/BRIEF-autoinstall-mode-resolution.md
source_issue: 2542
motivating_context: |
  Four issues sit on the same eight lines and were filed as separate defects:
  tsukumogami/tsuku#2542, #2544, #2545 and #2547. A fifth, #2550, is the
  documentation that describes the path. Two of them appear to
  contradict — one widens what a project declaration may do, the other asks
  whether it should do anything at all. Read as one mode-resolution model they
  do not conflict, because they answer different questions that the code
  collapses into a single boolean.
---

# PRD: Autoinstall Mode Resolution

## Status

Accepted

Absorbed [BRIEF: Autoinstall Mode Resolution](docs/briefs/BRIEF-autoinstall-mode-resolution.md); carried in Absorbed Brief.

Requirements are grouped for sequencing, so that the consent decision holds up
only what genuinely depends on it. **Group A** is behaviour, and holds under
every answer to that decision. **Group B** depends on the choice this PRD
deliberately does not make, which goes to a recorded decision at the design
altitude because it is a trust boundary and expensive to revisit. **Group C**
is documentation that is wrong today, whose correct content is knowable now.
**Non-functional** carries the test and regression obligations.

The contingent part is narrower than it first looked. The multiple-provider
gate seemed to need an exemption for declared tools, and such an exemption
would have been contingent on the consent decision. It does not need one:
narrowing the candidates to the declared recipe removes the gate's
precondition, so it stops firing of its own accord. That was checked against
the running code rather than reasoned about.

So of the five issues this PRD covers, tsukumogami/tsuku#2542, #2545 and #2547
are satisfiable in Group A, and #2550 in Group C. Only #2544 waits on the
decision.

## Absorbed Brief

The framing this document's requirements were written from. Carried here because
the brief held its contribution and nothing beyond it, and the Problem Statement
below states the same problem more fully — a reader would otherwise read one
idea twice.

**Who is affected and when.** Anyone whose project checks a `.tsuku.toml` into
its repository and whose command is provided by more than one recipe, which is
27 commands in the recipe tree and a shifting set in the published registry.
Four journeys exercised it: a developer joining a team and running a pinned
tool; a developer working in a repository they have not read, with a consent
mode set deliberately; a developer whose pipeline goes red on a runner with no
terminal, running the command that worked on their laptop; and a developer
running a tool their project never claimed to manage, in a project that
declares something else.

**What each should get.** The recipe the project named, at the version it
pinned, without needing to know which commands in the registry are ambiguous.
An accurate account of what a configured consent mode does and does not prevent
inside a cloned repository. A reason on the output rather than an exit code to
guess from. And no change at all to commands the project has not declared —
declaring `jq` is a statement about `jq`.

**The boundary the brief drew**, which the requirements below implement: in
scope are the declaration lookup and what it must return, recipe selection,
ambiguity the config did not resolve, whether an explicitly set mode is a floor,
whether a gate that changes the mode says so, the terminal guard's per-command
test, and bringing every document that describes this into line with whatever is
decided. Out of scope are activation's pin handling, the install path's
multi-satisfier picker, the registry's refresh cadence, the hook and shim
mechanisms themselves, and which recipes may be installed at all.

**The framing correction worth keeping.** The brief first claimed all five
failures reduce to one cause. They do not: the consent override would survive a
resolver that returned full recipe identity, because it is a project declaration
treated as unconditional authority to raise the mode rather than a lost fact.
Two adjacent causes in one function, which is why two issues that appeared to
contradict are one piece of work, and why neither one's fix implies the other's.

## Problem Statement

A project checks a `.tsuku.toml` into its repository declaring the tools it
needs and the versions it needs them at. A developer types one of those
commands. `tsuku run --help` promises that the pinned version is installed and
used automatically, with no confirmation prompt, because the project config is
treated as consent.

That promise holds only for commands that exactly one recipe provides.

The difficulty is structural rather than a slip at one line. `tsuku install` is
keyed by *recipe*: it reads the config's keys and installs them, so a
declaration's identity survives from the file to the install. `tsuku run` is
keyed by *command*, and a command maps to a set of recipes. Something has to
invert that mapping, and only the project config can — it is the one input that
knows which of several providers the project actually wanted.

The component that performs the inversion gets it right and then cannot say so.
It walks the candidate recipes, finds the one the project declared, and returns
that recipe's *version*. The recipe is dropped at the return statement. Every
decision downstream is therefore made from the binary index's own ranking, and
the index knows nothing about the project.

The consequences differ in kind, which is why they were filed as four issues.

**A tool the project did not declare gets executed, silently.** When a
different provider of the command is already installed at the declared version,
the fast path builds a directory name out of the wrong recipe and the right
version, finds a binary there, and replaces the process with it. A project
declaring `temurin = "21"` runs `corretto`'s `java`. Exit 0, nothing on stdout
or stderr, no prompt, no audit record. The index's ordering — installed first,
then alphabetical — makes this *more* likely rather than less, because the
recipe it prefers is the one whose directory exists. The precondition is that
two recipes ship the same version string, which is the defining property of a
family of distributions packaging one upstream: five recipes provide `java`,
and they all ship a `21`.

**A tool the project did not declare gets offered for installation.** Where
nothing is installed, the same wrong recipe reaches the install path, carrying
the declared version. A project declaring `fdclone` at some pin is offered
`fd` at that pin — a pairing nobody declared, and one the other recipe may not
even have a version for.

**A prompt appears that the documentation says will not.** A gate exists to
stop an ambiguous command silently installing an arbitrary recipe. It fires
whenever a command has several providers and cannot exempt a declared tool,
because nothing told it one was declared. It prints nothing when it fires.
Non-interactively the same path exits 13, "user declined", with no user
present.

**A declaration changes what happens to commands it does not name.** The check
deciding whether a terminal is required asks whether the project declares
anything at all, because asking whether it declares *this* command was not a
question the code could answer. Declaring `jq` changes what happens when
someone runs `hyperfine`.

Those four share one cause and one fix. A fifth failure sits beside them with a
different cause, and conflating the two is what made the issues look
contradictory. A user who sets `auto_install_mode = "suggest"` before working in
an unfamiliar repository finds that a `.tsuku.toml` shipped by that repository
overrides it. That is not lost information. It is a project declaration treated
as unconditional authority to raise the consent mode, with no floor beneath it.
The design document names that setting as the mitigation for exactly this
threat, and also, three hundred lines earlier, states that the mode is
overridden regardless of what the user configured. The document argues both
sides; the code implements one.

## Goals

`tsuku run` resolves a project declaration to the recipe the project named, or
to an error naming the recipes the project named. Never to a third thing.

The same answer is used at every point that decides something. R3 enumerates
those points; today they are decided from three separate readings of the same
possibly-wrong match.

Declaring a tool is a statement about that tool. It does not change what
happens to any command the project has not declared.

Nothing changes a user's effective consent mode without saying so.

The rule for how a project declaration and an explicitly configured consent
mode combine is decided once, recorded with its alternatives and reasoning, and
stated identically by the code and by every document that describes it.

## User Stories

**As a developer joining a team**, I want the tools my new project declares to
be the tools I get, so that I do not have to know which commands in the
registry happen to be ambiguous before I can trust what ran.

**As a developer who works in repositories I have not read**, I want to know
exactly what my configured consent mode does and does not prevent inside those
repositories, so that I can decide whether to work there at all. Whether it
prevents an unattended install of a declared tool is the Group B decision. That
it does not prevent the already-installed execution path is true under every
outcome, and R16a requires saying so.

**As the maintainer of a CI pipeline**, I want a run that will not proceed to
say which condition stopped it, so that I can fix it from the log rather than
by reproducing it locally. This story is only partly met: a run that stops in
suggest mode exits 1, which is indistinguishable from a general failure, and
this PRD does not fix that. See Known Limitations.

**As the author of a `.tsuku.toml`**, I want to find out at the time I write it
that I have declared two providers of the same command, rather than from a
teammate whose run was blocked. R8 deliberately withholds that signal from
config load, from parse time and from `tsuku install`, all of which have an
unambiguous job to do with such a config — so this story is met by no
requirement here and is recorded as an accepted gap rather than an oversight.

**As a developer running a script that calls tools my project does not
manage**, I want those calls to behave as they would in any other directory, so
that adding a tool to `.tsuku.toml` does not change unrelated commands.

**As a maintainer triaging a bug report**, I want the audit log to record every
install `tsuku run` performed and what decided the mode, including installs a
gate diverted, so that "tsuku installed something I did not expect" is
answerable from the machine it happened on.

## Requirements

Requirement and criterion numbers alike are stable identifiers, not an
ordering. Neither runs in document order, because a number is assigned once and
kept when the thing it names moves. An earlier draft listed the Non-functional
criteria's actual sequence here to make that concrete; the list went stale two
criteria later, which is the defect this document spent a review round
retiring, so the principle is stated and the sequence is not copied. The groups below
exist for sequencing — what can ship without waiting on the consent decision —
so numeric order does not follow section order, and a requirement keeps its
number if it moves between groups.

The five failures in the problem statement map to requirements as follows.
Executing an undeclared tool: R4. Offering an undeclared tool with a
transplanted version: R1 and R3. A declaration changing what happens to
undeclared commands: R9. A project configuration overriding an explicitly set
consent mode: R13, R14, R15, R15a, R16b and R11a. Listed rather than given as
a range, because numbers are identifiers and a range breaks the first time a
requirement moves — as R11a already has.

The fifth — a prompt appearing where the documentation says none will — splits
across both groups, and the problem statement should not be read as settling
it. That the prompt names the *wrong recipe* is a defect under every outcome
and is fixed by R3. Whether a prompt appears *at all* for a declared tool is
the Group B question: under a rule that forbids a repository-supplied file from
raising the consent mode, a prompt is correct behaviour and what gets fixed is
the documentation promising otherwise.

### Two observables, defined once

**"No confirmation prompt"** means all three of: nothing is read from stdin, no
prompt string is written to stdout or stderr, and the command does not exit 13
when stdin is closed. Criteria below use the phrase in that sense.

**"Executes recipe X"** is observable only through the executed binary itself,
because `tsuku run` replaces its own process. Criteria asserting it are
satisfied by a fixture binary that reports its own path when run.

### The gates, named once

Several requirements below quantify over checks in `tsuku run`, and the
quantifiers differ, so the checks are enumerated here with the two properties
the requirements turn on: whether the check can change the consent mode, and
whether it takes a recipe as input.

The table describes `Runner.Run` as it stands before this work. Requirements
about post-change behaviour consume the two sets it defines, which is safe only
because nothing this work adds changes a mode: R6's refusal installs nothing and
executes nothing. If a later change adds a mode-changing site, the sets shift
under requirements written against them, which is what R21's re-derivation
exists to catch.

The table is derived rather than asserted, and R21 requires the derivation to
be recorded where it can be checked. The rule is: every point in `Runner.Run`
between the index lookup and the mode dispatch that either reads or writes the
effective mode, or returns that reach an install or an exec without consulting
the mode — as `Runner.Run` stands before this work.

The second limb took three attempts and the failures are worth keeping, because
each produced a rule that answered differently from the table. "Ends the run"
admits every fatal error. "Returns before the dispatch is reached" admits the
config-load and index-lookup error returns, which are not rows, and would make
a check written against it fail on correct code from day one; it also made the
root guard derived, which contradicts its marking, since the guard ends the run
without reaching an install or an exec. The limb as now stated admits the fast
path and nothing else, which is what the table has.

A rule that answers differently depending on who applies it is the failure R21
exists to prevent, which is why the table is marked derived and informational
per row and why AC46 searches this predicate rather than an approximation of
it.

A check missing from this table makes every quantifier below silently wrong in
exactly the way the old ones were, so it is re-derived rather than trusted
whenever that span changes.

Rows are marked *derived* where the rule produces them and *informational*
where they are present for orientation only. A check comparing the rule's
output to this table compares against the derived rows; including an
informational row would make an exact match impossible.

| Check | Effect on the mode | Takes a recipe | |
|---|---|---|---|
| Root guard | Ends the run; no mode change | No | *informational* |
| Project declaration | **Raises** it, today and under two of the three live alternatives | The candidate set | *derived* |
| Configuration-permission gate | Lowers it out of auto when the config file is group- or world-accessible | No | *derived* |
| Verification gate | Lowers it out of auto when the recipe has no checksum or signature | Yes | *derived* |
| Multiple-provider gate | Lowers it out of auto when more than one recipe provides the command | The candidate set | *derived* |
| Already-installed fast path | Bypasses the mode entirely; returns before dispatch | Yes | *derived* |

Two sets are named for the requirements to quantify over, and both are defined
by enumeration rather than read off a column — the columns describe the rows,
they do not compute the sets. **"The mode-lowering gates"** are the
configuration-permission, verification and multiple-provider gates.
**"The recipe-reading gates"** are the verification and multiple-provider
gates: the project declaration and the fast path also consume a recipe, but
neither is a gate and neither is what R3a is about. Where a requirement
quantifies over gates it names which of the two sets it means.

Two rows are in the table because leaving them out is what made the earlier
version wrong, and both are load-bearing. The **project declaration** raises the
mode, which is a mode change, and is the one this PRD exists to discipline — so
a requirement about announcing mode changes that covered only the three gates
would exempt the very thing under discussion. R11a governs it. The
**already-installed fast path** neither raises nor lowers the mode but returns
before it is consulted at all, which is a third relationship the first column
has to be able to express, and it is what R16a turns on.

The **terminal check** is excluded by property rather than by location: it does
not read or write the effective mode, it decides whether an interactive prompt
is possible. R9 governs it alone. Were a check in the command layer to change
the mode, it would belong in this table; where it lives is not the test.

### Group A: what the project declared

These hold under every possible answer to the Group B decision below, and each
must be readable as invariant from its own text rather than from the heading it
sits under. Where a requirement mentions the consent mode, it says what it
requires under every value the mode can take, not what it requires under one.
They are implementable without waiting on the decision, and must be sequenced
so that they are.

**R1.** The project-declaration lookup shall report, for a given command, the
set of declared recipes that provide it — each with the version the project
declared for it — rather than a version alone. The set is empty when the
project declares no provider of the command.

**R2.** That set shall be deduplicated by recipe name, not by configuration
key. A single recipe declared under both a bare key and an org-scoped key that
reduces to the same bare name is one declaration, not two. The surviving entry
carries the version from the bare key, which is the precedence that holds today
and is not changed here.

**R3.** Where the set contains exactly one recipe, `tsuku run` shall use that
recipe and its declared version at every point that decides: selecting the
binary to execute, selecting the recipe to install, evaluating the verification
gate, evaluating the multiple-provider gate, and testing whether the command is
project-declared. No such decision shall be made from the binary index's
ranking.

**R3a.** Every recipe-reading gate shall evaluate the recipe and version that
will actually be installed or executed, never a sibling provider of the same
command. The
narrowing in R3 shall therefore be applied to the candidate list where it is
produced, so that every consumer reads the narrowed list **in the case where a
declaration narrowed it**.

That scoping is load-bearing and an earlier draft omitted it, which made this
requirement wrong. Narrowing is three-way rather than two: where no declaration
provides the command the full list passes through unchanged, because R5
requires an undeclared command to resolve exactly as it would with no
`.tsuku.toml` present — which is the index ranking. Narrowing the
zero-declaration case to the empty set would turn every undeclared command into
`ErrNoMatch`, and suppressing the multiple-provider gate for undeclared
ambiguous commands is not something R3b licenses. So the consumers still read
position zero of an un-narrowed list when nothing was declared, and that is
correct rather than residue.

The candidate list is
read in exactly six places: once where it is produced, twice to test its
cardinality, and three times to select an element positionally. Narrowing at
production makes all five consumers correct by inheritance **for the declared
case** — the positional
selections pick the declared recipe because it is the only element, and the
cardinality tests are computed over the narrowed set.

Stated this way it is also a review instrument. Any future positional read of
the candidate list introduced below the point of production is either correct
by inheritance or is a new defect, and that can be checked by position rather
than by reasoning about what the author meant. Given that this work exists
because of a positional selection nobody noticed, leaving behind a rule that
makes the next one visible is worth as much as the fix.

**R3b.** Where the project declared exactly one provider, the
multiple-provider gate shall not fire. This follows from R3 and R3a rather than
requiring an exemption: once the declaration has narrowed the candidates the
command is no longer ambiguous, so the gate's precondition has gone. Nothing
about the gate's own rule changes, and it still fires in the case R6
describes.

The implementation this excludes is the conservative one, which is why it is
stated rather than left as a consequence of R3. An engineer nervous about
touching a security gate fixes the identity defect and leaves the gate reading
the full candidate list "to be safe" — good judgement, badly applied here,
because the gate exists to catch ambiguity that the declaration has already
resolved. Reading the un-narrowed list keeps the prompt the documentation
denies, and it does so while looking like caution.

**R4.** Where the set contains exactly one recipe and that recipe is already
installed at the declared version, `tsuku run` shall execute it from that
recipe's version-specific directory. Where a *different* provider of the
command is installed at that version and the declared one is not, `tsuku run`
shall not execute the installed one; it shall proceed to install the declared
recipe under the effective consent mode, exactly as it would if nothing were
installed. Refusing outright would also satisfy "shall not execute it" and is
not the intended behaviour.

**R5.** Where the set is empty, the command shall be resolved and consented to
exactly as it would be with no `.tsuku.toml` present. This is not "unchanged
from today": today the presence of an unrelated declaration changes the
terminal check, which is the defect R9 removes. Where the two readings differ,
this one governs.

**R6.** Where the set contains more than one recipe, `tsuku run` shall install
nothing and execute nothing, and shall print a message containing all three of:
the declared recipe names; the configuration key and version each was declared
under; and at least one complete invocation that reaches a specific one of
them. The message shall name the recipes the project declared, not every
provider in the index.

**R7.** The refusal in R6 shall behave identically whether or not a terminal is
attached, and shall exit 10 — the code the install path already uses for an
unresolvable multi-candidate name — so that a script distinguishing this
condition needs no new handling.

**R8.** A configuration declaring more than one provider of the same command
shall remain valid. It shall not be rejected at parse time, at config load, or
by `tsuku install`, all of which have an unambiguous job to do with it. Only
dispatch of the bare command has no answer, and only dispatch refuses.

**R9.** The check deciding whether `tsuku run` requires a terminal shall test
whether the executed command is project-declared, using the same lookup as R1.
The presence of unrelated declarations shall not affect it.

**R10.** Every escape hatch named in the terminal check's message shall, when
followed verbatim from the state in which the message appeared, complete the
command the user was originally attempting. Parsing, or exiting zero without
running the intended command, does not satisfy this.

The invocation R6 requires is held to a different bar, because the command the
user attempted has no answer by construction — that is why R6 refused. It shall
instead run one of the declared recipes to completion. A user who wanted the
other one has been told both names and can pick; what they must not get is an
instruction that fails.

**R11.** Where any of the three mode-lowering gates changes the effective
consent mode, `tsuku run` shall write one line to stderr naming the gate and
the condition that fired it. This holds whatever the mode was and wherever it
came from, so it is invariant under the Group B decision. The
configuration-permission gate already does this and its line is the shape to
follow; the verification and multiple-provider gates are silent today. The root
guard is not in scope here: it aborts rather than changing the mode, and it
already explains itself.

**R12.** Every install `tsuku run` performs shall be recorded, and the record
shall name the origin of the consent mode as exactly one of: `default`, `flag`,
`environment`, `config`, `project`.

Origin means the source the mode was resolved *from*, before any gate ran. A
gate that then changed the mode is recorded by R12a instead, so no gate is ever
an origin and the five values are exhaustive.

Where more than one source supplied a value, the origin is the
highest-precedence source that supplied one — not the one that "determined" it,
which is a counterfactual test and selects nothing when a flag and a config
agree. Among the four existing sources the order is flag, environment, config,
default, which is the order `tsuku run` already resolves them in and is not
changed here. Where `project` ranks against those four is the Group B decision;
R12 requires only that whatever rank the decision assigns is what gets
recorded, and that `project` is not written at all under a rule that forbids a
declaration from setting the mode.

**R12a.** Where a mode-lowering gate changed the mode, the record shall also
name that gate.
Today the audit log is written only on the auto path, so precisely the installs
that did something unexpected are the ones that leave no trace.

**R20.** Where a declaration's version is not an exact pin — `latest`, or a
prefix — R1 shall report what the project declared verbatim, and resolution of
that string shall remain the installer's job, unchanged. This PRD does not
alter how such a string resolves; it requires only that carrying the recipe
identity does not quietly change what is carried alongside it.

### Group B: how far a project declaration may move the consent mode

**R13.** There shall be exactly one rule governing how a project declaration
and an explicitly configured consent mode combine, and it shall be recorded as
a decision naming the alternatives that were live and the reasoning that
selected between them, before it is implemented.

**R13a.** A *control* here is anything whose output is a verdict rather than a
value: a check, a gate, a guard, a review sign-off. The definition is given
because R13a quantifies over controls that do not exist yet, and "every gate"
without an enumeration is the ambiguity this document spent a round removing —
which can be closed by definition here and by nothing else, since the record
has not been written.

Where that decision introduces a control of its own — the disclosure form R11a
leaves to it, or the boundary condition a bounded rule would need —
the rule that control is evaluated against shall be written down before the
control exists. This is the one obligation this PRD cannot discharge on the
decision's behalf, because the controls do not exist yet, and it is the one
that matters: a control can be audited for having an empty subject by
inspection, and can be audited for having a subject narrower than its rule only
against a rule someone wrote down first. Every instance of that narrower defect
found while producing this document was found by comparing a check's scope to a
stated rule, and none by reading the check.

**R14.** The shipped behaviour and each of the following shall state the same
rule: `docs/designs/current/DESIGN-project-aware-exec.md`, which currently
states the implemented behaviour in one place and promises the opposite in two
others; `docs/guides/shell-integration.md`; the long help text of `tsuku run`;
`docs/guides/GUIDE-command-not-found.md`; and the `tsuku hook` long help text.
The last two describe the hook as advisory when it installs and executes, and
are tracked separately as tsukumogami/tsuku#2550.

The list is exhaustive as of this PRD. A document found later to describe the
rule is added to it rather than treated as out of scope by omission. The check
per document is a concrete assertion, not a reading: each statement about what
happens to a project-declared tool is either reproduced by running the command
it describes, or removed.

**R11a.** If the recorded rule permits a project declaration to raise the
effective consent mode, that elevation is itself a mode change and shall be
observable to the person it affects, not only to whoever reads the audit log
afterwards. Goals says nothing changes a user's mode without saying so, and a
requirement that covered only the three gates would exempt the one change this
PRD exists to discipline.

What form the disclosure takes is the decision's to settle, because a line on
every invocation in a project that declares its tools is a real cost and the
right answer may be once per project, or on first install of a given tool, or
on stderr every time. The requirement is that the decision record names the
form and the reasoning, and that the disclosure reaches the user before or at
the first install the elevation enables, on a surface they see without opening
the audit log. That admits all three candidate forms and still rules out the
audit log; "at the time it happens" would have forbidden two of the three,
since a once-per-project notice says nothing at the fifth install.

If the rule forbids the elevation entirely, this is satisfied vacuously. If the
rule bounds the elevation rather than forbidding it, it is not vacuous and the
bounded branch carries its own criterion.

**R15.** If the recorded rule permits a project declaration to raise the
effective consent mode, that elevation shall apply only to commands the project
declared, and to no other command, and "project declaration" shall be one of
the origins R12 records. If the rule forbids the elevation, the elevation
clause is satisfied vacuously and "project declaration" is not a reachable
origin.

**R15a.** The recorded rule shall say what happens when the user has configured
no consent mode at all, which is the most common configuration and is not
covered by any statement about explicitly set modes. A rule that forbids a
declaration from raising an explicitly set mode, and a rule that forbids it
from raising the unset default, are different rules.

**R16b.** The third mitigation the document offers — set `auto_install_mode =
suggest` — is contradicted by the code, and by the same document three hundred
lines earlier. Whether it becomes true or is withdrawn is R13's to settle, and
whichever way it lands, R16's obligation applies to the resulting text: if it
stays it must work, and if it goes the remaining list must still leave the user
something that does.

**This requirement exists to give the exemption a landing event**, not to
restate R16's carve-out. Group C ships first, and R16's criterion is signed off
against a mitigation list that still contains the exempt entry. The decision
then rewrites that list. Something has to re-open R16 at that moment, and
AC43's re-run clause hangs on R16b by name — so without R16b the obligation
degrades into a subordinate phrase inside a requirement that has already passed
and is therefore never revisited.

The failure that permits is not a misreading. An implementer who lands the
decision, updates the prose and considers R16 satisfied is reading correctly:
R16 *was* satisfied, when it was checked. The end state is a design document
offering a mitigation that does not work, attested to by a signed-off criterion
describing text that no longer exists — which is #2544 exactly, recreated by
the process meant to fix it.

### Group C: documentation defects that wait on nothing

These are true today and their correct content is knowable now, so grouping
them with the contingent requirements would hold back fixes that have no stake
in the decision. They are separated for sequencing, which is the only
reason the groups exist.

**R14a.** The command-not-found guide and the `tsuku hook` long help text shall
state that the hook calls `tsuku run`, that this installs and executes rather
than only suggesting, and which setting governs whether it installs. No
statement in either may describe the hook as advisory-only, and any sample
transcript shall be one the shipped hook produces. This is tracked as tsukumogami/tsuku#2550 and is
fixable independently of everything else here; it is named in this PRD because
R16's first mitigation depends on it, not because it waits on anything. Both currently describe it as advisory — the guide
asserts it four times and shows a transcript that the shipped hook cannot
produce — and the guide was authored six days after the hook stopped being
advisory, so this is a document that never described the code rather than one
that drifted. `tsuku hook install` prints one line saying a hook was
registered, and the default installer delegates to it, so nothing in the
install flow supplies the missing account either.

This is a separate failure from the one R13 settles, and the mechanism is worth
naming because it is not the same. The design document's first listed
mitigation — do not install shell hooks — works: uninstalling the hook does
prevent unattended installs. What fails is that the documentation a user would
consult in order to decide gives a false account of what the hook does, and
offers no route to the consent setting that governs it. The requirement is that
the account be true; no claim is made here about what any particular reader
concluded, and neither document makes a safety claim that could be quoted as
one. Both are also unlinked from the README and the website, as is the accurate
guide beside them, so the exposure is smaller than their confidence suggests.

**R16.** The design document shall state, accurately, what a user can actually
do about this threat: they clone a repository they have not read, run a command
in it, and the `.tsuku.toml` that repository ships causes a tool to be
installed and executed. That is the threat the document's own mitigations are
offered against, and it is named here because "the stated threat" appears in
the criteria and was nowhere stated.

At least one mitigation shall be listed, and every mitigation listed shall work
against that threat — except the `suggest` mitigation, whose fate the consent
decision settles, and which is held to the same bar once settled. Without that
carve-out a reviewer must either fail this requirement on a deliberately
pending item or waive a clause that says "every". Two of the three it currently offers fail for reasons that
have nothing to do with the consent decision, which is why this requirement is
here rather than in Group B:

- *Do not install shell hooks.* The mitigation works. What fails is that the
  documentation a user would consult to decide gives a false account of what
  the hook does, so they cannot know to apply it. R14a fixes that.
- *Use `TSUKU_CEILING_PATHS` to prevent config discovery in untrusted
  directories.* A real feature that cannot be used the way the sentence
  invites. The ceiling test runs before the config is looked for and matches
  exactly, so a ceiling naming a repository's own path does suppress that
  repository's config — but a ceiling on the *parent* is never reached, because
  the child directory is tested, does not match, and its `.tsuku.toml` is found
  on that same iteration. So the setting cannot serve as blanket protection
  over a directory of clones, which is the only shape in which a reader would
  reach for it. Making it work means naming each untrusted repository's exact
  path, which requires knowing a repository is hostile before cloning it. That
  is not a mitigation, it is a restatement of the problem.

**R16a.** A document that offers a setting as a mitigation shall state what
that mitigation does not cover. The consent mode does not reach the
already-installed fast path, which returns before any mode is dispatched, so no
answer to R13 changes what R4 governs — and a reader told only that `suggest`
protects them will supply the boundary themselves, which is how the current
overclaim happened. The requirement is that the boundary be written down, not
that the text avoid implying anything.

### Non-functional

**R17.** Multi-provider behaviour shall be tested against a fixture index whose
recipe names exist only in fixtures and appear in no published registry. A
static check shall fail if a test in the affected packages constructs a
multi-provider case any other way. This is stated structurally rather than as
a rule about what an author intended, because intent is not checkable: a grep
for a registry name cannot tell a deliberate multi-provider fixture from an
incidental mention, and several registry names are ordinary words.

The reason is that a test naming real recipes stops exercising anything the
moment the registry stops shipping that pair, and passes while doing so. The
recipe tree and the published manifest already disagree about membership, so
this is not hypothetical.

**R18.** For a command with exactly one provider, four things shall be
unchanged by this work: which recipe is chosen, which version is chosen,
whether a prompt appears, and the exit code. That list is closed; it is what
"unaffected" means here.

**R18a.** R18 freezes behaviour that is correct, not behaviour that is
defective, and one case has to be named because it is a change a literal
reading of R18 forbids. Today `tsuku run <installed-tool>` with no terminal and
no `.tsuku.toml` exits 12, because the terminal guard runs before the
already-installed fast path and blocks a path that would never have prompted.
That is the guard reporting on a decision that is not being made. Once R9 moves
the check to where the declaration is known, the command runs and exits with
the tool's own code.

The change is in scope and deliberate: it is the same defect R9 exists to fix,
found on a different input. It is stated here rather than left to a reviewer to
notice as an R18 violation, and the exit codes it moves are named in the
criteria so the change is visible rather than discovered.

**R19.** The binary index and the registry that `tsuku run` consults shall be
substitutable in tests, and a fixture index shall exist defining at least one
command with two providers that share a version string, and at least one with
three, since a criterion below exercises the three-provider refusal. The fixture recipes
shall be installable offline — a name that appears in no published registry has
no artefact behind it, so without this the criteria that require an install to
complete, and the fixture-binary observable for "executes recipe X", have
nowhere to install from. Every criterion in this
document about multi-provider behaviour depends on that seam, and R17 forbids
obtaining it from the real registry, so the seam is required work rather than
an assumption. The same seam shall be reachable from the `tsuku install` path,
which R8's criterion exercises against the same fixture configuration.

**R21.** The enumeration in the gates table shall be recorded with the
derivation that produced it, against the rule stated there. This is R3a's shape
and for R3a's reason: three requirements and four criteria quantify over sets
drawn from that table, so a table nobody can check makes all of them
unfalsifiable at once. The derivation belongs in the design rather than here —
a code-level enumeration in a requirements document goes stale the first time
someone moves a function — but the obligation to produce it, and the check that
it was produced, belong here.

The record shall state **where it looked**, not only what it found: the two
boundaries of the searched span named by identifier, and each site cited by
file, function and the role it plays in the table. Recording only the findings
would leave a reviewer two artefacts by the same author, and comparing them
catches a transcription slip and nothing else — while the alternative reading,
independently walking the function, is the re-derivation this requirement
exists to avoid.

Recording the span supports bounded searches instead: over that named span, the
two limbs of the rule are searched, and in both directions — no site absent
from the table's derived rows, and no derived row whose site is absent from the
code. That is the falsifier, it is mechanical, and being mechanical it shall
run as a check rather than a sign-off.

**When it is evaluated.** The derivation is produced during the design and the
check runs from then on, so it is not a one-time sign-off that passes and rots.
The table shall be re-derived, and the check re-run, whenever the searched span
changes. This work changes it: it adds consumers of the narrowed candidate
list and of the effective mode inside the span, and R3a requires those
consumers to read a single production site, which is itself a site within the
span. R6's refusal is not among them — it installs nothing and executes
nothing, so it reaches neither limb — and an earlier draft used it as the
example here, which was true only under a predicate that has since been
retired. Re-derivation is required here rather than only
described beside the table, because an obligation stated only in prose beside
the artefact it governs is the shape that has already gone stale twice in this
document.

## Acceptance Criteria

Coverage, so that what is unverified is visible rather than reconstructed by
eye. The right-hand column holds criterion identifiers and nothing else.

That is deliberate. It used to paraphrase each criterion, and a paraphrase goes
stale the moment the criterion it describes is edited — which happened three
times here while the link itself stayed intact, so a mapping check could not
see it. The same round supplied the proof: one requirement's rule was restated
in one branch's criterion and delegated to by reference in another's, one edit
was made, and only the restating branch broke. Identifiers cannot rot, so a
mechanical check over this table is sufficient.

| Requirement | Verified by |
|---|---|
| R1 | AC1, AC3, AC4, AC18 |
| R2 | AC11, AC12 |
| R3 | AC1, AC2, AC3, AC4 |
| R3a | AC19, AC47 |
| R3b | AC1, AC2 |
| R4 | AC5, AC6, AC7, AC8 |
| R5 | AC10, AC17 |
| R6 | AC9, AC13, AC14 |
| R7 | AC15 |
| R8 | AC16 |
| R9 | AC20 |
| R10 | AC25, AC26, AC27 |
| R11 | AC21 |
| R11a | AC35, AC40 [a] |
| R12 | AC22, AC23 |
| R12a | AC24 |
| R13 | AC28, AC39 |
| R13a | AC51 |
| R14 | AC29 |
| R14a | AC41, AC53 |
| R15 | AC32, AC33, AC36, AC37, AC38 [b] |
| R15a | AC31 |
| R16 | AC43 |
| R16b | AC30, AC34 [c], AC43 |
| R16a | AC6, AC42 |
| R17 | AC48 |
| R18 | AC44 |
| R18a | AC54, AC55 |
| R19 | AC49 |
| R20 | AC18 |
| R21 | AC45, AC46, AC50, AC52 |

- **[a]** AC35 and AC40 are reachable only under the raise-with-a-floor and
  bounded alternatives.
- **[b]** R15's origin-linkage clause is verified only through R12's enum, not
  separately.
- **[c]** AC34 is reachable only under the never-raise alternative.

A note states a fact about coverage — which criteria are reachable, and what a
row leaves unverified. It never restates requirement content, which would be
the paraphrase the identifier rule exists to retire, arriving in a smaller
container. The first draft of these notes broke that rule: two of the three
were their requirements' closing paragraphs in different words.

### Group A

No criterion here is false under any of the three live answers to the Group B
decision. That independence was previously asserted and turned out not to hold —
two criteria presumed the mode had already been raised by the declaration — so
it is now stated as the narrower and checkable property it actually is.

Two things it does not claim. Criteria that name no consent mode are those
whose outcome does not depend on one; criteria whose outcome does depend on the
mode name it. And "never false" is not "always exercised": under a rule that
raises `confirm` to `auto` for a declared tool, the criterion below that pins
confirm-mode behaviour has no reachable state and goes vacuous. R3 stays covered
because auto or suggest remains reachable under every outcome.

- [ ] **AC1** With the effective consent mode resolved to auto, a project declaring one
      provider of a multi-provider command and nothing installed: `tsuku run
      <command>` installs and executes the declared recipe at the declared
      version, with no confirmation prompt.
- [ ] **AC2** The same case, with no terminal attached, succeeds rather than exiting 13.
- [ ] **AC3** With the effective consent mode resolved to confirm, the same case
      prompts for the *declared* recipe at the declared version, and names no
      other recipe.
- [ ] **AC4** With the effective consent mode resolved to suggest, the same case prints
      an install instruction naming the declared recipe and installs nothing.
- [ ] **AC5** With a project declaring one provider and a *different* provider already
      installed at the declared version, `tsuku run <command>` does not execute
      the installed one. With the mode resolved to auto it installs the declared
      recipe; with the mode resolved to suggest it prints an install instruction
      naming the declared recipe and installs nothing.
- [ ] **AC6** With a project declaring one provider that is already installed at the
      declared version, `tsuku run <command>` executes it from that recipe's
      version directory and installs nothing, under every consent mode
      including an explicitly set `suggest`. This pins R16a's claim that the
      consent mode does not reach this path, so that a later Group B change
      cannot silently alter it.
- [ ] **AC7** With a project declaring one provider that is installed at a *different*
      version than declared, `tsuku run <command>` does not execute the
      installed version and resolves to the declared one.
- [ ] **AC8** With both the declared recipe and a sibling installed at the declared
      version, `tsuku run <command>` executes the declared one.
- [ ] **AC9** With three or more declared providers of one command, the refusal names
      all of them.
- [ ] **AC10** With a project declaring a recipe that does not provide the executed
      command, the following match the same command run from an isolated root
      where no `.tsuku.toml` is found anywhere on the discovery walk: exit code,
      whether a prompt appeared, and the recipe and version selected. Where
      neither run installs, stderr matches exactly; where either installs,
      stderr matches after normalising transfer rates and durations.
- [ ] **AC11** A recipe declared under both a bare key and an org-scoped key reducing to
      the same bare name is treated as one declaration, and resolves to that
      recipe rather than being refused as ambiguous.
- [ ] **AC12** In that case the version used is the one the bare key declares, matching
      the precedence that holds today.
- [ ] **AC13** With a project declaring two providers of one command, `tsuku run
      <command>` installs nothing, executes nothing, and prints both declared
      recipe names with the version declared for each. Providers the project
      did not declare do not appear.
- [ ] **AC14** That message names the configuration key and version each declared recipe
      came from, and at least one complete invocation reaching a specific one of
      them.
- [ ] **AC15** The refusal produces the same output and exit code with and without a
      terminal, and the exit code is 10.
- [ ] **AC16** `tsuku install` with the same two-provider configuration installs both
      recipes and does not error, and activation over the same configuration
      does not error either. Both read the file R8 requires to stay valid.
- [ ] **AC17** A `.tsuku.toml` naming a recipe that is in no index leaves every command
      resolved and consented to as it would be with no `.tsuku.toml` present,
      including the command whose name resembles the unknown recipe.
- [ ] **AC18** A declaration whose version is `latest` or a prefix resolves as it does
      today; the recipe chosen is still the declared one.
- [ ] **AC19** With the effective consent mode resolved to auto, a project declaring a
      recipe that has no checksum verification for the current platform, and a
      sibling provider of the same command that does: `tsuku run <command>`
      behaves according to the declared recipe's verification status and not
      the sibling's. Auto is named because the verification gate only runs from
      auto; from confirm or suggest it is unobservable. This is the criterion
      that catches a narrowing applied between the recipe-reading gates and the
      installer.
- [ ] **AC20** With a project declaring any tool, running an *undeclared* command under
      confirm mode with no terminal produces the not-interactive message and
      exit code, not a prompt written to a closed stdin.
- [ ] **AC21** Starting from an effective consent mode of auto, each of the
      configuration-permission, verification and multiple-provider gates, when
      it changes the mode, writes one line to stderr containing both a stable
      identifier for that gate and the condition that fired it. The three
      identifiers are distinct and each is asserted by name. The condition is
      asserted too, and for the gates that can fire for more than one reason it
      distinguishes them, so a constant string does not pass. Auto is named because all three gates only
      run from auto; there is no other state in which they fire.
- [ ] **AC22** Every install `tsuku run` performs appears in the audit log with an
      origin field holding exactly one of `default`, `flag`, `environment`,
      `config`, `project`.
- [ ] **AC23** With a mode supplied by more than one source at once, the origin recorded
      is the higher-precedence source. Each adjacent pair in the order is
      contested at least once — flag against environment, environment against
      config, config against default — so an implementation that orders any two
      of them wrongly fails, not only one that always writes the same value.
- [ ] **AC24** An install that a mode-lowering gate diverted away from auto, and that
      then proceeded after a prompt, appears in the audit log and names that
      gate.
- [ ] **AC25** Every escape hatch named in the terminal check's message, when followed
      verbatim from the state that produced it, completes the command the user
      was attempting.
- [ ] **AC26** The invocation named in the R6 refusal, when followed verbatim, runs one
      of the declared recipes to completion.
- [ ] **AC27** The terminal check's message names exactly the hatches that work
      from the state that produced it and no others, so that a hatch added
      later without a test is caught by the message no longer matching its
      expected set. The R6 refusal is deliberately excluded: R6 requires at
      least one working invocation, not all of them, so with two declared
      providers a message naming one is correct and an exactness rule would
      fail it. AC26 carries the refusal.

### Group B

Two items here are documentation review rather than automated tests, and are
marked `(sign-off)` rather than dressed up as assertions. "Gate" is reserved
for the runner checks in the table above. The rest are written per
named alternative, because the three alternatives are not two branches and a
bounded rule satisfies parts of each.

**Unconditional.**

- [ ] **AC28** (sign-off) A decision record exists naming all three live alternatives
      — never raise, raise-with-a-floor, and a bounded rule — with the evidence
      for each, and it carries its own numbered acceptance criteria for the
      behaviour it selects. Those criteria are what the implementation is
      tested against; this PRD cannot enumerate them without making the choice.
- [ ] **AC51** (sign-off) Each control the decision record introduces is accompanied by
      the rule it is evaluated against, stated in the record rather than left
      to the implementation, so that its scope can be compared to that rule.
- [ ] **AC29** (sign-off) Each of the documents in R14 is walked against two
      captured transcripts: a `tsuku run` of a declared tool in a cloned
      repository, and the same tool reached by typing its bare command with the
      hook installed. One transcript cannot reach statements about the hook,
      which is what two of the documents are about. Every statement in each
      document about what happens to a project-declared tool either matches a
      transcript or is removed. The documents are a checklist and each is
      signed off.

- [ ] **AC30** AC4, AC33 and AC37 each assert all four of: the install instruction
      naming the declared recipe was printed; no elevation disclosure of the
      kind R11a requires appeared; none of the three mode-lowering gate
      identifiers appeared on stderr; and the declared recipe was not already
      installed at the declared version when the run began.

      The first is the positive observable, without which a run that crashed
      before reaching the mode passes every negative assertion — suggest exits
      1, which is indistinguishable from a general failure. The second pins the
      mode's first leg: R11a makes the raising observable the way R11 makes the
      lowering observable, so asserting both absences covers the whole
      trajectory rather than only its second half. The last two guard against a
      pass that measures something else. If a gate fires,
      the observer sees a prompt and records that `suggest` was honoured when
      the outcome was confirm — a weaker guarantee that disappears the moment
      the recipe gains a checksum. If the recipe was already installed, the
      fast path returns before any mode is consulted (R4, R16a) and nothing
      installs, so the observer records a protection that was never exercised.

      Asserting the absence of the identifiers is deliberately preferred to
      enumerating the preconditions that would produce them. An enumeration of
      preconditions was tried twice here and was incomplete both times: it is
      easy to exclude the verification and multiple-provider gates by choosing
      a single-provider command with a checksum, and then to miss the
      configuration-permission gate entirely, whose precondition is filesystem
      state rather than anything about the recipe. R11 makes each gate emit a
      distinct identifier, so their absence is directly observable, and the
      assertion survives a fourth gate being added.
- [ ] **AC31** The recorded rule states what happens when no consent mode is configured
      anywhere, and the shipped behaviour in that configuration matches it.

**If the rule is that a repository-supplied file may never raise the mode.**

- [ ] **AC32** With no consent mode configured anywhere, a project-declared tool prompts
      under the default mode rather than installing unattended.
- [ ] **AC33** With `suggest` set by the flag, by the environment variable, and by the
      configuration key in turn, a project-declared tool prints an install
      instruction and installs nothing.
- [ ] **AC34** `DESIGN-project-aware-exec.md` no longer promises the project config is
      consent, and states what replaces that promise.

**If the rule is that a declaration raises `confirm` to `auto` with an explicit
`suggest` as a floor.**

- [ ] **AC35** The elevation is disclosed to the user in the form the decision record
      names, before or at the first install the elevation enables, on a surface
      the user sees without opening the audit log.

- [ ] **AC36** With no consent mode configured, a project-declared tool installs without
      prompting; an undeclared command in the same project does not.
- [ ] **AC37** With `suggest` set by any of the three routes, a project-declared tool
      prints an install instruction and installs nothing.
- [ ] **AC38** With `confirm` set explicitly rather than by default, the behaviour is
      whichever the record chose, and the record says which — an explicitly set
      `confirm` and an unset default are distinguishable states and the rule
      must not leave them undecided.

**If the rule is bounded in some other way.**

- [ ] **AC39** The record states the boundary as a condition on observable state, and
      each side of that boundary has a criterion here before implementation
      begins.
- [ ] **AC40** Where the bounded rule still permits an elevation, that elevation is
      disclosed on the terms R11a sets. A bounded rule is not a forbidding one,
      so R11a is not vacuous here.

### Group C

- [ ] **AC41** (sign-off) `docs/guides/GUIDE-command-not-found.md` and the
      `tsuku hook` long help text each state that the hook calls `tsuku run`,
      that this installs and executes, and which setting governs whether it
      installs; and neither contains a statement describing the hook as
      advisory-only. The third statement is the one R16's first mitigation
      turns on.
- [ ] **AC53** Every sample transcript in either of those documents is
      reproduced by running the shipped hook in the state that document
      describes. This is executable and is separated from AC41 for that
      reason: a sign-off marker over a clause a machine can settle invites it
      to be settled by reading.
- [ ] **AC42** Every document offering any setting as a mitigation — not only a
      consent setting, so a re-scoped `TSUKU_CEILING_PATHS` entry is covered —
      names at least one case that setting does not cover, and the
      already-installed execution path is among them wherever the setting is a
      consent mode.
- [ ] **AC43** `DESIGN-project-aware-exec.md` lists at least one mitigation
      against the untrusted-repository threat, states that threat in the terms
      R16 gives it, and every mitigation it lists other than the one R16b
      defers is
      demonstrated to work against that threat by following it and then
      triggering the tool the way that mitigation is meant to intercept. For
      the hook mitigation that means typing the bare command in a shell with no
      hook installed, not invoking `tsuku run`, which installs whether a hook
      exists or not and would fail a mitigation that works. Deleting the section satisfies
      the second half and fails the first, which is the point: the obligation
      is to leave the user with something that works, not to leave them with
      nothing false. This criterion is re-run after R16b lands, because R16b
      rewrites the list it checks and a sign-off taken before that would attest
      to text that no longer exists. `TSUKU_CEILING_PATHS`, if it is still listed, is checked
      with the ceiling set to a parent of the repository, which is the usage
      the document's wording invites — the blanket-protection shape, which is
      the one that fails. A ceiling naming the repository's own path is not the
      test, because it works and proves nothing about the claim.
### Non-functional

- [ ] **AC54** With no `.tsuku.toml`, no terminal, and the tool already
      installed, `tsuku run <tool>` executes it and exits with the tool's own
      code rather than 12. This is R18a's named exception and the only exit
      code this work moves for a single-provider command.
- [ ] **AC55** With no `.tsuku.toml`, no terminal, and no index entry for the
      command, `tsuku run <command>` reports that no recipe provides it, on
      stderr, before exiting. The message is required because moving the
      terminal check changes this case from a specific code with an
      explanatory line to a bare exit 1, and a silent failure is a worse
      outcome than the one being replaced.
- [ ] **AC44** For a command with exactly one provider, the recipe chosen, the version
      chosen, whether a prompt appears and the exit code are unchanged from
      before this work, in the installed and not-installed cases and under each
      consent mode. The existing single-provider tests pass unmodified.
- [ ] **AC45** The design records the gates table's derivation against the rule
      stated with the table, naming the span's two boundaries by identifier and
      citing each site by file, function and role.
- [ ] **AC52** The recorded derivation's site list and the table's derived rows
      are the same set, checked mechanically. AC50 pins the code against the
      derivation and this pins the derivation against the table; without it the
      two could agree while the table every quantifier in this document reads
      says something else.
- [ ] **AC46** Over that named span, two searches together produce exactly the
      site list the derivation records — no site absent from that list, and no
      listed site absent from the code. The comparison target is the recorded
      derivation, per AC50; AC52 is what makes that equivalent to comparing
      against the table, and reading this criterion as a direct comparison
      against the table would build the comparison AC50 forbids. Both directions are checked, because
      a row whose site was deleted otherwise passes forever. The first search
      is for reads and writes of the effective mode. The second is for returns
      that reach an install or an exec without consulting the mode, which is
      the predicate that admits the fast path and excludes the config-load and
      index-lookup error returns that also precede the dispatch and are not
      rows. Both run as checks rather than sign-offs.
- [ ] **AC50** The check in AC46 reads its expected site list out of the
      derivation AC45 records, rather than out of the table or out of a list
      maintained beside it. A recorded derivation that stops matching the code
      then fails a check rather than waiting for someone to notice, which is
      the trigger R21 says has already gone stale twice in this document.
- [ ] **AC47** (sign-off) The candidate list is narrowed at exactly one site, where it
      is produced. A reviewer confirms no positional read of it occurs above
      that site, and that every read below it is unqualified. This is the half
      of R3a that behaviour cannot show: an implementation narrowing separately
      at each consumer passes every other criterion here while discarding the
      property that makes the next positional read reviewable.
- [ ] **AC49** A fixture index exists with every property its dependent criteria
      need, which are: one command with two providers sharing a version string,
      and one command with three; no recipe name appearing in any published
      registry manifest; recipes that install with no network access; each
      installed binary reporting its own path when run, which is how "executes
      recipe X" is observed at all; one declared recipe with no checksum
      verification alongside a sibling that has one, without which AC19 has no
      fixture; version strings including `latest` and a prefix that resolve
      offline, since resolution is a separate step from installation and every
      version provider is a network service; and the same substitution
      reachable from the `tsuku install` path.
- [ ] **AC48** Every multi-provider test case is constructed against the
      fixture index AC49 describes. A static check fails if a test in the
      affected packages constructs one any other way, and that check is itself
      exercised by a deliberately non-conforming fixture.

## What This Document Does Not Enumerate

Three enumerations are deliberately incomplete here, and each names the
artefact **obliged** to complete it rather than the place it might happen. A
deferral to "later" is how a thing becomes the one nobody enumerates, and there
are three of them.

- **The escape-hatch sets in AC25 and AC27.** Which hatches a message names is
  scoped by what the implementation's diff adds or changes, so the set cannot
  be listed before that diff exists. **Obliged: the PLAN**, which enumerates it
  per changed message before the work is broken into issues.
- **The criteria for the behaviour the consent decision selects.** This PRD
  cannot enumerate them without making the choice. **Obliged: the decision
  record**, per AC28, and per R13a for the rule behind any control the record
  introduces.
- **The gates table re-derived after this work changes the span.** R21 requires
  it, and the pre-change table is the one printed here. **Obliged: the DESIGN**,
  which records the derivation, and the check in AC46 thereafter.

"The PRD is done" and "we know everything we must test" are different claims.
This section is the difference.

## Out of Scope

- **Activation dropping `"latest"` and prefix pins** (tsukumogami/tsuku#2543).
  The same configuration file, a different consumer, and separately owned. It
  is excluded rather than deferred: nothing here depends on it, and nothing it
  does depends on this.
- **The multi-satisfier picker for `tsuku install`** (tsukumogami/tsuku#2368).
  Settled work on a different index, reached by a different question. This PRD
  must be consistent with it or argue why not, and does not reopen it.
- **The registry manifest's refresh cadence.** The published registry that
  `tsuku run` consults lags the recipe tree, which is why membership of the
  affected set differs depending on where it is counted. That is a real
  question about a different system, and R17 exists so this work does not
  depend on its answer.
- **The consent story for hooks and shims as mechanisms.** Both delegate
  entirely to `tsuku run` and read no configuration of their own. Their
  installation and opt-in stories are untouched; only their documentation is in
  scope, and only where it describes what `tsuku run` does.
- **Which recipes may be installed at all.** The curated registry, checksum
  verification and the root guard are the surrounding controls and are
  unchanged.
- **A tool for reading the audit log.** R12 changes what is written to it. What
  reads it is deferred, as it already was.
- **A `.tsuku.toml` syntax for saying which provider wins a bare command.**
  This is the obvious answer to the loss of function R6 accepts, and it sits
  directly in the path of anyone implementing R6, so it is excluded explicitly
  rather than by omission. Deferred rather than rejected: if the refusal proves
  common enough to be a nuisance, a disambiguation key is the natural follow-on,
  and it should be designed against evidence of how often the case arises.
- **Other callers of the changed lookup.** The declaration lookup R1 changes has
  one production implementer and one production caller, both on this path, so
  there is no third party to migrate. Recorded because R1 changes a signature
  and a reader is entitled to ask.

## Known Limitations

Refusing a two-provider declaration (R6) means a project that legitimately
declares two distributions of one tool cannot dispatch the bare command through
`tsuku run` at all. That is a real loss of function for a real configuration —
a repository building against one JDK and testing against another. It is
accepted because the alternative is picking one silently, and because the
declaration itself remains valid: both tools install, and both remain reachable
by their version-specific paths.

R7 requires the refusal to behave identically with and without a terminal,
which forgoes the interactive picker the install path offers in the same
situation. The reasoning is in Decisions and Trade-offs below. The cost is that
two commands answer a superficially similar question differently, and a user
who has seen the picker may expect it here.

The exit code for suggest-mode output is today indistinguishable from a general
failure, because both are 1. This PRD does not fix that, but it becomes more
visible if the Group B decision routes declared tools into suggest mode. It is
also the reason the CI-maintainer user story is only partly met.

A project that has been silently executing the wrong recipe will change
behaviour on the first run after this ships. Where `temurin` was declared and
`corretto` was being executed, the declared recipe resolves correctly, is found
not to be installed, and an install happens — on a setup that appeared to work.
That is the fix working, but it is a visible change to a working configuration
and the user is owed a reason. It is a limitation rather than a requirement
because the alternative is continuing to run the wrong tool.

## Decisions and Trade-offs

**A two-provider declaration is refused at dispatch, not rejected at parse
time.** The configuration is not ambiguous about what to install — it is
ambiguous only about which recipe wins an unqualified command name. Those are
different claims, and rejecting the file conflates them. `.tsuku.toml` is read
by batch install, by activation and by `tsuku run`, and only the last has a
question it cannot answer; a validation error would punish two consumers that
were working correctly, and would constrain the design of a separate piece of
work on the same file for a reason that exists only in this package.

**The refusal names the declared recipes rather than every provider.** This is
the one thing `tsuku run` knows that `tsuku install` does not: the project has
already narrowed the field. Reporting all five providers of `java` when the
project declared two would discard that.

**No interactive picker, in either mode.** `tsuku install java` is a person
typing an ambiguous name in the moment, and asking them is right. A
`.tsuku.toml` declaring two providers is a file that was supposed to have
settled the question, and answering it per invocation is not reproducible — the
same command in the same repository would depend on who ran it. The failure
shape converges with the install path; the disambiguator diverges, because
there the fix is a flag and here the fix is editing the declaration.

**What the install path's PRD does and does not settle for this one.** It
settles a layering rule: an explicit naming terminates resolution rather than
joining the candidate set, so a direct-name match is not consulted against the
alias index at all. It does not settle that explicit intent generally outranks
index ordering — its stated reason is narrower, that recipe authors keep
control of their own canonical names, which is a claim about authors rather
than about users. So this PRD applies that layering rule where the other did
not reach, and says so; it does not claim the question was already answered.

It is also worth recording that the install path's PRD names `tsuku run`
exactly once, in its own out-of-scope list, and describes it as retaining
"existing first-match-or-error semantics". That was a deliberate exclusion
rather than an oversight. It is not an endorsement of the behaviour this PRD
treats as a defect: first-match is a reasonable answer when nothing has
disambiguated the command, and the defect here is that first-match overrides a
project configuration that did. The other document was not considering project
configuration at all.

**The multiple-provider gate needs no exemption, and finding that out moved
three issues out of the contingent group.** The gate's rule is that an
*ambiguous* command must not silently install an arbitrary recipe. A project
declaration is what makes the command unambiguous, so once the candidates are
narrowed the gate's precondition is absent and it does not fire. This is worth
stating precisely because the alternative framing — exempting declared tools
from a security gate — sounds like widening auto mode and would have had to be
argued as such, and it would have been argued in the middle of the consent
decision. Declining to apply a control whose precondition has gone is a
different claim, and it is the true one.

**The verification gate must move with the installer, and the reason is worth
keeping.** Today the verification gate and the installer read the same
candidate, so the checksum that is checked belongs to the recipe that gets
installed. Both are wrong about which recipe the project wanted, but they are
wrong together, and nothing fails open. Narrow the installer to the declared
recipe and leave the gate reading the index's ranking, and the gate passes on a
sibling's checksum while installing a recipe that has none — a security control
reporting success about an artefact it did not examine. That is worse than the
defect being fixed, and it would arrive through the fix. A reviewer who knows
why the two must move together will not later separate them for tidiness; one
who sees only an ordering constraint might.

**The post-install exec reads `tools/current`, and that is correct.** The fast
path fifteen lines earlier says explicitly that it execs from the
version-specific directory "not from `tools/current/`", so the neighbouring
line looks like a contradiction and was investigated as one. It is not: the
install that just ran repoints `current/<binary>` at the version it installed,
so the symlink names the right thing at the moment it is read. Recorded here,
and to be recorded in a comment at the site, because the next reader will ask
the same question and the answer currently exists only in an investigation.

**The consent question is not answered here.** It is a trust boundary, the
evidence points both ways, and it is expensive to revisit — so it goes to a
recorded decision with the alternatives named, rather than being settled by
whoever writes the requirements. Three alternatives are live and all three must
be weighed: that a repository-supplied file may never raise the consent mode,
which is what the existing environment-variable rule already enforces against a
`.envrc` in a cloned repository; that a declaration may raise `confirm` to
`auto` while an explicitly set `suggest` remains a floor; and a bounded rule
between them. The first forbids the second's elevation as well as the current
behaviour, which is easy to miss.

**The upstream brief's open question about interactive ambiguity is closed by
the third decision above.** Its open question about the consent floor remains
open by design and is owned by the decision record. Its carried assumption —
that the terminal guard's fix follows from the same answer without needing a
decision of its own — is confirmed: R9 consumes R1's lookup directly.
