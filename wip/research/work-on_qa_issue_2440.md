# QA: functional validation of `fix/2440-verify-additional`

Issue: tsukumogami/tsuku#2440 — `[[verify.additional]]` is accepted but never executed.

Result: **7 scenarios run, 7 passed, 0 failed.**

## Environment

- Branch: `fix/2440-verify-additional`
- Binary: `go build -o /tmp/tsuku-qa2440 ./cmd/tsuku` (build exit 0), reports
  `tsuku version v0.12.2-0.20260729033643-b3fb8d615746`
- Isolated homes: `/tmp/qa2440/tsuku`, `/tmp/qa2440b/tsuku`, `/tmp/qa2440c/tsuku`,
  `/tmp/qa2440d/tsuku`, each with `TSUKU_TELEMETRY=0`
- Platform: linux/amd64. Docker available and usable, so the sandbox path was
  exercised for real rather than stubbed.

## Scenario 1 — the issue's own reproduction (automatable)

Copied `recipes/j/jq.toml` and appended the impossible check from the issue:

```toml
[[verify.additional]]
command = "false"
pattern = "THIS_STRING_CANNOT_POSSIBLY_APPEAR"
```

`validate --strict` still accepts the recipe (exit 0), and now lists the check —
the exact omission the issue complained about:

```
Valid recipe: jq
  Actions: github_file, github_file
  Binaries: bin/jq
  Verification: jq --version
  Additional verification: false
validate_exit=0
```

`install --recipe ... --force` now fails, naming the check:

```
Note: Checksums for 'jq' will be computed during installation.
Error: installation verification failed: additional verification 1 failed: false: exit status 1
Output:
install_exit=6
```

The issue's validation script was also run verbatim. Under `set -euo pipefail` it
aborts at the `install` line with exit 6 and the message above; the
`FAIL: impossible additional check passed verification` branch is never reached.
This is the anticipated outcome and is stronger than the script assumed — the
check is caught one step earlier than the script was written to detect.

**PASS** — the impossible check does not pass.

### Sub-finding: standalone `verify jq` after a failed local-recipe install

After the failing install above, `tsuku verify jq` exits 0 and prints
`jq is working correctly`. This is not the additional-check feature failing. The
install was driven by a local file via `--recipe`, but `tsuku verify <tool>`
re-resolves the recipe by name through the loader chain
(`loadRecipeForTool`, `cmd/tsuku/outdated.go:199`), which returns the registry
copy of `jq.toml` — a recipe that has no additional check to run. The registry
cache at `$TSUKU_HOME/registry/j/jq.toml` was confirmed to contain only the
upstream `[verify]` block. That resolution behavior is pre-existing and untouched
by this branch. Scenario 2 tests the standalone path properly by putting the
check where `verify` actually looks.

## Scenario 2 — the standalone `tsuku verify` path (automatable)

Installed jq with a satisfiable additional check (`command = "jq --help"`,
`pattern = "Usage"`): `install_exit=0`, `✅ jq@1.8.2`.

With that same recipe placed at `$TSUKU_HOME/registry/j/jq.toml`, the check runs
and passes:

```
    Running: jq --version
  Additional check 1/1: jq --help
  Additional check passed: Usage
jq is working correctly
verify_exit=0
```

Overwriting the cached recipe with the impossible check makes verify fail:

```
  Step 1: Verifying installation via symlink...
    Running: jq --version
    Output: jq-1.8.2
  Additional check 1/1: false
Verification failed: additional verification 1 failed: false: exit status 1
verify_exit=7
```

The subtler case — command exits 0 but the pattern is absent — also fails, and
the message distinguishes it from a plain command failure:

```
  Additional check 1/1: jq --help
Verification failed: additional verification 1 output does not match expected pattern
  Command: jq --help
  Missing: THIS_STRING_CANNOT_POSSIBLY_APPEAR
verify_exit=7
```

**PASS** — both failure modes are caught on the host path and the message names
the failing check.

## Scenario 3 — validation rejections (automatable)

| Recipe | Exit | Error |
|---|---|---|
| additional entry with no `pattern` | 1 | `verify.additional[0].pattern: pattern is required` |
| additional entry with no `command` | 1 | `verify.additional[0].command: command is required` |
| `[[verify.additional]]` on `type = "library"` (talloc) | 1 | `verify.additional: library recipes do not run verification commands; remove verify.additional` |
| well-formed entry | 0 | lists `Additional verification: jq --help` |

The unmodified `recipes/t/talloc.toml` still validates clean (exit 0), so the
library rejection is triggered by the added field, not by the type check itself.

**PASS.**

Note: each error line is printed twice. This is pre-existing and not a regression
from this branch — a recipe with an empty `verify.command` (code untouched here)
produces the same doubling:

```
Errors:
  - verify.command: command is required
  - verify.command: command is required
```

Cosmetic, out of scope for #2440, worth a separate issue.

## Scenario 4 — `tsuku validate` output (automatable)

Single entry is listed (see Scenario 1). With three entries, all three are listed
in declaration order:

```
Valid recipe: jq
  Actions: github_file, github_file
  Binaries: bin/jq
  Verification: jq --version
  Additional verification: jq --help
  Additional verification: echo SECOND_CHECK
  Additional verification: echo THIRD_CHECK
exit=0
```

Ordering and short-circuit behavior were confirmed on the same recipe, where
entry 2 is unsatisfiable: entry 1 runs and passes, entry 2 fails and is correctly
numbered, entry 3 never runs.

```
  Additional check 1/3: jq --help
  Additional check passed: Usage
  Additional check 2/3: echo SECOND_CHECK
Verification failed: additional verification 2 output does not match expected pattern
  Missing: NOPE_NOT_HERE
  Got: SECOND_CHECK
verify_exit=7
```

**PASS.**

## Scenario 5 — placeholder behavior (automatable)

Installed version is 1.8.2; install dir is `/tmp/qa2440b/tsuku/tools/jq-1.8.2`.

| Case | command / pattern | Observed | Exit |
|---|---|---|---|
| `{version}` in command | `echo VER-{version}` / `VER-1.8.2` | `Additional check 1/1: echo VER-1.8.2`, passed | 0 |
| `{version}` in pattern | `echo VER-1.8.2` / `VER-{version}` | `Additional check passed: VER-1.8.2` | 0 |
| `{install_dir}` in command | `ls {install_dir}/bin` / `jq` | `Additional check 1/1: ls /tmp/qa2440b/tsuku/tools/jq-1.8.2/bin`, passed | 0 |
| `{install_dir}` in pattern | `echo {install_dir}` / `{install_dir}` | fails; `Missing: {install_dir}` reported literally | 7 |

The last row confirms the documented contract: `{install_dir}` is substituted in
the command but deliberately not in the pattern. The pattern reaches the matcher
as the literal string `{install_dir}` and therefore does not match the expanded
path in the output. This is the intended asymmetry (the sandbox resolves the
install dir to a container path that would never appear in host output), so the
failure here is the correct result, not a defect.

**PASS.**

## Scenario 6 — regression check (automatable)

Unmodified `recipes/j/jq.toml`, fresh home:

- `validate --strict`: exit 0, prints `Verification: jq --version` and no
  `Additional verification` line
- `install --recipe ... --force`: exit 0, `✅ jq@1.8.2`
- `verify jq`: exit 0, `jq is working correctly`

**PASS** — recipes without additional checks are unaffected.

## Scenario 7 — sandbox path (environment-dependent: needs Docker)

Run with `install --recipe ... --sandbox --force`. Without `--force` the command
refuses non-interactively (`sandbox testing of local recipe requires interactive
mode`), which is pre-existing behavior.

Satisfiable check:

```
Sandbox test PASSED
sandbox_exit=0
```

Impossible check (`command = "false"`):

```
Sandbox test FAILED
Verification failed with exit code: 0

Verification output:
jq-1.8.2

additional verification failed (exit 1): false

Error: sandbox verification failed with exit code 0
sandbox_exit=6
```

Note the primary verify exit code is 0 — the failure is entirely attributable to
the additional check, and the new "Verification output" block is what makes that
legible. Without it the user would see only "failed with exit code: 0".

Pattern-mismatch case in the sandbox:

```
Sandbox test FAILED
additional verification output missing pattern "NOPE_NOT_HERE": jq --help
Error: sandbox verification failed with exit code 0
sandbox_exit=6
```

**PASS** — the sandbox path enforces additional checks, covering the acceptance
criterion for `internal/sandbox/executor.go`.

## Unit tests

`go test ./internal/recipe/... ./internal/executor/... ./internal/sandbox/... ./cmd/tsuku/...`
— all four packages `ok`.

## Acceptance criteria

| Criterion | Status |
|---|---|
| Executes during `tsuku verify` and sandbox verification | Met (Scenarios 2, 7) |
| Absent pattern fails on host path and sandbox path; `Additional` carried through `PlanVerify` | Met (Scenarios 2, 7; `PlanVerify.Additional` threaded in `plan_generator.go`) |
| A declared check cannot be silently skipped | Met — enforced at install, standalone verify, and sandbox; malformed entries rejected at validation; library recipes rejected |
| Test covers the failing case | Met — branch adds failing-case tests in `verify_test.go`, `validator_test.go`, `executor_test.go`; independently reproduced above |

## Open observations (not blockers)

1. Duplicated validation error lines. Pre-existing, reproduces on untouched
   `verify.command` validation. Separate issue.
2. `tsuku verify <tool>` verifies against the registry recipe, not the local file
   a tool was installed from with `--recipe`. Pre-existing
   (`loadRecipeForTool`). It makes the issue's original script look like it
   passes verification when the local recipe's checks simply were not loaded.
   Not a defect in this change — install-time verification catches the case — but
   it is a sharp edge for recipe authors iterating on a local file.
