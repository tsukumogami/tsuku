# Tier 2 Dependency Resolution - Design Tracker

**Issue:** #948
**Branch:** `docs/library-verify-deps`
**PR:** #966
**Last Updated:** 2026-01-17

## Current Status

**Phase:** Design iteration - refining scope and validation behavior

The initial design focused narrowly on library → library dependencies. Through discussion and research, we've expanded scope to include tools and refined the validation model.

---

## Key Learnings

### 1. Binary vs Recipe Dependencies

Two distinct dependency graphs exist:

| Level | Format | Example | Source |
|-------|--------|---------|--------|
| **Binary** | Sonames | `libssl.so.3`, `libc.so.6` | Embedded in ELF/Mach-O at compile time |
| **Recipe** | Recipe names | `openssl`, `zlib` | Declared in TOML, tracked in state.json |

**Key insight:** Binary deps are deterministically inferrable from DT_NEEDED/LC_LOAD_DYLIB. Recipe deps are human-declared and may be incomplete.

**Verification goal:** Warn when binary deps don't match recipe declarations.

### 2. Burden Distribution

The concern about "recipe author burden" was analyzed. The burden is **concentrated, not distributed**:

| Category | % of Recipes | Examples | DT_NEEDED | Burden |
|----------|--------------|----------|-----------|--------|
| Go/Rust tools | ~40% | gh, ripgrep, terraform | Zero (static) | **None** |
| Pre-built downloads | ~30% | GitHub releases | Baked in | **Can't change** |
| Ecosystem installs | ~10% | npm_install, pip_install | Interpreter handles | **None** |
| Library recipes | ~15% | openssl, zlib, libyaml | They ARE deps | Need `provides` |
| Foundational builds | ~5% | ruby, python, perl | Many deps | **High** |

**Conclusion:** Only ~20% of recipes (libraries + foundational) need attention. Core maintainers bear the burden, not every contributor.

### 3. System Library Detection

**Problem:** Distinguishing system libs from tsuku-managed libs is hard, especially on macOS (dyld shared cache).

**Solution:** Hardcoded system library registry per OS/arch. This is maintenance but tractable (~50-100 patterns).

### 4. PURE SYSTEM Definition (Clarified)

**Critical insight:** A library is PURE SYSTEM because it is **inherently OS-provided**, not because "no tsuku recipe exists."

| Incorrect reasoning | Correct reasoning |
|---------------------|-------------------|
| "No recipe exists" → therefore PURE SYSTEM | Library is OS-provided → therefore no recipe exists |

**Why this matters:** If we defined PURE SYSTEM as "no recipe exists," then any library we haven't written a recipe for yet would be classified as PURE SYSTEM. That's wrong - it should be UNKNOWN.

**The three categories are defined by inherent properties:**

| Category | Definition | Detection Method |
|----------|------------|------------------|
| **PURE SYSTEM** | Library is inherently OS-provided (libc, libm, etc.) | Pattern matching (encodes our knowledge of OS-provided libs) |
| **TSUKU-MANAGED** | Library is managed by tsuku | Soname found in our index (state.json) |
| **UNKNOWN** | Library we don't recognize | Neither in index nor matching system patterns |

The system library registry patterns (`libc.so`, `libm.so`, `libpthread.so`, etc.) encode our knowledge of "these libraries are provided by the operating system." The absence of a tsuku recipe for them is a consequence of this classification, not the cause.

### 5. Existing Infrastructure

| Component | Location | Reusable? |
|-----------|----------|-----------|
| Cycle detection | `internal/actions/resolver.go` | ✅ Yes |
| Depth limiting | `MaxTransitiveDepth = 10` | ✅ Yes |
| Header parsing | `internal/verify/header.go` | ✅ Yes (already extracts deps) |
| Transitive expansion | `ResolveTransitive()` | ✅ Yes |

---

## Decisions Made

### 1. Hybrid Approach for System Library Detection
**Decision:** Pattern-based detection for system libraries + RPATH-aware resolution for tsuku-managed deps

**Status:** ✅ Decided

### 2. Error Category Constants
**Decision:** Use explicit values (10, 11, 12) instead of `iota + 100`

**Status:** ✅ Decided

### 3. RPATH Extraction Implementation
**Decision:** Use `debug/elf.DynString(DT_RUNPATH)` and `debug/macho.Rpath`

**Status:** ✅ Decided

### 4. Symlink Handling
**Decision:** Use `filepath.EvalSymlinks()` consistent with PR #963

**Status:** ✅ Decided

### 5. Burden Distribution is Acceptable
**Decision:** The ~20% of recipes needing updates (libraries + foundational) is tractable for core maintainers.

**Status:** ✅ Decided (pending user confirmation)

### 6. PT_INTERP Validation for PURE SYSTEM Libraries
**Decision:** Add PT_INTERP (dynamic linker) validation even for PURE SYSTEM libraries to detect ABI mismatches.

**Context:** While we classify PURE SYSTEM libraries (libc, libm, etc.) and skip them entirely during dependency resolution, this misses ABI mismatches. For example, a glibc-linked binary won't run on Alpine (musl) because the expected dynamic linker doesn't exist.

**Approach:**
```go
// Extract interpreter from ELF PT_INTERP segment
interp := getInterpreter(binary)  // e.g., "/lib64/ld-linux-x86-64.so.2"

if interp != "" && !fileExists(interp) {
    // Binary expects glibc but we're on musl (or vice versa)
    return Warning("Binary requires %s which is not present (ABI mismatch?)", interp)
}
```

**Why this matters:**
- The dynamic linker (PT_INTERP) is the kernel's entry point for running dynamically-linked binaries
- If `/lib64/ld-linux-x86-64.so.2` (glibc) doesn't exist, the binary cannot start at all
- This catches the most critical ABI mismatch: glibc vs musl
- Cost is minimal: one stat() call per binary

**Status:** ✅ Decided

---

## Pending Decisions

### 1. Scope: Tools + Libraries or Libraries Only?

**Context:** Tools also have DT_NEEDED entries. Should `tsuku verify <tool>` validate binary deps?

**Options:**
| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A | Libraries only | Simpler, matches original design | Inconsistent UX |
| B | Tools + Libraries | Unified model, more complete | More code paths |
| C | Unified `ValidateDependencies()` | Code reuse, consistent | Need to handle static binaries |

**Leaning:** Option C - unified function used by both `verifyTool()` and `verifyLibrary()`

**Status:** 🔄 Needs decision

### 2. Validation Behavior: Warn vs Fail

**Context:** What happens when binary has dep not declared in recipe?

**Options:**
| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A | Warn only | Non-blocking, graceful migration | May be ignored |
| B | Fail by default | Enforces correctness | Breaks existing recipes |
| C | Configurable | Flexibility | Complexity |

**Leaning:** Option A for MVP, consider C later

**Status:** 🔄 Needs decision

### 3. Recursion: Opt-in or Default?

**Context:** Should `tsuku verify` recursively validate dependencies?

**Options:**
| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A | Direct deps only (default) | Fast, simple | Misses transitive issues |
| B | `--deep` flag for recursive | User control | Extra flag to remember |
| C | Always recursive | Complete | Slow, verbose |

**Leaning:** Option B - match pattern of `--integrity` flag

**Status:** 🔄 Needs decision

### 4. Soname → Recipe Mapping

**Context:** Need soname → recipe mapping for Tier 2 validation.

**Decision:** Use **auto-discovery at install time**.

**Approach:**
- At library install: Extract DT_SONAME (ELF) / LC_ID_DYLIB (Mach-O) from binaries
- Store in `state.json` under `LibraryVersionState.Sonames`
- Build reverse index at verification time for Tier 2 lookups

**Out of scope:** Optional `provides` field for discoverability is a separate feature (issue #969).

**Status:** ✅ Decided - see `wip/research/soname-auto-discovery.md`

### 5. System Recipe Detection

**Context:** How do we know a recipe is "externally managed" (stops recursion)?

**Decision:** Infer from actions via `IsExternallyManaged()` method on `SystemAction` interface.

**Design:**
- Add `IsExternallyManaged() bool` to `SystemAction` interface
- Package manager actions return `true`: apt_install, brew_install, dnf_install, etc.
- Other actions return `false`: group_add, download, go_build, etc.
- Recipe-level: `IsExternallyManagedFor(target)` checks if ALL filtered steps are externally managed

**Why this approach:**
- Knowledge lives in the action (no central registry)
- Target-dependent (step filtering happens first)
- Follows existing pattern (`ImplicitConstraint()`)
- Extensible (new package managers just implement the method)

**Status:** ✅ Decided - see `wip/research/action-interface-pattern.md`

---

## Research Completed

| Topic | Document | Key Finding |
|-------|----------|-------------|
| Recent PRs review | `recent_pr_review.md` | Tier 1 implemented, integration point identified |
| Circular dependency handling | `circular-dependency-handling.md` | Existing cycle detection reusable |
| System dependency recipes | `system-dependency-recipes.md` | System deps use pkg manager actions |
| Tool binary dependencies | `tool-binary-dependencies.md` | Tools not currently validated for DT_NEEDED |
| Recipe vs binary mapping | `recipe-binary-dep-mapping.md` | No soname→recipe mapping exists |
| Current verify behavior | `current-verify-behavior.md` | Tools: execution; Libraries: Tier 1 only |
| Architecture debate | `debate-architecture.md` | FEASIBLE: ~1500 lines, 1-2 weeks |
| UX debate | `debate-ux.md` | CONCERNING: needs opt-in, author tooling |
| Skeptic debate | `debate-skeptic.md` | BLOCKERS revised: burden is tractable |
| Soname auto-discovery | `soname-auto-discovery.md` | Extract DT_SONAME at install, store in state.json |
| Action interface pattern | `action-interface-pattern.md` | Add `IsExternallyManaged()` to SystemAction |
| System library registry | `system-library-registry.md` | 47 patterns total, prefix matching |
| Static binary handling | `static-binary-handling.md` | Detect via PT_INTERP, show explicit message |

---

## Research Needed

~~All research items completed.~~

### ~~1. System Library Registry Design~~ ✅ COMPLETED
- 47 total patterns (18 Linux sonames, 12 Linux paths, 10 macOS, 5 path variables)
- Prefix matching on soname basename
- Handles macOS dyld cache (pattern-only, no file existence check)
- See `system-library-registry.md`

### ~~2. Static Binary Handling~~ ✅ COMPLETED
- Detect via PT_INTERP segment (ELF) and DT_NEEDED count
- Show: "No dynamic dependencies (statically linked)"
- No recipe metadata needed - infer from binary
- CGO-enabled Go handled by system lib patterns
- See `static-binary-handling.md`

### ~~3. `provides` Schema Design~~ ✅ RESOLVED
Moved to separate issue #969 (needs-design). Auto-discovery handles runtime; `provides` is optional for discoverability.

---

## Proposed Design Direction

Based on all research, the design should:

1. **Infer binary deps** from DT_NEEDED/LC_LOAD_DYLIB (already done in Tier 1)
2. **Detect static binaries** via PT_INTERP check, show "No dynamic dependencies (statically linked)"
3. **Validate PT_INTERP for ABI compatibility** - even before classifying deps, verify the dynamic linker exists:
   - Extract interpreter path from PT_INTERP segment (e.g., `/lib64/ld-linux-x86-64.so.2`)
   - If interpreter path exists but file doesn't exist → warn about ABI mismatch
   - This catches glibc binary on musl system (or vice versa) before any other validation
4. **Build soname index** from state.json at verification time
5. **Classify deps using THREE categories** (see `ruby-validation-example.md` and Key Learning #4):
   - **TSUKU-MANAGED**: Soname found in index → validate + recurse (or stop if externally-managed)
   - **PURE SYSTEM**: Library is inherently OS-provided (detected via pattern matching) → skip entirely
   - **UNKNOWN**: Neither in index nor matching system patterns → warn

   **Important:** PURE SYSTEM is defined by the library's inherent nature (OS-provided), not by the absence of a recipe. Pattern matching encodes our knowledge of what the OS provides.
6. **Compare binary deps to recipe declarations** and warn on mismatch
7. **Recursive validation via `--deep` flag** following recipe tree
8. **Stop recursion at externally-managed** recipes (detected via `IsExternallyManaged()` on actions)
9. **Warn, don't fail** for undeclared deps (graceful migration)

### Classification Order (CRITICAL)

The order of classification checks matters:

```
1. Is soname in our index? → YES → TSUKU-MANAGED (validate)
                          → NO  → Continue to step 2

2. Matches system pattern? → YES → PURE SYSTEM (skip)
                          → NO  → UNKNOWN (warn)
```

**Why this order?** A soname like `libssl.so.3` could theoretically match both (if we had a pattern for it). By checking the index first, we correctly identify it as tsuku-managed when we have an installed recipe providing it.

### Recursion Logic

For TSUKU-MANAGED deps, we decide whether to recurse:

```
if recipe.IsExternallyManagedFor(target):
    # e.g., openssl installed via apt_install
    validate(soname_provided)  # YES validate
    return                     # NO recurse (apt owns the rest)
else:
    # e.g., libyaml built from source
    validate(soname_provided)  # YES validate
    recurse(dep.dependencies)  # YES recurse (we own the tree)
```

### Example Output

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0)...

  Tier 1: Header validation
    bin/ruby: OK (ELF x86_64)

  Tier 2: Dependency validation
    Binary deps: libc.so.6, libm.so.6, libz.so.1, libyaml.so.0, libssl.so.3

    libc.so.6     → SYSTEM (glibc)
    libm.so.6     → SYSTEM (glibc)
    libz.so.1     → zlib ✓ (declared in recipe)
    libyaml.so.0  → libyaml ✓ (declared in recipe)
    libssl.so.3   → openssl ✓ (declared in recipe)

  Recursive validation (--deep):
    → zlib: OK
    → libyaml: OK
    → openssl: OK

ruby verified successfully
```

---

## Timeline

| Date | Event |
|------|-------|
| 2026-01-17 | Initial design created, PR #966 opened |
| 2026-01-17 | Rebased, updated based on recent PR review |
| 2026-01-17 | User raised scope questions (tools, recursion) |
| 2026-01-17 | Researched circular dependency handling |
| 2026-01-17 | Conducted 3-agent debate on proposal |
| 2026-01-17 | Analyzed burden distribution (80/15/5 split) |
| 2026-01-17 | Decided: auto-discovery for sonames, optional `provides` for discoverability |
| 2026-01-17 | Filed issue #969 for `provides` field (needs-design) |
| 2026-01-17 | Decided: `IsExternallyManaged()` on SystemAction interface |
| 2026-01-17 | Filed issue #970 for SystemAction refactoring (needs-design) |
| 2026-01-17 | Completed system library registry research (47 patterns) |
| 2026-01-17 | Completed static binary handling research |
| TBD | Resolve remaining pending decisions (3 items) |
| TBD | Update DESIGN-library-verify-deps.md |
| TBD | Design approved, status → Accepted |

---

## Files in wip/research/

| File | Purpose |
|------|---------|
| `tier2-design-tracker.md` | This file - tracks decisions and progress |
| `circular-dependency-handling.md` | Documents existing cycle detection |
| `system-dependency-recipes.md` | How tsuku handles system deps |
| `tool-binary-dependencies.md` | Analysis of tool binary deps |
| `recipe-binary-dep-mapping.md` | Soname → recipe mapping analysis |
| `current-verify-behavior.md` | Current verify command behavior |
| `debate-architecture.md` | Architecture agent analysis |
| `debate-ux.md` | UX agent analysis |
| `debate-skeptic.md` | Skeptic agent challenges |
| `soname-auto-discovery.md` | Auto-discovery design for sonames |
| `action-interface-pattern.md` | IsExternallyManaged() interface design |
| `system-library-registry.md` | System library patterns (47 total) |
| `static-binary-handling.md` | Static binary detection and UX |
| `recent_pr_review.md` | Review of recent library verification PRs |
| `ruby-validation-example.md` | Concrete walkthrough of three-category model |

## Related Issues

| Issue | Title | Status |
|-------|-------|--------|
| #948 | Tier 2 dependency resolution design | In progress (this design) |
| #969 | Optional `provides` field for discoverability | needs-design |
| #970 | Refactor SystemAction interface separation | needs-design |
