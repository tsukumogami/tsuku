<!-- decision:start id="data-root-guardrail" status="assumed" -->
### Decision: Guardrail against version-scoped data-root exports

**Context**

`recipes/n/nvm.toml` exports `NVM_DIR = "{install_dir}"`. `{install_dir}` expands
to `$TSUKU_HOME/tools/nvm-<version>`, so nvm's data root — every Node version the
user installed, their global npm packages, the `default` alias — lives in a
directory tsuku replaces on upgrade and eventually deletes. Generalized, the bug
class is *a recipe exporting a tool's data-root environment variable at a value
that is version-scoped*. The main fix repairs nvm. This decision is about whether
to also make the class unrepresentable, and in what shape.

The population is n=1. Of 1449 registry recipes, exactly one uses `set_env`
(`grep -rl set_env recipes/` → `recipes/n/nvm.toml`), one step, one variable, one
value. It is also the only `install_shell_init` user. No other TOML in the repo
uses `set_env` at all. So the rule costs nobody anything today, and its subject is
the recipe this design is already fixing.

Two facts about how validation is wired constrain everything downstream. First,
`--strict` erases the error/warning distinction in CI:
`cmd/tsuku/validate.go:139-141` sets `Valid = false` on any warning, and the
`Validate Recipes` job runs `tsuku validate --strict` over all 1449 registry
recipes plus 19 embedded ones (`.github/workflows/test.yml:435,446`). A warning
turns that job red exactly as an error does. Second, the validator has exactly
two non-test callers — `cmd/tsuku/validate.go:59` and
`cmd/tsuku/install_deps.go:281`. The install-path call is unconditional and does
not discriminate by source, so registry, `$TSUKU_HOME/recipes`-local, and embedded
recipes all get the full validator; errors abort the install
(`install_deps.go:295`) and warnings print and continue (`:305`). The sandbox
path does not validate at all: `runSandboxInstall`
(`cmd/tsuku/install_sandbox.go:56`) routes through `generateInstallPlan`, so the
curated nightly and `recipe-validation-core.yml` sandbox installs are untouched by
this rule at either severity.

**Assumptions**

- The denylist's contents are a judgement about which tools' variables designate
  data rather than program. That judgement is unfalsifiable today (n=1) and will
  be wrong at the margins. If it is wrong by omission, the rule silently does
  nothing for that variable — the same outcome as not shipping, with no new harm.
- No composite action decomposes into a `set_env` sub-step. Immaterial to the
  recommendation: a rule sited in `internal/recipe/validator.go` reads `r.Steps`
  (recipe source, pre-decomposition), so it is immune either way. It would matter
  only for the rejected `SetEnvAction.Preflight` siting.
- Locally-authored third-party recipes using `set_env` with a listed variable
  exist in numbers near zero. `set_env` is new, unused by 1448 of 1449 public
  recipes, and only became functional with the shell.d lifecycle work.
- The rule's phrasing is stated negatively (reject unstable values) so it does not
  depend on the outcome of decision 1. See Consequences for the coupling.

**Chosen: Ship it — a variable-name denylist, error severity, in
`internal/recipe/validator.go`**

Add a package-level set of environment variable names that name a tool's *data*
root, and reject a `set_env` step that exports one of them at a value that is not
stable — that is, a value containing `{install_dir}`, `{work_dir}`, or
`{version}`.

Starting list, all unambiguous data roots: `NVM_DIR`, `PYENV_ROOT`, `RBENV_ROOT`,
`GOENV_ROOT`, `NODENV_ROOT`, `PLENV_ROOT`, `ASDF_DATA_DIR`, `VOLTA_HOME`,
`SDKMAN_DIR`, `CARGO_HOME`, `RUSTUP_HOME`, `GOPATH`, `GOMODCACHE`, `GEM_HOME`,
`PNPM_HOME`, `BUN_INSTALL`, `DENO_DIR`, `NPM_CONFIG_PREFIX`, `PIPX_HOME`. The list
is data, not code; extending it is a one-line diff.

Siting: inside the `validateSteps` loop
(`internal/recipe/validator.go:492-559`), immediately after the existing
action-keyed `set_env` block at `:500-506` that rejects `set_env` in library
recipes. That block is the exact structural precedent — action-keyed,
recipe-context-inspecting, error-level — and `step`, `stepField`, and `r` are
already in scope. The rule reads `step.Params["vars"]` as raw pre-expansion
strings, which the validator already does elsewhere (`validatePathParam` at
`:661-676` explicitly detects unexpanded `{` placeholders). It must hand-assert
the `[]interface{}` of `map[string]interface{}` shape the way
`SetEnvAction.parseVars` does (`internal/actions/set_env.go:250-279`), because
`internal/recipe` cannot import `internal/actions` — the circular-import note at
`validator.go:130-131`.

The error message should teach, not just block: name the variable, say that it is
the tool's data root rather than its program directory, and point at the stable
surface this design establishes.

Rule and recipe fix must land in the **same PR**. A change to `validator.go` arms
the `Validate Recipes` job through the dedicated `validator` paths-filter
(`test.yml:392-394`), which then validates all 1449 recipes with `--strict` and
hits nvm. Splitting them across two PRs leaves main red, and the nightly
`schedule` run validates unconditionally (`test.yml:411`) so it would not stay
hidden.

**Rationale**

*Why ship at all.* The argument against — a rule with one subject cannot be
validated against real usage — is true, and it argues for keeping the rule narrow
and cheap, not for skipping it. The costs are asymmetric and not close. A false
positive costs a recipe author an error message they work around in one PR,
immediately, with a diagnostic naming the problem. A miss costs a user every
Node/Python/Ruby version they installed, silently, discovered weeks later, with
nothing to recover from. Beyond that, this design's whole output is a *convention*
for where tool data lives. A convention with no enforcement is a comment in a
design doc nobody reads; the validator rule is what makes it operative for the
second recipe, written by someone who was not in this conversation. And the
registry already has `pyenv`, `rustup`, and `asdf` — three tools one `set_env`
step away from this exact bug.

*Why a denylist rather than a structural rule.* Because there is no genuine
structural rule available. "Is this variable a data root?" is a fact about the
*tool*, not a property of the value's syntax — the value `{install_dir}` is
correct for `JAVA_HOME` and catastrophic for `NVM_DIR`, and nothing in the recipe
distinguishes them except the name. A denylist is therefore not a degraded
structural rule; it is the correct encoding of a fact that lives outside the
recipe. The evidence is concrete: the canonical documented example of `set_env` in
this project is literally `JAVA_HOME = "{install_dir}"`, in the action's own doc
comment (`internal/actions/set_env.go:92`) and in the recipe-author reference
(`plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md:453`),
and four existing validator tests use `{install_dir}` as their `set_env` fixture
value (`validator_test.go:1647,1677,1710,1733`). A blanket
"no `{install_dir}` in `set_env`" rule has zero false positives against today's
registry and guaranteed false positives against today's documentation and tests.

*Why error and not warning.* In CI they are the same thing — `--strict` collapses
them, and the `Validate Recipes` job is armed by the rule's own PR. The
distinction survives in exactly one place: whether `tsuku install <tool>` aborts
or prints a line and proceeds. Blocking is the proportionate response to "this
recipe will destroy the user's data on the next upgrade," and a warning printed
during an otherwise-successful install is precisely the notice nobody reads.
Shipping this as a warning would therefore pay the full CI cost and surrender the
only benefit that differs. The transition cost is nil because the nvm fix lands in
the same PR, and the sandbox nightlies do not run the validator at all.

*Why `validator.go` and not the alternatives.* `lint_test.go` is not a viable
host: it does not walk `recipes/`, and all of its tests are `testing.Short()`-
guarded while CI runs `go test -short ./...` on both PR (`test.yml:72`) and push
(`:70`) — nothing in that file executes in CI today. The detector-outside-the-
validator pattern (`DetectShadowedDeps`, `DetectHardcodedVersions`,
`DetectDownloadFileVersionMismatch`, wired from `cmd/tsuku/validate.go:76-118`) is
the established home for non-load-time checks, but those never reach the install
path, which is the coverage worth having. `SetEnvAction.Preflight` sees the same
literal value but puts recipe policy in the action layer and would trip
`{install_dir}` fixtures in four other test packages.

**Alternatives Considered**

- **Do not ship.** Legitimate, and the honest case for it is that a rule fitted to
  one example is a guess. Rejected on cost asymmetry: the rule is ~12 lines plus
  two tests, arms no expensive CI, and the failure it prevents is silent
  destruction of user data in a tool this registry already carries three
  candidates for.
- **Structural: flag any `{install_dir}` in a `set_env` value, with a schema
  opt-in for recipes that mean it.** Rejected on three counts. It forbids the only
  `set_env` example the project documents (`JAVA_HOME`), which is *correct* usage.
  It breaks four existing validator tests. And the opt-in requires a new field in
  `internal/recipe/types.go`, for which the schema has no precedent — there is no
  `allow_*`, `unsafe_*`, `override_*`, or `ignore_*` field anywhere in that file —
  and which drags `types.go` into the diff, arming the 30-minute
  `validate-golden-code.yml` job (that workflow's `paths:` filter includes
  `internal/recipe/types.go` at line 22 but not `validator.go`). An opt-in that
  every legitimate `*_HOME` recipe must set also degrades into reflex within three
  recipes.
- **Suffix heuristic: `*_ROOT`/`*_DIR` at `{install_dir}` is an error, `*_HOME` is
  fine.** Rejected as a denylist with worse precision and worse recall, dressed as
  structure. `VOLTA_HOME`, `SDKMAN_DIR`, `CARGO_HOME`, and `RUSTUP_HOME` are data
  roots ending in `_HOME`; `GOPATH` ends in neither suffix.
- **Same denylist, warning severity, wired from `cmd/tsuku/validate.go`.**
  Rejected: `--strict` means it pays the identical CI cost, and it gives up the
  install-path block, which is the only behaviour that differs.
- **`lint_test.go` as the host.** Rejected: it does not walk `recipes/` and is
  entirely `testing.Short()`-skipped in CI.

**Consequences**

*Files touched.* `internal/recipe/validator.go` (a `dataRootEnvVars` set plus a
~12-line block in `validateSteps` after `:506`); `internal/recipe/validator_test.go`
(two tests, ~45-60 lines — the harness needs no change, `set_env` is already in the
mock allowlist at `:150`, and the existing four `set_env` tests use
`TEST_LIB_HOME`/`TEST_TOOL_HOME`, which the denylist does not match);
`recipes/n/nvm.toml` (the main fix, same PR); and the `set_env` section of
`plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md`
(~`:435-490`) to document the rule. `internal/actions/set_env.go:92`'s `JAVA_HOME`
example stays valid and needs no edit.

*CI.* The change arms `Validate Recipes` (dedicated `validator` paths-filter,
`test.yml:392-394` — which is exactly the job you want re-running all 1449
recipes) and `Unit Tests`/`Lint Tests` via the `code` filter (`**/*.go`,
`test.yml:371-376`). It does **not** arm `validate-golden-code.yml`: neither
`validator.go` nor `lint_test.go` nor `recipes/**` appears in that workflow's
`paths:` list. It does not affect `curated-nightly.yml` or
`recipe-validation-core.yml`, which do sandbox installs that never call the
validator.

*Coverage.* The rule reaches `tsuku validate` and every `tsuku install`, across
all three loader sources — local, embedded, and registry
(`internal/recipe/loader.go:654-665`, `install_deps.go:281` with no source
discrimination). It does not reach the sandbox or plan-driven install paths.

*What gets harder.* Nothing today. Prospectively, a recipe author with a
legitimate reason to point a listed variable at a version-scoped path has no
escape hatch — the schema has no per-recipe suppression mechanism and no
comment-based one is possible, since TOML comments never reach the validator.
Every suppression mechanism in this repo is an external JSON exclusions file read
by a CI shell step, and none of those can exempt the install-path check. That is
acceptable at n=1 and is the honest reason to keep the list conservative; if a
legitimate case ever appears, removing the name from the list is a one-line diff.

*Coupling to decision 1.* The rule as phrased is negative — "a data-root variable
must not hold an unstable value" — which works regardless of what stable-path
surface decision 1 chooses. If decision 1 lands a per-tool stable placeholder
(e.g. a `{data_dir}` expanding to a tool-scoped stable directory), re-phrase the
rule positively: *a data-root variable's value must be rooted at that placeholder*.
That is strictly tighter — it also catches a raw literal path — and yields a
materially better error message, since the diagnostic can name the thing the
author should have written. Do not block this decision on that; ship the negative
phrasing and tighten it in the same PR if the placeholder exists.
<!-- decision:end -->
