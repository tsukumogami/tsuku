# Phase Review: DESIGN-structured-install-guide.md vs DESIGN-system-dependency-actions.md

## Summary

This assessment reviews the implementation phases across two related design documents to determine phase alignment, issue #722 coverage, dependency ordering, and consolidation recommendations.

## Documents Under Review

| Document | Status | Focus |
|----------|--------|-------|
| `DESIGN-structured-install-guide.md` | Proposed | Container building, sandbox execution, caching, schema format |
| `DESIGN-system-dependency-actions.md` | Proposed | Action vocabulary, platform filtering, distro detection |

## 1. Phase Alignment

### DESIGN-structured-install-guide.md Phases

| Phase | Description |
|-------|-------------|
| Phase 1 | Refactor require_system Action (remove install_guide, add packages/primitives) |
| Phase 2 | Primitive Framework (Primitive interface, core primitives, Describe()) |
| Phase 3 | Sandbox Execution (minimal base container, image caching, executor integration) |
| Phase 4 | User Consent and Host Execution |
| Phase 5 | Extension (additional primitives, container stripping) |

### DESIGN-system-dependency-actions.md Phases

| Phase | Description |
|-------|-------------|
| Phase 1 | Infrastructure (distro detection, WhenClause extension) |
| Phase 2 | Action Vocabulary (typed actions with Describe(), require_command extraction) |
| Phase 3 | Documentation Generation (Describe() for all actions, CLI display) |
| Phase 4 | Sandbox Integration (ExtractPackages, container building) |

### Alignment Analysis

The phases have **overlapping but distinct concerns**:

| Concern | Structured Guide | System Deps | Assessment |
|---------|-----------------|-------------|------------|
| Distro detection | Not covered | Phase 1 | **Gap in structured guide** |
| Action types | Polymorphic (packages/primitives in require_system) | Typed (apt_install, brew_cask, etc.) | **Design conflict** |
| Describe() method | Phase 2 | Phase 2-3 | Aligned |
| Container building | Phase 3 | Phase 4 | Aligned |
| Host execution | Phase 4 | Future Work | **Structured guide is premature** |

### Key Conflict: Action Granularity

**DESIGN-structured-install-guide.md** uses a polymorphic `require_system` action with `packages` or `primitives` parameters:

```toml
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }
```

**DESIGN-system-dependency-actions.md** uses typed actions per operation:

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }
```

These are **mutually exclusive** approaches. The system-dependency-actions design explicitly rejects the structured-guide approach in D1 (Action Granularity):

> "Option C (unified action with manager field): Recreates the original problem of generic containers with platform-specific content"

## 2. Issue #722 Coverage

### Acceptance Criteria from DESIGN-golden-plan-testing.md (lines 1505-1512)

| Criterion | Structured Guide | System Deps | Notes |
|-----------|-----------------|-------------|-------|
| Design for structured install_guide format | Yes | Yes | Both address |
| require_system supports structured specs | Yes | Yes (via typed actions) | Different approaches |
| Minimal base container (tsuku + glibc) | Yes (Phase 3) | References structured guide | Same container strategy |
| Sandbox executor builds custom containers | Yes (Phase 3) | Phase 4 (ExtractPackages) | Aligned |
| All recipes have structured install_guide | Yes | Yes | Both require migration |
| Golden plans for ALL recipes | Enables | Enables | Both enable |
| Execution validation for require_system | Yes (Phase 3) | Phase 4 | Both enable |

### What's Missing for Sandbox Testing?

1. **Distro detection** - Structured guide assumes `when = { os = ["linux"] }` is sufficient, but apt doesn't exist on all Linux distros. System-deps design adds `distro` filtering.

2. **Phase ordering** - Structured guide jumps to sandbox execution in Phase 3 without establishing distro detection infrastructure.

3. **Action vocabulary resolution** - The designs disagree on whether to use polymorphic `require_system` or typed actions.

## 3. Dependency Order

### Critical Path Analysis

```
DESIGN-system-dependency-actions.md
  Phase 1: Distro Detection (WhenClause.Distro, /etc/os-release parsing)
      |
      v
  Phase 2: Action Vocabulary (apt_install, brew_cask, etc. with Describe())
      |
      v
DESIGN-structured-install-guide.md
  Phase 2: Primitive Framework (if kept) OR adoption of typed actions
      |
      v
  Phase 3: Sandbox Execution (minimal container, ExtractPackages)
      |
      v
DESIGN-system-dependency-actions.md
  Phase 4: Sandbox Integration (container building from ExtractPackages)
```

### Dependencies Identified

1. **Distro detection MUST come first** - Without it, `apt_install` steps cannot be filtered correctly (apt exists on Debian/Ubuntu but not Fedora/Arch).

2. **Action vocabulary MUST be resolved before container building** - The sandbox executor needs to know how to extract package requirements from steps. The format differs:
   - Structured guide: `step.Params["packages"]` contains `{ apt: [...] }`
   - System deps: `step.Action == "apt_install"` and `step.Packages` directly

3. **No circular dependencies** - The designs can be ordered linearly.

### Recommended Build Order

1. Distro detection infrastructure (system-deps Phase 1)
2. Action vocabulary decision (resolve conflict)
3. Implement chosen action model with Describe()
4. Minimal base container + sandbox integration
5. Host execution (future, separate design)

## 4. Consolidation Recommendation

### Recommendation: Consolidate into Single Design

**Rationale:**

1. **Design conflict must be resolved** - Cannot implement both approaches. A single design forces the decision.

2. **Shared infrastructure** - Both designs need:
   - `Describe()` method on actions/primitives
   - Sandbox container building
   - Content-addressed external resources
   - Migration of existing recipes

3. **Overlapping phases** - The current split creates confusion about execution order.

4. **Upstream reference** - Issue #722 (per DESIGN-golden-plan-testing.md) expects a single blocking issue/design for "structured install_guide format."

### Consolidation Structure

If consolidated, the design should follow DESIGN-system-dependency-actions.md structure (typed actions) with container building details from DESIGN-structured-install-guide.md:

| Section | Source |
|---------|--------|
| Problem statement | Either (similar) |
| Distro detection | system-dependency-actions |
| Action vocabulary (typed) | system-dependency-actions |
| Describe() documentation | system-dependency-actions |
| Minimal container strategy | structured-install-guide |
| Container caching | structured-install-guide |
| Content-addressing | structured-install-guide |
| Host execution | Defer to future design |

### Alternative: Keep Two Designs with Clear Boundaries

If keeping two designs, clarify responsibilities:

| Design | Responsibility |
|--------|----------------|
| DESIGN-system-dependency-actions.md | **What**: Action types, platform filtering, documentation generation |
| DESIGN-structured-install-guide.md | **How**: Container building, caching, sandbox executor integration |

**Changes required:**
1. Structured guide adopts typed actions from system-deps design (remove polymorphic require_system)
2. Structured guide removes host execution phases (deferred to separate design)
3. System-deps design adds explicit reference to structured guide for container building
4. Both designs share same phase numbering or explicitly declare dependencies

## 5. Specific Recommendations

### For Issue #722 Acceptance

1. **Resolve action granularity conflict NOW** - Pick typed actions (system-deps) or polymorphic require_system (structured guide). The typed approach is cleaner and already vetted through agent research.

2. **Add distro detection to critical path** - Structured guide Phase 1 should be distro detection, not require_system refactoring.

3. **Defer host execution** - Both designs acknowledge this is future work. Remove Phase 4 (User Consent and Host Execution) from structured-install-guide.md to keep scope focused.

4. **Update phase references** - System-deps Phase 4 says "See DESIGN-structured-install-guide.md for container building details" but structured guide uses different action model. This creates confusion.

### Implementation Sequence

If proceeding with current designs, implement in this order:

1. **Merge/reconcile designs** - Resolve action granularity conflict
2. **system-deps Phase 1** - Distro detection
3. **system-deps Phase 2** - Action vocabulary with Describe()
4. **system-deps Phase 3** - Documentation generation
5. **structured-guide Phase 3** (adapted) - Sandbox execution with typed actions
6. **Recipe migration** - Convert docker.toml, cuda.toml, test-tuples.toml

## Conclusion

The two designs are **complementary in intent but conflicting in implementation**. The DESIGN-system-dependency-actions.md approach (typed actions per operation) is more consistent with tsuku's design principles and was reached through systematic agent research. The DESIGN-structured-install-guide.md container building and caching strategy is sound but should adopt the typed action vocabulary.

**Primary recommendation**: Consolidate into a single design using:
- Typed action vocabulary from DESIGN-system-dependency-actions.md
- Container building/caching details from DESIGN-structured-install-guide.md
- Distro detection as Phase 1
- Host execution deferred to future design

This consolidation directly serves issue #722's goal of enabling sandbox testing for recipes with system dependencies.
