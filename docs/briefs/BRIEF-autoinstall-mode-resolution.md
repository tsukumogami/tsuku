---
schema: brief/v1
status: Accepted
problem: |
  A project cannot reliably state which tool it wants. Depending on the
  command, `tsuku run` may install a different recipe than the one declared,
  at a version borrowed from a neighbouring declaration, prompt where the
  documentation promises it will not, or override the consent mode a user set
  to protect themselves in an unfamiliar repository.
outcome: |
  A developer in a project that declares a tool gets that tool, at that
  version, without needing to know whether some other recipe also provides the
  command, and is told when tsuku will not proceed rather than left to read an
  exit code. Declaring one tool is a statement about that tool alone. A
  developer who told tsuku never to install unattended knows exactly what that
  setting guarantees inside a repository they cloned.
motivating_context: |
  Two issues filed against the same function pull in opposite directions on
  what looks like one dial: tsukumogami/tsuku#2542 wants a project declaration
  to survive a gate that demotes it, and tsukumogami/tsuku#2544 asks whether an
  explicit `suggest` should be able to override a declaration at all. Treated
  as two patches they contradict. They are two questions the code collapses
  into one: which recipe a project declared, and how far a project config may
  move the consent mode.
---

# BRIEF: Autoinstall Mode Resolution

## Status

Accepted

Requirements, including what the project-declaration lookup must return, belong
to the downstream PRD. The trade between project convenience and an explicit
trust setting is deliberately left open here and is routed to a recorded
decision at the design altitude.

One assumption is carried rather than deferred: the terminal guard's
per-command fix is expected to follow from the same answer the rest of this
work needs, without a decision of its own. If it turns out to need one, it
leaves this brief's scope and is tracked separately.

## Problem Statement

`tsuku run rg` in a project that pins `ripgrep = "14.1.0"` installs ripgrep
14.1.0 and runs it, silently. That is the contract `tsuku run --help` states in
its own words, and it holds — right up to the moment the command is one that
more than one recipe provides.

`tsuku run` asks the project resolver a question it cannot answer. The resolver
is handed a command name, looks that command up in the binary index, walks the
resulting recipes until it finds one the project declared, and returns that
recipe's version. It does not return the recipe. By the time the answer reaches
the code that has to act on it, the identity of the declared recipe no longer
exists anywhere in the program.

Most of what goes wrong afterwards is that missing fact, surfacing in a
different place each time.

It surfaces as the wrong tool. `tsuku run` falls back to the first recipe the
index happened to list, so a project that declares `yarn` is offered `corepack`,
one that declares `helix` is offered `evil-helix`, and one that declares `cdk`
is offered `aws-cdk`. Counted against the recipe tree, 27 commands that a person
would actually type are claimed by more than one recipe: `yarn`, `yarnpkg`,
`pnpm`, `kubectl`, `kustomize`, `openssl`, `migrate`, `hx`, `cdk`, `delta`,
`fd`, `air`, `atlas`, `goose`, `argocd`, `gitleaks`, `jsonnet` and the rest.
Those are not obscure aliases. They are the kind of thing a project pins. (The
count excludes C header files installed by library recipes, which the index
also treats as commands. Counting those gives 77, and `version.h` alone accounts
for eleven of them.)

The exposure grows rather than shrinks as the registry does. `java`, `javac` and
`jar` are each claimed by five recipes, because the OpenJDK family deliberately
ships one per distribution — the same ambiguity the install path built a picker
for. Any curated registry that grows sideways, adding a second implementation of
a tool it already carries, adds commands to this set by construction.

Two cautions about all of these numbers. They come from the recipe tree, and
`tsuku run` consults the published registry, which lags it — the two agree that
the problem exists and disagree about which commands are affected. And the
membership moves whenever the registry does. Nothing downstream should treat a
list of affected commands as settled; the property is what matters, which is
that a command is claimed by more than one recipe.

It surfaces as a version welded to the wrong tool. A project that declares
`fdclone` and never mentions `fd` still gets `Install fd@0.0.0-nope?` when
someone runs `fd`: the pin from one recipe's declaration, paired with a
different recipe's name. Nothing about that combination was ever declared by
anyone.

It surfaces as the prompt the documentation promises will not appear. A gate
exists to stop an ambiguous command silently installing an arbitrary recipe,
and it fires on any command with several providers. It cannot exempt a
declared tool, because it has no way to know one was declared. It prints
nothing when it fires, so the user sees a prompt the help text says they will
not see and gets no hint why. Non-interactively the same path exits 13,
"user declined", with no user present.

And it surfaces in commands the project never declared at all. The check that
decides whether to skip the terminal guard asks whether the project declares
*anything*, because asking whether it declares *this command* was not a
question the code could answer. So a project that declares `jq` changes what
happens when someone runs `hyperfine`: instead of the guard's clear message
about needing a terminal, they get a prompt written into a closed stdin and
exit 13. Declaring one tool alters the behaviour of an unrelated one.

Those four share a shape worth stating plainly, because it is what makes them
one piece of work rather than four patches. A question that cannot be answered
gets answered by proxy. The first index match stands in for the declared
recipe, and the existence of a config file stands in for a declaration of this
particular command. Each proxy is wrong in a different direction, and all four
trace back to the same unanswered question, so a fix has to reach that question
rather than the places its absence shows.

There is a fifth failure with a different cause, and keeping the two apart
matters, because settling the first would not touch the second. A user who sets
`auto_install_mode = "suggest"` because they are about to work in an unfamiliar
repository finds that a `.tsuku.toml` shipped by that repository overrides it.
That is not the resolver losing information. It is `tsuku run` treating a
project declaration as unconditional authority to raise the consent mode, with
no floor beneath it, so the setting a user chose specifically to protect
themselves from a cloned repository is the one setting that repository can
switch off.
`DESIGN-project-aware-exec.md` names exactly this setting as the mitigation for
exactly this threat, twice. No code implements it. Whether it should is a real
question with two defensible answers; documenting a protection that does not
exist is not one of them.

The two causes sit next to each other in the same function and are settled
together for that reason. "Is this command declared, and as which recipe" and
"how far may a project config move the consent mode" are separate questions
that the code currently collapses into one boolean, which is why answering
either one alone leaves the other wrong.

## User Outcome

A developer working in a project that declares its tools gets those tools. Not
a recipe that sorts earlier in an index, not a version borrowed from a
neighbouring declaration — the recipe the project named, at the version it
pinned, whether one recipe provides that command or five. They do not have to
know which commands in the registry are ambiguous, and they do not have to
audit their own `.tsuku.toml` to find out which of their pins will misbehave.

A developer who has told tsuku never to install anything unattended knows what
that instruction is worth inside a repository they cloned. Either their setting
holds against the project's config file, or it doesn't and the document they
consulted says so plainly. What they don't get is the current arrangement,
where a design doc promises them a protection and the first thing they notice
is the tool they didn't agree to install.

A developer whose pipeline depends on the documented behaviour gets the
documented behaviour. Where tsuku will not proceed, they are told which
condition stopped it rather than left to guess from an exit code. Nothing
demotes a user's mode silently.

And a developer working in a project that declares one tool sees no change in
how any other command behaves. Declaring `jq` is a statement about `jq`.

## User Journeys

### Onboarding onto an unfamiliar repository that pins its tools

A developer joining a team clones a service repo whose `.tsuku.toml` declares
`kubectl = "1.31.0"`, and types `kubectl get pods` at an interactive terminal.
`kubectl` is a command two recipes provide, `kubectl` and `kubernetes-cli`.
Today the project's declaration is discarded
and they are asked to confirm an install of whichever recipe the index listed
first, at the version the project pinned for a different one — and having never
seen the repo before, they've no way to know the offer is wrong. What they
should get is the declared recipe at the declared version, installed and
executed without a prompt, exactly as they would for a command only one recipe
provides.

### Working in an unfamiliar repository with a deliberate trust setting

A security-conscious developer has set `auto_install_mode = "suggest"` in their
own config because they regularly read code from repositories they do not
trust. They clone one, run a command it declares, and today the tool is
downloaded, installed and executed with no prompt, no terminal, and an audit
log entry recording it as an auto-mode install. Whichever way the underlying
question is settled, what this developer needs is the same: the mode they chose
is never changed out from under them without their knowing, and the document
they consulted tells them accurately what that setting does and does not
protect them from.

### Watching a pipeline go red on the same command that works locally

A developer opens a pull request and the build fails on a runner with no
terminal, running a command their project declares. The same command worked
on their laptop that morning, because there a prompt appeared and they answered
it without thinking about it. On the runner the demoted mode reaches the same
prompt, finds nothing to read, and exits 13: a code that says a human declined.
They read the log looking for who declined, and there wasn't anybody. It should
have installed and run, and where it will not, the reason should be in the
output rather than encoded in an exit status describing something that never
happened.

### Running a tool the project never claimed to manage

A developer runs a benchmarking script from inside a project that declares `jq`
and nothing else. The script calls `hyperfine`, which isn't in the project's
config and never was. They should get the same behaviour they would get in any
directory with no `.tsuku.toml` at all — under confirm mode with no terminal,
the guard's message naming both escape hatches. Today the mere presence of a
declaration for an unrelated tool routes them past that guard and into a prompt
nothing can answer, so a config file they didn't write changes what happens to
a command it never mentions.

## Scope Boundary

### In

- **The declaration lookup.** The path that decides, for one command, whether a
  project declared it and as which recipe, including whatever that path must
  return in order to carry the answer.
- **Recipe selection.** Which recipe `tsuku run` installs and executes when a
  project declares one and several provide the command.
- **Ambiguity the config did not resolve.** What happens when a project
  declares two recipes that both provide the same command, so the config
  expresses no preference between them.
- **The floor question.** Whether an explicitly set consent mode is a floor a
  project config cannot raise, decided on the record rather than inherited from
  either issue.
- **Visible demotions.** Whether a gate that changes the effective mode says
  so, and in what voice.
- **The terminal guard's test** for whether the current command is
  project-declared, tracked as tsukumogami/tsuku#2545.
- **Documentation agreement.** Bringing `DESIGN-project-aware-exec.md`,
  `docs/guides/shell-integration.md` and `tsuku run --help` into line with
  whatever is decided, including saying what replaces the documented mitigation
  if that mitigation is withdrawn.

### Out

- **Activation dropping `"latest"` and prefix pins** (tsukumogami/tsuku#2543).
  A neighbouring defect in the same feature area, living in `internal/shellenv`
  rather than on this path, and owned separately. In scope only if the mode
  model turns out to depend on it, which is not expected.
- **How `tsuku install` resolves an ambiguous alias.** The multi-satisfier
  picker is settled work on a different index. This brief covers the `run`
  path, and where the two answers differ that difference has to be argued
  rather than assumed — but the picker itself is not reopened here.
- **The registry manifest's refresh cadence.** The binary index this path reads
  has not refreshed since March. That is a real question about a different
  system and is deliberately not pulled in.
- **The consent model for the command-not-found hook and for shims as
  mechanisms.** Both call `tsuku run` and inherit whatever it decides. Their
  own installation and opt-in stories are untouched.
- **Widening or narrowing which recipes may be installed at all.** The curated
  registry, checksum verification and the root guard are the surrounding
  controls and stay exactly as they are.

## Open Questions

- Does an explicitly set `suggest` outrank a project declaration, or does the
  design document withdraw the claim that it does? Both are legitimate
  outcomes, and the choice is made on the record at the design altitude because
  it is a trust boundary and expensive to revisit. The two inputs that should
  settle it are already in the References below and pull opposite ways: the
  threat model in `DESIGN-project-aware-exec.md`, which argues a checked-in
  declaration is deliberate consent, and the environment-variable rule in
  `DESIGN-auto-install.md`, which already refuses to let a file shipped by a
  cloned repository raise the consent mode without corroboration.
- When a project declares two recipes that both provide one command, is
  refusing with both names the right answer in an interactive session as well
  as a non-interactive one? The `install` path answers the interactive form of
  this question by asking the user. Whether a config file that was supposed to
  have settled the matter deserves the same treatment is the downstream
  question.

## References

- `docs/designs/current/DESIGN-project-aware-exec.md` — the design that made a
  project config consent, and the document that names a mitigation no code
  implements.
- `docs/designs/current/DESIGN-auto-install.md` — the consent modes, their
  resolution order, and the existing rule that an environment variable may
  lower the mode freely but may not raise it without corroboration. That rule
  is implemented and observable, not just specified: with no `auto_install_mode`
  set, `TSUKU_AUTO_INSTALL_MODE=auto` is refused and nothing installs. So tsuku
  already applies an escalation restriction to one repository-supplied file and
  not to the other, which is the fact the open question above has to account
  for either way.
- `docs/prds/PRD-multi-satisfier-picker.md` — how the install path answers
  "several recipes, which one", on a different index.
- `docs/guides/shell-integration.md` — the user-facing statement of the consent
  model.
