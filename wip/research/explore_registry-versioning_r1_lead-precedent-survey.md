# Lead: How do other package managers handle schema versioning?

## Findings

### Homebrew
Git-based recipes with no explicit schema negotiation. Format changes are pushed through the tap (git repo) and clients update by pulling. No version handshake -- compatibility is managed by keeping the client and tap in sync through frequent updates. Breaking changes are rare and handled by requiring client upgrades.

### Cargo Registries
Cargo's registry index includes a `v` field (integer) per index entry. The client checks this field and ignores entries with versions it doesn't understand. This is forward-compatible: old clients skip new-format entries without failing. The index also has a `config.json` with API version info.

### npm
The npm registry uses HTTP API versioning. The registry response format has evolved but maintains backward compatibility. No explicit schema version in package metadata. Breaking changes are handled through API endpoint versioning.

### Terraform
Terraform's provider registry protocol requires clients to ignore unknown object properties (forward-compatible by design). The protocol uses versioned API endpoints (`/v1/providers/...`). Clients negotiate capabilities through the discovery document. This is one of the more explicit handshake protocols.

### Docker Registry
The most instructive cautionary tale. Docker's v1 to v2 registry migration (2015-2019) was painful:
- Clients couldn't auto-upgrade
- Silent failures occurred when old clients hit v2 registries
- Manual migration was required
- Took ~4 years of deprecation timeline
- Required external migration tools

### Go Modules
Go modules use a v2+ path-based versioning approach. Breaking changes require a new import path (`/v2`). This avoids schema negotiation entirely by making incompatible versions separate entities.

### General Patterns

**What works:**
- Declarative version field that clients check (Cargo's integer `v` field)
- Additive-only changes with unknown field tolerance (Terraform, TOML/JSON default)
- Published deprecation timelines of 6-12 months (Docker, though painful)
- HTTP error responses (416/error) for incompatible clients, not silent 200 OK

**What fails:**
- Silent breaking changes with no client signal (Docker v1 -> v2 early days)
- Relying on clients to "just update" (Homebrew works because of auto-update, but distributed registries can't assume that)

## Implications

- Integer version fields (like Cargo) are simpler and sufficient -- semver is overkill for schema negotiation
- Forward compatibility through unknown-field tolerance is the baseline (tsuku already has this via TOML/JSON)
- The hard problem is breaking changes: tsuku needs an explicit signal, not silent failure
- Distributed registries can't rely on coordinated rollouts, so the protocol must handle version heterogeneity

## Surprises

- No decentralized package manager has a tested pattern for schema compatibility across heterogeneous registry implementations
- Most systems avoid the problem entirely by never making breaking changes
- Docker's 4-year migration is the strongest evidence that breaking changes should be extremely rare

## Open Questions

- For distributed registries, how should multiple instances negotiate schema versions when not centrally coordinated?
- Is there value in HTTP-level version negotiation (Accept headers) vs manifest-level fields for tsuku's case?

## Summary

Most package managers avoid breaking schema changes entirely; when forced, declarative version fields + additive-only changes + long deprecation timelines (6-12 months) work best. Cargo's integer version field with "skip what you don't understand" is the simplest proven pattern. The biggest open question is how distributed registries handle schema heterogeneity without central coordination.
