# Issue 24 Implementation Plan

## Summary

Add a `tsuku validate` command that validates recipe files without installing them, checking TOML syntax, required fields, action types, and security issues.

## Approach

Create a new `validate.go` command that:
1. Accepts a file path argument (recipe file)
2. Loads and parses the TOML file
3. Runs comprehensive validation checks
4. Outputs results in human-readable or JSON format

The command will use the existing `recipe.Loader.parseBytes()` method as a base, then add extended validation for action-specific parameters and security checks.

### Alternatives Considered
- **Extend existing validate function**: The current `validate()` in loader.go is minimal. Adding all validation there would bloat the loader. Better to create a dedicated validation package.
- **Make validation part of install flow**: This would slow down installs and doesn't help recipe development without attempting install.

## Files to Create
- `cmd/tsuku/validate.go` - New command implementation
- `internal/recipe/validator.go` - Comprehensive recipe validation logic
- `internal/recipe/validator_test.go` - Tests for validation

## Files to Modify
- `cmd/tsuku/main.go` - Register the new command

## Implementation Steps
- [x] Create recipe/validator.go with ValidationResult type and Validate function
- [x] Add action type validation (check action exists in registry)
- [x] Add action parameter validation (required params per action)
- [x] Add security checks (URL schemes, path traversal patterns)
- [x] Add version source validation
- [x] Create cmd/tsuku/validate.go command
- [x] Add --json flag support
- [x] Register command in main.go
- [x] Write unit tests for validator
- [ ] Write tests for validate command (covered by unit tests)

## Testing Strategy
- Unit tests: Test validator with valid/invalid recipe files
- Test various error cases: missing fields, invalid actions, security issues
- Test JSON output format

## Validation Checks to Implement

### Required Fields
- metadata.name
- steps array (non-empty)
- steps[].action
- verify.command

### Action Type Validation
- Action name must be registered in actions registry
- Each action has required parameters (e.g., download requires "url")

### Security Checks
- URL schemes: only allow http/https
- Path traversal: reject paths with ".." components
- verify.command: warn if it contains shell metacharacters without explanation

### Warnings (non-fatal)
- verify.pattern missing {version} placeholder
- Missing description
- Missing homepage

## Success Criteria
- [x] `tsuku validate <file.toml>` works on valid recipes
- [x] Invalid recipes produce clear error messages
- [x] --json flag outputs structured JSON
- [x] All tests pass

## Open Questions
None - straightforward implementation following existing patterns.
