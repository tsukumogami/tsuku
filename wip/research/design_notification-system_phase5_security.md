# Security Review: notification-system

## Dimension Analysis

### External Artifact Handling
**Applies:** Yes -- low severity

The notification system reads JSON files from two local directories: `$TSUKU_HOME/notices/<tool>.json` (failure notices) and `$TSUKU_HOME/cache/updates/<tool>.json` (update check entries). These files are written by tsuku itself during background update checks and auto-apply operations. The design does not download, fetch, or process any external network inputs directly.

However, the JSON files are read via `json.Unmarshal` without size limits. A malformed or adversarially large file in `$TSUKU_HOME/notices/` or `$TSUKU_HOME/cache/updates/` could cause excessive memory allocation during parsing. The practical risk is low: these directories are user-writable only (0755), and an attacker with write access to `$TSUKU_HOME` already has the ability to modify installed binaries directly.

The tool names derived from filenames (via `strings.TrimSuffix(e.Name(), ".json")`) flow into `fmt.Fprintf` format strings as `%s` arguments, not as format verbs. This is safe against format string injection.

**Severity:** Low. The input files are locally generated and stored in user-owned directories.

**Mitigations (optional, not blocking):**
- Consider adding a maximum file size check (e.g., 1 MB) before `json.Unmarshal` in `ReadEntry` and `readNotice` to guard against accidental corruption causing OOM.

### Permission Scope
**Applies:** Yes -- low severity

The notification system operates entirely within `$TSUKU_HOME`, a user-owned directory. It requires:

- **Filesystem reads:** `$TSUKU_HOME/notices/*.json`, `$TSUKU_HOME/cache/updates/*.json`, `$TSUKU_HOME/cache/updates/.notified` (sentinel stat).
- **Filesystem writes:** `$TSUKU_HOME/cache/updates/.notified` (touch sentinel), notice files (mark shown via rewrite).
- **stderr writes:** Notification output.
- **Environment variable reads:** `TSUKU_AUTO_UPDATE`, `TSUKU_NO_UPDATE_CHECK`, `CI`.
- **stdout TTY check:** Via `progress.IsTerminalFunc`.

No network access, no process spawning, no privilege escalation. The sentinel file `.notified` is created with mode 0644 in an existing directory. The `MarkShown` operation rewrites notice files via atomic rename (tmp file then rename), which is the existing pattern.

**Severity:** Low. All operations are confined to user-space directories with no escalation path.

### Supply Chain or Dependency Trust
**Applies:** No

This design does not introduce new dependencies, download external artifacts, or verify signatures. It consumes data structures that were already written by trusted tsuku processes (the background checker and auto-apply system). The notification rendering is purely a display layer over existing local data.

The upstream trust questions -- whether the background checker correctly validates version information from GitHub/PyPI/etc. -- are addressed in the background-update-checks design (Feature 2), not here.

### Data Exposure
**Applies:** Yes -- low severity

The notification system writes to stderr the following information:

- **Tool names and versions:** e.g., "Updated node 20.14.0 -> 20.15.0" or "Update failed: node -> 20.15.0: <error message>".
- **Aggregated update counts:** "5 updates available."
- **Error messages from failed installs.**

This information is not sensitive -- it describes which developer tools are installed and their versions. It does not include file paths, environment variables, credentials, or system information beyond what's already visible via `tsuku list`.

The suppression gate correctly prevents this output from leaking into CI logs when `CI=true` (unless explicitly overridden with `TSUKU_AUTO_UPDATE=1`). This is good practice -- CI logs are often stored and shared more broadly than terminal output.

One consideration: error messages from failed installs (`n.Error`) are displayed verbatim. If an install failure includes a URL with an embedded token or a filesystem path that reveals username, that would be printed to stderr. This is an existing concern in `displayUnshownNotices` and not new to this design.

**Severity:** Low. The data exposed is tool metadata, not credentials or PII.

**Mitigations (optional, not blocking):**
- Truncate error messages displayed in notifications to a reasonable length (e.g., 200 chars) and direct users to `tsuku notices` for full details. This limits accidental exposure from verbose error output.

## Recommended Outcome

**OPTION 3 - N/A with justification:** The notification system is a display layer that reads locally-generated JSON files from user-owned directories and writes formatted text to stderr. It introduces no new external inputs, no network access, no privilege changes, and no sensitive data handling. The two optional mitigations (file size cap on JSON reads, error message truncation) are minor hardening opportunities, not design-level concerns. No design changes are needed for security reasons.

## Summary

The notification system has a minimal security surface. It reads JSON files that tsuku itself wrote to user-owned directories, formats them as human-readable text, and writes to stderr. The suppression gate correctly prevents output leakage in CI environments. No external inputs are processed, no network calls are made, and no privilege escalation is possible. The design is sound from a security perspective.
