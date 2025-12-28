# Version Transformations in Recipe URLs

## Status

Proposed

## Context and Problem Statement

Some tools use non-standard version formats in their download URLs that are incompatible with simple `{version}` substitution. The canonical example is SQLite:

**SQLite version format:**
- Semantic version: `3.51.1`
- Download URL: `https://sqlite.org/2025/sqlite-autoconf-3510100.tar.gz`
  - Year path component: `2025` (changes with releases)
  - Transformed version: `3510100` (format: `3XXYY00` where X.Y is the minor.patch)

This creates two problems:

1. **Recipe maintenance burden**: Recipes must hardcode URLs and version strings, requiring manual updates for each release
2. **Version provider mismatch**: Even when a version provider (e.g., Homebrew) correctly reports `3.51.1`, the download URL requires a computed transformation

Currently, tsuku has a `TransformVersion()` function in `internal/version/transform.go` that supports basic transformations (`semver`, `strip_v`, etc.), but:
- It is **not integrated** into the execution flow
- It only handles simple regex extractions, not computed transformations like SQLite's encoding

The existing transformations are useful for normalizing version output (e.g., extracting `2.3.8` from `biome@2.3.8`), but they cannot compute derived values like SQLite's year or version code.

### Scope

**In scope:**
- Computed version transformations (e.g., `{sqlite_version}` producing `3510100`)
- Year or date-based URL components
- Integration into the download action's variable expansion
- Recipe syntax for declaring custom transforms

**Out of scope:**
- Changes to version providers (they continue to return semantic versions)
- General-purpose templating language
- Year/date components that cannot be derived from version strings (requires separate design)

**Important clarification:** Transforms must execute at **plan generation time** (during `Decompose()`), not at execution time. This ensures deterministic, reproducible plans.

## Decision Drivers

- **Minimal recipe maintenance**: Version updates should not require recipe edits
- **Predictable computation**: Transforms must be deterministic and auditable
- **Security**: No arbitrary code execution; transforms are a closed set
- **Consistency**: Follow existing `{variable}` expansion patterns
- **Forward compatibility**: Unknown transforms should fail loudly, not silently

### Key Assumptions

1. **Version providers return semantic versions**: The Homebrew provider returns `3.51.1`, not `3510100`. The transformation happens at URL construction time.

2. **Transforms are tool-specific**: SQLite's encoding is unique to SQLite. We're not designing a general-purpose transform DSL; we're adding named transforms for known tools.

3. **Year components are out of scope**: The SQLite download year (`2025`) cannot be derived from the version string alone. This requires external metadata (release date, website parsing) and is a separate problem that warrants its own design.

4. **Transforms must be deterministic**: A transform given the same input version always produces the same output. This is required for reproducible plans.

5. **Unknown transforms should fail loudly**: Currently, unknown variables in `ExpandVars()` are left unchanged. The implementation must detect unexpanded variables and return clear errors.

## Existing Patterns

### Current tsuku Implementation

The `internal/version/transform.go` module provides basic transformations:

| Format | Behavior | Example |
|--------|----------|---------|
| `raw` | No transformation | `v1.2.3` → `v1.2.3` |
| `semver` | Extract X.Y.Z | `biome@2.3.8` → `2.3.8` |
| `semver_full` | Extract X.Y.Z-pre+build | `v1.2.3-rc.1` → `1.2.3-rc.1` |
| `strip_v` | Remove leading v | `v1.2.3` → `1.2.3` |

These are **not integrated** into the download action. The `version_format` field exists in recipe metadata but is only used for verification output normalization.

### Nixpkgs Pattern

The [nixpkgs sqlite package](https://github.com/NixOS/nixpkgs/blob/master/pkgs/development/libraries/sqlite/default.nix) uses a dedicated Nix function in `archive-version.nix`:

```nix
# Converts "3.50.2" to "3500200"
lib: version:
  let
    fragments = lib.splitVersion version;
    major = builtins.head fragments;
    rest = lib.concatMapStrings (v: lib.fixedWidthString 2 "0" v) (lib.tail fragments);
  in
  major + rest + "00"
```

This approach:
- Uses a dedicated function per encoding scheme
- Is deterministic and auditable
- Requires updating the Nix expression when adding new schemes

### Homebrew Pattern

The [Homebrew sqlite formula](https://formulae.brew.sh/formula/sqlite) handles version transformation in Ruby code within the formula itself. The formula hardcodes the encoded version in the URL and uses `livecheck` regex to track upstream releases.

### SQLite Version Encoding

SQLite uses two distinct encoding schemes:

1. **Filename encoding** (`3XXYY00`): For filenames to sort correctly
   - `3.51.1` → `3510100`
   - Algorithm: major + (minor padded to 2 digits) + (patch padded to 2 digits) + "00"

2. **SQLITE_VERSION_NUMBER** (`X*1000000 + Y*1000 + Z`): For C preprocessor
   - `3.51.1` → `3051001`
   - Formula: `major*1000000 + minor*1000 + patch`

The download URLs use the **filename encoding** scheme.

## Considered Options

### Decision 1: Transform Syntax

How should recipes declare version transformations?

#### Option 1A: Named Transform Variables (`{sqlite_version}`)

Add new built-in variables that apply tool-specific transformations:

```toml
[[steps]]
action = "download"
url = "https://sqlite.org/{sqlite_year}/sqlite-autoconf-{sqlite_version}.tar.gz"
```

**Pros:**
- Simple, readable syntax
- Consistent with existing `{version}`, `{os}`, `{arch}` patterns
- No new concepts for recipe authors

**Cons:**
- Pollutes the global variable namespace
- Each new tool encoding requires adding new variables
- Ambiguity: `{sqlite_version}` could look like a typo; silent failure if not recognized
- No composability: cannot chain transforms

#### Option 1B: Transform Function Syntax (`{version|sqlite}`)

Add a pipe operator for applying named transforms:

```toml
[[steps]]
action = "download"
url = "https://sqlite.org/{release_year|sqlite}/sqlite-autoconf-{version|sqlite}.tar.gz"
```

**Pros:**
- Clear separation between variable and transform
- Single `version` variable with multiple output formats
- Extensible without namespace pollution
- Composable: `{version|strip_v|sqlite}` chains transforms
- Self-documenting: readers immediately see what transform is applied

**Cons:**
- New syntax concept to learn
- Requires parser changes to handle pipe operator (current `ExpandVars()` is a simple `strings.ReplaceAll` loop)
- May conflict with shell-like expectations

#### Option 1C: Explicit Transform Block

Declare transforms separately and reference by name:

```toml
[version_transforms]
sqlite_version = { source = "version", format = "sqlite" }
sqlite_year = { source = "release_date", format = "year" }

[[steps]]
action = "download"
url = "https://sqlite.org/{sqlite_year}/sqlite-autoconf-{sqlite_version}.tar.gz"
```

**Pros:**
- Explicit declaration of what transforms are used
- Can reference different source data (version, release_date)
- Self-documenting within the recipe

**Cons:**
- More verbose
- Duplicates variable concepts
- Adds complexity to recipe structure

### Decision 2: Transform Implementation

Where and how should transforms be implemented?

#### Option 2A: Hardcoded Transforms in Go

Implement each transform as a Go function:

```go
var transforms = map[string]func(string) (string, error){
    "sqlite": transformSQLiteVersion,
    "sqlite_year": transformSQLiteYear,
}
```

**Pros:**
- Type-safe, tested, fast
- Full Go ecosystem for complex logic
- Can include validation
- Discoverable: `tsuku transforms list` could show available transforms
- Security: closed set prevents code injection

**Cons:**
- Requires code changes for new transforms
- Not extensible by users (intentional for security)

#### Option 2B: Regex-Based Transform Definitions

Define transforms as regex patterns in a registry file:

```toml
[transforms.sqlite]
pattern = "^(\\d+)\\.(\\d+)\\.(\\d+)$"
replacement = "${1}${2:02d}${3:02d}00"
```

**Pros:**
- Declarative, no code changes needed
- Could be extended by users via custom registries

**Cons:**
- Regex cannot express all transforms (e.g., year lookup)
- Limited to string manipulation
- Error-prone pattern writing
- No arithmetic: SQLite encoding requires computation that regex cannot express
- Format string complexity: expressing "pad to 2 digits with zeros" requires non-standard regex extensions

#### Option 2C: Expression Language

Add a simple expression language for computed values:

```toml
[[steps]]
action = "download"
url = "https://sqlite.org/{year(release_date)}/sqlite-autoconf-{pad(minor, 2)}{pad(patch, 2)}00.tar.gz"
```

**Pros:**
- Flexible, can express any computation
- No need to add new transforms for each tool

**Cons:**
- Complex to implement securely
- Turing-complete languages are security risks (even "simple" expression languages have had vulnerabilities)
- Steep learning curve for recipe authors
- Testing burden: need to verify expression safety for all recipes
- Would likely require a third-party expression library

### Decision 3: Year/Date Component Resolution

How should date-based URL components (like SQLite's `2025/`) be handled?

#### Option 3A: Version Provider Metadata

Extend version providers to return release date alongside version:

```go
type VersionInfo struct {
    Version     string
    ReleaseDate time.Time  // new field
}
```

**Pros:**
- Most accurate: uses actual release date
- Works for any tool with dated releases
- Single source of truth

**Cons:**
- Requires updating all version providers
- Not all providers have date information
- Increases API complexity

#### Option 3B: Year Derivation from Embedded Metadata

For SQLite specifically, the download page embeds release data in a parseable format. Create a specialized provider:

```toml
[version]
source = "sqlite"
formula = "sqlite"  # for compatibility
```

**Pros:**
- Uses authoritative source (SQLite's own page)
- Can extract exact year, checksum, URL

**Cons:**
- Only works for SQLite
- Adds maintenance burden (parsing HTML comments)
- Fragile if SQLite changes their format

#### Option 3C: Homebrew Fallback

Since SQLite uses Homebrew bottles anyway, avoid the source download issue entirely:

```toml
[[steps]]
action = "homebrew"
formula = "sqlite"
```

**Pros:**
- Works today with no changes
- Homebrew handles all URL complexity
- Pre-built binaries are faster
- Well-tested (Homebrew CI)
- Current SQLite recipe already uses this approach

**Cons:**
- Doesn't solve the general problem
- Some users may want to build from source
- Doesn't help other tools with similar issues

### Evaluation Matrix

| Criterion | 1A (Named Vars) | 1B (Pipe Syntax) | 1C (Transform Block) |
|-----------|-----------------|------------------|---------------------|
| Minimal maintenance | Fair | Good | Fair |
| Predictable | Good | Good | Good |
| Consistency | Good | Fair | Fair |
| Forward compat | Poor | Good | Good |

| Criterion | 2A (Hardcoded) | 2B (Regex) | 2C (Expression) |
|-----------|----------------|------------|-----------------|
| Security | Good | Good | Poor |
| Flexibility | Poor | Fair | Good |
| Maintainability | Fair | Good | Poor |

| Criterion | 3A (Provider Metadata) | 3B (SQLite Parser) | 3C (Homebrew) |
|-----------|------------------------|--------------------|--------------------|
| Generality | Good | Poor | Poor |
| Accuracy | Good | Good | Good |
| Implementation effort | High | Medium | None |

### Uncertainties

- **Other tools with similar issues**: We've only deeply analyzed SQLite. Other tools may have different transformation needs that influence the design. Candidates to investigate: FFmpeg (date-based snapshot versions), OpenSSL, Python (various encoding schemes), CMake.
- **Year component necessity**: It's unclear how many recipes actually need year/date components in URLs. This may warrant a separate design for version provider metadata.
- **Scope of source builds**: Understanding which tools are commonly built from source helps prioritize which transforms to implement.
- **Frequency of new transforms**: If rare, hardcoded Go is fine. If frequent, consider a declarative registry.

### Validation Required

Before proceeding with implementation, survey 5-10 additional tools to confirm the scope of version transformation needs. The `sqlite-source.toml` recipe serves as a reference implementation for tsuku's build-from-source capabilities, so solving this for SQLite validates the approach for other tools with similar challenges.

## Decision Outcome

**Chosen: 1B (Pipe Syntax) + 2A (Hardcoded Go Transforms)**

### Summary

Use pipe syntax (`{version|sqlite}`) with transforms implemented as a closed set of Go functions. This enables tools like `sqlite-source.toml` to build from source with automatic version resolution. Year/date components that cannot be derived from version strings are deferred to a separate design.

### Rationale

This combination was chosen because:

1. **Pipe syntax (1B)** provides the best balance of clarity and extensibility:
   - Composable: `{version|strip_v|sqlite}` chains transforms naturally
   - Self-documenting: readers see exactly what transform is applied
   - No namespace pollution: new transforms don't add global variables
   - Consistent with Unix pipe metaphor

2. **Hardcoded Go (2A)** ensures security and maintainability:
   - Closed set of transforms prevents code injection attacks
   - Type-safe with comprehensive test coverage
   - Each transform is auditable and deterministic
   - Aligns with decision driver: "No arbitrary code execution"

### Alternatives Rejected

- **Option 1A (Named Variables)**: Pollutes global namespace and lacks composability. Silent failure for unknown variables is dangerous.
- **Option 1C (Explicit Transform Block)**: Over-engineered for the expected use case. Most recipes don't need transforms.
- **Option 2B (Regex-Based)**: Cannot express arithmetic operations required for SQLite encoding.
- **Option 2C (Expression Language)**: Security risk outweighs flexibility benefits.
- **Option 3A (Version Provider Metadata)**: High implementation effort for marginal benefit.
- **Option 3B (SQLite-Specific Parser)**: Fragile and solves only one tool.

### Trade-offs Accepted

By choosing this approach, we accept:

1. **Year components remain unsolved**: The SQLite year (`2025`) cannot be derived from the version. A separate design is needed for version provider metadata or year derivation.

2. **New transforms require code changes**: Adding a new transform means modifying Go code and releasing a new tsuku version.

3. **Transforms are not user-extensible**: Users cannot define custom transforms. This is intentional for security.

These are acceptable because:

- Year components are a separate problem that can be addressed in a follow-up design
- New transforms are expected to be infrequent (SQLite-like encodings are unusual)
- User-defined transforms would be a security risk without sandboxing

## Solution Architecture

### Overview

The solution adds a transform layer to the existing variable expansion system. When a variable includes a pipe operator (e.g., `{version|sqlite}`), the expansion system applies named transforms from a closed registry before substitution.

```
Recipe Definition                Variable Expansion              Final URL
─────────────────                ──────────────────              ─────────
url = "...{version|sqlite}..."   →  ExpandVarsWithTransforms()   →  "...3510100..."
                                          │
                                          ↓
                                    Transform Registry
                                    ┌─────────────────┐
                                    │ sqlite: func()  │
                                    │ strip_v: func() │
                                    │ semver: func()  │
                                    └─────────────────┘
```

### Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Transform Registry | `internal/version/transforms.go` | Map of transform names to functions |
| Variable Expander | `internal/actions/util.go` | Parse pipes and apply transforms |
| Transform Functions | `internal/version/transforms.go` | Individual transform implementations |

### Key Interfaces

**Transform Function Signature:**

```go
// TransformFunc applies a named transformation to a version string.
// Returns the transformed string and any error.
type TransformFunc func(input string) (string, error)
```

**Transform Registry:**

```go
// TransformRegistry maps transform names to their implementations.
// This is a closed set - user-defined transforms are not supported.
var TransformRegistry = map[string]TransformFunc{
    "sqlite":      transformSQLiteVersion,
    "strip_v":     transformStripV,
    "semver":      transformSemver,
    "semver_full": transformSemverFull,
}
```

**Modified ExpandVars:**

```go
// ExpandVarsWithTransforms expands variables with optional transform support.
// Variables can include pipe-separated transforms: {version|strip_v|sqlite}
// Returns the expanded string and any error (unknown variables or transforms).
func ExpandVarsWithTransforms(s string, vars map[string]string) (string, error)
```

### Data Flow

1. **Recipe Parsing**: Recipe loader reads `{version|sqlite}` as a literal string
2. **Plan Generation**: During `Decompose()`, `ExpandVarsWithTransforms()` is called
3. **Variable Lookup**: Parser extracts `version` from `{version|sqlite}`, looks up value
4. **Transform Application**: Parser identifies `sqlite` transform, applies it to value
5. **Error Detection**: If variable or transform unknown, return error (not silent failure)
6. **Substitution**: Final transformed value replaces the entire `{version|sqlite}` pattern

### Error Handling

```go
// Example error messages
"unknown variable 'foo' in '{foo|sqlite}'"
"unknown transform 'bar' in '{version|bar}'"
"transform 'sqlite' failed: invalid version format '3.x'"
```

Errors are returned from `ExpandVarsWithTransforms()` and propagated to the user with context about which recipe and step failed.

## Implementation Approach

### Phase 1: Transform Infrastructure

Add the transform registry and modify variable expansion.

| Task | Description |
|------|-------------|
| Add `TransformFunc` type | Define function signature in `internal/version/transforms.go` |
| Create `TransformRegistry` | Map transform names to functions |
| Migrate existing transforms | Move `strip_v`, `semver`, `semver_full` from `transform.go` |
| Add `transformSQLiteVersion` | Implement SQLite filename encoding: `3.51.1` → `3510100` |
| Modify `ExpandVars` | Add `ExpandVarsWithTransforms()` with pipe parsing |
| Add unexpanded detection | Scan for remaining `{.*}` patterns after expansion |

**Dependencies:** None

### Phase 2: Action Integration

Update actions to use the new expansion function. Note: There are two expansion paths that must both be updated.

| Task | Description |
|------|-------------|
| Update `actions/util.go` | Add `ExpandVarsWithTransforms()` |
| Update `download.go` | Use `ExpandVarsWithTransforms()` in `Decompose()` and `Execute()` |
| Update `github_archive.go` | Use new expansion in URL construction |
| Update `executor/plan_generator.go` | Unify `expandVarsInString()` with the new function |
| Update other actions | Any action that expands URLs |
| Add error propagation | Ensure transform errors surface to user |

**Important:** The codebase has two expansion paths: `actions.ExpandVars()` and `executor.expandVarsInString()`. Both must be updated to support transforms, or unified into a single implementation.

**Dependencies:** Phase 1

### Phase 3: Recipe Updates and Testing

Update recipes and add comprehensive tests.

| Task | Description |
|------|-------------|
| Add transform tests | Unit tests for each transform function |
| Add expansion tests | Test pipe parsing, chaining, error cases |
| Add integration tests | Build recipe with transforms, verify URLs |
| Document transforms | Add to recipe authoring guide |

**Dependencies:** Phase 2

### Files to Modify

| File | Changes |
|------|---------|
| `internal/version/transforms.go` | Add `TransformFunc`, `TransformRegistry`, `transformSQLiteVersion` |
| `internal/actions/util.go` | Add `ExpandVarsWithTransforms()` with pipe parsing |
| `internal/actions/download.go` | Use new expansion function |
| `internal/actions/github_archive.go` | Use new expansion function |
| `internal/executor/plan_generator.go` | Unify `expandVarsInString()` with new function or delegate to it |

### Recipe Syntax Example

After implementation, a recipe could use:

```toml
[metadata]
name = "some-tool"
version_format = "semver"

[version]
source = "github"
owner = "example"
repo = "some-tool"

[[steps]]
action = "download"
url = "https://example.com/{version|strip_v|some_transform}.tar.gz"
```

## Consequences

### Positive

- **Enables automated version updates**: Recipes with transforms can track upstream versions without hardcoding
- **Composable transforms**: Chaining like `{version|strip_v|sqlite}` handles multiple format requirements
- **Clear error messages**: Unknown variables/transforms fail explicitly, not silently
- **Security by design**: Closed transform set prevents code injection

### Negative

- **Parser complexity**: `ExpandVarsWithTransforms()` is more complex than simple string replacement
- **Breaking change risk**: Changing `ExpandVars()` signature affects existing callers
- **Year components unsolved**: Recipes with date-based URL paths still need workarounds
- **Limited flexibility**: Users cannot add custom transforms

### Mitigations

- **Backward compatibility**: Keep `ExpandVars()` unchanged; add new `ExpandVarsWithTransforms()` for actions that need it
- **Year components**: Address in a follow-up design for version provider metadata; for now, year components may need to be hardcoded with manual updates
- **Limited flexibility**: Document the process for requesting new transforms; most tools don't need custom encoding

## Security Considerations

### Download Verification

**Not directly applicable** - this design does not change download verification behavior. Transforms only affect URL construction, not download integrity checks.

However, there is an indirect security consideration: **URL integrity**. If a transform produces an incorrect URL, the download may:
- Fail with a 404 (safe)
- Download a different file than intended (dangerous)

**Mitigation:** Transforms are deterministic functions with comprehensive test coverage. Each transform is validated against known input/output pairs before release.

### Execution Isolation

**Not applicable** - this feature does not execute downloaded content. It only performs string transformations on version strings to construct URLs.

The transform functions run in the same process as tsuku with the same permissions. They:
- Do not access the filesystem
- Do not make network calls
- Do not spawn subprocesses
- Are pure string transformations

### Supply Chain Risks

**Moderate risk** - transforms affect which URLs are constructed, which indirectly affects what gets downloaded.

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Malicious transform added to codebase | Low | High | Code review required for all transforms |
| Transform bug causes wrong download | Low | Medium | Comprehensive unit tests |
| Recipe uses incorrect transform | Medium | Low | Clear error messages for transform failures |

**Transform as attack vector:** A malicious transform could theoretically redirect downloads to attacker-controlled URLs. This is prevented by:
1. **Closed transform set**: Users cannot define custom transforms
2. **Code review**: All transforms are reviewed before merge
3. **Testing**: Transforms are tested against known-good outputs

### User Data Exposure

**Not applicable** - this feature does not access or transmit user data. Transforms operate only on:
- Version strings (from version providers)
- Standard variable values (`os`, `arch`, `version`)

No user-specific information (paths, credentials, identifiers) is involved in transform operations.

### Security Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious transform | Code review, closed set | Insider threat (accepted) |
| Incorrect URL construction | Unit tests, deterministic functions | Novel edge cases |
| Expression language injection | No expression language (Option 2C rejected) | None |
| User-defined transforms | Not supported (intentional) | None |

