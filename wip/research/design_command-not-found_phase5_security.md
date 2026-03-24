# Security Review: command-not-found

## Dimension Analysis

### External Artifact Handling

**Applies:** Partially (install script only)

The runtime components of this design — hook files, the binary index, and `tsuku suggest` — handle no external artifacts at runtime. Hook files are static assets embedded in the Go binary via `embed.FS` and written to `$TSUKU_HOME/share/hooks/` during tsuku's own installation or upgrade. `tsuku suggest` is network-free and reads only the local SQLite file at `$TSUKU_HOME/cache/binary-index.db`.

The one point of exposure is `website/install.sh`, delivered via curl-pipe-sh. This is the same pattern used by rustup, nvm, and Homebrew, but it's worth naming the tradeoff clearly: the install script is executed without integrity verification by the user at the moment of download. A MITM attack or CDN compromise at install time would allow arbitrary code execution with the user's privileges. Mitigations already common in this pattern include serving over HTTPS with HSTS, publishing a checksummed release artifact that the install script validates before executing further, and advising users who require stronger guarantees to download and inspect the script manually.

Within this design specifically, `tsuku hook install` modifies rc files. The content written is a fixed source line pointing to a path under `$TSUKU_HOME` — no user-supplied data is included in what gets appended. This is not a separate external artifact risk; it's covered by the install-time trust already established when the user ran tsuku.

**Severity (install script only):** Medium. Standard for the pattern, well-understood, and mitigated by HTTPS delivery and optional checksum verification.

---

### Permission Scope

**Applies:** Yes

The feature touches three distinct permission surfaces:

**rc file writes (bash/zsh):** `tsuku hook install` appends a line to `~/.bashrc` or `~/.zshrc`. Writing to shell rc files is a meaningful permission: a bug or injection here would affect every subsequent shell session. The risk is bounded because the injected content is a static literal — it does not include any runtime variable that could shift post-injection. However, the implementation should guard against double-injection (idempotency) and use atomic writes (write to a temp file, then rename) to avoid partial writes that could corrupt the rc file.

**fish conf.d write:** Writing to `~/.config/fish/conf.d/tsuku.fish` is equivalent in scope to the rc file case. The same atomicity and idempotency concerns apply.

**No escalation risk:** The feature requires no sudo, no setuid, and no capabilities beyond what the running user already has. All writes are within the user's home directory.

**SQLite read:** `tsuku suggest` reads `$TSUKU_HOME/cache/binary-index.db`. This is a read-only operation on a file tsuku itself manages. SQLite parsing of attacker-controlled input would be a risk if the database came from an external source, but the binary index is built and written by tsuku — the trust boundary is the same as the tsuku binary itself.

**Severity:** Low overall. The rc file write is the highest-risk operation, and it's mitigated by the fixed content and the atomicity/idempotency controls noted above.

**Suggested mitigations:**

- Use atomic writes for rc file modifications (write to `.bashrc.tsuku-tmp`, then `os.Rename`).
- Check for the marker comment before appending (idempotency guard) to prevent duplicate entries across repeated `hook install` calls.
- `tsuku hook uninstall` should remove only the known marker block, not perform broad pattern replacement.

---

### Supply Chain or Dependency Trust

**Applies:** Yes (narrowly, for the eval construct)

The hook files are embedded in the binary — they are not fetched from a registry, CDN, or third party at runtime. Their contents are determined at build time, and the Go build is the supply chain trust boundary. This is the same trust level as the tsuku binary itself, so no new supply chain surface is introduced by the hook file shipping mechanism.

The one construct worth scrutiny is the `eval` in the bash hook:

```bash
eval "_tsuku_original_cnf_handle() $(declare -f command_not_found_handle | tail -n +2)"
```

The input to `eval` here is the shell's own serialization of a currently-defined function — produced by `declare -f`, which outputs a syntactically valid function body. This is not user input and not network input. However, it does mean that if a malicious earlier shell snippet defined `command_not_found_handle` with a crafted body designed to survive `declare -f` serialization and inject code when re-evaled, the tsuku hook would execute that injected code.

This is a realistic concern in environments where users source third-party shell configurations. The `declare -f` output is syntactically constrained by bash itself (it can't produce syntactically invalid shell), but a deliberately hostile function body could include subshell execution, redirects, or other effects that the `_tsuku_original_cnf_handle` wrapper would then run.

**Severity:** Low-to-Medium in hardened or shared environments. Low in typical single-user developer workstations.

**Suggested mitigations:**

- Document that the hook wraps any existing `command_not_found_handle`. Users in environments with strict shell security policies should audit what's in that handler before installing the hook.
- Consider whether the wrapping behavior is necessary for the common case. If most systems have no pre-existing handler, the `else` branch (simple definition without eval) covers the majority of installations. The eval branch could be made opt-in or removed if the use cases don't justify the complexity.
- If the eval branch is retained, add a comment in the hook file explaining why `declare -f` output is safe to eval (the reasoning is non-obvious and worth preserving for future maintainers).

---

### Data Exposure

**Applies:** Yes (limited)

**Command names as input:** Every unrecognized command the user types is passed to `tsuku suggest "$1"`. The command name is processed as a shell argument, not concatenated into a string, so there is no injection risk. However, the set of failed command lookups is behavioral data: it reveals what tools the user was looking for, what typos they make, and potentially what workflows they follow.

`tsuku suggest` is documented as network-free, reading only the local SQLite index. As long as this remains true — no telemetry, no reporting of unmatched commands — the data stays local. This is a design constraint worth making explicit in the implementation and enforcing in code review, because future contributors might reasonably think "failed lookups would be useful analytics" without recognizing the privacy implication.

**SQLite index contents:** The binary index maps command names to recipe names. This is derived from the public recipe registry and contains no user data.

**rc file modification:** The hook installation reads rc files to check for the marker and appends to them. The tool does not transmit rc file contents anywhere. The risk here is local: a bug in the rc file reader that logs or surfaces unexpected content. Standard care (don't log file contents, don't include rc file data in error messages) is sufficient.

**Severity:** Low. The design is network-free for the suggest path, which is the right call for a command-not-found handler that fires on every mistyped command.

**Suggested mitigations:**

- Add a comment or godoc note in the `tsuku suggest` implementation explicitly stating it must not make network calls. This makes the privacy constraint visible to future contributors.
- Ensure error messages from `tsuku suggest` don't echo back the full command string in contexts where it might be logged by the shell or a terminal session recorder. The human-readable output (`Command 'jq' not found. Install with: tsuku install jq`) already includes the command name, which is expected behavior — this is just a note to avoid adding verbose debug output that repeats the argument in unexpected places.

---

## Recommended Outcome

**OPTION 2 - Document considerations:**

The design is sound. The risks identified are either inherent to the curl-pipe-sh install pattern (well-understood and standard), or are implementation-level concerns that don't require design changes — they require care during coding. A Security Considerations section in the design document would capture the non-obvious points for implementers.

---

**Draft Security Considerations section:**

### Security Considerations

**Install script delivery.** The `website/install.sh` bootstrap script is delivered via curl-pipe-sh, the same pattern used by rustup and nvm. Users who require stronger guarantees can download the script and inspect it before execution. The install script should validate checksums of any artifacts it fetches before executing them.

**rc file writes.** `tsuku hook install` modifies `~/.bashrc` or `~/.zshrc`. Implementations must:
- Use atomic writes (write to a temp file, then rename) to prevent rc file corruption on interrupted writes.
- Check for the tsuku marker before appending (idempotency) so repeated `hook install` calls don't accumulate duplicate entries.
- On uninstall, remove only the known marker block.

**eval in bash hook.** The bash hook uses `eval` to preserve any existing `command_not_found_handle`. The input is `declare -f` output — the shell's own serialization of a currently-defined function — not user input. This is safe in typical environments. Users in hardened environments with strict shell security policies should audit their existing `command_not_found_handle` before installing the hook. A comment in the hook file should explain this reasoning for future maintainers.

**Command name privacy.** Every unrecognized command the user types is passed to `tsuku suggest` as a process argument. `tsuku suggest` must remain network-free: it reads only the local binary index and must not transmit command names or query results externally. This constraint should be enforced in code review and noted in the implementation.

**No privilege escalation.** This feature requires no sudo, no elevated capabilities, and no setuid. All operations are scoped to the current user's home directory.

---

## Summary

The design is appropriate for its threat model. The most significant risk — the curl-pipe-sh install script — is inherent to the delivery pattern and well-mitigated by HTTPS and checksum verification. The two runtime risks (eval of `declare -f` output, and the privacy implication of logging every mistyped command) are both low severity and addressed by implementation-level constraints rather than design changes. The recommended outcome is to document these considerations for implementers rather than change the architecture.
