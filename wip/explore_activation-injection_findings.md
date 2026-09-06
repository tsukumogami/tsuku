# Explore Findings: activation-injection

Round 1, six leads. Source files under `wip/research/`.

## The answer to the core question

**It is a class, and it is a placement class, not a knowledge class.**

The question was whether the two known defects are the whole set or the two
someone happened to find. Neither. tsuku knows how to write these guards — in
places it writes them better than average — and the defect is that the guards
sit one consumer deep from the choke point, while the design record cites each
guard's *existence* as proof it covers a sink it never runs on.

Six instances of the same shape, all confirmed against code:

| # | The control | Where it lives | Who calls it | The sink it does not cover |
|---|---|---|---|---|
| 1 | `SplitOrgKey` rejects `..` | `internal/project/orgkey.go:22` | one caller (`resolver.go:35`), **which discards the error** (`if err != nil \|\| !isOrg { continue }`) | parse, install, activation |
| 2 | `IsValidRecipeName` — doc says "the single source of truth ... a name accepted by one path is accepted by all" | `internal/recipe/name.go:21` | three, none on install or activation | `Config.ToolDir` and everything downstream |
| 3 | `runtimeDepNamePattern` + the `..`/`/`/`\`/null/leading-`-` rejections | `internal/recipe/validator.go:142-190` | `runtime_dependencies` only | tool names, config keys |
| 4 | `shellQuote` — a **correct POSIX quoter** | `internal/actions/set_env.go:252` | `set_env` only; unexported | `FormatExports`, one package away, which uses `%q` |
| 5 | `ValidateVersionString` / `ValidateRequested` | `internal/install`, `internal/version` | install-path sinks, `updates/apply.go:36` | activation, and `Manager.Install` itself |
| 6 | `ValidateSymlinkTarget` — containment on the *composed* path, the one control in `internal/install` that works | `internal/install/symlink.go:46` | one site | every `os.RemoveAll` / `os.Rename` in the same package |

And the shape is **symmetric**, which rules out "someone forgot about names":

- `filepath.Join(ToolsDir, name+"-"+version)` — version guarded, name not.
- `share/shell.d/{target}@{version}.{shell}` — target guarded by a strict regex
  (`internal/actions/shell_init.go:52`), version not checked at all
  (`shell_init.go:215-220`). Exact inverse, same filename family.

The single sharpest demonstration is `internal/updates/gc.go:77-93`: the author
pauses before an `os.RemoveAll`, writes eleven lines reasoning about *which* of
two competing version validators to use, concludes "Anything that could steer
os.RemoveAll out of the tools directory does not get there" — and on line 93
joins the entirely unvalidated `toolName` into the same expression.

## Three defects at the activation sink, not two

This is the finding that changes the fix. `ComputeActivation` consumes three
hostile-controlled values:

1. **`name`** → `cfg.ToolBinDir` → `filepath.Join` → `PATH`. Gated by `os.Stat`
   (the composed directory must exist). Fixed by a validator.
2. **`req.Version`** → the same composition, equally raw. `ValidateRequested` is
   already applied to this exact field by another consumer
   (`internal/updates/apply.go:36`), so the omission is an inconsistency, not an
   oversight. Fixed by a validator. **The brief's premise that "the version half
   is guarded" is true for the install-path sinks and false at this one.**
3. **`result.Dir`** → `export _TSUKU_DIR=%q` (`activate.go:151`) — **no `os.Stat`
   gate, no validation of any kind**, emitted on every activation whether or not
   a single tool is installed.

Point 3 is decisive. A hostile repository controls `Dir`: git tracks any byte
but NUL and `/` in a path component, so a repo ships a subdirectory named
`` a`...` `` or `a$(...)` containing a `.tsuku.toml`; `git clone` creates it and
`cd` fires `eval "$(tsuku hook-env bash)"`.

**`Dir` cannot be fixed by validating an input.** It is a legitimate absolute
path that happens to contain shell metacharacters. The only correct fix is at
the emitter. So:

- The quoter is **not** optional and cannot be subsumed by any validator.
- A name-only validator does not cover the activation path.
- Fixing `%q` alone neutralises the shell half of the name defect but leaves the
  path half open.

Three layers, none subsuming another: `IsValidRecipeName` is a blocklist and
`x$(id)y` passes it; the strict pattern closes command substitution in *names*;
the quoter closes everything reaching the emitter, including values that are no
recipe name at all.

## The quoter: four packages, four answers

| Package | Its answer to "make this shell-safe" | Correct? |
|---|---|---|
| `internal/shellenv` | Go's `%q` | **No** — `activate.go:134-152` |
| `cmd/tsuku` | nothing; bare `%s` inside hand-written `"…"` | **No** — `shellenv.go:40,47` |
| `internal/actions` | single quotes, `'\''` idiom | **Yes**, POSIX — `set_env.go:252` |
| `internal/install` | reject a denylist | Adequate; reject-not-quote — `manager.go:685` |

`cmd/tsuku/shellenv.go:40,47` is a **new in-scope finding and it is worse than
the known one**: it emits `export PATH="<binDir>:<currentDir>:$PATH"` and
`. "<cachePath>"` with no quoting whatsoever, while line 21 documents the command
as `eval $(tsuku shellenv)`. `%q` at least escapes `"`. Reach is narrower (it
interpolates `$TSUKU_HOME`), but dev builds set `DefaultHomeOverride` to the
*relative* `.tsuku-dev`, so `filepath.Abs` prepends the cwd and a checkout path
containing `$(…)` fires it.

**Placement: leaf `internal/shellquote`, standard library only, exporting
`POSIX` and `Fish`.** Not a cycle argument — `internal/shellenv` sits below both
`internal/install` and `internal/actions`, so local would compile. It is that
three packages plus `cmd/tsuku` need it, a correct copy already exists in a
fourth, and a quoter living inside a package named for per-directory PATH
activation guarantees a fifth variant. Leaf-package precedent exists:
`internal/httputil`, `internal/platform`, `internal/errmsg`.

**Two functions, not one.** POSIX single quotes are fully literal (POSIX.1-2017
§2.2.2). fish's single quotes recognise `\'` and `\\` and nothing else, so a
value containing a backslash correct under POSIX is wrong under fish. A single
`ShellQuote` that got fish wrong would be worse than today's bug because it
would look right. Also: fish's *double* quotes expand `$var` and, since fish
3.4, `$(cmd)` — so the current `%q` is exploitable on fish too. Backticks were
removed in fish 4.0, which is the detail behind the retracted fish-specific
claim: the bug is not fish-specific, and fish is not immune.

Two non-security facts about `%q` worth having: it renders a newline as literal
`\n`, so it **corrupts** such values rather than merely exposing them; and it
leaves `$HOME` to expand, so any legitimate path containing a dollar sign is
already broken today.

## The design record: three designs, not one row

The brief's line number is off by one — the PATH-injection row is at **418**;
419 is the "Auto-activation in cloned repos" row, which is accurate. An
implementer grepping for 419 edits the wrong row.

Every claim in `DESIGN-shell-env-activation.md`'s security section that asserts
a path-construction control is false: `:388` ("only produces paths within
`$TSUKU_HOME/tools/`" and "already guarded by the install pipeline's name
validation"), `:391` ("always under ... no arbitrary path injection"), `:393`,
`:401` ("structured output ... with validated values"), `:418` (both halves),
`:420` (wrong about where untrusted input enters — it treats hook-env output as
trustworthy because tsuku produced it, but the output is a function of the
repository's `.tsuku.toml`). `:411` overstates `TSUKU_CEILING_PATHS`.

The same shape appears in two other current designs:
`DESIGN-org-scoped-project-config.md:270` (twice — `parseDistributedName`
"rejects" `..` when it actually returns nil and lets the caller keep the raw
string, and a claim that `isValidRecipeName` keeps org names off the registry
fetch path, which it does not guard) and `:131` (says `SplitOrgKey` is "used by
both `runProjectInstall` and the resolver" — `runProjectInstall` uses a
*different* parser); `DESIGN-notification-routing.md:418`.

**The falsity is never in the existence of the guard. It is always in the claim
about where it is called.** A reviewer follows the citation, finds the function,
confirms it rejects `..`, and stops. That is a class of false record that
survives review by construction.

Two structural notes. The residual-risk column is more accurate than the
mitigation column, twice — whatever review produced the table checked that every
row had four cells, not that column three implied column four. And the project
already wrote the correct fact down elsewhere: `DESIGN-nvm-data-root.md:540`
says "`filepath.Join` cleans before it returns, so `../../../etc/passwd` escapes
silently", the precise negation of `:388`. Both are Current. Nothing reconciles
them.

No *user-facing* document claims tool names are validated. The source of the
belief is the design record citing itself.

## The test record is adverse, not absent

- **Three assertions pin the wrong quoter's exact output**
  (`activate_test.go:221`, `:378`, `:398` assert the double-quoted `%q` forms).
  A correct fix makes them fail. An implementer whose constraint is "don't break
  the tests" is steered back into double-quoting. Changing them is the fix, not
  a regression.
- **Five assertions compute their oracle by calling the function under test.**
  `TestComputeActivation_ProjectFound` asserts `strings.Contains(result.PATH,
  cfg.ToolBinDir("go","1.22"))` — satisfied by *any* `ToolBinDir`, including a
  traversing one. `setupProject` compounds it by creating the directory via the
  same call, so the `os.Stat` precondition is satisfied by construction too.
- **No fixture in the entire activation surface contains a character outside
  `[a-z0-9./:-]`.** Thirteen passing tests in `activate_test.go`, discriminating
  against neither defect.
- `TestSplitOrgKey`'s two traversal fixtures both carry a slash, so neither can
  tell which half of `../etc/passwd` did the work — and the slash is what routes
  it to the check at all.

The constructive half: both correct implementations and both correct *test
forms* already exist in-tree. `internal/actions/set_env_test.go:317` sources
emitted output in a real bash, asserts byte-identical round-trip **and** asserts
no side effect via `os.Stat` on a marker. `internal/install/symlink_test.go:199`
has the `tools-malicious` sibling-prefix fixture. Copy, don't reinvent.

CI reality: bash is present on `ubuntu-latest` and `macos-latest` and in-tree
tests already use `exec.LookPath("bash")` + `bash --norc --noprofile -c`. **fish
and zsh are in no workflow file.** A `LookPath("fish")`-guarded test would skip
on every CI run and read as coverage while being permanently green — worse than
no test. Either provision fish in the test job (two lines) or write a
table-driven unit test of the fish quoter against hand-derived expectations and
say in a comment why. zsh shares the `default:` branch with bash; a zsh test
adds nothing, and the test file should say so, so its absence is not read as an
oversight.

## Negative results, which bound the class

- **Process execution does not extend the class.** 139 non-test `exec` sites
  across 40 files: `argv[0]` is never externally supplied, only four pass a
  string to `sh -c` (all recipe-as-code by design), there is no `git`
  invocation and no `LD_PRELOAD` anywhere, and no reachable flag-injection
  instance. A genuine examined negative.
- **`internal/hook` is clean.** The rc snippet is two constant lines; it
  hardcodes `${TSUKU_HOME:-$HOME/.tsuku}` as text the *user's* shell expands,
  which is right. `internal/hooks` scripts are embedded verbatim.
- **`internal/shim` is the model for the class** — it refuses to interpolate at
  all and lets the shell resolve the name at runtime via `"$(basename "$0")"`.
- **`internal/actions` is well defended.** Archive extraction is contained by a
  kernel-enforced `os.Root` handle, deliberately anchored so an earlier entry's
  symlink cannot re-anchor a later one. No zip slip.
- **`.tsuku.toml` is the only repo-local file tsuku discovers**, and it has
  exactly one section, `[tools]`, whose values are a single optional `version`
  string. The input surface is small. `parseConfigFile` enforces only
  `MaxTools` and TOML type errors.

## Two traps the fix must avoid

**`ActivationResult.Skipped` is write-only in production.** Set at
`activate.go:117`, asserted only in tests, read by no production caller. The
issue's acceptance criterion says rejects must be "reported (not silently
skipped)" — routing them through `Skipped` would satisfy the letter and change
nothing. Parse-time rejection returning an error is the natural answer and
matches `parseConfigFile`'s existing shape (`parsing <path>: ...`).

**Validating the bare name does not stop `/` reaching `filepath.Join` at
activation.** `activate.go:73` iterates the **raw** map keys and never calls
`SplitOrgKey`. So a legitimate `tsukumogami/koto = "1.0"` makes activation
compute `tools/tsukumogami/koto-1.0/bin` while the installer put the tool at
`tools/koto-1.0` — the directory never exists, the entry lands in the unread
`Skipped`, and a supported config syntax silently fails to activate. That is a
functional bug sitting on the security one, and it means **`/` reaches
`filepath.Join` today through supported syntax, not only through a malicious
key.**

This directly contradicts the acceptance criterion the sibling chain wrote for
its Issue 4 ("a declared name containing `..`, `/` or `\` still activates
nothing"). As worded, that criterion is either satisfied by rejecting legitimate
org-scoped entries, or it pins the functional bug in place. It needs rewording,
and the two chains need to agree which of them fixes the raw-key iteration.

## Scope recommendation for this PR

**In:**
1. Leaf `internal/shellquote` with `POSIX` and `Fish`; lift the body of
   `internal/actions/set_env.go:250-254` verbatim for `POSIX`.
2. Replace the eight `%q` sites in `FormatExports` and the two unquoted `%s`
   sites in `cmd/tsuku/shellenv.go:40,47`.
3. Extract `IsStrictRecipeName` in `internal/recipe` (strict pattern layered on
   `IsValidRecipeName`) and route `validateRuntimeDependencyNames` through it,
   so there is one definition rather than a third copy.
4. Validate at `parseConfigFile` — the derived bare name after `SplitOrgKey`,
   **and the version** — rejecting with an error naming the offending key. This
   covers activation, install, shim install, auto-apply and the `tsuku run`
   fast path at once. For the version, reuse `install.ValidateRequested`, which
   `internal/updates/apply.go:36` already applies to this exact field; this
   codebase's disease is duplicate near-identical validators, not missing ones.
4b. Two `IsValidRecipeName` calls at `recipePath` and `Registry.cachePath` as
   sink-level backstop, documented as defence in depth beneath the boundary.
5. Correct the false claims in `DESIGN-shell-env-activation.md` (`:388`, `:391`,
   `:393`, `:401`, `:411`, `:418`, `:420`) and the two other designs.
6. Tests per the discriminating fixtures, not the illustrative ones.

**Out, with owners:**
- `--recipe` → `Metadata.Name` → `os.RemoveAll`/`os.Rename`, plus chokepoint
  hardening of the five `Config` `*Dir` helpers and containment assertions
  modelled on `ValidateSymlinkTarget`. **New issue.** Reachability confirmed:
  `Metadata.Name` becomes a directory component only via `cmd/tsuku/install.go:490`,
  whose sole caller is the `--recipe` branch — *not* reachable from a fetched
  distributed recipe, so this is a user-chosen local file, not a remote vector.
  The drive-by half of it (`install_project.go:72` carrying the raw key) is
  closed by item 4 above.
- `/`-walk discovery scope → **filed as #2555**.
- The consent prompt showing only the bare tool name and hiding the
  attacker-chosen registry source (`install_project.go:129-132`) → separate; its
  remedy is to show the source, not to escape a string.
- Activation's raw-key iteration (org-scoped tools never activate) → functional;
  needs an owner agreed with the sibling chain.
- Resolved version strings reaching `sh -c` unvalidated
  (`internal/version/transform.go:29` has no non-test caller) → folds into the
  chokepoint issue.

## Landed after the first report (round 1, sixth lead + sibling chain)

I reported this explore complete when one of six leads was still running — a
miscount, and the gate was passed on a false premise. Both items below arrived
after that and both change the scope. Recorded here rather than smoothed over,
because "an approval carries premises" is one of this batch's own findings.

### The version component escapes at a third sink, and that one execs

Predicted above; now confirmed from two directions. Reproduced here first-hand
against a build of `main`, with an entirely ordinary tool name:

```toml
[tools]
jq = "../../../../../../../../../../../../../../../../../../../../tmp/evil"
```

```
$ tsuku shell --shell bash
export PATH="/tmp/evil/bin:/home/…/.tsuku/bin:…"
```

The name is `jq`. **No name validator can reach this**, and `Skipped` stays
empty. `tsuku_autoinstall_consent` found the same thing at
`internal/autoinstall/run.go:100-108` — the already-installed fast path in
`Runner.Run`, which stats the constructed path and hands the process to it via
`syscall.Exec`, **returning before the mode dispatch and all four security
gates**. No consent mode, no audit record. It needs no activation hook at all,
which makes it broader than #2553's reported vector.

Two mechanical notes for whoever writes the test, both learned by getting it
wrong: `filepath.Clean` drops excess `..` past root, so over-deep is safe; but
the first component is `jq-..`, a literal directory name rather than an ascent,
so a traversal one segment shy lands somewhere harmless and looks like the bug
is absent. My first reproduction failed exactly that way and nearly went down as
a false negative.

Constraint from that chain, compatible with this fix: its R20 requires the
declared version to be reported **verbatim** by the resolver, so nothing may
normalise or rewrite versions on the read path. Rejecting at parse is fine — a
config that never loads never reaches the resolver. A *sanitising* fix at the
resolver would not be.

### The same key controls the registry fetch URL and the cache write path

`.tsuku.toml` gets full control of the central registry fetch — owner, repo, ref
and path. Verified first-hand:

```toml
[tools]
"../../../../example-nonexistent-owner/reg/main/tool" = "1"
```

```
recipe not found: HTTP 404 from
https://raw.githubusercontent.com/example-nonexistent-owner/reg/main/tool.toml
```

The default registry is `tsukumogami/tsuku`; the traversal climbed out of
`main/recipes/` and out of the repository. The host does not change, so this is
not SSRF — it is **recipe substitution**, and a recipe that resolves is then
parsed and executed by the normal install machinery. Wider than PATH injection,
same feed, same unvalidated key.

The same string is the disk-cache **write** key: `HTTPStore.Get` sets
`key := recipePath(name)`, and on a 200 `DiskCache.Put` does a bare
`filepath.Join(c.dir, key)` then `MkdirAll` + `WriteFile` with no containment
check — attacker-chosen path, attacker-chosen content. **Code-established, not
executed**: the researcher could not land a 200 (outbound requests gated in that
environment), and this wording should not be upgraded without one.

`FSStore.Get` (`internal/recipe/backing_store.go:68`) is the read equivalent for
local-directory registries — precisely the case `IsValidRecipeName`'s doc
comment names, from a function neither path calls.

### Scope amendment, ruled by the coordinator

Take **two extra `IsValidRecipeName` calls** — at `recipePath`
(`internal/recipe/provider_unified.go:271`) and `Registry.cachePath`
(`internal/registry/registry.go:95`). Two call sites of a function that already
exists; the exact case its own doc comment names; and declining to move a
control two lines while fixing the disease of controls sitting one consumer deep
would be committing the defect in the act of correcting it.

**Framing condition, and it matters:** these are **defence in depth, not the
fix**. The parse-time validator is the fix. If the design presents the sink
checks as the remedy, someone later removes the boundary check on the grounds
the sink is guarded, and the drive-by feed reopens. State the layer order
explicitly — boundary first, sink second, neither sufficient alone.

Everything else in the registry area stays with the deferred hardening issue.

## Settled during convergence

Decisions taken with `tsuku_project_consent` (the tsukumogami seat), recorded so
they are decisions rather than accidents.

**Charset: lowercase `[a-z0-9._-]`, reusing the extracted predicate.** This
flipped three times between us and is now closed on principle rather than
preference. The case *for* uppercase was real and is recorded so nobody thinks
it was free: `[A-Za-z0-9._-]` is exactly GitHub's repository-name charset, so
for org-scoped keys it is the precise upper bound on a legitimate name rather
than a guess; and `metadata.name` merely *warns* on non-lowercase
(`validator.go:280`), so an uppercase distributed recipe does resolve — the
narrowing rejects something reachable.

It loses to the one-definition principle. Taking uppercase would make this
validator `[A-Za-z0-9._-]` while `runtimeDepNamePattern` stays `[a-z0-9._-]`:
two patterns validating the same kind of string, differing by case, with a
comment between them. That is the exact shape of this entire bug. A documented
fork is still a fork, and it is how `IsValidRecipeName`'s "single source of
truth" comment became aspirational. If uppercase is worth supporting, it is
worth a deliberate separate decision that widens the shared predicate and
upgrades the metadata warning — not a wider rule smuggled into a security fix
and forked from the validator it exists to reuse.

Supporting fact: `validateRuntimeDependencyNames` *errors* on uppercase
(`validator.go:186`) while `validateMetadata` only warns, so tsuku already
treats uppercase names as second-class and inconsistently. Rejecting at the
`.tsuku.toml` boundary is consistent with the dependency path specifically —
"globally consistent" would be an overclaim.

**Fail closed, and name the key.** Parse-time rejection means a `.tsuku.toml`
with one malformed name fails to load entirely: `tsuku install` and activation
both refuse the whole file rather than skipping the bad entry. That is right for
a security boundary — do not partial-install around attacker-shaped input — but
it is a deliberate behaviour change, and the same rejection catches an honest
typo. The error must name the offending key: `invalid tool name "x$(id)y" in
.tsuku.toml`, not a bare parse failure.

**`validateRuntimeDependencyNames` is the extraction's own fixture.** At
`validator.go:186-191` it already applies `runtimeDepNamePattern` and *then*
`IsValidRecipeName` in sequence, with a comment calling it belt-and-suspenders.
That is precisely the call pair `IsStrictRecipeName` should encapsulate, and
refactoring that site to call it proves the extraction is faithful, because its
existing tests must still pass.

**The deferred issue's spine** is that `ValidateSymlinkTarget` checks containment
of the *composed* path, not cleanliness of a *component* — the lesson of this
whole sweep being that per-component blocklists are necessary and not
sufficient, while a containment assertion on the final path does not depend on
having enumerated every bad character.

## Decision: Crystallize
