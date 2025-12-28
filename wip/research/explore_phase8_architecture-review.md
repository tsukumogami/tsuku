# Phase 8: Architecture Review

## Architecture Clarity Assessment

**Overall: YES - The architecture is clear enough to implement**

The design document provides sufficient detail for implementation:

1. **Command structure clearly defined**: Follows established Cobra pattern with concrete examples from existing commands
2. **Recipe loading logic specified**: Dual-mode loading (registry vs file) mirrors `eval.go` pattern with explicit validation logic
3. **Output schema documented**: Complete JSON schema provided with field types and structure
4. **Data flow explicit**: 7-step linear flow from argument parsing to formatted output
5. **Implementation phases well-structured**: 6 phases with clear dependencies and scope

The design successfully references existing code patterns (`info.go`, `eval.go`, `helpers.go`) that provide working examples for each component. An experienced Go developer familiar with the tsuku codebase could implement this from the design without significant ambiguity.

**Minor clarity gaps**:
- Human-readable format specification is high-level (shows structure but not exact formatting code)
- Dependency computation approach not specified (but design correctly identifies this is out of scope by referencing existing `ResolveDependencies` pattern)

These gaps are acceptable because:
1. Human-readable format can reference `info.go` implementation style
2. Metadata extraction is straightforward struct field access

## Component Analysis

**All necessary components are identified. No critical gaps.**

The design correctly identifies:

### Core Components (Present)
1. **metadataCmd** (Cobra command definition) - Standard pattern
2. **metadataRun()** function - Main execution logic
3. **Recipe loading** - Via global `loader` and `loadLocalRecipe()` helper
4. **Output struct** - Defined with JSON tags
5. **Platform computation** - Via existing `GetSupportedPlatforms()` method
6. **Error handling** - Exit codes and error messages specified

### External Dependencies (Correctly Referenced)
1. **Global loader** (`*recipe.Loader`) - Initialized in main.go
2. **recipe.ParseFile()** - For --recipe file mode
3. **recipe.Recipe struct** - Source of all metadata
4. **printJSON()** helper - JSON output formatting
5. **printInfo()** helpers - Human-readable output
6. **Exit code constants** - From exitcodes.go

### Missing Components (Deliberate Omissions - GOOD)
The design correctly excludes:
- Version resolution (delegated to `eval` command)
- Installation state checking (delegated to `info` command)
- Dependency resolution logic (uses existing actions package if needed)

**One architectural observation**: The design includes dependency information in the output schema (lines 427-430) but doesn't specify how to populate this. Looking at `info.go`, dependencies are computed via `actions.ResolveDependencies(r)`. This should be clarified:

**Recommendation**: Explicitly state in Phase 2 that dependency extraction uses:
```go
directDeps := actions.ResolveDependencies(r)
installDeps := sortedKeys(directDeps.InstallTime)
runtimeDeps := sortedKeys(directDeps.Runtime)
```

This mirrors the approach in `info.go` lines 61-72 (for uninstalled tools).

## Interface Design Review

### CLI Interface (GOOD)

**Strengths**:
- Follows established patterns (`--recipe` from eval, `--json` from info/versions)
- Mutual exclusivity validation explicit (lines 490-491)
- Clear error messages for edge cases
- Platform flag pattern reusable from `eval.go`

**Potential issue**: The design doesn't specify whether `--os` and `--arch` flags should be supported. Looking at the scope (line 33: "Support querying specific fields or full recipe dump"), it seems this is intentional (platform queries are for recipe properties, not dynamic platform-specific resolution).

**Recommendation**: Add explicit statement in "Out of scope" that platform-specific URL resolution or step filtering is not part of metadata command (that's `eval`'s job with --os/--arch flags).

### JSON Schema (EXCELLENT)

The JSON schema (lines 409-447) is well-designed:

**Strengths**:
1. **Complete coverage**: All Recipe struct fields mapped
2. **Structured nesting**: Logical grouping (version_source, platform_constraints, dependencies, verification)
3. **Proper types**: Arrays, objects, optional fields clearly marked
4. **Consistent naming**: snake_case for JSON keys (matches TOML convention)
5. **Computed field included**: `supported_platforms` array alongside raw constraints

**Validation against recipe.Recipe struct**:

Checking types.go (lines 12-19, 145-174):
- ✅ Metadata fields: name, description, homepage, tier, type - COVERED
- ✅ Version fields: source, github_repo, tag_prefix, module, formula - COVERED
- ✅ Platform constraints: supported_os, supported_arch, unsupported_platforms - COVERED
- ✅ Dependencies: Merged from metadata.Dependencies fields - NEEDS CLARIFICATION
- ✅ Verification: command, pattern, mode - COVERED (mode missing from schema)
- ✅ Steps: action, note, when - COVERED

**Issues found**:

1. **Missing field: `version_format`** (line 64 in types.go: `VersionFormat string`)
   - Schema line 412 shows generic "version_format": "string" but it's at top level, not nested
   - Should be in metadata section OR at top level (check what info.go does)
   - Looking at info.go line 95: `VersionFormat: r.Metadata.VersionFormat` - it's in metadata
   - ✅ Actually present in schema line 421 - false alarm

2. **Missing field: `verification.mode`** (line 286 in types.go: `Mode string`)
   - Schema lines 434-437 show verification but no mode field
   - Should add: `"mode": "string (optional)"`

3. **Missing field: `verification.version_format`** (line 287 in types.go: `VersionFormat string`)
   - Verification can override the metadata-level version_format
   - Should add to verification object

4. **Missing field: `tier`** (line 156 in types.go: `Tier int`)
   - Schema doesn't include tier field (line 432 in design shows it but wrong location)
   - Should be in metadata or top-level (check info.go)
   - Looking at info.go: tier is NOT in JSON output - intentional omission?
   - Design line 432 includes it, so this should be clarified

5. **Missing field: `type`** (line 157 in types.go: `Type string`)
   - Schema doesn't include type field but line 433 shows it
   - ✅ Present in schema

6. **Steps structure simplified**: Schema lines 439-445 shows minimal step representation
   - Only action, note, when - missing Description and Params
   - Is this intentional simplification?
   - For metadata introspection, showing all params might be valuable
   - **Recommendation**: Include full step representation or document why params are excluded

7. **Dependencies merging logic unclear**:
   - types.go lines 152-155 show 4 dependency fields (dependencies, runtime_dependencies, extra_dependencies, extra_runtime_dependencies)
   - Schema lines 427-430 shows simplified install/runtime split
   - Merging logic should be specified (likely same as actions.ResolveDependencies)

### Error Handling (GOOD)

Error cases (lines 502-508) cover the main scenarios:

**Covered**:
- ✅ Recipe not found (registry mode) → ExitRecipeNotFound
- ✅ File not found (--recipe mode) → ExitGeneral
- ✅ Invalid TOML → ExitGeneral
- ✅ Mutual exclusivity violated → ExitUsage
- ✅ Platform computation error → ExitGeneral

**Additional cases to consider**:

1. **Empty/nil recipe result**: What if `loader.Get()` returns nil without error?
   - Unlikely in current implementation, but defensive check recommended

2. **GetSupportedPlatforms() edge case**: What if result is empty array?
   - This is valid (recipe supports no platforms) - should output empty array, not error
   - Design says "should be rare - indicates recipe bug" but this is actually valid
   - **Recommendation**: Don't treat empty platforms as error; output empty array

3. **File permission denied**: --recipe with unreadable file
   - Covered by "File not found" (OS error), but message should be clear

4. **Invalid platform constraints**: What if ValidatePlatformConstraints() fails?
   - This is out of scope (parser validation only), but worth noting
   - **Recommendation**: Add note that malformed constraints surface as computation errors

## Implementation Sequencing

**Phases are correctly ordered with proper dependencies**

### Phase 1: Core Command Structure ✅
- **Dependencies**: None (uses existing infrastructure)
- **Deliverable**: Skeleton command with basic recipe loading
- **Validation**: Can run `tsuku metadata <tool>` and get minimal output
- **Correct scope**: Focuses on wiring, not business logic

### Phase 2: Full Metadata Extraction ✅
- **Dependencies**: Phase 1 complete (command structure exists)
- **Deliverable**: Complete struct field mapping
- **Validation**: All recipe fields accessible
- **Correct scope**: Pure data extraction, no formatting yet

### Phase 3: Platform Computation ✅
- **Dependencies**: Phase 2 complete (struct has platform constraints)
- **Deliverable**: Computed supported_platforms array
- **Validation**: Array matches GetSupportedPlatforms() output
- **Correct scope**: Single focused feature

### Phase 4: JSON Output ✅
- **Dependencies**: Phase 3 complete (all data available)
- **Deliverable**: Working --json flag
- **Validation**: Valid JSON with complete schema
- **Correct scope**: JSON serialization only

### Phase 5: Human-Readable Output ✅
- **Dependencies**: Phase 4 complete (data model finalized)
- **Deliverable**: Default human-readable format
- **Validation**: Readable output without --json
- **Correct scope**: Formatting logic separate from data extraction

### Phase 6: Testing and Edge Cases ✅
- **Dependencies**: Phase 5 complete (full implementation done)
- **Deliverable**: Test coverage for all modes
- **Validation**: Tests pass for error cases
- **Correct scope**: Quality assurance, not feature development

**Sequencing is optimal**: Each phase builds on previous, no circular dependencies, clear incremental value.

**Alternative sequencing considered**: Could combine Phases 2+3 (metadata extraction + platform computation), but current split allows validating basic extraction before adding computed fields. Current approach is better for debugging.

## Simplification Opportunities

**Could this be simpler? Analysis of potential simplifications:**

### 1. Eliminate Human-Readable Output (REJECTED)

**Proposal**: Make it JSON-only (like kubectl get with -o json as default)

**Rationale for keeping dual format**:
- Consistency with info/versions commands (established pattern)
- Exploratory usage (developers inspecting recipes)
- Debugging (quickly seeing what's in a recipe without jq)

**Decision**: Keep dual format. Consistency outweighs implementation simplicity.

### 2. Remove Computed Platform List (REJECTED)

**Proposal**: Only output raw constraints (supported_os, supported_arch, unsupported_platforms)

**Rationale for keeping computed list**:
- Golden plan testing (motivating use case) needs to iterate over platforms
- Automation scripts shouldn't reimplement platform logic
- GetSupportedPlatforms() already exists and is tested

**Decision**: Keep computed list. This is core value proposition for automation.

### 3. Defer Dependencies Field (CONSIDERED)

**Proposal**: Remove dependencies from initial implementation (add later if needed)

**Analysis**:
- Dependencies require resolving with actions.ResolveDependencies()
- This is NOT static metadata (requires traversing recipe graph)
- Inconsistent with "static introspection" scope
- `info` command already provides this for registry tools

**Recommendation**: **REMOVE dependencies from metadata output**
- Violates "static metadata" principle (requires graph traversal)
- Already available via `tsuku info <tool> --json`
- Simplifies implementation (removes actions package dependency)
- If needed later, can add flag: `--include-deps` (opt-in)

**Impact**: Simplifies Phase 2, removes dependency on actions package, makes output truly static

### 4. Remove --recipe Flag (REJECTED)

**Proposal**: Only support registry lookups (like `info` command)

**Rationale for keeping --recipe**:
- Motivating use case (golden plan testing) requires testing uncommitted recipes
- Consistency with eval command
- Recipe development workflow needs this

**Decision**: Keep --recipe. Core requirement from issue #705.

### 5. Simplify Steps Output (RECOMMENDED)

**Proposal**: Only output step count or action names, not full step details

**Analysis**:
- Full step representation (action, note, when, params) is verbose
- Params include URLs, checksums, complex nested structures
- Most automation doesn't need this (platforms, version source, dependencies are key)
- Can always parse TOML directly for step details

**Recommendation**: **Simplify steps to action names only**
```json
"steps": ["download_archive", "install_binaries", "create_wrapper"]
```

Or minimal representation:
```json
"steps": [
  {"action": "download_archive", "when": {"os": "linux"}},
  {"action": "github_file"}
]
```

**Impact**: Reduces output size, simplifies parsing, focuses on metadata overview

### Summary of Simplification Recommendations

| Component | Keep/Remove | Rationale |
|-----------|-------------|-----------|
| Dual format | **Keep** | Consistency, exploratory use |
| Computed platforms | **Keep** | Core automation value |
| Dependencies | **Remove** | Not static metadata, available via `info` |
| --recipe flag | **Keep** | Core requirement |
| Full step details | **Simplify** | Too verbose, not core metadata |

**Overall complexity assessment**: Implementation is appropriately scoped. Recommended simplifications reduce scope without losing value.

## Data Flow Validation

**Does the flow make sense? Analysis of 7-step flow (lines 489-500):**

### Step 1-2: Argument Parsing & Validation ✅
```
Cobra → Validation (tool XOR --recipe)
```
- **Correct**: Mutual exclusivity enforced early (fail-fast)
- **Complete**: Covers all invalid input combinations
- **Missing**: No validation of recipe file path format (e.g., must end in .toml)
  - Recommendation: Add extension validation or document that any path is accepted

### Step 3: Recipe Loading ✅
```
If tool name → loader.Get(toolName)
If --recipe → recipe.ParseFile(path)
```
- **Correct**: Dual path matches eval.go pattern
- **Complete**: Both modes covered
- **Error handling**: File not found, invalid TOML handled
- **Missing**: No caching consideration (not needed - metadata is fast)

### Step 4: Metadata Extraction ✅
```
Access fields from recipe.Recipe struct
```
- **Correct**: Direct struct access (no computation)
- **Simple**: No transformation logic needed
- **Issue**: Dependencies require actions.ResolveDependencies() call (not just struct access)
  - If keeping dependencies, add explicit step: "Resolve dependencies via actions package"
  - If removing (per simplification recommendation), this step is truly simple

### Step 5: Platform Computation ✅
```
Call r.GetSupportedPlatforms()
```
- **Correct**: Uses existing tested method
- **Error handling**: Design mentions "should be rare" but this is valid operation
  - Recommendation: Don't treat as error; empty array is valid output
- **Performance**: O(|OS| × |Arch|) - trivial for tsuku's 2×2 matrix

### Step 6: Output Construction ✅
```
Build output struct with raw constraints + computed platforms
```
- **Correct**: Single struct creation
- **Complete**: All fields mapped
- **Issue**: Struct definition not shown in code (only JSON schema)
  - Recommendation: Add Go struct definition to design for clarity

### Step 7: Formatting ✅
```
If --json → printJSON(output)
Else → format human-readable → print
```
- **Correct**: Standard pattern from info.go
- **Complete**: Both modes covered
- **Error handling**: printJSON handles marshaling errors

### Data Flow Gaps Identified

1. **Dependency resolution** (if keeping dependencies field):
   ```
   Step 4.5: Resolve dependencies
     directDeps := actions.ResolveDependencies(r)
     installDeps := sortedKeys(directDeps.InstallTime)
     runtimeDeps := sortedKeys(directDeps.Runtime)
   ```

2. **Output struct definition missing**:
   - Design shows JSON schema but not Go struct
   - Should add:
   ```go
   type metadataOutput struct {
       Name                string                 `json:"name"`
       Description         string                 `json:"description"`
       Homepage            string                 `json:"homepage,omitempty"`
       VersionSource       versionSourceOutput    `json:"version_source"`
       VersionFormat       string                 `json:"version_format"`
       PlatformConstraints platformConstraintsOutput `json:"platform_constraints"`
       SupportedPlatforms  []string              `json:"supported_platforms"`
       // ... etc
   }
   ```

3. **Human-readable format implementation**:
   - Design shows structure but not formatting code
   - Should reference info.go pattern: `fmt.Printf("Name: %s\n", r.Metadata.Name)`

### Data Flow Validation: PASS with Minor Gaps

The flow is logical and complete. Gaps are in implementation details (struct definitions, formatting code), not in conceptual flow.

## Recommendations

### Critical (Must Address Before Implementation)

1. **Remove or Clarify Dependencies Field**
   - **Issue**: Dependencies require graph traversal (not static metadata)
   - **Options**:
     - A) Remove dependencies from output (recommended - violates static metadata principle)
     - B) Keep dependencies but document that this uses actions.ResolveDependencies()
     - C) Add --include-deps flag (opt-in)
   - **Impact**: Simplifies implementation, clarifies scope
   - **Decision needed**: Does "static metadata" include dependency graph traversal?

2. **Add Missing JSON Schema Fields**
   - **Issue**: verification.mode and verification.version_format missing
   - **Fix**: Add to schema:
     ```json
     "verification": {
       "command": "string",
       "pattern": "string (optional)",
       "mode": "string (optional)",
       "version_format": "string (optional)"
     }
     ```

3. **Add Go Struct Definition**
   - **Issue**: Only JSON schema shown, not Go struct
   - **Fix**: Add struct definition to Implementation Approach section
   - **Benefit**: Eliminates ambiguity about field types and tags

4. **Clarify Platform Computation Error Handling**
   - **Issue**: Design treats empty platform list as error
   - **Fix**: Document that empty array is valid output (recipe supports no platforms)
   - **Code**: Don't add error check in step 5; just return empty array

### High Priority (Should Address)

5. **Simplify Steps Output**
   - **Issue**: Full step details are verbose and not core metadata
   - **Fix**: Output action names only or minimal representation
   - **Benefit**: Reduces output size, focuses on metadata overview

6. **Document Dependency Merging Logic**
   - **Issue**: Recipe has 4 dependency fields, schema shows 2
   - **Fix**: If keeping dependencies, document how metadata.Dependencies and metadata.RuntimeDependencies are merged with Extra* fields
   - **Reference**: actions.ResolveDependencies() in internal/actions package

7. **Add Scope Clarification**
   - **Issue**: Whether --os/--arch flags are supported is unclear
   - **Fix**: Explicitly state in "Out of scope" that platform-specific resolution is not supported
   - **Rationale**: Metadata is recipe-level, not platform-specific (that's eval's job)

### Medium Priority (Nice to Have)

8. **Add File Extension Validation**
   - **Issue**: --recipe accepts any path (e.g., --recipe /etc/passwd)
   - **Fix**: Validate .toml extension or document that any path is accepted
   - **Error**: Graceful failure (TOML parser will reject invalid files)

9. **Add Example Output**
   - **Issue**: Design shows schema but no concrete example
   - **Fix**: Add example JSON output for a real recipe (e.g., ripgrep)
   - **Benefit**: Helps reviewers visualize output format

10. **Consider Output Versioning**
    - **Issue**: JSON schema becomes API; breaking changes affect automation
    - **Fix**: Add note about output versioning strategy
    - **Options**:
      - Semantic versioning of tsuku itself
      - Schema version field in output
      - Stability guarantee (no breaking changes without major version)

### Low Priority (Future Consideration)

11. **Field Selection Flag**
    - **Note**: Design explicitly defers this (Decision 3A)
    - **Revisit if**: Users complain about output size or parsing overhead
    - **Implementation**: --field flag with jq-like syntax

12. **Strict Validation Mode**
    - **Note**: Design mentions parser validation only
    - **Revisit if**: Need to catch semantic errors (e.g., nonsensical constraints)
    - **Implementation**: --strict flag that calls ValidatePlatformConstraints()

### Implementation Order

Based on criticality:

1. **Phase 0 (Pre-implementation)**:
   - Decide on dependencies field (keep/remove/flag)
   - Add Go struct definitions to design
   - Update JSON schema (add missing fields)
   - Clarify scope (--os/--arch not supported)

2. **Phase 1-6** (As specified in design):
   - Follow existing phases
   - Apply simplifications (remove dependencies, simplify steps)
   - Add validation for edge cases

3. **Phase 7 (Documentation)**:
   - Add example output to design doc
   - Document output schema stability guarantee
   - Update CLAUDE.md if needed

### Architecture Decision Validation

The design makes sound architectural choices:

✅ **Good Decisions**:
- Tool name OR --recipe (enables golden plan testing)
- Dual format (consistency with existing commands)
- Full dump (avoids premature optimization)
- Both raw and computed platforms (serves both use cases)

⚠️ **Questionable Decisions**:
- Including dependencies (violates "static metadata" scope)
- Full step details (verbose, low value)

❌ **Missing Decisions**:
- Whether to support --os/--arch flags
- How to version output schema
- Whether to validate platform constraints

### Final Verdict

**Architecture is sound and implementable with minor clarifications needed.**

The design successfully balances:
- Simplicity (no version resolution, no installation state)
- Completeness (all static metadata exposed)
- Consistency (follows established patterns)
- Automation value (computed platforms, dual format)

**Confidence level**: High (8/10)
- Deducted 2 points for dependency field ambiguity and missing struct definitions
- With recommended clarifications, would be 9.5/10 (only minor implementation details left)

**Recommended action**: Address critical recommendations (dependencies, schema fields, struct definitions), then proceed with implementation. The architecture is solid enough to build on.
