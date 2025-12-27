# Hardcoded Version Detection in Recipes

## Status

Accepted

## Context and Problem Statement

Recipes in tsuku define how to install developer tools. A well-formed recipe uses dynamic version placeholders (`{version}`) so that the same recipe definition works for any version. However, recipes can be merged with hardcoded version numbers in URLs, file paths, and other fields, locking them to a single version.

The curl recipe was recently merged with hardcoded `8.11.1` in multiple places:

```toml
# Hardcoded (problematic)
url = "https://curl.se/download/curl-8.11.1.tar.gz"
archive = "curl-8.11.1.tar.gz"
source_dir = "curl-8.11.1"
```

This violates recipe best practices and requires manual updates for each version bump. While `tsuku validate --strict` catches many issues, it does not detect hardcoded versions, allowing invalid recipes to pass review and CI.

The problem compounds with recipe contributions from external contributors who may not understand the templating system.

### Success Criteria

- Detection rate: Catch >90% of hardcoded version patterns in URLs, archives, and source directories
- False positive rate: <5% of warnings should be on legitimate static values
- Performance: <10ms additional validation time per recipe
- Coverage: All versioned download actions have detection rules

### Scope

**In scope:**
- Detection of version-like patterns in recipe fields (URLs, paths, action parameters)
- Distinguishing legitimate values from problematic hardcoded versions
- Integration with existing `tsuku validate` command
- Clear, actionable warnings that explain the issue

**Out of scope:**
- Auto-fixing hardcoded versions (complexity and risk too high)
- Validating checksums match versions (checksums are cryptographic, not pattern-based)
- Dependency version hardcoding in RPATH (tracked separately in #653)
- Version format normalization (semver vs calver vs custom)

## Decision Drivers

- **Catch errors before merge**: Primary goal is preventing hardcoded versions from being merged via PR review
- **Low false positive rate**: Detection must not flag legitimate values (e.g., tool names with numbers, API versions, ABI versions)
- **Actionable feedback**: Warnings must explain what's wrong and how to fix it
- **Minimal performance impact**: Validation runs frequently; detection must be fast
- **Extensibility**: Different action types may need different detection rules
- **Integration with existing validation**: Should enhance, not replace, current `tsuku validate`

## Implementation Context

### Existing Patterns

**Validation architecture:**
- `ValidateStructural()` in `internal/recipe/validate.go` - checks structure and field types
- `ValidateSemantic()` - delegates to registered validators (e.g., `VersionValidator`)
- `DetectRedundantVersion()` in `internal/version/redundancy.go` - detects redundant `[version]` sections
- CLI wrapper in `cmd/tsuku/validate.go` - orchestrates validation with `--strict` flag

**Existing partial detection:**
- The `download` action's `Preflight()` method (lines 53-56 of `internal/actions/download.go`) checks if URLs lack template variables, but only warns about missing `{version}` in the verify pattern, not hardcoded versions in URLs
- The `redundancy.go` module uses an analyze-compare-report pattern that serves as a reference implementation

**Template variable substitution:**
- `{version}` - resolved version (e.g., "1.29.3")
- `{version_tag}` - original tag (e.g., "v1.29.3")
- `{os}`, `{arch}` - platform identifiers
- `{install_dir}`, `{libs_dir}` - path references

**Recipe fields containing version-sensitive values:**

| Field | Action Types | Example |
|-------|-------------|---------|
| `url` | download, download_file | `https://example.com/tool-{version}.tar.gz` |
| `archive` | extract | `tool-{version}.tar.gz` |
| `source_dir` | configure_make, cmake_build | `tool-{version}` |
| `asset_pattern` | github_archive, github_file | `tool-*-{version}-*.tar.gz` |
| `checksum_url` | download | `https://example.com/checksums-{version}.txt` |
| `rpath` | set_rpath | `$ORIGIN/../lib:{libs_dir}/dep-{deps.dep.version}/lib` |

### Anti-patterns to Avoid

1. **False positives on tool names**: `python3`, `go1.21`, `ncursesw6-config` contain numbers but aren't hardcoded versions
2. **False positives on ABI versions**: `libncurses.so.6.5` is an ABI version, not the tool version
3. **Overly aggressive regex**: Must not flag every number pattern as suspicious
4. **Missing context**: Need to know when `{version}` is expected vs when static values are acceptable

### Research Summary

**Upstream constraints:**
- No upstream strategic design; this is standalone tactical work

**Patterns to follow:**
- Extend existing validation in `internal/recipe/validate.go`
- Use `ValidationWarning` for detection results (not errors, since edge cases exist)
- Register detector as a validator following existing patterns

**Implementation approach:**
- Detect version patterns in fields that typically use `{version}` placeholders
- Compare against known patterns (semver, calver, date-based)
- Warn when version-like patterns appear without template variables
- Allow suppression via recipe-level annotation for legitimate edge cases

## Considered Options

### Option 1: Pattern-Based Detection in Validate

Add a new validation step that scans recipe fields for version-like patterns:

```go
// internal/recipe/hardcoded.go
func DetectHardcodedVersions(r *Recipe) []ValidationWarning {
    patterns := []regexp.Regexp{
        // Semver: 1.2.3, v1.2.3, 1.2.3-beta
        regexp.MustCompile(`\b[vV]?\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?\b`),
        // Calver: 2024.01, 24.01
        regexp.MustCompile(`\b20\d{2}\.\d{2}(\.\d{2})?\b`),
    }
    // Scan url, archive, source_dir, etc.
}
```

**Pros:**
- Simple implementation with well-understood regex patterns
- Fast execution (single pass over recipe fields)
- Works with existing validation infrastructure

**Cons:**
- Potential false positives on tool names with version-like patterns
- Cannot distinguish between "8.11.1" in URL vs "ncursesw6-config" binary name
- Regex maintenance burden as new patterns emerge

### Option 2: Context-Aware Detection with Field Rules

Define per-field rules that understand what each field should contain:

```go
// internal/recipe/hardcoded.go
type FieldRule struct {
    Field        string
    Actions      []string   // Which actions this applies to
    ExpectPlaceholder bool  // Should contain {version}?
    AllowedPatterns []string // Exceptions (e.g., tool names)
}

var fieldRules = []FieldRule{
    {Field: "url", Actions: []string{"download", "download_archive"}, ExpectPlaceholder: true},
    {Field: "archive", Actions: []string{"extract"}, ExpectPlaceholder: true},
    {Field: "source_dir", Actions: []string{"configure_make", "cmake_build"}, ExpectPlaceholder: true},
    // download_file: url does NOT expect placeholder (static URLs are normal)
}
```

**Pros:**
- Reduces false positives by understanding field semantics
- Different rules for different action types (download vs download_file)
- Extensible as new actions are added

**Cons:**
- More complex implementation
- Rules must be maintained as action types evolve
- Still needs pattern matching for version detection

### Option 3: Comparative Analysis with Version Field

When a recipe has a `[version]` source, compare detected version strings against the version provider's expectations:

```go
// If recipe uses github_releases and version field exists,
// look for patterns matching github tag format
func DetectHardcodedVersions(r *Recipe) []ValidationWarning {
    if r.Version.Source == "" {
        // Can't compare without version source
        return nil
    }
    // Find version-like strings in fields
    // Compare against what the version source would provide
}
```

**Pros:**
- Semantic comparison: knows what version format to expect
- Can suggest fix: "use {version} instead of 1.2.3"
- Low false positive rate when version source is known

**Cons:**
- Only works when `[version]` section is present
- Cannot catch all cases (some recipes legitimately lack version section)
- Requires understanding each version source's format

### Option 4: Heuristic Scoring with Configurable Thresholds

Score each detected pattern based on context and confidence:

```go
type Detection struct {
    Field      string
    Value      string
    Pattern    string    // What was matched
    Score      float64   // 0.0-1.0 confidence
    Suggestion string    // How to fix
}

// Score based on:
// - Field type (url scores higher than binary name)
// - Pattern type (semver scores higher than single digit)
// - Context (has {version} nearby? likely placeholder missing)
```

**Pros:**
- Nuanced detection with confidence levels
- Can tune thresholds to balance precision/recall
- Explains reasoning in output

**Cons:**
- More complex to implement and maintain
- Scoring thresholds need tuning with real recipe corpus
- Additional complexity may not be justified for the current problem scope

**When this approach would be appropriate:**
This option becomes valuable when detection needs to be more nuanced, such as when dealing with a large corpus of recipes with many edge cases or when false positive rates need fine-tuning. For the initial implementation, the simpler context-aware approach (Option 2) provides sufficient precision.

## Decision Outcome

**Chosen: Option 2 (Context-Aware Detection with Field Rules) + elements of Option 1**

We choose context-aware detection because:

1. **Semantic understanding**: Knowing that `url` in `download` expects `{version}` but `url` in `download_file` is intentionally static dramatically reduces false positives
2. **Action-specific rules**: Different actions have different version expectations; a blanket regex approach cannot capture this
3. **Extensibility**: New actions can declare their version-related fields in a structured way
4. **Pragmatic pattern matching**: We still use regex from Option 1 for detecting version-like strings, but only in fields where they're suspicious

### Rationale

The original curl recipe problem was: `download_file` action with a version in the URL but no `{version}` placeholder. The fix was switching to `download` action with template variable. This tells us:

- **Action type matters**: `download_file` is for static assets; `download` is for versioned assets
- **Context matters**: The same URL pattern is problematic in one action but expected in another
- **Version source matters**: If a recipe declares a version source but doesn't use `{version}` in download URLs, that's suspicious

A pure regex approach (Option 1) would either miss cases or generate too many false positives. Context-aware detection lets us be precise.

## Solution Architecture

### Components

```
internal/recipe/
├── validate.go           # Existing validation (unchanged)
├── hardcoded.go          # NEW: Hardcoded version detection
└── types.go              # Recipe types (unchanged)

cmd/tsuku/
└── validate.go           # CLI wrapper (minor addition)
```

### Detection Flow

```
Recipe TOML
    │
    ▼
Parse Recipe
    │
    ▼
For each Step:
    │
    ├─► Get FieldRules for step.Action
    │       │
    │       ▼
    │   For each rule:
    │       │
    │       ├─► Get field value from step.Params
    │       │
    │       ├─► If ExpectPlaceholder && !hasPlaceholder(value):
    │       │       │
    │       │       ▼
    │       │   Scan for version patterns
    │       │       │
    │       │       ▼
    │       │   If found: Add ValidationWarning
    │       │
    │       └─► Continue to next rule
    │
    └─► Continue to next step
```

### Field Rules Definition

```go
// VersionFieldRule defines version detection behavior for a specific field
type VersionFieldRule struct {
    Field             string   // Field name in step params (e.g., "url")
    ExpectPlaceholder bool     // Should this field use {version}?
}

// ActionVersionRules maps action names to their version-sensitive fields
var ActionVersionRules = map[string][]VersionFieldRule{
    // Download actions
    "download": {
        {Field: "url", ExpectPlaceholder: true},
        {Field: "checksum_url", ExpectPlaceholder: true},
    },
    "download_archive": {
        {Field: "url", ExpectPlaceholder: true},
        {Field: "checksum_url", ExpectPlaceholder: true},
    },
    "download_file": {
        // No ExpectPlaceholder fields - static URLs are expected
    },
    // GitHub actions
    "github_archive": {
        {Field: "asset_pattern", ExpectPlaceholder: true},
    },
    "github_file": {
        {Field: "asset_pattern", ExpectPlaceholder: true},
    },
    // Extract action
    "extract": {
        {Field: "archive", ExpectPlaceholder: true},
    },
    // Build actions
    "configure_make": {
        {Field: "source_dir", ExpectPlaceholder: true},
    },
    "cmake_build": {
        {Field: "source_dir", ExpectPlaceholder: true},
    },
    "meson_build": {
        {Field: "source_dir", ExpectPlaceholder: true},
    },
    "cargo_build": {
        {Field: "source_dir", ExpectPlaceholder: true},
    },
    "go_build": {
        {Field: "source_dir", ExpectPlaceholder: true},
    },
}
```

### Version Pattern Detection

```go
// hasVersionPlaceholder checks if a string contains {version} or {version_tag}
func hasVersionPlaceholder(s string) bool {
    return strings.Contains(s, "{version}") || strings.Contains(s, "{version_tag}")
}

// versionPatterns detects common version formats
var versionPatterns = []*regexp.Regexp{
    // Semver: 1.2.3, v1.2.3, 1.2.3-beta.1, 1.2.3+build
    regexp.MustCompile(`\b[vV]?(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?\b`),
    // Two-part version: 1.2, v1.2
    regexp.MustCompile(`\b[vV]?(\d+)\.(\d+)\b`),
    // Date-based: 2024.01, 24.01.15
    regexp.MustCompile(`\b20\d{2}\.\d{2}(\.\d{2})?\b`),
}

// Minimum version segment to reduce false positives
// "3" alone is not a version, but "3.0" or "3.0.0" is
const minVersionSegments = 2
```

**Fast-path optimization:** If a field already contains `{version}` or `{version_tag}`, skip regex scanning entirely. This avoids unnecessary pattern matching on properly templated fields.

### Warning Output

```go
type HardcodedVersionWarning struct {
    Step      int      // Step index (1-based for user display)
    Action    string   // Action name
    Field     string   // Field containing hardcoded version
    Value     string   // Detected version string
    Suggested string   // Suggested fix with {version}
}

func (w HardcodedVersionWarning) String() string {
    return fmt.Sprintf(
        "step %d (%s): field '%s' contains hardcoded version '%s', use '%s' instead",
        w.Step, w.Action, w.Field, w.Value, w.Suggested,
    )
}
```

### Integration with Validate Command

The detection integrates with `--strict` mode:

```bash
$ tsuku validate --strict recipes/c/curl.toml
Error: validation failed with 3 warnings (--strict mode)

Warnings:
  - step 1 (download_file): field 'url' contains hardcoded version '8.11.1',
    use 'https://curl.se/download/curl-{version}.tar.gz' instead
  - step 2 (extract): field 'archive' contains hardcoded version '8.11.1',
    use 'curl-{version}.tar.gz' instead
  - step 4 (configure_make): field 'source_dir' contains hardcoded version '8.11.1',
    use 'curl-{version}' instead
```

### Edge Case Handling

**1. Tool names with version-like patterns:**

Skip detection when pattern appears as part of a known token:
- `python3` - single digit, not a version
- `ncursesw6-config` - part of tool name
- `lib*.so.6.5` - ABI version suffix (allowed in binary paths)

**2. Legitimate static versions:**

Some recipes legitimately need static values:
- API version in URLs: `/api/v2/download`
- Platform identifiers: `x86_64`, `aarch64`

Handle via exclusion patterns:
```go
var excludePatterns = []*regexp.Regexp{
    regexp.MustCompile(`/api/v\d+/`),      // API versions
    regexp.MustCompile(`x86_64|aarch64`),  // Architecture strings
}
```

**3. download_file action:**

`download_file` is designed for static assets (like shell scripts, config files). No version placeholder expected. Skip version detection for this action type.

## Implementation Approach

### Phase 1: Core Detection Logic

1. Create `internal/recipe/hardcoded.go` with:
   - `ActionVersionRules` map defining field expectations per action
   - `DetectHardcodedVersions(r *Recipe) []ValidationWarning` function
   - Version pattern regexes with exclusion patterns
   - `hasVersionPlaceholder()` helper for fast-path optimization

2. Add unit tests in `internal/recipe/hardcoded_test.go`:
   - Test each action type's rules
   - Test false positive scenarios (tool names, ABI versions)
   - Test edge cases (API versions, platform strings)
   - Test fast-path (fields with placeholders skip regex)

### Phase 2: Integration with Validate

1. Register detector in validation pipeline:
   - Call from `cmd/tsuku/validate.go` alongside redundancy detection
   - Include in `--strict` mode output

2. Update CLI output formatting:
   - Group warnings by category (redundancy, hardcoded versions, etc.)
   - Provide actionable suggestions

### Phase 3: Recipe Remediation (before CI enforcement)

1. Scan existing recipes with new detection (detection audit)
2. Fix any hardcoded versions found
3. Tune false positive thresholds based on audit results
4. Document patterns in CONTRIBUTING.md

### Phase 4: CI Integration (after remediation)

1. Update CI workflow to run `tsuku validate --strict` on all recipes
2. Add recipe linting step to PR checks

**Critical sequencing note:** CI integration must come after recipe remediation. Enabling strict checks before fixing existing recipes would break all PRs until remediation is complete.

## Security Considerations

### Download Verification

Not directly impacted. Detection runs during validation, before any downloads occur. No changes to download verification flow.

### Execution Isolation

Not applicable. Detection is a static analysis of TOML content; no code execution involved.

### Supply Chain Risks

**Positive impact**: Detection helps prevent locked-to-single-version recipes that could hide supply chain issues (e.g., a malicious version being hardcoded instead of using the latest from a trusted version source).

**Detection is best-effort**: This validation is defense-in-depth, not a security boundary. PR review remains the primary control for recipe quality and security. Reviewers should still manually verify version patterns, especially for custom version formats that may not match common patterns.

### User Data Exposure

Not applicable. Detection operates on recipe content only; no user data involved.

## Consequences

### Positive

- **Prevents invalid recipes**: CI catches hardcoded versions before merge
- **Better contributor experience**: Clear feedback helps contributors fix issues
- **Improved recipe quality**: Registry contains only properly templated recipes
- **Reduced maintenance burden**: No more manual version bumps for hardcoded recipes

### Negative

- **Additional validation time**: Minor performance cost (regex scanning per field)
- **False positive risk**: Must tune patterns carefully to avoid flagging legitimate values
- **Maintenance of rules**: Action type rules must be updated when new actions are added

### Risks

- **Over-detection**: If patterns are too aggressive, contributors may disable validation
- **Under-detection**: If patterns are too narrow, some hardcoded versions slip through
- **Edge case complexity**: Some recipes may legitimately need suppression mechanisms

### Mitigations

- Start with high-confidence patterns (semver only) and expand based on real-world feedback
- Provide clear documentation on when static versions are acceptable
- Consider recipe-level annotation for suppression (future enhancement)
