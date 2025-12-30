# Recipe Authoring UX Assessment: System Dependency Actions

## Overview

This assessment evaluates the developer experience implications of the proposed system dependency action design, focusing on Q3 (require semantics), Q4 (post-install config), and Q5 (manual fallback).

## Current State Analysis

Existing recipes fall into two categories:

1. **Self-contained tools** (gum, terraform, kubectx, minikube): Simple patterns using `github_archive`, `github_file`, or `download_archive`. These are 10-20 lines, easily authored, and require no system dependencies.

2. **System dependency tools** (docker, cuda): Use the current `require_system` action with embedded `install_guide` tables. These are awkward - the `install_guide` map conflates platform detection with installation guidance, making the recipe hard to extend.

The proposed design addresses the second category, but impacts both.

## Q3: Require Semantics

**Recommendation: Option C (idempotent install + final verify) as the default, with Option B (unless_command) as an escape hatch.**

### Analysis

Option A (separate verify step) is the most explicit but creates unnecessary verbosity for common cases. Looking at the Docker example, the proposed pattern requires 7 separate `[[steps]]` blocks just to express "install Docker everywhere and verify". This is a 3x-4x expansion from the current 1-block pattern.

Option B (`unless_command` guard) is attractive for optimization-conscious authors but introduces subtle complexity: what happens when the command exists but at the wrong version? Authors must understand idempotency semantics of their package manager.

Option C leverages package manager idempotency and is the simplest mental model: "run these installs, then check". The final `require_command` serves as both assertion and documentation.

### Ergonomic Concern

The design should provide a shorthand for the common "install and verify" pattern. Consider a composite action or table syntax:

```toml
[system_dependency]
command = "docker"
apt = { packages = ["docker-ce", "docker-ce-cli"], repo = {...} }
brew_cask = ["docker"]
dnf = ["docker"]
```

This would expand to the full step sequence internally. Authors who need fine-grained control can use individual steps.

## Q4: Post-Install Configuration

**Recommendation: Option A (separate actions) with a concession for tightly coupled operations.**

### Analysis

Option A (separate `group_add`, `service_enable` actions) provides maximum composability and clarity. Each action has a single responsibility, errors are isolated, and the sequence is obvious to readers.

Option B (`post_install` hooks) couples unrelated concerns. A `service_enable` failure shouldn't be conceptually linked to `apt_install`. Moreover, embedded hooks create schema complexity - the `apt_install` action now needs to understand service and group schemas.

### Verbosity Trade-off

The verbosity of Option A is acceptable because:
1. These configurations are infrequent (only a few recipes need them)
2. The explicit steps serve as documentation for complex installations
3. Error messages can pinpoint exactly which step failed

### Group/Service Symmetry

The proposed `group_add` and `service_enable` actions have natural symmetry with the install actions. However, consider that `service_enable` without `service_start` leaves Docker unusable. These two are almost always paired. A `service_manage` action with `enable = true, start = true` might reduce step count for this common case without sacrificing clarity.

## Q5: Manual/Fallback Instructions

**Recommendation: Hybrid - Option A (`manual` action) for primary use, Option B (fallback field) for graceful degradation.**

### Analysis

The `manual` action (Option A) is essential for tools like CUDA where automation is impossible or inadvisable. It should be the primary way to express "human intervention required."

The `fallback` field (Option B) addresses a different concern: "what if this automated action fails?" This is orthogonal and valuable. An `apt_install` might fail because the repo isn't configured, but the fallback guides the user forward.

### Proposed Semantics

```toml
[[steps]]
action = "apt_install"
packages = ["nvidia-cuda-toolkit"]
fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
when = { distro = ["ubuntu"] }

# For platforms we don't automate at all
[[steps]]
action = "manual"
text = "Download CUDA from https://developer.nvidia.com/cuda-downloads"
when = { os = ["darwin"] }  # Darwin explicitly unsupported
```

This combination is clearer than overloading either mechanism.

## Verbosity vs Clarity

The proposed design optimizes for explicitness at the cost of verbosity. For simple tools (the 80% case), this is acceptable - they don't use system dependencies anyway. For complex tools like Docker, the 40+ line recipe is a documentation benefit, not a burden.

However, the design should acknowledge that recipe authors will copy patterns. If the Docker recipe is the canonical example, authors need clear guidance on which parts are required vs optional. Consider structured comments or a "recipe cookbook" documentation.

## Migration Path

Current `require_system` recipes are few (docker, cuda reviewed). Migration is straightforward:

1. `require_system` with `install_guide` becomes multiple platform-specific install actions + `require_command`
2. Free-form text in `install_guide` becomes `manual` actions with `when` clauses
3. The `[verify]` section remains unchanged

The migration burden is low because these recipes are relatively rare and authored by maintainers, not end users.

## Error Messages and Debugging

The composable design significantly improves debugging:

- **Current**: `require_system` failure produces a wall of text for all platforms
- **Proposed**: Each action can produce focused error messages. `apt_install` failure says "apt install failed" with apt's output, not brew instructions.

The `when` clause filtering should log which actions were skipped and why. A `--verbose` or `--dry-run` mode showing the effective step sequence would greatly aid debugging.

## Summary Recommendations

1. **Q3**: Use Option C (idempotent + verify) as default mental model. Add `unless_command` escape hatch. Consider a composite shorthand for common cases.

2. **Q4**: Use Option A (separate actions). Consider a combined `service_manage` action for the enable+start pattern.

3. **Q5**: Support both - `manual` action for explicit human intervention, `fallback` field on install actions for graceful degradation.

4. **Ergonomics**: Provide a high-level composite syntax for common patterns to reduce verbosity in simple cases while keeping the full action vocabulary for complex cases.

5. **Documentation**: Invest in a "recipe cookbook" with annotated examples showing minimal vs complete patterns. The Docker example should be supplemented with simpler graduated examples.
