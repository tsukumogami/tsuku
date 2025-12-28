# Design: Metadata Command for Static Recipe Introspection

## Status

Proposed

## Context and Problem Statement

Automation tooling needs programmatic access to recipe metadata without executing installation plans. Currently, scripts that need to query recipe properties (supported platforms, version sources, dependencies) must either:

1. Parse TOML files directly (bypassing tsuku's validation and normalization)
2. Attempt plan generation with `tsuku plan`, which requires network calls to resolve versions

This creates friction for automation scenarios like:
- Testing frameworks that need to generate platform-specific golden plans
- CI pipelines that need to query platform support before attempting installation
- Documentation tooling that needs to extract recipe information for website generation
- Development workflows that need to validate recipe changes before committing

The `tsuku info` command exists for querying tool information, but it combines recipe metadata with installation state (whether the tool is installed, what version, location). It cannot be used with `--recipe` for testing local recipe files, and it doesn't provide clean programmatic access to static recipe properties.

The `tsuku eval` command can output plan JSON, but requires version resolution and constructs full installation plans. For queries about static recipe properties, this is unnecessarily expensive and requires network access.

### Scope

**In scope:**
- Read static metadata from recipes (local registry, remote registry, or `--recipe` file path)
- Output structured JSON with all statically available recipe properties
- Platform support detection from recipe constraints
- Version provider configuration (source type, repo/module identifiers)

**Out of scope:**
- Version resolution (use `tsuku eval` for that)
- Installation state (use `tsuku info` for that)
- Plan generation (use `tsuku plan` or `tsuku eval` for that)
- Recipe validation beyond what the parser provides
- Dependency resolution (requires graph traversal, use `tsuku info` for that)
- Platform-specific step filtering (no `--os`/`--arch` flags - use `tsuku eval` for that)

## Decision Drivers

- **Symmetry with existing commands**: Should follow patterns established by `tsuku install` and `tsuku eval` for loading recipes
- **Programmatic automation**: Output must be machine-readable (JSON) for scripting and CI integration
- **No network dependency**: Should work offline by reading recipe files only
- **No execution**: Should not trigger version resolution, plan generation, or installation checks
- **Completeness**: Should expose all static recipe properties, not a curated subset
- **Performance for bulk operations**: CI and testing workflows may query hundreds of recipes; command should be fast enough for loops
- **Discoverability**: Users familiar with `tsuku info` should understand when to use `metadata` instead
- **Stable output schema**: Metadata output becomes a programmatic interface; breaking changes should be intentional

## Implementation Context

### Existing Patterns

**Command structure** (`cmd/tsuku/`):
- Commands use Cobra library with standard pattern: `Use`, `Short`, `Long`, `Args`, `Run`
- Registered in `main.go` via `rootCmd.AddCommand()` in `init()` function
- Flags defined in command-specific `init()` functions
- Read-only commands follow simple pattern: load data, format output, exit

**Recipe loading** (`cmd/tsuku/helpers.go`, `install_sandbox.go`):
- Global `loader *recipe.Loader` shared across commands (initialized in main.go)
- Standard loading: `loader.Get(toolName)` searches local/embedded/registry
- File loading: `loadLocalRecipe(path)` wraps `recipe.ParseFile(path)`
- Priority: in-memory cache → local recipes → embedded → registry

**--recipe flag pattern** (`cmd/tsuku/eval.go`, `install.go`):
- Mutually exclusive with tool name argument
- Validation: `if recipePath != "" && len(args) > 0` → error
- File mode loads via `loadLocalRecipe(path)`
- Registry mode loads via `loader.Get(toolName)`

**JSON output pattern** (`cmd/tsuku/helpers.go`, `info.go`, `versions.go`):
- Shared helper: `printJSON(v interface{})` with 2-space indent
- Define inline struct with JSON tags in Run function
- Boolean flag: `cmd.Flags().Bool("json", false, "Output in JSON format")`
- Conditional output: check flag, print JSON or human-readable

### Conventions to Follow

- **Error handling**: Use `printError(err)` + `exitWithCode(code)` from helpers.go
- **Exit codes**: Use constants from exitcodes.go (ExitRecipeNotFound, ExitUsage, etc.)
- **Output suppression**: Respect `--quiet` flag via `printInfo()` helpers
- **Recipe struct**: Use `recipe.Recipe` from `internal/recipe/types.go`
- **Platform methods**: Use `r.GetSupportedPlatforms()`, `r.SupportsPlatform(os, arch)`

### Similar Implementations

**info.go** - Closest analog (lines 10-108):
- Loads recipe via `loader.Get(toolName)`
- Outputs JSON or human-readable format based on `--json` flag
- JSON structure includes metadata, platform constraints, dependencies
- Does NOT support `--recipe` flag (only registry lookups)

**eval.go** - Recipe loading pattern (lines 110-158):
- Supports both tool name AND `--recipe` file path (mutually exclusive)
- Validates flag combinations before proceeding
- Sets `recipeSource` variable to track origin (registry vs file path)
- Performs expensive operations (version resolution, plan generation)

**versions.go** - Simple read-only command (lines 9-75):
- Takes tool name, loads recipe, performs operation
- JSON output with inline struct definition
- No state changes, no installation dependencies

### Anti-patterns to Avoid

- **Mixing concerns**: Don't combine metadata output with installation state (that's `info`'s job)
- **Network calls**: Don't trigger version resolution (that's `eval`'s job)
- **Incomplete --recipe support**: If adding the flag, ensure it works same as tool name mode

## Considered Options

### Decision 1: Command Interface

How should users specify which recipe to query?

#### Option 1A: Tool Name Only

`tsuku metadata <tool>` - Only supports registry lookups, no --recipe flag.

**Pros:**
- Simple, minimal implementation
- Consistent with `versions`, `info` commands
- Clear scope: registry recipes only

**Cons:**
- Cannot test local recipe files before committing
- Breaks symmetry with `install` and `eval` which support --recipe
- Limits usefulness for recipe development workflow
- Golden plan testing (the motivating use case) needs to test uncommitted recipes

#### Option 1B: Tool Name OR --recipe (Mutually Exclusive)

`tsuku metadata <tool>` OR `tsuku metadata --recipe <path>` - Matches `eval` pattern.

**Pros:**
- Symmetric with `eval` command (established pattern)
- Supports recipe development workflow (test before commit)
- Enables golden plan testing with local modifications
- Follows existing validation pattern for mutual exclusivity

**Cons:**
- Slightly more complex argument validation
- Need to handle both code paths (registry vs file)
- More surface area for edge cases

### Decision 2: Output Format

Should the command support human-readable output or be JSON-only?

#### Option 2A: JSON Only

Always output JSON, no --json flag needed.

**Pros:**
- Clear signal that this is for automation, not humans
- Simpler implementation (one code path)
- Avoids maintaining two output formats
- Matches primary use case (programmatic consumption)

**Cons:**
- Less friendly for exploratory use (humans reading raw JSON)
- Inconsistent with existing commands (info, versions, list all support both)
- Requires piping to `jq` for human-readable viewing

#### Option 2B: Dual Format (JSON flag like info/versions)

`--json` flag toggles between human-readable and JSON output (default: human-readable).

**Pros:**
- Consistent with `info`, `versions`, `list` commands
- Friendly for both automation and exploration
- Users can read output directly for debugging

**Cons:**
- Two output formats to maintain
- Human-readable format needs design (what to show, how to format)
- Additional code complexity

### Decision 3: Output Schema Granularity

How much control should users have over what fields are returned?

#### Option 3A: Full Dump Only

Always output all recipe metadata fields, no filtering.

**Pros:**
- Simple implementation (no field selection logic)
- Complete information for all use cases
- Users can filter with `jq` if needed
- No need to maintain field selector syntax

**Cons:**
- Large output for simple queries (e.g., just want supported platforms)
- Wastes bandwidth for CI/automation with specific needs
- Forces users to parse entire structure for single field

#### Option 3B: Optional Field Selection

Support filtering to specific fields (e.g., `tsuku metadata <tool> --field platforms --field version_source`).

**Pros:**
- Efficient for targeted queries
- Reduces noise for automation scripts
- More flexible for different use cases

**Cons:**
- Complex implementation (field selector parsing, validation)
- Need to define field names/paths (dot notation for nested?)
- Maintenance burden (what if recipe structure changes?)
- Unclear what field names should be (camelCase? snake_case? TOML keys?)


### Decision 4: Platform Information Representation

How should platform support be represented in output?

#### Option 4A: Structured Platform Constraints

Output the raw recipe fields (`supported_os`, `supported_arch`, `unsupported_platforms`).

**Pros:**
- Faithful to recipe structure
- No computation required
- Users can implement their own platform logic
- Matches TOML schema exactly

**Cons:**
- Requires users to understand complementary hybrid model
- Automation scripts must reimplement platform matching logic
- Doesn't directly answer "does this recipe support linux/arm64?"

#### Option 4B: Computed Platform List

Include a `supported_platforms` array with all valid "os/arch" tuples.

**Pros:**
- Directly usable (no logic required)
- Answers "what platforms are supported?" explicitly
- Uses existing `GetSupportedPlatforms()` method (platform.go:98-111) with test coverage
- Golden plan testing can iterate over this list
- Handles complementary hybrid model (allowlist - denylist) correctly

**Cons:**
- Duplicates information that's already in constraints
- Slightly more computation (but trivial - just array generation)
- Output schema diverges from recipe TOML structure
- Exposes platform computation as stable API surface (requires compatibility on schema changes)

#### Option 4C: Both Raw and Computed

Include both constraint fields and computed platform list.

**Pros:**
- Serves both use cases (understanding constraints, automation)
- No information loss
- Maximum flexibility

**Cons:**
- Redundant information in output
- Larger payload
- Potentially confusing (which should users rely on?)

### Cross-Decision Interactions

The four decisions interact in important ways:

**Interface × Platform (1 × 4):**
- If supporting `--recipe` flag (1B), malformed platform constraints in external recipes will expose different error behavior:
  - Option 4A: Silent (raw output of invalid constraints)
  - Options 4B/4C: Visible error during platform computation

**Output Format × Granularity (2 × 3):**
- JSON-only (2A) makes field selection (3B) more valuable since users can't just read what they need
- Dual format (2B) reduces pressure for field selection since humans can scan human-readable output

**Granularity × Platform (3 × 4):**
- Full dump only (3A) pairs naturally with both raw and computed (4C) - provide everything
- Field selection (3B) with both raw+computed (4C) doubles cognitive load (which field names?)

**Output Format × Interface (2 × 1):**
- Dual format (2B) with --recipe (1B) requires testing both output modes with file loading edge cases
- JSON-only (2A) simplifies testing matrix

### Key Assumptions

1. **Recipe schema stability**: The metadata output JSON schema will become a programmatic interface. Breaking changes to recipe fields (`metadata`, `version`, `steps` structure) require corresponding versioning of the metadata output or risk breaking automation scripts.

2. **Platform computation reliability**: Options 4B and 4C depend on `GetSupportedPlatforms()` being correct. The method has test coverage in platform_test.go, but edge cases (invalid constraints, empty arrays) should be validated before exposing as canonical API.

3. **No version-aware metadata**: Metadata is extracted from recipe files as-is. If a recipe uses version-dependent templating (e.g., different binaries for different versions), this command cannot expose that without version resolution (which is explicitly out of scope).

4. **Parser validation sufficiency**: Relying on `recipe.ParseFile()` validation means invalid TOML files will error cleanly, but semantic issues (e.g., nonsensical platform constraints like `supported_os=["windows"]` in a Linux-only tool) won't be caught.

## Decision Outcome

**Chosen: 1B (Tool Name OR --recipe) + 2B (Dual Format) + 3A (Full Dump) + 4C (Both Raw and Computed)**

### Summary

The `tsuku metadata` command will support both registry lookups and local recipe files via `--recipe` flag (mirroring `eval`), output in both JSON and human-readable formats controlled by `--json` flag (consistent with `info`/`versions`), always return complete recipe metadata without field filtering, and include both raw platform constraint fields and a computed `supported_platforms` array for convenience.

### Rationale

**Decision 1 (Interface): Option 1B - Tool Name OR --recipe**

This choice satisfies the primary use case from issue #705: golden plan testing needs to query metadata from uncommitted recipe files. It also:
- Maintains symmetry with `eval` command, which already established this pattern
- Supports recipe development workflows (test local changes before committing)
- Follows existing mutual exclusivity validation pattern
- Requires minimal additional complexity (pattern already exists in eval.go)

Rejected 1A because it would force users to commit recipes before they can query metadata, breaking the golden plan testing workflow.

**Decision 2 (Output Format): Option 2B - Dual Format**

This choice prioritizes consistency with existing tsuku commands over implementation simplicity:
- `info`, `versions`, `list`, and other read-only commands all support `--json` flag
- Users expect `--json` as the standard way to get machine-readable output
- Human-readable default makes the command approachable for exploration/debugging
- Scripting workflows can reliably use `--json` flag

Rejected 2A (JSON-only) because it breaks the established tsuku pattern and forces users to pipe through `jq` for casual inspection.

**Decision 3 (Granularity): Option 3A - Full Dump Only**

For the first iteration, always output complete metadata:
- Simplest implementation (no field selector logic)
- Users can filter with `jq` if needed (already standard in CI/scripting)
- Recipe metadata is not large enough to cause performance issues
- Avoids premature design of field selector syntax
- Can add field selection later if real use cases emerge with measurable pain

Rejected 3B (field selection) as premature optimization - no evidence yet that full output is problematic. Rejected strawman 3C (predefined queries) as it has all the complexity of 3B with less flexibility.

**Decision 4 (Platform Representation): Option 4C - Both Raw and Computed**

Include both the raw constraint fields and a computed platform list:
- Raw fields (`supported_os`, `supported_arch`, `unsupported_platforms`) serve users who want to understand the recipe structure
- Computed `supported_platforms` array serves automation (golden plan testing can iterate without reimplementing platform logic)
- Follows "completeness" decision driver - provide all available information
- Minimal overhead (platform computation is trivial)
- Addresses both use cases without requiring users to choose

Rejected 4A because it forces automation scripts to reimplement tsuku's platform matching logic (violating DRY and risking divergence). Rejected 4B because losing the raw constraints makes it harder to understand or debug platform constraints.

### Trade-offs Accepted

**Dual output format maintenance**: We accept the burden of maintaining both JSON and human-readable output formats. This is consistent with existing commands and the code patterns are well-established.

**Redundant platform information**: We accept that the computed `supported_platforms` array duplicates information that could be derived from the constraint fields. This is worthwhile because it eliminates the need for users to understand tsuku's complementary hybrid platform model.

**No field selection**: We accept that users querying a single field (e.g., just platforms) must parse the full JSON output. This is mitigated by `jq` availability in CI environments and the small size of recipe metadata.

**Output schema as stable API**: By exposing computed fields like `supported_platforms`, we commit to maintaining compatibility when recipe schema evolves. This is acceptable because the platform model is already stable and tested.

## Solution Architecture

### Overview

The `tsuku metadata` command is a read-only CLI command that loads recipe files and outputs their static metadata. It follows the established tsuku command pattern: Cobra command definition, recipe loading via the global loader or direct file parsing, and formatted output with JSON flag support.

### Components

```
┌─────────────────────────────────────────────────────┐
│ cmd/tsuku/metadata.go                               │
│                                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │ metadataCmd (cobra.Command)                  │  │
│  │  - Use: "metadata <tool> | --recipe <path>" │  │
│  │  - Flags: --recipe, --json                   │  │
│  │  - Run: metadataRun()                        │  │
│  └────────────┬─────────────────────────────────┘  │
│               │                                     │
│               v                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │ metadataRun(cmd, args)                       │  │
│  │  1. Validate args (tool XOR --recipe)       │  │
│  │  2. Load recipe (loader.Get or ParseFile)   │  │
│  │  3. Extract metadata                         │  │
│  │  4. Compute platforms (GetSupportedPlatforms)│  │
│  │  5. Build output struct                      │  │
│  │  6. Format output (JSON or human-readable)   │  │
│  └────────────┬─────────────────────────────────┘  │
│               │                                     │
└───────────────┼─────────────────────────────────────┘
                │
                v
┌───────────────────────────────────────────────────┐
│ Dependencies:                                     │
│                                                   │
│  - loader (*recipe.Loader) - global loader        │
│  - recipe.ParseFile(path) - direct TOML parsing   │
│  - r.GetSupportedPlatforms() - platform calc      │
│  - printJSON(v) - JSON output helper              │
│  - printInfo() - human-readable output helper     │
└───────────────────────────────────────────────────┘
```

### Key Interfaces

**Input**:
- Command-line arguments: `<tool>` (optional) or `--recipe <path>`
- Flags: `--json` (boolean, default false)

**Output JSON Schema**:
```json
{
  "name": "string",
  "description": "string",
  "homepage": "string (optional)",
  "version_source": {
    "source": "string",
    "github_repo": "string (optional)",
    "tag_prefix": "string (optional)",
    "module": "string (optional)",
    "formula": "string (optional)"
  },
  "version_format": "string (optional)",
  "platform_constraints": {
    "supported_os": ["string"],
    "supported_arch": ["string"],
    "unsupported_platforms": ["string"]
  },
  "supported_platforms": ["os/arch"],
  "tier": "integer",
  "type": "string",
  "verification": {
    "command": "string",
    "pattern": "string (optional)",
    "mode": "string (optional)",
    "version_format": "string (optional)"
  },
  "steps": [
    {
      "action": "string"
    }
  ]
}
```

**Note on dependencies**: Dependency resolution requires traversing the recipe graph via `actions.ResolveDependencies()`, which violates the "static metadata" principle. Dependencies are available via `tsuku info <tool> --json` for installed or registry tools.

**Output Human-Readable Format**:
```
Name: <name>
Description: <description>
Homepage: <homepage> (if present)

Version Source: <source>
  GitHub Repo: <repo> (if applicable)
  Tag Prefix: <prefix> (if applicable)
  Module: <module> (if applicable)
  Formula: <formula> (if applicable)

Supported Platforms:
  - linux/amd64
  - linux/arm64
  - darwin/amd64
  - darwin/arm64

Platform Constraints:
  OS: linux, darwin
  Arch: amd64, arm64
  Unsupported: (none or list)

Tier: <tier>
Type: <type>

Verification:
  Command: <command>
  Pattern: <pattern> (if present)
  Mode: <mode> (if present)

Steps: <count> action(s)
```

### Data Flow

1. **Argument Parsing**: Cobra parses command-line args and flags
2. **Validation**: Check mutual exclusivity (tool name XOR --recipe)
3. **Recipe Loading**:
   - If tool name: `loader.Get(toolName)` → searches local/embedded/registry
   - If --recipe: `recipe.ParseFile(path)` → direct TOML parse
4. **Metadata Extraction**: Access fields from `recipe.Recipe` struct
5. **Platform Computation**: Call `r.GetSupportedPlatforms()` to get platform list
6. **Output Construction**: Build output struct with both raw constraints and computed platforms
7. **Formatting**:
   - If --json: `printJSON(output)` → marshals to stdout
   - Else: format human-readable text → print to stdout

### Error Handling

- **Recipe not found** (registry mode): Exit with `ExitRecipeNotFound`, suggest `tsuku recipes`
- **File not found** (--recipe mode): Exit with `ExitGeneral`, print OS error
- **Invalid TOML**: Exit with `ExitGeneral`, print parse error
- **Mutual exclusivity violated**: Exit with `ExitUsage`, print usage message
- **Platform computation error**: Exit with `ExitGeneral`, print error (should be rare - indicates recipe bug)

## Implementation Approach

### Phase 1: Core Command Structure

Create `cmd/tsuku/metadata.go` with:
- Cobra command definition (`metadataCmd`)
- Flag registration (`--recipe string`, `--json bool`)
- Command registration in `main.go` init()
- Basic argument validation (mutual exclusivity check)
- Recipe loading logic (registry or file mode)
- Minimal output (e.g., just name/description for testing)

**Dependencies**: Existing infrastructure (Cobra, loader, helpers)

### Phase 2: Full Metadata Extraction

Expand output to include all static fields:
- Metadata section (name, description, homepage, tier, type)
- Version section (source, repo, prefix, module, formula)
- Platform constraints (supported_os, supported_arch, unsupported_platforms)
- Verification (command, pattern, mode, version_format)
- Steps (action names only - simplified for metadata overview)

**Dependencies**: Phase 1 complete

### Phase 3: Platform Computation

Add computed `supported_platforms` field:
- Call `r.GetSupportedPlatforms()`
- Handle errors (malformed constraints)
- Add to output struct as string array of "os/arch" tuples

**Dependencies**: Phase 2 complete

### Phase 4: JSON Output

Implement JSON mode:
- Define output struct with JSON tags
- Conditional check for `--json` flag
- Use `printJSON()` helper for output
- Test with various recipes

**Dependencies**: Phase 3 complete

### Phase 5: Human-Readable Output

Implement human-readable format:
- Format similar to `tsuku info` but recipe-focused
- Readable platform list
- Grouped sections (metadata, version, platforms, dependencies, verification, steps)
- Handle optional fields gracefully (e.g., "none" for empty lists)

**Dependencies**: Phase 4 complete

### Phase 6: Testing and Edge Cases

Add test coverage:
- Unit tests for output struct marshaling
- Integration tests with sample recipes
- Error cases (missing file, invalid TOML, malformed platforms)
- Both registry and --recipe modes
- Both JSON and human-readable output

**Dependencies**: Phase 5 complete

## Consequences

### Positive

- **Enables golden plan testing**: Scripts can query supported platforms from uncommitted recipes without network calls or version resolution
- **Consistent command interface**: Follows established patterns (--recipe like eval, --json like info/versions)
- **Complete information**: Full metadata dump serves both human exploration and automation needs
- **Simple implementation**: No complex field selection logic or version resolution
- **Platform computation exposed**: Automation doesn't need to reimplement platform matching logic
- **Fast**: No network calls, no version resolution - just TOML parsing and struct access

### Negative

- **Dual output format maintenance**: Need to keep JSON and human-readable formats in sync as recipe schema evolves
- **Redundant platform data**: Both raw constraints and computed platforms increase output size slightly
- **No field filtering**: Users wanting a single field must parse full JSON (mitigated by jq)
- **Output schema becomes API**: Breaking changes to JSON structure could break automation scripts
- **Limited validation**: Only parser-level validation, no semantic checks on recipe contents

### Mitigations

- **Schema stability commitment**: Document the JSON schema and treat it as a versioned API surface; breaking changes require major version bump or versioning flag
- **Testing coverage**: Comprehensive tests ensure both output formats stay correct across recipe schema changes
- **Documentation**: Provide examples showing how to extract specific fields with `jq` for common use cases
- **Future field selection**: Implementation leaves room to add `--field` flag later if full dump proves problematic in practice

## Security Considerations

### Download Verification

**Not applicable** - The metadata command does not download any external artifacts. It only reads recipe files from:
1. Local filesystem (via `--recipe` flag)
2. Embedded recipes (bundled with tsuku binary)
3. Cached registry (already downloaded by `tsuku update-registry`)

No network requests are made during command execution. No binaries or packages are fetched.

### Execution Isolation

**Low risk** - This is a read-only command with minimal attack surface:

**File system access**:
- Reads recipe TOML files from known locations ($TSUKU_HOME/recipes, embedded, or user-specified path)
- No writes to filesystem
- No execution of external commands

**Network access**:
- None - command operates entirely offline

**Privilege escalation**:
- None - runs with user's normal permissions
- No sudo required
- No setuid binaries

**Risks and Mitigations**:

1. **Path traversal via --recipe**: User could specify `--recipe ../../../../etc/passwd`
   - Mitigated by TOML parser rejecting non-TOML content (no arbitrary file content disclosure)
   - OS-level file permissions prevent reading files the user cannot access
   - Information disclosure limited to file existence (equivalent to `test -r <path>`)

2. **TOML bomb** (resource exhaustion): Maliciously large TOML could consume excessive memory
   - Impact: Process OOM crash (no system-level impact)
   - Mitigation: Parser complexity is linear, no known pathological cases
   - Defense in depth: Could add explicit 10 MB file size limit if needed

3. **Misleading metadata**: Recipe could contain false platform support claims or malicious URLs
   - Command is read-only (doesn't execute steps or download URLs)
   - Automation should validate recipe source trustworthiness
   - This is equivalent to the risk of running `cat recipe.toml | jq`

**Trust model**: The metadata command outputs recipe contents "as-is" without semantic validation. When using `--recipe` with untrusted files, verify the recipe source first. This command is intended for testing local recipes during development.

**Additional mitigations**:
- Uses standard TOML parser (github.com/BurntSushi/toml v1.5.0) with no known vulnerabilities
- Standard library JSON marshaling prevents injection attacks
- No eval/exec of recipe contents - only static parsing
- Empty platform lists are valid output (not treated as errors)

### Supply Chain Risks

**Minimal impact** - This command exposes existing supply chain risks but does not introduce new ones:

**Recipe source trust**:
- Recipes come from the same sources as `tsuku install` (embedded, local, registry)
- If a malicious recipe exists in the registry, `metadata` will output its contents, but NOT execute it
- Unlike `install`, metadata does not download binaries or run installation steps

**Information disclosure risk**:
- A malicious recipe could contain misleading metadata (e.g., claim to support platforms it doesn't)
- Automation relying on metadata output could make incorrect decisions based on false metadata
- However, this is equivalent to the risk of running `cat recipe.toml` - the recipe file itself is the source of truth

**Upstream compromise**:
- If the tsuku recipe registry is compromised, malicious recipes could be distributed
- `metadata` would faithfully output the malicious recipe's metadata
- Actual exploitation requires user to run `tsuku install`, not `metadata`

**Mitigations**:
- This command is read-only - it cannot trigger installation or execution
- Users can inspect `--recipe` file contents before running metadata (standard file review)
- Golden plan testing workflow likely reviews recipes in version control before querying metadata
- Recipe registry trust model is out of scope for this command (inherited from broader tsuku security model)

### User Data Exposure

**Not applicable** - This command does not access or transmit user data:

**Local data accessed**:
- Recipe files only (TOML configuration, not user data)
- No access to installed tools, user files, or system state
- Does not read $TSUKU_HOME/state.json (installation state)

**Data sent externally**:
- None - command is fully offline
- No telemetry, no network requests, no external communication

**Privacy implications**:
- Command output may contain recipe metadata that reveals user's tooling interests if shared
- However, this is intentional (user explicitly queries metadata to inspect it)
- No passive data collection or transmission

### Summary

The metadata command has **minimal security risk** because it is:
1. Read-only (no writes, no execution)
2. Offline (no network access)
3. Limited scope (recipe files only, no user data)
4. Non-privileged (normal user permissions)

The primary risk is **misleading metadata in malicious recipes**, which is mitigated by:
- Command does not execute recipe contents
- Users can review recipe files before querying (especially with `--recipe` for local files)
- Recipe trust model is inherited from broader tsuku security design (registry integrity)

Residual risk: Automation scripts relying on metadata output could be misled by false metadata claims. This is acceptable because the alternative (parsing TOML directly or running `tsuku plan`) has equivalent or greater risk.
