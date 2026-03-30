<!-- decision:start id="pin-level-representation" status="assumed" -->
### Decision: Pin Level Representation

**Context**

Tsuku stores the user's install-time version constraint in `VersionState.Requested` (a plain string in state.json). When a user runs `tsuku install node@18`, the Requested field stores `"18"`. The PRD decided (D2) that pin level should be inferred from the number of version components: empty string means latest, one component means major pin, two means minor, three-or-more means exact. Channel-style constraints like `@lts` are stored with their `@` prefix.

The question is whether pin level should remain implicit (derived from Requested at runtime) or be materialized as an explicit field in state.json. This decision underpins auto-update features 2-9 in the channel-aware resolution design.

**Assumptions**

- No user will need a pin level that contradicts their version component count (e.g., typing "18.1" but wanting major-only updates). If this assumption is wrong, an explicit override field can be added later without migration issues since the implicit derivation still works as the default.
- Channel constraints (@lts) will be treated as a distinct pin type that resolves dynamically. The channel's update behavior is separate from numeric pin levels.

**Chosen: Implicit Derivation (No Schema Change)**

Add a `PinLevelFromRequested(requested string) PinLevel` pure function that computes pin level from the existing Requested string. Define `PinLevel` as a Go enum type (const block with iota) with values: `PinLatest`, `PinMajor`, `PinMinor`, `PinExact`, `PinChannel`. The function uses these rules:

- `""` (empty) -> `PinLatest`
- Starts with `@` -> `PinChannel`
- 1 dot-separated numeric component (`"18"`) -> `PinMajor`
- 2 dot-separated numeric components (`"1.29"`) -> `PinMinor`
- 3+ dot-separated numeric components (`"1.29.3"`) -> `PinExact`

No new fields in `VersionState`. No migration. The function lives in a new file (e.g., `internal/install/pin.go` or alongside version utilities) and is called at the point of use: update checks, version filtering, and display formatting.

CalVer versions like `"2024.01"` map to `PinMinor` (2 components). This is the accepted trade-off from PRD decision D2 -- CalVer users who want exact pinning use all three components.

**Rationale**

Pin level is a deterministic function of the Requested string. Storing it separately in state.json would introduce denormalization with zero current benefit: the derived value is always correct and cheap to compute (string split + count). Adding an explicit field creates sync risk -- if Requested changes but PinLevel doesn't get updated, the system behaves incorrectly. The existing `VersionState.Requested` field already captures the user's full intent. The "future override" argument is speculative; if needed later, an optional `PinLevelOverride` field can be added without breaking the implicit default.

This approach also aligns with `.tsuku.toml`'s `ToolRequirement.Version` string field, keeping both data models consistent: version constraints are always plain strings, pin level is always derived.

**Alternatives Considered**

- **Explicit PinLevel field**: Add `PinLevel string` to VersionState, computed at install time and stored alongside Requested. Rejected because it denormalizes state without current need. The field would always be derivable from Requested, creating redundancy and sync risk. Migration logic would be needed (yet another migrateX function in state.go) for a field that adds no information. If override support is ever needed, it can be added then as a separate optional field.

- **Structured constraint object**: Replace Requested with a JSON object containing raw input, pin level, and channel. Rejected because it's a breaking schema change to the VersionState struct, incompatible with the simple string model used in `.tsuku.toml`, and significantly over-engineered for representing something a string already captures. The migration burden is high with no proportional benefit.

**Consequences**

Pin level logic becomes a runtime computation rather than a stored value. Every code path that needs pin level calls the derivation function. This is a feature, not a cost: it means pin level is always consistent with Requested and can never drift. If performance ever matters (it won't -- this is a string split), results can be cached in memory without persisting to disk.

The main trade-off is that if tsuku later needs pin level overrides (letting users decouple pin level from their version string), a new field must be added at that point. This is an acceptable cost: adding an optional field is a non-breaking change, and the implicit derivation serves as the natural default.
<!-- decision:end -->
