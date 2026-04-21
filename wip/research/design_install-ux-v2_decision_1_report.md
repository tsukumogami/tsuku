<!-- decision:start id="install-reporter-injection" status="assumed" -->
### Decision: Reporter injection into internal/install/

**Context**

`internal/install/Manager` has no `Reporter` field today. Its methods — `InstallWithOptions`,
`InstallLibrary`, and helpers in bootstrap.go — call `fmt.Printf` directly, producing ~10
output sites across 5 files. The `progress.Reporter` interface already exists from PR #2280 and
is used by `internal/executor/Executor` via a `SetReporter()` / `getReporter()` pattern.

The goal is to route all install-package output through a single `Reporter` instance so the
TTY spinner owns the terminal for the full install duration without interleaving with raw
`fmt.Printf` lines. The install path in `cmd/tsuku/install_deps.go` already creates one
`TTYReporter` and sets it on the executor; the same reporter must reach the Manager.

`install.New(cfg)` is called from ~20 sites: install commands, remove, list, outdated, rollback,
updates/checker, and others. Most of these are read-only and produce no output. Only the install
path calls need a reporter.

**Assumptions**

- The install path remains single-threaded per Manager instance (no concurrent SetReporter calls).
  If Manager is ever used concurrently, a sync.RWMutex would need to guard the reporter field.
- plan_install.go will have the TTYReporter threaded in when this design is implemented. Today
  it creates install.New(cfg) without a reporter; that file will need a SetReporter call added.
- The recursive installWithDependencies pattern (fresh mgr per recursion) will either have
  mgr.SetReporter added per recursion, or be refactored to pass mgr as a parameter — either
  approach satisfies the shared-reporter constraint.

**Chosen: Struct field + SetReporter() on Manager (Option A)**

Add a `reporter progress.Reporter` field to `Manager`. Add two methods:

```go
func (m *Manager) SetReporter(r progress.Reporter) {
    m.reporter = r
}

func (m *Manager) getReporter() progress.Reporter {
    if m.reporter != nil {
        return m.reporter
    }
    return progress.NoopReporter{}
}
```

Replace all `fmt.Printf` / `fmt.Fprintf` output calls in manager.go, library.go, bootstrap.go,
remove.go, and update.go with `m.getReporter().Log(...)` or `.Warn(...)` as appropriate.

Callers that need TTY output add one line after `install.New(cfg)`:
```go
mgr.SetReporter(reporter)
```

All other callers (state queries, list, remove, background jobs) continue unchanged.

**Rationale**

This option satisfies all three stated constraints:

1. **Additive, not breaking.** No method signatures change. ~18 existing callers require zero
   modifications. The `SetReporter()` call is opt-in.

2. **Shared reporter instance.** The install path creates one `TTYReporter` and sets it on both
   `exec` and `mgr`. Recursive dep installs that create a fresh `mgr` must explicitly re-set the
   reporter — this is a one-line addition per recursion site, contained entirely within
   `cmd/tsuku/install_deps.go`.

3. **Mirrors established precedent.** The executor already uses this exact pattern. Contributors
   recognize it immediately. The `NoopReporter` fallback is proven safe.

**Alternatives Considered**

- **Option B — Reporter parameter on each output-producing method.** Rejected. While it offers
  compile-time enforcement that a reporter is always provided, it violates the minimal-churn
  constraint: changing the signatures of `InstallWithOptions`, `InstallLibrary`, and related
  methods forces simultaneous updates to 5+ files and 8+ call sites. The recursion cascade in
  `installWithDependencies` makes the parameter threading bleed into outer functions as well.
  The enforcement benefit is real but not worth a coordinated breaking change.

- **Option C — Thread via context.Context.** Rejected. `context.WithValue` is explicitly
  discouraged by Go documentation for service injection. `context.Context` does not currently
  flow through any Manager method — introducing it would require touching all method signatures,
  which violates the minimal-churn constraint more severely than Option B. The automatic
  propagation benefit (reporter travels with context through recursion) is real but can be
  achieved more simply by passing `mgr` as a parameter to the recursion rather than embedding
  the reporter in context. If the install path ever adopts context for cancellation, reporter
  injection via context can be reconsidered at that point.

**Consequences**

- `fmt.Printf` disappears from the install package. All output goes through `Reporter`.
- The TTY spinner can own the terminal for the full install duration without interleaving.
- Tests can set a `testReporter` on Manager and assert on logged output, mirroring the existing
  `executor/install_output_test.go` pattern.
- The silent-failure mode (forgot to call `SetReporter`, output disappears) is accepted:
  identical tradeoff to the executor, and bounded to sites that need TTY output.
- Background install paths (updates/apply.go) correctly get no output via `NoopReporter` fallback.
<!-- decision:end -->
