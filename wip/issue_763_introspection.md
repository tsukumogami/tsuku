# Issue #763 Introspection

## Staleness Check Results
- 6 sibling issues closed since creation
- Milestone position: middle
- 1 referenced file modified (design doc)

## Findings

### Design vs Implementation Gap
The design doc shows `Describe()` returning `string` with struct fields:
```go
func (a *AptInstallAction) Describe() string {
    return fmt.Sprintf("Install packages: sudo apt-get install %s",
        strings.Join(a.Packages, " "))
}
```

However, actual implementation uses `params map[string]interface{}` (no struct fields).

### Resolution
`Describe()` should take params to match existing pattern:
```go
func (a *AptInstallAction) Describe(params map[string]interface{}) string
```

This aligns with how `Execute()`, `Validate()`, and `Preflight()` work.

### Actions to Implement

Package installation actions (from issue #755):
- apt_install, apt_repo, apt_ppa
- brew_install, brew_cask
- dnf_install, dnf_repo
- pacman_install, apk_install, zypper_install

Configuration/verification actions (from issue #756):
- group_add, service_enable, service_start
- require_command, manual

## Recommendation
Proceed with implementation. The signature adjustment is a minor deviation that follows established patterns.
