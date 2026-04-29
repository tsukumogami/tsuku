# Phase 6 Security Re-review: DESIGN-pipx-pypi-version-pinning

**Design**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/docs/designs/DESIGN-pipx-pypi-version-pinning.md`
**Phase 5 reference**: `wip/research/design_pipx-pypi-version-pinning_phase5_security.md`
**Reviewer**: Phase 6 (post-fold-in verification)
**Date**: 2026-04-28

This document re-reviews the Security Considerations section after Phase 5's three recommendations were folded into the design. It probes for attack vectors Phase 5 may have missed, stress-tests the parser hardening, sanity-checks the "not applicable" justifications, and verifies the Phase 5 fold-in was complete.

---

## 1. Missed attack vectors

### 1a. Race condition between python-standalone resolution and the filter call

**Verified path:**
- `pipx_install.Decompose` calls `ResolvePythonStandalone()` (returns a path), then calls `getPythonVersion(pythonPath)` (subprocess).
- The new design adds: result of `getPythonVersion` is then passed to `provider.ResolveLatestCompatibleWith(ctx, pythonMajorMinor)`.
- The filter then drives `pip download <pkg>==<version>`, which itself re-invokes the same `pythonPath`.

**TOCTOU window:** Between the `--version` probe and the `pip download` invocation, the binary at `pythonPath` could in principle be swapped (e.g., concurrent `tsuku update python-standalone`, manual filesystem edit, malicious local process). The filter would then choose a release for the *probed* major.minor, but `pip download` would run against a *different* binary.

**Severity: low.** Reasoning:
- `$TSUKU_HOME/tools/` is owned and managed by tsuku; an attacker who can write to it has already won.
- Concurrent `tsuku` invocations are not guarded by a global lock today (this is a pre-existing property of tsuku, not introduced by this design). The race exists for *every* path that probes a binary's version then re-invokes it. The new code does not widen the window meaningfully — it adds one more consumer of `pythonMajorMinor` in the same function, between two existing `getPythonVersion`/`pip download` invocations that already trust the result.
- Worst-case outcome of a successful swap: `pip download` fails with its existing exit code (incompatible Python), or installs a working version that nominally matches a different Python — same risk surface as today.

**Recommendation:** No design change required. Optional follow-up: a workspace-level note that concurrent tsuku invocations on the same `$TSUKU_HOME` are not race-safe (likely already known internally; not specific to this design).

### 1b. Long-lived `tsuku` daemon scenarios

**Checked:** No evidence in the codebase of a long-lived tsuku daemon. The CLI binary is invoked per-command; updates happen via `tsuku update-registry`, `tsuku update`, etc., each running a fresh process. Background update checks (`internal/updates/`) run in-process for the duration of one command.

**Implication:** The `pythonMajorMinor` value computed in `Decompose` lives only for the duration of one `tsuku eval` or `tsuku install` invocation. There is no cache, no daemon-held state, no IPC handle that could be poisoned across long lifetimes.

**Severity: N/A.** No daemon to attack.

### 1c. Other vectors Phase 5 may have under-weighted

- **PyPI mirror/CDN cache poisoning.** PyPI metadata is fetched over HTTPS via the existing provider. Phase 5 covered "malicious PyPI response" generically; the same conclusion holds here (per-package DoS, no amplification). No new vector.
- **Concurrent recipe evaluation in CI.** Multiple `tsuku eval` invocations against the same `$TSUKU_HOME` could race on `getPythonVersion`. Same analysis as 1a — not new, not widened.
- **Error-message log injection.** The error template is plain ASCII and the four interpolated fields (package, X.Y, V, Z) come from controlled sources (recipe TOML and PyPI JSON). Even if a malicious PyPI response embedded ANSI escapes or newlines in `Requires-Python`, the design's ASCII-only validation in the parser would have already rejected the value, so it would never reach the message formatter. Worth confirming during implementation that the error message *format* path (separately from the parser) does not interpolate the raw `requires_python` of a *different* unparsed release; the design currently shows it does interpolate the latest release's `requires_python` in the example, which by construction has been parsed and accepted (or treated as compatible-on-null), but for the no-compatible branch the interpolated string is the *latest's* `requires_python` — which the parser may have rejected. **Minor recommendation:** the design's error template should sanitize the interpolated `<Z>` to printable ASCII at format time, or use the *parsed* form rather than the raw string. This is a one-line concern, low severity.

---

## 2. Adequacy of the three parser hardening checks

The design enforces three checks at `ParseSpecifier` entry:
1. Per-clause length cap of 256 bytes.
2. ASCII-only (any byte > 0x7F → reject).
3. Segment-magnitude cap (>6 digits or `math.MaxInt32` → reject).

### 2a. Could a 256-byte clause exhibit worst-case behavior?

A clause exactly at 256 bytes passes the length check. What is the worst-case parse cost for such a clause?

- **Tokenization on commas:** linear in length. 256 bytes of commas → 256 zero-length sub-clauses. After `TrimSpace`, each becomes empty → all rejected with `ErrMalformed`. Fast-fail, ~256 string ops. Bounded.
- **Operator longest-prefix match:** at most 2 bytes of lookahead (`>=`, `<=`, `==`, `!=`). Constant per sub-clause.
- **Integer parse:** segment-magnitude cap caps each integer at 6 digits. With four segments, max 24 digits per version + 3 dots = 27 bytes. A clause is `<op><version>` so max meaningful payload is ~30 bytes. Anything between 30 and 256 bytes is either whitespace, repeated commas, or trailing junk — all fast-fail.
- **Wildcard handling:** `==X.Y.*` / `!=X.Y.*` adds two more bytes. Still bounded.

**Verdict:** A 256-byte clause cannot drive worst-case behavior beyond linear scanning of 256 bytes. The 256-byte cap is generous (real-world max is ~64); reducing to 128 would not buy meaningful protection. No quadratic algorithm is reachable.

### 2b. Bypass scenarios for the three checks

- **Cap bypass via many short clauses.** A response could pack thousands of 64-byte clauses into a single `requires_python` string. The 256-byte cap is *per clause*, not per specifier total. **Gap identified.** Mitigation: the parser should also cap *total clauses per specifier* (e.g., 32) or *total specifier length* (e.g., 1024 bytes). Real-world `requires_python` has 1–4 clauses; 32 is a generous ceiling. This is a small additional check that closes the door cleanly.
- **ASCII-only bypass.** None apparent. The check is a single byte-loop and applies to the whole specifier.
- **Segment-magnitude cap bypass.** A version like `999999.999999.999999.999999` (6 digits each) sits exactly at the boundary. Each segment fits in `int32` (max 999999 < 2^31). Compare cost is constant. No bypass.

**Recommendation:** Add a fourth hardening check — total specifier length cap (e.g., 1024 bytes) or maximum clauses per specifier (e.g., 32). Phase 5 missed this. Severity is low (the per-package PyPI response is already 10 MB-bounded), but the check is one line and closes the "many small clauses" gap explicitly.

### 2c. Sufficiency overall

With the addition of a total-length / clause-count cap, the four checks cover:
- Single huge clause → length cap.
- Many small clauses → total cap.
- Non-ASCII smuggling → ASCII check.
- Integer overflow → magnitude cap.

This is sufficient for the bounded grammar described.

---

## 3. Re-checking "not applicable" justifications

### 3a. Recipe-side trust model claim

**Claim in design:** "Recipes do not declare Python version, do not declare specifier strings, and gain no new privilege. The auto-filter consumes only upstream PyPI metadata."

**Probe: could a malicious recipe author trigger the filter in a way that exfiltrates info via the error message?**

The recipe author controls:
- The `package` field (PyPI package name).
- The `executables` array.

The recipe author does *not* control:
- The bundled Python version (tsuku-side).
- PyPI's `requires_python` metadata for the package (upstream).
- The error template or its formatting.

**Exfiltration analysis:**
- A recipe author who can publish a PyPI package can also publish `requires_python = ">=99.0"` on every release of that package, forcing the no-compatible branch and the resulting error message. The error message echoes `<package>` (recipe-controlled), `<X.Y>` (tsuku-internal, public), `<V>` (latest release, public), `<Z>` (the `requires_python` string, attacker-controlled at upstream).
- The attacker thereby controls one substring (`<Z>`) of the error output. Could this be used to exfiltrate? **No** — the data flows *into* the error message, not out. There is no sensitive context to leak: the error is constructed from values the attacker already knows (their own PyPI metadata) plus tsuku-internal values that are public (bundled Python version, package name).
- Could `<Z>` carry a payload that exploits the *consumer* of the error (terminal escape sequences, log injection)? The design says ASCII-only on the parser side, but the no-compatible-branch case interpolates the *latest release's* `requires_python` — which the parser walked past. If the latest release's `requires_python` was parsed and rejected, the message's `<Z>` would be... empty? unspecified?

This is the gap noted in 1c above. The design doesn't fully specify what `<Z>` is when the latest release's specifier was rejected by parser hardening. **Recommendation:** the design should explicitly state that `<Z>` is either the parsed canonical form, a fixed `<malformed>` placeholder, or omitted entirely — and never the raw byte string from PyPI. This closes the log-injection / terminal-escape concern.

**Severity: low.** The attack requires publishing a malicious PyPI package, which already grants stronger primitives. But the design contract should be tightened.

### 3b. Other "not applicable" claims

- **No new credentials, file writes, network endpoints.** Verified by reading the design's deliverables. All five new/modified files are read-only against PyPI plus pure parsing. Confirmed N/A.
- **No new subprocess invocation.** Verified: `getPythonVersion` is hoisted earlier in `Decompose`, not introduced. Confirmed N/A.
- **User pin bypass is CLI-driven.** Verified: recipes have no field for version pins; the design's "Decisions Already Made" explicitly forbids it. Confirmed N/A.

---

## 4. Residual risk and follow-up signals

### 4a. Residual risks worth tracking

1. **Concurrent `$TSUKU_HOME` mutation.** Pre-existing, not introduced by this design. Worth a separate roadmap-level note if not already tracked.
2. **Error-message field interpolation safety.** The design should specify how `<Z>` is rendered when the latest release's `requires_python` is rejected by parser hardening. One-paragraph clarification needed in the design before implementation.
3. **PyPI metadata format drift.** The design acknowledges this in Consequences ("clause-rejection is loud"). Worth a telemetry signal.

### 4b. Recommended follow-up signals

- **Telemetry counter for `ErrTypeNoCompatibleRelease`.** When the no-compatible branch fires in production, capture: package name, bundled Python, latest release's parseability (parsed-but-incompatible vs. rejected-as-malformed). Helps distinguish "real upstream support drop" from "PyPI metadata format drift breaking our parser." Existing telemetry infrastructure (`internal/telemetry/`) can carry this.
- **Telemetry counter for parser-rejected clauses.** Even when the filter succeeds (a later release was compatible), track how many earlier releases were skipped due to parser rejection vs. genuine incompatibility. A spike in rejection rate signals format drift before user reports come in.
- **Follow-up issue: `requires_python` interpolation safety.** Title: "Sanitize `requires_python` rendering in `ErrTypeNoCompatibleRelease` message." Scope: ensure `<Z>` is parsed canonical form or fixed placeholder, never raw bytes. Small.
- **Follow-up issue: total-specifier-length / clause-count cap.** Title: "Add total-length cap to PEP 440 `ParseSpecifier`." Scope: one additional check at entry. Small.

### 4c. Issues that warrant escalation

None require escalation. Both follow-ups are small parser hardening additions that fit cleanly into Phase 1 of the existing implementation plan. The design's overall security posture remains sound.

---

## 5. Verification: were Phase 5's three recommendations correctly folded in?

Phase 5 recommended three parser hardening checks be added to Phase 1's deliverables and to the Security Considerations section.

### 5a. Phase 5's three recommendations

From `wip/research/design_pipx-pypi-version-pinning_phase5_security.md` (Recommended Outcome):

1. **Per-clause length cap of 256 bytes** at `ParseSpecifier` entry.
2. **ASCII-only input check.** Reject any byte > 0x7F as malformed.
3. **Segment-magnitude cap.** Reject version segments > 6 digits or `math.MaxInt32` as malformed.

### 5b. Phase 1 deliverables in the design (lines 612–629)

The design's Phase 1 section reads:

> "Three input-hardening checks (security review) are enforced at `ParseSpecifier` entry: per-clause length cap of 256 bytes, ASCII-only byte validation, and segment-magnitude cap (>6 digits or `math.MaxInt32` rejected). Each check has a dedicated negative test case."

Deliverables list includes:

> "- `internal/version/pep440/specifier.go` (includes the three input-hardening checks)
> - `internal/version/pep440/pep440_test.go` (positive cases from the L5 survey table; negative cases for each hardening check)"

**Verified:** All three Phase 5 recommendations are listed verbatim with explicit test coverage. **Match.**

### 5c. Security Considerations section in the design (lines 667–732)

The design's "PEP 440 parser as attack surface" bullet reads:

> "- **Per-clause length cap of 256 bytes.** Real-world clauses are <64 bytes; longer inputs are rejected as malformed.
> - **ASCII-only validation.** Any byte > 0x7F is rejected. PyPI's `requires_python` is ASCII in practice; this prevents Unicode confusable inputs from reaching the operator matcher.
> - **Segment-magnitude cap.** Version segments above 6 digits (or `math.MaxInt32`) are rejected as malformed, preventing integer overflow in comparison."

**Verified:** This matches Phase 5's "Suggested Security Considerations Section (replacement text)" (lines 300–368 of the Phase 5 review) almost word-for-word. The only minor difference is the design says "Mitigations beyond the bounded grammar, all enforced at `ParseSpecifier` entry:" where Phase 5 said "Mitigations beyond the bounded grammar:" — the design's version is slightly clearer (it points to the enforcement location). **Match.**

### 5d. Other Phase 5 items

Phase 5 also flagged a typo: the error-contract snippet declared `ResolverErrorType = "no_compatible_release"` but the codebase uses `type ErrorType int` with iota.

**Checked design lines 555–568:** the design now reads `const ErrTypeNoCompatibleRelease // appended to the existing iota block` with a comment explaining the iota pattern. **The typo is corrected.** Match.

### 5e. Verdict on fold-in

All three Phase 5 recommendations and the typo correction are present in the design. Fold-in is complete and accurate.

---

## Summary

**Phase 5 recommendations fold-in:** Verified complete. All three parser hardening checks appear in both Phase 1 deliverables and the Security Considerations section, with test coverage. The typo correction (`ErrorType` int via iota, not string) is also applied.

**New findings in Phase 6:**

1. **Missed parser cap (low):** The 256-byte per-clause cap does not bound *total* specifier length or clause count. Many small clauses could pile up. Recommend adding a total-length cap (~1024 bytes) or clause-count cap (~32) to `ParseSpecifier`. One-line check, fits cleanly in Phase 1.
2. **Error-message interpolation gap (low):** The no-compatible-branch error template interpolates `<Z>` (the latest release's `requires_python`). When that string was rejected by parser hardening, the design does not specify what `<Z>` renders as. Recommend: render parsed canonical form, or use `<malformed>` placeholder — never raw upstream bytes. Closes a theoretical log-injection / terminal-escape concern.
3. **TOCTOU between `getPythonVersion` and `pip download` (low):** Pre-existing in tsuku; not widened by this design. No design change needed.
4. **No daemon scenarios apply.** tsuku is per-invocation; no long-lived state to attack.
5. **Recipe-side exfiltration probe (no risk):** A malicious package author can control one substring of the error message but cannot exfiltrate sensitive context (none is interpolated). Tightening the `<Z>` rendering (item 2) closes the residual concern about payload smuggling into the error stream.

**Recommended follow-up signals:**

- Telemetry counter on `ErrTypeNoCompatibleRelease` (split by parsed-but-incompatible vs. malformed).
- Telemetry counter on parser-rejected clauses encountered during normal walks (catches PyPI format drift before user reports).
- Two small follow-up issues: total-length cap; sanitize `<Z>` interpolation.

**Overall security posture:** The design is security-clean. The two Phase 6 findings are minor parser hardening refinements, not architectural concerns. They fit into Phase 1's existing scope without affecting Decision 1, 2, or 3.
