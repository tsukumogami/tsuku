---
status: Planned
problem: Test fixture for mixed issue and milestone diagram
decision: Test
rationale: Test
---
# Mixed Diagram

## Status

Planned

## Implementation Issues

### Milestone: [Test](https://github.com/org/repo/milestone/1)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [#5](https://github.com/org/repo/issues/5) | Issue task | None | 1 |
| [#6](https://github.com/org/repo/issues/6) | Another issue | #5 | 1 |
| [M20](https://github.com/org/repo/milestone/20) | Downstream milestone | #6 | milestone |

### Dependency Graph

```mermaid
graph TD
    I5["#5: Issue task"]
    I6["#6: Another issue"]
    M20["M20: Downstream milestone"]
    I5 --> I6
    I6 --> M20

    classDef done fill:#c8e6c9
    classDef ready fill:#bbdefb
    classDef blocked fill:#fff9c4
    classDef needsDesign fill:#e1bee7

    class I5 ready
    class I6 blocked
    class M20 blocked
```

**Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design
