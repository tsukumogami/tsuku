# Security Review: Dev Environment Isolation Design

## Scope

Review of `docs/designs/DESIGN-dev-environment-isolation.md` focused on attack vectors, mitigation sufficiency, residual risk, and "not applicable" justification accuracy.

## Findings

### 1. Path Traversal in Environment Names (HIGH)

**Issue:** The design passes user-supplied environment names directly into `filepath.Join(tsukuHome, "envs", envName)`. There is no validation of `envName`. A malicious or careless input like `--env ../../` or `--env ../tools` would resolve outside the `envs/` directory, potentially overwriting the user's real `state.json`, tools, or other critical paths.

**Current mitigation:** None described. The design doc's pseudocode in `DefaultConfig()` uses `envName` directly.

**Recommendation:** Validate environment names against a strict allowlist pattern (e.g., `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`). Reject names containing path separators, dots-only names (`.`, `..`), or empty strings. After joining, verify the resolved path is still under `$TSUKU_HOME/envs/` using `filepath.Rel` or prefix checking on the cleaned absolute path.

**Severity:** High. This is exploitable via CLI flag or environment variable. In CI, a misconfigured `TSUKU_ENV` could silently corrupt the real installation.

### 2. Shared Download Cache -- Cross-Environment Poisoning via TOCTOU (MEDIUM)

**Issue:** The download cache is content-addressed by `sha256(url)`, not by content hash. Two environments share the same cache read-write. The design claims "a write to the cache from one environment is indistinguishable from a write by the parent. No new attack surface." This is technically true for the cache keying scheme, but the shared write access introduces a TOCTOU (time-of-check-time-of-use) window:

1. Environment A downloads a file, checksum passes, file is cached.
2. Between cache save and cache read by environment B, an attacker (or buggy recipe) replaces the `.data` file in the shared cache.
3. Environment B reads the poisoned file.

The existing `Check()` method verifies checksums when `expectedChecksum` is provided, which mitigates this for recipes that specify checksums. But if a recipe omits checksums (the `checksum` field is optional per the metadata schema), cached content is trusted based on URL hash alone.

**Current mitigation:** Checksum verification in `Check()` -- but only when the recipe provides a checksum. The `actualHash` field in metadata is stored but never verified on read.

**Recommendation:**
- On cache read, re-verify the `.data` file against the `actualHash` stored in metadata. This catches post-save tampering regardless of whether the recipe specifies a checksum.
- Consider making checksum mandatory for recipes (separate concern, but relevant here since shared cache amplifies the blast radius of a missing checksum).

**Severity:** Medium. Requires local file system access to exploit. The shared cache between environments means a compromised dev environment could poison downloads for the production environment if they share `$TSUKU_HOME`.

### 3. Symlink Check Conflict with Cache Sharing (MEDIUM)

**Issue:** The design proposes two cache-sharing mechanisms. The preferred approach (config-level override) avoids symlinks entirely. The fallback approach (symlinking `envs/<name>/cache/downloads` to `../../cache/downloads`) directly conflicts with the existing `containsSymlink()` security check in `download_cache.go`.

The design acknowledges this: "This requires relaxing the symlink check for paths within $TSUKU_HOME (a known-safe boundary)." But the claim that intra-`$TSUKU_HOME` symlinks are "known-safe" is flawed. If `$TSUKU_HOME` itself is set to a path containing symlinks (e.g., `/tmp/symlink-to-real-dir`), then "intra-$TSUKU_HOME" symlinks aren't necessarily safe. The boundary is only meaningful if `$TSUKU_HOME` resolves to a real path first.

**Current mitigation:** The design prefers the config-level approach, which avoids this entirely. Good.

**Recommendation:** If the symlink fallback is ever implemented, resolve `$TSUKU_HOME` to its real path (`filepath.EvalSymlinks`) before checking whether a symlink target is "within" it. Better yet, drop the symlink fallback from the design entirely since the config approach is cleaner.

**Severity:** Medium. Only relevant if the fallback mechanism is implemented. The preferred approach is sound.

### 4. Cache Permission Check Race Condition (LOW)

**Issue:** `validateCacheDirPermissions()` writes a test file to check writability, then checks permissions. The write-test-then-check pattern has a race: permissions could change between the test write and the `info.Mode().Perm()` check. Also, the test file `.tsuku-perm-check` could be pre-created by an attacker to influence the writability check.

This is pre-existing (not introduced by the design), but the shared cache amplifies it because more processes interact with the same cache directory.

**Current mitigation:** None specific to the race.

**Recommendation:** Use `os.Stat` directly instead of the probe-file approach. Check `info.Mode().Perm()` first, and handle read-only mounts via the error from actual cache operations rather than probing.

**Severity:** Low. Requires precise timing and local filesystem access.

### 5. Environment Cleanup and Stale State (LOW)

**Issue:** The design doesn't address what happens when `tsuku env clean` is run while another process is using that environment. Concurrent deletion of an active environment's `state.json` or lock file could cause data corruption or undefined behavior.

**Current mitigation:** Advisory file locking on `state.json` -- but `env clean` would need to respect the lock before deleting.

**Recommendation:** `env clean` should acquire the environment's state lock before deletion, or refuse to clean an environment with an active lock.

**Severity:** Low. Unlikely in practice but possible in CI with poor job orchestration.

### 6. "No New Attack Surface" Claim for Download Verification (REVIEW)

**Issue:** The Security Considerations section states: "No new attack surface" for download verification. This is mostly accurate but slightly misleading. The shared cache means a single poisoned cache entry now affects all environments, not just one. The blast radius increases even if the attack surface (the cache write path) doesn't change.

The distinction matters: if an attacker can write to the shared cache once (via any environment), every environment that reads that cache entry is affected. Previously, with separate `TSUKU_HOME` directories, cache poisoning was contained to one installation.

**Recommendation:** Acknowledge the increased blast radius in the Security Considerations section. The attack surface is unchanged but the impact of a successful attack is broader.

### 7. "Not Applicable" Assessment: Multi-User Isolation

**Issue:** The design explicitly scopes out "Multi-user isolation or security boundaries." This is correctly scoped out -- the feature is for single-user development workflows. However, the design should note that environments don't provide any security boundary between them. A malicious binary installed in env A has full read/write access to env B and the parent `$TSUKU_HOME`. This is stated for execution isolation but not for filesystem isolation.

**Recommendation:** Add a sentence in Security Considerations: "Environments share the same filesystem user. A tool installed in one environment can read or modify any other environment's files. Environments isolate state, not trust."

## Summary of Risk Assessment

| Finding | Severity | New to Design? | Action |
|---------|----------|-----------------|--------|
| Path traversal in env names | High | Yes | Must fix before implementation |
| Cache poisoning via TOCTOU | Medium | Amplified | Re-verify actualHash on read |
| Symlink fallback conflict | Medium | Yes | Drop fallback from design |
| Permission check race | Low | Pre-existing | Improve separately |
| Concurrent env cleanup | Low | Yes | Lock before delete |
| Blast radius understatement | Informational | Yes | Update security section |
| Filesystem isolation caveat | Informational | Yes | Update security section |

## Residual Risk

After addressing the high-severity path traversal issue, the remaining risks are acceptable for the stated audience (tsuku contributors and CI). The shared cache design is sound when checksums are present. The main residual risk is recipes without checksums -- but that's a pre-existing problem that this design amplifies rather than introduces.

No findings require escalation beyond the design review process. The path traversal finding should block implementation until addressed.
