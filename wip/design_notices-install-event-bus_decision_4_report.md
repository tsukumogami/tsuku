<!-- decision:start id="subscriber-registration" status="assumed" -->
### Decision: Subscriber registration mechanism

**Context**

The notices/install bus needs to know which subscribers receive events. In v1 there is exactly one subscriber (`internal/notices`); future candidates include telemetry and audit. The choice is between three styles for assembling that list: an explicit wiring helper in `main`, per-package `init()` self-registration on a global bus, or a constructor that takes subscribers as arguments.

The user constraint says every tsuku process must wire the full subscriber set so manual installs and the auto-apply subprocess behave identically. Investigation of the codebase showed that this is easier than it sounds: tsuku has exactly one `func main` (`cmd/tsuku/main.go`), and the auto-apply "subprocess" is the same binary re-invoked with the hidden `apply-updates` subcommand. There is no second `package main` to forget.

Decision 2 already committed the bus to be a value held by `cmd/tsuku` wiring and passed where needed, not a process-global. That commitment rules out registration mechanisms that depend on a package-level singleton.

**Assumptions**

- The auto-apply subprocess will continue to re-invoke the `tsuku` binary with a hidden subcommand rather than becoming a separate binary. (Confirmed by reading `cmd/tsuku/cmd_apply_updates.go` and `internal/updates/trigger.go`.)
- The v1 subscriber set is small (1) and the foreseeable set stays in single digits.
- Subscribers are config-dependent objects (e.g., `internal/notices` needs the notices directory path), so they cannot meaningfully self-register at package-init time.
- Tests construct a fresh bus per subject; they do not need to inject subscribers into the production bus.

**Chosen: Single explicit wiring helper in `cmd/tsuku`, with a `bus.New()` constructor plus per-subscriber `Subscribe(name, sub)` calls listed in one file**

Add a small `cmd/tsuku/wiring.go` (or equivalent — exact filename is a code style detail) that contains a single function responsible for building the bus and registering every subscriber:

```go
// newBusForCLI constructs the event bus with the canonical subscriber set.
// Called from main's init/main path; the result is threaded into the install
// manager and self-update path.
func newBusForCLI(cfg *config.Config) *bus.Bus {
    b := bus.New()
    b.Subscribe("notices", notices.NewSubscriber(notices.NoticesDir(cfg.HomeDir)))
    // future subscribers added here, one line each
    return b
}
```

`bus.Bus` exposes two methods: `New() *Bus` (zero subscribers) and `Subscribe(name string, sub Subscriber)`. Subscribers may be added only at wire-up time; v1 does not require runtime add/remove (consistent with Decision 2's "subscriber registration is static within a process" assumption). The bus package does not import any subscriber package; subscribers import `bus` to implement its `Subscriber` interface. Import direction stays clean.

The auto-apply path uses the same bus because it runs through the same `main` and the same wiring helper. No second registration point exists to drift.

Subscriber order in the wiring helper is the source of truth for delivery order, matching Decision 2's "registration order" guarantee.

**Rationale**

The decision is dominated by Decision 2's existing commitment that the bus is a value, not a global. That single fact eliminates Option B (per-package `init()` self-registration), which requires a package-level default bus. Once Option B is out, the remaining choice is stylistic: helper-with-Subscribe-calls (A) versus variadic constructor (C). They are functionally equivalent.

Three smaller factors favor the helper form (A):

1. **One named place to look.** A reader who wants to answer "what runs on InstallSucceeded?" opens `cmd/tsuku/wiring.go` and reads a short function. With a variadic constructor (`bus.New(a, b, c)`), the call site is still in `main.go`, but it's mixed in with all the other construction (registry, loader, providers). The named helper isolates "subscriber set" as its own concept.

2. **Conditional wiring is natural.** If a future subscriber is gated on `userCfg.TelemetryEnabled`, an `if` block inside the wiring helper is straightforward. The same pattern in a variadic constructor requires building a slice first and then passing it, which is the same code with extra indirection.

3. **Matches the prevailing codebase pattern for config-dependent wiring.** `recipe.NewLoader(providers...)` is paired with explicit `SetConstraintLookup(...)` post-construction; `registry.New(...)` is paired with provider chain construction in `main.go`. The "construct + configure" idiom is already familiar. A wiring helper sits comfortably alongside the existing construction code.

The codebase's `init()`-driven registration (action registry, cobra command registration) works because those are package-level singletons with no config dependency. The bus is not a package-level singleton (Decision 2) and its subscribers do have config dependencies (notices directory), so the action-registry analogy doesn't transfer.

The user's "must not be forgotten in a new entry point" constraint, on inspection, doesn't add weight to any option: tsuku has one entry point, and any future binary that installs tools would need to import the wiring helper regardless of registration style. The constraint is satisfied for free.

**Alternatives Considered**

- **Per-package `init()` self-registration (Option B).** Each subscriber package would call `bus.Register(...)` in its `init()`, and the bus would hold subscribers as package-level state. Rejected for three reasons: (1) it requires a global default bus, contradicting Decision 2's "bus is a value" commitment; (2) subscribers depend on resolved config (`notices` needs the notices directory), but `init()` runs before config resolution, so subscribers would have to defer construction or read config themselves, reintroducing the wiring step the pattern was meant to avoid; (3) tests would need a `bus.Reset()` between cases, which is exactly the kind of global mutation the codebase has been moving away from in newer packages.

- **Constructor-injected subscriber list (Option C).** `bus.New(subscribers...)` taking a variadic list. Rejected only marginally: it is functionally equivalent to the chosen option. The helper-with-`Subscribe`-calls form was preferred because it (a) gives a named single-purpose function (`newBusForCLI`) where the subscriber set lives, (b) makes conditional registration read more naturally than slice-building, and (c) leaves room for a future `Subscribe(name, sub)` call from a test harness if that ever proves useful, without changing the constructor signature. If team review prefers the variadic form, the semantic outcome is the same; this is a low-stakes preference.

- **Cobra-style decentralized registration inside `package main`.** Considered but rejected as a degenerate case of Option B: each `cmd_*.go` file would call a package-local `registerSubscriber(...)` from `init()`. Same global-state and config-timing problems, restricted to one package, with no compensating benefit.

**Consequences**

What becomes easier:
- Auditing the subscriber set: one function in one file lists everything.
- Adding a subscriber: one line in the wiring helper, plus the subscriber package itself. No import-side-effect chain to chase.
- Testing: subjects under test construct their own `bus.New()` with stub subscribers via `Subscribe`. No global to reset, no parallel-test hazard.
- Threading the bus into Decision 3's publish sites: the wiring helper returns a `*Bus`, which is passed to the install manager constructor and the self-update entry point exactly like other collaborators today.

What becomes harder:
- A future binary that needs to install tools (none today) would need to call `newBusForCLI` itself. This is the same cost as any other shared CLI wiring (registry, loader, provider chain) and is mitigated by keeping the helper in a place a new entry point would naturally find.
- Subscribers that want to be added at runtime are not supported. v1 does not need this; if a future use case appears (dynamic plugin loading), `Subscribe` already allows late registration before any publish.

What stays the same:
- No new dependency direction: `bus` does not import subscribers; subscribers implement the bus's `Subscriber` interface.
- No package-level state. No init-time side effects.
- The auto-apply subprocess sees the same subscriber set as the foreground because it runs through the same `main`.
<!-- decision:end -->
