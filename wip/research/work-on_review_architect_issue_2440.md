# Architect review: fix/2440-verify-additional

Scope: structural fit, layering, interface contracts, dependency direction.
Correctness, test coverage, and readability are other reviewers' lanes.

Result: 1 blocking, 5 advisory.

---

## BLOCKING

### B1. `PlanFormatVersion` 5 -> 6 without regenerating the golden corpus

`internal/executor/plan.go:22` bumps the constant to 6. Every golden plan file
in the repo still says `"format_version": 5`:

- 110 files under `testdata/golden/plans/**` (checked: all 110 carry 5).
- Plus the R2-published registry corpus, which `scripts/validate-golden.sh`
  can validate against via `TSUKU_GOLDEN_SOURCE=r2`.

`scripts/validate-golden.sh:523-535` generates a fresh plan and compares a
whole-file sha256 after deleting only `generated_at` and `recipe_source`.
`format_version` is inside the compared bytes, so every embedded recipe will
mismatch. `.github/workflows/validate-golden-code.yml` triggers on
`internal/executor/plan.go`, `internal/executor/plan_generator.go`, and
`internal/recipe/types.go` -- all three are touched by this branch -- and it
runs `validate-all-golden.sh --os linux --category embedded`, failing the job
on any mismatch.

This is not a hypothetical: `docs/designs/current/DESIGN-composite-action-checksum-support.md:86-92`
states the same consequence explicitly ("Bumping `format_version` from 5 to 6
forces regeneration of every golden file (155 embedded recipes x 3 platforms
~ 418 files)") and treats it as a cost that must be scoped deliberately.

Two further facts bear on the decision:

1. **The bump buys nothing in production today.** The only production reader
   of `FormatVersion` is `ValidatePlan` (`internal/executor/plan.go:363`),
   which accepts anything `>= 2`. The exact-equality gate that justifies a
   bump -- `ValidateCachedPlan` (`internal/executor/plan_cache.go:58`) -- has
   no non-test caller anywhere in the repo (neither does `CacheKeyWithHash`).
   `ToStoragePlan` copies `FormatVersion` into `install.Plan`, but
   `install.Plan` carries no verify section at all, so nothing downstream
   re-reads it.
2. **No recipe uses `[[verify.additional]]` yet.** A repo-wide search finds
   zero recipes with the field, so there is no already-cached or
   already-published plan that could silently drop additional entries. The
   compatibility hazard the bump protects against does not currently exist.

Also a coordination hazard: `DESIGN-composite-action-checksum-support`
reserves 5 -> 6 for a different change. If both land, `format_version: 6`
means two different things depending on which merged first.

**Resolution (pick one):**

- Drop the bump. The field is additive and `omitempty`, and
  `ComputePlanContentHash` already includes `Additional`
  (`internal/executor/plan_cache.go:147,191,233`), so plans that carry
  additional entries are already distinguishable by content hash whenever the
  cache validator is eventually wired up. This is the cheap option and is
  what the current facts support.
- Or keep the bump and regenerate the full golden corpus in the same PR, and
  reconcile the version number with the composite-action-checksum design.

Either way the current state -- code at 6, corpus at 5 -- cannot merge.

---

## ADVISORY

### A1. Placeholder substitution is documented as a contract but neither enforced nor implemented consistently

The documented rule (`internal/recipe/types.go:862-864` and
`plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md:125-128`)
is: `{version}` substitutes in both fields; `{install_dir}` substitutes in
`command` only, "don't rely on it inside `pattern`".

What the code actually does:

| surface | additional.command | additional.pattern | primary verify command |
|---|---|---|---|
| host (`cmd/tsuku/verify.go:236-241`) | `{version}` + `{install_dir}` | `{version}` + `{install_dir}` | `{version}` + `{install_dir}` (`:300-301`, `:385-386`) |
| sandbox (`internal/sandbox/executor.go:696-702`, `:536`) | `{version}` + `{install_dir}` | `{version}` only | `{install_dir}` only (`:689`) |

Two separate divergences:

- **Host vs sandbox on `pattern`.** The host substitutes `{install_dir}` into
  `additional.pattern`; the sandbox does not. A recipe that uses it passes
  `tsuku verify` locally and fails sandbox CI with a literal-brace pattern
  mismatch. The doc says "don't rely on it", but nothing enforces that -- the
  host implementation actively rewards relying on it. (The identical
  divergence already exists for the primary verify pattern:
  `substitutedVerifyPatterns` substitutes `{install_dir}`,
  `planVerifyPatterns` does not. So this change copies an existing wart
  rather than inventing one -- which is why this is advisory rather than
  blocking -- but it is now the second instance, and the second instance is
  when a wart becomes a pattern.)
- **Inside `buildSandboxScript`.** Two adjacent loops in the same function
  apply different substitution rules to the same kind of string: the primary
  verify command gets `{install_dir}` only, the additional commands get
  `{install_dir}` and `{version}`. The new loop is the more correct of the
  two, but a reader cannot tell which is intentional.

Suggested shape: one `substituteVerifyCommand(cmd, installDir, version)`
helper used by both loops in `buildSandboxScript`, and one decision on
`{install_dir}` in patterns -- either drop it from the host's `subst` closure
in `runAdditionalVerifications` (one line, makes the doc literally true), or
add a validator error for `{install_dir}` appearing in
`verify.additional[N].pattern`. Enforced beats documented for a field
semantics that two execution surfaces must agree on.

### A2. New verification evaluation logic bypasses the established shared evaluator

`internal/executor/plan_verify.go` exists specifically as the shared
evaluation seam -- its doc comment says "Used by both the sandbox and
validate packages to ensure consistent verification behavior". The additional
check's evaluation semantics (exit code must be 0, pattern must appear in
combined output, `{version}` substituted first) are implemented inline in
`internal/sandbox/executor.go:513-542` instead, and independently again in
`cmd/tsuku/verify.go:243-266`.

The marker-file I/O and the failure-message formatting genuinely belong in
the sandbox package, and splitting exit-failure from pattern-failure for
attribution is a legitimate reason not to call `CheckPlanVerification`
verbatim. So this is contained rather than wrong. But the substitution rules
and the pass/fail predicate are exactly the thing `plan_verify.go` was
created to keep in one place; putting them in `internal/executor` and letting
both callers format their own messages would have kept the seam intact.

Root cause worth noting: the canonical host-side verification implementation
lives in `package main` (`cmd/tsuku/verify.go`), where nothing can import it.
That is why every new verification surface has to reimplement the semantics.
Not this change's job to fix, but it is why the divergence in A1 keeps
recurring.

### A3. A third container-verification path remains, running only the primary check

`internal/validate/executor.go:299-351` (`buildPlanInstallScript` +
`checkVerification`, also `source_build.go:201`) is a parallel
container-verification implementation that reads `r.Verify.Pattern`,
`Patterns`, and `ExitCode`, and does not know about `Additional`.

Leaving it is acceptable, but only because it is dead: `validate.NewExecutor`
is instantiated exclusively from `internal/validate/*_test.go`, and
`Validate` / `ValidateSourceBuild` have no non-test callers. The same is true
of `Recipe.ToTOML` -- its only non-test callers are inside that dead path, so
the `[[verify.additional]]` serialization added at
`internal/recipe/types.go:157-164` currently serves nobody (and that
serializer still silently drops `exit_code`, `mode`, `version_format`, and
`reason`, so it was already lossy).

The hazard is that a dead-but-present third verification surface makes every
future verification feature re-answer "do I need to update this too?", and
the answer will keep being "not really, it's dead" until someone revives it
and ships a surface that quietly checks less. Recommend either deleting the
`validate.Executor` container path or marking it deprecated with a tracking
issue, so the next feature does not have to rediscover its status.

### A4. `PlanVerify` -> `recipe.VerifySection` reverse conversion is duplicated three times and each copy is lossy

`internal/executor/executor.go:474-479`, `:482-486`, and `:825-830` each
rebuild a `recipe.VerifySection` from a `PlanVerify`, copying only
`Command`, `Pattern`, `Patterns`. All three drop `ExitCode` (pre-existing)
and now also drop `Additional`.

Harmless today: the only consumers of `ctx.Recipe.Verify` are
`internal/actions/install_binaries.go:109` and
`internal/actions/composites.go:164,486`, and all three read `.Command` only,
for binary-name inference. And the plan-based install path
(`cmd/tsuku/plan_install.go`) does not run verification at all.

But this is precisely the failure mode issue #2440 exists to fix: a field
parsed into a struct that a conversion site forgets to carry. The change adds
a forward helper (`planAdditionalVerify`, `plan_generator.go:20-29`) and uses
it at both forward sites, which is the right instinct -- the reverse
direction should get the same treatment (a single
`(*PlanVerify).ToRecipeVerify()` used by all three call sites) so the next
field cannot be half-threaded.

### A5. The additional-failure description is produced but the default output surface never prints it

`checkAdditionalVerify` builds a precise failure string (which check, exit
code, missing pattern, captured output) and `readVerifyResults` appends it to
`SandboxResult.VerifyOutput`. `emitSandboxHumanReadable`
(`cmd/tsuku/install_sandbox.go:211-248`) never prints `VerifyOutput`; it
prints container stdout/stderr, which will not contain the additional
command's output because that was redirected into marker files. A human sees
"Sandbox test FAILED / Verification failed with exit code: 0" with no
explanation. Only `--json` mode surfaces it
(`install_sandbox.go:198`).

Pre-existing shape -- a primary pattern mismatch already reports "exit code
0" with no output -- so it is not a regression. Noting it because this change
adds a new producer for a channel the default consumer discards, which is the
same "field with no consumer" smell in a different layer. Also note
`SandboxResult.VerifyOutput` is documented as "Combined stdout+stderr of the
verify command" and now sometimes carries host-generated diagnostic text
appended after it; if that field is ever parsed rather than displayed, that
matters.

---

## Things that fit the architecture (no action)

- **`PlanAdditionalVerify` as a separate type from `recipe.AdditionalVerify`
  is correct.** It mirrors the existing `PlanVerify` / `VerifySection`
  split: the plan is a versioned JSON wire format, the recipe is a TOML
  parse target, and keeping them as distinct structs means a recipe-side
  field rename cannot silently change the plan schema. Reusing the recipe
  type here would have been the inconsistent choice. `PlanVerify` already
  duplicates `Command`/`Pattern`/`Patterns`/`ExitCode` for the same reason,
  and the new type follows it exactly -- including correctly omitting an
  `ExitCode` field, since `AdditionalVerify` has no exit-code override.
- **The indexed marker-file pair is the right protocol shape.** It extends
  the existing primary-verify protocol (exit marker + output marker under
  the single read-write `/workspace/output` mount) rather than inventing a
  new channel, and `additionalVerifyMarkers(i)` gives the writer
  (`buildSandboxScript`) and the reader (`checkAdditionalVerify`) a single
  source of truth for the names. Evaluating host-side is also the right
  layer: it keeps the sandbox script a plain recorder, keeps `set +e`
  semantics simple, and keeps the pass/fail decision in testable Go. The
  guards line up too -- both the script generator and `readVerifyResults`
  key off `plan.Verify != nil && Command != ""`, so neither can run without
  the other.
- **`validateDangerousPatterns` gaining a `field` parameter is the right fix
  for attribution.** The function already emitted field-tagged warnings; the
  field was simply hardcoded to `verify.command`. Threading the name keeps
  one dangerous-pattern implementation instead of forking a second one for
  additional entries, and `validateAdditionalVerify` passes
  `verify.additional[N].command`, which matches the field-path convention
  used elsewhere in the validator.
- **Rejecting `verify.additional` on library recipes in the validator**
  (`internal/recipe/validator.go:670-672`) closes the field-accepted-but-never-run
  hole at the layer that owns recipe shape, rather than by adding a runtime
  skip. Correct placement.
- **Dependency direction is clean.** No new imports invert the layering:
  `internal/sandbox` -> `internal/executor` (higher -> lower),
  `internal/executor` -> `internal/recipe`, `cmd/tsuku` -> `internal/*`.
  Nothing lower reaches upward.
