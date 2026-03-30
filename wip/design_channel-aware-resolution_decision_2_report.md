<!-- decision:start id="pin-boundary-enforcement" status="assumed" -->
### Decision: Pin Boundary Enforcement via Helper Function

**Context**

Tsuku's update and outdated commands don't respect version pins. When a user installs `node@17`, the Requested field ("17") is stored in state but never consulted during updates. The update command always resolves to the absolute latest version, and the outdated command hard-codes GitHub API calls, skipping non-GitHub tools entirely.

The ProviderFactory and VersionResolver interface already support what's needed. Every provider implements `ResolveVersion(ctx, version)` with fuzzy prefix matching -- calling `ResolveVersion(ctx, "17")` on the npm provider returns the latest 17.x release. The question is where to wire this logic.

**Assumptions**
- Named channel pins ("@lts") are out of scope. Numeric prefix pins ("17", "3.12", "1.29") are the target. Channel support can layer on top incrementally.
- All providers' `ResolveVersion()` implementations handle prefix matching correctly. Verified for npm, GitHub, and RubyGems; the pattern is consistent.

**Chosen: Helper Function (ResolveWithinBoundary)**

Add a `ResolveWithinBoundary(ctx context.Context, provider VersionResolver, requested string) (*VersionInfo, error)` function in the `version` package. When `requested` is empty, it calls `provider.ResolveLatest(ctx)`. When non-empty, it calls `provider.ResolveVersion(ctx, requested)`.

Both update.go and outdated.go read the `Requested` field from the tool's active VersionState and pass it to this function along with the provider obtained from `ProviderFactory.ProviderFromRecipe()`.

The outdated command also switches from hard-coded `res.ResolveGitHub()` calls to using ProviderFactory, which fixes the separate problem of non-GitHub tools being invisible to outdated checks.

**Rationale**

This approach requires zero changes to the VersionResolver interface and zero changes to any of the 15+ provider implementations. The existing `ResolveVersion()` method already does fuzzy prefix matching, which is exactly the pin boundary behavior needed. Centralizing the "pin or latest" branching in one function avoids duplicating it across commands and makes it easy to extend for channel-style pins later. The function is trivially testable with a mock provider.

**Alternatives Considered**
- **Inline call-site logic**: Putting if/else branching directly in update.go and outdated.go. Rejected because it duplicates logic across two commands (and any future commands that need pin awareness), making it harder to extend consistently.
- **New interface method (ResolveLatestWithinConstraint)**: Adding a method to VersionResolver that every provider must implement. Rejected because it forces changes to 15+ providers, all of which would have nearly identical implementations delegating to ResolveVersion(). The cost is disproportionate to the benefit.
- **Post-hoc filter on ListVersions**: Fetching all versions via VersionLister and filtering. Rejected because not all providers implement VersionLister, and it may fetch more data than needed when ResolveVersion() can short-circuit.

**Consequences**

The version package gains one new exported function (~10 lines). Update and outdated commands gain ~5 lines each to read the Requested field and use the helper. The outdated command undergoes a larger refactor to replace hard-coded GitHub resolution with ProviderFactory-based resolution, but that's independently needed regardless of this decision.

Future work: when "@lts" or named channel pins are added, the helper function is the single place to add dispatch logic for channel resolution.
<!-- decision:end -->
