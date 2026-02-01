---
summary:
  constraints:
    - Must preserve backward compatibility (no-constraint recipes run on all runners)
    - Must respect existing exclusion logic (library, require_system, execution-exclusions.json)
    - CLI already provides `tsuku info --json --metadata-only` for platform metadata
  integration_points:
    - .github/workflows/test-changed-recipes.yml (primary target - matrix detection step)
    - tsuku info --json --metadata-only (reads supported_platforms from recipe)
    - Batch-generated recipes with platform constraint fields (supported_os, supported_arch, etc.)
  risks:
    - tsuku info --metadata-only may not exist yet or may not return supported_platforms
    - Need Go build step before matrix detection to use tsuku binary
    - macOS job currently derives recipe path from tool name only (registry path) - embedded recipes need handling
  approach_notes: |
    Replace grep-based linux_only detection with tsuku info --json --metadata-only output.
    Parse supported_platforms array to determine which runners each recipe needs.
    The matrix step already runs on ubuntu-latest; add Go build + tsuku build before detection.
    Map platform IDs: linux-*-x86_64 -> ubuntu-latest, darwin-* -> macos-latest.
---
