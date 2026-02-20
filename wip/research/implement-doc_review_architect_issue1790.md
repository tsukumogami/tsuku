# Architect Review: Issue #1790

**Issue**: feat(recipe): add mesa-vulkan-drivers and vulkan-loader dependency recipes
**Commit**: 4914eb15969f5d773c41a677d02694d309b4b330
**Design doc**: docs/designs/DESIGN-gpu-backend-selection.md

## Files Reviewed

- `recipes/m/mesa-vulkan-drivers.toml` (new)
- `recipes/v/vulkan-loader.toml` (new)

## Architectural Assessment

Both recipes follow established patterns correctly. No blocking findings.

### Pattern Conformance

**Action type usage**: Both recipes use system PM actions (`apt_install`, `dnf_install`, `pacman_install`, `apk_install`, `zypper_install`) without explicit `when` clauses on individual steps. This matches the pattern established by `docker.toml` and `nvidia-driver.toml`, where the plan generator uses `ImplicitConstraint()` on `SystemAction` implementations to filter steps by Linux family (see `internal/executor/plan_generator.go:195-203`). No bypass of the action dispatch system.

**Metadata-level dependencies**: `vulkan-loader.toml` declares `dependencies = ["mesa-vulkan-drivers"]` at the metadata level, matching how `cuda-runtime.toml` declares `dependencies = ["nvidia-driver"]`. The dependency resolution flows through `generateDependencyPlans()` in the plan generator. Consistent with the existing pattern.

**Recipe type**: Both use `type = "library"`, consistent with the 16 other library recipes in the registry (`zstd`, `readline`, `libcurl`, `cuda-runtime`, etc.).

**Verify section**: Both use `mode = "output"` with a `reason` field, matching the pattern in `nvidia-driver.toml` and 10+ other recipes. The `Reason` field is defined at `internal/recipe/types.go:771`.

**Supported OS**: Both declare `supported_os = ["linux"]`, matching `nvidia-driver.toml` and `cuda-runtime.toml`. Platform filtering is handled by the recipe policy layer (`internal/recipe/policy.go`).

**Directory placement**: `recipes/m/mesa-vulkan-drivers.toml` and `recipes/v/vulkan-loader.toml` follow the first-letter directory convention.

### Design Alignment

The design doc's issue table (line 51-52) explicitly specifies two recipes for #1790: `vulkan-loader.toml` and `mesa-vulkan-drivers.toml`. The implementation matches. The dependency relationship (vulkan-loader depends on mesa-vulkan-drivers) is architecturally sound: the Vulkan loader discovers ICD drivers at runtime, and Mesa provides those ICD implementations. Without mesa-vulkan-drivers, the loader finds zero devices.

The design doc's solution sketch for vulkan-loader (lines 679-716) didn't include mesa-vulkan-drivers as a dependency, but that sketch was labeled as such and predates the issue breakdown. The issue description is the authoritative spec.

### Extensibility

These recipes complete the Vulkan half of the GPU runtime dependency chain. The downstream consumer (`recipes/t/tsuku-llm.toml`, issue #1776) will declare step-level `dependencies = ["vulkan-loader"]` on its Vulkan steps, which will transitively pull in mesa-vulkan-drivers. This follows the same transitive dependency pattern as cuda-runtime -> nvidia-driver.

## Findings

### Blocking: 0

None.

### Advisory: 0

None. Both recipes are pure TOML, follow every established convention, and introduce no new patterns. The header comments are thorough and include distro-specific notes that will help future maintainers understand package name differences across distributions.

## Summary

Clean implementation. Both recipes are structurally identical to the nvidia-driver.toml and cuda-runtime.toml recipes from #1789, following the same system PM action pattern, metadata conventions, and verify approach. No new patterns introduced; no existing patterns bypassed.
