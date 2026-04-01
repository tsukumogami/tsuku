<!-- decision:start id="binary-replacement-sequence" status="assumed" -->
### Decision: Binary Replacement Sequence

**Context**

The self-update mechanism needs to replace the running tsuku binary with a newly downloaded version. This is the most safety-critical part of the self-update feature: a failure at any point must never leave the user without a working binary (PRD R7). The replacement must also preserve a `.old` backup until the next successful update.

The binary may live at `$TSUKU_HOME/bin/tsuku` (the common case from the installer) or elsewhere (`/usr/local/bin/tsuku` from `go install`, a Homebrew prefix, etc.). The code must handle symlinks, cross-filesystem temp directories, and permission preservation. PRD D5 constrains the implementation to ~30 lines -- a simple, auditable code path separate from the managed tool system.

On Linux and macOS, `os.Rename` implements POSIX `rename(2)`, which atomically replaces the destination when source and destination are on the same filesystem. A running binary can be safely replaced because the OS keeps the old inode open until the process exits.

**Assumptions**

- tsuku will not be installed on read-only filesystems in normal use. If it is, self-update fails with a clear error. Consequence if wrong: users on read-only root need documentation on relocation.
- The microsecond window between the two renames (current->.old, temp->current) is an accepted risk. Every major self-updating CLI (gh, rustup, goreleaser-distributed binaries) accepts this same window. Consequence if wrong: a concurrent exec during self-update gets ENOENT, but this requires sub-microsecond timing on a user-initiated command.
- The backup `.old` file lives next to the actual binary (wherever it is), not always at `$TSUKU_HOME/bin/tsuku.old`. This refines PRD R7's literal text to handle binaries outside `$TSUKU_HOME/bin/`. Consequence if wrong: users expect `.old` at a fixed location.

**Chosen: Same-Directory Temp with Two-Rename Backup**

The replacement sequence:

1. Resolve the real binary path: `exePath, _ := os.Executable()` then `exePath, _ = filepath.EvalSymlinks(exePath)` to follow symlinks to the actual file.
2. Stat the current binary to capture permissions: `info, _ := os.Stat(exePath)`.
3. Create a temp file in the same directory: `tmp, _ := os.CreateTemp(filepath.Dir(exePath), ".tsuku-update-*")`. Same-directory placement guarantees same-filesystem, which guarantees `os.Rename` atomicity.
4. Download the new binary to the temp file. Verify SHA256 checksum against the release manifest.
5. Set permissions on the temp file: `os.Chmod(tmp.Name(), info.Mode())`.
6. Remove stale backup: `os.Remove(exePath + ".old")` (ignore error -- may not exist).
7. Create backup: `os.Rename(exePath, exePath + ".old")`.
8. Install new binary: `os.Rename(tmp.Name(), exePath)`.
9. If step 8 fails: restore from backup: `os.Rename(exePath + ".old", exePath)`. Clean up temp file.

Edge case handling:

- **Binary outside `$TSUKU_HOME/bin/`**: Works as-is. The `.old` backup and temp file are created next to the real binary. No special casing needed.
- **Symlink target**: `filepath.EvalSymlinks` resolves to the real path. The replacement happens at the real location; the symlink continues to point there.
- **Cross-filesystem temp**: Impossible by construction. `os.CreateTemp` uses `filepath.Dir(exePath)`, which is the same directory (and therefore same filesystem) as the target.
- **Insufficient permissions**: `os.CreateTemp` fails early (step 3), before touching the current binary. Clear error: "cannot write to directory containing tsuku binary".
- **Interrupted between renames (power loss)**: The `.old` backup exists at `exePath + ".old"` and the temp file exists. User can recover manually by renaming `.old` back, or the next tsuku invocation could detect and recover. This is the same accepted risk as gh, rustup, and all similar tools.

Backup lifecycle:

- Created during self-update (step 7).
- Preserved until the next successful self-update (step 6 of the next run removes it).
- If self-update fails, the backup is renamed back to the original location (step 9), so no `.old` persists from a failed attempt.

**Rationale**

This is the standard pattern used by gh (GitHub CLI), rustup, and virtually every self-updating Go binary. It satisfies PRD R7's requirements (atomic replacement, backup preservation) within PRD D5's complexity budget (~20 lines of core logic). The same-directory temp file placement is the critical design choice that eliminates cross-filesystem rename failures.

Alternative 4 (Hardlink+Overwrite) eliminates the microsecond gap between renames but requires a fallback path that is Alternative 1 anyway, pushing the code past the ~30 line target. The zero-gap property doesn't justify the complexity for a user-initiated CLI command.

The codebase already uses the same atomic-rename pattern in `AtomicSymlink` (`internal/install/symlink.go`), making this approach consistent with existing code.

**Alternatives Considered**

- **Staging in $TSUKU_HOME/tmp/**: Downloads to a staging directory, then copies to the binary's directory for the final rename. Rejected because the extra copy step adds I/O and complexity without improving the critical replacement phase. If you have write permission for the rename, you have it for a temp file in the same directory.

- **Direct Overwrite with Copy-Back Backup**: Uses `io.Copy` instead of `os.Rename` for the replacement. Rejected because `io.Copy` is not atomic -- an interrupted copy produces a corrupt binary, directly violating PRD R7's requirement that failure leaves the current binary functional.

- **Hardlink+Atomic Overwrite**: Creates a hard link as backup, then atomically overwrites the original via rename. Eliminates the microsecond gap. Rejected because hard links have filesystem restrictions requiring a fallback path (which is the Two-Rename approach), and the combined code exceeds the ~30 line simplicity target. The microsecond gap is not a real-world concern for CLI self-update.

**Consequences**

- The self-update code path stays simple and auditable (~20 lines of core replacement logic).
- A `.old` backup file appears next to the binary after each successful update, not necessarily at `$TSUKU_HOME/bin/tsuku.old`. Documentation should use the phrasing "next to the tsuku binary" rather than a fixed path.
- Users whose binary is in a directory requiring elevated permissions (e.g., `/usr/local/bin/` owned by root) will get a clear error and need to use `sudo tsuku self-update` or relocate their binary to `$TSUKU_HOME/bin/`.
- The pattern is directly testable: create a temp directory, write a fake binary, run the replacement sequence, verify the result.
<!-- decision:end -->
