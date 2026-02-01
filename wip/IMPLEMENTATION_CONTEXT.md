---
summary:
  constraints:
    - Guard must run after builder selection but before session creation
    - Must use ExitDeterministicFailed (exit code 9)
    - Error message must match UX table in design doc
  integration_points:
    - cmd/tsuku/create.go runCreate() - insert guard after line 248
    - builders.Builder.RequiresLLM() interface method
    - createDeterministicOnly flag
  risks:
    - None significant - simple boolean check on existing interface
  approach_notes: |
    Add if-check after builder lookup. Check createDeterministicOnly && builder.RequiresLLM().
    Print actionable error to stderr, exit with ExitDeterministicFailed.
    Add unit test for the guard behavior.
---

# Implementation Context: Issue #1314

Simple guard: check RequiresLLM() + createDeterministicOnly before session creation in create.go.
