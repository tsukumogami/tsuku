<!-- decision:start id="auto-install-entry-point" status="assumed" -->
### Decision: CLI entry point and exec handoff for auto-install

**Context**

Tsuku's auto-install feature (issue #1679) adds a way to detect a missing tool, install it on consent, and run it — all in one invocation. The flow is: look up the command in the local SQLite binary index, offer to install the matching recipe if consent is given, install it, then exec into the binary so the tool's own exit code propagates directly to the caller.

A downstream design (`#2168`, project-aware exec wrapper `tsuku exec`) will need to replicate this install-then-exec flow for project-pinned tools. The key constraint is that `#2168` must call the auto-install core as a library function, not shell out to `tsuku run`, to keep the dependency surface clean and avoid a subprocess indirection that would complicate exit code and signal propagation.

The existing codebase has `lookupBinaryCommand` and install logic in the `main` package (`cmd/tsuku/`), while shared logic lives in `internal/` packages (`internal/install/`, `internal/index/`, etc.). Shell hooks already exist (`tsuku hook`) and call `tsuku suggest` for read-only lookup, but they do not auto-install.

**Assumptions**
- `#2168` will be implemented as a separate cobra subcommand in the same binary, not a separate binary; this is consistent with all existing commands.
- On Unix, `syscall.Exec` is the right handoff primitive — it replaces the process image so the tool's exit code becomes the process exit code without a fork. Windows can use `os/exec` with `cmd.Run()` and pass the exit code explicitly (same pattern already used in `verify.go`).
- Consent UX (prompt before installing) is handled by `tsuku run` itself; non-interactive callers pass a `--yes` flag or the library function accepts a consent callback.
- The index must already be built (`tsuku update-registry` was run); `tsuku run` exits with `ExitIndexNotBuilt` (11) if not, matching the behavior of `tsuku suggest`.

**Chosen: Option 2 — `tsuku run` cobra subcommand with core logic extracted to `internal/autoinstall/`**

Implement a new `tsuku run <command> [args...]` cobra subcommand in `cmd/tsuku/cmd_run.go`. The command is a thin wrapper. All install-then-exec logic lives in a new `internal/autoinstall/` package that exports a stable `Run` function:

```go
// internal/autoinstall/run.go
type Options struct {
    Command  string
    Args     []string
    Consent  func(recipe string) bool  // nil means always-yes
    Cfg      *config.Config
}

func Run(ctx context.Context, opts Options) error
```

`cmd_run.go` calls `autoinstall.Run(...)` with a consent function that prints the prompt and reads stdin. `cmd_exec.go` (issue #2168) calls the same function with its own consent policy (project config, --yes flag, etc.).

The exec handoff uses `syscall.Exec(binaryPath, append([]string{command}, args...), os.Environ())` on Unix. Because `syscall.Exec` replaces the process, the tool's exit code becomes the process exit code with no wrapping. On Windows, the implementation falls back to `cmd.Run()` + explicit `os.Exit(exitCode)`, consistent with the pattern in `verify.go`.

**Rationale**

Option 2 satisfies the interface-stability constraint immediately — `#2168` imports `internal/autoinstall` directly with no refactoring debt. It follows the existing codebase pattern where shared logic lives in `internal/` packages and cobra commands are thin wrappers. The extraction cost is identical to Option 1 because the same code must be written either way; the only difference is where it lives.

**Alternatives Considered**

- **Option 1 — Embedded in `cmd_run.go`, no shared library**: Implements the feature correctly for issue #1679 but violates the interface-stability constraint. When `#2168` arrives, all install-then-exec logic must be extracted from `main` package to `internal/` — a refactor that could introduce regressions and delays `#2168`'s development. Rejected because the additional short-term simplicity creates guaranteed future cost.

- **Option 3 — Shell function/alias injection**: Provides transparent UX (no `tsuku run` prefix) but is a mechanism for shell integration, not a replacement for a Go library. It requires writing and maintaining shell code in three dialects (bash, zsh, fish), cannot provide consent UX reliably in non-interactive contexts (scripts, CI), and still requires a Go library surface for `#2168`. It could be layered on top of Option 2 as a future UX enhancement — the shell function would call `tsuku run` — but it does not address the library interface question. Rejected as a standalone answer to this decision.

**Consequences**

- `internal/autoinstall/` becomes a new stable internal package. Its `Run` function signature is the contract that `#2168` depends on; changes to it require coordinating both callers.
- `tsuku run jq -- --arg foo bar` is the user-facing invocation. The `--` separator is recommended in documentation to prevent flag collision between tsuku and the target binary.
- `syscall.Exec` on Unix means tsuku's deferred cleanup (signal handlers, temp files) will not run after the handoff. The `internal/autoinstall` package must complete all cleanup before calling `Exec`.
- The exit code table in `exitcodes.go` needs two additions: one for "tool not found in index" (suggesting `tsuku update-registry`) and one for when the user declines consent. These are tsuku-side exits only; after `syscall.Exec` the exit code belongs entirely to the tool.
<!-- decision:end -->
