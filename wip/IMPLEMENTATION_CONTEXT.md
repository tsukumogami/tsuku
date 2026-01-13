# Implementation Context: Issue #772

**Source**: docs/designs/current/DESIGN-golden-plan-testing.md and docs/designs/DESIGN-structured-install-guide.md

## Goal

Convert all recipes identified in discovery issue #758 to the new typed action format.

## Key Requirements

1. All recipes from discovery (#758) converted to typed actions
2. No explicit `when` clauses needed for PM actions (implicit constraints from #760)
3. Add `require_command` for verification
4. Add content-addressing (SHA256) for external resources
5. All converted recipes pass preflight validation
6. All converted recipes can be sandbox-tested

## Dependencies (All Closed)

- #758: chore(recipes): discover recipes requiring migration - CLOSED
- #770: feat(sandbox): integrate container building with sandbox executor - CLOSED
- #771: feat(sandbox): implement action execution in sandbox context - CLOSED
- #760: feat(actions): implement implicit constraints for PM actions - CLOSED

## Downstream Dependencies

This issue blocks:
- #773: refactor(actions): remove legacy install_guide support
- #774: feat(golden): enable golden files for system dependency recipes
