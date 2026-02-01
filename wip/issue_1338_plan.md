# Issue 1338 Plan

## Approach

Wire discovery fallback into install.go's normal install path. When a recipe isn't found and `--from` wasn't specified, run discovery to find the tool's source, then forward to the create+install pipeline.

## Changes

### 1. cmd/tsuku/install.go — Add discovery fallback in normal install loop

In the `for _, arg := range args` loop (line 195-199), after `runInstallWithTelemetry` returns an error:
- Check if the error is "recipe not found" (classifyInstallError returns ExitRecipeNotFound)
- If so, call `runDiscovery(toolName)`
- If discovery succeeds, print the discovery result reason, then forward to create pipeline (same as --from)
- If discovery fails, fall through to the existing error handling

Key detail: `installWithDependencies` prints "To create a recipe..." on stderr at lines 225-227 before returning the error. This message is confusing when discovery is about to try. Options:
- Suppress that message by checking if discovery fallback is available first
- Or accept the momentary noise (not great UX)

Better approach: Add a pre-check before calling runInstallWithTelemetry — first try loader.Get() to see if recipe exists. If not, try discovery immediately, before the full install flow. This avoids the noisy error message.

### 2. cmd/tsuku/install.go — Handle --deterministic-only with discovery

When discovery runs, the `--deterministic-only` constraint needs to propagate. The ChainResolver doesn't know about this flag, but create.go's pipeline does (via createDeterministicOnly). So discovery finds the builder+source, then the create pipeline enforces the deterministic-only guard.

### 3. Functional tests

Add scenarios:
- Discovery fallback finds tool via registry (use jq which is in discovery.json)
- Unknown tool shows actionable error (nonexistent-tool)
- Existing recipe install unchanged (already tested but good to verify)

## Files to modify
- `cmd/tsuku/install.go` — discovery fallback logic
- `test/functional/features/install.feature` — new scenarios
