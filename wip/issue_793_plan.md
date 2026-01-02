# Issue 793 Implementation Plan

## Summary

Create three testdata recipe files that exercise the M30 system dependency action vocabulary (16 action types total). These fixtures will provide test coverage and serve as reference examples for recipe authors.

## Approach

Create minimal, focused recipes organized by action category:
- Package manager recipe exercises all 10 PM actions (apt, dnf, brew, etc.)
- Configuration recipe exercises 3 config actions (group_add, service_enable, service_start)
- Verification recipe exercises 2 verification actions (require_command, manual)

Recipes will use implicit constraints as defined in the M30 design (package manager actions automatically apply to their target platform without explicit `when` clauses).

## Files to Create

- `testdata/recipes/sysdep-pm.toml` - Package manager actions (apt_install, brew_install, dnf_install, pacman_install, apk_install, zypper_install, brew_cask, apt_repo, apt_ppa, dnf_repo)
- `testdata/recipes/sysdep-config.toml` - Configuration actions (group_add, service_enable, service_start)
- `testdata/recipes/sysdep-verify.toml` - Verification actions (require_command, manual)

## Implementation Steps

- [ ] Create sysdep-pm.toml with package manager actions
- [ ] Create sysdep-config.toml with configuration actions
- [ ] Create sysdep-verify.toml with verification actions
- [ ] Validate all recipes with `tsuku validate testdata/recipes/sysdep-*.toml`
- [ ] Verify comprehensive coverage of all 16 M30 action types

## Success Criteria

- [ ] All three testdata recipes created
- [ ] Package manager actions use implicit constraints (no explicit `when` clauses)
- [ ] Configuration actions use explicit `when` clauses as appropriate
- [ ] All recipes pass `tsuku validate` without errors
- [ ] All 16 M30 action types are exercised across the three recipes

## Open Questions

None - all M30 actions are implemented and the validation infrastructure exists.
