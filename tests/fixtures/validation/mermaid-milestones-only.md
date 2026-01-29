---
status: Planned
problem: Test fixture for milestones-only diagram
decision: Test
rationale: Test
---
# Milestones Only Diagram

## Status

Planned

## Implementation Issues

### Milestone: [Test](https://github.com/org/repo/milestone/1)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [M10](https://github.com/org/repo/milestone/10) | First milestone | None | milestone |
| [M11](https://github.com/org/repo/milestone/11) | Second milestone | M10 | milestone |

### Dependency Graph

```mermaid
graph TD
    M10["M10: First milestone"]
    M11["M11: Second milestone"]
    M10 --> M11

    classDef done fill:#c8e6c9
    classDef ready fill:#bbdefb
    classDef blocked fill:#fff9c4
    classDef needsDesign fill:#e1bee7

    class M10 ready
    class M11 blocked
```

**Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design
