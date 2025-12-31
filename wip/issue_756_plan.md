# Issue 756 Implementation Plan

## Goal

Create Go structs for configuration (`group_add`, `service_enable`, `service_start`) and verification (`require_command`, `manual`) actions.

## Approach

Create a new file `internal/actions/system_config.go` for configuration and verification actions. These actions follow the existing action pattern:
- Embed `BaseAction` for default implementations
- Implement `Action` interface: `Name()`, `Execute()`, `IsDeterministic()`, `Dependencies()`
- Implement `Preflight` interface for parameter validation
- Register in `action.go` init()

**Key Design Decisions:**

1. **No new `SystemAction` interface**: The acceptance criteria mentions `SystemAction` interface, but:
   - Issue #755 (parallel issue for package actions) is still OPEN
   - The design doc shows these actions implementing the existing `Action` interface
   - The design mentions `ImplicitConstraint()` method for PM actions (not for config/verify actions)
   - Config/verify actions don't need implicit constraints (they use explicit `when` clauses)

   **Decision**: Implement using the existing `Action` interface pattern. If #755 introduces `SystemAction`, we can add it then.

2. **Stub implementations for Execute()**: Per the design doc, "Actions at this phase do NOT execute on the host - they provide: Implicit constraints (target matching), Parameter validation (preflight checks), Human-readable descriptions (documentation generation), Structured data (sandbox container building)". Execute() will log what would be done.

3. **Validate() vs Preflight()**: The acceptance criteria mentions `Validate() error` but the codebase uses `Preflight(params map[string]interface{}) *PreflightResult`. We'll use the established `Preflight` pattern.

## Files to Modify/Create

| File | Action | Description |
|------|--------|-------------|
| `internal/actions/system_config.go` | Create | Configuration and verification action structs |
| `internal/actions/system_config_test.go` | Create | Unit tests for all action structs |
| `internal/actions/action.go` | Modify | Register new actions in init() |

## Implementation Steps

### Step 1: Create system_config.go with action structs

Create file with:

```go
// GroupAddAction - adds user to a group
type GroupAddAction struct{ BaseAction }
func (a *GroupAddAction) Name() string { return "group_add" }
func (a *GroupAddAction) IsDeterministic() bool { return true }
func (a *GroupAddAction) Preflight(params map[string]interface{}) *PreflightResult
func (a *GroupAddAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error

// ServiceEnableAction - enables a systemd service
type ServiceEnableAction struct{ BaseAction }
func (a *ServiceEnableAction) Name() string { return "service_enable" }
func (a *ServiceEnableAction) IsDeterministic() bool { return true }
func (a *ServiceEnableAction) Preflight(params map[string]interface{}) *PreflightResult
func (a *ServiceEnableAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error

// ServiceStartAction - starts a systemd service
type ServiceStartAction struct{ BaseAction }
func (a *ServiceStartAction) Name() string { return "service_start" }
func (a *ServiceStartAction) IsDeterministic() bool { return true }
func (a *ServiceStartAction) Preflight(params map[string]interface{}) *PreflightResult
func (a *ServiceStartAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error

// RequireCommandAction - verifies a command exists
type RequireCommandAction struct{ BaseAction }
func (a *RequireCommandAction) Name() string { return "require_command" }
func (a *RequireCommandAction) IsDeterministic() bool { return true }
func (a *RequireCommandAction) Preflight(params map[string]interface{}) *PreflightResult
func (a *RequireCommandAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error

// ManualAction - displays manual installation instructions
type ManualAction struct{ BaseAction }
func (a *ManualAction) Name() string { return "manual" }
func (a *ManualAction) IsDeterministic() bool { return true }
func (a *ManualAction) Preflight(params map[string]interface{}) *PreflightResult
func (a *ManualAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error
```

**Parameters per action (from design doc):**

| Action | Required Fields | Optional Fields |
|--------|-----------------|-----------------|
| `group_add` | `group` | - |
| `service_enable` | `service` | - |
| `service_start` | `service` | - |
| `require_command` | `command` | `version_flag`, `version_regex`, `min_version` |
| `manual` | `text` | - |

### Step 2: Implement Preflight validation for each action

Each action validates required parameters:

- `group_add`: requires `group` (string, non-empty, valid group name)
- `service_enable`: requires `service` (string, non-empty, valid service name)
- `service_start`: requires `service` (string, non-empty, valid service name)
- `require_command`: requires `command` (string, non-empty, safe characters only)
  - If `min_version` specified, requires `version_flag` and `version_regex`
- `manual`: requires `text` (string, non-empty)

### Step 3: Implement Execute stubs

Each Execute() logs what would be done (no side effects):

```go
func (a *GroupAddAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    group, ok := GetString(params, "group")
    if !ok {
        return fmt.Errorf("group_add action requires 'group' parameter")
    }
    fmt.Printf("   Would add user to group: %s\n", group)
    fmt.Printf("   (Skipped - requires sudo and system modification)\n")
    return nil
}
```

### Step 4: Register actions in action.go init()

Add to `init()`:
```go
// System configuration actions
Register(&GroupAddAction{})
Register(&ServiceEnableAction{})
Register(&ServiceStartAction{})
Register(&RequireCommandAction{})
Register(&ManualAction{})
```

### Step 5: Create unit tests

Test coverage:
- `TestGroupAddAction_Name`
- `TestGroupAddAction_IsDeterministic`
- `TestGroupAddAction_Preflight_Valid`
- `TestGroupAddAction_Preflight_MissingGroup`
- `TestGroupAddAction_Preflight_InvalidGroupName`
- `TestGroupAddAction_Execute`
- `TestGroupAddAction_Execute_MissingGroup`
- (Similar tests for ServiceEnableAction, ServiceStartAction, ManualAction)
- `TestRequireCommandAction_Name`
- `TestRequireCommandAction_IsDeterministic`
- `TestRequireCommandAction_Preflight_Valid`
- `TestRequireCommandAction_Preflight_MissingCommand`
- `TestRequireCommandAction_Preflight_MinVersionWithoutDetection`
- `TestRequireCommandAction_Execute_CommandExists`
- `TestRequireCommandAction_Execute_CommandNotFound`
- `TestRequireCommandAction_Execute_VersionCheck`

### Step 6: Verify tests pass and linting is clean

```bash
go test ./internal/actions/... -v
go vet ./internal/actions/...
```

## Relationship to require_system

Note: `RequireSystemAction` already exists and combines command checking with install guidance. The new `RequireCommandAction` is a simplified version focused solely on command verification (no `install_guide` parameter), as specified in the design doc's action vocabulary.

The existing `RequireSystemAction` can be deprecated later or kept for backwards compatibility.

## Test Plan

1. Unit tests for all action structs (Name, IsDeterministic, Preflight, Execute)
2. Test parameter validation (required fields, type checking)
3. Test group/service name validation (alphanumeric, hyphen, underscore)
4. Test command name validation (no path separators, shell metacharacters)
5. Verify action registration works (Get() returns correct action)

## Validation

- [ ] All 5 action structs implemented
- [ ] All structs embed BaseAction
- [ ] All structs implement Action interface
- [ ] All structs implement Preflight interface
- [ ] All actions registered in init()
- [ ] Unit tests pass
- [ ] go vet passes
