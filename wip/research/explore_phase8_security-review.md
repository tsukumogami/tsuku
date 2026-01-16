# Phase 8 Security Review: Libtool Zig CC

## Attack Vectors Analysis

### Considered Attack Vectors in the Design

The design document identifies four security categories:
1. Download Verification - marked "Not applicable"
2. Execution Isolation - marked "Low risk"
3. Supply Chain Risks - marked "Not applicable"
4. User Data Exposure - marked "Not applicable"

### Additional Attack Vectors Not Considered

**1. Path Injection via Wrapper Arguments**

The proposed wrapper script uses a `for` loop to iterate over `$@` and match arguments:
```bash
for arg in "$@"; do
  case "$arg" in
    -print-prog-name=ld)
      echo "/path/to/zig-cc-wrapper/ld"
      exit 0
      ;;
  esac
done
```

This is generally safe, but there are edge cases:
- **Argument splitting attacks**: If a malicious build system passes carefully crafted arguments like `-print-prog-name=ld && malicious-command`, the shell's `case` statement handles this safely since it's string comparison, not command execution.
- **Newline injection in echoed path**: The path printed comes from Go's `fmt.Sprintf` with values from `filepath.Join`, which should be sanitized. However, if `wrapperDir` contains newlines or shell metacharacters, the echoed path could be manipulated.

**Risk**: LOW - The path is constructed server-side during wrapper generation, not from user input at wrapper execution time.

**2. Wrapper Script Overwrite/TOCTOU**

The design doesn't address the scenario where an attacker could:
- Overwrite the `ld` wrapper between when the path is echoed and when libtool executes it
- Replace the cc wrapper itself between tsuku installation and build execution

**Risk**: MEDIUM - This requires write access to `$TSUKU_HOME/tools/zig-cc-wrapper/`, which typically requires the same user privileges as the build process. However, in shared environments or compromised systems, this could be exploited.

**3. Symlink Following Attacks**

The design uses direct file paths returned from `filepath.Join`. If an attacker creates a symlink at any path component, the wrapper could point to an unexpected binary.

**Risk**: LOW - The wrapper directory is created by tsuku with 0755 permissions, and the user controls `$TSUKU_HOME`.

**4. Build System Trust Assumption**

The wrapper implicitly trusts that libtool (and the build system using it) is legitimate. A malicious configure script could:
- Query `-print-prog-name` for reconnaissance about the build environment
- Use the returned path to understand the user's tsuku installation layout

**Risk**: LOW - This is information disclosure, not code execution. Build systems already have full code execution capability.

**5. ld Wrapper Execution Context**

When libtool calls the returned ld path, it executes with the same environment as the build. The ld wrapper unconditionally forwards all arguments to `zig ld.lld`. There's no validation that:
- The arguments are reasonable linker flags
- The files being linked are from the expected build directory

**Risk**: LOW - This is expected behavior for a compiler wrapper. Malicious builds can already execute arbitrary code.

## Mitigation Assessment

### Current Mitigations Analysis

**"Download Verification - Not applicable"**
- **Assessment**: ADEQUATE. The change modifies wrapper script generation, not download behavior. The zig toolchain download process is unchanged.
- **Note**: The zig recipe (`zig.toml`) does NOT include checksum verification - it uses only post-install version verification. This is a pre-existing concern, not introduced by this design.

**"Execution Isolation - Low risk"**
- **Assessment**: ADEQUATE. The wrapper scripts run with the same permissions as before. The new shell logic is purely string comparison and path echoing, with no additional privilege escalation.
- **Gap**: The design doesn't mention that the wrapper creates wrapper scripts with 0755 permissions, which means any user with access to the tsuku home directory could read (but not modify without write access) the wrapper scripts.

**"Supply Chain Risks - Not applicable"**
- **Assessment**: ADEQUATE. No new external dependencies are introduced. All paths are internal to tsuku.

**"User Data Exposure - Not applicable"**
- **Assessment**: ADEQUATE. The wrapper modification doesn't access or transmit any user data.

### Missing Mitigations

**1. Input Validation on wrapperDir**

While unlikely to be exploited, the wrapper generation code should validate that `wrapperDir` doesn't contain shell metacharacters or newlines:

```go
if strings.ContainsAny(wrapperDir, "\n\r$`\"'\\") {
    return fmt.Errorf("wrapper directory contains invalid characters: %s", wrapperDir)
}
```

**Severity**: LOW - The wrapperDir comes from tsuku's internal path construction.

**2. Wrapper Script Permissions Hardening**

The design mentions wrappers are created with 0755 but doesn't discuss whether 0700 would be more appropriate for single-user installations.

**Severity**: LOW - 0755 is standard for executables and matches system conventions.

## Residual Risk

### Risks That Should Be Escalated

**1. Pre-existing: Zig Download Lacks Cryptographic Verification**

The zig recipe (`internal/recipe/recipes/z/zig.toml`) uses `download_archive` without `checksum_url` or `signature_url`. This means:
- The download relies solely on HTTPS transport security
- A MITM attack or compromised CDN could deliver malicious binaries
- Post-install verification (`zig version`) only confirms basic functionality, not integrity

This is NOT introduced by this design but IS relevant because zig is being used as a compiler fallback. A compromised zig binary could inject malicious code into any software built with tsuku's zig cc wrapper.

**Recommendation**: Escalate to product security. Consider adding zig's published SHA256 checksums to the recipe.

**2. Accepted: Partial GCC Compatibility Surface**

The design explicitly accepts that only `-print-prog-name=ld` is handled. Other introspection flags like:
- `-print-search-dirs`
- `-print-file-name`
- `-print-prog-name=ar` (design includes this proactively)
- `-v` (version/verbose, often used for detection)

Could cause failures or unexpected behavior in edge cases.

**Recommendation**: Accept with monitoring. Track issues related to zig cc compatibility.

### Risks Appropriately Accepted

1. Wrapper complexity increase (6 lines of shell) - appropriately scoped
2. Build system reconnaissance potential - inherent to any compiler wrapper
3. Symlink/TOCTOU concerns - mitigated by user-controlled home directory

## "Not Applicable" Justification Review

### Download Verification: VALID

The justification states: "This change modifies wrapper script generation, not download behavior."

**Verdict**: Correct. The download path for zig is completely separate from this change. The wrapper scripts are generated locally, not downloaded.

**Caveat**: The underlying zig download security should be reviewed separately (see Residual Risk section).

### Supply Chain Risks: VALID

The justification states: "No new external dependencies are introduced. The ld wrapper path is internal to tsuku's $TSUKU_HOME/tools/zig-cc-wrapper/ directory."

**Verdict**: Correct. All paths are constructed from tsuku-controlled locations. No new package sources, URLs, or external code is introduced.

### User Data Exposure: VALID

The justification states: "The wrapper modification doesn't access or transmit any user data. It returns a static path when a specific flag is detected."

**Verdict**: Correct. The wrapper only:
1. Reads its command-line arguments (from the build system)
2. Echoes a static path to stdout
3. Exits

No user files are read, no network requests are made, no telemetry is collected.

### Execution Isolation: APPROPRIATELY CATEGORIZED

The justification states: "Low risk. The wrapper scripts run with the same permissions as before."

**Verdict**: Appropriate risk level. The new code is:
- String comparison (`case "$arg"`)
- Path echoing (`echo "..."`)
- Clean exit (`exit 0`)

No privilege changes, no file system modifications, no network access.

## Recommendations

### High Priority

1. **Add zig checksum verification** (separate from this design)
   - Update `internal/recipe/recipes/z/zig.toml` to include `checksum_url`
   - Zig publishes SHA256 checksums at `https://ziglang.org/download/{version}/zig-{arch}-{os}-{version}.tar.xz.sha256`
   - This hardens the entire zig cc pathway, not just the libtool compatibility

### Medium Priority

2. **Add defensive comment in wrapper generation**
   - Document in `setupZigWrappers()` that the paths used in `fmt.Sprintf` must not contain shell metacharacters
   - Consider adding a runtime assertion: `if strings.ContainsAny(wrapperDir, "\n\r") { ... }`

3. **Consider `-print-prog-name=*` wildcard handling**
   - Instead of handling `ld`, `ar`, `ranlib` individually, consider:
   ```bash
   case "$arg" in
     -print-prog-name=*)
       tool="${arg#-print-prog-name=}"
       toolpath="/path/to/zig-cc-wrapper/$tool"
       if [ -x "$toolpath" ]; then
         echo "$toolpath"
         exit 0
       fi
       ;;
   esac
   ```
   - This would automatically support any wrapper that exists in the directory
   - **Trade-off**: Increases complexity but improves forward compatibility

### Low Priority

4. **Add integration test for wrapper security**
   - Test that the wrapper correctly handles arguments containing shell metacharacters
   - Example: Test build with file named `test;rm -rf /;.c` (safely)

5. **Document security model**
   - Add a section to developer documentation explaining the trust boundaries for zig cc wrapper
   - Clarify that the wrapper trusts: tsuku's path construction, the build system, and zig itself

### Specific Code Feedback

The proposed wrapper in the design document:
```bash
#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    -print-prog-name=ld)
      echo "/path/to/zig-cc-wrapper/ld"
      exit 0
      ;;
  esac
done
exec "/path/to/zig" cc -fPIC -Wno-date-time "$@"
```

**Suggestion**: Add a comment explaining the security implication:
```bash
#!/bin/sh
# Handle GCC-specific introspection flags for libtool compatibility
# Security note: echoed paths are hardcoded at wrapper generation time,
# not derived from user input at execution time
for arg in "$@"; do
  case "$arg" in
    -print-prog-name=ld)
      echo "/path/to/zig-cc-wrapper/ld"
      exit 0
      ;;
  esac
done
exec "/path/to/zig" cc -fPIC -Wno-date-time "$@"
```

## Summary

The security considerations in the design document are **adequate for the scope of changes**. The "not applicable" justifications are valid. The "low risk" assessment for execution isolation is appropriate.

The primary security concern is not introduced by this design but is **exposed** by it: tsuku uses zig as a compiler fallback, but the zig download lacks cryptographic verification. This should be addressed separately.

The design can proceed with the following conditions:
1. Accept the residual risk of partial GCC compatibility
2. Track as a separate issue: adding checksum verification to the zig recipe
3. Consider the medium-priority defensive measures for the implementation phase
