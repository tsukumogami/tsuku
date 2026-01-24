---
status: Current
problem: Tsuku lacks visibility into recipe usage patterns, platform distribution, and user preferences, making it impossible to prioritize maintenance and feature development based on actual usage data.
decision: Implement a privacy-first telemetry client in the CLI that collects anonymous usage statistics (action type, recipe name, version, platform, and dependency status) with environment variable opt-out and transparent first-run notice.
rationale: Usage data enables evidence-based decisions on recipe maintenance prioritization and platform support. The opt-out model ensures meaningful data volume while respecting user choice through easy disabling and clear privacy guarantees.
---

# Design: Telemetry CLI Integration

## Status

Current

## Context and Problem Statement

Tsuku currently has no visibility into which recipes are being used, how often, or on which platforms. This makes it difficult to:

1. **Prioritize maintenance**: Without usage data, we can't know which recipes deserve the most attention when they break or need updates.

2. **Understand user needs**: We don't know if users prefer version constraints (`@LTS`), explicit versions, or just `latest`. We don't know which tools are installed as dependencies vs explicitly.

3. **Make informed decisions**: Feature prioritization and platform support decisions are based on guesswork rather than data.

### Success Criteria

- Identify which 10 recipes represent 80% of installs within 30 days of launch
- Understand platform distribution (Linux vs macOS, amd64 vs arm64)
- Determine what percentage of installs are explicit vs dependencies

### Why Now?

The telemetry backend (tsuku-telemetry) is being designed now. Defining the CLI integration before the backend is finalized ensures the event schema meets CLI capabilities and avoids breaking changes later.

### Scope

**In scope:**
- Telemetry client implementation in CLI
- Event schema for install/update/remove actions
- Opt-out mechanisms (environment variable and config)
- First-run notice for transparency
- Documentation of what's collected

**Out of scope:**
- Backend implementation (separate repo: tsuku-telemetry)
- Dashboard visualization (separate repo: tsuku.dev)
- Command tracking (e.g., `tsuku list --json`) - deferred to future iteration
- Recipe creation telemetry - deferred to future iteration

## Decision Drivers

1. **Privacy by design**: Collect only what's necessary; no user identifiers, no PII
2. **Non-blocking**: Telemetry must never slow down user commands
3. **Transparent**: Users must know what's collected and how to opt out
4. **Enabled by default**: Opt-out (not opt-in) to get meaningful data volume
5. **Phased implementation**: Split into manageable issues that can be delivered incrementally
6. **Backend compatibility**: Schema must work with Cloudflare Analytics Engine constraints (20 blobs max)
7. **Testability**: Must be testable without sending real events (mock/dry-run mode)
8. **Debuggability**: Should support debug mode to inspect events before sending

## Assumptions

- **Network reliability**: Most users have internet when running commands. Users in airgapped environments can use `TSUKU_NO_TELEMETRY=1`.
- **CI detection**: Telemetry will NOT auto-disable in CI. Users can set `TSUKU_NO_TELEMETRY=1` in CI pipelines if desired.
- **Backend availability**: tsuku-telemetry must be production-ready before shipping telemetry. Events sent to a non-ready backend will silently fail.
- **Silent failures**: Telemetry failures produce no user-visible errors or warnings.
- **IP logging**: The backend does not log or store IP addresses. This is enforced by the Cloudflare Worker design.
- **Schema versioning**: Events include a `schema_version` field for future evolution.

## Uncertainties

- **Config system scope**: We haven't designed `tsuku config` yet. This will be a separate design effort for Issue B.
- **Backend readiness**: tsuku-telemetry#13 must be complete before any telemetry can be sent.

## Solution Architecture

### Event Schema

Events contain the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `action` | string | "install", "update", or "remove" |
| `recipe` | string | Recipe name (e.g., "nodejs") |
| `version_constraint` | string | User's original constraint (e.g., "@LTS", ">=1.0", or empty) |
| `version_resolved` | string | Actual version installed/updated to |
| `version_previous` | string | Previous version (for update/remove) |
| `os` | string | Operating system ("linux", "darwin") |
| `arch` | string | CPU architecture ("amd64", "arm64") |
| `tsuku_version` | string | Version of tsuku CLI |
| `is_dependency` | bool | True if installed as a transitive dependency |
| `schema_version` | string | Event schema version for evolution |

### Client Behavior

The telemetry client follows these principles:

- **Fire-and-forget**: Events are sent asynchronously in a background goroutine. The client never blocks the main command execution.
- **Timeout**: Each HTTP request has a 2-second timeout to prevent hanging on network issues.
- **Silent failures**: All errors (network, timeout, server errors) are silently ignored. Telemetry must never interfere with normal operation.
- **No retries**: Failed events are dropped. We're tracking trends, not exact counts.

### Opt-Out Mechanism

Telemetry is disabled when:
1. `TSUKU_NO_TELEMETRY` environment variable is set (any non-empty value)
2. (Phase 2) `telemetry` is set to `false` in config file

The opt-out check happens at client initialization. When disabled, `Send()` returns immediately without spawning goroutines.

### First-Run Notice

On first telemetry-enabled command, display a notice to stderr:

```
tsuku collects anonymous usage statistics to improve the tool.
No personal information is collected. See: https://tsuku.dev/telemetry

To opt out: export TSUKU_NO_TELEMETRY=1
```

State is persisted via a marker file (`~/.tsuku/telemetry_notice_shown`). The notice uses stderr to avoid interfering with command output (e.g., `tsuku list --json`).

### Integration Guidelines

**When to send events:**
- Send after the operation succeeds, not before
- For updates/removes, capture the previous version before the operation

**What constitutes a successful operation:**
- `install`: Tool files are in place and symlinks created
- `update`: New version installed and symlinks updated
- `remove`: Tool files and symlinks removed

**Determining `is_dependency`:**
- Track whether the install was triggered by user command (explicit) or by dependency resolution (dependency)
- This information should flow through the install call chain

### Debug Mode

When `TSUKU_TELEMETRY_DEBUG=1`:
- Print event JSON to stderr instead of sending
- Useful for users to verify what would be collected

## Security Considerations

### Data Minimization

**Collected:**
- Action type (install/update/remove)
- Recipe name (public information)
- Version information (public information)
- OS and architecture (broad categories)
- tsuku version (public information)
- Whether install was explicit or dependency

**NOT collected:**
- IP addresses (backend does not log)
- User identifiers (no UUID, no MAC hash, no fingerprinting)
- File paths
- Environment variables
- Hostnames
- Any personally identifiable information

### Network Security

- All telemetry sent over HTTPS to `https://telemetry.tsuku.dev/event`
- TLS 1.2+ required
- No sensitive data in request (even if intercepted, only recipe names visible)
- 2-second timeout prevents hanging on network issues

### User Control

- **Opt-out is easy**: Single environment variable (`TSUKU_NO_TELEMETRY=1`)
- **Opt-out is immediate**: No grace period, no "one last event"
- **Debug mode**: Users can see exactly what would be sent before enabling
- **First-run notice**: Users are informed before any telemetry is sent

### Backend Trust Model

The telemetry backend (tsuku-telemetry) is a Cloudflare Worker that:
- Does NOT log IP addresses
- Does NOT correlate events across sessions
- Stores only aggregate data in Cloudflare Analytics Engine
- Has 3-month retention (Analytics Engine default)

### Supply Chain Considerations

This feature does not affect tsuku's core security model:
- Does not change how binaries are downloaded or verified
- Does not introduce new dependencies
- Does not access or transmit any data about installed tools beyond recipe names
- Telemetry failures do not affect tool installation

### Abuse Vectors

| Vector | Risk | Mitigation |
|--------|------|------------|
| Backend compromise | Low | Only recipe names exposed; no user data |
| MITM on telemetry | Low | HTTPS required; no sensitive data in transit |
| Event injection | Low | Backend validates schema; no privilege escalation possible |
| DoS via telemetry | Low | Fire-and-forget; failures don't affect CLI |

## Implementation Phases

### Phase 1: Telemetry Client

Core telemetry functionality with environment variable opt-out:
- `internal/telemetry` package with client and event types
- Fire-and-forget HTTP client with 2-second timeout
- `TSUKU_NO_TELEMETRY=1` environment variable opt-out
- First-run notice with state file
- Integration into install/update/remove commands
- Debug mode via `TSUKU_TELEMETRY_DEBUG=1`

### Phase 2: Config System

General configuration infrastructure with telemetry setting:
- `tsuku config get/set` commands
- `~/.tsuku/config.toml` file
- `tsuku config set telemetry false` as alternative opt-out

## Consequences

### Positive

- Data-driven prioritization of recipe maintenance
- Understanding of platform distribution helps target testing
- Insight into version constraint usage informs feature development
- Transparency builds user trust

### Negative

- Some users may object to opt-out model (mitigated by easy opt-out and transparency)
- Backend dependency for full functionality (mitigated by silent failures)
- Additional code to maintain (~350 lines)

## Implementation Issues

- [#79](https://github.com/tsukumogami/tsuku/issues/79): Umbrella issue

### Phase 1: Core Telemetry
- [#82](https://github.com/tsukumogami/tsuku/issues/82): feat(telemetry): add client with schema and env var opt-out
- [#83](https://github.com/tsukumogami/tsuku/issues/83): feat(telemetry): add first-run notice
- [#84](https://github.com/tsukumogami/tsuku/issues/84): feat(telemetry): integrate into install/update/remove commands

### Phase 2: Config System
- [#85](https://github.com/tsukumogami/tsuku/issues/85): feat(config): add config system with telemetry setting

