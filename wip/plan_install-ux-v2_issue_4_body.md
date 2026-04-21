---
complexity: testable
complexity_rationale: Reclassifying ~20 reporter.Log() calls across 5 action files — wrong classification silently changes CI output; needs test coverage per the scoped test plan
---

## Goal

Convert or remove ~20 `reporter.Log()` calls across five action files per the Decision 4 classification table from the design. Extraction/command-in-progress calls become `reporter.Status(fmt.Sprintf(...))`. Per-file loop lines (individual "Linked:", "Installed:", "Already linked:", "Installed symlink:") are removed entirely. Bulk-count milestones, command output, retry notices, and skip notices remain as `reporter.Log()`.

## Files and classification

**extract.go**
- `"   Extracting: %s"` → `reporter.Status(fmt.Sprintf(...))`
- `"   Format: %s"` → remove
- `"   Strip dirs: %d"` → remove

**run_command.go**
- `"   Running: %s"` → `reporter.Status(fmt.Sprintf(...))`
- `"   Description: %s"` (both sites) → remove
- `"   Working dir: %s"` → remove
- `"   Command executed successfully"` → remove
- `"   Output: %s"` → keep as `reporter.Log()`
- `"   Skipping (requires sudo): %s"` → keep as `reporter.Log()`

**install_binaries.go**
- `"   Installing directory tree to: %s"` → `reporter.Status(fmt.Sprintf(...))`
- `"   Copying directory tree..."` → `reporter.Status(...)`
- `"   Installed (executable): %s -> %s"` per-file → remove
- `"   Installed: %s -> %s"` per-file → remove
- `"   Installing %d file(s)"` and `"   Directory tree copied..."` bulk lines → keep as `reporter.Log()` or consolidate

**install_libraries.go**
- `"   Installed symlink: %s"` per-file → remove
- `"   Installed: %s"` per-file → remove
- `"   Installing %d library file(s)"` bulk count → keep as `reporter.Log()`

**link_dependencies.go**
- `"   Linked: %s"`, `"   Linked (symlink): %s"`, `"   Already linked: %s"` per-file → remove
- `"   Linking %d library file(s) from %s"` bulk count → keep as `reporter.Log()`

## Acceptance Criteria

- All per-file loop lines listed above are removed from action files
- Extraction/command-in-progress lines use `reporter.Status(fmt.Sprintf(...))`, not `reporter.Log()`
- Bulk-count milestone lines ("Linking N library file(s)", "Installing N file(s)") still appear in `reporter.Log()`
- Command output ("Output: ..."), retry notices, and sudo-skip notices still appear in `reporter.Log()`
- `go test ./...` passes with no new failures

## Dependencies

None
