# Issue 1650 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-disambiguation.md`
- Sibling issues reviewed: #1648 (closed)
- Prior patterns identified:
  - `ConfirmFunc` in `internal/discover/llm_discovery.go` line 82: `type ConfirmFunc func(result *DiscoveryResult) bool`
  - `WithConfirmFunc` option pattern for injection
  - `DiscoveryMatch` struct already exists in `resolver.go` with required fields

## Gap Analysis

### Minor Gaps

1. **Existing struct overlap**: The issue asks for `ProbeMatch` with fields for builder name, source, downloads, version count, and repository presence. However, `DiscoveryMatch` already exists in `resolver.go` (lines 96-102) with these exact fields:
   ```go
   type DiscoveryMatch struct {
       Builder       string
       Source        string
       Downloads     int
       VersionCount  int
       HasRepository bool
   }
   ```
   The issue can either reuse `DiscoveryMatch` or create `ProbeMatch` as a distinct type. Both are valid; the design doc shows `ProbeMatch` in the Component 3 example code.

2. **Callback return type**: The issue specifies `func(matches []ProbeMatch) (int, error)` which returns an index. This differs from the existing `ConfirmFunc` pattern which returns `bool`. The difference is appropriate because disambiguation needs to select from multiple options (return index) versus simple yes/no confirmation.

3. **Integration point clarified by #1648**: The `disambiguate()` function in `disambiguate.go` currently returns `AmbiguousMatchError` for close matches. The callback will need to be invoked BEFORE returning that error, when there's no clear winner but interactive mode is available.

4. **toDiscoveryMatches helper exists**: The issue asks for `toProbeMatches([]probeOutcome) []ProbeMatch`. A similar helper `toDiscoveryMatches()` already exists at line 106-118 of `disambiguate.go`. If `ProbeMatch` mirrors `DiscoveryMatch`, this helper could be reused or renamed.

5. **EcosystemProbe struct location**: The callback field should be added to `EcosystemProbe` in `ecosystem_probe.go` (line 15-20), consistent with the design doc's Component 3 description.

### Moderate Gaps

None. The acceptance criteria align well with the existing patterns and the work done in #1648.

### Major Gaps

None. The implementation path is clear:
1. Define `ConfirmDisambiguationFunc` type and `ProbeMatch` struct in `resolver.go` or `ecosystem_probe.go`
2. Add `ConfirmDisambiguation` field to `EcosystemProbe` struct
3. Add `toProbeMatches()` helper (or rename existing helper)
4. Modify `disambiguate()` or `EcosystemProbe.Resolve()` to call the callback when matches are close
5. Write unit tests

## Recommendation

**Proceed**

The issue specification is complete and aligned with the current codebase state. The work done in #1648 provides the exact foundation needed:
- `disambiguate()` function is in place
- `AmbiguousMatchError` with `Matches` field exists
- `toDiscoveryMatches()` helper exists as a reference pattern

## Proposed Amendments

None required. The acceptance criteria are well-defined and compatible with the current implementation. One clarification for implementation:

- **Reuse vs. new type decision**: Either create `ProbeMatch` as specified (for semantic clarity that this is callback input) or reuse `DiscoveryMatch` (to avoid duplication). Both are acceptable; the issue specifies `ProbeMatch` so that should be followed.
