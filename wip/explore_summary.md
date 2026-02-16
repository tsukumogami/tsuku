# Exploration Summary: Secrets Manager

## Problem (Phase 1)
LLM API keys are read via scattered `os.Getenv()` calls with no centralized resolution and no config file alternative, forcing users to manage environment variables even when a persistent config file would be more convenient.

## Decision Drivers (Phase 1)
- Environment variables must keep priority (12-factor convention)
- Config file must enforce 0600 permissions (secrets contain API keys)
- Resolution logic centralized in one package, not scattered per-provider
- Must integrate with existing `$TSUKU_HOME/config.toml` and userconfig patterns
- Minimal API surface - callers just ask for a key by name

## Research Findings (Phase 2)
- Upstream design (DESIGN-llm-builder-infrastructure) explicitly specifies `internal/secrets/manager.go` with env > config > error resolution
- `internal/userconfig` already manages `$TSUKU_HOME/config.toml` via BurntSushi/toml - can extend with `[secrets]` section
- `internal/config` already defines `ConfigFile` path as `$TSUKU_HOME/config.toml`
- Current env var usage: `ANTHROPIC_API_KEY` (claude.go), `GOOGLE_API_KEY`/`GEMINI_API_KEY` (gemini.go), `GITHUB_TOKEN` (llm_discovery.go, config.go)
- `userconfig.saveToPath()` uses `os.Create()` with no permission control - needs fixing for secrets
- The `LLMIdleTimeout()` method already demonstrates env-var-over-config pattern we'll follow

## Options (Phase 3)
- D1: [secrets] section in config.toml vs separate secrets file vs encrypted file
- D2: Warn+tighten on write vs refuse permissive files vs silent enforcement

## Decision (Phase 5)

**Problem:**
Tsuku's LLM and discovery features require API keys that are currently read via scattered `os.Getenv()` calls across multiple packages, with no centralized resolution and no config file alternative. This forces users to manage environment variables even when persistent config file storage would be more convenient, and means each new provider re-implements the same key-lookup pattern with inconsistent error messages.

**Decision:**
Add an `internal/secrets` package with a `Get(name)` function that resolves keys by checking environment variables first, then a `[secrets]` section in `$TSUKU_HOME/config.toml`, then returning an error with guidance. File permissions are enforced at write time (0600, atomic temp+rename) with a warning on read if existing files are too permissive. Multi-env-var keys (like Google's two variable names) are handled via a hardcoded alias table.

**Rationale:**
Centralizing key resolution in one package eliminates scattered `os.Getenv()` patterns and gives consistent error messages across all providers. Using the existing `config.toml` with a `[secrets]` section follows the upstream design specification and avoids introducing a second config file. Warn-on-read plus enforce-on-write for permissions avoids breaking existing installations while ensuring secrets don't persist in world-readable files. This matches the permission model used by similar tools (AWS CLI, GitHub CLI).

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-16
