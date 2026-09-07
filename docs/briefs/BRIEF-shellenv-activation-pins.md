---
schema: brief/v1
status: Done
problem: |
  Documented `.tsuku.toml` forms silently do not activate. The "latest",
  "" and prefix version pins put nothing on PATH, and so do org-scoped
  keys, because activation builds a directory name from the declared
  strings rather than resolving them. Nothing is reported either way.
outcome: |
  Every form the guide documents activates the tool it names, resolved
  against what is installed -- version pins and org-scoped keys alike.
  Any declaration that cannot be honored says which tool and why, on
  stderr, without breaking the `eval` contract or becoming prompt noise.
motivating_context: |
  Reported as tsukumogami/tsuku#2543 after a workspace `.tsuku.toml`
  pinning a major version was found to have never activated anything.
  Two of the broken forms are a defect against the design that
  specifies activation; the third is a deferral that design recorded
  and never completed.
---

# BRIEF: Documented `.tsuku.toml` forms that silently do not activate

## Status

Done

The brief frames the problem and the boundary. The requirements contract, the
reporting shape, and the choice of where the shared version-matching code lives
are downstream.

**Amended after acceptance, 2026-09-06.**

*What was accepted:* the problem framed as the non-exact version pins alone —
`"latest"`, `""` and prefix forms silently not activating.

*What changed:* org-scoped keys were added to the problem statement, the outcome
and the scope boundary.

*Why:* org-scoped keys fail the same way through the other half of the same
path. Activation iterates raw `[tools]` keys and never calls `SplitOrgKey`, so
`"tsukumogami/koto" = "1.0"` looks for `tools/tsukumogami/koto-1.0/bin` while
the installer wrote `tools/koto-1.0`. It is the same defect one derivation away
in the same loop, so widening the problem statement was preferred to a second
chain rewriting that loop. Ruled by the maintainer seat after the defect was
reproduced against the shipped binary and a first-party config was found
affected.

## Problem Statement

A developer writes `nodejs = "latest"` in `.tsuku.toml`, the form tsuku's own
guide uses in its worked example, and `cd`s into the project. Nothing goes on
PATH. Nothing is printed. The command exits 0, stderr is empty, and neither
`--verbose` nor `--debug` changes that. Node is installed the whole time.

The same happens for `nodejs = "26"` and for an empty version. Only an exact pin
works, so three of the four version forms tsuku documents are inert, and there's
no way to find that out from inside the tool. `tsuku install` reads the same file
and resolves the same forms correctly, so someone can install every tool their
project declares and still end up with an empty PATH.

The same happens to an org-scoped key. `"tsukumogami/koto" = "latest"` is the
documented form for a tool from a distributed registry, and activation looks for
`tools/tsukumogami/koto-latest/bin` — wrong in both halves at once, because it
never derives the bare name and never resolves the version. A real workspace
config on this machine declares two tools that way and neither has ever
activated; it goes unnoticed because the tools are installed by a setup script,
which is exactly the shape of config most likely to hit it.

So the failure is not specific to version strings. It is that activation builds
a directory name out of whatever the file said, instead of resolving what the
file said into what is installed — in the name half and the version half alike.
The version half has two mechanisms behind it and the name half one, and keeping
all three apart matters for what gets corrected.

`latest` and prefix forms miss by accident. Activation builds a directory path
by interpolating the declared string into
`$TSUKU_HOME/tools/<name>-<version>/bin`, then skips the entry when that
directory doesn't exist. An exact pin names a directory that exists. `latest`
names `tools/nodejs-latest/bin` and a prefix names `tools/nodejs-26/bin`, and
tsuku never creates either. The empty form is different: it never reaches the
interpolation at all. It's caught by an explicit branch above it, carrying a
comment that says so — `// No version pinned -- skip (would need resolution, out
of scope for the activation skeleton)`.

So the three broken version forms are two kinds of broken. One was knowingly
unimplemented, and whoever deferred it left a note saying the user would get
nothing. Two were believed to work and silently didn't: they fall through to the
stat-miss below that branch, where nobody intended anything. The deferral has a
paper trail. The accident has none — and worse, as the documentation shows.

The org-scoped key is a third kind and the plainest: the loop iterates the raw
`[tools]` map keys and never calls `SplitOrgKey`, which exists for precisely
this and is called from `internal/project`'s own resolver. Nothing was deferred
and nothing was contradicted; the derivation was simply never wired in on this
path. It lands in the same `Skipped` slice as the rest, and so is equally
invisible.

`docs/designs/current/DESIGN-shell-env-activation.md` specifies activation, and
it contradicts itself about exactly this. Line 196 states that a project
declaring `go = "1.22"` gets `$TSUKU_HOME/tools/go-1.22.5/bin` on PATH — the
newest installed match, which is the behavior being asked for here. Line 133
specifies the `{name}-{version}` interpolation, which can only ever produce
`tools/go-1.22/bin`. Line 212 then reasons about the same `go = "1.22"`
declaration as though the string named an exact release that might not be
installed, conflating a version boundary with a version.
`docs/guides/shell-integration.md` repeats the `go-1.22.5` promise to users. So
against `latest` and prefix pins the design is wrong, and the correct semantics
were written down twice and implemented neither time. Against the empty form it
is honest and incomplete.

The reporting half has the same shape. That design already recorded a mitigation
— print a warning to stderr when skipping, which costs nothing when there is
nothing to skip — and it was never built. `ActivationResult` carries a `Skipped`
field for exactly this purpose, populated correctly, and no non-test caller
reads it. The information exists, is computed, and reaches nobody.

## User Outcome

A developer writes any form the guide documents and gets the tool they asked
for. `latest` and `""` put the newest installed version on PATH. A prefix puts
the newest installed version inside that boundary on PATH, using the same
dot-boundary rule that stops `"1"` from matching `10.0.0`. An exact pin resolves
to the same version it resolves to today. And an org-scoped key activates the
tool it names, at the bare name the installer actually wrote it under.

Reporting, unlike resolution, changes for every form including the exact one.
Any declaration that cannot be honored is reported — `nodejs = "26.9.9"` on a
machine that does not have it is silently skipped today, which is the same
defect wearing different clothes. Failures the developer has to tell apart get
told apart: nothing installed matches this pin, this version string is not a
form tsuku accepts, and this is a channel pin activation does not resolve.
Someone who writes `>=26` out of npm or Cargo habit learns they wrote something
tsuku does not take, rather than receiving the same silence as a documented form
that was never implemented.

The message reaches stderr and only stderr, because both entry points are
consumed as `eval "$(...)"` and a line on stdout is a broken shell rather than a
warning. And it's reporting the developer keeps rather than mutes: a warning
that repeats on every prompt gets the hook deleted from `PROMPT_COMMAND`, and a
developer who does that has lost the fix entirely. Proportionality is part of
whether the reporting works at all, not a matter of taste.

Underneath, the developer stops needing to know that activation and installation
read their config differently. Both read the same file the same way.

## User Journeys

### Joining a project that pins to a channel

Someone joins a team and clones a repository they did not configure. Its
`.tsuku.toml` declares `jq = "latest"`, the form the guide puts in its own
example, and they `cd` into it. They have jq installed. The prompt hook fires,
activation resolves `latest` against the versions in `$TSUKU_HOME/tools`, and
the newest one is on PATH by the time the prompt returns. Nothing is printed,
because nothing went wrong. Today jq is absent from PATH and there's no output
at all.

An empty version, `jq = ""`, is the same story from the newcomer's side — the
guide says the two forms are equivalent, and after this change they behave that
way. It reaches the failure by a different route in the code, which matters for
what gets corrected but not for what this person sees.

### Writing a prefix pin from the guide

Someone maintaining their own project's pin file reads the version-strings
section of the shell-integration guide, which tells them a prefix resolves to
the newest release inside the boundary, and writes `go = "1.22"` for a project
that needs Go 1.22.x. Go 1.22.5 is installed. They don't use the prompt hook, so
they run `eval "$(tsuku shell)"` explicitly and `go version` reports 1.22.5 —
the path the design's own worked example promised. The contrast with the section
above is deliberate: the same declaration has to resolve identically whether it
arrives through the hook or through a command someone typed. Today
they get their global Go, with no indication the declaration was ignored.

### Hitting a pin nothing satisfies

Someone joins a project pinned to a version they've never installed, or removes
a version another project still declares, and `cd`s into the directory.
Activation can't honor that entry. It says so on stderr, naming the tool, while
still activating every other tool in the file that it can. Now they know to run
`tsuku install` instead of spending an afternoon on why their PATH looks wrong.
Today the good half of the file activates and the bad half is dropped in
silence.

### Writing a constraint from another ecosystem

Someone used to npm or Cargo writes `nodejs = ">=26"`. That isn't one of the
four forms tsuku accepts, and no amount of installing will make it work. They're
told the version string itself is invalid, in a sentence distinct from the one
about nothing being installed. Today it takes the same silent path as `latest`,
which makes a mistake in the file indistinguishable from a documented form that
was never implemented.

## Scope Boundary

**In scope**

- Resolving all four documented version forms at activation time — exact,
  prefix, `""` and `latest` — against the set of versions actually installed,
  and deriving the bare tool name from an org-scoped key so it resolves to the
  directory the installer wrote rather than to one built from the raw key.
  The guarantee is that the four forms resolve consistently with each other and
  stay consistent as the code changes; how many code paths deliver that is the
  design's call, not this brief's.
- Selecting the newest matching installed version, with an ordering that is
  correct for real version strings rather than lexicographic.
- Reporting any declaration that cannot be honored, on stderr, for both
  `tsuku shell` and `tsuku hook-env`. This covers the exact form too, and it
  keeps three failures distinguishable from one another: nothing installed
  matches this pin, this version string is not a form tsuku accepts, and this is
  a channel pin that activation does not resolve. The invalid-form case is part
  of this contract rather than scope added to it — leaving it out would put a
  hole exactly where someone cannot tell a typo from an unimplemented feature.
- Reusing tsuku's existing pin-matching and version-ordering rules rather than
  writing a second matcher inside `internal/shellenv`. Whatever structural
  change that reuse requires is in scope; choosing among the available
  structures is a design decision, not a framing one.
- Correcting `docs/designs/current/DESIGN-shell-env-activation.md` and
  `docs/guides/shell-integration.md` so the specified algorithm, the worked
  example, and the implementation agree, and so the design states what
  activation does when a declaration cannot be honored. This follows from the
  behavior change rather than standing on its own: once the code resolves
  prefixes against installed versions and reports what it skipped, a design
  saying otherwise is wrong.

**Out of scope**

- Installing anything. Activation resolves against what is present and does not
  fetch, which the design records as a security property. Install-on-demand is
  separate work.
- Resolving a declaration against the recipe registry. Registry lookups are
  cache-backed and activation runs on every prompt, so a cold or partial cache
  would let tsuku reject a `.tsuku.toml` that is perfectly valid. That trades a
  silent skip for a loud wrong answer.
- Validating that a declared tool *name* exists. Detecting a typo like `ripgep`
  requires the registry, and is out for the same reason.
- Resolving channel pins such as `@lts` at activation. They stay unresolvable
  here; only the reporting changes, per the in-scope bullet above.
- The two `tsuku install` reporting defects found alongside this one — the
  unpinned-version warning firing before resolution, so the same output advises
  pinning a recipe and then reports it as unfindable, and "recipe not found in
  `<registry>`" naming an arbitrary one of the three configured registries, and
  a different one on each run of the same command, when the name is unknown in
  all of them. Both are pre-existing and
  visible against the same `.tsuku.toml` this work is tested with, so someone
  who trips over them here has not found a regression. Neither touches
  `internal/shellenv`, and both are tracked as tsukumogami/tsuku#2546.

## Open Questions

- Do `tsuku shell` and `tsuku hook-env` report identically? The difference
  between them is the trigger rather than the message: `tsuku shell` is one-shot
  and asked for, while `hook-env` fires unbidden on every prompt, so a warning
  that helps once can be noise sixty times an hour. If they differ, that is a
  decision to state rather than an accident to inherit. The PRD closes this.
- What does activation do when it cannot read the installed set at all — for
  instance while another tsuku process holds the state lock? The answer has the
  same two horns as the reporting contract, arriving from a different direction,
  and activation must not be able to stall a shell prompt. The PRD closes this.

## References

- `docs/designs/current/DESIGN-shell-env-activation.md` — the design that
  specifies activation, contradicts itself about prefix pins, and records the
  unbuilt stderr-warning mitigation.
- `docs/guides/shell-integration.md` — the user-facing statement of the four
  version forms and of prefix matching.
