# Security Review: project-configuration (Phase 6)

Review of the Security Considerations section in DESIGN-project-configuration.md and the Phase 5 security report.

## 1. Attack Vectors Not Considered

### 1.1 TOML Bomb / Resource Exhaustion

Neither document considers a `tsuku.toml` with an extremely large number of tool entries (thousands) or deeply nested inline tables. While `BurntSushi/toml` is well-tested, a file declaring 10,000 tools would trigger 10,000 install attempts. The batch install loop has no upper bound on tool count.

**Recommendation:** Cap the number of tools in a single `tsuku.toml` at a reasonable limit (e.g., 256). This is a simple validation check after parsing.

### 1.2 Recipe Name Injection into File Paths

Recipe names from `tsuku.toml` flow into `$TSUKU_HOME/tools/<name>-<version>/` directory creation. If recipe names aren't validated against path traversal characters (e.g., `../`, embedded nulls), a crafted `tsuku.toml` could attempt to write outside `$TSUKU_HOME/tools/`. This likely fails at the registry lookup stage (no recipe would match), but the validation gap deserves explicit mention.

**Assessment:** Low risk due to the curated registry acting as a gatekeeper -- names that don't match a recipe fail before any filesystem operation. However, the design should explicitly state that recipe name validation (alphanumeric + hyphens only) is enforced before any filesystem operations, not just implicitly by registry lookup failure.

### 1.3 Race Condition on Config File

Between discovery (stat) and parsing (open+read), the file could be swapped. This is a classic TOCTOU issue. In practice, the window is tiny and exploitation requires local filesystem access (at which point the attacker has broader capabilities). Not a realistic concern for the threat model, but worth documenting as explicitly accepted.

### 1.4 TSUKU_CEILING_PATHS Manipulation

If an attacker can set environment variables (e.g., through a compromised `.bashrc` or `.env` file in the repo), they could set `TSUKU_CEILING_PATHS` to bypass the `$HOME` ceiling, causing traversal to reach directories outside the user's home. This is a narrow scenario -- if the attacker controls env vars, they have broader attack surface already.

**Assessment:** Low risk. Document that `TSUKU_CEILING_PATHS` restricts (never expands) traversal scope. The implementation should treat `$HOME` as an unconditional ceiling that `TSUKU_CEILING_PATHS` can only supplement, never override.

### 1.5 Concurrent Batch Install Side Effects

When `tsuku install` (no args) runs multiple installs sequentially, a later tool's post-install actions execute in an environment where earlier tools are already installed. A carefully ordered `tsuku.toml` could ensure tool A is installed first, then tool B's post-install runs with tool A available on `$PATH`. This creates a dependency chain the user didn't explicitly authorize.

**Assessment:** Low risk given the curated registry. If third-party or community recipes are ever supported, this becomes medium risk.

## 2. Mitigation Sufficiency

### 2.1 "Explicit Invocation" Mitigation -- Partially Sufficient

Both documents cite "requires explicit invocation" as the primary mitigation for untrusted repo configs. This is necessary but not sufficient on its own. The Phase 5 report correctly identifies the gap: once a user runs `tsuku install`, they may not inspect the tool list. The suggestion for a confirmation prompt is the right call.

**Gap:** The design document's Security Considerations section doesn't include the confirmation prompt suggestion from Phase 5. The design says "Batch install prints the discovered config file path and full tool list before installing" -- but printing is passive. Users running `tsuku install` in a freshly cloned repo likely won't read scrolling output before it proceeds.

**Recommendation:** Adopt Phase 5's suggestion: require interactive confirmation for batch install by default. Add `--yes` / `-y` for CI. This is the single most impactful mitigation missing from the design.

### 2.2 Symlink Resolution -- Sufficient (Now Incorporated)

Phase 5 identified symlink traversal as a risk and recommended `filepath.EvalSymlinks`. The design document has incorporated this into `LoadProjectConfig`'s doc comment. This is sufficient.

### 2.3 "latest" Version Warning -- Partially Sufficient

Both documents identify the "latest" keyword as a reproducibility and supply chain risk. The design includes a warning. However, warnings are easy to ignore.

**Recommendation:** Consider a `--strict` mode (or `strict = true` in user config) that rejects "latest" and empty versions entirely. This gives security-conscious teams a way to enforce pinning without relying on developer discipline.

### 2.4 Dry-Run Mitigation -- Insufficient as Primary Defense

`--dry-run` is listed as a mitigation in both documents. It's a useful diagnostic tool, but nobody runs `--dry-run` before every `tsuku install`. It shouldn't be counted as a mitigation for untrusted config files -- it's a debugging feature.

### 2.5 Parent Traversal Mitigations -- Sufficient

The combination of `$HOME` ceiling, `TSUKU_CEILING_PATHS`, symlink resolution, and printing the config path covers the parent traversal risk adequately. The design's first-match-no-merge strategy is itself a mitigation -- it prevents surprising config composition from multiple directories.

## 3. "Not Applicable" Justifications

### 3.1 Data Exposure -- Correctly Marked Not Applicable

Phase 5 marks Data Exposure as "No" and provides a thorough justification. The design reads a local file and passes names/versions to the existing install pipeline. No new network calls, no new telemetry fields, no credential handling. This is correct.

**One nuance:** If a `tsuku.toml` is committed to a public repository, the tool requirements become public knowledge. An attacker could use this to inventory a project's toolchain and target known vulnerabilities in those specific tool versions. This is not a data exposure in the traditional sense (no secrets leak), but it's worth noting as an information disclosure consideration. It does not warrant changing the "Not Applicable" designation.

## 4. Residual Risk Assessment

### 4.1 Residual Risks That Should Be Escalated

**Delegated install authority without confirmation (Medium, escalate).**
This is the most significant residual risk. The design shifts tool selection from the user to the config file author. Without an interactive confirmation step, running `tsuku install` in any cloned repository triggers installs based on someone else's choices. The Phase 5 report identified this and recommended a confirmation prompt. The design document has not adopted this recommendation. This should be escalated as a design amendment before implementation.

### 4.2 Residual Risks Acceptable as Documented

**Users ignoring warnings (Low).** Users may ignore "latest" version warnings. Acceptable -- warnings are the right tool here, with an optional strict mode as a future enhancement.

**Registry expansion weakening name confusion defense (Low).** The curated registry is currently small. As it grows, the odds of confusingly similar recipe names increase. Acceptable now, but worth revisiting if/when community recipe contributions are supported.

**Shared directories between project and $HOME (Low).** Parent traversal can pick up configs from shared directories below `$HOME`. The ceiling paths mechanism provides an escape hatch. Acceptable.

**No vulnerability database check (Low).** Tsuku doesn't check whether a pinned version has known CVEs. This is a feature gap, not a design flaw. Acceptable for v1, worth tracking as a future enhancement.

## 5. Summary of Findings

### Must-Fix (Before Implementation)

1. **Add interactive confirmation for batch install.** Print the config file path, full tool list with versions, and require user confirmation before proceeding. Add `--yes`/`-y` flag for non-interactive contexts. This is the Phase 5 report's primary recommendation and the most important security improvement.

### Should-Fix (During Implementation)

2. **Cap tool count in tsuku.toml.** Add a reasonable upper bound (e.g., 256 tools) to prevent resource exhaustion from maliciously large configs.

3. **Ensure $HOME ceiling is unconditional.** `TSUKU_CEILING_PATHS` should add additional ceilings, never remove the `$HOME` ceiling. Document this explicitly.

4. **Validate recipe names before filesystem operations.** Don't rely solely on registry lookup failure to catch path traversal characters in recipe names.

### Nice-to-Have (Future)

5. **Strict mode for version pinning enforcement.** Let security-conscious teams reject "latest" and empty versions.

6. **Vulnerability awareness.** Track known-vulnerable versions and warn during install.

## 6. Phase 5 Report Quality Assessment

The Phase 5 report is well-structured and correctly identifies the three most significant risk areas. Its recommendations (confirmation prompt, symlink resolution, "latest" warning) are all sound. The "Data Exposure: No" justification is thorough and correct.

Two areas where Phase 5 could have gone deeper:
- Resource exhaustion from large configs was not considered
- The interaction between batch install ordering and post-install actions was not explored
- The TOCTOU window between config discovery and parsing was not mentioned (though it's low-risk)

Overall, the Phase 5 report reaches the right conclusions and its primary recommendation (confirmation prompt) should be adopted into the design.
