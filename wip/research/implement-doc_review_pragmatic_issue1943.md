# Pragmatic Review: Issue #1943 (--env flag for environment variable passthrough)

**0 blocking, 2 advisory**

The implementation is well-scoped to the issue requirements. The three-layer design (CLI flag parsing in `resolveEnvFlags`, field on `SandboxRequirements`, filtering in `filterExtraEnv`) matches the design doc and doesn't over-abstract. No dead code, no speculative generality.

## Advisory

1. **No unit tests for `resolveEnvFlags`** -- `cmd/tsuku/install_sandbox.go:153`. This function has two branches (KEY=VALUE passthrough vs KEY-only host read) and the host-read branch calls `os.Getenv`. The `filterExtraEnv` function in the sandbox package is well-tested, but `resolveEnvFlags` has zero test coverage. The KEY-only branch silently produces `KEY=` when the host var is unset, which is correct docker-compatible behavior but worth a test to document that contract.

2. **`TestSandboxRequirements_ExtraEnvField`** -- `internal/sandbox/executor_test.go:863`. This test constructs a struct literal and asserts the fields have the values it just set. It tests Go struct semantics, not application logic. Harmless but zero signal.
