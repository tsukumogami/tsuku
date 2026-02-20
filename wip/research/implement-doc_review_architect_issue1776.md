# Architect Review: Issue #1776

**Issue**: feat(recipe): add tsuku-llm recipe with GPU-filtered variant selection
**Commit**: 4cc537e9aeb260f64ed9ca91f48981f12be596d5
**File changed**: `recipes/t/tsuku-llm.toml` (new)

## Summary

This change adds a single recipe TOML file. It is the first consumer of the `gpu` field on `WhenClause` (added in #1774) and the first recipe to use step-level `dependencies` for GPU runtime provisioning.

## Findings

### No blocking findings.

The recipe fits cleanly into the existing architecture:

**1. WhenClause usage follows established patterns.**
The `gpu` field is used in `when` clauses exactly as `libc` and `os` are used in other recipes. The field exists on `WhenClause` in `internal/recipe/types.go:251` (`GPU []string`), and the matching logic in `Matches()` handles it. No new filtering mechanism is introduced.

**2. Step-level dependencies use the existing mechanism.**
`dependencies = ["cuda-runtime"]` on CUDA steps and `dependencies = ["vulkan-loader"]` on Vulkan steps use the `Step.Dependencies` field (`internal/recipe/types.go:350`), which the plan generator already filters -- unmatched steps have their dependencies skipped. No parallel dependency pattern.

**3. Action type is consistent.**
All steps use `action = "github_file"`, which is a registered action. The `binary`, `repo`, and `asset_pattern` parameters match the pattern used by `kind.toml`, `k3d.toml`, and other `github_file` recipes.

**4. Version provider is registered.**
`source = "github_releases"` is a known version source (`internal/version/validation.go:36`), and `github_file` actions have a natural redundancy mapping to `github_releases` (`internal/version/redundancy.go:29`).

**5. Metadata fields are valid.**
`supported_os`, `supported_arch`, and `supported_libc` are all recognized fields on `Metadata` (`internal/recipe/types.go:164-166`). The recipe constrains to `glibc` only, consistent with the design doc's statement that no musl-linked variants are built.

**6. Dependency chain is complete.**
The referenced dependency recipes exist: `cuda-runtime` (`recipes/c/cuda-runtime.toml`) depends on `nvidia-driver` (`recipes/n/nvidia-driver.toml`); `vulkan-loader` (`recipes/v/vulkan-loader.toml`) depends on `mesa-vulkan-drivers` (`recipes/m/mesa-vulkan-drivers.toml`). No dangling references.

**7. No dispatch bypass, provider inline instantiation, state contract violation, or dependency inversion.**
This is a data file (TOML recipe), not Go code. It flows through the existing recipe loading, plan generation, and action execution pipeline without requiring any code changes.

### Advisory findings

**A1. Windows step included despite design doc deferral (advisory).**
The design doc (`docs/designs/DESIGN-gpu-backend-selection.md`, line 144) lists "Windows support" as deferred: "The pipeline builds a Windows CPU variant but the recipe doesn't include it yet." However, `recipes/t/tsuku-llm.toml:98-104` does include a Windows amd64 CPU step. This is strictly additive and doesn't break anything, but it's a divergence from the documented scope. Either update the design doc or remove the Windows step to match.

**A2. Recipe has no `version_format` field (advisory).**
Most recipes that use `github_releases` as a version source don't set `version_format`, so this is consistent with the codebase norm. However, given that the asset patterns use `v{version}` (e.g., `tsuku-llm-v{version}-darwin-arm64`), the version provider needs to strip the `v` prefix correctly. The `github_releases` provider handles this by default, so no issue, but a `tag_prefix = "v"` in the `[version]` section would make the expectation explicit, as `scaleway-cli.toml` does not need it but other recipes using `github_releases` might.

## Overall Assessment

The recipe fits the existing architecture with no structural violations. It uses `WhenClause.GPU` exactly as designed, step-level dependencies for GPU runtime provisioning, and the standard `github_file` action for all variants. The 9 steps (2 macOS, 6 Linux GPU-filtered, 1 Windows) produce mutually exclusive coverage: each platform+GPU combination matches exactly one step. The dependency recipes are already in place from prior issues (#1789, #1790).

No blocking issues. Two minor advisories about design doc sync and optional explicitness.
