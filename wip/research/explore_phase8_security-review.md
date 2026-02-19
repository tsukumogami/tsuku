# Security Review: Pipeline Blocker Tracking Design

**Design document:** `docs/designs/DESIGN-pipeline-blocker-tracking.md`
**Reviewed:** 2026-02-18
**Scope:** Security analysis of failure classification, data remediation, and dashboard display changes

---

## 1. Attack Vectors Not Considered in the Design

### 1.1 Regex Injection via Crafted Package Names (Medium)

The design proposes using `reNotFoundInRegistry` (pattern: `recipe (\S+) not found in registry`) to extract dependency names from error message text. The regex uses `\S+` which matches any non-whitespace sequence. If a package source or error message contains crafted content that matches this pattern, the extracted "dependency name" could be arbitrary text.

**Current code path:** The error messages originate from `tsuku create` CLI output captured via `cmd.CombinedOutput()`. The `\S+` capture group will match anything up to the next whitespace, including characters like `../`, `"`, `<`, `>`, etc.

**Concrete scenario:** If a package ecosystem (e.g., Homebrew) contains a formula whose description or metadata causes `tsuku create` to emit an error like `recipe ../../etc/passwd not found in registry`, the regex would extract `../../etc/passwd` as a dependency name. This value would then be written to:
- The `blocked_by` field of `FailureRecord` in JSONL files
- The `blocked_by` field in queue entries displayed on the dashboard
- The blocker maps used by `computeTopBlockers()` and `computeTransitiveBlockers()`
- The `requeue-unblocked.sh` script's `recipe_exists()` check

The impact varies by consumer:
- **JSONL files:** The extracted name is stored as data, not executed. Low impact.
- **Dashboard display:** The `esc()` function (see Section 1.4) sanitizes for HTML. Low impact.
- **`requeue-unblocked.sh`:** The `recipe_exists()` function constructs file paths from the extracted name: `$RECIPES_DIR/$first/$name.toml`. This is a path traversal risk -- see Section 1.2.

**Mitigation recommendation:** Add validation to `extractBlockedByFromOutput()` (the proposed new function) that rejects dependency names containing path-special characters (`/`, `\`, `..`). The existing `QueueEntry.Validate()` already checks ecosystem prefixes for path traversal but doesn't validate `blocked_by` values.

### 1.2 Path Traversal in `requeue-unblocked.sh` (Medium)

The `recipe_exists()` function in `requeue-unblocked.sh` (line 48-53) constructs file paths from blocker names:

```bash
recipe_exists() {
  local name="$1"
  local first="${name:0:1}"
  first="$(echo "$first" | tr '[:upper:]' '[:lower:]')"
  [[ -f "$RECIPES_DIR/$first/$name.toml" ]] || [[ -f "$EMBEDDED_DIR/$name.toml" ]]
}
```

If a `blocked_by` value contains `../` sequences, this test could check for file existence outside the recipes directory. While `[[ -f ... ]]` is read-only (it only checks existence), a crafted name like `../../../etc/passwd` would cause the function to report the blocker as "resolved" (since `/etc/passwd` exists), triggering premature requeue of the blocked package.

This is currently theoretical because blocker names come from the regex extraction described above, but once the remediation script populates `blocked_by` fields, the attack surface expands since those values persist in committed data files.

**Mitigation recommendation:** Add a validation check in `recipe_exists()` that rejects names containing `/`, `\`, or `..`:

```bash
recipe_exists() {
  local name="$1"
  if [[ "$name" == */* ]] || [[ "$name" == *..* ]]; then
    return 1  # Reject path traversal attempts
  fi
  ...
}
```

### 1.3 Manipulated Blocker Data and Premature Requeue (Low-Medium)

The `requeue-unblocked.sh` script determines whether to requeue a package by checking if all entries in its `blocked_by` list now have recipes in the registry. If an attacker (or a bug in the remediation script) can inject incorrect `blocked_by` values, two outcomes are possible:

**Scenario A: Empty or wrong `blocked_by` causes premature requeue.**
If a package's `blocked_by` is set to `["nonexistent-dep-xyz"]` when the real blocker is `glib`, and someone later creates a recipe for `nonexistent-dep-xyz`, the package requeues while still missing `glib`. The subsequent batch run will fail again, and the package returns to `failed`/`blocked` status. This is a denial-of-progress but self-correcting -- the package never gets into a permanently broken state.

**Scenario B: Extra `blocked_by` entries prevent requeue.**
If `blocked_by` is set to `["glib", "nonexistent-forever-dep"]`, the package stays blocked even after `glib` gets a recipe. This is a more persistent denial of progress because the bogus dependency will never be resolved.

The design's remediation script is the primary vector for both scenarios since it mass-updates `blocked_by` fields. The PR review process is the intended mitigation, but reviewing hundreds of data file changes is realistically infeasible at the individual-record level.

**Mitigation recommendation:** The remediation script should:
1. Log every change it makes (package ID, old category, new category, extracted blockers) to a summary file
2. Cross-reference extracted dependency names against known package ecosystem names (Homebrew formula list, etc.) to flag suspicious extractions
3. Include a count of unique dependency names extracted; an unexpectedly high number of unique names would indicate regex misfires

### 1.4 XSS Risk from Package Names in Dashboard HTML (Low -- Adequately Mitigated)

All dashboard pages (`index.html`, `blocked.html`, `failure.html`, etc.) use an `esc()` function for rendering user-controlled data:

```javascript
function esc(s) {
    if (s == null) return '';
    const div = document.createElement('div');
    div.textContent = String(s);
    return div.innerHTML;
}
```

This is the standard DOM-based HTML entity escaping pattern. It converts `<`, `>`, `&`, and `"` to their HTML entity equivalents. It handles the primary XSS vectors for innerHTML injection.

**Audit of all rendering paths:**

- `index.html` line 559: `esc(b.dependency)` -- dependency names are escaped
- `index.html` line 560: `esc(b.count)` -- counts are escaped
- `blocked.html` line 306: `esc(blocker)` -- filter buttons use escaping
- `blocked.html` line 335: `esc(dep)` -- blocker names in the top blockers panel
- `blocked.html` line 365: `esc(d)` -- dependency badges use escaping
- `blocked.html` line 368: `esc(pkg.name)` -- package names are escaped
- `blocked.html` line 369: `esc(pkg.ecosystem)` -- ecosystems are escaped

The `encodeURIComponent()` function is correctly used for URL parameters (e.g., `href="?blocker=${encodeURIComponent(blocker)}"`).

**One minor gap:** In `blocked.html` line 337, `pkgs.length` is rendered without `esc()`:
```javascript
'<span class="blocker-count">' + pkgs.length + ' blocked</span>'
```
However, `.length` on an array always returns a number, so this is safe.

**Assessment:** The XSS mitigation is sufficient. The `esc()` function is consistently applied to all data-derived strings rendered via innerHTML. Package names, dependency names, ecosystem names, and error messages all pass through escaping before rendering.

### 1.5 Cycle in Transitive Blocker Computation (Low)

The current `computeTransitiveBlockers()` implementation (dashboard.go line 452-474) uses memoization to handle cycles. When a dependency is first visited, `memo[dep]` is set to an empty slice. The recursive call checks `if result, ok := memo[dep]; ok` and returns the empty slice for any dependency being processed, breaking the cycle.

The proposed redesign in the design document uses a similar pattern with `memo[dep] = 0` as a sentinel. This is correct for cycle detection.

However, if the `blocked_by` data is manipulated to create long chains (A blocks B blocks C blocks ... blocks Z), the recursion depth could become large. Go has a default goroutine stack of 1MB that grows up to 1GB, so stack overflow is unlikely for realistic data sizes. JavaScript's `computeTransitiveBlockers` equivalent in the frontend would be more constrained, but the frontend doesn't do transitive computation -- it reads pre-computed counts from `dashboard.json`.

**Assessment:** No action needed. The cycle detection is sound, and realistic data sizes won't cause stack issues.

---

## 2. Sufficiency of Existing Mitigations

### 2.1 Supply Chain Risk Mitigation -- Partially Sufficient

The design states: "the remediation script is reviewed in the PR, and its output (modified data files) is included in the diff for inspection."

This is a reasonable mitigation for a one-time script, but it has practical limitations:
- Reviewing hundreds of data file changes individually is not feasible; reviewers will rely on the script logic being correct
- The script uses `jq` for JSON manipulation, which is trusted but adds a dependency on the system's `jq` version

**Gap:** The design doesn't mention what happens if the remediation script is run multiple times. If someone re-runs it on already-remediated data, it could produce incorrect results (e.g., double-extracting or overwriting correct `blocked_by` with empty values if the category was already changed). The script should be idempotent.

**Recommendation:** Add an idempotency check to the script (skip records that already have `category: "missing_dep"` and non-empty `blocked_by`).

### 2.2 Error Message Coupling Mitigation -- Sufficient

The design acknowledges that the regex depends on the error message format "recipe X not found in registry" and mitigates by noting this is a single regex (`reNotFoundInRegistry`) reused across locations. If the message format changes, both locations break visibly. This is adequate.

### 2.3 "Not Applicable" Justifications

#### Download Verification: "Not Applicable" -- Correct
The design doesn't modify download or verification logic. The changes are purely in classification, display, and metadata files. This justification is accurate.

#### Execution Isolation: "Not Applicable" -- Mostly Correct, with Caveat
The design claims no new processes are spawned. This is true for the core code changes, but the remediation script (`scripts/remediate-blockers.sh`) does spawn `jq` processes and invokes `go run ./internal/dashboard/cmd/` to regenerate the dashboard. These are developer-machine operations, not production tool installations, so the risk is low. The "not applicable" assessment is reasonable but should acknowledge the remediation script's process spawning for completeness.

#### User Data Exposure: "Not Applicable" -- Correct
Failure records contain package names, error messages, and timestamps. No user-specific data is involved. The dashboard is already public. This justification is accurate.

---

## 3. Residual Risk Assessment

### 3.1 Risks That Should Be Escalated

**None require immediate escalation.** All identified risks have low to medium severity and are bounded:
- The path traversal in `recipe_exists()` is read-only (file existence check) and self-correcting (premature requeue leads to a re-failure that gets reclassified)
- The regex injection produces incorrect metadata, not code execution
- The remediation script runs once under PR review, not in automated production pipelines

### 3.2 Residual Risks Accepted

1. **Regex extraction accuracy:** The `\S+` pattern may over-match in edge cases (e.g., a package name ending with punctuation like `openssl@3.` could capture the trailing period). The existing `extractMissingRecipes()` function has this same limitation and it hasn't caused problems in practice.

2. **Large diff review burden:** The remediation PR will have a large diff that reviewers will verify at the script level, not the record level. A subtle bug in the script's `jq` filter could affect many records without being caught in review. The risk is low because the affected data only influences dashboard display and requeue timing, not tool installation.

3. **Data consistency during partial failures:** If the remediation script crashes partway through, some JSONL files will be updated and others won't. The script should be designed to be re-runnable (idempotent), but the design doesn't explicitly state this.

---

## 4. "Not Applicable" Justification Review

| Section | Justification | Assessment |
|---------|---------------|------------|
| Download Verification | Not applicable | **Correct.** No changes to binary download, checksum, or signature verification. |
| Execution Isolation | Not applicable | **Mostly correct.** The remediation script spawns processes but only in developer/CI context, not during tool installation. |
| User Data Exposure | Not applicable | **Correct.** Only pipeline metadata is involved; no user data collected or exposed. |
| Supply Chain Risks | Low risk, mitigated | **Accurate but understated.** The path traversal in `recipe_exists()` and the regex injection vector are real but bounded. The mitigation (PR review) is appropriate for the risk level. |

---

## 5. Specific Findings by Prompt Question

### Can a compromised remediation script inject malicious data affecting downstream behavior?

**Yes, but the impact is limited to pipeline operations.** A compromised script could:
1. Set `blocked_by` to incorrect values, causing packages to be permanently stuck in `blocked` status (denial of progress)
2. Set `category` to incorrect values, misleading the dashboard
3. Flip queue entries from `failed` to `blocked` for packages that aren't actually blocked by dependencies

It **cannot**:
- Cause arbitrary code execution (the data files are JSON metadata, not executable)
- Affect tool installation for end users (these data files are pipeline-internal)
- Compromise the registry or recipe content (the script doesn't modify TOML recipe files)

The PR review mitigation is appropriate for this risk level.

### Can malicious error messages manipulate extracted dependency names?

**Yes, within bounds.** The `\S+` regex will capture any non-whitespace sequence after "recipe " and before " not found in registry". If the CLI error message contains crafted text matching this pattern, arbitrary strings end up in `blocked_by`. However:
- The error messages originate from `tsuku create` running on a controlled CI machine
- The package names come from ecosystem registries (Homebrew, npm, etc.) which have naming constraints
- The extracted names are used as data (map keys, display strings), not as commands or file content

The risk is that a Homebrew formula with an unusual name pattern could cause incorrect blocker tracking, but not code execution.

### Can manipulated blocker data cause premature or prevented requeue?

**Yes to both.** See Section 1.3 above. Premature requeue is self-correcting (the package will fail again and get reclassified). Prevented requeue is more concerning because a bogus dependency name that will never have a recipe created would leave the package permanently blocked. However, an operator reviewing the dashboard's "Top Blockers" panel would notice an unfamiliar dependency name and could manually investigate.

### Are there XSS risks from package names containing HTML/JS?

**No, the existing `esc()` function adequately prevents XSS.** All data-derived strings are escaped before innerHTML insertion across all dashboard pages. See Section 1.4 for the full audit.

---

## 6. Recommendations Summary

| Priority | Recommendation | Rationale |
|----------|---------------|-----------|
| Medium | Validate extracted dependency names in `extractBlockedByFromOutput()` -- reject names containing `/`, `\`, `..`, `<`, `>` | Prevents path traversal and reduces injection surface |
| Medium | Add path traversal check to `recipe_exists()` in `requeue-unblocked.sh` | Prevents crafted blocker names from triggering false-positive recipe existence |
| Low | Make remediation script idempotent with explicit skip for already-remediated records | Prevents issues from accidental re-runs |
| Low | Include remediation summary report with unique dependency name count for anomaly detection | Catches regex extraction misfires during PR review |
| Info | Acknowledge process spawning in Execution Isolation section | Completeness, not a risk driver |
