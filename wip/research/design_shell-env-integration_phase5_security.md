# Security Review: Shell Env Integration Design

Date: 2026-04-02
Phase: 5 — Security Review

---

## Scope

This review covers the four design changes proposed in the shell-env-integration work:

1. `envFileContent` update — adds shell detection (`$BASH_VERSION`/`$ZSH_VERSION`) and sources `$TSUKU_HOME/env.local`
2. `EnsureEnvFile()` migration — extracts `TSUKU_NO_TELEMETRY=1` from existing `env` into `env.local` before rewriting
3. Installer update — writes telemetry opt-out to `env.local` instead of appending to `env`
4. `tsuku doctor --fix` — calls `EnsureEnvFile()` and `shellenv.RebuildCache()` to repair stale files

Code reviewed: `internal/config/config.go`, `internal/shellenv/cache.go`, `internal/shellenv/doctor.go`, `internal/actions/shell_init.go`, `website/install.sh`.

---

## 1. External Artifact Handling

**Applies:** Partially — the shell.d init cache is built from outputs produced by installed tool binaries.

The `install_shell_init` action's `source_command` path runs a tool binary (e.g., `niwa shell-init bash`) and captures stdout to write to `$TSUKU_HOME/share/shell.d/tool.bash`. That captured output is later concatenated by `RebuildShellCache()` into `.init-cache.bash`, which the updated `env` will source at every shell login.

This design does not add new external inputs beyond what already existed in `install_shell_init`. The changes being reviewed (env file content, `EnsureEnvFile()` migration, installer, doctor) don't download or execute external URLs. The trust boundary for tool binaries is established at install time, not here.

**Mitigations already in place:**
- `validateCommandBinary()` enforces that the `source_command` binary resolves to a path within the tool's install directory before execution (symlink-resolved containment check).
- Shell is restricted to an allowlist (`bash`, `zsh`, `fish`) — arbitrary values are rejected.
- `RebuildShellCache()` rejects symlinks and non-regular files via `Lstat` before reading.
- Hash verification in `RebuildShellCache()` can exclude files whose content has changed since install.

**No new risk introduced by this design.**

---

## 2. Permission Scope

**Applies:** Yes — the changes write to `$TSUKU_HOME`, which is user-space. No privilege escalation is involved.

All files are read/written under `$TSUKU_HOME` (typically `~/.tsuku`):
- `$TSUKU_HOME/env` — managed file, rewritten by `EnsureEnvFile()`
- `$TSUKU_HOME/env.local` — user-owned file, created by the installer or migration
- `$TSUKU_HOME/share/shell.d/` — directory and cache files, restricted to 0700/0600

**Permissions assigned:**
- `shell.d/` directory: 0700 (owner-only read/write/execute)
- `shell.d/*.bash`, `shell.d/*.zsh`, `.init-cache.*`: 0600 (owner-only read/write)
- `env` and `env.local` are written with `os.WriteFile(..., 0644)` (current implementation) — world-readable, which is appropriate since they contain no secrets (PATH manipulation, a `TSUKU_NO_TELEMETRY=1` flag)

**No escalation risk.** These are all standard user-space files. Tsuku doesn't use setuid, doesn't write to system directories, and doesn't call any privileged commands.

**Minor note:** `env.local` is likely created with 0644 (mirroring `env`). Since it may contain `TSUKU_NO_TELEMETRY=1` (not a secret), 0644 is appropriate. If users add other content to `env.local`, they control permissions.

---

## 3. Supply Chain and Dependency Trust

**Applies:** Yes — shell.d scripts come from tool binaries, and this design surfaces their output at shell login for all users.

The shell.d pipeline is: tool binary → stdout → `shell.d/tool.bash` → `.init-cache.bash` → sourced by `$TSUKU_HOME/env` at login.

The proposed change makes sourcing the init cache the **default behavior** for all users whose `env` file is updated (either via `tsuku install` or `tsuku doctor --fix`). Before this change, sourcing the cache was only active for users who set up `eval "$(tsuku shellenv)"`. Expanding the affected population raises the stakes for the pipeline's integrity.

**Trust chain:**
1. Recipes come from the tsuku registry (fetched from GitHub, checksums verified at download time per `install.sh`)
2. Tool binaries are installed by tsuku into `$TSUKU_HOME/tools/` with per-file checksums
3. `install_shell_init` runs the binary at install time and writes stdout to `shell.d/`
4. Content hashes of shell.d files are stored in `state.json` at write time
5. `RebuildShellCache()` verifies stored hashes before including files in the cache

**What this design doesn't change:**
- The hash verification in step 4–5 is opt-in (caller must pass `contentHashes` map). Whether `doctor --fix` calls `RebuildShellCache()` with or without hash verification is not specified in the current design documents. This is the main open question.

**Risk:** If `tsuku doctor --fix` calls `RebuildShellCache()` without hash verification (no `contentHashes` argument), an attacker who can write to `$TSUKU_HOME/share/shell.d/` (e.g., via a path traversal bug in an unrelated tool, or a compromised tool install) could inject arbitrary shell code that loads at every login. Hash verification would catch this; skipping it wouldn't.

**Severity:** Medium. The attacker already needs write access to `$TSUKU_HOME`, which is user-owned. This isn't a privilege escalation — it's persistence within the user's account. But it's worth specifying that `--fix` should pass the stored hashes.

**Mitigation recommendation:** The design should explicitly state that `doctor --fix` calls `RebuildShellCache()` with the content hashes from `state.json`, not without them. The `CheckShellD()` function already receives `contentHashes` — `--fix` should use the same map when calling `RebuildShellCache()`.

---

## 4. Data Exposure

**Applies:** Minimally — the migration reads `$TSUKU_HOME/env` and may write `TSUKU_NO_TELEMETRY=1` to `env.local`. No secrets are involved.

The migration in `EnsureEnvFile()` reads the existing `env` file content, scans for `TSUKU_NO_TELEMETRY` lines, and appends them to `env.local`. No credentials, tokens, API keys, or sensitive personal data are in scope.

`env.local` and `env` are both local files under `$TSUKU_HOME`. They are not transmitted anywhere — the telemetry client reads `TSUKU_NO_TELEMETRY` from the runtime environment via `os.Getenv()`, not from the file directly. The migration is a local file operation.

**No data exposure risk introduced by this design.**

---

## 5. Shell Injection (Domain-Specific)

### 5a. Can a tool write malicious content to `.init-cache.bash`?

**Risk:** Low, with one caveat.

`RebuildShellCache()` wraps each tool's content in `( # begin toolname\n...\n) 2>/dev/null || true`. This subshell boundary provides error isolation (a syntax error in one tool's script won't break others), but it does **not** prevent execution of valid shell commands. If `shell.d/tool.bash` contains `rm -rf ~/important-dir`, the subshell wrapper will execute it successfully.

This is not a new risk introduced by this design — the same was true before. What changes is that the cache is now sourced automatically for all users (via the updated `env`) rather than only for users who opted in with `eval "$(tsuku shellenv)"`. The broader reach increases the impact surface.

The existing mitigations (symlink rejection, hash verification, `validateCommandBinary()` containment check, shell allowlist) collectively make it hard for a recipe to smuggle malicious content. The most realistic vector is a compromised tool binary that generates harmful output when its `shell-init` command is run. The containment check in `validateCommandBinary()` ensures the binary is within the tool's install directory, and recipe changes to the registry go through CI.

**Verdict:** The isolation wrapper is error isolation, not security isolation. The design's security relies on the integrity of installed binaries and the hash verification chain. That chain is sound if `--fix` uses stored hashes (see section 3).

### 5b. Can a user's `env.local` contain harmful content?

**Risk:** No meaningful new risk.

`env.local` is a user-owned file, sourced at shell login via the updated `env`. Users can already write arbitrary content to their `.bashrc`/`.zshenv`. The `env.local` mechanism gives users a documented place to add customizations — it doesn't expand what they can do, it just gives them a named file.

The security model here is: `env.local` is in `$TSUKU_HOME`, which is user-owned. A user can harm themselves, and a system-level attacker who can write to `$TSUKU_HOME` already has full control of the user's shell environment via other paths. There's no privilege escalation vector from `env.local`.

**One scenario worth noting:** If a user accidentally sets `TSUKU_HOME` to a world-writable directory (e.g., `/tmp/tsuku`), then `env.local` could be written by other users. This is not specific to this design — the same risk applies to the existing `env` file. The design inherits the security properties of `$TSUKU_HOME` as a whole.

### 5c. Is the `$BASH_VERSION`/`$ZSH_VERSION` detection spoofable in a harmful way?

**Risk:** Very low.

A user could theoretically set `BASH_VERSION=1` in a non-bash shell (e.g., dash) to make the env file attempt to source `.init-cache.bash`. This would cause the wrong cache file to be sourced — but in practice:
- The cache file is sourced with `.`, which interprets it in the current shell. If the content contains bash-specific syntax, the current shell (dash) would encounter a syntax error. The `2>/dev/null || true` wrapper inside the cache file partially mitigates this.
- This is a self-harm scenario — the user controls their own environment variables.
- There is no privilege escalation: sourcing a bash script in dash produces errors, not elevated permissions.

The design's decision document (Decision 1) notes this as a known non-issue: "A user who deliberately sets `$BASH_VERSION` or `$ZSH_VERSION` as regular environment variables could mislead detection — extremely unlikely and not a security concern here." This assessment is correct.

**No meaningful risk.**

### 5d. Does `env.local` sourcing at the end of `env` create privilege escalation risks?

**Risk:** No.

Sourcing `env.local` at the end of `env` does not create any new privilege escalation path:
- `env` is already in user space (`$TSUKU_HOME`), not `/etc/profile.d/` or any root-owned directory
- `env.local` is also in user space
- The shell sourcing these files (bash/zsh started by the user) is already running with user-level permissions

The pattern `[ -f "$TSUKU_HOME/env.local" ] && . "$TSUKU_HOME/env.local"` is safe when `$TSUKU_HOME` is not writable by other users. This is the same condition the existing `env` file relies on.

If `$TSUKU_HOME` is set to a system path (e.g., `/etc/tsuku` via environment variable manipulation), an attacker with write access to that path could inject into the user's shell. But an attacker who can write to `/etc/tsuku` already has system-level access, making this moot.

---

## 6. Migration Safety

### TOCTOU (Time-of-Check/Time-of-Use)

**Risk:** Low, practically mitigated.

`EnsureEnvFile()` follows a read-compare-write pattern without atomic file locking. Between reading `env` and writing `env.local` + overwriting `env`, another process could modify the file. In practice:
- Concurrent `tsuku install` calls are unlikely; shell config files are not hot paths
- The consequence of a race is a lost update to `env.local` (the migration might run twice, or the TSUKU_NO_TELEMETRY line might be appended twice to `env.local`) — not a security concern, just a correctness concern
- `RebuildShellCache()` already uses a file lock for the cache rebuild; the same lock pattern could be applied to `EnsureEnvFile()` if needed, but this is an engineering decision, not a security requirement

**Verdict:** Not a security issue. The edge case is benign duplication in `env.local` (writing `TSUKU_NO_TELEMETRY=1` twice), which is functionally harmless.

### Path Traversal

**Risk:** None.

All paths are derived from `$TSUKU_HOME` via `filepath.Join()`:
- `env`: `filepath.Join(c.HomeDir, "env")`
- `env.local`: `filepath.Join(c.HomeDir, "env.local")`

The migration extracts `TSUKU_NO_TELEMETRY` lines from the file's text content — it doesn't interpret paths from the file content. There's no user-supplied path component in either the read or write operations.

If `$TSUKU_HOME` itself is set to a path with `..` components, the config package uses it directly without sanitization — but this is a pre-existing property of the config package, not introduced by this design.

### Write-Permission Risks

**Risk:** None beyond existing.

The migration writes `env.local` with the same permissions as `env`. No setuid binary is involved. No system directories are written to. The write path is entirely within user space.

One correctness point: if `env.local` already exists when migration runs (e.g., a user created it manually before upgrading), the migration should append rather than overwrite, to avoid clobbering existing content. The architecture review (phase 6) already flagged this — it's an implementation detail, not a security concern.

---

## Summary of Findings

| Area | Risk | Severity | Status |
|------|------|----------|--------|
| External artifact handling | No new risk | — | Existing mitigations sufficient |
| Permission scope | No escalation | — | Correct 0700/0600 for shell.d |
| Supply chain trust | Hash verification scope for `--fix` | Medium | Needs explicit spec |
| Data exposure | No sensitive data involved | — | N/A |
| Shell injection via shell.d | Isolation is error-only, not security | Low | Pre-existing, broader audience |
| env.local content | User self-harm only | Low | No escalation possible |
| $BASH_VERSION spoofing | Self-harm, no escalation | Very low | Acceptable |
| env.local privilege escalation | No escalation path | — | N/A |
| TOCTOU | Benign duplication | Very low | Not a security issue |
| Path traversal | No user-supplied paths | — | N/A |
| Write permissions | User space only | — | N/A |

**One finding needs design attention:** The design doesn't specify whether `doctor --fix` calls `RebuildShellCache()` with hash verification (content hashes from `state.json`) or without. This should be made explicit. Using stored hashes is the correct choice and preserves the integrity chain for the broader audience now affected by auto-sourcing the init cache.

---

## Recommended Outcome

**OPTION 2 — Document considerations**

The design is sound. The one finding (hash verification scope for `--fix`) is a clarification that belongs in the Security Considerations section of the design doc rather than a blocking design change.
