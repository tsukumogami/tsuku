---
status: Proposed
problem: |
  When gem_install recipes are decomposed into gem_exec for reproducible installs,
  the executeLockDataMode() function creates bare symlinks instead of self-contained
  wrapper scripts. The symlinked executables use #!/usr/bin/env ruby shebangs that
  resolve to system ruby, which can't find gems in the isolated install directory.
  This breaks all 83 rubygems recipes at runtime.
decision: |
  Replace bare symlink creation in gem_exec.go's executeLockDataMode() with wrapper
  script generation, and extract the wrapper template to shared code in gem_common.go.
  The wrapper sets GEM_HOME, GEM_PATH, and PATH to the isolated install directory,
  using the same proven pattern as gem_install.go. The ruby bin directory is derived
  from the existing findBundler() call, with a guard ensuring only tsuku-managed ruby
  is used.
rationale: |
  Reusing the gem_install wrapper pattern is the lowest-risk fix for a bug that
  affects every rubygems recipe. Extracting the template to shared code prevents
  the two paths from diverging again, which is exactly the condition that caused
  this bug. Alternatives like recipe-level env vars (83-file scatter) or absolute
  symlinks (doesn't solve GEM_HOME) were rejected for not addressing the root cause.
---

# DESIGN: gem_exec wrapper scripts for decomposed recipes

## Status

**Status: Proposed**

## Context and Problem Statement

Tsuku installs Ruby gems through two code paths. The original `gem_install` action runs `gem install` directly and creates self-contained bash wrapper scripts that set `GEM_HOME`, `GEM_PATH`, and `PATH` before launching the gem executable. The decomposed path converts `gem_install` into `gem_exec` at plan generation time, capturing a `Gemfile.lock` for reproducible installs via bundler.

The problem: `gem_exec`'s `executeLockDataMode()` creates bare relative symlinks from `<install>/bin/<exe>` to the bundler-installed scripts at `ruby/<version>/bin/<exe>`. This was an oversight when `executeLockDataMode()` was implemented -- the wrapper pattern from `gem_install` wasn't carried forward to the new code path. These scripts use `#!/usr/bin/env ruby` shebangs, which resolve to whatever ruby is first in `PATH` at runtime. That ruby doesn't know about the gems installed in the isolated `GEM_HOME`, so the executable fails immediately:

```
$ solargraph --version
can't find gem solargraph (>= 0.a) with executable solargraph (Gem::GemNotFoundException)
```

This affects all 83 rubygems recipes since they all use `gem_install`, which decomposes to `gem_exec` during plan generation. Every decomposed gem recipe is broken at runtime.

There's a secondary issue: the `install_binaries` action that follows `gem_exec` in the plan gets the wrong path for the executable. It looks for `.gem/bin/<exe>` but the actual binary is at `ruby/<version>/bin/<exe>`, causing checksum verification to fail with "lstat ... no such file or directory".

### Scope

**In scope:**
- Making `gem_exec` lock_data mode produce working executables
- Fixing the binary path reported to `install_binaries`
- Ensuring wrappers work with tsuku's managed ruby

**Out of scope:**
- Changing the decomposition pipeline itself (gem_install -> gem_exec works correctly)
- Modifying the `gem_install` direct path (it works fine already)
- Adding new gem features or changing the recipe format

## Decision Drivers

- **Correctness**: Gem executables must work after installation. This is a blocking bug.
- **Consistency**: Both code paths (direct and decomposed) should produce functionally equivalent results.
- **Self-containment**: Wrappers must not depend on system ruby or global gem state.
- **Relocatability**: Wrappers should work if the install directory moves (tsuku allows this for version management).
- **Minimal change**: The fix should be as small as possible since it touches a critical install path.

## Considered Options

### Decision 1: How to create working executables in lock_data mode

The core question is how `gem_exec`'s `executeLockDataMode()` should expose gem executables to users. Currently it creates bare symlinks that lack the environment setup needed to find installed gems. The working `gem_install` path shows that wrapper scripts solve this, but there are different ways to structure them.

The key constraint is that bundler installs gems into a versioned subdirectory (`ruby/<version>/`) with executables that use `#!/usr/bin/env ruby` shebangs. At runtime, the correct ruby must be on PATH and `GEM_HOME`/`GEM_PATH` must point to the install directory so ruby can find the gem's libraries.

#### Chosen: Bash wrapper scripts (same approach as gem_install)

Replace the bare symlink creation in `executeLockDataMode()` (lines 470-492 of `gem_exec.go`) with wrapper script generation that matches what `gem_install.go` does (lines 218-234). The wrapper:

1. Resolves its own location by following symlinks (handles `$TSUKU_HOME/bin/<exe>` -> `tools/<name>-<ver>/bin/<exe>`)
2. Derives `INSTALL_DIR` from `SCRIPT_DIR` (one directory up from `bin/`)
3. Sets `GEM_HOME` and `GEM_PATH` to `INSTALL_DIR`
4. Adds the tsuku ruby `bin/` directory to `PATH`
5. Calls `exec ruby "$SCRIPT_DIR/<exe>.gem" "$@"` to run the original bundler-generated script

The ruby bin directory is discovered at install time using `findBundler()`, which already exists in `gem_exec.go`. The directory containing the bundler executable is the ruby bin directory.

This approach reuses a proven pattern. The `gem_install` wrapper has been working for the direct install path. Using the same structure means both paths produce identical wrapper scripts, making behavior consistent regardless of which path was taken.

#### Alternatives Considered

**Environment variables in the recipe TOML**: Add `env_vars` to gem recipes that set `GEM_HOME`/`GEM_PATH` at runtime.
Rejected because this pushes complexity into every recipe (83 files) instead of fixing the action once. It also can't handle relocatability since the paths would be hardcoded at recipe authoring time.

**Modify the symlink targets to use full paths**: Instead of relative symlinks, create absolute symlinks to the bundler-installed scripts and rely on tsuku's PATH setup to find the right ruby.
Rejected because this doesn't solve the `GEM_HOME`/`GEM_PATH` problem. Even if ruby is correct, it still won't find the gems without the environment variables. Absolute symlinks also break relocatability.

**Bundler binstubs (`bundle binstubs <gem>`)**: Let bundler generate its own wrapper-like scripts via the binstubs feature.
Rejected because binstubs depend on the `Gemfile` and `.bundle/config` being present at runtime. They resolve paths relative to the bundle config directory, not the install directory. This breaks self-containment since the wrapper would need bundler infrastructure at runtime rather than just ruby and the gem files.

### Decision 2: How to find the ruby bin directory at install time

The wrapper script needs to hardcode the ruby `bin/` directory in its `PATH` line so the correct ruby is used at runtime. The question is where to get this path during `executeLockDataMode()`.

`gem_install.go` gets it by looking up the `gem` executable and using its parent directory. `gem_exec.go` already has `findBundler()` which locates the `bundle` executable in tsuku's ruby installation. The directory containing `bundle` is the same ruby `bin/` directory we need.

#### Chosen: Derive from findBundler() result with tsuku-managed guard

Take the directory of the bundler path returned by `findBundler()`. This is already called during `executeLockDataMode()` (line 381), so the path is available. Use `filepath.Dir(bundlerPath)` to get the ruby bin directory.

**Important guard**: `findBundler()` has a system fallback that returns paths like `/usr/bin/bundle`. If the bundler path isn't under `ctx.ToolsDir`, the implementation must fail with a clear error ("gem_exec lock_data mode requires tsuku-managed ruby") rather than hardcoding a system path into the wrapper. This ensures the relocatability invariant holds -- system paths aren't relocatable.

#### Alternatives Considered

**Search for ruby directly**: Use `exec.LookPath("ruby")` or glob for `ruby-*/bin/ruby` in the tools directory.
Rejected because `findBundler()` is more reliable -- if bundler was found, ruby must be in the same directory. A separate lookup could find a different ruby installation.

**Store ruby path during decomposition**: Pass the ruby bin path as a parameter in the decomposed step.
Rejected because it adds a new parameter to the action schema and the ruby installation path might change between plan generation and execution (e.g., ruby gets updated). Better to discover it at execution time.

## Decision Outcome

**Chosen: Bash wrappers + derive ruby path from findBundler()**

### Summary

The fix replaces bare symlink creation in `gem_exec.go`'s `executeLockDataMode()` with wrapper script generation, and extracts the wrapper template to shared code so both gem paths use identical logic.

After bundler installs the gems, instead of creating `bin/<exe> -> ../ruby/<ver>/bin/<exe>` symlinks, the code will:

1. Verify the bundler path is under `ctx.ToolsDir` (prevents hardcoding system paths)
2. Copy the bundler-generated script to `rootBinDir/<exe>.gem` (the source lives in a different directory than `rootBinDir`, so a cross-directory copy is needed)
3. Create a bash wrapper at `rootBinDir/<exe>` that sets `GEM_HOME`, `GEM_PATH`, and `PATH`
4. The wrapper uses `BASH_SOURCE[0]` with symlink resolution to locate itself, then derives paths from its location

The ruby bin directory is obtained from `filepath.Dir(bundlerPath)` where `bundlerPath` comes from the existing `findBundler()` call, after confirming it's a tsuku-managed path. This is the same directory that contains the `ruby` executable.

The wrapper template is extracted to `internal/actions/gem_common.go` so both `gem_install.go` and `gem_exec.go` call the same function. This prevents the two paths from diverging again, which is exactly what caused this bug.

For the secondary issue (wrong binary path for `install_binaries`), the fix ensures the wrapper scripts are at the expected `bin/<exe>` location, which is where `install_binaries` looks for them.

Edge cases the implementation must handle:
- Multiple executables per gem (e.g., solargraph has both `solargraph` and `solargraph-runtime`)
- The bundler-generated script might be in different locations depending on bundler version (the `findBundlerBinDir()` fallback logic handles this)
- Existing symlinks must be cleaned up before creating wrapper files
- System bundler fallback: must error if `findBundler()` returns a path outside `ctx.ToolsDir`

### Rationale

Reusing the `gem_install` wrapper pattern is the lowest-risk approach. The wrapper script template is proven code that handles symlink resolution, PATH setup, and GEM environment configuration. Both decisions reinforce each other: the wrapper script needs a ruby path, and `findBundler()` naturally provides it since bundler lives alongside ruby.

The alternative of pushing environment setup into recipes or symlink targets would either scatter the fix across 83 files or fail to solve the core problem (`GEM_HOME`/`GEM_PATH` must be set).

## Solution Architecture

### Overview

The change is contained within a single function: `executeLockDataMode()` in `internal/actions/gem_exec.go`. The current symlink creation block (lines 470-492) gets replaced with wrapper script generation.

### Components

**Modified function: `executeLockDataMode()`**

Current flow:
```
bundler install -> find executables -> create symlinks
```

New flow:
```
bundler install -> find executables -> get ruby bin dir -> create wrapper scripts
```

**Wrapper script template** (reused from gem_install.go):

```bash
#!/bin/bash
# tsuku wrapper for <exe> (sets GEM_HOME/GEM_PATH for isolated gem)
SCRIPT_PATH="${BASH_SOURCE[0]}"
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
    SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
    [[ $SCRIPT_PATH != /* ]] && SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_PATH"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
INSTALL_DIR="$(dirname "$SCRIPT_DIR")"
export GEM_HOME="$INSTALL_DIR"
export GEM_PATH="$INSTALL_DIR"
export PATH="<ruby-bin-dir>:$PATH"
exec ruby "$SCRIPT_DIR/<exe>.gem" "$@"
```

### Key Interfaces

No new interfaces. The function signature of `executeLockDataMode()` stays the same. The only external change is that `bin/<exe>` is now a bash script instead of a symlink, which is transparent to callers since both are executable.

### Data Flow

1. `executeLockDataMode()` calls `findBundler(ctx)` -> gets `/path/to/ruby-<ver>/bin/bundle`
2. Verify bundler path is under `ctx.ToolsDir` (guard against system fallback)
3. `filepath.Dir(bundlerPath)` -> `/path/to/ruby-<ver>/bin/` (ruby bin dir)
4. `findBundlerBinDir(installDir)` -> finds where bundler put gem executables (e.g., `ruby/<ver>/bin/`)
5. For each executable:
   a. Copy from bundler bin dir to `rootBinDir/<exe>.gem` (the source may be in a different directory than `rootBinDir`, so rename isn't sufficient -- must copy across directories)
   b. Write wrapper script to `rootBinDir/<exe>` with mode 0755
6. `install_binaries` finds `rootBinDir/<exe>` (now a wrapper) at expected path

## Implementation Approach

This is a single-phase change since all modifications are in one file.

### Changes

1. **`internal/actions/gem_exec.go`**: Replace lines 470-492 in `executeLockDataMode()`. Remove the symlink creation loop. Add:
   - Ruby bin dir discovery: `rubyBinDir := filepath.Dir(bundlerPath)`
   - For each executable: rename original to `.gem`, write wrapper script
   - Use `fmt.Sprintf` with the same wrapper template as `gem_install.go`

2. **Extract shared wrapper template**: Extract the wrapper template to a shared function in `internal/actions/gem_common.go` so both `gem_install.go` and `gem_exec.go` use the same code. This is required, not optional -- the current bug exists precisely because two code paths diverged. A shared template prevents recurrence.

3. **Add tsuku-managed bundler guard**: Before using `filepath.Dir(bundlerPath)`, verify the path is under `ctx.ToolsDir`. If not, return an error directing the user to install ruby via tsuku.

4. **Add null byte validation**: `gem_exec.go`'s executable name validation (lines 351-361) should check for null bytes and control characters, matching the validation in `gem_install.go` (lines 104-107).

### Testing

- Existing `gem_exec` tests should be updated to verify wrapper scripts are created instead of symlinks
- Add a test case that validates the wrapper script content (GEM_HOME, GEM_PATH, PATH lines)
- Functional test: `tsuku install solargraph` should produce a working `solargraph --version`

## Security Considerations

### Download Verification

Not applicable to this change. Download verification happens upstream in the install pipeline (checksum validation of gem packages). This change only affects how executables are exposed after installation.

### Execution Isolation

The wrapper scripts run gem executables with the same permissions as the user. They set `GEM_HOME` and `GEM_PATH` to the isolated install directory, which actually *improves* isolation compared to the current bare symlinks. With bare symlinks, the system ruby's global gem path could leak in. With wrappers, the environment is explicitly scoped to the install directory.

The wrapper uses `exec ruby` which replaces the bash process, so there's no persistent shell process. The `PATH` modification is scoped to the exec'd process only.

### Supply Chain Risks

No change to supply chain risk from this fix. The gem packages come from the same source (rubygems.org via bundler with lockfile enforcement). The wrapper script is generated locally from a template, not downloaded from an external source.

**Trust boundary note**: The `lock_data` content is generated by tsuku's own decomposition pipeline at plan generation time and replayed at install time with `BUNDLE_FROZEN=true`. This design assumes `lock_data` is always tsuku-generated. If `lock_data` ever accepted external content, `GIT` source entries in the lockfile could point to arbitrary repositories. This is a pre-existing trust boundary, not introduced by this change, but worth documenting: the `os.WriteFile(lockPath, ...)` call site should carry a comment noting that `lock_data` must be trusted input.

### User Data Exposure

No change to user data exposure. The wrapper scripts don't transmit data. They only set environment variables (`GEM_HOME`, `GEM_PATH`, `PATH`) and exec the gem executable. These environment variables contain only local filesystem paths within the tsuku install directory.

## Consequences

### Positive

- All 83 rubygems recipes work correctly after decomposition
- Consistent behavior between direct (`gem_install`) and decomposed (`gem_exec`) paths
- Wrappers are relocatable, working if the install directory is moved
- The `install_binaries` secondary issue is resolved since wrappers live at `bin/<exe>`

### Negative

- Wrapper scripts are bash-specific (won't work on systems without bash)

### Mitigations

- Bash is already a requirement for tsuku on all supported platforms (Linux, macOS). The `gem_install` path already uses bash wrappers without issue.
- Template extraction to shared code is a required part of the implementation, preventing the two paths from diverging again.
