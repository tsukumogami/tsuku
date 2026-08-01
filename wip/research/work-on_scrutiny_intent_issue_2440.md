# Intent scrutiny — issue #2440, commit e1a3fdf5

Reviewed: `e1a3fdf5` on `fix/2440-verify-additional`, against the issue's
load-bearing intent — *a recipe must not be able to end up in a state where a
declared verification check is silently skipped.*

Note on scope: the worktree has **uncommitted modifications** to
`internal/executor/plan_cache.go`, `internal/recipe/validator.go`, and
`internal/sandbox/executor.go` that already close three of the findings below.
The review target is the commit, so those are recorded as blocking against
`e1a3fdf5` with a note that the working tree fixes them. One placeholder gap
survives even in the working tree.

Build, vet, and `go test ./cmd/tsuku ./internal/executor ./internal/recipe
./internal/sandbox` all pass in the current tree.

---

## BLOCKING

### B1. `{version}` / `{install_dir}` are not substituted the same way on the two paths, and the new documentation asserts that they are

The new doc text in
`plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md`
says:

> `{version}` and `{install_dir}` are substituted in both fields.

The new doc comment on `AdditionalVerify` in `internal/recipe/types.go` repeats
the claim ("`{version}` and `{install_dir}` are substituted in both fields").

What the code does:

| | host `cmd/tsuku/verify.go` | sandbox `internal/sandbox/executor.go` |
|---|---|---|
| `command` | `{version}` yes, `{install_dir}` yes (`runAdditionalVerifications`, the local `subst` closure) | `{install_dir}` only (`buildSandboxScript`, `strings.ReplaceAll(a.Command, "{install_dir}", installDir)`) |
| `pattern` | `{version}` yes, `{install_dir}` yes (same closure) | `{version}` only (`checkAdditionalVerify`: `strings.ReplaceAll(a.Pattern, "{version}", plan.Version)`) |

Two concrete divergences at the commit:

- An additional `command` containing `{version}` (e.g. `foo-{version} --help`)
  resolves on the host and runs with a literal `{version}` in the container.
- An additional `pattern` containing `{install_dir}` matches on the host and is
  compared against a literal `{install_dir}` in the container.

Both are cases where a recipe passes one verification path and fails the other
for a reason the author cannot see in the recipe. The documentation contradicts
the code, which is the reporting bar for blocking here.

The uncommitted working tree adds `addCmd = strings.ReplaceAll(addCmd,
"{version}", plan.Version)` in `buildSandboxScript`, closing the command half.
**The pattern half is still open**: `checkAdditionalVerify` still substitutes
only `{version}`, so `{install_dir}` in an additional `pattern` remains a
host-only feature while the doc claims parity.

Two acceptable resolutions: substitute `{install_dir}` in the sandbox pattern
too (the sandbox already computes `installDir` as
`$TSUKU_HOME/tools/<tool>-<version>`, though note the script uses the shell
variable form, so a host-side `strings.ReplaceAll` would need the literal
expansion, not `$TSUKU_HOME/...`); or narrow the doc to state what actually
holds on each path. Note that the *primary* check has the same asymmetry
pre-existing (`planVerifyPatterns` only does `{version}`; `buildSandboxScript`
only does `{install_dir}`), so matching it is defensible — but then the doc has
to say so.

### B2. Library recipes: `[[verify.additional]]` is accepted and never runs

`validateVerify` (`internal/recipe/validator.go:663`) returns early for
`RecipeTypeLibrary`, before `validateAdditionalVerify` is reached.
`internal/recipe/loader.go:738` exempts libraries from requiring
`verify.command`. Neither verification path runs recipe commands for a library:
the host goes through `verifyLibrary` (`cmd/tsuku/verify.go:641`), which does
dlopen / dependency / checksum checks and never executes `Verify.Command`; the
sandbox's `readVerifyResults` returns `(true, -1, "")` immediately when
`plan.Verify == nil || plan.Verify.Command == ""`, and `GeneratePlan` only
constructs a `PlanVerify` when `Command != ""` — so `Additional` is dropped
before it can reach `checkAdditionalVerify`.

At `e1a3fdf5`, therefore, a library recipe carrying `[[verify.additional]]`
validates green and its declared checks never execute. That is the exact state
the issue says must be unreachable, reproduced in a corner the fix didn't
cover. Blocking.

The uncommitted working tree adds an explicit error
(`"library recipes do not run verification commands; remove verify.additional"`)
before the early return. That is the right shape of fix — reject loudly rather
than accept-and-drop.

### B3. Plan content hash does not distinguish plans by their additional checks

At `e1a3fdf5`, `internal/executor/plan_cache.go` was not touched.
`verifyForHash` carries `Command` / `ExitCode` / `Pattern` / `Patterns` and
nothing else, and both `planContentForHashing` and `convertDepsForHashing`
populate only those four. So two plans that differ *only* in their
`[[verify.additional]]` entries produce an identical `ComputePlanContentHash`.
Any consumer that validates a cached plan via `CacheKeyWithHash` +
`ValidateCachedPlan` would accept a stale plan that omits the newly-declared
check — the check would then silently not run, again the exact failure the
issue names.

The uncommitted working tree adds `Additional []PlanAdditionalVerify` to
`verifyForHash` and copies it in both conversion sites. Correct fix. See A1
for why it still doesn't buy much in practice.

---

## ADVISORY

### A1. The production install path never populates `ContentHash`, so the hash is not actually the thing protecting the cache

`cmd/tsuku/install_deps.go:94` builds the key with `executor.CacheKeyFor(...)`,
which leaves `ContentHash` empty, and `ValidateCachedPlan` skips the hash
comparison entirely when the key's hash is `""`
(`internal/executor/plan_cache.go:76`). `CacheKeyWithHash` has no non-test
caller. In practice a cached plan is validated on format version and platform
only.

Consequence for this change: the `PlanFormatVersion` 5 -> 6 bump invalidates
every cached plan exactly once, which covers the rollout. After that, editing a
recipe to *add* an additional check does not invalidate the cached v6 plan for
the same tool/version/platform, so the newly-declared check will not run until
something else forces regeneration (`--fresh`, a version change, another format
bump).

This is pre-existing and equally true of `verify.command` and `pattern` — the
change does not make it worse, and fixing it is out of scope for #2440. But it
means the "cached plan outlives the check it should run" hole is closed for the
rollout, not permanently, and the B3 fix is defensive rather than load-bearing.
Worth a sentence in the commit message or a follow-up issue rather than a
silent assumption that the hash covers it.

### A2. A third verification path still runs only the primary check

`internal/validate/executor.go` has its own container verification:
`buildPlanInstallScript` (:296-346) emits only `r.Verify.Command`, and
`checkVerification` (:333-350) evaluates only `Pattern`/`Patterns`/`ExitCode`
via `CheckPlanVerification`. Additional checks are dropped there exactly as they
were dropped everywhere before this commit.

Mitigating: `Executor.Validate` has no non-test caller — `internal/sandbox`
superseded it (`cmd/tsuku/create.go:710` and `install_sandbox.go:116` construct
`sandbox.NewExecutor`). So this is dormant code, not an active silent skip. But
it is the precise shape of half-built surface a later author trips over: if that
path is ever revived, additional checks vanish again with no signal. Either
delete it or leave a pointer comment next to `buildPlanInstallScript` saying it
does not implement `verify.additional`.

### A3. The "no `exit_code` override" choice is consistent with the host, inconsistent with the sandbox — but stated

`runAdditionalVerifications` fails on any non-zero exit. That matches the host
primary check, which also effectively requires exit 0: `runHiddenToolVerification`
and `runVisibleToolVerification` both do `if err != nil { return ... }` on
`CombinedOutput()` and never read `r.Verify.ExitCode` at all. It does *not*
match the sandbox/validate primary check, which honors `ExitCode` through
`CheckPlanVerification`.

The asymmetry is deliberate and documented in three places (doc reference,
`AdditionalVerify` comment, `runAdditionalVerifications` comment), so it reads
as a stated contract rather than an accident, and the conservative direction
(stricter) is the right default for a check whose whole point is not being
skipped. Noting it mainly because the underlying oddity — the host primary path
silently ignoring `verify.exit_code` — is a separate pre-existing bug that this
change now sits next to and partially masks.

### A4. Substring matching is consistent with the primary check

Both `matchVerifyPatterns` (host) and `CheckPlanVerification` (sandbox/validate)
use `strings.Contains`. `runAdditionalVerifications` and `checkAdditionalVerify`
both use `strings.Contains`. Host trims output before matching, sandbox does
not — same as the primary check on each path. Consistent; no finding.

### A5. `patterns` (the AND-list) is not available on an additional entry

An additional entry supports a single `pattern`. An author who needs "run a
second command and match two strings in its output" has to declare the same
command twice. The doc explicitly steers authors to `patterns` for the
one-command/several-strings case, so it is not a trap, and the validator's
required-`pattern` rule keeps the degenerate form out. Acceptable as-is;
mentioning so a later author who wants `patterns` here knows it was a
deliberate omission and not an oversight.

### A6. Error attribution is right, with one small gap on the host

Host failures are unambiguous — `"additional verification command failed: <cmd>"`
and `"additional verification output does not match expected pattern\n
Command: ... Missing: ... Got: ..."` — clearly distinct from the primary
check's `"verification command failed"` / `"pattern mismatch"` /
`"output does not match expected pattern(s)"`. Sandbox failures carry the entry
index (`"additional verification %d ..."`). Ordering helps too: on both paths
additional checks run only after the primary check has passed
(`readVerifyResults` returns early when `!verified`), so a failure can never be
misattributed across the boundary.

Gap: the host error carries the command text but not the index, so two entries
with the same command and different patterns produce indistinguishable errors.
The index is printed only under `--verbose`. Trivial to add; not worth blocking.

### A7. Sandbox failure detail lives in `VerifyOutput` while `VerifyExitCode` still reports the primary command's code

`readVerifyResults` returns `(false, verifyExitCode, output + "\n" + failure)`
on an additional-check failure, so a caller sees `VerifyExitCode == 0` with
`Verified == false` and the reason only inside the output blob. That is the same
shape a primary pattern mismatch already produces, so it is internally
consistent, and the appended text names the failing check. Fine.

### A8. Marker protocol under partial container abort behaves correctly

`buildSandboxScript` emits `set +e` at the top of the verify block
(`internal/sandbox/executor.go:684`), before both the primary command and the
additional loop. So a failing primary check or a failing additional command does
not abort the script under the file-level `set -e`, and every subsequent marker
pair is still written. Verified by reading the emitted script order:
`set +e` -> primary redirect -> `echo $?` -> per-entry redirect -> `echo $?`.

The abort cases are fail-closed:

- `tsuku install --plan` fails: `set -e` is still active there, the script dies
  before the verify block, no markers exist at all, and `readVerifyResults`
  returns `false` at the primary exit-marker read.
- Container killed midway (timeout, OOM), or an additional command that itself
  calls `exit`: the remaining marker files are absent and
  `checkAdditionalVerify` returns `"additional verification N did not run"`.
- Marker present but unparsable: explicit
  `"produced an unreadable exit code"` failure rather than a default-zero.

The "no evidence a check ran is not evidence it passed" stance is implemented
as written and is the correct reading of the issue's intent. No finding.

---

## Verdict on the intent

The direction is right and the reasoning in the commit message for implementing
rather than removing is sound (no parse path checks `MetaData.Undecoded()`, so
deleting the field would have converted a silent skip into a silently discarded
key — strictly worse). Zero registry recipes use the field, so there is no
migration surface.

With B1's remaining half, B2, and B3 closed, the change is a sufficient
foundation. Left open, it does not fully satisfy the issue's own acceptance
criterion that "a recipe cannot end up in a state where a declared verification
check is silently skipped" — the library-recipe case (B2) is a live instance of
exactly that state, and the doc/code split (B1) means an author can write a
recipe that verifies on one path and not the other with no signal.
