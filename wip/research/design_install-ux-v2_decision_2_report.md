# Decision: Output Channel Classification for `tsuku install`

**Decision ID**: design_install-ux-v2_decision_2  
**Date**: 2026-04-21  
**Status**: Decided

## Question

For each output category in `tsuku install`, which channel should it use:
`Status()` (ephemeral spinner text), `Log()` (permanent line), `DeferWarn()`, or silenced?

## Background

The `progress.Reporter` interface provides:

- `Status(msg)` — ephemeral: overwrites spinner line on TTY, no-op on non-TTY
- `Log(format, args)` — permanent: writes a line after clearing spinner (TTY), plain line on non-TTY
- `DeferWarn(format, args)` — permanent warning queued to end
- `Stop()` — clears spinner

The goal is:
- **TTY**: one in-place line for the whole install, e.g. `⣾ Installing serve@14.2.6...`
- **Non-TTY (CI)**: 3–5 permanent lines total, e.g. `Installing serve@14.2.6` → `Installed serve@14.2.6`
- The existing user test produced 40+ lines. Nearly all middle-phase output must be silenced.

## Current State of Each Category

### 1. "Generating plan for {tool}@{version}" — `install_deps.go:111`

`printInfof(...)` → `fmt.Printf` → stdout, permanent, always visible.

**Decision: Status()**

This is pure orchestration noise. The user only needs to know *what* is being installed, not the plan-generation phase. On TTY it drives the spinner; on non-TTY it disappears.

### 2. "Installing eval-time dependencies: [nodejs]" — `eval.go:333`

`fmt.Fprintf(os.Stderr, ...)` → stderr, no reporter, no quiet-mode guard.

**Decision: Status()**

Eval deps are an implementation detail of plan generation. The spinner text `Installing nodejs (eval dep)...` or simply updating the existing spinner message is sufficient. If the dep install fails, that becomes a `Log()` error.

### 3. "Installing {dep}..." / "Installed {dep}" — `eval.go:338,342`

Same context as #2. These are the per-dep lifecycle lines within eval dep installation.

**Decision: Status()** for the in-progress line; **silence** the "Installed {dep}" line.

The outer spinner already communicates activity. The completion line adds no value because the parent install will either succeed or show an error.

### 4. "Checking runtime dependencies for {tool}..." — `install_deps.go:307`

`printInfof(...)` → stdout. Emitted inside `installWithDependencies` before the inner loop.

**Decision: Status()**

Ephemeral progress. Not something the user needs to retain.

### 5. "Resolving runtime dependency '{dep}'..." — `install_deps.go:310`

`printInfof(...)` → stdout. One line per dep, inside the loop.

**Decision: Status()**

Same reasoning. Replace spinner text for each dep as it resolves; no permanent record needed.

### 6. "Installed library to: {path}" — `internal/install/library.go:42`

`fmt.Printf("   Installed library to: %s\n", libDir)` → stdout, no reporter.

**Decision: Silence (remove or guard behind verbose flag)**

Library installation is an internal sub-step of tool installation. The user cares that the tool installed, not where each transient library directory landed. If the install fails, the error message supplies the path. For debugging, a `--verbose` flag or `DeferWarn` could surface it, but it must not appear in normal output.

### 7. Verify sub-steps — `cmd/tsuku/verify.go`

`printInfof("Verifying {tool} (version {v})...\n")`, `printInfof("  Running: ...\n")`, `printInfof("  Output: ...\n")`, `printInfof("  Step 1/2/3/4/5...\n")`, etc.

These all go through `printInfof` → `fmt.Printf`. Only called when `opts.Verbose == true`, which is set by the `install` flow (`Verbose: true` in `RunToolVerification`).

**Decision:**
- "Verifying {tool}@{version}" sub-step label → **Status()** (already has a `reporter.Log("Verifying %s@%s", ...)` call in `install_deps.go:569` — that one single line stays as `Log()`)
- All inner sub-step lines (`Step 1`, `Running:`, `Output:`, `Pattern matched`, `Tier 2`, soname results, etc.) → **Silence in post-install path** (set `Verbose: false` when called from install flow)

The single `reporter.Log("Verifying %s@%s", ...)` call at the top of the verification block in `install_deps.go` is the only permanent line needed. Everything inside `RunToolVerification` should be silent during install; verbose detail remains available via `tsuku verify --verbose`.

### 8. Action sub-steps — `internal/actions/` via `reporter.Log()`

Examples: `"   Extracting: ..."`, `"   Linking N files"`, `"   Running: mv ..."`, `"   Flake ref: ..."`, `"   PKG_CONFIG_PATH: ..."`, etc.

These use `reporter.Log(...)` directly, so they already go through the reporter. Currently they produce permanent lines.

**Decision: Convert all action sub-step `reporter.Log()` calls to `reporter.Status()`**

Action internals are implementation noise. On TTY the user gets live feedback through the spinner. On non-TTY (CI) they disappear, which is the right trade-off: if a step fails, the error message is what matters. The download progress bar (via `progress_writer.go`) already uses a separate mechanism and is not affected.

Exception: if an action detects a meaningful condition worth warning about (e.g., "Skipping: requires sudo"), that should use `reporter.DeferWarn()` so it surfaces after the install completes without interrupting the flow.

### 9. "📍 Installed to: {path}" — `internal/install/manager.go:124,135,153`

`fmt.Printf(...)` → stdout. Called from `InstallWithOptions`, no reporter access.

**Decision: Silence (remove)**

The completion line `reporter.Log("%s@%s installed", toolName, version)` in `install_deps.go:604` already confirms success. The install path is an implementation detail; users who need it can run `tsuku info <tool>`. The emoji also violates project conventions.

### 10. "🔗 Wrapped N binaries: ..." / "🔗 Symlinked: ..." — `internal/install/manager.go:126–139`

Same location as #9. `fmt.Printf(...)` → stdout.

**Decision: Silence (remove)**

The symlink/wrapper creation is a sub-step of installation. Success is implied by the top-level completion line. The emoji violates project conventions. Failure already surfaces as an error.

### 11. PATH guidance ("To use the installed tool, add this to your shell profile...") — `install_deps.go:606-607`

`printInfo()` + `printInfof(...)` → stdout.

**Decision: DeferWarn() or Log()**

This is actionable for the user and should survive. On a fresh install where `$TSUKU_HOME/bin` is not yet in PATH, the user genuinely needs this. Use `DeferWarn()` so it appears after the completion line rather than interleaved with spinner output. If the shell hook is already configured (detectable via `shellenv`), this notice can be skipped.

### 12. Error messages — `fmt.Fprintf(os.Stderr, ...)` throughout

**Decision: Always permanent (keep as-is)**

Errors must always be visible. No change needed. Error output goes to stderr and is not gated by the reporter or quiet mode.

## Summary Table

| # | Output text | Current mechanism | Decision | Target channel |
|---|-------------|-------------------|----------|----------------|
| 1 | Generating plan for {tool}@{v} | `printInfof` (stdout) | Change | `Status()` |
| 2 | Installing eval-time dependencies: [...] | `fmt.Fprintf(stderr)` | Change | `Status()` |
| 3 | Installing {dep}... / Installed {dep} | `fmt.Fprintf(stderr)` | Change | `Status()` / silence |
| 4 | Checking runtime dependencies for {tool}... | `printInfof` (stdout) | Change | `Status()` |
| 5 | Resolving runtime dependency '{dep}'... | `printInfof` (stdout) | Change | `Status()` |
| 6 | Installed library to: {path} | `fmt.Printf` (stdout) | Change | Silence |
| 7 | Verify sub-steps (Step 1, Running:, Output:, etc.) | `printInfof` (stdout) | Change | Silence (set `Verbose: false` from install) |
| 7a | Verifying {tool}@{v} (top-level) | `reporter.Log(...)` | Keep | `Log()` |
| 8 | Action sub-steps (Extracting:, Linking N, Running: mv, etc.) | `reporter.Log(...)` | Change | `Status()` (warn-worthy → `DeferWarn()`) |
| 9 | 📍 Installed to: {path} | `fmt.Printf` (stdout) | Change | Silence |
| 10 | 🔗 Wrapped N binaries / Symlinked | `fmt.Printf` (stdout) | Change | Silence |
| 11 | PATH guidance | `printInfo/f` (stdout) | Change | `DeferWarn()` |
| 12 | All error messages | `fmt.Fprintf(stderr)` | Keep | stderr (permanent) |

## Expected permanent `Log()` lines per normal install

For `tsuku install serve` (with no library deps, no eval deps):

1. `Installing serve@14.2.6` — top-level status set as `Status()` at the start; *no permanent line at start*
2. `Verifying serve@14.2.6` — `Log()` ← this is the only mid-install permanent line
3. `serve@14.2.6 installed` — `Log()` at completion
4. PATH guidance (if needed) — `DeferWarn()` flushed after completion

Total: **2–3 permanent lines** on non-TTY. Errors add lines if they occur.

## Assumptions

1. The `reporter` is available at all call sites that need `Status()`. Currently several call sites (`eval.go`, `library.go`, `manager.go`) use `fmt.Printf/Fprintf` because they have no reporter reference. Threading the reporter through will require small API changes.
2. Action `reporter.Log()` calls in `internal/actions/` can be bulk-converted to `reporter.Status()`. This is a mechanical change with no logic implications.
3. Verify sub-steps in `RunToolVerification` are already behind `opts.Verbose`. The install path already passes `Verbose: true`; changing to `Verbose: false` from install context silences them without affecting `tsuku verify --verbose`.
4. The PATH guidance skip (when shell hook already configured) is a nice-to-have. The `DeferWarn()` conversion is sufficient even without the skip logic.

## Rejected alternatives

**Keep action sub-steps as `Log()`**: Rejected. This is the primary source of the 40+ line output. Each action in a multi-step recipe emits 3–8 lines, making CI logs unreadable.

**Silence verify completely (no `Log()` for "Verifying...")**: Rejected. The single top-level "Verifying {tool}@{v}" line is useful CI signal, especially when verification takes a few seconds.

**Route all output through a single "summary" line at the end**: Rejected. The intermediate `Verifying` line is worth keeping because verification can be slow (runs the binary, checks checksums) and users/CI benefit from knowing the install completed before verification started vs. during verification.

**Use `DeferWarn()` for action sub-steps instead of silencing**: Rejected. A deferred dump of 20 action sub-steps at the end is still noise. Only genuinely actionable conditions (sudo-skip, security bypass) warrant `DeferWarn()`.
