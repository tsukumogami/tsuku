# Security Analysis: Self-Update Auto-Apply Revision

**Design**: DESIGN-self-update.md (Accepted)
**Focus**: Security implications of auto-apply vs manual-only self-update
**Reviewer**: Architect (security focus)

---

## 1. Background Binary Replacement: New Attack Surface vs Manual

### Finding: No new attack surface from auto-apply itself (Advisory)

The download-verify-replace sequence is identical whether triggered by the background checker or `tsuku self-update`. The code path (`applySelfUpdate`) is shared. The trust model -- SHA256 from `checksums.txt` hosted alongside the binary in the same GitHub release -- is the same either way.

What changes with auto-apply is the **timing and visibility** of execution:

- **Manual**: User initiates, sees output, can abort.
- **Background**: Runs in a detached process spawned by `PersistentPreRun`. No stdout/stderr to observe. User discovers the replacement after the fact via the `.self-update-applied` notification file.

This means a compromised release (attacker gains write access to the GitHub release) has a **shorter window to detection** under auto-apply. With manual-only, the user might never run `tsuku self-update` and never pull the bad binary. With auto-apply, the background process fetches it on the next invocation.

However, this is the same tradeoff Claude Code, rustup, and gh accept. The design correctly identifies this in the "Authenticity limitations" section and proposes cosign as future hardening. The current trust model matches industry standard.

**Verdict**: The auto-apply path doesn't introduce new attack surface at the protocol level. The reduced user visibility is an accepted tradeoff, not a vulnerability. The design is sound here.

### Finding: CI environment suppression gap (Blocking)

The `UpdatesAutoApplyEnabled()` method suppresses auto-apply in CI environments (`CI=true`) unless `TSUKU_AUTO_UPDATE=1` overrides. This prevents tool updates from breaking CI builds.

The `UpdatesSelfUpdate()` method has **no CI suppression**. The design says `checkAndApplySelf()` is gated on `userCfg.UpdatesSelfUpdate()`, which is a simple bool with no CI awareness.

This means: in a CI environment where `CI=true`, managed tool auto-apply is correctly suppressed, but tsuku will silently replace its own binary during the background check. A CI job could start with tsuku v0.5.0 and mid-pipeline a background process replaces it with v0.6.0. Subsequent `tsuku` invocations in the same pipeline use the new binary. This violates reproducibility expectations and could break CI if the new version has incompatible behavior.

**Location**: Design doc Decision 3, step 1 checks `userCfg.UpdatesSelfUpdate()`. Should additionally check CI suppression, matching `UpdatesAutoApplyEnabled()`'s behavior. Alternatively, `UpdatesSelfUpdate()` in `internal/userconfig/userconfig.go` should incorporate the same CI detection logic.

---

## 2. Notification File Tampering (.self-update-applied)

### Finding: Spoofable but low-impact (Advisory)

The `.self-update-applied` file lives at `$TSUKU_HOME/cache/updates/.self-update-applied`. It is:
- Written by the background process (same user)
- Read and deleted by `PersistentPreRun` (same user)
- Contains old and new version strings
- Used solely for a one-shot stderr message

**Spoofing scenario**: A malicious actor with write access to `$TSUKU_HOME/cache/updates/` creates this file with arbitrary version strings. On the next tsuku invocation, the user sees a fake "tsuku has been updated from v0.5.0 to v0.6.0" message.

**Impact**: Cosmetic only. The notification doesn't gate any behavior -- no command flow depends on it. The user sees a misleading message, but the actual binary is unchanged. The worst case is confusion ("I thought I updated, but `tsuku --version` still shows the old version").

**Threat model**: An attacker with write access to `$TSUKU_HOME` can do far worse (replace the binary directly, modify `state.json`, poison recipes). The notification file doesn't expand the attack surface beyond what `$TSUKU_HOME` write access already grants.

**Potential hardening** (not blocking): If the file included a hash of the new binary, `PersistentPreRun` could verify the running binary matches. This would detect spoofing but adds complexity for a low-value attack. Not worth it at this stage.

**Verdict**: The notification file is spoofable but the impact is cosmetic. The threat requires `$TSUKU_HOME` write access, which already implies full compromise of the tsuku installation. No design change needed.

---

## 3. Race Conditions: Background Replacement vs Foreground Execution

### Finding: Two-rename gap is acceptable (Advisory)

The design correctly identifies the microsecond gap between `rename(current, current.old)` and `rename(temp, current)` where no binary exists at the expected path. A concurrent `exec("tsuku")` during this window gets ENOENT. This is the same gap accepted by gh, rustup, and every other self-updating CLI using the two-rename pattern.

### Finding: Background process replaces binary of its own parent (Advisory)

The background checker (`tsuku check-updates`) is spawned by `PersistentPreRun` and runs as a child process (via `cmd.Start()` without `Wait()`). It resolves its own binary path via `os.Executable()` and replaces it. On Linux, the running parent process is unaffected because the kernel keeps the old inode open via the file descriptor.

One subtlety: the background process is *itself* the binary being replaced. `os.Executable()` in the background process returns the same path as the foreground. The background process will:
1. Rename itself (the running binary) to `.old` -- safe, kernel holds inode
2. Rename temp to the original path -- safe, new file gets new inode
3. Continue executing from the old inode until exit

This is correct behavior on Linux and macOS. No issue.

### Finding: Concurrent foreground install + background self-update (Advisory)

The foreground process runs `MaybeAutoApply` (which installs tool updates), while the background process runs `checkAndApplySelf` (which replaces the tsuku binary). These are independent: tool installation writes to `$TSUKU_HOME/tools/`, binary replacement writes to whatever directory the binary lives in. No shared state is modified by both.

The file lock (`.self-update.lock`) prevents concurrent self-update attempts. The state lock (`state.json.lock`) prevents concurrent tool installations. These lock scopes don't overlap, which is correct.

**Verdict**: No new race conditions beyond the accepted two-rename gap.

---

## 4. Config Opt-Out Sufficiency (updates.self_update = false)

### Finding: Opt-out is adequate for user control, but missing environment variable override (Advisory)

The existing update controls follow a layered pattern:
- `updates.enabled` -- master switch, overridden by `TSUKU_NO_UPDATE_CHECK=1`
- `updates.auto_apply` -- auto-apply for tools, overridden by `TSUKU_AUTO_UPDATE=1`, suppressed by `CI=true`
- `updates.self_update` -- self-update control, **no env var override, no CI suppression**

The config-level opt-out (`updates.self_update = false`) is sufficient for individual users who want manual control. However, the design breaks the precedent set by the other update controls:

1. **No environment variable**: There's no `TSUKU_NO_SELF_UPDATE=1` equivalent. System administrators or CI configurations that need to disable self-update across all users can't do so without modifying each user's config file. The other controls (`TSUKU_NO_UPDATE_CHECK`, `TSUKU_AUTO_UPDATE`) provide this escape hatch.

2. **No CI suppression**: As noted in finding 1 above, `CI=true` doesn't suppress self-update. This is inconsistent with `UpdatesAutoApplyEnabled()`.

**Recommendation**: Add a `TSUKU_NO_SELF_UPDATE=1` env var check to `UpdatesSelfUpdate()` and incorporate CI suppression. This aligns with the established pattern and provides the control surfaces operators expect.

---

## 5. Additional Security Observations

### Finding: Downgrade protection is solid

The design explicitly handles the case where the resolved "latest" is older than the running version (semver comparison exits with no action). This prevents an attacker who can manipulate the GitHub API response from forcing a downgrade to a known-vulnerable version. Well designed.

### Finding: Checksum-only verification is industry standard but worth noting

The `checksums.txt` and binary are served from the same origin (GitHub release). An attacker who compromises the release can replace both. The design acknowledges this and proposes cosign as future work. This is the right call -- it matches gh, rustup, cargo-binstall.

### Finding: No response size limit on checksums.txt download (Advisory)

The design says checksums.txt is downloaded using `httputil.NewSecureClient()`. There's no mention of a size limit on the response body. A malicious or corrupted checksums.txt could be arbitrarily large. The httputil client has a 30s timeout which provides some protection, but an explicit `io.LimitReader` (e.g., 1MB) would be a cheap defense against memory exhaustion. Low priority since the file is served from GitHub's CDN.

---

## Summary

| # | Finding | Level | Action |
|---|---------|-------|--------|
| 1 | CI environment suppression gap: `UpdatesSelfUpdate()` lacks `CI=true` suppression, unlike `UpdatesAutoApplyEnabled()` | **Blocking** | Add CI suppression to self-update check, matching the established pattern |
| 2 | Notification file spoofable but impact is cosmetic | Advisory | No change needed |
| 3 | Two-rename gap and background replacement are standard, no new race conditions | Advisory | No change needed |
| 4 | No env var override for self-update (breaks precedent from other update controls) | Advisory | Add `TSUKU_NO_SELF_UPDATE=1` env var support |
| 5 | No response size limit on checksums.txt download | Advisory | Add `io.LimitReader` during implementation |
