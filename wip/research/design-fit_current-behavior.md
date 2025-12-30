# Assessment: require_system Current Behavior vs Design Implications

## Executive Summary

The `require_system` action today is purely a **validation/verification action** - it does NOT execute system installations. It checks if a command exists, optionally validates its version, and provides human-readable installation guidance if the check fails. The design in `DESIGN-system-dependency-actions.md` proposes a fundamentally different model: actual execution of `apt_install`, `brew_cask`, etc.

**Key finding:** The design conflates two distinct concerns:
1. **Verification** (what `require_system` does today)
2. **Installation** (what the new `apt_install`, `brew_cask` actions would do)

For host execution, tsuku's current behavior of "tell users what to do" should be preserved. The design's installation actions should only apply in sandbox mode.

---

## How require_system Currently Works

### Implementation Details

Location: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/require_system.go`

The `RequireSystemAction` performs a **read-only validation**:

```go
func (a *RequireSystemAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // Step 1: Check if command exists via exec.LookPath()
    cmdPath, err := exec.LookPath(command)
    if err != nil {
        // Return SystemDepMissingError with install_guide
        return &SystemDepMissingError{Command: command, InstallGuide: guide}
    }

    // Step 2: Optionally check version via running command with version_flag
    if versionFlag != "" && versionRegex != "" {
        versionStr, err := detectVersion(command, versionFlag, versionRegex)
        // ...
    }

    // Step 3: Compare against min_version if specified
    if minVersion != "" && !versionSatisfied(versionStr, minVersion) {
        return &SystemDepVersionError{...}
    }

    return nil // Success - dependency satisfied
}
```

### What Happens When require_system Runs

1. **On host (normal installation):**
   - Checks if command exists in PATH
   - If missing: Returns `SystemDepMissingError` with platform-specific `install_guide`
   - If version mismatch: Returns `SystemDepVersionError`
   - Installation STOPS with an error message containing the guide
   - **tsuku does NOT attempt to install anything**

2. **In sandbox:**
   - Same behavior - checks for command in sandbox's PATH
   - Will typically fail since sandboxes don't have system tools
   - Error propagates to sandbox execution framework

3. **CLI output on failure:**
   ```
   required system dependency not found: docker

   Installation guide:
   brew install --cask docker
   ```

### How install_guide is Displayed

When `require_system` fails, it returns a structured error type that includes the installation guidance:

```go
type SystemDepMissingError struct {
    Command      string
    InstallGuide string  // Platform-specific guide from recipe
}

func (e *SystemDepMissingError) Error() string {
    msg := fmt.Sprintf("required system dependency not found: %s", e.Command)
    if e.InstallGuide != "" {
        msg += fmt.Sprintf("\n\nInstallation guide:\n%s", e.InstallGuide)
    }
    return msg
}
```

The error message is displayed directly to the user via stderr. The CLI does not interpret or process the guide - it's free-form text passed through.

### Platform Selection for install_guide

The `getPlatformGuide()` function implements hierarchical lookup:

1. Exact platform tuple (e.g., `darwin/arm64`)
2. OS-only key (e.g., `darwin`)
3. Fallback key (`fallback`)

This allows recipes like:

```toml
[steps.install_guide]
"darwin/arm64" = "brew install docker (Apple Silicon optimized)"
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
fallback = "Visit https://docker.com"
```

---

## Current Recipe Usage

### docker.toml

```toml
[[steps]]
action = "require_system"
command = "docker"
version_flag = "--version"
version_regex = "Docker version ([0-9.]+)"

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/ for platform-specific installation"
fallback = "Visit https://docs.docker.com/get-docker/ for installation instructions"
```

### cuda.toml

```toml
[[steps]]
action = "require_system"
command = "nvcc"
version_flag = "--version"
version_regex = "release ([0-9.]+)"
min_version = "11.0"

[steps.install_guide]
darwin = "CUDA is not supported on macOS. Consider using cloud GPU instances or Linux."
linux = "Visit https://developer.nvidia.com/cuda-downloads for platform-specific installation"
fallback = "CUDA requires NVIDIA GPU drivers and cannot be installed via tsuku. See https://developer.nvidia.com/cuda-toolkit"
```

**Observation:** Both recipes use free-form text in `install_guide`, not structured commands. They're providing human guidance, not machine-executable instructions.

---

## Related CLI Commands

### check-deps Command

Location: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/check_deps.go`

Checks dependencies without attempting installation:

```go
func checkSystemDependency(r *recipe.Recipe, status DepStatus) DepStatus {
    // Uses exec.LookPath and version detection
    // Returns status: "installed", "missing", or "version_mismatch"
    // Includes InstallGuide for display
}
```

Output example:
```
Dependencies for my-tool

System Dependencies (require external installation):
  docker              missing
      brew install --cask docker

Provisionable Dependencies (managed by tsuku):
  nodejs              installed (18.17.0)
```

### System Dependency Detection During Install

Location: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/install_deps.go`

```go
// isSystemDependencyPlan returns true if the plan only contains require_system steps.
func isSystemDependencyPlan(plan *executor.InstallationPlan) bool {
    for _, step := range plan.Steps {
        if step.Action != "require_system" {
            return false
        }
    }
    return true
}
```

When a plan is system-dependency-only:
- tsuku validates the dependencies exist
- Does NOT create state entries or tool directories
- Prints: "tsuku doesn't manage this dependency. It validated that it's installed."

---

## System Package Actions (Stubs)

Location: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/system_packages.go`

There are stub implementations for `apt_install`, `yum_install`, `brew_install` that explicitly DO NOT execute:

```go
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    packages, ok := GetStringSlice(params, "packages")
    // ...
    fmt.Printf("   Would install via apt: %v\n", packages)
    fmt.Printf("   (Skipped - requires sudo and system modification)\n")
    return nil
}
```

**These are intentional no-ops.** They exist for:
1. Recipe validation (checking the `packages` parameter exists)
2. Plan generation (showing what would be installed)
3. Sandbox dry-run capability

---

## Gap Analysis: Design vs Reality

### What the Design Implies

From `DESIGN-system-dependency-actions.md`:

1. Replace `require_system` with composable actions (`apt_install`, `brew_cask`, etc.)
2. These actions would actually execute package installations
3. Add consent flow for privileged operations
4. Enable host execution in "Phase 4"

### What Should Actually Change

**Preserve current behavior for host execution:**
- `require_system` (or `require_command`) validates but does NOT install
- Installation guidance displayed to users
- Users run commands manually

**Add sandbox execution:**
- In sandbox mode, `apt_install`, `brew_cask`, etc. execute in the container
- Golden plan testing can verify recipes work end-to-end
- Container provides isolation for privileged operations

**Split the concern:**
1. **Verification actions** (`require_command`): Always run, check existence
2. **Installation actions** (`apt_install`, etc.): Only run in sandbox mode

---

## Concrete Changes Needed

### Minimal Changes (Preserve Current Behavior)

1. **Rename `require_system` to `require_command`:**
   - Clearer name - it requires a command to exist
   - Remove `install_guide` parameter (move to `manual` action)
   - Keep version detection capability

2. **Add `manual` action for installation guidance:**
   ```toml
   [[steps]]
   action = "manual"
   text = "brew install --cask docker"
   when = { os = ["darwin"] }
   ```
   - Displayed to user when reached
   - Can be platform-filtered with `when` clause

3. **Keep existing stub actions as-is:**
   - `apt_install`, `brew_install`, etc. remain stubs for validation
   - Print "would install" message
   - No actual execution on host

### Sandbox-Only Additions

4. **Implement sandbox execution for install actions:**
   - When `ctx.IsSandbox == true`, actually run `apt-get install`, `brew install`, etc.
   - Sandbox provides isolation
   - Enables golden plan testing

5. **No consent flow needed for sandbox:**
   - Container is ephemeral
   - User already consented by running sandbox mode

---

## Revised Action Vocabulary

| Action | Host Behavior | Sandbox Behavior |
|--------|---------------|------------------|
| `require_command` | Check command exists, fail with error if not | Same |
| `manual` | Print text to user | Same (or skip in automated tests) |
| `apt_install` | Print "would install" (stub) | Execute `apt-get install` |
| `brew_install` | Print "would install" (stub) | Execute `brew install` |
| `brew_cask` | Print "would install" (stub) | Execute `brew install --cask` |
| `dnf_install` | Print "would install" (stub) | Execute `dnf install` |
| `group_add` | Print "would add" (stub) | Execute `usermod -aG` |
| `service_enable` | Print "would enable" (stub) | Execute `systemctl enable` |

---

## Impact on Design Document

The design doc should be updated to clarify:

1. **Execution context matters:**
   - Host: Verification only, no automatic installation
   - Sandbox: Full execution capability

2. **User experience unchanged for host:**
   - Users still get "install X using Y" messages
   - tsuku does not modify the host system (beyond `$TSUKU_HOME`)

3. **Consent flow is sandbox-scoped:**
   - No consent needed on host (nothing executes)
   - Sandbox consent is already given by running in sandbox mode

4. **Phase 4 (Host Execution) should be reconsidered:**
   - Maybe never enable host system modification
   - Or make it opt-in with very explicit warnings

---

## Conclusion

The current `require_system` behavior is correct and should be preserved. The design document conflates verification with installation. For tsuku's philosophy of being self-contained and not requiring sudo:

- **Host mode**: Tell users what to install (current behavior)
- **Sandbox mode**: Actually execute installations (new capability)

This maintains the "no system dependencies" philosophy for host installations while enabling comprehensive testing in isolated environments.
