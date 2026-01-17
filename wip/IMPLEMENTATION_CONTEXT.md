# Implementation Context: Issue #979

**Source**: docs/designs/DESIGN-library-verify-deps.md (Tier 2 Dependency Resolution)

## Issue Summary

Add `IsExternallyManaged()` method to `SystemAction` interface to distinguish package manager actions from other system actions. This enables the verification system to determine when to skip recursive dependency validation.

## Design Rationale

From the Complete Validation Model:

```
3. AM I TSUKU-MANAGED?
   - Check via IsExternallyManaged() on recipe actions
   → If externally-managed: STOP here (validated in step 2, done)

5. RECURSE into TSUKU-MANAGED dependencies
   - Skip EXTERNALLY-MANAGED (validated in 4c, but don't recurse - pkg manager owns internals)
```

Dependencies installed via `apt_install`, `brew_install`, etc. are managed by the system package manager, not tsuku. We verify they provide expected sonames but don't recurse into their internal dependencies.

## Three Categories

| Category | Definition | Action |
|----------|------------|--------|
| PURE SYSTEM | Inherently OS-provided (libc, libSystem) | Verify accessible, skip recursion |
| TSUKU-MANAGED | Built/managed by tsuku | Verify provides expected soname, recurse |
| EXTERNALLY-MANAGED | Tsuku recipe delegating to pkg manager | Verify provides expected soname, skip recursion |

## Dependencies

- **None** - This issue has no blockers

## Downstream Dependents

- **#984** (IsExternallyManagedFor) - Uses this method to query at recipe level
- **#989** (Recursive validation) - Uses this to determine recursion behavior

## Implementation Pattern

Package manager actions return `true`:
- AptInstallAction, AptRepoAction, AptPPAAction
- BrewInstallAction, BrewCaskAction
- DnfInstallAction, DnfRepoAction
- PacmanInstallAction, ApkInstallAction, ZypperInstallAction

Other system actions return `false`:
- GroupAddAction, ServiceEnableAction, ServiceStartAction
- RequireCommandAction, ManualAction

## Exit Criteria

From acceptance criteria:
- [ ] `SystemAction` interface includes `IsExternallyManaged() bool` method
- [ ] All 10 package manager actions implement the method returning `true`
- [ ] All 5 other system actions implement the method returning `false`
- [ ] Unit tests pass for all implementations
- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/actions/...` passes
