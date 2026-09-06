---
schema: design/v1
status: Proposed
problem: |
  Values a cloned repository declares reach four sinks unchecked: a path
  component, shell-evaluated text, an exec target, and a registry fetch URL.
  Six correct controls already exist in the tree, each wired to one or two
  consumers and to none of these paths. The technical problem is not writing
  checks but choosing a placement that no future consumer can bypass, and a
  quoter shape that the next emitter cannot ignore.
decision: |
  Validate at `parseConfigFile`, the single point where `.tsuku.toml` becomes
  a `ProjectConfig`, using one strict name predicate extracted into
  `internal/recipe` and shared with the existing dependency-reference
  consumer, plus the existing pin rule extracted into a leaf `internal/pinsyntax`
  because `internal/project` cannot import `internal/install` without a cycle.
  Quote with a dependency-free leaf `internal/shellquote` exposing separate
  POSIX and Fish functions, called by both emitters. Carry per-declaration
  refusals as diagnostics on `ConfigResult`, printed to stderr by each
  consumer. Add sink-level rejection at the registry name path as a backstop
  beneath the boundary, not as the remedy.
rationale: |
  Sink-level validation is what produced the current state: six guards, each
  covering one consumer. A boundary covers every consumer including ones not
  yet written, and `parseConfigFile` is the only place every consumer passes
  through. A leaf quoter is chosen over one local to activation because four
  packages already hold four different answers to shell quoting, and the
  correct one being unexported in a fifth is why the wrong one got written.
upstream: docs/prds/PRD-activation-injection.md
user_visible_surface: true
---

# DESIGN: Guarding externally-supplied values at the config boundary

## Status

Proposed

## Context and Problem Statement

`internal/project/config.go` parses `.tsuku.toml` into a `ProjectConfig`.
`parseConfigFile` currently enforces one thing — `len(cfg.Tools) > MaxTools` —
and every other value passes through untouched. From there the map's keys and
version strings reach:

- `Config.ToolDir(name, version)`, which is
  `filepath.Join(ToolsDir, name+"-"+version)`. `Join` calls `Clean`, so `..` in
  either component escapes. `ToolBinDir` appends `bin` and the result is
  prepended to `PATH` (`internal/shellenv/activate.go:90`).
- `FormatExports` (`internal/shellenv/activate.go:123`), which emits with `%q`
  — a Go string-literal quoter that escapes `"` and `\` and not `$` or the
  backtick.
- `Runner.Run`'s already-installed fast path
  (`internal/autoinstall/run.go:100-108`), which stats the composed path and
  `syscall.Exec`s it, before the mode dispatch and all four security gates.
- `RegistryProvider.recipePath`, which becomes both an HTTP path and a
  disk-cache write key.

The technical problem is placement, not detection. The tree already contains
`recipe.IsValidRecipeName`, the strict pattern beside
`runtimeDepNamePattern`, `install.ValidateRequested`,
`install.ValidateVersionString`, `ValidateSymlinkTarget`, and a correct POSIX
`shellQuote` in `internal/actions/set_env.go:252`. Each is called by one or two
consumers. A design that adds a seventh guard at a seventh site reproduces the
defect it is fixing.

Two properties make this harder than "call the existing checks":
`IsValidRecipeName` is a blocklist and accepts `x$(id)y`, one of the two
reproduced attack values; and the correct quoter is unexported in a package
that sits *above* the one that needs it in the import graph.

## Decision Drivers

- **D1. No consumer may bypass the check, including one not yet written.**
  Six guards each covering one consumer is the defect. Cited by R13.
- **D2. One definition of a valid name.** A second near-identical rule is the
  mechanism by which `IsValidRecipeName`'s "single source of truth" comment
  became false. Cited by R9.
- **D3. The quoter must be harder to bypass than to use.** A sibling chain
  adds an emitted variable one PR after this lands; if reaching for `%q` is
  as easy as reaching for the quoter, the defect returns. Cited by R6, R7.
- **D4. Rejection must reject, never repair.** A sibling chain requires the
  declared version be reported verbatim, so no normalisation on the read path.
  Cited by R3.
- **D5. Nothing valid today may break.** 1449 registry names, and the
  documented pin forms. Cited by R12.
- **D6. Two dialects, not one.** POSIX single quotes are fully literal; fish's
  recognise `\'` and `\\`. One function covering both is wrong for one of
  them, and looks right. Cited by R6.
- **D7. The fix must be cherry-pickable against `main`.** A sibling PR moves
  `activate.go` to a new package immediately after this lands.

## Considered Options

### Option A — validate at each sink

Add a check before each of `ToolDir`, `ToolBinDir`, `recipePath`, the exec
fast path, and the emitters.

Rejected. This is precisely the current architecture, and the current
architecture is the bug: `internal/updates/gc.go:83-93` shows an author
pausing before an `os.RemoveAll` to reason for eleven lines about which
version validator to use, then joining an unvalidated name into the same
expression. The sinks are not a closed set, so D1 fails by construction —
`Runner.Run`'s fast path was added after the others and inherited nothing.

Not a strawman: it has a real advantage this design gives up. A sink-level
check protects against a caller that constructs a `ProjectConfig` directly in
Go, bypassing the parser. That is why the registry backstop survives as R10
rather than the whole option being discarded.

### Option B — validate inside the `Config` path helpers

Give `ToolDir`, `ToolBinDir`, `LibDir`, `AppDir` and `CurrentSymlink` error
returns and validate there.

Rejected for *this* change, and it is the strongest rejected option. It
satisfies D1 for path sinks and is where a chokepoint belongs if one had to be
chosen for paths alone. Three reasons it loses here. It does not cover
`FormatExports`, whose `Dir` value is not a tool name and never passes a path
helper — so it cannot be the whole fix. Changing five widely-called signatures
is a large mechanical diff that would bury a security change and delay it,
against D7. And it validates late: the value has already been accepted into a
config object and carried across package boundaries, so the diagnostic arrives
far from the file that caused it, hurting R4's requirement that the error name
the offending key.

It remains the right shape for the deferred install-path hardening, which is
filed separately, and this design does not foreclose it.

### Option C — validate at `parseConfigFile` (chosen)

One check where the file becomes a config object.

### Option D — quoter local to `internal/shellenv`

Put the shell quoter next to `FormatExports`.

Rejected on D3, and it has a real advantage this design gives up: no new
package, and the quoter sits beside its principal caller where a reader of
`FormatExports` finds it without knowing it exists. Against a codebase with
fewer emitters that would win.

It also compiles — `internal/shellenv` sits below `internal/install`
and `internal/actions`, so there is no cycle. But `cmd/tsuku/shellenv.go` also
emits evaluated shell text and would have to import a package named for
per-directory PATH activation to get a string function, which reads as a
layering mistake and invites a copy instead. Four packages already hold four
different answers to this question; a fifth is the predictable outcome of
leaving the correct one somewhere awkward to reach.

## Decision Outcome

**Validate at the boundary; quote from a leaf.**

`parseConfigFile` gains validation of each declared key and version before it
returns a `ProjectConfig`. Every consumer — activation, `tsuku install`,
`tsuku shim install`, background auto-apply, and the `tsuku run` fast path —
reaches its values through that function, so all five inherit the **security
guarantee** without changing: a refused declaration is simply absent from the
map they read (D1, R13).

The **diagnostic** is a different matter and all five do change, because a
per-declaration refusal cannot travel on the error return that aborts the whole
load. See "How a refusal reaches the user" below. An earlier draft of this
section claimed no consumer changes at all, which was true of the security
property and false of the requirement that a refusal be visible.

The name rule is extracted as one exported predicate in `internal/recipe`,
layering the strict character rule over `IsValidRecipeName`, and
`validateRuntimeDependencyNames` is refactored to call it (D2, R9).

That refactor is **not** behaviour-preserving, and an earlier draft of this
design claimed it was. `IsValidRecipeName` rejects `..` by substring
(`internal/recipe/name.go:31`), and two fixtures pin the consequence —
`internal/recipe/name_test.go:24` and
`internal/recipe/validator_runtime_deps_names_test.go:59` both assert
`foo..bar` is rejected. R2 requires it accepted, because `..` is a path-segment
rule rather than a substring one. So adopting R2's semantics changes those two
fixtures, and the change is deliberate: a name with an internal doubled dot is
not traversal, and rejecting it was over-broad. Every *other* existing test
must pass unchanged, which is what still makes the extraction checkable.

Shell quoting moves to `internal/shellquote`, a leaf importing only the
standard library, exporting `POSIX` and `Fish`. `FormatExports` and
`cmd/tsuku/shellenv.go` both call it (D3, D6, R6, R7).

`recipePath` and `Registry.cachePath` call `IsValidRecipeName` as a backstop
beneath the boundary, not as the remedy (R10).

## Solution Architecture

### The validation boundary

```
.tsuku.toml
    |
    v
parseConfigFile (internal/project/config.go)
    |  for each key:
    |    SplitOrgKey(key) ---- err ----> refuse, naming the key
    |         |
    |         +-- source half --> source rule (wider than the name rule)
    |         +-- bare name ----> recipe.IsStrictRecipeName
    |    for each value:
    |         version ---------> pinsyntax.ValidateRequested
    v
ProjectConfig + per-declaration diagnostics
    |
    +--> activation      +--> tsuku install    +--> shim install
    +--> auto-apply      +--> tsuku run fast path
```

`SplitOrgKey`'s error is propagated rather than discarded (R1a). Its current
sole caller discards it, which is why the traversal reaches a sink at all.

**The version rule cannot be called where it lives.** `internal/project` cannot
import `internal/install`: the cycle is `project -> install -> shellenv ->
project` (`internal/install/precedence.go:7`, `internal/shellenv/activate.go:15`).
So `install.ValidateRequested` is extracted into a leaf, `internal/pinsyntax`,
importing only `fmt`, `strings` and `unicode` — which is everything the current
function uses — and `internal/install` re-exports or delegates to it. This is
the same extraction shape as the name predicate and carries the same proof
obligation: `install`'s existing tests must pass unchanged.

That edge is fragile in both directions, which is worth naming given D7. The
sibling change moving `activate.go` out of `shellenv` would break the cycle
from the other side — but relying on that would couple this fix to a PR that
lands after it, and the whole point of D7 is that this one stays independently
cherry-pickable.

**The source half gets a different rule from the bare name**, deliberately.
GitHub permits uppercase in an owner and a repository; `BurntSushi/toml` parses
`.tsuku.toml` in this repository. Each segment must be non-empty, must not be
`.` or `..`, and must contain no separator, but case is unrestricted. R9's
one-definition principle governs the name rule and stops there — extending it
to the source is the mistake this paragraph exists to prevent.

### How a refusal reaches the user

R4 forbids the write-only-field answer and R5 makes refusal per-declaration, so
`parseConfigFile`'s `error` return cannot carry it — that return aborts the
whole load, which is now reserved for a TOML parse failure. The carrier is a
diagnostics slice on `ConfigResult`, populated at parse and surfaced by each
consumer.

This is the one place the "no consumer changes" claim does not hold, and the
distinction matters: **the security property is inherited without any consumer
changing, because a refused declaration is already absent from the map they
read. The diagnostic is not.** Five call sites have to print it:
`internal/shellenv/activate.go:48`, `cmd/tsuku/install_project.go:55`,
`cmd/tsuku/cmd_shim.go:65`, `cmd/tsuku/cmd_run.go:95` and
`internal/updates/apply.go`. `cmd_run.go:95` discards the load error entirely
today (`projectCfg, _ :=`), so it needs the most work.

Diagnostics go to **stderr**, without exception. `cmd/tsuku/hook_env.go:51`
prints `FormatExports` to stdout and the shell hook evaluates it, so a
diagnostic written to stdout would be executed rather than read.

### The name predicate

One exported function in `internal/recipe`, layering the strict character rule
over the existing minimal one. The layering is not decoration: the minimal rule
is a blocklist that accepts `x$(id)y`, and the strict rule is the only control
that rejects `a:b` — where the colon is not a shell metacharacter and is not
traversal, but *is* the `PATH` separator.

That case is worth stating in the architecture because it determines the rule's
shape. `<tools>/a:b-1.0/bin` splits into two `PATH` entries, the second
`b-1.0/bin`, relative and resolved against the working directory. A containment
assertion passes it — the composed path really is inside the tools tree — and
the quoter cannot reach it, because `PATH` splitting happens after the shell's
word parsing. An allowlist is the only shape that stops it, which is why R2 is
written as one and why a denylist of shell metacharacters is not an acceptable
implementation.

### The quoter

`internal/shellquote`, standard library only:

- `POSIX(s)` wraps in single quotes with `'` rewritten as `'\''`. Body lifted
  verbatim from `internal/actions/set_env.go:252`, which is already correct.
- `Fish(s)` wraps in single quotes, escaping `\` first and then `'`. Order is
  load-bearing: escaping quotes first would then escape the backslash it just
  introduced.

Both return `''` for the empty string, and neither escapes newlines, which are
literal inside single quotes in both dialects.

To satisfy D3, emitters take the value rather than a format string — an
`export`/`set` helper that quotes internally — so that adding a variable
without quoting requires deliberately bypassing the helper rather than merely
forgetting to call it.

**One emission is not a single value and must not be quoted as one.**
`cmd/tsuku/shellenv.go:39` emits `export PATH="<binDir>:<currentDir>:$PATH"`,
where the trailing `$PATH` has to keep expanding — it is how the user's
existing `PATH` survives. Applying the helper to the whole statement quotes
`$PATH` too and silently discards the user's `PATH` for everyone following the
`eval $(tsuku shellenv)` the command documents. The correct emission quotes the
two interpolated components and leaves the expansion live:

```
export PATH='<binDir>':'<currentDir>':"$PATH"
```

This is the only place in the change where the safe transformation and the
correct one diverge, and it ships in Batch 1 — the batch described as
independently landable, which is exactly when a silent `PATH` wipe would be
least expected.

## Implementation Approach

Four batches. The order is chosen so each lands independently reviewable and
the security-critical pieces come first.

**Batch 1 — the quoter.** Add `internal/shellquote` with both functions and
their tests. Rewrite the eight `%q` sites in `FormatExports` and the two
unquoted sites in `cmd/tsuku/shellenv.go`. Three existing tests assert the
broken output and must be rewritten — that is the fix, not collateral. This
batch is independently landable and reduces exposure without touching
validation.

**Batch 2 — the name predicate.** Extract the strict rule in `internal/recipe`,
refactor `validateRuntimeDependencyNames` through it, keep its tests green.

**Batch 3 — the boundary.** Wire both predicates plus `install.ValidateRequested`
into `parseConfigFile`, propagate `SplitOrgKey`'s error, and delete the
now-dead fallback branch in `effectivePin`. Per-declaration refusal for value
failures; whole-file only for TOML parse failure.

**Batch 4 — backstop and record.** `IsValidRecipeName` at `recipePath` and
`Registry.cachePath`. Correct the ten enumerated claims across the three design
documents.

CI note: fish is in no workflow file. Batch 1 adds it to the Go test job, two
lines. A `LookPath`-guarded fish test would skip on every run and read as
coverage while being permanently green, which is worse than no test — and the
POSIX/fish divergence is exactly what a skipped test would miss.

## Security Considerations

**Closed.** Path traversal through either component of the `name-version`
composition, at every consumer. Command substitution and expansion in every
emitted value, both dialects, both branches. `PATH`-separator injection through
a colon in a name. Registry substitution through a traversing recipe name.

**Deliberately not closed here, with owners.** Discovery walking to `/` (#2555)
— a config a stranger owns still applies, which this design does not address.
The install path's `os.RemoveAll`/`os.Rename` reachable via `--recipe`, whose
entry point is a local file the user chose rather than a cloned repository.

**Residual risks accepted**, stated in full because a partial list is how a
gap becomes a claim.

- A caller constructing a `ProjectConfig` directly in Go bypasses the boundary.
  R10's backstop covers the registry path; the path helpers stay unguarded
  until the deferred hardening lands. Closing it is Option B, rejected on scope
  and not on merit.
- **`state.json` is a second read path into the same sinks.** `install/list.go:48`,
  `cmd_rollback.go:56` and `install/remove.go:49,150,257` — the last reaching
  `os.RemoveAll` — compose `ToolDir` from recorded names and versions that
  never pass this boundary. A malformed name cannot get *into* state once the
  boundary holds, but an existing one is not retroactively cleaned, and nothing
  validates on load.
- **The run path escalates to auto mode for project-declared tools.**
  `internal/autoinstall/run.go:117-121` sets `ModeAuto` because a tool is
  declared in the project config, so `evil-owner/evil-repo:jq = "1.0"` —
  impeccable under every rule this design adds — installs and executes from an
  attacker-chosen registry with no prompt at all. The PRD files the neighbouring
  problem as "the consent prompt should show the source"; on this path there is
  no prompt to fix. That is a consent gap rather than a validation gap, and this
  design does not close it.

**A control this design deliberately does not claim.** Containment assertion on
composed paths is *not* part of this fix, and the colon case is why it would
not be sufficient if it were.

## Consequences

### Positive

- One rule, one place. A future consumer of `ProjectConfig` inherits the
  guarantee without knowing it exists.
- The correct quoter becomes reachable by name, so the sibling chain's new
  emitted variable has an obvious right answer.
- Three false claims in the design record become true ones.

### Negative

- **Lowercase-only rejects an uppercase name that resolves today.** Accepted
  deliberately: `metadata.name` only warns on non-lowercase, so the case exists
  by omission, and forking the rule to allow it would recreate the divergence
  this fix exists to remove.
- **Per-declaration refusal changes behaviour.** A malformed entry that is
  silently skipped today becomes a named error. That is the point, but it will
  surface configs that were quietly broken.
- **Five tests change.** Three assert the broken quoter's output. Two more —
  `internal/recipe/name_test.go:24` and
  `internal/recipe/validator_runtime_deps_names_test.go:59` — pin `foo..bar` as
  rejected, which R2's segment semantics reverse.
- **Batch 2 alone loosens runtime-dependency validation** before the boundary
  lands, since `foo..bar` becomes acceptable there first. Harmless, and worth
  knowing if the batches are reviewed separately.
- **A fifth quoting answer becomes possible.** The leaf package reduces the
  incentive but does not prevent it; only the emit-helper shape does, and that
  is a convention rather than a compiler constraint.

### Mitigations

The lowercase narrowing is checkable rather than asserted: R12's criterion
sweeps all 1449 registry names. The behaviour change is bounded by R5 — a
refusal takes down one declaration, not the file — and by R4's requirement that
the error name the key, so a surfaced config says what to fix.
