# Lead: information density

## Findings

### Event Taxonomy During `tsuku install <tool>`

The executor and install_deps orchestrate a deterministic sequence:

1. **Version Resolution (Phase 1)**
   - Location: `install_deps.go:getOrGeneratePlan()` calls `resolver.ResolveVersion()`
   - Event: Version constraint resolved to concrete version (e.g., "1.7" → "1.7.3")
   - User need: Mostly transient; already suppressed except when `--fresh` is used
   - Current output: `printInfof("Generating plan for %s@%s\n", cfg.Tool, resolvedVersion)` (one line)

2. **Dependency Resolution & Installation**
   - Location: `install_deps.go:installWithDependencies()` (line 290-316)
   - Events: 
     - Announce dependency set ("Checking dependencies for X...")
     - Per-dependency: announce resolution attempt ("Resolving dependency 'foo'...")
     - Recursively install each dependency depth-first (`installSingleDependency()`)
   - User need: **Critical design question** — users need to understand if dependencies are being installed, but current granularity is excessive
   - Current output:
     ```
     printInfof("Checking dependencies for %s...\n", toolName)
     printInfof("  Resolving dependency '%s'...\n", dep)  // per-dep
     ```
   - Executor shows per-dependency step execution (line 775): `fmt.Printf("   Step %d/%d: %s\n", i+1, len(dep.Steps), step.Action)`

3. **Plan Generation** (Phase 2 — happens only if not cached)
   - Location: `install_deps.go:getOrGeneratePlan()` → `executor.GeneratePlan()`
   - Events: Decompose composite actions to primitives; validate each action
   - User need: Opaque to user; only matters if it fails
   - Current output: Silent unless error

4. **Main Tool Step Execution**
   - Location: `executor.go:ExecutePlan()` (lines 451–502)
   - For each step (download, extract, chmod, install_binaries, run_command, etc.):
     ```
     fmt.Printf("Step %d/%d: %s\n", i+1, len(allSteps), step.Action)
     // Then action-specific sub-output
     ```
   - Per-action sub-output examples (from individual actions):
     - `download_file.go:104`: `fmt.Printf("   Downloading: %s\n", url)`
     - `download.go:304`: `fmt.Printf("   Downloading: %s\n", url)` + checksum fetch + signature verification (lines 473, 552)
     - `extract.go`: no explicit step announcement; happens silently in action body
     - `install_binaries.go:Execute()`: no explicit output; just returns
     - `cargo_build.go`: `fmt.Printf("   Downloading crate...")`, then `fmt.Printf("   Extracting crate...")`, then `fmt.Printf("   Installing executables...")`
     - `run_command.go:98`: `fmt.Printf("   Running: %s\n", command)` + optional description + optional working_dir
     - `chmod.go`, `set_env.go`, `link_dependencies.go`: mostly silent; no user-facing status
   - User need: **Highly context-dependent**
     - Download progress: essential for large files (40 MB→400 MB+); less relevant for small files
     - Extract: typically <1 sec; showing it is visual noise
     - install_binaries: silent is fine; user expects tools to appear
     - run_command: output depends on command; may include actual tool initialization
   - Current output: Step line + variable sub-output per action

5. **Post-Install Phase (Phase 3)**
   - Location: `executor.go:ExecutePhase()` (lines 519–556) for "post-install" steps
   - Events: `install_shell_init` (write .d files), cleanup action recording
   - User need: Silent on success; only show if it fails
   - Current output: `fmt.Printf("Executing phase %q: %d steps\n", phase, len(phaseSteps))` then per-step

6. **Verification Phase (Phase 4)**
   - Location: `install_deps.go:RunToolVerification()` (referenced at line 579)
   - Events: Run recipe's verify command, pattern match output
   - User need: Show progress for slow verifications; suppress on fast success
   - Current output: `printInfo("Verifying installation...")` (one line); then results

### Action Type Categorization

**Composite actions** (decompose to primitives during plan generation):
- `download` — resolves to `download_file` + signature/checksum steps
- `cargo_build`, `go_build`, `cmake_build`, etc. — decompose to download, extract, configure, make
These emit nested sub-steps during plan generation, then invisible during plan execution.

**Transient-output actions** (user needs spinner, not logged):
- `download_file`, `extract` — typically <5 seconds; show progress bar only
- `chmod`, `set_rpath`, `link_dependencies` — <1 second; silent or minimal
- `install_binaries` (when small) — silent

**Long-running actions** (show detailed progress):
- `cargo_install`, `go_install`, `npm_install`, `pipx_install` — can take 30+ seconds; need feedback
- `run_command` (depends on command; e.g., `configure` runs for 5-30 sec) — show output or spinner
- `configure_make`, `cmake_build`, `go_build`, `cargo_build` — 10-300 seconds; compilation is verbose

**Silent system actions**:
- `require_system` — validation only; no install
- `require_command` — check if tool exists; no install
- `manual_action` — warn user to do something manually; no automated output

**Low-density actions** (informational, not actionable):
- `set_env`, `shell_init`, `group_add`, `service_enable` — silent on success

### Download Progress Mechanics

Currently:
- `actions/download_file.go` and `actions/download.go` each call `httputil.HTTPDownload()`
- This uses `progress.Writer` to display per-download progress bar on stdout/stderr
- Progress bar shows: `[===>   ] 45% (12MB/27MB) 2.3MB/s ETA: 00:06`
- **Problem**: Download progress is shown as separate widget; no unified status line with step name

**Key insight**: When downloading `kubectl-1.29.3-linux-amd64` (40 MB), progress bar is essential. When downloading a 2 KB checksum file, showing progress is wasted real estate. When downloading multiple files in sequence, current UX shows `Step 1/5: download_file`, then `Downloading: ...`, then progress bar — three separate messages for one coherent action.

### Dependency Resolution Information Flow

**Main tool dependencies** (declared in recipe metadata):
- `recipe.Metadata.Dependencies` — install-time dependencies (e.g., openssl for go_build)
- `recipe.Metadata.RuntimeDependencies` — runtime dependencies (e.g., ruby for some gems)
- All are installed before plan generation; thus dependency steps appear *before* main tool steps in total count

**Nested dependency steps** (dependencies of dependencies):
- If tool A depends on B, and B depends on C:
  - `executor.installDependencies()` (line 657) recursively processes depth-first
  - Each dependency gets its own work directory and install directory
  - Each dependency's steps are executed and shown separately
  - **Current output overhead**: For A → [B, C], users see:
    ```
    Installing dependency: B@1.2.3
       Step 1/3: download_file
       Step 2/3: extract
       Step 3/3: install_binaries
    ✓ Installed B@1.2.3
    Installing dependency: C@1.0.0
       Step 1/2: download_file
       Step 2/2: install_binaries
    ✓ Installed C@1.0.0
    Executing plan: A@2.0.0
       Total steps: 6 (including 5 from dependencies)
    Step 1/6: download_file
    ...
    ```

### Verbosity Audit

**Lines per typical install** (happy path, no errors):
- Version resolution: 1 line
- Dependency check announcement: 1 line per dependency + 1 line per dep (1–5 deps typical)
- Main executor header: 3 lines (tool name, work dir, step count)
- Per-step: 1 line for step name, 1–3 lines for sub-actions
- Per-dependency install: 5 lines (header, steps, footer)
- Post-install: 1 line
- Verification: 1 line
- Final success: 2–3 lines

**Real-world example** (hypothetical: `tsuku install kubectl` with dependencies on `openssl`):
```
Generating plan for kubectl@1.29.3
Checking dependencies for kubectl...
  Resolving dependency 'openssl'...
Installing dependency: openssl@3.1.2
   Step 1/3: download_file
   Downloading: https://...openssl-3.1.2.tar.gz
   [==>        ] 23% (5.2MB/22MB) 1.8MB/s ETA: 00:09
   Step 2/3: extract
   Step 3/3: configure_make
   Skipping (requires sudo): run sudo make install
   ✓ Installed openssl@3.1.2

Executing plan: kubectl@1.29.3
   Work directory: /tmp/action-validator-xyz
   Total steps: 6 (including 3 from dependencies)

Step 1/6: download_file
   Downloading: https://...kubectl-1.29.3.tar.gz
   [========>  ] 78% (31.2MB/40MB) 8.5MB/s ETA: 00:01
Step 2/6: extract
Step 3/6: install_binaries
Step 4/6: chmod
Step 5/6: run_command
   Running: ln -s {install_dir}/bin/kubectl {current_dir}/kubectl
   ✓ Command executed successfully
Step 6/6: run_command
   Running: kubectl version --client

Verifying installation...
kubectl version v1.29.3

Installation successful!
```

That's **~35 lines for one install with one dependency**. For a tool with 3–5 transitive dependencies, output exceeds 100 lines.

### What Actions Actually Communicate

**Essential information** (users need this):
- Tool and version being installed
- If dependencies are being installed (and which ones)
- Progress on large downloads (>5 MB)
- Any manual intervention required (system dependencies, sudo commands)
- Verification result (pass/fail)

**Noise** (can be suppressed):
- "Step 1/5: download_file" (action name is not meaningful; URL is)
- Extraction step announcement (always succeeds; never actionable if shown)
- chmod step (silent is fine)
- Final status lines for each transient action
- Per-line output from small operations (<1 sec)

**Ambiguity**:
- Dependency resolution messages — should users see every dependency, or a summary?
- Long-running action output — should `run_command` show stdout, or just indicate it ran?
- Build logs — for `cargo_build`, `go_build`, users may want to see compiler output for debugging

### Comparison: Density in Peer CLIs

**cargo install** (Rust):
```
  Installing package from crates.io
    Updating index
    Downloaded ...
    Downloading ...
    Compiling ...
    Finished release ...
    Installing ...
```
Shows: phase transition + key milestones. No per-package step detail. Download size not shown. Compilation progress via spinner.

**brew install** (Homebrew):
```
==> Downloading https://...
######################################################################## 100.0%
==> Uncompressing ... from cache
==> Installing ...
```
Shows: phase name, progress bar for downloads, minimal per-step. No step count.

**npm install**:
```
npm notice
npm notice New minor version of npm available: 10.5.0 -> 10.6.0
npm notice To update run: npm install -g npm@latest
npm notice
added 156 packages in 5.3s
```
Shows: operation, count, duration. No step detail. Silent on success.

**Observation**: All three avoid step counters for the user. They show phases (Downloading, Compiling, Installing) and key events. Only downloads get a progress bar. Most output is deferred to end-of-operation summary.

## Implications

### Density Decision Tree

**For the happy path**, the UX redesign should:

1. **Hide step names** — Users don't care if the step is called "download_file" vs. "download_archive". They care about the semantic action (Downloading) and the artifact (the URL or name).

2. **Show tool + version once** — Not per-step. "Installing kubectl 1.29.3" at the top; subsequent lines are progress, not step names.

3. **Unify download progress** — Instead of:
   ```
   Step 1/6: download_file
   Downloading: https://...
   [==>  ] 45% ...
   ```
   Show:
   ```
   ↻ Downloading kubectl 1.29.3 (40 MB) ... [==>  ] 45%
   ```
   Or (in-place update):
   ```
   ↻ Downloading kubectl 1.29.3 ... 12.5 MB / 40 MB
   ```

4. **Aggregate dependency output** — Instead of showing every `Resolving dependency 'X'...` announcement:
   - Announce at the start: "Installing with 3 dependencies (openssl, zlib, libffi)"
   - Show per-dependency progress as one line: "↻ Installing openssl 3.1.2"
   - Suppress per-step output within each dependency unless it takes >5 seconds

5. **Show only long-running operations** — Extract, chmod, and small installs should be silent (or show spinner if >2 seconds). Only actions taking >3–5 seconds should appear in the status line.

6. **Deferred summary** — After completion, show a one-line summary (or two):
   ```
   ✓ kubectl 1.29.3 installed to ~/.tsuku/tools/kubectl-1.29.3
   ```

### Dependency Visibility Decision

**Current state**: "Checking dependencies for kubectl... Resolving dependency 'openssl'..." (too chatty)

**Proposed minimum**: Show dependency count and names in plan header:
```
Planning kubectl 1.29.3
  Dependencies: openssl, zlib (will be installed)
```

**Proposed medium**: Show dependency progress as single line per dependency:
```
↻ Installing openssl 3.1.2
✓ openssl 3.1.2
↻ Installing zlib 1.3.1
✓ zlib 1.3.1
↻ Installing kubectl 1.29.3
```

**Proposed maximum** (current approach): Show every step of every dependency (too verbose).

## Surprises

1. **No action emits human-readable descriptions by design** — Actions have `Name()` (e.g., "download_file") but no `Description()` or `UserLabel()` method. The step name and the action name are the same. This means the UI can't easily show "Downloading kubectl" vs. "Step 1/6: download_file" without adding a new abstraction.

2. **Composites hide complexity** — `download` is a composite that decomposes to `download_file` + signature/checksum steps during plan generation. Once a plan is generated, those sub-steps are invisible. This means the executor never sees "download" — it only sees primitives. **Implication**: Any UX that tries to show "Downloading..." must happen during action execution, not during step planning.

3. **Progress writer is already split** — `download_file.go` and other download actions call `httputil.HTTPDownload()` directly. To unify download progress into a single status line, we'd need to pass a progress callback (or unified Writer) to the HTTP layer, which doesn't exist today.

4. **Dependency depth is actually shallow** — The executor validates max depth of 5 and max total of 100 (lines 906–922). In practice, most recipes have 0–2 transitive dependencies. This means the "output explosion" from deep trees is unlikely; the real issue is per-step noise, not tree size.

5. **Phase system exists but is underused** — Executor supports "install" and "post-install" phases (line 509), allowing separation of main install from shell init. But it's not exposed to the UX; users don't see "Phase: post-install" announcements. This could be a hook for cleaner sequencing.

## Open Questions

1. **Should step names appear at all in minimal mode?**
   - Option A: Silent on success; show only tool name and progress. Show step names only on failure (`--verbose` flag).
   - Option B: Show step name only for long-running steps (>3 sec).
   - Option C: Show step name for all steps, but suppress step count (drop "Step 1/6").
   - **Needed from product**: What does a user do if they see no output for 30 seconds? Do they think the tool froze? Or is a spinner enough?

2. **What granularity for dependency announcements?**
   - Per-dependency lines (show names), or aggregated count?
   - Should dependency versions be shown, or just names?
   - If a dependency fails, should we unwind gracefully or show the full tree of what-depends-on-what?

3. **For build actions (cargo_build, go_build, cmake_build), should we show compiler output?**
   - These are long-running (10+ seconds). Do users want to see `Compiling ...` and `Linking ...` lines, or just a spinner?
   - Should there be a `--verbose` flag that enables full output?

4. **In non-TTY mode (pipes, CI), what should the output look like?**
   - One line per action (action name + result)?
   - Action name + duration + bytes/count (e.g., `download_file: 2.3s (40.2 MB)`)?
   - Silent until completion, then summary?
   - JSON for CI integration?

5. **Should the "total steps" count include dependency steps, or only main tool steps?**
   - Current: Includes all (e.g., "Total steps: 6 (including 3 from dependencies)")
   - User-focused: Might prefer "Installing kubectl... installing 3 dependencies..." without a step count at all.

6. **Is there a way to measure user frustration with current output?**
   - Usage data: Do users pipe output to `less` or `grep`?
   - Issue tracking: Any complaints about verbosity?
   - Telemetry: Can we measure `--quiet` adoption?

## Summary

The executor currently emits 1 line per step (name) plus variable per-action sub-output, resulting in 20–50 lines for typical installs. All peer CLIs hide step names and show only semantic phases + progress. Actions lack a `UserLabel()` method, so meaningful output (e.g., "Downloading kubectl 1.29.3") requires new abstractions or action-level changes. The biggest decision is whether to show step names at all: dropping them (plus step count) would cut noise ~70% with minimal loss of debugging info, while unifying download progress into a single in-place line would improve clarity. Dependency resolution currently announces every dependency by name; aggregating to a count or summary line would further reduce noise. Non-TTY fallback (pipes, CI) and verbose mode (`--verbose` with full compiler output for builds) remain open design questions.

