# Design: Flexible Recipe Verification

- **Status**: Proposed
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
3. **Backward compatibility**: Existing recipes must continue working
4. **Fail-safe defaults**: Missing or incorrect verification should be obvious, not silent
5. **Minimal configuration**: Prefer convention over configuration where possible
6. **Validation coverage**: CI should catch recipes with inadequate verification

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

**Relevance to tsuku**: The insight that version info is only available post-install aligns with tsuku's verify step. The two-phase approach is overkill for tsuku.

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
| Strong | Core functionality works | `test do` block | `installCheckPhase` | `mode = "functional"` |
| Medium | Correct version installed | Discouraged but used | Version assertions | `mode = "version"` (default) |
| Weak | Tool executes without crash | Fallback | - | `mode = "functional"` + exit code |

**Implications for tsuku:**
1. **Verification tiers** - Support functional tests as strongest option, version verification as automatable baseline, exit-code-only as fallback
2. **Homebrew corpus reuse** - Import tests from Homebrew formulas for tools that have them (reduces recipe author burden)
3. **Version format transforms** - Still needed for version verification tier (separate concern from verification strategy)
4. **Keep it simple** - One verification step per recipe, not multiple phases like Nix
5. **Clear opt-in** - Functional mode requires explicit `reason` field to document why used instead of version verification

## Considered Options

### Option 1: Version Transform Directives

Add explicit transformation directives to normalize version strings before pattern matching.

```toml
[verify]
command = "biome --version"
pattern = "Version: {version}"
version_transform = "strip_prefix:biome@"
```

Or with multiple transforms:
```toml
version_transforms = ["strip_prefix:v", "strip_suffix:-0"]
```

**Pros:**
- Explicit and self-documenting - maintainer intent is clear
- Handles any version format mismatch
- Recipe author has full control
- No magic or heuristics
- Predictable behavior

**Cons:**
- Verbose for common cases (v-prefix is very common)
- Recipe authors must understand transform syntax
- Doesn't address tools without version output
- Adds complexity to recipe format
- Doesn't compose well - chaining multiple transforms is awkward
- Requires defining all transform types upfront

### Option 2: Automatic Version Normalization

Automatically extract semver from version strings using heuristics.

The executor would:
1. Strip common prefixes: `v`, `release-`, `tool@`
2. Extract semver pattern: `\d+\.\d+\.\d+`
3. Use normalized version for `{version}` expansion

```toml
[verify]
command = "biome --version"
pattern = "Version: {version}"
# biome@2.3.8 automatically becomes 2.3.8
```

**Pros:**
- Zero configuration for common cases
- Existing recipes may work without changes
- Simple recipe format
- Gradual improvement over status quo

**Cons:**
- Magic behavior may surprise users
- Heuristics won't work for all formats
- Hard to debug when normalization fails silently
- Creates invisible contract - authors don't know what heuristics match
- Heuristics may break when tools change output format
- Doesn't address tools without version output

**Note:** This option alone is insufficient for a package manager where correctness matters. However, the auto-normalization approach could be combined with explicit fallbacks (see Option 5).

### Option 3: Verification Modes

Introduce explicit verification modes that change how verification works.

```toml
[verify]
mode = "version"  # default - requires {version} pattern match
command = "tool --version"
pattern = "{version}"
```

```toml
[verify]
mode = "functional"  # tool runs successfully = verified
command = "tool --help"
# pattern optional, just checks exit code
```

```toml
[verify]
mode = "output"  # custom output matching, no version requirement
command = "tool info"
pattern = "Tool v"
```

**Pros:**
- Clear intent - mode declares verification strategy
- Validator can enforce appropriate patterns per mode
- Accommodates tools without version output
- Extensible for future modes

**Cons:**
- Breaking change if mode becomes required
- Three concepts to learn instead of one
- Doesn't solve version format mismatch (still need transforms)

### Option 4: Tiered Verification with Fallback

Combine version verification with fallback strategies. Require version verification by default, but allow explicit opt-out with justification.

```toml
[verify]
command = "tool --version"
pattern = "{version}"
version_format = "semver_core"  # optional: extract 1.2.3 from any format
```

For tools without version support:
```toml
[verify]
command = "tool --help"
pattern = "Usage:"
skip_version_check = true
skip_reason = "Tool does not support --version"
```

**Pros:**
- Version check is the default expectation
- Explicit opt-out requires justification
- Validator can flag recipes without version checks
- Combines format normalization with fallback support

**Cons:**
- Two features in one (format + fallback)
- `skip_reason` is verbose
- May encourage lazy opt-outs

### Option 5: Smart Defaults with Override

Use automatic normalization as default, but allow explicit `{version_raw}` for cases where the raw version string is needed.

```toml
[verify]
command = "biome --version"
pattern = "Version: {version}"  # auto-normalized: biome@2.3.8 → 2.3.8
```

```toml
[verify]
command = "go version"
pattern = "go{version_raw}"  # raw: go1.21.0 (no normalization)
```

For tools without version output, use empty pattern with explicit command:
```toml
[verify]
command = "gofumpt -h"
pattern = "usage:"  # no {version} = no version check
```

**Pros:**
- Common case (normalized version) is simple
- Raw version available when needed
- Empty/non-version patterns work naturally
- Backward compatible

**Cons:**
- Two placeholders to understand
- Auto-normalization is still heuristic-based
- No explicit "I know this doesn't have version check" signal

## Options Comparison

| Criterion | Option 1 | Option 2 | Option 3 | Option 4 | Option 5 |
|-----------|----------|----------|----------|----------|----------|
| Version accuracy | Good | Fair | Good | Good | Good |
| Recipe simplicity | Poor | Good | Fair | Fair | Good |
| Backward compat | Good | Good | Poor | Good | Good |
| Fail-safe defaults | Good | Poor | Good | Good | Fair |
| Minimal config | Poor | Good | Fair | Fair | Good |
| Validation coverage | Fair | Poor | Good | Good | Fair |

### Uncertainties

- **Normalization accuracy**: We haven't measured how many recipes would benefit from auto-normalization vs. break from it
- **Recipe author burden**: Unknown how many recipe authors would find transforms confusing vs. helpful
- **Version format diversity**: The full range of version formats in the wild is not fully catalogued
- **Validator strictness**: Unclear whether strict validation should require version checks or just encourage them

### Assumptions

The following assumptions underlie this design:

1. **Version source diversity**: Version strings come from various providers (GitHub, PyPI, npm, crates.io, goproxy). The solution must handle formats from all providers, not just GitHub tags.

2. **Install-time verification**: Verification happens immediately after installation, not lazily on first use. This catches failures early.

3. **Single verification per recipe**: Cross-platform differences are handled by platform-specific patterns within the same recipe, not multiple verification strategies.

4. **stdout capture**: The verify command's stdout is captured for pattern matching. Most tools output version to stdout; stderr handling is out of scope.

5. **Exit code semantics**: Non-zero exit codes indicate verification failure, regardless of output matching. Some tools may need special handling.

6. **String matching**: Pattern matching is substring-based (`strings.Contains`), not semver-aware comparison. The pattern must appear literally in output.

## Decision Outcome

**Chosen option: Hybrid of Option 1 (Transform Directives) + Option 3 (Verification Modes)**

This approach provides explicit version format transforms for the version mismatch problem, combined with verification modes for tools that don't support version output. It prioritizes explicitness and correctness over convenience.

### Rationale

Version verification is chosen as the **foundation** of tsuku's verification strategy because:

1. **Universal applicability**: ~95% of tools support `--version` or equivalent, making version verification the most automatable baseline
2. **Supply chain detection**: Version checks detect wrong/outdated versions, rollback attacks, and some supply chain compromises
3. **No custom code required**: Unlike functional tests, version verification works without per-recipe test authoring

This is the **first verification method** we support, not the only one. Future work will add:
- **Functional testing** (stronger behavioral guarantees)
- **Cryptographic verification** (supply chain integrity)
- **Checksum pinning** (tamper detection)

These are **complementary**, not competing alternatives - each addresses different concerns.

### Design Alignment with Decision Drivers

1. **Version accuracy** (Driver 1): Explicit transforms ensure the recipe author controls exactly how version strings are normalized. No heuristic guessing.

2. **Recipe simplicity** (Driver 2): Common transforms (strip `v` prefix) can have shorthand names. Most recipes won't need transforms at all.

3. **Backward compatibility** (Driver 3): Existing recipes continue working. New fields are optional.

4. **Fail-safe defaults** (Driver 4): The default mode is `version`, requiring `{version}` in pattern. The `functional` mode is a fallback for tools that genuinely cannot report version, not an alternative strategy.

5. **Validation coverage** (Driver 6): The validator can enforce that:
   - `version` mode has `{version}` in pattern
   - `functional` mode has a `reason` explaining why version check isn't possible
   - Empty patterns are only allowed with explicit mode

### Alternatives Rejected

- **Option 2 (Auto-normalization)**: Magic heuristics are inappropriate for a package manager where correctness matters. Silent failures undermine user trust.

- **Option 4 (Tiered with fallback)**: The `skip_version_check` + `skip_reason` approach conflates two concerns. Separate mode selection is cleaner.

- **Option 5 (Smart defaults)**: The `{version}` vs `{version_raw}` split adds cognitive overhead. Recipe authors shouldn't need to understand normalization internals.

### Trade-offs Accepted

By choosing this option, we accept:

1. **More verbose recipes for edge cases**: Tools with unusual version formats need explicit transform configuration.

2. **Two new concepts**: Recipe authors must understand both transforms and modes (though most recipes need neither).

3. **Migration burden**: Recipes currently using weak patterns should be updated to use appropriate modes.

These are acceptable because:
- Verbosity is localized to the ~40 recipes with version mismatches
- The common case (tool outputs clean version) requires zero configuration
- Migration can be gradual; existing recipes continue working

## Solution Architecture

### Overview

The solution adds two optional fields to the `[verify]` section:

1. **`mode`**: Declares the verification strategy (`version`, `functional`, or `output`)
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

# Functional mode: verify core functionality works (strongest)
# Test adapted from Homebrew formula
[verify]
mode = "functional"
command = "sh"
args = ["-c", "echo '{\"foo\":1, \"bar\":2}' | jq .bar | grep -q '^2$'"]
reason = "Verifies jq can parse JSON and execute filters (from Homebrew test)"

# Functional mode: minimal verification for tools without --version
[verify]
mode = "functional"
command = "gofumpt -h"
pattern = "usage:"  # optional in functional mode
reason = "Tool does not support --version flag"

# Output mode: custom pattern without version requirement
[verify]
mode = "output"
command = "tool info"
pattern = "Tool v"
```

### Verification Modes

| Mode | Purpose | Pattern | `{version}` |
|------|---------|---------|-------------|
| `version` (default) | Verify exact version installed | Required, must contain `{version}` | Expanded from resolved version |
| `functional` | Verify tool runs successfully | Optional | Not expanded |
| `output` | Custom output matching | Required | Not expanded |

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

- **`version_format` with non-version mode**: If `mode = "functional"` and `version_format` is set, the format is ignored (no `{version}` to expand)
- **Pattern without `{version}` in version mode**: Validator warns but allows; pattern is matched literally
- **Unknown `version_format`**: Treated as `raw` with warning; allows forward compatibility
- **Transform fails to extract version**: Falls back to raw version with warning

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
- Require `reason` field for `functional` mode
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
- Update recipes without version output to use `functional` mode

### Phase 6 (Future): Homebrew Test Import

**Goal**: Leverage Homebrew's test corpus to provide stronger verification for tsuku recipes.

**Approach**: Manual curation rather than automated parsing
- Homebrew's JSON API does not include `test do` blocks
- Ruby formula parsing is complex (Ruby DSL with Homebrew helpers)
- Many Homebrew tests require translation (e.g., `pipe_output`, `assert_equal`, `testpath`)

**Strategy**:
1. Identify high-value tools (top 50 by install count in tsuku)
2. Check if Homebrew formula has a `test do` block
3. Manually translate test to tsuku `[verify]` format
4. Document source in recipe comment: `# Test adapted from Homebrew formula`
5. Track in registry metadata which recipes have Homebrew-derived tests

**Example translations**:

| Homebrew (Ruby) | Tsuku (TOML) |
|-----------------|--------------|
| `pipe_output("#{bin}/jq .bar", '{"foo":1, "bar":2}')` | `command = "sh"`, `args = ["-c", "echo '...' \| jq .bar"]` |
| `(testpath/"test.txt").write("data")` | Use temporary directory in shell script |
| `system bin/"rg", "pattern", testpath` | `command = "sh"`, `args = ["-c", "echo 'data' > test.txt && rg 'pattern' ."]` |

**Maintenance**: Update tests when Homebrew formulas change (quarterly audit)

**Out of scope for Phase 6**: Automated Ruby formula parsing, `test_dependencies` handling, complex multi-step tests requiring Homebrew helpers

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

## Security Considerations

### Download Verification

**Applicable as secondary layer** - While this feature does not perform the download itself, it provides post-installation verification that the correct artifact was installed. This is the second layer of defense after checksum verification.

**Security benefit**: Proper version verification increases confidence that the expected tool version was installed, not a different (potentially compromised) version.

**Risks**:
- If verification is bypassed or misconfigured, a wrong version could be installed silently
- The `functional` mode provides minimal verification (only checks command runs)
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
| Weak verification in `functional` mode | Require `reason` field; validator flags missing reasons | Lazy authors may provide poor justifications |
| Malicious verify commands | Validator warns about dangerous patterns; recipe review process | Sophisticated attacks may evade pattern detection |
| Wrong version installed silently | Default to `version` mode; strict validation | User must explicitly opt into weaker modes |
| Version format transforms hide issues | Transforms are explicit and auditable in recipe | None - transforms are visible |
| Command injection via version strings | Version string validation (allowlist chars, max length) | Compromised provider could still serve malicious content within constraints |
| Conditional execution in verify commands | Expanded pattern detection for `\|\|`, `&&`, `eval`, `$()` | Novel obfuscation techniques may evade detection |

## Future Verification Methods

This design establishes version verification as the **foundational layer** of tsuku's verification strategy. The following methods address complementary concerns and should be explored as future enhancements.

### Verification Taxonomy

Different verification methods provide different guarantees. They are **complementary**, not competing alternatives:

| Method | What it Guarantees | What it Cannot Detect | Relationship |
|--------|-------------------|----------------------|--------------|
| **Version verification** | Correct version installed | Backdoored binary that reports correct version | Foundation - automatable baseline |
| **Functional testing** | Core functionality works | Version-specific bugs not covered by test | Complements version (behavior proof) |
| **Cryptographic verification** | Artifact is authentic/untampered | Compromised upstream that signs malicious releases | Complements version (integrity proof) |
| **Checksum pinning** | Installed binary hasn't changed | Initial compromise if checksum computed post-attack | Temporal complement (ongoing integrity) |
| **Dependency verification** | Runtime deps are present | Tool bugs unrelated to deps | Precondition for functional tests |

### Defense-in-Depth Layers

```
┌─────────────────────────────────────────────────────────────────┐
│ Layer 4: Functional Testing (Future)                            │
│ Guarantees: Tool performs core operations correctly             │
│ Example: echo '{"foo":1}' | jq .foo → 1                         │
├─────────────────────────────────────────────────────────────────┤
│ Layer 3: Checksum Pinning (Future)                              │
│ Guarantees: Installed binary hasn't been modified post-install  │
│ Example: SHA256 of $TSUKU_HOME/tools/jq-1.7/bin/jq              │
├─────────────────────────────────────────────────────────────────┤
│ Layer 2: Version Verification (This Design)                     │
│ Guarantees: Correct version was installed and reports correctly │
│ Example: jq --version → jq-1.7                                  │
├─────────────────────────────────────────────────────────────────┤
│ Layer 1: Cryptographic Verification (Existing/Partial)          │
│ Guarantees: Downloaded artifact matches expected checksum       │
│ Example: SHA256 of downloaded archive                           │
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

**Approach**: Manual curation rather than automated parsing (see Phase 6 in Implementation)

**Complexity**: High (if automated), Medium (if manually curated)

**Dependencies**: Functional testing framework

**Suggested issue**: `Explore: Import verification tests from Homebrew formulas`

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

| Feature | Priority | Complexity | Dependencies |
|---------|----------|------------|--------------|
| Version verification (this design) | **Now** | Medium | - |
| Platform-specific verification | High | Low | This design |
| Functional testing | High | Medium | This design |
| Cryptographic signatures | Medium | High | Download changes |
| Checksum pinning | Medium | Medium | State management |
| Upstream test import | Low | High | Functional testing |

### Design Principles for Future Work

When adding verification methods:

1. **Layered, not exclusive**: Each method addresses different concerns; all should be usable together
2. **Opt-in enhancement**: Version verification remains the required baseline; additional methods are optional
3. **Clear failure modes**: Each layer should report distinct, actionable errors
4. **Independent evolution**: Recipes can adopt new methods without changing existing verification

