# Issue #763 Implementation Summary

## Goal

Implement `Describe()` method on all typed actions that generates human-readable, copy-pasteable shell commands for installation instructions.

## Implementation

### Interface Extension

Added `Describe(params map[string]interface{}) string` to the `SystemAction` interface in `internal/actions/system_action.go`. The method signature follows the existing pattern used by `Execute()`, `Validate()`, and `Preflight()`.

### Package Manager Actions

| Action | Output Format |
|--------|--------------|
| `apt_install` | `sudo apt-get install -y <packages>` |
| `apt_repo` | `curl -fsSL <key_url> \| sudo gpg --dearmor -o /etc/apt/keyrings/repo.gpg && echo "deb [signed-by=/etc/apt/keyrings/repo.gpg] <url> stable main" \| sudo tee /etc/apt/sources.list.d/repo.list` |
| `apt_ppa` | `sudo add-apt-repository ppa:<ppa>` |
| `brew_install` | `brew install <packages>` |
| `brew_cask` | `brew install --cask <packages>` |
| `dnf_install` | `sudo dnf install -y <packages>` |
| `dnf_repo` | `sudo dnf config-manager --add-repo <url>` |
| `pacman_install` | `sudo pacman -S --noconfirm <packages>` |
| `apk_install` | `sudo apk add <packages>` |
| `zypper_install` | `sudo zypper install -y <packages>` |

### System Config Actions

| Action | Output Format |
|--------|--------------|
| `group_add` | `sudo usermod -aG <group> $USER` |
| `service_enable` | `sudo systemctl enable <service>` |
| `service_start` | `sudo systemctl start <service>` |
| `require_command` | `Requires: <command>` or `Requires: <command> (version >= <min_version>)` |
| `manual` | Returns the `text` parameter directly |

### Additional Changes

The config actions (`GroupAddAction`, `ServiceEnableAction`, `ServiceStartAction`, `RequireCommandAction`, `ManualAction`) now also implement `Validate()` and `ImplicitConstraint()` methods to fully satisfy the `SystemAction` interface.

## Testing

Added comprehensive tests for all Describe() implementations:

- `apt_actions_test.go`: 3 test functions (AptInstallAction, AptRepoAction, AptPPAAction)
- `brew_actions_test.go`: 2 test functions (BrewInstallAction, BrewCaskAction)
- `dnf_actions_test.go`: 2 test functions (DnfInstallAction, DnfRepoAction)
- `linux_pm_actions_test.go`: 3 test functions (PacmanInstallAction, ApkInstallAction, ZypperInstallAction)
- `system_config_test.go`: 5 test functions (GroupAddAction, ServiceEnableAction, ServiceStartAction, RequireCommandAction, ManualAction)

All tests verify:
- Empty string returned for missing/invalid params
- Correct command format for valid params
- Multiple packages/items properly joined

## Files Modified

- `internal/actions/system_action.go` - Interface extension
- `internal/actions/apt_actions.go` - apt_install, apt_repo, apt_ppa
- `internal/actions/brew_actions.go` - brew_install, brew_cask
- `internal/actions/dnf_actions.go` - dnf_install, dnf_repo
- `internal/actions/linux_pm_actions.go` - pacman_install, apk_install, zypper_install
- `internal/actions/system_config.go` - group_add, service_enable, service_start, require_command, manual

## Verification

- All tests pass: `go test ./...`
- Vet passes: `go vet ./...`
- Build succeeds: `go build -o tsuku ./cmd/tsuku`
