# Lead: Installer script and self-update shell setup

## Findings

### 1. Installer Script Location and Behavior
**File:** `/website/install.sh` (synced from the public tsuku repo root; served as `https://get.tsuku.dev/now`)

The installer creates `~/.tsuku/env` (lines 120-133) with the following content:
```bash
# tsuku shell configuration
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
```

The installer then sources this file in shell config files:
- **bash:** Adds `. "$ENV_FILE"` to `~/.bashrc` and `~/.bash_profile` (lines 159-178)
- **zsh:** Adds `. "$ENV_FILE"` to `~/.zshenv` (line 183)

**Key finding:** The installer writes a simple sourcing line pointing to `~/.tsuku/env`, which is **static PATH-only content** (lines 119-124 match lines 444-447 of `internal/config/config.go`).

### 2. The Static ~/.tsuku/env Content
**File:** `internal/config/config.go`, lines 442-447

The canonical env file content is defined as:
```go
const envFileContent = `# tsuku shell configuration
# Add tsuku directories to PATH
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
`
```

This file is **never updated after the initial installation**. It only exports PATH and does **NOT** source the init cache.

### 3. When ~/.tsuku/env is Created
**File:** `internal/install/manager.go`, lines 67-72

The `EnsureEnvFile()` method is called during the first tool installation (via `InstallWithOptions`), not during tsuku's own initialization. It's a non-fatal operation with a warning if it fails.

### 4. What the Init Cache Needs but Doesn't Get
**File:** `internal/shellenv/cache.go`, lines 13-23 and `cmd/tsuku/shellenv.go`, lines 42-48

The shell init cache (`.init-cache.bash`, `.init-cache.zsh`, etc.) is built by `RebuildShellCache()` when `tsuku install` runs a tool with `install_shell_init`. The cache is located at `$TSUKU_HOME/share/shell.d/.init-cache.{shell}`.

The `tsuku shellenv` command (lines 42-48) sources this cache:
```go
cachePath := filepath.Join(homeDir, "share", "shell.d", ".init-cache."+shell)
if _, err := os.Stat(cachePath); err == nil {
    fmt.Fprintf(os.Stdout, ". \"%s\"\n", cachePath)
}
```

**CRITICAL GAP:** The installer writes `~/.tsuku/env` which only sets PATH. It does **NOT** source the init cache. Therefore:
- **New installations**: The init cache is never sourced because `~/.tsuku/env` doesn't have the line to source it
- **Post-update**: Self-update only replaces the binary (see below) and never modifies shell configs

### 5. The Self-Update Mechanism
**Files:** `cmd/tsuku/cmd_self_update.go` and `internal/updates/self.go`

**What self-update does:**
1. Downloads the latest tsuku binary (lines 82-86 of `cmd_self_update.go`)
2. Verifies checksum (lines 172-237 of `self.go`)
3. Replaces the running binary atomically with backup (lines 250-261 of `self.go`)
4. **Does NOT touch shell configurations, env files, or init caches**

**Background updates via `CheckAndApplySelf()`** (`self.go`, lines 83-167):
- Checks for new tsuku versions
- Writes cache entry regardless of update outcome (line 114)
- If a newer version exists and self-update is enabled, applies it
- Writes a success notice to `$TSUKU_HOME/notices/` (lines 157-164)
- **Still does NOT modify any shell setup**

**Tool updates via auto-apply** (`updates/apply.go`, lines 60-210):
- Installs pending tool updates
- Garbage collects old versions (line 160)
- **Does NOT rebuild the init cache** (which happens only when `tsuku install` is called)

### 6. Hook Installation (Separate from Env Setup)
**File:** `internal/hook/install.go`

The hook installation (command-not-found handler) is independent of the env file:
- Hooks are written to `$TSUKU_HOME/share/hooks/` (line 121)
- The hook marker block (`. "$TSUKU_HOME/share/hooks/tsuku.bash"`) is appended to rc files separately (lines 138-162)
- This is idempotent (checked at line 150: `if strings.Contains(string(existing), markerComment) { return nil }`)

**The installer calls `tsuku hook install` at the end** (`website/install.sh`, line 235), which registers this hook. It's separate from the env file sourcing.

### 7. Shell Init Cache Rebuild
**File:** `internal/actions/shell_init.go` and `internal/shellenv/cache.go`

When a tool with `install_shell_init` is installed:
1. Files are written to `$TSUKU_HOME/share/shell.d/{tool}.{shell}` (line 159 of `shell_init.go`)
2. Content hashes are recorded for cleanup (line 174)
3. `tsuku shellenv` would source the cache if invoked manually
4. **But nothing in the normal init sequence calls `tsuku shellenv`** — it must be manually added to shell config

## Implications

### New Users Installed via Installer
**Status: Broken for tools with shell init**

New users who run the installer script will:
1. Get `~/.tsuku/env` with only PATH exports ✓
2. Get `~/.bashrc` or `~/.zshrc` sourcing `~/.tsuku/env` ✓
3. Have the hook registered ✓
4. **NOT have the init cache sourced in their shell config** ✗

If they install a tool with shell functions (e.g., `niwa` with `install_shell_init`), those functions won't be available because `~/.tsuku/env` doesn't source `.init-cache.bash`.

### Self-Update Path
**Status: Broken (but inherited from installation)**

When tsuku self-updates:
1. The binary is replaced ✓
2. Shell configurations are **NOT updated** ✗
3. If they were broken at installation time, they stay broken
4. No mechanism in self-update to fix the init cache sourcing gap

### Existing Users (Pre-Shell.d Installs)
**Status: Depends on when they installed**

- Users who installed before `shell.d` was added: No init cache exists; tools without shell integration work fine
- Users who installed after `shell.d` was added but before this issue was discovered: Same situation as new users (broken)

## Surprises

1. **~/.tsuku/env is not regenerated on tool install** — `EnsureEnvFile()` is called but only makes the file idempotent (doesn't update existing content)
2. **The installer and the CLI have different concepts of initialization**:
   - Installer: Sets PATH via `~/.tsuku/env`
   - CLI: `tsuku install` with `install_shell_init` assumes sourcing the init cache
   - These two paths never meet
3. **Self-update is completely separate from shell setup** — no gap-filling happens during binary updates
4. **The init cache is built but not wired into the standard shell initialization** — it exists in `share/shell.d/.init-cache.*` but only `tsuku shellenv` knows to source it

## Open Questions

1. **Should `~/.tsuku/env` be updated to source the init cache?** This would be the simplest fix: modify `website/install.sh` to write an additional line, and ensure the CLI's `EnsureEnvFile()` also includes it.

2. **When should the init cache be rebuilt?** It's rebuilt on each `tsuku install`, but what if a tool is installed via `tsuku install` with `--no-shell-init`, then later installed again normally? The cache would be built then.

3. **Should self-update trigger a shell reconfig?** Or should the fix be at installation time to prevent the gap from existing?

4. **Is there a post-install setup step users should run?** Something like `tsuku setup` that rebuilds the init cache and sources it?

5. **What about fish shell support?** The init action supports bash/zsh/fish, but the installer script only handles bash/zsh and fish separately at lines 221-224. Need to verify fish shell.d sourcing.

## Summary

The installer script correctly writes `~/.tsuku/env` with PATH exports and sources it in shell configs, but `~/.tsuku/env` does not source the init cache file (`~/.tsuku/share/shell.d/.init-cache.{shell}`). This means tools with `install_shell_init` (like niwa) will have their shell functions written to disk but not available in new shell sessions. The self-update mechanism only replaces the binary and does not address this gap, so users who installed before this gap was identified or new users will have the same broken state. The fix requires either (1) updating `~/.tsuku/env` to unconditionally source the init cache, or (2) changing the installer to add an explicit sourcing line for the cache to shell configs, or (3) adding a post-install/post-update setup command.

