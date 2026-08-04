---
status: Current
spawned_from:
  issue: 2468
  repo: tsukumogami/tsuku
problem: |
  executor.ToStoragePlan copies a hand-written subset of InstallationPlan into
  install.Plan, so the plan written to state.json is not the plan that was
  generated. Six fields go over the side: ResolvedStep.Phase, Platform.LinuxFamily,
  and the plan-level Dependencies, Verify, RecipeType, and Binaries. The widest
  consequence is tsuku plan export, which writes the stored plan to a file
  documented as matching tsuku eval output and usable with tsuku install --plan;
  that file installs without dependencies and without verification, on the one
  path that has no recipe to fall back to.
decision: |
  Mirror the six fields onto the install storage types and carry them in both
  directions. Guard the converter structurally with a reflection field census plus
  a whole-value JSON round trip, so a field added to the executor plan types in
  future fails a test before it can be forgotten. Give records already in
  state.json a defined meaning via a StorageVersion marker on install.Plan: the
  plan cache treats a pre-fix record as a miss and regenerates, and plan show and
  plan export keep working but warn that the stored plan may omit dependencies and
  verification.
rationale: |
  The fields are load-bearing on the plan-based install path, where the plan is
  the only source of truth. A structural guard is the only kind that survives the
  next field addition; six more assertions repeat the mistake one size larger.
  StorageVersion covers both read sites and costs no golden-file churn, where
  bumping PlanFormatVersion would change 110 golden files, claim the executor plan
  format changed when it did not, and still only cover the less reachable of the
  two read paths.
---

# DESIGN: Carry every plan field into state.json

## Status

Current

## Context and Problem Statement

`executor.ToStoragePlan` and `executor.FromStoragePlan` (`internal/executor/plan_conversion.go`) convert between the executor's `InstallationPlan` and the `install.Plan` that gets written into `state.json`. Both are hand-written field lists, and both lists are short.

Six fields never make the trip:

| Field | Owner | What it means |
|---|---|---|
| `Dependencies` | `InstallationPlan` | nested install-time dependency plans |
| `Verify` | `InstallationPlan` | the recipe's verification command and patterns |
| `RecipeType` | `InstallationPlan` | `"tool"` or `"library"` |
| `Binaries` | `InstallationPlan` | binary names, mirrored from recipe metadata |
| `Phase` | `ResolvedStep` | lifecycle phase named by the recipe |
| `LinuxFamily` | `Platform` | the Linux family a family-specific plan targets |

Issue #2468 names the first five. `Platform.LinuxFamily` is the sixth: `install.PlanPlatform` declares only `OS` and `Arch`. It is functional content by the codebase's own definition, since `ComputePlanContentHash` hashes it.

`FromStoragePlan` cannot restore what was never written, so a plan read back out of `state.json` is a plan with no dependency tree, no verification block, no recipe type, no binary list, no phases, and no Linux family.

### Where this actually bites

The issue frames the consequence as a cache-hit reinstall executing a lossy plan. That framing is overstated, and it matters, because it points the tests at the wrong path.

`GetCachedPlan` returns `toolState.Versions[version].Plan`, and `IsVersionInstalled` tests `toolState.Versions[version]` for existence. The second is implied by the first. So on the ordinary install path a cache hit guarantees the already-installed short-circuit fires and returns before `ExecutePlan` is reached. The only way past it is a hidden entry.

The wide exposure is elsewhere and needs no hidden entries or multi-version setup:

```
tsuku plan export gh -o gh.json
tsuku install --plan gh.json
```

`plan export` writes the stored `install.Plan` straight out as JSON. Its help text says "The JSON format matches 'tsuku eval' output" and offers the file for offline installation. It does not match, precisely because of these six fields. `runPlanBasedInstall` then reads that file into an `InstallationPlan` and, in its own words, has "no recipe loader to walk" -- `plan.Dependencies` is the only source of dependencies it has, `plan.Verify` is the only verification it has, `plan.RecipeType` decides whether the tool is treated as a library, and `plan.Binaries` is the fallback that names the executables for recipes like `pipx_install` that leave no `install_binaries` step.

Two documented commands, and the second installs something different from what the first described.

### Why it was never caught

`internal/executor/plan_conversion_test.go` round-trips a plan and asserts field by field. Its assertion list is the same list as the converter's, so the test agrees with the bug. A field the converter forgets is a field the test never mentions.

## Decision Drivers

- **The plan-based install path has no fallback.** Everywhere else a missing field degrades into a recipe lookup. There, the plan is all there is.
- **Six more assertions do not fix this class.** The defect is a hand-written list with no structural check. The next field to be added will be forgotten the same way unless something fails on its own.
- **`state.json` has no schema version.** Its migrations are detected structurally. Whatever is added has to have a defined meaning for records already on disk.
- **`internal/executor` imports `internal/install`.** The storage types cannot reference the executor types; they have to mirror them, which is already the established pattern for `PlanPlatform` and `PlanStep`.
- **Golden plans are eval output.** They serialize `executor.InstallationPlan`. Adding fields to `install.Plan` does not touch them; changing `PlanFormatVersion` changes all 110.

## Considered Options

### Decision 1: What a read of a pre-fix record means

Once `install.Plan` carries `Dependencies`, `Verify`, and `RecipeType`, every record already in `state.json` decodes with all three at their zero value -- which is also what a legitimately dependency-free, verification-free plan looks like. The two are indistinguishable.

Only those three raise the question. `Phase` is documented as "empty means the recipe did not name one, let the action decide", which is exactly what `StepPhase` implements. `LinuxFamily` empty means "not family-specific". `Binaries` nil falls back to per-step inference. Those three zero values are already correct.

#### Chosen: a `StorageVersion` marker on `install.Plan`

`install.Plan` gains `StorageVersion int`, written by `ToStoragePlan` as `install.PlanStorageVersion` (1). A record written before this change has no such key and decodes as 0.

Each read site gets a defined behavior:

| Read site | Behavior when `StorageVersion < PlanStorageVersion` |
|---|---|
| plan cache, `getOrGeneratePlanWith` | treat as a cache miss, regenerate |
| `tsuku plan show`, `tsuku plan export` | proceed, and warn on stderr that the stored plan predates the fix and may omit dependencies and verification, naming `tsuku install <tool> --fresh` as the refresh |
| `tsuku install --plan` (added by #2496) | proceed, and warn that the plan file predates the format that carries every field, naming `tsuku eval <tool>` as the way to produce a current one |

The third row is the follow-up. Marking only `install.Plan` covered reads of `state.json` and not a plan file already on disk, because `loadPlanFromSource` decodes into `executor.InstallationPlan`, which had no field to receive the key. #2496 put the marker on the executor plan too, stamped at plan generation so `tsuku eval` output carries it.

`StorageVersion` versions the conversion into `state.json`. `FormatVersion`, already on the same struct, versions the plan itself. The doc comments say which is which, because two version numbers on one struct is a readability cost that has to be paid down explicitly.

#### Alternatives considered

- **Accept and run degraded.** Define absent as "none" and let pre-fix records mean it. Free, and it answers the issue's fifth acceptance criterion by fiat. Rejected because it leaves every already-installed tool exporting an incomplete plan indefinitely -- a pinned tool's entry is not rewritten until someone reinstalls it -- with no signal to the user. The bug survives the fix on every machine that has tools installed today.

- **Bump `PlanFormatVersion` 5 -> 6.** `ValidateCachedPlan` already rejects a version mismatch, so pre-fix records would fail validation and regenerate with no new machinery. Rejected on coverage and cost. `ValidateCachedPlan` is called only from the plan cache, which is the less reachable of the two read paths; `plan show` and `plan export` read `VersionState.Plan` raw and never see it. And the bill is 110 golden files plus a version-history entry claiming the executor plan format changed, when the eval output is byte-identical before and after. The comment on `internal/executor/plan.go:20-25` records the maintainers declining a bump for `PlanVerify.Additional` on the grounds that an old record still behaved correctly; that criterion is about behavior, and it does not license paying a broad cost for narrow coverage.

### Decision 2: How to stop the next field from being dropped

#### Chosen: reflection field census plus whole-value JSON round trip

Two guards, and they only work together.

The **census** walks `InstallationPlan`, `ResolvedStep`, `Platform`, `PlanVerify`, `DependencyPlan`, and `PlanAdditionalVerify` by reflection and asserts that the test fixture leaves no exported field at its zero value. Add a field to any of those types and this fails, naming the field, before anyone has thought about the converter.

The **round trip** takes the fixture through `ToStoragePlan`, marshals the `install.Plan` to JSON, unmarshals it back, runs `FromStoragePlan`, and compares the result to the fixture with `reflect.DeepEqual`. Once the census has forced the new field into the fixture, this fails unless the converter carries it.

The JSON hop is not decoration. It is what `state.json` does, and it is what makes a field missing from the *storage type* fail rather than only a field missing from the converter body.

#### Alternatives considered

- **Add six assertions to the existing test.** Rejected: it fixes today and not tomorrow, and tomorrow is how this defect got here.
- **Census alone.** Rejected: it proves the fixture is complete, not that the converter carries anything.
- **Round trip alone.** Rejected: it passes trivially for a field nobody thought to put in the fixture, which is exactly the failure mode.

## Decision Outcome

Mirror the six fields onto the storage types, carry them both ways, guard the converter with the two-part structural test, and mark stored records with `StorageVersion` so a pre-fix record has a defined meaning at both read sites.

## Solution Architecture

### Storage types (`internal/install/state.go`)

```go
// PlanStorageVersion versions the conversion of an executor plan into a
// state.json record -- not the plan format itself, which Plan.FormatVersion
// versions. Version 1 is the first that carries the plan's dependencies,
// verify block, recipe type, binaries, per-step phases, and Linux family.
// A record written before version 1 existed decodes as 0 and cannot be
// trusted to be complete.
const PlanStorageVersion = 1
```

`Plan` gains `StorageVersion int`, `Dependencies []PlanDependency`, `Verify *PlanVerify`, `RecipeType string`, `Binaries []string`. `PlanPlatform` gains `LinuxFamily string`. `PlanStep` gains `Phase string`. New mirror types `PlanDependency`, `PlanVerify`, and `PlanAdditionalVerify` match their executor counterparts field for field.

All new fields carry `omitempty`, so a plan with nothing to say about them serializes exactly as it does today.

### Converter (`internal/executor/plan_conversion.go`)

`ToStoragePlan` and `FromStoragePlan` gain step, dependency, and verify conversion in both directions. Dependency conversion recurses. `ToStoragePlan` stamps `StorageVersion: install.PlanStorageVersion`; `FromStoragePlan` does not read it, because the executor plan has no field for it and the read sites, not the converter, decide what a stale record means.

Both halves of that last sentence changed in #2496. The executor plan now has the field, so both converters carry the value rather than stamping or discarding it -- stamping would have let `tsuku install --plan` on an unmarked file write a record claiming to be complete, silencing the read sites above after a single install. The read sites still decide what a stale record means; the converters just stop destroying the evidence.

### Read sites

`getOrGeneratePlanWith` (`cmd/tsuku/install_deps.go`, the cache read near line 95) skips a cached plan whose `StorageVersion` is below current, falling into the existing regeneration path. This is a different region and an unrelated concern from the already-installed short-circuit further down the same file.

`cmd/tsuku/plan.go` warns on stderr from the one helper both `plan show` and `plan export` already share for loading state.

### What this does not do

`InstallationPlan.Dependencies` is install-time only: `generateDependencyPlans` builds it from `ResolveDependenciesForTarget(...).InstallTime` and returns nil when that map is empty. Runtime dependencies never enter it. Restoring the field therefore does not give #2469 the runtime dependency list it needs; that remains a `VersionState` question.

## Implementation Approach

1. Mirror types and `PlanStorageVersion` onto `internal/install/state.go`.
2. Carry all six fields through both converters, recursing into dependencies.
3. Replace the round-trip test with the census-plus-round-trip guard.
4. Gate the plan cache read on `StorageVersion`.
5. Warn from `plan show` and `plan export` on a pre-fix record.
6. Cover the `plan export` -> `install --plan` path end to end.

## Security Considerations

Restoring `Verify` closes a silent skip: `internal/sandbox/executor.go` treats a nil verify block as "nothing to check" rather than an error, so a plan that lost it passed verification by not having any. Restoring `Dependencies` means a plan-based install of a tool with dependencies installs them instead of silently producing a tool that cannot run.

Neither field relaxes a check. Download checksums were never affected -- they live on the steps, which were always carried -- so this changes what is verified, not whether downloads are.

`StorageVersion` is advisory and fails safe: an unrecognized or absent value means "regenerate" and "warn", never "trust".

## Consequences

**Good.** A stored plan is the plan that was generated. `plan export` output finally matches its documented contract. The next field added to the plan types fails a test rather than shipping. Pre-fix records are identified rather than assumed.

**Bad.** `install.Plan` carries two version numbers, which needs the doc comments to earn it. `state.json` entries grow by the size of the dependency tree and verify block -- bounded by the plan the executor already held in memory.

**Neutral.** Touching `internal/executor/plan.go` and `plan_conversion.go` arms `validate-golden-code.yml`. Golden plans serialize `executor.InstallationPlan`, which is unchanged by this work, so the regenerated plans should be identical; a red run there would be upstream bottle drift, not this change.
