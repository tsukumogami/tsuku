# Design Documents

This directory contains design documents for tsuku features and architectural decisions.

## Directory Structure

| Directory | Status | Description |
|-----------|--------|-------------|
| `docs/designs/` | Proposed, Accepted, Planned | Active designs under discussion or awaiting implementation |
| `docs/designs/current/` | Current | Implemented designs representing current architecture |
| `docs/designs/archive/` | Superseded | Replaced designs with links to their successors |

## Status Lifecycle

Design documents progress through the following statuses:

| Status | Description |
|--------|-------------|
| **Proposed** | Design under discussion, not yet approved |
| **Accepted** | Design approved, awaiting implementation |
| **Planned** | Design approved, issues and milestones created |
| **Current** | Implementation complete, stable |
| **Superseded** | Replaced by another design (includes link to replacement) |

## Creating New Designs

1. Create a new file in `docs/designs/` with status "Proposed"
2. Follow the design document template (title, status, context, decision, consequences)
3. After approval, update status to "Accepted" or "Planned"
4. When implementation is complete, move to `current/` with status "Current"
5. If superseded by a new design, move to `archive/` with status "Superseded by [link]"
