---
summary:
  constraints:
    - Must include three categories: toolchains, build tools, libraries
    - Consistent table format with Recipe/Required By/Rationale columns
    - Must explain CI validation with --require-embedded flag
  integration_points:
    - docs/EMBEDDED_RECIPES.md (new file to create)
    - Referenced by CI workflow in issue #1048
  risks:
    - List could be incomplete - but runtime validation catches this
  approach_notes: |
    Simple documentation task. Create docs/EMBEDDED_RECIPES.md with:
    1. Toolchain recipes (go, rust, nodejs, python, ruby, perl)
    2. Build tool recipes (make, cmake, meson, ninja, zig, pkg-config, patchelf)
    3. Library recipes (libyaml, zlib, openssl)
    4. Notes section explaining validation and maintenance
---

# Implementation Context: Issue #1046

This is a simple documentation issue - create EMBEDDED_RECIPES.md documenting which recipes must remain embedded and why.
