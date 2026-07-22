package updates

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/version"
)

// checkFailureNoticeThreshold is how many consecutive failed checks a tool
// must accumulate before a KindCheckFailure notice surfaces to the user.
// A single failed check is often a transient blip (a rate limit, a flaky
// network); only a run of failures indicates the tool has silently stopped
// being considered for auto-update.
const checkFailureNoticeThreshold = 3

// RecipeLoader loads recipes for tools. This interface allows testing without
// depending on the global loader in cmd/tsuku.
type RecipeLoader interface {
	LoadRecipe(ctx context.Context, toolName string, state *install.State, cfg *config.Config) (*recipe.Recipe, error)
}

// RunUpdateCheck performs a background update check for all installed tools.
// It acquires an advisory flock to prevent concurrent checks, re-checks sentinel
// freshness after lock acquisition (double-check pattern), iterates tools, and
// writes per-tool cache files.
//
// bus may be nil; when non-nil, the self-update path publishes
// Updated / UpdateFailed events with Tool == SelfToolName so the
// notices and telemetry subscribers can react. Pass SourceAuto since
// RunUpdateCheck is always the background trigger path.
func RunUpdateCheck(ctx context.Context, cfg *config.Config, userCfg *userconfig.Config, loader RecipeLoader, bus *installevents.Bus) error {
	cacheDir := CacheDir(cfg.HomeDir)
	interval := userCfg.UpdatesCheckInterval()

	// Ensure cache directory exists before locking
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create update cache directory: %w", err)
	}

	// Acquire exclusive lock
	lockPath := cacheDir + "/" + LockFile
	lock := install.NewFileLock(lockPath)
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("acquire update check lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Double-check: re-verify staleness after lock acquisition.
	// Another process may have completed a check while we waited for the lock.
	if !IsCheckStale(cacheDir, interval) {
		return nil
	}

	mgr := install.New(cfg)
	tools, err := mgr.List()
	if err != nil {
		return fmt.Errorf("list installed tools: %w", err)
	}

	state, err := mgr.GetState().Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	res := version.New()
	factory := version.NewProviderFactory()
	noticesDir := notices.NoticesDir(cfg.HomeDir)

	for _, tool := range tools {
		// Check context deadline
		if ctx.Err() != nil {
			break
		}

		// mgr.List() returns one row per retained version directory, not one
		// per tool: a tool with several old versions still on disk (pending
		// garbage collection) would otherwise be checked -- and would burn a
		// version-resolution network call -- once per stale directory. Only
		// the active version's result can ever become an auto-apply
		// candidate, so skip the rest.
		if !tool.IsActive {
			continue
		}

		var requested string
		if ts, ok := state.Installed[tool.Name]; ok {
			if vs, vok := ts.Versions[ts.ActiveVersion]; vok {
				requested = vs.Requested
			}
		}

		// Skip exact-pinned tools
		if install.PinLevelFromRequested(requested) == install.PinExact {
			continue
		}

		entry := checkTool(ctx, tool, requested, state, cfg, loader, res, factory)
		recordConsecutiveCheckFailures(cacheDir, noticesDir, entry)

		// Write result (best effort, matching version cache pattern)
		_ = WriteEntry(cacheDir, entry)
	}

	// Check for tsuku self-update (separate from managed tools)
	if err := CheckAndApplySelf(ctx, cfg, userCfg, cacheDir, res, bus, installevents.SourceAuto); err != nil {
		log.Default().Debug("self-update check", "error", err)
	}

	// Touch sentinel after all tools processed
	_ = TouchSentinel(cacheDir)
	return nil
}

// recordConsecutiveCheckFailures updates entry.ConsecutiveCheckFailures from
// the prior cached entry for the same tool and, once a run of failures
// crosses checkFailureNoticeThreshold, writes a KindCheckFailure notice so
// the failure is no longer invisible: a checkTool error alone never reaches
// notices (IsPendingEntry excludes it from auto-apply, and nothing else
// looks at the check-cache directory), so without this a tool can silently
// stop being considered for auto-update indefinitely. A check that recovers
// clears any standing KindCheckFailure notice -- but only one of our own;
// an unrelated pending notice (e.g. an apply failure awaiting review) is
// left alone.
func recordConsecutiveCheckFailures(cacheDir, noticesDir string, entry *UpdateCheckEntry) {
	if prior, _ := ReadEntry(cacheDir, entry.Tool); prior != nil {
		entry.ConsecutiveCheckFailures = prior.ConsecutiveCheckFailures
	}

	if entry.Error == "" {
		entry.ConsecutiveCheckFailures = 0
		if n, _ := notices.ReadNotice(noticesDir, entry.Tool); n != nil && n.Kind == notices.KindCheckFailure {
			_ = notices.RemoveNotice(noticesDir, entry.Tool)
		}
		return
	}

	entry.ConsecutiveCheckFailures++
	if entry.ConsecutiveCheckFailures >= checkFailureNoticeThreshold {
		_ = notices.WriteNotice(noticesDir, &notices.Notice{
			Tool:                entry.Tool,
			Error:               entry.Error,
			Kind:                notices.KindCheckFailure,
			ConsecutiveFailures: entry.ConsecutiveCheckFailures,
			Timestamp:           entry.CheckedAt,
			Shown:               false,
		})
	}
}

// checkTool checks a single tool and returns an UpdateCheckEntry.
// Errors are captured in the entry's Error field rather than returned.
func checkTool(
	ctx context.Context,
	tool install.InstalledTool,
	requested string,
	state *install.State,
	cfg *config.Config,
	loader RecipeLoader,
	res *version.Resolver,
	factory *version.ProviderFactory,
) *UpdateCheckEntry {
	now := time.Now()
	entry := &UpdateCheckEntry{
		Tool:          tool.Name,
		ActiveVersion: tool.Version,
		Requested:     requested,
		CheckedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}

	// Load recipe
	r, err := loader.LoadRecipe(ctx, tool.Name, state, cfg)
	if err != nil {
		entry.Error = fmt.Sprintf("load recipe: %v", err)
		return entry
	}

	// Create provider
	provider, err := factory.ProviderFromRecipe(res, r)
	if err != nil {
		entry.Error = fmt.Sprintf("create provider: %v", err)
		return entry
	}

	entry.Source = provider.SourceDescription()

	// Resolve latest within pin boundary
	withinPin, err := version.ResolveWithinBoundary(ctx, provider, requested)
	if err != nil {
		entry.Error = fmt.Sprintf("resolve within pin: %v", err)
		return entry
	}
	if withinPin.Version != tool.Version {
		entry.LatestWithinPin = withinPin.Version
	}

	// Resolve latest overall. When there is no pin (or the pin is "latest"),
	// ResolveWithinBoundary above already called provider.ResolveLatest and
	// withinPin *is* the overall latest -- reuse it instead of making an
	// identical second network request for the same answer.
	if requested == "" || requested == "latest" {
		entry.LatestOverall = withinPin.Version
		return entry
	}

	overall, err := provider.ResolveLatest(ctx)
	if err != nil {
		// Non-fatal: we have within-pin, just skip overall
		entry.LatestOverall = ""
		return entry
	}
	entry.LatestOverall = overall.Version

	return entry
}
