# Lead: What do the committed design and test records claim about these controls, and is the line-419 row the only false claim?

Scope note: this covers only claims about controls over **filesystem path construction**
or **shell-evaluated text**. Claims about XSS in the dashboard, ANSI sanitization in the
terminal reporter, URL credential scrubbing, prompt injection in the LLM pipeline, and
checksum/signature verification were read and set aside as out of scope.

One correction to the brief before anything else: the PATH-injection row is at
**line 418**, not 419. Line 419 is the "Auto-activation in cloned repos" row, which is
accurate. An implementer grepping for line 419 will edit the wrong row.

---

## Findings

### Part 1 — the design record

#### The activation design's security section, claim by claim

`docs/designs/current/DESIGN-shell-env-activation.md` has a four-part Security
Considerations section (lines 384-421). Every claim in it that asserts a
path-construction control is false. The one true claim in the section is about hooks
being opt-in.

| Doc:line | Exact claim | True? | What the code does |
|---|---|---|---|
| DESIGN-shell-env-activation.md:388 | "the resolution only produces paths within `$TSUKU_HOME/tools/`, which is a controlled directory" | **False** | `Config.ToolBinDir` (`internal/config/config.go:437`) → `ToolDir` (`:432`) → `filepath.Join(c.ToolsDir, fmt.Sprintf("%s-%s", name, version))`. `filepath.Join` calls `Clean`, so `name = "../../../../tmp/evil"` yields `/tmp/evil-<version>/bin`. The result is not constrained to `ToolsDir`; nothing checks it afterwards. |
| DESIGN-shell-env-activation.md:388 | "Path traversal in tool names is already guarded by the install pipeline's name validation." | **False** | There is no name validation in `internal/install`. `Manager.Install` (`internal/install/manager.go:153`) calls `m.config.ToolDir(name, version)` with no check on `name`. `ValidateVersionString` is called in `Activate` (`:429`) and `RemoveVersion` (`remove.go:123`) — the *version* only, and not on the `Install` path at all. `recipe.IsValidRecipeName` exists (`internal/recipe/name.go:21`) but has exactly three non-test callers: `internal/distributed/cache.go:68`, `internal/index/rebuild.go:181`, `internal/recipe/validator.go:190`. None is in `internal/install`. |
| DESIGN-shell-env-activation.md:391 | "Tool bin directories are always under `$TSUKU_HOME/tools/` -- no arbitrary path injection" | **False** | Same as above. The word "always" is the load-bearing part and it is wrong. |
| DESIGN-shell-env-activation.md:393 | "The existing name validation in the install pipeline prevents path traversal characters in tool names" | **False** | Restatement of :388. No such validation exists. |
| DESIGN-shell-env-activation.md:401 | "The hook-env output is structured (export/unset statements with validated values)" | **False** | `FormatExports` (`internal/shellenv/activate.go:117-155`) emits eight `fmt.Fprintf` lines using `%q`. Go's `%q` is `strconv.Quote`: it escapes `"`, `\`, and non-printables, and does **not** escape `$` or backtick. Verified: `%q` of `$(id)` is `"$(id)"`, of `` `id` `` is `` "`id`" ``, of `$HOME` is `"$HOME"`. Emitted as `export PATH="$(id)"` and `eval`'d, `id` runs. No value is validated anywhere before reaching `FormatExports`. |
| DESIGN-shell-env-activation.md:411 | "`TSUKU_CEILING_PATHS` prevents traversal into untrusted parent directories" | **Overstated** | `isCeiling` (`internal/project/config.go:141`) is exact-match set membership. `$HOME` stops the walk only when the walk passes through `$HOME` exactly. From `/tmp/x` the walk goes `/tmp/x` → `/tmp` → `/` and never sees `$HOME`. `TSUKU_CEILING_PATHS` prevents traversal only for ceilings the user configured in advance. This is the separately-owned `/`-walk defect; recorded here because it is a false control claim, not to fix it. |
| DESIGN-shell-env-activation.md:418 | "All paths constrained to $TSUKU_HOME/tools/, name validation" (Mitigation cell, severity Low) | **False, both halves** | As above. And the Residual Risk cell in the same row — "Tool names with unusual characters could construct unexpected paths" — describes the actual behaviour. |
| DESIGN-shell-env-activation.md:420 | "Structured output (export/unset only), subprocess invocation" (Mitigation cell, severity Low) | **False** | The residual-risk cell scopes the risk to "If tsuku binary is compromised" — but the values come from a `.tsuku.toml` in the working directory, not from the binary. The threat model in this row is wrong about where the untrusted input enters. |

Two structural things about this table are worth naming separately from the individual
falsehoods.

First, the residual-risk column of row 418 records the true behaviour one cell to the
right of the claim that it is mitigated. Whoever wrote the row knew, and the severity
rating swallowed it.

Second, row 420 gets the *entry point* wrong. It treats hook-env's output as trustworthy
because tsuku produced it. But hook-env's output is a function of `.tsuku.toml`, and
`.tsuku.toml` is the repository's file. The design's own "Untrusted Repository Config"
section (:404-412) correctly identifies `.tsuku.toml` as the untrusted input — and then
the summary table forgets it two rows later.

#### The same false-record shape in two other current designs

The activation row is **not** the only false claim. The same shape — cite a guard that
exists somewhere, assert it covers this sink — appears in two more current designs about
path construction.

| Doc:line | Exact claim | True? | What the code does |
|---|---|---|---|
| DESIGN-org-scoped-project-config.md:270 | "The `parseDistributedName` function already rejects keys containing `..`." | **Misleading** | `parseDistributedName` (`cmd/tsuku/install_distributed.go:37`) returns **nil** for a `..` key, and its own doc comment (`:35-36`) says such names "fall through to the regular recipe lookup path". In `runProjectInstall` (`cmd/tsuku/install_project.go:73-79`) a nil return sets `entry.Distributed = nil` and the full traversal string is kept as `entry.Name`. It is a downgrade, not a rejection. |
| DESIGN-org-scoped-project-config.md:270 | "The binary index's `isValidRecipeName` continues to reject `/` in recipe names, so org-scoped names never reach the registry fetch path." | **False** | `IsValidRecipeName` guards `internal/index/rebuild.go:181` and `internal/distributed/cache.go:68`. It does **not** guard the registry fetch path: `Registry.FetchRecipe` (`internal/registry/registry.go:100`) checks only `name == ""`, and `cachePath` (`:90`) and `recipeURL` (`:81`) check only `name == ""` before `filepath.Join(r.CacheDir, firstLetter, name+".toml")` and URL interpolation. The conclusion ("never reach the registry fetch path") does not follow from the premise, and the premise is about a different function. |
| DESIGN-org-scoped-project-config.md:131 | "Used by both `runProjectInstall` and the resolver." (of `splitOrgKey`) | **False** | `SplitOrgKey` (`internal/project/orgkey.go:16`) has exactly one non-test caller: `internal/project/resolver.go:35`, reached only from `cmd/tsuku/cmd_run.go:100`. `runProjectInstall` uses `parseDistributedName` instead — a second, separate parser for the same key format, in a different package, with different behaviour on rejection. The design describes a single choke point; the code has two parsers and only one consumer of the validating one. |
| DESIGN-notification-routing.md:418 | "Recipe names are validated kebab-case when added to the registry, so the practical exposure is low" | **False** | `validateMetadata` (`internal/recipe/validator.go:271-282`) errors only on a space in the name and *warns* (not errors) on uppercase. There is no charset check on `metadata.name`. The name that reaches `WriteNotice` is the state key, not the validated recipe metadata, in any case. |
| DESIGN-background-update-checks.md:70 | "Tool names are kebab-case and filesystem-safe (no hashing needed, unlike the version cache's SHA256 approach...)" | **Unenforced assumption** | Stated as a key assumption and used to justify `filepath.Join(homeDir, "cache", "updates")` + `<toolname>.json` (`internal/updates/cache.go:180`, `:19`). Nothing enforces it. The 1449 recipes in `recipes/` do all match `^[a-z0-9][a-z0-9._-]*$`, so the assumption holds for the curated registry — but not for `.tsuku.toml` keys or distributed recipes, which is precisely where the untrusted input is. |

#### Claims that are true (the useful contrast)

Three claims about these controls check out, and each is more useful to the implementer
than the false ones, because each names an in-tree reference implementation.

- **DESIGN-binary-index.md:648-652** — "Before any name is passed to URL or path
  construction, it is validated to reject names containing `/`, `..`, or null bytes."
  True at that sink: `internal/index/rebuild.go:181` calls `recipe.IsValidRecipeName`
  before `GetCached`/`FetchRecipe`. Note the scoping: the claim is about `ListAll()`'s
  consumers, and it is true there. The activation design makes the same-shaped claim
  without the call.

- **DESIGN-shell-d-lifecycle.md** (Security Considerations) — "`set_env` values are
  attacker-influenced if a recipe is. Values are validated after placeholder
  substitution — no newlines, names constrained to an identifier pattern — and
  single-quoted with POSIX escaping, so shell metacharacters round-trip as data rather
  than executing." **True.** `shellQuote` at `internal/actions/set_env.go:252` is
  `"'" + strings.ReplaceAll(value, "'", `'\''`) + "'"`, used at `:176`;
  `validateEnvName` (`:230`) and `validateEnvValue` (`:243`) do the rest. A correct
  POSIX shell quoter already exists in this repository, one package away from
  `FormatExports`. It is unexported.

- **DESIGN-extract-symlink-escape.md** and `internal/install/symlink.go:46-67` — the
  containment check appends the separator before the prefix comparison, with the comment
  "e.g., /home/user/.tsuku/tools-malicious should not match /home/user/.tsuku/tools".
  The correct form of the sibling-prefix check also already exists in-tree.

The most striking true claim is in a design about a *different* feature:
**DESIGN-nvm-data-root.md:540** — "`filepath.Join` cleans before it returns, so
`../../../etc/passwd` escapes silently." That is exactly the fact
DESIGN-shell-env-activation.md:388 asserts the opposite of. The project's own written
record already contains the correct reasoning about `filepath.Join`; the activation
design was written as if it did not.

#### User-facing documentation

Swept `README.md`, `docs/guides/shell-integration.md`, `docs/GUIDE-shell-env-customization.md`,
`docs/ENVIRONMENT.md`, `website/`, and `plugins/`.

**No user-facing document claims tool names are validated, restricted, or constrained.**
`docs/guides/shell-integration.md:35` says only "Each entry maps a recipe name to a
version string" and documents no charset. `plugins/tsuku-recipes/AGENTS.md:40` says
recipe metadata name is "kebab-case", which is guidance to recipe authors, not a claim
about enforcement on config keys. `docs/guides/GUIDE-plan-based-installation.md:46` has a
heading "Tool Name Validation" that turns out to be about checking a plan file matches
the named tool — not a character control, not a false claim.

One user-facing false claim does exist, and it is about path construction:
`docs/guides/shell-integration.md:105` — "It won't look above `$HOME`." and `:74` —
"walking up from your current directory (stopping at `$HOME`)". Both are false for any
working directory not under `$HOME`, per `isCeiling`'s exact-match semantics. This
belongs to the separately-owned `/`-walk defect; flagging it so whoever fixes that
defect also fixes the guide.

So the *source* of the belief that name validation exists is not user documentation. It
is the design record citing itself: DESIGN-shell-env-activation.md:388 cites "the install
pipeline's name validation", DESIGN-org-scoped-project-config.md:270 cites
`isValidRecipeName`, DESIGN-notification-routing.md:418 cites "validated kebab-case when
added to the registry". Each cites a control in a neighbouring subsystem. None checked
whether the call happens on the path it is describing.

### Part 2 — the test record

#### 1. Does any existing test claim to cover tool-name handling or path construction?

`internal/config/config_test.go:81-99` is the whole of it:

```go
func TestToolDir(t *testing.T) {
	cfg := &Config{ToolsDir: "/home/user/.tsuku/tools"}
	got := cfg.ToolDir("golang", "1.21.0")
	want := "/home/user/.tsuku/tools/golang-1.21.0"
```

Two tests, one input each, both `("golang", "1.21.0")`. Nothing about traversal,
metacharacters, or containment.

The nearest thing to a traversal test in this area is
`internal/project/orgkey_test.go:58-77`:

```go
{ name: "path traversal rejected",  key: "../etc/passwd", wantErr: true },
{ name: "path traversal in middle", key: "myorg/../other", wantErr: true },
```

It tests a function that is not on the activation path.

Across `internal/shellenv/*_test.go`, `internal/project/*_test.go`,
`internal/config/config_test.go`, and `cmd/tsuku/shell_test.go`, **every tool name used
in every fixture is `go`, `node`, `python`, `jq`, `ripgrep`, `golang`, or
`tsukumogami/koto`.** No fixture anywhere contains `..`, a metacharacter, or a `/` in a
bare name. Grep for `../` or `$(` in those files returns only
`internal/shellenv/cache_test.go:453`, which is shell.d file *content* passed through
verbatim, not a value going through `FormatExports`.

#### 2. Does any test cover `FormatExports` output, and does it assert safety?

Six tests: `TestFormatExports_Bash` (:211), `_Zsh` (:232), `_Fish` (:248), `_Nil` (:273),
`_DeactivationBash` (:370), `_DeactivationFish` (:390).

They assert output *shape*, never safety, and two of them assert the exact wrong output:

```go
// activate_test.go:221
if !strings.Contains(output, `export PATH="/tools/go-1.22/bin:/usr/bin"`) {
// activate_test.go:378
if !strings.Contains(output, `export PATH="/original/bin:/usr/bin"`) {
// activate_test.go:398
if !strings.Contains(output, `set -gx PATH "/original/bin:/usr/bin"`) {
```

These are the double-quoted forms `%q` produces. A correct POSIX single-quoting fix makes
all three fail. **The test record does not merely fail to catch the bug; it pins it.** An
implementer whose working constraint is "don't break the tests" is steered back to a
double-quoting scheme. These assertions must be rewritten as part of the fix, and the
report of "3 tests changed" should not be read as regression.

The remaining assertions are prefix checks — `strings.Contains(output, "export PATH=")`,
`strings.Contains(output, "set -gx PATH")` — which are indifferent to quoting entirely.

Every fixture value is `/tools/go-1.22/bin:/usr/bin`, `/home/user/project`, `/usr/bin`,
`/original/bin:/usr/bin`. Characters used: letters, digits, `/`, `-`, `.`, `:`. Not one
metacharacter in the entire file. This is the "test on a value with no metacharacters at
all" case, six times over.

#### 3. Which assertions are illustrative rather than discriminating?

The version-sorting failure the brief describes — fixtures chosen to explain a mechanism,
whose properties cancel against the wrong implementation — is present here in two
distinct forms.

**Form A: the oracle is computed by the code under test.**
`TestComputeActivation_ProjectFound` (:70-79):

```go
goBin := cfg.ToolBinDir("go", "1.22")
if !strings.Contains(result.PATH, goBin) {
```

`ComputeActivation` builds `result.PATH` by calling `cfg.ToolBinDir(name, req.Version)`
(`activate.go:90`). The assertion calls the same function with the same arguments and
checks the answers match. Any implementation of `ToolBinDir` — including one that
traverses out of `$TSUKU_HOME` — satisfies this. The setup helper compounds it:
`setupProject` (`:29`) creates the directory the `os.Stat` check will look for by calling
`cfg.ToolBinDir` too, so the *existence precondition* is also satisfied by construction.
Same pattern at `:150-153`, `:334-368`, and `cmd/tsuku/shell_test.go:67-70`. Five
assertions that cannot distinguish a correct `ToolBinDir` from a traversing one.

**Form B: the assertion is on a property the wrong implementation happens to have.**
`if !strings.HasSuffix(result.PATH, ":/usr/bin:/bin")` (:79) checks the *shape* of the
concatenation. Correct and wrong path resolution both produce a colon-joined string
ending in the base PATH. `TestFormatExports_Fish`'s `if strings.Contains(output,
"export")` (:269) checks a keyword, which no quoting change affects.

**A third instance, in the guard that does exist.** `TestSplitOrgKey`
(`orgkey_test.go:6`) has two traversal fixtures. Both contain a `/`. But `SplitOrgKey`'s
first statement is `if !strings.Contains(key, "/") { return "", key, false, nil }` — the
`..` check at `orgkey.go:22` sits *after* that early return. So `SplitOrgKey("..")`
returns bare `".."` with `err == nil`, and no fixture would notice, because the test
author reached for `../etc/passwd` — a shape that carries a slash along with the `..`,
and the slash is what routes it to the check. Both halves of the traversal are present in
both fixtures, so the test cannot tell which half did the work.

Worse, the one consumer discards the error anyway
(`internal/project/resolver.go:35-39`): `if err != nil || !isOrg { continue }`. A
malformed key is silently skipped. The control is not just one consumer deep — its one
consumer swallows it.

#### 4. Fixtures that WOULD discriminate

Two groups. Each fixture is paired with the specific wrong implementation it rules out.
All are cheap; none needs new infrastructure.

##### Group A — the name validator

Where it goes: the head of the `for _, name := range toolNames` loop in
`ComputeActivation` (`internal/shellenv/activate.go:78`), with the *derived bare name*,
not the raw key.

| # | Fixture (tool name / config key) | Expected | Wrong implementation it catches |
|---|---|---|---|
| A1 | `../../../../tmp/evil` | reject | The baseline. Rejects nothing today. |
| A2 | `a/../../b` | reject | "Reject only the literal string `..`" — `name == ".."` is false here, and `filepath.Join(tools, "a/../../b-1.0")` is `tools/../b-1.0`, one level out. |
| A3 | `..` (alone) | reject | "Call `filepath.Clean(name)` and compare to `name`" — `Clean("..")` is `".."`, so the comparison passes and `..` is accepted. It is defanged at *this* sink by the `-version` suffix, but the validator is meant to be reusable and the next sink (`internal/updates/cache.go`, `internal/notices/notices.go`) has no suffix. |
| A4 | `../tools-evil/x` with `ToolsDir = <home>/.tsuku/tools` | reject | "Join, then check the result has prefix `$TSUKU_HOME/tools`" — the join lands on `<home>/.tsuku/tools-evil/x-1.0`, which passes `strings.HasPrefix`. The correct form (append the separator first) already exists at `internal/install/symlink.go:60-64`, and the fixture pattern already exists at `internal/install/symlink_test.go:199` as `tools-malicious`. Lift both. |
| A5 | `llama.cpp` | **accept** | "Reject any `.`" or "reject any name that is not `^[a-z0-9-]+$`". This is a real recipe: `recipes/l/llama.cpp.toml`. Of 1449 recipes it is the only name containing a dot, which is exactly why an over-strict validator ships green. |
| A6 | `tsukumogami/koto` as a config key | **accept**, resolving to bare `koto` | "Validate the raw config key" — org-scoped keys contain `/`, so `recipe.IsValidRecipeName` applied to the key rejects every legitimate org-scoped entry. This is the fixture that forces split-then-validate rather than validate-the-key. |
| A7 | `tsukumogami/registry:mytool@2.0.0` as a config key | **accept**, resolving to bare `mytool` | Same as A6 for the fully-qualified form; also catches "strip only `@version`" and "split on the first `/` and use the remainder". |
| A8 | `evil/../../../tmp/x:tool` as a config key | reject | Catches "validate the bare name only" — the *source* half also reaches path construction downstream (`internal/distributed/cache.go` `repoDir`). Both halves need checking. |
| A9 | Name containing a null byte (`"go\x00/evil"`) | reject | Catches a regex-based validator anchored with `$` rather than `\z` in a language where that matters, and catches "check `strings.Contains(name, "/")`" implementations that stop at the first NUL when the value later reaches a syscall. `IsValidRecipeName` already has this check — a new validator that forgets it is a regression against the existing one. |
| A10 | A name that passes the validator but whose bin dir does not exist | tool skipped, no PATH entry, no error | Guards the *other* direction: a validator that rejects too much would show up as tools silently vanishing from PATH, which is indistinguishable from the existing skip behaviour. This fixture pins skip-vs-reject as different outcomes. |
| A11 | Version `../../../etc` with a legitimate name | reject | The *version* component is also unvalidated at this sink. `ValidateVersionString` exists (`internal/install/state.go:413`) and is called from `Activate` and `RemoveVersion` — never from `internal/shellenv`, and never from `Manager.Install`. The brief's premise that the version half is guarded holds for the install-path sinks, not for this one. Half a fix here leaves the same hole through the other component. |

The single most valuable structural point: **A6 and A1 together are unsatisfiable by any
validator applied to the raw key.** Whichever fixture the implementer writes first, the
other forces the split-then-validate shape. Write both or the shape is not pinned.

##### Group B — the shell quoter

Where it goes: replacing all eight `%q` sites in `FormatExports`
(`internal/shellenv/activate.go:134,138,146,147,148,150,151,152`).

The repository already contains the correct answer and the correct test. `shellQuote`
(`internal/actions/set_env.go:252`) is a POSIX single-quote escaper.
`TestSetEnvAction_QuotesValues` (`internal/actions/set_env_test.go:317-355`) sources the
emitted script in a real bash, asserts the values round-trip, **and** asserts no side
effect:

```go
{"name": "WITH_META", "value": "$(touch pwned);`id`"},
...
want := "a b c|it's|$(touch pwned);`id`"
if string(out) != want { ... }
if _, err := os.Stat(filepath.Join(home, "pwned")); !os.IsNotExist(err) {
    t.Error("command substitution in a value was executed by the shell")
}
```

That is the strongest form the brief asks about, and it exists, twenty lines long, in
this tree. It should be copied, not reinvented.

| # | Fixture (value emitted through `FormatExports`) | Assertion | Wrong implementation it catches |
|---|---|---|---|
| B1 | `$(touch pwned)` in `Dir`, bash | eval in `bash --norc --noprofile`, then `os.Stat` on `pwned` must be `IsNotExist`; and `$_TSUKU_DIR` must read back byte-identical | `%q`. Also catches "escape `$` by hand inside double quotes" if the escaping is incomplete. The `os.Stat` half is what makes it a safety assertion rather than a string assertion. |
| B2 | `` `id` `` (backtick form) | same | "Escape `$` but not the backtick" — the exact case the brief names. A quoter written by patching `%q` to also escape `$` passes B1 and fails B2. |
| B3 | `$HOME` | reads back as the four literal characters `$HOME`, not the expansion | Catches a quoter that escapes command substitution but leaves parameter expansion. Silent value corruption even without an attacker. |
| B4 | `it's a path` (embedded single quote) | reads back byte-identical | Catches the naive `"'" + value + "'"` single-quote wrapper, which terminates the quote early and turns the rest into shell code. This is the failure mode *introduced by* the obvious fix. |
| B5 | Same values through the **fish** branch, evaluated in `fish -c` | reads back byte-identical, no side effect | The important one. Fish's rules differ in both directions: inside double quotes fish expands `$var` **and** `$(cmd)` (3.4+), so B1/B3 are live there too; inside single quotes fish recognises only `\'` and `\\`, so the bash `'\''` idiom **does not work** — a shared POSIX quoter applied to the fish branch is a new bug that B1-B4 all pass. Without B5, "we fixed the quoter" means "we fixed bash". |
| B6 | A value containing a literal newline | reads back with a real newline | `%q` renders a newline as the two characters `\` `n`, which bash inside double quotes leaves as `\n`. Not a safety bug, but proof that `%q` neither quotes safely nor round-trips, and it discriminates against any escape-based (rather than quote-based) fix. |
| B7 | A plain value with no metacharacters, e.g. `/usr/bin:/bin` | reads back byte-identical — assert on the **round-trip**, never on the emitted literal | Catches the regression the current tests embody: an assertion on the exact output string is satisfied by whatever implementation generated the expectation. Stating this as a rule for the new tests is as important as the fixtures. |
| B8 | Deactivation path, both shells, with a metacharacter `PrevPath` | same as B1/B5 | The deactivation branch (`activate.go:127-141`) is a separate `switch` with its own three `Fprintf` calls. A fix applied only to the activation branch passes every activation fixture. |
| B9 | The `_TSUKU_STATE_STAMP` value the sibling chain (#2554) adds | goes through the quoter | Not a test of today's code — a test the quoter's *shape* should make natural. If the quoter is a function callers must remember to call, this fixture is the only thing standing between the fix and a fourth `%q` landing one issue later. Consider a typed `shellValue` or an emit helper that takes the value rather than a format string, so the awkward thing is bypassing it. |

##### Is evaluate-in-a-real-shell realistic here?

Yes for bash, with in-tree precedent. `internal/actions/set_env_test.go:57` and `:320`
and `cmd/tsuku/shelld_lifecycle_test.go:77` all do `exec.LookPath("bash")` with
`t.Skip("bash not available")` and run `bash --norc --noprofile -c`. CI runs on
`ubuntu-latest` and `macos-latest` (`.github/workflows/test.yml`), so bash is always
present and the test always actually runs.

**Fish and zsh are not installed in CI.** Grepping all 60 workflow files for `fish` or
`zsh` returns nothing, and the Makefile has no shell-provisioning target. A
`LookPath("fish")`-guarded test would skip on every CI run and would be green whether or
not anyone ever ran it locally. That is worse than no test, because it reads as coverage.

Realistic recommendation, in order:

1. **B1-B4, B6-B8 in bash**: write them exactly like `TestSetEnvAction_QuotesValues`.
   Real coverage, runs on every CI job, zero new infrastructure.
2. **B5 in fish**: write the same test, guarded by `LookPath("fish")`, **and** add
   `sudo apt-get install -y fish` to the Go test job in `.github/workflows/test.yml` so
   it is not permanently skipped. This is a two-line workflow change. Without it, do not
   write the test — write a table-driven unit test of the fish quoter against
   hand-derived expected fish escapes instead, and say in a comment that it is a
   second-best because no fish is available.
3. **zsh**: the bash branch and the zsh branch of `FormatExports` are the same code path
   (`default:` in both switches). A zsh test adds nothing the bash test does not already
   cover. Skip it, and note in the test file *why*, so the next reader does not read the
   absence as an oversight.

---

## Implications

The design record is why nobody looked. Three separate current designs assert that a
path-construction control covers a sink it does not cover, and in each case the cited
control is real — it just lives somewhere else. A reviewer who follows the citation finds
`IsValidRecipeName`, or `ValidateVersionString`, or `parseDistributedName`, confirms the
function exists and rejects `..`, and stops. The falsity is never in the existence of the
guard; it is always in the claim about *where it is called*. That is a class of false
record that survives review by construction, and it will keep producing defects until the
guards move to the choke points the designs already describe.

The test record is worse than empty. Three assertions pin the exact output of the wrong
quoter, so the fix must break them. Five assertions compute their expected value by
calling the function under test, so they hold for a traversing `ToolBinDir`. There is no
fixture anywhere in the activation surface containing a character outside `[a-z0-9./:-]`.
The suite reads as coverage — thirteen tests in `activate_test.go`, all passing — and
discriminates against neither defect.

The constructive news is that both correct implementations and both correct test forms
already exist in this repository, written by the same project for adjacent features:
`shellQuote` at `internal/actions/set_env.go:252`, the separator-appending containment
check at `internal/install/symlink.go:60`, the evaluate-and-assert-no-side-effect test at
`internal/actions/set_env_test.go:317`, and the `tools-malicious` sibling-prefix fixture
at `internal/install/symlink_test.go:199`. The fix is mostly a matter of exporting and
reusing what is there, and the strongest argument for a particular shape is that the
project already chose it once.

---

## Surprises

**The project already wrote down the correct fact about `filepath.Join`, in a different
design.** `DESIGN-nvm-data-root.md:540`: "`filepath.Join` cleans before it returns, so
`../../../etc/passwd` escapes silently." That is the precise negation of
`DESIGN-shell-env-activation.md:388`. Both are in `docs/designs/current/`. Both are
Current status. Nothing reconciles them.

**`SplitOrgKey`'s `..` check is unreachable for slash-free keys.** `orgkey.go:19` returns
early for any key without `/`, and the `..` rejection is at `:22`. `SplitOrgKey("..")`
returns `("", "..", false, nil)`. Both traversal fixtures in `orgkey_test.go` carry a
slash, so nothing notices. The guard the org-scoped design points to as the path-traversal
control has a hole its own tests are shaped not to find.

**The one consumer of that guard discards its error.** `resolver.go:37`:
`if err != nil || !isOrg { continue }`. The error is not merely unpropagated; a malformed
key is treated identically to a bare key and skipped.

**Two parsers for one key format.** `SplitOrgKey` (`internal/project`) and
`parseDistributedName` (`cmd/tsuku`) parse `owner/repo:recipe@version` with different
code and opposite failure behaviour: one returns an error, the other returns nil and lets
the caller keep the raw string. `DESIGN-org-scoped-project-config.md:131` says
`splitOrgKey` is "Used by both `runProjectInstall` and the resolver", which describes the
single-parser design that was intended. The implementation has two, and the design record
was never updated.

**`parseDistributedName`'s nil-on-traversal is documented as a feature.** Its comment
(`install_distributed.go:35-36`) says traversal names "fall through to the regular recipe
lookup path and produce a not-found error." That is a deliberate choice to *not* reject,
justified by an assumption about what happens downstream. In `runProjectInstall` the
downstream is `entry.Name` carrying the traversal string forward.

**The residual-risk column of DESIGN-shell-env-activation.md is more accurate than the
mitigation column, twice.** Row 418's residual risk names the real behaviour. Row 421's
residual risk ("User could inject malicious PATH via env var manipulation") is also
accurate about a real capability. The mitigation column is where the record goes wrong,
consistently. Whatever review process produced this table checked that every row had four
cells, not that column three implied column four.

---

## Open Questions

- **Where does the validator belong?** Every design that cites a name control cites one
  in a different package. Putting a fourth one at the head of `ComputeActivation`'s loop
  fixes activation and reproduces the pattern. The alternative — validating at
  `Config.ToolDir`, the sink all fourteen call sites share — changes a currently
  infallible signature to return an error, which is a much larger diff and touches
  `internal/install`, `internal/verify`, `internal/autoinstall`, and four `cmd/tsuku`
  files. That call is the design's to make, but it should be made explicitly rather than
  by default, because "add the check at the caller I am touching" is how the class got
  here.

- **Should `recipe.IsValidRecipeName` be the validator, or a new one?** It rejects `/`,
  which is right for a bare name and wrong for a raw key (A6). It does not reject
  absolute paths or control characters beyond NUL. Reusing it means the split-then-validate
  shape is mandatory; writing a new one risks a fourth definition of "valid name" in a
  tree that already has three (`IsValidRecipeName`, the `^[a-z0-9._-]+$` pattern in
  `validator.go:151`, and `validateNoticeName`).

- **Does the `..` hole in `SplitOrgKey` matter to any real caller today?** Its one
  consumer builds a lookup map and skips errors, so probably not — but the fix for this
  defect is likely to add a second consumer, and the hole should close before that
  happens rather than after.

- **Do the false claims get corrected, and by whom?** Correcting
  `DESIGN-shell-env-activation.md:388-420` is straightforward and belongs with this fix.
  `DESIGN-org-scoped-project-config.md:131` and `:270` and
  `DESIGN-notification-routing.md:418` describe other features and are not this change's
  to rewrite — but leaving them is leaving three more citations a future reviewer will
  follow and trust.

- **Is `internal/updates/cache.go` a live third sink?** `DESIGN-background-update-checks.md:70`
  assumes filesystem-safe tool names and joins them into a path with no suffix to defang
  a bare `..`. Whether an attacker-supplied name reaches it depends on whether the
  update-check path ever iterates `.tsuku.toml` keys rather than `state.json` keys. Not
  established here; it belongs to whoever is mapping path sinks.

---

## Summary

The line-418 row is not the only false claim: three current designs assert a
path-construction control that the code does not apply at the sink they describe
(activation's "install pipeline name validation" and "paths constrained to
$TSUKU_HOME/tools", the org-scoped design's "isValidRecipeName ... so org-scoped names
never reach the registry fetch path" and its claim that `splitOrgKey` is used by
`runProjectInstall`, and notification-routing's "recipe names are validated kebab-case"),
while the design record elsewhere already states the correct fact about `filepath.Join`
and the repository already contains a correct POSIX quoter, a correct containment check,
and a correct evaluate-in-bash safety test. The existing tests are not merely absent but
adverse: three assert the exact output of the wrong quoter so the fix must break them,
five compute their expected value by calling the function under test, and no fixture in
the entire activation surface contains a character outside `[a-z0-9./:-]`. The biggest
open question is whether the validator goes at the head of `ComputeActivation`'s loop —
fixing activation while reproducing the one-consumer-deep pattern that produced this
class — or at `Config.ToolDir`, the choke point all fourteen call sites already share.
