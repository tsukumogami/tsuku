package main

import (
	"fmt"
	"runtime"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// installLibrary handles installation of library recipes
// Libraries are installed to $TSUKU_HOME/libs/{name}-{version}/ and track used_by
// Note: used_by tracking is handled by the caller after tool installation completes
func installLibrary(libName, reqVersion, parent string, mgr *install.Manager, telemetryClient *telemetry.Client) error {
	// Load recipe
	r, err := loader.Get(libName)
	if err != nil {
		return fmt.Errorf("library recipe not found: %w", err)
	}

	// Check if we can skip installation (reuse existing version)
	// For now, just check if any version is installed
	existingVersion := mgr.GetInstalledLibraryVersion(libName)
	if existingVersion != "" && reqVersion == "" {
		printInfof("Library %s@%s already installed, reusing\n", libName, existingVersion)
		return nil
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}
	if err != nil {
		return fmt.Errorf("failed to create executor for library: %w", err)
	}
	defer exec.Cleanup()

	// Set tools directory for finding other installed tools
	cfg, _ := config.DefaultConfig()
	exec.SetToolsDir(cfg.ToolsDir)
	exec.SetDownloadCacheDir(cfg.DownloadCacheDir)
	exec.SetKeyCacheDir(cfg.KeyCacheDir)

	// Create downloader and cache for plan generation
	// Downloader enables Decompose to download files (e.g., GHCR bottles with auth)
	// DownloadCache persists downloads for reuse during plan execution
	var downloadCache *actions.DownloadCache
	var downloader actions.Downloader
	if cfg.DownloadCacheDir != "" {
		downloadCache = actions.NewDownloadCache(cfg.DownloadCacheDir)
		predownloader := validate.NewPreDownloader()
		downloader = validate.NewPreDownloaderAdapter(predownloader)
	}

	// Generate plan for library installation
	plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		RecipeSource:  "registry",
		Downloader:    downloader,
		DownloadCache: downloadCache,
	})
	if err != nil {
		return fmt.Errorf("failed to generate library plan: %w", err)
	}

	// Execute the plan
	if err := exec.ExecutePlan(globalCtx, plan); err != nil {
		return fmt.Errorf("library installation failed: %w", err)
	}

	version := plan.Version
	if version == "" {
		version = "dev"
	}

	// Check if this specific version is already installed
	if mgr.IsLibraryInstalled(libName, version) {
		printInfof("Library %s@%s already installed\n", libName, version)
		return nil
	}

	// Install to libs directory
	// Note: used_by tracking is handled by the caller (installWithDependencies) after
	// tool installation completes, since we need the tool's version for proper tracking
	opts := install.LibraryInstallOptions{}

	if err := mgr.InstallLibrary(libName, version, exec.WorkDir(), opts); err != nil {
		return fmt.Errorf("failed to install library to permanent location: %w", err)
	}

	// Send telemetry event
	if telemetryClient != nil {
		event := telemetry.NewInstallEvent(libName, "", version, true) // Libraries are always dependencies
		telemetryClient.Send(event)
	}

	printInfof("Library %s@%s installed successfully\n", libName, version)
	return nil
}
