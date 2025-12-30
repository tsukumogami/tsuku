# Design Fit Assessment: Use Case Alignment

## Assessment Summary

This document evaluates whether the two system dependency designs correctly address the two actual use cases that motivated them.

**Designs assessed:**
- `docs/DESIGN-system-dependency-actions.md` (action vocabulary)
- `docs/DESIGN-structured-install-guide.md` (structured primitives, parent design)

**Actual use cases:**
1. **Documentation Generation**: tsuku generates human-readable instructions from machine-readable specs; it does NOT execute system installs on the host
2. **Sandbox Container Building**: tsuku builds custom minimal containers for sandbox testing by extracting dependencies from recipes

---

## 1. Documentation Generation Use Case

**User's clarification**: tsuku doesn't execute `require_system` installations - it only tells users what to do. The design should make recipes machine-readable so tsuku can GENERATE documentation for users on the platform they're running.

### Does the design support generating human-readable instructions from machine-readable specs?

**Assessment: PARTIALLY**

The structured-install-guide design includes a `GenerateInstallGuide` function (lines 546-553) and mentions "Human-Readable Text Generation" as a section. This shows the design considers this use case.

```go
func GenerateInstallGuide(primitives []Primitive) string {
    var steps []string
    for _, p := range primitives {
        steps = append(steps, p.Describe())
    }
    return strings.Join(steps, "\n")
}
```

**Strengths:**
- Each primitive has a `Describe()` method that returns human-readable text
- The example output shows clear instructions: "Add APT repository...", "Run: sudo apt-get install...", etc.
- The structured format enables template-based generation

**Gaps:**
- This is framed as "for display purposes (when not executing)" - a secondary concern
- No explicit design for platform-specific instruction rendering
- No discussion of localization, formatting options, or output formats (terminal vs markdown vs HTML)

### Can tsuku filter steps by the current platform and show only relevant instructions?

**Assessment: YES**

The `when` clause mechanism (both designs) provides platform filtering:

```toml
when = { os = ["linux"] }
when = { os = ["darwin"] }
when = { distro = ["ubuntu", "debian"] }
```

The action-vocabulary design extends this with `distro` field for finer-grained Linux filtering.

**Strengths:**
- Platform filtering via `when` is well-designed
- Steps are already filtered during plan generation
- Distro detection mechanism is clearly specified

**Gaps:**
- The designs don't explicitly describe the "show instructions to user" workflow
- No mockup of what `tsuku install docker` outputs when it can't execute

### Is there a clear path from structured primitives to user-facing text?

**Assessment: WEAK**

The path exists but is underdeveloped:

1. **Recipe parsing** -> Creates plan with platform-filtered steps
2. **Primitive extraction** -> Gets structured package specs
3. **Describe() calls** -> Generates text per primitive
4. **???** -> Display to user

The final step is not designed. Questions unanswered:
- What does the CLI output look like?
- How are multi-step instructions formatted?
- What's the UX for "here's what you need to do manually"?

### Verdict: Documentation Generation

The designs provide the *foundation* for documentation generation but don't treat it as a primary use case. The structured primitives enable it, but the user-facing experience is not designed.

---

## 2. Sandbox Container Building Use Case

**User's clarification**: Today tsuku uses a fat container for `--sandbox`. The design should enable slim containers where tsuku identifies dependencies and builds custom containers per recipe.

### Does the design support extracting dependency lists from recipes?

**Assessment: YES - WELL DESIGNED**

The structured-install-guide design explicitly addresses this with `DeriveContainerSpec` (lines 618-665):

```go
func DeriveContainerSpec(plan *executor.InstallationPlan) (*ContainerSpec, error) {
    spec := &ContainerSpec{
        Base:       MinimalBaseImage,
        Packages:   make(map[string][]string),
        Primitives: nil,
    }
    // ... extracts packages and primitives from require_system steps
}
```

**Strengths:**
- Clear API for extracting dependencies
- Handles both simple (`packages`) and complex (`primitives`) forms
- Returns typed `ContainerSpec` for downstream use
- Error handling for recipes that can't be sandbox-tested

### Can these be used to build minimal containers?

**Assessment: YES - WELL DESIGNED**

The design covers the full pipeline:

1. **Minimal base container** (lines 581-598): Dockerfile starting from scratch with only tsuku + glibc
2. **Derived container generation** (lines 593-598): Generates Dockerfile from base + packages
3. **Container image caching** (lines 675-689): Content-addressed cache by package set hash
4. **Sandbox executor integration** (lines 602-668): Modified executor builds or retrieves containers

**Strengths:**
- "Minimal container" is an explicit design choice (Decision 3A)
- Forces complete dependency declarations
- Caching avoids rebuilding for repeated package sets
- Clear implementation path in Phase 3

### Is the relationship between the two designs clear?

**Assessment: SOMEWHAT CLEAR**

The action-vocabulary design explicitly states the relationship (lines 456-465):

> This design doc defines the action vocabulary for system dependencies. It feeds back into [DESIGN-structured-install-guide.md] which addresses:
> - Sandbox testing for recipes with system dependencies
> - Minimal base container strategy
> ...
> The two designs are complementary:
> - **This doc**: What actions exist and how they compose
> - **Original doc**: How to execute them safely in sandbox and on host

**Gaps:**
- The structured-install-guide doesn't reference the action-vocabulary design
- The designs have overlapping primitive definitions (both define apt, brew, etc.)
- It's unclear which design is authoritative for the primitive vocabulary

### Verdict: Sandbox Container Building

This use case is well-addressed. The designs provide clear mechanisms for extracting dependencies, building minimal containers, and caching images.

---

## 3. Misalignments

### Does the design imply host execution when it shouldn't?

**Assessment: YES - SIGNIFICANT MISALIGNMENT**

Both designs spend substantial effort on host execution:

**Action vocabulary design:**
- Security Constraints section (lines 256-294) for host execution
- Tiered consent flow for privileged operations
- Audit logging requirements
- Group and repository allowlisting

**Structured install guide design:**
- Phase 4 is "User Consent and Host Execution" (lines 771-776)
- User Consent Flow section (lines 519-539) with interactive prompts
- Execution Isolation section describes both sandbox AND host contexts
- Entire "Primitive Execution" section (lines 485-517) shows `Execute()` methods

**The problem:** If tsuku TODAY only generates documentation (use case 1) and builds sandbox containers (use case 2), then:
- Security Constraints for host execution are premature
- Tiered consent flow is premature
- Audit logging is premature
- The `Execute()` method on primitives may not be needed for host context

**Specific misleading content:**
- "Machine-executable: Tsuku can install system dependencies automatically (with user consent)" - implies host execution
- Phase 4 treats host execution as in-scope for this design
- User consent flow mockup shows `tsuku install docker` executing apt-get

### Are the Phase descriptions accurate for these use cases?

**Assessment: PHASES ARE MIXED**

Looking at the structured-install-guide phases:

| Phase | Description | Use Case Alignment |
|-------|-------------|-------------------|
| Phase 1 | Refactor require_system Action | Both use cases |
| Phase 2 | Primitive Framework | Both use cases |
| Phase 3 | Sandbox Execution | Use case 2 (sandbox) |
| Phase 4 | User Consent and Host Execution | **NEITHER use case** |
| Phase 5 | Extension | Both use cases |

Phase 4 is out of scope for the actual use cases. It introduces host execution which the user explicitly clarified tsuku does NOT do today.

The action-vocabulary design phases are similarly mixed:
- Phases 1-3: Relevant (infrastructure, actions, repos)
- Phase 4: "System Configuration" includes `group_add`, `service_enable` - only relevant for sandbox or future host execution
- Phase 5: "Security and Consent" - premature for current use cases

### What's missing or overcomplicated?

**Missing:**
1. **Documentation generation UX** - No design for how `tsuku install X` displays instructions when execution isn't possible
2. **Fallback text for sandbox** - What happens when a primitive isn't supported in sandbox? (e.g., `manual`)
3. **Partial platform coverage** - What if a recipe has apt packages but not brew? How does tsuku communicate this?

**Overcomplicated:**
1. **Host execution security model** - Tiered consent, audit logging, group allowlisting - all premature
2. **Service and group primitives** - `group_add`, `service_enable`, `service_start` - only relevant in sandbox (containers) or future host execution
3. **Privilege escalation discussion** - Extensive security analysis for a feature that's not being implemented

---

## 4. Recommendations

### Reframe the designs around actual use cases

**Recommendation 1: Explicitly state what tsuku does NOT do**

Add a section early in both designs:

```markdown
## Non-Goals

- **Host execution**: tsuku does not execute system package installations on the user's machine. It generates instructions for users to follow manually.
- **Privilege escalation**: tsuku never runs sudo on the host.
```

This prevents readers from inferring that the designs enable host execution.

### Separate phases by use case

**Recommendation 2: Reorder phases to match actual use cases**

Current phases mix concerns. Proposed reordering:

| Phase | Description | Use Case |
|-------|-------------|----------|
| Phase 1 | Infrastructure (distro detection, when clause) | Both |
| Phase 2 | Primitive vocabulary (parsing, validation) | Both |
| Phase 3 | Documentation generation (Describe(), CLI output) | Use case 1 |
| Phase 4 | Sandbox container derivation and caching | Use case 2 |
| Phase 5 (Future) | Host execution with consent | Future work |

This makes it clear that host execution is explicitly deferred.

### Design the documentation generation UX

**Recommendation 3: Add documentation generation design**

The designs are missing this critical piece. Add a section covering:

1. **CLI output format** when `require_system` steps can't be executed:
   ```
   $ tsuku install docker

   Docker requires system dependencies that tsuku cannot install directly.

   For Ubuntu/Debian, run:
     1. Add Docker repository:
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        echo "deb [signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list
     2. Install packages:
        sudo apt-get update && sudo apt-get install docker-ce docker-ce-cli containerd.io
     3. Add yourself to docker group:
        sudo usermod -aG docker $USER

   After completing these steps, run: tsuku install docker --verify
   ```

2. **Platform detection** for instruction selection
3. **Verification command** to check if system deps are satisfied

### Move host execution to Future Work

**Recommendation 4: Extract host execution to dedicated future work section**

The current designs conflate two concerns:
- **Now**: Sandbox execution (automated, in container, Phase 3)
- **Future**: Host execution (user consent, on host, not in scope)

Move all of these to a "Future Work: Host Execution" section:
- Security constraints (tiered consent, audit logging)
- User consent flow
- Privilege escalation paths
- Group and repository allowlisting

This keeps the designs focused on the actual use cases.

### Clarify primitive execution contexts

**Recommendation 5: Define execution context explicitly**

The `Primitive` interface should clarify WHERE it executes:

```go
type Primitive interface {
    // Validate checks parameters without side effects
    Validate() error

    // ExecuteInSandbox runs the primitive in a container context (as root)
    ExecuteInSandbox(ctx *SandboxContext) error

    // GenerateInstructions returns human-readable steps for manual execution
    GenerateInstructions() []string

    // Note: Host execution is NOT implemented. Primitives do not run on the user's machine.
}
```

This makes it explicit that:
- Sandbox execution is supported
- Documentation generation is supported
- Host execution is NOT supported (today)

### Consolidate primitive definitions

**Recommendation 6: Single source of truth for primitives**

Both designs define primitives. This creates confusion about which is authoritative.

Consolidate:
- Action-vocabulary design: Defines the TOML syntax and `when` clause semantics
- Structured-install-guide design: Defines the execution/rendering implementation

Add cross-reference: "For primitive syntax and composition, see DESIGN-system-dependency-actions.md. This document covers execution and rendering."

---

## Summary

| Use Case | Alignment | Notes |
|----------|-----------|-------|
| Documentation Generation | Partial | Foundation exists but UX not designed |
| Sandbox Container Building | Strong | Well-designed with clear implementation path |
| Host Execution (NOT a use case) | Overcovered | Significant design effort on something tsuku doesn't do |

**Key insight**: The designs are good technical work but have scope creep toward host execution, which conflicts with the user's clarification that tsuku only generates instructions, it doesn't execute them.

**Primary recommendations:**
1. Explicitly state non-goals (no host execution)
2. Reorder phases to match actual use cases
3. Design the documentation generation UX (currently missing)
4. Move host execution to Future Work
