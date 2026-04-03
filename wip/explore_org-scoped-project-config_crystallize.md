# Crystallize Decision: org-scoped-project-config

## Chosen Type

Design Doc

## Rationale

The exploration established clear requirements (issue #2230 defines exactly what should work) but surfaced multiple viable implementation paths for TOML syntax, resolver integration, and auto-registration behavior. The core question is architectural: how should project install integrate with the distributed provider system, and how should the resolver handle org-prefixed config keys? Decisions made during exploration (eliminating dotted keys, value-side encoding, explicit registries) need a permanent record.

## Signal Evidence

### Signals Present

- **What to build is clear, how is not**: Issue #2230 fully defines the bug. The open question is the implementation approach -- syntax choice, resolver changes, auto-registration flow.
- **Technical decisions between approaches**: Quoted keys vs inline tables vs separate registries. Auto-registration vs explicit declaration. Resolver prefix-stripping vs dual-key lookup.
- **Architecture/integration questions remain**: How `runProjectInstall` integrates with `ensureDistributedSource`, how the resolver handles the key mismatch between binary index and config.
- **Multiple viable implementation paths**: 7 TOML syntax options evaluated, 3 found practical. Multiple resolver strategies possible.
- **Decisions made during exploration**: Eliminated dotted keys, array-of-tables, value-side encoding, explicit `[registries]` section -- these need permanent documentation.
- **Core question is "how should we build this?"**: Yes.

### Anti-Signals Checked

- **What to build is still unclear**: Not present. Requirements are well-defined by the issue.
- **No meaningful technical risk**: Not present. Resolver mismatch and lazy provider loading are real risks.
- **Problem is operational**: Not present. It's architectural.

## Alternatives Considered

- **No Artifact**: One person could implement this, and it was a short exploration. But decisions about syntax elimination and architectural approach need a permanent record. Demoted by anti-signal.
- **PRD**: Requirements were given as input (issue #2230), not discovered during exploration. Anti-signal present.
- **Plan**: No upstream design doc exists yet. Technical approach still has open decisions. Two anti-signals.
