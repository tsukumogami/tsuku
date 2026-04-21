<!-- decision:start id="build-action-verbosity" status="assumed" -->
### Decision: Build Action Verbosity

**Context**

tsuku supports source-build recipes that use `configure_make`, `cargo_build`, and `go_build` — actions that run 10 to 300 seconds and generate substantial compiler output. Today all three actions use `cmd.CombinedOutput()`: subprocess stdout and stderr are captured into a buffer, nothing is shown on success, and the full buffer is appended to the error on failure. This pattern already satisfies the core constraint (diagnostic output available on failure without re-running), but it has no explicit connection to the new Reporter architecture.

The install-ux redesign (Decisions 1–4) establishes a `Reporter` interface where `Status()` is transient and TTY-only, and `Log()` is permanent on both TTY and non-TTY. The question is where build subprocess output routes: Status, Log, or conditionally through a verbose flag.

A `--verbose` flag already exists globally in `cmd/tsuku/main.go` (maps to slog INFO level); it does not currently affect action output. An undocumented `TSUKU_DEBUG` env var in `cargo_build.go` already gates success output display, indicating the designers expected build output to be hidden by default.

**Assumptions**

- The "Debug:" printf statements in `configure_make.go` (curl detection, git-core listing) are temporary debugging artifacts that will be cleaned up before or alongside the Reporter migration, not permanent output.
- Build actions (configure_make, cargo_build, go_build) are the primary long-running actions. Other actions (extract, download, install_binaries) either complete quickly or have their own progress feedback via Decision 3/4 mechanisms.
- The error propagation path from action → executor → cmd layer renders the full captured output readably. If error rendering truncates output, the capture-on-failure guarantee weakens.
- Running in --auto mode without user confirmation.

**Chosen: Spinner only, output captured + shown on failure**

Build subprocess output is captured via `CombinedOutput()`. The Reporter shows a spinner with a static message during the build (e.g., "Building cmake 3.28.1..."), produced by the action's `StatusMessage()` implementation (per Decision 2). On success, the spinner clears and a permanent `Reporter.Log` line is emitted ("Built cmake 3.28.1"). On failure, the captured output is included in the returned error and surfaces as a permanent error block in the terminal.

On non-TTY output (CI), `Reporter.Status` is a no-op. The build emits one `Reporter.Log` line at start ("Building cmake 3.28.1...") and one at completion or failure. This matches the "per-phase text lines" pattern from Decision 3 without any special build-action handling.

The `TSUKU_DEBUG` env var in `cargo_build.go` (which prints captured output on success) remains as a debugging escape hatch for recipe authors; it becomes the documented precedent for what `--verbose` could enable in a future iteration.

**Rationale**

Option 1 is already implemented in all three actions. The constraint "must not require re-running with --verbose to debug a failure" is satisfied by the existing `CombinedOutput` + error-append pattern. Choosing Option 1 means the Reporter migration for build actions requires only adding spinner setup and a final Log call — no subprocess plumbing changes.

Option 2 (transient passthrough) provides zero benefit on non-TTY: Status calls are discarded on CI. On TTY it shows the "most recent" compiler line, but at a 100ms tick rate against burst-mode compiler output, most lines are missed. The user sees a rapidly-changing status line that's less readable than the clean spinner. It also requires replacing CombinedOutput with a tee pipe (feeding both Status and a capture buffer), adding complexity to all three actions for marginal TTY benefit.

Option 3 (verbose flag) is the right long-term direction for recipe authors who want to watch builds — and the TSUKU_DEBUG pattern in cargo_build already shows this need exists. But it requires wiring verboseFlag into ExecutionContext, switching from CombinedOutput to a streaming pipe in all three actions, and handling the spinner-disable-on-verbose interaction. This is non-trivial scope for the initial Reporter migration. It's better addressed as a follow-on issue once the core Reporter infrastructure is in place.

The decisive constraint is "must not require re-running with --verbose to debug a failure." Option 1 satisfies this today. The other options either don't improve on it (Option 2) or add scope that can wait (Option 3).

**Alternatives Considered**

- **Transient status passthrough**: Stream each compiler line to `Reporter.Status()`, overwriting in-place on TTY. Rejected because `Status` is a no-op on non-TTY (zero CI benefit), compiler output arrives in bursts that exceed the 100ms tick rate (most lines are missed on TTY anyway), and it requires replacing `CombinedOutput` with a tee pipe in all three actions — significant complexity for marginal gain.

- **--verbose flag enables full build output streaming**: With `--verbose`, build subprocess stdout/stderr stream directly to the terminal instead of being captured. Rejected for the initial migration because it requires wiring verboseFlag into ExecutionContext, switching subprocess invocation from CombinedOutput to streaming in three actions, and handling the spinner interaction during verbose streaming. The TSUKU_DEBUG precedent in cargo_build suggests this is the right eventual direction, but the scope belongs in a follow-on issue after the Reporter infrastructure is established.

**Consequences**

- All three build actions (configure_make, cargo_build, go_build) keep their `CombinedOutput` invocation.
- Each action gains a `Reporter.Status("Building <tool> <version>...")` call before the subprocess and a `Reporter.Log("Built <tool> <version>")` call on success.
- The `TSUKU_DEBUG` escape hatch in `cargo_build.go` is preserved and documented; it becomes the model for a future `--verbose` streaming mode.
- The `fmt.Printf("   Debug: ...")` statements in `configure_make.go` should be removed (or gated behind `TSUKU_DEBUG`) during the Reporter migration to avoid permanent Log noise.
- Non-TTY/CI installs with source builds see exactly two lines per build action: one at start, one at completion (or the error + full compiler output on failure).
- A future issue can add `--verbose` streaming for recipe authors who want to watch compiler output, with the infrastructure established by this migration in place.
<!-- decision:end -->
