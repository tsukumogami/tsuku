# Architect Review: Issue 4 - feat(cli): implement tsuku registry subcommands

## Scope

New files: `cmd/tsuku/registry.go`, `cmd/tsuku/registry_test.go`
Modified files: `cmd/tsuku/main.go` (command registration), `internal/userconfig/userconfig.go` (RegistryEntry, StrictRegistries, Registries fields)

## Findings

### 1. CLI surface overlap between `registry` and `update-registry` -- Advisory

`cmd/tsuku/main.go:102` registers `updateRegistryCmd` and line 111 registers `registryCmd`. These are sibling commands on root with overlapping naming: `tsuku update-registry` refreshes the central registry cache, while `tsuku registry` manages distributed sources. A user running `tsuku registry update` would expect it to exist but it doesn't; `update-registry` handles a different concept (cache refresh of the central registry).

This isn't blocking today because the two commands manage genuinely different things (cache refresh vs. source configuration), and `update-registry` predates the distributed registry feature. However, when Issue 5+ lands the distributed HTTP fetching layer, `update-registry` will likely need to refresh distributed caches too. At that point, consolidating under `tsuku registry refresh` would prevent the two surfaces from diverging. Worth tracking but not blocking this PR.

**Severity: Advisory.**

### 2. Command structure follows established pattern -- No issue

`registry.go` follows the same pattern as `config.go` and `cache.go`: parent command with `Run: func() { cmd.Help() }`, subcommands registered via `init()`, and `rootCmd.AddCommand(registryCmd)` in `main.go:init()`. Exit codes use the existing constants (`ExitGeneral`, `ExitUsage`). Error formatting matches the `fmt.Fprintf(os.Stderr, ...)` convention used everywhere else. No parallel pattern introduced.

### 3. Validation logic duplicated between command and test -- Advisory

`registry.go:130-137` contains a two-step validation sequence (call `discover.ValidateGitHubURL`, then check slash count). `registry_test.go:232-239` duplicates this into `validateRegistrySourceForTest()`. If the validation logic in `runRegistryAdd` changes, the test helper must be updated in lockstep. Extracting a `validateRegistrySource(source string) error` function from the command handler would let the test call the real function and remove the duplication risk.

This doesn't compound -- only two callsites, both in the same package -- so it's advisory.

**Severity: Advisory.**

### 4. State access in `printToolsFromSource` -- No issue

`registry.go:206-213` creates a `config.DefaultConfig()` and `install.New()` to read state. This follows the same pattern used in `remove.go`, `list.go`, and other commands that need to read installed tool state. The dependency direction is correct: `cmd/tsuku` imports `internal/config` and `internal/install`, not the reverse.

### 5. userconfig additions fit the existing schema pattern -- No issue

`RegistryEntry` struct and the `Registries` / `StrictRegistries` fields on `Config` use the same TOML tagging conventions (`toml:"...,omitempty"`) as existing fields. The `omitempty` tags ensure empty registries don't produce spurious TOML output. `DefaultConfig()` doesn't initialize these fields (zero values are correct: `nil` map, `false` strict mode), which matches how `Secrets` is handled.

The new fields are consumed by `registry.go` (list, add, remove commands), satisfying the state contract requirement that every persisted field has at least one consumer.

### 6. No `Get`/`Set` integration for `strict_registries` -- Advisory

`userconfig.go` gains the `StrictRegistries` field, but `Get()` and `Set()` (the switch statements at lines 323 and 365) don't handle it. This means `tsuku config get strict_registries` returns "unknown config key" and `tsuku config set strict_registries true` errors. The plan says `tsuku registry list` should display strict_registries status (which it does), but the `config` command is the established way to manage boolean settings.

This is advisory because the acceptance criteria for Issue 4 don't require it, and adding it later is a self-contained change to `userconfig.go`'s switch statements. But users will likely try `tsuku config set strict_registries true` before discovering the correct approach isn't yet documented.

**Severity: Advisory.**

## Summary

No blocking findings. The change follows the established CLI command structure, uses correct dependency directions, and extends the userconfig schema cleanly. Three advisory items: potential future CLI surface overlap with `update-registry`, duplicated validation logic, and missing `config get/set` support for `strict_registries`.
