# Security Review Phase 3: update-registry TTL bypass / stale recipe after fix

## Evaluation Questions

---

### 1. Does the research change your understanding of the problem?

**Yes, in one important respect -- the scope of the fix options narrows slightly.**

Phase 1 security assessment identified the TTL-only freshness check as the root cause
and flagged `CacheMetadata.ContentHash` as an unused field that could enable content-hash
comparison. The Phase 2 maintainer research confirms both points and adds useful precision:

- The manifest (`recipes.json`) already exists and is fetched on every `update-registry`
  run, but `ManifestRecipe` carries no per-recipe content hash or version. That means
  there is currently no cheap path to detect upstream recipe changes without a per-recipe
  network fetch.
- `fetchRemoteRecipe` issues plain GET requests with no conditional headers (no `ETag`,
  no `If-Modified-Since`). Even "checking" whether a recipe changed requires a full
  roundtrip today.
- The maintainer assessment introduces Option B (extend the manifest with per-recipe
  content hashes) as the most efficient fix. This is architecturally sound and aligns
  with the existing CI pipeline that regenerates `recipes.json`.

The security picture itself does not change: the "already fresh" path suppresses recipe
fixes for users within the TTL window, and the exposure window can extend to 7 days via
the stale fallback. What changes is that Option B (manifest-side hashes) is now
identified as a realistic and efficient path that does not require N per-recipe fetches
on every invocation. This is relevant for issue drafting -- the fix options section should
describe both Option A (force all fetches on explicit invocation) and Option B
(manifest-hash-gated refresh) without prescribing implementation.

---

### 2. Is this a duplicate of an existing issue?

**No.**

Phase 1 confirmed no open duplicate issues in the repository. The maintainer assessment
does not surface any duplicates either. The specific combination of (a) explicit
`update-registry` invocation, (b) TTL-based skip, and (c) no user-visible workaround hint
is a distinct, previously unfiled scenario.

---

### 3. Is the scope still appropriate for a single issue?

**Yes, and it remains well-bounded.**

All three Phase 2 perspectives (user, maintainer, security) converged on this being a
single defect: `update-registry` with no flags treats an explicit user intent to refresh
as equivalent to a background/implicit cache check. The fix is contained to either:

- `RefreshAll` (cached_registry.go) -- add content-hash comparison or remove TTL gate for
  explicit invocations, or
- the command layer (update_registry.go) -- route the no-flag path through
  `forceRegistryRefreshAll` rather than the TTL-gated `RefreshAll`.

Neither fix touches install logic, recipe parsing, version resolution, or any other
subsystem. A broader redesign (e.g., adding content hashes to the manifest, conditional
HTTP headers, or a server-side invalidation mechanism) may follow as a separate issue but
is not required to close this one.

The scope boundary is: **`update-registry` with no flags must not skip a recipe that may
have changed upstream.** Everything beyond that is a separate optimization.

---

### 4. Are we ready to draft?

**Yes.**

All three review perspectives are complete, the root cause is confirmed in code, the
fix options are clearly enumerated, and there are no blockers to drafting. A few points
to carry into the draft:

- State the expected behavior explicitly: after `tsuku update-registry`, a subsequent
  `tsuku install <tool>` should use the recipe content as it existed on the registry at
  the time of the `update-registry` run.
- Name the workaround (`tsuku update-registry --all` or `tsuku update-registry --recipe
  infisical`) so users can unblock themselves.
- Name both fix options without prescribing implementation -- this is a bug report, not a
  design document.
- Reproduce steps should note that the user must have cached the recipe within the last
  24 hours for this to trigger (which is the common case: the user just tried and failed
  to install the tool).

No concerns block drafting. The only mild ambiguity -- how recently the user cached the
recipe -- does not prevent filing or implementing the fix.

---

### 5. What context from research should be included in the issue?

**Include:**

1. **Root cause (one paragraph, no code dump).** `update-registry` skips network fetches
   for recipes whose local cache is younger than the 24-hour TTL. This is a time-only
   check (`isFresh` in `cached_registry.go`) with no comparison against upstream content.
   A recipe patched in the registry is invisible to users who cached it within the past
   24 hours.

2. **The "already fresh" message is misleading.** It implies the local copy matches the
   registry. It only means the copy is younger than the TTL. Users who see it
   reasonably stop investigating.

3. **The workaround exists but is invisible.** `tsuku update-registry --all` forces a
   full refresh regardless of TTL. `tsuku update-registry --recipe infisical` refreshes a
   single recipe unconditionally. Neither is mentioned in the default output when recipes
   are skipped.

4. **Fix options (named, not prescribed):**
   - Make `update-registry` (no flags) always fetch all cached recipes, treating explicit
     invocation as a force-refresh. This is the behavior users expect.
   - Compare per-recipe content hashes during `update-registry`: the manifest could carry
     a hash per recipe (generated by CI), allowing the client to detect upstream changes
     without fetching every recipe unconditionally.

5. **Security framing (kept brief and factual).** If the upstream registry fixes a
   recipe's download URL or checksum, the TTL cache silently withholds that fix for up to
   24 hours even when users explicitly run `update-registry`. The 7-day stale-fallback
   window extends that exposure further in degraded-network scenarios. This is the
   population most likely to need the fix.

**Omit from the issue (include in design if pursued):**
- Conditional HTTP headers (`ETag`, `If-Modified-Since`) -- implementation detail
- Server-side cache invalidation or push mechanisms -- out of scope for this bug
- Stale-fallback redesign -- separate concern
- `ContentHash` field internals -- implementation detail
- Specific line numbers -- they change and belong in PR reviews

---

## Security Severity Assessment (unchanged from Phase 1)

**MEDIUM-HIGH (supply-chain).**

Functional breakage (wrong `homebrew` action causing a segfault, as in the infisical
case) is a high-visibility symptom. But the same caching mechanism applies identically to
a fix that corrects a malicious download URL, a wrong checksum, or a compromised
distribution source. In those scenarios, a user who explicitly ran `update-registry` after
a security fix landed would still install the compromised binary. The "already fresh"
message provides false assurance. For a supply-chain-sensitive tool like a package
manager, that failure mode justifies MEDIUM-HIGH severity.

No change to this assessment is warranted by the Phase 2 findings.
