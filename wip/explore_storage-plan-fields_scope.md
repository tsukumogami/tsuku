# Explore Scope: storage-plan-fields

## Visibility

Public

## Core Question

`executor.ToStoragePlan` copies a subset of `InstallationPlan` into `install.Plan`,
so the plan written to `state.json` is not the plan that was generated. What is
actually lost, what reads the lossy record, and what does an entry already on disk
mean once the missing fields exist?

## Context

Issue tsukumogami/tsuku#2468 names five dropped fields: `ResolvedStep.Phase` and the
plan-level `Dependencies`, `Verify`, `RecipeType`, `Binaries`. The converter is a
hand-written field list on both sides, with no structural guard, and the existing
round-trip test asserts field-by-field over exactly the fields that are carried --
which is why the loss went unnoticed.

There is no schema version on `state.json` itself; its migrations are detected
structurally (`Version != "" && ActiveVersion == ""`). The stored plan does carry
`format_version`, and `ValidateCachedPlan` already gates on it.

## In Scope

- `internal/executor/plan_conversion.go`, `internal/executor/plan.go`,
  `internal/install/state.go` (the storage plan types)
- `internal/executor/plan_conversion_test.go` (the guard against recurrence)
- The read paths that consume a stored plan
- What an entry written before the fix means

## Out of Scope

- tsukumogami/tsuku#2469 (wrapper scripts on activate/rollback) and every other
  open sibling
- The already-installed short-circuit in `cmd/tsuku/install_deps.go` (#2463's region)
- Changing what plan generation puts in a plan

## Research Leads

1. **Is the list of five fields complete?**
   The converter is a hand-written field list. If it missed five, it may have missed
   more.

2. **Which read paths consume a stored plan, and what does each one use the missing
   fields for?**
   The issue points at the install-time plan cache. That is one call site; there may
   be others with different blast radius.

3. **Does a cache-hit reinstall really execute the lossy plan?**
   The issue's central claim. `getOrGeneratePlan` returns the cached plan, but what
   runs between that return and `ExecutePlan` decides whether the claim holds.

4. **What does an absent value mean for each field, field by field?**
   Some zero values are the documented correct encoding; some are indistinguishable
   from real data loss. Only the second group needs a backfill answer.

5. **What structural signal exists to tell a pre-fix stored plan from a post-fix one?**
   The brief flags this as the hard question. `state.json` has no schema version, but
   the plan record may carry something usable.

6. **Can the restored `Dependencies` field serve #2469?**
   #2469 is blocked on runtime dependencies not being persisted and names the stored
   plan as the likely source.

7. **How do you make a hand-written converter fail a test when a future field is
   added and forgotten?**
   Adding five assertions to the existing test fixes today and not tomorrow.
