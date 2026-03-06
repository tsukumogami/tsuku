# Crystallize Decision: registry-versioning

## Chosen Type
Design Doc

## Rationale
The exploration established clear requirements -- the user defined the exact migration lifecycle (new CLI supports both formats, registry pre-announces migration, old CLIs warn, registry migrates, old CLIs error). The open questions are architectural: exact manifest field changes, CLI behavior at each state, cache interaction with version-incompatible manifests, and multi-registry deprecation tracking. This maps directly to "what to build is clear, how to build it is not."

## Signal Evidence
### Signals Present
- What to build is clear, but how to build it is not: The user articulated the lifecycle sequence; the research surfaced multiple approaches for implementation (integer vs semver, per-recipe vs manifest-only, inline vs separate endpoint)
- Technical decisions between approaches: Integer vs semver schema version, where version checks are inserted, how stale-if-error interacts with version incompatibility
- Architecture questions remain: Cache invalidation on version mismatch, multi-registry deprecation tracking, CLI version comparison infrastructure
- Multiple viable implementation paths: Discovery registry pattern vs new approach, inline deprecation vs separate endpoint
- Core question is "how should we build this?": Yes -- the migration protocol is defined, the technical design is not

### Anti-Signals Checked
- What to build is still unclear: Not present. Requirements are well-defined.
- No meaningful technical risk: Not present. Cache interaction and multi-registry behavior have real trade-offs.
- Problem is operational, not architectural: Not present. This is a protocol design problem.

## Alternatives Considered
- **PRD**: Ranked lower (score 1, demoted by anti-signal). Requirements are already clear and agreed. No stakeholder alignment needed.
- **Plan**: Ranked lower (score 0, demoted). No source artifact exists to decompose.
- **No artifact**: Ranked lower (score 0, demoted). Design decisions remain open; direct implementation risks wrong choices.
