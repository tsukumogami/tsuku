---
status: Current
upstream: docs/prds/PRD-auto-update.md
spawned_from:
  issue: 2182
  repo: tsukumogami/tsuku
problem: |
  Tsuku has no self-update path. Users must re-run the installer script to get
  new versions, which creates friction and lets the binary fall behind silently.
  Feature 2's background checks can detect a newer tsuku release, but there's no
  mechanism to act on that information.
decision: |
  Tsuku auto-updates itself during the background update check, using the same
  resolution, verification, and two-rename replacement as managed tools but through
  a separate code path. The background checker resolves the latest release via
  GitHubProvider, constructs the download URL from GoReleaser's naming convention,
  verifies SHA256 from checksums.txt, and replaces the binary atomically.
  A manual tsuku self-update command provides the same flow as a fallback.
  Auto-apply is on by default and controlled by updates.self_update in user config.
rationale: |
  The self-update code path is separate from the recipe pipeline (no bootstrap
  risk), and the two-rename rollback provides identical safety whether triggered
  automatically or manually. Defaulting to auto-apply keeps tsuku current without
  user intervention, matching the update behavior users expect from modern CLI
  tools. Direct asset name construction avoids API calls and rate limits.
  Same-directory temp file guarantees rename atomicity without staging
  infrastructure.
---

# DESIGN: Self-Update Mechanism

## Status

Current

## Context and Problem Statement

Tsuku has no self-update path today. Users must re-run the installer script to get new versions, which creates friction and means tsuku can silently fall behind. Feature 2's background update check infrastructure can detect when a newer tsuku release exists, but there's no command to act on that information.

The self-update mechanism must be separate from the managed tool system (PRD decision D5) to avoid bootstrap risk -- a broken recipe pipeline that can't update tsuku through itself. The rename-in-place pattern is well-understood and keeps the self-update path simple. Since the two-rename rollback provides identical safety whether triggered automatically or by a user command, tsuku should auto-apply updates by default during the background check, similar to how Claude Code and other modern CLI tools handle self-updates.

GoReleaser produces platform-specific binaries (`tsuku-{os}-{arch}`) with SHA256 checksums, published as GitHub releases on `tsukumogami/tsuku`. The `buildinfo` package already has `Version()` and `Commit` variables injected via ldflags at build time.

## Decision Drivers

- **Atomic replacement**: A failed update must never leave the user without a working tsuku binary
- **Simplicity**: PRD D5 explicitly chose a separate code path (~30 lines) over treating tsuku as a managed tool, to avoid bootstrap risk
- **Verification**: Downloaded binaries must be checksum-verified before replacement
- **Integration**: Should work with Feature 2's existing update cache so `tsuku outdated` can report tsuku's staleness
- **No version pinning**: tsuku always tracks latest stable (PRD R7)
- **Auto-apply by default**: tsuku should stay current without user intervention, matching modern CLI tool behavior (Claude Code, rustup). Manual `tsuku self-update` is a fallback, not the primary path

## Considered Options

### Decision 1: Release asset resolution and download

The self-update command needs to find the latest release, download the correct binary for the current platform, and verify its checksum. GoReleaser produces binaries named `tsuku-{os}-{arch}` alongside a `checksums.txt` file with SHA256 hashes. The existing codebase has version resolution infrastructure (`GitHubProvider.ResolveLatest`), but the download path should stay simple per D5.

Key assumptions:
- GoReleaser's binary naming convention (`tsuku-{os}-{arch}`) will remain stable. Low risk since tsuku controls `.goreleaser.yaml`.
- `checksums.txt` is always present in releases and uses SHA256. GoReleaser produces this by default and the config explicitly sets `algorithm: sha256`.
- GitHub releases remain the sole distribution channel.

#### Chosen: Direct asset name construction with checksums.txt parsing

Construct the expected asset name deterministically from `runtime.GOOS` and `runtime.GOARCH` using the known GoReleaser naming pattern, then download `checksums.txt` to extract the expected SHA256 hash before downloading the binary.

The flow:

1. **Resolve latest version**: Use `GitHubProvider.ResolveLatest()` against `tsukumogami/tsuku`. Both `buildinfo.Version()` and the resolved version are normalized by stripping any `v` prefix before comparison (`buildinfo.Version()` returns `"v0.5.0"` from GoReleaser ldflags, while the resolver returns `"0.5.0"`). If equal, report "already up to date" and exit. If the resolved version is older than the current version (semver comparison), exit with no action -- this prevents downgrade attacks where a compromised "latest" points to a vulnerable old release.

2. **Construct asset name**: `fmt.Sprintf("tsuku-%s-%s", runtime.GOOS, runtime.GOARCH)`. This matches the GoReleaser template exactly because Go's `runtime.GOOS`/`runtime.GOARCH` values are what GoReleaser uses.

3. **Construct download URLs**: Standard GitHub release download pattern:
   ```
   https://github.com/tsukumogami/tsuku/releases/download/{tag}/tsuku-{os}-{arch}
   https://github.com/tsukumogami/tsuku/releases/download/{tag}/checksums.txt
   ```

4. **Download and parse checksums.txt**: Fetch the small file using `httputil.NewClient()` (reuses the codebase's hardened HTTP client). Parse line-by-line (`{sha256hash}  {filename}`), validating that each hash is exactly 64 lowercase hex characters. Find the line matching the target asset name. If `checksums.txt` is missing from the release or doesn't contain the target asset, abort with a hard error -- never download a binary without a checksum to verify against.

5. **Download binary with verification**: Download to a temp file using the same `httputil.NewClient()`. Verify SHA256 matches, abort on mismatch.

6. **Hand off**: Pass the verified temp file path to the replacement mechanism (Decision 2).

This keeps the core logic under 30 lines because it avoids API-based asset discovery entirely and computes everything from known conventions.

#### Alternatives considered

- **GitHub Releases API asset listing**: Use `FetchReleaseAssets()` to list all assets, then pattern-match with `MatchAssetPattern()`. Rejected because we control the naming convention -- discovery adds no value when we already know the exact filename. The extra API call also counts against rate limits.

- **Full recipe pipeline**: Define tsuku as a recipe and use the standard install/update path. Rejected per PRD D5 -- the managed tool system can't update itself if it's broken. It would also mean tsuku appears in `tsuku list` alongside managed tools, blurring the manager/managed boundary.

- **Hardcoded latest-release redirect**: Use GitHub's `.../releases/latest/download/{asset}` redirect URL to skip version resolution. Rejected because it loses the ability to compare versions before downloading -- users would download on every invocation even when already up to date, and there's no version string for output ("Updated to v0.5.0").

### Decision 2: Binary replacement strategy

The replacement sequence is the most safety-critical part: a failure at any point must never leave the user without a working binary (PRD R7). The binary may live at `$TSUKU_HOME/bin/tsuku` (common case) or elsewhere (`/usr/local/bin/tsuku` from `go install`, a Homebrew prefix). The code must handle symlinks, cross-filesystem temp directories, and permission preservation within PRD D5's ~30 line complexity budget.

On Linux and macOS, `os.Rename` implements POSIX `rename(2)`, which atomically replaces the destination when source and destination are on the same filesystem. A running binary can be safely replaced because the OS keeps the old inode open until the process exits.

Key assumptions:
- tsuku won't be installed on read-only filesystems in normal use. If it is, self-update fails with a clear error.
- The microsecond window between the two renames (current->.old, temp->current) is an accepted risk. Every major self-updating CLI (gh, rustup) accepts this same window.
- The backup `.old` file lives next to the actual binary (wherever it is), not always at `$TSUKU_HOME/bin/tsuku.old`. This refines PRD R7's literal text to handle binaries outside `$TSUKU_HOME/bin/`.

#### Chosen: Same-directory temp with two-rename backup

The replacement sequence:

1. Resolve the real binary path: `os.Executable()` then `filepath.EvalSymlinks()` to follow symlinks to the actual file.
2. Stat the current binary to capture permissions: `os.Stat(exePath)`.
3. Create a temp file in the same directory: `os.CreateTemp(filepath.Dir(exePath), ".tsuku-update-*")`. Same-directory placement guarantees same-filesystem, which guarantees `os.Rename` atomicity.
4. Write the downloaded binary to the temp file. (Download and verification happen in Decision 1's flow.)
5. Set permissions on the temp file: `os.Chmod(tmp.Name(), info.Mode())`.
6. Remove stale backup: `os.Remove(exePath + ".old")` (ignore error -- may not exist).
7. Create backup: `os.Rename(exePath, exePath + ".old")`.
8. Install new binary: `os.Rename(tmp.Name(), exePath)`.
9. If step 8 fails: restore from backup: `os.Rename(exePath + ".old", exePath)`. Clean up temp file.

Edge case handling:

- **Binary outside `$TSUKU_HOME/bin/`**: Works as-is. The `.old` backup and temp file are created next to the real binary.
- **Symlink target**: `filepath.EvalSymlinks` resolves to the real path. The replacement happens at the real location; the symlink continues to point there.
- **Cross-filesystem temp**: Impossible by construction. `os.CreateTemp` uses `filepath.Dir(exePath)`, the same directory and filesystem as the target.
- **Insufficient permissions**: `os.CreateTemp` fails early (step 3), before touching the current binary. Clear error message.
- **Interrupted between renames**: The `.old` backup exists and the temp file exists. User can recover manually, or the next tsuku invocation could detect and recover. Same accepted risk as gh, rustup, and similar tools.

Backup lifecycle: created during self-update (step 7), preserved until the next successful self-update (step 6 removes it), restored on failure (step 9).

#### Alternatives considered

- **Staging in `$TSUKU_HOME/tmp/`**: Download to a staging directory, then copy to the binary's directory for the final rename. Rejected because the extra copy adds I/O and complexity without improving the critical replacement phase. If you have write permission for the rename, you have it for a temp file in the same directory.

- **Direct overwrite with copy-back backup**: Use `io.Copy` instead of `os.Rename` for the replacement. Rejected because `io.Copy` is not atomic -- an interrupted copy produces a corrupt binary, directly violating PRD R7.

- **Hardlink + atomic overwrite**: Create a hard link as backup, then atomically overwrite the original via rename. Eliminates the microsecond gap between renames. Rejected because hard links have filesystem restrictions (can't span mounts, some filesystems disallow them) requiring a fallback path that is the two-rename approach anyway. The combined code exceeds the ~30 line target for a gap that's not a real-world concern on a user-initiated CLI command.

### Decision 3: Update cache integration

Feature 2's `RunUpdateCheck()` iterates installed tools from `state.json`, resolves their latest versions, and writes per-tool cache entries to `$TSUKU_HOME/cache/updates/<toolname>.json`. Feature 3's `MaybeAutoApply` reads those entries and applies updates. tsuku itself has no recipe and no `state.json` entry (PRD D5), so it needs a separate check-and-apply path within the same background flow.

The challenge: (1) injecting a self-check into the existing background check flow without a recipe, (2) auto-applying the update using the binary replacement logic from Decision 2, and (3) notifying the user on the next invocation.

Key assumptions:
- The tool name "tsuku" won't collide with a managed recipe. Holds because D5 excludes tsuku from the managed tool system.
- `buildinfo.Version()` returns a semver-parseable string for release builds. Dev builds return "dev-..." which won't match any release, so the check correctly reports "no update available."
- The background process can replace the binary safely because no tsuku instance holds an open file handle on itself -- the OS keeps the old inode alive until the process exits.

#### Chosen: Background auto-apply with well-known constant

Add `checkAndApplySelf()` called at the end of `RunUpdateCheck`, after the tool loop. This function:

1. Checks `userCfg.UpdatesSelfUpdate()` -- returns early if disabled.
2. Gets the current version from `buildinfo.Version()`. Both `buildinfo.Version()` and the resolved version are normalized by stripping any `v` prefix before comparison.
3. Creates a `GitHubProvider` for `tsukumogami/tsuku` directly (no recipe needed).
4. Calls `ResolveLatest()` to get the newest release.
5. Writes a standard `UpdateCheckEntry` with `Tool: SelfToolName`, `ActiveVersion` set to the normalized current version, and `LatestOverall` set to the resolved version.
6. If a newer version is available: acquires the self-update file lock, runs the full download-verify-replace flow from Decision 1 and Decision 2, and writes a success notice via `internal/notices/WriteNotice` containing the old and new version strings.

A package-level constant `const SelfToolName = "tsuku"` identifies the self-update entry. `MaybeAutoApply` skips entries where `entry.Tool == updates.SelfToolName` since self-updates are handled by their own code path (not via `tsuku update <tool>`). This is defense-in-depth -- the existing `LatestWithinPin != ""` filter would also exclude the tsuku entry since that field is never populated, but the explicit guard makes the exclusion visible and survives future filter changes. Consumers like `tsuku outdated` and Feature 5 check this constant to format the display differently.

**Notification mechanism:** On the next tsuku invocation, the existing `displayUnshownNotices` flow in `PersistentPreRun` picks up the self-update notice. The notice is formatted using `IsSelfUpdate()` to produce "tsuku has been updated from v0.5.0 to v0.6.0" rather than the standard tool update format.

**Failure handling:** If the background auto-apply fails (network error, checksum mismatch, permission error), the failure is logged to the cache entry but the current binary is untouched (rollback guarantees from Decision 2 apply). The user can retry manually with `tsuku self-update`. No notification is shown for failed background attempts -- the failure surfaces through `tsuku outdated` showing the available version.

The entry uses the same cache directory as tool entries (`$TSUKU_HOME/cache/updates/tsuku.json`), so `ReadAllEntries` picks it up without consumer changes beyond display formatting.

#### Alternatives considered

- **Manual-only self-update**: Require users to run `tsuku self-update` explicitly, with the background check only caching version availability. Rejected because it adds friction that isn't justified by safety concerns -- the two-rename rollback provides identical protection whether triggered automatically or manually. Users who want manual control can set `updates.self_update = false`.

- **Auto-apply through MaybeAutoApply**: Route self-updates through the existing `MaybeAutoApply` code path instead of a separate function. Rejected because `MaybeAutoApply` calls `tsuku update <tool>`, which goes through the recipe pipeline. tsuku has no recipe (D5), so the managed tool path can't handle it. The self-update needs its own download-and-replace logic.

- **Separate self-update cache file**: Write tsuku's check to `$TSUKU_HOME/cache/self-update.json` with its own struct. Rejected because it fragments every consumer. `ReadAllEntries` wouldn't see it, so `tsuku outdated`, Feature 5 notifications, and any future consumer would each need separate file handling. The constant already prevents confusion with managed tools.

## Decision Outcome

The self-update mechanism has three components: a resolution and download flow, a binary replacement sequence, and background auto-apply with notification.

Tsuku auto-updates itself during the background update check that runs on each invocation. The `checkAndApplySelf()` function, called at the end of `RunUpdateCheck`, resolves the latest stable release from `tsukumogami/tsuku` using `GitHubProvider.ResolveLatest()` and compares it against `buildinfo.Version()` (both normalized by stripping the `v` prefix). If the resolved version is older than the current version (semver comparison), it exits with no action to prevent downgrade attacks. If a newer version is available, it acquires a non-blocking file lock on `$TSUKU_HOME/cache/updates/.self-update.lock`, constructs the download URL from the known GoReleaser naming convention (`tsuku-{os}-{arch}`), downloads `checksums.txt` to get the expected SHA256 hash, and downloads the binary to a temp file in the same directory as the running binary. After verifying the checksum, it performs the two-rename replacement: rename the current binary to `.old`, rename the temp file into place. If the second rename fails, it restores from the `.old` backup. On success, it writes a notice via the existing `internal/notices/` system.

The binary path is resolved via `os.Executable()` plus `filepath.EvalSymlinks()`, so self-update works regardless of where tsuku is installed -- `$TSUKU_HOME/bin/`, `/usr/local/bin/`, or behind a symlink. The temp file is created in the same directory as the binary (`os.CreateTemp(filepath.Dir(exePath), ...)`), which guarantees same-filesystem atomicity for `os.Rename`. Permission bits are copied from the existing binary before the replacement.

On the next invocation, the existing `displayUnshownNotices` flow in `PersistentPreRun` picks up the notice and prints "tsuku has been updated from v0.5.0 to v0.6.0" to stderr. The `tsuku self-update` command provides the same flow as a manual fallback for users who want to trigger an update immediately or who have disabled auto-apply.

Auto-apply is controlled by `updates.self_update` in user config (default: true). It is also suppressed when `CI=true` or `TSUKU_NO_SELF_UPDATE=1` is set, matching the suppression pattern used by `UpdatesAutoApplyEnabled()`. When disabled, the background check still writes the cache entry so `tsuku outdated` can report availability, but skips the download-and-replace step.

The three decisions reinforce each other: the direct asset name construction (D1) keeps the download path simple enough to stay within D5's complexity budget. The same-directory temp file placement (D2) avoids cross-filesystem issues without adding staging infrastructure. The well-known constant (D3) reuses the entire cache ecosystem without schema changes, while giving the auto-apply path a clean way to identify and handle tsuku's entry separately from managed tools.

## Solution Architecture

### Overview

Two new files and three modifications compose the solution. The core self-update logic (download, verify, replace) lives in `internal/updates/self.go` and is called from both the background checker and the manual `self-update` command. No new packages are needed.

### Components

**`internal/updates/self.go`** (new file)
- `const SelfToolName = "tsuku"` -- well-known constant
- `const SelfRepo = "tsukumogami/tsuku"` -- GitHub repository
- `checkAndApplySelf(ctx, cacheDir, resolver, exePath)` -- resolves latest version, writes cache entry, and if a newer version is available and auto-apply is enabled (not CI, not suppressed by env var), downloads, verifies, and replaces the binary. Writes a notice via `internal/notices/` on success.
- `applySelfUpdate(ctx, exePath, tag, assetName)` -- the download-verify-replace core: fetches checksums.txt, downloads binary to same-directory temp, verifies SHA256, performs two-rename replacement with rollback.
- `IsSelfUpdate(entry *UpdateCheckEntry) bool` -- convenience check against `SelfToolName`

**`cmd/tsuku/cmd_self_update.go`** (new file)
- `selfUpdateCmd` cobra command registered in `main.go`
- `runSelfUpdate()` calls `applySelfUpdate()` directly after resolving the latest version. Provides interactive output ("Downloading...", "Updated to v0.6.0") that the background path omits.
- No flags. Self-update always tracks latest stable.

**`internal/updates/checker.go`** (modified)
- `RunUpdateCheck` gains a `checkAndApplySelf()` call after the tool iteration loop, gated on `userCfg.UpdatesSelfUpdate()` (which checks config, `CI=true`, and `TSUKU_NO_SELF_UPDATE`)

**`internal/updates/apply.go`** (modified, from Feature 3)
- `MaybeAutoApply` gains a one-line skip: `if entry.Tool == SelfToolName { continue }`. Self-updates are handled by their own code path in `checkAndApplySelf`, not via `tsuku update <tool>`. This is defense-in-depth -- the existing `LatestWithinPin` filter would also exclude the entry, but the explicit guard makes the exclusion visible.

**`cmd/tsuku/main.go`** (modified)
- `rootCmd.AddCommand(selfUpdateCmd)` added to command registration
- `self-update` added to the PersistentPreRun skip list (alongside `check-updates`, `hook-env`, etc.) to avoid redundant background check spawns during self-update
- Self-update success notices are displayed by the existing `displayUnshownNotices` flow (no new notification mechanism needed)

### Key Interfaces

```go
// Well-known constant identifying tsuku's own cache entry.
const SelfToolName = "tsuku"

// checkAndApplySelf resolves tsuku's latest version, writes a cache entry,
// and auto-applies the update if a newer version is available.
// The caller (RunUpdateCheck) gates this on userCfg.UpdatesSelfUpdate().
// When autoApply is false, only the cache entry is written (no download/replace).
func checkAndApplySelf(ctx context.Context, cacheDir string, resolver *version.Resolver, exePath string, autoApply bool)

// applySelfUpdate downloads, verifies, and replaces the tsuku binary.
// Used by both the background checker and the manual self-update command.
func applySelfUpdate(ctx context.Context, exePath string, tag string, assetName string) error

// IsSelfUpdate returns true if the cache entry represents tsuku itself.
func IsSelfUpdate(entry *UpdateCheckEntry) bool
```

### Data Flow

**Background auto-apply (primary path):**

```
tsuku <any command> (PersistentPreRun spawns background check)
  |
  v
tsuku check-updates (background process)
  |
  v
RunUpdateCheck iterates installed tools (existing)
  |
  v
checkAndApplySelf(ctx, cacheDir, resolver, exePath)
  |
  v
GitHubProvider.ResolveLatest("tsukumogami/tsuku")
  |
  v
Normalize versions (strip "v" prefix), semver compare
  |
  +-- equal or older: write cache entry only -> done
  |
  +-- newer available + updates.self_update enabled:
        |
        v
      Acquire non-blocking lock (.self-update.lock)
        |
        +-- lock held: skip apply, cache entry written -> done
        |
        +-- lock acquired: applySelfUpdate(ctx, exePath, tag, assetName)
              |
              v
            Download checksums.txt, parse SHA256
            Download binary to same-dir temp, verify SHA256
            Two-rename replacement with rollback
              |
              +-- SUCCESS: WriteNotice("tsuku updated from v0.5.0 to v0.6.0")
              +-- FAILURE: log error, current binary untouched
  |
  v
Write $TSUKU_HOME/cache/updates/tsuku.json
MaybeAutoApply skips entry where Tool == SelfToolName

---

Next tsuku invocation (PersistentPreRun):
  |
  v
displayUnshownNotices (existing flow)
  |
  +-- self-update notice: print "tsuku has been updated from v0.5.0 to v0.6.0"
  +-- no notices: continue normally
```

**Manual self-update (fallback):**

```
tsuku self-update
  |
  v
Resolve latest version, normalize and compare
  |
  +-- equal: "tsuku is already up to date (0.5.0)" -> exit
  +-- older: "Current version is newer than latest release" -> exit
  |
  +-- newer available:
        |
        v
      Acquire non-blocking lock (.self-update.lock)
        +-- lock held: "Another self-update is running" -> exit
        +-- lock acquired: applySelfUpdate(ctx, exePath, tag, assetName)
              |
              +-- SUCCESS: "Updated tsuku from v0.5.0 to v0.6.0"
              +-- FAILURE: "Self-update failed: <error>. Current binary restored."
```

## Implementation Approach

### Phase 1: Core self-update logic and background auto-apply

Add the self-update core (`applySelfUpdate`) and background integration (`checkAndApplySelf`) to `internal/updates/`. This is the primary update path -- tsuku auto-updates during the background check. Add the `MaybeAutoApply` skip guard and the `PersistentPreRun` notification check.

Deliverables:
- `internal/updates/self.go` (new: constants, `checkAndApplySelf`, `applySelfUpdate`, `IsSelfUpdate`)
- `internal/updates/self_test.go`
- `internal/updates/checker.go` (modified: call `checkAndApplySelf` after tool loop)
- `internal/updates/apply.go` (modified: skip `SelfToolName` in `MaybeAutoApply`)
- `internal/userconfig/userconfig.go` (modified: CI and env var suppression for `UpdatesSelfUpdate`)

### Phase 2: Manual self-update command

Add the `tsuku self-update` command as a manual fallback. Calls `applySelfUpdate()` directly with interactive output.

Deliverables:
- `cmd/tsuku/cmd_self_update.go` (new: cobra command, `runSelfUpdate`)
- `cmd/tsuku/cmd_self_update_test.go`
- `cmd/tsuku/main.go` (modified: register command, add to PersistentPreRun skip list)

### Phase 3: Outdated display integration

Update `tsuku outdated` to display tsuku's own staleness from the cache entry, formatted distinctly from managed tools ("tsuku v0.6.0 available" rather than the regular tool update format).

Deliverables:
- `cmd/tsuku/outdated.go` (modified: check for `SelfToolName` entry, format differently)

## Security Considerations

**Integrity verification.** Downloaded binaries are verified against SHA256 checksums from the release's `checksums.txt`. This detects corruption during transfer and CDN-level tampering. The checksum parser validates hash format (64 hex characters) and rejects malformed lines.

**Authenticity limitations.** The checksum file is hosted alongside the binary in the same GitHub release. If an attacker gains write access to the release (compromised maintainer credentials, CI pipeline injection), they can replace both artifacts and the checksum verification passes. This is the same trust model used by gh, rustup, cargo-binstall, and other self-updating CLIs that distribute via GitHub releases.

**Future hardening: cosign signatures.** GoReleaser supports Sigstore cosign signing with keyless OIDC-based identity. Adding cosign verification to the self-update flow would tie binary authenticity to the GitHub Actions build identity, not just the release's contents. This is tracked as a follow-up enhancement, not a launch blocker, because the current trust model matches industry standard practice.

**Downgrade protection.** The version comparison uses semver ordering, not just equality. If the resolved "latest" version is older than the running binary, self-update exits with no action. This prevents a compromised release from pointing "latest" at a known-vulnerable old version.

**Background binary replacement.** The auto-apply runs in a background process spawned by `PersistentPreRun`. The running tsuku instance is unaffected because the OS keeps the old inode open until the process exits. The replacement binary takes effect on the next invocation. This is the same mechanism used by Claude Code's auto-updater and other CLI tools that update in the background.

**CI environment suppression.** Auto-apply is suppressed when `CI=true` or `TSUKU_NO_SELF_UPDATE=1` is set, matching the suppression pattern used by `UpdatesAutoApplyEnabled()` for managed tools. This prevents the background process from replacing the tsuku binary mid-pipeline in CI, which would break reproducibility.

**Concurrent self-update.** A non-blocking file lock (`$TSUKU_HOME/cache/updates/.self-update.lock`) prevents two simultaneous self-update operations (background or manual) from corrupting the binary via competing rename sequences. If the lock is held, the background checker skips the apply step silently; the manual command exits with an error message.

**Missing checksums.** If `checksums.txt` is absent from a release or doesn't contain an entry for the target binary, self-update aborts with a hard error. No binary is ever downloaded without a checksum to verify against.

**Transport security.** All downloads use HTTPS. Go's `net/http` client validates TLS certificates against the system trust store. No HTTP fallback is supported.

**Permission safety.** The temp file is created in the same directory as the target binary, so `os.CreateTemp` fails early if the user lacks write permission -- before any modification to the existing binary. The `.old` backup retains the original binary's permission bits via `os.Rename`.

**No data transmission.** The self-update command only makes GET requests to GitHub (API for version resolution, CDN for artifact download). No local state, tool list, or usage data is transmitted.

## Consequences

### Positive

- Tsuku stays current automatically without user intervention
- Users who prefer manual control can disable auto-apply via `updates.self_update = false` and use `tsuku self-update`
- The self-update code path is simple (~20 lines of replacement logic) and auditable
- Same-directory temp file eliminates cross-filesystem rename failures by construction
- Checksum verification via `checksums.txt` ensures binary integrity without API rate limit consumption
- Existing `GitHubProvider` infrastructure handles version resolution, rate limits, and error handling

### Negative

- The self-update code couples to the GoReleaser naming convention. A naming template change in `.goreleaser.yaml` requires a corresponding code change.
- A microsecond gap exists between the two renames where no binary is at the expected path. A concurrent `exec` during this window would get ENOENT.
- Users whose binary is in a directory requiring elevated permissions (e.g., `/usr/local/bin/` owned by root) get an error and must use `sudo` or relocate the binary.
- The `.old` backup lives next to the actual binary, which may not be `$TSUKU_HOME/bin/`. Documentation must use "next to the tsuku binary" rather than a fixed path.

### Mitigations

- The naming convention coupling is low risk: both sides live in the same repository and would be updated together. A CI check could verify the constructed name matches the GoReleaser config.
- The rename gap is sub-microsecond on modern filesystems. A concurrent `tsuku` invocation during this window is extremely unlikely and would get ENOENT, recoverable by retrying the command.
- Permission errors are detected early (temp file creation fails before touching the current binary) with a clear error message suggesting `sudo` or relocation.
- The `.old` location is deterministic (`exePath + ".old"`) and discoverable via `tsuku self-update --help` documentation.
