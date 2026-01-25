---
status: Current
problem: Recipe verification currently uses inconsistent patterns to match version output, causing mismatches between version provider formats and tool output formats, weak verification for tools without version support, and inadequate validation coverage in CI.
decision: Implement version format transforms and an output mode fallback to provide flexible yet robust verification that covers all tools while maintaining security through required justification fields and validator enforcement.
rationale: Version format transforms (semver, strip_v, etc.) cover ~95% of real-world cases with explicit configuration, while output mode with required reason fields ensures weak verification is documented and intentional. This layered approach provides the foundation for future verification methods like functional testing and cryptographic verification.
---

# Design: Flexible Recipe Verification

## Status

Current
- **Issue**: #192
- **Author**: @dangazineu
- **Created**: 2025-12-06

## Context and Problem Statement

Tsuku recipes include a `[verify]` section that validates tool installation by running a command and checking its output against a pattern. The pattern can include `{version}` which expands to the resolved version string.

**Current situation:**

Of 134 recipes in the registry:
- ~60 use clean `{version}` matching (tool outputs exact version)
- ~25 use prefixed patterns like `"v{version}"` or `"tool {version}"`
- ~40 use tool name only or partial patterns (no version verification)
- ~10 use partial version checks like `"1."` or `"Version:"`
- 3 use empty patterns (just verify command succeeds)

**The problem:**

1. **Version format mismatch**: GitHub tags often differ from tool output formats:
   - Tag `biome@2.3.8` vs output `Version: 2.3.8`
   - Tag `v1.29.0` vs output `1.29.0`
   - Tag `2.4.0-0` vs output `2.4.0`

2. **No version support**: Some tools lack `--version` flags entirely:
   - `gofumpt` outputs usage, not version
   - Some tools have no version reporting

3. **Weak fallbacks**: Current workarounds provide poor validation:
   - Empty patterns only verify command succeeds
   - Tool name patterns don't verify correct version installed
   - Partial patterns like `"1."` are too permissive

**Why this matters now:**

The validator now runs `tsuku validate --strict` in CI (#184). Many recipes fail strict validation or use workarounds that provide weak installation guarantees. Users deserve confidence that the correct version was installed.

**Desired outcome:**

Recipes should verify the exact installed version matches the requested version, while providing clear error messages when version extraction fails. For tools that don't support version output, recipes should be able to specify alternative verification that still provides meaningful installation confidence.

### Scope

**In scope:**
- Version format transformation for `{version}` placeholder
- Alternative verification methods for tools without version output
- Validator awareness of verification strategies
- Backward compatibility with existing recipes

**Out of scope:**
- Cryptographic verification (checksums, signatures)
- Runtime version detection outside verify step
- Changes to version resolution logic

## Decision Drivers

1. **Version accuracy**: Users should know the exact version installed
2. **Recipe simplicity**: Common cases should be simple, edge cases shouldn't complicate normal usage
3. **Sensible defaults**: Default behavior should work for most recipes without configuration
4. **Fail-safe defaults**: Missing or incorrect verification should be obvious, not silent
5. **Minimal configuration**: Prefer convention over configuration where possible
6. **Validation coverage**: CI should catch recipes with inadequate verification

## Considered Options

### Option 1: Automatic Heuristic-Based Version Normalization

Apply automatic heuristics to normalize version strings without explicit configuration. The system would detect common patterns (leading `v`, tool prefixes like `biome@`, etc.) and strip them automatically before matching.

**Pros:**
- Zero configuration for most recipes
- Works out of the box without recipe authors needing to learn new concepts
- Reduces boilerplate in recipe definitions

**Cons:**
- Magic behavior that is hard to debug when it fails
- Heuristics may incorrectly transform versions in edge cases
- Package managers require correctness over convenience; implicit behavior can mask real issues
- Difficult to predict what transformation will be applied to a given version string

### Option 2: Explicit Format Transforms with Verification Modes

Introduce explicit `version_format` transforms (semver, strip_v, raw) and verification `mode` fields (version, output) that recipe authors configure intentionally. Defaults remain unchanged (raw format, version mode) so existing recipes continue to work.

**Pros:**
- Predictable, self-documenting behavior
- Recipe authors control exactly how version strings are normalized
- Fails explicitly rather than silently applying wrong transforms
- Supports fallback for tools without version output via output mode with required justification
- Enables validator to enforce documentation of weak verification

**Cons:**
- Introduces two new concepts (mode and version_format) that recipe authors must learn
- More verbose recipes for edge cases with unusual version formats
- Requires migration effort for ~40 existing recipes with version mismatches

### Option 3: Pattern-Side Inline Transforms

Allow transforms to be specified inline within the pattern using pipe syntax, such as `{version|semver}` or `{version|strip_v}`. The transformation would be co-located with its usage.

**Pros:**
- Transformation logic is visible directly in the pattern where it's applied
- No separate field needed
- Familiar syntax for users accustomed to template languages

**Cons:**
- Requires custom parsing of the pattern syntax
- Less discoverable than a dedicated field
- Harder to validate and provide good error messages
- Doesn't address the fallback problem for tools without version support

## External Research

### Homebrew

**Approach**: Homebrew uses a `test do` block in formulas that runs arbitrary shell commands after installation. The [Formula Cookbook](https://docs.brew.sh/Formula-Cookbook) encourages functional tests over version checks:

> "We want tests that don't require any user input and test the basic functionality of the application. For example `foo build-foo input.foo` is a good test and (despite their widespread use) `foo --version` and `foo --help` are bad tests. **However, a bad test is better than no test at all.**"

**What Homebrew tests verify (examples from homebrew-core):**

- **jq**: Parses JSON and extracts fields: `pipe_output("#{bin}/jq .bar", '{"foo":1, "bar":2}')`
- **ripgrep**: Searches file contents: creates test file, runs `rg "pattern" testpath`
- **tinyxml2**: Compiles and links against library: writes C++ code, compiles with `-ltinyxml2`, executes binary
- **cmake**: Generates build files: writes CMakeLists.txt, runs `cmake .`

**Trade-offs**:
- Pro: Tests actual functionality (parsing, searching, linking), not just presence
- Pro: Catches runtime issues version checks miss (segfaults, missing dependencies)
- Pro: Flexible - any shell command can be a test
- Con: Requires writing custom tests per formula (human effort)
- Con: Many formulas lack tests - they're optional in Homebrew

**Relevance to tsuku**: Homebrew and tsuku have complementary verification goals:

| Verification Type | What it guarantees | When to use |
|-------------------|-------------------|-------------|
| **Functional test** (Homebrew-style) | Tool performs core functionality correctly | When test exists or can be written |
| **Version verification** (tsuku default) | Correct version installed, not compromised/outdated | Automatable baseline for all tools |
| **Exit code check** (fallback) | Tool runs without crashing | When tool lacks version output |

**Key insight**: Version checks aren't useless - they're insufficient *as the sole verification*. Homebrew acknowledges "a bad test is better than no test" and many formulas use version checks as fallback. Tsuku should support both: version verification as automatable baseline, functional tests as stronger optional layer.

**Opportunity**: Homebrew's test corpus could be imported. For tools with Homebrew tests, tsuku recipes could offer functional verification mode with tests adapted from formulas. This provides stronger guarantees without requiring recipe authors to write tests from scratch.

### asdf / mise

**Approach**: asdf plugins define version detection via a `list-all` script that fetches available versions, but verification is implicit - if the tool runs, it's considered installed. [mise](https://mise.jdx.dev/) (asdf successor) adds native software verification using Cosign/Minisign signatures and SLSA provenance for supported backends.

**Trade-offs**:
- Pro: Simple - no explicit verification step
- Pro: mise adds cryptographic verification for aqua tools
- Con: No version output validation
- Con: Can't detect partial or corrupted installs

**Relevance to tsuku**: The cryptographic verification in mise is out of scope, but their approach of implicit verification (tool runs = success) is similar to tsuku's empty pattern fallback.

### Nix

**Approach**: Nix has two verification phases: `checkPhase` (runs tests before install) and `installCheckPhase` (runs after install). From [nixpkgs docs](https://ryantm.github.io/nixpkgs/stdenv/stdenv/):
> "Version info and natively compiled extensions generally only exist in the install directory, and thus can cause issues."

Python packages specifically run `checkPhase` as `installCheckPhase` because version info only exists post-install.

**Trade-offs**:
- Pro: Separation of build-time vs install-time checks
- Pro: Explicit about when version info is available
- Con: Complex two-phase system
- Con: Many tests need disabling due to sandbox restrictions

**Relevance to tsuku**: The insight that version info is only available post-install aligns with tsuku's verify step. For nix-based recipes, tsuku could potentially leverage `installCheckPhase` tests from nixpkgs derivations.

### Research Summary

**Common patterns:**
- All systems run verification AFTER installation (not at build time)
- Multiple verification strengths exist: functional tests (strongest) → version checks (baseline) → exit code (weakest)
- Cryptographic verification is separate from output verification
- Test corpus reuse is valuable (Homebrew has ~5000 formulas with tests)

**Key differences:**
- **Homebrew**: Functional tests preferred, but acknowledges version checks have value ("bad test better than no test")
- **Nix**: Distinguishes build-time (`checkPhase`) vs install-time (`installCheckPhase`) checks
- **mise**: Adds optional cryptographic verification layer (Cosign/Minisign)
- **asdf**: No explicit verification, relies on tool execution as proof of success

**Verification hierarchy across systems:**

| Strength | What it verifies | Homebrew | Nix | Tsuku (proposed) |
|----------|------------------|----------|-----|------------------|
| Strong | Core functionality works | `test do` block | `installCheckPhase` | `mode = "functional"` (v2) |
| Medium | Correct version installed | Discouraged but used | Version assertions | `mode = "version"` (default) |
| Weak | Tool outputs expected pattern | Fallback | - | `mode = "output"` |

**Implications for tsuku:**
1. **Verification tiers** - Support version verification as automatable baseline (v1), output mode as weak fallback (v1), functional tests as strongest option (v2)
2. **Homebrew corpus reuse** - Import tests from Homebrew formulas for tools that have them (reduces recipe author burden) - v2
3. **Version format transforms** - Needed for version verification tier (separate concern from verification strategy)
4. **Keep it simple** - One verification step per recipe, not multiple phases like Nix
5. **Clear opt-in** - Output mode requires explicit `reason` field to document why version verification isn't possible

## Decision Framework

The verification design requires several **independent decisions**. These are not mutually exclusive options - they address different concerns that compose together.

### Decision 1: Version Format Transformation

**Question**: How should tsuku handle version string format mismatches between providers and tool output?

**Context**: Version providers (GitHub, npm, etc.) return version strings like `v1.29.0` or `biome@2.3.8`, but tools often output different formats like `1.29.0` or `Version: 2.3.8`.

**Options considered**:

| Option | Approach | Pros | Cons |
|--------|----------|------|------|
| A. Explicit transforms | `version_format = "semver"` | Predictable, self-documenting | Requires learning syntax |
| B. Automatic heuristics | Strip `v`, extract semver | Zero config for common cases | Magic, hard to debug |
| C. Multiple placeholders | `{version}` vs `{version_raw}` | Flexible | Two concepts to learn |
| D. Pattern-side transforms | `{version\|semver}` | Co-located with usage | Custom parsing needed |

**Decision**: **Option A - Explicit transforms**

Use four predefined formats that cover ~95% of real-world cases:
- `semver` - Extract `X.Y.Z` from any format (`biome@2.3.8` → `2.3.8`)
- `semver_full` - Extract `X.Y.Z[-pre][+build]`
- `strip_v` - Remove leading `v` (`v1.2.3` → `1.2.3`)
- `raw` (default) - No transformation

**Rationale**: Package managers require correctness over convenience. Explicit transforms are self-documenting and fail predictably. The common formats cover most cases without requiring recipe authors to learn complex syntax.

---

### Decision 2: Fallback for Tools Without Version Output

**Question**: How should tsuku verify tools that don't support `--version`?

**Context**: Some tools (like `gofumpt`) don't have version flags. We need a fallback that provides some verification confidence.

**Options considered**:

| Option | Approach | Security | Practicality |
|--------|----------|----------|--------------|
| A. Exit code only | Just check command succeeds | Weak | High |
| B. Output pattern | Match expected output string | Medium | High |
| C. Require functional tests | Must write behavioral tests | Strong | Low |
| D. Skip verification | Allow opting out entirely | None | High |

**Decision**: **Option B - Output pattern with required justification**

For v1, tools without version output use `mode = "output"`:
```toml
[verify]
mode = "output"
command = "gofumpt -h"
pattern = "usage:"
reason = "Tool does not support --version flag"
```

**Rationale**: Output patterns provide better confidence than exit code alone, while remaining practical for recipe authors. The required `reason` field documents why version verification isn't possible and enables validator enforcement.

---

### Decision 3: Verification Mode Naming

**Question**: What should the verification modes be called in v1, given that we plan to add Homebrew-style functional tests later?

**Context**: The initial design used `mode = "functional"` for the weak fallback. But true functional testing (verifying tool behavior) is planned for future work.

**Options considered**:

| Option | v1 Modes | Future Compatibility |
|--------|----------|---------------------|
| A. Use "functional" now | `version`, `functional` | Confusing when real tests added |
| B. Rename to "output" | `version`, `output` | Clean - reserve "functional" for future |
| C. Split now | `version`, `output`, `functional` (reserved) | Clear but more complex |

**Decision**: **Option B - Use "output" for v1 fallback, reserve "functional"**

v1 modes:
- `version` (default) - Verify exact version via `{version}` pattern
- `output` - Custom pattern matching without version (weak fallback)

Future (v2+):
- `functional` - Homebrew-style behavioral tests (e.g., `echo '{"foo":1}' | jq .foo`)

**Rationale**: Names should match what they do. "Output" accurately describes "check command output matches pattern." Reserving "functional" prevents confusion when we add real behavioral testing later.

---

### Decision 4: Validator Strictness

**Question**: How should `--strict` validation handle recipes with weak verification?

**Options considered**:

| Option | Behavior |
|--------|----------|
| A. Errors only | Only error on invalid syntax |
| B. Warn on weak patterns | Warn if version mode lacks `{version}` |
| C. Require justification | Error if output mode lacks `reason` |

**Decision**: **Combination of B and C**

- Warn if `version` mode pattern doesn't contain `{version}` (likely mistake)
- Error if `output` mode lacks `reason` field (missing documentation)

**Rationale**: The `reason` field ensures weak verification is documented and intentional.

---

### Assumptions

The following assumptions underlie this design:

1. **Version source diversity**: Version strings come from various providers (GitHub, PyPI, npm, crates.io, goproxy). The solution must handle formats from all providers, not just GitHub tags.

2. **Install-time verification**: Verification happens immediately after installation, not lazily on first use. This catches failures early.

3. **Single verification per recipe**: Cross-platform differences are handled by platform-specific patterns within the same recipe, not multiple verification strategies.

4. **stdout capture**: The verify command's stdout is captured for pattern matching. Most tools output version to stdout; stderr handling is out of scope.

5. **Exit code semantics**: Non-zero exit codes indicate verification failure, regardless of output matching. Some tools may need special handling.

6. **String matching**: Pattern matching is substring-based (`strings.Contains`), not semver-aware comparison. The pattern must appear literally in output.

## Decision Outcome

Based on the decisions above, the v1 implementation provides:

1. **Version verification with explicit transforms** (Decision 1) - The default mode that works for ~95% of tools
2. **Output mode as fallback** (Decisions 2 & 3) - For tools without `--version`, with required justification
3. **Functional mode reserved** (Decision 3) - For future Homebrew-style behavioral tests

### Why Version Verification First

Version verification is the **foundation** of tsuku's verification strategy because:

1. **Universal applicability**: ~95% of tools support `--version` or equivalent, making it the most automatable baseline
2. **Supply chain detection**: Version checks detect wrong/outdated versions, rollback attacks, and some supply chain compromises
3. **No custom code required**: Unlike functional tests, version verification works without per-recipe test authoring

This is the **first verification method** we support, not the only one. Future work will add:
- **Functional testing** (stronger behavioral guarantees) - v2
- **Cryptographic verification** (supply chain integrity) - future
- **Checksum pinning** (tamper detection) - future

These are **complementary**, not competing alternatives - each addresses different concerns.

### Design Alignment with Decision Drivers

1. **Version accuracy** (Driver 1): Explicit transforms ensure the recipe author controls exactly how version strings are normalized. No heuristic guessing.

2. **Recipe simplicity** (Driver 2): Four predefined formats (`semver`, `semver_full`, `strip_v`, `raw`) cover ~95% of cases. Most recipes won't need transforms at all.

3. **Sensible defaults** (Driver 3): Default mode is `version` with `raw` format - recipes only need configuration for edge cases.

4. **Fail-safe defaults** (Driver 4): The default mode is `version`, requiring `{version}` in pattern. The `output` mode is a fallback for tools that genuinely cannot report version, not an alternative strategy.

5. **Validation coverage** (Driver 6): The validator enforces that:
   - `version` mode has `{version}` in pattern (warn if missing)
   - `output` mode has a `reason` explaining why version check isn't possible (error if missing)

### Trade-offs Accepted

1. **More verbose recipes for edge cases**: Tools with unusual version formats need explicit `version_format` configuration.

2. **Two new concepts**: Recipe authors may need to learn `version_format` (for transforms) and `mode` (for fallback). Most recipes need neither.

These are acceptable because:
- Verbosity is localized to the ~40 recipes with version mismatches
- The common case (tool outputs clean version) requires zero configuration

## Solution Architecture

### Overview

The solution adds two optional fields to the `[verify]` section:

1. **`mode`**: Declares the verification strategy (`version` or `output`; `functional` reserved for v2)
2. **`version_format`**: Specifies how to transform the version string before pattern expansion

### Recipe Format

```toml
# Default: version mode with no transformation
[verify]
command = "tool --version"
pattern = "{version}"

# Version mode with format transformation
[verify]
mode = "version"  # optional, this is the default
command = "biome --version"
pattern = "Version: {version}"
version_format = "semver"  # strips prefixes like "biome@", "v", extracts X.Y.Z

# Output mode: fallback for tools without --version
[verify]
mode = "output"
command = "gofumpt -h"
pattern = "usage:"
reason = "Tool does not support --version flag"

# Future (v2): Functional mode with Homebrew-style behavioral tests
# [verify]
# mode = "functional"
# command = "sh"
# args = ["-c", "echo '{\"foo\":1}' | jq .foo"]
# expected_output = "1"
# reason = "Verifies jq can parse JSON (from Homebrew test)"
```

### Verification Modes

| Mode | Purpose | Pattern | `{version}` | Strength |
|------|---------|---------|-------------|----------|
| `version` (default) | Verify exact version installed | Required, must contain `{version}` | Expanded from resolved version | Strong |
| `output` | Fallback for tools without version | Required | Not expanded | Weak |
| `functional` (v2) | Homebrew-style behavioral tests | TBD | Not expanded | Strongest |

### Version Format Transforms

The `version_format` field accepts:

| Format | Transformation | Example |
|--------|---------------|---------|
| `semver` | Extract `X.Y.Z` from any format | `biome@2.3.8` → `2.3.8`, `v1.2.3-rc.1` → `1.2.3` |
| `semver_full` | Extract `X.Y.Z[-prerelease][+build]` | `v1.2.3-rc.1+build` → `1.2.3-rc.1+build` |
| `strip_v` | Remove leading `v` | `v1.2.3` → `1.2.3` |
| `raw` | No transformation (explicit) | `go1.21.0` → `go1.21.0` |

Custom transforms can be added later (e.g., `strip_prefix:biome@`) but the common cases above cover ~95% of needs.

### Edge Cases

- **`version_format` with output mode**: If `mode = "output"` and `version_format` is set, the format is ignored (no `{version}` to expand)
- **Pattern without `{version}` in version mode**: Validator warns but allows; pattern is matched literally
- **Unknown `version_format`**: Treated as `raw` with warning; allows forward compatibility
- **Transform fails to extract version**: Falls back to raw version with warning
- **`mode = "functional"` in v1**: Validator errors with message to use `output` mode instead (reserved for v2)

### Component Changes

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Recipe Types   │────▶│    Validator     │────▶│    Executor     │
│  (types.go)     │     │  (validator.go)  │     │  (executor.go)  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
  Add mode,             Enforce mode-         Apply version_format
  version_format,       specific rules        transform before
  reason fields                               pattern expansion
```

### Data Flow

1. **Parse**: Recipe TOML parsed, new fields populated in `VerifySection`
2. **Validate**: Validator checks mode-specific requirements
3. **Execute**: Executor applies `version_format` transform to resolved version
4. **Match**: Transformed version substituted into pattern, matched against output

## Implementation Approach

### Phase 1: Type Definitions and Parsing

- Add `Mode`, `VersionFormat`, `Reason` fields to `VerifySection` in `types.go`
- Update TOML parsing to handle new fields
- Add constants for valid mode and format values

### Phase 2: Version Format Transforms

- Create `internal/version/transform.go` for transformation logic
- Implement `TransformVersion(version string, format string) (string, error)`
- Add version string validation before transformation (allowlist: `[a-zA-Z0-9._+-]`, max 128 chars)
- Add transform functions: `semver`, `semver_full`, `strip_v`
- Unknown formats fall back to `raw` with warning
- Unit tests for each transform and validation

### Phase 3: Validator Updates

- Add mode-specific validation rules
- Warn if `version` mode pattern lacks `{version}`
- Require `reason` field for `output` mode
- Error if `mode = "functional"` used (reserved for v2)
- Expand dangerous pattern detection to include `||`, `&&`, `eval`, `exec`, `$()`, backticks
- Update `--strict` to enforce mode requirements

### Phase 4: Executor Integration

- Update `expandVars` in executor to call `version.TransformVersion` before substitution
- Handle missing format (default to `raw`)
- Handle transform errors gracefully (log warning, use raw version)
- Integration tests with sample recipes

### Phase 5: Recipe Migration

- Audit existing recipes for verification patterns
- Update recipes with version mismatches to use appropriate `version_format`
- Update recipes without version output to use `output` mode with `reason`

---

**Phases 1-5 implement Layer 2 (Version Verification) of the defense-in-depth strategy. The following phases are future work for additional layers.**

---

### Phase 6 (Future - Layer 3): Post-Install Checksum Pinning

**Goal**: Detect post-installation tampering by storing and verifying checksums of installed binaries.

**Scope**:
- Compute SHA256 of installed binaries after successful installation
- Store checksums in `state.json`
- `tsuku verify` recomputes and compares against stored values
- Detect tampering, corruption, or unauthorized modifications

**Implementation considerations**:
- Extend `state.json` schema to include binary checksums
- Handle tool updates (recompute checksums on upgrade)
- Consider optional periodic verification via `tsuku doctor`

### Phase 7 (Future - Layer 4): Functional Testing Framework

**Goal**: Support Homebrew-style behavioral tests that verify tool functionality, not just presence.

**Scope**:
- Implement `mode = "functional"` with test orchestration
- Support `[[verify.tests]]` array for multiple test cases
- Handle expected output matching, exit codes, timeouts
- Temporary directory management for file-based tests

**Recipe format**:
```toml
[verify]
mode = "functional"

[[verify.tests]]
name = "parse_json"
command = "sh"
args = ["-c", "echo '{\"foo\":1}' | jq .foo"]
expected_output = "1"
```

### Phase 8 (Future - Layer 4): Upstream Test Import

**Goal**: Leverage existing test corpora from Homebrew and Nix to populate functional tests without writing them from scratch.

**Depends on**: Phase 7 (Functional Testing Framework)

#### 8a: Homebrew Test Import

**Approach**: Manual curation rather than automated parsing
- Homebrew's JSON API does not include `test do` blocks
- Ruby formula parsing is complex (Ruby DSL with Homebrew helpers)
- Many Homebrew tests require translation (e.g., `pipe_output`, `assert_equal`, `testpath`)

**Strategy**:
1. Identify high-value tools (top 50 by install count in tsuku)
2. Check if Homebrew formula has a `test do` block
3. Manually translate test to tsuku `[verify]` format
4. Document source in recipe comment: `# Test adapted from Homebrew formula`

**Example translations**:

| Homebrew (Ruby) | Tsuku (TOML) |
|-----------------|--------------|
| `pipe_output("#{bin}/jq .bar", '{"foo":1}')` | `command = "sh"`, `args = ["-c", "echo '...' \| jq .bar"]` |
| `(testpath/"test.txt").write("data")` | Use temporary directory in shell script |
| `system bin/"rg", "pattern", testpath` | `command = "sh"`, `args = ["-c", "echo 'data' > test.txt && rg 'pattern' ."]` |

#### 8b: Nix Test Import

**Approach**: Leverage `installCheckPhase` from nixpkgs derivations for nix-based recipes

**Strategy**:
1. For recipes using `nixpkgs` action, check if derivation has `installCheckPhase`
2. Translate Nix test expressions to tsuku `[verify]` format
3. May be more automatable than Homebrew since Nix expressions are structured

**Advantages**:
- Nix tests are already designed for post-install verification
- Natural fit for tsuku recipes that use `nixpkgs` action
- Nix derivations are in a parseable format (not Ruby DSL)

**Challenges**:
- Nix tests may depend on sandbox features not available in tsuku
- Some tests require `nix-shell` environment
- Test expressions can be complex

### Phase 9 (Future - Layer 1): Signature and Provenance Verification

**Goal**: Complete Layer 1 by adding cryptographic signature verification and SLSA provenance attestation support.

**Current state**: Layer 1 only supports SHA256 checksums, which verify integrity but not authenticity.

**Scope**:
- Signature verification (Cosign, Minisign, GPG)
- SLSA provenance attestation verification
- Key management and trust model

**Recipe format**:
```toml
[download]
url = "https://example.com/tool-{version}.tar.gz"
checksum = "sha256:abc123..."  # existing

[download.signature]
method = "cosign"  # or "minisign", "gpg"
public_key_url = "https://example.com/cosign.pub"

[download.provenance]
method = "slsa"
source_repo = "github.com/org/tool"
```

**Challenges**:
- Not all upstream projects provide signatures
- Key distribution and trust model (how to ship/update trusted keys?)
- Multiple signature standards to support
- SLSA provenance is still emerging

**Dependencies**: Download infrastructure changes

## Security Considerations

### Download Verification

**Applicable as secondary layer** - While this feature does not perform the download itself, it provides post-installation verification that the correct artifact was installed. This is the second layer of defense after checksum verification.

**Security benefit**: Proper version verification increases confidence that the expected tool version was installed, not a different (potentially compromised) version.

**Risks**:
- If verification is bypassed or misconfigured, a wrong version could be installed silently
- The `output` mode provides weak verification (checks pattern, not version)
- Version format transforms could theoretically mask version mismatches

### Execution Isolation

**Scope**: The verify command runs with the same permissions as the tsuku process (typically user-level, no sudo).

**Risks**:
- Verify commands execute arbitrary shell commands defined in recipes
- A malicious recipe could execute harmful commands during verification
- The `reason` field in `functional` mode is user-visible but not executed
- Version strings from external providers could contain shell metacharacters

**Existing mitigations** (unchanged by this design):
- Recipes come from trusted registry (tsuku-registry repo)
- Verify commands are visible in recipe files
- The validator warns about dangerous patterns (`rm`, `| sh`, etc.)

**New mitigations added by this design**:
- Version string validation before expansion (allowlist characters, max length)
- Expanded dangerous pattern detection (`||`, `&&`, `eval`, `$()`)

### Supply Chain Risks

**Applicable as detection layer** - Version verification can detect certain supply chain attacks where the binary has been replaced with a different version. It complements but does not replace checksum verification.

**Detection scenarios**:
- Upstream silently changes what a version tag points to
- Attacker replaces binary but forgets to update version output
- Rollback attacks where old vulnerable version is served

**Limitations**:
- Cannot detect sophisticated attacks where attacker also modifies version output
- Relies on external version providers which could themselves be compromised

### User Data Exposure

**Not applicable** - This feature does not access or transmit user data. It only:
- Reads version strings from the recipe/version provider
- Runs verify commands and captures stdout
- Compares output against patterns

No new data is collected or transmitted.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Weak verification in `output` mode | Require `reason` field; validator flags missing reasons | Lazy authors may provide poor justifications |
| Malicious verify commands | Validator warns about dangerous patterns; recipe review process | Sophisticated attacks may evade pattern detection |
| Wrong version installed silently | Default to `version` mode; strict validation | User must explicitly opt into weaker modes |
| Version format transforms hide issues | Transforms are explicit and auditable in recipe | None - transforms are visible |
| Command injection via version strings | Version string validation (allowlist chars, max length) | Compromised provider could still serve malicious content within constraints |
| Conditional execution in verify commands | Expanded pattern detection for `\|\|`, `&&`, `eval`, `$()` | Novel obfuscation techniques may evade detection |

## Consequences

### Positive

- **Correctness**: Recipes explicitly declare their verification strategy
- **Debuggability**: When verification fails, the mode and format are visible
- **Flexibility**: Three modes cover all known use cases
- **Gradual adoption**: Existing recipes work unchanged

### Negative

- **Complexity**: Two new concepts (mode, format) to document and teach
- **Migration work**: ~40 recipes need updates for proper version verification
- **Validator strictness**: Strict mode will flag more recipes initially

### Mitigations

- Clear documentation with examples for each mode
- Migration can be phased; start with highest-value recipes
- Validator warnings (not errors) for backward compatibility initially

## Future Verification Methods

This design establishes version verification as the **foundational layer** of tsuku's verification strategy. The following methods address complementary concerns and should be explored as future enhancements.

### Verification Taxonomy

Different verification methods provide different guarantees. They are **complementary**, not competing alternatives:

| Method | What it Guarantees | What it Cannot Detect | Relationship |
|--------|-------------------|----------------------|--------------|
| **Version verification** (v1) | Correct version installed | Backdoored binary that reports correct version | Foundation - automatable baseline |
| **Output matching** (v1 fallback) | Tool produces expected output | Wrong version if output is version-agnostic | Weak fallback when version unavailable |
| **Functional testing** (v2) | Core functionality works | Version-specific bugs not covered by test | Complements version (behavior proof) |
| **Cryptographic verification** (future) | Artifact is authentic/untampered | Compromised upstream that signs malicious releases | Complements version (integrity proof) |
| **Checksum pinning** (future) | Installed binary hasn't changed | Initial compromise if checksum computed post-attack | Temporal complement (ongoing integrity) |

### Defense-in-Depth Layers

```
┌─────────────────────────────────────────────────────────────────┐
│ Layer 4: Functional Testing (Phases 7-8)                        │
│ Guarantees: Tool performs core operations correctly             │
│ Example: echo '{"foo":1}' | jq .foo → 1                         │
├─────────────────────────────────────────────────────────────────┤
│ Layer 3: Checksum Pinning (Phase 6)                             │
│ Guarantees: Installed binary hasn't been modified post-install  │
│ Example: SHA256 of $TSUKU_HOME/tools/jq-1.7/bin/jq              │
├─────────────────────────────────────────────────────────────────┤
│ Layer 2: Version Verification (Phases 1-5 - This Design)        │
│ Guarantees: Correct version was installed and reports correctly │
│ Example: jq --version → jq-1.7                                  │
├─────────────────────────────────────────────────────────────────┤
│ Layer 1: Cryptographic Verification (Partial - Phase 9)         │
│ Guarantees: Downloaded artifact matches expected checksum       │
│ Current: SHA256 checksums only                                  │
│ Future: Signatures (Cosign, Minisign, GPG) + SLSA provenance    │
└─────────────────────────────────────────────────────────────────┘
```

### Future Feature: Functional Testing

**Purpose**: Verify installed tools actually work, not just that they report the correct version.

**Value proposition**:
- Catches corrupted or incomplete installations that pass version checks
- Verifies tool works on the target system (architecture, dependencies)
- Provides strongest runtime guarantee

**Potential recipe format**:
```toml
[[verify.tests]]
name = "parse_json"
command = "sh"
args = ["-c", "echo '{\"foo\":1}' | jq .foo"]
expected_output = "1"

[[verify.tests]]
name = "filter_array"
command = "sh"
args = ["-c", "echo '[1,2,3]' | jq '.[1]'"]
expected_output = "2"
```

**Complexity**: Medium
- Test orchestration framework needed
- Timeout handling for hanging tests
- Temporary directory management for file-based tests

**Dependencies**: Version verification (this design) should be stable first

**Suggested issue**: `Explore: Functional test suites for recipe verification`

### Future Feature: Cryptographic Signature Verification

**Purpose**: Verify binary authenticity using signatures (Cosign, Minisign, GPG) and SLSA provenance.

**Value proposition**:
- Protects against compromised mirrors serving modified binaries
- Verifies binaries were built by legitimate CI systems
- Industry best practice for supply chain security

**Potential recipe format**:
```toml
[download]
url = "https://example.com/tool-{version}.tar.gz"

[download.signature]
method = "cosign"  # or "minisign", "gpg"
public_key_url = "https://example.com/cosign.pub"
```

**Complexity**: High
- Multiple signature standards to support
- Key distribution and trust model
- Not all upstream projects provide signatures

**Dependencies**: Download infrastructure changes

**Suggested issue**: `Explore: Cryptographic signature verification for supply chain security`

### Future Feature: Post-Install Checksum Pinning

**Purpose**: Detect post-installation tampering by storing and verifying checksums of installed binaries.

**Value proposition**:
- Detects malware injection after installation
- Catches disk corruption affecting binaries
- Enables periodic integrity verification

**How it would work**:
1. After installation, compute SHA256 of installed binaries
2. Store checksums in `state.json`
3. `tsuku verify` recomputes and compares against stored values

**Complexity**: Medium
- Extends `state.json` schema
- Must handle tool updates (recompute checksums)

**Dependencies**: Stable state management

**Suggested issue**: `Explore: Post-install checksum pinning for tamper detection`

### Future Feature: Upstream Test Import

**Purpose**: Import tests from upstream package managers (Homebrew, Nix) to reduce recipe maintenance burden.

**Value proposition**:
- Leverages community-vetted tests
- Reduces per-recipe test authoring effort
- Keeps tests in sync with upstream

**Sources**:
- **Homebrew**: `test do` blocks from formulas - manual curation due to Ruby DSL
- **Nix**: `installCheckPhase` from derivations - potentially more automatable, natural fit for `nixpkgs` action recipes

**Approach**: See Phase 6 in Implementation for detailed strategy

**Complexity**: High (if automated), Medium (if manually curated)

**Dependencies**: Functional testing framework (v2)

**Suggested issues**:
- `Explore: Import verification tests from Homebrew formulas`
- `Explore: Import verification tests from Nix derivations`

### Future Feature: Platform-Specific Verification

**Purpose**: Define different verification strategies per platform (Linux vs macOS, different architectures).

**Value proposition**:
- Handles tools with platform-dependent behavior
- Enables more accurate verification on each platform

**Potential recipe format**:
```toml
[verify.linux]
command = "tool --version"
pattern = "Version: {version}"

[verify.darwin]
command = "tool -v"  # Different flag on macOS
pattern = "tool {version}"
```

**Complexity**: Low - extends existing recipe format

**Dependencies**: Current verification modes

**Suggested issue**: `Explore: Platform-specific verification modes`

### Roadmap Summary

| Feature | Phase | Layer | Priority | Complexity |
|---------|-------|-------|----------|------------|
| Version verification | 1-5 | 2 | **Now** | Medium |
| Checksum pinning | 6 | 3 | Future | Medium |
| Functional testing framework | 7 | 4 | Future | Medium |
| Upstream test import | 8 | 4 | Future | High |
| Signatures + SLSA provenance | 9 | 1 | Future | High |
| Platform-specific verification | TBD | 2 | Future | Low |

### Design Principles for Future Work

When adding verification methods:

1. **Layered, not exclusive**: Each method addresses different concerns; all should be usable together
2. **Opt-in enhancement**: Version verification remains the required baseline; additional methods are optional
3. **Clear failure modes**: Each layer should report distinct, actionable errors
4. **Independent evolution**: Recipes can adopt new methods without changing existing verification

## Implementation Issues

### Milestone: Version Verification

- [#196](https://github.com/tsukumogami/tsuku/issues/196): feat(recipe): add verification mode and version format fields
- [#197](https://github.com/tsukumogami/tsuku/issues/197): feat(version): implement version format transforms with validation
- [#198](https://github.com/tsukumogami/tsuku/issues/198): feat(validator): enforce verification mode rules and security checks
- [#199](https://github.com/tsukumogami/tsuku/issues/199): feat(executor): apply version format transforms during verification
- [#200](https://github.com/tsukumogami/tsuku/issues/200): chore(recipes): migrate recipes to use verification modes
- [#201](https://github.com/tsukumogami/tsuku/issues/201): docs: document verification modes and version format transforms

### Milestone: Defense-in-Depth Verification

- [#202](https://github.com/tsukumogami/tsuku/issues/202): feat(verify): version verification (Layer 2) [umbrella]
- [#203](https://github.com/tsukumogami/tsuku/issues/203): feat(verify): post-install checksum pinning (Layer 3) [needs-design]
- [#204](https://github.com/tsukumogami/tsuku/issues/204): feat(verify): functional testing framework (Layer 4) [needs-design]
- [#205](https://github.com/tsukumogami/tsuku/issues/205): feat(verify): import tests from Homebrew formulas (Layer 4) [needs-design]
- [#207](https://github.com/tsukumogami/tsuku/issues/207): feat(verify): import tests from Nix derivations (Layer 4) [needs-design]
- [#208](https://github.com/tsukumogami/tsuku/issues/208): feat(verify): signature and SLSA provenance verification (Layer 1) [needs-design]

