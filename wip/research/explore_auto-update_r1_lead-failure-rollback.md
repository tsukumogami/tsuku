# Lead: What are the failure modes of auto-update and how do tools handle rollback?

## Findings

### 1. Tsuku's Current Installation Architecture (Rollback-Relevant)

Tsuku already has several properties that make rollback natural:

**Multi-version coexistence.** Tools are installed to `$TSUKU_HOME/tools/<name>-<version>/` directories. Multiple versions can exist simultaneously, tracked in `state.json` under `versions` map per tool. The `active_version` field records which version symlinks point to. This is defined in `internal/install/state.go` (lines 76-103).

**Atomic installation via staging.** `InstallWithOptions()` in `internal/install/manager.go` (lines 61-201) copies files to a `.staging` directory first, then does `os.Rename()` to the final path. This means a tool directory is either fully present or absent -- never half-installed. The staging dir lives in the same parent (`$TSUKU_HOME/tools/.name-version.staging`) to guarantee same-filesystem rename atomicity.

**Atomic symlink switching.** `AtomicSymlink()` in `internal/install/symlink.go` (lines 14-41) creates a `.tmp` symlink then renames it over the target. There is never a moment where the symlink is absent. Wrapper scripts use the same temp-then-rename pattern (`manager.go` lines 417-429).

**State file atomic writes.** `saveWithLock()` in `internal/install/state.go` (lines 219-247) writes to `state.json.tmp` then renames. State is never partially written.

**Existing rollback on symlink failure.** If symlink creation fails after tool directory installation, the code already removes the tool directory (`manager.go` lines 145-151). This is a limited form of rollback.

**Checksum verification.** Binary checksums are computed at install time and stored in `VersionState.BinaryChecksums` (`state.go` line 19). `VerifyBinaryChecksums()` in `internal/install/checksum.go` can detect post-install corruption. The `tsuku verify` command exposes this.

### 2. Failure Modes Taxonomy

#### 2a. Network Failures

- **During download:** Connection drops, DNS failures, timeouts, TLS errors. The download action writes to a temp file in the work directory. If it fails, the work directory gets cleaned up by `Executor.Cleanup()`. No state is modified because installation hasn't reached the commit phase.
- **During version resolution:** The version provider API call fails. This happens before any download, so there's nothing to roll back. The check just fails.
- **During checksum/signature fetch:** `checksum_url` or `signature_url` download fails. Same as download failure -- work directory cleanup handles it.
- **Partial download:** HTTP connection drops mid-stream. The file on disk is incomplete. Without checksum verification, this could produce a corrupt binary. With checksum verification (which tsuku enforces via `checksum_url` or signatures), the mismatch is caught before installation.

#### 2b. Corrupt or Invalid Binaries

- **Checksum mismatch after download:** Caught by the download action's verification step. Installation aborts before reaching the staging phase.
- **Binary works on download but fails at runtime:** Architecture mismatch, missing system library, etc. Not caught during installation. Can only be detected by post-install verification (`tsuku verify`) or when the user runs the tool.
- **Extraction failure:** Corrupt archive, unsupported format, truncated tarball. Caught during the extract action. Work directory cleanup handles it.

#### 2c. Disk and Filesystem Failures

- **Disk full during staging copy:** `copyDir()` fails, staging directory is cleaned up (`manager.go` line 93). No state change.
- **Disk full during `os.Rename()`:** Rename is metadata-only on same filesystem, so this is unlikely. But if the filesystem is truly full (no inode space), rename fails and staging is cleaned up.
- **Disk full during state.json write:** The temp file write fails, so the rename never happens. Old state.json remains intact. However, the tool directory may already be in place without a state entry -- this is a gap.
- **Permission errors:** Writing to `$TSUKU_HOME` fails. Caught early in `EnsureDirectories()`. Not a rollback scenario.

#### 2d. Recipe Format Incompatibility

- **New recipe version requires newer tsuku:** A recipe uses actions not supported by the installed tsuku version. The executor fails on the unknown action. No state change since execution never completes.
- **Recipe schema change between versions:** The TOML parser rejects the recipe or misinterprets fields. Fails at parse time, before any side effects.
- **Recipe template variables change:** A variable like `{version}` in a URL template resolves differently. This would produce a download error or checksum mismatch. Caught by existing verification.

#### 2e. State Corruption

- **state.json corruption:** If the file is corrupted (e.g., power loss during a non-atomic write on a non-journaling FS), `json.Unmarshal` fails. The state manager returns an error. Currently there's no recovery from this -- the user would need to rebuild state or manually fix it.
- **Stale staging directories:** A previous failed installation may leave `.staging` directories. The code already cleans these up at the start of `InstallWithOptions()` (`manager.go` line 78).

#### 2f. Concurrent Access

- **Two tsuku processes updating the same tool:** File locking via `FileLock` on `state.json.lock` prevents concurrent state writes. However, the tool directory rename is not covered by this lock -- two processes could race to rename staging to final.

### 3. Rollback Strategies Survey

#### 3a. Nix: Immutable Store + Generation Symlinks

Nix never modifies installed packages. Each profile change creates a new "generation" (a numbered symlink). Rollback is switching a symlink to a previous generation number. The old store paths remain until garbage collected.

**Relevance to tsuku:** Tsuku's multi-version model is structurally similar. Old versions in `$TSUKU_HOME/tools/<name>-<old-version>/` persist after an update. The `active_version` in state.json is analogous to Nix's generation pointer. Rollback means: (1) switch `active_version` back, (2) re-point symlinks. Tsuku's `Activate()` method already does this.

**Trade-off:** Requires keeping old versions around. Disk usage grows. Needs a garbage collection mechanism to prune old versions.

#### 3b. Rustup/self-replace: Atomic Binary Swap

For self-updating binaries, the `self-replace` crate (by mitsuhiko/Armin Ronacher) implements: on Unix, rename new binary over old (atomic). On Windows, rename old binary aside then place new binary (non-atomic but recoverable).

**Relevance to tsuku:** Tsuku's self-update would need this pattern. The Go equivalent is straightforward on Unix (`os.Rename`). The running process can rename its own executable on Unix. Windows needs special handling (rename-aside, then copy new, then clean up old on next run).

**Trade-off:** Simple and proven. No rollback possible after the rename unless you keep the old binary somewhere. Need to verify the new binary works before committing.

#### 3c. Homebrew: No Rollback

Homebrew upgrades in place. There's no built-in rollback. Users must manually find old formula commits. The `brew pin` command can prevent upgrades but doesn't help after the fact.

**Relevance to tsuku:** This is the anti-pattern to avoid.

#### 3d. Docker/Container Images: Tag-Based Rollback

Container registries keep tagged images. Rollback means pulling a previous tag. The old image layers remain cached locally.

**Relevance to tsuku:** Analogous to tsuku's version-based directories. The key insight is that rollback requires the old artifact to still exist on disk.

### 4. Recommended Rollback Strategy for Tsuku

Given tsuku's existing architecture, the natural strategy is **keep-old-version**:

1. **Before update:** Record current `active_version` for the tool.
2. **Install new version:** Use existing staging + rename flow. The new version goes into a new directory (`<name>-<new-version>/`). Old directory (`<name>-<old-version>/`) is untouched.
3. **Switch symlinks:** Use `Activate()` to point to new version.
4. **Verify (optional):** Run post-install verification if the recipe defines it.
5. **On failure at any step:** Don't switch symlinks (steps 1-2 failures), or switch back to old version (step 3-4 failures). The old directory is still there.

This works because tsuku already installs to versioned directories and supports multi-version. The only new requirement is: **don't remove the old version during auto-update until success is confirmed.**

For **self-update of the tsuku binary itself**, the strategy is different:
1. Download new tsuku binary to a temp location.
2. Verify checksum/signature.
3. Rename old binary to `tsuku.old` (backup).
4. Rename new binary into place (atomic on Unix).
5. On next run, if `tsuku.old` exists and current binary is healthy, delete `tsuku.old`.
6. If the new binary is broken, the user can manually `mv tsuku.old tsuku` to recover. Or a "recovery mode" in the wrapper script can do it.

### 5. Deferred Error Reporting Patterns

The requirement is: if an auto-update fails, report the failure when the tool is next executed (or when tsuku is next invoked).

#### 5a. File-Based Error Queue

Write a file to `$TSUKU_HOME/notices/` (or similar) containing the error details. On next tsuku invocation, check for pending notices, display them, and delete the files.

**Implementation sketch:**
```
$TSUKU_HOME/notices/
  update-failed-kubectl-2024-01-15.json
  {
    "tool": "kubectl",
    "from_version": "1.28.0",
    "to_version": "1.29.0",
    "error": "checksum mismatch",
    "timestamp": "2024-01-15T10:30:00Z",
    "displayed": false
  }
```

**When to display:**
- On any `tsuku` command invocation (like Homebrew's "auto-update ran X hours ago" messages).
- On `tsuku install/update/list` specifically.
- Via the shell hook (`tsuku hook-env`) which already runs on every prompt -- but this must be very fast, so it should only check for file existence, not parse JSON.

**Advantages:** Survives process restarts. Can accumulate multiple failures. Simple to implement.

**Disadvantages:** Adds filesystem I/O on every invocation. Need to handle cleanup of stale notices.

#### 5b. State File Field

Add a `pending_errors` or `update_failures` field to `state.json`. Checked on each tsuku invocation.

**Advantages:** No new files. Already atomically written.

**Disadvantages:** Couples error reporting to state management. State file reads are already on the hot path but adding writes for error clearing adds overhead.

#### 5c. Wrapper Script Approach (for managed tools)

If a tool is invoked via a wrapper script in `$TSUKU_HOME/bin/`, the wrapper could check for a failure marker file before exec'ing the real binary. This would report the failure literally "when the tool is next executed" -- not just on next tsuku invocation.

**Example wrapper augmentation:**
```sh
#!/bin/sh
_fail="$TSUKU_HOME/notices/update-failed-kubectl.txt"
if [ -f "$_fail" ]; then
  cat "$_fail" >&2
  rm "$_fail"
fi
exec "/path/to/kubectl" "$@"
```

**Advantages:** Reports at the exact moment the user runs the affected tool. Very intuitive.

**Disadvantages:** Only works for tools using wrapper scripts (not plain symlinks). Adding wrapper overhead to every tool invocation. The check is fast (stat + conditional read) but non-zero.

#### 5d. Hook-Based Approach

Tsuku already has `tsuku hook-env` running on every prompt via shell integration (`internal/hooks/tsuku-activate.bash`). This hook could check for pending notices.

**Advantages:** Reports on the next shell prompt after failure. Natural integration point.

**Disadvantages:** Must be extremely fast -- any latency here is felt on every keypress. Should do stat-only check, not JSON parsing. Only works if shell integration is active.

#### 5e. Hybrid Approach (Recommended)

Use file-based error queue (`$TSUKU_HOME/notices/`) with two display mechanisms:
1. **On next tsuku command:** Check for notices at startup, display to stderr, mark as displayed.
2. **Via hook-env (optional):** If shell integration is active, do a fast `stat()` check for the notices directory having entries. If so, emit a one-line hint: "tsuku: update failures pending, run 'tsuku notices' to see details."

This gives immediate awareness without slowing down the prompt hook.

### 6. Gap: State-Directory Consistency

One gap in the current architecture: if installation succeeds (tool directory created) but `state.json` update fails, the tool directory exists without a state entry. This is a minor form of "leak" that `tsuku doctor` could detect and clean up. The reverse (state entry without directory) is already handled -- `Activate()` checks for directory existence.

For auto-update, this gap matters more because updates happen unattended. A state-directory reconciliation check should run periodically or be part of `tsuku doctor`.

## Implications

1. **Tsuku's multi-version architecture is already rollback-friendly.** The keep-old-version strategy requires almost no new installation machinery -- just discipline about not removing the old version directory until the new one is confirmed working. The `Activate()` method is the rollback mechanism.

2. **Self-update needs a different strategy** because the tsuku binary isn't in a versioned directory. The rename-aside-then-swap pattern from `self-replace` is the standard approach.

3. **Deferred error reporting needs a new subsystem** (file-based notices directory) since tsuku has no existing mechanism for cross-invocation messaging. The shell hook integration provides an optional fast-path for awareness.

4. **The biggest risk in auto-update isn't the update itself -- it's detection of "update succeeded but tool is broken."** Checksum verification catches download corruption, but doesn't catch runtime incompatibilities (missing system library, changed CLI flags, etc.). Post-update health checks (if defined in recipes) would close this gap.

5. **Concurrent auto-updates need coordination.** If tsuku auto-update runs in the background while the user runs `tsuku update` manually, the file lock on state.json prevents state corruption, but tool directory races are possible. A per-tool lock file would solve this.

## Surprises

1. **Tsuku's existing atomicity is strong.** The staging-then-rename pattern for installation, atomic symlinks, and atomic state writes mean that most failure modes leave the system in a clean state. The main gap is the window between tool-directory creation and state-file update, which is small but exists.

2. **The wrapper script approach for deferred errors is appealing but only works for tools that use wrappers** (those with runtime dependencies). Tools installed with plain symlinks would need a different mechanism. This creates an inconsistent UX unless all tools switch to wrappers (which adds overhead) or the file-based notice approach is used universally.

3. **Nix's generation model maps cleanly to tsuku's multi-version model.** Tsuku already has the equivalent of Nix generations (version directories) and profile switching (Activate). The main missing piece is garbage collection of old versions after successful updates.

## Open Questions

1. **Should auto-update remove old versions automatically, or require explicit cleanup?** Keeping old versions enables rollback but consumes disk. A policy like "keep the previous version for 7 days after successful update" balances both, but adds complexity.

2. **How should self-update verification work?** After replacing the tsuku binary, how do we confirm the new binary works before declaring success? Running `tsuku --version` in a subprocess? What if the new binary has a startup crash?

3. **Should deferred errors be displayed once or persist until acknowledged?** Showing once is less annoying but risks the user missing the message. Persisting until acknowledged (`tsuku notices --ack`) is safer but noisier.

4. **What about tools where the update is a reinstall (same version directory)?** Some updates might not change the version number (e.g., recipe fix for same version). In this case, the old directory would be overwritten. The staging pattern handles this atomically, but rollback requires a backup copy of the old directory, not just keeping it.

5. **Should there be a per-tool lock to prevent concurrent updates?** The current file lock only covers state.json, not the tool directory rename. Two concurrent updates of the same tool could race.

## Summary

Tsuku's existing multi-version installation with staging-then-rename and atomic symlinks already provides the foundation for safe rollback -- the primary strategy is to keep the old version directory intact and use `Activate()` to switch back if the update fails. The main new subsystem needed is a file-based deferred error queue (`$TSUKU_HOME/notices/`) with display hooks on tsuku invocation and optionally via shell integration. The biggest open question is how to detect "update succeeded but tool is broken at runtime" since checksum verification only catches download-time corruption, not runtime incompatibilities.
