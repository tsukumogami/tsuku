package main

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// setInstalledInIndex updates the installed flag for recipe in the binary index.
// If the index is not built or cannot be opened, the error is logged at WARN
// level and the function returns without failing the caller.
func setInstalledInIndex(recipe string, installed bool) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		slog.Warn("binary index: failed to load config", "err", err)
		return
	}

	// If the cache directory doesn't exist the index was never built — nothing to do.
	if _, err := os.Stat(cfg.CacheDir); os.IsNotExist(err) {
		return
	}

	dbPath := filepath.Join(cfg.CacheDir, "binary-index.db")
	idx, err := index.Open(dbPath, cfg.RegistryDir)
	if err != nil {
		slog.Warn("binary index: failed to open", "err", err)
		return
	}
	defer func() { _ = idx.Close() }()

	if err := idx.SetInstalled(globalCtx, recipe, installed); err != nil {
		if !errors.Is(err, index.ErrIndexNotBuilt) {
			slog.Warn("binary index: failed to set installed flag", "recipe", recipe, "installed", installed, "err", err)
		}
	}
}
