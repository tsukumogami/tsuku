# Pragmatic Review: Issue #1790

## Scope

Two new recipe files:
- `recipes/m/mesa-vulkan-drivers.toml` (new)
- `recipes/v/vulkan-loader.toml` (new)

## Findings

### 1. vulkan-loader depends on mesa-vulkan-drivers -- design doc says otherwise

**File**: `recipes/v/vulkan-loader.toml:24`
**Severity**: Advisory

The design doc sketch for vulkan-loader.toml (DESIGN-gpu-backend-selection.md, lines 679-716) does not include a `dependencies` field. The implementation adds `dependencies = ["mesa-vulkan-drivers"]`. This is a reasonable choice -- the loader is useless without ICD drivers -- but it's a deviation from the design sketch. The design doc describes tsuku-llm's Vulkan steps depending on `vulkan-loader`, and the loader discovering ICDs at runtime. It doesn't explicitly model mesa-vulkan-drivers as a separate recipe at all.

That said, the issue title says "mesa-vulkan-drivers and vulkan-loader dependency recipes" and the dependency direction (loader depends on drivers) makes sense operationally. This is advisory because the design sketch was explicitly labeled "sketch" and the dependency is correct for the use case.

### 2. openSUSE package choice for mesa-vulkan-drivers is indirect

**File**: `recipes/m/mesa-vulkan-drivers.toml:50`
**Severity**: Advisory

`Mesa-vulkan-device-select` is a Vulkan layer for GPU device selection, not the ICD drivers themselves. On openSUSE, the actual ICD driver packages are `Mesa-vulkan-drivers` (or `libvulkan_radeon` / `libvulkan_intel`). The device-select layer does pull in mesa Vulkan dependencies transitively, but it's an indirect path. Compare: the nvidia-driver recipe uses a direct package for openSUSE (`nvidia-driver-G06-kmp-default`). Consider using `Mesa-vulkan-drivers` directly if that package exists on openSUSE Tumbleweed/Leap.

### 3. Verify command for mesa-vulkan-drivers relies on shell globbing

**File**: `recipes/m/mesa-vulkan-drivers.toml:53`
**Severity**: Advisory

```toml
command = "ls /usr/share/vulkan/icd.d/*.json /etc/vulkan/icd.d/*.json 2>/dev/null"
```

This works but has a subtle behavior: if both directories have JSON files, it succeeds. If neither has any, `ls` outputs nothing (stderr suppressed) and the pattern `.` won't match an empty string. That's actually correct -- empty output means no match. But if only `/usr/share/vulkan/icd.d/` has files and `/etc/vulkan/icd.d/` doesn't exist, `ls` still succeeds because it found files in the first path. Fine.

The `2>/dev/null` handles the missing directory case. The pattern `.` matches any character in the output. This is reasonable for a verify command that just needs to confirm "some ICD JSON files exist." No blocking issue.

### 4. No blocking findings

Both recipes follow the established patterns from `nvidia-driver.toml` and `docker.toml`:
- System PM actions per distro family (apt, dnf, pacman, apk, zypper)
- `type = "library"` for non-binary recipes
- `supported_os = ["linux"]` since Vulkan drivers are Linux-only in this context
- Verify commands that confirm the library/driver presence without version pinning
- Comments explaining distro-specific package names

The dependency chain (`vulkan-loader` -> `mesa-vulkan-drivers`) correctly models the Vulkan stack: the loader dispatches to ICD drivers, so both must be present.

## Summary

No blocking issues. The recipes are minimal, follow existing patterns, and correctly model the Vulkan dependency chain. Two advisory notes: the openSUSE package for mesa-vulkan-drivers is indirect (uses a layer package rather than the driver package directly), and the vulkan-loader dependency on mesa-vulkan-drivers is an addition not present in the design doc sketch (but sensible).
