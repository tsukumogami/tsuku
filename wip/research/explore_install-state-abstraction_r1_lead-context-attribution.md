# Lead: Would context.Context-based attribution threading actually work?

## Position

**Yes, it would work for `Source` attribution specifically, and it would meaningfully reduce the file-count cost.** The Manager has zero `context.Context` plumbing today, but neither does it have `installevents.Source` plumbing today — both are net-new additions. The relevant question is which threading model is cheaper *and* less invasive long-term. For `Source` alone, `ctx` is roughly the same call-site cost as a positional param, but with two real wins: (a) future cross-cutting concerns ride the same channel for free, (b) deep internal helpers (`publishInstallOutcome`, `publishRemoveOutcome`) don't need to grow positional params they immediately forward. The cost is one well-documented carve-out from the standard "don't use ctx.Value for config" guidance, which tsuku can defend on the same grounds the broader Go community defends request-ID and tenant-ID threading.

**Caveat:** `ctx`-based attribution is a strict subset solution. It pays off only if `ctx` becomes the *general* threading mechanism for cross-cutting concerns (cancellation, deadlines, attribution, dry-run, audit IDs). If the design decides each cross-cutting concern gets its own bespoke mechanism, threading `ctx` just for `Source` adds a new pattern for marginal benefit.

## Findings

### 1. Does the Manager currently accept `context.Context` anywhere?

**No.** Not a single Manager method takes `ctx`, and the install package doesn't import `context`. Verified via `grep -rn "\"context\"" /home/dgazineu/dev/niwaw/tsuku/tsuku/public/tsuku/internal/install/` — zero hits.

Current signatures (`/home/dgazineu/dev/niwaw/tsuku/tsuku/public/tsuku/internal/install/manager.go` and `remove.go`):

```go
func (m *Manager) Install(name, version, workDir string, src installevents.Source) error
func (m *Manager) InstallWithOptions(name, version, workDir string, opts InstallOptions) (err error)
func (m *Manager) Rollback(name, toVersion string, src installevents.Source) error
func (m *Manager) Remove(name string, src installevents.Source) error
func (m *Manager) RemoveVersion(name, version string, src installevents.Source) (err error)
func (m *Manager) RemoveAllVersions(name string, src installevents.Source) (err error)
func (m *Manager) InstallLibrary(name, version, workDir string, opts LibraryInstallOptions) error
```

`InstallOptions` (`manager.go:68-76`) carries `Source` as a field but no context. `LibraryInstallOptions` doesn't carry `Source` at all yet — that's an inconsistency the design left in place.

**Where does ctx live in the CLI path?** `cmd/tsuku/main.go:195` creates `globalCtx, globalCancel = context.WithCancel(context.Background())`, canceled on SIGINT/SIGTERM. Some commands route it through `cmd.Context()` (Cobra), others via the `globalCtx` package var (`cmd/tsuku/install_distributed_test.go:454` confirms this is reachable). For `tsuku update` specifically: `cmd/tsuku/update.go:92` uses `context.Background()` for `loadRecipeForTool` — even *inside the CLI command*, ctx isn't threaded coherently. The `mgr.InstallWithOptions` call at `cmd/tsuku/install_deps.go:551` receives no ctx whatsoever. **The CLI path drops ctx at the Manager boundary today.**

So the comparison isn't "add Source via ctx vs. add Source as a param"; it's "add ctx threading (which carries Source) vs. add Source as a param (where ctx still isn't threaded)." Threading ctx solves more problems but costs more line-touches in this one PR.

### 2. Call-site cost: ctx threading vs. Source-as-param

**Files currently threading `Source` (verified):**

```
cmd/tsuku/cmd_run.go
cmd/tsuku/install_project.go
cmd/tsuku/install_lib.go
cmd/tsuku/install.go
cmd/tsuku/remove.go
cmd/tsuku/install_deps.go
cmd/tsuku/cmd_apply_updates.go
cmd/tsuku/create.go
cmd/tsuku/eval.go
cmd/tsuku/update.go
cmd/tsuku/cmd_rollback.go
internal/install/manager.go
internal/install/remove.go
internal/updates/checker.go
internal/updates/self.go
```

That's **15 files** currently touching `Source` parameters. The issue body calls this out as "the file-count cost it incurs is now visible evidence to reconsider."

**Cost of threading ctx instead (estimate):**

- The 11 CLI files (`cmd/tsuku/*.go`) would each need ~the same touch — replace `src` with a `ctx` (built via `installevents.WithSource(parentCtx, src)`). Net: similar file count, slightly more code per site (one wrapper call before the Manager call).
- The 2 install files (`manager.go`, `remove.go`) would replace `src installevents.Source` params with `ctx context.Context`. The 4 internal helpers (`publishInstallOutcome`, `publishRemoveOutcome`, the deferred publish closures) would consume `ctx` and call a `sourceFromContext(ctx)` helper. Net: similar file count, but with the future-proofing benefit.
- `internal/updates/{checker,self}.go` already accept `ctx context.Context` (verified: `self.go:88` takes `ctx context.Context, ..., src installevents.Source`). They'd shrink (drop the `src` param, encode it via `WithSource(ctx, ...)` before calling Manager). **Net negative for `internal/updates/` — fewer params, not more.**

**Verdict:** File count is approximately the same — ~15 files touched either way. The *line count per file* is slightly higher for ctx (need a wrapper before each Manager call, e.g., `ctx := installevents.WithSource(parentCtx, installevents.SourceManual)`). But: at the deep helper level, ctx wins because internal forwarding doesn't need to grow new positional params each time we add a new attribution dimension.

### 3. Cost of pulling `Source` from ctx at publish callsites

The pattern is well-understood. Typed key, value helper, public setter:

```go
// In installevents/context.go
type sourceKey struct{}

func WithSource(ctx context.Context, src Source) context.Context {
    return context.WithValue(ctx, sourceKey{}, src)
}

func SourceFromContext(ctx context.Context) Source {
    if ctx == nil {
        return ""  // empty Source -> Bus.Publish drops the event with a log line
    }
    v, _ := ctx.Value(sourceKey{}).(Source)
    return v
}
```

**Publish-site change:** `m.publishInstallOutcome(name, version, priorActiveVersion, opts.Source, err)` becomes `m.publishInstallOutcome(ctx, name, version, priorActiveVersion, err)`, and inside the helper: `src := installevents.SourceFromContext(ctx)`.

**Safety properties:**
- Typed key `sourceKey struct{}` prevents collisions across packages. Standard Go idiom (per `context` package docs: "The provided key must be comparable and should not be of type string or any other built-in type to avoid collisions between packages using context.").
- Missing-key case: returns zero-value `Source("")`. The bus *already* drops empty-Source events with a log warning (`bus.go:135-142`): "Empty-Source events are dropped with a log line — every publisher must specify a Source." So forgetting to set the source surfaces loudly in tests and trace logs, equivalent to today's behavior when you pass `""` as a positional param.

**Test impact:** Tests construct the source ctx inline:

```go
ctx := installevents.WithSource(context.Background(), installevents.SourceManual)
err := mgr.Install(ctx, "jq", "1.7", workDir)
```

vs. current:

```go
err := mgr.Install("jq", "1.7", workDir, installevents.SourceManual)
```

Marginally more verbose per test. The 11 `manager_events_*_test.go` cases would all need this update. Not a small change but not architectural either.

### 4. Go community guidance on context.WithValue for non-cancellation data

**The standard guidance.** The `context` package docs ([pkg.go.dev/context](https://pkg.go.dev/context)) say:

> Use context Values only for request-scoped data that transits processes and APIs, not for passing optional parameters to functions.

The Go FAQ and Russ Cox's original blog post are stricter still: "context.Value should inform, not control."

**Where Source sits on that spectrum.**

`Source` is request-scoped in the strict sense: it's set once per CLI invocation (or once per auto-update worker tick), it doesn't change mid-flight, and it doesn't control behavior — it only *informs* the lifecycle event subscribers (notices and telemetry) about what triggered the operation. The Manager code path doesn't branch on Source today; it just forwards it to the publish call. **By the "inform, not control" test, Source qualifies.**

By comparison, the canonical valid uses of `context.Value` in production Go are:
- Request IDs and trace IDs (Go blog, `net/http`-based services)
- Authenticated user / tenant identity (any auth middleware example)
- Logger handles enriched with the above

`Source` is structurally identical to "trace ID for a CLI invocation": one value, set at the entry point, consumed by observability code at the boundary, never inspected by business logic.

**Where it differs:** Server-side request IDs survive across goroutine boundaries and HTTP handler hierarchies — that's the "transits processes and APIs" justification. A CLI install pipeline is a single linear call chain on the main goroutine. The "must use ctx because of goroutine fan-out" argument is weaker here.

**Uber Go Style Guide** ([github.com/uber-go/guide](https://github.com/uber-go/guide/blob/master/style.md)) doesn't prohibit `ctx.Value` for attribution but recommends explicit parameters when feasible. **Google's go-styleguide internal doc, surfaced via [google.github.io/styleguide/go/decisions](https://google.github.io/styleguide/go/decisions#contexts)**, says: "Don't use context.Value to pass optional parameters" but allows it for "request-scoped metadata, request IDs, and user identifiers."

**Verdict on tsuku's right answer:** The guidance is genuinely ambivalent on this case. `Source` is closer to "request-scoped metadata" than "optional parameter" because it's set once at the boundary and used purely for tagging. The community would not flag this as misuse. The strongest "purist" objection — "but the compiler won't tell you when you forget" — is already addressed by the bus's empty-Source-drops-with-log behavior.

### 5. Composability with the lifecycle event bus

**Today's `Bus.Publish` signature** (`internal/installevents/bus.go:131`):

```go
func (b *Bus) Publish(event Event)
```

No ctx. The event itself carries `GetSource() Source` (verified at `bus.go:135`).

**Two integration choices:**

**Option A — ctx stays at Manager, event still carries Source.** The Manager extracts Source from ctx at the publish site and builds the event normally:

```go
func (m *Manager) publishInstallOutcome(ctx context.Context, name, version, priorActiveVersion string, err error) {
    src := installevents.SourceFromContext(ctx)
    // ... build Installed{..., Source: src} as today
    m.bus.Publish(evt)
}
```

`Bus.Publish` signature unchanged. This is the minimum-disruption integration and the recommended one — the bus stays a pure pub/sub mechanism, ctx is purely a transport convenience between CLI and Manager.

**Option B — `Bus.Publish(ctx, event)`.** Forces ctx through the bus and on to every subscriber's Handle. This couples subscribers to ctx (they currently only see `Event`), invents a question about what happens when a subscriber spawns a goroutine that outlives ctx, and provides no benefit if Source already lives on the event payload.

**Rule out Option B.** Option A is cleaner and matches how, e.g., `slog.Logger` keeps ctx-aware handlers without forcing ctx into every record's payload.

### 6. Does ctx generalize to other cross-cutting concerns?

**Cancellation/deadlines:** Already what ctx is for. Threading ctx through the Manager unlocks SIGINT-aware long-running installs essentially for free (e.g., aborting a slow archive extract). This is a real win the Source-as-param approach can't deliver.

**Dry-run (lead 2's question):** Could go on ctx, but lead 2 owns the analysis. Quick note: dry-run *controls* behavior (skips state writes), so by the "inform, not control" rule, it's exactly the case the Go community pushes back on. Probably better as an `InstallOptions` field, not ctx.

**Audit/trace ID for cross-pipeline correlation:** Classic ctx use case if/when it materializes.

**Telemetry correlation ID (already exists?):** Worth checking whether `telemetry.Client` has a session ID concept that should be in ctx instead of plumbed separately.

**Where ctx stops being clean:** anything the install logic *branches on*. `IsExplicit`, `RequestedVersion`, `Binaries`, `Plan` — all of these influence what files get written. Those belong in `InstallOptions`, full stop. The clean line is: **ctx for read-only attribution and lifecycle (cancel, source, trace-id), InstallOptions for inputs that change what happens.**

## Implications

**The design should seriously consider ctx-based attribution as a complement to the cross-cutting concerns abstraction, not as a full alternative to it.**

Specifically:

1. **As a replacement for the current `Source` positional param:** Yes, viable, modest net win. Roughly equal file count, slightly more lines per site, but pays for itself the next time a cross-cutting concern needs threading. The "ctx unlocks cancellation as a side benefit" argument tilts this toward "do it."

2. **As an alternative to a broader install-state abstraction (lead 1's question):** No. ctx solves the "how does Source reach the publish site" sub-problem, but doesn't address the structural questions about state mutation locality, transaction boundaries, or test seams. Lead 1's full restructure stands on independent merits.

3. **The original design's rejection of "defer-based instrumentation" was correct but didn't consider ctx.** The rejected pattern was `defer bus.Publish(buildEvent(err))` — i.e., constructing the event payload at the defer site. The rejection rationale ("Rollback in apply.go:164-182 happens outside the function that would carry the defer, destroying locality") is unrelated to whether the *attribution* (Source) travels via ctx or positional param. Those are orthogonal axes. The design conflated them.

4. **Migration story:** ctx threading is incrementally adoptable. Start with the Manager-public surface, leave `InstallOptions.Source` as a transitional fallback for one release, then remove. No big-bang refactor required.

## Surprises

- **The design doc literally never mentions `context` or `ctx`.** Zero hits across the whole file. Given how prominent ctx is in modern Go API design, this is a meaningful gap — not just an unconsidered alternative, but an unconsidered axis.
- **`internal/updates/self.go:88` already takes both `ctx` and `src` as separate params** (`func CheckAndApplySelf(ctx context.Context, ..., src installevents.Source) error`). The codebase already has the pattern where ctx exists but Source is still positional — exactly the inconsistency a ctx-based attribution would eliminate.
- **The bus's empty-Source-drops-with-log behavior is a built-in safety net** for the "what if you forget to set ctx value" failure mode. The design already anticipated the "what if Source is empty" case for positional params; that same guardrail catches ctx misuse for free.
- **`LibraryInstallOptions` doesn't carry `Source` at all**, while `InstallOptions` does. The design left a known inconsistency that ctx-based attribution would naturally heal — both methods would just consume ctx.
- **Zero existing `context.WithValue` usage in the entire tsuku codebase** (`grep -rn "context.WithValue\|ctx.Value"` returns nothing under the source tree). This is greenfield — no precedent to align with, no existing typed-key conventions to inherit. That's both an opportunity (clean slate) and a risk (no team muscle memory).

## Open Questions

Things lead 6 (sketch + migration) needs to address:

1. **Should `LibraryInstallOptions` get a Source field as part of this work**, regardless of whether ctx wins? It's an existing inconsistency that any "thread attribution everywhere" effort must close.
2. **Where exactly does ctx enter the Manager?** Adding `ctx` as the first param to every public method is the standard pattern, but it's a large signature change. Alternative: add a `Manager.WithContext(ctx)` builder method that returns a ctx-bound view. Or: keep ctx as a field on Manager set once at construction. Each has different ergonomics for the CLI command path.
3. **Migration sequencing.** Can `installevents.WithSource(ctx, src)` and the positional `src` param coexist for one release? If yes: low-risk staged rollout. If no (e.g., because of double-publish risk in the lifecycle code): big-bang change.
4. **Does the project's auto-update worker (`internal/updates/`) need its own ctx convention** for `SourceProjectAuto` vs. `SourceAuto`? It's already passing both ctx and Source separately — this is the cleanest place to demonstrate the migration.
5. **Should the bus subscribers (notices, telemetry) also receive ctx**, separate from the event payload? Probably no for notices, probably yes for telemetry (HTTP calls want cancellation). This is the "Option A vs. B" question from sub-question 5 — needs an explicit decision.
6. **What's the test seam look like in practice?** `manager_events_test.go` has 200+ lines of test setup; how disruptive is the migration to those tests? Sketch one before recommending.

## Summary

Threading `context.Context` for `Source` attribution would work technically and aligns with mainstream Go practice for request-scoped metadata, though the design doc never considered it — `context.Context` appears zero times in DESIGN-notices-install-event-bus.md. The file-count cost is roughly equal to the current Source-as-positional-param approach (about 15 files touched), but ctx pays for itself by simultaneously unlocking SIGINT-aware cancellation and providing a clean channel for future read-only cross-cutting concerns (trace IDs, source attribution), without invading `InstallOptions` for every new tagging dimension; the biggest open question is whether the design treats this as a one-off swap for Source or commits to ctx as the general home for attribution-class concerns going forward, because the latter framing is what justifies the migration churn.
