<!-- decision:start id="auto-install-core-api-location" status="assumed" -->
### Decision: Auto-install core API and shared interface boundary

**Context**

The auto-install feature (`tsuku run`, issue #1679) and the upcoming project-aware exec wrapper (`tsuku exec`, issue #2168) share the same core flow: look up a command in the binary index, apply mode logic (suggest/confirm/auto), optionally install, then exec with the original args. Both commands live in `cmd/tsuku/` as `package main`.

The shared `ProjectVersionResolver` interface is the key coupling point. It must exist somewhere both commands can reference, and its design must allow the project config package (#1680) — which doesn't exist yet — to plug in a version pin without modifying the auto-install core. Go's standard approach for this kind of inversion is an interface defined in the consuming package (or a shared internal package), not in the implementation package.

Currently, `cmd/tsuku/lookup.go` holds `lookupBinaryCommand()` as a package-local helper shared by commands within `package main`. That precedent works for lookup, but the auto-install core is more substantial: it owns install-then-exec flow, mode resolution, TTY detection, and the `ProjectVersionResolver` contract. Burying that logic in `package main` would make it harder to test in isolation and would create an invisible dependency surface for #2168.

**Assumptions**

- `tsuku exec` (#2168) will be implemented as `cmd/tsuku/cmd_exec.go` in `package main`, making it a same-package consumer of the auto-install core — so Option 1 is technically valid today.
- The project config package (#1680) will be a new `internal/` package that implements `ProjectVersionResolver`. This is a future dependency that must plug in without modifying the auto-install core.
- No consumers outside of `cmd/tsuku/` are expected in the near term, but `internal/` placement keeps the option open.
- The `lookupBinaryCommand` helper can be absorbed into the new package as part of this work; `cmd_suggest.go` will call through a thin wrapper or import directly.

**Chosen: Extract to `internal/autoinstall/`**

Create a new package at `internal/autoinstall/` containing:

- `ProjectVersionResolver` interface — one method: `ProjectVersionFor(command string) (version string, ok bool, err error)`. Returns the pinned version from project config, or `ok=false` if no pin exists. This interface is defined here so both `cmd_run.go` and `cmd_exec.go` import the same type.
- `Runner` struct — holds `Config`, `Index` (or wraps lookup), and any I/O writers. Constructed via `autoinstall.NewRunner(cfg, opts...)`.
- `Runner.Run(ctx, command string, args []string, resolver ProjectVersionResolver) error` — the full install-then-exec flow. Returns an error that wraps the child's exit code when exec fails, preserving it for the caller.
- Mode constants: `ModeAuto`, `ModeSuggest`, `ModeConfirm` — shared by both commands, not duplicated.

`lookupBinaryCommand` moves from `cmd/tsuku/lookup.go` into this package (or is replaced by a method on `Runner`). `cmd_suggest.go` can either import from `internal/autoinstall/` directly or keep a thin adapter in `cmd/tsuku/lookup.go` that delegates to the new package.

**Rationale**

Placing the core in `internal/autoinstall/` matches Go's established pattern in this codebase: `internal/hook/` is used by hook commands, `internal/index/` is used by lookup and suggest, `internal/userconfig/` is used by config commands. Shared logic that crosses command boundaries goes in `internal/`. Defining `ProjectVersionResolver` here gives #1680 a clear, stable import target — it implements the interface without touching the auto-install core. Testability is also cleaner: the package has no dependency on cobra or `os.Exit`, making unit tests straightforward.

The same-package option (Option 1) works today but creates a false sense of locality: `cmd/tsuku/autoinstall.go` would hold what is really a library function, making future extraction harder and discouraging independent testing. The naming question (Option 3) is the only real difference from Option 2 — `internal/run/` shadows the `run` command name confusingly, and `internal/exec/` implies OS-level exec semantics rather than the install-then-exec orchestration this package performs. `internal/autoinstall/` names the domain precisely.

**Alternatives Considered**

- **Option 1 — Keep in `cmd/tsuku/`**: Define `ProjectVersionResolver` and `RunAutoInstall()` in `cmd/tsuku/autoinstall.go`. Technically valid since `cmd_run.go` and `cmd_exec.go` are in the same package. Rejected because it buries a testable library in `package main`, makes isolation testing harder, and sets a precedent that future commands (or plugins) would need to break when they want to reuse the logic.
- **Option 3 — `internal/run/` or `internal/exec/`**: Identical to Option 2 in substance. Rejected on naming: `internal/run/` conflicts with the `tsuku run` command's conceptual space, and `internal/exec/` implies low-level OS exec rather than the higher-level install-gate-exec orchestration. `internal/autoinstall/` is unambiguous.

**Consequences**

- `cmd_run.go` and `cmd_exec.go` both import `internal/autoinstall/` — clean, explicit dependency.
- `lookupBinaryCommand` in `cmd/tsuku/lookup.go` either moves to `internal/autoinstall/` or becomes a thin adapter; either way, `cmd_suggest.go` still works.
- When #1680 lands, it implements `autoinstall.ProjectVersionResolver` — no changes to `internal/autoinstall/` needed.
- `internal/autoinstall/` can be tested with standard `go test` using injectable writers and mock resolvers, without any cobra or main-package coupling.
- If a third command (e.g., a future `tsuku shell`) needs the same flow, it imports the same package — no duplication.
<!-- decision:end -->
