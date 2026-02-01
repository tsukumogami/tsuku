## Summary

`tsuku info <nonexistent-tool>` prints "Tool not found in registry" but exits with code 0. Other commands like `tsuku versions` correctly exit with code 3 for the same condition.

## Reproduction

```bash
tsuku info nonexistent-tool
echo $?  # prints 0
```

**Actual output:**
```
Tool 'nonexistent-tool' not found in registry.
```
Exit code: 0

**Expected:** Non-zero exit code (3, matching `tsuku versions` behavior for not-found).

## Environment

- tsuku v0.3.2-dev (built from source)
- Linux amd64
- Isolated test environment
