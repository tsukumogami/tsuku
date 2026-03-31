package updates

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/version"
)

// RecipeLoader loads recipes for tools. This interface allows testing without
// depending on the global loader in cmd/tsuku.
type RecipeLoader interface {
	LoadRecipe(ctx context.Context, toolName string, state *install.State, cfg *config.Config) (*recipe.Recipe, error)
}

// RunUpdateCheck performs a background update check for all installed tools.
// It acquires an advisory flock to prevent concurrent checks, re-checks sentinel
// freshness after lock acquisition (double-check pattern), iterates tools, and
// writes per-tool cache files.
func RunUpdateCheck(ctx context.Context, cfg *config.Config, userCfg *userconfig.Config, loader RecipeLoader) error {
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
	defer lock.Unlock()

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

	for _, tool := range tools {
		// Check context deadline
		if ctx.Err() != nil {
			break
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
		// Write result (best effort, matching version cache pattern)
		_ = WriteEntry(cacheDir, entry)
	}

	// Touch sentinel after all tools processed
	_ = TouchSentinel(cacheDir)
	return nil
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

	// Resolve latest overall
	overall, err := provider.ResolveLatest(ctx)
	if err != nil {
		// Non-fatal: we have within-pin, just skip overall
		entry.LatestOverall = ""
		return entry
	}
	entry.LatestOverall = overall.Version

	return entry
}
