# Architecture Review: DESIGN-notices-install-event-bus

Reviewer note: ground-truthed against `internal/install/manager.go`,
`internal/install/remove.go`, `internal/updates/apply.go:130-200`,
`internal/updates/self.go:164`, `cmd/tsuku/update.go:201-394`,
`cmd/tsuku/cmd_apply_updates.go`, `cmd/tsuku/main.go`, and
`internal/progress/inbox_reporter.go`.

## 1. Is the architecture clear enough to implement?

Mostly yes. A competent Go engineer can produce the bus, the subscriber,
and the publishers without major back-and-forth. The vocabulary, the
subscriber reaction table (Activated/InstallFailed/Removed), the
publish-on-state-change predicate, and the synchronous-with-recover
delivery semantics are all spelled out concretely. The example data flows
(manual update, failed auto-apply with rollback, self-update, remove)
walk through the exact field values an implementer would need.

Gaps that will cause questions:

- **`Logger` interface is undefined.** The design references
  `installevents.NewBus(log Logger)` (Solution Architecture diagram +
  Key Interfaces) but never defines the `Logger` method set, where it's
  obtained, or how a `nil` value behaves. Security Considerations says
  "the bus's logger should route to the same destination as other tsuku
  diagnostic output (the trace file under `$TSUKU_HOME/log/`)" but
  doesn't point to a concrete package or constructor. The actual codebase
  has `log.Default().Debug(...)` (used in `apply.go:174`) — the design
  should either say "use `log.Default()`" or define a tiny `Logger`
  interface and adapt it.
- **`Manager.Remove`'s status is ambiguous.** `remove.go:15-50` shows a
  deprecated `Remove(name)` that wraps `RemoveAllVersions`. Decision 3
  lists six lifecycle methods including `Manager.Remove`; the publisher
  contract section in Decision 1 only lists `RemoveVersion` and
  `RemoveAllVersions`. The implementer should either remove the
  deprecated wrapper or be told to publish from the underlying methods
  only (and the wrapper inherits the publish for free).
- **`InstallFailed.Err` after publication.** Subscriber calls
  `e.Err.Error()` without a nil check. If a future publisher emits
  `InstallFailed` with `Err: nil` to mean "user-cancelled", the
  subscriber panics. Not a blocker (recover catches it), but worth a
  one-line guard in the subscriber stub.
- **Timestamps.** The event struct carries `Timestamp time.Time`. Who
  sets it — publisher or subscriber? Existing publishers use
  `time.Now()` at write time (e.g., `apply.go:157, 190`). The design
  should say "publisher sets `Timestamp` to `time.Now()` at the publish
  site" so multiple subscribers see the same value.

## 2. Missing components or interfaces?

- **Logger.** See above. The `Logger` type is the most visible gap —
  it appears in the diagram and one prose paragraph but never as a
  defined contract.
- **Auto-apply subprocess wiring.** `cmd_apply_updates.go` is a separate
  subcommand running in the auto-apply subprocess (re-invocation of the
  same binary). The design says "the same wiring runs in the auto-apply
  subprocess" but `cmd/tsuku/events_wiring.go` is described in Phase 5
  as called from "Root command (or relevant per-command setup)". The
  implementer needs an explicit instruction: `newEventBus` must be
  called from both the foreground command tree's `PersistentPreRun` and
  from `cmd_apply_updates.go`'s setup. Without that, the auto-apply
  subprocess would silently no-op (nil-safe Publish), defeating the
  whole design. This is the highest-risk omission.
- **Interaction with `InboxReporter`.** `apply.go:153` writes a notice;
  on the next line `InboxReporter.Stop()` "overwrites with a richer
  notice only when warnings were accumulated". The design says
  `InboxReporter` keeps writing directly (Decision 1 Key Assumptions)
  but the ordering invariant (subscriber writes first, InboxReporter
  overwrites) is not stated. If the bus's `Publish` happens after
  `InboxReporter.Stop()`, the richer notice gets clobbered. Needs a
  one-sentence ordering note in the design.
- **Constructor option for `updates.CheckAndApplySelf`.** The design
  uses `updates.WithEventBus(bus)`. `CheckAndApplySelf` is a function,
  not a constructor; the options pattern is unusual for free functions
  in this codebase. Spell out whether it becomes a struct method or
  takes a `*Bus` argument.

## 3. Phase sequencing

The 5-phase plan is mostly correct but has one ordering smell.

- Phase 1 (events package + bus) is foundation; correct.
- Phase 2 (notices subscriber) depends only on Phase 1; correct.
- Phase 3 (publishers in `install.Manager`) depends on Phase 1, not on
  Phase 2 (publishers don't know about subscribers). Correct.
- Phase 4 (self-update publisher + call-site rewiring) is where the
  trouble is: it removes the existing direct `notices.WriteNotice`
  calls in `apply.go`, `self.go`, and `cmd/tsuku/update.go`, but Phase
  5 ("Wiring and validation") is what actually constructs the bus and
  registers the subscriber. Between the end of Phase 4 and the end of
  Phase 5, the binary builds and runs but writes no notices at all —
  every Publish hits a nil bus and silently no-ops. That's a broken
  intermediate state inside a single PR. Recommendation: either merge
  Phase 4 and Phase 5 into one phase ("rewire call sites and wire the
  bus in one commit"), or swap them so wiring happens first and the
  call-site removal happens with the bus already operational.
- Phase 5 also currently mixes "wire the foreground bus" with "wire the
  auto-apply subprocess bus" without naming both. See item 2 above.

The doc claims "the design fits one PR end-to-end". If that's true, the
phase boundaries are advisory and the broken-intermediate concern
disappears at the commit level. Worth stating explicitly.

## 4. Simpler alternatives

### 4a. Subscriber as function vs struct

The notices subscriber holds only one piece of state (the directory
path) and has no lifecycle. A `func(dir string) func(Event)` closure
would do everything `Subscriber` does:

```go
func NoticesHandler(dir string) func(installevents.Event) {
    return func(e installevents.Event) { /* switch */ }
}
```

Trade-off: a struct is easier to test (you can call methods on it
without indirection through a returned closure), easier to extend
(adding a `Logger` field later doesn't change the constructor's return
type), and matches the codebase's existing pattern (`StateManager`,
`Manager`, `InboxReporter` are all structs with constructors). The
design's choice is defensible — the cost is two lines of struct
declaration — but the doc should note the choice was deliberate, not
default.

### 4b. `installevents` package vs putting events in `internal/notices`

This is the strongest "simpler alternative" question. Today there is
exactly one subscriber. Putting `Activated`, `InstallFailed`, `Removed`,
and `Bus` inside `internal/notices` would let `install.Manager` import
just `notices` instead of a new package, and would collapse the
subscriber to a private method on a notices-package value.

The design's implicit answer is "we expect more subscribers" — telemetry
mentioned in passing in Security Considerations ("a future telemetry or
UI subscriber"), the door for `--quiet` wiring conditionals, and the
auditability argument ("one named function listing the subscriber set").
None of those are concrete today.

The package-level cost of `internal/installevents` is small (≈100 LOC of
event + bus), and the dependency direction — `install` imports
`installevents`, `notices` imports `installevents`, both isolated from
each other — is genuinely cleaner than the alternative
(`install` imports `notices` to publish, which inverts the layering and
makes `notices` a dependency of every install code path). The new
package earns its keep on dependency grounds alone, even with a single
subscriber. The design should make this argument explicitly rather than
leave it implicit.

### 4c. Activate predicate vs caller-supplied prior version

The publish-on-state-change predicate inside `Manager.Activate` is
listed as load-bearing (Consequences section, "Negative"). The
alternative is for `Activate` to publish unconditionally and let the
caller decide whether to call it.

Current rollback site (`apply.go:173`): `mgr.Activate(entry.Tool,
previousVersion)`. If `Activate` always published, the rollback case
would need:

```go
if previousVersion != "" && previousVersion != currentActive {
    mgr.Activate(entry.Tool, previousVersion, SourceRollback)
}
```

The predicate's natural home is inside `Activate` because `Activate`
already knows both the old and new active version (`manager.go:257`:
`if toolState.ActiveVersion == version { return nil }`). The check is
already there for symlink work; reusing it for event publication is
free. Moving the predicate to the caller means every caller learns to
do it, and the rollback site has to load state just to compare — work
`Activate` already does.

Verdict: the design has the right answer. The "load-bearing invariant"
framing oversells the fragility — the existing early-return at line
257 already encodes this invariant for non-event reasons; the event
publication just piggybacks on the same condition. The doc should
mention this co-location (the event predicate and the no-op short-circuit
are the *same* check) as a structural argument, not just "a future
contributor must respect this".

## 5. Implementation hazards

- **Source threading through ~10 call sites.** The design acknowledges
  this. Spot-check from the grep above:
  - `cmd/tsuku/install_deps.go:547` — `InstallWithOptions`, add Source
    to `InstallOptions`.
  - `cmd/tsuku/plan_install.go:107` — same.
  - `cmd/tsuku/install_lib.go:148` — calls `InstallLibrary`, which
    the design doesn't mention. Either `InstallLibrary` is out of scope
    (it has its own state model) or this is a missed call site.
  - `cmd/tsuku/cmd_shim.go:85` — `mgr.Install(recipeName)`. Two-arg
    form. If `Install` keeps the bare signature and only
    `InstallWithOptions` takes the Source, this caller is fine via the
    `DefaultInstallOptions()` default. But a default Source value
    (`SourceInstall`? unset?) needs naming. The design should specify
    the default explicitly: events with an empty Source are a code
    smell.
  - `cmd/tsuku/activate.go:46` — `mgr.Activate(toolName, version)`.
    With ActivateOpts, this becomes `mgr.Activate(toolName, version,
    install.ActivateOpts{Source: SourceInstall})`. Auditable but
    boilerplate-y at every call site.
  - `cmd/tsuku/cmd_rollback.go:62` — `mgr.Activate(...,
    SourceRollback)`. The rollback subcommand and the in-loop rollback
    in `apply.go:173` should pass the same Source. Worth a test.
  - `cmd/tsuku/remove.go:83, 93` — `RemoveVersion`/`RemoveAllVersions`.
    Add RemoveOpts.
- **Nil-safe Publish creates silent test pass-throughs.** Decision 4
  defends nil-safe Publish as a feature ("test setup small while
  ensuring production wiring fails loudly"). The production failure
  mode is "no notice gets written" — not loud. An integration test
  that checks the notice file post-update is the only way to catch a
  forgotten wiring. The design mentions one such test in Phase 5; it
  should be elevated to a non-negotiable acceptance criterion. A unit
  assertion in `cmd/tsuku/main.go`'s init wouldn't help because the
  failure is per-subcommand (e.g., wiring foreground but forgetting
  `cmd_apply_updates.go`).
- **Auto-apply subprocess vs foreground wiring symmetry.** Already
  raised in item 2. The likeliest implementer mistake is wiring the
  root command's `PersistentPreRun` and assuming `apply-updates`
  inherits it. Cobra subcommands do inherit `PersistentPreRun` from
  the root *only if the subcommand doesn't override it*, and
  `cmd_apply_updates.go` may or may not. The implementer should be
  told to verify both code paths construct a bus.
- **`InboxReporter.Stop()` overwrite ordering.** See item 2. If the
  subscriber writes the success notice synchronously inside
  `bus.Publish` called from `Manager.Install`, and `InboxReporter.Stop()`
  is called outside `Manager.Install` (in `apply.go` after the call),
  the InboxReporter's "richer notice on warnings" still wins. Good.
  But the design should call this out so the implementer doesn't
  reorganize it accidentally.
- **`InstallFailed` from `Manager.Install`'s failure path.** The
  current `manager.go:78-220` (`InstallWithOptions`) has many error
  returns before state mutation. The design needs to specify whether
  *every* error return publishes `InstallFailed`, or only those past a
  certain point. Validation errors (invalid version string) probably
  shouldn't publish; partial-install errors definitely should. A
  single defer-pattern at the top of the function is the natural
  implementation but conflicts with Decision 3's rejection of
  defer-based instrumentation. Worth resolving in the design.
- **`Manager.Install` two-arg form (line 68) vs the proposed Source
  field.** `Install` calls `InstallWithOptions(name, version, workDir,
  DefaultInstallOptions())`. If `DefaultInstallOptions()` returns
  `Source: ""` (zero value), every two-arg `mgr.Install` call publishes
  events with an empty Source. The design should either require Source
  to be set explicitly (failing fast at publish time if empty) or pick
  a deliberate default. The default-fail path is preferable —
  forgotten Source is a bug.

## 6. API surface — `ActivateOpts` / `RemoveOpts`

Adding `ActivateOpts` / `RemoveOpts` is justified but not strongly
justified. The design's reasoning: "consistency with the existing
`InstallWithOptions` pattern". This is true but `InstallWithOptions`
exists because `InstallOptions` already had 6 fields (CreateSymlinks,
IsHidden, Binaries, RuntimeDependencies, RequestedVersion, Plan) —
options structs paid off because there were many independent knobs.
`Activate` and `Remove*` get options structs to carry a single field
(`Source`). The cost is:

- Every existing call site grows from `mgr.Activate(tool, version)` to
  `mgr.Activate(tool, version, install.ActivateOpts{Source: ...})`.
- Test fixtures need updating.
- Future evolution gets the options-struct benefit.

The cheaper alternative is a method parameter: `Activate(name, version
string, src Source) error`. Same call-site touch count, half the
boilerplate per call.

The design's argument is "options struct is future-proof for adding
fields". That's hindsight wisdom — there's no second field on the
horizon. A method parameter today, with the option to migrate to
options struct when a second field appears, is the smaller change.

That said, this is a stylistic call and either choice is defensible.
The design picks the more verbose option for a clean reason
(consistency); flagging this lets the implementer push back if the
team prefers the lighter form.

## Recommendations

Concrete file-level changes to the design before implementation begins:

1. **Define `Logger`.** In "Key Interfaces" replace the bare `log
   Logger` placeholder with either an explicit two-method interface
   (`Debugf`, `Warnf`) or "the bus uses `log.Default()` from
   `internal/log`". Pointer-nil should be acceptable.

2. **Add an explicit subsection "Auto-apply subprocess wiring".** State
   that `cmd_apply_updates.go` must call `newEventBus` and thread it
   into the `install.Manager` and `updates.CheckAndApplySelf`
   constructed inside that subcommand, mirroring the foreground
   wiring. This is the highest-leverage edit.

3. **Specify Phase 4/5 ordering.** Either merge phases 4 and 5, or
   reverse them. The current order produces an intermediate state
   where notices are silently dropped.

4. **Specify `Source` defaults and validation.** Add a paragraph: the
   zero-value `Source` is invalid; `Manager.Install`'s two-arg form
   needs an explicit default (e.g., `SourceInstall`); publishers should
   either fail or log when Source is empty. State that
   `DefaultInstallOptions()` will set `Source: SourceInstall`.

5. **Spell out `InstallFailed` publication boundary in
   `InstallWithOptions`.** Which error returns count as "install
   failed" and which (e.g., invalid version string) are not
   user-visible at all. A single `defer` plus a `success bool` named
   return is the natural pattern; the design currently rules out
   `defer` (Decision 3) but the rejection was specifically about
   *rollback* defers in `apply.go`, not in-function success/failure
   defers in `Manager.Install`. Clarify.

6. **Specify Timestamp ownership.** "Publisher sets `Timestamp =
   time.Now()` at the publish site" — one sentence in Decision 1.

7. **Note the InboxReporter ordering invariant.** One paragraph in
   Solution Architecture: "the InboxReporter's `Stop()` runs after the
   install returns, after `bus.Publish` has fired its subscribers, so
   the InboxReporter's richer-on-warnings notice continues to overwrite
   the subscriber's plain success notice. Do not reorder."

8. **Address `InstallLibrary`.** Either bring it into scope (publishers
   thread Source through it too) or explicitly exclude it with a
   reason. Currently invisible in the design.

9. **Address `Manager.Remove` (deprecated wrapper).** Note that
   publication happens at `RemoveVersion`/`RemoveAllVersions`; the
   deprecated `Remove` inherits it via delegation. Or, recommend
   deleting `Remove` as part of this work.

10. **Soften the "load-bearing invariant" framing for the Activate
    predicate.** Note that the early-return at `manager.go:257` already
    encodes the no-op-Activate check for non-event reasons; the
    publication piggybacks on the same condition. This makes the
    invariant structurally durable, not a contributor-discipline
    issue.

11. **Elevate the "drift end-to-end" integration test in Phase 5 to an
    acceptance criterion**, not a "Validation" bullet. Without it,
    nil-safe Publish hides forgotten wiring.

12. **Consider downgrading `ActivateOpts` / `RemoveOpts` to a Source
    parameter.** Or at least state the design picked options structs
    deliberately over the lighter parameter form, so a reviewer doesn't
    push back during PR review with the same observation.
