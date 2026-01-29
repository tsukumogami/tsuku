---
status: Planned
problem: Test fixture for invalid node naming
decision: Test
rationale: Test
---
# Invalid Node Diagram

## Status

Planned

## Implementation Issues

### Milestone: [Test](https://github.com/org/repo/milestone/1)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [#1](https://github.com/org/repo/issues/1) | Valid task | None | 1 |

### Dependency Graph

```mermaid
graph TD
    I1["#1: Valid task"]
    badNode["Invalid naming"]

    classDef done fill:#c8e6c9
    classDef ready fill:#bbdefb
    classDef blocked fill:#fff9c4
    classDef needsDesign fill:#e1bee7

    class I1 ready
    class badNode ready
```

**Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design
