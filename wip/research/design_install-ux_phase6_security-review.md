# Security Review: DESIGN-install-ux — Phase 6 Assessment

Reviewer focus: Security Considerations section (lines 596–637 of the design document).

---

## Q1: Attack Vectors Not Considered

The design identifies three categories: ANSI injection, goroutine/terminal-state leakage,
and secret leakage during migration. Two additional vectors are not addressed.

**Goroutine panic path (not covered)**

The design says "if the install path panics or exits abnormally without calling Stop()."
In Go, a panic that is not recovered will terminate the process immediately — the goroutine
does not leak in that case. But if the goroutine itself panics (for example, because a
nil `io.Writer` was passed to `NewTTYReporter`), the entire process terminates. The
goroutine is not recovering panics from its `fmt.Fprint(s.output, ...)` calls. This is
a reliability issue more than a security issue, but panics from a background goroutine
produce confusing "goroutine N panicked" output that cannot be attributed to the user's
action. Recommendation: wrap the animate loop body with a recover that logs to a fallback
or silently stops the spinner.

**StatusMessage parameter map is caller-controlled (not covered)**

`ActionDescriber.StatusMessage(params map[string]interface{})` receives the resolved
step parameters. The design specifies sanitization at the TTYReporter boundary (inside
`Status()`, `Log()`, etc.), which correctly covers this. However, the design does not
mention that the param map itself can contain values from network responses, not just
from the recipe TOML. For example, the `download` action receives a `url` param that
was already URL-expanded using values from a version provider (GitHub API response,
PyPI JSON, etc.). A compromised version provider returning a tag like
`v1.0.0\x1b[2Jinstalled` would produce that string in the `url` param before
`path.Base()` is applied. The design's mitigation (sanitize at Reporter boundary) covers
this, but the threat model paragraph should name it: the threat is not limited to
recipe-controlled values; it includes anything resolved from external network sources
that flows into `StatusMessage` params.

**No mention of Log/Warn format string injection**

`Reporter.Log(format string, args ...any)` takes a printf-style format string. The
design does not constrain who controls the `format` argument. If any callsite passes
a recipe-sourced string as the format argument rather than as an arg (a classic
`fmt.Printf(userString)` anti-pattern), it would bypass sanitization. This is a
code-review concern for the Phase 6 migration, not a design flaw — but the design
should note it explicitly as a migration checklist item alongside the secret-leakage
warning.

---

## Q2: Are the Mitigations Sufficient?

### ANSI Injection Mitigation

The design now specifies:

1. A complete stripping function (`SanitizeDisplayString`) covering CSI, OSC, and raw `\x1b`.
2. Application at the Reporter boundary (inside `Status`, `Log`, `Warn`, `DeferWarn`).
3. Printable ASCII validation for URL basenames.
4. Explicit exemption for `ProgressWriter` byte counters.

This is sufficient in structure. Three sharpening points:

**Point 1 — The CSI regex `\x1b\[[0-9;]*[A-Za-z]` is slightly too narrow.**
The full ECMA-48 CSI sequence allows parameter bytes in the range 0x30–0x3F
(`0-9`, `;`, `<`, `=`, `>`, `?`) and intermediate bytes in the range 0x20–0x2F
before the final byte. The specified regex `[0-9;]*` misses `?` and `<=>`, leaving
`\x1b[?25l` (hide cursor, a practical attack) and `\x1b[?1049h` (alternate screen)
unstripped. The corrected regex is `\x1b\[[\x30-\x3F]*[\x20-\x2F]*[A-Za-z]`.
This is a real gap: the hide-cursor and alternate-screen sequences use the `?` prefix
and are commonly injected.

**Point 2 — DCS sequences (`\x1bP...ST`) are not listed.**
The design lists CSI and OSC. DCS (Device Control String) sequences can set terminal
window titles on some emulators and are used in the Sixel graphics protocol. While less
commonly exploited, a complete stripping function should also strip
`\x1bP[^\x1b]*(\x1b\\|\x07)`. The design's catch-all "raw `\x1b` characters not
covered by the above" partially addresses this, but the explicit DCS pattern should
be named so the implementer doesn't miss it.

**Point 3 — The URL basename check says "0x20–0x7E" but 0x20 is space.**
Allowing space (0x20) in a displayed filename is fine visually. However, a filename
that is entirely spaces (e.g., the result of a crafted URL like
`https://host/v1.0.0/   .tar.gz`) should be treated as invalid and omitted rather than
displayed as blank spinner text. The check should require at least one non-space
character.

### Goroutine Lifecycle Mitigation

The design recommends `Stop()` method and `defer reporter.Stop()`. This is adequate.
The additional note about context cancellation is a useful-but-optional enhancement.
The primary risk (terminal left in a broken state on panic) is addressed if
`Stop()` writes a reset sequence (`\033[?25h\r\033[K`) before returning.

The design does not specify that `Stop()` should be idempotent. If install
orchestration calls `Stop()` and then `FlushDeferred()` calls back into the Reporter,
a non-idempotent Stop could cause a double-close panic on the stop channel. The design
should require idempotent `Stop()`.

### Secret Leakage During Migration

The mitigation (review each of 396 callsites, add interface comment) is correct in
intent. The format string injection concern noted in Q1 should be added to this
checklist. Otherwise, this mitigation is realistic and proportionate.

---

## Q3: Is the SanitizeDisplayString Approach Technically Correct?

The approach is correct in architecture: centralized stripping at the Reporter boundary
is the right place, equivalent to how web frameworks HTML-escape at the template layer
rather than at each data-construction callsite.

The specified regex coverage has the gap noted above (`?` and other parameter prefix
characters in CSI sequences). The precise regexes the design specifies are:

- CSI: `\x1b\[[0-9;]*[A-Za-z]` — misses `\x1b[?25l`, `\x1b[?1049h`
- OSC: `\x1b\][^\x07]*(\x07|\x1b\\)` — correct
- Raw `\x1b` fallback — catches anything not handled above

The raw `\x1b` fallback does catch `\x1b[?25l` if it gets past the CSI regex (because
the `?` causes the CSI regex not to match, leaving `\x1b` as a standalone raw escape
that the third rule strips). However, after stripping `\x1b`, the remaining `[?25l` is
left in the string, which is harmless but visually noisy. The cleaner fix is a broader
CSI regex.

A vetted alternative is the `github.com/acarl005/stripansi` library or the approach
used by `github.com/mgutz/ansi`. If the team prefers no new dependency, the
self-contained correct regex is:

```
\x1b([@-Z\\-_]|\[[^\x07]*?[\x40-\x7E]|\][^\x07]*?(\x07|\x1b\\))
```

This covers CSI (including `?` prefix), OSC, and other two-character ESC sequences
in a single pass without a catch-all fallback.

---

## Q4: Residual Risk Requiring Escalation

Two items need explicit acknowledgement before the design is closed:

**R1 — CSI regex gap (Medium, needs fix before implementation)**

The specified CSI regex `\x1b\[[0-9;]*[A-Za-z]` does not strip `\x1b[?25l` (hide
cursor) or `\x1b[?1049h` (alternate screen buffer). Both are practical terminal
injection payloads. An implementer following the design as written would produce a
`SanitizeDisplayString` function with this gap. The fix is a one-line regex change;
it should be incorporated into the design before Phase 1 implementation begins.

**R2 — Migration scope for format string anti-pattern (Low, needs checklist entry)**

The Phase 6 migration involves 396 `fmt.Printf` callsites, some of which may pass
recipe-sourced strings as the format argument. The migration checklist should
explicitly check that every `fmt.Printf(recipeValue)` pattern is converted to
`reporter.Log("%s", recipeValue)` not `reporter.Log(recipeValue)`. This is not
escalation-worthy on its own but should be a named checklist item in the
implementation issue for Phase 6.

Neither item requires halting or redesigning the feature. R1 is a concrete
implementation detail that needs correction before code is written; R2 is a migration
quality gate.

---

## Verdict

**CONDITIONAL PASS**

The security section is well-structured and substantially correct. It identifies the
right threats, applies the right architectural pattern (sanitize at the boundary), and
specifies a complete threat surface. Two corrections are needed before implementation:

1. Fix the CSI regex to include `[\x30-\x3F]*[\x20-\x2F]*` parameter/intermediate
   bytes instead of `[0-9;]*`, to cover hide-cursor and alternate-screen sequences.
2. Add format string injection as a named migration checklist item in the Phase 6
   secret-leakage paragraph.

The goroutine lifecycle section should add a requirement that `Stop()` is idempotent.

No new dependency, privilege, or architecture risk was found beyond what the design
already addresses. The residual risk after the two corrections is low.
