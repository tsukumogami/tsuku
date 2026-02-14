---
summary:
  constraints:
    - Levenshtein distance threshold must be exactly 2 or less (research-backed)
    - Check only against registry entries (~500 tools), not all ecosystem packages
    - Exact matches (distance 0) must NOT trigger warnings
    - Returns warning, does not block installation
  integration_points:
    - internal/discover/typosquat.go (new file)
    - internal/discover/typosquat_test.go (new file)
    - internal/discover/chain.go (call CheckTyposquat before probe stage)
    - DiscoveryRegistry type for registry entry access
  risks:
    - False positives for short package names (go, rg, fd have many neighbors)
    - Performance if registry grows beyond ~500 entries
    - TyposquatWarning struct must match expected fields in issue
  approach_notes: |
    Create typosquat.go with Levenshtein distance implementation and
    CheckTyposquat function. The function signature is specified in issue:

    func CheckTyposquat(toolName string, registry *DiscoveryRegistry) *TyposquatWarning

    Integration happens in ChainResolver.Resolve() before iterating stages.
    This is Phase 2 of the disambiguation design - a separate concern from
    ecosystem disambiguation that runs before any probe stages.
---

# Implementation Context: Issue #1649

**Source**: docs/designs/DESIGN-disambiguation.md (Decision 2: Typosquatting Detection)

## Design Excerpt

### Decision 2: Typosquatting Detection

When a user requests `rgiprep`, should the system warn that this might be a typo for `ripgrep`? The challenge is balancing security (catching typosquats) with usability (not flooding users with false positives).

#### Chosen: Edit Distance Against Registry Entries

Check requested tool names against the discovery registry entries (top ~500 tools) using Levenshtein distance. If distance ≤2, display a warning:

```
Warning: "rgiprep" is similar to "ripgrep" (distance 1)
  ripgrep: crates.io, 1.2M downloads/month
  rgiprep: npm, 23 downloads/month
Continue with "rgiprep"? [y/N]
```

This runs before disambiguation—it's a separate concern that applies even to single matches.

**Why Levenshtein over Damerau-Levenshtein**: Simpler implementation, and the transposition advantage matters less for short package names. Both catch the same common typos at distance ≤2.

**Why distance ≤2**: Research shows this catches 45% of real typosquats while keeping false positives low. Distance 1 misses too many (bta vs bat), distance 3+ has too many false positives.

**Why registry entries only**: Checking against all ecosystem packages would be too slow. The registry contains the most popular tools—exactly the ones typosquatters target.

### Component 2: Typosquatting Detector (from Solution Architecture)

Add `typosquat.go` with edit distance checking:

```go
// CheckTyposquat compares a tool name against registry entries.
// Returns a warning if the name is suspiciously similar (distance ≤2).
func CheckTyposquat(toolName string, registry *Registry) *TyposquatWarning {
    for _, entry := range registry.Entries() {
        dist := levenshtein(toolName, entry.Name)
        if dist > 0 && dist <= 2 {
            return &TyposquatWarning{
                Requested:     toolName,
                Similar:       entry.Name,
                Distance:      dist,
                SimilarSource: entry.Source,
            }
        }
    }
    return nil
}
```

Integration point: Call before ecosystem probe in the chain resolver or at the start of disambiguation.

### Implementation Phase 2: Typosquatting Detection

**Files to create:**
- `internal/discover/typosquat.go` - Edit distance checking
- `internal/discover/typosquat_test.go` - Unit tests

**Files to modify:**
- `internal/discover/chain.go` - Add typosquat check before probe

**Deliverable:** Warning displayed for suspiciously similar names.
