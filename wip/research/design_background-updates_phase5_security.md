# Security Review: background-updates

## Dimension Analysis

### External Artifact Handling
**Applies:** Yes

The design downloads and executes external tool binaries as part of the auto-apply step. `runInstallWithTelemetry` drives this through the existing recipe executor, which fetches artifacts from URLs specified in recipes.

**Risks:**
- The apply-updates subprocess runs recipe-driven installs without interactive user confirmation. If a recipe's download URL is compromised (e.g., a CDN supply chain compromise), the background process installs the malicious binary without the user seeing any install prompt.
- The design does not mention checksum re-validation at apply time. The `UpdateCheckEntry` fields include `LatestWithinPin` and `LatestOverall` but no pre-fetched digest. If the check-updates subprocess captured a checksum at check time, that checksum is not in the cache entry, so the apply subprocess fetches and validates at install time — which is correct. However, the gap between check time and apply time (potentially hours or across a network change) means the artifact fetched at apply time may differ from what was checked.

**Mitigations already present:** The recipe executor performs checksum verification. `isActionableError` in `apply.go` already treats checksum errors as high-priority, bypassing the failure-suppression threshold and surfacing them immediately. No change needed here.

**Severity:** Low — the existing checksum layer handles this adequately; the background context does not weaken it.

---

### Permission Scope
**Applies:** Yes

**Risks:**
- The apply-updates subprocess is launched with `os.Executable()` and runs as the same user. It writes to `$TSUKU_HOME` (tools/, state.json, notices/, cache/updates/), which is user-owned. No privilege escalation.
- `SysProcAttr{Setpgid:true}` detaches the process from the parent's process group. This prevents the subprocess from receiving SIGINT when the user presses Ctrl+C, which is the intended behavior. It does not grant any elevated permissions and does not change the effective UID/GID.
- The 3-second timeout for `DiscoverManifest` in `init()` constrains how long the process can block on network I/O during startup. This is a correctness fix, not a security concern per se, but it does reduce the window for a slow network adversary to stall startup indefinitely.

**Mitigations:** The file lock (`state.json.lock`) prevents concurrent installs from racing each other. The lock is released on subprocess exit via OS cleanup even if the process is killed.

**Severity:** Low — no new permission paths are introduced.

---

### Supply Chain or Dependency Trust
**Applies:** Yes (inherited, not new)

The design doesn't change how recipes are sourced or how tool artifacts are verified. Trust anchors remain: the embedded recipe registry, the central registry cache, and user-configured distributed sources.

**Risk specific to this design:** Auto-apply runs installs without the user initiating them. If a distributed registry (user-configured) serves a maliciously crafted recipe that passes validation, the background process will install it automatically on the next check cycle. The user never typed `tsuku install <tool>`.

This risk exists today with synchronous auto-apply but is slightly amplified in the background model because the operation is invisible and non-cancellable once spawned.

**Mitigation:** Auto-apply only processes tools already installed (entries derived from `state.json`), not net-new tools. A malicious recipe would need to replace an existing tool's recipe with a compromised version. This is a meaningful constraint. The risk is the same as for the synchronous path — the subprocess model doesn't make it worse.

**Severity:** Medium (existing risk, not introduced by this design). No design change needed; worth documenting.

---

### Data Exposure
**Applies:** Yes

The subprocess inherits the parent's environment, which may contain secrets (API keys, tokens). The design uses `cmd.Stdin = nil`, `cmd.Stdout = nil`, `cmd.Stderr = nil`, which correctly severs I/O channels, but the environment is not explicitly cleared.

**Risk:** If `exec.Command` inherits the full parent environment, any secrets in env vars are passed to the subprocess. For a same-user subprocess installing tools on behalf of that user, this is the same exposure level as the current synchronous path. The subprocess doesn't transmit env vars anywhere; it uses them only if recipe execution explicitly reads them (e.g., `$TSUKU_GITHUB_TOKEN`).

**Telemetry:** The apply subprocess calls `tc.SendUpdateOutcome(...)`. This transmits: tool name, old version, new version, outcome, and error classification. No credentials or file paths are transmitted. This matches the existing synchronous path.

**Severity:** Low — no new exposure compared to existing synchronous apply.

---

### Subprocess Spawning
**Applies:** Yes — this is the central new attack surface.

The spawn path is:
```go
binary, err := os.Executable()  // resolves /proc/self/exe or equivalent
cmd := exec.Command(binary, "apply-updates")
```

**Risk 1 — Binary path manipulation:** `os.Executable()` returns the path to the running binary. On Linux this resolves via `/proc/self/exe`, which is reliable. On macOS it uses `os.Args[0]` if `/proc` is unavailable, which can be spoofed by a parent process setting `argv[0]` to a different path. However, this risk applies equally to the existing `spawnChecker()` call for `check-updates` and is an OS-level concern, not specific to this design.

**Risk 2 — Argument injection:** The subcommand is the fixed string `"apply-updates"`. No user-controlled data is passed as arguments to the subprocess. This is correct: the subprocess reads its parameters from cache files in `$TSUKU_HOME/cache/updates/`, not from command-line arguments. There is no injection vector here.

**Risk 3 — Environment inheritance:** Covered under Data Exposure above. No new risk.

**Risk 4 — Double-spawn race:** `CheckAndSpawnUpdateCheck` uses a probe lock to deduplicate check spawns. The design should apply the same deduplication logic to `MaybeSpawnAutoApply`. Without it, rapid successive command invocations could spawn multiple apply-updates processes before any acquires `state.json.lock`. The lock inside the subprocess handles correctness (only one will proceed), but spawning many processes wastes resources and creates noise. This is a reliability concern more than a security concern, but a crafted tight loop of `tsuku list` calls could exhaust process slots.

**Recommendation:** `MaybeSpawnAutoApply` should use a probe lock (separate from the check-updates lock) to deduplicate spawns, mirroring `CheckAndSpawnUpdateCheck`. This is already implied by the design's reference to the "trigger.go pattern" but should be made explicit.

**Severity:** Low overall; the deduplication gap is worth a design note.

---

### State File Integrity
**Applies:** Yes — this is a material risk in the new design.

The apply-updates subprocess reads `$TSUKU_HOME/cache/updates/<tool>.json` to determine which tools to install and at what versions. In the synchronous path, these cache files are written by the check-updates subprocess (same user, same binary) and consumed immediately. In the background apply model, there is a window between when the cache file is written and when apply-updates reads it. A local attacker with write access to `$TSUKU_HOME` could craft or modify a cache entry to trigger installation of an arbitrary version string.

**Concrete attack scenario:** An attacker who has compromised another process running as the same user writes `$TSUKU_HOME/cache/updates/ripgrep.json` with `LatestWithinPin: "1.0.0-malicious"` and `Tool: "ripgrep"`. The next `tsuku list` invocation spawns apply-updates, which installs that version. If the recipe for `ripgrep@1.0.0-malicious` doesn't exist, the install fails silently. If the attacker also controls a distributed registry, the install can succeed.

**Existing mitigations:** File permissions on `$TSUKU_HOME` are 0755 (directory), so only the owning user can write. This makes the threat model "same-user compromise," which is already game-over for most purposes. The recipe executor validates checksums, preventing silent substitution of binaries.

**Gap:** The cache entries are not signed or authenticated. There is no HMAC or similar integrity marker. For the synchronous path this was acceptable; for a background path that runs unattended it warrants documentation.

**Recommendation:** Document that `$TSUKU_HOME` should not be world-writable (the code uses 0755, which is correct). Consider adding a note that `cache/updates/` file ownership is validated at read time (checking that the file owner matches the running user UID). This is a low-cost hardening step.

**Severity:** Low in practice (requires same-user code execution), but worth documenting as a threat model boundary.

---

### Notice File Integrity
**Applies:** Yes, but low severity.

Notice files at `$TSUKU_HOME/notices/<tool>.json` are read by `DisplayNotifications` in the next command invocation. A crafted notice file could display a misleading error message to the user (e.g., falsely claiming a checksum failure or a disk-full condition). This is a UI spoofing risk, not a code execution risk.

**Risk:** An attacker with write access to `$TSUKU_HOME/notices/` can plant a notice file with an arbitrary `Error` string. The next `tsuku` command will display this string to the user via `DisplayNotifications`. The string is displayed as text output, not interpreted as code or markup.

**Mitigations already present:** Notice content is treated as a display string only. There is no templating or command execution triggered by notice content. The `Tool` field is derived from the filename (not the JSON body) in `ReadAllNotices`, which iterates directory entries. So a crafted `Tool` field in JSON doesn't cause filesystem operations on unexpected paths.

**Recommendation:** The design's addition of a `Kind` field to `Notice` should use a closed enumeration (or at minimum validate at read time) to prevent a crafted `Kind` value from causing unexpected display behavior in future code paths. Document that notice content is untrusted display data.

**Severity:** Very low — no code execution path, cosmetic spoofing only.

---

### Process Isolation
**Applies:** Yes

**`SysProcAttr{Setpgid:true}`** creates a new process group for the subprocess. This has two effects relevant to security:

1. **Signal isolation:** SIGINT and SIGTERM sent to the parent's process group (e.g., Ctrl+C in terminal) do not propagate to the subprocess. This is intentional and correct. It does not introduce a new attack surface.

2. **Session membership:** The subprocess is not a session leader and does not acquire a controlling terminal. It cannot read from or write to the terminal. This is correct behavior for a background worker.

**Risk — orphaned processes:** If the apply-updates subprocess is blocked waiting on a network operation that never times out (e.g., a hung TCP connection to a recipe download URL), it runs indefinitely in the background. The design does not specify a maximum lifetime for the subprocess. A network adversary who can cause a connection to hang (rather than fail) can keep a background tsuku process alive indefinitely on the user's machine.

**Recommendation:** The apply-updates subcommand should set an overall execution deadline context (e.g., 5 minutes) that cancels all in-progress installs. This is separate from the per-request HTTP timeout that should already exist in the HTTP client.

**Severity:** Low to medium — the orphan risk is real but limited to the user's own machine.

---

## Recommended Outcome

OPTION 2 - Document considerations:

**Draft Security Considerations section:**

> **Security Considerations**
>
> The apply-updates subprocess runs with the same user permissions as the parent tsuku process. It reads install parameters from cache files in `$TSUKU_HOME/cache/updates/` and writes results to `$TSUKU_HOME/notices/`. Both directories are owned by the running user (mode 0755); world-writable `$TSUKU_HOME` configurations are unsupported.
>
> **Subprocess spawning:** The subprocess command is `tsuku apply-updates` with no user-controlled arguments. Install parameters are read from the cache directory, not passed on the command line, eliminating argument injection vectors. Spawn deduplication via a probe lock prevents multiple concurrent apply-updates processes.
>
> **Cache file integrity:** Cache entries are not cryptographically signed. They are trusted as same-user-written files under the assumption that `$TSUKU_HOME` is not writable by other users. A local attacker with write access to the cache directory could craft an entry to trigger installation of a specific version string; the recipe executor's checksum validation provides the last line of defense.
>
> **Notice file integrity:** Notice file content is rendered as display text only. No notice field is interpreted as code or used to construct filesystem paths. The `Kind` field is validated against a closed set of known values at read time.
>
> **Orphaned processes:** The apply-updates subprocess should be given an overall execution deadline (recommended: 5 minutes) to prevent indefinite hangs caused by stalled network connections. The per-request HTTP client timeout does not cover this case.
>
> **Telemetry:** The apply-updates subprocess may transmit telemetry events (tool name, version, outcome, error classification) consistent with the foreground install path. No credentials, file paths, or environment variables are included in telemetry payloads.

Two additional implementation recommendations for Phase 5:
1. Add a top-level context deadline to the apply-updates subcommand (e.g., 5 minutes).
2. Explicitly confirm that `MaybeSpawnAutoApply` uses a dedicated probe lock to deduplicate spawns (the design implies this via the trigger.go pattern but should be specified).

---

## Summary

The design is sound and introduces no new privilege escalation or injection vectors. The two items that should be addressed before shipping are: (1) a top-level execution deadline on the apply-updates subprocess to prevent indefinite hangs from stalled network connections, and (2) explicit spawn deduplication (probe lock) in `MaybeSpawnAutoApply` to prevent resource waste from rapid successive command invocations. All other dimensions are either well-mitigated by existing infrastructure (checksum validation, flock-based state protection) or are inherited risks unchanged from the current synchronous apply path.
