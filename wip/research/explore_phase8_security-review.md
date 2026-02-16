# Security Review: Secrets Manager Design

**Reviewer:** Pragmatic Reviewer (Security Focus)
**Design:** `docs/designs/DESIGN-secrets-manager.md`
**Date:** 2026-02-16

---

## 1. Attack Vectors Not Considered

### 1.1 Secret Passed via CLI Argument (HIGH)

The design specifies `tsuku config set secrets.anthropic_api_key sk-ant-...` as the primary way to write secrets. This puts the secret value in:

- Shell history (`~/.bash_history`, `~/.zsh_history`) -- persists after reboot
- `/proc/<pid>/cmdline` -- visible to all users on the system while the command runs
- System audit logs (if enabled)

This is a well-known problem. Tools that handle this well (e.g., `gh auth login`, `docker login`, `aws configure`) read secrets from stdin or prompt interactively. The design mentions this nowhere.

**Recommendation:** The `tsuku config set secrets.*` command should read the value from stdin when it detects a `secrets.*` key, or at minimum offer a `--stdin` flag. This is more important than file permission enforcement since shell history is typically 0600 but retained indefinitely.

### 1.2 Secret Leakage via TOML Serialization Roundtrip

The design uses `toml.NewEncoder(f).Encode(c)` for writing. When the config struct gains a `Secrets map[string]string` field, the TOML encoder will write secrets as plaintext values. If a user runs `tsuku config set telemetry false` (a non-secret operation), the `Save()` function re-serializes the entire config including any previously stored secrets. This is correct behavior, but it means:

- Any bug in the Save() path that fails between write and rename could leave a partial file with secrets
- The design addresses this with atomic writes, which is correct

No additional action needed; the atomic write mitigates this. Noted for completeness.

### 1.3 Temp File Left Behind on Crash

The design mentions atomic writes (temp file + rename) but doesn't specify cleanup behavior if the process is killed (SIGKILL, OOM) between temp file creation and rename. The temp file would remain with 0600 permissions containing secrets.

**Current mitigation:** 0600 permissions on the temp file limit exposure. This is acceptable residual risk, but the implementation should use a temp file in the same directory (as noted in the design) and use a recognizable name pattern (e.g., `.config.toml.tmp`) so users can identify orphans.

### 1.4 GITHUB_TOKEN Scope Creep

The design lists `github_token` as a managed secret, but the codebase uses `GITHUB_TOKEN` in 6+ distinct locations with different trust requirements:

| Location | Usage | Trust Level |
|----------|-------|-------------|
| `internal/llm/factory.go` (via discover) | GitHub API for repo verification | Read-only public data |
| `internal/version/resolver.go` | Version resolution via GitHub API | Read-only public data |
| `internal/version/provider_tap.go` | Raw content fetch from GitHub | Read-only public data |
| `internal/builders/github_release.go` | Download release assets | Read-only public data |
| `internal/discover/validate.go` | Validate GitHub repos | Read-only public data |
| `internal/discover/llm_discovery.go` | HTTP GET for GitHub API | Read-only public data |

All of these only need public read access, but a stored `GITHUB_TOKEN` could have write/admin scopes. The design doesn't recommend scope guidance or validate token permissions. This isn't a blocking concern since the user controls what token they provide, but the error guidance message should recommend a read-only token.

### 1.5 Secrets in Debug/Verbose Output

The design says "No logging of secret values" in `Get()`, but the callers pass resolved secrets to SDK constructors (`anthropic.NewClient(option.WithAPIKey(apiKey))`). If any of these SDKs log the key in debug/verbose mode, the secret is exposed. The design can't control SDK behavior, but this is worth documenting as a known limitation.

### 1.6 Config File Backup Exposure

Many editors (vim, emacs, nano) and backup tools create copies of files being edited:
- `config.toml~` (vim backup)
- `#config.toml#` (emacs autosave)
- `config.toml.bak` (various tools)

These copies won't have 0600 permissions enforced by tsuku. Since the design recommends `tsuku config set` as the write interface (not manual editing), this is low risk. But if documentation ever suggests manual editing, this becomes relevant.

---

## 2. Mitigation Sufficiency Analysis

### 2.1 File Permission Enforcement: Adequate

The warn-on-read, enforce-on-write strategy is sound and pragmatic. The residual risk (brief window between first write and permission tightening for pre-existing files) is minimal because:
- The write itself sets 0600 atomically (new file via rename)
- The only window is a read of a pre-existing 0644 file that now contains secrets, which implies the user already wrote secrets to a 0644 file through some other mechanism

**One gap:** The design says "On read, check file permissions" but doesn't specify what happens when `config.toml` is a symlink. If a user symlinks `config.toml` to a file in a shared directory, `os.Stat()` follows the symlink and reports the target's permissions. The `os.Rename()` for atomic writes would replace the symlink, not the target. This is actually safer behavior (it breaks the symlink), but could surprise users. Low priority.

### 2.2 Atomic Writes: Adequate

Temp file + rename in the same directory is the standard pattern. The design correctly notes same-filesystem requirement for `os.Rename()`.

### 2.3 Secret Value Non-Logging: Adequate

The `Get()` function returns values without logging. The `tsuku config get secrets.*` shows `(set)` / `(not set)`. This is correct.

### 2.4 No Encryption at Rest: Acceptable

The design explicitly accepts this trade-off and compares to `~/.aws/credentials` and `~/.config/gh/hosts.yml`. This is the right call. Encryption at rest with a passphrase would add complexity disproportionate to the threat model (single-user workstation).

---

## 3. Residual Risk Assessment

### Risks That Should Be Documented (not escalated)

| Risk | Severity | Notes |
|------|----------|-------|
| Shell history exposure from CLI args | Medium | Should be mitigated by stdin reading (see 1.1) |
| Secret in process memory until GC | Low | Standard Go behavior, no practical exploit path |
| Orphan temp files after crash | Low | 0600 permissions limit exposure |
| SDK-level debug logging | Low | Outside design's control |

### Risks That Need Escalation

**None.** The threat model is appropriate for a single-user developer tool. The design correctly identifies that users wanting stronger protection (HSM, keychain, encrypted vault) should use environment variables sourced from external secret managers. This is the same approach used by AWS CLI, GitHub CLI, and similar tools.

---

## 4. "Not Applicable" Justification Review

### 4.1 "Download Verification: Not applicable" -- CORRECT

The secrets manager doesn't download anything. It reads env vars and a local file. This is genuinely not applicable.

### 4.2 "Supply Chain Risks: Not directly applicable" -- PARTIALLY CORRECT

The design says this isn't directly applicable but then discusses two supply chain risks (key leakage, process environment). This is the right approach: acknowledging the indirect relationship. However, there's a third indirect risk the design doesn't mention:

**Compromised config.toml via recipe action.** If a malicious recipe contains an action that modifies `$TSUKU_HOME/config.toml`, it could exfiltrate secrets by appending them to a different file or sending them over the network. This is really a recipe sandbox concern (not this design's problem), but the secrets manager makes the attack more valuable by centralizing secrets in a known location.

This doesn't change the design -- it's an inherent consequence of having a secrets file -- but it's worth noting in the security considerations section.

### 4.3 "Execution Isolation" -- ADEQUATE

The design correctly notes no elevated privileges are needed. All operations are within `$TSUKU_HOME`.

---

## 5. Permission Enforcement Approach Assessment

### 5.1 Overall Approach: Sound

The three-part strategy is well-designed:
1. New files: create with 0600 from the start
2. Reads: warn if permissive, don't modify
3. Writes: always enforce 0600

This follows the principle of least surprise. Reads are side-effect-free; writes are the mutation point.

### 5.2 Implementation Concern: os.Create() Still in Code

The current `saveToPath()` in `internal/userconfig/userconfig.go` (line 132) uses `os.Create(path)` which defaults to 0666 (modified by umask). The design says to switch to `os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY, 0600)` for the atomic write path. This is correct, but the implementation must ensure **every** write path goes through the new logic -- not just when secrets are present.

The design says "if the config contains any secrets, the file is created with `os.OpenFile(..., 0600)`". This means non-secret configs would still use the old 0644 path. The problem: a user could store secrets, then remove them, and the file would still have the old keys cached in the file content from before removal. The implementation should probably always use 0600 once any `[secrets]` section has ever been written, or better yet, always use 0600 regardless (simpler logic, negligible downside).

**Recommendation:** Always use 0600 for `config.toml`. The conditional logic ("only if secrets are present") adds complexity for no real benefit. No user needs other-readable access to their tsuku config.

### 5.3 Race Condition: Check-then-Act on Permissions

The design says "On read, check file permissions. If wider than 0600 and contains secrets, log a warning." This check-then-read has a TOCTOU (time-of-check-time-of-use) window where permissions could change between the stat and the read. In practice, this is irrelevant because:
- The attacker would already need filesystem access
- The warning is informational, not a security gate
- No security decision depends on this check

No action needed.

### 5.4 umask Interaction

`os.OpenFile(path, flags, 0600)` creates a file with mode `0600 & ~umask`. With a typical umask of 0022, this produces 0600 (correct). With a permissive umask of 0000, still 0600 (correct). With a restrictive umask of 0077, this produces 0600 (correct). The 0600 mode works correctly regardless of umask since it only sets owner bits.

Actually, that's not quite right. `0600 & ~0077 = 0600` -- yes, still correct because 0600 has no group/other bits to mask. So umask interaction is safe.

### 5.5 Windows Compatibility

The design is Linux/macOS focused (0600 permissions, `/proc` references). On Windows, file permissions work differently (ACLs). The design doesn't mention Windows, and tsuku appears to target Unix systems based on the codebase. This is acceptable as-is but should be noted if Windows support is ever considered.

---

## 6. Pragmatic Review Findings

### 6.1 The "Unknown Key" Fallback Is Unnecessary Scope (Advisory)

The design specifies that unknown keys (not in `knownKeys`) fall back to uppercasing the name and checking the env var. This is gold-plating. Every secret tsuku needs is known at compile time. Supporting arbitrary keys:
- Makes the API surface harder to reason about
- Creates an implicit contract where any string could be a valid key
- The `KnownKeys()` function becomes misleading (there are more valid keys than it returns)

**Recommendation:** Remove the unknown key fallback. If a caller passes an unknown key to `Get()`, return an error. This keeps the interface tight and avoids future confusion.

### 6.2 GITHUB_TOKEN Migration Scope Is Larger Than Acknowledged

The design lists 4 call sites to migrate, but the codebase has GITHUB_TOKEN in at least 6 non-test files:
- `internal/llm/factory.go` (via discovery, indirect)
- `internal/llm/claude.go` (line 22)
- `internal/llm/gemini.go` (line 24, 26)
- `internal/discover/llm_discovery.go` (line 798)
- `internal/discover/validate.go` (line 78)
- `internal/version/resolver.go` (line 80)
- `internal/version/provider_tap.go` (line 168)
- `internal/builders/github_release.go` (line 830)
- `internal/search/factory.go` (lines 17, 24, 35, 38)
- `cmd/tsuku/config.go` (line 153)

The Phase 3 migration will need to cover all of these, not just the 4 listed. This isn't a security issue but an underestimated scope issue.

### 6.3 The `IsSet()` Function Is a Minor YAGNI Concern (Advisory)

`IsSet(name string) bool` could be `Get(name) != ""` at every call site. Currently, the only use case listed is `factory.go` for provider detection. This is borderline -- having `IsSet()` does read slightly better than checking error returns. Keep it, but it's worth noting it's a convenience, not a necessity.

---

## 7. Summary of Recommendations

### Must Address Before Implementation

1. **Add stdin reading for `tsuku config set secrets.*`** -- Shell history exposure is a more practical attack vector than file permissions. Read secret values from stdin or via interactive prompt. (Section 1.1)

2. **Always use 0600 for config.toml** -- The conditional "only if secrets present" logic adds complexity for no benefit. Just always write 0600. (Section 5.2)

### Should Address (Advisory)

3. **Document shell history risk** in the security considerations section, even if stdin reading is implemented. Users should know the risk.

4. **Remove unknown key fallback** -- Every secret is known at compile time. Arbitrary key support is unnecessary scope. (Section 6.1)

5. **Update Phase 3 migration scope** -- The design lists 4 migration sites but the codebase has 8+ non-test files using `GITHUB_TOKEN`, `TAVILY_API_KEY`, or `BRAVE_API_KEY` directly. (Section 6.2)

6. **Add scope guidance for GITHUB_TOKEN** -- Error messages should recommend a read-only personal access token. (Section 1.4)

7. **Note recipe-action exfiltration risk** -- A malicious recipe could read `config.toml`. This isn't this design's problem to solve, but it should be documented as a known interaction. (Section 4.2)

### No Action Needed

- Temp file cleanup (0600 mitigates adequately)
- TOCTOU on permission check (informational only)
- Symlink behavior (safer by default)
- SDK debug logging (outside design scope)
- Windows compatibility (not a target platform)
- Encryption at rest (correctly scoped out)
