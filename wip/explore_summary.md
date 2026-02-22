# Exploration Summary: gem-exec-wrappers

## Problem (Phase 1)
Decomposed gem recipes create bare symlinks that fail at runtime because `#!/usr/bin/env ruby` finds the system ruby, which doesn't know about the isolated GEM_HOME. All 83 rubygems recipes are affected.

## Decision Drivers (Phase 1)
- Correctness: executables must work after install
- Consistency: direct and decomposed paths should produce equivalent results
- Self-containment: no dependency on system ruby or global gem state
- Relocatability: wrappers must handle install dir moves
- Minimal change: touches a critical install path

## Research Findings (Phase 2)
- `gem_install.go` (lines 218-234) creates bash wrapper scripts that set GEM_HOME/GEM_PATH/PATH
- `gem_exec.go` (lines 470-492) creates bare relative symlinks
- `findBundler()` in gem_exec.go already locates ruby's bin directory
- The wrapper template from gem_install is proven and handles symlink resolution
- Decomposition pipeline (gem_install -> gem_exec via lock_data) is correct; only the executable exposure is broken

## Options (Phase 3)
- **Bash wrappers (chosen)**: Same approach as gem_install, create self-contained wrapper scripts
- **Recipe env_vars**: Push GEM_HOME/GEM_PATH into recipe TOML (rejected: 83 files, no relocatability)
- **Absolute symlinks**: Full paths instead of relative (rejected: doesn't solve GEM env vars)

## Decision (Phase 5)

**Problem:**
When `gem_install` recipes are decomposed into `gem_exec` for reproducible installs, the `executeLockDataMode()` function creates bare symlinks instead of self-contained wrapper scripts. The symlinked executables use `#!/usr/bin/env ruby` shebangs that resolve to system ruby, which can't find gems in the isolated install directory. This breaks all 83 rubygems recipes at runtime.

**Decision:**
Replace bare symlink creation in `gem_exec.go`'s `executeLockDataMode()` with wrapper script generation, and extract the wrapper template to shared code in `gem_common.go`. The wrapper sets `GEM_HOME`, `GEM_PATH`, and `PATH` to the isolated install directory, using the same proven pattern as `gem_install.go`. The ruby bin directory is derived from the existing `findBundler()` call, with a guard ensuring only tsuku-managed ruby is used.

**Rationale:**
Reusing the `gem_install` wrapper pattern is the lowest-risk fix for a bug that affects every rubygems recipe. Extracting the template to shared code prevents the two paths from diverging again -- exactly the condition that caused this bug. Alternatives like recipe-level env vars (83-file scatter) or absolute symlinks (doesn't solve GEM_HOME) were rejected for not addressing the root cause.

## Current Status
**Phase:** 8 - Review Final (complete)
**Last Updated:** 2026-02-22
