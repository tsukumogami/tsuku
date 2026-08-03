# Decision Report: what a pre-fix stored plan means on read

**Tier:** 3 (standard, fast path -- Phases 0, 1, 2, 6).
Three options were identified up front and Phase 1 research is already on record in
`wip/explore_storage-plan-fields_findings.md`. The choice is costly to undo, not
irreversible: a `format_version` value that ships wrong is corrected by another bump.
No adversarial bakeoff.

## Question

Once `install.Plan` carries `Dependencies`, `Verify`, and `RecipeType`, every record
already in `state.json` decodes with all three at their zero value. Nil dependencies
and nil verify are also what a legitimately dependency-free, verification-free plan
looks like. What does a read of such a record mean?

Only those three fields raise the question. `Phase`, `LinuxFamily`, and `Binaries`
have zero values that are already the correct encoding of "not set" and need no
answer -- see the findings file.

## Constraints

- `state.json` has no schema version. Its migrations are detected structurally.
- The stored plan does carry `format_version`, and `ValidateCachedPlan` already
  rejects a mismatch against `PlanFormatVersion`.
- 110 golden plan files carry `"format_version": 5`.
- Two read paths matter: the plan cache (`cmd/tsuku/install_deps.go:100`) and
  `tsuku plan show` / `tsuku plan export` (`cmd/tsuku/plan.go`), which read
  `VersionState.Plan` raw and never call `ValidateCachedPlan`.

## Alternatives

### A. Accept and run degraded

Define absent as "none" and let pre-fix records mean what they say.

Free. Requires no code beyond the converter fix. Every currently installed tool keeps
exporting an incomplete plan until it is reinstalled or its version moves -- which for
a pinned tool is never. The bug survives the fix on every machine that already has
tools installed, and nothing tells the user.

### B. Bump `PlanFormatVersion` 5 -> 6

Let the existing validator invalidate pre-fix records.

Reuses machinery that is already wired: the cache read logs "Cached plan invalid,
regenerating..." and regenerates. The maintainers' own stated criterion for bumping,
recorded on `internal/executor/plan.go:20-25`, is whether an old record still behaves
correctly -- and here it does not.

Two problems, and together they sink it.

It covers the narrow path and misses the wide one. `ValidateCachedPlan` is called only
from the plan cache, and that path is nearly unreachable: a cache hit implies
`Versions[version]` exists, which means the already-installed short-circuit fires
first. `plan show` and `plan export` never call the validator at all, so the exposure
that actually reaches users is untouched.

And it pays broadly for that narrow fix. 110 golden files change, and the version
history would claim the executor plan format changed when it did not -- the eval
output is byte-identical before and after this fix. `FormatVersion` versions the plan;
what changed is the conversion into storage.

### C. Storage-schema marker on `install.Plan`

Add `StorageVersion`, written by `ToStoragePlan`, absent (zero) on every pre-fix
record. Each read site decides what to do with a record below the current value.

Identifies pre-fix records exactly, at every read site rather than one. No golden
churn -- golden plans serialize `executor.InstallationPlan`, and `install.Plan` never
appears in them. Leaves the executor plan format alone, which is honest, because the
executor plan format did not change.

The cost is a second version number on a type that already has one, which is a real
readability tax and has to be paid down with a doc comment that says plainly what each
one versions.

## Decision: C

`install.Plan` gains `StorageVersion`, and pre-fix records get a defined meaning at
both read sites rather than one:

| Read site | Behavior below current `StorageVersion` |
|---|---|
| plan cache (`getOrGeneratePlanWith`) | cache miss; regenerate |
| `tsuku plan show`, `tsuku plan export` | still works, warns on stderr that the stored plan predates the fix and may omit dependencies and verification, and names `tsuku install <tool> --fresh` as the refresh |

### Why C over B

B costs 110 golden files and a false entry in the plan format's version history, and
buys invalidation on the one read path that is nearly unreachable. C costs about
thirty lines and buys it on both, including the one users actually hit. When the
cheaper option also has the wider coverage, the trade is not close.

### Why C over A

A is free and answers the issue's fifth acceptance criterion by fiat: absent means
none, and pre-fix records are wrong about that forever. The brief asks whether
existing installs quietly keep the bug. Under A they do, and quietly is the operative
word -- there is no signal to the user that the plan they just exported is missing its
dependency tree. C costs little and removes the "quietly".

### Confidence

High against B: the coverage argument is structural, not a judgment call. Medium-high
against A: A is genuinely cheaper and a reviewer could reasonably prefer it, but it
leaves the exposure in place with no way for a user to notice.

## Assumptions

- Pre-fix records are long-lived. A pinned tool's `state.json` entry is not rewritten
  until the user reinstalls, so "it will age out" is not a plan.
- Warning rather than refusing on `plan export` is right. The command should keep
  working; a user exporting an old plan deliberately should get the file and the
  caveat, not an error.

## Rejected

| Option | Reason |
|---|---|
| A. Accept and run degraded | Leaves every existing install silently exporting incomplete plans, with no signal |
| B. Bump `PlanFormatVersion` | 110 golden files and a false version-history entry, to cover one of the two read paths, and the less reachable one |
