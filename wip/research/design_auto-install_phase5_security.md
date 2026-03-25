# Security Review: auto-install

## Dimension Analysis

### External Artifact Handling

**Applies:** Yes

The auto-install flow triggers the existing tsuku install pipeline, which downloads and executes external binaries on the user's behalf. This is the core risk surface that the design inherits rather than introduces.

**Risks:**

1. **Verification coverage gap (medium):** The download pipeline enforces HTTPS and supports checksum verification via `checksum_url` or `signature_url` with PGP fingerprint pinning. However, the design relies on individual recipes opting into verification. A recipe that omits `checksum_url` and `signature_url` produces only a plan-time warning (`"no upstream verification..."`), not a hard failure. In `auto` mode, a silently installed recipe with no upstream verification provides no integrity guarantee beyond transport security.

2. **TOCTOU on cached artifacts (low):** The download cache saves artifacts keyed by URL. In `auto` mode, a locally cached artifact could be used for installation without re-verifying its checksum if the cache was populated before upstream verification metadata existed for that recipe. The existing `DownloadAction` does re-verify from `checksum_url` on cache hit, but this depends on the recipe declaring `checksum_url` in the first place.

3. **Compressed response smuggling (low, largely mitigated):** The `doDownloadFile` implementation explicitly rejects compressed responses and sets `Accept-Encoding: identity`. This is already handled.

**Mitigations already present:** HTTPS enforcement, checksum URL verification, PGP signature support with fingerprint pinning, SSRF-protected HTTP client.

**Additional mitigation needed:** In `auto` mode specifically, recipes without `checksum_url` or `signature_url` should be treated as non-installable (hard block) rather than warned. The silent nature of `auto` removes the last human checkpoint, so verification must be mechanical. Document this requirement for the auto-install detailed design.

**Severity:** Medium for auto mode without verification enforcement; low for confirm mode where the user provides a final checkpoint.

#### Re-review

**Addressed.** The updated design explicitly states in the Security Gates section: "in `auto` mode, recipes without upstream verification (`checksum_url` or `signature_url`) are ineligible for silent install and fall back to `confirm`." This is also shown in the data flow diagram and in the Decision Outcome summary. The gate falls back to `confirm` rather than blocking outright, which is a reasonable UX choice — the user still gets a chance to install after seeing the verification gap. No further change needed on this finding.

---

### Permission Scope

**Applies:** Yes

**Risks:**

1. **`syscall.Exec` process replacement (medium):** The design uses `syscall.Exec` on Unix to replace the tsuku process with the installed tool after installation, preserving exit code fidelity. `syscall.Exec` inherits the full environment — including all environment variables, open file descriptors, and the process's effective UID/GID — from the calling tsuku process. If tsuku is running with elevated privileges (e.g., a user who set `TSUKU_HOME` to a system path, or a wrapper script that runs tsuku with sudo for some reason), the installed binary would inherit those privileges. The design does not document any privilege check before `syscall.Exec`.

   In practice, tsuku is designed to run without sudo and the risk is low in the normal case, but it warrants an explicit check: tsuku should refuse `syscall.Exec` if the effective UID is 0 (root), since no tool install should result in running an arbitrary binary as root.

2. **Filesystem writes are bounded to `$TSUKU_HOME` (low):** Install actions write to `$TSUKU_HOME/tools/` and the audit log writes to `$TSUKU_HOME/audit.log`. Both paths derive from the user-controlled `TSUKU_HOME` variable. If `TSUKU_HOME` is set to a path the user doesn't own or a sensitive location, writes could go to unintended places. However, this is a general tsuku concern, not specific to auto-install.

3. **Audit log file creation race (low):** The audit log is created with mode 0600 on first write. If the directory is world-writable (e.g., `/tmp/tsuku`), a race between directory creation and log creation could allow another process to create the log file first with different permissions. The design does not specify atomic creation. Mitigation: open with `O_CREATE|O_EXCL` on first creation, or create the parent directory with 0700.

4. **No network permission escalation:** The feature doesn't introduce new network access beyond what install already uses. The binary index lookup is local SQLite, matching the design's "offline lookup" decision driver.

**Additional mitigation needed:** Document that `Runner.Run` must check `os.Geteuid() == 0` and return an error before calling `syscall.Exec`. Add a note on audit log creation using `O_CREATE|O_EXCL` or equivalent to prevent the file permission race.

**Severity:** Medium for the `syscall.Exec` + elevated privilege combination; low for the audit log race.

#### Re-review

**Addressed (root guard).** The updated design adds `ExitForbidden = 14` to `exitcodes.go` and explicitly lists "Root guard: if `os.Geteuid() == 0`, return `ExitForbidden` — tsuku never execs as root" as the first security gate in `Runner.Run`. This is reflected in both the Security Gates section and the Decision Outcome summary.

**Partially addressed (audit log race).** The design specifies the audit log is "created on first write with mode 0600" but does not specify `O_CREATE|O_EXCL` or an atomic creation mechanism. This remains a low-severity gap. It is not a blocker for proceeding to implementation, but the implementation should use `O_CREATE|O_EXCL` or rely on a 0700 parent directory. This can be captured as an implementation note rather than a design change.

---

### Supply Chain or Dependency Trust

**Applies:** Yes

**Risks:**

1. **Registry as the sole trust anchor (high for auto mode):** Recipes are sourced from the tsuku registry, fetched over HTTPS and cached locally. The binary index is derived from this registry. In `confirm` mode, the user sees the recipe name before installation — a weak but real checkpoint. In `auto` mode, a compromised or malicious recipe that appears in the registry installs without any user review. The design references "existing recipe verification (checksums, signatures)" as a mitigation, but that only applies after the recipe itself is already fetched. If the registry metadata is tampered with (e.g., a recipe's `checksum_url` is changed to point to a malicious artifact whose checksum matches the new binary), the verification chain is broken at the source.

2. **Binary index as an indirection attack surface (medium):** The binary index maps command names to recipes. A recipe that declares a widely-used command name (e.g., `curl`, `git`) as one of its binaries would appear as a candidate when the user runs `tsuku run curl`. In `auto` mode, the first match from the index would be installed silently. The design mentions conflict resolution will be addressed in the Block 1 detailed design, but this gap is directly relevant to auto-install security.

3. **`ProjectVersionResolver` interface trust (low):** The `Runner.Run` signature accepts a `ProjectVersionResolver`. The design states this comes from `#1680` (project config). A malicious `tsuku.toml` in a cloned repo could specify a version constraint that resolves to a known-vulnerable or malicious version. However, the design explicitly requires consent for installs triggered from project config (`tsuku install` with no args requires explicit invocation), so this risk is bounded.

4. **Recipe freshness and stale cache (low):** The recipe cache has a default TTL of 24 hours with a 7-day stale fallback. If a recipe is updated to revoke a compromised version, a user's stale cache could still offer the old recipe. This is a general tsuku concern, but auto-install amplifies it by reducing friction.

**Additional mitigation needed:**
- The detailed auto-install design must specify that `auto` mode only proceeds when the matched recipe has upstream verification (checksum or signature). This is the single most important supply chain control.
- The binary index conflict resolution policy (Block 1) must be defined before `auto` mode is implemented. Without it, auto-install on an ambiguous command name is unsafe.
- Consider an allow-list mechanism for `auto` mode: only recipes that have been explicitly installed at least once (or that the user explicitly adds to a trusted list) can be auto-installed silently. First-time installs in `auto` mode could fall back to `confirm`.

**Severity:** High for `auto` mode without verification enforcement or index conflict resolution; medium for `confirm` mode.

#### Re-review

**Addressed (verification gate).** The verification gate finding is addressed — see External Artifact Handling re-review above.

**Addressed (binary index conflict gate).** The updated design explicitly adds: "in `auto` mode, a command that resolves to more than one recipe in the index falls back to `confirm` mode for this install." This appears in the Security Gates section, the Decision Outcome summary, and the data flow diagram. This directly closes the ambiguous-command-name attack surface for auto mode.

**Not addressed (allow-list for first-time installs).** The prior review suggested considering an allow-list so only previously-installed or explicitly-trusted recipes can be auto-installed silently. The updated design does not adopt this. The verification gate and conflict gate together provide a reasonable substitute: a recipe with upstream verification and a unique command name binding is meaningfully constrained. The allow-list suggestion was advisory, not a blocker, and its omission is acceptable given the two mechanical gates now in place.

---

### Data Exposure

**Applies:** Yes, partially

**Risks:**

1. **Audit log content (low):** The audit log records `recipe`, `version`, `mode`, and timestamp for each auto-install. This reveals the user's tooling choices and when they use certain tools. The log is written at 0600 (user-readable only), which is appropriate. However:
   - The log persists indefinitely. No rotation or size cap is specified. Over time it could accumulate a detailed record of tool usage.
   - If `$TSUKU_HOME` is on a shared or synced filesystem (e.g., NFS home, Dropbox), the log is exposed to those systems.
   - The design does not specify what happens to the log on `tsuku remove` — a user removing a tool might expect its install history to be cleared.

2. **Command arguments not logged (positive):** The audit log records only the recipe and version, not the command arguments passed to `tsuku run`. This is correct; arguments could contain secrets (tokens, passwords passed on the command line).

3. **No telemetry introduced:** The design adds no new network transmission of user data. The existing telemetry worker is a separate component and not modified here.

4. **Environment variable exposure via `syscall.Exec` (low):** The process environment is passed to the installed tool via `syscall.Exec`. If the user's environment contains secrets (e.g., `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`), the installed tool receives them. This is standard UNIX process behavior and not unique to tsuku, but users may not expect it when using `tsuku run` to try an unfamiliar tool.

**Additional mitigation needed:**
- Document in the design that the audit log has no rotation or retention policy and is the user's responsibility to manage. A future improvement would be log rotation (e.g., max 1MB, keep last 3 files).
- Add a note that environment variable inheritance via `syscall.Exec` is intentional but users should be aware when using `auto` mode with tools they haven't explicitly vetted.

**Severity:** Low. No novel data exposure; existing patterns apply.

#### Re-review

**No change.** Data exposure findings were low-severity and advisory. The updated design does not add audit log rotation policy or explicit `syscall.Exec` environment inheritance documentation. These remain minor documentation gaps but are not blockers. They are appropriate to capture in the Security Considerations section of the design doc.

---

### Consent Bypass Attacks

**Applies:** Yes

This dimension addresses whether an attacker can manipulate the mode resolution chain to substitute `auto` for the user's configured `confirm` or `suggest`.

**Mode resolution chain (highest to lowest priority):**
1. `--mode=<value>` CLI flag
2. `TSUKU_AUTO_INSTALL_MODE` environment variable
3. `auto_install_mode` in `$TSUKU_HOME/config.toml`
4. Default: `confirm`

**Risks:**

1. **Environment variable injection — `.envrc` / CI secrets (high):** The highest-priority mode source after the CLI flag is `TSUKU_AUTO_INSTALL_MODE`. A malicious `.envrc` file in a cloned repository (loaded automatically by `direnv` or similar tools) could set `TSUKU_AUTO_INSTALL_MODE=auto`. A user who clones a repository containing a `.envrc` and has `direnv` active would silently upgrade from their configured `confirm` mode to `auto`. This is not hypothetical — `.envrc` injection is a known attack vector for tools that consume environment variables. The design mentions sanitizing environment variables for shell hooks but does not address this for `tsuku run` itself.

2. **Config file manipulation (medium):** `$TSUKU_HOME/config.toml` is user-owned (0600 implied by directory structure). If an attacker has write access to `$TSUKU_HOME`, they already have full compromise. However, a misconfigured `$TSUKU_HOME` with world-writable permissions (e.g., set by a broken install script) could allow config injection. This is a general concern but the design should specify expected permissions for `$TSUKU_HOME/config.toml` (0600).

3. **CLI flag injection via argument smuggling (medium):** If `tsuku run` is invoked by a shell wrapper or script that constructs the command line from user-controlled input (e.g., a Makefile that calls `tsuku run $TOOL`), an attacker controlling `$TOOL` could inject `--mode=auto jq` as the tool name. The design specifies that the library has no TTY dependency and that `cmd_run.go` does the TTY check, but it does not specify argument sanitization. Standard practice is to use `--` to terminate option parsing before the command name, but the design does not explicitly require this.

4. **TSUKU_HOME manipulation (medium):** If an attacker can control `TSUKU_HOME`, they control which `config.toml` is read. A `.envrc` setting `TSUKU_HOME=/tmp/evil` with an `evil/config.toml` containing `auto_install_mode = "auto"` would silently enable auto mode. This also affects the audit log destination and recipe cache — a controlled `TSUKU_HOME` can point to a pre-poisoned registry cache.

5. **Command name as recipe selector (low, by design):** The design intentionally uses the command name to look up a recipe. A user running `tsuku run <malicious-name>` where `<malicious-name>` was suggested by an attacker (e.g., a phishing site) would install the matching recipe in auto mode. This is a social engineering vector, not a consent bypass, but it is amplified by auto mode.

**Mitigations already in design (partial):** The parent design (`DESIGN-shell-integration-building-blocks.md`) mentions sanitizing `TSUKU_HOME` and `TSUKU_REGISTRY_URL` in shell hooks. However, this is scoped to the shell hook context (Block 2), not to `tsuku run` itself.

**Additional mitigations needed:**

- **Critical:** The auto-install design must document that `TSUKU_AUTO_INSTALL_MODE` is explicitly excluded from environment inheritance for shell-hook-invoked contexts, or at minimum, that the variable is read but validated against the user's persistent configuration. The strongest option: in non-interactive contexts, `auto` via environment variable requires corroboration from the config file (env alone cannot escalate to `auto`). This prevents `.envrc` injection from bypassing the user's configured mode.
- **Important:** Specify that `$TSUKU_HOME/config.toml` must be owned by the current user and have mode 0600 before being read. If permissions are wrong, refuse to honor the config value and fall back to `confirm`.
- **Important:** The `tsuku run` command must use `--` before the command name in all subprocess invocations to prevent argument injection via the command name.
- **Low:** Document the `TSUKU_HOME` risk and recommend that users not allow repositories to override `TSUKU_HOME` via `.envrc`.

**Severity:** High for the `.envrc` injection path (environment variable overrides user config, silently enables auto); medium for config file and argument injection.

#### Re-review

**Addressed (env var escalation prevention).** The updated design adds an explicit restriction: "`TSUKU_AUTO_INSTALL_MODE=auto` via environment variable is only honoured when `auto_install_mode = "auto"` is also set in the persistent `$TSUKU_HOME/config.toml`, or when `--mode=auto` is passed explicitly as a flag. The environment variable alone cannot escalate from `confirm` to `auto`." This closes the `.envrc` injection path. The design also notes the env var can still downgrade mode, which is the safe direction.

**Addressed (config file permission enforcement).** The updated design adds the second security gate: "if `$TSUKU_HOME/config.toml` is not mode 0600 or not owned by the current user, log a warning and treat `auto_install_mode` as unset (effective mode falls back to `confirm`)."

**Partially addressed (argument smuggling).** The design states that "users should use `--` to separate tsuku flags from the target command's flags" and this guidance appears in both the solution architecture and the Consequences/Mitigations section. However, it remains a recommendation rather than an enforcement mechanism. Because cobra processes flags before positional arguments, the real risk is a user-constructed command line (e.g., a Makefile with `tsuku run $TOOL`) where `$TOOL` contains `--mode=auto somecommand`. This is mitigated somewhat by the env var escalation restriction (the attacker still needs to get `auto` into config), but it remains a low-severity gap. Acceptable at design level.

**Not addressed (TSUKU_HOME manipulation).** The design does not add explicit protections against `TSUKU_HOME` being overridden by a `.envrc`. This was low-severity and is acceptable. The Security Considerations section should document the risk.

---

## Recommended Outcome

**OPTION 2 — Findings addressed; Security Considerations section drafted below.**

All five blocking findings from the prior review have been incorporated into the updated design. The two high-severity issues (env var escalation and verification gate) and the three medium-severity issues (root execution guard, config permission enforcement, index conflict gate) are now explicit security gates in `Runner.Run`, documented in the Security Gates section, reflected in the data flow diagram, and summarised in the Decision Outcome. The design is ready to proceed to implementation.

Remaining low-severity gaps (audit log creation race, audit log retention policy, `syscall.Exec` environment inheritance documentation, `TSUKU_HOME` override risk) are advisory and do not require further design changes. They are captured in the Security Considerations section below.

---

## Security Considerations Section (for DESIGN-auto-install.md)

The following is a draft of the Security Considerations section to be appended to the design document before the Consequences section.

---

### Security Considerations

#### Security Gates

`Runner.Run` enforces four security gates before any install or exec proceeds. These are checked in order; the first failure aborts the operation.

**1. Root execution guard**

If `os.Geteuid() == 0`, `Runner.Run` returns `ExitForbidden` immediately. Tsuku never installs or execs a binary as root. Users running tsuku under `sudo` or as root will receive a clear error directing them to run as a non-root user. This applies to all modes, not just `auto`.

**2. Config file permission check**

Before honoring the `auto_install_mode` value from `$TSUKU_HOME/config.toml`, tsuku verifies that the file is owned by the current user and has mode 0600. If either check fails, tsuku logs a warning and treats `auto_install_mode` as unset, falling back to `confirm`. This prevents a world-writable or group-writable config from silently enabling auto mode.

**3. Verification gate (auto mode only)**

In `auto` mode, if the matched recipe has no `checksum_url` and no `signature_url`, tsuku falls back to `confirm` mode for that install. The silent nature of `auto` removes the last human checkpoint, so upstream artifact verification is required before any unattended install. The fallback to `confirm` gives the user an explicit opportunity to review the gap rather than blocking the install entirely.

**4. Conflict gate (auto mode only)**

In `auto` mode, if the binary index returns more than one recipe for the requested command, tsuku falls back to `confirm` mode for that install. Ambiguous command-to-recipe mappings require explicit user selection; auto mode cannot choose silently between competing recipes.

#### Environment Variable Escalation

`TSUKU_AUTO_INSTALL_MODE=auto` set in the environment is only honored when `auto_install_mode = "auto"` is also present in `$TSUKU_HOME/config.toml`, or when `--mode=auto` is passed explicitly on the command line. An environment variable alone cannot escalate from `confirm` to `auto`.

This prevents a malicious `.envrc` in a cloned repository — loaded automatically by direnv or similar tools — from silently upgrading the user's consent level. The env var can still downgrade mode in the safe direction (e.g., `auto` → `confirm`, `auto` → `suggest`) without corroboration.

#### Audit Log

Auto-mode installs are recorded in `$TSUKU_HOME/audit.log` as append-only NDJSON, with mode 0600 on creation. The log records recipe name, version, timestamp, and mode. Command arguments are intentionally excluded; they may contain secrets passed on the command line.

The audit log has no automatic rotation or retention policy. Users with long-running systems or heavy auto-install use should manage log size manually. A future `tsuku audit` command may add rotation support. If `$TSUKU_HOME` is on a shared or synced filesystem (NFS home directory, cloud sync folder), the log will be accessible to those systems.

#### `syscall.Exec` Environment Inheritance

On Unix, `syscall.Exec` replaces the tsuku process with the installed tool and passes the full environment to the new process. Any environment variables present in the tsuku process — including `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`, or similar credentials — will be visible to the installed tool.

This is standard UNIX process behavior and is intentional: tools installed and run via `tsuku run` should behave identically to tools run directly. Users should apply the same judgment to `tsuku run <tool>` as they would to running `<tool>` directly, particularly in `auto` mode where installation happens without a separate review step.

#### `TSUKU_HOME` Override Risk

If a repository's `.envrc` sets `TSUKU_HOME` to an attacker-controlled path, tsuku will read config, registry cache, and audit log from that path. Combined with a pre-crafted `config.toml` at that location, this could enable auto mode without the user's knowledge. Users should configure direnv to disallow overriding `TSUKU_HOME` from repository `.envrc` files, for example by setting `TSUKU_HOME` in the user-level direnv config (`~/.config/direnv/direnvrc`) rather than leaving it unset.

#### Out-of-Scope Threats

- **Registry compromise:** A compromised tsuku registry that serves malicious recipes or tampered `checksum_url` values would break the verification chain at its source. This is addressed by the registry's own integrity controls (HTTPS, signed releases) and is outside the scope of the auto-install design.
- **Social engineering:** A user can be persuaded to run `tsuku run <malicious-recipe>` just as they can be persuaded to run any command. Auto mode amplifies this by removing the install confirmation step, but the mitigation is user awareness rather than a design control.
- **Stale recipe cache:** The 24-hour TTL with 7-day stale fallback is a general tsuku concern. Auto-install inherits this behavior without modification.
