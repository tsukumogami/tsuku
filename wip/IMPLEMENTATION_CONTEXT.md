## Problem

The sandbox executor cannot run tests on Alpine Linux because the tsuku binary is dynamically linked against glibc, while Alpine uses musl libc.

```
/workspace/sandbox.sh: line 13: tsuku: not found
```

The binary exists but the dynamic linker fails because `/lib64/ld-linux-x86-64.so.2` doesn't exist on Alpine.

## Context

This was discovered while implementing #774 (golden files for system dependency recipes). Sandbox testing works for debian, rhel, arch, and suse families, but fails on Alpine.

## Options

1. **Static build**: Build tsuku with `CGO_ENABLED=0` for a fully static binary
2. **Musl cross-compile**: Build a separate Alpine-compatible binary using musl toolchain
3. **Container-based solution**: Build tsuku inside the Alpine container before running tests

## Acceptance Criteria

- [ ] Sandbox tests pass on Alpine Linux containers
- [ ] Alpine golden files can be validated in CI
