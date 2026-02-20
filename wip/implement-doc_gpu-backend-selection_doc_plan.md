# Documentation Plan: gpu-backend-selection

Generated from: docs/designs/DESIGN-gpu-backend-selection.md
Issues analyzed: 13
Total entries: 4

---

## doc-1: docs/when-clause-usage.md
**Section**: GPU Filter (new section after Libc Filter)
**Prerequisite issues**: #1774
**Update type**: modify
**Status**: pending
**Details**: Add a "GPU Filter" section documenting the `gpu` field on `when` clauses. Show syntax examples for filtering steps by GPU vendor (`nvidia`, `amd`, `intel`, `apple`, `none`). Include a note that `gpu` values are mutually exclusive (one vendor per system) and valid values list. Follow the same structure as the existing Libc Filter section.

---

## doc-2: README.md
**Section**: GPU-Aware Installation (new subsection under Features or after System Dependencies)
**Prerequisite issues**: #1776, #1778
**Update type**: modify
**Status**: pending
**Details**: Add a section explaining that tsuku detects GPU hardware and selects the right binary variant for tools that ship GPU-accelerated builds. Use tsuku-llm as the concrete example: NVIDIA gets CUDA, AMD/Intel gets Vulkan, no GPU gets CPU, macOS gets Metal. Mention that GPU runtime dependencies (CUDA, Vulkan loader) are provisioned automatically. Keep it brief -- 1 example block showing `tsuku install tsuku-llm` and a short explanation of what happens on different hardware.

---

## doc-3: README.md
**Section**: Secrets Management (add `llm.backend` to config keys list)
**Prerequisite issues**: #1777
**Update type**: modify
**Status**: pending
**Details**: Document the `llm.backend` config key in the README. Add a short subsection or paragraph near the existing `tsuku config` documentation explaining that `tsuku config set llm.backend cpu` forces the CPU variant for tsuku-llm regardless of detected GPU hardware. Mention this is useful when GPU drivers are broken or unavailable. Add `llm.backend` to the known config keys if a list exists, or document it alongside the config set/get examples.

---

## doc-4: docs/GUIDE-system-dependencies.md
**Section**: GPU Runtime Dependencies (new section)
**Prerequisite issues**: #1776, #1778
**Update type**: modify
**Status**: pending
**Details**: Add a section explaining how tsuku handles GPU runtime dependencies as a special case of system dependencies. Cover the dependency chain: tsuku-llm's CUDA step depends on `cuda-runtime` (downloaded from NVIDIA redistributables to `$TSUKU_HOME`), which depends on `nvidia-driver` (system PM). Vulkan steps depend on `vulkan-loader` (system PM). Explain that tsuku detects GPU hardware via sysfs and provisions the right runtime automatically. Mention that `nvidia-driver` and `vulkan-loader` recipes use system package manager actions (like the existing Docker recipe example already in this guide). Note the `llm.backend cpu` override for cases where GPU detection picks wrong.
