---
summary:
  constraints:
    - File must be at data/dep-mapping.json
    - Structure keyed by ecosystem with nested dep-name -> recipe-name mappings
    - "pending" is valid for deps without tsuku recipes yet
  integration_points:
    - data/dep-mapping.json consumed by gap analysis script (#1203) and batch pipeline
    - blocked_by field in failure records references tsuku recipe names via this mapping
  risks:
    - None significant - this is a static JSON data file
  approach_notes: |
    Create the JSON file with initial Homebrew mappings. Add a README in data/
    explaining the file's purpose.
---

# Implementation Context: Issue #1200

**Source**: docs/designs/DESIGN-priority-queue.md

Simple data file creation - add dep-mapping.json with initial Homebrew dependency name mappings.
