# Issue 423 Implementation Plan

## Summary

Extend mermaid.sh's MM10 validation and related checks to accept `M<number>` milestone nodes alongside `I<number>` issue nodes, update the MM10 documentation, and add golden file tests covering issues-only, milestones-only, and mixed diagrams.

## Approach

The most direct approach: widen the regex/grep patterns in mermaid.sh wherever `I<number>` is matched to also accept `M<number>`, then separate issue nodes from milestone nodes so that MM06/MM09 cross-referencing only applies to issue nodes (milestones don't appear in the table's issue column). MM11 (class assignment) and MM15 (status check) should include milestone nodes for class validation but skip GitHub status lookups for milestones.

### Alternatives Considered

- **Separate validation pass for milestones**: Add a parallel code path that extracts milestone nodes independently. Rejected because most validation logic (MM10, MM11, MM04, MM05) is identical for both node types; duplicating it adds maintenance burden.
- **Generic node type registry**: Make node prefixes configurable. Over-engineered for two known types; can refactor later if more types appear.

## Files to Modify

- `.github/scripts/checks/mermaid.sh` - Widen node patterns from `I[0-9]+` to `[IM][0-9]+`, split extracted nodes into issue vs milestone sets for MM06/MM09/MM15
- `.github/scripts/docs/MM10.md` - Document that `M<number>` milestone nodes are also valid

## Files to Create

- `tests/fixtures/validation/mermaid-issues-only.md` - Golden file: design doc with only I-nodes (should pass)
- `tests/fixtures/validation/mermaid-milestones-only.md` - Golden file: design doc with only M-nodes (should pass)
- `tests/fixtures/validation/mermaid-mixed.md` - Golden file: design doc with both I and M nodes (should pass)
- `tests/fixtures/validation/mermaid-invalid-node.md` - Golden file: design doc with bad node name (should fail MM10)
- `tests/run-mermaid-golden.sh` - Test runner that validates each fixture against expected pass/fail

## Implementation Steps

- [ ] Update `ISSUE_NODES` extraction (line 105) to also capture `M<number>` nodes; rename variable to `DIAGRAM_NODES_ALL` or similar
- [ ] Introduce `HAS_DEPENDENCY_DIAGRAM` check that looks for either `I` or `M` prefix nodes
- [ ] Update MM07 messages to mention both `I<number>` and `M<number>`
- [ ] Update MM10 validation (line 251) to allow `M<number>` alongside `I<number>` in `OTHER_NODES` filter
- [ ] Split nodes: `ISSUE_NODES` (I-prefix only) for MM06/MM09 table cross-ref, `ALL_NODES` (I+M) for MM11 class check
- [ ] Update MM15 loop to skip `M<number>` nodes (no GitHub issue to query for milestones)
- [ ] Update MM10.md to document milestone node format
- [ ] Create golden file fixtures (4 files)
- [ ] Create test runner script that runs `mermaid.sh --skip-status-check` on each fixture and checks exit code
- [ ] Run test runner to verify all golden files produce expected results

## Testing Strategy

- **Golden file tests**: 4 fixture documents exercising issues-only, milestones-only, mixed, and invalid-node scenarios. Test runner asserts expected exit codes.
- **Manual verification**: Run `mermaid.sh --skip-status-check` against an existing design doc that uses milestone nodes to confirm it passes.

## Risks and Mitigations

- **MM06/MM09 false positives for milestones**: Milestone nodes shouldn't be cross-referenced against the issue table. Mitigation: separate `ISSUE_NODES` (I-prefix) from `MILESTONE_NODES` (M-prefix) and only use `ISSUE_NODES` for MM06/MM09.
- **Edge pattern in MM15**: The blocker map currently only matches `I[0-9]+` edges. Mixed edges like `M1 --> I2` need to work. Mitigation: update edge regex to `[IM][0-9]+`.

## Success Criteria

- [ ] `mermaid.sh --skip-status-check` passes on a doc with `M<number>` nodes
- [ ] `mermaid.sh --skip-status-check` still rejects nodes like `X1` or `task1`
- [ ] MM10.md documents both `I<number>` and `M<number>` formats
- [ ] Golden file test runner passes for all 4 fixture files
- [ ] Existing design docs continue to validate correctly (no regressions)

## Open Questions

None.
