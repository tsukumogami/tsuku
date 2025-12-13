# Homebrew Builder Infrastructure Integration Analysis

**Date:** 2025-12-13
**Purpose:** Document existing tsuku patterns to inform Homebrew LLM builder implementation

## Executive Summary

The tsuku codebase has a mature LLM builder infrastructure proven by `GitHubReleaseBuilder`. Homebrew support already exists as an action (`homebrew_bottle`) with sophisticated bottle fetching from GHCR. This analysis identifies reusable patterns and integration points for a future `HomebrewBuilder`.

## 1. Builder Interface

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/builder.go`

### Interface Definition

```go
type Builder interface {
    Name() string
    CanBuild(ctx context.Context, packageName string) (bool, error)
    Build(ctx context.Context, req BuildRequest) (*BuildResult, error)
}
```

### BuildRequest Structure

```go
type BuildRequest struct {
    Package   string  // Tool name (e.g., "gh", "ripgrep")
    Version   string  // Optional specific version (empty = latest)
    SourceArg string  // Builder-specific argument
}
```

**For HomebrewBuilder:**
- `Package`: User-facing tool name
- `SourceArg`: Homebrew formula name (may differ from package name)
- Example: `tsuku create libyaml --from homebrew:libyaml`

### BuildResult Structure

```go
type BuildResult struct {
    Recipe            *recipe.Recipe
    Warnings          []string
    Source            string  // e.g., "homebrew:libyaml"
    RepairAttempts    int     // Number of LLM repair cycles
    Provider          string  // LLM provider name ("claude", "gemini")
    ValidationSkipped bool
    Cost              float64 // USD cost for LLM calls
}
```

**Key Fields for Homebrew:**
- `RepairAttempts`: Track repair loop iterations (max 2)
- `Provider`: Which LLM generated the recipe
- `ValidationSkipped`: True if no container runtime available
- `Cost`: Token costs from LLM generation

### Existing Builder Reference: CargoBuilder

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go`

Pattern observed:
1. **CanBuild**: Queries ecosystem API to verify package exists
2. **Build**: Fetches metadata, discovers executables, generates recipe
3. Non-LLM builder: Simpler, deterministic recipe generation

**Contrast with GitHubReleaseBuilder (LLM-based):**
1. Uses LLM factory to get provider
2. Runs conversation loop with tools
3. Validation + repair loop (up to 2 attempts)
4. Progress reporting via callbacks

## 2. Existing Homebrew Support

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/homebrew_bottle.go`

### Current Capabilities

The `homebrew_bottle` action is a fully-functional primitive for installing Homebrew bottles:

#### Bottle Fetching Process

1. **Anonymous GHCR Token**: `getGHCRToken(formula)` - Obtains OAuth token
2. **Manifest Query**: `getBlobSHA(formula, version, platform, token)` - Finds platform blob
3. **Download**: `downloadBottle(formula, blobSHA, token, path)` - Fetches tarball
4. **SHA256 Verification**: `verifySHA256(path, expectedSHA)` - Integrity check
5. **Extraction**: Delegates to `ExtractAction` with `strip_dirs: 2`
6. **Placeholder Relocation**: `relocatePlaceholders(dir, installPath)`

#### Placeholder Relocation Logic

Homebrew bottles contain placeholders that must be replaced:

```go
var homebrewPlaceholders = [][]byte{
    []byte("@@HOMEBREW_PREFIX@@"),
    []byte("@@HOMEBREW_CELLAR@@"),
}
```

**Text files:** Direct string replacement
**Binary files:** RPATH fixup using:
- Linux: `patchelf --set-rpath $ORIGIN` (or `$ORIGIN/../lib`)
- macOS: `install_name_tool -add_rpath @loader_path` + `codesign -f -s -`

#### Platform Tags

```go
func getPlatformTag(os, arch string) (string, error) {
    switch {
    case os == "darwin" && arch == "arm64":  return "arm64_sonoma", nil
    case os == "darwin" && arch == "amd64":  return "sonoma", nil
    case os == "linux" && arch == "arm64":   return "arm64_linux", nil
    case os == "linux" && arch == "amd64":   return "x86_64_linux", nil
    default: return "", fmt.Errorf("unsupported platform")
    }
}
```

**Note:** macOS platform tags are hardcoded to Sonoma. This may need LLM intelligence to handle version-specific bottles (e.g., `ventura`, `monterey`).

### What Can Be Reused

1. **GHCR fetching**: Token, manifest, blob download - all working
2. **Placeholder relocation**: Complex binary patching logic
3. **Platform tag mapping**: OS/arch to Homebrew conventions
4. **Extraction workflow**: `strip_dirs: 2` pattern

### What Needs Extension

1. **Formula metadata**: Need to fetch from Homebrew API (description, homepage)
2. **Binary discovery**: LLM must determine which files are executables vs libraries
3. **Version handling**: Homebrew API only exposes stable version, not historical versions
4. **Dependency handling**: Bottles have dependencies - LLM must signal DependencyMissing

## 3. Action System

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/action.go`

### Action Interface

```go
type Action interface {
    Name() string
    Execute(ctx *ExecutionContext, params map[string]interface{}) error
}
```

### ExecutionContext

Provides runtime context for actions:

```go
type ExecutionContext struct {
    Context          context.Context
    WorkDir          string            // Temp directory
    InstallDir       string            // ~/.tsuku/tools/.install/
    ToolInstallDir   string            // ~/.tsuku/tools/{name}-{version}/
    ToolsDir         string            // ~/.tsuku/tools/
    DownloadCacheDir string            // ~/.tsuku/cache/downloads/
    Version          string            // Resolved version
    VersionTag       string            // Original tag (may include "v" prefix)
    OS               string            // runtime.GOOS
    Arch             string            // runtime.GOARCH
    Recipe           *recipe.Recipe
    ExecPaths        []string          // Additional PATH entries
    Resolver         *version.Resolver // For GitHub API, etc.
    Logger           log.Logger
}
```

### Registered Actions (Relevant to Homebrew)

From `init()` function:

**Atomic actions:**
- `download`, `extract`, `chmod`, `install_binaries`, `set_env`, `run_command`
- `set_rpath`, `install_libraries`, `link_dependencies`

**Composite actions:**
- `homebrew_bottle` - **Already implemented**
- `download_archive`, `github_archive`, `github_file`

**Ecosystem primitives:**
- `npm_install`, `pipx_install`, `cargo_install`, `gem_install`, `cpan_install`, `go_install`, `nix_install`

### Actions HomebrewBuilder Would Use

A Homebrew-generated recipe would primarily use:

1. **`homebrew_bottle`** - Fetch and extract bottle from GHCR
2. **`install_binaries`** - Symlink executables to `~/.tsuku/bin`
3. **`install_libraries`** - Install shared libraries if needed
4. **`chmod`** - Fix permissions if bottle has issues

**Example recipe structure (predicted):**

```toml
[metadata]
name = "libyaml"
description = "YAML Parser"
homepage = "https://github.com/yaml/libyaml"

[version]
source = "homebrew"
formula = "libyaml"

[[steps]]
action = "homebrew_bottle"
formula = "libyaml"

[[steps]]
action = "install_binaries"
binaries = ["bin/yaml-config"]

[verify]
command = "yaml-config --version"
```

**Atomic vs Composite Pattern:**

- `homebrew_bottle` is composite (calls `ExtractAction`, `relocatePlaceholders`)
- LLM would generate a recipe using `homebrew_bottle`, not lower-level actions

## 4. Version Provider

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/version/provider_homebrew.go`

### Existing HomebrewProvider

```go
type HomebrewProvider struct {
    resolver *Resolver
    formula  string
}

func NewHomebrewProvider(resolver *Resolver, formula string) *HomebrewProvider
```

### Methods

```go
func (p *HomebrewProvider) ListVersions(ctx context.Context) ([]string, error)
func (p *HomebrewProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error)
func (p *HomebrewProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error)
func (p *HomebrewProvider) SourceDescription() string
```

### Implementation Details

**API:** `https://formulae.brew.sh/api/formula/{formula}.json`

**Response structure (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/version/homebrew.go`):**

```go
type homebrewFormulaInfo struct {
    Name     string
    FullName string
    Versions struct {
        Stable string
        Head   string
        Bottle bool
    }
    Revision      int
    VersionScheme int
    Deprecated    bool
    Disabled      bool
    Versioned     []string // e.g., ["openssl@1.1", "openssl@3"]
}
```

### Limitation for LLM Builder

**Only exposes stable version:**
- `ResolveLatest()` returns `Versions.Stable`
- `ListVersions()` returns stable + versioned formulae (e.g., `openssl@1.1`)
- No historical versions available via API

**Implication:**
- HomebrewBuilder cannot target specific historical versions
- Must use latest stable or versioned formula
- LLM must understand this constraint

### What LLM Builder Needs from Version Provider

1. **Formula existence check**: `CanBuild()` would call `ResolveLatest()` to verify formula exists
2. **Metadata extraction**: Description, homepage from `homebrewFormulaInfo`
3. **Deprecation check**: Refuse to build if `Deprecated: true` or `Disabled: true`
4. **Bottle availability**: Check `Versions.Bottle` field

## 5. Integration Points

### Where HomebrewBuilder Should Live

**Recommended location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/homebrew_builder.go`

**Registry integration:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/registry.go`

```go
type Registry struct {
    mu       sync.RWMutex
    builders map[string]Builder
}

func (r *Registry) Register(b Builder)
func (r *Registry) Get(name string) (Builder, bool)
```

**Pattern:** Builder registered by name, CLI routes `--from homebrew:formula` to it

### How HomebrewBuilder Should Call LLM Client

**LLM Factory:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/llm/factory.go`

```go
factory, err := llm.NewFactory(ctx, llm.WithConfig(cfg))
provider, err := factory.GetProvider(ctx)
```

**Provider interface:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/llm/provider.go`

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
}

type CompletionRequest struct {
    SystemPrompt string
    Messages     []Message
    Tools        []ToolDef
    MaxTokens    int
}
```

**Conversation loop pattern (from GitHubReleaseBuilder):**

1. Build system prompt (Homebrew-specific instructions)
2. Build initial user message (formula metadata, bottle manifest)
3. Define tools: `fetch_formula_file`, `inspect_bottle`, `extract_pattern`
4. Loop until `extract_pattern` called or max turns reached
5. Execute tool calls, append results to messages
6. Return extracted pattern

### How HomebrewBuilder Should Handle Dependencies

**Parent design protocol (from DESIGN-llm-recipe-builder.md):**

```
Dependencies signal up, not recurse:
- If formula requires libyaml, return DependencyMissing error
- Parent layer decides whether to install dependency or abort
- Prevents infinite recursion in builder layer
```

**Implementation:**

```go
// In homebrew_builder.go
type HomebrewDependencyError struct {
    Formula      string
    Dependencies []string
}

func (e *HomebrewDependencyError) Error() string {
    return fmt.Sprintf("formula %s requires dependencies: %s",
        e.Formula, strings.Join(e.Dependencies, ", "))
}
```

**LLM prompt should instruct:**
> "If this formula has runtime dependencies (check bottle manifest or formula JSON),
> call a hypothetical report_dependencies tool instead of extract_pattern.
> Dependencies must be installed separately."

**Note:** Tool use protocol may need extension for dependency reporting.

### Validation Integration

**Executor:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/executor.go`

```go
type Executor struct {
    detector      *RuntimeDetector
    predownloader *PreDownloader
    logger        log.Logger
    image         string
    limits        ResourceLimits
    tsukuBinary   string
}

func (e *Executor) Validate(ctx context.Context, r *recipe.Recipe, assetURL string) (*ValidationResult, error)
```

**Validation flow:**
1. Detect container runtime (Podman/Docker)
2. Serialize recipe to TOML
3. Mount tsuku binary into container
4. Run `tsuku install {formula}` in isolated container
5. Check verification command output
6. Return pass/fail

**HomebrewBuilder integration (from GitHubReleaseBuilder pattern):**

```go
// After LLM generates recipe pattern
r, err := generateRecipe(packageName, formulaName, formulaMeta, pattern)

// Build bottle URL for validation
bottleURL := buildBottleURL(formulaName, version, platform)

// Validate in container
result, err := b.executor.Validate(ctx, r, bottleURL)

if !result.Passed {
    // Build repair message
    repairMsg := b.buildRepairMessage(result)
    messages = append(messages, llm.Message{Role: llm.RoleUser, Content: repairMsg})
    // Continue conversation loop for repair
}
```

### Error Sanitization

**Sanitizer:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/sanitize.go`

**Purpose:** Remove sensitive data before sending errors to LLM

**Redaction patterns:**
- Home directories: `/home/user` → `$HOME`, `C:\Users\user` → `%USERPROFILE%`
- IP addresses: `192.168.1.1` → `[IP]`
- Credentials: `api_key=xxx` → `[REDACTED]`
- Max length: 2000 chars (truncate with `... [truncated]`)

**Usage in repair loop:**

```go
sanitizer := validate.NewSanitizer()
sanitizedOutput := sanitizer.Sanitize(result.Stdout + "\n" + result.Stderr)
// Send sanitized output to LLM for repair analysis
```

### Error Parsing

**Parser:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/errors.go`

**Categories:**
- `binary_not_found` - Executable not found after install
- `extraction_failed` - Archive corruption or format mismatch
- `verify_failed` - Verification command failed
- `permission_denied` - File permission issues
- `download_failed` - Network or 404 errors

**Usage:**

```go
parsed := validate.ParseValidationError(result.Stdout, result.Stderr, result.ExitCode)
// parsed.Category used for telemetry
// parsed.Suggestions included in repair prompt
```

**HomebrewBuilder-specific errors to expect:**
- `binary_not_found` - LLM guessed wrong executable name
- `extraction_failed` - Wrong `strip_dirs` value
- `download_failed` - Platform tag mismatch (e.g., requested `sonoma` but only `ventura` exists)

## 6. LLM Infrastructure Patterns

### Provider System

**Auto-detection (from factory.go):**

```go
// Claude: ANTHROPIC_API_KEY
if os.Getenv("ANTHROPIC_API_KEY") != "" {
    provider, _ := NewClaudeProvider()
    factory.providers["claude"] = provider
}

// Gemini: GOOGLE_API_KEY or GEMINI_API_KEY
if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
    provider, _ := NewGeminiProvider(ctx)
    factory.providers["gemini"] = provider
}
```

**Failover with circuit breaker:**

```go
func (f *Factory) GetProvider(ctx context.Context) (Provider, error) {
    // Try primary provider first
    if provider, ok := f.providers[f.primary]; ok {
        if breaker := f.breakers[f.primary]; breaker.Allow() {
            return provider, nil
        }
    }
    // Fallback to any available provider
}
```

### Tool Definitions

**Tool names (from tools.go):**

```go
const (
    ToolFetchFile      = "fetch_file"
    ToolInspectArchive = "inspect_archive"
    ToolExtractPattern = "extract_pattern"
)
```

**For HomebrewBuilder, analogous tools:**

```go
const (
    ToolFetchFormulaFile   = "fetch_formula_file"   // Fetch .rb file from Homebrew tap
    ToolInspectBottle      = "inspect_bottle"       // Download bottle, list contents
    ToolExtractPattern     = "extract_pattern"      // Signal completion
)
```

**Tool definition pattern:**

```go
func buildToolDefs() []llm.ToolDef {
    return []llm.ToolDef{
        {
            Name:        "fetch_formula_file",
            Description: "Fetch the Homebrew formula file (.rb) to understand dependencies and build metadata.",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "formula": map[string]any{
                        "type":        "string",
                        "description": "Formula name (e.g., 'libyaml')",
                    },
                },
                "required": []string{"formula"},
            },
        },
        // ...
    }
}
```

### Repair Loop Pattern

**From GitHubReleaseBuilder.generateWithRepair():**

```go
for attempt := 0; attempt <= MaxRepairAttempts; attempt++ {
    // Run conversation to get pattern
    pattern, usage, err := b.runConversationLoop(...)
    totalUsage.Add(*usage)

    // Skip validation if no executor
    if b.executor == nil {
        return pattern, &totalUsage, repairAttempts, true, nil
    }

    // Generate recipe and validate
    r, err := generateRecipe(...)
    result, err := b.executor.Validate(ctx, r, assetURL)

    if result.Passed {
        return pattern, &totalUsage, repairAttempts, false, nil
    }

    // Build repair message
    repairMsg := b.buildRepairMessage(result)
    messages = append(messages, llm.Message{Role: llm.RoleUser, Content: repairMsg})
    repairAttempts++
}

return nil, &totalUsage, repairAttempts, false,
    fmt.Errorf("recipe validation failed after %d repair attempts", repairAttempts)
```

**Key elements:**
1. Max 2 repair attempts (3 total tries)
2. Validation optional (skip if no container runtime)
3. Sanitize errors before sending to LLM
4. Parse error category for telemetry
5. Track usage across all turns

## 7. Key Technical Decisions

### Homebrew-Specific Challenges

1. **Platform tag complexity**: Homebrew uses OS-version tags (`sonoma`, `ventura`, `monterey`) not just `darwin`. LLM must infer which to use or try fallbacks.

2. **Binary vs library distinction**: Bottles contain both executables and shared libraries. LLM must determine which files go to `~/.tsuku/bin` vs stay in tool directory.

3. **Dependency handling**: Homebrew bottles have dependencies (e.g., `libyaml` → `libffi`). LLM must detect and signal dependencies rather than recursing.

4. **Version constraints**: Homebrew API only exposes stable version. Builder cannot target historical versions unless using versioned formulae (`python@3.11`).

5. **Placeholder relocation**: Already implemented, but LLM must understand that binaries require RPATH fixup, which `homebrew_bottle` action handles automatically.

### Recommended LLM Prompt Structure

**System prompt should include:**

```
You are an expert at analyzing Homebrew bottles to create installation recipes for tsuku.

Homebrew bottles are pre-built binaries hosted on GitHub Container Registry (GHCR).
Each bottle is a tarball with the following structure:
- Top-level directories: {formula}/{version}/
- Executables typically in: bin/, sbin/
- Libraries typically in: lib/
- Placeholders (@@HOMEBREW_PREFIX@@) are auto-relocated by tsuku

Your task:
1. Analyze the formula metadata to understand what this package provides
2. Determine which files are user-facing executables vs internal libraries
3. Identify runtime dependencies (if any, signal DependencyMissing)
4. Choose appropriate platform tags (darwin: arm64_sonoma, sonoma; linux: x86_64_linux, arm64_linux)
5. Call extract_pattern with the installation plan

Tools available:
- fetch_formula_file: Read the .rb file to understand dependencies
- inspect_bottle: Download a bottle and list its contents
- extract_pattern: Signal you're ready to generate the recipe
```

**User message should include:**

```
Formula: {formula_name}
Description: {description}
Stable version: {stable_version}
Has bottle: {bottle_available}
Versioned formulae: {versioned_list}

Available bottles (from GHCR manifest):
- arm64_sonoma.{version}.tar.gz
- sonoma.{version}.tar.gz
- x86_64_linux.{version}.tar.gz
- arm64_linux.{version}.tar.gz

Please analyze this formula and determine:
1. Which binaries should be installed to ~/.tsuku/bin
2. Whether it has runtime dependencies
3. The appropriate verification command
```

### Differences from GitHubReleaseBuilder

| Aspect | GitHubReleaseBuilder | HomebrewBuilder |
|--------|---------------------|-----------------|
| **Asset source** | GitHub releases API | Homebrew API + GHCR |
| **Asset pattern** | Inferred from asset names | Fixed GHCR convention |
| **Platform tags** | Flexible (LLM infers) | Homebrew-specific tags |
| **Archive structure** | Variable | Always `{formula}/{version}/` |
| **Version handling** | Full release history | Only stable + versioned formulae |
| **Dependencies** | Rare | Common (bottles reference other bottles) |
| **Post-extraction** | None | Placeholder relocation required |
| **Verification** | Typical `--version` | May need library-specific commands |

## 8. Recommended Implementation Approach

### Phase 1: Non-LLM Builder (Deterministic)

Build a simple HomebrewBuilder that:
1. Queries Homebrew API for formula metadata
2. Generates a recipe using `homebrew_bottle` action
3. Hardcodes binary discovery (use formula name as executable)
4. No repair loop, no validation

**Purpose:** Validate integration points without LLM complexity

### Phase 2: LLM Builder with Validation

Add LLM intelligence:
1. Implement conversation loop with tools
2. Let LLM discover executables from bottle contents
3. Add container validation
4. Implement repair loop (max 2 attempts)

**Purpose:** Prove LLM can improve upon hardcoded heuristics

### Phase 3: Dependency Handling

Extend protocol:
1. Add dependency detection tool
2. Return `HomebrewDependencyError` when dependencies found
3. Let parent layer decide dependency installation order

**Purpose:** Enable complex formulae with dependencies

## 9. Open Questions

1. **Platform tag selection**: Should LLM try multiple tags (sonoma → ventura fallback) or hardcode to latest macOS version?

2. **Binary discovery heuristics**: Should we fetch bottle during `CanBuild()` to verify it's viable, or defer until `Build()`?

3. **Dependency recursion limit**: How deep should dependency chains go? (e.g., A → B → C → ...)

4. **Version provider integration**: Should `HomebrewProvider` expose bottle availability in `VersionInfo`?

5. **Action parameter design**: Should `homebrew_bottle` take explicit `binaries` param, or discover from bottle contents?

## 10. References

### Key Files Reviewed

- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/builder.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/github_release.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/homebrew_bottle.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/action.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/version/provider_homebrew.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/version/homebrew.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/llm/provider.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/llm/factory.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/llm/tools.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/executor.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/sanitize.go`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/errors.go`

### Design Documents

- Parent design: `DESIGN-llm-recipe-builder.md` (vision repo)
- Builder failure protocol: NotFound, Unsupported, DependencyMissing, ValidationFailed, Transient, Configuration
- Container validation: Network isolation (`--network=none` for verification phase)
- Multi-provider LLM: Claude, Gemini with tool use
- Repair loop: Iterative improvement with sanitized error feedback
- Dependency protocol: Signal up, not recurse

---

**Conclusion:** The tsuku codebase provides a complete LLM builder infrastructure. Homebrew support exists as a mature action, but lacks intelligent binary discovery and dependency handling. A HomebrewBuilder can leverage existing patterns from GitHubReleaseBuilder while addressing Homebrew-specific challenges (platform tags, dependency signaling, bottle structure parsing).
