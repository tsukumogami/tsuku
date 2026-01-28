---
summary:
  constraints:
    - CONTRIBUTING.md updates must reference EMBEDDED_RECIPES.md for the definitive embedded recipe list
    - Keep troubleshooting entries concise with clear symptoms and solutions
    - Incident response playbook should be actionable, not theoretical
    - Don't duplicate existing golden file testing documentation
  integration_points:
    - CONTRIBUTING.md - main file to update
    - EMBEDDED_RECIPES.md - reference for embedded recipe list
    - docs/designs/DESIGN-recipe-registry-separation.md - source design doc
  risks:
    - Flowchart must accurately reflect the three-directory structure
    - Error messages must match actual CLI output
    - Nightly validation documentation must match actual workflow behavior
  approach_notes: |
    This is a documentation-only issue. Add the following sections to CONTRIBUTING.md:
    1. Recipe category decision flowchart (embedded vs registry vs testdata)
    2. Three directory explanation table
    3. Troubleshooting: "works locally fails in CI" and "recipe not found (network)"
    4. Nightly validation documentation
    5. Security incident response playbook
---

# Implementation Context: Issue #1038

This is issue #1038: "docs(contributing): document recipe separation for contributors"

**Milestone**: M32 - Cache Management and Documentation

**Dependencies**: #1036 (completed), Registry Cache Policy milestone (M48, completed)

## Key Requirements

1. **Recipe Category Guidance**: Decision flowchart + three-directory table
2. **Troubleshooting**: "Works locally fails in CI" + network-related "recipe not found"
3. **Nightly Validation**: Document the 2 AM UTC validation workflow
4. **Incident Response**: Security playbook for repository compromise

## Three Recipe Directories

| Directory | Purpose | Embedded | When to Use |
|-----------|---------|----------|-------------|
| `internal/recipe/recipes/` | Action dependencies | Yes | Only recipes in EMBEDDED_RECIPES.md |
| `recipes/` | User-installable tools | No (registry fetch) | Most new recipes |
| `testdata/recipes/` | CI feature coverage | Yes | Testing package manager actions |
