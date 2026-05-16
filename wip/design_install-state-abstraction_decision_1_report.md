<!-- decision:start id="install-state-abstraction-shape" status="assumed" -->
### Decision: Architectural shape for reducing cross-cutting blast radius in the install pipeline

**Question**

What architectural shape best reduces the blast radius of cross-cutting changes in the install pipeline?

**Context**

PR #2412 (lifecycle event bus) shipped successfully but threading the new `Source` attribution parameter exposed structural friction in `install.Manager`. The exploration produced six leads. Their accumulated evidence:

- `installWithDependencies` grew from 7 to 10 positional parameters in three months (Lead 2).
- Five install-touching PRs in three months each landed a new cross-cutting concern: one every 4-6 weeks (Lead 2).
- Three in-tree workarounds exist that authors built to avoid threading cost: 9 package-level CLI flag globals, a parallel `runDryRun` that bypasses the install pipeline, and a stranded `telemetryClient` parameter PR #2412 could not remove (Lead 2).
- The headline "36 files" framing of PR #2412 decomposes into 16 one-time-cost files, 5 net-negative files, and only 11 files / ~120 LOC of recurring `Source`-threading cost (Lead 5).
- The real structural smell is `mgr.GetState().UpdateTool(name, func(ts){...})` leaking state semantics into the CLI layer at three sites in `install_deps.go` (Lead 6).
- Manager doesn't take `ctx` anywhere; Ctrl-C during the final atomic-rename window is a silent hazard (Lead 4).
- `Source` behaves like a process-level attribute — every call site passes a literal constant (Lead 1), making it the textbook case for ctx-based attribution.
- The lifecycle event bus must compose unchanged (DESIGN-notices-install-event-bus.md is Current and load-bearing).

The exploration eliminated three shapes: the literal Java-style repository pattern (over-engineered; Manager already is the repository), command/middleware/decorator chains (N=2-3, M=5 doesn't justify indirection), and aggressive `OperationOptions` consolidation (essentially "Candidate A plus shared struct"; doesn't address `state.json` exposure).

Four shapes remain in scope: Candidate A (status quo), Candidate B (installops Service layer), Candidate C (ctx-attribution + recursion-collapse), Composite C-then-B (sequenced), Composite B+C (single design).

**Assumptions**

- **12-month forecast: 3 new cross-cutting concerns.** The base rate is one every 4-6 weeks across the last five PRs. Even discounting that rate by half for noise, 3 concerns over 12 months is the conservative forecast. The plausible inventory: dry-run (a workaround already exists), context.Context for cancellation (definitively needed), an audit/correlation ID for telemetry (`telemetryClient` survives as a parameter precisely because no clean home exists). This assumption matters because Lead 6's recommendation is explicitly conditional on it: B if forecast >= 2, C otherwise.
- **`state.json` exposure via `mgr.GetState().UpdateTool(name, func(ts){...})` counts as a smell to heal, not a convenience to preserve.** The pattern appears three times in `install_deps.go` writing four orthogonal state fields (IsExplicit, RequiredBy, InstallDependencies, cleanup actions). Treating this as a smell justifies addressing it; treating it as convenience makes Candidate C sufficient. The evidence (3 sites, 4 distinct fields, anonymous closures over state) reads as a smell.
- **The team prefers incremental migration over single-shot commits.** Decision Driver 4 ("Bounded blast radius") states this explicitly. This biases toward shapes with multi-PR migration paths.
- **Cancellation is independently worth shipping.** Driver 2 calls this out. Any chosen shape should either deliver cancellation or compose cleanly with a parallel cancellation effort.
- **The lifecycle event bus stays untouched.** Driver 3. Constrains all shapes; Service-layer publishing must preserve the verb-per-event vocabulary, synchronous-with-recover delivery, publish-after-state invariant, and subscriber locality.

**Chosen: Composite C-then-B (Candidate C now; revisit Candidate B as a follow-up)**

Land Candidate C as the immediate work, with a charter for a follow-up evaluation of Candidate B once 1-2 additional cross-cutting concerns have landed (or 3-4 months elapse, whichever first).

**What Candidate C entails (this design's scope):**

1. **Thread `context.Context` through `install.Manager` public methods.** `Install`, `InstallWithOptions`, `Rollback`, `Remove`, `RemoveVersion`, `RemoveAllVersions`, `Activate`, `InstallLibrary` all take `ctx context.Context` as the first parameter. Roughly 10 method signatures, ~30 call sites, ~15 files touched. Same file-count shape as Source-as-param; net negative for `internal/updates/` which already has ctx but currently threads Source separately.

2. **Add `installevents.WithSource(ctx, src)` / `installevents.SourceFromContext(ctx)` helpers** with a typed `srcKey struct{}`. Manager extracts Source at publish callsites (`publishInstallOutcome`, `publishRemoveOutcome`). Source as a positional/struct-field parameter is removed from Manager's public surface. The bus's existing empty-Source-drops-with-log behavior catches the "you forgot to set ctx value" failure mode.

3. **Collapse `installWithDependencies` trailing-arg recursion into a request struct.** A new local `installArgs` struct (in `cmd/tsuku/install_deps.go`) carries `Tool`, `ReqVersion`, `VersionConstraint`, `IsExplicit`, `Parent`, `Reporter`, and `TelemetryClient`. The recursive call constructs a sub-args by copying with overrides instead of threading 10 positional parameters. This is Lead 6's "half of Candidate B available in isolation."

4. **Manager accepts ctx but Source no longer travels as a positional/struct param.** `InstallOptions.Source` is removed; tests construct ctx inline via `installevents.WithSource(context.Background(), installevents.SourceManual)`. `LibraryInstallOptions` inherits Source via ctx for free, healing the existing inconsistency (Lead 4).

5. **Cancellation lands as a free bonus.** With ctx threaded, the atomic-rename window in `manager.go` can check `ctx.Err()` and abort cleanly. SIGINT propagates from `globalCtx` in `cmd/tsuku/main.go` to Manager.

**What this design explicitly defers (Decision 2 and Decision 3 territory):**

- The `mgr.GetState().UpdateTool(name, func(ts){...})` exposure smell. Candidate C does not hide `state.json` semantics behind named methods. Decision 2 will rule on whether to add semantic methods (`MarkExplicit`, `RecordDependency`) directly on Manager (option b) or defer to a future Service layer (option c, which couples to a Candidate B follow-up).
- Library install scope. Candidate C heals the `LibraryInstallOptions` Source inconsistency as a side effect of ctx-threading, but does not restructure the library path. Decision 3 will rule on whether libraries get equal treatment or stay parallel.
- The Service-layer restructure. If forecast and cadence continue post-Candidate-C, a future design opens Candidate B as a follow-up. The Candidate B implementation Lead 6 sketched (5 PRs, ~600 lines new package, semantic state methods on Service) remains the off-the-shelf option.

**Rationale**

The decision criteria, in priority order, score as follows:

1. **Reduces per-concern threading cost.** A: minimal (only options-form methods benefit from `InstallOptions` absorption; Rollback/Remove* still take new positional args). B: maximal (Request structs absorb all future fields). C: substantial (ctx absorbs read-only attribution permanently; request-struct refactor of `installWithDependencies` absorbs operation-specific recursion). C is 80% of B's threading reduction at roughly 30-40% of B's cost.

2. **Composes unchanged with the lifecycle event bus.** A: trivially. B: requires moving publish responsibility from Manager to Service; possible but non-trivial. C: trivially — Bus.Publish signature unchanged, only the *source* of `Source` changes (from positional param to ctx extraction at publish callsites).

3. **Unlocks cancellation.** A: no. B: only if it also threads ctx (which would be an additive scope expansion). C: yes, by construction. This is decisive — cancellation is independently worth shipping and C delivers it as a side effect.

4. **Bounded blast radius.** A: zero blast radius (no work). B: ~5 PRs, ~600 lines new package, ~30 construction sites, comparable to PR #2412. C: ~15 files, ~150 LOC, same shape as PR #2412's Source-threading cost (which was empirically only 11 files / ~120 LOC). C is meaningfully smaller and incrementally adoptable (ctx can be added method by method; positional Source can stay during the transition).

5. **Addresses underlying smells.** A: addresses nothing. B: addresses both (`state.json` exposure via semantic methods on Service; `installWithDependencies` trailing-arg via request structs). C: addresses one (`installWithDependencies` trailing-arg via request struct); leaves `state.json` exposure for Decision 2 or a future Candidate B. This is C's principal weakness — it is honest about being a partial solution.

6. **Forecast-robust.** A: holds only if forecast is 0-1 concerns. B: holds at any forecast >= 2; over-invested at forecast = 0-1. C: holds across the range 1-3 concerns (cheap enough to pay off even at low cadence, sufficient through moderate cadence). Composite C-then-B holds across the full range because B can still be opened later if cadence persists.

**Why not pure C?** Pure C leaves Decision 2 (state.json exposure) and Decision 3 (libraries) unresolved with no upgrade path. The phrasing "Composite C-then-B" makes the upgrade path explicit: if 1-2 more cross-cutting concerns land after C, the team revisits B with concrete evidence instead of a 12-month forecast assumption.

**Why not pure B?** Two reasons. First, B's upfront cost (≈ PR #2412 again) is bigger than the empirical recurring cost (11 files / 120 LOC per concern). At a forecast of 3 concerns over 12 months, B saves roughly 30-45 file touches against a 600-line new package and 30 construction-site changes. The arithmetic doesn't clearly favor B. Second, B doesn't deliver cancellation on its own, so a separate ctx-threading effort would land anyway — duplicating most of C's blast radius. Doing C first, watching cadence, then deciding on B with one more datapoint is strictly better than committing to B blind.

**Why not B+C simultaneously?** The combined blast radius exceeds PR #2412's. Decision Driver 4 (bounded blast radius) explicitly weights this against. The argument for B+C is "fix everything at once"; the argument against is "we tried that recently and the file count was the complaint that started this design."

**Why not status quo (A)?** Lead 2's recurring-pattern evidence weighs decisively against pure A. Five PRs in three months each adding a cross-cutting concern, three in-tree workarounds, and `installWithDependencies` growing from 7 to 10 positional parameters: this is structural pain, not isolated incidents. Doing nothing accepts that the next concern reproduces PR #2412's pattern, and the next workaround compounds the existing three.

**Cross-cutting effects on Decisions 2 and 3**

- **Decision 2 (state.json exposure).** Candidate C does not preempt Decision 2. The clean options open to Decision 2 after C lands are: (a) keep `mgr.GetState()` exposed; (b) add semantic methods (`MarkExplicit`, `RecordDependency`) directly on Manager; (d) standalone `StateOps` type. Decision 2 option (c) ("move semantic methods to a new abstraction layer") is naturally deferred to the follow-up Candidate B work. Recommend Decision 2 picks (b) — semantic methods on Manager are cheap (~3-5 method additions), heal the structural leak, and require no new package. This composes cleanly with a future Service layer (semantic methods migrate to Service if Candidate B lands).

- **Decision 3 (InstallLibrary scope).** Candidate C heals the existing `LibraryInstallOptions`-missing-Source inconsistency as a side effect of ctx-threading. Libraries automatically receive Source via ctx; no new structural work. Decision 3 option (a) ("libraries stay parallel") becomes the natural pick because C's ctx-threading already brings libraries into uniform attribution behavior without restructuring the library path. Option (b) ("libraries join the restructure") is empty under C since C isn't a layered restructure. Decision 3's option (c) ("follow-up design") couples to a future Candidate B if it lands.

**Consequences**

What changes:
- `install.Manager` public methods take `ctx context.Context` as the first parameter (~10 signatures, ~30 call sites).
- `installevents` gets `WithSource(ctx, src)` and `SourceFromContext(ctx)` helpers with a typed key.
- `InstallOptions.Source` and `LibraryInstallOptions` (no current Source field) both transition to ctx-based attribution.
- `installWithDependencies` becomes a function over a local `installArgs` struct; trailing-arg threading collapses to copy-with-overrides.
- SIGINT during install propagates: `globalCtx` cancellation reaches Manager and aborts cleanly at safe interruption points.

What becomes easier:
- Adding a read-only cross-cutting concern (audit ID, trace ID, request ID) — store on ctx, extract at consumption sites. Zero new positional params.
- Adding cancellation-aware long-running steps (slow archive extract, network calls in actions) — they receive ctx for free.
- Testing — ctx construction is the standard pattern test authors already know from the executor and plan generator.

What becomes harder:
- Adding a cross-cutting concern that controls behavior (dry-run, force, skip-security). These don't fit ctx (per the "inform, not control" guideline). They have to land as struct fields. Decision 2 / a future design has to decide where: `InstallOptions` for install-specific, a new `OperationOptions` for cross-method, or — if Candidate B eventually lands — Request struct fields.
- Hiding `state.json` semantics from the CLI layer (Decision 2's job, deferred from this decision).
- A future migration to Candidate B (Service layer) becomes additive but non-trivial: ctx is already threaded so Service inherits it cleanly, but the Manager surface still has to be split between low-level primitives and high-level lifecycle verbs.

**Alternatives Considered**

- **Candidate A (status quo)** — Continue threading cross-cutting concerns as method parameters. Rejected because Lead 2's recurring-pattern evidence (one concern every 4-6 weeks, 3 in-tree workarounds, `installWithDependencies` 7→10 positional params) shows the cumulative cost is real and compounding. Doing nothing accepts that the next concern reproduces PR #2412's pattern.

- **Candidate B (installops Service layer)** — New `internal/install/installops` package with a Service type that wraps Manager, owns lifecycle verbs, publishes events, and exposes semantic state methods. Rejected as the primary choice because: (a) B's upfront cost (~5 PRs, ~600 lines new package, ~30 construction-site updates) is comparable to PR #2412, and the savings (~10-20 file touches across 5 plausible future concerns) only marginally exceed C's savings; (b) B doesn't deliver cancellation on its own — a separate ctx-threading effort would still be needed, duplicating most of C's work; (c) committing to B requires the 12-month forecast to firmly support 2+ concerns, but Lead 6's recommendation is explicitly conditional on this judgment and the conservative read accommodates either C or B. Preserved as an explicit follow-up: revisit Candidate B once 1-2 additional cross-cutting concerns have landed or 3-4 months have elapsed.

- **Composite B+C (simultaneous)** — Land both in one design. Service layer takes ctx and uses ctx-attribution internally. Rejected because the combined blast radius exceeds PR #2412's, which directly contradicts Decision Driver 4 (bounded blast radius). The "fix everything at once" framing is exactly what produced the PR #2412 size that motivated this design.

- **Aggressive `OperationOptions` shared struct (Candidate D from exploration)** — Already eliminated during exploration as "Candidate A plus shared struct" that doesn't address the `state.json` exposure smell. Not re-litigated.

- **Literal Java-style repository pattern** — Already eliminated during exploration. Go's struct-with-methods on `*Manager` is already the repository pattern in this codebase's shape (5 lifecycle ops, one storage backend). Not re-litigated.

- **Command/middleware/decorator chains** — Already eliminated during exploration. N=2-3 concerns and M=5 operations don't justify the indirection cost.

<!-- decision:end -->

---

```yaml
decision_result:
  status: "decided"
  chosen: "Composite C-then-B (Candidate C now; Candidate B as charter follow-up)"
  confidence: "medium"
  rationale: >-
    Candidate C (ctx-attribution + installWithDependencies recursion-collapse)
    delivers 80% of B's threading reduction at 30-40% of B's cost, unlocks
    cancellation as a free side effect (independently worth shipping per
    Driver 2), composes trivially with the lifecycle event bus, and is
    incrementally adoptable. It leaves the state.json exposure smell for
    Decision 2 and the Service-layer restructure as an explicit follow-up
    once 1-2 more cross-cutting concerns land or 3-4 months elapse, at
    which point the decision can be made with empirical cadence data
    instead of a forecast assumption. Pure B is too big and duplicates
    C's ctx-threading work; pure A is contradicted by Lead 2's
    recurring-pattern evidence; B+C exceeds PR #2412's blast radius.
  key_assumptions:
    - "12-month forecast of new cross-cutting concerns is approximately 3 (conservative read of one-per-4-6-weeks base rate; plausible inventory is dry-run + ctx/cancellation + audit/correlation ID)"
    - "state.json exposure via mgr.GetState().UpdateTool(...) counts as a smell to heal, not a convenience to preserve — but Decision 2 can address it without coupling to this decision"
    - "Incremental migration is strongly preferred over single coordinated commits (Decision Driver 4)"
    - "Cancellation is independently worth shipping (Driver 2)"
    - "The lifecycle event bus stays untouched (Driver 3)"
    - "Source-via-ctx.Value is defensible Go practice for this case because Source is request-scoped metadata (set once at boundary, never changes, only informs subscribers — does not control branching)"
  rejected_options:
    - name: "Candidate A (status quo)"
      reason: "Contradicted by Lead 2's recurring-pattern evidence: one new concern every 4-6 weeks, 3 in-tree workarounds, installWithDependencies grew 7→10 positional params in 3 months. Doing nothing accepts that the next concern reproduces PR #2412's pattern."
    - name: "Candidate B (installops Service layer, as primary choice)"
      reason: "Upfront cost (~5 PRs, ~600 lines new package, ~30 construction sites) is comparable to PR #2412 itself; doesn't deliver cancellation on its own (would still need ctx-threading); B's savings only marginally exceed C's at the conservative forecast. Preserved as explicit follow-up if cadence persists post-C."
    - name: "Composite B+C simultaneously"
      reason: "Combined blast radius exceeds PR #2412's, directly contradicting Decision Driver 4 (bounded blast radius). The 'fix everything at once' framing is exactly what motivated this design in the first place."
    - name: "Aggressive OperationOptions shared struct (Candidate D)"
      reason: "Already eliminated in exploration as 'Candidate A plus shared struct'; doesn't address state.json exposure smell. Not re-litigated."
    - name: "Literal Java-style repository pattern"
      reason: "Already eliminated in exploration. Go's struct-with-methods on *Manager already is the repository pattern for this codebase's shape (5 lifecycle ops, one storage backend)."
    - name: "Command/middleware/decorator chains"
      reason: "Already eliminated in exploration. N=2-3 concerns × M=5 operations doesn't justify indirection cost."
  report_file: "wip/design_install-state-abstraction_decision_1_report.md"
```
