# Decision Report: reporter.Log() to reporter.Status() Classification

**Decision prefix:** design_install-ux-v2_decision_4  
**Date:** 2026-04-21

## Question

Which current `reporter.Log()` calls in actions should become `reporter.Status()` to achieve the single-status-line goal without silencing CI output?

## Findings From Code Review

### Behavior contract (from `internal/progress/reporter.go`)

- `Log()`: permanent line on both TTY and non-TTY (stderr). On TTY it kills the spinner first.
- `Status()`: ephemeral. On TTY it updates the single spinner line. On non-TTY it is a **no-op** (returns immediately without writing anything).
- The goal is: CI (non-TTY) sees 3-5 lines per install. TTY sees only the spinner.

### Log() calls by file

**`extract.go`** (lines 146-149):
```
reporter.Log("   Extracting: %s", archiveName)
reporter.Log("   Format: %s", format)
reporter.Log("   Strip dirs: %d", stripDirs)  // conditional
```

**`link_dependencies.go`** (lines 132-183):
```
reporter.Log("   Linking %d library file(s) from %s", len(entries), libVersionDir)
reporter.Log("   - Already linked: %s", entry.Name())    // per-file
reporter.Log("   + Linked (symlink): %s -> %s", ...)     // per-file
reporter.Log("   + Linked: %s", entry.Name())            // per-file
```

**`run_command.go`** (lines 77-120):
```
reporter.Log("   Skipping (requires sudo): %s", cmdPattern)
reporter.Log("   Description: %s", description)          // conditional
reporter.Log("   Running: %s", command)
reporter.Log("   Working dir: %s", workingDir)           // conditional
reporter.Log("   Output: %s", outputStr)                 // conditional: non-empty output
reporter.Log("   Command executed successfully")
```

**`install_binaries.go`** (lines 166-349):
```
reporter.Log("   Installing %d file(s)", len(outputs))               // binaries mode
reporter.Log("   Installed (executable): %s -> %s", src, dest)       // per-file
reporter.Log("   Installed: %s -> %s", src, dest)                    // per-file
reporter.Log("   Installing directory tree to: %s", ctx.InstallDir)  // directory mode
reporter.Log("   Copying directory tree...")
reporter.Log("   Directory tree copied to %s", ctx.InstallDir)
reporter.Log("   %d output(s) will be symlinked: %v", ...)
```

**`install_libraries.go`** (lines 76-115):
```
reporter.Log("   Installing %d library file(s)", len(matches))
reporter.Log("   Installed symlink: %s", relPath)    // per-file
reporter.Log("   Installed: %s", relPath)            // per-file
```

**`download_file.go`** (lines 98-208):
```
reporter.Log("Using cached: %s", dest)       // cache hit: keep as Log()
reporter.Log("Retry %d/%d after %v...", ...) // retry warning: keep as Log()
reporter.Log("Downloading %s", name)         // non-TTY only guard already present
```

## Classification Decision

### Category A: Extraction details — silence entirely

`"   Format: %s"` and `"   Strip dirs: %d"` in `extract.go`.

These are format-detection internals. Format is always deterministic from the filename. Strip-dirs is a recipe parameter the user already wrote. Neither tells the user anything actionable. **→ Remove both Log() calls.**

The `"   Extracting: %s"` line is borderline but should become **Status()** (see category C below).

### Category B: Individual file operations — silence entirely

Per-file lines in `link_dependencies.go`:
```
reporter.Log("   - Already linked: %s", entry.Name())
reporter.Log("   + Linked (symlink): %s -> %s", ...)
reporter.Log("   + Linked: %s", entry.Name())
```

Per-file lines in `install_binaries.go` (binaries mode):
```
reporter.Log("   Installed (executable): %s -> %s", src, dest)
reporter.Log("   Installed: %s -> %s", src, dest)
```

Per-file lines in `install_libraries.go`:
```
reporter.Log("   Installed symlink: %s", relPath)
reporter.Log("   Installed: %s", relPath)
```

These are sub-items within an already-announced bulk operation. A recipe that links 6 library files currently produces 7 lines for that one step (1 header + 6 per-file). With Status() these would be invisible on non-TTY; with removal they vanish everywhere. Since the summary line ("Linking N library files from X", "Installing N file(s)") already conveys the milestone, individual file lines carry no actionable value in CI. **→ Remove all per-file Log() calls.** On TTY the summary line plus spinner is enough. On CI the summary milestone line is enough.

Exception: `"   - Already linked: %s"` is a skip notice; it could be promoted to a debug log rather than removed, but it must not remain as `Log()`.

### Category C: Current activity — Status()

Lines that announce what is happening *right now* (before the result is known):

**`extract.go`:**
```go
reporter.Status("Extracting %s", archiveName)   // replaces Log
```

**`run_command.go`:**
```go
reporter.Status("Running: %s", command)          // replaces Log
```

**`install_binaries.go` (directory mode):**
```go
reporter.Status("Copying directory tree...")     // replaces Log
```

These are in-progress activities. They belong on the spinner line (TTY) and as no-ops on CI. The distinction from Log() candidates: these describe what is *happening*, not what *happened*.

Note: `"   Working dir: %s"` and `"   Description: %s"` in `run_command.go` are configuration details that can be removed outright (the recipe author knows the working dir; description is visible in the recipe).

### Category D: Directory operations — Status()

**`install_binaries.go`:**
```go
reporter.Status("Installing directory tree to %s", ctx.InstallDir)
```

This is a setup announcement before the copy begins. It belongs in Status() alongside the copy line, or can be folded into the copy Status() call as a single message.

### Category E: Phase completion — silence or Log() with consolidation

**`run_command.go`:**
```go
reporter.Log("   Command executed successfully")
```

**`install_binaries.go` (directory mode):**
```go
reporter.Log("   Directory tree copied to %s", ctx.InstallDir)
reporter.Log("   %d output(s) will be symlinked: %v", len(outputs), ...)
```

"Command executed successfully" is redundant: absence of an error already signals success. **→ Remove.**

"Directory tree copied to X" followed by "N outputs will be symlinked" is two lines describing the same completion event. They can be collapsed into one Log() milestone:
```go
reporter.Log("Installed directory tree (%d output(s))", len(outputs))
```

The "Installing directory tree to: X" line (currently Log) becomes Status(), so the only permanent CI line for a directory install is the single completion Log.

### Category F: Key milestones — keep as Log()

These should remain `Log()` because they represent a completed, observable step:

- `"Using cached: %s"` — tells CI the download was skipped from cache (valuable)
- `"Retry %d/%d after %v..."` — network retry is a notable event
- `"Downloading %s"` — already gated on non-TTY (`!progress.ShouldShowProgress()`)
- `"Linking %d library file(s) from %s"` — summary of link_dependencies step
- `"Installing %d file(s)"` — summary of install_binaries binaries mode
- `"Installing %d library file(s)"` — summary of install_libraries
- `"Skipping (requires sudo): %s"` — skip notices are meaningful in CI

## Full Classification Table

| File | Current Log() message | Decision | Rationale |
|------|----------------------|----------|-----------|
| extract.go | `Extracting: %s` | Status() | In-progress activity |
| extract.go | `Format: %s` | Remove | Internal detail |
| extract.go | `Strip dirs: %d` | Remove | Recipe parameter, not news |
| link_dependencies.go | `Linking %d library file(s) from %s` | Keep Log() | Milestone |
| link_dependencies.go | `- Already linked: %s` | Remove | Per-file skip detail |
| link_dependencies.go | `+ Linked (symlink): %s -> %s` | Remove | Per-file detail |
| link_dependencies.go | `+ Linked: %s` | Remove | Per-file detail |
| run_command.go | `Skipping (requires sudo): %s` | Keep Log() | Skip notice |
| run_command.go | `Description: %s` | Remove | Redundant with recipe |
| run_command.go | `Running: %s` | Status() | In-progress activity |
| run_command.go | `Working dir: %s` | Remove | Config detail |
| run_command.go | `Output: %s` | Keep Log() | Command output is signal |
| run_command.go | `Command executed successfully` | Remove | Redundant with no-error |
| install_binaries.go | `Installing %d file(s)` | Keep Log() | Milestone |
| install_binaries.go | `Installed (executable): %s -> %s` | Remove | Per-file detail |
| install_binaries.go | `Installed: %s -> %s` | Remove | Per-file detail |
| install_binaries.go | `Installing directory tree to: %s` | Status() | In-progress setup |
| install_binaries.go | `Copying directory tree...` | Status() | In-progress activity |
| install_binaries.go | `Directory tree copied to %s` | Consolidate → Log() | Milestone (merge with next) |
| install_binaries.go | `%d output(s) will be symlinked: %v` | Consolidate → Log() | Merge with previous |
| install_libraries.go | `Installing %d library file(s)` | Keep Log() | Milestone |
| install_libraries.go | `Installed symlink: %s` | Remove | Per-file detail |
| install_libraries.go | `Installed: %s` | Remove | Per-file detail |
| download_file.go | `Using cached: %s` | Keep Log() | Notable skip |
| download_file.go | `Retry %d/%d after %v...` | Keep Log() | Notable event |
| download_file.go | `Downloading %s` | Keep Log() | Already non-TTY gated |

## Expected Output Reduction

For a typical archive-download-extract-install recipe (e.g., node):

**Before:**
```
Downloading node-v25.9.0-linux-x64.tar.gz
   Extracting: node-v25.9.0-linux-x64.tar.gz
   Format: tar.gz
   Strip dirs: 1
   Installing 3 file(s)
   Installed (executable): bin/node -> bin/node
   Installed: CHANGELOG.md -> CHANGELOG.md
   Installed: LICENSE -> LICENSE
```
8 lines for one install.

**After:**
```
Downloading node-v25.9.0-linux-x64.tar.gz
   Installing 3 file(s)
```
2 lines for the same install. The extract step becomes Status() only (spinner on TTY, silent on CI).

For a `run_command` recipe:

**Before:**
```
   Running: mv bin/node bin/node.real
   Command executed successfully
```
2 lines.

**After:**
```
(nothing on CI — command execution is Status() on TTY only)
```
0 lines (the action's parent step already has a milestone line from a higher-level reporter caller).

## Assumptions

1. The higher-level install orchestrator (in `internal/install/` or `internal/executor/`) emits its own step-level Log() lines (e.g., "Installing node 25.9.0"). Action-level details are sub-steps, so removing action-level Log() calls does not leave CI output empty — the orchestrator lines remain.
2. `reporter.Status()` accepts a format string. The current interface signature is `Status(msg string)` — callers will need to use `fmt.Sprintf` before calling, or the interface should be extended. This is a mechanical API concern, not a classification concern.
3. "Command output" (`reporter.Log("   Output: %s", outputStr)`) is kept as Log() because a command's stdout/stderr may contain the only diagnostic signal when a subsequent step fails. This is valuable in CI.
4. The `nix_portable.go`, `gem_install.go`, `cargo_build.go`, `go_build.go`, etc. have similar patterns (config dump at start, "X output:\n%s", "X completed successfully") but are out of scope for this decision. The same classification principle applies: config dump → remove, in-progress step → Status(), tool output → Log(), completion notice → remove.

## Rejected Alternatives

- **All Log() → Status()**: Would silence CI entirely for action sub-steps. Command output (`"   Output: %s"`) carries diagnostic value that must not be a no-op on CI.
- **All Log() → keep**: The status quo. Produces 40+ lines per install on CI. Rejected by the user-observed problem.
- **Per-file lines → Status()**: Superficially appealing for TTY (shows current file). Rejected because these lines fire in a tight loop and the spinner already shows the operation name. On CI they would be no-ops (fine), but on TTY they would constantly kill and restart the spinner, producing flickering. Better to silence them entirely.
- **"Command executed successfully" → Log()**: Redundant. If the command failed, an error is returned. If it succeeded, no announcement is needed. Keeping it adds a line to CI output with zero signal value.
