# Design Decisions Log: pipx-pypi-version-pinning

## Phase 1: Decomposition

- **Three decision questions identified, all standard tier.**
  - D1: Filter location + bundled-Python source (coupled; merged from
    three open questions in the skeleton).
  - D2: PEP 440 specifier subset for the evaluator.
  - D3: Failure-message contract when no compatible release exists.
- **Out of scope: cross-provider symmetry.** Decided in /explore;
  carried forward as constraint.
- **Out of scope: azure-cli.** Decided in /explore; deferred to a
  separate follow-up.
- **Constraint: recipes carry no version pins.** Decided in /explore;
  carried forward.
