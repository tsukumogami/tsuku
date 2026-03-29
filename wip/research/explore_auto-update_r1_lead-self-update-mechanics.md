# Lead: How do other CLI tools and package managers handle self-update?

## Findings

### 1. The Unix rename-in-place pattern (dominant approach)

The most common self-update strategy for CLI tools on Unix/Linux is **download-alongside-and-rename**. It works because Unix filesystems decouple file names (directory entries) from file data (inodes). A running process holds a file descriptor to the inode, not the directory entry. You can `os.Rename()` the old binary away, place the new one at the same path, and the running process continues executing from the old inode until it exits.

The canonical sequence:
1. Download new binary to a temp file in the same directory (same filesystem required for atomic rename)
2. Rename current binary to `.target.old`
3. Rename new binary to the original path
4. Delete `.target.old`
5. If step 3 fails, roll back by renaming `.target.old` back

This is the approach used by:
- **minio/selfupdate** (Go library): `CommitBinary()` renames `/path/to/target` to `/path/to/.target.old`, then `/path/to/.target.new` to `/path/to/target`. Rollback is attempted if the final rename fails. Source: [minio/selfupdate apply.go](https://github.com/minio/selfupdate/blob/master/apply.go)
- **creativeprojects/go-selfupdate** (Go library): Similar pattern, downloads from GitHub/GitLab/Gitea releases and replaces the running binary. Source: [go-selfupdate](https://pkg.go.dev/github.com/creativeprojects/go-selfupdate)
- **rhysd/go-github-selfupdate** (Go library): Detects latest GitHub release, downloads, replaces. Source: [go-github-selfupdate](https://github.com/rhysd/go-github-selfupdate)

### 2. Rustup: the --self-replace spawn pattern

Rustup uses a more elaborate approach because it also needs to replace proxy binaries (cargo, rustc, etc.) and handle Windows constraints.

**Unix path:** `run_update()` spawns a child process with `--self-replace` that performs the actual binary swap. The parent process checks the child's exit status. On Unix the rename-in-place pattern works fine because open file descriptors keep the old inode alive. Source: [rustup self_update/unix.rs](https://github.com/rust-lang/rustup/blob/main/src/cli/self_update/unix.rs)

**Windows path:** Windows locks the executable file while it's running, so you can't delete or rename it. Rustup's approach: (1) move the running exe aside, (2) create a copy opened with `FILE_FLAG_DELETE_ON_CLOSE`, (3) spawn it, (4) the copy waits for the parent to exit, then the OS auto-deletes it. This is complex and historically brittle -- multiple issues (#1367, #1188, #2441) document failures. Source: [rustup issue #2441](https://github.com/rust-lang/rustup/issues/2441)

**Auto-self-update control:** Rustup supports `RUSTUP_AUTO_SELF_UPDATE` with values `enable`, `disable`, and `check-only`. When enabled, `rustup update` also updates rustup itself. Source: [rustup issue #3821](https://github.com/rust-lang/rustup/issues/3821)

### 3. self-replace crate (Rust, by Armin Ronacher)

A dedicated library that abstracts the platform differences:
- **Unix:** Atomic rename of a new file next to the current executable. For deletion, the file is just unlinked.
- **Windows:** Moves the running exe aside, creates a copy, and uses OS-level deletion hooks.
- **Cleanup:** Leaves dotfile artifacts (`.original-name.random-suffix`) that could accumulate if power is cut at the wrong moment.

Source: [self-replace crate](https://docs.rs/self-replace/latest/self_replace/)

### 4. Mise (formerly rtx): straightforward download-and-replace

Mise's `self-update` command downloads the latest release from GitHub and replaces the binary. It re-downloads even when already on the latest version (a known quirk discussed in [issue #7664](https://github.com/jdx/mise/discussions/7664)). The mechanism is a direct download-and-replace with no elaborate spawn or sidecar pattern. Past issues include "Text file busy" errors during self-upgrade, indicating that even straightforward approaches hit edge cases.

### 5. Proto (moonrepo): self-as-managed-tool

Proto takes an interesting approach: `proto install proto` treats proto itself as a managed tool. The self-upgrade was rewritten multiple times to handle edge cases. Known failure modes include:
- "Text file busy" errors (the process is writing to its own executable on some systems)
- Race conditions with auto-clean during upgrade
- The "is proto process running?" check was softened from a hard error to a confirmation prompt

Source: [proto changelog](https://github.com/moonrepo/proto/blob/master/CHANGELOG.md)

### 6. Claude Code: background updater with channel model

Claude Code (native installer) checks for updates on startup and periodically while running. Updates download and install in the background, taking effect on next launch (swap-on-restart pattern). Users choose a channel at install time (`latest` or `stable`). Enterprise deployments can disable via `DISABLE_AUTOUPDATER=1`. The npm-based installer had significant self-update issues (ENOTEMPTY errors, shadowing between npm-global and native installs).

Source: [Claude Code setup docs](https://code.claude.com/docs/en/setup), [issue #4117](https://github.com/anthropics/claude-code/issues/4117)

### 7. GitHub CLI (gh): delegates to package manager

The gh CLI does not have a built-in self-update command. Users update through whatever package manager installed it (Homebrew, apt, WinGet, etc.). This sidesteps the binary replacement problem entirely but means users in non-package-manager environments are stuck with manual downloads.

Source: [gh CLI discussion #4630](https://github.com/cli/cli/discussions/4630)

### 8. Homebrew: git pull + rebuild

Homebrew updates itself by running `git pull` on its own repository (since Homebrew is Ruby scripts, not a compiled binary). The `brew update` command is automatically run before `brew install` and `brew upgrade`. There's a 30-day auto-cleanup cycle. A separate `homebrew-autoupdate` tap provides background periodic updates via launchd.

### Summary of approaches

| Tool | Approach | Windows? | Rollback? | Auto-check? |
|------|----------|----------|-----------|-------------|
| minio/selfupdate | rename-in-place | Yes (limited) | Yes (best-effort) | N/A (library) |
| rustup | spawn --self-replace | Yes (complex) | No explicit | Yes (configurable) |
| self-replace crate | rename-in-place | Yes (move-aside) | No | N/A (library) |
| mise | download-and-replace | No | No | No |
| proto | self-as-tool | Partial | Retry-based | Via `proto upgrade` |
| Claude Code | background download, swap-on-restart | Via native installer | Unclear | Yes (channel-based) |
| gh | delegates to pkg manager | Via pkg manager | Via pkg manager | No |
| Homebrew | git pull (not binary) | N/A (macOS) | git revert | Yes (auto before install) |

### 9. Tsuku's current state

Tsuku has **no self-update infrastructure**. The binary is installed as a standalone file at `$TSUKU_HOME/bin/tsuku` via the install.sh script (line 113: `mv "$TEMP_DIR/tsuku" "$BIN_DIR/tsuku"`). It is not managed as a "tool" in tsuku's own registry.

Managed tools use a different architecture: they live in `$TSUKU_HOME/tools/<name>-<version>/` with symlinks in `$TSUKU_HOME/tools/current/`. This multi-version, symlink-based system already supports atomic installation (staging directory + rename), version switching (`Activate`), and rollback on symlink failure.

Key existing infrastructure that could be reused:
- `AtomicSymlink()` in `internal/install/symlink.go` -- creates symlinks atomically via temp-and-rename
- Atomic installation via staging directory in `Manager.InstallWithOptions()` -- copies to `.staging`, then renames
- `CachedVersionLister` in `internal/version/cache.go` -- file-based TTL cache for version lists
- Version resolution from GitHub releases already works via `version.ResolveGitHub()`
- `outdated` command already compares installed vs. latest for managed tools

## Implications

1. **Unix self-update is a solved problem.** The rename-in-place pattern (download temp, rename old, rename new) is reliable and well-understood. Multiple Go libraries implement it. Tsuku should use this pattern, not invent something new. The `minio/selfupdate` or `creativeprojects/go-selfupdate` libraries are production-tested options, or the pattern is simple enough to implement directly (about 30 lines of Go).

2. **Windows support is the hard part.** If tsuku ever targets Windows, the binary replacement problem gets significantly harder. For now, tsuku only supports linux and darwin, so the Unix rename-in-place pattern is sufficient. This should be noted as a future concern, not a current blocker.

3. **Tsuku's self-update differs structurally from tool updates.** Managed tools use the multi-version symlink system. The tsuku binary itself sits directly in `$TSUKU_HOME/bin/tsuku` with no version directory or symlink indirection. Two options:
   - **Option A**: Treat tsuku as a managed tool in its own registry (like proto does). This unifies the model but adds complexity -- what happens if the update mechanism itself is broken?
   - **Option B**: Keep self-update as a separate code path with the simple rename-in-place pattern. Simpler, more resilient, but two update mechanisms to maintain.

4. **Swap-on-restart vs. immediate replacement.** Claude Code downloads in the background and swaps on restart. Most CLI tools do immediate replacement. For tsuku, immediate replacement makes more sense -- CLI tools run briefly (unlike long-running IDE extensions), so "next launch" is already imminent. The rename-in-place pattern means the current invocation continues with the old binary and the next invocation gets the new one automatically.

5. **Existing cache infrastructure is directly reusable.** The `CachedVersionLister` with TTL-based file caching is exactly what auto-update checking needs. Extending it to cache "latest version of tsuku" alongside "latest versions of managed tools" is straightforward.

## Surprises

1. **Proto's "Text file busy" error.** Even on Unix, you can hit "text file busy" (ETXTBSY) if you try to *write* to a running executable rather than renaming it. The rename-in-place pattern avoids this, but a naive `os.WriteFile()` to the binary's path would fail. This is a subtle but important distinction -- you must rename, not overwrite.

2. **gh CLI has no self-update at all.** A major CLI tool with millions of users simply delegates to package managers. This is a legitimate design choice, not an oversight. It avoids the entire binary replacement problem at the cost of update friction.

3. **Mise re-downloads even when current.** The lack of version comparison before downloading suggests self-update is often simpler than expected in practice -- tools don't always bother with the "am I already current?" check.

4. **Claude Code's auto-update has caused significant user pain.** Issues #4117 and #3168 show that automatic updating can go wrong in ways that leave users unable to use the tool at all. The npm/native installer conflict is a cautionary tale about having multiple installation methods that don't coordinate their update paths.

5. **Rollback is rare in practice.** Most tools don't implement explicit rollback for self-update. Rustup doesn't. Mise doesn't. Proto doesn't. Only library-level implementations (minio/selfupdate) offer it. The assumption seems to be that if a download succeeds and passes checksum verification, it will work. Tsuku's requirement for rollback is more ambitious than the norm.

## Open Questions

1. **Should tsuku treat itself as a managed tool?** The proto approach (self-as-tool) is elegant but creates a bootstrap problem. If the update mechanism itself is broken, you can't self-update to fix it. The separate code path is more resilient. This is a design decision, not a research question.

2. **What happens to in-flight commands during self-update?** If a user runs `tsuku install foo` in one terminal and `tsuku self-update` in another, the first command continues with the old binary (Unix inode semantics). But should tsuku detect concurrent runs and warn? Proto changed its "is process running?" check from a hard error to a prompt.

3. **How does GoReleaser's release format affect self-update?** Tsuku uses GoReleaser for releases, producing platform-specific binaries and checksums. The self-update mechanism needs to know the release naming convention (`tsuku-{os}-{arch}`) and checksum file format to download the right artifact. This is already solved in `install.sh` but needs to be replicated in Go code.

4. **Should self-update go through the same checksum verification as install.sh?** The install script verifies SHA256 checksums. Self-update should do the same, plus potentially verify signatures if tsuku adopts code signing.

5. **What's the right UX for self-update?** `tsuku self-update`, `tsuku update --self`, `tsuku upgrade`? Convention varies: rustup uses `rustup self update` (subcommand), mise uses `mise self-update` (hyphenated command), proto uses `proto upgrade`.

## Summary

The dominant pattern for CLI self-update on Unix is rename-in-place: download the new binary to a temp file, rename the old binary away, rename the new one in, then clean up -- this works because Unix processes hold file descriptors to inodes, not directory entries. Tsuku's existing atomic installation infrastructure (staging directories, atomic symlinks, cached version resolution) provides most of the building blocks, but the tsuku binary itself sits outside the managed-tool system at `$TSUKU_HOME/bin/tsuku`, so self-update needs either a dedicated code path or a decision to treat tsuku as its own managed tool. The biggest open design question is whether to unify self-update with tool update (cleaner model, bootstrap risk) or keep them separate (two code paths, more resilient).
