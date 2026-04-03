# Lead: How is ~/.tsuku/env generated?

## Findings

### 1. Where `~/.tsuku/env` is created
The env file is created in **two places**:

#### A. Install Script (website/install.sh)
**File:** `/website/install.sh`, lines 120-133
- Creates the file during initial tsuku installation
- Writes using `cat > "$ENV_FILE"` (shell redirect)
- Content: PATH exports for bin/ and tools/current/ directories
- Also appends `TSUKU_NO_TELEMETRY=1` if user opts out during install
- **Location:** Lines 119-133 write the static content; lines 126-133 conditionally append telemetry opt-out

#### B. Go Code: `internal/config/config.go`
**Function:** `EnsureEnvFile()` (lines 452-464)
- Called by: `internal/install/manager.go` line 70, in `InstallWithOptions()`
- Triggered: Every time a tool is installed via `tsuku install`
- Behavior: Idempotent — only writes if file doesn't exist or has different content
- Uses constant `envFileContent` defined at lines 444-446
- Content: Same as install.sh (PATH exports only)
- Permissions: 0644 (world-readable)

### 2. When `~/.tsuku/env` is created

| Scenario | Who Creates | When |
|----------|------------|------|
| Initial setup | `install.sh` | User runs install script from get.tsuku.dev |
| First tool install | Go code (`EnsureEnvFile`) | User runs `tsuku install <tool>` |
| Subsequent installs | Go code (`EnsureEnvFile`) | Every subsequent `tsuku install <tool>` (idempotent check) |
| Other commands | None | `tsuku doctor`, `tsuku shellenv`, etc. do NOT create/update env file |

### 3. Sync between install.sh and Go code
**Lines 443 & 444 in config.go:**
```go
// Keep in sync with website/install.sh (lines 115-119).
const envFileContent = `# tsuku shell configuration
...
```
The comment explicitly references that `envFileContent` must match install.sh. Currently they do match (both only export PATH).

### 4. What the env file contains (and what it lacks)

**Current content:**
```bash
# tsuku shell configuration
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
```

**What it does NOT contain:**
- Sourcing of `.init-cache.bash` or `.init-cache.zsh` (the shell init caches built for tools with install_shell_init)
- Any shell hooks
- Any other tool initialization

**Contrast with `tsuku shellenv` output:**
The `shellenv` command (cmd/tsuku/shellenv.go, lines 42-48) detects the current shell and outputs BOTH:
1. The PATH export (same as env file)
2. A conditional source of `.init-cache.<shell>` if it exists

**Key code (shellenv.go lines 42-48):**
```go
// Source the shell init cache if it exists.
// Detect the current shell to pick the right cache file.
shell := detectShellForEnv()
cachePath := filepath.Join(homeDir, "share", "shell.d", ".init-cache."+shell)
if _, err := os.Stat(cachePath); err == nil {
    fmt.Fprintf(os.Stdout, ". \"%s\"\n", cachePath)
}
```

### 5. Post-install cache rebuild
**File:** `cmd/tsuku/plan_install.go`, lines 115-128
- After `install_shell_init` actions run, post-install phase detects which shells were written to
- For each affected shell, calls `shellenv.RebuildShellCache(cfg.HomeDir, shell)`
- This rebuilds the `.init-cache.bash` and `.init-cache.zsh` files
- **But env file is never updated** — it remains unchanged

### 6. Updates to env file after creation
- **Never updated by tsuku itself** after initial creation
- Install script only runs once (initial setup)
- `EnsureEnvFile()` checks if content matches and skips rewrite if identical (idempotent)
- No mechanism to add init cache sourcing or update env file when tools install shell functions
- Environment variable `TSUKU_NO_TELEMETRY` can be appended by install.sh, but Go code does NOT respect or maintain this

### 7. Shell hook registration (not env file)
**File:** `internal/hook/install.go`
- Registers separate hooks in `.bashrc`, `.zshrc`, `.config/fish/conf.d/tsuku.fish`
- Hooks source `${TSUKU_HOME:-$HOME/.tsuku}/share/hooks/tsuku.bash` etc.
- This is separate from the env file and from shell.d init caches

## Implications

1. **The env file is a minimal bootstrap**: It only sets PATH, expecting users to rely on either:
   - The install script's shell config modification (adds `. "$ENV_FILE"` to dotfiles)
   - OR manual use of `eval "$(tsuku shellenv)"` for one-off sessions

2. **Critical gap between PATH setup and function availability**:
   - Users' `.bashrc` sources `~/.tsuku/env` (setup by install.sh)
   - But `~/.tsuku/env` does NOT source the init cache
   - So tool shell functions (installed by `tsuku install <tool>` with `install_shell_init`) silently don't load
   - The init cache exists and is built correctly, but is never sourced

3. **The init cache is orphaned**:
   - `RebuildShellCache()` is called after every tool installation
   - Cache files are built and maintained correctly
   - But nothing sources them in normal shell sessions (only `eval "$(tsuku shellenv)"` in interactive one-offs)

4. **Install.sh and Go code divergence risk**:
   - Both write the env file independently
   - Currently synchronized by a code comment (line 443), but this is a manual sync point
   - If Go code's `envFileContent` is updated but install.sh isn't, or vice versa, they could diverge
   - The telemetry opt-out added by install.sh (lines 126-133) is never added by Go code

## Surprises

1. **Go code never adds `TSUKU_NO_TELEMETRY` to env file**: The install script conditionally appends this (lines 127-132 of install.sh), but `EnsureEnvFile()` uses a constant `envFileContent` that never includes it. If a user installs tsuku via install.sh with telemetry opt-out, then later runs `tsuku install <tool>`, the Go code will NOT preserve the telemetry opt-out in the env file. (**Bug candidate**?)

2. **`EnsureEnvFile()` is non-fatal**: Called in `InstallWithOptions()` at line 70 with error suppression — warnings are printed but installation continues. This means env file corruption or permission issues won't block tool installation.

3. **No update path after init cache changes**: There's no command or mechanism to update an existing `.tsuku/env` file after initial creation. If the logic changes (e.g., to add init cache sourcing), users' files won't automatically get the new content.

## Open Questions

1. **Is the telemetry opt-out loss a bug or accepted behavior?** If the env file is only meant to bootstrap PATH (not to persist all options), why does install.sh append it? Does the telemetry code check `TSUKU_NO_TELEMETRY` env var directly, or does it require it in the env file?

2. **How should the init cache be sourced in new terminals?** The current design has three paths:
   - Install script adds `. "$ENV_FILE"` to shell config
   - `tsuku shellenv` outputs both PATH and init cache source
   - Prompt hooks (`hook-env`) can trigger activation
   
   But only the last two actually load the init cache. Why doesn't the env file source it?

3. **Should the env file be generated or static?** Currently it's static (constant `envFileContent`). Should it be templated to include init cache sourcing, or is there a reason to keep it minimal?

4. **What prevents `EnsureEnvFile()` from running during doctor/other commands?** Only `InstallWithOptions()` calls it. Should `tsuku doctor --fix` rebuild the env file to include init cache sourcing?

## Summary

The `~/.tsuku/env` file is created in two places: (1) by the install script during initial setup, and (2) by `EnsureEnvFile()` during every `tsuku install <tool>` command. It remains static and only contains PATH exports; it never sources the `.init-cache.bash`/`.zsh` files that are built when tools install shell functions. The file is idempotent (skips rewrite if content matches) but is never updated after creation, meaning tools' shell functions silently fail to load in new terminals, even though the init cache files exist and are correct.

