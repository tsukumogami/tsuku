# Lead: tsuku doctor and shell setup detection

## Findings

### What tsuku doctor Checks

`cmd/tsuku/doctor.go` (lines 17-230) implements a comprehensive health check with seven checks:

1. **Home directory exists** (lines 42-55): Verifies `$TSUKU_HOME` directory exists and is a directory
2. **tools/current in PATH** (lines 57-76): Checks if `$TSUKU_HOME/tools/current` is in `$PATH`; suggests `eval $(tsuku shellenv)` if missing
3. **bin directory in PATH** (lines 78-96): Checks if `$TSUKU_HOME/bin` is in `$PATH`; suggests `eval $(tsuku shellenv)` if missing
4. **State file accessible** (lines 98-111): Verifies `state.json` exists or gracefully handles no tools installed
5. **Shell.d health** (lines 113-172): Detailed check via `shellenv.CheckShellD()` that verifies:
   - Shell integration scripts exist for installed tools
   - Init cache files are fresh (not stale)
   - Content hashes match (integrity verification)
   - Symlinks are not present (security check)
   - Shell syntax is valid (bash/zsh `-n` check)
   - Reports `tsuku doctor --rebuild-cache` if cache is stale
6. **Orphaned staging directories** (lines 174-192): Warns about leftover `.staging-*` directories; suggests manual removal
7. **Stale notices** (lines 194-218): Warns about auto-update failure notices older than 30 days; suggests cleanup

### Shell Integration Check Detail

The shell health check (`internal/shellenv/doctor.go`, `CheckShellD()` lines 52-142) returns `ShellDCheckResult` containing:

- `ActiveScripts`: Tools that have shell.d files, keyed by shell name (bash/zsh)
- `CacheStale`: Map of shell to whether cache doesn't match current shell.d concatenation
- `HashMismatches`: Files where on-disk content doesn't match stored SHA-256 hash
- `Symlinks`: Unsafe symlinks detected in shell.d
- `SyntaxErrors`: Files failing shell syntax validation

Cache staleness check (`doctor.go` lines 146-174) reconstructs the expected cache by:
1. Reading all matching `.{shell}` files sorted alphabetically
2. Wrapping each with isolation comments: `# tsuku: {toolname}` and error-isolation subshells
3. Comparing against actual `.init-cache.{shell}` file

### Critical Gap: No Check for Init Cache Being Sourced

**The doctor does NOT verify that the init cache is being sourced in user shells.** This is the core problem:

- `internal/config/config.go` (lines 249-262) shows `envFileContent` only sets PATH:
  ```
  # tsuku shell configuration
  # Add tsuku directories to PATH
  export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
  ```

- `cmd/tsuku/shellenv.go` (lines 42-48) shows the correct pattern — it DOES source the init cache:
  ```go
  // Source the shell init cache if it exists.
  shell := detectShellForEnv()
  cachePath := filepath.Join(homeDir, "share", "shell.d", ".init-cache."+shell)
  if _, err := os.Stat(cachePath); err == nil {
    fmt.Fprintf(os.Stdout, ". \"%s\"\n", cachePath)
  }
  ```

This means users who source `~/.tsuku/env` (via installer script) get PATH but NOT shell functions, while `eval $(tsuku shellenv)` gets both.

### The --rebuild-cache Flag

Line 160 of doctor.go references `tsuku doctor --rebuild-cache` when cache is stale, **but the flag is never defined**. A grep search finds only the error message, not the flag implementation. This suggests the feature was documented/promised but not implemented yet.

### The Notices System

`internal/notices/notices.go` (lines 1-152) provides infrastructure for recording failed tool updates, but it is **NOT used for shell integration warnings**. Notices are only written by `internal/updates/apply.go` when auto-updates fail.

### Shell Integration Workflow

When `install_shell_init` action runs (lines 82-132 in `internal/actions/shell_init.go`):

1. Writes tool shell functions to `$TSUKU_HOME/share/shell.d/{target}.{shell}`
2. Records SHA-256 content hash in `CleanupAction` (for integrity verification)
3. Doctor command can verify these hashes were not tampered with
4. **But nothing automatically updates the env file** or alerts the user to source the init cache

### State Manager Integration

Doctor reads `state.json` (lines 118-135) to extract content hashes for hash verification, showing awareness of the install history.

## Implications

1. **Doctor is a diagnostic tool, not a repair tool**: It can detect problems (stale cache, hash mismatches, syntax errors) but cannot fix them. The `--rebuild-cache` suggestion exists but is not implemented.

2. **The env file is outdated for the current tooling model**: It was designed when tools only needed PATH. Now that `install_shell_init` exists, the env file should also source the init cache, but it doesn't.

3. **Doctor could be extended to detect the shell setup problem**: It could check whether `~/.tsuku/env` contains the init cache sourcing line, or whether the user is actually sourcing it via `.bashrc`.

4. **Repair strategy would need two parts**:
   - Auto-update the env file to source the init cache (like `shellenv.go` does)
   - Or recommend users switch from sourcing `~/.tsuku/env` to `eval $(tsuku shellenv)`

5. **Existing users are silently broken**: Anyone with an old env file has shell functions that won't load in new terminals, and doctor doesn't warn about this.

## Surprises

1. The `--rebuild-cache` flag is referenced in error output but not implemented — this is a UX gap.

2. The `notices` system exists and is sophisticated (consecutive failure tracking, actionable error bypass), but it's only used for update failures, not shell integration problems.

3. Doctor checks content hash integrity but the env file itself is never checked or updated.

4. The `shellenv` command does the right thing (sources init cache), but it's not the default path — users are encouraged to use the env file instead.

## Open Questions

1. Should `--rebuild-cache` be implemented as a doctor flag? If so, who decides which shell(s) to rebuild?

2. Should the env file be updated to source the init cache, or should users be migrated to `eval $(tsuku shellenv)`?

3. Should doctor have an "auto-repair" mode that fixes the env file and rebuilds caches?

4. How do we detect that a user has an old env file? Check the file content against the expected `envFileContent`?

5. Could the notices system be extended to warn about missing init cache sourcing on first install of a tool with `install_shell_init`?

## Summary

Doctor is a comprehensive diagnostic tool that detects shell.d health problems (stale cache, hash mismatches, syntax errors) but does not detect the fundamental issue: the `~/.tsuku/env` file is missing the init cache sourcing line that was added to `shellenv.go`. The `--rebuild-cache` flag referenced in error messages is not implemented. Doctor could be extended with an auto-repair mode and a check for whether the init cache is actually being sourced in user shells, making it the ideal place to fix shell integration for users with old env files.

