# Design: Info Command Enhancements for Static Recipe Introspection

## Status

Current

## Context and Problem Statement

The `tsuku info` command currently combines recipe metadata with installation state, and always performs dependency resolution. This creates performance and usability issues for scenarios that only need static recipe information:

**Current limitations**:
1. **Always resolves dependencies**: Even when only querying recipe metadata, `info` traverses the dependency graph via `actions.ResolveDependencies()`, which is slow for recipes with deep dependency trees
2. **No `--recipe` flag support**: Unlike `install` and `eval`, `info` only works with registry tools, making it impossible to query metadata from uncommitted local recipe files during development
3. **Cannot skip dependency resolution**: No way to perform fast static queries when full dependency information isn't needed

This creates friction for automation scenarios like:
- Testing frameworks that need to generate platform-specific golden plans
- CI pipelines that need to query platform support before attempting installation
- Documentation tooling that needs to extract recipe information for website generation
- Development workflows that need to validate recipe changes before committing

The `tsuku eval` command can output plan JSON, but requires version resolution and constructs full installation plans. For queries about static recipe properties, this is unnecessarily expensive and requires network access.

### Scope

**In scope:**
- Add `--recipe` flag to `tsuku info` for loading local recipe files
- Add `--metadata-only` flag to skip dependency resolution for fast static queries
- Expand JSON output schema to include static recipe properties
- Platform support detection from recipe constraints

**Out of scope:**
- Version resolution (use `tsuku eval` for that)
- Plan generation (use `tsuku plan` or `tsuku eval` for that)
- Recipe validation beyond what the parser provides
- Platform-specific step filtering (no `--os`/`--arch` flags - use `tsuku eval` for that)

## Decision Drivers

- **Symmetry with existing commands**: Should follow patterns established by `tsuku install` and `tsuku eval` for loading recipes
- **Programmatic automation**: JSON output must be machine-readable for scripting and CI integration
- **No network dependency**: Static metadata queries should work offline by reading recipe files only
- **No execution**: `--metadata-only` mode should not trigger version resolution, plan generation, or installation checks
- **Completeness**: Should expose all static recipe properties, not a curated subset
- **Performance for bulk operations**: CI and testing workflows may query hundreds of recipes; fast path should be available for loops
- **Backward compatibility**: Existing `tsuku info <tool>` behavior must remain unchanged by default
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

### Current info.go Implementation

**info.go** - Current implementation (lines 10-108):
- Loads recipe via `loader.Get(toolName)`
- Outputs JSON or human-readable format based on `--json` flag
- JSON structure includes metadata, platform constraints, dependencies
- Does NOT support `--recipe` flag (only registry lookups)
- ALWAYS resolves dependencies via the dependency graph

### Conventions to Follow

- **Error handling**: Use `printError(err)` + `exitWithCode(code)` from helpers.go
- **Exit codes**: Use constants from exitcodes.go (ExitRecipeNotFound, ExitUsage, etc.)
- **Output suppression**: Respect `--quiet` flag via `printInfo()` helpers
- **Recipe struct**: Use `recipe.Recipe` from `internal/recipe/types.go`
- **Platform methods**: Use `r.GetSupportedPlatforms()`, `r.SupportsPlatform(os, arch)`

### Anti-patterns to Avoid

- **Breaking existing behavior**: Default `tsuku info <tool>` must work exactly as before
- **Network calls in metadata mode**: Don't trigger version resolution when `--metadata-only` is used
- **Incomplete --recipe support**: If adding the flag, ensure it works same as tool name mode

## Considered Options

### Decision 1: Flag Design

How should we add static metadata querying to `tsuku info`?

#### Option 1A: Add --metadata-only Flag Only

Add `--metadata-only` flag to skip dependency resolution, but no `--recipe` flag (still registry-only).

**Pros:**
- Simpler implementation (fewer flag combinations)
- Clear performance benefit for existing use cases
- Minimal behavior change

**Cons:**
- Cannot test local recipe files before committing
- Breaks symmetry with `install` and `eval` which support --recipe
- Doesn't solve golden plan testing use case (needs uncommitted recipes)
- Inconsistent UX (why does eval support --recipe but info doesn't?)

#### Option 1B: Add Both --recipe and --metadata-only Flags

Add `--recipe` for local file loading AND `--metadata-only` to skip dependency resolution.

**Pros:**
- Symmetric with `eval` command (established pattern)
- Supports recipe development workflow (test before commit)
- Enables golden plan testing with local modifications
- Follows existing validation pattern for mutual exclusivity
- Full feature parity: fast queries for both registry and local recipes

**Cons:**
- More complex flag validation (two new flags + mutual exclusivity)
- Need to handle both code paths (registry vs file)
- More surface area for edge cases
- Additional testing matrix

### Decision 2: Output Format

Should we change the existing JSON output format or add new fields conditionally?

#### Option 2A: Always Include Full Schema

Always output all static recipe fields in JSON, regardless of flags.

**Pros:**
- Simple implementation (one schema)
- Complete information for all use cases
- No conditional field logic

**Cons:**
- Larger output even when not needed
- Breaking change to existing JSON output (could break scripts)
- No way to distinguish "requested full metadata" from normal info

#### Option 2B: Dual Format (Expand Conditionally)

Keep existing JSON schema by default, expand with static recipe fields when `--metadata-only` is used.

**Pros:**
- Backward compatible (existing scripts don't break)
- Clear signal that `--metadata-only` provides different data
- Can optimize output for each use case

**Cons:**
- Two output schemas to maintain
- Flag changes output structure (unusual pattern)
- Users must understand mode differences

#### Option 2C: Always Expand Schema (Additive Only)

Always include new static recipe fields in JSON output (additive, not breaking).

**Pros:**
- Backward compatible (existing fields unchanged)
- Consistent output schema regardless of flags
- Users can rely on fields always being present

**Cons:**
- Slightly larger output by default
- Redundant data when dependencies are resolved (some fields overlap)
- More work to extract all static fields even when not needed

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

Support filtering to specific fields (e.g., `tsuku info <tool> --metadata-only --field platforms --field version_source`).

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

**Flag Design × Platform (1 × 4):**
- If supporting `--recipe` flag (1B), malformed platform constraints in external recipes will expose different error behavior:
  - Option 4A: Silent (raw output of invalid constraints)
  - Options 4B/4C: Visible error during platform computation

**Output Format × Granularity (2 × 3):**
- Always expand schema (2C) with full dump (3A) = simplest implementation
- Conditional expansion (2B) with field selection (3B) = most complex
- Backward compatibility concerns differ: 2A breaks scripts, 2B/2C don't

**Granularity × Platform (3 × 4):**
- Full dump only (3A) pairs naturally with both raw and computed (4C) - provide everything
- Field selection (3B) with both raw+computed (4C) doubles cognitive load (which field names?)

**Flag Design × Output Format (1 × 2):**
- Both flags (1B) with conditional expansion (2B) = `--metadata-only` changes output schema
- Both flags (1B) with always expand (2C) = flags control data source, not schema

### Key Assumptions

1. **Recipe schema stability**: The metadata output JSON schema will become a programmatic interface. Breaking changes to recipe fields (`metadata`, `version`, `steps` structure) require corresponding versioning of the metadata output or risk breaking automation scripts.

2. **Platform computation reliability**: Options 4B and 4C depend on `GetSupportedPlatforms()` being correct. The method has test coverage in platform_test.go, but edge cases (invalid constraints, empty arrays) should be validated before exposing as canonical API.

3. **No version-aware metadata**: Metadata is extracted from recipe files as-is. If a recipe uses version-dependent templating (e.g., different binaries for different versions), this command cannot expose that without version resolution (which is explicitly out of scope for `--metadata-only` mode).

4. **Parser validation sufficiency**: Relying on `recipe.ParseFile()` validation means invalid TOML files will error cleanly, but semantic issues (e.g., nonsensical platform constraints like `supported_os=["windows"]` in a Linux-only tool) won't be caught.

## Decision Outcome

**Chosen: 1B (Both Flags) + 2C (Always Expand Schema - Additive) + 3A (Full Dump) + 4C (Both Raw and Computed)**

### Summary

The `tsuku info` command will be enhanced with two new flags: `--recipe <path>` for loading local recipe files (mirroring `eval` and `install`), and `--metadata-only` to skip dependency resolution for fast static queries. The JSON output schema will be expanded to always include static recipe properties (additive change - backward compatible), with both raw platform constraint fields and a computed `supported_platforms` array for convenience.

### Rationale

**Decision 1 (Flag Design): Option 1B - Both Flags**

This choice satisfies the primary use case from issue #705: golden plan testing needs to query metadata from uncommitted recipe files. It also:
- Maintains symmetry with `eval` and `install` commands, which already established the `--recipe` pattern
- Supports recipe development workflows (test local changes before committing)
- Provides performance optimization via `--metadata-only` for both registry and local recipes
- Follows existing mutual exclusivity validation pattern
- Requires minimal additional complexity (pattern already exists in eval.go and install.go)

Rejected 1A because it would force users to commit recipes before they can query metadata, breaking the golden plan testing workflow.

**Decision 2 (Output Format): Option 2C - Always Expand Schema (Additive)**

This choice prioritizes backward compatibility and consistency:
- Existing scripts using `tsuku info <tool> --json` continue to work (additive schema changes don't break JSON parsing)
- Users get consistent output schema regardless of flags (easier to understand and document)
- New static fields are always available (no need to remember when to use `--metadata-only`)
- Simpler implementation (no conditional field logic based on flags)

Rejected 2A (always include, potentially breaking) because it would break existing scripts. Rejected 2B (conditional expansion) because having flags change output schema is confusing and makes the command harder to use.

**Decision 3 (Granularity): Option 3A - Full Dump Only**

For the first iteration, always output complete metadata:
- Simplest implementation (no field selector logic)
- Users can filter with `jq` if needed (already standard in CI/scripting)
- Recipe metadata is not large enough to cause performance issues
- Avoids premature design of field selector syntax
- Can add field selection later if real use cases emerge with measurable pain

Rejected 3B (field selection) as premature optimization - no evidence yet that full output is problematic.

**Decision 4 (Platform Representation): Option 4C - Both Raw and Computed**

Include both the raw constraint fields and a computed platform list:
- Raw fields (`supported_os`, `supported_arch`, `unsupported_platforms`) serve users who want to understand the recipe structure
- Computed `supported_platforms` array serves automation (golden plan testing can iterate without reimplementing platform logic)
- Follows "completeness" decision driver - provide all available information
- Minimal overhead (platform computation is trivial)
- Addresses both use cases without requiring users to choose

Rejected 4A because it forces automation scripts to reimplement tsuku's platform matching logic (violating DRY and risking divergence). Rejected 4B because losing the raw constraints makes it harder to understand or debug platform constraints.

### Trade-offs Accepted

**Backward compatibility constraint**: We commit to maintaining the existing `tsuku info` behavior by default. Existing scripts must continue working without modification.

**Additive schema expansion**: By always including new static fields, we accept slightly larger JSON output even when users only need installation state. This is acceptable because recipe metadata is small and the consistency benefit outweighs the size cost.

**Redundant platform information**: We accept that the computed `supported_platforms` array duplicates information that could be derived from the constraint fields. This is worthwhile because it eliminates the need for users to understand tsuku's complementary hybrid platform model.

**No field selection**: We accept that users querying a single field (e.g., just platforms) must parse the full JSON output. This is mitigated by `jq` availability in CI environments and the small size of recipe metadata.

**Output schema as stable API**: By exposing computed fields like `supported_platforms`, we commit to maintaining compatibility when recipe schema evolves. This is acceptable because the platform model is already stable and tested.

## Solution Architecture

### Overview

The `tsuku info` command will be enhanced to support two new flags: `--recipe` for loading local recipe files and `--metadata-only` for skipping dependency resolution. The existing JSON output schema will be expanded with static recipe fields, and the command logic will be modified to conditionally skip dependency resolution based on the `--metadata-only` flag.

### Components

```
┌─────────────────────────────────────────────────────┐
│ cmd/tsuku/info.go (MODIFIED)                        │
│                                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │ infoCmd (cobra.Command)                      │  │
│  │  - Use: "info <tool> | --recipe <path>"     │  │
│  │  - Flags: --recipe, --metadata-only, --json │  │
│  │  - Run: infoRun() [MODIFIED]                │  │
│  └────────────┬─────────────────────────────────┘  │
│               │                                     │
│               v                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │ infoRun(cmd, args) [MODIFIED]                │  │
│  │  1. Validate args (tool XOR --recipe)       │  │
│  │  2. Load recipe (loader.Get or ParseFile)   │  │
│  │  3. Check installation state (if not        │  │
│  │     --metadata-only)                         │  │
│  │  4. Resolve dependencies (if not            │  │
│  │     --metadata-only)                         │  │
│  │  5. Compute platforms (GetSupportedPlatforms)│  │
│  │  6. Build output struct (expanded schema)   │  │
│  │  7. Format output (JSON or human-readable)   │  │
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
- Flags:
  - `--json` (boolean, default false) - existing
  - `--recipe <path>` (string) - new
  - `--metadata-only` (boolean, default false) - new

**Expanded JSON Output Schema** (additive changes marked with NEW):
```json
{
  "name": "string",
  "installed": "boolean",
  "version": "string (if installed)",
  "location": "string (if installed)",
  "description": "string",
  "homepage": "string (optional)",
  "version_source": {                    // NEW
    "source": "string",
    "github_repo": "string (optional)",
    "tag_prefix": "string (optional)",
    "module": "string (optional)",
    "formula": "string (optional)"
  },
  "version_format": "string (optional)",  // NEW
  "platform_constraints": {               // NEW
    "supported_os": ["string"],
    "supported_arch": ["string"],
    "unsupported_platforms": ["string"]
  },
  "supported_platforms": ["os/arch"],     // NEW (computed)
  "tier": "integer",                      // NEW
  "type": "string",                       // NEW
  "verification": {                       // NEW
    "command": "string",
    "pattern": "string (optional)",
    "mode": "string (optional)",
    "version_format": "string (optional)"
  },
  "dependencies": ["string"],
  "steps": [                              // NEW
    {
      "action": "string"
    }
  ]
}
```

**Note on `--metadata-only` behavior**:
- When `--metadata-only` is set:
  - `installed` field is omitted
  - `version` and `location` fields are omitted (no installation state check)
  - `dependencies` field may be omitted or simplified (no graph traversal)
- When NOT set (default behavior):
  - All fields are included
  - Dependency resolution happens as before
  - Installation state is checked

**Human-Readable Format** (enhanced with static metadata when available):
```
Name: <name>
Installed: <yes/no> (omitted if --metadata-only)
Version: <version> (if installed, omitted if --metadata-only)
Location: <path> (if installed, omitted if --metadata-only)
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

Dependencies: <list> (omitted if --metadata-only)

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
4. **Conditional State Check** (skip if `--metadata-only`):
   - Check if tool is installed
   - Get installation version and location
5. **Conditional Dependency Resolution** (skip if `--metadata-only`):
   - Traverse dependency graph
   - Build dependency list
6. **Metadata Extraction**: Access fields from `recipe.Recipe` struct
7. **Platform Computation**: Call `r.GetSupportedPlatforms()` to get platform list
8. **Output Construction**: Build output struct with all available fields (installation state + static metadata)
9. **Formatting**:
   - If --json: `printJSON(output)` → marshals to stdout
   - Else: format human-readable text → print to stdout

### Error Handling

- **Recipe not found** (registry mode): Exit with `ExitRecipeNotFound`, suggest `tsuku recipes`
- **File not found** (--recipe mode): Exit with `ExitGeneral`, print OS error
- **Invalid TOML**: Exit with `ExitGeneral`, print parse error
- **Mutual exclusivity violated**: Exit with `ExitUsage`, print usage message
- **Platform computation error**: Exit with `ExitGeneral`, print error (should be rare - indicates recipe bug)

## Implementation Approach

### Phase 1: Add Flag Definitions

Modify `cmd/tsuku/info.go`:
- Add `--recipe string` flag definition
- Add `--metadata-only bool` flag definition
- Add flag registration in `init()` function

**Dependencies**: None (existing infrastructure)

### Phase 2: Add Recipe Loading Logic

Modify `infoRun()` function:
- Add mutual exclusivity validation (tool name XOR --recipe)
- Add conditional recipe loading (registry mode vs file mode)
- Preserve existing loader.Get() path for backward compatibility
- Add loadLocalRecipe() call for --recipe mode

**Dependencies**: Phase 1 complete

### Phase 3: Conditional Dependency Resolution

Modify `infoRun()` function:
- Wrap existing dependency resolution in `if !metadataOnly` check
- Ensure installation state check is also skipped when `--metadata-only`
- Preserve default behavior (full resolution) when flag not set

**Dependencies**: Phase 2 complete

### Phase 4: Expand JSON Output Schema

Modify JSON output struct definition:
- Add version_source fields
- Add platform_constraints fields
- Add supported_platforms (computed)
- Add tier, type fields
- Add verification fields
- Add steps array (simplified)
- Use json omitempty tags for conditional fields

**Dependencies**: Phase 3 complete

### Phase 5: Platform Computation

Add platform computation logic:
- Call `r.GetSupportedPlatforms()`
- Handle errors (malformed constraints)
- Add to output struct as string array of "os/arch" tuples
- Include both raw constraints and computed platforms

**Dependencies**: Phase 4 complete

### Phase 6: Update Human-Readable Output

Modify human-readable formatting:
- Add static metadata sections
- Handle optional fields gracefully
- Maintain existing format for installation state
- Add conditional sections based on `--metadata-only`

**Dependencies**: Phase 5 complete

### Phase 7: Testing and Edge Cases

Add test coverage:
- Unit tests for expanded output struct marshaling
- Integration tests with sample recipes
- Error cases (missing file, invalid TOML, malformed platforms)
- Both registry and --recipe modes
- Both with and without --metadata-only flag
- Both JSON and human-readable output
- Backward compatibility tests (existing behavior unchanged)

**Dependencies**: Phase 6 complete

## Consequences

### Positive

- **Enables golden plan testing**: Scripts can query supported platforms from uncommitted recipes without network calls or version resolution
- **Consistent command interface**: Follows established patterns (--recipe like eval/install, --json like existing commands)
- **Backward compatible**: Existing `tsuku info <tool>` usage works exactly as before
- **Performance optimization**: `--metadata-only` provides fast path for static queries
- **Complete information**: Expanded schema serves both human exploration and automation needs
- **Simple implementation**: Builds on existing code patterns and infrastructure
- **Platform computation exposed**: Automation doesn't need to reimplement platform matching logic
- **Fast static queries**: No network calls, no version resolution when using `--metadata-only`

### Negative

- **Flag interaction complexity**: Three flags (--recipe, --metadata-only, --json) create multiple modes to document and test
- **Expanded output schema**: More fields increase maintenance burden as recipe schema evolves
- **Redundant platform data**: Both raw constraints and computed platforms increase output size slightly
- **No field filtering**: Users wanting a single field must parse full JSON (mitigated by jq)
- **Output schema becomes API**: Breaking changes to JSON structure could break automation scripts
- **Mode confusion**: Users must understand when to use --metadata-only vs default behavior

### Mitigations

- **Schema stability commitment**: Document the JSON schema and treat it as a versioned API surface; breaking changes require major version bump or versioning flag
- **Testing coverage**: Comprehensive tests ensure expanded schema and flag combinations work correctly
- **Documentation**: Provide clear examples showing use cases for each flag combination
- **Backward compatibility tests**: Automated tests ensure existing `tsuku info` usage continues working
- **Clear flag descriptions**: Help text explains when to use --metadata-only and --recipe

## Security Considerations

### Download Verification

**Not applicable** - The enhanced info command does not download any external artifacts. It only reads recipe files from:
1. Local filesystem (via `--recipe` flag)
2. Embedded recipes (bundled with tsuku binary)
3. Cached registry (already downloaded by `tsuku update-registry`)

No network requests are made during command execution. No binaries or packages are fetched.

### Execution Isolation

**Low risk** - This remains a read-only command with minimal attack surface:

**File system access**:
- Reads recipe TOML files from known locations ($TSUKU_HOME/recipes, embedded, or user-specified path)
- Reads installation state from $TSUKU_HOME/state.json (when not using --metadata-only)
- No writes to filesystem
- No execution of external commands

**Network access**:
- None when using --metadata-only (command operates entirely offline)
- Existing behavior (dependency resolution) may trigger network calls in default mode

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

**Trust model**: The info command outputs recipe contents "as-is" without semantic validation. When using `--recipe` with untrusted files, verify the recipe source first. This command is intended for testing local recipes during development.

**Additional mitigations**:
- Uses standard TOML parser (github.com/BurntSushi/toml v1.5.0) with no known vulnerabilities
- Standard library JSON marshaling prevents injection attacks
- No eval/exec of recipe contents - only static parsing
- Empty platform lists are valid output (not treated as errors)

### Supply Chain Risks

**Minimal impact** - This command exposes existing supply chain risks but does not introduce new ones:

**Recipe source trust**:
- Recipes come from the same sources as `tsuku install` (embedded, local, registry)
- If a malicious recipe exists in the registry, `info` will output its contents, but NOT execute it
- Unlike `install`, info does not download binaries or run installation steps

**Information disclosure risk**:
- A malicious recipe could contain misleading metadata (e.g., claim to support platforms it doesn't)
- Automation relying on metadata output could make incorrect decisions based on false metadata
- However, this is equivalent to the risk of running `cat recipe.toml` - the recipe file itself is the source of truth

**Upstream compromise**:
- If the tsuku recipe registry is compromised, malicious recipes could be distributed
- `info` would faithfully output the malicious recipe's metadata
- Actual exploitation requires user to run `tsuku install`, not `info`

**Mitigations**:
- This command is read-only - it cannot trigger installation or execution (even without --metadata-only)
- Users can inspect `--recipe` file contents before running info (standard file review)
- Golden plan testing workflow likely reviews recipes in version control before querying metadata
- Recipe registry trust model is out of scope for this command (inherited from broader tsuku security model)

### User Data Exposure

**Not applicable** - This command does not access or transmit user data:

**Local data accessed**:
- Recipe files only (TOML configuration, not user data)
- Installation state ($TSUKU_HOME/state.json) when not using --metadata-only
- No access to user files or system state beyond tsuku installation directory

**Data sent externally**:
- None - command is fully offline
- No telemetry, no network requests, no external communication

**Privacy implications**:
- Command output may contain recipe metadata that reveals user's tooling interests if shared
- However, this is intentional (user explicitly queries metadata to inspect it)
- No passive data collection or transmission

### Summary

The enhanced info command has **minimal security risk** because it is:
1. Read-only (no writes, no execution)
2. Offline (no network access when using --metadata-only)
3. Limited scope (recipe files and installation state only, no user data)
4. Non-privileged (normal user permissions)

The primary risk is **misleading metadata in malicious recipes**, which is mitigated by:
- Command does not execute recipe contents
- Users can review recipe files before querying (especially with `--recipe` for local files)
- Recipe trust model is inherited from broader tsuku security design (registry integrity)

Residual risk: Automation scripts relying on metadata output could be misled by false metadata claims. This is acceptable because the alternative (parsing TOML directly or running `tsuku plan`) has equivalent or greater risk.
