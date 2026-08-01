# Pragmatic review — fix/2440-verify-additional

Scope reviewed: all non-wip changes on the branch (host verify runner, plan type +
generator, plan content hash, plan format version, sandbox script + marker
evaluation, recipe validator, ToTOML, validate command output, docs, tests).

Verdict on proportionality: the core is proportional. ~120 lines of non-test code
to take a parsed-but-dead field and make it run on both execution paths (host
install verification and sandbox marker verification) is about right. Five of the
seven touched surfaces are load-bearing. Two-plus are not.

---

## BLOCKING

### B1. Dependency plans carry `additional` that nothing ever runs

`internal/executor/plan_generator.go:835` populates `DependencyPlan.Verify.Additional`,
and `internal/executor/plan_cache.go:233` hashes it. No consumer reads it:
`internal/sandbox/executor.go:514` only walks the top-level `plan.Verify.Additional`,
and the only place a `DependencyPlan.Verify` is turned back into a runnable
`recipe.VerifySection` — `internal/executor/executor.go:824-830` — drops the field
(it copies Command/Pattern/Patterns only). Nothing in the executor executes a
dependency's verify command at all; `depRecipe.Verify` exists purely as context for
binary-name inference.

Net effect: a dependency recipe declaring `[[verify.additional]]` gets those entries
serialized into plan.json, where a reader will reasonably assume they run. They
never do — the exact silent-skip class of bug this issue exists to close, just moved
one level down.

Fix: drop line 835 (and the `Additional:` line in the dep branch of
`planContentForHashing`, plan_cache.go:233). One line each. Do not "wire it up" —
that would mean building a dependency verification path that does not exist today.

---

## ADVISORY

### A1. `tsuku validate` output listing additional commands — scope creep
`cmd/tsuku/validate.go:180-187`. Printing each additional command in the validate
summary is cosmetic and not needed for the field to execute. Three lines, inert.
Keep or drop; it does not compound.

### A2. `validateDangerousPatterns` field-attribution refactor
`internal/recipe/validator.go:734` gained a `field` parameter so warnings can point
at `verify.additional[0].command`. Threading the parameter touches ~10 call sites in
the function. This is polish beyond "make the field run" — the pre-existing behavior
would have mislabeled the warning, not dropped it. Small and correct; noted only as
added surface.

### A3. Library rejection is a new hard validation error for a shape nobody uses
`internal/recipe/validator.go:670-672` turns `[[verify.additional]]` on a library
recipe into an error. No registry recipe uses `verify.additional` at all (grepped
`recipes/`: zero hits), so this rejects only hypothetical future or third-party/tap
recipes. It is defensible — it closes the same silent-skip trap for the library path
— but it is added scope relative to "implement the field", and it makes previously
valid recipes invalid. Keep if you want the guarantee; be aware it is a validation
behavior change riding along with the fix.

### A4. Doc comment contradicts the host implementation on `{install_dir}`
`internal/recipe/types.go:862-863` states `{install_dir}` is substituted in Command
only. `cmd/tsuku/verify.go:237-245` substitutes it in both Command and Pattern (the
`subst` closure is applied to each), while the sandbox path
(`internal/sandbox/executor.go:536`) substitutes only `{version}` in the pattern. So
a recipe using `{install_dir}` in an additional pattern passes on the host and fails
in the sandbox. Pick one: either stop substituting `{install_dir}` in the host
pattern (matches the doc), or substitute it in the sandbox too and fix the comment.

### A5. Test redundancy (three small items)
- `internal/executor/plan_generator_test.go:501` `TestPlanAdditionalVerify` unit-tests
  a 6-line mapper that `TestGeneratePlan_CarriesAdditionalVerify` (line 560) already
  exercises end to end. Note the end-to-end one `t.Skipf`s on any GeneratePlan error
  (network), so it can silently no-op in CI — if you keep only one, keep the unit test.
- `internal/sandbox/executor_test.go:804`
  `TestBuildSandboxScript_AdditionalVerifyVersionSubstitution` is two asserts that
  belong inside `TestBuildSandboxScript_AdditionalVerify` (line 776), which already
  builds a script from the same helper.
- `internal/recipe/validator_test.go:480`
  `TestValidateBytes_LibraryWithoutAdditionalStillValid` guards the pre-existing
  library early-return, which this change does not restructure.

Overall the test set (~20 functions) is generous but each failure mode is real; only
the above overlap.

---

## Explicitly NOT findings

- **PlanFormatVersion 5 -> 6 is warranted.** `plan_cache.go:58` rejects any cached
  plan whose format version differs, so without the bump a plan cached before this
  change would be served with no `additional` field and the checks would silently not
  run — reintroducing the bug for the cache's lifetime. One-line change, and v5 set
  the same precedent for `ExitCode` (#1942).
- **`verifyForHash.Additional`** (plan_cache.go:147, 191) is necessary for the
  top-level plan: two v6 plans differing only in their additional checks must not
  share a cache identity. (The dep-level copy is B1.)
- **ToTOML change** (`internal/recipe/types.go:157-164`) is real, not speculative:
  `internal/validate/source_build.go:127` and `internal/validate/executor.go:211`
  serialize the recipe into the validation workspace, so dropping the entries there
  would hand the container a recipe with the checks removed.
- **`planAdditionalVerify` helper** has two callers; not a single-caller wrapper.
- **Docs** (`plugins/tsuku-recipes/.../verification-reference.md:109-141`) — a schema
  field that starts executing needs its reference updated. In scope.
