# Crystallize Decision: update-warnings-notifications

## Chosen Type

Design Doc

## Rationale

What to build is clear (InboxReporter + taxonomy formalization + version fallback + success notices). How to build it still has open decisions: static vs. wildcard fallback insertion, how the InboxReporter accumulates per-tool notices, whether the interactive path uses fanoutReporter or writes notices separately, how Kind-based dispatch replaces the Error convention, and how EvalContext carries fallback info out-of-band. These are architectural decisions that need to be on permanent record — the wip/ files will be cleaned before merge and the decisions would be lost.

## Signal Evidence

### Signals Present

- **What to build is clear, how to build it is not**: The user arrived with a clear vision (InboxReporter, single notification API, two notice types). The research surfaced multiple implementation paths for version fallback (Decompose vs. checker.go), notice accumulation (per-Warn vs. flush-on-complete), and interactive-path persistence (fanoutReporter vs. separate write).
- **Technical decisions need to be made between approaches**: Decompose-time vs. checker-time fallback. PlanConfig.OnWarning vs. EvalContext field for out-of-band fallback signaling. Static vs. wildcard 404 handling. All real trade-offs with different complexity and correctness implications.
- **Architecture, integration, system design questions remain**: InboxReporter interface design, Notice schema extension (new Kind values, lifecycle rules per Kind), EvalContext modification for fallback info, fanoutReporter for interactive path.
- **Multiple viable implementation paths surfaced**: Version fallback location (2 options), fallback signal mechanism (2 options), notice accumulation strategy (2 options).
- **Architectural decisions made during exploration that should be on record**: Single-view lifecycle for fallback notices, InboxReporter as abstraction point, Kind as lifecycle routing key, Decompose as fallback insertion point. These decisions live only in wip/ and will be lost at branch close.
- **Core question is "how should we build this?"**: The user framed this from the start as a platform capability design question, not a requirements question.

### Anti-Signals Checked

- "What to build is still unclear": Not present. Requirements and goals are clearly established.
- "No meaningful technical risk or trade-offs": Not present. Real trade-offs identified across all sub-problems.
- "Problem is operational, not architectural": Not present. This is a cross-cutting architectural change.

## Alternatives Considered

- **PRD**: Ranked lower. Requirements were given as input to the exploration, not discovered by it. The PRD vs. Design Doc tiebreaker ("identified → PRD, given → Design Doc") applies clearly.
- **Plan**: Ranked lower with demotion. Anti-signal "Open architectural decisions need to be made first" is present — fallback insertion point and InboxReporter interface are not yet specified.
- **Decision Record**: Ranked lower with demotion. Anti-signal "Multiple interrelated decisions need a design doc" applies — the decisions made here are interdependent (Kind taxonomy, InboxReporter interface, fallback mechanism, notice schema) and need a cohesive document, not four separate decision records.
- **No Artifact**: Ranked lower with demotion. Anti-signal "Any architectural, dependency, or structural decisions were made during exploration" applies strongly — four explicit design decisions are recorded in the decisions file.
