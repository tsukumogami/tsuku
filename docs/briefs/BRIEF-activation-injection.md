---
schema: brief/v1
status: Draft
problem: |
  Values a cloned repository controls reach two kinds of sink unguarded: a
  filesystem path component, and text a login shell evaluates. The guards
  that would stop them already exist, wired to other consumers, and the
  design record cites them as covering sinks they never run on.
outcome: |
  A repository the user has not read cannot change what runs on their
  machine by declaring a tool. Malformed declarations are refused at the
  point the file is read, with the offending key named, and the documents
  describing these controls describe what the code actually does.
motivating_context: |
  Filed as tsukumogami/tsuku#2553 after a review reproduced two defects in
  activation. A bounded exploration of the surrounding class found the two
  reported instances were part of a wider pattern and that one component of
  the reported composition had been left out of the report.
---

# BRIEF: Guarding externally-supplied values at the config boundary

## Status

Draft

Framed under `/scope` from tsukumogami/tsuku#2553 and this chain's own
exploration. The downstream PRD owns the requirements and the acceptance
criteria; this brief owns the problem, the outcome, and where the feature
ends.

One finding of that exploration is not yet filed anywhere: the declared
version escapes the same composition as the name, at a sink that executes
rather than only shaping `PATH`. Whether it is recorded on the existing issue
or takes its own is a filing decision outstanding at the time of writing. The
fix does not wait on it.

## Problem Statement

A `.tsuku.toml` is a file that comes with a repository. Cloning a repository
and working in it is not an act of trust in that file's contents, but tsuku
treats it as one: every value it declares flows into path construction and
into shell text without being checked at any point.

Two consequences have been reproduced. A declared tool name becomes a
directory component through `filepath.Join`, which cleans, so `..` in a name
escapes `$TSUKU_HOME/tools` and puts an attacker-chosen directory at the front
of `PATH`. And the shell-export formatter quotes with Go's `%q`, which
produces a Go string literal rather than shell-quoted text — it escapes `"`
and `\` but not `$` or the backtick — so command substitution in any emitted
value executes when the hook output is evaluated.

The **declared version** reaches the same composition as the name and is
equally unchecked, which is the part the original report missed. A perfectly
ordinary tool name with a traversal in its version puts a directory outside
the tools tree onto `PATH`, and reaches a `tsuku run` fast path that stats the
constructed path and replaces the process before any consent mode is
evaluated or any audit record is written. That route needs no shell
integration at all.

The deeper problem is not that these checks are missing. It is **where the
checks that exist are wired**. This codebase already contains a recipe-name
validator whose own documentation calls it "the single source of truth" for
well-formed names; a strict character pattern applied to dependency
references, justified in its comment on the grounds that those references get
"interpolated into shell-visible paths"; a version validator; a containment
assertion that correctly compares a composed path against its root; and a
correct POSIX shell quoter. Each of them is called by one or two consumers and
not by the paths that need them. The guards are good. They sit one consumer
deep from the point where untrusted data enters, so every other consumer
walks past them.

That placement is also what made the gap invisible. Three current design
documents — the activation record, the org-scoped project config record, and
the notification-routing record — each assert that a path-construction
control covers a sink, and in each case the control is real and lives
somewhere else. A reviewer follows the citation, finds the function, confirms
it rejects `..`, and stops. The activation record goes further and rates the
reported defect Low under a mitigation that was never built, while the
residual-risk cell one column to the right describes the actual behaviour.
Correcting that record is part of the work rather than tidying after it: it is
what stopped anyone looking.

## User Outcome

Someone clones a repository they have not read, works in it, and installs or
runs the tools it declares. Nothing that repository wrote decides what
executes on their machine. If the repository declares something malformed —
whether hostile or a typo — tsuku says so and names the key it objected to,
rather than dropping the declaration silently and carrying on. How much of the
file a single bad declaration takes down with it is a separate question, still
open; see Open Questions.

Someone maintaining tsuku who wants to know whether a control exists reads the
design record and gets a true answer. Someone adding a new value to the shell
output finds one correct answer already in the codebase instead of inventing
another.

## User Journeys

### Cloning and activating in an untrusted repository

A developer with tsuku's activation hook installed clones a repository to try
it out and `cd`s in. The repository's `.tsuku.toml` declares a tool whose name
or version escapes the tools directory, or whose name carries a command
substitution. Today the escape lands on `PATH` silently, or the substitution
executes at the next prompt. Afterwards the declaration is refused when the
config is read, nothing it named reaches `PATH`, and the developer is told
which declaration was rejected.

### Running a project tool with no shell integration

A developer who has never installed a hook and never run `tsuku shell` clones
a repository and runs one of its tools. The already-installed fast path builds
a binary path from the declared version and execs it. This journey matters
because it is the widest one — no opt-in of any kind stands between the clone
and the exec — and because it bypasses the consent and audit machinery
entirely. Afterwards the declaration never survives config load, so the path
is never built.

### Writing a config by hand and getting it wrong

Someone adds a tool to their own `.tsuku.toml` and mistypes the name. The same
check that refuses the hostile declaration refuses this one. The journey's
outcome is not "rejected" but "told what to fix": the error names the
offending key and says what shape a name has to be. This journey is why the
rejection is a diagnostic rather than a silent skip, and it is served by the
same mechanism as the security case rather than by a separate one.

### Deciding whether a control exists

A contributor reviewing a change, or a maintainer triaging a report, reads the
activation design record (`DESIGN-shell-env-activation.md`) to find out
whether tool names are validated. Today they are told the paths are
constrained and the names validated, and both halves are false. Afterwards the
document states what the code does, so the next person to ask this question
gets an answer they can act on.

## Scope Boundary

### In

- Both externally-supplied components of a declared tool — the name and the
  version — wherever they become part of a filesystem path.
- Every value tsuku emits into text a shell evaluates, in both the POSIX and
  fish dialects. That includes the `shellenv` command
  (`cmd/tsuku/shellenv.go`), which interpolates into hand-written double
  quotes and escapes nothing, under a command its own help documents as
  something to evaluate.
- The recipe-name path through the registry, where the same declaration
  reaches a fetch URL and a cache write.
- Defence at the sinks as well as at the boundary, so a future caller that
  bypasses config load is still refused.
- The claims the design record makes about all of the above.

### Out

Each exclusion below is real work that a reader could reasonably assume is
here, and each has a named owner.

- **`.tsuku.toml` discovery walking to `/`**, so a config in a world-writable
  directory applies to every user beneath it. Filed as #2555. It is a
  discovery-scope defect rather than an input-validation one, and it changes
  semantics this feature does not touch.
- **The install path's destructive sinks** — an unvalidated name reaching
  `os.RemoveAll` and `os.Rename`, and the structural hardening that would
  close them at the `Config` path helpers. Its sharp entry point is a local
  file the user chose with `--recipe`, not a cloned repository, and the
  drive-by half is closed by this feature's boundary anyway. Its own issue.
- **Org-scoped declarations never activating.** A supported syntax that
  silently resolves to the wrong directory. Functional rather than security,
  and it belongs to the chain already rewriting that loop.
- **`tsuku shellenv` emitting POSIX syntax for every shell**, which makes a
  documented fish path unusable. Filed as #2556.
- **The install consent prompt showing only the bare tool name** and hiding
  the registry source a declaration chose. Its remedy is to display the
  source, not to escape a string, so it does not share this feature's fix.
- **Non-exact version pin resolution** and the reporting contract for skipped
  tools. A separate chain owns both.
- Any general audit of tsuku. The boundary of this feature is values that
  originate outside the user's control and end in a filesystem path, in text a
  shell evaluates, or in a URL that decides where a recipe is fetched from.
  Process execution was examined and ruled out.

## Open Questions

- **Whether a rejected declaration invalidates the whole file or only that
  entry.** The loud-diagnostic requirement is satisfied either way, so this is
  a separate choice from the one the User Outcome argues for. The security
  requirement is only that the offending value never reaches a sink; it says
  nothing about whether the declaration's siblings survive.

  Two different failures are easy to discuss as one, and the PRD should not.
  A **TOML parse failure** means the file cannot be read at all — whole-file
  refusal is the only coherent answer, because nothing is known about what it
  says. A **value validation failure** means the file parsed, every key is
  known, and exactly one is bad. The argument that a config which failed
  validation cannot be partially trusted borrows its force from the parse
  case, where the untrusted region genuinely is the whole file. It does not
  transfer.

  On the second: refusing only the entry keeps a shared repository working for
  everyone when one declaration is malformed, and denies an attacker a cheap
  way to disable activation for a whole team by planting one bad entry. The
  consumer evidence gathered so far points the same way — a `.tsuku.toml`
  fanned out to many working roots turns one bad key into no tools anywhere,
  and a non-interactive installer that is deliberately non-fatal reports that
  as a single warning line inside an otherwise-successful run.

  **This chain's lean is per-entry, and the PRD confirms or overturns it.**
  Said plainly rather than dressed as a balanced weighing, because it is not
  balanced: no case favouring whole-file has survived examination yet. The one
  worth hunting before the PRD settles this is a pinned toolchain where
  getting some tools and not others is worse than getting none.

- Whether this brief's framing is carried entirely by the downstream PRD, in
  which case `/scope`'s consolidation judgment removes it. That question is
  answered against both documents once they exist, not here.
