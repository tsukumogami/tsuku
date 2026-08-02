# Lead: What can the existing test infrastructure express for a multi-version shell.d lifecycle test, and where are its blind spots?

All paths are relative to the worktree root
`/home/dgazineu/dev/niwaw/tsuku/tsuku+shell_d_lifecycle-2743e957/public/tsuku/.claude/worktrees/shell-d-lifecycle`.

## Findings

### 1. Isolated `$TSUKU_HOME` helpers

There is exactly one shared helper package, `internal/testutil`, with two relevant
exported functions:

- `func TempDir(t *testing.T) (string, func())` — `internal/testutil/testutil.go:13`.
  Wraps `os.MkdirTemp` and returns an explicit cleanup closure (not `t.TempDir`;
  it predates the idiom). Call site: `internal/install/manager_test.go:1516`.
- `func NewTestConfig(t *testing.T) (*config.Config, func())` — `internal/testutil/testutil.go:23`.
  Builds a fully populated `*config.Config` rooted at a fresh temp dir and
  `MkdirAll`s twelve subdirectories (`tools/`, `tools/current/`, `recipes/`,
  `registry/`, `libs/`, `apps/`, `cache/` and its four children, `config.toml`).
  Call site: `internal/install/manager_test.go:1492`.

This is the closest thing to a "fake `$TSUKU_HOME`" and it is reusable from any
package. **Blind spot:** `NewTestConfig` does *not* create `share/`,
`share/shell.d/`, or write `$TSUKU_HOME/env`. Every shell.d test today makes
those itself — see `internal/install/remove_test.go:482-484` and
`internal/shellenv/doctor_test.go` (`writeTestFile` local helper). And
`cfg.HomeDir` is a `os.MkdirTemp` path, so `$TSUKU_HOME` in a subshell must be
passed explicitly; there is no ambient env-var manipulation helper.

Other packages roll their own. The PR #2442 branch adds a local one,
`setEnvHome(t, name, version) (string, *ExecutionContext)` in
`internal/actions/set_env_test.go`, which builds `home/tools/<name>-<version>`
and an `ExecutionContext` wired to it. It is unexported and package-local to
`internal/actions`.

### 2. Installing a tool without network

Three distinct capabilities exist, at different altitudes:

**(a) `Manager.Install` needs no network at all.** `Manager.InstallWithOptions`
(`internal/install/manager.go:114`) takes a `workDir` and copies
`<workDir>/.install/...` into `tools/<name>-<version>/`. It never resolves a
version, never fetches a recipe, never runs an action.
`TestInstallWithOptions_PreservesExistingVersions`
(`internal/install/manager_test.go:1491-1571`) is the exact precedent for a
two-version setup: it seeds `1.0.0` directly into state via
`mgr.state.UpdateTool`, hand-builds a `workDir/.install/bin/mytool`, then calls
`InstallWithOptions(testCtx(), "mytool", "2.0.0", workDir, opts)` and asserts
both versions coexist. `testCtx()` (`internal/install/manager_test.go:23`)
supplies the `installevents.SourceManual` the bus requires.

**(b) Actions can be executed directly against a synthetic `ExecutionContext`.**
This is what PR #2442 does. `InstallShellInitAction.Execute` derives
`$TSUKU_HOME` as `filepath.Dir(ctx.ToolsDir)` (`internal/actions/shell_init.go:107-109`)
and writes `share/shell.d/<target>.<shell>`, recording a `CleanupAction` at
`internal/actions/shell_init.go:141-150`. No network, no download.

**(c) Full offline recipes exist as fixtures.** `testdata/recipes/tool-a.toml`
is a complete recipe whose only step is `run_command` writing a shell script
into `.install/bin/`. `testdata/recipes/` has 24 such files. The functional
harness resolves `TSUKU_REGISTRY_URL` to the repo root
(`test/functional/suite_test.go` via `iRun`, `test/functional/steps_test.go:39`),
so local recipes are found without hitting the network — but any recipe with a
`download`/`download_archive` step still fetches from upstream.

For HTTP-level fakes there is `httptest.NewTLSServer` precedent in
`internal/actions/download_test.go:68,232,522` and
`internal/executor/plan_generator_test.go:1209,1285,1464,1615`. There is **no**
fake action registry and no action-level dependency injection seam — actions are
concrete structs resolved by name.

### 3. Existing real-shell tests

**Yes, two of them, and they are the most important finding here.**

**(a) The functional suite already sources `$TSUKU_HOME/env` in bash.**
`test/functional/suite_test.go:168` registers:

```
ctx.Step(`^I source home file "([^"]*)" and can run "([^"]*)"$`, iSourceHomeFileAndCanRun)
```

Implementation at `test/functional/steps_test.go:291-308`:

```go
script := fmt.Sprintf(`. "%s" && %s`, fullPath, command)
cmd := exec.Command("bash", "-c", script)
cmd.Env = append(os.Environ(), "TSUKU_HOME="+state.homeDir, "TSUKU_NO_TELEMETRY=1")
out, err := cmd.CombinedOutput()
if err != nil { return ctx, fmt.Errorf(...) }
```

Used by four scenarios in `test/functional/features/environment.feature:104,118,119,127`,
e.g. *"shell function from tool init script is available after sourcing env"*.

**Blind spot, and it is the decisive one:** this step asserts only that the
command exits zero. It discards `out`. There is no step of the form *"and the
output is X"* for a sourced-env command. To assert "`NVM_DIR` points at
`tools/nvm-0.40.5`" you either add a new step, or smuggle the assertion into the
command string — and the Gherkin regex is `"([^"]*)"`, so the command may not
contain a double quote. An unquoted `test $NVM_DIR = $TSUKU_HOME/tools/nvm-0.40.5`
would work (the subshell gets `TSUKU_HOME`, and `homeDir` is the deterministic
`<repoRoot>/.tsuku-test`), but it is brittle and silently passes if `NVM_DIR` is
empty *and* the right-hand side happens to be empty. A new step that captures
output is the honest option.

**(b) PR #2442's `TestSetEnvAction_ReachesUserShell`** (`internal/actions/set_env_test.go`
on branch `origin/fix/2439-set-env-exports`). The technique:

```go
bashPath, err := exec.LookPath("bash")
if err != nil { t.Skip("bash not available") }
home, ctx := setEnvHome(t, "nvm", "0.40.6")
os.WriteFile(filepath.Join(home, "env"), []byte(config.EnvFileContent), 0644)
// ... SetEnvAction.Execute, then InstallShellInitAction.Execute ...
shellenv.RebuildShellCache(home, "bash")
cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c",
    `. "$TSUKU_HOME/env"; printf %s "${`+varName+`-}"`)
cmd.Env = []string{"TSUKU_HOME=" + home, "HOME=" + home, "PATH=/usr/bin:/bin"}
out, _ := cmd.Output()
```

Three things about it are worth copying: the `--norc --noprofile` isolation, the
hermetic three-entry `cmd.Env` (no `os.Environ()` leakage), and the
`printf %s "${VAR-}"` read-back which distinguishes unset from empty. It also
plants a decoy init script that records `NVM_DIR_AT_INIT` so ordering bugs
cannot hide behind a later overwrite — the same trick generalizes directly to
"which version's content is in the file".

**Does it generalize to multi-version?** Partially. The technique (write `env`,
run actions, rebuild cache, read back in bash) generalizes cleanly. What does
*not* generalize is the harness: `setEnvHome` fabricates an `ExecutionContext`
by hand and never touches `install.Manager` or `state.json`. A multi-version
lifecycle test needs `Manager.Install` twice, `Manager.RemoveVersion` once, and
the state-driven `CleanupActions` bookkeeping — none of which is reachable from
`internal/actions`.

### 4. Where a multi-version lifecycle test can live

The import graph forbids the obvious answer.

Verified with `go list -deps`:

- `internal/install` imports `internal/shellenv` (`internal/install/remove.go:400`,
  `internal/install/update.go:58`) — so `shellenv.RebuildShellCache` is reachable
  from an `internal/install` test with no new edge.
- `internal/actions` → `internal/version` → `internal/install`
  (`internal/version/resolve.go:7` imports `internal/install`). **So
  `internal/actions` transitively depends on `internal/install`.**
- Every test file in `internal/install` today declares `package install`
  (13/13). An internal test file in that package therefore **cannot** import
  `internal/actions` — that is a cycle.

Three viable homes, in order of preference:

1. **`cmd/tsuku`, `package main`.** All 32 test files there are `package main`.
   `cmd/tsuku` already imports `internal/install` (`cmd/tsuku/activate.go`,
   `cmd/tsuku/cmd_rollback.go`), `internal/shellenv` (`cmd/tsuku/doctor.go`,
   `cmd/tsuku/install_deps.go`), `internal/executor`, and `internal/actions`.
   It is also where the real orchestration lives — `cmd/tsuku/plan_install.go:117-137`
   is the only place that runs the post-install phase and then rebuilds the shell
   cache. A test here can exercise `Manager.Install` ×2 → `RemoveVersion` →
   `RebuildShellCache` → `bash -c`. Existing precedent: `cmd/tsuku/doctor_test.go`
   already sets `TSUKU_HOME`.
2. **An external test package `package install_test` in `internal/install/`.**
   Go permits an external test package to import packages that import the package
   under test, so `install_test` may import `internal/actions`. This works but
   would be the first `_test`-suffixed package in that directory, and it can only
   reach exported API (`Manager.Install`, `Activate`, `Rollback`, `RemoveVersion`
   are all exported; `mgr.state` is not, so state seeding must go through
   `GetState()` / `RecordCleanup`).
3. **`test/functional`** (godog). Highest fidelity — real binary, real shell —
   but requires network for any real recipe and needs a new step definition for
   output assertions (§3a). `remove.feature`, `rollback.feature`, and
   `versions.feature` already exist as scenario homes.

There is no dedicated integration-test *package*. Build-tag conventions in use:
`//go:build integration` (`integration_test.go:1`, `internal/llm/*_test.go`) and
`//go:build e2e` (`internal/updates/e2e_test.go:1`, `internal/llm/local_e2e_test.go:1`).
`testdata/` convention: `testdata/recipes/` for fixture recipes,
`testdata/golden/plans/{embedded,<letter>}/<recipe>/` for golden plans.

### 5. Shell gating and shell availability in CI

**Bash:** available on `ubuntu-latest`, which is where `unit-tests`
(`.github/workflows/test.yml:42`), `lint-tests` (`:100`), and `functional-tests`
(`:122`) all run. The existing functional step at `test/functional/steps_test.go:296`
calls `exec.Command("bash", ...)` with no availability guard at all, so the repo
already assumes bash unconditionally in that suite. PR #2442 is more careful and
uses `exec.LookPath("bash")` + `t.Skip`.

**Zsh and fish:** no Go code anywhere does `exec.LookPath("zsh")` or
`LookPath("fish")` directly. The one place a shell binary is invoked outside
tests is `checkShellSyntax` (`internal/shellenv/doctor.go:178-195`), which runs
`<shell> -n <file>` and **returns nil when `LookPath` fails** — i.e. the syntax
check silently no-ops when zsh is absent. `internal/shellenv/doctor_test.go:30,208`
writes `.zsh` files but only asserts on classification, never on execution. So:
there are no executing zsh or fish tests today, and I could not verify from the
repo whether zsh is present on the runner image — the existing code is written to
degrade silently either way, which is the precedent to follow.

Independently, `$TSUKU_HOME/env` itself only supports bash and zsh
(`internal/config/config.go:446-465`): it selects the cache file on
`$BASH_VERSION` / `$ZSH_VERSION` and sources nothing otherwise. A `sh -c` or
`fish` subshell sourcing `env` is a no-op. Fish cannot be tested through this
path at all.

**`testing.Short()` gating is a trap.** The unit-test job runs
`go test -short ./...` on PRs (`.github/workflows/test.yml:72`) **and**
`go test -short -race -coverprofile=coverage.out ./...` on push to main (`:70`).
`-short` is always set. Any test guarded by `if testing.Short() { t.Skip() }`
**never runs in CI**. That is deliberate for `lint_test.go:16,23,40,47,54,98`
(which is instead invoked by name in the `lint-tests` job, `.github/workflows/test.yml:112`)
and for the container/LLM suites, but it means a shell.d lifecycle test must
**not** be short-gated or it is dead weight.

Also note `.github/workflows/test.yml:77-86`, "Check for test artifacts": the job
fails if `git status --porcelain` is non-empty after the run. Tests must write
only to temp dirs.

### 6. CI jobs on a PR

From `.github/workflows/test.yml`, all gated on a `matrix` job
(`.github/workflows/test.yml` job `matrix`) that runs `dorny/paths-filter`:

| Job | Trigger | Notes |
|---|---|---|
| `matrix` | always | computes `code`/`rust`/`llm`/`recipes`/`functional` flags |
| `unit-tests` | `code == true` | `go test -short ./...`, plus go.mod-tidy check and the artifact check |
| `lint-tests` | `code == true` | `go test -run 'Test(GolangCILint\|GoFmt\|GoModTidy\|GoVet\|Govulncheck)' .` — this is where golangci-lint actually runs |
| `functional-tests` | `code == true` | `make test-functional-critical` on PRs (only `@critical`-tagged scenarios), full `make test-functional` on push to main |
| `integration-linux` / `integration-macos` | `code \|\| recipes` | 60-min timeout, real installs over the network — the slow ones |
| `validate-recipes` | recipe/validator changes or schedule | |
| `rust-test`, `llm-integration`, `llm-quality` | path-scoped | `llm-quality` has a 240-minute timeout |

**Important for scoping:** `functional-tests` on a PR runs only `@critical`
scenarios. A new lifecycle scenario in `test/functional/features/` is invisible on
PRs unless tagged `@critical` — at which point it runs on every PR touching any
`.go` file.

**Golden-plan validation** is a separate workflow,
`.github/workflows/validate-golden-code.yml`. It triggers on a path allowlist
that includes `internal/executor/plan.go`, `internal/executor/plan_generator.go`,
`internal/executor/plan_conversion.go`, `internal/actions/action.go`,
`internal/actions/decomposable.go`, `internal/actions/composites.go`,
`internal/recipe/types.go`, `internal/recipe/loader.go`, and `internal/version/*.go`.
It does **not** trigger on `internal/actions/set_env.go` or
`internal/actions/shell_init.go`. The single job `validate-all` builds tsuku and
runs `./scripts/validate-all-golden.sh --os linux --category embedded`, which
regenerates plans **live against upstream** and diffs them against
`testdata/golden/plans/`.

That is the warned-about hazard, and PR #2442 demonstrates it exactly: it touched
`internal/recipe/types.go` and `internal/executor/plan.go`, which pulled in the
workflow, which then failed on `gcc-libs` because the Homebrew bottle checksum
had drifted upstream. The committed fix is four files of pure churn —
`testdata/golden/plans/embedded/gcc-libs/v15.2.0-linux-{arch,debian,rhel,suse}-amd64.json`
each swapping `606c8f50…` for `eebab973…` and the `size` field, nothing to do
with the change under review.

`./scripts/regenerate-golden.sh <recipe> [--version|--os|--arch|--recipe|--category]`
is the tool for producing that churn. It **requires `GITHUB_TOKEN`** (auto-detected
from `gh auth token`, hard-fails otherwise), writes to
`testdata/golden/plans/embedded/<recipe>/` or `testdata/golden/plans/<letter>/<recipe>/`,
and is wrapped by `scripts/regenerate-all-golden.sh` and `scripts/validate-all-golden.sh`.

A second workflow, `.github/workflows/validate-golden-execution.yml`, fires on
changes to `testdata/golden/plans/embedded/**/*.json` — so committing the
regenerated goldens *itself* triggers an execution-validation run (with an R2
health-check job in front of it). Budget for two rounds.

### 7. Mutation testing precedent

**None.** No mutation-testing tool in `go.mod` (test deps are `cucumber/godog
v0.15.1` and `stretchr/testify v1.11.1` only), nothing in `scripts/` (40 scripts,
all recipe/R2/golden/queue tooling), no workflow, and no doc. Every `grep -ri
mutation` hit across `docs/` refers to *state mutation* in the domain sense
(`docs/designs/current/DESIGN-notices-install-event-bus.md`,
`DESIGN-auto-apply-rollback.md`), not to mutation testing.

So "mutation-test the guards" has no tooling to lean on and must mean the manual
discipline: for each guard, apply a specific one-line defect to the
implementation, run the new test, confirm it **fails**, revert. The defects worth
enumerating for this bug, given the code as it stands:

1. Make the re-render a no-op on the remove path (delete the re-render call in
   `Manager.RemoveVersion` / `executeCleanupActions`) — the test must fail.
2. Re-render the file but write the *removed* version's content (swap which
   version the content is derived from) — this is the defect that a
   file-existence or hash-only assertion cannot see.
3. Update `ContentHash` in `state.json` without rewriting the file on disk — the
   inverse; catches a fix that only repairs bookkeeping.
4. Skip the `RebuildShellCache` after re-rendering — the shell.d file is correct
   but `.init-cache.bash` still holds the old content, and only a subshell read
   catches it.
5. Re-render `00-env-<tool>.<shell>` but not the `install_shell_init` file (or
   vice versa) — the brief explicitly requires both writers to be covered.

Defects 2 and 4 are precisely the ones that "assert on file contents" misses, and
4 is invisible to *any* filesystem assertion on `share/shell.d/<tool>.bash`.

### 8. Lint config

`.golangci.yaml` (`version: "2"`, `default: none`) enables: `govet`, `errcheck`,
`staticcheck` (`all` minus `ST*` and `QF*`), `ineffassign`, `unused`, `misspell`
(US locale), `gosec`, `bodyclose`, `dupl`.

Patterns that will bite test code:

- **`errcheck` runs on test files.** The exclusion list
  (`.golangci.yaml`, `settings.errcheck.exclude-functions`) covers `os.Remove`,
  `os.RemoveAll`, `os.Setenv`, `fmt.Fprintf`, and various `Close`s — but **not**
  `os.WriteFile`, `os.MkdirAll`, or `cmd.Run`. Every fixture write in a new test
  must check its error. This matches the style already used in
  `internal/install/remove_test.go` and PR #2442's `setEnvHome`.
- **`dupl` at threshold 250 tokens.** Near-identical multi-version scenario
  bodies (install v1, install v2, remove one, assert) repeated across shells or
  across upgrade/activate/rollback/remove will trip it. Table-driven subtests
  avoid the problem.
- **`gosec`** excludes G204 (subprocess with variable), G301/G302/G304/G306
  (file/dir perms and opens), so `exec.Command(bashPath, ...)` and fixture writes
  are fine.
- `misspell` with `locale: US`.

`go vet` and `gofmt` are enforced separately by `lint_test.go:22,45` via the
`lint-tests` CI job.

## Implications

The cheapest test that satisfies the acceptance criteria lives in `cmd/tsuku`
(`package main`). That package already imports `install`, `actions`, `executor`,
and `shellenv`; it is the only place where all four meet without an import cycle,
and it is where the real post-install orchestration lives
(`cmd/tsuku/plan_install.go:117-137`). Setup is entirely offline:
`testutil.NewTestConfig` for the home, two `Manager.InstallWithOptions` calls
against hand-built `workDir/.install/` trees, action `Execute` calls to write the
two shell.d files per version, `Manager.RemoveVersion`, then
`exec.Command(bash, "--norc", "--noprofile", "-c", `. "$TSUKU_HOME/env"; printf %s "${NVM_DIR-}"`)`
with a hermetic `cmd.Env`. Copy PR #2442's read-back verbatim; it is already the
right shape.

The three constraints that shape everything else: the test must **not** be
`testing.Short()`-gated (CI always passes `-short`); it must write only to temp
dirs (the artifact check fails the build otherwise); and if it goes into
`test/functional` instead, it must be tagged `@critical` to run on PRs and needs
a brand-new step that captures output, because `iSourceHomeFileAndCanRun`
throws away everything but the exit code.

On the golden-plan hazard: the shell.d fix should stay out of
`internal/executor/plan*.go`, `internal/recipe/types.go`, `internal/actions/action.go`,
`decomposable.go`, and `composites.go` if at all possible. Touching any of them
arms `validate-golden-code.yml`, which regenerates plans against live upstream
and will fail on unrelated Homebrew bottle drift — then committing the
regenerated goldens arms `validate-golden-execution.yml` in turn. If the fix
genuinely needs a plan-shape change, budget a `GITHUB_TOKEN`-authenticated
`./scripts/regenerate-golden.sh gcc-libs` round and expect reviewer questions
about unrelated checksum churn.

Mutation testing is entirely manual here. It should be written into the PR
description as an explicit list of applied defects with observed failures,
because there is no tool output to point at and no repo convention to inherit.

## Surprises

**`internal/actions` transitively imports `internal/install`,** via
`internal/version/resolve.go:7`. This kills the intuitive plan of putting the
lifecycle test in `internal/install` alongside the existing multi-version and
cleanup tests — all 13 test files there are `package install`, and adding an
`actions` import creates a cycle. The workaround (`package install_test`) exists
but would be a first for that directory.

**`Manager.Activate` does not touch shell.d at all** —
`internal/install/manager.go:411-470` updates symlinks and `state.json` and
returns. `Manager.Rollback` (`:353`) just delegates to `Activate`. So the
activate and rollback arms of the bug are not "stale re-render", they are
"no re-render code path exists to be stale".

**`RebuildShellCache` already has a hash-verification mode that silently drops
mismatched files** (`internal/shellenv/cache.go:113-127`): if a stored
`ContentHash` disagrees with disk, the file is excluded from `.init-cache.<shell>`
with a stderr warning. But the two install-side callers pass no hashes at all
(`internal/install/remove.go:400`, `internal/install/update.go:58`,
`cmd/tsuku/plan_install.go:134`, `cmd/tsuku/install_deps.go:595`) — only
`cmd/tsuku/doctor.go:66` does. This means the observable symptom of the bug
depends on which command you run: after `remove`, the variable points at the
deleted version; after `doctor --fix`, the tool disappears from the shell
entirely. A subshell assertion distinguishes those two; a file-content assertion
does not.

**`doctor` already detects the exact condition and cannot repair it.**
`CheckShellD` (`internal/shellenv/doctor.go:52`) populates `HashMismatches` by
comparing stored hashes to disk (`:105-118`). But `doctor --fix`
(`cmd/tsuku/doctor.go:63-73`) only acts on `CacheStale`, never on
`HashMismatches` — and the rebuild it runs *excludes* the mismatched file. So the
detection half of the repair path is already built.

**The `--plan` install path never records cleanup actions.**
`cmd/tsuku/plan_install.go:125-137` collects `exec.GetCleanupActions()` and
rebuilds the shell cache, but never calls `mgr.RecordCleanup`. Only
`cmd/tsuku/install_deps.go:613` does. Adjacent to this lead but worth handing to
whoever owns the write-path question.

**`testutil.TempDir` is not `t.TempDir`** — it is a hand-rolled
`os.MkdirTemp` + cleanup-closure pair from before `t.TempDir` existed
(`internal/testutil/testutil.go:13-20`). New code in `internal/actions` uses
`t.TempDir()` directly. Both idioms are live; there is no stated preference.

## Open Questions

- Is zsh installed on the `ubuntu-latest` runner image? Nothing in the repo
  answers this, and the one place that would care (`checkShellSyntax`) is written
  to no-op when it is missing. If a zsh arm of the lifecycle test is wanted, this
  needs verifying against the runner image manifest, and the test needs a
  `LookPath`-based skip either way.
- Where does the fix actually live, and can it re-render without an import cycle?
  Re-rendering requires re-deriving the shell.d content for the newly-active
  version, which means either replaying the stored `Plan` (needs `executor`,
  which imports `install` — a cycle from inside `install`) or storing the
  rendered content/template in `state.json`. That choice determines whether the
  test can be a `internal/install` test at all, or must sit in `cmd/tsuku`. This
  is a design question, not a test-infra one, but it gates the test's home.
- Can a two-version *offline* install be driven through the real binary in
  `test/functional`? `testdata/recipes/tool-a.toml` proves offline recipes work,
  but I did not verify how an explicitly-requested version (`install foo@1.0.0`)
  is resolved for a recipe with no `[version]` source. If it works, a
  fully-offline functional scenario is possible; if not, the functional route
  needs a network-dependent recipe like `nvm`.
- Should the new bash-subshell assertion be a reusable helper? Three call sites
  will exist (PR #2442's, plus at least two here). Nothing in `internal/testutil`
  can host it today without pulling `os/exec` into a package that four dozen
  tests import, and `internal/actions` cannot export to `internal/install`.

## Summary

The pieces for a real-subshell multi-version test all exist but no single package holds them: `testutil.NewTestConfig` gives an isolated `$TSUKU_HOME`, `Manager.InstallWithOptions` installs two versions with zero network, PR #2442 supplies a copyable hermetic `bash --norc --noprofile` read-back, and `test/functional` already sources `$TSUKU_HOME/env` — but `internal/actions` transitively imports `internal/install` via `internal/version/resolve.go:7`, so the test has to live in `cmd/tsuku` (or an external `install_test` package) rather than beside the existing multi-version tests. The three hard constraints are that CI always passes `-short` (so `testing.Short()` gating makes a test dead), the existing functional step throws away command output and runs only `@critical` scenarios on PRs, and touching any plan-generation file arms `validate-golden-code.yml`, which fails on live upstream checksum drift unrelated to the change. The biggest open question is whether the fix can re-render shell.d content from inside `internal/install` at all — `executor` imports `install`, so replaying the stored plan is a cycle, and that design choice determines both where the fix lands and where its test can live.
