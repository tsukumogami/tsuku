# Security Review: notification-system (Phase 6)

Review of the Phase 5 security analysis and the DESIGN-notification-system.md Security Considerations section.

## Review Scope

Evaluated against four questions:
1. Are there attack vectors not considered?
2. Are mitigations sufficient for identified risks?
3. Are any "not applicable" justifications actually applicable?
4. Is there residual risk that should be escalated?

---

## 1. Attack Vectors Not Considered

### 1.1 Symlink attacks on sentinel file (NEW - Low)

The `.notified` sentinel file is created in `$TSUKU_HOME/cache/updates/`. The design uses `os.Chtimes` or `os.WriteFile` to touch it. If an attacker with local access replaces `.notified` with a symlink pointing elsewhere (e.g., `~/.bashrc`), the touch operation would update the mtime of the target file. The `os.WriteFile` fallback path could overwrite the symlink target with empty content.

**Practical risk:** Low. Requires write access to `$TSUKU_HOME`, at which point the attacker can modify installed binaries directly. However, this is a class of bug that has historically led to CVEs in package managers, so it's worth noting.

**Recommendation:** Use `os.Lstat` before writing to detect symlinks, or use `os.OpenFile` with `O_NOFOLLOW` (platform-dependent). Alternatively, create the sentinel via atomic tmp+rename like the existing `WriteEntry` and `WriteNotice` patterns already do.

### 1.2 Tool name injection in filesystem paths (NEW - Low)

Tool names are derived from filenames (`strings.TrimSuffix(e.Name(), ".json")`) and also used to construct file paths in `WriteNotice`, `WriteEntry`, `ReadEntry`, etc. The existing code uses `filepath.Join(dir, toolName+".json")`. If a tool name contains path separators (`../`) or null bytes, this could write outside the intended directory.

Looking at the existing code: `ReadAllEntries` and `ReadAllNotices` iterate directory entries, so the names come from the filesystem. But `WriteNotice` and `WriteEntry` accept arbitrary strings. The auto-apply path passes `entry.Tool` which was read from a JSON file that tsuku itself wrote, but a corrupted or maliciously crafted cache file could contain `../../etc/something` as the tool name.

**Practical risk:** Low. The JSON files are written by tsuku, and `$TSUKU_HOME` is user-owned. The notification system doesn't write new files -- it reads notices and cache entries, then writes to stderr. The only filesystem write is the sentinel touch and `MarkShown` rewrite, both using tool names from directory listings (safe).

**Recommendation:** Not blocking. If the project ever adds path sanitization as a general practice, the notification renderer would inherit it.

### 1.3 Notification timing as an information side-channel (NEW - Negligible)

The timing and content of stderr notifications could theoretically reveal installed toolchain information to co-tenants on shared systems (CI runners, dev containers). The Phase 5 report addresses CI log exposure but doesn't consider terminal co-access scenarios.

**Practical risk:** Negligible. This is standard for any CLI tool. The information (tool names and versions) is not sensitive and is already visible via `tsuku list`, process arguments, and filesystem inspection.

### 1.4 Race conditions in sentinel mtime comparison (NEW - Low)

The design compares sentinel mtime to cache directory mtime using two separate `stat` calls. Between the two calls, a background check could write new cache entries, causing a TOCTOU race. Result: a notification might be suppressed (sentinel appears fresh relative to dir) or shown an extra time.

**Practical risk:** Negligible. The consequence is at most one missed or one duplicate notification. No security impact -- purely a UX timing issue.

---

## 2. Adequacy of Mitigations for Identified Risks

### 2.1 JSON file size (Phase 5: optional mitigation)

The Phase 5 report correctly identifies unbounded `json.Unmarshal` as a low-severity issue and suggests a 1 MB cap. This is adequate. The existing `ReadEntry`, `readNotice`, and `ReadAllEntries`/`ReadAllNotices` all use `os.ReadFile` which loads the entire file into memory.

**Assessment:** The optional mitigation is proportional. A 1 MB cap would be defensive without being necessary. The files are written by tsuku with `json.MarshalIndent` on small structs (UpdateCheckEntry has ~7 fields, Notice has 5 fields), so realistic file sizes are under 1 KB. An attacker who can write multi-megabyte files to `$TSUKU_HOME` can do worse things.

**Recommendation:** Accept the Phase 5 recommendation as-is. Implement the size cap if the project adopts a general hardening pass; don't prioritize it for this design.

### 2.2 Error message content exposure (Phase 5: optional mitigation)

The Phase 5 report notes that error messages from failed installs are displayed verbatim on stderr, and these could contain URLs with embedded tokens or filesystem paths revealing usernames. The mitigation suggests truncation to 200 characters.

**Assessment:** This is the most actionable finding in the Phase 5 report. Error messages from `installFn` flow through `fmt.Errorf("install %s@%s: %w", ...)` in `applyUpdate`, then into `Notice.Error`, then onto stderr. The error chain could include HTTP response bodies, filesystem paths, or environment-derived URLs.

Truncation alone doesn't solve the problem -- a token could appear in the first 200 characters. A more effective approach would be to sanitize URLs by stripping query parameters and authentication components before storing in the notice.

**Recommendation:** Upgrade from "optional, not blocking" to "implement before v1.0." URL sanitization in error messages is a better mitigation than truncation.

### 2.3 CI suppression gate (Phase 5: positive finding)

The Phase 5 report correctly identifies that the suppression gate prevents notification output from appearing in CI logs. The precedence order is sound: `TSUKU_AUTO_UPDATE=1` overriding CI detection is necessary and correctly placed highest.

**Assessment:** Adequate. One subtlety: the `CI` environment variable check should probably accept multiple truthy values or just check for non-empty, since some CI systems set `CI=1` rather than `CI=true`. Looking at the existing `UpdatesAutoApplyEnabled()` pattern would confirm. This isn't a security issue per se, but a gap in the suppression gate means CI logs get notification noise.

---

## 3. "Not Applicable" Justification Review

### 3.1 Supply Chain or Dependency Trust: marked N/A

**Phase 5 justification:** The design doesn't introduce new dependencies, download external artifacts, or verify signatures. It consumes data written by trusted tsuku processes.

**Review verdict: Correctly N/A.** The notification system is purely a display layer. The trust boundary is upstream in the background checker and version providers. The Phase 5 report correctly scopes this out by referencing the Feature 2 design.

One observation: the notification system trusts that files in `$TSUKU_HOME/notices/` and `$TSUKU_HOME/cache/updates/` were written by tsuku. If another process writes a crafted JSON file to these directories, the notification system would display the attacker-controlled tool name, version, and error message on stderr. This is a form of local privilege escalation (from "write access to user dir" to "control terminal output") but the Phase 5 report already addresses this implicitly by noting that write access to `$TSUKU_HOME` gives the attacker access to installed binaries.

**No change needed.**

### 3.2 Overall OPTION 3 (N/A) recommendation

**Phase 5 conclusion:** No design changes needed for security reasons.

**Review verdict: Agree.** The notification system has a minimal attack surface. It reads local files, formats text, and writes to stderr. The identified risks are all low severity and require prior local access to `$TSUKU_HOME`. The two optional mitigations (file size cap, error message sanitization) are good hardening practices but don't represent design-level security concerns.

---

## 4. Residual Risk Assessment

### Risks that should NOT be escalated

- **JSON parsing without size limits:** Low impact, requires local write access.
- **Sentinel symlink attack:** Low impact, requires local write access, mitigated by existing directory permissions.
- **Tool name path traversal:** Theoretical, doesn't apply to the notification read path.
- **TOCTOU in sentinel comparison:** UX issue only, no security consequence.

### Risks that merit tracking (not escalation)

- **Error message content exposure:** The error-to-stderr pipeline could leak sensitive URL components. This is pre-existing (the current `displayUnshownNotices` has the same issue) and not introduced by this design. However, since this design formalizes and extends the notification display, it's the right place to add URL sanitization. Track as a hardening item for the v1.0 milestone, not as a design blocker.

### Nothing requires escalation

The notification system operates within user-space on locally-generated data. All identified attack vectors require the attacker to already have write access to `$TSUKU_HOME`, at which point they can modify installed binaries directly. The notification system doesn't expand the attack surface beyond what already exists.

---

## Summary

The Phase 5 security analysis is thorough and reaches the correct conclusion. The "N/A with justification" recommendation is appropriate for a display layer over local data.

Four additional attack vectors were identified (symlink on sentinel, tool name path traversal in write paths, information side-channel via timing, and TOCTOU in sentinel comparison). All are low or negligible severity and don't change the overall assessment.

The strongest recommendation is to upgrade the error message sanitization from "optional" to "tracked hardening item." The current design pipes raw error strings to stderr, and these could contain URL tokens or sensitive path components. This pre-dates the notification design but gets formalized here.

No design changes are needed. No risk requires escalation.
