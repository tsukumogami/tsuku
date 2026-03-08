# Crystallize Decision: unified-release-versioning

## Chosen Type
Design Doc

## Rationale
The exploration established clear requirements -- unify all artifacts under a single `v*` tag, enforce version lockstep between CLI, dltest, and llm, retire the separate llm release pipeline. But the implementation involves real architectural decisions across multiple subsystems: CI pipeline structure, artifact naming conventions, compile-time version pinning, gRPC protocol evolution, and recipe schema updates. Multiple viable approaches were identified for each dimension, with trade-offs that need explicit evaluation before implementation.

## Signal Evidence
### Signals Present
- **What to build is clear, how is not**: The goal (unified release with version lockstep) is unambiguous. The implementation spans CI workflows, Go code (version pinning), proto definitions (gRPC handshake), recipe configs, and GoReleaser config -- with options at each layer.
- **Technical decisions between approaches**: Artifact naming has 3 options (no version, version in all, keep mix). Version pinning has 2 phases (ldflags only vs ldflags + gRPC handshake). Pipeline consolidation has a phased approach vs big-bang merge.
- **Architecture/integration questions remain**: How version enforcement works across binaries with different invocation patterns (subprocess vs gRPC daemon). How GPU build dependencies affect pipeline structure. How artifact naming interacts with recipe resolution.
- **Multiple viable implementation paths**: Each research lead surfaced distinct options with documented trade-offs (e.g., Lead 6's naming options A/B/C, Lead 7's phase 1/phase 2 approach).
- **Core question is "how should we build this?"**: Requirements are settled. The exploration focused entirely on implementation architecture.

### Anti-Signals Checked
- **What to build is still unclear**: Not present -- the what is well-defined across all 8 leads
- **No meaningful technical risk or trade-offs**: Not present -- real trade-offs exist (e.g., breaking changes from naming standardization, proto evolution coordination)
- **Problem is operational, not architectural**: Not present -- this spans CI, Go code, proto, and recipe schema

## Alternatives Considered
- **PRD**: Scored 0 (demoted). Requirements are already clear and agreed on (anti-signal). The exploration didn't surface requirement gaps -- it surfaced implementation questions.
- **Plan**: Scored 0 (demoted). No source artifact exists to decompose (anti-signal). The implementation sequence needs architectural decisions captured first.
- **No artifact**: Scored -1 (demoted). This is complex multi-component work spanning CI pipelines, recipe schema, proto definitions, and Go code across 5 implementation phases. Not suitable for direct implementation without documenting the technical decisions.
