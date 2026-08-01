# Completeness scrutiny — issue #2440, commit e1a3fdf5

Reviewer focus: does every acceptance criterion have a corresponding implementation,
and are the claims verifiable from the diff?

## Acceptance criteria walk-through

| AC | Status | Evidence |
|----|--------|----------|
| 1. `[[verify.additional]]` executes during `tsuku verify` and sandbox verification (or is removed) | Met, with one carve-out (see B2) | `cmd/tsuku/verify.go:329` (hidden) and `:408` (visible) call `runAdditionalVerifications`; `internal/sandbox/executor.go:696-704` emits per-entry markers, `:508-541` evaluates them |
| 2. Absent pattern fails on host path, sandbox path, and `Additional` carried through `PlanVerify` | Met | `cmd/tsuku/verify.go:262-265`; `internal/sandbox/executor.go:534-537`; `internal/executor/plan.go:104-113` + `plan_generator.go:304, 835` |
| 3. If removed: validate rejects | N/A — implement direction chosen | Commit message argues the removal direction is unavailable because no parse path checks `MetaData.Undecoded()`; I verified `toml.Unmarshal` use in `internal/recipe/loader.go`, the claim holds |
| 4. No recipe can end up with a silently skipped declared check | **Not met for library recipes** | See B2 |
| 5. A test covers the failing case | Met | Host: `cmd/tsuku/verify_test.go` cases "command exits non-zero", "pattern absent from output", "first entry passes, second fails". Sandbox: `internal/sandbox/executor_test.go` `TestReadVerifyResults_AdditionalNonZeroExit`, `_AdditionalPatternMissing`, `_AdditionalMarkersMissing`. Validator: both missing-field cases. All pass locally (`go test ./internal/recipe ./internal/executor ./internal/sandbox ./cmd/tsuku -run 'Additional|Verify|FormatVersion|CachedPlan'` → ok) |

Search for other consumers of a recipe's verify section (grep for `.Verify`,
`PlanVerify`, `CheckPlanVerification` across every non-test `.go` file) turned up
six additional sites; each is triaged below.

## BLOCKING

### B1. The issue's own Validation script now fails — install aborts before the verify step

The script in the issue runs under `set -euo pipefail`:

```
/tmp/tsuku-addv install --recipe "$recipe" --force
if PATH=... /tmp/tsuku-addv verify jq > /dev/null 2>&1; then ...
```

Post-install verification is not new: `cmd/tsuku/install_deps.go:641-663`
(`installWithDependencies`) calls `RunToolVerification` whenever the recipe has a
verify command, and returns `installation verification failed: ...`. `--recipe`
routes there via `runRecipeBasedInstall` → `runInstall` (`cmd/tsuku/install.go:143,
475-503`), and the error reaches `handleInstallError` → `exitWithCode` (non-zero).

With this commit, the impossible additional check now fires *during install*, so
`tsuku install --recipe jq-addv.toml --force` exits non-zero, `set -e` aborts the
script, and it never reaches the `verify` call or prints
`PASS: additional check enforced`. The script's exit status is a failure.

The behavior is correct — failing earlier is strictly better. What is wrong is the
evidence gate: whoever runs the issue's validation block verbatim gets a red result
for a change that works. Resolution is script-level, not code-level: run the install
as `/tmp/tsuku-addv install --recipe "$recipe" --force || true` (or drop `set -e`
around it) and keep the `verify` assertion as the gate, then record the amended run
in the PR. This must be resolved before the issue can be shown as validated.

### B2. Library recipes still silently skip declared additional checks (AC 4)

`validateVerify` returns early for `type = "library"` (`internal/recipe/validator.go:664-666`),
before `validateAdditionalVerify` runs. Library recipes are exempt from the
"verify command is required" rule (`internal/recipe/loader.go:738`,
`internal/recipe/validate.go:50`), and library verification goes through
`verifyLibrary` (`cmd/tsuku/install_lib.go:187`), which checks sonames and files and
never looks at `Verify.Additional`. Plan generation also gates on
`Verify.Command != ""` (`internal/executor/plan_generator.go:298`), so the sandbox
path drops it too.

Confirmed empirically with a throwaway test in `internal/recipe` (removed
afterwards): a library recipe with

```toml
[verify]

[[verify.additional]]
command = "false"
pattern = ""
```

validates clean (`valid=true errors=[]`) and nothing ever runs the check — the exact
trap described in the issue, with `type = "library"` added. AC 4 says "a recipe
cannot end up in a state where a declared verification check is silently skipped";
today it still can.

Blast radius is small (no library recipe in the registry uses the field), and the
fix is small too: in `validateVerify`, run `validateAdditionalVerify` before the
library early-return, and reject `verify.additional` on library recipes outright
(nothing can execute it) rather than accepting it. A test asserting the rejection
would close the criterion.

## ADVISORY

### A1. `plan.go` claims the plan-based install path runs the entries; it does not

`internal/executor/plan.go:100-104`: "Additional carries the recipe's
`[[verify.additional]]` entries so the plan-based install path and the sandbox can
run them." `tsuku install --plan` (`cmd/tsuku/plan_install.go`) performs no
verification at all — there is no `RunToolVerification` call anywhere in that path
(only `cmd/tsuku/verify.go:1158` and `cmd/tsuku/install_deps.go:659` call it). The
only consumer of `PlanVerify.Additional` in the tree is
`internal/sandbox/executor.go`. Reword to name the sandbox alone; the claim as
written is not supported by the diff.

### A2. `internal/validate` container path never runs additional checks

`internal/validate/executor.go:299-330` (`buildPlanInstallScript`) appends only
`r.Verify.Command` to the container script, and `checkVerification` (`:334-351`)
pattern-matches only `Pattern`/`Patterns`. `internal/validate/source_build.go:201`
shares `checkVerification`. Both also copy the recipe into the container via
`r.ToTOML()`, which drops the field entirely (see A3). A recipe with an impossible
additional check would pass this validator.

Downgraded from blocking because `Executor.Validate` and `ValidateSourceBuild` have
no non-test callers anywhere in the repo (verified by grep across `cmd/` and
`internal/`) — the sandbox package superseded them. Still worth either wiring the
additional entries in or noting the path as dead.

### A3. `Recipe.ToTOML` drops `[[verify.additional]]`

`internal/recipe/types.go:139-155` hand-writes the verify section and emits only
`command`, `pattern`, `patterns` — additional entries (and `exit_code`, `mode`,
`reason`) are lost on round-trip. Consumers are the two `internal/validate` sites in
A2, so it is not user-reachable today, but it is the same class of silent-drop bug
the issue is about. `WriteRecipe` (`internal/recipe/writer.go:30-37`) uses the TOML
encoder on the whole struct and is fine.

### A4. Plan content hash ignores `Additional`

`verifyForHash` (`internal/executor/plan_cache.go:144-150`) and both conversion
sites (`:187-193`, `:228-235`) omit the new field, so two plans differing only in
their additional checks hash identically. The format-version bump to 6 invalidates
today's caches once, but a later recipe edit that only touches
`[[verify.additional]]` produces a plan the content hash cannot distinguish from the
stale one. In practice `cmd/tsuku/install_deps.go:94` builds the cache key without a
content hash at all, so this is latent rather than active — but it is one more place
the new field was not threaded.

### A5. Host and sandbox substitute placeholders differently

Host `runAdditionalVerifications` substitutes both `{version}` and `{install_dir}`
in both the command and the pattern. The sandbox substitutes `{install_dir}` in the
command only (`internal/sandbox/executor.go:698`) and `{version}` in the pattern
only (`:534`). So an additional check using `{version}` in its command, or
`{install_dir}` in its pattern, behaves differently on the two paths — the sandbox
would run/match a literal placeholder. This mirrors the pre-existing asymmetry for
the primary check, so it is not a regression, but the doc added by this commit
(`plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md`)
states "`{version}` and `{install_dir}` are substituted in both fields", which is
only true on the host. Either fix the sandbox substitution (two `strings.ReplaceAll`
calls) or qualify the doc.

### A6. The plan-wiring test can silently skip

`TestGeneratePlan_CarriesAdditionalVerify` (`internal/executor/plan_generator_test.go`)
calls `t.Skipf` when `GeneratePlan` errors, and the recipe uses the `nodejs_dist`
version source, which resolves over the network. It passes here (verified with
`-run ... -v`), but in an offline CI runner it degrades to a skip, leaving
`TestPlanAdditionalVerify` — a pure converter test — as the only guard on the wiring.
The dependency variant (`generateSingleDependencyPlan`, `plan_generator.go:835`) has
no test at all. Consider a static-source recipe so the test cannot skip.

### A7. Two more places the field is dropped, harmlessly

`internal/executor/executor.go:473-486` (plan-fallback recipe context) and
`:824-829` (dependency recipe) rebuild a `recipe.VerifySection` field by field
without `Additional`. Both feed only `actions.ExecutionContext.Recipe`, where the
verify command is read for binary-name inference (`internal/actions/install_binaries.go:109`,
`internal/actions/composites.go:164, 486`) — never for verification. No behavior
issue; worth a comment or a shared converter so the next field addition does not
have to rediscover that these sites are inert.

## Things checked and found correct

- `set -e` in the sandbox script is disabled (`set +e`) before the verify block, so a
  failing additional command cannot abort the script and the markers are always
  written (`internal/sandbox/executor.go:684`).
- Missing markers are treated as a failure, and the primary-check failure path
  returns before the additional block so failures stay attributable — both covered by
  tests.
- `cmd/tsuku/validate.go:184-186` lists the additional commands in the summary, which
  closes the "the additional check isn't even listed" observation from the issue.
- The validator rejects an entry missing either field, with tests for both, and a
  positive test that a well-formed entry validates.
- `PlanFormatVersion` bump to 6 is matched in `TestFormatVersionConstant`, and
  `plan_cache_test.go` now derives the expected message from the constant.
- `go build ./...` clean; targeted tests in the four touched packages pass.
