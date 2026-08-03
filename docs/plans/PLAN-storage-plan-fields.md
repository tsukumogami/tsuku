---
schema: plan/v1
status: Draft
execution_mode: single-pr
upstream: docs/designs/current/DESIGN-storage-plan-fields.md
milestone: "Plan storage fidelity"
issue_count: 5
---

# PLAN: Carry every plan field into state.json

## Status

Draft

## Scope Summary

Stop `executor.ToStoragePlan` dropping six fields on the way into `state.json`, guard
the converter so the next field addition fails a test instead of shipping, and give
records already on disk a defined meaning at both read sites.

Closes tsukumogami/tsuku#2468.

## Decomposition Strategy

**Vertical, single PR.** Every issue touches the same three files or their direct
consumers, and issues 2 through 5 are meaningless without issue 1's types. Splitting
across PRs would land a schema change with no writer, then a writer with no reader.
The sequence is strict.

## Issue Outlines

### Issue 1: fix(install): mirror the dropped plan fields onto the storage types

**Complexity**: simple

**Goal**: Add to `internal/install/state.go` the fields `install.Plan` needs to hold a
whole executor plan: `StorageVersion`, `Dependencies`, `Verify`, `RecipeType`,
`Binaries` on `Plan`; `LinuxFamily` on `PlanPlatform`; `Phase` on `PlanStep`. Add the
mirror types `PlanDependency`, `PlanVerify`, `PlanAdditionalVerify`. Add the
`PlanStorageVersion` constant.

**Acceptance Criteria**:
- `install.Plan` has `StorageVersion int`, `Dependencies []PlanDependency`,
  `Verify *PlanVerify`, `RecipeType string`, `Binaries []string`
- `install.PlanPlatform` has `LinuxFamily string`; `install.PlanStep` has `Phase string`
- `PlanDependency` mirrors `executor.DependencyPlan`; `PlanVerify` mirrors
  `executor.PlanVerify`; `PlanAdditionalVerify` mirrors `executor.PlanAdditionalVerify`
- Every new field carries `omitempty`, so a plan with none of them serializes byte for
  byte as it does today
- `PlanStorageVersion` is 1, with a doc comment distinguishing it from
  `Plan.FormatVersion`
- `go build ./...` passes

**Dependencies**: None

---

### Issue 2: fix(executor): carry all six fields through the plan converters

**Complexity**: testable

**Goal**: Make `ToStoragePlan` and `FromStoragePlan` in
`internal/executor/plan_conversion.go` lossless. Convert steps with `Phase`, platform
with `LinuxFamily`, the verify block, the recipe type, the binaries, and the dependency
tree recursively. Stamp `StorageVersion` on the way out.

**Acceptance Criteria**:
- `ToStoragePlan` copies `Phase`, `LinuxFamily`, `Dependencies` (recursively), `Verify`
  (including `Additional` and `ExitCode`), `RecipeType`, `Binaries`
- `FromStoragePlan` restores every one of them
- `ToStoragePlan` sets `StorageVersion: install.PlanStorageVersion`
- Nil `Verify` stays nil in both directions; empty `Dependencies` stays empty, not an
  empty non-nil slice
- Nested dependencies at two levels deep round-trip intact
- `go test ./...` passes

**Dependencies**: <<ISSUE:1>>

---

### Issue 3: test(executor): guard the converter against the next dropped field

**Complexity**: testable

**Goal**: Replace the field-by-field round-trip test in
`internal/executor/plan_conversion_test.go` with the two structural guards: a
reflection census asserting the fixture leaves no exported field zero, and a
whole-value round trip through JSON compared with `reflect.DeepEqual`.

**Acceptance Criteria**:
- A census walks `InstallationPlan`, `ResolvedStep`, `Platform`, `PlanVerify`,
  `DependencyPlan`, `PlanAdditionalVerify` by reflection and fails naming any exported
  field the fixture leaves at its zero value
- The round trip goes fixture -> `ToStoragePlan` -> `json.Marshal` -> `json.Unmarshal`
  -> `FromStoragePlan` and compares to the fixture with `reflect.DeepEqual`
- Adding a field to any covered executor type fails the census; populating it without
  updating the converter fails the round trip
- Both failure modes are demonstrated by mutation and recorded in the PR body
- The nil-input cases from the existing tests survive
- `go test ./...` passes

**Dependencies**: <<ISSUE:2>>

---

### Issue 4: fix(cli): give a pre-fix stored plan a defined meaning at both read sites

**Complexity**: testable

**Goal**: Treat a stored plan whose `StorageVersion` is below current as untrusted.
`getOrGeneratePlanWith` (`cmd/tsuku/install_deps.go`, the plan-cache read) skips it and
regenerates. `tsuku plan show` and `tsuku plan export` still work but warn on stderr
that the plan predates the fix and may omit dependencies and verification.

**Acceptance Criteria**:
- A cached plan with `StorageVersion` 0 does not satisfy the cache read; the plan is
  regenerated
- A cached plan at the current `StorageVersion` is used as before
- `plan show` and `plan export` on a pre-fix record write a warning to stderr naming
  `tsuku install <tool> --fresh`, and still produce their output on stdout
- `plan export` exit code is unchanged by the warning
- The change stays in the cache-read region of `install_deps.go` and does not touch
  the already-installed short-circuit
- `go test ./...` passes

**Dependencies**: <<ISSUE:2>>

---

### Issue 5: test(cli): cover plan export into plan-based install

**Complexity**: testable

**Goal**: Prove the user-visible break is closed: a plan stored by an install, exported,
and reinstalled with `--plan` carries its dependencies, verify block, recipe type, and
binaries. This is the acceptance test for the whole change; the unit round trip proves
the converter, this proves the path.

**Acceptance Criteria**:
- A test builds an `InstallationPlan` with a dependency, a verify block, a non-empty
  `RecipeType`, and `Binaries`; stores it through `ToStoragePlan` into a `state.json`
  under `t.TempDir()`; reads it back the way `plan export` does; and asserts the
  decoded plan still has all four
- The assertion is on what a plan-based install would consume, not on the presence of a
  JSON key
- Test writes stay inside `t.TempDir()` so the working tree is clean after the run
- Not gated behind `testing.Short()`
- `go test ./...` passes

**Dependencies**: <<ISSUE:2>>, <<ISSUE:4>>

---

## Plugin skill assessment

Required by the root `CLAUDE.md` plugin-maintenance table, since this change touches
`internal/executor/` and `cmd/tsuku/`. Assess `plugins/tsuku-recipes/` and
`plugins/tsuku-user/` against the diff and update whatever the change makes wrong or
newly undocumented. Record the assessment in the PR body either way -- "no skill
documents this surface" is an outcome, not a skipped step.

## Implementation Sequence

1. Issue 1 -- types, no behavior.
2. Issue 2 -- the fix itself.
3. Issues 3, 4 in either order.
4. Issue 5 -- needs 4 for the warning path.
5. Plugin skill assessment, then verification and PR.
