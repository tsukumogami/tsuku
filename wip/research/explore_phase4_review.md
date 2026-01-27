# Phase 4 Review: Registry Recipe Cache Policy Design

## Review Questions Addressed

### 1. Is the problem statement specific enough to evaluate solutions against?

**Assessment: Mostly adequate, with minor gaps**

The problem statement correctly identifies four core issues:
1. Recipes cached indefinitely (no TTL)
2. No metadata tracking (timestamps, access times)
3. Network failures cause hard errors despite cached data
4. No size limits (unbounded growth)

**Gaps identified:**

- **Missing quantification**: "GB of cached data" is vague. A typical recipe TOML is 1-5KB, so 150 recipes would be ~750KB total. The 500MB limit from upstream design seems excessive for recipe caching alone. This suggests the limit may be inherited from download cache patterns without recipe-specific analysis.

- **Access pattern assumptions**: The design assumes LRU eviction is appropriate, but recipes are typically accessed in bursts (install once, rarely re-read). Access recency may not correlate with future need.

- **TTL justification missing**: Why 24 hours specifically? The upstream design mandates this but doesn't explain why 24 hours balances freshness vs. network overhead.

**Recommendation**: Add a section quantifying expected cache sizes (150 recipes * ~3KB average = ~450KB baseline) and clarifying that the 500MB limit is a generous upper bound that includes future registry growth.

---

### 2. Are there missing alternatives we should consider?

**Yes, several alternatives are worth documenting:**

#### Metadata Storage - Missing Option 1D: Extended File Attributes

Use filesystem extended attributes (xattr) to store metadata directly on recipe files.

**Pros:**
- No sidecar files
- Single file per recipe
- Native filesystem support (Linux, macOS)
- Atomic with file operations

**Cons:**
- Not supported on all filesystems (FAT32, some network mounts)
- Harder to inspect without xattr tools
- No Windows support
- No codebase precedent

**Assessment**: Not recommended due to portability concerns, but worth mentioning as rejected alternative.

#### Stale Fallback - Missing refinement for Option 2B

The current stale-if-error description doesn't specify *how stale* is acceptable. Consider:

- **2B-Bounded**: Stale-if-error up to 7 days, then hard fail
- **2B-Unbounded**: Stale-if-error regardless of age (current description implies this)

The bounded variant prevents users from unknowingly using recipes that are months or years old.

**Recommendation**: Specify a maximum staleness threshold (e.g., 7 days) beyond which stale fallback is disabled with a more prominent warning.

#### LRU Eviction - Missing Option 3D: Time-based Eviction

Evict recipes older than N days, regardless of access pattern.

**Pros:**
- Simple to implement
- Guarantees eventual cleanup
- Doesn't require access time tracking

**Cons:**
- May evict frequently-used recipes
- Doesn't directly address size limits

**Assessment**: Could complement LRU for recipes that are cached but never re-accessed.

#### Missing consideration: Partial refresh

None of the options address partial refresh scenarios:
- What if only 3 of 10 cached recipes are stale?
- Does `update-registry` refresh all or only stale entries?
- Should expired entries be refreshed lazily on access or eagerly on command?

**Recommendation**: Add clarification that `update-registry` refreshes all cached recipes regardless of staleness.

---

### 3. Are the pros/cons for each option fair and complete?

**Assessment: Generally fair, with some additions needed**

#### Option 1A (JSON Sidecars)

Missing pro: **Survives recipe file replacement** - If the user manually updates a cached recipe file, the sidecar can indicate "modified locally" vs "fetched from registry."

Missing con: **Sync complexity** - If a recipe file is deleted but metadata remains (or vice versa), the cache is in an inconsistent state. Need cleanup logic.

#### Option 1B (Embedded Headers)

Missing pro: **Portable** - Moving a cached recipe file preserves its metadata.

Missing con: **Validation complexity** - Must strip metadata before validating recipe against registry source for integrity checks.

#### Option 1C (Central DB)

Missing pro: **Fast enumeration** - Can list all cached recipes without scanning directories.

Missing con: **Recovery difficulty** - If `cache.json` is corrupted, all metadata is lost. No way to recover timestamps from individual files.

#### Option 2A (Strict TTL)

The "Users always get fresh-ish data" pro is misleading. Even with strict TTL, users get stale data for the duration of the TTL. The real pro is **predictable behavior** - cache hit/miss is deterministic based on TTL.

#### Option 2B (Stale-If-Error)

Missing con: **Security implications** - A compromised recipe could persist longer during network issues. If a malicious recipe is pushed and then the legitimate version restored, users with network issues would continue using the malicious version.

#### Option 3A (Evict on Write)

Missing pro: **Deterministic** - Cache size is always bounded.

Missing con: **Installation latency** - Eviction during install adds latency to user-facing operation. Could evict recipes that will be needed moments later (installing multiple tools from same author).

#### Option 3B (Evict on Threshold)

Missing clarification: What's the "threshold" and "headroom"? The design mentions 80%/60% but doesn't explain why these values.

#### Option 4B (Templates Only)

Missing con: **Harder to test** - With structured types, tests can assert error type; with templates, tests must parse message strings.

---

### 4. Are there unstated assumptions that need to be explicit?

**Yes, several assumptions should be stated:**

1. **Assumption: Recipe files are small**
   - The design implicitly assumes recipes are kilobytes, not megabytes
   - If recipes grow (embedded binaries, large metadata), cache sizing changes dramatically
   - Make explicit: "Recipes are expected to remain under 10KB each"

2. **Assumption: Single user per cache**
   - The design doesn't address multi-user scenarios ($TSUKU_HOME shared)
   - Should state: "The registry cache is per-user and not designed for shared access"

3. **Assumption: Network latency dominates**
   - Stale-if-error assumes network failures are rare and transient
   - In environments with persistent network restrictions (air-gapped, corporate firewalls), this assumption fails
   - Should state: "This design targets environments with generally reliable but occasionally failing network access"

4. **Assumption: File system timestamps are reliable**
   - LRU eviction using access times requires `atime` to be enabled
   - Many systems mount with `noatime` or `relatime` for performance
   - If using file modification time instead, access tracking is lost
   - Make explicit: "Access time tracking uses metadata sidecars, not filesystem atime"

5. **Assumption: Recipe changes are infrequent**
   - The 24-hour TTL assumes recipes don't change multiple times per day
   - For rapidly-developed recipes, this could cause consistency issues
   - Should state: "Recipe updates are expected to be daily or less frequent"

6. **Assumption: Registry is single-source**
   - The design assumes one registry (GitHub). Multiple registries or mirrors aren't considered.
   - Make explicit: "This design targets single-registry deployments"

---

### 5. Is any option a strawman (designed to fail)?

**Assessment: No obvious strawmen, but some imbalance exists**

#### Option 1B (Embedded Headers)

While listed with reasonable pros, this option has a fundamental issue that makes it impractical: TOML comment parsing is non-standard and would require custom parser logic. The cons understate this - it's not just "harder to update" but "requires non-trivial parser changes." However, it's not a strawman because the portability benefit is genuine.

#### Option 2A (Strict TTL)

This option is fairly presented, but the "Poor" user experience rating in the evaluation table may be overstated. For users with reliable networks, strict TTL provides simpler, more predictable behavior. The option is viable for power users who prefer freshness guarantees.

#### Option 3C (Manual Only)

This has the weakest position but represents a legitimate design philosophy (explicit over implicit). It's not a strawman - package managers like Homebrew and npm take this approach. The "Poor" disk management rating is accurate but not disqualifying.

#### Option 4B (Templates Only)

This option is underdeveloped compared to 4A. It lacks concrete examples of what "message templates" would look like and how they'd be organized. This makes it harder to evaluate fairly. Adding example templates would strengthen the comparison.

---

## Additional Findings

### Consistency with Existing Codebase Patterns

The design correctly identifies `internal/version/cache.go` as the reference pattern. However, there are additional patterns worth noting:

1. **Download cache** (`internal/actions/download_cache.go`):
   - Uses `.data` + `.meta` sidecar pattern
   - Includes security checks (symlink detection, permission validation)
   - No TTL - relies on checksum verification

2. **Error types** (`internal/registry/errors.go`):
   - Already has 9 error types with classification
   - `Suggestion()` method provides actionable guidance
   - Adding cache-specific types fits this pattern well

3. **Cache commands** (`cmd/tsuku/cache.go`):
   - Existing `cache info` and `cache clear` commands
   - Registry cache should integrate with this structure
   - Consider `cache clear --registry` flag

**Recommendation**: The registry cache should:
- Use the sidecar metadata pattern (consistent with version cache)
- Include the security checks from download cache
- Add new error types to existing errors.go
- Integrate with existing cache CLI commands

### Missing Security Consideration

The design references security in DESIGN-recipe-registry-separation.md but doesn't address cache-specific security:

1. **Cache poisoning window**: During the TTL window, a compromised recipe persists. The design should document this risk.

2. **Stale fallback amplifies risk**: With stale-if-error, a malicious recipe could persist indefinitely if the network is unavailable. Consider a maximum staleness bound (e.g., 7 days) after which stale fallback is disabled.

3. **No integrity verification**: Unlike download cache which verifies checksums, recipes are cached without integrity verification. If the cache file is modified locally, there's no detection.

**Recommendation**: Add a "Cache Security" section addressing these points, or explicitly note they're out of scope for this design.

### Evaluation Table Adjustments

The evaluation table rates options but doesn't weight decision drivers. Suggest adding weights:

| Decision Driver | Weight | Rationale |
|-----------------|--------|-----------|
| User experience during network issues | High | Primary user-facing concern |
| Disk space management | Medium | Only matters for constrained environments |
| Operational visibility | Medium | Important for debugging |
| Consistency with existing patterns | High | Reduces implementation risk |
| Backwards compatibility | High | Must not break existing workflows |
| Configurability | Low | Power user feature |

With weights, stale-if-error (2B) becomes the clear winner for fallback behavior.

---

## Summary of Recommendations

### Critical (must address before decision)

1. **Specify maximum staleness bound**: Add a 7-day maximum for stale-if-error fallback to prevent indefinite malicious recipe persistence.

2. **Clarify 500MB limit rationale**: The limit seems excessive for recipe caching (~450KB baseline). Either justify or reduce.

3. **Add security section**: Document cache poisoning risk and mitigation.

### Important (should address)

4. **Document access time tracking mechanism**: Clarify that LRU uses sidecar metadata, not filesystem atime.

5. **Specify `update-registry` behavior**: Does it refresh all recipes or only stale ones?

6. **Add bounded staleness variant**: Option 2B should specify "stale-if-error up to 7 days."

### Minor (nice to have)

7. **Add xattr as rejected alternative**: For completeness in metadata storage options.

8. **Expand Option 4B**: Add example message templates.

9. **Integrate with existing cache CLI**: Plan for `cache clear --registry` and `cache info` registry section.

---

## Conclusion

The problem statement and options analysis are solid foundations for decision-making. The main gaps are:
1. Security implications of stale fallback need explicit bounds
2. Cache size limit needs recipe-specific justification
3. Several assumptions should be made explicit

The recommended approach (JSON sidecars + stale-if-error + on-write eviction + structured errors) aligns well with existing codebase patterns and provides the best user experience during network issues.
