# Design: Post-Install Checksum Pinning (Layer 3)

- **Status**: Proposed
- **Issue**: #203
- **Author**: @dangazineu
- **Created**: 2025-12-27
- **Upstream Design**: docs/DESIGN-version-verification.md (Phase 6)

## Context and Problem Statement

After successful installation, tsuku should store checksums of installed binaries and verify them on subsequent runs to detect tampering, corruption, or unauthorized modifications.

**Current situation:**

The existing verification system (Layer 2) confirms that the correct version was installed by checking version output. However, it cannot detect:
- Post-installation binary modification (malware injection)
- Disk corruption affecting executables
- Accidental file overwrites

**What exists today:**

1. **Download-time checksums**: The `download_file` action verifies SHA256 checksums against recipe-specified values
2. **Plan storage**: `state.json` stores full installation plans with download checksums in `Plan.Steps[].Checksum`
3. **Binary tracking**: `VersionState.Binaries` lists binary names provided by each installed version

**Gap:** No mechanism to verify that installed binaries remain unchanged after installation.

### Scope

**In scope:**
- Compute SHA256 of installed binaries after successful installation
- Store checksums in `state.json`
- `tsuku verify` recomputes and compares against stored values
- Handle tool updates (recompute checksums on upgrade)
- Report tamper detection clearly

**Out of scope:**
- Checksum verification for non-binary files (libraries, configs, data)
- Periodic automatic verification (`tsuku doctor` integration is future work)
- Checksum pinning for files outside `$TSUKU_HOME/tools/`

## Decision Drivers

1. **Security**: Detect post-installation tampering with minimal performance overhead
2. **Simplicity**: Leverage existing infrastructure (SHA256, state.json, verify command)
3. **Performance**: Avoid unnecessary I/O during normal operations
4. **Backward compatibility**: Existing installations should gracefully degrade
5. **User clarity**: Verification failures should be actionable

## Upstream Design Reference

The upstream design (DESIGN-version-verification.md Phase 6, lines 464-478) provides high-level scope:

> **Goal**: Detect post-installation tampering by storing and verifying checksums of installed binaries.
>
> **Scope**:
> - Compute SHA256 of installed binaries after successful installation
> - Store checksums in `state.json`
> - `tsuku verify` recomputes and compares against stored values
> - Detect tampering, corruption, or unauthorized modifications

The upstream design leaves several decisions unresolved. This document addresses them.

## Decision Framework

### Decision 1: What Files to Checksum

**Question**: Should we checksum only binaries or all installed files?

**Options considered**:

| Option | Scope | Storage Size | Verification Time | Security Coverage |
|--------|-------|--------------|-------------------|-------------------|
| A. Binaries only | Files in `$TSUKU_HOME/bin/` symlink targets | Small (~1-5 entries) | Fast | Partial |
| B. All executables | All files with execute permission | Medium | Medium | Better |
| C. All files | Everything in tool directory | Large | Slow | Complete |
| D. Configurable | Recipe specifies which files to verify | Flexible | Varies | Varies |

**Decision**: **Option A - Binaries only**

**Rationale**:
- The `Binaries` list is already tracked in `VersionState.Binaries`
- Binary files are the primary attack surface for code execution
- Keeps verification fast (<100ms for typical tools)
- Libraries and data files are less likely tampering targets
- Can expand to Option B later without breaking changes

**Trade-off accepted**: We don't detect tampering of library files or shell completions. This is acceptable because:
- Binaries are the most security-sensitive files
- Library tampering would likely be detected through version verification (Layer 2) or functional testing (Layer 4)

---

### Decision 2: When to Compute Checksums

**Question**: At what point during installation should checksums be computed?

**Options considered**:

| Option | Timing | Complexity | Risk |
|--------|--------|------------|------|
| A. After `install_binaries` action | In action executor | Medium | May miss late modifications |
| B. After all actions complete | Before state save | Low | Captures final state |
| C. During state save | In state manager | Low | Tight coupling |
| D. Lazy on first verify | In verify command | Low | First verify slower |

**Decision**: **Option B - After all actions complete, before state save**

**Rationale**:
- Captures the final installed state after all recipe actions
- No risk of missing modifications from later actions
- Natural integration point in the install flow
- Single location for checksum computation logic

**Implementation location**: `Manager.Install()` in `internal/install/manager.go`, after `executeAllActions()` returns successfully but before `saveToolState()`.

---

### Decision 3: Storage Schema

**Question**: How should checksums be stored in `state.json`?

**Options considered**:

| Option | Schema Location | Structure |
|--------|-----------------|-----------|
| A. In VersionState | `VersionState.BinaryChecksums map[string]string` | Flat map |
| B. In Plan | `PlanStep.InstalledChecksum` | Alongside download checksums |
| C. New top-level | `BinaryIntegrity map[tool][version][path]` | Separate concern |
| D. Extended Binaries | `VersionState.Binaries []BinaryInfo{Name, Checksum}` | Replace string slice |

**Decision**: **Option A - Add BinaryChecksums field to VersionState**

```go
type VersionState struct {
    Requested        string            // What user requested
    Binaries         []string          // Binary names
    BinaryChecksums  map[string]string // path -> SHA256 hex
    InstalledAt      time.Time
    Plan             *Plan
}
```

**Rationale**:
- Simple flat map of relative paths to checksums
- Colocated with related binary metadata
- Easy to query during verification
- Backward compatible (missing field = no checksums = skip verification)

**Path format**: Relative to tool install directory (e.g., `bin/jq`, `bin/rg`)

---

### Decision 4: Verification Behavior

**Question**: How should `tsuku verify` behave when checksums are missing or mismatched?

**Options considered**:

| Scenario | Option A: Strict | Option B: Graceful | Option C: Configurable |
|----------|-----------------|-------------------|------------------------|
| No checksums stored | Error | Skip with note | Flag-controlled |
| Checksum mismatch | Error | Warning | Flag-controlled |
| File missing | Error | Error | Error |
| New file in directory | Ignore | Ignore | Configurable |

**Decision**: **Option B - Graceful with clear messaging**

**Rationale**:
- Existing installations lack checksums; strict mode would break them
- Mismatch should warn loudly but not fail (user may have intentionally modified)
- Missing file is always an error (corrupted installation)
- Supports gradual rollout without forcing reinstalls

**Output format**:
```
$ tsuku verify jq
jq 1.7
  Version: OK (jq-1.7)
  Path: OK (/home/user/.tsuku/bin/jq -> ../tools/jq-1.7/bin/jq)
  Integrity: OK (1 binary verified)

$ tsuku verify jq  # with tampered binary
jq 1.7
  Version: OK (jq-1.7)
  Path: OK (/home/user/.tsuku/bin/jq -> ../tools/jq-1.7/bin/jq)
  Integrity: MODIFIED
    bin/jq: expected abc123..., got def456...
    WARNING: Binary may have been modified after installation.
    Run 'tsuku install jq --reinstall' to restore original.

$ tsuku verify jq  # old installation without checksums
jq 1.6
  Version: OK (jq-1.6)
  Path: OK (/home/user/.tsuku/bin/jq -> ../tools/jq-1.6/bin/jq)
  Integrity: SKIPPED (no stored checksums - pre-v0.X installation)
```

---

### Decision 5: Reinstall Behavior

**Question**: What happens to checksums on reinstall or upgrade?

**Options considered**:

| Scenario | Behavior |
|----------|----------|
| `tsuku install jq` (same version) | Recompute and overwrite checksums |
| `tsuku install jq@1.8` (upgrade) | New version gets new checksums |
| `tsuku install jq@1.7` (downgrade) | Old version may have stale checksums from previous install |
| `tsuku install jq --reinstall` | Force reinstall, recompute checksums |

**Decision**: Always compute checksums for the version being installed.

**Rationale**: Checksums represent the state immediately after installation. If a version is installed multiple times, the latest install's checksums are the reference. This handles both fresh installs and reinstalls uniformly.

---

### Decision 6: Hash Algorithm

**Question**: Which hash algorithm to use?

**Options considered**:

| Option | Security | Performance | Ecosystem |
|--------|----------|-------------|-----------|
| SHA256 | Good | Fast | Standard in package managers |
| SHA512 | Better | Similar | Less common |
| BLAKE3 | Excellent | Faster | Emerging, requires new dependency |

**Decision**: **SHA256**

**Rationale**:
- Already used throughout tsuku for download verification
- Existing `VerifyChecksum()` function in `internal/actions/util.go`
- Standard format, easy to verify externally
- No new dependencies

## Solution Architecture

### Overview

```
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   Install Flow   │────▶│   State Save     │────▶│   Verify Flow    │
│                  │     │                  │     │                  │
│ 1. Download      │     │ 4. Compute       │     │ 6. Load stored   │
│ 2. Extract       │     │    checksums     │     │    checksums     │
│ 3. Install bins  │     │ 5. Save to       │     │ 7. Recompute     │
│                  │     │    state.json    │     │ 8. Compare       │
└──────────────────┘     └──────────────────┘     └──────────────────┘
```

### Data Flow

1. **Installation completes** - All recipe actions finish successfully
2. **Compute checksums** - Walk installed binaries, compute SHA256 for each
3. **Store in state** - Add `BinaryChecksums` map to `VersionState`
4. **Persist** - Save to `state.json` as normal
5. **Verify command** - Load checksums, recompute, compare
6. **Report** - Display verification status with clear actionable messages

### Component Changes

**`internal/install/state.go`:**
```go
type VersionState struct {
    Requested       string            `json:"requested,omitempty"`
    Binaries        []string          `json:"binaries,omitempty"`
    BinaryChecksums map[string]string `json:"binary_checksums,omitempty"` // NEW
    InstalledAt     time.Time         `json:"installed_at"`
    Plan            *Plan             `json:"plan,omitempty"`
}
```

**`internal/install/checksum.go`** (new file):
```go
// ComputeBinaryChecksums walks the installed tool directory and computes
// SHA256 checksums for each binary file.
func ComputeBinaryChecksums(toolDir string, binaries []string) (map[string]string, error)

// VerifyBinaryChecksums recomputes checksums and compares against stored values.
// Returns a slice of mismatches, or nil if all match.
func VerifyBinaryChecksums(toolDir string, stored map[string]string) ([]ChecksumMismatch, error)

type ChecksumMismatch struct {
    Path     string
    Expected string
    Actual   string
    Error    error // non-nil if file is missing or unreadable
}
```

**`internal/install/manager.go`:**
- After `executeAllActions()`, before `saveToolState()`:
  ```go
  checksums, err := ComputeBinaryChecksums(toolDir, binaries)
  if err != nil {
      return fmt.Errorf("computing binary checksums: %w", err)
  }
  versionState.BinaryChecksums = checksums
  ```

**`cmd/tsuku/verify.go`:**
- After existing version/PATH verification:
  ```go
  // Phase 3: Binary integrity check
  if versionState.BinaryChecksums != nil {
      mismatches, err := install.VerifyBinaryChecksums(toolDir, versionState.BinaryChecksums)
      // Report results
  } else {
      fmt.Printf("  Integrity: SKIPPED (no stored checksums)\n")
  }
  ```

### Edge Cases

| Scenario | Handling |
|----------|----------|
| Binary is symlink to another binary | Resolve symlink, checksum target |
| Binary is script (not ELF/Mach-O) | Hash script content (same algorithm) |
| Binary has been deleted | Report as `ERROR: binary missing` |
| Permissions changed but content same | Pass (we verify content, not metadata) |
| Tool directory moved | Path is relative; checksums still valid |
| Large binary (>100MB) | Same algorithm; may take longer but still fast |

## Implementation Approach

### Phase 1: Checksum Computation (Core)

1. Create `internal/install/checksum.go` with:
   - `ComputeBinaryChecksums(toolDir, binaries)` function
   - Resolve symlinks to get actual file paths
   - Compute SHA256 using existing `util.ComputeChecksum()` pattern
   - Return map of relative path to hex-encoded checksum

2. Add unit tests for:
   - Normal binary files
   - Symlinks within tool directory
   - Missing binaries (error case)
   - Permission denied (error case)

### Phase 2: State Schema Extension

1. Add `BinaryChecksums` field to `VersionState` in `state.go`
2. Ensure JSON serialization handles nil/empty map correctly
3. Add backward compatibility test: load old state.json without field

### Phase 3: Install Integration

1. In `Manager.Install()`, after actions complete:
   - Get list of binaries from recipe/action results
   - Compute checksums
   - Store in `VersionState.BinaryChecksums`

2. Add integration test: install tool, verify checksums stored

### Phase 4: Verify Command Extension

1. Add integrity verification step to `verify.go`:
   - Load stored checksums from state
   - Recompute current checksums
   - Compare and report mismatches

2. Handle graceful degradation:
   - No checksums stored: "SKIPPED (pre-vX installation)"
   - Mismatch: "MODIFIED" with details and remediation hint

3. Add tests for all verification scenarios

### Phase 5: Documentation

1. Update `tsuku verify --help` to mention integrity checking
2. Add section to user guide on tamper detection
3. Document the "reinstall to fix" workflow

## Security Considerations

### Download Verification

**Not applicable** - This feature does not download external artifacts. It operates solely on files already present in `$TSUKU_HOME/tools/` after installation completes. Download verification is handled by Layer 1 (cryptographic verification) during the install phase.

### Execution Isolation

**Scope**: This feature requires read access to:
- `$TSUKU_HOME/tools/{name}-{version}/` - to compute checksums of installed binaries
- `$TSUKU_HOME/state.json` - to read/write stored checksums

**Permissions**: No elevated privileges required. Runs with same user permissions as tsuku process.

**Risks**:
- None beyond existing install/verify permissions
- No network access required
- No privilege escalation possible

### Supply Chain Risks

**Threat model**:

What this feature protects against:
- Post-installation binary modification by malware
- Accidental file corruption
- Unauthorized modifications by other users (on shared systems)
- Supply chain attacks that modify binaries after download verification passed

What this feature does NOT protect against:
- Compromised download that passes initial checksum (handled by Layer 1)
- Modification of state.json itself (stored alongside binaries with same permissions)
- Kernel-level attacks that intercept file reads
- Time-of-check to time-of-use (TOCTOU) attacks during verification

**State file trust concern**: Attacker modifies both binary and its checksum in state.json.

**Mitigation**: None in scope. The state file is in `$TSUKU_HOME/state.json`, protected by the same filesystem permissions as the binaries. If an attacker can write to state.json, they can also write to binaries.

**Future enhancement**: Signed state file using a key stored outside `$TSUKU_HOME`. Out of scope for this design.

### User Data Exposure

**Not applicable** - This feature does not access or transmit user data. It only:
- Reads binary files to compute checksums
- Reads/writes checksums to local state.json
- No data is sent externally
- No telemetry or analytics

### Additional Security Analysis

**Checksum collision resistance**: No known practical attacks against SHA256 for this use case. Collision attacks require chosen-prefix attacks, not relevant for detecting tampering where attacker cannot control original binary.

**Performance impact**:
- Installation: Additional ~10-50ms per binary for checksum computation. Negligible compared to download/extraction time.
- Verification: Same computation during `tsuku verify`. Acceptable for explicit verification command.
- Normal usage: No impact on `tsuku install`, `tsuku list`, or tool execution.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| State file tampering | Same filesystem permissions as binaries | Attacker with write access can modify both |
| SHA256 collision | No practical attack known for tampering detection | Theoretical future weakness |
| TOCTOU during verify | None (read-only verification) | Minimal; race condition would require timing attack during verify |
| Partial coverage (binaries only) | Document limitation clearly | Libraries/configs not protected |

## Consequences

### Positive

- **Tamper detection**: Users can verify installed binaries haven't been modified
- **Corruption detection**: Catches disk corruption affecting executables
- **Minimal overhead**: Only binaries are checksummed, not entire tool directory
- **Backward compatible**: Old installations work without checksums
- **Leverages existing code**: Reuses SHA256 implementation from download verification

### Negative

- **State file size increase**: ~100-200 bytes per installed version
- **Not foolproof**: Attacker with write access can modify both binary and checksum
- **Partial coverage**: Only protects binaries, not libraries or configs

### Mitigations

- State file size is minor compared to Plan storage (already ~1-5KB per version)
- Clear documentation about security model and limitations
- Future work can extend to full directory checksums if needed
