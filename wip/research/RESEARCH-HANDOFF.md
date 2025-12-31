# Research Handoff: Linux Platform Targeting Model

## Context

Tsuku is designing a system dependency model that extends platform targeting from `(os, arch)` to include Linux distribution awareness. Initial design proposed `target = (platform, distro)`, but review revealed this oversimplifies the Linux ecosystem.

### The Problem

The Linux ecosystem is not flat. It has hierarchy:
- **Family** (debian, rhel, arch, independent)
- **Distro** (ubuntu, fedora, manjaro)
- **Version** (22.04, 40, rolling)

Additionally:
- Binary compatibility varies (glibc vs musl)
- Package managers have different models (imperative vs declarative)
- Some distros (NixOS, Gentoo) are fundamentally different

### Design Questions to Answer

1. **Hierarchy Model**: Is `(platform, family, distro, version)` the right model, or is there a simpler abstraction?
2. **Binary Strategy**: Can tsuku ship one binary per architecture, or do we need glibc/musl variants?
3. **Support Tiers**: Which distros are Tier 1 (tested), Tier 2 (supported), Tier 3 (unsupported)?
4. **Edge Cases**: Should NixOS/Gentoo/Alpine be explicitly unsupported, or handled differently?
5. **Filter Dimensions**: Is `package_manager` a better filter than `distro` for system dependency actions?

## Research Phases

### Phase 1: Foundation (Parallel)

| Track | Focus | Spec |
|-------|-------|------|
| **P1-A** | Prior Art Deep Dive | [SPEC-prior-art.md](./SPEC-prior-art.md) |
| **P1-B** | GitHub Binary Survey | [SPEC-binary-survey.md](./SPEC-binary-survey.md) |
| **P1-C** | Package Manager Inventory | [SPEC-package-managers.md](./SPEC-package-managers.md) |
| **P1-D** | Ecosystem Analysis | [SPEC-ecosystem-analysis.md](./SPEC-ecosystem-analysis.md) |

### Phase 2: Core Model (Sequential, depends on Phase 1)

- Family taxonomy and hierarchy decision
- C library and static binary deep analysis
- Imperative vs declarative package manager classification
- ID_LIKE chain empirical analysis

### Phase 3: Implementation (Depends on Phase 2)

- Tsuku action model compatibility audit
- Container base image strategy
- Golden file and CI matrix design
- Detection and fallback algorithms

## Reference Documents

Detailed investigation paths by domain:

| Domain | Document | Paths |
|--------|----------|-------|
| Linux Hierarchy | [investigation_linux-hierarchy.md](./investigation_linux-hierarchy.md) | 7 paths |
| Binary Compatibility | [investigation_binary-compatibility.md](./investigation_binary-compatibility.md) | 7 paths |
| Package Ecosystems | [investigation_package-ecosystems.md](./investigation_package-ecosystems.md) | 10 paths |
| Tsuku Action Model | [investigation_action-model.md](./investigation_action-model.md) | 10 paths |
| Real-World Fragmentation | [investigation_fragmentation.md](./investigation_fragmentation.md) | 8 paths |

## Previous Design Artifacts

Context from earlier design work:

| Document | Purpose |
|----------|---------|
| [../docs/DESIGN-system-dependency-actions.md](../../docs/DESIGN-system-dependency-actions.md) | Typed action vocabulary |
| [../docs/DESIGN-structured-install-guide.md](../../docs/DESIGN-structured-install-guide.md) | Sandbox container building |
| [plan-outline.md](../plan-outline.md) | Implementation issue plan (21 issues) |

## Design Decisions Already Made

These decisions were made before research was scoped, and may be revisited:

| Decision | Choice | Confidence |
|----------|--------|------------|
| ID_LIKE matching | Explicit (recipe lists all distros) | Medium - needs validation |
| Sandbox default | Canonical (Ubuntu LTS) | High |
| Distro dimension | Conditional (only when recipe needs it) | Medium - hierarchy may change this |
| Plan field | `target_distro` presence implies distro-specific | High |

## Success Criteria

Phase 1 research is complete when:

1. We understand how 5+ prior art tools solved this problem
2. We have empirical data on what binaries popular tools ship
3. We have a complete package manager inventory with distro mapping
4. We know CI provider and container ecosystem defaults
5. We can make an informed decision on hierarchy model

## Timeline

- Phase 1: Parallel execution, time-boxed to focused research
- Phase 2: Begins after Phase 1 synthesis
- Phase 3: Begins after model decisions are locked

## Handoff Notes

- Each Phase 1 spec is self-contained with clear deliverables
- Researchers should write findings to `wip/research/` with consistent naming
- Findings should be factual; recommendations come in synthesis phase
- Flag any discoveries that invalidate earlier assumptions
