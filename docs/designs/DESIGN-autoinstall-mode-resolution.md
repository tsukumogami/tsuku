---
schema: design/v1
status: Planned
upstream: docs/prds/PRD-autoinstall-mode-resolution.md
problem: |
  `tsuku run` is keyed by command while `.tsuku.toml` declares recipes, so it
  must invert a many-to-one mapping to learn what a project asked for. The
  resolver that performs the inversion returns a version and discards which
  recipe it matched, so six later reads of the candidate list decide from the
  binary index's own ranking instead. Separately, a declaration raises the
  consent mode unconditionally, which contradicts the rule the project already
  applies to environment variables and the mitigation its own design document
  offers.
decision: |
  Replace the version-only resolver with one that returns the set of declared
  recipes providing a command, deduped by bare recipe name, and consume it
  inside a new `Runner.candidates` method so the region above the narrowing
  inside `Run` is empty of the list entirely. Narrowing is three-way: zero declarations
  pass the full list through, one narrows to it, more than one refuses at exit
  10 naming the declared recipes. Move the terminal check inside `Runner.Run`
  where the declaration is known, after the gates. On consent, adopt bounded
  elevation: a declaration may raise `confirm` to `auto` for the commands it
  declares and nothing else, an explicitly set `suggest` is a floor, and every
  elevation that results in an install is disclosed naming the authorizing file.
rationale: |
  Scope confinement is chosen over a named type because Go assignability makes
  a named slice type convertible in both directions, so it cannot produce the
  compile error it appears to promise. Bounded elevation is chosen over strict
  parity because strict parity does not fix the harm it is aimed at — the
  registration path that admits an unapproved source runs before any consent
  mode is consulted — while the security case for elevation rests on one leg,
  enumeration, with the remainder being product value. The four conditions are
  written as requirements rather than assumed, which is the difference between
  this and unbounded elevation.
---

# DESIGN: Autoinstall Mode Resolution

## Status

Planned

## Context and Problem Statement

The requirements are in `docs/prds/PRD-autoinstall-mode-resolution.md` and are
not restated here. What this document settles is how they are built, and the
one question the PRD deliberately left open.

Two facts about the existing code shape every decision below.

**The candidate list is read six times, all inside `Runner.Run`.** Produced at
`internal/autoinstall/run.go:64`; cardinality at `:78` and `:153`; positional
at `:101`, `:112` and `:120`. One producer, five consumers. That count is what
makes the fix reviewable by position rather than by reconstructing intent, and
it is why the shape of the narrowing matters more than the shape of the data.

**The `Runner` is already fully injectable and the layer above it is not.**
`Lookup`, `Installer`, `Exec` and `RecipeHasVerification` are function-shaped
fields (`internal/autoinstall/autoinstall.go:64-100`), and `Run` reaches the
index, the installer and the process through nothing else. The wiring in
`cmd/tsuku/cmd_run.go:97-100` builds the lookup closure inline over an
unexported free function with no seam, which is why `cmd_run.go` has no test
covering it and why the terminal check as currently placed cannot be exercised.

## Decision Drivers

- **Reviewability by position.** The defect being fixed is a positional read
  nobody noticed. A fix that leaves the next positional read equally invisible
  has not paid for itself.
- **Group A must not wait on the consent decision.** The identity defects ship
  regardless of how D1 lands; a design that couples them has broken the PRD's
  central structural claim.
- **No control may report over less than its rule covers.** Three defects found
  while writing the PRD were of that shape, and one more was found while
  writing this document.
- **Existing precedent is preferred to new invention** where one exists —
  exit codes, error-envelope shapes, and the escalation asymmetry the project
  already applies to environment variables.
- **The registry membership is not stable.** Tests must not name registry
  recipes; the recipe tree and the published manifest already disagree.

## Considered Options

### D1. How far may a project declaration move the consent mode

This is the decision the work exists to record, and tsukumogami/tsuku#2544's
first acceptance criterion asks for it by name. Three alternatives were live
and each had a case made by someone who did not hold it.

**(a) Strict parity.** A repository-supplied file may never raise the consent
mode. `docs/designs/current/DESIGN-auto-install.md` Decision 2 already refuses
exactly this for `.envrc` — "prevents a malicious `.envrc` in a cloned
repository from silently bypassing the user's consent configuration" — while
permitting free downgrade. On this reading, "the project config is treated as
consent" was the mistake and the promise is what gets retracted. Its strongest
form is not that auto-install is dangerous but that the project already decided
this question for a structurally identical input and decided it the other way;
holding two incompatible positions about whether a repository-supplied file may
raise a consent mode is a defect whichever is right, and parity generalises a
rule that is already implemented rather than arguing a new one into being.

**(b) Bounded elevation.** A declaration raises `confirm` to `auto`; an
explicitly set `suggest` is a floor the config cannot raise. The case: `confirm`
is the unset default rather than a choice, so overriding it costs a user
nothing they asked for, while `suggest` is never a default, so a user who set it
has said "never install unattended" — and a file shipped by the cloned
repository is the one case where the config's author and the person being
protected are different people.

**(c) Bounded elevation with its conditions written as requirements.** The same
behaviour as (b), with the properties it depends on stated as load-bearing text
rather than assumed.

**What decided it.** Three things, none of which was available when the
alternatives were framed.

First, **the distinguisher is one leg rather than three.** The case for (b)
rested on a declaration being enumerated, version-controlled and reviewable
where an environment variable is ambient and unbounded. Two of those three
separate nothing: a `.envrc` is also a file in the repository and also
version-controlled — direnv ships an allow mechanism precisely because
reviewable is not reviewed. What survives is enumeration, and its sharpest
counter is correct: the attacker picks the bound, so a hostile repository
declares exactly the tool it wants and no more. Boundedness limits magnitude
rather than establishing consent.

Second, **(a) does not fix the harm it is aimed at.** A `.tsuku.toml` can cause
`tsuku install` to auto-register an attacker-named source non-interactively
(tsukumogami/tsuku#2552). That registration happens on the install path, before
any consent mode is consulted. Under (a) the user gets `Install jq@1.7? [y/N]`
— a prompt that does not name the source — so (a) converts a silent install
from an attacker-registered source into a prompted one, disclosing nothing
about the part that matters. The test applied was whether switching to (a)
reduces harm; it relocates it.

Third, **the one real consumer has no stake.** A workspace that depends on
`.tsuku.toml` and prompted this investigation pairs it with a setup script that
runs `tsuku install` explicitly — written as a workaround *because*
declaration-as-consent did not work. It would not change a line under any of
the three. One workspace is not a survey, but it is the only usage evidence
anyone produced, and it is evidence that (b)'s convenience is worth less than
assumed.

**Chosen: (c).** Bounded elevation, with four conditions as requirements:

1. **The elevation is bounded to declared commands.** A declaration raises the
   mode for the commands it names and for no other. This is #2542's fix and
   #2545's, and it comes free with them.
2. **The elevation cannot reach an unregistered source.** Verified for the run
   path three ways below; false on the install path, and the claim is scoped
   accordingly.
3. **An explicitly set mode is honoured as given.** Only the *unset default* is
   raised. This follows from the argument rather than being added to it: the
   case for raising `confirm` is precisely that `confirm` is a default nobody
   chose, and that reasoning does not survive someone choosing it. So `suggest`
   is a floor, and so is an explicitly set `confirm`, and an explicitly set
   `auto` is already what it is. AC38 asked what an explicitly set `confirm`
   does and this is the answer: the same as an explicitly set anything.
4. **Every elevation that results in an install is disclosed**, on the terms
   D5 sets.

One case makes condition 3 load-bearing rather than tidy. `resolveMode`
downgrades an environment-supplied `auto` to `confirm` when the persistent
config does not corroborate it — that is the anti-`.envrc` control this
decision spends a page comparing itself to. Its output is a `confirm` whose
origin is `environment`, not `default`. Raising every `confirm` would take that
control's output and put it straight back to `auto`, which would not merely
decline parity but reverse it. Condition 3 forbids that, because the origin is
not `default`.

Condition 3 also fixes `project`'s rank in R12's origin order, which R12 left
to this decision. A declaration outranks `default` and nothing else, so the
recorded origin is `project` only where the mode would otherwise have been
`default`. Where a flag, the environment or the config supplied a value, that
source is both honoured and recorded. The
honest statement of why is that the security case for (b) over (a) is one leg,
and the remainder is product value — `confirm` is a default nobody chose and
prompting for every declared tool is friction without value in the common case
of a repository the user already trusts. That is a legitimate basis for a
product decision and it is not a security argument; this document does not
dress it as one. What makes (c) rather than (b) defensible is that the
conditions are written down, and in particular that disclosure is required:
bounded elevation without disclosure is the status quo with better
documentation.

**The criteria this decision is tested against**, which AC28 requires the record
to carry rather than leave to the PRD, because the PRD could not enumerate them
without making the choice:

- **D1-1.** With no consent mode configured anywhere and a declared command,
  the tool installs without prompting. The elevation happened.
- **D1-2.** With no consent mode configured anywhere and an *undeclared*
  command in the same project, the behaviour is unchanged from a directory with
  no `.tsuku.toml`. The elevation did not leak.
- **D1-3.** With `suggest` set by the flag, by the environment variable, and by
  the configuration key in turn, a declared command prints an install
  instruction and installs nothing. The floor holds through all three routes.
- **D1-4.** With `confirm` set explicitly by any of those three routes, a
  declared command prompts. An explicitly set mode is not raised, which is
  condition 3 and the half of R15a the earlier draft left unanswered.
- **D1-5.** With `TSUKU_AUTO_INSTALL_MODE=auto` set and no corroborating
  config — the state where the escalation restriction downgrades to `confirm` —
  a declared command prompts. The declaration does not re-raise the output of
  that control.
- **D1-6.** Every install reached through D1-1 discloses the recipe, the
  version, the authorizing file's path and the recipe's source, before the
  install begins.

D1-5 is the criterion that distinguishes this decision from the one it was
nearly confused with, and D1-2 is the one that distinguishes bounded elevation
from unbounded.

**A precondition that has already partly failed, recorded so it can be
rechecked.** (c) depends on a declaration being unable to influence *which
registry* a recipe comes from. On the run path that holds, three ways:
`installArgs` (`cmd/tsuku/install_deps.go:182-198`) has no source field at all;
`runInstaller.Install` (`cmd/tsuku/cmd_run.go:152-162`) passes a bare recipe
name taken from the binary index rather than from the config key; and none of
`ensureDistributedSource`, `autoRegisterSource` or `checkSourceCollision` is
reachable from it. It does **not** hold on the install path, where
`parseDistributedName` keeps the source component and registers it. So the
scoped claim is true and the unscoped one is false, and the unscoped one is
what a reader would derive from the argument above if it were not said here.
**If the source component is ever honoured on the run path, this decision is
reopened.**

### D2. What the declaration lookup returns

**Considered:** a slice of value structs; a named collection type with `Sole()`
and `Len()`; a narrowed `[]index.BinaryMatch` plus a parallel version map.

The named-collection option was rejected on a concrete rather than stylistic
ground: `Sole() (ProjectDeclaration, bool)` has a false branch that conflates
zero with many, and R5 and R6 require opposite behaviour for those two, so the
caller consults `Len()` anyway and the method earns nothing.

The parallel-map option was rejected because a slice plus a map keyed by recipe
carries an unstated invariant — every element has an entry — that nothing
enforces, and the defect being fixed *is* a lost recipe-to-version association.
Reinstating it as a lookup that can miss is the wrong direction. It also splits
the narrowing across a package boundary.

**Chosen:**

```go
type ProjectDeclaration struct {
    Recipe     string // bare recipe name
    Version    string // as declared, verbatim (R20)
    ConfigKey  string // the .tsuku.toml key (R6, AC14)
    ConfigPath string // the file that declared it (R13a, D5)
}

type ProjectDeclarationResolver interface {
    DeclarationsFor(ctx context.Context, matches []index.BinaryMatch) ([]ProjectDeclaration, error)
}
```

`ConfigKey` is load-bearing rather than decorative: AC14 requires the refusal to
name the key each recipe was declared under. `ConfigPath` is there for the same
kind of reason and would otherwise be discovered missing at implementation:
D5's disclosure rule requires the authorizing file's path, the disclosure is
emitted inside `internal/autoinstall`, and that package has no route to
`project.ConfigResult.Path` except through this struct. It is identical for
every element; carrying it per-declaration is the cost of not adding a second
method to the interface. The `ok` bool disappears into
`len(set) == 0`, which is how R1 defines the empty case — the current
signature's defect is returning two values that can disagree about what was
found, and a `Len()`-plus-`ok` pair would reintroduce that class.

Taking `matches` rather than `command` removes a redundant index open. Today
`cmd_run.go:97-100` hands one closure to both the runner and the resolver;
`Run` calls it at `run.go:64` and `ProjectVersionFor` calls it again at
`resolver.go:61`, and `lookupBinaryCommand` opens and closes the SQLite file
per call with no cache — two opens when a `.tsuku.toml` is found, one when not.
Under `DeclarationsFor` the resolver never looks up.

**Dedup belongs in the resolver.** It needs `SplitOrgKey` and `bareToOrg`,
neither reachable from `internal/autoinstall`, which never sees config keys.
One rule the current code never had to state: `ProjectVersionFor` returns on
the first hit, so its bare-before-org precedence is per-iteration and
incidental. Collecting all matches forces it to be stated per recipe — for each
distinct bare name the config declares, take the bare key's version if present,
else the first org key's, which is well-defined because `buildBareToOrgMap`
sorts.

**Correction, from building it (Issue 2).** That sentence is the rule AC11a
names as the dangerous wrong one, because it has no carve-out for a collision.
It holds only where the config declares at most one org-scoped source for the
bare name. Where it declares two or more, the set is one declaration per
source and a bare key is dropped rather than being allowed to pick: the
collapse identifies the bare key with each org key separately, so it adds no
recipe the set does not already carry and cannot say which of the two it
meant.

`Resolver.Tools()` is removed: zero production callers, and its documented
purpose is what `hasProjectTools` did, which R9 deletes.

**`bareToOrg`'s stderr warning is replaced by a refusal, not kept alongside
one.** It fires when two org-scoped keys reduce to one bare name, and an
earlier draft of this design kept it because R2 collapsed that case to a single
declaration with a sorted-first winner, leaving the warning as the user's only
signal. R2a reverses that: two org keys with differing org components are two
declarations, so the case reaches R6 and refuses.

The reason the earlier answer was wrong is that a warning does not stop the
wrong tool from running, and the winner was chosen by `buildBareToOrgMap`'s
sort — an ordering the user never expressed. That is precisely the behaviour
tsukumogami/tsuku#2542 is about, surviving inside the fix for it. The same
shape was independently observed as a live defect in the activation resolver,
where two colliding org keys put one tool on `PATH` twice and reported nothing;
seeing it happen rather than reasoning about it is what settled this.

Keeping both would be worse than either. A refusal that also prints a warning
about the condition it just refused invites a later reader to delete one of
them, and there is no way to tell from the code which.

**Correction, from building it (Issue 2).** "The case reaches R6 and refuses"
is true of what the warning was about and not of everything it fired on. Its
condition was two or more org-scoped keys reducing to one bare name, which
includes two keys naming the *same* source — `org-a/koto` and
`org-a/koto@2.0.0`, which `SplitOrgKey` reduces alike. Those denote one recipe,
so they stay one declaration and are still settled by a sort with nothing
printed. The warning went for that subset too.

That is accepted rather than overlooked. R2a's stated harm is running the wrong
tool, and here there is one recipe to install whichever key wins, so the pick
can only choose a version of the right tool — the same trade R2 and AC12
already make when a bare key outranks an org key. It is weaker than R2's rule
in one respect worth writing down: R2 names its winner, and this one is
whichever key is lowest by string order.

### D3. How the single-production-site property is enforced

**Considered:** plain reassignment of `matches`; a distinct named type; scope
confinement.

**The named type does not work, and this is worth recording because it will be
proposed again.** `type declaredCandidates []index.BinaryMatch` and
`[]index.BinaryMatch` are assignable in both directions without conversion,
because assignability holds when the underlying types are identical and at
least one side is unnamed — and the slice literal type is unnamed. So the wrong
edit compiles. And it leaves nothing behind: passing a plain
`[]index.BinaryMatch` to a parameter of the named type compiles with **no
conversion at all**, so extracting the consumers into functions taking the
named type does not even produce a reviewable artefact. R3a's "checkable by position"
is the ceiling, not a waypoint toward compiler enforcement.

**Plain reassignment** leaves the wrong edit — a shadowed `matches :=` inside a
block — catchable by a reviewer only. `.golangci.yaml` enables `govet` without
`shadow`, which is off by default, and `ineffassign` catches an assignment
never read rather than a shadow read within its own block.

**Chosen: scope confinement.**

```go
// candidates looks command up in the binary index and narrows the result to the
// recipe the project declared, where it declared exactly one that provides the
// command.
//
//   - no declaration provides it: matches unchanged, nil declaration. The
//     command resolves exactly as it would with no .tsuku.toml present (R5).
//   - exactly one does: matches filtered to that recipe, plus the declaration.
//   - more than one does: AmbiguousDeclarationError carrying all of them (R6).
func (r *Runner) candidates(
    ctx context.Context,
    command string,
    resolver ProjectDeclarationResolver,
) ([]index.BinaryMatch, *ProjectDeclaration, error)
```

The reason this beats reassignment is not tidiness. AC47 asks a reviewer to
confirm narrowing happens at one site and that no positional read occurs above
it. Under reassignment that is a genuine read of ~150 lines hunting a shadow
nothing in the linter set sees. Under confinement the region above the
narrowing site inside `Run` is **empty by construction** — the list does not
exist there — so half of AC47 is discharged without reading anything, and what
remains is roughly thirty lines with three return paths.

Its cost is R21: the span the gates table is derived over is bounded by the
index lookup and the mode dispatch, and those now sit in two functions, so
AC46's search spans both. The recorded derivation must name two **function**
boundaries rather than two line markers. R21 already requires re-derivation
because this work changes the span regardless.

**Narrowing is three-way, and the zero case is not a narrowing.** The branch
sits below `run.go:78`'s `ErrNoMatch`, which stays on the raw list: an empty
index result is a lookup failure rather than a declaration outcome. When
nothing is declared the full list passes through, and the consumers below read
position zero of an un-narrowed list — which is required, because R5 says an
undeclared command resolves as it would with no `.tsuku.toml`, and that is the
index ranking. Narrowing the zero case to the empty set would turn every
undeclared command into `ErrNoMatch`, and suppressing the multiple-provider
gate for undeclared ambiguous commands is not something R3b licenses.

So **"correct by inheritance" holds for the declared case and is not a general
property of the consumers.** The PRD's R3a said otherwise and has been
corrected.

One bookkeeping consequence: R3a counts five consumers, and after this change
four of them read the narrowed list. The `len(matches) == 0` test stays above
the narrowing, on the raw list, because an empty index result is a lookup
failure rather than a declaration outcome. AC47 is still discharged — that read
is a cardinality test rather than a positional selection, and AC47's question is
about positional reads — but a reviewer counting five and finding four should
find this sentence rather than a discrepancy.

**Correction, from building it (Issue 3).** The count is now two, not four, and
the paragraph above is the sentence a reviewer would otherwise trip on. Once
the list is confined to `candidates`, the three positional selections inside
`Run` have no reason to stay separate: `Run` reads position zero once, into a
`match` that the sites below read instead, and the only other read of the list
is the conflict gate's `len(matches) > 1`. So below the narrowing there is one
positional selection and one cardinality test; above it, on the raw list,
`len(matches) == 0` and the whole-list hand-off to `DeclarationsFor`, which
ranges rather than selecting.

That strengthens the property rather than weakening it — R3a's review
instrument asks a reviewer to check positional reads, and there is now one to
check — but it means the arithmetic in this paragraph and in R3a is the
pre-implementation arithmetic. Both are left standing with this note beside
them, because the counting was the reasoning that produced the confinement and
deleting it would leave the conclusion with nothing under it.

### D4. The refusal's shape

Exit 10 is settled by R7, and `ExitAmbiguous = 10` exists — but `tsuku run`
uses it nowhere, so R6 needs its own sentinel:

```go
type AmbiguousDeclarationError struct {
    Command      string
    Declarations []ProjectDeclaration
}
```

with a new case in the switch at `cmd_run.go:130-145`.

**Convergence with the install path is on failure shape, divergence is on the
disambiguator, and the divergence is argued rather than inherited.**
`docs/prds/PRD-multi-satisfier-picker.md` answers multi-candidate ambiguity
with a TTY picker (R4) and a non-TTY error listing candidates (R5). This design
takes R5's shape in both modes and declines R4's picker in either.

The reason is not that a picker is bad. `tsuku install java` is a person typing
an ambiguous name in the moment, and asking them is right. A `.tsuku.toml`
declaring two providers is a *file* that was supposed to have settled the
question, and answering it per invocation is not reproducible — the same
command in the same repository would depend on who ran it.

Two further notes on that document, so a reader knows it was consulted rather
than assumed. Its R11 — direct-name match beats a multi-satisfier alias — is
sometimes read as settling that explicit intent outranks index ordering. It
does not: its stated reason is namespace ownership, that recipe authors keep
control of their own canonical names, which is a claim about authors. What is
carryable is the layering rule underneath, that an explicit naming terminates
resolution rather than joining the candidate set, and this design applies that
where #2368 did not reach. And #2368 names `tsuku run` once, in its own
out-of-scope list, as retaining "existing first-match-or-error semantics" —
a deliberate exclusion, and not an endorsement, since it was not considering
project configuration.

**The invocation the refusal offers is `tsuku install <recipe>` followed by the
version-specific path.** `tsuku run` has no per-recipe disambiguator, and a
`.tsuku.toml` disambiguation key is explicitly out of scope, so there is no
one-command answer and inventing CLI surface here would exceed the requirement.
What exists and works is: install the declared recipe by name, then invoke it at
`$TSUKU_HOME/tools/<recipe>-<version>/bin/<command>`. AC26 requires the named
invocation to run one of the declared recipes to completion rather than to be a
single token, so this satisfies it. `AmbiguousDeclarationError` needs no extra
field: the message is composed from the `Recipe` and `Version` it already
carries, and the tools directory is the runner's own configuration.

**What the refusal knows that the install path does not:** the config has
already narrowed the field. Reporting all five providers of `java` when the
project declared two would discard that, so R6 requires the declared names,
their versions and their config keys — strictly better than a picker over the
full candidate set.

### D5. Gate announcements and the elevation disclosure

The three mode-lowering gates each emit a stable, distinct identifier plus the
condition that fired it. That is R11, and it turns out to be the test seam for
every "the mode was not diverted" assertion in the criteria — asserting the
absence of the three identifiers is what lets a test establish that no gate
intervened, without enumerating the preconditions that would make one fire.
An enumeration was attempted twice while writing the PRD and was incomplete
both times. **A later simplification that collapses the three identifiers into
one generic line removes that seam along with them.**

For R11a, both reviewers converged independently on **disclose once per install
the elevation enables**, from different arguments. Per-invocation is wrong on
the facts: most invocations of a declared tool install nothing, because the
already-installed fast path returns before any mode is dispatched, so a line
there reports a decision that is not being made — and trains the reader to
ignore the line that matters. Per-project is wrong on the consequence: the
second declared tool then installs in silence.

Per-install lands once per tool per version per project by construction, needs
no new state, and rides on output the user already receives.

**The rule R13a requires, written before the control exists:** *an install
whose recipe or whose consent mode was determined by a project declaration
shall state, before the install begins, the recipe, the version, the path of
the file that authorized it, and the recipe's source.*

The source is there because the security section concludes it must be: the run
path inherits #2552's registration exposure, so a recipe can be reached from a
source the user never approved, and a disclosure naming only the file would not
say so.

"Determined the recipe or the mode" rather than "raised the mode", and the
widening is not cosmetic. R3b removes the multiple-provider gate's precondition
for a declared command, so a user in an explicitly configured `auto` who runs
an ambiguous command in a repository that declares one provider gets a silent
install where they previously got a prompt. No elevation occurred, so the
narrow rule says nothing; no gate changed the mode, so R11 says nothing. A
repository-supplied file would have converted a prompt into a silent install
with nothing reporting it — which is this document's own driver inverted, and
it ships in Group A whatever D1 decided.

The file path is the part not to drop. "This was authorized by
`/home/you/src/theirrepo/.tsuku.toml`" is a sentence someone can act on;
without it, disclosure tells the user an install happened, which they can
already see.

### D6. The audit record

`auditEntry` gains an origin field holding one of `default`, `flag`,
`environment`, `config`, `project`, and a field naming the gate that lowered
the mode where one did. `writeAuditLog` takes them rather than hardcoding
`"auto"`, and is called on every install rather than only the auto path —
which is the gap R12a names, where precisely the installs a gate diverted leave
no trace.

Nothing reads the log but two tests, one of which asserts `mode == "auto"` and
is updated rather than removed. `DESIGN-auto-install.md:311` records that no
tooling consumes it, and a reader is entitled to that fact rather than to an
assurance that the change is safe.

### D7. The test seam

**Most of it already exists**, which the PRD overstated. `Runner` is injectable
through four function fields, and `TSUKU_HOME` redirection relocates the index
path, the registry cache **and** `RecipesDir` — which `main.go` makes the
highest-priority recipe provider with a flat layout, so a fixture TOML there is
loaded by the real provider chain, offline, ahead of embedded and central.

**Chosen: `TSUKU_HOME` redirection plus an index built in-process.** No
`update-registry` and no server: `registry.Registry.GetCached`/`ListCached` are
plain reads of the cache dir, so writing fixture TOMLs into
`$TSUKU_HOME/registry/<letter>/<name>.toml` and calling `Rebuild` produces a
genuine index with no network. A package var at the lookup boundary is added
separately, for testing the *wiring* in `cmd_run.go` — which has no test today
— but not as the fixture source, because a hand-written `[]index.BinaryMatch`
is exactly what AC48's check must reject.

**Two of R19's eight properties do not survive contact, and both are recorded
rather than assumed.**

**A recipe with no checksum verification cannot be constructed.** The reason
is a defect in a control outside this work's scope, reported through the
project's security policy and tracked there rather than described here.

**AC19 has been restated so that it does not depend on that defect.** It now
asserts which recipe the gate is *invoked with* rather than what the gate
answers — which is R3a's actual claim, and is observable today through the
injectable `RecipeHasVerification` field. The fixture property the original
form would have needed is therefore not required either.

The defect is real and its fix has a blast radius outside this PRD, so it is
tracked separately and not folded in here. What this work owes it is
documentary, and the obligation is discharged by *removing* the affected
control from the mitigations `DESIGN-project-aware-exec.md` offers against the
untrusted-repository threat, rather than by describing what is wrong with it.
A document must not offer a mitigation that does not hold; it is under no
obligation to explain a defect that is being handled elsewhere.

A consequence worth stating plainly: **R3a's protection is forward-looking.**
The hazard it guards against — narrowing the installer without narrowing the
gate, so that a control reports success about an artefact it did not examine —
is not reachable while the gate is unavailable as a control. That is not a
reason to relax R3a. It is a reason to build it now, so that the protection is
already in place when the gate becomes live rather than being remembered then.

**`latest` resolves offline; a prefix does not.** `http_json` takes its
endpoint from the recipe and does not enforce HTTPS at runtime, so a fixture
pointing at an `httptest` server resolves `latest` through production code with
no internet. "Offline" here means no external network rather than no sockets,
and AC49 should say so, because a hermetic-CI reader will read it the other
way. A prefix is different: `ResolveWithinBoundary` narrows one only for
providers implementing `VersionLister`, and `HTTPJSONProvider` has none, so the
constraint passes through verbatim. AC18's prefix half needs a fixture provider
or it is cut.

**Correction, from building it (Issue 1).** The sentence above originally said
the HTTPS rule is the validator's alone and the validator does not run on the
install path. The second half is wrong: `cmd/tsuku/install_deps.go` calls
`recipe.ValidateRecipe` before any plan is generated and hard-fails on its
errors, and `internal/recipe/validator.go` rejects a non-HTTPS `http_json`
`url`. So a fixture recipe pointing at a plain `httptest` endpoint cannot be
installed at all, and `tsuku install` against the fixture — which AC16 needs —
is impossible in that shape.

What Issue 1 built instead: the fixture recipes name an unreachable
`https://...invalid` endpoint, and installs pin an exact version.
`HTTPJSONProvider.ResolveVersion` returns an exact version unchanged without
fetching, so a pinned install completes with no socket opened. The local
`httptest` endpoint stays, and `latest` resolution is demonstrated against it
through the production provider directly. The consequence for later units is
that a `tsuku install` against the fixture must pin `indexfixture.SharedVersion`
— an empty constraint silently falls back to the `dev` version and `latest`
fails.

**Correction, from building it (Issue 2).** "AC18's prefix half needs a fixture
provider or it is cut" is wrong, and R20 is why. R20 puts resolution of a
non-exact version outside this work — it stays the installer's job, unchanged —
and asks only that carrying the recipe identity does not change what is carried
alongside it. AC18's subject is therefore which recipe was chosen, and both
halves are observable at the declaration layer, where the string is carried
verbatim and never resolved. The mechanism the sentence above describes is
accurate; the conclusion drawn from it was not. No fixture version provider is
needed and none was built.

**The R17 check is an AST pass keyed on element count, not a grep on names.**
A grep cannot work, for the reason R17 itself gives: several registry names are
ordinary words, so text matching cannot separate a deliberate fixture from an
incidental mention. The mechanism is a sanctioned constructor plus a check that
it is the only way in — a `go/parser` pass over `_test.go` files in
`internal/autoinstall`, `internal/project`, `internal/index` and `cmd/tsuku`,
which are the only packages that construct or consume `BinaryMatch`, failing on
a composite literal of two or more elements, or a recipe map reaching `Rebuild`
with two keys yielding the same command.

Keying on count rather than name is what makes it writable. Roughly
twenty-five single-element literals across `autoinstall_test.go` and
`resolver_test.go` are legitimate single-provider cases under R18; any
name-keyed rule flags every one of them. The count rule flags none, and flags
exactly three real violations — `internal/index/lookup_test.go:43-46`,
`internal/autoinstall/autoinstall_test.go:330-333` and
`internal/project/resolver_test.go:134-137`, each naming a pair that exists in
the recipe tree — which migrate to the shared helper.

AC48's requirement that the check be exercised by a non-conforming fixture is
met by putting a two-element literal naming real recipes under `testdata/`,
which the existing AST walker already skips, and asserting the checker reports
it. `internal/index/lookup_test.go:81-85` is a ready-made positive control: a
three-provider case built from names that appear nowhere in the tree.

The repo already has the scan shape twice — `lint_test.go:102-153` for the AST
walk and `cmd/tsuku/install_reinstall_test.go:644-684` for a per-package source
scan ending in a matched-nothing guard, which this check copies so it cannot
pass by scanning zero files.

**The fixture work is lifting two existing helpers, not writing new ones.**
`newReinstallHarness` (`cmd/tsuku/install_reinstall_test.go:108-137`) already
gives a temp `TSUKU_HOME`, a swapped loader with in-memory recipes, and steps
that install offline; `internal/index/rebuild_test.go:14-131` already gives
`stubRegistry`, `minimalRecipeTOML`, `openTestIndex` and `buildIndexWithRows`
across 41 call sites. The rest of the tree is thinner than it looks:
`testdata/recipes/` is consumed only by a shell script and a workflow,
`internal/sandbox/` is Docker recipe validation, and the other `TSUKU_HOME`
redirections build no index.

## Decision Outcome

Group A is built as: a declaration-set resolver (D2), consumed inside
`Runner.candidates` so the region above the narrowing inside `Run` holds no list at all
(D3), with narrowing three-way and the zero case an explicit passthrough; a
refusal carrying the declared recipes, versions and config keys at exit 10
(D4); the terminal check moved inside `Run`, after the gates, where the
declaration is known; per-gate announcements (D5); and an audit record naming
the mode's origin and any gate that changed it (D6).

Group B is bounded elevation with its four conditions as requirements (D1).
Group C is the documentation, which is wrong today and does not wait.

**Placement of the terminal check is load-bearing.** It sits after all four
gates, immediately before the dispatch switch. The gates only lower `auto` to
`confirm`, so a check placed before them lets a gate-lowered run reach the
prompt with no TTY and exit 13 — which is verbatim the problem this PRD opens
with. After the gates closes that, and it is what makes AC2 and AC20 both
reachable.

## Solution Architecture

The order inside `Run` becomes: root guard, `candidates()`, already-installed
fast path, the four gates, terminal check, dispatch.

`candidates()` contains the lookup with its existing `ErrIndexNotBuilt` and
`StaleIndexWarning` handling, the `ErrNoMatch` check on the raw result, the
call to `DeclarationsFor`, and the three-way switch. It returns the possibly
narrowed list, the declaration where there was exactly one, and an error.

`internal/project` loses its `autoinstall.LookupFunc` import, its `lookup`
field, `NewResolver`'s second parameter and `Tools()`, all of which become dead
together once `command` leaves the signature.

`autoinstall.Runner` gains an `IsTerminal func() bool` field beside `Lookup`,
`Installer` and `Exec`, following that package's existing function-field
convention rather than `cmd/tsuku/config.go`'s package-level var, which is a
different seam in a different package. A nil field is treated as
not-a-terminal, so a caller that forgets to wire it gets the safe answer.

`cmd/tsuku` gains `ErrNotInteractive` and `AmbiguousDeclarationError` cases in
the switch, wires `IsTerminal`, and loses `hasProjectTools`.

**One hatch in the message it carries is broken today and must not survive the
move.** The message offers "set `TSUKU_AUTO_INSTALL_MODE=auto` or use
`--mode=auto`". `resolveMode` downgrades an environment-supplied `auto` to
`confirm` unless the persistent config corroborates it — the escalation
restriction working as designed — so a reader who follows the first hatch
verbatim reproduces the failure they were escaping. R10 requires every named
hatch to complete the command and AC27 requires the message to name exactly the
working set, so the message names `--mode=auto` alone, or names the environment
variable together with the corroboration it requires. Moving the message behind
`ErrNotInteractive` without fixing it would carry a known-wrong instruction
across a change that touches the line.

**The moved check's predicate is `effectiveMode == ModeConfirm &&
!r.IsTerminal()`, with no declaration term**, and that is not an oversight. R9
says the check must test whether the executed command is project-declared, and
an implementer reading it literally will write a declaredness term — which
reintroduces the exact defect the move exists to fix: a declared command
elevated to `auto` and then lowered to `confirm` by the configuration-permission
gate would skip the check and prompt at a closed stdin. By the time the check
runs, `effectiveMode` already encodes declaredness, because the elevation and
every gate that could undo it have already run. Testing the mode is testing the
declaration, one step later and correctly.

## The Gates Table, Re-derived

R21 obliges this document to record the derivation, and AC45, AC46, AC50 and
AC52 are all unfalsifiable without it. What follows is that record.

**The rule has to change, not just the derivation.** The PRD states it as
"every point in `Runner.Run` between the index lookup and the mode dispatch".
After the extraction in D3 the index lookup is not in `Runner.Run` at all, so
the rule applied literally no longer spans the sites that moved, and AC52's
two-way comparison would fail on correct code. The rule becomes:

> Every point in `Runner.candidates` or `Runner.Run` that reads or writes the
> effective mode, or returns before the mode dispatch is reached without having
> reached an install or an exec.

**The span's two boundaries, by identifier:** the `r.Lookup` call inside
`Runner.candidates`, and the mode dispatch switch in `Runner.Run`.

**The derived rows after this work**, each cited by file, function and role:

| Row | File | Function | Role |
|---|---|---|---|
| Project declaration | `internal/autoinstall/run.go` | `Run` | Raises the mode where it was `default` and the command is declared |
| Configuration-permission gate | `internal/autoinstall/run.go` | `Run` | Lowers out of auto on permissive config file |
| Verification gate | `internal/autoinstall/run.go` | `Run` | Lowers out of auto on a recipe without verification |
| Multiple-provider gate | `internal/autoinstall/run.go` | `Run` | Lowers out of auto when the candidate list holds more than one |
| Already-installed fast path | `internal/autoinstall/run.go` | `Run` | Returns before the dispatch, having reached an exec |

**Informational, and not derived:** the root guard in `Run`, which ends the run
without reaching an install or an exec; and the `ErrNoMatch` and
`AmbiguousDeclarationError` returns in `candidates`, which do the same. The rule
excludes all three by the same clause, which is why that clause is phrased
around reaching an install or an exec rather than around returning early.

**What changed from the pre-change table.** The fast path and the four
mode-affecting rows are unmoved. The multiple-provider gate's row is unchanged
in text but its behaviour narrows, because the list it counts is now narrowed
where a declaration applies — which is D3's point and not a table change. What
is new is the span: two functions rather than one.

**AC46 searches this rule over that span in both directions; AC50 requires the
check to read its expected site list out of this section rather than out of the
table above; AC52 compares the two.** The list is here, in one place, so those
three point at the same artefact.

## Implementation Approach

The ordering below is by dependency. An earlier draft put the fixture work last
and the resolver first; both were wrong, and the reasons are worth keeping
because they are not obvious.

1. **The fixture index and the R17 check.** First, not last: R17 forbids
   constructing a multi-provider case any other way, so every multi-provider
   test in steps 2 through 4 depends on it. Testable at this point: the three
   existing violations migrate and the check catches its own negative control.
2. **The resolver and `candidates()` together**, including the
   `AmbiguousDeclarationError` sentinel. These cannot land separately: the
   design has `internal/project` lose `NewResolver`'s second parameter and
   `Tools()` once `command` leaves the signature, but `run.go` and `cmd_run.go`
   call the old surface, and the three-way narrowing includes the branch that
   returns the sentinel. The alternative — add `DeclarationsFor` alongside
   `ProjectVersionFor` and delete the old one in a later step — is available if
   the combined change proves too large to review, and is the only sanctioned
   way to split it. Testable: R1 through R6 and the identity defects.
3. **The refusal's output and exit code.** Testable: R7, R10, AC13, AC14.
4. **The terminal check's move**, its sentinel, the `IsTerminal` field, and the
   `ErrNoMatch` message that stops that path being silent. Testable: R5, R9,
   R18a.
5. **Gate announcements and the elevation disclosure.** Testable: R11, R11a.
6. **The audit record**, which consumes the origin plumbing and the gate
   identifiers from step 5. Testable: R12, R12a.
7. **Documentation.** Splits: the hook documentation is Group C and waits on
   nothing, so it can land at any point; the consent-model documents wait on
   step 5, since the disclosure form is what they describe.

Steps 1 through 6 are Group A and do not wait on D1.

## Security Considerations

The elevation this design permits is bounded to commands the project declared,
and never reaches an undeclared one — which is the same principle that makes
tsukumogami/tsuku#2545 a defect rather than a separate concern.

Three properties the decision depends on, each checkable:

- **A declaration cannot reach an unregistered source on the run path.**
  Verified three ways above. It does *not* hold on the install path
  (tsukumogami/tsuku#2552), and the scoped claim is the only true one.
- **`suggest` is a floor with no exceptions**, including the shim path, where
  the invoking party is a Makefile and nobody is watching. The counter-argument
  accepted here is that a shim carries two deliberate opt-ins — the declaration
  and the shim installation — where the interactive case carries one.
- **Every elevation that results in an install is disclosed**, naming the
  authorizing file. Without this, bounded elevation is the status quo with
  better documentation.

**A prerequisite defect that no control here reaches, and that is more urgent
than anything this design adds.** A declared version string is unvalidated —
`parseConfigFile` checks the tool count and nothing else — and it reaches
`ToolBinDir`, which joins it into a directory name. `filepath.Join` cleans, so
`..` segments escape: a version of `../../../../../../../tmp/evil` yields
`/tmp/evil/bin`, which the fast path stats and execs. That happens before the
mode dispatch and before all four gates, so every control in this design misses
it, and R4 does not touch it because R4 governs which recipe the fast path
selects rather than whether the path stays inside the tools directory.

The fix belongs at `parseConfigFile`, where the untrusted input enters — not at
the resolver, which R20 requires to pass versions through verbatim.
`SplitOrgKey` already rejects `..` in configuration *keys* with a comment saying
so; the same discipline was never applied to values. It is tracked separately
and is a prerequisite of this work rather than part of it, and **R16a's
statement of what the fast path admits is corrected accordingly**: the boundary
a reader must be told is not "an already-installed sibling may run" but "a path
the configuration named may run".

**The install path's registration composes with run-path elevation.** The
run-path claim above proves a declaration cannot *register* a source. It does
not prove the run path cannot *reach* an unapproved one, and those differ: once
tsukumogami/tsuku#2552's non-interactive registration installs a distributed
tool, the index carries it under a bare name, and a bare declaration matches it
on the run path with no source component anywhere in sight. So (c) does not
create that exposure but it does inherit it, silently and under `auto`, on
every subsequent run. Either the run path checks that a resolved recipe's
source is registered, or R13a's disclosure names the recipe's source alongside
the authorizing file. The second is cheaper and is what the disclosure is for.

**What actually bounds the blast radius, stated positively.** Registry curation
alone: a declaration selects among recipes the user's own configured registries
already carry, and cannot introduce one. That is the whole of it on the run
path.

Two things that sound like they bound it and do not. Checksum verification is
not available to lean on — see below — so no part of this decision rests on
it. And "the declaration is reviewable" is a property of an artifact rather
than an act anyone performed; it is not cited here for the same reason
`suggest` is not cited as a working mitigation.

**The verification gate is not available as a control here** (D7), for a
reason reported through the project's security policy and tracked there. Any
reasoning that treats it as active — including this document's own R3a hazard
— is unsound, so none is used. That is recorded rather than quietly relied
upon, and it is the second of the two gates that could have demoted an
elevated install.

Two consequences for sequencing rather than for the decision. Bounded elevation
moves into `auto` exactly the population that never opted into it, and one of
the controls the R14 documentation would otherwise describe to them cannot be
offered — so that documentation stops listing it, which is what R14 requires of
any mitigation that does not hold. And when the control does become available
it will lower most declared installs back to `confirm` for any recipe without a
static checksum, returning the friction D1's product-value case was built on
removing. The value of the choice therefore depends on the disposition of a
defect this design defers, which is on the page rather than discovered later.

## Consequences

**Positive.** The identity defects are fixed at one site rather than three. A
redundant index open disappears. `cmd_run.go` becomes testable at the wiring
boundary for the first time. Three exit-code paths become truthful, and a
fourth defect — `tsuku run <installed-tool>` failing in CI because a guard
blocked a path that never prompts — is fixed on the way past.

**Negative.** `Run`'s logic spans two functions, so R21's derivation names
function boundaries. One exit code moves for a single-provider command, named
in R18a rather than discovered. Two of R19's eight fixture properties are not
deliverable without prerequisite work, and AC19 in particular cannot be written
until the verification gate does something.

**Mitigations.** The moved exit code and the changed no-match path both have
criteria. The unbuildable fixture properties are named here rather than left
for whoever writes the tests. And the derivation obligation is R21's, which
already required re-derivation because this work changes the span.
