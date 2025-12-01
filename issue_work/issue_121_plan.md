# Issue 121 Implementation Plan

## Summary

Implement `GoBuilder` in `internal/builders/go.go` to generate recipes for Go modules, following the existing builder pattern (CargoBuilder, CPANBuilder).

## Approach

The Go builder will query proxy.golang.org to validate module existence, infer executable names from module paths (last path segment), and generate recipes that use the `go_install` action with `dependencies = ["go"]`.

### Alternatives Considered

- **Query GitHub for repository metadata**: Would limit to GitHub-hosted modules only; proxy.golang.org works for all public modules.
- **Parse go.mod for executable discovery**: Would require downloading the module; simpler to infer from module path (matches how `go install` determines binary name).

## Files to Create

- `internal/builders/go.go` - GoBuilder implementation
- `internal/builders/go_test.go` - Unit tests

## Files to Modify

None - the builder will be registered by consumers (e.g., `tsuku create`)

## Implementation Steps

- [ ] Step 1: Create `GoBuilder` struct with constructor and Name() method
- [ ] Step 2: Implement `isValidGoModule()` validation function
- [ ] Step 3: Implement `inferExecutableName()` helper function
- [ ] Step 4: Implement `CanBuild()` method with proxy.golang.org query
- [ ] Step 5: Implement `Build()` method to generate recipes
- [ ] Step 6: Create unit tests for all functions

## Testing Strategy

- Unit tests with httptest mock server for proxy.golang.org API
- Test cases:
  - `TestGoBuilder_Name` - verify builder name
  - `TestGoBuilder_CanBuild_ValidModule` - successful module lookup
  - `TestGoBuilder_CanBuild_NotFound` - module not found returns false
  - `TestGoBuilder_CanBuild_InvalidName` - invalid module path validation
  - `TestGoBuilder_Build_SimpleModule` - standard Go module
  - `TestGoBuilder_Build_CmdSubpath` - module with /cmd/toolname path
  - `TestGoBuilder_Build_NotFound` - error on nonexistent module
  - `TestIsValidGoModule` - module path validation
  - `TestInferGoExecutableName` - executable name inference

## API Details

### proxy.golang.org Endpoints Used

- `GET /{module}/@latest` - Returns JSON: `{"Version":"v1.2.3","Time":"..."}`
- Status 200: module exists
- Status 404: module not found
- Status 410: module marked as retracted

### Module Path Encoding

Go module paths with uppercase letters need encoding:
- `github.com/User/Repo` -> `github.com/!user/!repo`
- Uppercase letters become `!` + lowercase

### Executable Name Inference

Per design document:
- `github.com/jesseduffield/lazygit` -> `lazygit` (last path segment)
- `github.com/golangci/golangci-lint/cmd/golangci-lint` -> `golangci-lint`
- `mvdan.cc/gofumpt` -> `gofumpt`

## Recipe Format Generated

```toml
[metadata]
name = "lazygit"
description = "Go CLI tool from github.com/jesseduffield/lazygit"
homepage = "https://pkg.go.dev/github.com/jesseduffield/lazygit"
dependencies = ["go"]

[version]
source = "goproxy"

[[steps]]
action = "go_install"
module = "github.com/jesseduffield/lazygit"
executables = ["lazygit"]

[verify]
command = "lazygit --version"
```

## Success Criteria

- [ ] `GoBuilder` implements the `Builder` interface
- [ ] Validates module exists via proxy query
- [ ] Infers executable name from module path (last segment)
- [ ] Generated recipe includes `dependencies = ["go"]`
- [ ] Generated recipe uses `go_install` action
- [ ] Generated recipe uses `goproxy` version source
- [ ] Unit tests for executable inference
- [ ] All tests pass with >70% coverage for new code

## Open Questions

None - all requirements are clear from the design document and existing builder patterns.
