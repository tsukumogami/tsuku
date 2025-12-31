# Issue #763 Implementation Plan

## Goal
Implement `Describe(params map[string]interface{}) string` method on all typed actions to generate human-readable, copy-pasteable shell commands.

## Approach
Add `Describe()` to the `SystemAction` interface since all target actions implement it. This maintains consistency with the existing `Validate()` and `Preflight()` methods.

## Actions to Implement

### Package Installation Actions
| Action | Describe Output |
|--------|-----------------|
| apt_install | `sudo apt-get install -y <packages>` |
| apt_repo | `curl -fsSL <key_url> \| sudo gpg --dearmor -o /etc/apt/keyrings/<name>.gpg && echo "deb [signed-by=...] <url> ..." \| sudo tee /etc/apt/sources.list.d/<name>.list` |
| apt_ppa | `sudo add-apt-repository ppa:<ppa>` |
| brew_install | `brew install <packages>` |
| brew_cask | `brew install --cask <packages>` |
| dnf_install | `sudo dnf install -y <packages>` |
| dnf_repo | `sudo dnf config-manager --add-repo <url>` |
| pacman_install | `sudo pacman -S --noconfirm <packages>` |
| apk_install | `sudo apk add <packages>` |
| zypper_install | `sudo zypper install -y <packages>` |

### Configuration/Verification Actions
| Action | Describe Output |
|--------|-----------------|
| group_add | `sudo usermod -aG <group> $USER` |
| service_enable | `sudo systemctl enable <service>` |
| service_start | `sudo systemctl start <service>` |
| require_command | `Requires: <command> (version >= <min_version>)` (informational) |
| manual | Returns the `text` parameter directly |

## Implementation Steps

1. Add `Describe()` to SystemAction interface in `system_action.go`
2. Implement `Describe()` for apt actions in `apt_actions.go`
3. Implement `Describe()` for brew actions in `brew_actions.go`
4. Implement `Describe()` for dnf actions in `dnf_actions.go`
5. Implement `Describe()` for linux PM actions in `linux_pm_actions.go`
6. Implement `Describe()` for config actions in `system_config.go`
7. Add tests for each `Describe()` implementation
8. Update design doc dependency graph

## Test Strategy
For each action, test:
- Valid params → expected shell command
- Missing/empty params → reasonable fallback or empty string

## Files to Modify
- `internal/actions/system_action.go` - Add interface method
- `internal/actions/apt_actions.go` - 3 Describe implementations
- `internal/actions/brew_actions.go` - 2 Describe implementations
- `internal/actions/dnf_actions.go` - 2 Describe implementations
- `internal/actions/linux_pm_actions.go` - 3 Describe implementations
- `internal/actions/system_config.go` - 5 Describe implementations
- Test files for each of the above
