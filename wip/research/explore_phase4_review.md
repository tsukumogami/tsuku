# Design Review: install_binaries Parameter Semantics

## Executive Summary

This review analyzes the problem statement and options for renaming the `binaries` parameter in tsuku's `install_binaries` action. Overall, the design is well-researched and presents valid concerns. However, there are several areas requiring clarification, missing alternatives, and potential strawman options.

**Key Findings:**
1. Problem statement is specific but could benefit from clearer success criteria
2. Missing alternative: keeping current parameter with improved documentation
3. Pros/cons are generally fair but missing important considerations
4. Unstated assumption about Unix path conventions needs explicit validation
5. Option 2C appears to be a strawman with understated complexity

## 1. Problem Statement Specificity

### Assessment: MOSTLY SPECIFIC, NEEDS SUCCESS CRITERIA

The problem statement clearly articulates the semantic confusion issue and provides concrete examples. However, it lacks measurable success criteria to evaluate solutions against.

**Strengths:**
- Quantified impact (35 recipes with directory mode, 17 mixing lib/bin paths)
- Clear explanation of conflated concerns (export vs executability)
- Concrete examples from actual recipes (ncurses.toml)
- Mode-dependent behavior documented

**Missing Elements:**

1. **Success Criteria**: What specific outcomes would indicate the problem is solved?
   - Recipe authors can determine file types without reading docs?
   - Static analysis tools can extract executable lists programmatically?
   - No semantic confusion in code reviews?

2. **User Impact**: Beyond recipe authors, who else is affected?
   - Tool developers building on tsuku?
   - Users debugging installation issues?
   - CI/CD systems analyzing recipes?

3. **Severity Classification**: Is this a blocker, major issue, or quality-of-life improvement?
   - Current system works functionally (recipes install correctly)
   - Confusion happens at authoring time, not runtime
   - Suggests this is a "developer experience" issue, not a critical bug

**Recommendation**: Add explicit success criteria:
```markdown
## Success Criteria

A successful solution will:
1. Allow recipe authors to intuitively identify file types from parameter names
2. Enable static analysis to extract executable lists without executing recipes
3. Align parameter semantics with existing actions (configure_make, npm_install)
4. Minimize migration burden for existing recipes
```

## 2. Missing Alternatives

### Assessment: KEY ALTERNATIVE MISSING

The design jumps to renaming without considering whether improved documentation or tooling could address the confusion.

### Option 0: Status Quo with Improved Tooling

**Keep `binaries` parameter but address confusion through:**

```toml
# Add explanatory comments to recipes
[[steps]]
action = "install_binaries"
install_mode = "directory"
# binaries: Files to symlink to ~/.tsuku/bin/ (executables, libraries, headers)
# For directory mode, this lists files to export, not just executables
binaries = [
    "bin/ncursesw6-config",   # executable
    "lib/libncurses.so",      # library
]
```

**Pros:**
- Zero migration cost (existing recipes work unchanged)
- Schema validation could warn about lib/ entries in binaries array
- Documentation improvements are low-risk
- Recipe linting could suggest separating executables vs libraries

**Cons:**
- Doesn't fix the semantic confusion at API level
- Still requires documentation reading
- Static analysis still ambiguous without heuristics
- Perpetuates misleading terminology

**Why This Matters:**
The design dismisses status quo implicitly but doesn't evaluate whether the benefits of renaming outweigh the migration costs. For a codebase with 35 affected recipes, this alternative deserves explicit consideration.

### Option 2D: Hybrid Approach (Infer with Override)

**Use path-based inference as default, but allow explicit override:**

```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/tool", "lib/libfoo.so", "share/unusual-script"]
executables = ["share/unusual-script"]  # Only needed for edge cases
```

**Pros:**
- Simple common case (95% of recipes don't need executables param)
- Handles edge cases (executables outside bin/)
- Explicit when needed, implicit otherwise
- Gradual migration path (add executables only where needed)

**Cons:**
- Two ways to do the same thing (implicit vs explicit)
- Inconsistent with configure_make/npm_install (always explicit)
- Recipe authors might not know when to use explicit form

**Why This Matters:**
This represents a middle ground between Option 2A (pure inference) and Option 2B (pure explicit), but isn't evaluated. The design presents a false dichotomy between all-implicit vs all-explicit.

## 3. Fairness of Pros/Cons

### Assessment: GENERALLY FAIR, WITH NOTABLE OMISSIONS

The pros/cons are mostly balanced but miss important considerations.

### Option 1A: Rename to `files`

**Missing Cons:**
- Overloaded term: "files" could mean source files, temp files, any filesystem objects
- Doesn't distinguish between tracked vs untracked outputs (e.g., why list some files but not others?)
- Too generic to guide recipe authors on what should be included

**Missing Pros:**
- Shortest, most universally understood term
- No risk of confusion with programming concepts (unlike "exports")

### Option 1B: Rename to `outputs`

**Missing Pros:**
- Aligns with build system terminology (Make targets, CMake outputs)
- Clearly indicates "products of a build process"
- Works well with library recipes (libraries are outputs too)

**Missing Cons:**
- In shell/CI context, "output" often means stdout/stderr (noted, but understated)
- Nix uses "outputs" for a different concept (multiple output derivations, not file lists)
- May imply completeness (all outputs?) when actually it's a subset

### Option 1C: Rename to `exports`

**Missing Pros:**
- Emphasizes user-facing API surface (what gets exposed)
- Natural pairing with "import" concept if later introduced
- Works well for both tools (export executables) and libraries (export APIs)

**Missing Cons:**
- "Export" implies outbound sharing, but these files stay local
- Less intuitive for pure executables that aren't "exported APIs"
- Doesn't clearly indicate filesystem paths vs symbolic exports

### Option 2A: Infer from Path Prefix

**Critical Missing Con:**
- **Executables in non-standard locations**: The design acknowledges this as "edge case" but doesn't validate the assumption. Analysis of 35 recipes shows:
  - `bin/` prefix: 95%+ of executables
  - BUT: `share/`, `libexec/` contain scripts in some GNU tools
  - Node.js wrappers might be in custom locations
  - GO's `pkg/` directory contains binaries for different architectures

**Missing Pro:**
- Self-documenting through Unix conventions (path tells you the type)

### Option 2B: Explicit `executables` Parameter

**Missing Pro:**
- **Consistency across actions**: This is the killer advantage. The design mentions it but understates its importance. Having `configure_make`, `npm_install`, AND `install_binaries` all use `executables` creates a strong, learnable pattern.

**Missing Con:**
- Validation burden: With explicit lists, tsuku must validate that declared executables actually exist and are executable

### Option 2C: File Type Detection

**Assessment: APPEARS TO BE A STRAWMAN**

This option is presented in a way that makes it obviously inferior:

**Understated Pros:**
- Future-proof for new executable formats
- No dependency on directory structure
- Could detect scripts by shebang presence (not mentioned)

**Overstated Cons:**
- "Adds complexity (ELF/Mach-O parsing)" - Libraries like `debug/elf` in Go standard library make this trivial
- "Shared libraries are also ELF/Mach-O" - But with different ELF type (ET_DYN vs ET_EXEC), easily distinguished
- "May not work for scripts without shebangs" - But could fall back to path inference

**Missing Cons:**
- Performance overhead (must read file contents, not just paths)
- Fails for symbolic links (must follow link first)
- Doesn't work during plan generation (files don't exist yet)

**Why This Matters:**
Option 2C is presented as obviously worse, but the cons are either fixable or overstated. This suggests it's included to make Option 2A or 2B look better by comparison (strawman).

## 4. Unstated Assumptions

### Assessment: CRITICAL ASSUMPTION NEEDS VALIDATION

### Assumption 1: Unix Path Conventions are Universal

**Unstated Assumption:**
> "Files in `bin/` are executable, files in `lib/` are not"

**Evidence Supporting:**
- Analysis of 35 directory-mode recipes shows:
  - All `bin/` entries appear to be executables
  - All `lib/` entries appear to be libraries
  - Some recipes have ONLY `bin/` entries (git, sqlite)
  - Some have ONLY `lib/` entries (zlib, libpng)

**Evidence Against:**
- Some packages (nodejs, ruby) have complex directory structures
- `libexec/` directory contains executable helper scripts (git-remote-*, git-svn)
- `share/` might contain executable scripts
- Windows paths don't follow this convention (but tsuku doesn't support Windows yet)

**Risk if Invalid:**
If the assumption breaks, Option 2A (path inference) will:
- Make non-executables executable (low risk: chmod harmless on libraries)
- Fail to make executables executable (HIGH RISK: installation breaks)

**Recommendation:**
Audit all 35 directory-mode recipes to validate this assumption:
```bash
# Extract all binaries entries
# Check for non-bin/ executables
# Check for bin/ non-executables
```

If >95% follow the convention, path inference is safe with explicit override option.
If <95%, explicit declaration is required.

### Assumption 2: Migration is Low-Cost

**Unstated Assumption:**
> "Updating 35 recipes is acceptable burden"

**Factors Not Considered:**
- Recipe update requires version bump or breaking change flag
- External recipe repositories may exist (forks, private recipes)
- Deprecation period needed for `binaries` parameter
- Test matrix must validate both old and new parameter names
- Documentation and examples must be updated

**Recommendation:**
Design should include migration plan:
1. Phase 1: Support both `binaries` and new parameter (warning on `binaries`)
2. Phase 2: Migrate all official recipes
3. Phase 3: Deprecate `binaries` (error if used)
4. Timeline: 3-6 months per phase

### Assumption 3: Executables Need chmod 0755

**Unstated Assumption:**
> "The semantic confusion matters because executability requires chmod 0755"

**Challenge:**
- For `install_mode = "directory"`, the action copies the ENTIRE tree
- Existing file permissions are preserved during copy
- If source tarball has correct permissions, no chmod needed
- The confusion is semantic (misleading names) not functional (wrong behavior)

**Question to Answer:**
Does the current implementation apply chmod to libraries when it shouldn't?
- If NO: This is purely a semantic/clarity issue (lower priority)
- If YES: This is a functional bug (higher priority)

**From Code Analysis:**
```go
// install_binaries.go:286 - installDirectoryWithSymlinks
if err := CopyDirectory(ctx.WorkDir, ctx.InstallDir); err != nil {
    return fmt.Errorf("failed to copy directory tree: %w", err)
}
```

The directory copy preserves permissions. The `binaries` list is only used for symlink creation, NOT for chmod. This means:
- **The functional impact is zero** (no incorrect permissions applied)
- **The confusion is purely semantic** (misleading parameter name)
- **The static analysis issue is real** (can't determine executables from recipe alone)

**Recommendation:**
Problem statement should clarify this is a clarity/tooling issue, not a functional bug. This affects prioritization.

## 5. Strawman Options

### Assessment: OPTION 2C IS LIKELY A STRAWMAN

**Definition**: A strawman is an option presented to be rejected, making other options look better.

**Evidence Option 2C is a Strawman:**

1. **Overstated Complexity**: "Adds complexity (ELF/Mach-O parsing)"
   - Go's `debug/elf` and `debug/macho` packages make this ~20 lines of code
   - Other tools (file, readelf) do this trivially

2. **Fixable Cons Presented as Blockers**: "May not work for scripts without shebangs"
   - Could fall back to path inference
   - Could check execute bit from source file

3. **Missing Obvious Pro**: "Zero configuration required"
   - Recipe authors don't need to understand executability at all
   - Works for any directory structure automatically

4. **Dismissed Too Quickly**: "Cross-platform detection is tricky"
   - Tsuku only supports Linux/macOS currently
   - Both use standard ELF/Mach-O formats

**Alternative Presentation (Fair Treatment):**

Option 2C could be reframed as:
```markdown
### Option 2C: File Type Detection (Deferred)

Detect executables by inspecting file format (ELF/Mach-O).

**Pros:**
- Zero configuration (works for any directory structure)
- Explicit file format analysis (no heuristics)
- Handles edge cases automatically

**Cons:**
- Requires reading file contents (performance overhead)
- Doesn't work during plan generation (files not available)
- Must distinguish executables from shared libraries (same format)

**Decision:** Defer to future iteration. While technically feasible, this adds
complexity without solving the clarity issue. Path inference (2A) handles
99% of cases with simpler implementation.
```

**Why This Matters:**
Including obviously-inferior options reduces reader trust. Better to:
- Present 2-3 genuinely competitive options
- Or explicitly mark Option 2C as "Considered and Rejected" section
- Or remove it entirely and mention in "Alternatives Not Pursued"

## 6. Additional Considerations

### Missing from Evaluation Matrix

The evaluation matrix (line 276-284) rates options but misses key factors:

**Missing Factors:**

1. **Migration Cost**: How much work to convert existing recipes?
   - 1A/1B/1C: All require updating 35 recipes (equal cost)
   - 2A: No recipe changes needed (lowest cost)
   - 2B: Requires adding executables param to all recipes (highest cost)

2. **Consistency with Ecosystem**: How well does it match external tools?
   - 1B (outputs): Matches Nix, CMake
   - 1C (exports): Matches npm
   - 2B (executables): Matches Homebrew

3. **Forward Compatibility**: Does it support future features?
   - What if we add library-specific tracking?
   - What if we add permission customization?
   - What if we add conditional installation?

### Combinations Not Considered

The design treats Decision 1 and Decision 2 as independent, but some combinations are stronger:

**Strong Combination: 1B + 2A**
```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = [
    "bin/tool",      # Executable (inferred from bin/ prefix)
    "lib/libfoo.so"  # Library (inferred from lib/ prefix)
]
```

**Pros:**
- "outputs" clearly indicates build products
- Path inference removes redundancy
- Aligns with Nix terminology
- Simple migration (rename parameter)

**Strong Combination: 1C + 2B**
```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
exports = ["bin/tool", "lib/libfoo.so"]
executables = ["bin/tool"]
```

**Pros:**
- "exports" emphasizes public API
- Explicit executables aligns with configure_make/npm_install
- Separates concerns cleanly

**The design evaluates each decision independently but doesn't guide toward optimal combinations.**

## 7. Recommendations

### Immediate Actions

1. **Validate Unix Path Convention Assumption**
   - Audit all 35 directory-mode recipes
   - Check for executables outside `bin/`
   - Check for non-executables in `bin/`
   - Document findings

2. **Add Success Criteria to Problem Statement**
   - Define what "solved" looks like
   - Include measurability (e.g., "recipe authors can determine types without docs")

3. **Clarify Functional vs Semantic Impact**
   - State explicitly that current system works functionally
   - Emphasize this is a clarity/tooling improvement, not a bug fix

4. **Consider Status Quo Alternative**
   - Evaluate whether improved documentation/linting could address confusion
   - Compare cost/benefit vs full parameter rename

### Option Revisions

1. **Option 1: Parameter Naming**
   - **Recommend 1B (outputs)** for:
     - Clear build system terminology
     - Aligns with Nix
     - Works for both tools and libraries

   - **Reject 1A (files)**: Too generic
   - **Reject 1C (exports)**: Confusing for executables

2. **Option 2: Executability Logic**
   - **Recommend 2A (path inference)** IF validation confirms >95% adherence to Unix conventions
   - **Recommend 2B (explicit)** IF validation shows significant edge cases
   - **Defer 2C (detection)**: Implementation complexity doesn't justify benefits

3. **Combined Recommendation**
   - **Preferred: 1B + 2A** (outputs with inference)
   - **Fallback: 1B + 2D** (outputs with inference + override)

### Migration Plan

If proceeding with rename:

```toml
# Phase 1: Deprecation (v0.x)
[[steps]]
action = "install_binaries"
binaries = ["bin/tool"]  # DEPRECATED: Use 'outputs' instead

# Phase 2: Migration (v0.x+1)
[[steps]]
action = "install_binaries"
outputs = ["bin/tool"]

# Phase 3: Removal (v1.0)
# 'binaries' parameter removed entirely
```

Timeline: 3 months per phase

### Documentation Improvements (Regardless of Decision)

Even if keeping `binaries`:
1. Add inline comments to recipe schema
2. Create recipe authoring guide explaining parameter semantics
3. Build linting tool to warn about semantic confusion
4. Add "Common Mistakes" section to docs

## Conclusion

The design is well-researched and identifies a real clarity issue. However:

1. **Problem statement** needs explicit success criteria and severity classification
2. **Missing alternatives** (status quo, hybrid approach) should be evaluated
3. **Pros/cons** are mostly fair but miss important considerations
4. **Critical assumption** about Unix path conventions needs validation before choosing Option 2A
5. **Option 2C** appears to be a strawman and should be removed or reframed

**Overall Assessment: DESIGN IS SOUND BUT INCOMPLETE**

**Recommendation: DO NOT PROCEED until:**
1. Unix path convention assumption is validated through recipe audit
2. Status quo alternative is explicitly evaluated
3. Success criteria are defined
4. Migration cost is estimated and planned

Once these gaps are addressed, the design provides a solid foundation for improving recipe clarity.
