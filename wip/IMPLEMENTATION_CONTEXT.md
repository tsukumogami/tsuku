---
summary:
  constraints:
    - Action only checks and reports - never runs privileged commands
    - Uses exec.LookPath for package manager detection (accepts PATH trust assumption)
    - Must return structured DependencyMissingError for CLI formatting
    - Root detection via os.Getuid() == 0; prefer doas over sudo if available
  integration_points:
    - internal/actions/ - register action in registry
    - DependencyMissingError - structured error for CLI aggregation
    - Plan generator - collects ALL missing deps before failing
  risks:
    - Package detection commands may vary across Alpine versions
    - Need to ensure isInstalled() handles both installed and not-installed correctly
  approach_notes: |
    Create system_dependency action that:
    1. Takes name (library) and packages map (family -> package name)
    2. Checks if package is installed via apk info -e (Alpine focus first)
    3. Returns DependencyMissingError with install command if missing
    4. Root detection determines sudo/doas prefix
    Alpine-only scope initially; extensible to Debian/RHEL later.
---

# Implementation Context: Issue #1112

**Source**: docs/designs/DESIGN-platform-compatibility-verification.md

## Key Design Details

### DependencyMissingError Structure
```go
type DependencyMissingError struct {
    Library string  // e.g., "zlib"
    Package string  // e.g., "zlib-dev"
    Command string  // e.g., "sudo apk add zlib-dev"
    Family  string  // e.g., "alpine"
}
```

### Action Execute Flow
1. Get `name` and `packages` params
2. Get family from ctx.Target.LinuxFamily()
3. Look up package name for family
4. Check if installed with isInstalled()
5. If missing, return DependencyMissingError with getInstallCommand()

### Root Detection
```go
if os.Getuid() != 0 {
    if _, err := exec.LookPath("doas"); err == nil {
        prefix = "doas "
    } else {
        prefix = "sudo "
    }
}
```

### Package Detection (Alpine)
```go
func isInstalled(pkg string, family string) bool {
    switch family {
    case "alpine":
        cmd := exec.Command("apk", "info", "-e", pkg)
        return cmd.Run() == nil
    }
}
```
