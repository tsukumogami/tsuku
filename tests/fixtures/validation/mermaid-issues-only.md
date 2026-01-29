---
status: Planned
problem: Test fixture for issues-only diagram
decision: Test
rationale: Test
---
# Issues Only Diagram

## Status

Planned

## Implementation Issues

### Milestone: [Test](https://github.com/org/repo/milestone/1)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [#1](https://github.com/org/repo/issues/1) | First task | None | 1 |
| [#2](https://github.com/org/repo/issues/2) | Second task | #1 | 1 |

### Dependency Graph

```mermaid
graph TD
    I1["#1: First task"]
    I2["#2: Second task"]
    I1 --> I2

    classDef done fill:#c8e6c9
    classDef ready fill:#bbdefb
    classDef blocked fill:#fff9c4
    classDef needsDesign fill:#e1bee7

    class I1 ready
    class I2 blocked
```

**Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design
