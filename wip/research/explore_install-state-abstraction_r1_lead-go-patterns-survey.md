# Lead: Which Go-idiomatic patterns plausibly fit the current call-site shape?

## Findings

### Current shape baseline (what we are comparing against)

Before evaluating patterns, the current shape needs naming. `install.Manager`
is already a struct-with-methods. In Go, that IS the repository pattern in
the GoF sense — `*Manager` aggregates `config`, `state *StateManager`,
`reporter progress.Reporter`, `bus *installevents.Bus`, and exposes
`Install`, `InstallWithOptions`, `Rollback`, `Activate`, `Remove`,
`RemoveVersion`, `RemoveAllVersions`, `List`, `GetState`. State mutation
goes through `state.UpdateTool(name, func(*ToolState))` which already uses
an inversion-of-control closure — the closure parameter is itself a small
pattern. Cross-cutting concerns attach via:

- Constructor functional options (`WithEventBus`)
- Mutator method (`SetReporter`)
- Options struct per call (`InstallOptions`)
- Method parameter (`src installevents.Source` on Install/Rollback/Remove*)

The four wrapper layers above the Manager (`runInstall`,
`runInstallWithReporter`, `installWithDependencies`, `installLibrary`)
are not part of `install.Manager` — they live in `cmd/tsuku/` and re-pack
arguments before calling `mgr.InstallWithOptions`. Adding `Source` had to
re-thread through all four wrapper layers PLUS the Manager methods PLUS
`InstallOptions.Source`. That is the structural pain the issue is about.

`state.json` ownership today: `StateManager` is the only path to disk;
`Manager` embeds a pointer to it; callers can reach `mgr.GetState()` to
bypass the Manager entirely (and several do — `install_deps.go`,
`install_distributed.go`, `plan_install.go`). So state ownership leaks
upward through `GetState()`.

#### Pattern 1: Repository pattern (interface + concrete implementation)

A "true" repository in the Java/DDD sense extracts `InstallRepository`
as an interface (`Install`, `Rollback`, `Remove*`, `List`) with a single
production implementation backed by `*Manager`. In a tsuku-flavored read,
this means writing the interface declaration in a new package consumed
by `cmd/tsuku`, with `install.Manager` becoming the production impl.
The call-site changes from `mgr := install.New(cfg, install.WithEventBus(bus))`
to `var repo InstallRepository = install.New(cfg, install.WithEventBus(bus))`.

Cross-cutting concerns: still attach the same way (functional options,
options struct). `state.json` ownership: hidden behind the interface
boundary — no more `GetState()` escape hatch, callers cannot reach
`StateManager` directly. Operation sketch:

```go
type InstallRepository interface {
    Install(ctx context.Context, req InstallRequest) error
    Rollback(ctx context.Context, req RollbackRequest) error
    Remove(ctx context.Context, req RemoveRequest) error
    List(ctx context.Context) ([]InstalledTool, error)
}

func (m *Manager) Install(ctx context.Context, req InstallRequest) error { ... }
```

The honest read: in Go, this is `*Manager` plus a typed-request struct
plus the optional interface declaration. The interface adds essentially
nothing unless callers genuinely need a fake implementation (most don't —
they construct a real `*Manager` against a temp `TSUKU_HOME`). The
typed-request struct DOES help — it lets a new cross-cutting field
(`Source`, `DryRun`, future `AuditTag`) be added in one place without
re-threading through wrapper layers. But the interface itself is
overhead in Go: Go's structural typing means consumers can use `*Manager`
directly and accept it via interface only where genuinely needed.

#### Pattern 2: Service layer with options-struct-per-call

Generalize `InstallOptions` into one struct per public verb:
`InstallOptions`, `RollbackOptions`, `RemoveVersionOptions`,
`RemoveAllVersionsOptions`. The Manager's public surface becomes:

```go
type RollbackOptions struct {
    ToVersion string
    Source    installevents.Source
}

func (m *Manager) Install(opts InstallOptions) error
func (m *Manager) Rollback(opts RollbackOptions) error
func (m *Manager) RemoveVersion(opts RemoveVersionOptions) error
func (m *Manager) RemoveAllVersions(opts RemoveAllVersionsOptions) error
```

Cross-cutting concerns attach as new fields on each options struct, with
a default supplied by a `DefaultXxxOptions()` constructor (today's
pattern). Call sites that don't care about the new field keep using
defaults; call sites that do, set the field. `state.json` ownership:
still inside `Manager` and `StateManager`; nothing changes about state
plumbing. Operation sketch (Install):

```go
opts := install.DefaultInstallOptions()
opts.Name = "ripgrep"
opts.Version = "14.1.0"
opts.WorkDir = workDir
opts.Source = installevents.SourceManual
return mgr.Install(opts)
```

This is the smallest delta from current shape. It is also the smallest
solution to the issue's pain: the wrapper-layer threading problem
becomes "pack into options once at the CLI edge, unpack once inside the
Manager." A new orthogonal concern (`--dry-run`) becomes a field on
each relevant options struct plus a flag at the CLI; the wrapper layers
just forward the struct.

#### Pattern 3: Command pattern with handler middleware chain

Each operation becomes a Command value: `InstallCommand`, `RollbackCommand`,
`RemoveCommand`. A central `Executor` dispatches commands through a
configured middleware chain — `SourceAttributionMiddleware`,
`TelemetryMiddleware`, `EventBusMiddleware`, `LockingMiddleware`. Each
middleware wraps `Handler` (cobra-style: `Handler func(ctx, cmd Command) error`).
The terminal handler does the real `*Manager` work. Operation sketch:

```go
type Command interface{ cmdName() string }
type InstallCommand struct {
    Name, Version, WorkDir string
    Source                 installevents.Source
}

type Handler func(ctx context.Context, cmd Command) error

func WithEventBus(bus *installevents.Bus) Middleware { ... }
// Wires Publish before/after handler invocation, picks event verb from cmd type.

exec := install.NewExecutor(mgr,
    install.WithSourceFromContext(),
    install.WithEventBus(bus),
    install.WithTelemetry(client),
)
return exec.Run(ctx, install.InstallCommand{Name: "ripgrep", Source: src})
```

Cross-cutting concerns attach by adding a Middleware. The call-site
becomes shape-uniform: every operation goes through `exec.Run(ctx, cmd)`.
`state.json` ownership: still in `Manager`, but the only path to it is
the terminal handler, so all I/O gets the same instrumentation pipeline.

Cobra (which this repo uses) is the canonical Go reference for this
shape: `cobra.Command.RunE` wrapped in `PersistentPreRunE` chains is the
same idea, applied to CLI dispatch. Kubernetes controller-runtime uses
the same shape for reconcile loops with predicates and webhooks
(reference: github.com/kubernetes-sigs/controller-runtime, the
`controller.Options.Reconciler` is wrapped via builder functions).

The mismatch: middleware chains pay off when the SAME chain runs across
MANY operations. tsuku has ~5 lifecycle operations. The infrastructure
cost (Command interface, Handler type, Middleware composition,
type-switch on Command inside event-publishing middleware) is non-trivial
for 5 operations. This pattern fits better when you're adding the 10th
operation, not the 1st cross-cutting concern.

#### Pattern 4: Functional-options builder (aggressive `With...`)

Push everything into builder options. Construction-time options
(`WithEventBus`, `WithReporter`, `WithTelemetry`) plus per-call options
(`InstallWithName`, `InstallWithVersion`, `InstallFromSource`):

```go
mgr := install.New(cfg,
    install.WithEventBus(bus),
    install.WithReporter(reporter),
    install.WithTelemetry(client),
)
return mgr.Install(ctx,
    install.WithName("ripgrep"),
    install.WithVersion("14.1.0"),
    install.WithSource(installevents.SourceManual),
)
```

Cross-cutting concerns attach as new `With...` options at either layer.
`state.json` ownership: unchanged. Operation sketch is what's above.

The Go idiom precedent is grpc-go (`grpc.WithInsecure()`,
`grpc.WithTransportCredentials()`) and aws-sdk-go-v2 (functional options
on client constructors and operation calls). The aws-sdk-go-v2 specific
form — `Client.Operation(ctx, &OperationInput, optFns ...func(*Options))`
— is interesting because it MIXES this with an options struct: required
fields go in the input struct, transient/optional cross-cutting concerns
go in functional options.

Honest read: aggressive functional-options on per-call positions adds
no fewer characters to call sites than a fields-on-struct approach, AND
loses go-vet field-name checking. The construction-time version of this
is already in use (`WithEventBus`) and works well. Pushing it to per-call
position to handle `Source` would mean writing six `WithSource(...)`
options at six call sites; six `opts.Source = src` lines work just as
well.

#### Pattern 5: Decorator chain (Manager wrapped in decorators)

Wrap `*Manager` in decorators that each handle one concern:

```go
var inst Installer = install.New(cfg)
inst = telemetry.NewInstallerDecorator(inst, client)
inst = events.NewInstallerDecorator(inst, bus)
inst = source.NewInstallerDecorator(inst, src)
return inst.Install("ripgrep", "14.1.0", workDir)
```

Each decorator is a struct implementing `Installer` that wraps another
`Installer`. Cross-cutting concerns attach as new decorators.
`state.json` ownership: terminal decorator (the real `*Manager`) owns
state; intermediate decorators see only the public method calls.

The Go idiom precedent is net/http middleware (`http.Handler` wrapping
`http.Handler`). It works well there because every layer takes the
same `(w, r)` shape and the wrapping is mechanical.

The mismatch with install operations: each install method has a
different signature (Install takes name/version/workDir/options;
Remove takes name/source; Rollback takes name/toVersion/source). A
decorator must implement ALL of them. Adding a new operation to the
interface forces every existing decorator to grow a new method. This
is fragile. http.Handler works because the interface is one method;
`Installer` would have ~7. Decorators are also order-dependent in
non-obvious ways (does Source attribution happen before or after the
event bus sees the call?). Go does not have a precedent for 7-method
decorator chains in any popular library I can recall.

#### Pattern 6: Context-attribution + functional facade

Operations remain package-level functions or methods on a lean Manager,
but cross-cutting attribution rides on `context.Context`. The CLI sets
`ctx = installevents.WithSource(ctx, src)` once; the Manager reads
`installevents.SourceFromContext(ctx)` inside the publish closure.
Operation sketch:

```go
ctx := installevents.WithSource(context.Background(), installevents.SourceManual)
return mgr.Install(ctx, "ripgrep", "14.1.0", workDir)
```

Inside the Manager:

```go
func (m *Manager) Install(ctx context.Context, name, version, workDir string) (err error) {
    src := installevents.SourceFromContext(ctx) // defaults to "" -> dropped by bus
    defer func() { m.publishInstallOutcome(ctx, name, version, prior, src, err) }()
    ...
}
```

Cross-cutting concerns attach as new `context.Value` keys plus
package-level `WithX` helpers. `state.json` ownership: unchanged.
Reporter and EventBus stay on the Manager (they're long-lived
collaborators, not per-call attributes); Source moves to context
(it IS a per-call attribute).

This is what kubernetes-sigs uses for request-scoped data
(client-go uses context for cancellation and dispatcher metadata).
Reference: github.com/golang/go/issues/57752 discusses Go's tension
about how much to put in context.Value; the consensus is "request-scoped
data that crosses API boundaries" — which is exactly what `Source` is.

The wrapper-layer threading pain disappears entirely: `runInstall`,
`installWithDependencies`, `installLibrary` accept and pass `ctx`
unchanged (which they should already be doing). The Source value gets
set ONCE at the CLI entry point. New cross-cutting concerns (dry-run,
audit) become new context keys. The cost: context.Value lookups are
untyped at the call site (a typed `SourceFromContext` helper hides
that), and Go folklore is allergic to using context for non-cancellation
data. Lead 4 is dedicated to this question.

#### Comparison table (Findings 1 closes here)

| Pattern | Cross-cut attachment | Call-site shape | State.json ownership | Migration |
|---|---|---|---|---|
| 1. Repository (interface) | Functional options + request struct | `mgr.Install(ctx, req)` | Behind interface | Medium |
| 2. Options struct per call | Field on options struct | `opts.Source=src; mgr.Install(opts)` | Unchanged (Manager) | Small |
| 3. Command + middleware | Add Middleware | `exec.Run(ctx, cmd)` | Behind terminal handler | Large |
| 4. Functional-options builder | New `With...` option | `mgr.Install(ctx, With...)` | Unchanged | Medium |
| 5. Decorator chain | New decorator | `inst.Install(...)` (wrapped) | In terminal | Medium-large |
| 6. Context + functional facade | New context key + helper | `mgr.Install(ctx, n, v, dir)` | Unchanged | Small |

### Reference projects observed

- **cobra** (already in this repo as `github.com/spf13/cobra`): uses
  Command-pattern dispatch with `PersistentPreRunE` / `RunE` chains.
  Closest precedent for Pattern 3. Note that cobra's pattern works for
  command-line dispatch where every command IS the same conceptual
  shape (parse, run, return error); install ops differ more.

- **grpc-go** (`google.golang.org/grpc`): functional options on client/
  server construction (`grpc.WithTransportCredentials`, `grpc.WithInsecure`).
  Closest precedent for Pattern 4 at construction time. grpc also has
  interceptors (`UnaryInterceptor`) which are middleware-chain style,
  precedent for Pattern 3 at call time — but grpc has 1 conceptual
  operation (RPC call) wrapped over many endpoints, which is the
  opposite of tsuku's situation (many operations, few cross-cuts).

- **aws-sdk-go-v2** (`github.com/aws/aws-sdk-go-v2`): hybrid of
  Pattern 2 (input struct per operation) and Pattern 4 (per-call
  functional options for transient overrides). The shape is
  `client.Operation(ctx, &OperationInput{...}, optFns ...func(*Options))`.
  This is probably the most honest, idiomatic-Go composition the
  industry has converged on. The fact that they use BOTH together is
  meaningful: input structs for the operation's data, functional
  options for the operation's behavior.

- **go-git** (`github.com/go-git/go-git`): uses Pattern 2 heavily
  (`*git.CloneOptions`, `*git.PullOptions`, etc.). Each operation
  takes one options pointer. Cross-cutting concerns (auth, progress)
  are fields on the options struct. No middleware chain. The library
  has dozens of operations and this scales well.

- **kubernetes-sigs/controller-runtime**: builder pattern for
  controller construction (`builder.For(...).Owns(...).Complete(...)`)
  but actual reconcile dispatch is plain method calls with context
  threading attribution. Precedent for Pattern 6 at the reconcile
  loop level.

### Codebase fit analysis

How each pattern composes with what tsuku already has:

- `installevents.Bus` (subscribers pattern): All six patterns compose
  cleanly. The bus is a sink the Manager writes to; none of these
  patterns change that. Pattern 3 (middleware) and Pattern 5
  (decorator) could OPTIONALLY relocate publishing OUT of the Manager
  and INTO middleware/decorator, but that fights the design's
  explicit Decision 3 ("publish from inside Manager methods, not
  from state.json shim or defer wrapper at apply.go level"). Trying
  to move publishing to a middleware re-litigates that decision and
  loses the prior-ActiveVersion snapshot semantics that the current
  publish-after-state invariant depends on.

- `install.Option` and `install.WithEventBus` (functional options):
  Pattern 4 is a literal extension of this; Pattern 1 (interface) and
  Pattern 2 (options struct per call) compose orthogonally — `WithEventBus`
  stays at construction, options struct handles per-call. Pattern 3
  (middleware) and Pattern 5 (decorator) replace `WithEventBus` with
  their own composition mechanism, which is a regression — we'd be
  trading a working, idiomatic pattern for a more complex one.

- `InstallOptions` struct: Pattern 2 is a literal generalization;
  Pattern 6 (context) sits orthogonal — context carries cross-cutting
  attribution, options struct carries operation data. Pattern 3 and
  Pattern 5 replace it with Command/decorator wrapping, which is
  redundant.

- `progress.Reporter` (injectable interface): All six patterns compose
  cleanly. The reporter is set once on the Manager via `SetReporter`.
  Pattern 6 could move it to context, but reporter has lifecycle (Stop,
  FlushDeferred) that's better suited to a Manager-owned object than
  a context-scoped one.

### Long-term cost comparison

| Pattern | (a) Initial migration | (b) Add new orthogonal concern | (c) New contributor ramp | (d) Test one op in isolation |
|---|---|---|---|---|
| 1. Repository | Medium: extract interface, change wrappers to use request struct | Low: add field to request struct | Low: explicit method set on interface | Easy: fake implements interface |
| 2. Options struct per call | Small: rename/restructure existing InstallOptions to per-verb | Low: add field to relevant options struct | Very low: same as today, just more uniform | Same as today: construct real Manager against temp dir |
| 3. Command + middleware | Large: Command interface, Handler type, Middleware chain, type-switch dispatch | Very low: add Middleware to chain | High: must understand Command type-switch, middleware order | Hard: tests must thread same middleware chain |
| 4. Functional-options builder | Medium: write `With...` per per-call option | Low: add new `With...` | Medium: per-call options less greppable than struct fields | Same as today |
| 5. Decorator chain | Medium-large: define `Installer` interface, write each decorator | Medium: write a new decorator implementing 7 methods | High: must trace through decorator layers | Hard: must wire same decorator stack |
| 6. Context + functional facade | Small: define context helpers, replace param-threading with ctx-reads | Low: new context key + helper | Medium: requires knowing the context convention | Easy if context helpers default sensibly |

## Implications

Patterns that survive scrutiny:

- **Pattern 2 (options struct per call)** is the smallest-delta solution
  to the issue's pain. It pushes the four-wrapper-layer threading
  problem to a single struct-construction at the CLI edge. Every concern
  that has been threaded through wrappers (Source today, dry-run
  tomorrow) becomes a field, not a parameter. This is what go-git,
  aws-sdk-go-v2, and most idiomatic-Go libraries converge on.

- **Pattern 6 (context + functional facade)** is the most-Go-idiomatic
  solution for the specific cross-cut of `Source`. Source IS request-
  scoped attribution data crossing API boundaries — that's the
  textbook context.Value use case. Lead 4 needs to decide whether
  this is "the right tool" or "the tool everyone says not to use"
  here.

- **Pattern 1 (repository/interface)** survives if and only if it
  is the request-struct half without the interface-extraction half.
  As a pure interface extraction, it adds Go-overhead with little
  benefit. As "request struct per operation," it collapses into
  Pattern 2.

Patterns that get eliminated:

- **Pattern 3 (Command + middleware)** fights Decision 3 in the
  current event-bus design (publish-from-Manager, not from a
  wrapper). The infrastructure cost is large for 5 operations.
  Worth reconsidering ONLY if tsuku reaches 15+ operations or
  starts demanding plug-in cross-cuts (audit, tracing, retry).
  Not now.

- **Pattern 4 (aggressive functional-options on per-call)** offers
  no real benefit over Pattern 2 for per-call data, and the
  construction-time form of it is already in use and working.
  Don't expand it to call sites.

- **Pattern 5 (decorator chain)** does not fit a 7-method interface.
  Go has no idiomatic precedent for 7-method decorator chains, and
  the asymmetric method signatures make ordering bugs likely. Eliminate.

## Surprises

The biggest surprise: **the issue's framing ("DAO/Repository pattern
from Java") points at the wrong solution by Go conventions**. The
"repository pattern" in Java means "interface + concrete impl"; that
specific shape is mostly ceremony in Go. What the issue actually
wants — a layer that owns the full lifecycle of an installed tool
and absorbs threading pain — is best served by **request structs per
operation** (Pattern 2) or **context-based attribution** (Pattern 6),
neither of which is a literal repository. Pattern 1 evaluated as a
literal port of Java's DAO is the WORST viable option in Go.

Second surprise: **the wrapper-layer threading pain is mostly a
cmd/tsuku problem, not a Manager problem**. `runInstall`,
`installWithDependencies`, `installLibrary` each have ~7-8 params and
each grew by one when Source was added. The Manager itself only
needed Source on three methods. Both Pattern 2 (struct-up-front) and
Pattern 6 (context-up-front) collapse the wrapper threading without
touching the Manager much. This suggests **the structural fix is at
the cmd/tsuku boundary, not the install/Manager boundary**.

Third surprise: **the current `GetState()` escape hatch is itself a
design smell that Pattern 1 (interface) would mechanically force us
to remove**. Three production files
(install_distributed.go, install_deps.go, plan_install.go) reach
through `mgr.GetState().UpdateTool(...)` to mutate state without
going through Manager methods, which means they bypass the event
bus entirely. ANY abstraction worth doing should close this hole.
This is leverage Pattern 2 alone does not provide — request structs
don't force `GetState()` to disappear unless we explicitly remove
it. So a Pattern 2 + "deprecate GetState" combo might be the
actual answer.

## Open Questions

- **For lead 4 (context.Context)**: Is `installevents.WithSource(ctx,
  src)` an acceptable use of context.Value in this codebase, or does
  it cross the "context is for cancellation only" line some Go style
  guides draw? What's the precedent in the existing codebase for
  context.Value carrying non-cancellation data?

- **For lead 6 (sketch)**: If Pattern 2 (options struct per call) wins,
  what does the migration look like for the four wrapper layers in
  cmd/tsuku/install_deps.go? Specifically, does `installWithDependencies`
  become a method on a new `InstallSession` value that holds the shared
  cross-cuts, or do we just pack `InstallOptions` once at the CLI edge
  and pass it down by value? The session-value form would also absorb
  the `visited map[string]bool` cycle-detection state and the
  `telemetryClient` and `reporter` parameters — that's a different
  cleanup but related.

- **For lead 6 (sketch)**: Should `mgr.GetState()` be deprecated as
  part of the migration? Three call sites bypass Manager methods to
  mutate state directly, which is exactly the kind of cross-cutting
  bypass the issue is concerned about. A complete abstraction should
  close this hole.

- **For lead 4 or 6**: Library install (`State.Libs`) has its own
  `state.UpdateLibrary` path. Does the same abstraction shape apply,
  or are libraries fundamentally different? The scope says
  out-of-scope for detail but in-scope for "does this apply too" —
  worth a sentence in the final design.

## Summary

Of the six patterns surveyed, **options-struct-per-call (Pattern 2)**
and **context-based attribution (Pattern 6)** are the two Go-idiomatic
shapes that fit the existing Manager/options/bus/reporter composition
without fighting it; the literal Java repository (Pattern 1 with an
extracted interface) and the more elaborate command-pattern/decorator
chains (Patterns 3 and 5) are over-engineered for ~5 lifecycle
operations. The main implication is that the threading pain reported
in the issue is mostly a `cmd/tsuku` wrapper-layer problem rather than
a Manager-shape problem, so the smallest viable fix is to consolidate
per-call options at the CLI edge and let the Manager unpack once.
The biggest open question is whether `context.Context` is an acceptable
carrier for `Source` (and future cross-cutting attribution) in this
codebase — Lead 4 owns that call, and it determines whether the final
sketch in Lead 6 looks like "options struct" or "options struct +
context".
