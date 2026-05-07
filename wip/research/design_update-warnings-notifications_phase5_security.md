# Security Review: update-warnings-notifications

## Dimension Analysis

### External Artifact Handling
**Applies:** Partially

The design itself does not introduce new download paths. `InboxReporter` only accumulates text strings and writes JSON to a local file. The version-fallback detection in `GitHubArchiveAction.Decompose` calls the already-existing `FetchReleaseAssets` and `ListGitHubVersions` APIs — both of which were in use before this design. No new external artifact downloads are added.

However, one secondary concern is worth noting: the fallback detection adds a retry against the GitHub API (`ListGitHubVersions`) to find a prior version with a matching asset. The retry logic consumes additional external API data at plan generation time (inside `Decompose`). The API responses are already treated as untrusted data (version strings are validated before use), so this is low severity and requires no structural change.

The `version_fallback:` prefix embedded in the Warn message is used by `InboxReporter.Stop()` as a signal to escalate `Kind`. This string originates from the internal codebase (`GitHubArchiveAction.Decompose`), not from GitHub API response data. The prefix check is correct as long as external strings (asset names, release tag names from GitHub) are never passed verbatim as the format string to `Warn()`. The existing code uses the format-string form (`reporter.Warn("version_fallback: installed %s instead of %s...", fallback, requested)`) which is correct. No risk here, but it's worth calling out for implementers.

**Severity:** Low. No new attack surface on artifact downloads.

---

### Permission Scope
**Applies:** Yes

This is the highest-relevance dimension for this design.

**Filesystem write in `$TSUKU_HOME/notices/`**

`WriteNotice` uses `filepath.Join(noticesDir, notice.Tool+".json")` to construct the output path. The `notice.Tool` value originates from `UpdateCheckEntry.Tool`, which comes from the cache directory filenames written by the `check-updates` subprocess. Those filenames are set from recipe names at the time of the check (e.g., `entry.Tool = "gh"`). Recipe names in the registry are kebab-case, validated when recipes are added.

However, there is currently no explicit sanitization of `notice.Tool` inside `WriteNotice` itself. If a malformed entry were somehow written to the update cache (e.g., `tool = "../bin/evil"`), `filepath.Join` would resolve it, potentially writing outside `$TSUKU_HOME/notices/`. This is a pre-existing concern that the design inherits and amplifies: `InboxReporter.Stop()` is a new write path that didn't exist before, using the same `notice.Tool` value. The design should verify, or the implementation should add, a containment check on `notice.Tool` before constructing the path in `WriteNotice`.

Concrete recommendation: validate that `notice.Tool` contains no path separators (`/`, `\`) and no `..` components before constructing the path in `WriteNotice`. This is a one-line addition: `if strings.ContainsAny(notice.Tool, "/\\") || notice.Tool == ".." { return fmt.Errorf("invalid tool name: %q", notice.Tool) }`.

**Background subprocess writing to disk**

The `apply-updates` subprocess runs as the current user, which is correct. `InboxReporter.Stop()` calls `notices.WriteNotice()` which uses `os.MkdirAll(noticesDir, 0755)` and writes files with `0644`. These are appropriate permissions for a user-owned directory. No privilege escalation is possible.

**`Messages []string` field size**

The `InboxReporter` accumulates all `Warn`/`DeferWarn` calls in memory and writes them to disk on `Stop()`. There is no bound on the number or length of messages. An install that emits many warnings (e.g., via a recipe defect or a high-retry fallback loop) could write an unbounded JSON file to `$TSUKU_HOME/notices/`. This is low severity (the directory is user-owned and disk use from a single tool notice is unlikely to be large in practice), but a cap (e.g., 50 messages, each truncated to 512 characters) would be prudent for production hardening.

**Severity:** Medium for the path traversal gap in `WriteNotice`; Low for the unbounded message accumulation.

---

### Supply Chain or Dependency Trust
**Applies:** No

This design does not introduce new package dependencies, build steps, or external artifact fetching paths. All new code is within the existing Go module. The version-fallback retry is against the GitHub Releases API, which was already trusted before this change. No new trust boundaries are crossed.

The `InboxReporter` is purely in-process: it accumulates strings from reporter call sites within the install engine and writes JSON. Its trust model is identical to other `progress.Reporter` implementations.

---

### Data Exposure
**Applies:** Yes, minor

**What is written to disk**

`$TSUKU_HOME/notices/<tool>.json` will now contain a `Messages []string` array. These messages are produced by `Warn()` call sites in the install engine. Today those call sites emit information like version numbers, asset names, shell names, and install paths. None of these are secrets, but install paths (`$TSUKU_HOME/tools/...`) and asset names are user-specific system information.

The file is written with `0644` permissions (readable by any process running as the same user, and readable by other users on multi-user systems where `$TSUKU_HOME` is under the user's home directory with default umask). This is consistent with the existing notice files and is not a regression.

**API tokens**

The existing `progress.Reporter` documentation in `reporter.go` includes an explicit security notice: callers must not pass values from `internal/secrets/` to any Reporter method. The new `InboxReporter` inherits this requirement. The design does not discuss this constraint; the implementation must preserve it. Since `GitHubArchiveAction.Decompose` only passes version strings and asset names (never tokens) to `Warn()`, this requirement is satisfied in the described implementation.

**`warnShellInitChanges` migration**

The design migrates `warnShellInitChanges` from `fmt.Fprintf(os.Stderr, ...)` to `reporter.Warn(...)`. Shell init change messages describe file paths and shell names — no sensitive content. The migration is safe.

**ANSI injection**

All existing `progress.Reporter` implementations pass output through `SanitizeDisplayString()` before writing to terminal or accumulating. `InboxReporter` writes to a JSON file, not a terminal. If `InboxReporter` skips the sanitization pass (since its output never hits a terminal directly), ANSI sequences from recipe-generated strings could end up stored verbatim in the notice file and then rendered unsanitized when `renderUnshownNotices` passes them to `fmt.Fprintf(os.Stderr, ...)`. The implementation should apply `SanitizeDisplayString` to messages in `InboxReporter` before accumulation, or at minimum before writing to disk.

**Severity:** Low for data exposure; Low-Medium for the ANSI injection vector through the notice file render path.

---

## Recommended Outcome

**OPTION 2 - Document considerations:**

The design is sound and the architecture is appropriate. Two gaps require implementer attention:

**1. Tool name path validation in `WriteNotice`** (Medium)

`WriteNotice` uses `notice.Tool` to construct a filename without validating for path separators. Add an explicit check before the `filepath.Join` call:

```go
if strings.ContainsAny(notice.Tool, "/\\") || notice.Tool == ".." {
    return fmt.Errorf("invalid tool name for notice path: %q", notice.Tool)
}
```

This closes a theoretical path traversal window that the new `InboxReporter` write path inherits.

**2. ANSI sanitization in `InboxReporter`** (Low-Medium)

Call `progress.SanitizeDisplayString` on each message in `InboxReporter` before accumulation (or before the JSON write). Messages stored in the notice file are later passed to `fmt.Fprintf(os.Stderr, ...)` in `renderUnshownNotices`. Without sanitization, recipe-sourced strings containing ANSI sequences survive through the file and reach the terminal on the next interactive command.

**3. Message accumulation cap** (Low, optional hardening)

Consider capping accumulated messages in `InboxReporter` at a reasonable bound (e.g., 50 messages, each limited to 512 characters) to prevent an unusual install from writing an oversized notice file.

**4. Secrets guard** (Informational)

The existing `progress.Reporter` contract prohibits passing secrets to reporter methods. This must be preserved in the `InboxReporter` implementation. Since the described Warn call sites only pass version strings and asset names, no change is required — but include this explicitly in the code comment on `InboxReporter`.

---

## Summary

The design introduces no new network-facing attack surface or privilege escalation risk. The two issues requiring attention before implementation are: the absence of tool-name path validation in `WriteNotice` (inherited by the new `InboxReporter` write path, Medium severity) and the potential for ANSI sequences to pass through the notice file into the terminal via `renderUnshownNotices` if `InboxReporter` omits sanitization (Low-Medium). Both are straightforward to address in implementation without structural changes to the design.
