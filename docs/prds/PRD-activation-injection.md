---
schema: prd/v1
status: Draft
problem: |
  A `.tsuku.toml` comes with a repository, and tsuku treats every value it
  declares as trusted. Tool names and declared versions become filesystem
  path components without being checked, so a declaration can place a
  directory outside the tools tree onto PATH or select a binary the shell
  then runs. Emitted shell text is quoted with a Go string-literal quoter
  rather than a shell quoter, so substitutions in emitted values execute
  when the output is evaluated. The checks that would stop all of this
  already exist in the codebase, wired to consumers other than these.
goals: |
  Values a repository declares are checked once, where the file is read, so
  every consumer inherits the guarantee instead of each re-deriving it.
  Nothing tsuku emits into shell-evaluated text can be interpreted as
  anything but data. A malformed declaration produces a diagnostic naming
  it rather than silent behaviour. The documents describing these controls
  describe what the code does.
absorbed:
  - docs/briefs/BRIEF-activation-injection.md
source_issue: 2553
---

# PRD: Guarding externally-supplied values at the config boundary

## Status

Draft

Absorbed [BRIEF-activation-injection](docs/briefs/BRIEF-activation-injection.md); carried in Absorbed Brief.

One finding behind these requirements is not yet filed anywhere: the declared
version escapes the same composition as the name, at a sink that executes
rather than only shaping `PATH`. Whether it is recorded on the existing issue
or takes its own is a filing decision outstanding at the time of writing. The
fix does not wait on it.

## Absorbed Brief

This feature's framing was written as a separate BRIEF and folded in here: the
problem it stated and the outcome it described are carried by the Problem
Statement, Goals and User Stories below, so keeping both would have cost a
reader two documents for one idea.

**Why this feature exists.** A `.tsuku.toml` arrives with a repository, and
cloning a repository is not an act of trust in the files it contains. tsuku
reads it as though it were. The framing that matters is not that some checks
are missing but that the checks which exist are wired to the wrong consumers,
one step away from the paths that need them — which is also why three design
documents could describe controls covering sinks they never run on, and why
nobody looked.

**What should be different afterwards.** Someone clones a repository they have
not read, works in it, installs or runs what it declares, and nothing that
repository wrote decides what executes on their machine. A malformed
declaration — hostile or a typo — is named rather than dropped. A maintainer
asking whether a control exists gets a true answer from the design record. A
contributor adding a value to tsuku's shell output finds one correct answer
already in the codebase instead of inventing another.

**The four entry points that exercise it** are carried as User Stories: an
activation hook firing on directory entry; `tsuku run` with no shell
integration at all, which is the widest path because no opt-in stands in front
of it; someone mistyping a name in their own config, which is the common case
and the reason the refusal has to be a diagnostic; and a reviewer reading the
design record to decide whether a control is real.

## Problem Statement

Cloning a repository isn't an act of trust in the files it contains, but
tsuku reads `.tsuku.toml` as though it were. Every value that file declares
flows into path construction and into shell text without being checked at any
point between the file and the sink.

Three consequences are reproduced. A declared **tool name** becomes a
directory component through a path join that cleans, so `..` in a name escapes
`$TSUKU_HOME/tools` and puts an attacker-chosen directory at the front of
`PATH`. A declared **version** reaches the same composition, equally
unchecked, and additionally reaches a fast path that stats the constructed
location and replaces the process with whatever it finds — before any consent
mode is evaluated and before any audit record is written, with no shell
integration involved at all. And **emitted shell text** is quoted with Go's
`%q`, which escapes `"` and `\` but not `$` or the backtick, so command
substitution in any emitted value executes when the hook output is evaluated.

A fourth consequence has no shell and no activation in it: the same
declaration determines the URL a recipe is fetched from, and the traversal
climbs out of the configured registry repository entirely.

What makes this a class rather than four bugs is where the existing checks
sit. tsuku contains a recipe-name validator documented as "the single source
of truth", a strict character pattern applied to dependency references
justified on the grounds that those references reach shell-visible paths, two
version validators, a containment assertion that correctly compares a composed
path against its root, and a correct POSIX shell quoter. Each is called by one
or two consumers and by none of the paths above. The guards are sound; they just sit one
consumer deep from the point where untrusted data enters.

That placement is also why the gap persisted. Three current design documents
assert that a path-construction control covers a sink, and in each case the
control is real and lives elsewhere. One risk table rates the reported defect
Low under a mitigation that was never built, while the residual-risk cell in
the same row describes the actual behaviour.

## Goals

- A value a repository declares is checked once, at the point the file becomes
  a configuration object, so that no consumer can reach a sink with an
  unchecked value and a future consumer doesn't have to remember to check.
- Nothing tsuku emits into text a shell evaluates can be interpreted as
  anything other than data, under every shell tsuku supports.
- A malformed declaration produces a diagnostic that names it. Whether hostile
  or a typo, the person reading the error can act on it.
- One definition of what a well-formed name is, reused rather than restated.
- The design record describes the controls that exist.

## User Stories

**As someone who cloned a repository I have not read**, I want the tools it
declares to be unable to change what executes on my machine, so that working
in an unfamiliar checkout isn't a decision I have to make carefully.

**As someone who runs a project tool with no shell integration installed**, I
want the same protection, so that the absence of an opt-in feature isn't what
stands between me and arbitrary execution.

**As someone editing my own `.tsuku.toml`**, I want a mistyped tool name to
tell me what's wrong and which key it objected to, so that the check that
protects me from a hostile config also helps me fix my own.

**As a maintainer deciding whether a control exists**, I want the design record
to answer that question correctly, so that following a citation and finding a
real function is the end of the investigation rather than a false floor.

**As a contributor adding a new value to tsuku's shell output**, I want a
correct quoter to be the obvious thing to reach for, so that the next emitted
value doesn't reintroduce this defect.

## Requirements

### Functional

**R1.** Tool names declared in `.tsuku.toml` are validated when the file is
read. The value validated is the **derived bare name** — the name that
survives org-key splitting — not the raw key, because a legitimate org-scoped
key contains `/` and validating the raw key would reject every one of them.

**R1a.** A key whose org-split **fails** is itself a refusal. The split's error
is propagated, never discarded, and a declaration that yields no bare name is
never treated as if it had passed validation. This is stated separately
because the existing splitter already rejects traversal in org-scoped keys and
its only caller discards that error — a refusal derived from "the bare name
was invalid" would miss every key that never produced a bare name at all.

**R1b.** The org-scoped half of a key is validated too, not only the bare name.
An `owner/repo` source reaches path and URL construction downstream, so a key
whose source half carries traversal is refused even when its bare name is
well-formed.

The source rule is **not** R2. It must be wider: GitHub permits `[A-Za-z0-9-]`
in an owner and `[A-Za-z0-9._-]` in a repository, and uppercase owners are
ordinary — `BurntSushi/toml` is what parses `.tsuku.toml` in this very
repository. Applying R2's lowercase-only rule to the source half would refuse
it. Each of the two segments must be non-empty, must not be `.` or `..`, and
must contain no path separator; case is not restricted. R9's one-definition
principle applies to the *name* rule and must not be extended here, which is
the mistake this paragraph exists to prevent.

**R2.** A valid bare name is a single safe path segment: it consists only of
lowercase letters, digits, `.`, `_` and `-`; it is non-empty; it does not
begin with `-` or `.`; and it is not `..`. The rule is an **allowlist**:
anything outside that character set is refused, without enumerating what is
dangerous. Two reasons it must be an allowlist rather than a denylist of
known-bad characters. A denylist of `/`, `\`, `..` and null accepts
`x$(id)y`, one of the two reproduced attack values. And a denylist extended to
cover shell metacharacters still accepts `a:b`, where the colon is not a shell
metacharacter at all but *is* the PATH separator — so the composed entry
splits into two, the second relative to the working directory. This codebase
denylists in three places, so the denylist is the shape to expect and the one
R2 rules out.

The `..` rejection is a **path-segment** rule, not a substring rule, so
`foo..bar` is accepted. Where R2 and R9 conflict, R2 defines the accept/reject
set and the shared definition is brought to it — the two rules R9 points at
currently test `..` by substring, so making them one rule is not a purely
mechanical extraction and must not silently adopt the looser semantics in
either direction.

**R3.** Declared versions in `.tsuku.toml` are validated when the file is read.
The rule is the one already applied to this same field at the resolution
boundary — it accepts the documented pin forms and rejects `..`, `/` and `\`.
It is *not* the other same-named version check in the codebase, which permits
`/`, never tests for `..`, and therefore accepts a traversal; that one answers
"is this a plausible version token", which is a different question from "is
this safe to compose into a path". Validation **rejects**; it never
normalises, repairs, or rewrites a version string.

**R4.** A declaration that fails R1, R1a, R1b, R2 or R3 is refused, and the
refusal names the offending key and states what shape was expected. The
refusal reaches the user — on the command's error output, from every command
that reads the config. Appending the key to a field no production caller reads
does not satisfy this; the activation result already has such a field, and
every silent skip today goes into it.

**R5.** A refusal invalidates **that declaration**, not the whole file. Other
declarations in the same file continue to be honoured. This applies to value
validation only; a file that cannot be parsed as TOML is refused whole.

**R6.** Every value tsuku emits into text that a shell evaluates is quoted
such that it cannot be interpreted as anything but data — no parameter
expansion, no command substitution, no word splitting, no globbing. This holds
for POSIX shells and for fish, whose quoting rules differ.

**R7.** R6 covers every current emitter. There are two: the activation
formatter in `internal/shellenv`, which quotes with a Go string-literal
quoter, and the `shellenv` command (`cmd/tsuku/shellenv.go`), which
interpolates into hand-written double quotes and escapes nothing at all under
a command its own help documents as something to evaluate. Fixing only the
first satisfies neither R6 nor R7.

**R8.** A value that round-trips through emission and evaluation is byte-identical
to the value tsuku held, including values containing newlines.

**R9.** The name rule has one definition in the codebase. Existing consumers of
the equivalent rule are routed through that definition rather than keeping
parallel copies.

**R10.** The recipe-name path that determines a fetch URL and a cache write key
is guarded at the sink as well as at the boundary, so a caller that reaches it
without passing through config load is still refused.

**R11.** Three named documents state what the code does, in the specific
claims listed here. This is a closed set, not an open sweep:

- `docs/designs/current/DESIGN-shell-env-activation.md` — the security
  section's claims that resolution only produces paths within the tools tree,
  that traversal is guarded by install-pipeline name validation, that bin
  directories are "always" under the tools tree, and that hook output is
  "structured (export/unset statements with validated values)"; the
  self-contradicting risk row, whose mitigation cell claims a control the
  residual-risk cell in the same row describes as absent; and the row treating
  hook output as trustworthy because tsuku produced it, when its content is a
  function of the repository's config.
- `docs/designs/current/DESIGN-org-scoped-project-config.md` — the claim that
  the distributed-name parser rejects `..` when it returns nil and lets the
  caller keep the raw string; the claim that a name check keeps org-scoped
  names off the registry fetch path, which it does not guard; and the claim
  that the org-key splitter is used by both the project-install path and the
  resolver, when the install path uses a different parser.
- `docs/designs/current/DESIGN-notification-routing.md` — the claim that
  recipe names are validated kebab-case when added to the registry.

### Non-functional

**R12.** No configuration that is valid today and that doesn't exercise a
defect becomes invalid, with one recorded exception: an uppercase *tool name*,
which R9's shared rule refuses and which the Decisions section records as
chosen rather than free. The exception is confined to the bare name — it does
not extend to org sources (R1b) or to versions, both of which remain
case-permissive. Every tool name in the recipe registry continues to be
accepted.

**R13.** A declaration refused at config load is refused for every command
that reads that config — activation, install, shim install, background
auto-apply, and the run path.

## Acceptance Criteria

Criteria are written to fail a wrong implementation, not to illustrate a right
one. Where a fixture is named, it is named because a plausible wrong
implementation passes without it.

### Name validation

- [ ] A declared name of `../../../../tmp/evil` is refused, and no PATH entry
      outside the tools directory is produced. **The escaped bin directory
      must exist on disk for this fixture.** Activation gates on a stat before
      adding anything to PATH, so without that precondition today's unfixed
      code also produces no entry, and the criterion passes against an
      implementation that does nothing.
- [ ] A declared name of `a/../../b` is refused. (Fails an implementation that
      only rejects the literal string `..`.)
- [ ] A declared name of `..` is refused. (Fails an implementation that cleans
      the name and compares it to itself.)
- [ ] A declared name of `../tools-evil/x` is refused, and the assertion that
      catches it is containment of the composed path with a separator
      appended, not a string prefix. (Fails an implementation that checks the
      joined result begins with the tools directory, since `tools-evil`
      prefixes `tools`.)

**The criteria above are all satisfied by a minimal implementation** that
merely stops discarding the org-split error and applies the existing blocklist
of `/`, `\`, `..` and null to the bare name. That implementation violates R2
and leaves every value below accepted. These are the criteria that separate
the strict rule from the blocklist, and they are the ones that matter most:

- [ ] A declared name of `x$(id)y` is refused. This is one of the two
      reproduced attack values, it contains no `/`, no `\`, no `..` and no
      null byte, and it passes the existing blocklist untouched.
- [ ] A declared name of `` a`id`b `` is refused.
- [ ] A declared name of `tool name` (containing a space) is refused.
- [ ] A declared name of `a;b` is refused.
- [ ] A declared name of `-rf` is refused. (Leading `-`; a name that becomes a
      flag rather than an operand if it ever reaches an argv position.)
- [ ] A declared name containing a newline is refused.
- [ ] A declared name of `a:b` is refused, and the error names the colon.
      **This is the fixture that proves an allowlist rather than a denylist.**
      A colon is not a shell metacharacter, so it lands in no plausible
      denylist — and this codebase denylists in three separate places, so a
      denylist is the shape an implementer reaches for. It is also the PATH
      separator: `<tools>/a:b-1.0/bin` is parsed by the shell as *two* entries,
      the second being `b-1.0/bin`, which is relative to the working
      directory. R6 and R7 cannot backstop this one, because PATH's
      colon-splitting happens after the shell's word parsing — quoting
      protects the value from the parser, not from `PATH`'s own semantics.
      Nor does a containment assertion catch it: `<tools>/a:b-1.0/bin` really
      is inside the tools tree, so R10's backstop passes it. The allowlist is
      the only control that stops it.
- [ ] A declared name of `.hidden` is refused. The strict pattern *accepts* a
      leading dot, since `.` is in the character class, so this is the only
      fixture separating "applied the pattern" from "applied the pattern plus
      R2's prefix rules". Neither `-rf` nor `..` covers it.
- [ ] A declared name containing a non-ASCII letter — `café`, or a Cyrillic
      homoglyph of an ASCII name — is refused. No ASCII denylist notices
      these; only an allowlist does.
- [ ] A declared name of `UPPER` is refused, and the error says tool names
      must be lowercase rather than reporting a bare pattern mismatch.

Org-key refuse cases, which the bare-name rule alone does not reach:

- [ ] A declared key of `../../../../example-nonexistent-owner/reg/main/tool`
      is refused. This key fails the *org-split* rather than the bare-name
      check — it never yields a bare name — so an implementation that refuses
      only on "the derived bare name was invalid" lets it through. (Fails an
      implementation that discards the split error, which is precisely what
      the splitter's only current caller does.)
- [ ] A declared key of `evil/../../../tmp/x:tool` is refused, for the same
      reason: the source half reaches path construction downstream, so
      validating only the bare name is insufficient.
- [ ] A declared key of `ow$(id)ner/repo:jq` is refused. Its bare name (`jq`)
      is impeccable and its source half contains no `..`, so it survives both
      the org-split and any bare-name check. Only R1b catches it. (Fails every
      implementation that validates the derived name and trusts the source.)

Further refuse cases:

- [ ] A declared name containing a null byte is refused.
- [ ] A declared key of `jq@2.0.0` is **refused**, and the error says the
      version belongs in the value or, for a distributed recipe, in the
      org-scoped form. See the decision below — this is the deliberate half of
      an asymmetry, not an oversight.

Accept cases, which pin the rule against over-rejection:

- [ ] A declared name of `llama.cpp` is **accepted**, and its bin directory
      reaches the emitted PATH. Asserted on the PATH, not on the absence of an
      error: an implementation that validates correctly and then routes the
      survivor into the unread skip field satisfies "accepted" and activates
      nothing. That is the write-only-field trap R4 closes on the refusal
      side, arriving from the other direction. (Also fails an over-strict
      rule; this is a real recipe and the only one of 1449 registry names
      containing a dot.)
- [ ] A declared name of `hdrhistogram_c` is **accepted**. (The only registry
      name containing an underscore character; fails a rule that allows only
      `[a-z0-9.-]`.)
- [ ] A declared name of `7zip` is **accepted**. R2 bars only `-` and `.` as a
      first character, so a leading digit is legal — but zero of the 1449
      registry names start with one, so the corpus sweep does not exclude
      `^[a-z][a-z0-9._-]*$`, which is the natural pattern to write.
- [ ] A declared name of `foo..bar` is **accepted**. (Pins R2's segment
      semantics against the substring test both reused rules currently use, so
      the extraction required by R9 cannot silently adopt the looser form.)
- [ ] A declared key of `tsukumogami/koto` is **accepted** and resolves to the
      bare name `koto`. (Fails an implementation that validates the raw key,
      which would reject every org-scoped declaration.)
- [ ] A declared key of `tsukumogami/registry:mytool@2.0.0` is **accepted** and
      resolves to the bare name `mytool`.
- [ ] A declared key of `BurntSushi/toml` is **accepted**. Uppercase is legal
      in a GitHub owner, and this one parses `.tsuku.toml` in this repository.
      (Fails an implementation that applies R2's lowercase-only bare-name rule
      to the source half — which R9's one-definition principle actively
      encourages, and which passes every refuse fixture and both all-lowercase
      org accepts.)
- [ ] A name that passes validation but whose directory does not exist is
      skipped without an error, and that outcome is distinguishable from a
      refusal. (Pins skip and reject as different outcomes, so an
      over-rejecting validator cannot hide as the pre-existing skip
      behaviour.)
### Version validation

- [ ] A declared version of `../../../../../../../tmp/evil` on a tool named
      `jq` is refused, and no path outside **the tools directory** is
      produced. The assertion is against the tools directory, not against
      `$TSUKU_HOME` — with `jq-` consuming the first segment, `../..` resolves
      to `<tools>/bin` and three `..` is the minimum that escapes, landing at
      `$TSUKU_HOME/bin`. A fixture with too few segments, or an assertion
      anchored at `$TSUKU_HOME`, reports a false negative.
- [ ] `latest`, an exact pin, a prefix pin, the empty string, `1.2.3-rc1`, and
      a leading-`@` channel such as `@lts` are all accepted. The last two are
      named because a rule derived from the common cases rejects both: a
      prerelease suffix and a channel marker are the version forms most likely
      to be lost to an over-strict pattern, and the pin matcher special-cases
      a leading `@`.
- [ ] A declared version of `1.2.3-RC1` is accepted. The version rule is
      case-permissive and must not be harmonised with R2's lowercase-only name
      rule — a real risk in a change whose theme is one definition reused.
- [ ] A declared version of `v1.2.3` is accepted **and reaches the resolver as
      `v1.2.3`**. (Fails a sanitising implementation. Every other accept
      fixture here is a fixed point of a leading-`v` strip, so this is the
      only one that detects one.)
- [ ] A declared version containing a colon — `1.0:evil` — is refused. The
      colon attack has a twin on the version component: `<tools>/jq-1.0:evil/bin`
      splits exactly as the name case does. The delegated rule is an allowlist
      and already rejects it, so this criterion pins that property rather than
      adding a rule; it exists because R3 delegates, and a delegation needs a
      test proving the delegate covers the case.
- [ ] No accepted version is altered by validation. The value the resolver
      receives is the value the file declared.

### Shell quoting

- [ ] Output built from a value containing `$(...)` is evaluated in a real
      shell; no side effect occurs and the value reads back byte-identical.
      The side-effect assertion is what makes this a safety test rather than a
      string comparison.
- [ ] The same for a value containing a backtick. (Fails an implementation
      that escapes `$` but not the backtick.)
- [ ] The same for a value containing `$HOME`, which must read back as four
      literal characters rather than expanding.
- [ ] The same for a value containing a single quote. (Fails the naive
      single-quote wrapper, which is the failure the obvious fix introduces.)
- [ ] The same for a value containing a literal newline, which must survive
      the round trip as a newline.
- [ ] The same for a plain value with no metacharacters, asserted on the
      round-trip rather than on the emitted literal. (An assertion on the exact
      emitted string is satisfied by whatever implementation generated the
      expectation, which is how the current tests came to pin the defect.)
- [ ] All of the above hold for the deactivation output as well as the
      activation output, which is a separate branch.
- [ ] A value containing **two adjacent backslashes** round-trips
      byte-identical under **fish**. This is the criterion that separates the
      two dialects, and without it the whole section is passed by an
      implementation that uses the POSIX quoter for fish as well. POSIX single
      quotes are fully literal; fish's recognise `\\` and `\'`. So `a\\b`
      POSIX-quotes to `'a\\b'`, which fish reads back as `a\b` — silently
      corrupted. Every other fixture in this section round-trips identically
      under both dialects, so every other fixture is blind to this mistake,
      which the investigation identified as worse than the defect being fixed
      because it looks correct.
- [ ] The remaining fixtures above hold under fish as well as POSIX. If fish
      cannot be run where the suite runs — it is currently in no workflow
      file, while bash is present and already used this way by existing tests
      — the fish rules are asserted against hand-derived expectations and the
      test file says why. A fish test guarded on the binary being present
      would skip on every run and read as coverage, which is worse than
      having none.

### Refusal behaviour

- [ ] A file with one malformed declaration and several valid ones honours the
      valid ones — asserted by the valid tools' bin directories reaching the
      emitted PATH, not by the absence of an error.
- [ ] The refusal is written to **stderr**, asserted per consumer by capturing
      the two streams separately. Stderr here is load-bearing rather than
      conventional: `tsuku hook-env` prints activation output to **stdout** and
      the shell hook evaluates it, so a diagnostic on stdout is executed rather
      than read — a new injection vector introduced by the fix for an injection
      vector. Anyone later moving it to stdout for tidiness reopens that.
- [ ] The refusal names the offending key and reaches the user on the
      command's error output. Asserted on
      what the command prints, not on a struct field — appending the key to a
      field no production caller reads passes any assertion phrased as "the
      refusal names the key", and the activation result already has exactly
      such a field.
- [ ] A file that is not valid TOML is refused in its entirety.

### Reuse and record

- [ ] The name rule exists in exactly one place. The existing consumer of the
      equivalent rule calls it, and that consumer's own tests still pass —
      which is what demonstrates the extraction is faithful rather than
      merely similar.
- [ ] Each of the ten claims enumerated under R11 either states what the code
      does or is removed. Checked against that list, not against the design
      corpus at large. The activation record's claims are at lines 388, 391,
      393, 401, 418 and 420 of `DESIGN-shell-env-activation.md`; note the risk
      row is at **418**, not 419 — 419 is a different row and is accurate, so a
      reviewer working from the wrong line edits the wrong thing.
- [ ] Every one of the 1449 tool names in `recipes/` is accepted by the new
      rules. (The version half of R12 names no corpus: there is no
      `.tsuku.toml` checked into this repository — tests build them inline —
      so the version side is pinned by the named accept fixtures above rather
      than by a corpus sweep.)
- [ ] A declaration refused at config load is refused identically by
      activation, install, shim install, auto-apply and the run path. Asserted
      per command, because "the boundary covers everyone" is the claim most
      likely to be true of the implementation the author had in mind and false
      of the one that ships.

- [ ] The refusal states what shape was expected, not only that the value was
      rejected. A message naming the key and saying "invalid" satisfies R4's
      first clause and fails this one.

### Sink-level defence

- [ ] A recipe name that reaches the fetch-URL and cache-write path without
      passing through config load is refused there. Exercised by calling that
      path directly, since no config-driven route reaches it once R1-R3 hold —
      which is the point of a backstop and also the reason it needs its own
      criterion rather than riding on a config fixture.
- [ ] The boundary check and the sink check are both present. Removing either
      one leaves a failing test. (A backstop that duplicates the boundary is
      untestable as a backstop unless the test can distinguish them.)

### Emitter coverage

- [ ] The `shellenv` command's output survives the same evaluation test as the
      activation formatter's. (Fails an implementation that fixes the
      activation formatter and stops — which passes every other quoting
      criterion here, since they all exercise activation.)
- [ ] **After evaluating `shellenv` output, `PATH` still contains the `PATH`
      that preceded it.** That command emits
      `export PATH="<binDir>:<currentDir>:$PATH"`, and the trailing `$PATH` is
      the one expansion in the file that must keep expanding. An implementation
      that single-quotes the whole statement round-trips both interpolated
      components byte-identical, satisfies every "no substitution occurs"
      criterion above, and silently discards the user's `PATH` for everyone
      following the `eval $(tsuku shellenv)` the command's own help documents.
      This is the only place where the safe fix and the correct fix diverge,
      so it is the only criterion that catches over-quoting.

## Decisions and Trade-offs

### The blast radius of a refusal is the declaration, not the file

Closes the upstream brief's Open Question.

The alternatives were refusing the whole file on any validation failure, or
refusing only the offending declaration. Both satisfy the security
requirement, which is that the bad value never reaches a sink. What's actually
being chosen is what happens to the declaration's siblings.

Per-entry won on three grounds. A `.tsuku.toml` is usually a shared team file,
and a config fanned out to many working roots turns one bad key into no tools
anywhere. A consumer of tsuku reported that shape during review: a config
materialised to many roots, installed by a deliberately non-fatal setup script,
would surface the failure as a single warning line inside an otherwise-
successful run. That report is unattributed here and is offered as a
plausibility argument rather than as measured evidence. Whole-file refusal also
hands an attacker a cheap denial of service: plant one malformed entry and
nobody in that repository gets project tools. And the argument that a config
which failed validation cannot be partially trusted borrows its force from the
**parse-failure** case, where the untrusted region genuinely is the whole
file, and doesn't transfer to the validation case, where the file parsed and
exactly one key is bad. R5 keeps whole-file refusal for parse failure for
precisely that reason.

The cost is that a partially-activated environment could be mistaken for a
full one. R4's named diagnostic answers it, and R4 is required either way.

The counter-case the upstream brief named as the one worth hunting — a pinned
toolchain where getting some tools and not others is worse than getting none —
was looked for and does not hold, for a reason that settles the question rather
than merely surviving it. **Partial activation is already tsuku's behaviour.** A
declared tool whose directory isn't present is skipped today and always has
been, so a config naming five tools of which one isn't installed already
activates four. Per-entry refusal doesn't introduce partial activation; it
makes one more case behave like the case that already ships. The counter-case
is an argument against existing behaviour, not against this change, and taking
it seriously would mean whole-file failure whenever any declared tool is
missing.

Recorded provenance: this reverses an earlier ruling recorded during this
chain's exploration, since deleted, which had bundled "must fail loudly"
together with "must fail whole". The bundling was the defect — the opposite of
a silent skip is a loud skip, not a whole-file refusal.

### `@version` in a key is org-scoped only

An asymmetry falls out of the splitter's control flow and has to be decided
rather than inherited. `tsukumogami/registry:mytool@2.0.0` is accepted, because
the splitter strips an `@version` suffix — but only on the branch it reaches
after confirming the key contains a `/`. A plain `jq@2.0.0` takes the early
return, comes back whole, and meets the strict pattern, which rejects the `@`.
So the org-scoped form works and its plain sibling does not, by accident.

Decided: the plain form is refused, and the error says where the version goes.
Two reasons. It costs nothing today — a key of `jq@2.0.0` builds a directory
name the installer never writes, so it already fails, silently, by never
activating; refusing it loudly is strictly better than what ships. And
accepting it would mean stripping `@version` from non-org keys too, which
changes parsing for every consumer of the config and is a larger change than
this feature should make.

Note this concerns `@` in a **name**. `@` in a **version** is legitimate and
stays so: a leading-`@` channel is a documented pin form and the pin matcher
special-cases it. R2 governs names; R3 governs versions; only R2 rejects `@`.

### Validation belongs at config load, not at the sinks

The alternative was validating at each sink. Config load wins because it is
the single point where the file becomes a configuration object, so every
consumer inherits the result; because sink-level validation is what produced
the current state, in which six correct guards each cover one consumer; and
because the sinks aren't a closed set — a future consumer would have to
remember. R10 keeps a sink-level check as well, deliberately as a backstop
beneath the boundary rather than as the remedy, so that neither is mistaken
for the whole job.

### Rejecting, never repairing

A sibling chain requires the declared version to be reported verbatim, so
nothing may rewrite a version on the read path. R3 states this as a
requirement rather than leaving it as an implementation preference, because a
sanitising implementation would satisfy a naive reading of "validated".

### The strict pattern is a sole control, not defence in depth

Worth stating because it was misjudged once already, including by the reviewer
who approved the narrowing below. The allowlist looks like belt-and-braces over
the existing name blocklist. For one attack class it is the only control there
is.

A colon in a tool name is not path traversal. `<tools>/a:b-1.0/bin` composes to
a path that is genuinely *inside* the tools tree, so a containment assertion
correctly says yes — and by then `PATH` has already been split on the colon
into two entries, the second of which (`b-1.0/bin`) is relative and resolves
against the working directory, which for a cloned repository is a directory
the attacker wrote. The quoter cannot reach it either: colon-splitting happens
after the shell's word parsing, so quoting protects the value from the parser
and not from `PATH`'s own semantics.

That is path-*separator* injection rather than path *traversal*, and it defeats
both of the other controls this feature adds. The allowlist is what stops it.
Any later proposal to relax R2 toward a denylist — the shape this codebase uses
in three other places — has to answer this case specifically.

### One definition, even at the cost of a narrower rule

R9 forces the name rule to be shared with the existing dependency-reference
consumer. That makes the rule lowercase-only, which will reject an uppercase
name that resolves today, since the metadata check only warns on
non-lowercase. Widening to allow uppercase was considered and rejected: it
would fork this rule from the one it reuses, differing by case with a comment
between them, which is the exact shape of the defect being fixed. If uppercase
is worth supporting, it is worth a separate decision that widens the shared
rule everywhere.

Note the weighing changed after this was first decided. The narrowing was
accepted on consistency grounds, against what was then believed to be a
defence-in-depth control. Given the section above, R9's rule carries security
weight with no substitute, so the reachable-case objection is now weighed
against that rather than against tidiness. The decision is unchanged; its
reasons are stronger than the ones originally recorded.

## Out of Scope

Each exclusion is recorded so that a reader doesn't mistake its absence for an
oversight. Two carry issue numbers; the rest name where the work sits rather
than a tracked item, because no issue exists for them yet.

- **Config discovery walking to the filesystem root**, so a config in a
  world-writable directory applies to every user beneath it. Filed as #2555.
  A discovery-scope defect, not an input-validation one.
- **The install path's destructive sinks** — an unvalidated name reaching
  recursive removal and rename — and the structural hardening that would close
  them at the path helpers. Its sharp entry point is a local file chosen with
  an explicit flag rather than a cloned repository, and the drive-by half is
  closed by R1-R3 anyway. Its own issue.
- **Org-scoped declarations never activating**, a supported syntax that
  resolves to the wrong directory. Functional rather than security; owned by
  the chain already rewriting that resolution.
- **`shellenv` emitting POSIX syntax under every shell**, which makes a
  documented fish path unusable. Filed as #2556. R6 and R7 fix that emitter's
  *quoting*; they don't give it a fish dialect.
- **The install consent prompt showing only the bare tool name** and hiding the
  registry source a declaration selected. Its remedy is to display the source,
  not to escape a value.
- **Non-exact version pin resolution** and the reporting contract for skipped
  tools. A separate chain owns both. R3 covers the declared string; the
  resolved string remains that chain's to guard, and neither check subsumes
  the other.
- **Any general audit.** The boundary is values originating outside the user's
  control that end in a filesystem path, in text a shell evaluates, or in a URL
  selecting where a recipe is fetched from. Process execution was examined and
  ruled out.
