# Architecture Review: Discovery Telemetry Events

## Executive Summary

The design document for discovery telemetry is **architecturally sound and ready for implementation** with minor clarifications needed. It follows established patterns from the existing telemetry system, provides clear component boundaries, and has correctly sequenced implementation phases. The design shows good understanding of the codebase and makes appropriate trade-offs.

**Key Findings:**
1. Event structure and action types are clear and well-defined
2. Backend dispatch pattern correctly mirrors LLM event handling
3. Implementation phases are properly sequenced with clear dependencies
4. Minor ambiguities in instrumentation timing and error categorization need clarification
5. A simpler alternative exists for the stats dashboard integration

**Recommendation:** Proceed with implementation after addressing the clarifications below.

---

## 1. Architecture Clarity Assessment

### 1.1 Event Structure (DiscoveryEvent)

**Status:** ✅ Clear and implementable

The proposed `DiscoveryEvent` struct is well-designed:

```go
type DiscoveryEvent struct {
    Action        string `json:"action"`                    // discovery_* action type
    ToolName      string `json:"tool_name"`                 // normalized tool name searched
    Stage         string `json:"stage,omitempty"`           // winning stage: "registry", "ecosystem", "llm"
    Confidence    string `json:"confidence,omitempty"`      // "registry", "ecosystem", "llm"
    Builder       string `json:"builder,omitempty"`         // builder name (github, cargo, pip, etc.)
    Source        string `json:"source,omitempty"`          // source identifier (e.g., "sharkdp/bat")
    MatchCount    int    `json:"match_count,omitempty"`     // number of matches (disambiguation)
    ErrorCategory string `json:"error_category,omitempty"`  // error type on failure
    DurationMs    int64  `json:"duration_ms"`               // total resolution time
    OS            string `json:"os"`
    Arch          string `json:"arch"`
    TsukuVersion  string `json:"tsuku_version"`
    SchemaVersion string `json:"schema_version"`
}
```

**Strengths:**
- Matches the existing pattern (Event, LLMEvent, DiscoveryEvent)
- All fields have clear purposes and correspond to DiscoveryResult struct
- Omitempty tags correctly applied to optional fields
- Common fields (OS, Arch, TsukuVersion, SchemaVersion) match existing events

**Potential Ambiguity:**
- Stage and Confidence appear redundant (both would be "registry", "ecosystem", or "llm")
- The design doesn't specify if both should always have the same value
- Recommendation: Remove one or clarify their distinct purposes in the spec

### 1.2 Action Types

**Status:** ✅ Clear and complete

The six action types cover all resolution outcomes:

| Action | Use Case | Fields Required |
|--------|----------|-----------------|
| `discovery_registry_hit` | Registry found match | ToolName, Stage, Builder, Source, DurationMs |
| `discovery_ecosystem_hit` | Ecosystem probe found match | ToolName, Stage, Builder, Source, DurationMs |
| `discovery_llm_hit` | LLM discovery found match | ToolName, Stage, Builder, Source, DurationMs |
| `discovery_not_found` | All stages missed | ToolName, DurationMs, ErrorCategory |
| `discovery_disambiguation` | Multiple matches returned | ToolName, MatchCount, Stage, Builder, Source, DurationMs |
| `discovery_error` | Fatal error occurred | ToolName, ErrorCategory, DurationMs |

**Gap Identified:**
The design mentions "per-stage timing" in the Decision 2 section but doesn't include it in the struct. If you want per-stage latency, add:
```go
RegistryMs   int64 `json:"registry_ms,omitempty"`
EcosystemMs  int64 `json:"ecosystem_ms,omitempty"`
LLMMs        int64 `json:"llm_ms,omitempty"`
```

If you only need total duration, remove the mention of "per-stage timing" from the design doc.

### 1.3 Instrumentation Points

**Status:** ⚠️ Needs clarification

The design states: "After the chain completes (success or failure), it calls `telemetry.SendDiscovery()` with the result."

**Current code analysis:**
```go
// chain.go:Resolve() returns either:
// 1. (*DiscoveryResult, nil) on success
// 2. (nil, &NotFoundError{}) when all stages miss
// 3. (nil, error) on hard errors (context cancellation, budget exhaustion)
```

**Questions needing clarification:**
1. **Where exactly does instrumentation happen?**
   - Inside `ChainResolver.Resolve()` before returning?
   - In `tryDiscoveryFallback()` after calling `runDiscovery()`?
   - In a wrapper around the resolver?

2. **How do we capture per-stage timing?**
   - The current `ChainResolver` doesn't track which stages were attempted or their latency
   - Need to add timing instrumentation to the loop in `chain.go:24-44`

3. **What happens on soft errors?**
   - Current code logs soft errors and continues to next stage
   - Should these emit `discovery_error` events or be invisible to telemetry?

**Recommendation:**
Add a timing-aware wrapper to the chain resolver:

```go
// In chain.go
type stageResult struct {
    stageName string
    duration  time.Duration
    err       error
}

func (c *ChainResolver) ResolveWithTelemetry(ctx context.Context, toolName string) (*DiscoveryResult, []stageResult, error) {
    // Track attempts and timing
    // Return results + stage metadata for telemetry
}
```

Alternatively, add a telemetry callback to the resolver:
```go
type TelemetryHook func(event DiscoveryEvent)

func (c *ChainResolver) SetTelemetryHook(hook TelemetryHook) {
    c.telemetryHook = hook
}
```

### 1.4 Backend Dispatch

**Status:** ✅ Clear and correct

The backend dispatch pattern correctly mirrors the LLM event handling:

**Current pattern (from index.ts:419-473):**
```typescript
const llmActions: LLMActionType[] = [
  "llm_generation_started",
  "llm_generation_completed",
  // ...
];

if (typeof event.action === "string" && llmActions.includes(event.action as LLMActionType)) {
  // Handle LLM event
}
```

**Proposed pattern:**
```typescript
const discoveryActions: DiscoveryActionType[] = [
  "discovery_registry_hit",
  "discovery_ecosystem_hit",
  // ...
];

if (typeof event.action === "string" && discoveryActions.includes(event.action as DiscoveryActionType)) {
  // Handle discovery event
}
```

This is correct and follows the established pattern. No issues.

### 1.5 Blob Layout

**Status:** ✅ Correct with minor optimization opportunity

**Proposed layout:**
```
blob0:  action            (discovery_*)
blob1:  tool_name
blob2:  stage
blob3:  confidence
blob4:  builder
blob5:  source
blob6:  match_count       (as string)
blob7:  error_category
blob8:  duration_ms       (as string)
blob9:  os
blob10: arch
blob11: tsuku_version
blob12: schema_version
```

**Analysis:**
- 13 blobs total (same as standard events, fits Analytics Engine limits)
- Index on `tool_name` is correct for per-tool queries
- Blob positions are logical and grouped by purpose

**Optimization opportunity:**
Since Stage and Confidence appear to always be identical, consider removing blob3 (confidence) and using only blob2 (stage). This would save one blob position or allow for future expansion.

**Current usage across event types:**
- Standard events: 13 blobs (blob0-12)
- LLM events: 16 blobs (blob0-15)
- Discovery events: 13 blobs (blob0-12)
- **Total blob usage: 42 out of 20 maximum per dataset**

**CRITICAL ISSUE IDENTIFIED:**
Analytics Engine has a **20 blob maximum per dataset**, not per event type. The current design assumes different event types can use overlapping blob positions, but that's **incorrect**.

**Fix required:**
Discovery events must use blob13-25 (not blob0-12) to avoid collision with standard events. LLM events already use blob0-15 in a different dataset or namespace.

**Check with existing implementation:**
Looking at index.ts:450-470, LLM events use blob0-15 in the **same dataset** (tsuku_telemetry). Standard events use blob0-12. This means they're **already colliding** unless the backend is doing something clever.

**Re-reading the code more carefully:**
All events write to the same dataset (env.ANALYTICS), but each event has different blob content. Analytics Engine doesn't enforce schema per dataset - it just stores blobs. The queries must know which blob positions to read based on action prefix.

**Correction:** The design is correct. Different event types can reuse blob positions because queries filter by action first, then read the appropriate blobs. No collision occurs.

### 1.6 Stats Endpoint

**Status:** ✅ Clear and sufficient

The `/stats/discovery` endpoint spec is clear:

```json
{
  "generated_at": "2026-02-02T...",
  "period": "all_time",
  "total_lookups": 1234,
  "by_stage": {
    "registry": 800,
    "ecosystem": 350,
    "llm": 10,
    "not_found": 74
  },
  "top_not_found": ["sometool", "othertool"],
  "error_rate": 0.02
}
```

**Missing specification:**
- How many tools to return in `top_not_found`? (suggest: 20)
- Should `top_not_found` include counts? (suggest: yes, return objects with name+count)
- Should the endpoint support time filtering (last 30 days, etc.)? (suggest: no for MVP)

**Recommended schema enhancement:**
```json
{
  // ... existing fields ...
  "top_not_found": [
    {"name": "sometool", "count": 45},
    {"name": "othertool", "count": 32}
  ]
}
```

---

## 2. Missing Components and Interfaces

### 2.1 Factory Functions

**Status:** ⚠️ Missing specification

The design mentions "factory functions per action" but doesn't specify their signatures. Based on the existing pattern (Event and LLMEvent), we can infer:

```go
// Required factory functions (add to design doc):

func NewDiscoveryRegistryHitEvent(toolName, builder, source string, durationMs int64) DiscoveryEvent
func NewDiscoveryEcosystemHitEvent(toolName, builder, source string, durationMs int64) DiscoveryEvent
func NewDiscoveryLLMHitEvent(toolName, builder, source string, durationMs int64) DiscoveryEvent
func NewDiscoveryNotFoundEvent(toolName, errorCategory string, durationMs int64) DiscoveryEvent
func NewDiscoveryDisambiguationEvent(toolName, builder, source string, matchCount int, durationMs int64) DiscoveryEvent
func NewDiscoveryErrorEvent(toolName, errorCategory string, durationMs int64) DiscoveryEvent

func newBaseDiscoveryEvent() DiscoveryEvent // internal helper
```

### 2.2 Error Categorization

**Status:** ⚠️ Missing specification

The `ErrorCategory` field is used in `discovery_not_found` and `discovery_error` actions but the design doesn't define the allowed values.

**Suggested categories based on codebase analysis:**

For `discovery_not_found`:
- `"all_stages_missed"` - All stages returned (nil, nil)
- `"registry_miss_ecosystem_filtered"` - Registry missed, ecosystem found but quality filter rejected

For `discovery_error`:
- `"context_cancelled"` - Context deadline exceeded or cancelled
- `"budget_exhausted"` - LLM API budget/quota exceeded (future)
- `"ecosystem_timeout"` - Ecosystem probe timed out before completing all queries
- `"llm_failure"` - LLM generation failed (future)
- `"normalization_error"` - Tool name normalization failed
- `"unknown"` - Catch-all for unexpected errors

**Recommendation:** Add an ErrorCategory constants section to the design doc.

### 2.3 SendDiscovery Method

**Status:** ⚠️ Needs interface specification

The design states "Add `SendDiscovery()` to `client.go`" but doesn't specify the signature or behavior.

**Inferred from existing pattern:**

```go
// SendDiscovery sends a discovery event asynchronously. It never blocks and never returns errors.
// If telemetry is disabled, this is a no-op.
// If debug mode is enabled, the event is printed to stderr instead of being sent.
func (c *Client) SendDiscovery(event DiscoveryEvent) {
    if c.disabled {
        return
    }

    if c.debug {
        data, _ := json.Marshal(event)
        fmt.Fprintf(os.Stderr, "[telemetry] %s\n", data)
        return
    }

    // Fire-and-forget: spawn goroutine, no waiting
    go c.sendJSON(event)
}
```

This matches the existing `SendLLM()` pattern exactly. The design should include this in the "CLI telemetry" phase.

### 2.4 Backend Validation Function

**Status:** ⚠️ Needs specification

The design mentions `validateDiscoveryEvent()` but doesn't define validation rules.

**Inferred validation rules based on action type:**

```typescript
function validateDiscoveryEvent(event: DiscoveryTelemetryEvent): string | null {
  // Common required fields
  if (!event.tool_name) return "tool_name is required";
  if (!event.os) return "os is required";
  if (!event.arch) return "arch is required";
  if (!event.tsuku_version) return "tsuku_version is required";
  if (event.duration_ms === undefined) return "duration_ms is required";

  switch (event.action) {
    case "discovery_registry_hit":
    case "discovery_ecosystem_hit":
    case "discovery_llm_hit":
      if (!event.stage) return `stage is required for ${event.action}`;
      if (!event.builder) return `builder is required for ${event.action}`;
      if (!event.source) return `source is required for ${event.action}`;
      break;

    case "discovery_disambiguation":
      if (!event.stage) return "stage is required for discovery_disambiguation";
      if (!event.builder) return "builder is required for discovery_disambiguation";
      if (!event.source) return "source is required for discovery_disambiguation";
      if (event.match_count === undefined) return "match_count is required for discovery_disambiguation";
      break;

    case "discovery_not_found":
      // tool_name and duration_ms are sufficient
      break;

    case "discovery_error":
      if (!event.error_category) return "error_category is required for discovery_error";
      break;
  }

  return null;
}
```

**Recommendation:** Add validation rules table to the design doc.

### 2.5 Dashboard Implementation

**Status:** ✅ Clear but incomplete integration spec

The design says "Add a section to `website/stats/index.html`" but doesn't specify:
- Where in the page layout (top, bottom, separate tab?)
- How it integrates with existing stats (combined view or separate?)
- Chart library/format to use (matches existing dashboard style?)

**Recommendation:** Add a mockup or detailed layout description to the design doc.

---

## 3. Implementation Phase Sequencing

### 3.1 Phase Dependencies

**Status:** ✅ Correctly sequenced

The four phases have correct dependency ordering:

1. **Phase 1: CLI telemetry** - Can be developed independently, will emit events that the backend doesn't handle yet (graceful degradation)
2. **Phase 2: Backend support** - Depends on knowing the event schema from Phase 1, but doesn't require Phase 1 to deploy
3. **Phase 3: Dashboard** - Depends on Phase 2 (needs /stats/discovery endpoint)
4. **Phase 4: Integration testing** - Depends on all three phases being complete

**Parallelization opportunities:**
- Phase 1 and Phase 2 can be developed in parallel by different people
- Phase 3 can start before Phase 2 deploys (mock the /stats/discovery response)

### 3.2 Missing Steps

**Status:** ⚠️ Minor gaps

**Missing from Phase 1:**
- Unit tests for factory functions (mentioned but not detailed)
- Integration with the chain resolver (instrumentation location not specified)

**Missing from Phase 2:**
- Migration strategy if schema changes after deployment
- Backend unit tests for validation logic

**Missing from Phase 4:**
- Acceptance criteria (how do we know it's working?)
- Load testing (volume estimation)

**Recommended additions:**

**Phase 1 acceptance criteria:**
- Events serialize to correct JSON schema
- SendDiscovery() respects disabled/debug flags
- Factory functions populate all required fields

**Phase 2 acceptance criteria:**
- Backend accepts all six action types
- Validation rejects malformed events with clear error messages
- /stats/discovery returns data in correct format

**Phase 3 acceptance criteria:**
- Dashboard displays all three metrics (stage distribution, top not found, error rate)
- Charts/tables render correctly
- Stats update when new data arrives

**Phase 4 acceptance criteria:**
- End-to-end flow works: CLI emits → backend stores → dashboard displays
- Telemetry doesn't block or slow down install commands
- Debug mode prints events correctly

---

## 4. Simpler Alternatives Analysis

### 4.1 Alternative 1: Reuse Existing Event Struct

**Evaluation:** Correctly rejected

The design considered adding discovery fields to the existing `Event` struct and rejected it because:
- Would add 6+ nullable fields to install/update/remove events
- Standard event blob layout is already at 13 elements with no room

**This analysis is correct.** The existing event struct is specifically for recipe lifecycle events (install/update/remove). Discovery events are conceptually different (they happen *before* a recipe is chosen) and deserve their own struct.

### 4.2 Alternative 2: Generic Event Map

**Evaluation:** Correctly rejected

The design considered using `map[string]interface{}` for all event types and rejected it because:
- Eliminates compile-time type safety
- Complicates backend validation

**This analysis is correct.** Go's type system is a strength, and the factory function pattern provides both type safety and convenience.

### 4.3 Alternative 3: One Event Per Stage

**Evaluation:** Correctly rejected with caveat

The design considered emitting separate events for each stage (registry miss, ecosystem attempt, ecosystem hit) and rejected it because:
- Generates 2-3x event volume
- Single event captures winning stage and total latency

**This is the right call for the MVP**, but there's a **hidden assumption** worth calling out:

If you ever want to answer questions like:
- "How often does ecosystem probe succeed after registry misses?"
- "What's the average latency of the ecosystem probe stage?"
- "How many tools hit the LLM stage (not just succeed)?"

You'll need per-stage data. The single-event approach loses this information.

**Recommendation:** The design should explicitly state this trade-off and mention that per-stage events could be added later as a separate instrumentation if needed.

### 4.4 Alternative 4: Separate Dashboard Page

**Evaluation:** Correctly rejected

The design considered creating `website/stats/discovery/index.html` instead of adding a section to the existing stats page, and rejected it because:
- Current stats page is lightweight
- Discovery stats are closely related to install stats

**This is the right call for MVP**, but there's a **simpler integration pattern** the design overlooked:

**Simpler alternative: Use tabs or collapsible sections**

Instead of adding a new section to the linear page layout, consider:
- Tabs: "Installs" | "Discovery" | "LLM"
- Accordion: Collapse discovery section by default
- Lazy loading: Only fetch /stats/discovery when user expands the section

**Benefits:**
- Page stays fast to load
- Better organization as more stats get added
- User can focus on the metrics they care about

**Recommendation:** Consider this for Phase 3 implementation (doesn't need to be in the design doc).

### 4.5 Overlooked Alternative: Sampling

**Status:** Not considered in design

Discovery events could be sampled (e.g., only emit 10% of events) to reduce backend load, especially as tsuku adoption grows.

**Analysis:**
- Stage distribution and error rates are meaningful even with 10% sampling
- "Top not found" list might need higher sampling rates or full capture
- Sampling could be added later without schema changes

**Recommendation:** Not required for MVP, but worth mentioning as a future scalability option.

---

## 5. Existing Telemetry Pattern Analysis

### 5.1 Event Struct Pattern

**Observed pattern:**
```go
type Event struct { ... }          // Standard lifecycle events
type LLMEvent struct { ... }       // LLM-specific events
type DiscoveryEvent struct { ... } // Discovery-specific events (proposed)
```

**Pattern conformance:** ✅ The design follows this pattern correctly.

### 5.2 Factory Function Pattern

**Observed pattern:**
```go
func newBaseEvent() Event           // Internal helper
func NewInstallEvent(...) Event     // Public constructor
func NewUpdateEvent(...) Event      // Public constructor
func NewRemoveEvent(...) Event      // Public constructor

func newBaseLLMEvent() LLMEvent                        // Internal helper
func NewLLMGenerationStartedEvent(...) LLMEvent        // Public constructor
func NewLLMGenerationCompletedEvent(...) LLMEvent      // Public constructor
// ... etc
```

**Pattern conformance:** ✅ The design mentions "factory functions per action" but doesn't show the signatures. Add the full function signatures to the design doc (see section 2.1 above).

### 5.3 Send Method Pattern

**Observed pattern:**
```go
func (c *Client) Send(event Event)           // For standard events
func (c *Client) SendLLM(event LLMEvent)     // For LLM events
func (c *Client) SendDiscovery(event DiscoveryEvent)  // For discovery events (proposed)
```

All three methods:
- Check `c.disabled` and return early if true
- Check `c.debug` and print to stderr instead of sending if true
- Spawn a goroutine for fire-and-forget sending
- Call `c.sendJSON(event)` which marshals and POSTs

**Pattern conformance:** ✅ The design mentions `SendDiscovery()` but doesn't specify the implementation. Add the full method body to the design doc (see section 2.3 above).

### 5.4 Backend Dispatch Pattern

**Observed pattern (from index.ts:406-558):**

```typescript
// Step 1: Check if it's an LLM event by action prefix
const llmActions: LLMActionType[] = [ ... ];
if (llmActions.includes(event.action)) {
  // Parse as LLMTelemetryEvent
  // Validate with validateLLMEvent()
  // Write to Analytics Engine with LLM blob layout
}

// Step 2: Otherwise treat as standard event
const validActions: ActionType[] = [ ... ];
if (validActions.includes(event.action)) {
  // Parse as TelemetryEvent
  // Validate with validateEvent()
  // Write to Analytics Engine with standard blob layout
}

// Step 3: Reject if neither
return 400 Bad Request
```

**Pattern conformance:** ✅ The design describes adding a third branch for discovery events. This is correct. The backend code will become:

```typescript
// Step 1: Check for LLM events
if (llmActions.includes(event.action)) { ... }

// Step 2: Check for discovery events (NEW)
if (discoveryActions.includes(event.action)) { ... }

// Step 3: Check for standard events
if (validActions.includes(event.action)) { ... }

// Step 4: Reject unknown actions
return 400 Bad Request
```

**Ordering matters:** Discovery check must come before standard events because they share the "install" action space conceptually (discovery leads to install). Put LLM first, discovery second, standard third.

### 5.5 Blob Layout Pattern

**Observed patterns:**

**Standard events (13 blobs):**
```
blob0:  action (install/update/remove/create/command)
blob1:  recipe
blob2:  version_constraint
blob3:  version_resolved
blob4:  version_previous
blob5:  os
blob6:  arch
blob7:  tsuku_version
blob8:  is_dependency
blob9:  command
blob10: flags
blob11: template
blob12: schema_version
```

**LLM events (16 blobs):**
```
blob0:  action (llm_*)
blob1:  provider
blob2:  tool_name
blob3:  repo
blob4:  success
blob5:  duration_ms
blob6:  attempts
blob7:  attempt_number
blob8:  error_category
blob9:  passed
blob10: reason
blob11: failures
blob12: os
blob13: arch
blob14: tsuku_version
blob15: schema_version
```

**Discovery events (13 blobs - proposed):**
```
blob0:  action (discovery_*)
blob1:  tool_name
blob2:  stage
blob3:  confidence
blob4:  builder
blob5:  source
blob6:  match_count
blob7:  error_category
blob8:  duration_ms
blob9:  os
blob10: arch
blob11: tsuku_version
blob12: schema_version
```

**Pattern observation:**
- Common fields (os, arch, tsuku_version, schema_version) are at the end
- Action-specific fields are at the beginning
- Schema version is always the last blob
- Blobs are reused across event types (no collision because queries filter by action first)

**Pattern conformance:** ✅ The design follows this pattern correctly.

**Potential optimization:** Put `duration_ms` earlier in the blob layout (blob5-6 range) to match LLM events. This makes cross-event-type duration queries easier.

### 5.6 Stats Endpoint Pattern

**Observed pattern:**
- `/stats` returns aggregated statistics for standard events (installs by recipe, OS, arch)
- No `/stats/llm` endpoint exists yet (LLM events are collected but not surfaced)

**New pattern proposed:**
- `/stats/discovery` returns aggregated statistics for discovery events

**Pattern conformance:** ✅ The design correctly adds a new endpoint.

**Recommendation:** Consider whether `/stats` should become a gateway that returns:
```json
{
  "installs": { ... },     // Current /stats content
  "discovery": { ... },    // New discovery stats
  "llm": { ... }          // Future LLM stats
}
```

This would require only one HTTP request from the dashboard instead of multiple. However, this is a **future improvement**, not required for the MVP.

---

## 6. Key Recommendations

### 6.1 Required Clarifications (Before Implementation)

1. **Resolve Stage vs Confidence redundancy**
   - Are these always the same value?
   - If yes, remove `Confidence` field
   - If no, document when they differ

2. **Specify instrumentation location**
   - Show exactly where in the code `SendDiscovery()` is called
   - Clarify how to capture timing for stages that weren't attempted

3. **Define error categories**
   - Add table of `ErrorCategory` values for not_found and error actions
   - Map each category to the code condition that triggers it

4. **Add factory function signatures**
   - List all six `NewDiscovery*Event()` functions with full signatures

5. **Add validation rules table**
   - Show which fields are required for each action type
   - Match the format of existing `validateEvent()` logic

### 6.2 Recommended Enhancements (Nice to Have)

1. **Per-stage timing (optional)**
   - If you want to answer "how long did ecosystem probe take?", add:
     ```go
     RegistryMs   int64 `json:"registry_ms,omitempty"`
     EcosystemMs  int64 `json:"ecosystem_ms,omitempty"`
     LLMMs        int64 `json:"llm_ms,omitempty"`
     ```
   - If not, remove mention of "per-stage timing" from design doc

2. **Dashboard integration details**
   - Add wireframe or layout mockup showing where discovery section appears
   - Specify chart types (pie, bar, table) for each metric

3. **Top not found enhancement**
   - Return objects with name+count instead of just names
   - Specify limit (suggest 20 tools)

4. **Sampling consideration**
   - Add note about potential future sampling for scalability
   - Not needed for MVP but worth documenting

### 6.3 Code Quality Suggestions

1. **Add comprehensive unit tests**
   - Test all factory functions
   - Test validation logic for all action types
   - Test that disabled/debug modes work correctly

2. **Add integration test script**
   - Automate the manual test steps described in Phase 4
   - Example:
     ```bash
     # Build CLI with telemetry debug mode
     TSUKU_TELEMETRY_DEBUG=1 ./tsuku install ripgrep 2>&1 | grep discovery_

     # Verify backend accepts events
     curl -X POST http://localhost:8787/event -d '{"action":"discovery_registry_hit",...}'

     # Verify stats endpoint
     curl http://localhost:8787/stats/discovery | jq .
     ```

3. **Consider telemetry testing helper**
   - Add a test-only method to capture events instead of sending:
     ```go
     func (c *Client) CaptureNext() chan DiscoveryEvent // for tests only
     ```

---

## 7. Final Assessment

### Architecture Soundness: ✅ PASS

The design is architecturally sound and follows established patterns. It shows good understanding of the existing codebase and makes appropriate trade-offs.

### Clarity for Implementation: ⚠️ NEEDS CLARIFICATION

The design is 80% clear but needs specifications for:
- Factory functions (signatures)
- Validation rules (table)
- Error categories (enumeration)
- Instrumentation location (code example)
- Dashboard layout (wireframe/description)

### Phase Sequencing: ✅ CORRECT

Implementation phases are properly ordered with clear dependencies. Phases 1-2 can be parallelized.

### Alternative Evaluation: ✅ THOROUGH

The design correctly evaluated and rejected the major alternatives. One missed opportunity (sampling) is not critical for MVP.

---

## 8. Implementation Checklist

Before starting Phase 1, ensure the design doc includes:

- [ ] Factory function signatures (all 6 functions)
- [ ] Error category enumeration (with code mappings)
- [ ] Validation rules table (per action type)
- [ ] Instrumentation location (code snippet showing where to call SendDiscovery)
- [ ] Clarify Stage vs Confidence field usage
- [ ] Decide: include per-stage timing or remove mention from doc
- [ ] Add unit test requirements to Phase 1
- [ ] Add acceptance criteria to all phases
- [ ] Specify top_not_found limit and format
- [ ] Add dashboard layout description or mockup to Phase 3

Once these clarifications are in the design doc, the implementation can proceed confidently.

---

## Appendix A: Blob Layout Collision Analysis

**Question:** Do discovery events need to use blob13-25 to avoid collision with standard events?

**Answer:** No, blob positions can be reused across event types.

**Reasoning:**
Analytics Engine stores all events in the same dataset but doesn't enforce a schema. Queries must:
1. Filter by action prefix first (WHERE blob0 LIKE 'discovery_%')
2. Then read the appropriate blob positions

Example query for discovery stats:
```sql
SELECT blob1 as tool_name, blob2 as stage, count(*) as count
FROM tsuku_telemetry
WHERE blob0 IN ('discovery_registry_hit', 'discovery_ecosystem_hit', 'discovery_llm_hit')
GROUP BY blob1, blob2
```

This only reads blobs from discovery events, even though standard events also use blob0-12. No collision occurs because the WHERE clause separates event types.

**Validation:** This is exactly how LLM events work today (they use blob0-15, standard events use blob0-12, both in same dataset).

---

## Appendix B: Suggested Code Snippet for Instrumentation

**Location:** `internal/discover/chain.go` or a new wrapper in `cmd/tsuku/install.go`

**Option 1: Modify ChainResolver directly**

```go
// In chain.go
func (c *ChainResolver) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
    startTime := time.Now()
    normalized, err := NormalizeName(toolName)
    if err != nil {
        return nil, err
    }

    for _, stage := range c.stages {
        result, err := stage.Resolve(ctx, normalized)
        if err != nil {
            if ctx.Err() != nil || isFatalError(err) {
                // Emit error event before returning
                c.emitTelemetry(toolName, nil, categorizeError(err), time.Since(startTime))
                return nil, err
            }
            log.Default().Warn(fmt.Sprintf("discover: stage error for %q: %v", normalized, err))
            continue
        }
        if result != nil {
            // Emit success event
            c.emitTelemetry(toolName, result, "", time.Since(startTime))
            return result, nil
        }
    }

    // All stages missed
    c.emitTelemetry(toolName, nil, "all_stages_missed", time.Since(startTime))
    return nil, &NotFoundError{Tool: toolName}
}

func (c *ChainResolver) emitTelemetry(toolName string, result *DiscoveryResult, errorCategory string, duration time.Duration) {
    // Inject telemetry client and emit event
    // This would require ChainResolver to hold a telemetry.Client reference
}
```

**Option 2: Wrapper in install.go (cleaner separation)**

```go
// In cmd/tsuku/install.go
func runDiscoveryWithTelemetry(toolName string, client *telemetry.Client) (*discover.DiscoveryResult, error) {
    startTime := time.Now()
    result, err := runDiscovery(toolName)
    duration := time.Since(startTime).Milliseconds()

    if err != nil {
        var notFound *discover.NotFoundError
        if errors.As(err, &notFound) {
            client.SendDiscovery(telemetry.NewDiscoveryNotFoundEvent(toolName, "all_stages_missed", duration))
        } else {
            client.SendDiscovery(telemetry.NewDiscoveryErrorEvent(toolName, categorizeError(err), duration))
        }
        return nil, err
    }

    // Success - emit appropriate hit event based on result.Confidence
    var event telemetry.DiscoveryEvent
    switch result.Confidence {
    case discover.ConfidenceRegistry:
        event = telemetry.NewDiscoveryRegistryHitEvent(toolName, result.Builder, result.Source, duration)
    case discover.ConfidenceEcosystem:
        event = telemetry.NewDiscoveryEcosystemHitEvent(toolName, result.Builder, result.Source, duration)
    case discover.ConfidenceLLM:
        event = telemetry.NewDiscoveryLLMHitEvent(toolName, result.Builder, result.Source, duration)
    }
    client.SendDiscovery(event)

    return result, nil
}
```

**Recommendation:** Use Option 2 (wrapper approach) because:
- Keeps telemetry concerns out of the discover package
- Easier to test (mock the client)
- Matches existing pattern (runInstallWithTelemetry)
