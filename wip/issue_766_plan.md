# Issue 766 Implementation Plan

## Goal

Update the CLI to display system dependency instructions for recipes that have system-level actions.

## Analysis

### Existing Infrastructure

1. **Platform Detection** (`internal/platform/family.go`):
   - `DetectTarget()` - returns `Target{Platform: "linux/amd64", LinuxFamily: "debian"}`
   - `DetectFamily()` - returns linux family string

2. **Plan Filtering** (`internal/executor/filter.go`):
   - `FilterStepsByTarget(steps []recipe.Step, target platform.Target)` - filters steps

3. **Action Description** (`internal/actions/*.go`):
   - All `SystemAction` implementations have `Describe(params map[string]interface{}) string`
   - Returns copy-pasteable shell commands

4. **CLI Install Flow** (`cmd/tsuku/install.go`, `cmd/tsuku/install_deps.go`):
   - Entry point is `installCmd`
   - Main logic in `installWithDependencies()`
   - Recipe loaded via `loader.Get(toolName)`

### Implementation Strategy

The issue requires displaying system dependency instructions when a recipe has system-level actions. This should happen during the `install` command when system dependencies are detected.

**New components needed:**

1. **System Deps Display** (`cmd/tsuku/sysdeps.go`):
   - Function to check if a recipe has system dependency steps
   - Function to format and display instructions grouped by category
   - Integration with `--quiet` flag
   - `--target-family` flag for override

2. **Family Name Mapping** for human-readable headers:
   - "debian" -> "For Ubuntu/Debian"
   - "rhel" -> "For Fedora/RHEL"
   - etc.

### User Documentation

Per user request, add documentation explaining:
- What system dependencies are
- How to interpret the output
- How to use `--target-family` flag

## Implementation

### 1. Add `--target-family` flag to install command

```go
// In install.go init()
installCmd.Flags().StringVar(&installTargetFamily, "target-family", "", "Override detected linux_family (debian, rhel, arch, alpine, suse)")
```

### 2. Create `cmd/tsuku/sysdeps.go`

Functions:
- `hasSystemDeps(recipe) bool` - check if recipe has system dependency steps
- `displaySystemDeps(recipe, target) error` - format and display instructions
- `familyDisplayName(family string) string` - human-readable family names

### 3. Integration Point

In `installWithDependencies()`, after loading recipe and before proceeding with installation:
1. Detect target (or use override)
2. Filter steps for target
3. If system deps exist and not quiet, display instructions
4. Continue with installation (or exit if only system deps)

### 4. Output Format

```
This recipe requires system dependencies:

For Ubuntu/Debian:
  sudo apt-get install -y docker-ce docker-ce-cli containerd.io
  sudo usermod -aG docker $USER

After installing dependencies, run this command again.
```

### 5. Documentation

Add `docs/system-dependencies.md` explaining:
- What system dependencies are
- When they appear
- How to use `--target-family`
- Examples

## File Changes

| File | Change |
|------|--------|
| `cmd/tsuku/install.go` | Add `--target-family` flag |
| `cmd/tsuku/sysdeps.go` | New file with system deps display logic |
| `cmd/tsuku/install_deps.go` | Integrate system deps check |
| `docs/system-dependencies.md` | New user documentation |

## Acceptance Criteria Mapping

- [x] CLI detects current platform and linux_family via `DetectTarget()` (exists)
- [x] Filters recipe steps for current platform+linux_family (exists)
- [x] Uses `Describe()` to generate instructions (exists)
- [ ] Groups instructions by family header
- [ ] Shows `require_command` verification at the end
- [ ] Respects `--quiet` flag
- [ ] `--target-family` flag to override detected family
- [ ] Integration tests for instruction display
