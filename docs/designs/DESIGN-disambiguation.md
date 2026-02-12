---
status: Proposed
problem: |
  When multiple ecosystem registries return matches for a tool name (e.g., "bat" exists
  on crates.io as sharkdp/bat and on npm as bat-cli), the ecosystem probe stage silently
  picks the highest-priority registry without user awareness. This creates security risk
  because users may install the wrong package, and usability problems because there's no
  way to select an alternative match or understand why a particular source was chosen.
decision: |
  Implement a three-tier disambiguation system: auto-select when one match dominates
  (>10x downloads), prompt interactively when matches are close, and error with
  suggestions in non-interactive mode. Add typosquatting detection via edit distance
  (threshold ≤2) against registry entries to warn about suspiciously similar names.
  Reuse the existing confirmation UX patterns from LLM discovery for consistency.
rationale: |
  The 10x popularity threshold balances automation with safety—clear winners don't need
  confirmation, but close calls require human judgment. Edit-distance checking addresses
  typosquatting at the point of discovery rather than relying solely on registry-level
  filtering. Reusing LLM discovery's confirmation patterns ensures consistent UX and
  avoids creating parallel systems.
---

# DESIGN: Disambiguation for Multiple Ecosystem Matches

## Status

Proposed

## Upstream Design Reference

This design implements the "Disambiguation Strategy" section of [DESIGN-discovery-resolver.md](DESIGN-discovery-resolver.md). It addresses the design questions raised in [issue #1321](https://github.com/tsukumogami/tsuku/issues/1321).

## Context and Problem Statement

The ecosystem probe stage queries seven package registries in parallel (Homebrew, crates.io, PyPI, npm, RubyGems, Go, CPAN) and returns matches that pass quality filtering. When multiple registries have packages with the same name, the current implementation silently selects the highest-priority registry (cask > homebrew > crates.io > ...) without user awareness.

**Current user experience**: When a user runs `tsuku install bat`, the ecosystem probe finds matches on crates.io (sharkdp/bat) and npm (bat-cli). Today, the system silently selects crates.io based on a hardcoded priority list. The user sees only "Installing bat from crates.io..." with no indication that npm was also an option.

This creates two problems:

1. **Security risk**: Users may unknowingly install a different package than intended. "bat" on npm (bat-cli, a testing framework) is not the same as "bat" on crates.io (sharkdp/bat, a cat replacement). Installing the wrong one could be a security issue if the user trusts one maintainer but not another.

2. **Usability problem**: When the automatic selection is wrong, users have no recourse except to know about and use the `--from` flag. There's no indication that alternatives exist or why a particular source was chosen.

**Collision frequency**: Analysis of the top 500 tools in the discovery registry shows approximately 15-20% have matches on multiple ecosystems. Most are clear winners (one ecosystem has 100x+ more downloads), but roughly 3-5% have close matches that benefit from user input.

The batch recipe generation pipeline faces a related challenge: when processing tools at scale, it needs deterministic disambiguation without human interaction, but must track which tools had ambiguous matches for later review.

### Scope

**In scope:**
- Ranking algorithm for multiple ecosystem matches
- Interactive disambiguation prompt for close matches
- Non-interactive mode behavior (auto-select or error)
- Typosquatting detection via edit distance
- Integration with existing confirmation UX patterns
- Batch pipeline disambiguation tracking

**Out of scope:**
- Changes to ecosystem probe query logic (what registries to query)
- Changes to quality filter thresholds (how to filter results)
- New ecosystem registry integrations
- LLM-based disambiguation (already handled by LLM discovery stage)

## Decision Drivers

- **Security-sensitive operation**: Installing the wrong package could execute untrusted code
- **Reuse existing patterns**: LLM discovery already has confirmation UX, ranking, and metadata display
- **Batch pipeline compatibility**: Must support deterministic selection for automated recipe generation
- **CLI usability**: Clear feedback about what's being installed and why
- **Typosquatting defense**: Detect suspiciously similar names to registry entries
- **Ecosystem data limitations**: Not all registries expose download counts

## External Research

### Prior Art: Package Manager Disambiguation

| Package Manager | Approach | User Experience |
|-----------------|----------|-----------------|
| **Homebrew** | Explicit flags (`--formula`/`--cask`) | User specifies type when ambiguous |
| **Cargo** | Explicit flags (`-p`, `--bin`) | User specifies which binary/package |
| **npm** | Prevention at registry | Blocks similar names at publish time |
| **pip** | No disambiguation | Developer manages manually via sys.path |

**Key insight**: Different package managers make fundamentally different trade-offs. Homebrew and Cargo push disambiguation to users. npm prevents collisions upfront. tsuku's approach should balance automation (clear winners) with user control (ambiguous cases).

### Edit Distance for Typosquatting Detection

Research on typosquatting in package managers shows:
- Levenshtein distance ≤2 catches 45% of historical typosquats
- Damerau-Levenshtein (adding transpositions) is better for common typos
- SpellBound (npm research) achieves 99.4% detection with 0.5% false positives by combining edit distance with popularity anomalies

**Recommendation**: Use Levenshtein distance ≤2 against the top ~500 registry entries. Flag matches but don't block—display a warning with both names and download counts.

### Popularity Data Availability

| Registry | Downloads Available | Alternative Signal |
|----------|--------------------|--------------------|
| npm | Weekly downloads | Version count |
| crates.io | Recent downloads | Version count |
| PyPI | No (API limitation) | Version count |
| RubyGems | Daily downloads | Version count |
| CPAN | river.total (deps) | Version count |
| Go | No | Version count |
| Homebrew | No | Curated (trusted) |

**Key insight**: Popularity data isn't universally available. The system must fall back to version count or static priority when downloads are missing.

## Considered Options

### Decision 1: Disambiguation Algorithm

The core question is how to decide which ecosystem match to select when multiple registries return results for the same tool name.

The algorithm must handle three scenarios: (1) one match clearly dominates, (2) matches are close and need user input, (3) non-interactive mode where prompting isn't possible. It must also work with incomplete data—not all registries provide download counts.

#### Chosen: Popularity-Based Auto-Select with Interactive Fallback

Implement a three-tier approach:

1. **Single match**: Auto-select, no prompt needed
2. **Clear winner** (>10x downloads vs runner-up, plus secondary signals): Auto-select with informational message showing alternatives. Secondary signals required: version count ≥3 AND has repository link.
3. **Close matches** (within 10x): Prompt user interactively to choose
4. **Missing popularity data**: Prompt user interactively (never auto-select on priority alone)
5. **Non-interactive**: Error with numbered list of options and `--from` suggestion

When popularity data is unavailable, do NOT fall back to priority-based auto-selection—prompt instead. Priority ordering is only used for ranking within prompts, not for automatic selection. This prevents attackers from squatting in higher-priority ecosystems.

**Why 10x threshold with secondary signals**: The 10x gap alone is insufficient—download counts can be gamed for ~$50. Requiring version count ≥3 and repository presence adds cost to attacks. A package must demonstrate sustained activity (versions) and transparency (source code) to auto-select.

**Why prompt when popularity data missing**: Priority-based auto-selection without popularity data allows ecosystem-squatting attacks. If "foo" exists on crates.io (popular) and an attacker registers "foo" on Homebrew (higher priority), priority fallback would auto-select the attacker's package. Prompting ensures human review when we lack objective popularity signals.

#### Alternatives Considered

**Always prompt for multiple matches**: Require user confirmation even when one match has 100x more downloads.
This is the most secure option—users explicitly approve every cross-ecosystem selection. Rejected because it creates unnecessary friction for the majority of cases where one ecosystem clearly dominates. With ~15% of tools having multiple matches but only ~3-5% having close matches, always prompting would slow down the common case for marginal security benefit. The 10x threshold ensures user input where it matters most.

**Use LLM to disambiguate**: Send multiple matches to the LLM for analysis.
Rejected because LLM discovery is already the fallback when ecosystem probe fails. Adding LLM to ecosystem probe would add latency and cost. The deterministic algorithm is sufficient for registry data.

**Require explicit `--from` for all ambiguous tools**: Error immediately when multiple matches exist.
Rejected because it's too disruptive. Many tools exist on multiple registries but have a clear winner. Requiring explicit source selection would frustrate users.

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

#### Alternatives Considered

**Block instead of warn**: Refuse to install anything within edit distance 2 of a popular tool.
Rejected because legitimate tools may have similar names. Blocking creates false positives that frustrate users. Warning with comparison lets users make informed decisions.

**Phonetic similarity (Soundex/Metaphone)**: Check if names sound alike.
Rejected because package names are typed, not spoken. Edit distance better matches actual user errors (typos) than phonetic similarity.

**Check all ecosystem packages**: Compare against everything in the quality filter.
Rejected because it would add ~50-100ms latency (comparing against thousands of packages at ~50μs each) and would catch too many false positives. Focusing on registry entries (~500 packages, <5ms) targets the high-value packages that typosquatters actually target.

### Decision 3: Interactive Prompt Design

When matches are close and user input is needed, what information should be displayed? The prompt must help users make informed decisions without overwhelming them with data.

#### Chosen: Ranked List with Key Metadata

Display a numbered list sorted by popularity, showing:
- Registry source (crates.io, npm, etc.)
- Download count (if available) or version count
- Whether it has a linked repository
- Ecosystem priority rank

```
Multiple sources found for "bat":

  1. crates.io: sharkdp/bat
     Downloads: 45K/day | Versions: 87 | Has repository

  2. npm: bat-cli
     Downloads: 200/day | Versions: 12 | Has repository

  3. rubygems: bat
     Downloads: 50/day | Versions: 3 | No repository

Select source [1-3, or 'q' to cancel]:
```

In non-interactive mode (piped stdin or `--yes` with ambiguous matches), print the same list as an error and suggest `--from`:

```
Error: Multiple sources found for "bat". Use --from to specify:
  tsuku install bat --from crates.io:sharkdp/bat
  tsuku install bat --from npm:bat-cli
  tsuku install bat --from rubygems:bat
```

#### Alternatives Considered

**Simple y/N confirmation for top match**: Ask "Install bat from crates.io? [y/N]" without showing alternatives.
Rejected because users can't make informed decisions without seeing what alternatives exist. The whole point of disambiguation is presenting choices.

**JSON output for machine parsing**: Output structured data for scripting.
Rejected as the primary interface, but could be added via `--json` flag in the future. The human-readable format is the priority for this design.

**Show full metadata (stars, age, owner)**: Display the same rich metadata as LLM discovery confirmation.
Rejected because ecosystem probe doesn't fetch GitHub metadata—that would require additional API calls. Keep it simple with what's available from registry APIs.

### Decision 4: Batch Pipeline Integration

The batch recipe generation pipeline processes tools without human interaction. How should it handle disambiguation?

#### Chosen: Deterministic Selection with Tracking

In batch mode, always select deterministically using popularity ranking with priority fallback. Track ambiguous selections in the batch metrics for later human review:

```json
{
  "tool": "bat",
  "selected": "crates.io:sharkdp/bat",
  "alternatives": ["npm:bat-cli", "rubygems:bat"],
  "selection_reason": "10x_popularity_gap",
  "downloads_ratio": 225
}
```

This allows the pipeline to continue while flagging tools that might need manual verification. The `disambiguations.json` seed file can be updated based on these reports.

#### Alternatives Considered

**Pause batch for ambiguous tools**: Skip tools that need disambiguation, queue them for manual processing.
Rejected because it would create a growing backlog of unprocessed tools. Better to make a deterministic choice and review later.

**Require all tools in disambiguations.json**: Only process tools that have explicit disambiguation entries.
Rejected because it inverts the workflow. Most tools don't have name collisions. The system should handle the common case automatically and track exceptions.

### Decision 5: Where Disambiguation Lives

Should disambiguation be a separate stage in the resolver chain, integrated into ecosystem probe, or a new package?

#### Chosen: Separate File in Ecosystem Probe Package

Create `internal/discover/disambiguate.go` to house the disambiguation logic:

1. `EcosystemProbe.Resolve()` calls into disambiguation functions
2. Ranking algorithm and selection logic in dedicated file for testability
3. `ConfirmFunc` callback pattern for interactive prompts (same as LLM discovery)
4. Return single best result with alternatives stored in metadata

This keeps the ecosystem probe focused on querying while disambiguation logic is self-contained and testable.

#### Alternatives Considered

**New resolver stage**: Add `DisambiguationResolver` as stage 2.5 between ecosystem probe and LLM.
Rejected because disambiguation isn't a separate resolution step—it's part of handling ecosystem probe results. A new stage would complicate the chain unnecessarily.

**Inline in ecosystem_probe.go**: Add all disambiguation logic directly to the existing file.
Rejected because it would make ecosystem_probe.go too large and mix concerns. Separate file improves testability and code organization without adding package complexity.

**Shared package with LLM discovery ranking**: Extract ranking logic to a shared utility.
Deferred. While LLM discovery has similar ranking (confidence/stars), the inputs differ (ProbeResult vs DiscoveryResult). Premature abstraction. If patterns converge, refactor later.

## Decision Outcome

**Chosen: Popularity-based auto-select with interactive fallback, edit-distance typosquatting detection, and batch pipeline tracking**

### Summary

The disambiguation system integrates into the ecosystem probe stage with three components:

**Ranking algorithm**: Sort matches by downloads (descending), fall back to version count, then ecosystem priority. A match with >10x the downloads of the runner-up is auto-selected with an informational message. Close matches trigger interactive prompting.

**Typosquatting detection**: Before disambiguation, check the tool name against registry entries (top ~500) using Levenshtein distance. If distance ≤2, display a warning comparing the requested name to the registry entry with popularity data. This catches common typos like `rgiprep` → `ripgrep`.

**Interactive prompt**: Display a numbered list with registry, downloads, version count, and repository status. Users select by number. Non-interactive mode errors with the same list formatted as `--from` suggestions.

**Batch integration**: Select deterministically using the ranking algorithm. Track selections with alternatives in batch metrics for human review. Ambiguous selections (close matches where the algorithm picked the first) are flagged.

The existing `ConfirmFunc` pattern from LLM discovery is reused for consistency. The `--yes` flag auto-approves clear winners (>10x gap) but errors on close matches (within 10x).

### Rationale

This approach balances automation with user control:
- Clear winners don't slow users down
- Ambiguous cases get human input
- Typosquatting attempts are caught early
- Batch pipeline remains deterministic while tracking edge cases

The 10x threshold is consistent with LLM discovery's quality scoring approach. Edit-distance checking at threshold ≤2 aligns with academic research on typosquatting detection.

## Solution Architecture

### Component 1: Disambiguation Logic in Ecosystem Probe

Extend `ecosystem_probe.go` with disambiguation:

```go
// disambiguate ranks multiple probe results and selects the best one.
// Returns the selected result with alternatives stored in metadata.
func (p *EcosystemProbe) disambiguate(
    ctx context.Context,
    toolName string,
    results []probeResult,
) (*DiscoveryResult, error) {
    // Sort by downloads DESC, then version count DESC, then priority ASC
    ranked := rankProbeResults(results)

    // Check for clear winner (>10x gap)
    if len(ranked) > 1 && isClearWinner(ranked[0], ranked[1]) {
        return p.selectWithMessage(ranked, "auto-selected based on popularity")
    }

    // Interactive: prompt user
    if p.confirm != nil && isInteractive() {
        return p.promptUser(ranked)
    }

    // Non-interactive with close matches: error
    if len(ranked) > 1 && !isClearWinner(ranked[0], ranked[1]) {
        return nil, &AmbiguousMatchError{Tool: toolName, Matches: ranked}
    }

    // Non-interactive with clear winner or single match: auto-select
    return p.selectWithMessage(ranked, "selected from ecosystem probe")
}

func isClearWinner(first, second probeResult) bool {
    if first.Downloads == 0 || second.Downloads == 0 {
        return false // Can't determine without download data
    }
    return first.Downloads >= second.Downloads * 10
}
```

### Component 2: Typosquatting Detector

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

### Component 3: Interactive Prompt

Reuse the confirmation pattern from LLM discovery:

```go
type ConfirmDisambiguationFunc func(matches []ProbeMatch) (int, error)

// promptUser displays matches and returns the user's selection.
func (p *EcosystemProbe) promptUser(ranked []probeResult) (*DiscoveryResult, error) {
    if p.confirmDisambiguation == nil {
        return nil, errors.New("no confirmation function for disambiguation")
    }

    matches := toProbeMatches(ranked)
    selection, err := p.confirmDisambiguation(matches)
    if err != nil {
        return nil, err
    }

    return ranked[selection].toDiscoveryResult(), nil
}
```

### Component 4: Batch Tracking

Extend batch metrics to track disambiguation:

```go
type DisambiguationRecord struct {
    Tool            string   `json:"tool"`
    Selected        string   `json:"selected"`
    Alternatives    []string `json:"alternatives"`
    SelectionReason string   `json:"selection_reason"` // "single_match", "10x_popularity_gap", "priority_fallback"
    DownloadsRatio  float64  `json:"downloads_ratio,omitempty"`
}
```

### Data Flow

```
Tool Name: "bat"
    │
    ▼
Typosquat Check (vs registry)
    │
    ├─ Warning if similar to known tool
    │
    ▼
Ecosystem Probe (parallel)
    │
    ├─ crates.io: {name: bat, downloads: 45000, versions: 87}
    ├─ npm: {name: bat-cli, downloads: 200, versions: 12}
    └─ rubygems: {name: bat, downloads: 50, versions: 3}
    │
    ▼
Quality Filter
    │
    ├─ All pass thresholds
    │
    ▼
Disambiguation
    │
    ├─ Rank by downloads: crates.io (45K) > npm (200) > rubygems (50)
    ├─ Check ratio: 45000/200 = 225x → Clear winner
    │
    ▼
Auto-select crates.io with message:
"Installing bat from crates.io (sharkdp/bat, 45K downloads/day)
 Also available: npm (bat-cli, 200/day), rubygems (bat, 50/day)"
```

## Implementation Approach

### Phase 1: Core Disambiguation Logic

**Files to create:**
- `internal/discover/disambiguate.go` - Ranking and selection logic
- `internal/discover/disambiguate_test.go` - Unit tests

**Files to modify:**
- `internal/discover/ecosystem_probe.go` - Integrate disambiguation
- `internal/discover/resolver.go` - Add AmbiguousMatchError type

**Deliverable:** Ecosystem probe ranks results and auto-selects clear winners.

### Phase 2: Typosquatting Detection

**Files to create:**
- `internal/discover/typosquat.go` - Edit distance checking
- `internal/discover/typosquat_test.go` - Unit tests

**Files to modify:**
- `internal/discover/chain.go` - Add typosquat check before probe

**Deliverable:** Warning displayed for suspiciously similar names.

### Phase 3: Interactive Prompt

**Files to modify:**
- `cmd/tsuku/create.go` - Add ConfirmDisambiguationFunc callback
- `cmd/tsuku/install.go` - Same callback integration
- `internal/discover/ecosystem_probe.go` - Call confirmation for close matches

**Deliverable:** Interactive selection for ambiguous matches.

### Phase 4: Non-Interactive Error

**Files to modify:**
- `internal/discover/disambiguate.go` - AmbiguousMatchError formatting
- `cmd/tsuku/create.go` - Error handling with --from suggestions

**Deliverable:** Clear error with suggestions in CI pipelines.

### Phase 5: Batch Integration

**Files to modify:**
- `internal/batch/orchestrator.go` - Track disambiguation decisions
- `internal/dashboard/dashboard.go` - Display disambiguation metrics

**Deliverable:** Batch pipeline tracks and reports ambiguous selections.

## Security Considerations

### Download Verification

Disambiguation doesn't change download verification. The selected package goes through the same verification flow (checksums, version verification) as any other ecosystem probe result.

### Execution Isolation

No change to execution isolation. Disambiguation is a selection mechanism, not an execution mechanism.

### Supply Chain Risks

**Typosquatting mitigation**: Edit-distance checking against registry entries catches common typosquats. The warning displays both packages with popularity data, helping users identify impostor packages.

**Wrong package installation**: The disambiguation prompt shows key metadata (downloads, versions, repository) to help users verify they're selecting the intended package. Non-interactive mode errors rather than silently selecting the wrong package.

**Popularity gaming defense**: Download counts alone can be gamed (~$50 for bots). The auto-select criteria require secondary signals (version count ≥3, repository presence) to increase attack cost. Additionally, sandbox validation (existing system) catches packages that don't function correctly.

**Priority order exploitation**: When popularity data is unavailable, we prompt instead of auto-selecting based on priority. This prevents attackers from squatting in higher-priority ecosystems (e.g., Homebrew) to beat legitimate packages in lower-priority ones.

**Batch pipeline risks**: Deterministic selection could consistently pick the wrong package if an attacker registers a popular-looking typosquat. Mitigation: Track selections in batch metrics with a `HighRisk` flag when using priority fallback; require human review for flagged selections; update `disambiguations.json` when issues are discovered.

**Limitations**: This defense-in-depth approach protects against casual squatters but not well-resourced attackers who can maintain multiple versions and link a real repository. The ultimate defense remains sandbox validation and the existing registry override for the top ~500 tools.

### User Data Exposure

No user data is accessed or transmitted by disambiguation logic.

## Consequences

### Positive

- **Clear winners install fast**: >10x popularity gap means no prompt needed
- **Ambiguous cases get human judgment**: Close matches prompt for selection
- **Typosquatting defense**: Edit-distance warnings catch common attacks
- **Consistent UX**: Reuses confirmation patterns from LLM discovery
- **Batch compatible**: Deterministic selection with tracking for review

### Negative

- **Additional latency for prompts**: Close matches and missing popularity data require user input
- **More prompts than minimum**: By prompting when popularity data is missing (rather than auto-selecting on priority), users see more prompts. This is a deliberate security trade-off.
- **False positives in typosquat detection**: Legitimate tools with similar names may trigger warnings

### Neutral

- **10x threshold is a heuristic**: May need adjustment based on real-world usage patterns
- **Registry size limits typosquat detection**: Only checks against ~500 entries, not all packages

## Uncertainties

- **Download count comparability**: npm weekly downloads are significantly higher than crates.io recent downloads for equivalent popularity. The 10x threshold assumes rough comparability, but may need ecosystem-specific normalization if cross-ecosystem comparisons prove misleading.
- **User behavior with prompts**: Will users read the metadata and make informed choices, or just press "1"? May need UX research. If users blindly accept defaults, the security benefit of prompting is reduced.
- **Edit-distance threshold**: The ≤2 threshold is based on npm research. May need adjustment for shorter package names common in tsuku (e.g., "go", "rg", "fd" have many legitimate neighbors).
- **Non-interactive prevalence**: If CI/CD pipelines are common (non-interactive mode), the error-on-ambiguity behavior may be too disruptive. May need to reconsider warning + auto-select as an alternative.
