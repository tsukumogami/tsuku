package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/distributed"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// distributedInstallArgs holds the parsed components of a distributed install request.
type distributedInstallArgs struct {
	// Source is the "owner/repo" identifier.
	Source string
	// RecipeName is the recipe to install (defaults to repo name if not specified).
	RecipeName string
	// Version is the version constraint (empty means latest).
	Version string
}

// parseDistributedName parses a tool name that contains "/" into its distributed
// components. It handles these formats:
//   - owner/repo                  -> source=owner/repo, recipe=repo, version=""
//   - owner/repo:recipe           -> source=owner/repo, recipe=recipe, version=""
//   - owner/repo@version          -> source=owner/repo, recipe=repo, version=version
//   - owner/repo:recipe@version   -> source=owner/repo, recipe=recipe, version=version
//
// Returns nil if the name doesn't contain "/" (not a distributed name).
func parseDistributedName(name string) *distributedInstallArgs {
	if !strings.Contains(name, "/") {
		return nil
	}

	// Split version from the end first (@ can appear in version part)
	version := ""
	nameWithoutVersion := name
	if atIdx := strings.LastIndex(name, "@"); atIdx > 0 {
		nameWithoutVersion = name[:atIdx]
		version = name[atIdx+1:]
	}

	// Split source from recipe name (: separator)
	source := nameWithoutVersion
	recipeName := ""
	if colonIdx := strings.Index(nameWithoutVersion, ":"); colonIdx > 0 && colonIdx < len(nameWithoutVersion)-1 {
		source = nameWithoutVersion[:colonIdx]
		recipeName = nameWithoutVersion[colonIdx+1:]
	}

	// Default recipe name to repo name
	if recipeName == "" {
		parts := strings.SplitN(source, "/", 2)
		if len(parts) == 2 {
			recipeName = parts[1]
		}
	}

	return &distributedInstallArgs{
		Source:     source,
		RecipeName: recipeName,
		Version:    version,
	}
}

// isDistributedName returns true if the tool name looks like a distributed
// source reference (contains "/").
func isDistributedName(name string) bool {
	return strings.Contains(name, "/")
}

// ensureDistributedSource validates the source, checks registration status,
// and auto-registers if needed. Returns an error if the source is invalid
// or if strict_registries blocks the install.
//
// The autoApprove parameter skips the interactive confirmation prompt (--yes flag).
func ensureDistributedSource(source string, autoApprove bool) error {
	// Validate the source format
	if err := validateRegistrySource(source); err != nil {
		return err
	}

	// Check if we already have a provider for this source
	if hasDistributedProvider(source) {
		return nil
	}

	// Load user config to check registration
	userCfg, err := userconfig.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if already registered (provider may not exist if config was
	// added after CLI started -- create it dynamically)
	if _, registered := userCfg.Registries[source]; registered {
		return addDistributedProvider(source)
	}

	// Not registered -- check strict mode
	if userCfg.StrictRegistries {
		return fmt.Errorf(
			"source %q is not registered and strict_registries is enabled\n\n"+
				"To allow this source, run:\n  tsuku registry add %s",
			source, source,
		)
	}

	// Prompt for confirmation (unless --yes)
	if !autoApprove {
		prompt := fmt.Sprintf("Install from unregistered source %q?", source)
		if !confirmWithUser(prompt) {
			return fmt.Errorf("installation cancelled: source %q not approved", source)
		}
	}

	// Auto-register the source
	if err := autoRegisterSource(userCfg, source); err != nil {
		return fmt.Errorf("failed to auto-register source %q: %w", source, err)
	}

	fmt.Fprintf(os.Stderr, "Auto-registered source %q\n", source)

	// Dynamically add a provider to the loader so the recipe can be fetched
	// in the same install session without requiring a restart
	return addDistributedProvider(source)
}

// autoRegisterSource adds a distributed source to the user config with
// AutoRegistered=true.
func autoRegisterSource(userCfg *userconfig.Config, source string) error {
	if userCfg.Registries == nil {
		userCfg.Registries = make(map[string]userconfig.RegistryEntry)
	}
	userCfg.Registries[source] = userconfig.RegistryEntry{
		URL:            fmt.Sprintf("https://github.com/%s", source),
		AutoRegistered: true,
	}
	return userCfg.Save()
}

// addDistributedProvider creates a new DistributedProvider for the source
// and adds it to the global loader. Skips if a provider already exists.
func addDistributedProvider(source string) error {
	if hasDistributedProvider(source) {
		return nil
	}

	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid source format: %s", source)
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		return err
	}

	cacheDir := filepath.Join(cfg.CacheDir, "distributed")
	cache := distributed.NewCacheManager(cacheDir, distributed.DefaultCacheTTL)
	ghClient := distributed.NewGitHubClient(cache)
	provider := distributed.NewDistributedProvider(parts[0], parts[1], ghClient)

	loader.AddProvider(provider)
	return nil
}

// checkSourceCollision checks whether a tool is already installed from a
// different source. Returns an error if the user declines the replacement.
//
// Same-source reinstalls don't trigger a collision check.
// The force parameter skips the interactive collision prompt (--force flag).
func checkSourceCollision(toolName, newSource string, force bool) error {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return nil // Best-effort; don't fail install on config errors
	}

	mgr := install.New(cfg)
	toolState, err := mgr.GetState().GetToolState(toolName)
	if err != nil || toolState == nil {
		return nil // Not installed, no collision
	}

	existingSource := toolState.Source
	if existingSource == "" {
		existingSource = "central"
	}

	// Same source -- no collision
	if existingSource == newSource {
		return nil
	}

	if force {
		return nil
	}

	prompt := fmt.Sprintf(
		"Tool %q is already installed from %q. Replace with version from %q?",
		toolName, existingSource, newSource,
	)
	if !confirmWithUser(prompt) {
		return fmt.Errorf("installation cancelled: would replace %q from %q with %q", toolName, existingSource, newSource)
	}

	return nil
}

// recordDistributedSource updates the ToolState to record the distributed
// source and recipe hash after a successful install.
func recordDistributedSource(toolName, source, recipeHash string) error {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return err
	}

	mgr := install.New(cfg)
	return mgr.GetState().UpdateTool(toolName, func(ts *install.ToolState) {
		ts.Source = source
		ts.RecipeHash = recipeHash
	})
}

// computeRecipeHash computes the SHA256 hash of recipe TOML bytes.
// Returns the hex-encoded hash string.
func computeRecipeHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}

// fetchRecipeBytes fetches raw recipe bytes from a distributed source.
// This is used to compute the recipe hash for the audit trail.
func fetchRecipeBytes(source, recipeName string) ([]byte, error) {
	return loader.GetFromSource(globalCtx, recipeName, source)
}

// distributedTelemetryTag returns the opaque telemetry tag for distributed
// installs. The actual owner/repo is never sent to telemetry.
func distributedTelemetryTag() string {
	return "distributed"
}

// hasDistributedProvider checks if the loader already has a provider for the
// given source.
func hasDistributedProvider(source string) bool {
	return loader.ProviderBySource(recipe.RecipeSource(source)) != nil
}
