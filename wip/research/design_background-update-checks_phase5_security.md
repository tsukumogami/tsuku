# Security Review: background-update-checks

## Dimension Analysis

### External Artifact Handling

**Applies:** Yes

The background process queries version providers (GitHub API, PyPI, npm, crates.io, etc.) and writes structured JSON results to `$TSUKU_HOME/cache/updates/<toolname>.json`. No binaries are downloaded or executed during the check phase.

**Severity: Low.** The data flowing inward is version strings and metadata, not executable content. A malicious or compromised provider could return fabricated version numbers, but the check phase only stores them -- it doesn't act on them. The actual risk materializes in Feature 3 (auto-apply), which re-resolves download URLs and (for recipes with checksums) verifies integrity at install time.

One gap worth noting: the design stores version strings without any validation of their format. A provider returning a pathological version string (e.g., extremely long, containing path separators, or null bytes) could cause unexpected behavior when used as a filename component or compared later. The `ValidateRequested` function from Feature 1 covers the `Requested` field but there's no equivalent sanitization for `LatestWithinPin` or `LatestOverall` values received from providers. In practice, version providers return well-formed strings, and the downstream `ResolveVersion` call in Feature 3 would fail on garbage input, so the blast radius is limited to a confusing cache entry.

The atomic write pattern (temp file + rename) is correct for preventing partial reads of cache files. This is well-handled.

### Permission Scope

**Applies:** Yes

**Severity: Low.** The trigger runs inside the user's shell prompt hook and spawns a child process as the same user. All files are written to `$TSUKU_HOME/cache/updates/`, which lives under the user's home directory with standard permissions. The lock file at `$TSUKU_HOME/cache/updates/.lock` is created with mode 0644 (per the existing `filelock.go` implementation's `os.OpenFile` call). The sentinel `.last-check` presumably uses the same pattern.

No privilege escalation occurs. The background process runs as the same user with the same permissions as the shell. No setuid, no sudo, no elevated capabilities.

One minor observation: the lock file is created 0644 (world-readable). For a lock file this is harmless -- its content is irrelevant, only the flock state matters. But the per-tool cache files should also be created with 0644 or 0600. The design doesn't specify the permission mode for cache files. If they're created 0644 (the common Go default), other users on a shared system can read which tools and versions are installed. This is a minor information disclosure, not a privilege issue. Using 0600 would be slightly more defensive but isn't critical.

### Supply Chain or Dependency Trust

**Applies:** Yes

**Severity: Low-Medium.** The check phase trusts version numbers returned by upstream registries. A compromised registry could claim a malicious version is "latest," and the cache would faithfully store that claim. The design doc correctly notes that Feature 3 performs checksum verification at download time -- but only for recipes that define checksums.

The design doc's Security Considerations section acknowledges cache poisoning via local filesystem access but does not explicitly address the upstream trust question: what happens if GitHub API, PyPI, or npm returns a poisoned version listing? The answer is nuanced:

- For the check phase alone, the impact is limited to displaying incorrect "update available" notifications (Feature 5) or triggering unnecessary update attempts (Feature 3).
- Feature 3's checksum verification is the real defense, but the design doc should acknowledge that recipes without checksums have no verification layer at all. This isn't a new vulnerability introduced by this design -- `tsuku update` already trusts provider responses -- but background checks increase the attack surface by automating the query cadence.

The 10-second context deadline is a reasonable bound against slow or stalling providers, preventing a compromised endpoint from holding the process indefinitely.

### Data Exposure

**Applies:** Yes

**Severity: Low.** Two exposure vectors exist:

1. **Network exposure**: The background process sends HTTP requests to version providers, revealing which tools are installed and implicitly that tsuku is in use. This is equivalent to what `tsuku outdated` already does, but now it happens automatically on a recurring schedule. For users behind corporate proxies or in environments where outbound traffic is monitored, this creates a predictable fingerprint of the user's toolchain. The design doesn't mention any mechanism to batch or anonymize these requests.

2. **Local disk exposure**: Cache files in `$TSUKU_HOME/cache/updates/` list all installed tools with their current and available versions. On a shared system, if file permissions are too open, another user could enumerate the tools installed. The filenames themselves (`<toolname>.json`) reveal installed tools even without reading the file contents.

Neither vector is severe. The network traffic is the same as manual usage, just automated. The local files are under the user's home directory. The `TSUKU_NO_UPDATE_CHECK=1` kill switch provides a full opt-out, which is the right escape hatch for sensitive environments.

The design doc's Security Considerations section doesn't mention the data exposure dimension at all. This is a gap -- not because it's high severity, but because users in regulated or air-gapped environments need to know that background checks send network traffic and that the kill switch exists for this reason.

### Process Lifecycle

**Applies:** Yes

**Severity: Low.** The spawn protocol is:

1. Non-blocking flock probe to detect if a check is already running
2. Release the probe lock
3. `exec.Command(os.Args[0], "check-updates").Start()` to spawn a detached process
4. Parent returns immediately

The background process then acquires a blocking flock and runs until completion or the 10-second deadline.

**Race window between probe release and child lock acquisition.** Between step 2 (parent releases probe lock) and the child process acquiring its own blocking flock (step 4 in the data flow), another trigger could probe the lock, find it free, and spawn a second checker. The design's double-check (re-check sentinel freshness after acquiring the lock in the background process) mitigates wasted work but doesn't prevent the extra process spawn. In practice, the window is tiny (milliseconds) and the consequence is just a redundant process that exits quickly after losing the flock race or finding a fresh sentinel. This is not a security issue, just a minor inefficiency that's already implicitly handled.

**Environment inheritance.** The design doc correctly notes that the child inherits the parent's environment, including any secrets. Since the child is the same binary running as the same user, this isn't an escalation. However, if the user's environment contains `HTTP_PROXY` or `HTTPS_PROXY`, the background process will use them, which is the correct behavior. If the environment contains credentials for private registries (e.g., `NPM_TOKEN`), those would be used by the version provider queries. This is expected and necessary for the feature to work with private registries.

**Zombie/orphan processes.** The design uses `exec.Command().Start()` without `Wait()`, creating a child that the parent doesn't reap. On Unix, the child is reparented to init/systemd, which handles reaping. The 10-second timeout ensures the process doesn't run indefinitely. If the timeout context doesn't cleanly terminate provider HTTP calls, the process could linger with open connections. The design should ensure that the context cancellation propagates to HTTP clients (Go's `net/http` respects context cancellation by default, so this is likely fine in practice).

**No `TryLockExclusive` in existing filelock.go.** The current `FileLock` implementation only has blocking `LockShared()` and `LockExclusive()`. The design's trigger protocol requires a non-blocking try-lock (`LOCK_EX|LOCK_NB`). This means either the implementation adds `TryLockExclusive()` to `FileLock` or uses raw `syscall.Flock` directly. The design mentions this as an assumption. If implemented incorrectly (e.g., accidentally using a blocking lock in the trigger path), it would block the shell prompt, which is a usability issue rather than a security issue.

## Existing Security Considerations Assessment

The design doc's Security Considerations section covers four topics:

1. **Flock DoS vector** -- Correctly identified and bounded by same-user threat model.
2. **Cache poisoning via filesystem** -- Correctly identified, references Feature 1's design, notes checksum mitigation.
3. **Environment inheritance** -- Correctly identified as expected behavior.
4. **No network in trigger path** -- Correctly stated, good latency isolation.

**What's adequate:**
- The local filesystem threat model is well-reasoned. Same-user access means the attacker already has full control.
- The separation between trigger (no network) and checker (network) is clearly stated.
- The flock DoS analysis correctly concludes it's bounded by existing permissions.

**What's missing or could be strengthened:**
- No mention of upstream provider trust (a compromised registry returning fabricated versions). The Feature 1 design mentions cache poisoning from local access but not from network responses. This is the most notable gap.
- No mention of data exposure (network traffic revealing installed tools, cache files on disk enumerating the toolchain). Worth documenting for users in sensitive environments, even if low severity.
- No mention of the race window between probe release and child lock acquisition. This is minor but worth a sentence acknowledging it's harmless.
- No mention of cache file permission mode. A one-liner noting files should use standard user-only permissions (or explicitly choosing 0644 as acceptable) would close this.
- The checkpoint about recipes without checksums being unprotected even with Feature 3's verification deserves a clearer callout. The current text says "download-time checksum verification (for recipes that define checksums)" in passing, but the implication -- that recipes without checksums have zero integrity protection in the full check-then-apply pipeline -- should be more prominent.

## Recommended Outcome

**OPTION 2: Document considerations.**

No design changes are needed. The architecture is sound, the threat model is correctly scoped to same-user permissions, and the separation between check (metadata only) and apply (binary download with checksums) is the right boundary. The gaps identified are documentation-level: upstream provider trust, data exposure for sensitive environments, cache file permissions, and the checksum coverage caveat. Adding 3-4 sentences to the Security Considerations section would close all gaps.

## Summary

The design's security posture is appropriate for its scope. The check phase handles only version metadata (not binaries), runs as the same user with no privilege escalation, and isolates all network activity to the background process. The main gaps in the existing Security Considerations section are (1) no mention of upstream provider trust -- a compromised registry could inject false version claims that flow through to Feature 3, bounded only by checksum verification for recipes that define them -- and (2) no acknowledgment of data exposure from automated network queries and on-disk tool enumeration, which matters for users in regulated environments. These are documentation additions, not design changes.
