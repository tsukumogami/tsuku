# Crystallize Decision: tool-lifecycle-hooks

## Chosen Type
Design Doc

## Rationale
The exploration clearly established what to build: lifecycle hooks that enable
post-install shell integration, pre-uninstall cleanup, and pre-upgrade migration.
The core open question is architectural -- how to extend tsuku's action system,
compose per-tool shell init with shellenv, and track cleanup state. Multiple
viable implementation paths were identified (phase qualifier on WhenClause vs
separate [lifecycle] sections, declarative actions vs constrained scripts, shell.d
sourcing vs opt-in eval lines). Six architectural decisions were made during
exploration that need permanent documentation. A Design Doc captures these
decisions and evaluates the remaining trade-offs.

## Signal Evidence

### Signals Present
- **What to build is clear, but how to build it is not**: We know tools need
  lifecycle hooks. The open question is the implementation architecture -- recipe
  schema extensions, executor changes, shell composition model.
- **Technical decisions need to be made between approaches**: Phase qualifier on
  WhenClause vs separate recipe sections. Declarative actions (Level 1) vs
  constrained scripts (Level 2). Automatic shell.d sourcing vs opt-in eval.
- **Architecture, integration, or system design questions remain**: How hooks
  interact with the executor, how cleanup is tracked in state, how shellenv
  composes with per-tool init scripts, how remove/update flows gain lifecycle
  awareness.
- **Exploration surfaced multiple viable implementation paths**: Two approaches
  for recipe schema (phase qualifier vs lifecycle sections), three security levels
  (declarative, constrained, full), two composition models (shell.d vs opt-in).
- **Architectural decisions were made during exploration that need to be on record**:
  Six decisions captured in decisions.md -- declarative-first security model,
  shell.d composition, action system extension, post-install priority, state-tracked
  cleanup, graceful failure.
- **The core question is "how should we build this?"**: Requirements are clear
  from the tool survey (8-12 critical tools, 200+ that benefit). The design space
  is the open question.

### Anti-Signals Checked
- **What to build is still unclear**: Not present. The tool survey and niwa use
  case make requirements concrete.
- **No meaningful technical risk or trade-offs**: Not present. Security model,
  startup performance, cleanup reliability all involve real trade-offs.
- **Problem is operational, not architectural**: Not present. This is a new
  capability requiring schema, executor, and state management changes.

## Alternatives Considered
- **PRD**: Ranked lower (score 0, demoted). Requirements were given as input
  (the user stated the need; the tool survey confirmed it). The "what" is settled;
  a PRD would restate known requirements without advancing the design.
- **Plan**: Ranked lower (score -1, demoted). No upstream design doc exists for
  this topic. Technical approach has strong directional decisions but needs formal
  evaluation before decomposing into issues.
- **No Artifact**: Ranked lower (score -2, demoted). Six architectural decisions
  were made during exploration. These must be documented permanently before wip/
  is cleaned. Direct implementation without a design doc would lose the decision
  rationale.

## Deferred Types
- **Decision Record**: Some fit -- we chose between approaches. But the scope is
  broader than a single choice; multiple interconnected decisions compose into a
  system design. Design Doc is the right container.
