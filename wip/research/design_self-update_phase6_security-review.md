# Security Review: self-update (Phase 6)

Review of the Phase 5 security analysis and the DESIGN-self-update.md document.

## 1. Attack Vectors Not Considered

### 1.1 Downgrade Attack

**Severity: Medium**

The design has no protection against version downgrade. If an attacker compromises a GitHub release and publishes an older, vulnerable version as "latest," the self-update mechanism will install it. `GitHubProvider.ResolveLatest()` trusts whatever GitHub returns as the latest release. The version comparison only checks "is the resolved version different from current" -- there is no enforcement that the resolved version is strictly newer.

The design says "compare against `buildinfo.Version()` -- if equal, report already up to date." But it does not address the case where the resolved version is older than the current version. A compromised release could set the latest tag to an old version with known vulnerabilities, and tsuku would happily "update" to it.

**Recommendation:** Add a semver comparison that rejects downgrades unless an explicit `--allow-downgrade` flag is passed. Log a warning if the resolved version is older than the running version.

### 1.2 Release Artifact Substitution via Draft/Pre-release Manipulation

**Severity: Low-Medium**

The design uses `GitHubProvider.ResolveLatest()` but doesn't specify how pre-releases and draft releases are filtered. If `ResolveLatest` does not properly exclude drafts and pre-releases, an attacker with write access could create a draft release with malicious binaries and manipulate it into appearing as the latest stable release.

The existing codebase handles this for managed tools, but the self-update path bypasses the recipe system entirely. The Phase 5 report doesn't examine whether the provider's filtering logic applies uniformly when used outside the recipe context.

**Recommendation:** Verify that `GitHubProvider.ResolveLatest()` explicitly filters out pre-release and draft releases when called for self-update. Document this assumption.

### 1.3 Binary Path Confusion via PATH Manipulation

**Severity: Low**

`os.Executable()` returns the path used to start the process, which may not be the canonical installation path. On some systems, if `PATH` is manipulated to point to a symlink farm or wrapper script, `filepath.EvalSymlinks()` may resolve to an unexpected location. An attacker with write access to a directory earlier in PATH could place a malicious `tsuku` wrapper that, when self-updated, gets replaced by the legitimate binary -- but the attacker's directory now contains a legitimate binary they can later replace again.

This is an edge case requiring prior local access, but it means self-update could write the new binary to a location the user doesn't expect.

**Recommendation:** After resolving the binary path, verify it lives in a sensible location (either `$TSUKU_HOME/bin/` or a well-known system path). Warn if the resolved path seems unusual.

### 1.4 Concurrent Self-Update Race

**Severity: Low**

The design doesn't mention any locking for the self-update operation itself. If a user runs `tsuku self-update` twice concurrently (e.g., from two terminal sessions), both processes could attempt the two-rename sequence simultaneously, leading to corruption. The managed tool path uses `state.json.lock` for concurrency control, but self-update operates outside that system.

**Recommendation:** Use a file lock (e.g., `$TSUKU_HOME/self-update.lock` or a lock adjacent to the binary) to prevent concurrent self-update operations.

### 1.5 Stale .old File as Attack Surface

**Severity: Low**

The `.old` backup persists indefinitely until the next self-update. If the `.old` file is from a version with known vulnerabilities, it remains on disk as an executable. An attacker with local access could invoke `tsuku.old` directly, or a confused script could reference it.

The Phase 5 report notes this but classifies it as "None" severity because permissions match the original binary. While true, the persistence of a known-vulnerable executable is a residual risk worth acknowledging.

**Recommendation:** Consider removing `.old` after a configurable grace period or after the user's next successful `tsuku` invocation. At minimum, document that `.old` files should be cleaned up periodically.

## 2. Assessment of Existing Mitigations

### 2.1 SHA256 Checksum Verification -- Adequate but Insufficient Alone

The Phase 5 report correctly identifies this as integrity-only verification. The mitigation is well-analyzed. The recommendation to add cosign signing is the right path forward.

**Gap:** The design doesn't specify what happens if `checksums.txt` is missing from the release. If the download silently proceeds without verification when the checksum file is absent, this is a significant weakness. The design should mandate that a missing `checksums.txt` is a hard failure.

### 2.2 HTTPS Transport Security -- Adequate

The Phase 5 assessment is correct. Go's TLS implementation is solid. Note that Go does not do certificate pinning (contrary to what the Phase 5 report implies on line 79: "HTTPS with certificate pinning in Go's standard library"). Go validates against the system trust store, which is standard and appropriate, but it's not certificate pinning. A compromised CA could still issue a valid certificate. This distinction doesn't change the severity assessment but the claim should be corrected.

### 2.3 Permission Safety -- Adequate

The early-failure design for permission errors is well thought out. The Phase 5 assessment is accurate.

### 2.4 Two-Rename Replacement -- Adequate with Caveats

The microsecond gap is acceptable as documented. The rollback logic (restore from `.old` on failure) is correct. However, the design doesn't address what happens if the process is killed (SIGKILL) between the two renames. In that case, the binary is at `exePath + ".old"` and nothing is at `exePath`. The user would need manual recovery.

This is the same risk accepted by gh, rustup, etc., and is adequately disclosed in the design's "Consequences" section.

## 3. "Not Applicable" Justification Review

The Phase 5 report does not explicitly use "Not Applicable" for any dimension -- it marks all four dimensions (External Artifact Handling, Permission Scope, Supply Chain, Data Exposure) as applicable. This is correct.

However, within the Permission Scope analysis, risks #1 and #4 are marked "Severity: None" with reasoning that amounts to "not applicable." Both are justified:

- **Temp file permissions before chmod (None):** Correct. The file starts more restrictive, not less.
- **Binary in root-owned directory (None by design):** Correct. Early, clean failure.

Within Data Exposure, risk #3 is "Severity: None" -- also justified. No local data is transmitted.

**No incorrectly dismissed risks found.**

## 4. Residual Risk Assessment

### Risks Requiring Escalation

**No risks require escalation to block launch.** The design's trust model matches industry standard (gh, rustup, cargo-binstall). The following risks should be tracked as post-launch hardening:

### Tracked Residual Risks

1. **Cosign signature verification (Medium):** Both the Phase 5 report and the design acknowledge this. It's correctly positioned as a follow-up enhancement. Should be the first post-launch security hardening item.

2. **Downgrade attack protection (Medium):** Not addressed in either document. Should be added to the design as an implementation requirement, not deferred to post-launch. This is a low-effort mitigation (a semver comparison) with meaningful security value.

3. **Missing checksums.txt handling (Medium):** The design should explicitly state that a missing `checksums.txt` is a hard error. This is a one-line implementation detail but important to codify.

4. **Concurrent self-update locking (Low):** Should be addressed in implementation. Low effort, prevents a class of corruption bugs.

## 5. Summary

The Phase 5 security analysis is thorough and well-structured. It correctly identifies the primary gap (checksum-only verification without signatures) and appropriately calibrates severity levels against industry baselines. The design's Security Considerations section (which was populated from the Phase 5 recommendations) is adequate for launch.

**Gaps in the Phase 5 analysis:**
- Downgrade attacks not considered
- Concurrent self-update race not considered
- Stale `.old` file persistence not considered
- Missing `checksums.txt` error handling not specified
- Incorrect claim about Go's "certificate pinning"

**Recommended pre-launch additions to the design:**
1. Reject version downgrades by default (semver comparison)
2. Hard-fail on missing `checksums.txt`
3. Add a file lock for self-update concurrency

**Recommended post-launch hardening:**
1. Cosign signature verification (already tracked)
2. Clean up `.old` files after grace period
3. Path sanity check on resolved binary location
