# Security Review: install-ux

## Dimension Analysis

### External Artifact Handling

**Applies:** Yes — Medium severity

The design does not itself introduce new download or execution logic, but it wraps and exposes external artifact data through the display layer. Specifically:

- `ProgressWriter` tracks bytes from HTTP response bodies sourced from external CDNs and servers. The callback formats file sizes as integers, so the data path is safe by construction for that piece.
- `ActionDescriber.StatusMessage(params)` receives `map[string]interface{}` constructed from recipe TOML parameters. These parameters include URLs, tool names, versions, package names, and other recipe-controlled strings that will be rendered via `Reporter.Status()` and `Reporter.Log()`. A recipe from the distributed registry (or a crafted local recipe) can inject arbitrary strings into these display paths.

The design acknowledges this in its Security Considerations section and proposes stripping ANSI sequences from recipe-sourced values. That mitigation is necessary but the scope needs sharpening: the sanitization point should be defined precisely. If sanitization happens inside `StatusMessage()` implementations individually, it's easy to miss one. If it happens at the Reporter boundary (`Status()` and `Log()` methods), it's centralized but affects all callers including tsuku-generated strings, adding overhead. The design recommends the former (sanitize only recipe-sourced values in `StatusMessage`), which requires each implementation author to remember to sanitize — a convention, not enforcement.

**Mitigations:**
- Apply ANSI stripping in a single helper (`progress.SanitizeMessage(s string) string`) and require its use in all `StatusMessage()` return values. Document this requirement in the `ActionDescriber` interface comment.
- Alternatively, apply stripping inside `TTYReporter.Status()` and `TTYReporter.Log()` for all inputs, accepting the small overhead. Given the 100ms tick rate, the cost is negligible.
- The `ProgressWriter` path is safe as designed: only basename (extracted by Go's `path.Base`, which trims path separators) and integer byte counts are formatted.

---

### Permission Scope

**Applies:** No significant change

This design is a display-layer refactor. It does not change what filesystem paths are read or written, what network connections are made, what subprocesses are spawned, or what privileges are requested. The `TTYReporter` goroutine writes only to `os.Stderr` (an already-open file descriptor). No new permissions are acquired.

The goroutine lifecycle risk (mentioned in the design's Security Considerations) is an operational correctness issue rather than a permission escalation risk: a leaked goroutine that leaves a partial spinner on the terminal is annoying but does not grant elevated access. Low severity as a security concern; medium severity as a reliability concern.

The design's recommendation to implement `Stop()` and defer it addresses the reliability concern. From a security angle, a goroutine stuck writing `\r\033[K` to stderr cannot cause privilege escalation or information disclosure beyond what the process already outputs.

---

### Supply Chain or Dependency Trust

**Applies:** No new risk introduced

The design adds no new external dependencies. It replaces the existing `progress.Writer` widget (an internal package) with a new `ProgressWriter` type (also internal). The `TTYReporter` uses `golang.org/x/term` for `term.IsTerminal`, which is already in the module graph for existing TTY detection elsewhere in the codebase.

Recipe trust is unchanged by this design: the executor already processes recipe TOML parameters to run actions. The install-ux change only affects how those parameters are displayed, not how they are resolved, verified, or executed. The existing recipe validation and registry verification path is out of scope for this design.

---

### Data Exposure

**Applies:** Yes — Medium severity

The design introduces a deferred warning queue (`DeferWarn`/`FlushDeferred`). This queue is populated during install and flushed at the end. If warnings contain sensitive values (API keys resolved from `internal/secrets/`, token values, private registry credentials), they would be buffered in memory during the install and then written to stderr on flush.

The design's Security Considerations section flags this: "The migration of `fmt.Printf` calls must be reviewed to ensure no secret-bearing variables are included in status messages." This is a convention constraint applied during the migration in Phase 6, where 396 `fmt.Printf` occurrences are reclassified. The risk is that a reviewer misclassifies one call that happens to include a resolved secret.

Concrete exposure vectors:
1. A `fmt.Printf` that currently logs a resolved API key or token is migrated to `reporter.Log()` instead of being removed — it now appears on stderr and in any captured CI log.
2. A `DeferWarn` call queues a message containing a resolved credential; the deferred buffer sits in memory longer than the original call-site pattern.
3. `StatusMessage(params)` implementations format a `params` map entry that happens to contain a secret value passed through recipe parameters.

The third vector is low probability because recipe TOML parameters are not typically secret-bearing, but it warrants explicit documentation.

**Mitigations:**
- During Phase 6 migration, require each `fmt.Printf` → `reporter.*` migration to be reviewed for secret-bearing variables. Add a checklist item to the implementation issue.
- Document in the `Reporter` interface comment that callers must not pass values from `internal/secrets/` to any Reporter method.
- Consider adding a `secrets.IsSensitive(v string) bool` guard or a typed `RedactedString` wrapper that the reporter recognizes and refuses to log, as a defense-in-depth measure for future callers.

---

### Terminal Injection

**Applies:** Yes — High severity (partially mitigated by design)

This is the most significant security dimension for this design. The `TTYReporter` uses raw `\r\033[K` escape sequences to implement in-place spinner redraws. Any string passed to `Reporter.Status()` that contains ANSI escape sequences will be interpreted by the terminal emulator.

Attack vectors:

1. **Recipe-controlled tool name**: A recipe sets `name = "my-tool\033[2J"` (clear screen). When `StatusMessage` returns `"Downloading my-tool\033[2J 1.0.0"` and `TTYReporter` writes it with its own `\r\033[K` prefix, the terminal processes the injected escape and clears the screen. More serious sequences can move the cursor, hide text, or produce visual confusion.

2. **Recipe-controlled URL**: The `download_file` action's `StatusMessage` implementation is specified to use `basename(url)`. A URL with a crafted path component like `/releases/v1.0.0/tool-\033[31mmalicious\033[0m-linux.tar.gz` would inject color codes into the status line.

3. **Recipe-controlled version string**: Version strings are sourced from version providers (GitHub tags, crates.io versions, etc.). While version strings are constrained in practice, a compromised version provider or tag like `v1.0.0\033[?25l` (hide cursor) would persist until the terminal is reset.

4. **Deferred warnings**: `DeferWarn` content flushed at install completion writes to the terminal in the same path. If any action includes recipe-sourced text in a `DeferWarn` call, the same injection applies.

The design acknowledges this and proposes stripping `\033[...m` and `\033[...K` patterns. However, ANSI escape sequences are broader than these two patterns. The full set includes:
- CSI sequences: `\033[` followed by parameter bytes and a final byte (e.g., `\033[2J`, `\033[?25l`, `\033[H`)
- OSC sequences: `\033]` ... `\007` or `\033\\` (terminal title injection, hyperlink injection)
- DCS sequences: `\033P` ... `\033\\`
- SS2/SS3, RIS (`\033c`), and others

Stripping only `\033[...m` and `\033[...K` misses cursor movement (`\033[H`, `\033[A`), screen clear (`\033[2J`), cursor visibility (`\033[?25l/h`), and terminal title injection via OSC (`\033]0;injected title\007`). OSC title injection in particular is invisible to users but modifies their terminal window title, which some shells use as trusted context.

**Severity: High** because:
- The attack surface is any recipe in the distributed registry or a local recipe file
- Terminal injection can produce visual confusion (screen clear, cursor hiding) that survives the install process
- OSC injection can silently modify terminal state visible outside tsuku's output window
- The partial mitigation proposed in the design (only stripping `...m` and `...K` patterns) leaves the most dangerous sequences unaddressed

**Mitigations:**

1. Use a complete ANSI/VT100 stripping library or a well-tested regex that covers all CSI sequences, OSC sequences, and raw escape characters. A correct pattern for CSI sequences is `\x1b\[[0-9;]*[A-Za-z]`; for OSC it is `\x1b\][^\x07]*(\x07|\x1b\\)`. The standard Go approach is to use the `github.com/acarl005/stripansi` package or equivalent, or to implement stripping against the VT100 specification.

2. Apply stripping at the `TTYReporter` boundary — inside `Status()`, `Log()`, `Warn()`, and `DeferWarn()` — rather than relying on each `StatusMessage()` implementation to sanitize its own output. This is defense-in-depth: callers can't forget it.

3. For the URL basename case specifically, validate that the result of `path.Base(url)` contains only printable ASCII characters and URL-safe bytes before including it in a status message. Reject or redact filenames that contain non-printable characters.

4. Document the sanitization requirement in the `Reporter` interface comment so future callers writing directly to the reporter (bypassing `StatusMessage`) apply the same care.

---

## Recommended Outcome

**OPTION 2 - Document considerations:**

The design is well-structured and does not introduce architectural security regressions. However, the terminal injection dimension requires a more complete mitigation than the design currently specifies. The implementer needs the following guidance added to or clarifying the Security Considerations section:

---

**Draft Security Considerations section (replacement for the existing ANSI injection paragraph):**

**ANSI escape code injection**

Status messages include tool names, versions, URL basenames, and package names sourced from recipe TOML files or external version providers. A crafted recipe or compromised version provider could include ANSI/VT100 escape sequences in these values.

The proposed mitigation of stripping `\033[...m` (SGR color) and `\033[...K` (erase line) patterns is insufficient. ANSI escape sequences include cursor movement (`\033[H`, `\033[A/B/C/D`), screen operations (`\033[2J`), cursor visibility (`\033[?25l`), and OSC sequences (`\033]0;title\007`) that can modify terminal state silently. A complete stripping approach must cover all CSI, OSC, and DCS sequences, plus raw `\033` characters not followed by a recognized sequence.

Implementation requirements:
1. Define `progress.SanitizeDisplayString(s string) string` that strips all ANSI/VT100 escape sequences using a complete regex or a vetted stripping library.
2. Apply `SanitizeDisplayString` inside `TTYReporter.Status()`, `Log()`, `Warn()`, and `DeferWarn()` for all inputs. Do not rely on `StatusMessage()` implementors to sanitize their own output.
3. For URL basenames, apply `path.Base()` first, then verify the result contains only printable ASCII (0x20–0x7E) before including it in a status message. Replace non-printable characters with `?` or omit the filename.
4. The `ProgressWriter` byte counter path (formatted integers) requires no sanitization.

**Goroutine lifecycle and terminal state**

`TTYReporter` starts a background goroutine on the first `Status()` call. If the install path panics or exits abnormally without calling `Stop()`, the goroutine may leak, leaving the terminal with a partial spinner line. The goroutine should select on both a stop channel and `context.Done()` if a context is available. `TTYReporter` should expose `Stop()` and install orchestration should `defer reporter.Stop()` immediately after construction.

**Secret leakage during migration**

Phase 6 migrates 396 `fmt.Printf` occurrences to `reporter.*` calls. Some existing `fmt.Printf` calls may format secret-bearing variables (resolved API keys, tokens, registry credentials from `internal/secrets/`). Each migrated call must be individually reviewed for secret-bearing values. The `Reporter` interface comment should state that callers must not pass values from `internal/secrets/` to any Reporter method. Calls that currently expose secrets should be removed, not migrated.

---

## Summary

This design is a display-layer refactor with no new network, filesystem, or privilege escalation risks. The primary security concern is terminal injection via ANSI escape sequences in recipe-sourced strings rendered by `TTYReporter`. The existing Security Considerations section identifies the issue but specifies an incomplete mitigation — the full ANSI/VT100 escape sequence set must be sanitized, not just SGR and erase-line patterns, and sanitization should be applied at the `TTYReporter` boundary rather than per `StatusMessage()` implementation. The design is otherwise sound and can proceed with the updated security guidance incorporated.
