# Deterministic Decision-Making from LLM Outputs
## Research Phase 2: Extracting Decision Patterns from Existing Code

**Research Date:** 2026-02-10
**Objective:** Design a system where LLM extracts structured quality signals from web content, and a deterministic algorithm makes the final decision about tool sources.

---

## Executive Summary

The tsuku codebase already implements a sophisticated deterministic decision-making system for **ecosystem probe** results. This system demonstrates the exact pattern needed for LLM-discovered sources:

1. **Stage 1: Information Extraction** - Ecosystem probers query registries and extract quality signals (downloads, version count)
2. **Stage 2: Deterministic Quality Filter** - A threshold-based filter rejects low-quality matches using OR logic
3. **Stage 3: Priority Ranking** - When multiple sources pass filtering, priority ranking selects the winner

The proposed LLM Discovery stage can follow this identical pattern, with the LLM serving as the information extractor instead of ecosystem probers.

---

## Current Architecture: Three-Stage Resolver Chain

The discovery system uses a chain resolver that tries stages in sequence:

```
RegistryLookup → EcosystemProbe → LLMDiscovery (stub)
```

Each stage is a `Resolver` that returns `(nil, nil)` for a miss (soft) or `(DiscoveryResult, nil)` for a hit. Once any stage hits, the chain stops.

### Key Data Structures

**DiscoveryResult** - Output from any resolver stage:
```go
type DiscoveryResult struct {
    Builder    string      // e.g., "github", "npm", "pypi"
    Source     string      // builder-specific arg (e.g., "owner/repo")
    Confidence Confidence  // "registry", "ecosystem", or "llm"
    Reason     string      // human-readable explanation
    Metadata   Metadata    // downloads, stars, description, age
}
```

**Metadata** - Quality signals for disambiguation UX:
```go
type Metadata struct {
    Downloads   int    // Monthly downloads (0 if unavailable)
    AgeDays     int    // Days since first publish
    Stars       int    // GitHub stars
    Description string // Short description
}
```

---

## Pattern 1: Quality Filter with Threshold-Based Decisions

### Current Implementation: EcosystemProbe Quality Filter

**File:** `internal/discover/quality_filter.go`

The `QualityFilter` uses per-registry thresholds to reject low-quality matches. Key insight: it uses **OR logic** - a match passes if ANY threshold is met.

**Current Thresholds:**
```
crates.io:  MinDownloads=100 OR MinVersionCount=5
npm:        MinDownloads=100 OR MinVersionCount=5
rubygems:   MinDownloads=1000 OR MinVersionCount=5
pypi:       MinDownloads=0 OR MinVersionCount=3
go:         MinDownloads=0 OR MinVersionCount=3
cpan:       MinDownloads=1 (river.total) OR MinVersionCount=3
cask:       No threshold (fail-open)
unknown:    No threshold (fail-open)
```

**Decision Logic:**
```go
func (f *QualityFilter) Accept(builderName string, result *builders.ProbeResult) (bool, string) {
    thresh, ok := f.thresholds[builderName]
    if !ok {
        return true, "no threshold configured"  // fail-open
    }
    
    // Pass if ANY threshold is met (OR logic)
    if thresh.MinDownloads > 0 && result.Downloads >= thresh.MinDownloads {
        return true, fmt.Sprintf("downloads %d >= %d", result.Downloads, thresh.MinDownloads)
    }
    if thresh.MinVersionCount > 0 && result.VersionCount >= thresh.MinVersionCount {
        return true, fmt.Sprintf("version count %d >= %d", result.VersionCount, thresh.MinVersionCount)
    }
    
    return false, fmt.Sprintf("downloads %d < %d and version count %d < %d", ...)
}
```

### Real-World Application: Squatter Filtering

Test case `TestQualityFiltering_PrettierSquatter` demonstrates how this prevents installing fake packages:

**Scenario:**
- prettier exists on npm (legitimate, 45 million downloads)
- prettier exists on crates.io (squatter, 87 downloads, 3 versions)

**Outcome:**
- Quality filter rejects crates.io (fails both thresholds: 87 < 100, 3 < 5)
- Quality filter accepts npm (45M >= 100)
- Priority ranking picks npm (priority 5 vs 3, but crates.io is filtered out anyway)

**Result:** User installs the legitimate npm package, not the squatter.

---

## Pattern 2: Priority Ranking for Multiple Matches

### Current Implementation: EcosystemProbe Priority Ranking

**File:** `internal/discover/ecosystem_probe.go`

When multiple sources pass the quality filter, priority ranking selects the winner:

```go
priority := map[string]int{
    "cask":      1,
    "homebrew":  2,
    "crates.io": 3,
    "pypi":      4,
    "npm":       5,
    "rubygems":  6,
    "go":        7,
    "cpan":      8,
}
```

**Ranking Logic:**
```go
// Sort by priority (lower number = higher priority)
sort.Slice(matches, func(i, j int) bool {
    pi := p.priority[matches[i].builderName]
    pj := p.priority[matches[j].builderName]
    // Unknown builders get lowest priority
    if pi == 0 { pi = 999 }
    if pj == 0 { pj = 999 }
    return pi < pj
})

// Take the first match
best := matches[0]
```

### When Priority Ranking Applies

Test case `TestQualityFiltering_PriorityRankingAfterFilter` shows the flow:

1. **Quality Filter Stage:** Rejects low-quality squatters
2. **Priority Ranking Stage:** Orders remaining matches by ecosystem preference
3. **Selection:** Takes the highest-priority match

This is a two-tier system:
- Tier 1: Quality (binary filter)
- Tier 2: Preference (priority ranking)

---

## Pattern 3: Graceful Degradation with Partial Signals

### Challenge: Incomplete Metadata

**Test Case:** `TestQualityFiltering_GracefulDegradation`

Scenario: Secondary API calls fail, so some metrics are unavailable (= 0).

**Current Solution:** OR logic allows passing with either signal:
- If downloads API fails but version count is available: use version count alone
- If version count API fails but downloads is available: use downloads alone
- If both fail: fall through to next stage (or fail-open if unconfigured builder)

**Example:**
```go
// npm with only version count available
result := &builders.ProbeResult{
    Downloads:    0,  // API failed
    VersionCount: 10, // Available
}
// Passes npm threshold because VersionCount >= 5
```

This is critical for LLM discovery: we may have incomplete information from web search results.

---

## Pattern 4: Multiple Signals Combined with Confidence Scores

### Metadata Structure in Registry Entries

**File:** `internal/discover/registry.go`

Registry entries store all quality signals for disambiguation and ranking:

```go
type RegistryEntry struct {
    Builder        string `json:"builder"`
    Source         string `json:"source"`
    Binary         string `json:"binary,omitempty"`
    Description    string `json:"description,omitempty"`
    Homepage       string `json:"homepage,omitempty"`
    Repo           string `json:"repo,omitempty"`
    Disambiguation bool   `json:"disambiguation,omitempty"`
    Downloads      int    `json:"downloads,omitempty"`        // Quality signal
    VersionCount   int    `json:"version_count,omitempty"`    // Quality signal
    HasRepository  bool   `json:"has_repository,omitempty"`   // Quality signal
}
```

These signals are collected during the `ProbeAndFilter` stage:

```go
// Enrich entry with quality metadata from ProbeResult
e.Downloads = result.Downloads
e.VersionCount = result.VersionCount
e.HasRepository = result.HasRepository
```

The `Disambiguation` flag (currently boolean) indicates entries requiring user confirmation.

---

## Current Validation Patterns

### Builder-Specific Validators

**File:** `internal/discover/validate.go`

Each builder has a validator that:
1. Checks source existence (e.g., GitHub repo exists)
2. Checks validity constraints (e.g., not archived)
3. Extracts metadata (description, homepage, repo URL)

**Example: GitHubValidator**
```go
func (g *GitHubValidator) validate(source string) (*EntryMetadata, error) {
    // Calls GitHub API to verify repo exists
    if repo.Archived {
        return nil, fmt.Errorf("github repo %s is archived", source)
    }
    return &EntryMetadata{
        Description: repo.Description,
        Homepage:    repo.Homepage,
        Repo:        repo.HTMLURL,
    }, nil
}
```

**Pattern:** Validation is separate from quality filtering. An entry can:
- Pass validation (source exists and is usable) but fail quality filtering (low popularity)
- Exist on multiple sources (validated by ecosystem probe) and need disambiguation

---

## Applying the Pattern to LLM Discovery

### The User's Insight

> "If we provide the LLM with a schema for how to describe the different potential matches for a tool on its web search, it may allow us to produce a deterministic decision making algorithm."

The existing patterns show this approach works. Here's how to apply it:

### Step 1: LLM as Information Extractor

**Input to LLM:** Web search results + structured schema
```
Tool Name: prettier
Web Results: [
  {
    Title: "Prettier - Code Formatter",
    URL: "https://npmjs.com/package/prettier",
    Snippet: "over 45 million downloads...",
    Link: "https://github.com/prettier/prettier"
  },
  {
    Title: "prettier - Crates.io",
    URL: "https://crates.io/crates/prettier",
    Snippet: "87 total downloads, 3 versions...",
  },
  ...
]

Extract for each potential match:
{
  builder: string              // "npm", "crates.io", "github"
  source: string              // "prettier/prettier" or "prettier"
  confidence_signals: {
    downloads: number         // Parsed from text
    version_count: number     // Parsed from text
    has_repository: boolean   // URL indicates linked repo
    is_archived: boolean      // Parsed from text
    description: string       // From search result
  }
  warnings: string[]          // "Low downloads", "Few versions", "Unmaintained"
  builder_indicators: string[]  // How we identified the builder
}
```

**Output:** Structured JSON with multiple candidate sources

### Step 2: Deterministic Algorithm Selects Winner

Apply existing decision patterns:

**Filter Stage 1: Validation**
- Use existing builder validators to confirm source exists
- Reject archived repos, removed packages, etc.

**Filter Stage 2: Quality Threshold**
- Apply existing per-registry thresholds to LLM-extracted signals
- Same OR logic: (Downloads >= MinDownloads) OR (VersionCount >= MinVersionCount)
- Reject low-quality squatters

**Stage 3: Priority Ranking**
- Apply existing priority order: cask > homebrew > crates.io > ...
- Take highest-priority match

**Stage 4: Confidence Scoring**
- Combine LLM confidence (from extraction task) with quality metrics
- May warrant new thresholds for LLM confidence

### Step 3: Handling Multiple Candidates

**Scenario:** Web search finds prettier on:
1. npm (45M downloads, legitimate)
2. crates.io (87 downloads, squatter)
3. PyPI (not actually prettier, wrong tool)

**Algorithm Flow:**
1. LLM extracts candidates with confidence signals
2. Validation stage filters out PyPI (not the right tool based on description)
3. Quality filter rejects crates.io squatter (87 downloads < 100 threshold)
4. Only npm remains
5. Return npm as LLM discovery result

---

## Key Decision-Making Rules

### Rule 1: OR Logic for Quality Signals

A match passes if **any** quality threshold is met:
```
Pass = (Downloads >= MinDownloads) OR (VersionCount >= MinVersionCount)
```

This allows graceful degradation when one metric is unavailable.

### Rule 2: Per-Builder Thresholds

Different ecosystems have different standards:
- Mature ecosystems (npm, rubygems) have higher thresholds
- Niche ecosystems (cpan) have lower thresholds
- Unconfigured builders (cask) fail-open (always pass)

For LLM-discovered sources, we could apply standard thresholds based on the builder identified by the LLM.

### Rule 3: Priority Over Quality

Once quality filtering completes, priority ranking determines the winner:
```
Winner = Minimum Priority (numerically) Among Passed Matches
```

This ensures we prefer official ecosystem sources over generic hosting.

### Rule 4: Exact Name Match (Case-Insensitive)

Before quality filtering, ecosystem probe applies:
```
if !strings.EqualFold(outcome.result.Source, toolName) {
    continue  // Reject name mismatch
}
```

For LLM discovery, similar name matching should apply before threshold evaluation.

---

## Proposed Structure: LLMDiscoveryCandidate

To implement deterministic decision-making for LLM discovery, define:

```go
type LLMDiscoveryCandidate struct {
    // Core identification
    Builder     string                  // "npm", "pypi", "crates.io", "github"
    Source      string                  // builder-specific source
    
    // Quality signals (from LLM web search extraction)
    Downloads    int                     // Parsed from search results
    VersionCount int                     // Parsed from search results
    HasRepository bool                   // Linked repo URL found
    Description  string                  // Extracted from search result
    
    // Confidence metrics
    LLMConfidence    float64             // 0.0-1.0, LLM's confidence in extraction
    SignalConfidence map[string]float64  // Per-signal confidence
    
    // Diagnostic info
    SourceURLs   []string                // Search result URLs supporting this candidate
    WarningFlags []string                // ["low_downloads", "unmaintained", "archive"]
}

type LLMDiscoveryResult struct {
    Candidates      []LLMDiscoveryCandidate
    RejectionReasons map[string]string    // candidate key -> reason
}
```

---

## Testing Patterns: Already Established

The codebase includes comprehensive test patterns for decision-making:

### 1. Squatter Filtering Tests
- `TestQualityFiltering_PrettierSquatter` - Legitimate npm vs crates.io squatter
- `TestQualityFiltering_HttpieSquatter` - PyPI vs crates.io squatter
- `TestQualityFiltering_AllSquattersFiltered` - Multiple squatters all filtered

### 2. Priority Ranking Tests
- `TestQualityFiltering_PriorityRankingAfterFilter` - All pass filter, priority decides

### 3. Degradation Tests
- `TestQualityFiltering_GracefulDegradation` - Works with partial signals
- `TestQualityFiltering_RejectionReason` - Diagnostic messages

### 4. Integration Tests
- `TestChain_RegistryMissFallsToEcosystemProbe` - Stage fallthrough
- `TestChain_RegistryHitSkipsEcosystemProbe` - Stage short-circuit

These patterns should be replicated for LLM discovery testing.

---

## Decision Algorithm Pseudocode

```
function SelectLLMDiscoveryResult(webSearchResults, toolName):
    // Step 1: Extract candidates from web results
    candidates = LLM.ExtractCandidates(webSearchResults, schema={
        builder, source, downloads, version_count, has_repository, description, warnings
    })
    
    // Step 2: Validation stage
    validated = []
    for each candidate in candidates:
        try {
            builder.Validate(candidate.Source)
            validated.append(candidate)
        } catch {
            rejection_reasons[candidate.key] = error
            continue
        }
    
    // Step 3: Quality filtering
    accepted = []
    for each candidate in validated:
        if AcceptByQualityFilter(candidate.Builder, candidate):
            accepted.append(candidate)
        else:
            rejection_reasons[candidate.key] = rejection_reason
    
    // Step 4: Priority ranking
    if len(accepted) == 0:
        return nil  // Miss: soft fail, try next resolver stage
    
    sort(accepted, by=priority[builder])
    winner = accepted[0]
    
    // Step 5: Return discovery result
    return DiscoveryResult{
        Builder: winner.Builder,
        Source: winner.Source,
        Confidence: ConfidenceLLM,
        Reason: fmt.Sprintf("LLM discovered in %s (%.1f%% confidence)", 
                             winner.Builder, winner.LLMConfidence*100),
        Metadata: Metadata{
            Downloads: winner.Downloads,
            Description: winner.Description,
            AgeDays: EstimateAge(winner.Source, winner.Builder),
            Stars: EstimateStars(winner.Source, winner.Builder),
        },
    }

function AcceptByQualityFilter(builderName, candidate):
    thresh = qualityThresholds[builderName]
    if thresh == nil:
        return true  // Fail-open for unknown builders
    
    // OR logic: pass if any threshold is met
    if thresh.MinDownloads > 0 && candidate.Downloads >= thresh.MinDownloads:
        return true
    if thresh.MinVersionCount > 0 && candidate.VersionCount >= thresh.MinVersionCount:
        return true
    
    return false
```

---

## Implementation Roadmap

### Phase 1: LLM Extraction (Currently Stubbed)
- Implement LLM web search integration
- Design extraction schema for ecosystem builders
- Validate LLM output against schema
- Add confidence scoring to extracted signals

### Phase 2: Deterministic Decision Algorithm
- Implement validation stage (reuse existing builders.Validator)
- Implement quality filter (reuse QualityFilter with LLM signals)
- Implement priority ranking (reuse EcosystemProbe priority map)
- Add comprehensive tests following existing patterns

### Phase 3: Confidence Integration
- Combine LLM confidence with quality metrics
- Potentially add new thresholds for LLM-discovered sources
- Log extraction confidence and decision reasoning
- Add telemetry for LLM discovery hit/miss rates

### Phase 4: User Confirmation UX
- Use Metadata fields to display decision reasoning
- Use Disambiguation flag for marginal cases
- Show alternative candidates and rejection reasons
- Allow user to override algorithm

---

## Conclusion

The tsuku codebase already demonstrates a proven pattern for deterministic decision-making:

1. **Extraction:** Ecosystem probers extract quality signals from registries
2. **Filtering:** Quality thresholds reject low-quality squatters
3. **Ranking:** Priority ordering selects the best source
4. **Confidence:** Metadata and signals guide user confirmation

LLM discovery can follow this exact pattern, with the LLM serving as the extractor and the deterministic algorithm handling disambiguation. This provides:

- **Reproducibility:** Same input → same decision
- **Auditability:** Clear decision rules and reasoning
- **Reliability:** Proven patterns from ecosystem stage
- **Extendability:** New signals can be added without changing core logic

The key insight is that the LLM excels at structured information extraction from unstructured web content, while deterministic algorithms excel at combining signals into reliable decisions.
