# Issue 394 Implementation Plan

## Summary

Create detailed refactoring issues for each identified high-churn file, proposing specific functional splits that follow Go best practices for single-responsibility and cohesive modules.

## Approach

For each file, analyze functional responsibilities and propose splits that:
1. Follow Go best practices (single responsibility per file/package)
2. Minimize cross-file dependencies
3. Group related functionality together
4. Make the codebase easier to navigate and test

This is a meta-task that produces issues, not code changes.

### Alternatives Considered
- **Single mega-refactoring PR**: Rejected - high risk, difficult to review, likely to cause conflicts
- **Arbitrary LOC splits**: Rejected - splitting by line count rather than function leads to poor cohesion
- **No refactoring**: Rejected - growing merge conflicts indicate structural problems

## Files to Analyze

Each file needs analysis with specific split proposals:

### 1. internal/version/resolver.go (1,269 lines, HIGH priority)

**Current responsibilities:**
- HTTP client factory with security hardening (lines 46-147)
- Resolver struct with 9 registry URL fields (lines 31-44)
- 7 nearly identical `NewWith*Registry` constructors (lines 149-364) - significant duplication
- GitHub version resolution (lines 384-577)
- npm version resolution + validation (lines 579-867)
- HashiCorp resolution (lines 721-742)
- NodeJS resolution (lines 869-925)
- Go toolchain resolution (lines 927-1076)
- Go module proxy resolution (lines 1078-1248)
- Version utilities (normalization, comparison) (lines 744-820)
- Custom source resolution (lines 1250-1269)

**Proposed split:**
1. `httpclient.go` - HTTP client factory, security utilities
2. `options.go` - Functional options pattern to replace duplicated constructors
3. `github.go` - GitHub-specific resolution (move from resolver)
4. `npm.go` - npm resolution (already exists `npm_test.go`, consolidate)
5. `nodejs.go` - Node.js distribution resolution
6. `version_utils.go` - normalizeVersion, compareVersions, isValidVersion

The package already has separate provider files (`provider_*.go`). The main resolver.go could delegate to these and focus on being a facade.

### 2. cmd/tsuku/install.go (603 lines, MEDIUM priority)

**Current responsibilities:**
- CLI command definition (lines 19-74)
- Installation orchestration with telemetry (lines 97-473)
- Package manager auto-bootstrap (lines 101-153)
- Library installation (lines 476-543)
- Dependency path resolution (lines 155-181)
- Dry-run execution (lines 546-568)
- Runtime dependency resolution (lines 570-603)

**Proposed split:**
1. `install.go` - CLI command definition, flags, arg parsing
2. `install_deps.go` - Dependency resolution and auto-bootstrap
3. `install_lib.go` - Library installation logic
4. Move core installation logic to `internal/install/` package

### 3. internal/install/state.go (613 lines, MEDIUM priority)

**Current responsibilities:**
- State types (ToolState, LibraryVersionState) (lines 15-66)
- StateManager with locking (lines 68-240)
- Tool operations (UpdateTool, RemoveTool, RequiredBy) (lines 241-319)
- Library operations (UpdateLibrary, UsedBy) (lines 321-427)
- LLM usage tracking (RecordGeneration, CanGenerate, etc.) (lines 482-613)
- Migration logic (lines 458-480)
- Validation utilities (lines 443-456)

**Proposed split:**
1. `state.go` - Core types and StateManager
2. `state_tool.go` - Tool-specific operations
3. `state_lib.go` - Library-specific operations
4. `state_llm.go` - LLM usage tracking (distinct domain, could be own package)

### 4. internal/builders/github_release.go (1,110 lines, LOW priority)

**Current responsibilities:**
- Builder struct and options (lines 34-136)
- CanBuild/Build entry points (lines 138-241)
- Conversation loop logic (lines 253-427)
- Tool execution (lines 429-475)
- Validation repair loop (lines 254-343)
- GitHub API interactions (lines 572-753)
- Recipe generation (lines 756-858)
- Prompt building (lines 860-998)
- Archive inspection (lines 1059-1078)

**Assessment:** While large, this file is relatively stable and has good internal cohesion. The LLM conversation flow is inherently complex. Split might not be beneficial.

**Recommendation:** Evaluate but likely skip unless pain points emerge.

## Implementation Steps

- [x] Create refactoring issue for `internal/version/resolver.go` with detailed split proposal (#397)
- [ ] Create refactoring issue for `cmd/tsuku/install.go` with detailed split proposal
- [ ] Create refactoring issue for `internal/install/state.go` with detailed split proposal
- [ ] Evaluate `internal/builders/github_release.go` and decide if issue needed
- [ ] Document refactoring guidelines in CONTRIBUTING.md or docs/

## Testing Strategy

This is a documentation/issue creation task. Success is validated by:
- Issues are created with clear, actionable proposals
- Each issue includes specific file/function movements
- Issues reference this tracking issue (#394)

## Risks and Mitigations

- **Risk:** Proposed splits might break imports or circular dependencies
  - **Mitigation:** Carefully analyze import graphs before proposing splits

- **Risk:** Split proposals might be subjective
  - **Mitigation:** Focus on objective criteria (LOC, change frequency, functional cohesion)

## Success Criteria

- [ ] Issue created for resolver.go refactoring
- [ ] Issue created for install.go refactoring
- [ ] Issue created for state.go refactoring
- [ ] Evaluation documented for github_release.go
- [ ] Refactoring guidelines documented

## Open Questions

None - task is well-defined.
