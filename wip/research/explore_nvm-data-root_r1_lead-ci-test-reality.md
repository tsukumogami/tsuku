# Lead: What does CI actually run on a pull request, and where can a "user data survives an nvm upgrade" assertion live such that it actually executes and actually catches a regression?

## Findings

### 1. Workflows that trigger on `pull_request`

33 workflow files carry a `pull_request:` trigger. The ones that matter for this
change are `test.yml`, `test-recipe.yml`, `integration-tests.yml`, and
`validate-golden-code.yml`. Everything else is docs/website/telemetry/skill
validation and is irrelevant to a recipe + Go change.

#### `.github/workflows/test.yml` — the main gate

Trigger (`.github/workflows/test.yml:18-31`):

```yaml
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review, converted_to_draft]
    branches: [main]
    paths:
      - '**/*.go'
      - 'go.mod'
      - 'go.sum'
      - 'test/scripts/**'
      - 'test/functional/**'
      - 'test-matrix.json'
      - 'testdata/**'
      - '.github/workflows/test.yml'
      - 'recipes/**/*.toml'
      - 'internal/recipe/recipes/**/*.toml'
```

Every job in this workflow is gated on a `dorny/paths-filter` step
(`.github/workflows/test.yml:367-396`). The critical detail is that the `code`
filter **does not include `recipes/**`**:

```yaml
            code:
              - '**/*.go'
              - 'go.mod'
              - 'go.sum'
              - 'test/functional/**'
              - '.github/workflows/**'
            ...
            recipes:
              - 'internal/recipe/recipes/**/*.toml'
              - 'recipes/**/*.toml'
            ...
            functional:
              - 'test/functional/**'
```

So a **recipe-only PR** (touching just `recipes/n/nvm.toml`) triggers the
workflow but runs *no Go tests at all* — `unit-tests`, `lint-tests` and
`functional-tests` are all `if: ${{ needs.matrix.outputs.code == 'true' }}`
(lines 41, 99, 121) and evaluate to false. Only `validate-recipes` and the two
`integration-*` jobs run (they check `code == 'true' || recipes == 'true'`).

**Claim: "the unit test job always passes `-short`" — CONFIRMED.**
`.github/workflows/test.yml:65-73`:

```yaml
      - name: Run tests
        # PRs: fast feedback without race detector or coverage
        # Push to main: full validation with race detector and coverage
        run: |
          if [ "${{ github.event_name }}" = "push" ]; then
            go test -short -race -coverprofile=coverage.out ./...
          else
            go test -short ./...
          fi
```

Both branches pass `-short`. Anything gated behind `testing.Short()` never runs
in this job, on PRs *or* on push to main. 19 tests use `testing.Short()`; the
files are `lint_test.go`, `internal/builders/llm_integration_test.go`,
`internal/verify/dltest_test.go`, `internal/llm/{claude,client}_test.go`,
`internal/batch/orchestrator_test.go`,
`internal/sandbox/{container_spec,integration}_test.go`,
`internal/version/{fossil_integration,provider_cask}_test.go`. The lint checks
are the notable ones, and they get their own job that omits `-short`
(`.github/workflows/test.yml:116`):

```yaml
      - name: Run lint tests
        run: go test -v -run 'Test(GolangCILint|GoFmt|GoModTidy|GoVet|Govulncheck)' .
```

**Claim: "the unit test job fails if `git status --porcelain` is non-empty" — CONFIRMED.**
`.github/workflows/test.yml:77-85`:

```yaml
      - name: Check for test artifacts
        run: |
          # Fail if tests left any files in the source tree
          if [ -n "$(git status --porcelain)" ]; then
            echo "::error::Tests left artifacts in the source tree. Tests must clean up after themselves."
            echo "Changed files:"
            git status --porcelain
            exit 1
          fi
```

There is a second, earlier tidiness gate at lines 57-63 (`go mod tidy` +
`git diff --exit-code go.mod go.sum`). Practical consequence: any new test must
use `t.TempDir()` or clean up. `.tsuku-test/` is gitignored (`.gitignore`, last
line), as is `/tsuku-test`, so the functional harness is safe here.

**Claim: "functional tests run only `@critical` on PRs" — MOSTLY TRUE, with an
important escape hatch.** `.github/workflows/test.yml:137-145`:

```yaml
      - name: Run functional tests
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          if [ "${{ github.event_name }}" = "push" ] || [ "${{ needs.matrix.outputs.functional }}" = "true" ]; then
            make test-functional
          else
            make test-functional-critical
          fi
```

`functional` is true when the PR touches `test/functional/**`. So a PR that adds
a new `.feature` file runs the **full** functional suite on that PR. It is only
on *later* PRs that don't touch `test/functional/**` that the new scenario is
skipped unless tagged `@critical`. This matters: a new untagged scenario passes
green on the PR that adds it and then silently stops running.

**Claim: "integration jobs are the long (60-minute) ones and may not run on PRs" —
HALF WRONG.** They are the 60-minute ones (`.github/workflows/test.yml:477`,
`543`: `timeout-minutes: 60`), but they **do** run on PRs:

```yaml
  integration-linux:
    name: "Linux Integration Tests"
    needs: matrix
    # Run when code files or recipes changed (skip for docs-only changes)
    if: ${{ needs.matrix.outputs.code == 'true' || needs.matrix.outputs.recipes == 'true' }}
    runs-on: ubuntu-latest
    timeout-minutes: 60
```

What they run is fixed by `test-matrix.json`, not by the diff. The `ci.linux`
list is `github_actionlint_with_version`, `github_btop_no_version`,
`github_argo-cd_binary`, `github_bombardier_simple`, `archive_golang_directory`,
`archive_nodejs_checksum`, `pipx_ruff_basic`, `archive_perl_relocatable`,
`tap_waypoint-tap_short_form`. **nvm is not in `test-matrix.json` at all.** Each
entry is a single `./tsuku install --force <tool>` — there is no upgrade step and
no post-install assertion beyond install exit code.

`.github/workflows/integration-tests.yml` is a separate workflow (checksum
pinning, homebrew, etc.), also `pull_request`-triggered on `**/*.go` /
`test/scripts/**`, also `timeout-minutes: 60`. It runs shell scripts under
`test/scripts/`, not Go tests.

#### `.github/workflows/test-recipe.yml` — the one that actually installs nvm on a PR

Triggered by `recipes/**/*.toml` (lines 34-36). It detects changed recipes from
the diff and installs each across five Linux families in sandbox containers plus
native macOS arm64/x86_64, four jobs at `timeout-minutes: 60`. The install
command (`.github/workflows/test-recipe.yml:432-438`):

```yaml
                TSUKU_HOME="$FAMILY_HOME" TSUKU_TELEMETRY=0 \
                  ./tsuku-linux-amd64 install --sandbox --force \
                    --dangerously-suppress-security \
                    --recipe "$recipe_path" \
                    --target-family "$family" \
                    --env GITHUB_TOKEN="$GITHUB_TOKEN" \
                    --env TSUKU_REGISTRY_URL="$TSUKU_REGISTRY_URL" \
                    --json > "$RESULT_FILE" 2>"$LOG_FILE" || true
```

So editing `recipes/n/nvm.toml` does get nvm installed on ~7 platform/family
combinations on the PR. It installs **once**. There is no second version, no
`nvm install <node>`, no upgrade. It cannot catch this bug.

#### `.github/workflows/curated-nightly.yml`

`nvm.toml` has `curated = true`, so nvm is enrolled in the nightly cross-platform
validation (`curated-nightly.yml:35`: `grep -rl 'curated = true' recipes/
internal/recipe/recipes/`), which calls `recipe-validation-core.yml`
(`timeout-minutes: 120` per platform). Schedule: `cron: '0 3 * * *'`. Again:
install-once validation, not an upgrade scenario.

### 2. Functional tests: location and the `@critical` mechanism

Godog/Gherkin. Feature files in `test/functional/features/*.feature` (27 files),
suite bootstrap in `test/functional/suite_test.go`, step definitions in
`test/functional/steps_test.go`. Tags are plain Gherkin tags filtered by an env
var — no build tags, no registry.

`test/functional/suite_test.go:60-67`:

```go
	opts := &godog.Options{
		Format:   "pretty",
		Paths:    paths,
		TestingT: t,
	}
	if tags := os.Getenv("TSUKU_TEST_TAGS"); tags != "" {
		opts.Tags = tags
	}
```

`Makefile:22-30`:

```make
# Run functional tests (builds test binary first)
test-functional: build-test
	TSUKU_TEST_BINARY=$(CURDIR)/tsuku-test go test -v ./test/functional/...
	rm -rf .tsuku-test

# Run only critical functional tests
test-functional-critical: build-test
	TSUKU_TEST_BINARY=$(CURDIR)/tsuku-test TSUKU_TEST_TAGS=@critical go test -v ./test/functional/...
	rm -rf .tsuku-test
```

`TestFeatures` skips entirely when `TSUKU_TEST_BINARY` is unset
(`suite_test.go:43-46`), which is why `go test -short ./...` in the unit-tests
job does not run functional scenarios.

`@critical` counts per file: environment 7, create 4, cache 4, verify 2, install
2, update-registry 1, everything else 0. Concrete example
(`test/functional/features/install.feature:7-12`):

```gherkin
  @critical
  Scenario: Install a simple tool
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    And the file "tools/current/actionlint" exists
    And I can run "actionlint -version"
```

Other tags the harness understands, parsed in `suite_test.go:117-137`:
`@requires-no-<binary>` (strips PATH dirs containing that binary),
`@empty-registry`, `@fake-llm-binary`. Adding a new tag means editing that loop.

Available steps (`suite_test.go:151-172`) include `I run "..."`,
`the file "X" exists` / `does not exist` / `contains` / `does not contain`,
`I create home file "X" with content:`, `I source home file "X" and can run "Y"`,
`I can run "X"`, `I set env "K" to "V"`, `I run from "DIR" "CMD"`. Notably there
is **no** step for "install version N then version M", no step that captures
command output into a variable, and no step for asserting on a path derived from
an env var. A real nvm upgrade scenario would need new step definitions.

`iRun` (`steps_test.go:48-52`) sets `TSUKU_REGISTRY_URL` to the repo root — a
local filesystem path — so recipe *resolution* is offline. Downloads still hit
the network.

### 3. Existing tests for nvm / shell.d / set_env, and what PR #2465 added

`nvm` appears in 8 test files, all Go, all as a *string* in fixtures — no
functional feature file mentions nvm (`grep -rn "nvm" test/functional/features/`
returns nothing). `set_env` is the only action nvm uses that is unique to it:
`grep -rln "set_env" recipes/ internal/recipe/recipes/` returns exactly one file,
`recipes/n/nvm.toml`. **nvm is the only recipe in the registry using `set_env`.**

`git show --stat d396aeec` (PR #2465, "fix(shellenv): keep shell.d correct for
the active version across the tool lifecycle"): 47 files, +3929/-407. Test files
touched: `cmd/tsuku/{doctor,post_install,shelld_lifecycle,update}_test.go`,
`internal/actions/{preflight,set_env,shell_init_naming,shell_init}_test.go`,
`internal/executor/{executor,phase}_test.go`,
`internal/install/{shelld_lifecycle,shelld,update_test}.go`,
`internal/recipe/validator_test.go`, `internal/shellenv/{cache,doctor,selection}_test.go`,
`internal/updates/gc_test.go`.

**Assessment: these tests assert on observable behavior, and they do it well.**
The headline addition is `cmd/tsuku/shelld_lifecycle_test.go` (449 lines), whose
file comment states the standard explicitly (lines 18-25):

```go
// These tests drive a synthetic tool through install, removal, activation and
// rollback, then read the result out of a real bash subshell that sources
// $TSUKU_HOME/env. The subshell is the point: a fragment can be correct on disk
// and still be missing from the init cache the user's shell actually sources,
// and a test that inspects share/shell.d passes in exactly that case.
//
// cmd/tsuku is where they live because it is the only package where install,
// actions and shellenv meet -- internal/install cannot import internal/actions.
```

And `internal/actions/set_env_test.go:48-53` names the prior failure the
exploration brief alludes to:

```go
// TestSetEnvAction_ReachesUserShell is the end-to-end check the action was
// missing: it installs exports plus a tool init script that consumes them,
// rebuilds the shell cache, then sources $TSUKU_HOME/env in a real bash
// subshell and reads the variable back. Asserting on the generated file alone
// is what let the "set_env does nothing" bug survive.
```

So the "prior bug survived because its test stopped one layer short" story is
about the *pre-#2465* state. #2465 fixed the test standard along with the code.
What #2465 did **not** do is assert anything about *data written into* the
directory `NVM_DIR` names. Its assertions are all "does the shell see the right
value / did the right fragment reach the cache":
`assertShellSees` (`cmd/tsuku/shelld_lifecycle_test.go:210-236`) checks the
export value, the init var, and the source-time record. Nothing writes a file
into the tool directory and checks it survives.

`recipes/n/nvm.toml` was touched by #2465 (5 lines) — that is where the
`NVM_DIR = "{install_dir}"` comment block at lines 7-12 came from.

The deletion mechanisms that destroy the user's Node versions:
- `internal/install/remove.go:150-153` — `RemoveVersion` does
  `os.RemoveAll(m.config.ToolDir(name, version))`.
- `internal/updates/gc.go:24` — `GarbageCollectVersions(reaper, toolsDir,
  toolName, activeVersion, previousVersion, retention, now)` deletes any
  `<tool>-<version>` dir past retention that isn't active or the rollback target.

### 4. Linters (`.golangci.yaml`)

Enabled: `govet`, `errcheck`, `staticcheck` (`all`, minus `ST*` and `QF*`),
`ineffassign`, `unused`, `misspell` (US locale), `gosec`, `bodyclose`, `dupl`.
`default: none`, so nothing else is on. `contextcheck` is explicitly commented
out.

**`dupl` at 250 — CONFIRMED** (`.golangci.yaml:60-62`):

```yaml
    dupl:
      # Increase threshold - version providers have similar structures by design
      threshold: 250
```

This bites when writing table-driven test helpers that mirror an existing one.
Existing suppressions: `internal/builders/npm_test.go:321` and
`internal/builders/pypi_test.go:221` both carry
`//nolint:dupl // Test structure similar to other builder tests by design`.

**`errcheck` runs on test files and does NOT exclude `os.WriteFile`/`os.MkdirAll`
— CONFIRMED.** The `exclude-functions` list (`.golangci.yaml:64-84`) covers only
`Close` variants, `os.Remove`/`os.RemoveAll`, `flock.Close`, `os.Setenv`, and
`fmt.Fprintf`. `run:` has no `tests: false`, so golangci-lint's default
(`tests: true`) applies. Empirical proof: there are **33** occurrences of
`_ = os.MkdirAll(` / `_ = os.WriteFile(` in `*_test.go` and **zero** bare
statement-position calls to either. `issues.exclusions.presets` is
`[comments, std-error-handling]`; `std-error-handling` covers the
stdout/stderr/`Close`/`Flush`/`os.Remove*`/`print*`/`os.(Un)Setenv` pattern —
neither `WriteFile` nor `MkdirAll` is in it.

`gosec` has a long `excludes` list (G404, G204, G304, G305, G302, G306, G301,
G107, G110, G104, G115, G702-G705, G101, G117, G118, G122) — in practice gosec
will not fight a test that writes files or runs `bash`.

Not enabled, so not a concern: `gocritic`, `revive`, `gocyclo`, `lll`, `funlen`,
`goconst`, `nakedret`, `wsl`, `godot`.

Beyond golangci there are repo-local lint tests in `lint_test.go`:
`TestGoFmt`, `TestGoModTidy`, `TestGoVet`, `TestGovulncheck`, `TestNoStdlibLog`
(forbids importing stdlib `log`, skips `*_test.go`). All are `testing.Short()`-
guarded but the `lint-tests` job runs them without `-short`.

### 5. Testing an nvm upgrade without network

**There is no network-free path to a real `nvm install <node>`.** `nvm install`
downloads a Node tarball from nodejs.org; nothing in this repo stubs that. The
56 test files using `httptest` all stub *tsuku's own* fetches — version providers
(`internal/version/*_test.go`), builders (`internal/builders/*_test.go`),
registry (`internal/registry/*_test.go`), and the download action
(`internal/actions/download_test.go`, `signature_test.go`,
`composites_fallback_test.go`). None of them drives `internal/install.Manager`
end-to-end through a fake origin server; the install-manager tests bypass
downloading entirely by handing the manager a pre-populated staging directory.

That bypass is exactly the useful lever. `cmd/tsuku/shelld_lifecycle_test.go`
already installs two versions of a synthetic tool with **zero network**:
`installVersion` (lines 124-187) writes a fake binary and init script into a
`t.TempDir()` staging dir, calls `h.mgr.InstallWithOptions(...)`, then runs the
real `SetEnvAction` and `InstallShellInitAction` and the real
`finishPostInstall`. It then reads results out of a hermetic bash subshell
(`shellVar`, lines 195-208):

```go
	cmd := exec.Command(h.bash, "--norc", "--noprofile", "-c",
		`. "$TSUKU_HOME/env"; printf %s "${`+name+`-}"`)
	cmd.Env = []string{"TSUKU_HOME=" + h.home, "HOME=" + h.home, "PATH=/usr/bin:/bin"}
```

Measured cost: `go test -short -run 'TestShellDLifecycle' ./cmd/tsuku/` →
`ok github.com/tsukumogami/tsuku/cmd/tsuku 1.719s`. It is not `testing.Short()`-
guarded, uses `t.TempDir()` (so the porcelain check is satisfied), and skips
cleanly when bash is absent (`t.Skip("bash not available")`, line 78).

Other fixture machinery that exists: `testdata/recipes/*.toml` (29 local recipes
used by integration matrix entries via `--recipe`), `TSUKU_HOME` override
honoured everywhere, `TSUKU_REGISTRY_URL` accepting a local directory path (the
functional harness points it at the repo root), and `--sandbox` container
installs. No `file://` download URLs anywhere in `recipes/` or `testdata/recipes/`.

### 6. `validate-golden-code.yml`

Arming files (`.github/workflows/validate-golden-code.yml:6-45`): `pull_request`
on `main`, paths `cmd/tsuku/eval.go`,
`internal/executor/{plan_generator,plan,plan_conversion}.go`,
`internal/actions/{decomposable,action,composites,download}.go`,
`internal/recipe/{types,loader,platform}.go`, the nine package-manager
decomposers (`homebrew`, `cargo_install`, `npm_install`, `pipx_install`,
`gem_install`, `go_install`, `nix_install`, `fossil_archive`, `apply_patch`),
`internal/version/*.go` (excluding `*_test.go`), the workflow file itself, and
`testdata/golden/code-validation-exclusions.json`.

What it does (lines 90-108): builds tsuku, runs
`./scripts/validate-golden-exclusions.sh --file testdata/golden/code-validation-exclusions.json --check-issues`,
then `./scripts/validate-all-golden.sh --os linux --category embedded`.
`timeout-minutes: 30`.

Failure mode:

```yaml
            echo "::error::Golden files are out of date due to code changes."
            echo "::error::See output above for which recipes failed and regeneration commands."
            exit 1
```

i.e. it regenerates plans for every embedded recipe and diffs against committed
golden JSON; any plan-shape change fails until goldens are regenerated.

**Relevance to this fix:** the explicit non-trigger list at lines 49-65 says
"execution-only files" don't arm it, and `internal/actions/set_env.go` is not in
the arming list. But `internal/actions/action.go` **is** (line 15), and so is
`internal/recipe/types.go` (line 22) and `internal/recipe/loader.go`. If the fix
introduces a new recipe field (e.g. a `data_dir` concept) or touches the action
interface / `PhaseDeclarer` plumbing in `action.go`, this workflow arms and the
goldens for every embedded recipe must be regenerated in the same PR. Note also
that nvm is a *registry* recipe (`recipes/n/nvm.toml`), not embedded, so the
`--category embedded` run does not cover nvm's own golden file — that is
`validate-recipe-golden-files.yml`'s job.

## Implications

**A recipe-only fix gets almost no test coverage on the PR.** If the fix is
purely `recipes/n/nvm.toml`, the `code` paths-filter is false and `unit-tests`,
`lint-tests` and `functional-tests` are all skipped. What runs is
`validate-recipes` (`tsuku validate --strict` over every recipe),
`integration-linux`/`integration-macos` (a fixed tool list that excludes nvm),
and `test-recipe.yml` (installs nvm once per family). None of those installs a
Node version or upgrades nvm. A recipe-only fix would ship with an
install-succeeds check and nothing else. That argues for the fix carrying a Go
component (a validator rule and/or an action change) so the Go jobs arm at all.

**The fast guard belongs in `cmd/tsuku/shelld_lifecycle_test.go`.** It is the
only place where install + actions + shellenv meet (the file says so), it
already stands up two versions of a synthetic tool with no network, it reads
outcomes through a real bash subshell, it runs in 1.7 seconds, and it is not
`testing.Short()`-guarded so it executes under `go test -short ./...` on every
PR that touches Go. The missing assertion is one the existing harness is three
lines short of supporting: nothing currently writes a user file into the
directory the exported variable names and checks it is still there after the
second install.

**A `@critical` Gherkin scenario is the wrong home for the fast guard.** It needs
network (real nvm, real Node), needs new step definitions (multi-version install,
capturing a path from an env var), and `@critical` scenarios run on every PR that
touches any `.go` file — putting a two-minute network download in that path
would slow every PR in the repo. Conversely an *untagged* scenario runs only on
PRs that touch `test/functional/**`, which means it would pass green on the PR
that adds it and then never run again. Neither option is good.

**`test-matrix.json` is the lever for the slow test, but its current shape is
install-only.** Entries are `{id, tool, desc, recipe}` and the job body is a
single `./tsuku install --force`. Making it express "install, then install a
second version, then assert" means changing the runner loop in `test.yml`, not
just adding a JSON row. A shell script under `test/scripts/` invoked from
`integration-tests.yml` is a closer fit to how that workflow already works
(`test-checksum-pinning.sh`, `test-homebrew-recipe.sh`).

**The lint config constrains how the test is written.** `errcheck` on test files
without a `WriteFile`/`MkdirAll` exclusion means every fixture write needs `if
err := ...; err != nil { t.Fatalf }` or an explicit `_ =`. `dupl` at 250 means a
new harness method that mirrors `assertShellSees` closely may need
`//nolint:dupl`. And the porcelain check means `t.TempDir()`, always.

## Surprises

1. **`recipes/**` is not in the `code` paths-filter.** Recipe changes trigger
   `test.yml` but run none of the Go jobs. This is easy to misread from the
   trigger block alone, which *does* list `recipes/**/*.toml`.

2. **The full functional suite runs on PRs that touch `test/functional/**`.**
   The `@critical`-only rule has an escape hatch
   (`needs.matrix.outputs.functional == 'true'`). The perverse effect is that a
   new untagged scenario is green on its own PR and dormant forever after.

3. **nvm is the only recipe in the entire registry that uses `set_env`.** A
   validator rule about `set_env` values would have exactly one subject today.
   That makes a static rule cheap to add and cheap to keep correct, but also
   means it protects against a future mistake more than it proves the present fix.

4. **PR #2465's tests are genuinely behavioral, not intermediate-state.** They
   source `$TSUKU_HOME/env` in a hermetic `bash --norc --noprofile` and read
   variables back, and they record what the init script *saw at source time* so
   ordering bugs can't be masked by a later overwrite. The gap is narrower than
   "the test stopped one layer short": #2465 asserted the shell gets the right
   *value* for `NVM_DIR` and never asked what happens to *content stored under*
   that value.

5. **Integration jobs do run on PRs** — the brief's guess that they might not is
   wrong. They just don't test nvm, because `test-matrix.json` doesn't list it.

6. **`assertFragmentsOnDisk`'s doc comment already articulates the exact failure
   mode of this bug** (`cmd/tsuku/shelld_lifecycle_test.go:249-251`): "Deletion
   is the failure mode this catches, and it is silent: doctor walks directory
   entries, so a fragment that is simply gone produces no complaint of any kind."
   Same shape, different victim — here it is the user's Node versions rather than
   a shell fragment.

## Open Questions

- Does the fix introduce a new recipe field (a `data_dir`-style concept) or reuse
  an existing path? If it touches `internal/recipe/types.go`,
  `internal/recipe/loader.go`, or `internal/actions/action.go`,
  `validate-golden-code.yml` arms and every embedded golden must be regenerated
  in the same PR.
- Should the validator reject `set_env` exporting `{install_dir}` outright, or
  only for a known list of data-root variable names? Rejecting `{install_dir}`
  broadly would break the legitimate `JAVA_HOME`-style use the action's own
  doc comment cites (`internal/actions/set_env.go:99`).
- Where does the stable data root live —
  `$TSUKU_HOME/share/nvm`, `$TSUKU_HOME/data/nvm`, or `$NVM_DIR` outside
  `$TSUKU_HOME` entirely? The answer determines whether the guard asserts "the
  path is outside `tools/`" or the stronger "content written there survives".
  Prefer the latter regardless of where it lands.
- Is anyone willing to accept a nightly-only end-to-end that catches this in ~24
  hours rather than at PR time? If not, the only PR-time coverage is the
  synthetic-tool guard, and the real-nvm test becomes documentation of a manual
  check.
- Does `nvm ls` work in the CI runner's non-interactive bash without a login
  shell? The functional harness's `iSourceHomeFileAndCanRun` uses `bash -c '. env
  && cmd'`, which should work, but nvm's own completion/`ls` path has been known
  to want more.

## Recommendation

### (a) Fast guard, runs on every PR

**File:** `cmd/tsuku/shelld_lifecycle_test.go`
**Test:** `TestShellDLifecycle_UpgradePreservesExportedDataRoot`

Extend the existing `shellDHarness` with one method and add one test. Shape:

```go
// The recipe exports a data root. Whatever path the user's shell is told to
// use, content the user puts there has to still be there after an upgrade.
func TestShellDLifecycle_UpgradePreservesExportedDataRoot(t *testing.T) {
	h := newShellDHarness(t)

	h.installVersion(lifecycleV1)

	// Read the data root the way nvm does: out of the user's shell.
	root := h.shellVar(envDirVar)
	if root == "" {
		t.Fatal("the shell did not get the exported data root")
	}
	userData := filepath.Join(root, "versions", "node", "v22.0.0", "bin", "node")
	if err := os.MkdirAll(filepath.Dir(userData), 0o755); err != nil {
		t.Fatalf("seeding user data: %v", err)
	}
	if err := os.WriteFile(userData, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seeding user data: %v", err)
	}

	h.installVersion(lifecycleV2)

	// The shell must still name a data root, and the user's file must still be
	// under whatever it now names.
	newRoot := h.shellVar(envDirVar)
	if newRoot != root {
		t.Errorf("the exported data root moved on upgrade: %q -> %q", root, newRoot)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "versions", "node", "v22.0.0", "bin", "node")); err != nil {
		t.Errorf("the user's data under the exported root did not survive the upgrade: %v", err)
	}

	// And it must survive reclamation of the superseded version, which is what
	// actually deletes it today.
	if err := h.mgr.RemoveVersion(lifecycleCtx(), h.tool, lifecycleV1); err != nil {
		t.Fatalf("RemoveVersion(%s) error = %v", lifecycleV1, err)
	}
	if _, err := os.Stat(filepath.Join(h.shellVar(envDirVar), "versions", "node", "v22.0.0", "bin", "node")); err != nil {
		t.Errorf("removing the superseded version deleted the user's data: %v", err)
	}
}
```

Why this shape:
- Every assertion goes through `h.shellVar(envDirVar)` — the value a real bash
  subshell got after sourcing `$TSUKU_HOME/env`. Nothing hard-codes the path, so
  the test does not encode the fix's implementation.
- Both `newRoot != root` and the `Stat` are needed. The first catches "the root
  moved"; the second catches "the root is stable but the content was reaped".
- The `RemoveVersion` leg is the mutation-critical one: today `RemoveVersion`
  does `os.RemoveAll(m.config.ToolDir(...))` (`internal/install/remove.go:151`),
  so with the current recipe the seeded file is deleted and the test fails. That
  is the one-line-defect check: revert `set_env`'s value back to
  `{install_dir}` and this test goes red on the `Stat` after `RemoveVersion`.
- Cost: adds ~1s to a 1.7s test binary. Runs under `go test -short ./...` in the
  `unit-tests` job on every PR touching Go. No network. `t.TempDir()` throughout,
  so the porcelain check passes. Handle every `os.WriteFile`/`os.MkdirAll` error
  explicitly — `errcheck` will not let them slide.

**Caveat:** this guards the *mechanism* (a `set_env`-exported root survives the
version lifecycle) with a synthetic tool, not nvm specifically. Pair it with a
cheap static rule so nvm itself is covered:

**File:** `internal/recipe/validator.go` (+ `validator_test.go`)
Reject `set_env` with a value of `{install_dir}` for variables that name a data
root — or, more defensibly, add a validator error when a recipe both exports
`{install_dir}` via `set_env` *and* the exported name is on a small list of
known data-root variables (`NVM_DIR`, `RBENV_ROOT`, `PYENV_ROOT`, `GOPATH`,
`CARGO_HOME`). This runs in `go test -short ./...` **and** in the
`validate-recipes` job (`tsuku validate --strict` over every recipe), which is
the only Go-adjacent job that runs on a recipe-only PR. Mutation check: put
`{install_dir}` back in `recipes/n/nvm.toml` and `validate-recipes` fails.

### (b) Slow end-to-end

**File:** `test/scripts/test-nvm-data-root.sh`
**Job:** a new job in `.github/workflows/integration-tests.yml`

`integration-tests.yml` already runs shell scripts from `test/scripts/` on
`pull_request` (paths `**/*.go`, `go.mod`, `go.sum`, `test/scripts/**`) with
`timeout-minutes: 60` per job and the `TSUKU_REGISTRY_URL` pointed at the PR
branch. The script shape:

```bash
export TSUKU_HOME="$(mktemp -d)"
./tsuku install nvm --version <older> --force
bash -lc '. "$TSUKU_HOME/env"; nvm install 22'   # network
./tsuku install nvm --version <newer> --force    # the upgrade
bash -lc '. "$TSUKU_HOME/env"; nvm ls | grep -q "v22"' || fail
./tsuku remove nvm --version <older>             # reclamation
bash -lc '. "$TSUKU_HOME/env"; nvm ls | grep -q "v22"' || fail
bash -lc '. "$TSUKU_HOME/env"; node --version'   # the real observable
```

The last line is the assertion that matters: not "nvm ls prints something" but
"the Node the user installed still runs". Two nvm versions pinned explicitly so
the test does not depend on what upstream tagged this week.

**Trigger:** put it in `integration-tests.yml` so it runs on PRs touching Go or
`test/scripts/**`. If runtime turns out to exceed ~5 minutes, move it to
`scheduled-tests.yml` (`cron: '0 2 * * *'`) and keep only the fast guard on PRs.
Do **not** add it to `test-matrix.json` — that structure is
`{id, tool, desc, recipe}` fed to a single `tsuku install`, and expressing an
upgrade sequence there would mean rewriting the runner loop in `test.yml`.

Also worth doing, one line: add `recipes/**/*.toml` to the `code` paths-filter in
`.github/workflows/test.yml:371-376`, or add a `recipes` clause to the
`functional-tests` job's `if`. Right now a recipe regression can land without a
single Go test running.

## Summary

CI on a PR runs `go test -short ./...` (so `testing.Short()`-gated tests never
execute), a `git status --porcelain` artifact check, `@critical`-only functional
scenarios unless the PR touches `test/functional/**`, and 60-minute integration
jobs that do run on PRs but test a fixed tool list from `test-matrix.json` that
excludes nvm — and critically, a recipe-only change runs no Go jobs at all
because `recipes/**` is absent from the `code` paths-filter. The fast guard
belongs in `cmd/tsuku/shelld_lifecycle_test.go`, whose existing harness already
installs two versions of a synthetic tool with no network in 1.7s and reads
results out of a real bash subshell: seed a user file under the path
`h.shellVar(envDirVar)` returns, install the second version, remove the first,
and assert the file is still there — a test that fails today and passes only once
the data root stops being the garbage-collected `{install_dir}`. The biggest open
question is where the stable data root lands, since that determines whether the
slow real-nvm end-to-end (a new `test/scripts/test-nvm-data-root.sh` wired into
`integration-tests.yml`, asserting `node --version` still runs rather than merely
that `nvm ls` prints something) can run on every PR or has to drop to nightly.
