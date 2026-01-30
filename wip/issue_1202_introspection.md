# Issue #1202 Introspection: Queue Seed Script for Homebrew

**Date**: 2026-01-29
**Issue**: [#1202](https://github.com/tsukumogami/tsuku/issues/1202) - feat(scripts): add queue seed script for Homebrew
**Milestone**: M50: Visibility Infrastructure Schemas
**Age**: 1 day

## Staleness Signals

- **3 sibling issues closed** since creation (#1199, #1200, #1201)
- **Milestone position**: middle (issue 4 of 5)
- **1 referenced file modified**: docs/designs/DESIGN-priority-queue.md

## Current State Assessment

### Dependencies (All Met)
- ✅ #1199: Priority queue schema exists at `data/schemas/priority-queue.schema.json`
- ✅ Example file exists at `data/examples/priority-queue.json`
- ✅ Validation script exists at `scripts/validate-queue.sh`
- ✅ Dependency mapping exists at `data/dep-mapping.json`

### Schema Validation
The priority queue schema has been implemented with all required fields:
- `id` (pattern: `^[a-z0-9_-]+:[a-z0-9_@.+-]+$`)
- `source`, `name`, `tier`, `status`, `added_at`, `metadata`
- `status` enum: pending, in_progress, success, failed, skipped
- `tier` range: 1-3

### Existing Infrastructure
- **Recipe count**: 17 recipes currently exist
- **Validation pattern**: `scripts/validate-queue.sh` uses `pipx run check-jsonschema`
- **Sister script**: `scripts/validate-failures.sh` follows same pattern

### Homebrew API Investigation
The Homebrew API provides:
- **Formula API**: `https://formulae.brew.sh/api/formula.json` (too large, 10MB+)
- **Individual formula**: `https://formulae.brew.sh/api/formula/<name>.json`
- **Analytics API**: `https://formulae.brew.sh/api/analytics/install-on-request/30d.json`

Analytics data format:
```json
{
  "number": 1,
  "formula": "awscli",
  "count": "218,640",
  "percent": "2.73"
}
```

## Gap Analysis

### 1. Tier 1 Curation List Not Specified

**Issue**: The design doc mentions "tier 1 = hardcoded curation list" but doesn't specify which packages.

**Evidence**:
- Issue body says "tier 1 = hardcoded curation list"
- Design doc (line 241): "tier 1 (critical): Top 100 most-requested tools (manual curation)"
- No curation list exists in the codebase
- Current recipes (17 tools) could be candidates: cmake, gcc-libs, go, ninja, nodejs, openssl, patchelf, perl, pkg-config, python-standalone, ruby, rust, zig, zlib, etc.

**Impact**: Script cannot assign tier 1 without knowing which formulas to prioritize.

**Recommendation**: Either:
- Hardcode an initial tier 1 list based on current recipes + top Homebrew analytics
- Make tier 1 assignment a manual process (all seeded packages start as tier 2/3)
- Reference existing recipes as tier 1 candidates

### 2. Download Count Threshold for Tier 2 Needs Clarification

**Issue**: Design says "tier 2 = >10K weekly downloads" but analytics data shows monthly counts.

**Evidence**:
- Issue body: "tier 2 = >10K weekly downloads"
- Design doc (line 242): "tier 2 (popular): >10K weekly downloads or >1K GitHub stars"
- Analytics API provides 30-day counts (e.g., awscli: 218,640 installs/30d)
- Need to clarify: 10K/week = ~43K/month?

**Impact**: Script may incorrectly assign tiers if threshold calculation is wrong.

**Recommendation**: Clarify whether to:
- Convert monthly analytics to weekly average (divide by 4.3)
- Use a monthly threshold instead (adjust design to match API)

### 3. Metadata Structure Not Fully Defined

**Issue**: The schema allows optional `metadata` object but doesn't specify what Homebrew-specific fields to include.

**Evidence**:
- Schema line 60-63: `"metadata": { "type": "object", "description": "Source-specific data (optional)" }`
- Example shows: `"metadata": { "formula": "ripgrep", "tap": "homebrew/core" }`
- Homebrew API provides: name, desc, homepage, bottle availability, dependencies

**Impact**: Script implementer must decide what metadata to capture.

**Recommendation**: Specify minimum Homebrew metadata fields:
- `formula` (name in Homebrew)
- `tap` (default to "homebrew/core")
- `has_bottles` (boolean for validation readiness)
- `homepage` (optional, for context)

### 4. Rate Limit Handling Strategy Not Specified

**Issue**: Issue body says "handles rate limits with retry/backoff" but doesn't specify parameters.

**Evidence**:
- Issue acceptance criteria mentions rate limits but no specifics
- Homebrew API rate limits are not documented in issue
- No guidance on retry count, backoff strategy, or timeout

**Impact**: Script may be too aggressive (gets blocked) or too conservative (runs slowly).

**Recommendation**: Specify:
- Initial delay (e.g., 1 second)
- Backoff multiplier (e.g., 2x)
- Max retries (e.g., 3)
- Total timeout (e.g., 5 minutes)

### 5. Design Document Changes Since Issue Creation

**Finding**: The design doc was modified 3 times since issue creation:
- 2026-01-28: #1199 (schemas) marked done
- 2026-01-29: #1200 (dep-mapping) marked done
- 2026-01-29: #1201 (validation scripts) marked done

**Impact**: Issue body is still accurate. Design doc updates were tracking changes, not modifying requirements.

**Status**: No issue body changes needed.

## Implementation Readiness

### What's Clear
✅ Script location: `scripts/seed-queue.sh`
✅ Flags: `--source homebrew`, `--limit N`
✅ Output location: `data/priority-queue.json`
✅ Output schema: fully defined in `data/schemas/priority-queue.schema.json`
✅ Validation: can use existing `scripts/validate-queue.sh` pattern
✅ Homebrew analytics endpoint: `install-on-request/30d.json`
✅ Dependencies: all met (#1199)

### What Needs Clarification
⚠️ **Tier 1 curation list**: Which formulas are tier 1?
⚠️ **Tier 2 threshold**: Weekly vs monthly download conversion
⚠️ **Metadata fields**: Which Homebrew-specific fields to capture
⚠️ **Rate limit params**: Retry count, backoff strategy, timeout

### What's Blocked
❌ None - all dependencies are met

## Recommendations

### Option A: Proceed with Sensible Defaults (Recommended)

The missing details are implementation choices that don't block progress:

1. **Tier 1**: Use existing 17 recipes as initial curation, expand based on analytics top 20
2. **Tier 2**: Convert monthly analytics to weekly (count / 4.3), apply >10K threshold
3. **Metadata**: Include `formula`, `tap`, `has_bottles`, `homepage`
4. **Rate limits**: 1s initial delay, 2x backoff, 3 retries, 5min timeout

**Rationale**: These are reasonable defaults that align with design intent. Can be refined later without breaking the schema.

### Option B: Clarify Before Implementation

Pause implementation to get explicit guidance from issue author on:
- Exact tier 1 formula list
- Weekly vs monthly threshold calculation
- Required metadata fields
- Rate limit parameters

**Rationale**: Ensures script matches exact intent, but delays milestone progress.

## Recommendation: **Proceed with Option A**

**Key finding**: All dependencies are met. The gaps are implementation details (curation list, threshold conversion, metadata fields) that can use sensible defaults without risking schema compatibility.

**Blocking concerns**: None. The schema is stable, validation scripts exist, and the Homebrew API is accessible.

## Next Steps

1. Implement seed script with defaults from Option A
2. Add validation test using `scripts/validate-queue.sh`
3. Document tier 1 curation logic in script comments
4. Test with `--limit 10` to validate end-to-end flow
5. Update design doc if implementation reveals refinements needed
