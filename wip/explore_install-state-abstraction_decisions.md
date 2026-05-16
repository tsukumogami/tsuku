# Exploration Decisions: install-state-abstraction

## Round 1
- Mode: --auto. Recording decisions as they're made; default tier is 2 (micro-protocol). Tier 3+ decisions invoke the decision skill.
- Six leads chosen (not 8): leads 3 (Go patterns) and 6 (sketch + migration) carry the most weight; leads 1 (baseline) and 5 (cost model) are evidence prerequisites for downstream judgment; leads 2 (concerns inventory) and 4 (context.Context) test the assumption that this is a recurring class of problem. Rationale: keep agents focused; cluster only if a 7th lead appears during convergence.
- Adversarial demand lead skipped: label is needs-design (not needs-prd or bug). Per Phase 1 --auto pre-gate, default to not firing.

### Convergence decisions (post-discovery)

- **Java-style literal repository pattern eliminated.** Lead 3 found it over-engineered for ~5 lifecycle ops with no second storage backend; Lead 6 confirmed Manager already IS a repository in Go-idiomatic terms.
- **Command/middleware/decorator-chain patterns eliminated.** Over-engineered for 2-3 cross-cutting concerns × 5 operations. Pays off when N×M combinations would otherwise be hand-written; not the case here.
- **Aggressive `OperationOptions` struct (Candidate D) deprioritized.** It's essentially "Candidate A plus shared struct" and doesn't address the structural state.json leak that Lead 6 surfaced.
- **Two shapes survive into Crystallize:** Candidate C (context.Context-based attribution + standalone recursion-collapse refactor) and Candidate B (new `installops` Service layer above Manager). Status quo (Candidate A) remains a defensible "no restructure" position.
- **Crystallize artifact type: design doc.** The remaining questions are decision-class judgments (12-month forecast, state.json exposure taste, B-vs-C trade-off) — exactly what /design's structured decomposition is built to resolve. Issue #2413 has `needs-design` label. No re-investigation needed.
