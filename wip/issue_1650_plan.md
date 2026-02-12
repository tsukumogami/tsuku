# Issue 1650 Implementation Plan

## Goal

Add `ConfirmDisambiguationFunc` callback type for interactive disambiguation prompts.

## Analysis

### Existing Patterns

The `ConfirmFunc` pattern in `llm_discovery.go`:
```go
type ConfirmFunc func(result *DiscoveryResult) bool

type LLMDiscovery struct {
    confirm ConfirmFunc
}

func WithConfirmFunc(fn ConfirmFunc) LLMDiscoveryOption {
    return func(d *LLMDiscovery) {
        d.confirm = fn
    }
}
```

### Current State from #1648

- `disambiguate()` returns `AmbiguousMatchError` for close matches
- `DiscoveryMatch` struct exists with required fields
- `toDiscoveryMatches()` helper converts probeOutcome to DiscoveryMatch

### Key Decision: ProbeMatch vs DiscoveryMatch

The issue specifies `ProbeMatch` as a new type. However, `DiscoveryMatch` already has the exact same fields. Options:

1. **Create ProbeMatch** (as specified) - clearer semantics, callback input is distinct from error output
2. **Reuse DiscoveryMatch** - avoids duplication, callback uses existing type

**Decision**: Follow the spec and create `ProbeMatch`. The semantic separation is valuable: `ProbeMatch` is callback input for user selection, `DiscoveryMatch` is error output for non-interactive mode. They serve different purposes even if fields happen to match.

## Implementation Steps

### Step 1: Define Types (resolver.go)

Add to `resolver.go` after `DiscoveryMatch`:

```go
// ProbeMatch represents a single ecosystem match for interactive disambiguation.
// Used as callback input to ConfirmDisambiguationFunc.
type ProbeMatch struct {
    Builder       string // e.g., "crates.io", "npm"
    Source        string // e.g., "sharkdp/bat", "bat-cli"
    Downloads     int    // Monthly downloads (0 if unavailable)
    VersionCount  int    // Number of published versions
    HasRepository bool   // Whether package has linked source repo
}

// ConfirmDisambiguationFunc prompts the user to select from multiple matches.
// Returns the selected index (0-based) or an error if cancelled.
type ConfirmDisambiguationFunc func(matches []ProbeMatch) (int, error)
```

### Step 2: Add Field to EcosystemProbe (ecosystem_probe.go)

Add callback field to struct:
```go
type EcosystemProbe struct {
    probers              []builders.EcosystemProber
    timeout              time.Duration
    priority             map[string]int
    filter               *QualityFilter
    confirmDisambiguation ConfirmDisambiguationFunc // optional callback for interactive mode
}
```

### Step 3: Add Option Function (ecosystem_probe.go)

Follow the same pattern as LLM discovery:
```go
// EcosystemProbeOption configures an EcosystemProbe.
type EcosystemProbeOption func(*EcosystemProbe)

// WithConfirmDisambiguation sets a callback for interactive disambiguation.
func WithConfirmDisambiguation(fn ConfirmDisambiguationFunc) EcosystemProbeOption {
    return func(p *EcosystemProbe) {
        p.confirmDisambiguation = fn
    }
}
```

Update `NewEcosystemProbe` to accept options:
```go
func NewEcosystemProbe(probers []builders.EcosystemProber, timeout time.Duration, opts ...EcosystemProbeOption) *EcosystemProbe {
    p := &EcosystemProbe{...}
    for _, opt := range opts {
        opt(p)
    }
    return p
}
```

### Step 4: Add Helper Function (disambiguate.go)

Add conversion helper after `toDiscoveryMatches`:
```go
// toProbeMatches converts probeOutcomes to ProbeMatches for callback display.
func toProbeMatches(matches []probeOutcome) []ProbeMatch {
    result := make([]ProbeMatch, len(matches))
    for i, m := range matches {
        result[i] = ProbeMatch{
            Builder:       m.builderName,
            Source:        m.result.Source,
            Downloads:     m.result.Downloads,
            VersionCount:  m.result.VersionCount,
            HasRepository: m.result.HasRepository,
        }
    }
    return result
}
```

### Step 5: Update disambiguate Signature (disambiguate.go)

Modify `disambiguate` to accept the callback:
```go
func disambiguate(toolName string, matches []probeOutcome, priority map[string]int, confirm ConfirmDisambiguationFunc) (*DiscoveryResult, error)
```

And invoke callback when available before returning AmbiguousMatchError:
```go
// No clear winner: try interactive disambiguation if callback available
if confirm != nil {
    probeMatches := toProbeMatches(matches)
    selected, err := confirm(probeMatches)
    if err != nil {
        return nil, err
    }
    return toDiscoveryResult(matches[selected]), nil
}

// Non-interactive: return ambiguous error
return nil, &AmbiguousMatchError{...}
```

### Step 6: Update Resolve Call (ecosystem_probe.go)

Pass the callback to disambiguate:
```go
return disambiguate(toolName, matches, p.priority, p.confirmDisambiguation)
```

### Step 7: Unit Tests (disambiguate_test.go)

Add tests:
1. Callback invoked with correct ProbeMatch data
2. Callback selection is honored (returns correct result)
3. Callback error propagates correctly
4. Nil callback falls through to AmbiguousMatchError

## Files to Modify

1. `internal/discover/resolver.go` - Add ProbeMatch and ConfirmDisambiguationFunc types
2. `internal/discover/ecosystem_probe.go` - Add field, option function, update constructor
3. `internal/discover/disambiguate.go` - Add toProbeMatches, update signature, invoke callback
4. `internal/discover/disambiguate_test.go` - Add callback tests
5. `internal/discover/ecosystem_probe_test.go` - Update any affected test calls

## Testing Strategy

1. Unit test the callback invocation with mock function
2. Test index bounds validation
3. Test error propagation from callback
4. Verify existing tests still pass (no callback = same behavior)
