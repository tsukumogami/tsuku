# Security Review: Verification Self-Repair Design

## Executive Summary

The verification self-repair design introduces **low overall risk** to the tsuku system. The feature operates within existing security boundaries (sandbox isolation, timeout enforcement) and doesn't introduce new attack surfaces. The security claims in the design document are accurate with minor clarifications needed.

**Recommendation**: Approve with minor documentation improvements.

---

## 1. Attack Vector Analysis

### 1.1 Malicious Tool Output Manipulation

**Threat**: A malicious binary could craft output designed to trick the pattern analyzer into producing an incorrect but passing verification.

**Analysis**:
- The analyzer looks for patterns like "usage:" and the tool name in output
- A malicious binary could easily emit these patterns
- However, this doesn't grant new capabilities - the malicious binary is already installed and can execute

**Risk Level**: Minimal

**Mitigations**:
- Re-validation runs the repaired recipe in sandbox - the tool must actually produce matching output
- Pattern matching is conservative (false negatives fall to LLM, not false passes)
- The verification step is already trust-on-install (we're verifying the binary works, not that it's safe)

**Residual Risk**: Negligible - a malicious binary already has full control within the sandbox; gaming verification patterns provides no additional capability.

### 1.2 Fallback Command Exploitation

**Threat**: Running `--help` or `-h` could trigger unintended behavior in certain tools.

**Analysis**:
- These flags are near-universal conventions for showing help text
- Some tools might interpret these differently (e.g., tools with `help` subcommands)
- Execution is sandboxed with existing timeout and resource limits

**Risk Level**: Low

**Potential Edge Cases**:
1. Tools that interpret `-h` as "host" rather than "help" (e.g., some network tools)
2. Tools that require arguments after flags (most would error safely)
3. Tools with interactive `--help` (blocked by no-TTY sandbox environment)

**Mitigations**:
- Sandbox timeout (default 2 minutes per the existing `ResourceLimits.Timeout`)
- Network isolation (`network: "none"` unless `RequiresNetwork` is set)
- Read-only mounts for sensitive paths
- Commands execute with same privileges as original verification

**Residual Risk**: Minimal - edge cases produce failures, not vulnerabilities.

### 1.3 Denial of Service via Repair Loop

**Threat**: A tool that consistently fails verification could cause repeated repair attempts, consuming resources.

**Analysis**:
- The orchestrator limits repair attempts via `MaxRepairs` (default: 2)
- Self-repair adds at most 2 additional sandbox executions (--help, then -h)
- Total worst-case: original + 2 fallbacks = 3 executions before falling back to LLM

**Risk Level**: Low

**Mitigations**:
- `MaxRepairs` limit prevents infinite loops
- Sandbox timeout prevents long-running commands
- Resource limits (memory, PIDs) prevent fork bombs

**Residual Risk**: None beyond existing mitigations.

### 1.4 Information Disclosure via Telemetry

**Threat**: Repair metadata transmitted via telemetry could leak sensitive information.

**Analysis**:
- `RepairMetadata` contains: Type, Original command, Repaired command, Method, ExitCode
- Tool name and verification commands are already public (in recipes)
- No user paths, environment variables, or other PII are captured

**Risk Level**: Minimal

**Data Flow**:
```
Sandbox stdout/stderr → Analyzer (local only)
      ↓
RepairMetadata → BuildResult → Telemetry (if enabled)
      ↓
Transmitted: {tool_name, repair_method, success/failure}
```

**Mitigations**:
- Telemetry can be disabled via `TSUKU_TELEMETRY=0`
- Only metadata (not full output) is transmitted
- No file paths or user data included

**Residual Risk**: None - transmitted data is less sensitive than existing telemetry.

### 1.5 Recipe Injection via Pattern

**Threat**: Malformed output could inject unexpected patterns into repaired recipes.

**Analysis**:
- The analyzer sets `pattern = "usage:"` or the tool name
- Patterns are used for regex matching during verification
- A malicious tool could try to inject regex metacharacters

**Risk Level**: Low

**Mitigations**:
- The `SuggestedPattern` comes from fixed strings ("usage:") or the known tool name
- Tool names are validated at recipe creation time (kebab-case, limited characters)
- Repaired recipes are re-validated in sandbox before being returned

**Residual Risk**: Minimal - pattern injection would cause verification failure, not arbitrary execution.

---

## 2. Review of Design Document Security Claims

### 2.1 "Download Verification - Not Applicable"

**Assessment**: Accurate

**Reasoning**: The feature modifies `verify` sections, not `steps`. Downloads are handled by actions (`download_archive`, `github_archive`, etc.) which have their own checksum verification. The self-repair logic never touches download URLs or checksums.

### 2.2 "Execution Isolation - Low Impact"

**Assessment**: Accurate with clarification

**Clarification Needed**: The design should explicitly note that fallback commands inherit the same isolation as the original verification:
- Same container image
- Same network mode (`none` unless `RequiresNetwork`)
- Same resource limits (memory, CPU, PIDs, timeout)
- Same mount configuration

**Code Reference**: `internal/sandbox/executor.go` lines 250-275 configure these limits consistently.

### 2.3 "Supply Chain Risks - Not Applicable"

**Assessment**: Accurate

**Reasoning**: This feature is post-download. The binary is already installed before verification runs. The self-repair logic cannot change:
- Where binaries come from (download URLs)
- How they're verified (checksums)
- What gets installed (extraction logic)

### 2.4 "User Data Exposure - Minimal"

**Assessment**: Accurate

**Reasoning**:
- Reads: sandbox stdout/stderr (ephemeral, container-local)
- Writes: `BuildResult.RepairMetadata` (struct, not user data)
- Transmits: tool name and repair method (not sensitive)

No additional data exposure beyond existing LLM repair flow.

---

## 3. Mitigation Table Review

| Risk | Design Mitigation | Adequacy | Notes |
|------|------------------|----------|-------|
| Malicious tool output tricks analyzer | Conservative pattern matching; false negatives to LLM | Adequate | Correct approach - fail-safe to existing path |
| Fallback command hangs | Sandbox timeout | Adequate | Uses existing `ResourceLimits.Timeout` |
| Repair produces wrong recipe | Re-validation in sandbox | Adequate | Same validation as original recipe |

**Additional Mitigations Not Mentioned**:
1. **Network isolation**: Fallback commands run without network access (unless recipe requires it)
2. **No TTY**: Interactive prompts will fail/timeout harmlessly
3. **Exit code filtering**: Only exit codes 1-2 trigger repair (not 127/command not found)

---

## 4. Gaps and Recommendations

### 4.1 Document Exit Code Filtering

**Gap**: The design mentions exit codes 1-2 as indicators of "invalid argument" but doesn't explicitly document this as a security boundary.

**Recommendation**: Add to Security Considerations:
> Exit codes outside the expected range (1-2) bypass self-repair. Exit code 127 (command not found) indicates the binary isn't installed and is not eligible for verification repair.

### 4.2 Consider Pattern Escaping

**Gap**: If tool names contain regex metacharacters, the suggested pattern could behave unexpectedly.

**Recommendation**: The `VerifyFailureAnalysis.SuggestedPattern` should escape the tool name when using it as a pattern:
```go
// Escape tool name for use in regex pattern
escapedName := regexp.QuoteMeta(toolName)
```

**Risk if not addressed**: Low - tool names are validated to be kebab-case, so this is defensive coding rather than a security fix.

### 4.3 Add Telemetry Schema Version

**Gap**: The design mentions a new telemetry event (`verify_self_repair`) but the current telemetry schema doesn't include verification-specific events.

**Recommendation**: When implementing Phase 4 (Telemetry), define a new event type in `internal/telemetry/event.go`:
```go
type VerifySelfRepairEvent struct {
    Action       string // "verify_self_repair"
    ToolName     string
    Method       string // "output_detection" or "fallback_help"
    Success      bool
    // ... standard fields
}
```

This is a design completeness issue, not a security concern.

---

## 5. Questions Addressed

### Q1: Are there attack vectors we haven't considered?

**Answer**: The analysis above covers the primary vectors. One minor addition:

**Symlink/Path Traversal in Tool Name**: If a malicious recipe could specify a tool name with path components, it might affect pattern matching. However, recipe validation already prevents this (tool names must be valid identifiers).

### Q2: Are the mitigations sufficient for the risks identified?

**Answer**: Yes. The existing sandbox infrastructure provides adequate isolation. The self-repair logic:
- Runs in the same sandbox as existing verification
- Has the same resource limits and timeouts
- Uses conservative pattern matching that fails safely

### Q3: Is there residual risk we should escalate?

**Answer**: No. All residual risks are within acceptable bounds for a tool that downloads and executes third-party binaries. The verification self-repair feature doesn't materially change the threat model.

### Q4: Are any "not applicable" justifications actually applicable?

**Answer**: No. The justifications are accurate:
- **Download Verification**: Truly not applicable - no downloads occur
- **Supply Chain Risks**: Truly not applicable - feature is post-installation

---

## 6. Conclusion

The verification self-repair design is security-sound. It operates within existing security boundaries and doesn't introduce new attack surfaces. The design document's security analysis is accurate and complete.

**Final Recommendation**: Approve for implementation with the minor documentation improvements noted in Section 4.
