---
summary:
  constraints:
    - 10x popularity threshold requires secondary signals (version count >= 3, has repository) for auto-select
    - Must handle missing popularity data by prompting, never auto-select on priority alone
    - Downloads can be gamed (~$50), so secondary signals add attack cost
    - AmbiguousMatchError type must have Tool and Matches fields for downstream formatting
  integration_points:
    - ecosystem_probe.go - integrate disambiguate() call for multiple matches
    - resolver.go - add AmbiguousMatchError type
    - probeOutcome or builders.ProbeResult - add VersionCount and HasRepository fields
  risks:
    - Download count comparability across ecosystems (npm weekly vs crates.io recent)
    - Priority-based fallback when popularity missing could enable ecosystem squatting
    - Need to verify probeOutcome struct has or can get version count and repository data
  approach_notes: |
    Create disambiguate.go with:
    1. rankProbeResults() - sort by downloads DESC, version count DESC, priority ASC
    2. isClearWinner() - check 10x gap + version count >= 3 + has repository
    3. disambiguate() - entry point called from ecosystem probe

    Extend probeOutcome with VersionCount and HasRepository fields.
    Add AmbiguousMatchError to resolver.go with minimal Error() message.

    Key behaviors:
    - Single match → auto-select
    - Clear winner (>10x + secondary signals) → auto-select with message
    - Close matches → prompt or error (handled by downstream issues)
    - Missing popularity data → never auto-select, must prompt/error
---

# Implementation Context: Issue #1648

**Source**: docs/designs/DESIGN-disambiguation.md (Planned)

## Design Excerpt

### Core Disambiguation Logic

The disambiguation system implements a three-tier approach:

1. **Single match**: Auto-select, no prompt needed
2. **Clear winner** (>10x downloads vs runner-up, plus secondary signals): Auto-select with informational message showing alternatives
3. **Close matches** (within 10x): Prompt user interactively to choose
4. **Missing popularity data**: Prompt user interactively (never auto-select on priority alone)
5. **Non-interactive**: Error with numbered list of options and `--from` suggestion

### Ranking Algorithm

Sort matches by:
- Downloads DESC
- Version count DESC
- Priority ASC (ecosystem priority used as tiebreaker)

### Clear Winner Detection

A match is a "clear winner" when:
- First match has >= 10x downloads of second match
- First match has version count >= 3
- First match has repository link

```go
func isClearWinner(first, second probeResult) bool {
    if first.Downloads == 0 || second.Downloads == 0 {
        return false // Can't determine without download data
    }
    if first.VersionCount < 3 {
        return false // Need version history
    }
    if !first.HasRepository {
        return false // Need source transparency
    }
    return first.Downloads >= second.Downloads * 10
}
```

### AmbiguousMatchError

Error type for non-interactive mode:
- `Tool string` - the requested tool name
- `Matches []probeResult` - ranked matches for formatting
- `Error() string` - minimal message (formatted suggestions in Issue #1652)

### Files to Create/Modify

**Create:**
- `internal/discover/disambiguate.go` - ranking and selection logic
- `internal/discover/disambiguate_test.go` - unit tests

**Modify:**
- `internal/discover/ecosystem_probe.go` - integrate disambiguation
- `internal/discover/resolver.go` - add AmbiguousMatchError type
