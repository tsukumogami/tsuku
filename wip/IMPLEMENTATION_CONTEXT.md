## Summary

The Cargo Builder CI workflow fails because the installed `hyperfine` binary is not found in PATH during the verification step.

## Error

```
/home/runner/work/_temp/xxx.sh: line 1: hyperfine: command not found
Process completed with exit code 127
```

## Context

- Started failing on main: 2026-01-13
- Affects both Linux and macOS jobs
- The installation step succeeds but the binary isn't accessible in PATH for verification

## Investigation

The workflow installs hyperfine via tsuku but the PATH setup may not be taking effect for the verification step, or the binary symlink isn't being created correctly.

## Acceptance Criteria

- [ ] Cargo Builder: Linux passes
- [ ] Cargo Builder: macOS passes
