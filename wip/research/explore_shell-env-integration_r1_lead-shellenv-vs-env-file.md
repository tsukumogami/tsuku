# Lead: tsuku shellenv vs ~/.tsuku/env

## Findings

### 1. What `tsuku shellenv` does (cmd/tsuku/shellenv.go, lines 12-52)

`tsuku shellenv` is a command that **outputs shell commands** to configure PATH and load shell initialization scripts. It does two things:

1. **Outputs PATH export** (line 40):
   ```bash
   export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
   ```

2. **Conditionally sources the init cache** (lines 42-48):
   - Detects the current shell via `$SHELL` environment variable (defaulting to bash)
   - Checks if a shell-specific init cache exists at `$TSUKU_HOME/share/shell.d/.init-cache.<shell>`
   - If it exists, sources it: `. "/path/to/.init-cache.bash"`

**Usage:** `eval $(tsuku shellenv)` — intended to be run once at shell login or run interactively.

**Purpose:** It's a one-time initialization command that emits both static PATH setup and dynamic shell initialization.

### 2. What ~/.tsuku/env contains (internal/config/config.go, lines 425-427)

`~/.tsuku/env` is a **static file** created by `EnsureEnvFile()` that contains:

```bash
# tsuku shell configuration
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
```

**When created:** During every `tsuku install` invocation (internal/install/manager.go, line 70). The file is idempotent — it's only rewritten if missing or incorrect.

**When sourced:** Users are expected to source this in their `.bashrc` or `.zshrc` via hook install, or via the website installer (website/install.sh).

**What it does:** Sets PATH to include `$TSUKU_HOME/bin` and `$TSUKU_HOME/tools/current`. It does **NOT** source the shell.d init cache.

### 3. The gap: ~/.tsuku/env doesn't source the init cache

**Critical finding:** `~/.tsuku/env` outputs only the PATH export. It **does not** source `.init-cache.<shell>` even though `tsuku shellenv` does.

This means:
- Tools that use `install_shell_init` (like niwa) get shell functions written to `share/shell.d/{tool}.<shell>`
- `RebuildShellCache` (internal/shellenv/cache.go) aggregates these into `.init-cache.<shell>`
- But `.init-cache.<shell>` is **never sourced** when `.bashrc` sources `~/.tsuku/env`
- Users only get the init cache if they run `eval $(tsuku shellenv)` interactively

### 4. Why both mechanisms exist

**Timeline and design intent:**

1. **`~/.tsuku/env` (March 2026, commit 5f92f95e):** A persistent env file created during `tsuku install` so that shell setup persists across shells/terminals without requiring the install script. It's meant to be sourced from `.bashrc`/`.zshrc`.

2. **`tsuku shellenv` (earlier, predates investigated commits):** A one-off command for "users who install tsuku without the install script, or for development builds" (cmd/tsuku/shellenv.go, lines 15-18). It's a safety valve for non-standard installations.

3. **Shell integration Track B (March 28, 2026, commit b3e39be7):** Introduced `internal/shellenv/` package with activation, caching, and the shell.d system:
   - `install_shell_init` action writes tool-specific init scripts to `share/shell.d/`
   - `RebuildShellCache` aggregates these into `.init-cache.<shell>`
   - `tsuku hook-env` and `tsuku shell` commands can output the cache sourcing
   - **But:** This all assumes the init cache sourcing is added to shell config

4. **Design intent separation:**
   - `~/.tsuku/env` = static, persistent PATH setup for login shells (via .bashrc)
   - `tsuku shellenv` = dynamic output for interactive setup or alternate environments
   - `tsuku shell` / `tsuku hook-env` = project-aware per-directory activation (not the same as global setup)

### 5. The architectural issue

**Design document (DESIGN-shell-env-activation.md)** states the integration strategy explicitly:

Lines 164-180 explain the hybrid approach:
- `tsuku shellenv` is **unchanged** — static PATH only
- New `tsuku shell` and `tsuku hook-env` are project-specific
- `tsuku hook install --activate` installs prompt hooks that call `tsuku hook-env`

**But the init cache problem is not addressed in this design:** The document focuses on **per-project** PATH activation, not on sourcing the global init cache from login shells. There's no mention of how tool shell functions (from `install_shell_init`) should appear in a new terminal.

### 6. Evidence from code structure

**internal/shellenv/cache.go (RebuildShellCache):**
- Lines 13-29: Reads `share/shell.d/*.{shell}` files
- Lines 135-145: Wraps each tool's content in error-isolation blocks
- Writes aggregated result to `.init-cache.{shell}`
- **Assumption:** Someone will source `.init-cache.{shell}`

**cmd/tsuku/shellenv.go (line 47):**
```go
fmt.Fprintf(os.Stdout, ". \"%s\"\n", cachePath)
```
Sources it conditionally. **But this output is not in `~/.tsuku/env`.**

### 7. Design intent clarification

From the code and comments:

1. **`~/.tsuku/env` design intent:** A persistent file that users source once in `.bashrc`/`.zshrc` for basic PATH setup. Meant to work in containers, CI, and non-shell-init scenarios.

2. **`tsuku shellenv` design intent:** A fallback for users without the env file, or for interactive one-off setup. Outputs the same PATH plus any available init cache.

3. **`tsuku shell` / `tsuku hook-env` design intent:** Project-aware PATH modification on directory change. Solves the per-project tool versioning problem, not the global init cache problem.

**Implicit assumption:** Tools' shell functions are either:
- Loaded via `eval $(tsuku shellenv)` interactively (works)
- Loaded via prompt hook `tsuku hook-env` in new projects (works for project tools)
- **NOT loaded in new login shells** via `~/.tsuku/env` alone (the gap)

## Implications

1. **Users installing tools with `install_shell_init`:** Their shell functions won't work in new login terminals unless they either:
   - Run `eval $(tsuku shellenv)` manually
   - Install prompt hooks (which only activate on directory change within a project)
   - Manually add `. "$TSUKU_HOME/share/shell.d/.init-cache.<shell>"` to their `.bashrc`

2. **The env file was designed before shell.d existed:** Commit 5f92f95e (March 1, 2026) predates commit b3e39be7 (March 28, 2026) by 27 days. The env file's design didn't account for the init cache.

3. **This is a silent failure:** No warning, no error. Users just find that tool functions don't work in new terminals.

## Surprises

1. **`tsuku shellenv` is not what gets sourced:** I initially expected `~/.tsuku/env` to contain the output of `tsuku shellenv`. Instead, it's a hardcoded static file that was designed independently.

2. **The init cache is "orphaned":** `RebuildShellCache` atomically writes the cache and wraps files in error isolation (lines 135-145, internal/shellenv/cache.go), but nothing guarantees it gets sourced at login time.

3. **Design document doesn't address this:** DESIGN-shell-env-activation.md is purely about per-project activation. It doesn't mention sourcing the global init cache or integrating with `~/.tsuku/env`.

4. **The design was sequential, not integrated:** The env file was introduced first for basic PATH. The shell.d system was added later without updating how `~/.tsuku/env` is constructed.

## Open Questions

1. **Is this a bug or by design?** Should `~/.tsuku/env` source the init cache?

2. **If by design:** How are users expected to get tool shell functions in new terminals?
   - The README doesn't mention `eval $(tsuku shellenv)` or manually sourcing the init cache
   - Hook installation with `--activate` requires a `.tsuku.toml`, which is project-specific
   - The installer (website/install.sh) doesn't add cache sourcing to `.bashrc`

3. **What's the actual user flow?** 
   - Install a tool with `install_shell_init` → where do the shell functions appear?
   - Answer seems to be: only in projects with `.tsuku.toml`, or if user runs `eval $(tsuku shellenv)` manually

4. **Should there be a consolidation plan?**
   - Should `~/.tsuku/env` be generated dynamically (e.g., by running `tsuku shellenv > ~/.tsuku/env`)?
   - Should `tsuku hook install` (without `--activate`) add cache sourcing for login shells?

5. **Is fish shell supported for login-time setup?** The detection in cmd/tsuku/shellenv.go (lines 54-65) handles bash, zsh, and fish, but `~/.tsuku/env` is static (can't have shell-specific sourcing).

## Summary

`tsuku shellenv` outputs PATH setup **plus init cache sourcing** (if cache exists); `~/.tsuku/env` outputs **only PATH** as a static file created during install. The init cache (shell functions from tools) is rebuilt on `install_shell_init` but has no guarantee of being sourced at login time — only `tsuku shellenv` or project-specific hooks load it. This gap exists because the env file predates the shell.d system and was never updated; the design document for shell-env-activation focuses on per-project activation, not on fixing the login-shell integration.

