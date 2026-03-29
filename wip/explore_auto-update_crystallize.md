# Crystallize Decision: auto-update

## Chosen Type
PRD

## Rationale
The user chose PRD to formally capture the requirements for auto-update before proceeding to design. While the seed description provided strong initial requirements, the exploration surfaced additional scope (channel-aware resolution, adjacent stories, notification levels) and boundary decisions that benefit from a structured requirements document. The PRD will serve as the contract for what the auto-update system must do, with the design doc following to address how.

## Signal Evidence
### Signals Present
- Single coherent feature emerged: auto-update is a unified capability covering self-update, tool update, pinning, caching, rollback, and notifications
- User stories or acceptance criteria are missing: the seed was directional; formal acceptance criteria, edge cases, and priority ordering haven't been captured

### Anti-Signals Checked
- Requirements were provided as input: present (the user gave detailed behavioral requirements in the seed), but the user chose PRD to formalize and extend them beyond the initial seed

## Alternatives Considered
- **Design Doc**: Scored highest (6/0) because the core open questions are architectural (how to build, not what to build). Expected to follow the PRD.
- **No Artifact**: Demoted because architectural decisions from exploration need permanent recording.
- **Plan**: Demoted because technical approach decisions are still open.

## User Override
The user selected PRD over the recommended Design Doc. This is a valid choice -- the PRD will formalize requirements before the design phase addresses architecture.
