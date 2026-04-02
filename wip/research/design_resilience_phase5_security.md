# Security Review: resilience

## Dimension Analysis

### External Artifact Handling
**Applies:** No. No new external inputs. GC and failure tracking operate on locally-created files.

### Permission Scope
**Applies:** Minimal. GC deletes directories in user-owned $TSUKU_HOME/tools/. The deletion path validates directory names match the tool-version naming convention before removing. No new permissions required.

### Supply Chain or Dependency Trust
**Applies:** No. No new artifact sources or dependencies.

### Data Exposure
**Applies:** No. ConsecutiveFailures counter is a local integer in a user-owned JSON file.

## Recommended Outcome
**OPTION 3 - N/A with justification.** GC path should validate directory names before deletion to prevent path traversal via crafted tool names, but tool names are already validated at install time. No new security surface.

## Summary
Hardening changes with minimal security surface. GC operates within $TSUKU_HOME/tools/ on directories tsuku created. Failure counting is a local counter in existing notice files.
