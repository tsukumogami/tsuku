# Demand Validation: Auto-Update Capabilities for Tsuku

## Research Lead: lead-adversarial-demand

**Topic**: Auto-update capabilities for tsuku (both self-update and managed tool auto-update)

**Visibility**: Public

---

## Question 1: Is demand real?

**Confidence: Absent**

No external users have requested auto-update or self-update functionality. The GitHub issue tracker for tsukumogami/tsuku contains ~2,100+ issues, all authored by a single contributor (`dangazineu`). There are no issues titled or tagged with auto-update, self-update, or automatic updating. The one bot contributor (`tsuku-batch-generator[bot]`) files batch recipe PRs, not feature requests.

Issue #2103 ("fix(update): update does not switch active version when target is already installed") is the closest related issue. It's a bug in the existing manual `tsuku update` command, filed by the maintainer. It describes a symlink activation problem, not a request for automatic updating.

No community discussion, feature request, or thumbs-up signal was found for this topic.

## Question 2: What do people do today instead?

**Confidence: Medium (inferred from codebase, not user reports)**

The codebase provides two manual update mechanisms:

1. **`tsuku update <tool>`** (`cmd/tsuku/update.go`): Updates a single installed tool to its latest version by re-running the install flow with latest version resolution. Includes `--dry-run` flag.

2. **`tsuku outdated`** (`cmd/tsuku/outdated.go`): Checks all installed tools against their version providers and lists those with newer versions available. Supports `--json` output.

For CLI self-update, the only mechanism is the installer script referenced in deprecation warnings: `curl -fsSL https://get.tsuku.dev/now | bash`. There is no `tsuku self-update` command.

No workaround scripts, cron-based update patterns, or user-reported manual workflows were found in issues, docs, or code comments.

## Question 3: Who specifically asked?

**Confidence: Absent**

No one has explicitly requested this feature. The exploration scope document (`wip/explore_auto-update_scope.md`) frames this as a maintainer-initiated investigation. The scope document references "the user" wanting a "comprehensive auto-update system inspired by Claude Code's channel model," but this appears to refer to the maintainer's own design intent, not an external request.

There are zero GitHub issues requesting auto-update, self-update, version channel pinning, or automatic rollback from any contributor.

## Question 4: What behavior change counts as success?

**Confidence: Low (derived from scope doc, not acceptance criteria in issues)**

The scope document (`wip/explore_auto-update_scope.md`) lists desired behaviors:

- Version channel pinning (major/minor/patch granularity)
- Time-cached update checks with configurable intervals
- Force-check on command
- Automatic rollback on update failure
- Deferred failure reporting (report on next tool execution)
- Configurable update notifications

However, these are exploration targets in a working document, not acceptance criteria in a filed issue. No measurable goals or success metrics exist in the issue tracker or design documents.

## Question 5: Is it already built?

**Confidence: High (confirmed absent)**

Auto-update is not built. The codebase has:

- **Manual update**: `tsuku update <tool>` resolves the latest version and reinstalls. No caching, no background checks, no automatic triggering.
- **Manual outdated check**: `tsuku outdated` queries version providers on demand with no caching layer.
- **No self-update command**: The CLI has no `self-update` subcommand. The only self-update path is re-running the installer script.
- **No update check cache**: No time-based staleness checking, no stored last-check timestamps.
- **No rollback mechanism**: Updates overwrite in place with no preserved previous version for rollback.
- **No version pinning/channels**: Tools are installed at a specific version or "latest" with no channel concept.

The `helpers.go` deprecation warning system is the closest thing to proactive version awareness -- it warns when the CLI needs upgrading based on registry metadata -- but it's passive (triggered only when accessing a registry that declares a deprecation).

## Question 6: Is it already planned?

**Confidence: Low**

The scope document (`wip/explore_auto-update_scope.md`) exists in the `wip/` directory, indicating active exploration. However:

- No GitHub issues exist for auto-update design or implementation
- No design document (`DESIGN-auto-update.md` or similar) exists in `docs/designs/`
- No roadmap file (`ROADMAP-*.md`) exists in the repository
- No milestone references auto-update
- The scope document is a working exploration artifact, not a committed plan

The topic is at the "should we investigate?" stage, not the "planned for implementation" stage.

---

## Calibration

**Demand not validated.**

The majority of questions returned Absent or Low confidence. Specifically:

| Question | Confidence |
|----------|------------|
| 1. Is demand real? | Absent |
| 2. Current workarounds? | Medium (inferred) |
| 3. Who asked? | Absent |
| 4. Success criteria? | Low |
| 5. Already built? | High (confirmed absent) |
| 6. Already planned? | Low |

This is a **single-maintainer project with no external contributors or feature requests**. The absence of external demand signal is expected for a project at this stage -- it does not mean the feature lacks merit. The maintainer may have product intuition that hasn't been externalized into issues yet.

Key distinction: this is **demand not validated**, not **demand validated as absent**. There is no evidence that auto-update was evaluated and rejected. The feature simply hasn't been requested by anyone other than the maintainer exploring the idea. The exploration scope document suggests the maintainer sees this as a natural evolution of the manual update flow.

The existing manual `update` and `outdated` commands provide a foundation. Bug #2103 (update not switching active version) suggests the manual update path still has rough edges that would need fixing before layering automation on top.
