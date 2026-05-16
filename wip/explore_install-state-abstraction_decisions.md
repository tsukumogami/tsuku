# Exploration Decisions: install-state-abstraction

## Round 1
- Mode: --auto. Recording decisions as they're made; default tier is 2 (micro-protocol). Tier 3+ decisions invoke the decision skill.
- Six leads chosen (not 8): leads 3 (Go patterns) and 6 (sketch + migration) carry the most weight; leads 1 (baseline) and 5 (cost model) are evidence prerequisites for downstream judgment; leads 2 (concerns inventory) and 4 (context.Context) test the assumption that this is a recurring class of problem. Rationale: keep agents focused; cluster only if a 7th lead appears during convergence.
- Adversarial demand lead skipped: label is needs-design (not needs-prd or bug). Per Phase 1 --auto pre-gate, default to not firing.
