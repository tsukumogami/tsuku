# Exploration Findings: install-state-abstraction

## Core Question

Would consolidating install state operations behind a single abstraction
(a repository-style layer, or whatever idiom fits Go best) simplify the
install pipeline and reduce the blast radius of cross-cutting changes?

## Round 1

### Key Insights

1. **The Java-style repository pattern as drawn in the issue is over-engineered for this codebase.**
   Five Go-idiomatic patterns were surveyed (repository, options struct,
   command/middleware, decorator, ctx-attribution, OperationOptions).
   Only two survive scrutiny against tsuku's actual shape: options-struct-per-call
   and ctx-based attribution. The literal repository abstraction adds
   indirection without proportional benefit because the Manager surface
   for state-mutating ops is just 5 methods and there is no second storage
   backend to abstract over. (Lead 3, Lead 6)

2. **The empirical "file count" framing of the issue is partly contradicted by the data, but cumulative pain is real.**
   PR #2412's 36 files decompose into **16 one-time** (bus, subscribers,
   schema, renderer, docs), **5 net-negative** (direct-write removal),
   and only **11 files / ~120 LOC** of actual `Source`-threading cost.
   The big PR size was dominated by bus introduction, not by cross-cutting
   plumbing. (Lead 5)
   However: `installWithDependencies` grew from **7 to 10 positional
   parameters in 3 months**, and 5 install-touching PRs across that window
   each landed a new cross-cutting concern (#2198, #2201, #2213, #2280,
   #2412) — a cadence of roughly one every 4-6 weeks. (Lead 2)

3. **Three in-tree workarounds prove the threading cost is driving authors to detour around it.**
   - 9 package-level CLI flag globals in `cmd/tsuku/install.go` (e.g.,
     `installFresh`, `installDryRun`, `installSkipSecurity`) bypass the
     wrapper chain entirely
   - A parallel `runDryRun` function in `cmd/tsuku/install.go:430`
     that reimplements the install flow rather than threading a
     `dryRun bool` through the wrappers — and is already drifting
     (no shadowed-deps warning)
   - The legacy `telemetryClient *telemetry.Client` parameter survived
     PR #2412's bus migration because removing it would have required
     threading the bus deeper or moving one Send call to a subscriber
   These are evidence the cost is real and producing maintenance debt. (Lead 2)

4. **The real structural smell isn't `Manager`'s signature — it's `state.json` leaking out via `mgr.GetState()`.**
   `cmd/tsuku/install_deps.go` calls `mgr.GetState().UpdateTool(name, func(ts){...})`
   at lines 223, 475, 584 to write `IsExplicit`, `RequiredBy`,
   `InstallDependencies`, and cleanup-action state from the CLI layer.
   Manager owns state via `m.state.UpdateTool` but ALSO exposes
   `m.GetState()` so callers do their own writes. Any restructure that
   doesn't address this leaves the cross-cutting problem half-solved. (Lead 6)

5. **`Source` behaves like a process-level attribute, not a per-call argument.**
   Lead 1 found that **every** call site that passes `Source` passes a
   literal constant (`SourceManual`, `SourceAuto`, `SourceProjectAuto`).
   None compute it dynamically per operation. This is the strongest
   structural argument for ctx-based attribution: the value's lifetime
   is "this CLI invocation," not "this operation." 10 wrapper signatures
   currently change just to forward an unchanging value.

6. **`context.Context` is not threaded through `Manager` anywhere, and would unlock cancellation as a free bonus.**
   None of `Manager.Install`, `InstallWithOptions`, `Rollback`,
   `RemoveVersion`, `RemoveAllVersions`, `Remove`, or `Activate` takes
   `ctx`. The executor and plan generator take ctx, but it's dropped
   before reaching the Manager. A user hitting Ctrl-C during the final
   `os.Rename` window of an install will not interrupt cleanly. Threading
   ctx costs ≈15 files (same shape as Source-as-param), and once it's
   there, Source-via-ctx becomes essentially free. (Lead 4, Lead 1)

7. **The lifecycle event bus has already eliminated one future class of cross-cutting cost.**
   A structured audit log subscriber would be a 2-file addition (subscriber + tests),
   not a 10-file threading exercise. This is concrete evidence the
   recently-shipped bus is doing useful cross-cutting work. (Lead 5)
   But ctx is the most expensive plausible cross-cutting concern and the
   bus **cannot carry it** — cancellation flows top-down through call
   stacks, not bottom-up through event hubs. So consolidation that rides
   the bus doesn't help here.

8. **The trailing-arg recursion in `installWithDependencies` is half the problem and can be fixed without a full restructure.**
   Collapsing the recursion into a request-struct argument (where Source
   and any future cross-cutting flag is a field) removes most of the
   trailing-arg threading pain without introducing a new abstraction
   layer. This is "half of Candidate B," available in isolation. (Lead 6)

### Tensions

1. **Lead 2 (recurring class) vs Lead 5 (one-off LOC count) on whether the file count is a real problem.**
   Lead 5 measured one PR (#2412) carefully and found that cross-cutting
   threading was only 11 files / 120 LOC of the total. Lead 2 measured
   the *pattern* across 5 PRs and found a recurring cadence (one new
   concern every 4-6 weeks) plus 3 in-tree workarounds.
   **Resolution**: both can be true. The marginal cost per concern is
   small, but the cumulative pain is real and is producing maintenance
   debt (workarounds, parameter explosion). Lead 5's "the file count
   wasn't catastrophic" framing is accurate for *one* PR; Lead 2's
   "this is structural" framing is accurate for *the trend*.

2. **Candidate B (installops layer) vs Candidate C (ctx-attribution) recommendation.**
   Lead 6 recommends Candidate B conditional on "2+ new cross-cutting
   concerns expected in next 12 months"; otherwise Candidate C plus a
   standalone recursion-collapse refactor. Lead 2's evidence (one
   concern every 4-6 weeks) suggests the condition for B is likely
   satisfied, but Lead 5's cost projection (Candidate B's upfront cost is
   comparable to PR #2412 again; lifetime saving is ~10-20 file touches
   across 5 concerns) suggests the ROI is marginal even at high cadence.
   **Resolution**: the design phase has to weigh these explicitly.
   This is not a tension that further research closes — it's a
   product/maintenance-strategy call.

3. **`state.json` exposure via `mgr.GetState()` — smell or appropriate level of abstraction?**
   Lead 6 calls it the "actual leaky abstraction." But it's also the
   pragmatic way `IsExplicit`, `RequiredBy`, and cleanup actions are
   written from the CLI layer today. A restructure that hides
   `state.UpdateTool` behind semantic methods (`MarkExplicit`,
   `RecordDependency`) adds a real API surface. Whether that surface is
   worth it depends on whether you read CLI-layer state writes as a bug
   or a feature.

### Gaps

None critical. The remaining unknowns are decision-class, not
investigation-class:

- **12-month forecast of new cross-cutting concerns** is a product
  judgment, not something further research can resolve.
- **Are the asymmetric methods (`Activate`, `InstallLibrary`, `ExposeHidden`) intentional invariants or accidents?**
  Lead 1 flags this; the design phase will need to decide whether
  any restructure preserves the asymmetry (e.g., `Activate` is
  internally called by `Install` for recovery, so it shouldn't publish)
  or heals it (libraries on the bus too).
- **Should the design treat `ctx` as a one-off swap for `Source` only, or commit to it as the general home for read-only cross-cutting attribution?**
  Lead 4 surfaced this; it's the framing question that distinguishes
  Candidate C as a "narrow fix" from Candidate C as a "principled choice."

### Decisions

Recorded inline in `wip/explore_install-state-abstraction_decisions.md`:
- Java-style literal repository pattern: **eliminated** by Lead 3 + Lead 6.
- Command/middleware/decorator-chain patterns: **eliminated** as
  over-engineered for ~5 lifecycle ops.
- Aggressive `OperationOptions` struct (Candidate D): **deprioritized**
  in favor of Candidate B and C; it's essentially "Candidate A plus
  shared struct" and doesn't address the `state.json` leak.

### User Focus

Auto mode — user instructed to drive to a reviewable design doc.
Convergence is sufficient to crystallize; further rounds would generate
more research but not more decisions.

## Accumulated Understanding

The exploration set out to evaluate the issue's hypothesis: would a
repository-style abstraction reduce the blast radius of cross-cutting
changes in the install pipeline? The data refines the hypothesis in
three ways:

**1. The diagnosis was partially right.** The install pipeline does have
a recurring class of cross-cutting threading problem. Five PRs in three
months each added one such concern. The `installWithDependencies`
signature grew from 7 to 10 positional parameters. Authors are
demonstrably routing around the cost (9 package-level CLI globals, a
parallel `runDryRun`, a stranded `telemetryClient` parameter). The
"file count was too high" complaint reflects real maintenance debt,
even though the per-PR LOC numbers (PR #2412 was ~120 LOC of pure
cross-cutting plumbing) understate the cognitive cost.

**2. The proposed shape (literal repository pattern) is the wrong fix.**
Go's struct-with-methods is itself the repository pattern in many
readings. `install.Manager` is already a repository over `state.json`.
The literal Java-style abstraction would add indirection without addressing
the actual smells: (a) the wrapper recursion in `cmd/tsuku/install_deps.go`,
and (b) `state.json` writes happening from the CLI layer via
`mgr.GetState().UpdateTool(...)`. The codebase needs different surgery,
not a bigger abstraction.

**3. Two viable shapes emerge from the analysis:**

   - **Shape C — context.Context-based attribution + recursion collapse.**
     Thread `ctx` through Manager methods (≈15 files), store `Source`
     in ctx via a typed key, extract it at the publish callsites, and
     separately collapse `installWithDependencies`'s trailing-arg
     recursion into a request struct. Picks up cancellation
     (Ctrl-C during `os.Rename`) as a free bonus. Doesn't address the
     `state.json` leak. Cost: ≈15-20 files, comparable to PR #2412,
     but unlocks cancellation and Source-via-ctx becomes essentially
     free for future read-only attribution concerns.

   - **Shape B — `internal/install/installops` layer.**
     A new Service layer between CLI and Manager. Service owns the
     lifecycle verbs (Install, Rollback, Remove) and publishes events.
     Manager keeps low-level state and symlink primitives. The
     request-struct shape (`InstallRequest`, `RollbackRequest`, etc.)
     absorbs future cross-cutting concerns as fields. Hides
     `state.json` semantics behind named methods (`MarkExplicit`,
     `RecordDependency`). Incremental migration possible (5 PRs).
     Cost: roughly comparable to PR #2412 upfront, with measurable
     savings (~10-20 file touches) across 5 plausible future concerns.

These two shapes are not mutually exclusive. A reasonable design
sequence is Shape C first (cheap, picks up cancellation, addresses
Source narrowly) and Shape B later if the cross-cutting cadence
continues. But landing both simultaneously is also defensible.

**The status-quo option (Candidate A) remains a defensible position**
if the design phase judges that (a) the cumulative cross-cutting cost
is overstated by the workaround evidence, (b) the
`mgr.GetState().UpdateTool` pattern is fine as a CLI-layer convenience
rather than a smell, and (c) the 12-month forecast is closer to 1
new concern than 3+.

The design phase needs to:
1. Make the forecast judgment (decision-class, not technical).
2. Decide whether `state.json` exposure via `mgr.GetState()` is a smell
   the restructure must address, or a pragmatic convenience to preserve.
3. Decide between Shape B, Shape C, both, or neither.
4. If a restructure is pursued, scope the work for libraries
   (`InstallLibrary` currently parallels the tool path with no bus
   integration).

## Decision: Crystallize

Convergence is sufficient. Further research rounds would not produce
new decisions — the remaining questions are design-phase judgments
about forecast, taste, and scope, not investigation gaps. Proceeding
to Phase 4 (Crystallize artifact type).
