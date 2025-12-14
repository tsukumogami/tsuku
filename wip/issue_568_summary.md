# Issue 568/569 Summary

## What Was Implemented

Added NetworkValidator interface to declare action network requirements for sandbox testing. Actions that fetch external dependencies (ecosystem primitives, system package managers) return true; actions that work with cached or pre-downloaded content return false.

## Changes Made

- `internal/actions/action.go`: Added NetworkValidator interface and RequiresNetwork() method to BaseAction
- `internal/actions/cargo_build.go`: Added RequiresNetwork() = true
- `internal/actions/cargo_install.go`: Added RequiresNetwork() = true
- `internal/actions/go_build.go`: Added RequiresNetwork() = true
- `internal/actions/go_install.go`: Added RequiresNetwork() = true
- `internal/actions/cpan_install.go`: Added RequiresNetwork() = true
- `internal/actions/npm_install.go`: Added RequiresNetwork() = true
- `internal/actions/pip_install.go`: Added RequiresNetwork() = true
- `internal/actions/pipx_install.go`: Added RequiresNetwork() = true
- `internal/actions/gem_install.go`: Added RequiresNetwork() = true
- `internal/actions/system_packages.go`: Added RequiresNetwork() = true for apt/yum/brew
- `internal/actions/nix_install.go`: Added RequiresNetwork() = true
- `internal/actions/nix_realize.go`: Added RequiresNetwork() = true
- `internal/actions/run_command.go`: Added RequiresNetwork() = true (conservative default)
- `internal/actions/action_test.go`: Added comprehensive tests for NetworkValidator

## Key Decisions

- **Separate interface instead of adding to Action**: Allows optional implementation without breaking existing code
- **BaseAction default is false**: Most actions work offline with cached content; require explicit opt-in for network
- **run_command returns true**: Conservative default since arbitrary commands may need network access
- **nix actions require network**: Both nix_install and nix_realize fetch from nix cache

## Trade-offs Accepted

- Interface assertion at call site: Callers must check if action implements NetworkValidator before calling. This is acceptable since the sandbox testing code will handle this centrally.

## Test Coverage

- New tests added: 2 test functions (TestBaseAction_RequiresNetwork, TestNetworkValidator_AllActions)
- Coverage: Comprehensive table-driven tests cover 15 network-requiring actions and 18 offline actions

## Known Limitations

- Network requirements are binary (true/false). Some actions might have conditional requirements based on parameters (e.g., cargo_build with vendored dependencies). For initial implementation, we use the conservative classification.

## Future Improvements

- Consider parameter-dependent network requirements if specific use cases emerge
- Could add RequiresNetworkFor(params) method for conditional requirements
