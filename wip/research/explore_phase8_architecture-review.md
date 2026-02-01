# Architecture Review: Ecosystem Probe Design

**Reviewer**: Architecture Analysis Agent
**Date**: 2026-02-01
**Design Document**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-ecosystem-probe.md`

## Executive Summary

The ecosystem probe design is **ready for implementation** with one critical clarification needed around the priority map initialization. The architecture is well-specified, the goroutine fan-out pattern is clearly defined, and the design makes pragmatic trade-offs given API limitations. The implementation phases are correctly sequenced and align well with existing code patterns.

**Key Findings**:
- Architecture is clear and implementable
- Goroutine pattern is well-specified with proper timeout handling
- Priority ranking approach is reasonable given API constraints
- One missing specification: how/where the priority map is initialized
- Implementation phases are correctly ordered
- Strong alignment with existing builder patterns

**Recommendation**: Proceed with implementation after clarifying priority map initialization in the design.

---

## Question 1: Is the Architecture Clear Enough to Implement?

### Assessment: YES, with one clarification needed

The design provides clear specifications for:

**Well-Defined Components**:
1. **`EcosystemProbe` struct** (lines 156-162): Fields are specified (probers, timeout, priority map)
2. **Data flow** (lines 166-175): Eight-step process from chain resolver through result collection
3. **Builder probe pattern** (lines 180-212): Concrete examples for cargo and go builders
4. **Error handling** (lines 215-219): Soft vs hard errors, logging levels
5. **Integration points** (lines 237-239): How to wire into ChainResolver

**Missing Specification**:

The design specifies that `EcosystemProbe` has a `priority map[string]int` field (line 161) and mentions "rank by priority map" (line 174), but doesn't specify:
- Where this map is initialized (constructor? package-level constant?)
- The exact priority values for each builder
- Whether priorities are configurable or hardcoded

The design mentions "static priority ranking" and lists an order (line 63: "Homebrew Cask > crates.io > PyPI > npm > RubyGems > Go > CPAN") but this needs to be codified in the implementation specification.

**Recommendation**: Add a section to the design showing:
```go
func NewEcosystemProbe(probers []EcosystemProber, timeout time.Duration) *EcosystemProbe {
    return &EcosystemProbe{
        probers: probers,
        timeout: timeout,
        priority: map[string]int{
            "cask":      1,  // Homebrew Cask (GUI apps, but macOS-specific)
            "crates.io": 2,  // Rust ecosystem (CLI-focused)
            "pypi":      3,  // Python ecosystem (many CLI tools)
            "npm":       4,  // Node ecosystem (larger, more web-focused)
            "gem":       5,  // Ruby ecosystem
            "go":        6,  // Go modules
            "cpan":      7,  // Perl ecosystem
        },
    }
}
```

This makes the priority explicit and documents the rationale in code.

### Clarity of Goroutine Pattern

**Excellent specification** (lines 166-175, 79-88):

1. **Launch mechanism**: "goroutine-per-builder" (line 80)
2. **Context handling**: "shared `context.WithTimeout(ctx, 3s)`" (line 81)
3. **Result collection**: Channel-based with `probeOutcome{builderName, result, err}` (line 169)
4. **Timeout behavior**: "Collector waits for all goroutines or timeout, whichever comes first" (line 170)
5. **Cancellation**: "When the context expires, any in-flight requests are cancelled" (line 82)

The design doesn't include the full collection loop code, but provides enough detail to implement:

```go
// Implied implementation from design:
ctx, cancel := context.WithTimeout(ctx, p.timeout)
defer cancel()

type probeOutcome struct {
    builderName string
    result      *ProbeResult
    err         error
}

resultCh := make(chan probeOutcome, len(p.probers))

// Launch goroutines
for _, prober := range p.probers {
    go func(pr EcosystemProber) {
        result, err := pr.Probe(ctx, name)
        resultCh <- probeOutcome{pr.Name(), result, err}
    }(prober)
}

// Collect results
var outcomes []probeOutcome
for i := 0; i < len(p.probers); i++ {
    select {
    case outcome := <-resultCh:
        outcomes = append(outcomes, outcome)
    case <-ctx.Done():
        // Timeout: collect whatever we have
        break
    }
}
```

This pattern is standard Go idiom and the design describes it clearly enough to implement without ambiguity.

---

## Question 2: Are There Missing Components or Interfaces?

### Assessment: NO critical missing components, but one enhancement opportunity

**Present and Accounted For**:

1. **`EcosystemProber` interface** (lines 141-152): Already exists in `ecosystem_probe.go`, correctly extends `SessionBuilder`
2. **`ProbeResult` struct** (lines 146-151): Already defined with all necessary fields
3. **`EcosystemProbe` resolver** (lines 156-162): Struct fields specified
4. **Builder probe implementations**: Pattern provided for all 7 builders (lines 180-212)
5. **Integration with `ChainResolver`**: Clear from existing code at `cmd/tsuku/create.go:681`

**Existing Code Alignment**:

The stub in `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/discover/ecosystem_probe.go` (lines 24-34) shows:
- Interface is already defined
- `Resolve()` signature matches the chain pattern
- Ready to replace stub with real implementation

The chain resolver in `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/discover/chain.go` (lines 24-46):
- Correctly handles `(nil, nil)` as soft miss
- Distinguishes soft errors (logs, continues) from hard errors (stops chain)
- Already wired in `cmd/tsuku/create.go:686-687`

**Minor Enhancement Opportunity**:

The design could specify a **constructor function** for `EcosystemProbe`:
```go
func NewEcosystemProbe(probers []EcosystemProber, timeout time.Duration) *EcosystemProbe
```

This is standard Go practice and makes testing easier (dependency injection). The design implies this (Phase 3, line 237), but doesn't specify the signature.

**Metadata Collection Gap**:

The design states that `DiscoveryResult.Metadata` will hold alternative matches (line 100), but doesn't specify the schema. The existing `Metadata` struct in `resolver.go` (lines 17-23) has:
- `Downloads int`
- `AgeDays int`
- `Stars int`
- `Description string`

But for ecosystem probe results, most of these will be 0 (as acknowledged in lines 46-47). The design should clarify that alternatives are stored in a different field or that Metadata is per-result rather than per-alternative.

**Recommendation**: Add a field to `DiscoveryResult`:
```go
type DiscoveryResult struct {
    Builder    string
    Source     string
    Confidence Confidence
    Reason     string
    Metadata   Metadata
    Alternatives []Alternative // Multiple ecosystem matches
}

type Alternative struct {
    Builder  string
    Source   string
    Priority int // For display ordering
}
```

This makes disambiguation UI (#1321) straightforward to implement.

---

## Question 3: Are Implementation Phases Correctly Sequenced?

### Assessment: YES, well-ordered with correct dependencies

**Phase Breakdown Analysis**:

| Phase | Description | Dependencies | Risk Level |
|-------|-------------|--------------|------------|
| 1 | Add `Probe()` to builders | None (reuses existing fetch methods) | Low |
| 2 | Implement `EcosystemProbe.Resolve()` | Phase 1 complete | Medium |
| 3 | Wire into ChainResolver | Phases 1-2 complete | Low |
| 4 | Tests | Phases 1-3 complete | Low |

**Why This Ordering Works**:

1. **Phase 1 (Builder Probes)**: Each builder can be implemented independently. The design correctly identifies that these are "thin wrapper[s] around existing fetch method[s]" (line 224), which minimizes risk. Each builder's `Probe()` reuses:
   - `CargoBuilder.fetchCrateInfo()` (already exists, line 171 in `cargo.go`)
   - `NpmBuilder.fetchPackageInfo()` (already exists, line 166 in `npm.go`)
   - `GoBuilder.fetchModuleInfo()` (already exists, line 145 in `go.go`)

   This is **safe parallel work** that can be done in any order.

2. **Phase 2 (Resolver Logic)**: Can't start until Phase 1 is done because it needs to call `prober.Probe()`. This is the "core of the design" (line 231) and contains the complexity: goroutine management, timeout handling, result filtering, priority ranking. Correctly placed after Phase 1.

3. **Phase 3 (Wiring)**: Simple integration work. The design correctly notes this is "in `cmd/tsuku/create.go` (`runDiscovery()`)" (line 237). Looking at `create.go:681`, the stub is already there:
   ```go
   stages = append(stages, &discover.EcosystemProbe{})
   ```

   Just needs to change to:
   ```go
   stages = append(stages, discover.NewEcosystemProbe(probers, 3*time.Second))
   ```

   This can't be done until Phase 2 is complete (needs constructor).

4. **Phase 4 (Tests)**: Correctly last. Tests need all components to validate integration.

**Potential Optimization**:

The design could split Phase 1 into two sub-phases:
- **1a**: Add `Probe()` to one builder (e.g., cargo) as a proof-of-concept
- **1b**: Stub out the other 6 builders to return `&ProbeResult{Exists: false}` temporarily

This would allow Phase 2 development to start earlier with one real builder, then swap in the rest. However, this is an optimization, not a requirement. The current sequencing is correct.

**No Circular Dependencies**:

The design correctly avoids circular dependencies:
- Builders don't depend on the probe resolver
- Probe resolver depends only on the `EcosystemProber` interface
- Chain resolver depends on the `Resolver` interface, not concrete types

This is clean separation of concerns.

---

## Question 4: Are There Simpler Alternatives We Overlooked?

### Assessment: Design makes the right trade-offs for simplicity

**Simplicity Analysis by Decision**:

### Decision 1: Existence-First Metadata (lines 42-54)

**Chosen**: Probe() returns existence + whatever metadata the API provides
**Simpler alternative**: Just call `CanBuild()` and skip the `ProbeResult` struct

**Analysis**: The design correctly rejects the simpler alternative because:
1. `CanBuild()` returns `(bool, error)` with no `Source` string
2. The probe needs the builder-specific source (e.g., "bat" for crates.io, "@sharkdp/bat" for npm)
3. Adding a new method is cheaper than making two API calls per builder (once for existence, once for source)

**Conclusion**: Current approach is **simpler in total system complexity**, even though it adds one method per builder.

### Decision 2: Static Priority vs Popularity (lines 56-74)

**Chosen**: Static ecosystem ranking
**Rejected alternatives**:
- Secondary API calls for download stats (lines 49-52)
- Skip filtering entirely (line 68)
- Always prompt the user (line 72)

**Analysis**: The design correctly identifies that:
1. Secondary API calls double HTTP requests (npm has `api.npmjs.org/downloads`, PyPI has `pypistats.org`)
2. Stats services may rate-limit or go down independently of package registries
3. The marginal value of download counts doesn't justify latency cost
4. Static priority is deterministic and testable

**Alternative We Could Consider**:

Use **ecosystem-specific heuristics without secondary calls**:
- npm: Check if package is in a scope (`@org/pkg` suggests higher legitimacy)
- PyPI: Parse project URLs for GitHub org vs personal repo
- crates.io: No additional signal available
- Go: Use the `Age` field already in the API response

This would add **minimal complexity** (string parsing) without additional network calls. However, the design's static priority is simpler and good enough for v1. Heuristics can be added later if needed.

**Conclusion**: Current approach is **appropriately simple** for initial implementation.

### Decision 3: Goroutine-Per-Builder vs Worker Pool (lines 76-88)

**Chosen**: One goroutine per builder, all parallel
**Rejected alternative**: Worker pool with limited concurrency (lines 85-87)

**Analysis**: The design correctly notes:
1. Only 5-6 builders, not hundreds
2. Worker pool overhead not justified
3. Worst-case latency is `timeout`, not `N * per-builder-timeout`

**Simpler Alternative We Could Consider**:

**Sequential with early exit**:
```go
for _, prober := range priorityOrderedProbers {
    if result, err := prober.Probe(ctx, name); err == nil && result.Exists {
        return result // Stop at first match
    }
}
```

**Pros**:
- Simpler code (no goroutines, no channels)
- Fewer concurrent HTTP requests

**Cons**:
- Slower: worst-case is sum of all timeouts (6 * 500ms = 3s), not just 3s timeout
- Doesn't discover multiple matches for disambiguation
- User experience: feels slower because it's actually slower

**Conclusion**: Goroutine approach is **correct trade-off**. Parallelism is worth the minimal complexity for 6 concurrent requests.

### Simplest Alternative: Skip Ecosystem Probe Entirely

**What if we just went straight to LLM after registry miss?**

**Pros**:
- Zero additional code in the probe stage
- LLMs can find packages in any ecosystem

**Cons**:
- Costs money for every tool not in the curated registry (~500 entries)
- Requires API keys (barrier to entry)
- Non-deterministic (same input can produce different results)
- Slower (LLM latency > registry API latency)

**Conclusion**: The ecosystem probe is **justified**. Most developer tools are in npm, PyPI, or crates.io. Querying these for free, deterministically, and quickly (3s) is better UX than always hitting an LLM.

---

## Question 5: Is the Goroutine Fan-Out/Channel Collection Pattern Well-Specified?

### Assessment: YES, well-specified and follows Go idioms

**Pattern Specification Quality**:

The design specifies (lines 79-82, 168-170):
1. **Launch**: "goroutine-per-builder" with shared context
2. **Communication**: Channel with `probeOutcome{builderName, result, err}`
3. **Timeout**: `context.WithTimeout(ctx, 3*time.Second)`
4. **Collection**: "Collector waits for all goroutines or timeout, whichever comes first"
5. **Cancellation**: Context expiry cancels in-flight requests

**Implementation Clarity**:

From the specification, the implementation is straightforward:

```go
func (p *EcosystemProbe) Resolve(ctx context.Context, name string) (*DiscoveryResult, error) {
    ctx, cancel := context.WithTimeout(ctx, p.timeout)
    defer cancel()

    type probeOutcome struct {
        builderName string
        result      *ProbeResult
        err         error
    }

    resultCh := make(chan probeOutcome, len(p.probers))

    // Fan out: launch all probes in parallel
    for _, prober := range p.probers {
        go func(pr EcosystemProber) {
            result, err := pr.Probe(ctx, name)
            resultCh <- probeOutcome{pr.Name(), result, err}
        }(prober)
    }

    // Collect: gather results until all complete or timeout
    var outcomes []probeOutcome
    for i := 0; i < len(p.probers); i++ {
        select {
        case outcome := <-resultCh:
            outcomes = append(outcomes, outcome)
        case <-ctx.Done():
            // Timeout or cancellation: stop collecting
            break
        }
    }

    // Filter, rank, return best match (steps 6-9 from design)
    return p.selectBestMatch(outcomes, name)
}
```

**Potential Issue: Goroutine Leak Prevention**:

The design doesn't explicitly address goroutine leak prevention. If the context times out, in-flight HTTP requests should be cancelled. The design implies this (line 82: "in-flight requests are cancelled"), but it's worth verifying that each builder's `fetchXInfo()` method:
1. Accepts a `context.Context`
2. Uses `http.NewRequestWithContext(ctx, ...)`
3. Will return when `ctx.Done()` closes

**Verification from Existing Code**:

Looking at the three builder examples:
- `CargoBuilder.fetchCrateInfo()` (line 171 in `cargo.go`): ✅ Uses `http.NewRequestWithContext(ctx, "GET", ...)`
- `NpmBuilder.fetchPackageInfo()` (line 166 in `npm.go`): ✅ Uses `http.NewRequestWithContext(ctx, "GET", ...)`
- `GoBuilder.fetchModuleInfo()` (line 145 in `go.go`): ✅ Uses `http.NewRequestWithContext(ctx, "GET", ...)`

All builders correctly use context-aware HTTP requests. Goroutine leaks are prevented.

**Channel Buffering**:

The pattern should use a buffered channel sized to the number of probers:
```go
resultCh := make(chan probeOutcome, len(p.probers))
```

This ensures goroutines can send results even if the timeout fires before all results are collected. Without buffering, goroutines could block on send after timeout, causing leaks.

**Recommendation**: Add to the design (error handling section):
> "Use a buffered channel sized to `len(probers)` to prevent goroutine leaks if timeout fires before all results arrive."

**Race Condition Check**:

The pattern doesn't have data races because:
1. Each goroutine writes to a different `probeOutcome` struct
2. The channel serializes communication
3. The slice `outcomes` is only appended to by the collecting goroutine

No shared mutable state = no races.

---

## Question 6: Does the Priority Ranking Approach Make Sense Given API Limitations?

### Assessment: YES, pragmatic and well-justified

**API Limitations Reality Check**:

The design states (lines 15-16):
> "After examining all seven ecosystem builder implementations, **none of the registry APIs expose download counts** in their standard response"

**Verification**:

From the builder code reviewed:

| Builder | API Response Struct | Download Field? | Age Field? |
|---------|---------------------|-----------------|------------|
| Cargo | `cratesIOCrateResponse` (line 28-35, cargo.go) | ❌ No | ❌ No |
| npm | `npmPackageResponse` (line 23-32, npm.go) | ❌ No | ❌ No |
| Go | `goProxyLatestResponse` (line 23-26, go.go) | ❌ No | ✅ `Time` field |
| PyPI | (not reviewed, but design states "none") | ❌ No | ❌ No |
| Gem | (not reviewed) | ❌ No | ❌ No |
| CPAN | (not reviewed) | ❌ No | ❌ No |
| Cask | (not reviewed) | ❌ No | ❌ No |

The design's claim is **accurate**: download counts aren't available without secondary API calls.

**Why Secondary Calls Are Correctly Rejected**:

The design notes (lines 49-52):
- npm has `api.npmjs.org/downloads` (separate service)
- PyPI has `pypistats.org` (separate service)
- This "doubles the number of HTTP requests per probe"
- "adds latency" and "creates dependencies on stats services that may rate-limit or go down"

**Cost/Benefit Analysis**:

| Approach | HTTP Requests | Latency | Reliability | Determinism |
|----------|--------------|---------|-------------|-------------|
| Static priority | 6 (one per builder) | 3s (parallel) | High (uses registry APIs) | Perfect |
| Popularity-based (secondary calls) | 12 (double) | 5-6s (more round trips) | Lower (stats services can fail) | Perfect |
| LLM fallback only | 0 (skips probe) | 5-10s (LLM latency) | Medium (API can fail) | Low (non-deterministic) |

**Conclusion**: Static priority is the **best trade-off** for this use case.

**Priority Ranking Rationale**:

The design proposes (line 63):
> Homebrew Cask > crates.io > PyPI > npm > RubyGems > Go > CPAN

**Evaluation**:

| Ecosystem | Rationale | Concerns |
|-----------|-----------|----------|
| Cask | macOS GUI apps, well-curated | Not cross-platform, may not match CLI tools |
| crates.io | Rust CLI tools are popular (ripgrep, bat, fd, etc.) | Smaller ecosystem than npm/PyPI |
| PyPI | Huge ecosystem, many CLI tools | Lots of noise, web frameworks |
| npm | Largest ecosystem | Even more noise, web-focused |
| RubyGems | Declining but still has CLI tools (bundler, rake) | Less active than others |
| Go | Many CLI tools (kubectl, hugo, etc.) | Different naming (module path vs binary name) |
| CPAN | Legacy but stable | Very old ecosystem |

**Potential Issue: Go Priority**:

The design ranks Go as 6th (second-to-last), but many popular CLI tools are Go-based:
- kubectl (Kubernetes)
- hugo (static site generator)
- goreleaser (release automation)
- gh (GitHub CLI)

However, these are likely in the curated registry (~500 entries), so they'd be caught in Stage 1 before the probe. For **tools not in the registry**, crates.io/PyPI/npm are more likely to have lesser-known tools because their ecosystems are larger.

**Conclusion**: Priority ranking is **reasonable for v1**. Can be adjusted based on real usage data from telemetry (#1319).

**Alternative Ranking to Consider**:

Instead of static global priority, use **contextual priority**:
- If the tool name matches `[a-z0-9-]+` (simple kebab-case), prioritize crates.io/Go
- If it starts with `@`, prioritize npm (scoped packages)
- If it ends in `.py`, prioritize PyPI

This adds minimal complexity (regex matching) and could improve accuracy. However, the design's static priority is simpler and defensible.

**Recommendation**: Implement static priority as specified, add telemetry in Phase 4 to track:
- Which ecosystems are selected
- How often multiple ecosystems match the same tool
- User override rate (manual `--from` flag usage after probe)

Use this data to refine the priority ranking in a future iteration.

---

## Alignment with Existing Code

### Strong Alignment Found

**1. Builder Pattern Consistency**:

All existing builders follow the same pattern:
- `fetchXInfo(ctx, name)` method that queries the registry API
- `CanBuild()` wraps `fetchXInfo()` and returns `(bool, error)`
- Uses `http.NewRequestWithContext(ctx, ...)` for cancellable requests

The design's `Probe()` method fits this pattern perfectly:
```go
func (b *CargoBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
    info, err := b.fetchCrateInfo(ctx, name)  // Reuses existing method
    if err != nil {
        return &ProbeResult{Exists: false}, nil
    }
    return &ProbeResult{Exists: true, Source: name}, nil
}
```

**2. Chain Resolver Integration**:

The existing `ChainResolver` (lines 24-46 in `chain.go`) already handles:
- `(nil, nil)` as soft miss (line 41-42)
- Soft errors: log and continue (line 36-39)
- Hard errors: stop chain (line 33-35)

The design's `EcosystemProbe.Resolve()` returns:
- `(nil, nil)` on no matches (line 172) ✅
- `(*DiscoveryResult, nil)` on success (lines 173-174) ✅
- Soft errors for individual builder failures (line 215) ✅

**Perfect fit** with existing infrastructure.

**3. Discovery Result Structure**:

The design uses the existing `DiscoveryResult` struct (lines 26-41 in `resolver.go`):
- `Builder string` ✅ Set to builder name (line 174)
- `Source string` ✅ Set from `ProbeResult.Source` (line 150)
- `Confidence` ✅ Set to `"ecosystem"` (line 100, 173)
- `Reason string` ✅ Human-readable explanation
- `Metadata` ✅ Optional metadata (though mostly zeros for ecosystem probe)

No changes to existing types needed.

**4. Wiring in `runDiscovery()`**:

The current stub at `create.go:681`:
```go
stages = append(stages, &discover.EcosystemProbe{})
```

The design implies changing to (Phase 3):
```go
// Need to construct list of probers first
probers := []discover.EcosystemProber{
    builders.NewCargoBuilder(httpClient),
    builders.NewNpmBuilder(httpClient),
    builders.NewPypiBuilder(httpClient),
    builders.NewGoBuilder(httpClient),
    // ... etc
}
stages = append(stages, discover.NewEcosystemProbe(probers, 3*time.Second))
```

**Minor Issue**: The design doesn't specify where `httpClient` comes from. Looking at `create.go`, there's no global HTTP client visible. The builders currently create their own clients in their `NewXBuilder()` constructors (e.g., `cargo.go:62-67`).

**Recommendation**: Either:
1. Have `runDiscovery()` create a shared HTTP client with the 3-second timeout
2. Let each builder use its default 60-second timeout (which will be cancelled by the probe's 3-second context timeout)

Option 2 is simpler. The context cancellation will stop the HTTP requests anyway, so per-builder timeouts don't matter.

---

## Risks and Mitigations

### Risk 1: Cask False Positives (Medium Probability, Low Impact)

**Issue**: Homebrew Cask contains macOS GUI apps that share names with CLI tools
**Example**: `bat` could match both the Rust CLI tool and a hypothetical macOS app

**Mitigation in Design**:
- Line 92: "Cask may produce noise... Can be excluded in a follow-up if it's a problem"
- Line 102: "Cask is included initially"

**Recommendation**: Include Cask in Phase 1, add telemetry in Phase 4 to track:
- How often Cask is selected
- User override rate after Cask selection
- False positive reports

Can remove Cask in a follow-up if data shows it's noisy.

### Risk 2: Static Priority Doesn't Match User Expectations (Medium Probability, Medium Impact)

**Issue**: A Go developer searching for a Go tool may be surprised if the npm version is selected

**Mitigation in Design**:
- Line 91: "A Go developer probably expects `go install`... We won't know until real usage data surfaces"
- Line 266-267: Users can override with `--from`
- Line 100: "Full list available in `DiscoveryResult.Metadata` for downstream disambiguation (#1321)"

**Additional Mitigation**: Add a confirmation prompt when multiple ecosystems match:
```
Found "bat" in multiple ecosystems:
  1. crates.io (Rust) [recommended]
  2. npm (Node.js)
  3. go (Go modules)
Install from crates.io? [Y/n]
```

This is **out of scope** for this design (handled by #1321), but worth noting.

### Risk 3: Timeout Too Aggressive (Low Probability, Low Impact)

**Issue**: 3-second timeout may be too short for slow networks or rate-limited APIs

**Mitigation in Design**:
- Line 158: Timeout is a configurable field, not hardcoded
- Line 217: Individual API failures are soft errors, don't stop the chain
- Line 82: Context cancellation is graceful (returns partial results)

**Recommendation**: Add telemetry to track:
- How often the timeout fires before all probers return
- Which probers are slowest
- Partial result rate

Can adjust timeout in a config file if needed.

### Risk 4: Name Confusion / Supply Chain (High Impact, Low Probability)

**Issue**: A malicious package with the same name as a legitimate tool could be auto-selected

**Mitigation in Design**:
- Lines 62-63: Exact name match required (rejects partial matches)
- Line 264: "Curated registry covers popular tools" (they never reach the probe)
- Line 266: "Create pipeline's existing verification steps" (checksums, etc.)

**Additional Mitigation Not in Design**:
- Consider adding a warning for newly published packages (using Go builder's `Age` field)
- Consider adding a warning if the package has no homepage/repository URL

These are enhancements, not blockers for v1.

---

## Recommendations

### Critical (Must Fix Before Implementation)

1. **Clarify priority map initialization** in the design:
   - Show the `NewEcosystemProbe()` constructor signature
   - Document the exact priority values for each builder
   - Explain the rationale for the ordering

### High Priority (Should Add to Design)

2. **Specify the alternatives storage mechanism**:
   - Add `Alternatives []Alternative` field to `DiscoveryResult`, or
   - Clarify that only the top match is returned and disambiguation (#1321) will re-query

3. **Add buffered channel guidance**:
   - Specify `make(chan probeOutcome, len(probers))` to prevent goroutine leaks

### Medium Priority (Nice to Have)

4. **Add telemetry hooks** to the design:
   - What events should be tracked (builder selected, timeout fired, multiple matches, etc.)
   - Where in the code these events fire
   - What metadata to include (latency, builder name, match count)

5. **Specify HTTP client sharing**:
   - Should builders share one HTTP client or create their own?
   - Does the shared 3-second context timeout override individual builder timeouts?

### Low Priority (Future Considerations)

6. **Document potential for popularity-based ranking v2**:
   - If tsuku builds its own usage database from telemetry, the priority map could become dynamic
   - Note this as a future enhancement in the design's "Consequences" section

---

## Conclusion

The ecosystem probe design is **architecturally sound and ready for implementation**. It makes pragmatic trade-offs given API limitations, follows Go idioms for concurrent HTTP requests, and integrates cleanly with the existing chain resolver infrastructure.

**Key Strengths**:
1. Reuses existing builder fetch methods (low risk)
2. Well-specified goroutine pattern with proper timeout/cancellation
3. Correctly sequenced implementation phases
4. Strong alignment with existing code patterns
5. Realistic about API constraints and avoids overengineering

**Single Critical Gap**:
- Priority map initialization needs to be specified

**Recommendation**: Add the priority map initialization to the design, then **proceed with implementation**. The design is clear enough that an engineer familiar with Go can implement all four phases without ambiguity.

The static priority ranking is a reasonable v1 approach. Telemetry from real usage will inform whether to adjust priorities, add heuristics, or implement disambiguation UI. This is good engineering: ship the simplest thing that works, measure, iterate.

**Estimated Implementation Effort** (based on design phases):
- Phase 1: 2-3 hours (7 builders × 20 minutes each)
- Phase 2: 4-6 hours (core logic, filtering, ranking)
- Phase 3: 1 hour (wiring)
- Phase 4: 4-6 hours (unit tests, integration tests)

**Total**: ~2 days for an experienced Go developer

The design is well-scoped and achievable.
