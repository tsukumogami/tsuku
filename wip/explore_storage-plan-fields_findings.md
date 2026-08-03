# Explore Findings: storage-plan-fields

Round 1. All findings are from reading the code at `7b95f03b`.

## Lead 1: Is the list of five fields complete?

**No. There are six.** The issue misses `Platform.LinuxFamily`.

`executor.Platform` (`internal/executor/plan.go:122`) carries `OS`, `Arch`, and
`LinuxFamily`. `install.PlanPlatform` (`internal/install/state.go:53`) declares only
`OS` and `Arch`, and `ToStoragePlan` copies only those two. `LinuxFamily` is set by
plan generation when the recipe uses family-specific steps, and
`ComputePlanContentHash` hashes it (`plan_cache.go:119,163`), so it is functional
content by the codebase's own definition -- and it goes over the side with the rest.

Full loss list, executor -> storage:

| Field | Owner |
|---|---|
| `Phase` | `ResolvedStep` |
| `Dependencies` | `InstallationPlan` |
| `Verify` | `InstallationPlan` |
| `RecipeType` | `InstallationPlan` |
| `Binaries` | `InstallationPlan` |
| `LinuxFamily` | `Platform` |

Everything else on `InstallationPlan` and `ResolvedStep` is carried.

## Lead 2: Which read paths consume a stored plan?

`FromStoragePlan` has exactly one call site: `cmd/tsuku/install_deps.go:100`, the
plan cache. Two more paths read `VersionState.Plan` directly without converting:

| Path | What it does | Uses the dropped fields? |
|---|---|---|
| `cmd/tsuku/install_deps.go:100` | plan cache read, then `ExecutePlan` | yes, via the executor |
| `cmd/tsuku/plan.go` `plan show` | prints the stored plan | displays whatever is there |
| `cmd/tsuku/plan.go` `plan export` | writes the stored plan to a JSON file | the file is fed back to `install --plan` |
| `internal/install/state_tool.go:182` | reads `Plan.RecipeSource` for source migration | no |

What consumes each field once a plan is executing:

- `Dependencies` -- `Executor.ExecutePlan` installs the nested plans. On the
  `install --plan` path this is the **only** source of dependencies; the code says so
  in as many words (`cmd/tsuku/plan_install.go:95`: "this path has no recipe loader
  to walk").
- `Verify` -- `internal/sandbox/executor.go:458,684` skip verification entirely when
  it is nil. A silent skip, not an error.
- `RecipeType` -- `cmd/tsuku/plan_install.go:49` builds the minimal recipe's
  `Metadata.Type` from it; `executor.go:824,839,966` branch on `"library"` for
  dependencies. Absent means "tool".
- `Binaries` -- `ExtractBinariesFromPlan` falls back to it when no `install_binaries`
  step exists. `pipx_install` leaves no such step, so for those recipes it is the only
  source of binary names.
- `Phase` -- `StepPhase` falls back to `actions.DefaultPhase(step.Action)`.
- `LinuxFamily` -- provenance and content hashing.

## Lead 3: Does a cache-hit reinstall really execute the lossy plan?

**Not on the ordinary path. The issue's headline claim is overstated.**

`GetCachedPlan` (`internal/install/state_tool.go:142`) returns
`toolState.Versions[version].Plan`. `IsVersionInstalled`
(`internal/install/manager.go:409`) tests `toolState.Versions[version]` for
existence. The second is implied by the first: a cache hit means the version key is
present, so the already-installed short-circuit at `install_deps.go:518` fires and
returns before `ExecutePlan` is ever reached. The only way past it is
`isHiddenTool`, which negates the short-circuit for hidden entries.

So the cached lossy plan is executed on exactly one narrow path: reinstalling a
hidden entry. Everything else short-circuits.

**The real, wide exposure is `tsuku plan export`.** It writes the stored `install.Plan`
straight to a JSON file, documented as "The JSON format matches 'tsuku eval' output"
and intended for offline installation. It does not match, precisely because of these
six fields. Feed that file to `tsuku install --plan` -- the documented workflow -- and
you get an install with no dependencies, no verification, `RecipeType` blank, and no
`Binaries` fallback, on the one path that has no recipe to fall back to. Two commands,
both documented, no hidden entries and no multi-version setup required.

That is the framing the fix and its tests should use.

## Lead 4: What does an absent value mean, field by field?

This splits cleanly, and only half of it needs a backfill answer.

**Zero value is already the correct encoding -- nothing to decide:**

- `Phase`. Empty means "the recipe did not name one, let the action decide", which is
  what `StepPhase` implements and what the field comment on `plan.go:134` documents.
  No recipe in `recipes/` sets `phase` today (only `recipes/README.md` documents it),
  so today every generated plan has `Phase: ""` anyway and the drop is latent. It
  becomes real the first time a recipe names a phase.
- `LinuxFamily`. Empty means "not family-specific", which is what generation means by
  it.
- `Binaries`. Nil falls back to per-step inference, which is correct for every recipe
  that decomposes to `install_binaries`. Degraded, with a working fallback, for the
  rest.

**Zero value is indistinguishable from data loss -- needs a decision:**

- `Dependencies`. Nil means "this tool has no install-time dependencies". A pre-fix
  record says that about every tool.
- `Verify`. Nil means "no verification declared". 1308 of 1449 recipes declare one, so
  nil is almost always a lie about a pre-fix record.
- `RecipeType`. Empty means "tool". A pre-fix record says that about libraries too.

## Lead 5: What structural signal exists?

`state.json` has no schema version, as the brief says. But the stored plan carries
`format_version`, and `ValidateCachedPlan` (`internal/executor/plan_cache.go:58`)
already rejects any plan whose version is not the current `PlanFormatVersion`, with
the cache read logging "Cached plan invalid, regenerating...". The invalidation
machinery exists and is wired up; the only question is whether to trip it.

Cost of tripping it: `PlanFormatVersion` is 5, and 110 golden plan files under
`testdata/golden/plans/embedded/` carry `"format_version": 5`. A bump changes one
integer in each. Nothing else in those files depends on the constant.

Precedent runs both ways, and the comment on `plan.go:20-25` states the criterion the
maintainers actually used: `PlanVerify.Additional` was added without a bump because
"a v5 plan written before it existed decodes with an empty Additional and behaves
exactly as it did." That is a test of whether the old record still behaves correctly.
Here it does not -- an old record behaves as if the tool had no dependencies and no
verification.

`ValidatePlan` accepts any version >= 2, so a bump does not break external plan files.

## Lead 6: Can the restored `Dependencies` serve #2469?

**No.** `InstallationPlan.Dependencies` is install-time only.
`generateDependencyPlans` (`internal/executor/plan_generator.go:754-758`) builds it
from `actions.ResolveDependenciesForTarget(...).InstallTime` and returns nil when that
map is empty; runtime dependencies never enter it. #2469 needs runtime dependencies
with resolved versions -- what `resolveRuntimeDeps` (`cmd/tsuku/install_deps.go:688`)
computes into `InstallOptions.RuntimeDependencies` as a name -> version map, and what
`VersionState` does not persist.

Restoring `Plan.Dependencies` is worth doing on its own merits and leaves #2469
exactly where it was. The PR should say so rather than implying the block is lifted.

## Lead 7: How do you guard a hand-written converter?

Adding six assertions repeats the mistake one size larger. Two structural guards
together close it:

1. A **field census** over `InstallationPlan`, `ResolvedStep`, `Platform`,
   `PlanVerify`, `DependencyPlan` by reflection, asserting the test fixture leaves no
   exported field at its zero value. Add a field to any of those types and this fails,
   naming the field, before anyone has to think about the converter.
2. A **whole-value round trip** -- fixture -> `ToStoragePlan` -> JSON -> back ->
   `FromStoragePlan` -> `reflect.DeepEqual` against the fixture. Once the census
   forces the field into the fixture, this fails unless the converter carries it.

The JSON hop in the middle matters: it is what `state.json` actually does, and it is
what makes a field missing from the *storage* type fail rather than a field missing
from the converter alone.

## Gaps and Open Questions

One question is genuinely contested and the rest follow from it: **what happens on a
read of a plan record written before the fix?** Options identified: accept and run
degraded; bump `PlanFormatVersion` so the existing validator invalidates them; add a
separate storage-schema marker. This needs the decision framework.

## Decision: Crystallize
