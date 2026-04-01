# Security Review: Self-Update Implementation

**Reviewer:** Architect (security focus)
**Files reviewed:**
- `internal/updates/self.go`
- `cmd/tsuku/cmd_self_update.go`
- `internal/httputil/client.go`
- `internal/install/filelock.go`
- `internal/userconfig/userconfig.go`
- `docs/designs/DESIGN-self-update.md`

---

## 1. Checksum Verification (SHA256, parsing, validation)

**Status: Sound**

The implementation correctly:
- Downloads `checksums.txt` before the binary (line 159-178)
- Applies `io.LimitReader` with 1MB cap to `checksums.txt` (line 174)
- Uses `io.TeeReader` to compute SHA256 while writing the binary (line 207), avoiding a second pass
- Validates hash format: exactly 64 characters, lowercase hex only (`isHexString`, lines 273-280)
- Normalizes hash to lowercase before comparison (line 262)
- Rejects lines with unexpected format (skips lines where `len(parts) != 2`, line 258)
- Returns hard error if asset not found in checksums (line 269)
- Returns hard error on hash format validation failure (line 264)

No issues found. The checksum-before-binary ordering is correct -- the expected hash is known before the binary download begins.

## 2. Binary Replacement Atomicity (two-rename with rollback)

**Status: Sound with accepted risk**

The replacement sequence (lines 232-243):
1. Remove stale `.old` backup (ignore error)
2. `Rename(exePath, exePath+".old")` -- backup current
3. `Rename(tmpPath, exePath)` -- install new

Rollback on step 3 failure (lines 238-241):
- `Rename(exePath+".old", exePath)` -- restore backup
- `Remove(tmpPath)` -- clean temp

The same-directory temp file (`os.CreateTemp(filepath.Dir(exePath), ...)` at line 199) guarantees same-filesystem, which guarantees `os.Rename` atomicity. This is correct by construction.

**Accepted risk:** The microsecond window between the two renames where no binary exists at `exePath`. This is documented in the design and matches the pattern used by gh, rustup, and similar tools.

## 3. Downgrade Protection

**Status: Sound**

`CompareSemver` (lines 284-309) performs numeric comparison of each dotted component. In `CheckAndApplySelf`, line 100-101: `if cmp >= 0 { return nil }` -- exits with no action if current >= latest. In `cmd_self_update.go`, lines 43-49: separate messages for equal and newer-than-latest cases.

The `strconv.Atoi` fallback to 0 on parse failure (lines 296, 299) means malformed version components are treated as 0, which is safe -- a malformed "latest" would compare as older than any real version.

**Note:** `CompareSemver` ignores pre-release metadata (anything after a hyphen within a component). However, `IsDevBuild` already gates out pre-release versions at line 56-58, so pre-release versions never reach the comparison. No issue.

## 4. File Lock for Concurrent Updates

**Status: Sound**

Both paths acquire a non-blocking exclusive lock:
- Background: `self.go` lines 110-118
- Manual: `cmd_self_update.go` lines 61-69

`TryLockExclusive` uses `flock(LOCK_EX|LOCK_NB)` on Unix (per `filelock_unix.go`). Non-blocking means a second concurrent update silently exits (background) or errors (manual). Lock is released via `defer`.

The lock file path is deterministic (`$TSUKU_HOME/cache/updates/.self-update.lock`), shared between background and manual paths.

## 5. TOCTOU Between Download and Replacement

**Status: No exploitable TOCTOU**

The sequence is:
1. Download checksums.txt
2. Parse expected hash
3. Download binary to temp file, computing hash during write
4. Compare hashes (line 215)
5. Set permissions (line 226)
6. Two-rename replacement (lines 232-243)

There is no window where a different process could swap the temp file between verification and rename, because:
- The temp file has a random name (`.tsuku-update-*` pattern)
- The file lock prevents concurrent self-updates
- Between hash verification (line 215) and first rename (line 233), only `os.Stat` and `os.Chmod` are called -- no re-read of the temp file

The temp file path is stored in a local variable, not derived from any shared state. No TOCTOU.

## 6. HTTP Client Usage

**Status: Sound**

Line 157: `client := httputil.NewSecureClient(httputil.ClientOptions{})` -- uses the codebase's hardened client with:
- SSRF protection on redirects (blocks private/loopback/link-local IPs)
- HTTPS-only redirect enforcement
- DNS rebinding protection
- Timeouts on dial, TLS handshake, response headers
- Compression disabled by default (decompression bomb protection)

Both the checksums request (line 160) and binary request (line 185) use `http.NewRequestWithContext`, propagating the caller's context for cancellation.

## 7. Temp File Cleanup on Error Paths

**Status: Sound**

Every error path after temp file creation cleans up:
- Write failure (line 209): `tmpFile.Close(); os.Remove(tmpPath)`
- Checksum mismatch (line 216): `os.Remove(tmpPath)`
- Stat failure (line 222): `os.Remove(tmpPath)`
- Chmod failure (line 227): `os.Remove(tmpPath)`
- Backup rename failure (line 234): `os.Remove(tmpPath)`
- Install rename failure (line 240): `os.Remove(tmpPath)` (plus rollback)

No leak paths identified.

## 8. io.LimitReader on checksums.txt

**Status: Applied correctly**

Line 174: `io.ReadAll(io.LimitReader(checksumsResp.Body, 1<<20))` -- 1MB limit. A checksums.txt file is typically a few hundred bytes (one line per asset). 1MB is generous but prevents memory exhaustion from a malicious server.

**Note:** There is no `io.LimitReader` on the binary download (line 207). This is acceptable -- the binary is expected to be ~10-20MB, and applying a reasonable limit (e.g., 200MB) would require knowing an upper bound. The 30-second HTTP client timeout provides the practical bound. Not a finding.

## 9. Permission Preservation

**Status: Sound**

Lines 220-229:
1. `os.Stat(exePath)` captures current binary's mode
2. `os.Chmod(tmpPath, info.Mode())` applies it to the new binary

This preserves execute bits and any special permissions. The temp file is created with default permissions from `os.CreateTemp` and then explicitly set, so there's no window where the installed binary has wrong permissions -- the chmod happens before the rename.

## 10. CI Suppression

**Status: Sound**

`UpdatesSelfUpdate()` in `userconfig.go` (lines 444-455) checks:
1. `TSUKU_NO_SELF_UPDATE=1` -- hard disable
2. `CI=true` (case-insensitive) -- CI environment detection
3. Config file `updates.self_update` -- user preference (default: true)

`CheckAndApplySelf` calls `userCfg.UpdatesSelfUpdate()` at line 66 and returns early if false.

The `self-update` command is also in the `PersistentPreRun` skip list (confirmed in `main.go` line 69), preventing the background checker from spawning during a manual self-update. This avoids the race of a background update competing with a manual update.

---

## Additional Observations

### No binary download size limit

The binary download at line 207 uses raw `io.Copy` with no size limit. A compromised release could serve an arbitrarily large file. The HTTP client's 30s timeout is the only bound. This is low risk in practice (GitHub CDN won't serve multi-GB files for a known release), and adding a limit would require maintaining a max-size constant. **Advisory** -- not blocking.

### Tag parameter in URL construction

Lines 154-155 construct URLs using `tag` from `provider.ResolveLatest()`. The tag comes from the GitHub API (releases endpoint), which returns whatever tag string was set on the release. A compromised release could set a tag containing path traversal characters (e.g., `../../../`). However, GitHub's release download URLs are routed through their CDN, and path components are URL-encoded by `fmt.Sprintf` when used in HTTP requests. Go's `net/http` will URL-encode the path. **No exploitable issue**, but worth noting.

### `.old` backup file not cleaned on success

After a successful self-update, the `.old` file is left on disk (only removed at the start of the *next* update, line 232). This is by design (documented in the design doc as "preserved until the next successful self-update"). Provides manual recovery path.

### No signature verification

As documented in the design's Security Considerations, the current model trusts GitHub releases (checksum file and binary from the same source). Cosign signing is planned as a follow-up. This is the standard trust model for GitHub-distributed CLI tools.
