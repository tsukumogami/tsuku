---
status: Proposed
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
  A standalone tsuku self-update command resolves the latest release via
  GitHubProvider, constructs the download URL from GoReleaser's naming convention,
  verifies SHA256 from checksums.txt, and replaces the binary using a same-directory
  temp file with two-rename backup. Background version awareness reuses Feature 2's
  cache via a well-known SelfToolName constant, excluded from auto-apply.
rationale: |
  Direct asset name construction avoids API calls and rate limits. Same-directory
  temp file guarantees rename atomicity without staging infrastructure. The
  well-known constant reuses the entire cache ecosystem without schema changes
  while cleanly separating tsuku from managed tools.
---

# DESIGN: Self-Update Mechanism

## Status

Proposed

## Context and Problem Statement

Tsuku has no self-update path today. Users must re-run the installer script to get new versions, which creates friction and means tsuku can silently fall behind. Feature 2's background update check infrastructure can detect when a newer tsuku release exists, but there's no command to act on that information.

The self-update mechanism must be separate from the managed tool system (PRD decision D5) to avoid bootstrap risk -- a broken updater that can't fix itself. The rename-in-place pattern is well-understood and keeps the self-update path simple.

GoReleaser produces platform-specific binaries (`tsuku-{os}-{arch}`) with SHA256 checksums, published as GitHub releases on `tsukumogami/tsuku`. The `buildinfo` package already has `Version()` and `Commit` variables injected via ldflags at build time.

## Decision Drivers

- **Atomic replacement**: A failed update must never leave the user without a working tsuku binary
- **Simplicity**: PRD D5 explicitly chose a separate code path (~30 lines) over treating tsuku as a managed tool, to avoid bootstrap risk
- **Verification**: Downloaded binaries must be checksum-verified before replacement
- **Integration**: Should work with Feature 2's existing update cache so `tsuku outdated` can report tsuku's staleness
- **No version pinning**: tsuku always tracks latest stable (PRD R7)
- **Feature 3 exclusion**: `MaybeAutoApply` must not auto-apply tsuku updates -- self-update is a deliberate user action

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

Feature 2's `RunUpdateCheck()` iterates installed tools from `state.json`, resolves their latest versions, and writes per-tool cache entries to `$TSUKU_HOME/cache/updates/<toolname>.json`. tsuku itself has no recipe and no `state.json` entry (PRD D5). Yet R8 requires that tsuku's own version appears in the periodic check so `tsuku outdated` and Feature 5 notifications can surface self-update availability.

The challenge: (1) injecting a self-check into the existing background check flow without a recipe, and (2) ensuring Feature 3's `MaybeAutoApply` skips tsuku since self-update is a deliberate user action.

Key assumptions:
- The tool name "tsuku" won't collide with a managed recipe. Holds because D5 excludes tsuku from the managed tool system.
- `buildinfo.Version()` returns a semver-parseable string for release builds. Dev builds return "dev-..." which won't match any release, so the check correctly reports "no update available."

#### Chosen: Append self-check to RunUpdateCheck with well-known constant

Add a `checkSelf()` function called at the end of `RunUpdateCheck`, after the tool loop. This function:

1. Checks `userCfg.UpdatesSelfUpdate()` -- returns early if disabled.
2. Gets the current version from `buildinfo.Version()`.
3. Creates a `GitHubProvider` for `tsukumogami/tsuku` directly (no recipe needed).
4. Calls `ResolveLatest()` to get the newest release.
5. Writes a standard `UpdateCheckEntry` with `Tool: SelfToolName`, `ActiveVersion: buildinfo.Version()`, empty `Requested` and `LatestWithinPin`, and `LatestOverall` set to the resolved version.

A package-level constant `const SelfToolName = "tsuku"` identifies the self-update entry. `MaybeAutoApply` skips any entry where `entry.Tool == updates.SelfToolName`. Consumers like `tsuku outdated` and Feature 5 check this constant to format the display differently (e.g., "tsuku v0.5.0 available (run: tsuku self-update)" instead of the regular tool update format).

The entry uses the same cache directory as tool entries (`$TSUKU_HOME/cache/updates/tsuku.json`), so `ReadAllEntries` picks it up without consumer changes beyond display formatting.

#### Alternatives considered

- **Add `IsSelfUpdate` boolean field to `UpdateCheckEntry`**: A new field on the struct, with `MaybeAutoApply` checking the flag instead of the tool name. Rejected because it adds a schema change for exactly one entry. The well-known constant achieves the same disambiguation without modifying the shared data model.

- **Separate self-update cache file**: Write tsuku's check to `$TSUKU_HOME/cache/self-update.json` with its own struct. Rejected because it fragments every consumer. `ReadAllEntries` wouldn't see it, so `tsuku outdated`, Feature 5 notifications, and any future consumer would each need separate file handling. The constant already prevents confusion with managed tools.

## Decision Outcome

The self-update mechanism has three components: a resolution and download flow, a binary replacement sequence, and integration with the background update cache.

When a user runs `tsuku self-update`, the command acquires a non-blocking file lock on `$TSUKU_HOME/cache/updates/.self-update.lock` to prevent concurrent self-update runs from corrupting the binary via competing rename sequences. It then resolves the latest stable release from `tsukumogami/tsuku` using `GitHubProvider.ResolveLatest()` and compares it against `buildinfo.Version()` (both normalized by stripping the `v` prefix). If already current, it exits early. If the resolved version is older than the current version (semver comparison), it exits with no action to prevent downgrade attacks. Otherwise, it constructs the download URL from the known GoReleaser naming convention (`tsuku-{os}-{arch}`), downloads `checksums.txt` to get the expected SHA256 hash, and downloads the binary to a temp file in the same directory as the running binary. After verifying the checksum, it performs the two-rename replacement: rename the current binary to `.old`, rename the temp file into place. If the second rename fails, it restores from the `.old` backup. The old backup persists until the next successful self-update.

The binary path is resolved via `os.Executable()` plus `filepath.EvalSymlinks()`, so self-update works regardless of where tsuku is installed -- `$TSUKU_HOME/bin/`, `/usr/local/bin/`, or behind a symlink. The temp file is created in the same directory as the binary (`os.CreateTemp(filepath.Dir(exePath), ...)`), which guarantees same-filesystem atomicity for `os.Rename`. Permission bits are copied from the existing binary before the replacement.

For background awareness (R8), `RunUpdateCheck` gains a `checkSelf()` call after the tool loop. It uses `GitHubProvider` directly (no recipe) to check `tsukumogami/tsuku` and writes a standard `UpdateCheckEntry` with `Tool: SelfToolName` (a well-known constant). `MaybeAutoApply` skips this entry with a one-line guard. `tsuku outdated` and Feature 5 notifications check the constant to format differently: "tsuku v0.5.0 available (run: tsuku self-update)."

The three decisions reinforce each other: the direct asset name construction (D1) keeps the download path simple enough to stay within D5's complexity budget. The same-directory temp file placement (D2) avoids cross-filesystem issues without adding staging infrastructure. The well-known constant (D3) reuses the entire cache ecosystem without schema changes, while cleanly excluding tsuku from auto-apply.

## Solution Architecture

### Overview

Two new files and two modifications compose the solution. The `self-update` command is a standalone cobra command that handles resolution, download, verification, and binary replacement in a single flow. The background check integration adds ~20 lines to the existing `checker.go`. No new packages are needed.

### Components

**`cmd/tsuku/cmd_self_update.go`** (new file)
- `selfUpdateCmd` cobra command registered in `main.go`
- `runSelfUpdate()` function containing the end-to-end flow:
  1. Acquire non-blocking file lock (`$TSUKU_HOME/cache/updates/.self-update.lock`)
  2. Resolve latest version via `GitHubProvider.ResolveLatest()`
  3. Normalize versions (strip `v` prefix), compare with semver -- exit if current or older
  4. Construct asset URL and checksums URL from tag
  5. Download and parse `checksums.txt` (hard error if missing or asset not found)
  6. Download binary to same-directory temp file via `httputil.NewClient()`
  7. Verify SHA256
  8. Copy permissions from current binary
  9. Two-rename replacement with rollback on failure
- No flags. Self-update always tracks latest stable.

**`internal/updates/self.go`** (new file)
- `const SelfToolName = "tsuku"` -- well-known constant
- `const SelfRepo = "tsukumogami/tsuku"` -- GitHub repository
- `checkSelf(ctx context.Context, cacheDir string, res *version.Resolver) *UpdateCheckEntry` -- resolves tsuku's latest version and returns a cache entry
- `IsSelfUpdate(entry *UpdateCheckEntry) bool` -- convenience check against `SelfToolName`

**`internal/updates/checker.go`** (modified)
- `RunUpdateCheck` gains a `checkSelf()` call after the tool iteration loop, gated on `userCfg.UpdatesSelfUpdate()`

**`internal/updates/apply.go`** (modified, from Feature 3)
- `MaybeAutoApply` gains a one-line skip: `if entry.Tool == SelfToolName { continue }`. This is defense-in-depth -- `checkSelf` already writes an empty `LatestWithinPin`, which the existing filter would skip. The explicit guard makes the exclusion visible and survives future changes to the filter logic.

**`cmd/tsuku/main.go`** (modified)
- `rootCmd.AddCommand(selfUpdateCmd)` added to command registration
- `self-update` added to the PersistentPreRun skip list (alongside `check-updates`, `hook-env`, etc.) to avoid redundant background check spawns during self-update

### Key Interfaces

```go
// Well-known constant identifying tsuku's own cache entry.
const SelfToolName = "tsuku"

// checkSelf resolves tsuku's latest version from GitHub releases.
// Returns nil if self-update checks are disabled or version can't be resolved.
func checkSelf(ctx context.Context, cacheDir string, res *version.Resolver) *UpdateCheckEntry

// IsSelfUpdate returns true if the cache entry represents tsuku itself.
func IsSelfUpdate(entry *UpdateCheckEntry) bool
```

The `selfUpdateCmd` doesn't expose a public API -- it's a cobra command with internal logic only.

### Data Flow

```
tsuku self-update
  |
  v
Acquire non-blocking lock ($TSUKU_HOME/cache/updates/.self-update.lock)
  |
  +-- lock held: "Another self-update is running" -> exit
  |
  +-- lock acquired:
        |
        v
      GitHubProvider.ResolveLatest("tsukumogami/tsuku")
        |
        v
      Normalize versions (strip "v" prefix), semver compare
        |
        +-- equal: "tsuku is already up to date (0.5.0)" -> exit
        +-- older: "Current version is newer than latest release" -> exit
        |
        +-- newer available:
        |
        v
      Construct URLs:
        binary: github.com/.../releases/download/v0.6.0/tsuku-linux-amd64
        checksums: github.com/.../releases/download/v0.6.0/checksums.txt
        |
        v
      Download checksums.txt, parse SHA256 for target asset
        |
        v
      exePath = os.Executable() -> filepath.EvalSymlinks()
      info = os.Stat(exePath)
      tmp = os.CreateTemp(filepath.Dir(exePath), ".tsuku-update-*")
        |
        v
      Download binary to tmp, verify SHA256
        |
        v
      os.Chmod(tmp, info.Mode())
      os.Remove(exePath + ".old")         // clean stale backup
      os.Rename(exePath, exePath + ".old") // backup current
      os.Rename(tmp, exePath)              // install new
        |
        +-- SUCCESS: "Updated tsuku from v0.5.0 to v0.6.0"
        |
        +-- FAILURE at final rename:
              os.Rename(exePath + ".old", exePath)  // restore backup
              os.Remove(tmp)                         // clean temp
              "Self-update failed: <error>. Current binary restored."
```

Background check integration (separate flow):

```
tsuku check-updates (background process)
  |
  v
RunUpdateCheck iterates installed tools (existing)
  |
  v
checkSelf(ctx, cacheDir, resolver)
  |
  v
Writes $TSUKU_HOME/cache/updates/tsuku.json
  {tool: "tsuku", active_version: "0.5.0", latest_overall: "0.6.0", ...}
  |
  v
tsuku outdated reads cache -> displays "tsuku v0.6.0 available"
MaybeAutoApply reads cache -> skips entry where Tool == SelfToolName
```

## Implementation Approach

### Phase 1: Cache integration

Add `checkSelf()` to the background checker and the `SelfToolName` constant. This enables `tsuku outdated` to report self-update availability immediately, even before the `self-update` command exists. Add the `MaybeAutoApply` skip guard.

Deliverables:
- `internal/updates/self.go` (new: constant, `checkSelf`, `IsSelfUpdate`)
- `internal/updates/self_test.go`
- `internal/updates/checker.go` (modified: call `checkSelf` after tool loop)
- `internal/updates/apply.go` (modified: skip `SelfToolName` in `MaybeAutoApply`)

### Phase 2: Self-update command

Add the `tsuku self-update` command with the full resolution, download, verification, and replacement flow. This is the user-facing feature.

Deliverables:
- `cmd/tsuku/cmd_self_update.go` (new: cobra command, `runSelfUpdate`)
- `cmd/tsuku/cmd_self_update_test.go`
- `cmd/tsuku/main.go` (modified: register command)

### Phase 3: Outdated display integration

Update `tsuku outdated` to display tsuku's own staleness from the cache entry, formatted distinctly from managed tools.

Deliverables:
- `cmd/tsuku/outdated.go` (modified: check for `SelfToolName` entry, format differently)

## Security Considerations

**Integrity verification.** Downloaded binaries are verified against SHA256 checksums from the release's `checksums.txt`. This detects corruption during transfer and CDN-level tampering. The checksum parser validates hash format (64 hex characters) and rejects malformed lines.

**Authenticity limitations.** The checksum file is hosted alongside the binary in the same GitHub release. If an attacker gains write access to the release (compromised maintainer credentials, CI pipeline injection), they can replace both artifacts and the checksum verification passes. This is the same trust model used by gh, rustup, cargo-binstall, and other self-updating CLIs that distribute via GitHub releases.

**Future hardening: cosign signatures.** GoReleaser supports Sigstore cosign signing with keyless OIDC-based identity. Adding cosign verification to the self-update flow would tie binary authenticity to the GitHub Actions build identity, not just the release's contents. This is tracked as a follow-up enhancement, not a launch blocker, because the current trust model matches industry standard practice.

**Downgrade protection.** The version comparison uses semver ordering, not just equality. If the resolved "latest" version is older than the running binary, self-update exits with no action. This prevents a compromised release from pointing "latest" at a known-vulnerable old version.

**Concurrent self-update.** A non-blocking file lock (`$TSUKU_HOME/cache/updates/.self-update.lock`) prevents two simultaneous `tsuku self-update` runs from corrupting the binary via competing rename sequences.

**Missing checksums.** If `checksums.txt` is absent from a release or doesn't contain an entry for the target binary, self-update aborts with a hard error. No binary is ever downloaded without a checksum to verify against.

**Transport security.** All downloads use HTTPS. Go's `net/http` client validates TLS certificates against the system trust store. No HTTP fallback is supported.

**Permission safety.** The temp file is created in the same directory as the target binary, so `os.CreateTemp` fails early if the user lacks write permission -- before any modification to the existing binary. The `.old` backup retains the original binary's permission bits via `os.Rename`.

**No data transmission.** The self-update command only makes GET requests to GitHub (API for version resolution, CDN for artifact download). No local state, tool list, or usage data is transmitted.

## Consequences

### Positive

- Users can update tsuku with a single command instead of re-running the installer
- Background checks surface self-update availability in `tsuku outdated` and future notifications
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
- The rename gap is sub-microsecond on modern filesystems and only affects a user-initiated command (not background processes). gh, rustup, and every major self-updating CLI accept this same risk.
- Permission errors are detected early (temp file creation fails before touching the current binary) with a clear error message suggesting `sudo` or relocation.
- The `.old` location is deterministic (`exePath + ".old"`) and discoverable via `tsuku self-update --help` documentation.
