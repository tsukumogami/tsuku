# M57 Documentation Gap Analysis

## Executive Summary

**Status:** FINDINGS
**Finding Count:** 4
**Severity:** Medium (missing user-facing docs) + Low (missing internal docs)

Milestone M57 (Visibility Infrastructure Schemas) added significant infrastructure for batch recipe generation:
- JSON schemas (priority-queue, failure-record)
- Example data files
- Dependency mapping structure
- Four validation/utility scripts

While the design document and data/README.md cover these features, there are notable documentation gaps in user-facing and developer-facing areas.

---

## Milestone M57 Deliverables

### What Was Added

Based on git log analysis and file inspection:

1. **JSON Schemas** (#1199)
   - `data/schemas/priority-queue.schema.json` - Queue structure with tiered priority
   - `data/schemas/failure-record.schema.json` - Per-ecosystem-environment failure records
   - `data/examples/priority-queue.json` - Example valid queue
   - `data/examples/failure-record.json` - Example valid failure records

2. **Dependency Mapping** (#1200)
   - `data/dep-mapping.json` - Maps ecosystem dep names to tsuku recipe names

3. **Validation Scripts** (#1201)
   - `scripts/validate-queue.sh` - Validates priority queue against schema
   - `scripts/validate-failures.sh` - Validates failure files against schema

4. **Utility Scripts** (#1202, #1203)
   - `scripts/seed-queue.sh` - Populates queue from Homebrew API
   - `scripts/gap-analysis.sh` - Queries failure data for blocked packages

### Design Documentation

**DESIGN-priority-queue.md** (676 lines) comprehensively documents:
- Problem statement and context
- Decision rationale (single file, tiered classification, latest-only failures)
- Schema structure with field definitions
- Data flow and file locations
- Security considerations
- Implementation approach with script descriptions

The design doc is thorough and complete for its scope.

---

## Documentation Coverage Analysis

### 1. Root README.md

**Current Status:** No mention of M57 features

**Gap:** The main README.md does not reference:
- Priority queue infrastructure
- Failure record tracking
- Data schemas location
- Validation scripts

**Impact:** Medium

**Rationale:** The root README targets end users installing tools. M57's visibility infrastructure is internal tooling for batch recipe generation. However, if users will eventually interact with these features (e.g., viewing queue status, checking failure reasons), the README should at least acknowledge their existence.

**Recommendation:**
- Add a "Batch Recipe Generation" or "Registry Scaling Infrastructure" section
- Briefly mention that tsuku tracks package priority and generation failures
- Link to data/README.md for details

---

### 2. data/README.md

**Current Status:** Comprehensive coverage of schemas and dep-mapping

**Coverage:**
- Documents `dep-mapping.json` structure and purpose
- Documents both JSON schemas (priority-queue, failure-record)
- Explains example files location
- Describes maintenance workflow for dep-mapping

**Strengths:**
- Clear structure with examples
- Explains supply chain control point (code review)
- Links schema validation to data files

**Minor Gap:** Does not mention the validation scripts

**Impact:** Low

**Recommendation:**
- Add a "Validation" section mentioning `scripts/validate-queue.sh` and `scripts/validate-failures.sh`
- Example:
  ```markdown
  ## Validation

  Schemas are validated in CI using:
  - `scripts/validate-queue.sh` - Validates data/priority-queue.json
  - `scripts/validate-failures.sh` - Validates data/failures/*.json
  ```

---

### 3. scripts/README.md

**Current Status:** Does not exist

**Gap:** The `scripts/` directory contains 27 scripts but has no README documenting their purpose, usage, or organization.

**Impact:** Medium

**Rationale:** M57 added 4 new scripts:
- `validate-queue.sh`
- `validate-failures.sh`
- `seed-queue.sh`
- `gap-analysis.sh`

Without a scripts/README.md, developers cannot easily discover:
- What scripts exist
- When to use each script
- Which scripts are developer tools vs. CI-only

**Recommendation:**
Create `scripts/README.md` with:
- Overview of script categories (validation, generation, infrastructure)
- Table of scripts with one-line descriptions
- Section for M57 visibility infrastructure scripts:
  - validate-queue.sh - Validates priority queue schema
  - validate-failures.sh - Validates failure records schema
  - seed-queue.sh - Populates queue from Homebrew API
  - gap-analysis.sh - Queries blocked packages by dependency

---

### 4. CI Workflow Integration

**Current Status:** No CI workflow validates schemas

**Gap Analysis:**
- Searched `.github/workflows/` for references to:
  - `validate-queue`
  - `validate-failures`
  - `priority-queue`
  - `failure-record`
- **Result:** No matches found

**Impact:** Medium

**Rationale:**
- Issue #1201 created validation scripts explicitly for CI validation
- Design doc states: "The schemas are complete when CI can parse and validate queue entries without manual intervention"
- Without CI integration, schema validation is manual and error-prone

**Expected CI Integration:**
Either a dedicated workflow (e.g., `.github/workflows/validate-data-schemas.yml`) or integration into an existing workflow (e.g., `test.yml`) that runs:
```yaml
- name: Validate priority queue schema
  run: ./scripts/validate-queue.sh

- name: Validate failure records schema
  run: ./scripts/validate-failures.sh
```

**Recommendation:**
This is a **functional gap**, not just documentation. The validation scripts exist but are not invoked by CI. This should be addressed in a follow-up issue.

For documentation purposes:
- Once CI integration exists, document it in data/README.md
- Add CI badge to data/README.md showing validation status

---

### 5. Developer Guides

**Current Status:** No guide mentions M57 features

**Gap:** Existing guides in `docs/` do not reference:
- Priority queue workflow
- Failure record analysis
- Using gap-analysis.sh for dependency planning

**Relevant Guides:**
- No "GUIDE-batch-generation.md" exists
- No "GUIDE-registry-scaling.md" exists

**Impact:** Low (for now)

**Rationale:**
- M57 is Phase 0 infrastructure
- Downstream batch generation (Phase 1+) is not yet implemented
- No users are currently interacting with these features

**Future Recommendation:**
When batch generation is implemented (issues #1188, #1189 from DESIGN-priority-queue.md), create:
- `docs/GUIDE-batch-recipe-generation.md` covering:
  - Queue management workflow
  - Interpreting failure records
  - Using gap-analysis.sh to prioritize dependency recipes
  - Re-queue triggers (Phase 2)

---

## Findings Summary

| # | Finding | Severity | Affected File(s) | Recommendation |
|---|---------|----------|------------------|----------------|
| 1 | No mention of visibility infrastructure in root README | Medium | README.md | Add section acknowledging batch generation infrastructure |
| 2 | data/README.md does not mention validation scripts | Low | data/README.md | Add "Validation" section |
| 3 | scripts/README.md does not exist | Medium | scripts/README.md (missing) | Create README documenting all scripts |
| 4 | CI workflows do not validate schemas | Medium | .github/workflows/ | Add CI job for schema validation (functional issue, not just docs) |

---

## Files Analyzed

### Documentation Files
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/README.md` (618 lines)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/data/README.md` (44 lines)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-priority-queue.md` (676 lines)
- `scripts/README.md` (does not exist)

### Data Files
- `data/schemas/priority-queue.schema.json` (70 lines)
- `data/schemas/failure-record.schema.json` (68 lines)
- `data/examples/priority-queue.json` (44 lines)
- `data/examples/failure-record.json` (28 lines)
- `data/dep-mapping.json` (37 lines)

### Scripts
- `scripts/validate-queue.sh` (34 lines)
- `scripts/validate-failures.sh` (50 lines)
- `scripts/seed-queue.sh` (header inspected, ~200+ lines total)
- `scripts/gap-analysis.sh` (header inspected, ~100+ lines total)

### CI Workflows
- Searched 37 workflow files in `.github/workflows/`
- No references to M57 validation scripts found

---

## Recommendations Priority

**High Priority:**
1. Create `scripts/README.md` documenting all scripts including M57 additions
2. Integrate schema validation scripts into CI (functional gap)

**Medium Priority:**
3. Add visibility infrastructure section to root README.md
4. Add validation scripts reference to data/README.md

**Low Priority (Future Work):**
5. Create batch generation guide when Phase 1 is implemented

---

## Conclusion

Milestone M57 delivered complete design documentation (DESIGN-priority-queue.md) and adequate data-level documentation (data/README.md). The primary gaps are:

1. **Missing scripts/README.md** - High-value addition for developer onboarding
2. **No CI integration** - Functional gap that undermines the purpose of validation scripts
3. **Root README silence** - Users have no visibility into registry scaling work

These gaps do not block downstream milestones but should be addressed for maintainability and transparency.
