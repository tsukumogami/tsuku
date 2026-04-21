<!-- decision:start id="action-status-description" status="assumed" -->
### Decision: Action description strategy for install UX status line

**Context**

tsuku's executor currently prints raw action names during installation ("Step 2/6: download_file"). The new install UX needs a meaningful status line like "Downloading kubectl 1.29.3 (40 MB)" for each step. The `Action` interface in `internal/actions/action.go` has no facility for producing this text, so the executor must get it from somewhere.

The executor already contains a partial implementation: `formatActionDescription()` in `internal/executor/executor.go` is a switch/case that generates dry-run descriptions for 8 of the ~35 action types. This function is in the wrong package (executor rather than actions), covers a minority of actions, and diverges from action definitions over time.

The codebase has three prior examples of optional action interfaces: `Decomposable` (decompose to primitive steps), `NetworkValidator` (declare network requirements), and `Preflight` (validate parameters without side effects). All three follow the same pattern: define a separate interface, check via type assertion at the call site, fall back gracefully when not implemented.

**Assumptions**

- External recipe consumers may implement the `Action` interface directly without embedding `BaseAction`. The public repo and plugin ecosystem makes this likely.
- The install UX status line is shown per-step during execution against an `InstallationPlan`, so descriptions apply to primitive steps (already fully resolved: URL, filename, binary names, size).
- A generic fallback (the action name itself) is acceptable for the initial release; coverage improves incrementally.

**Chosen: Optional descriptor interface (ActionDescriber)**

Define a new optional interface in `internal/actions/`:

```go
// ActionDescriber is implemented by actions that can produce a human-readable
// status message for display during installation. The executor checks for this
// interface via type assertion and falls back to the action name if not implemented.
type ActionDescriber interface {
    // StatusMessage returns a short, human-readable description of what this
    // step will do. Params are the resolved step parameters. Returns "" to
    // use the generic fallback (action name).
    StatusMessage(params map[string]interface{}) string
}
```

Call site in the executor (or reporter):

```go
var msg string
if d, ok := action.(ActionDescriber); ok {
    msg = d.StatusMessage(step.Params)
}
if msg == "" {
    msg = step.Action // fallback
}
```

Priority implementations for the first release (covers the majority of recipe steps):
- `download_file`: "Downloading {basename(url)}" with size if `step.Size > 0` → "Downloading kubectl-linux-amd64 (40 MB)"
- `extract`: "Extracting {archive}"
- `install_binaries`: "Installing {binary names}"
- `cargo_build` / `cargo_install`: "Building {crate} with cargo"
- `github_archive` / `github_file`: handled via download_file after decomposition (primitives only)
- `npm_exec` / `pip_exec` / `gem_exec`: "{package manager} install {package}"

Dynamic values (tool name, version, size) come from the already-resolved step params. The `Size` field on `ResolvedStep` carries the pre-computed file size from plan generation.

**Rationale**

This approach matches the three existing optional interface patterns in the same package (`Decomposable`, `NetworkValidator`, `Preflight`). It does not add a method to the `Action` interface — a Go interface change that would break any external implementor who doesn't embed `BaseAction`. Description logic stays in the action that understands its own parameters, avoiding a cross-package switch/case that drifts out of sync as new actions are added. Rollout is incremental: the most-used actions get descriptions first, and the fallback (action name) is no worse than current behavior.

**Alternatives Considered**

- **Add StatusMessage to Action interface**: Would require updating all 35+ action implementations and breaks the `Action` interface contract for external implementors in this public repo who don't embed `BaseAction`. The mandatory nature (must return something for every action) also creates artificial pressure to add descriptions for obscure actions like `service_enable` or `group_add` where the action name is already clear. Rejected because it violates the explicit constraint against breaking the Action interface.

- **Executor-constructed from plan metadata (switch/case)**: The executor's existing `formatActionDescription()` is already this approach, covering 8 of 35+ action types. It places knowledge about action semantics in the executor package rather than the actions package, causing drift as new actions are added without corresponding executor updates. An executor-side switch also cannot access action-specific logic (e.g., a cargo_install action knowing which crate name is the display-friendly field vs. internal parameters). Rejected because it extends an existing debt pattern rather than resolving it.

**Consequences**

- `internal/actions/` gains one new interface type (~8 lines).
- The reporter or executor gains one type-assertion check and fallback (~5 lines).
- The `formatActionDescription()` in executor.go can be removed once the high-priority actions implement `ActionDescriber`; the dry-run display switches to the same path.
- External contributors adding new actions are not required to implement `ActionDescriber`, but the pattern is clear and discoverable alongside `NetworkValidator` and `Decomposable`.
- Coverage improves incrementally: start with 6-8 high-frequency action types, expand in subsequent PRs or as contributors add recipes for new action types.
<!-- decision:end -->
