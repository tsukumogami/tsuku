package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/sandbox"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// runSandboxInstall runs the installation in an isolated sandbox container.
// It supports three modes:
//  1. Tool from registry: tsuku install <tool> --sandbox
//  2. Local recipe file: tsuku install --recipe <path> --sandbox
//  3. External plan: tsuku install --plan <path> --sandbox
func runSandboxInstall(toolName, planPath, recipePath string) error {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var plan *executor.InstallationPlan
	var r *recipe.Recipe

	switch {
	case planPath != "":
		// Load plan from file or stdin
		plan, err = loadPlanFromSource(planPath)
		if err != nil {
			return err
		}
		// Validate plan
		if err := validateExternalPlan(plan, toolName); err != nil {
			return err
		}
		if toolName == "" {
			toolName = plan.Tool
		}

	case recipePath != "":
		// Load recipe from local file
		r, err = loadLocalRecipe(recipePath)
		if err != nil {
			return fmt.Errorf("failed to load recipe: %w", err)
		}
		if toolName == "" {
			toolName = r.Metadata.Name
		}
		// Generate plan from recipe
		plan, err = generatePlanFromRecipe(r, toolName, cfg)
		if err != nil {
			return fmt.Errorf("failed to generate plan: %w", err)
		}

	default:
		// Load recipe from registry
		r, err = loader.Get(toolName)
		if err != nil {
			return fmt.Errorf("recipe not found: %w", err)
		}
		// Generate plan from recipe
		plan, err = generatePlanFromRecipe(r, toolName, cfg)
		if err != nil {
			return fmt.Errorf("failed to generate plan: %w", err)
		}
	}

	// Compute sandbox requirements from plan
	reqs := sandbox.ComputeSandboxRequirements(plan)

	// For local recipe files, show confirmation prompt (unless --force or --yes)
	if recipePath != "" && !installForce {
		if !confirmSandboxExecution(recipePath, reqs) {
			return fmt.Errorf("sandbox testing canceled")
		}
	}

	printInfof("Running sandbox test for %s...\n", toolName)
	printInfof("  Container image: %s\n", reqs.Image)
	if reqs.RequiresNetwork {
		printInfo("  Network access: enabled (ecosystem build)")
	} else {
		printInfo("  Network access: disabled (binary installation)")
	}
	printInfof("  Resource limits: %s memory, %s CPUs, %v timeout\n",
		reqs.Resources.Memory, reqs.Resources.CPUs, reqs.Resources.Timeout)
	printInfo()

	// Create sandbox executor with download cache directory
	// This allows the sandbox to use pre-downloaded files from plan generation
	detector := validate.NewRuntimeDetector()
	sandboxExec := sandbox.NewExecutor(detector, sandbox.WithDownloadCacheDir(cfg.DownloadCacheDir))

	// Run sandbox test
	result, err := sandboxExec.Sandbox(globalCtx, plan, reqs)
	if err != nil {
		return fmt.Errorf("sandbox execution failed: %w", err)
	}

	// Handle skipped sandbox (no container runtime)
	if result.Skipped {
		printInfo("Sandbox testing skipped (no container runtime available)")
		printInfo("Install Podman or Docker to enable sandbox testing.")
		return nil
	}

	// Report results
	if result.Passed {
		printInfo("Sandbox test PASSED")
		if result.Stdout != "" {
			printInfo()
			printInfo("Container output:")
			printInfo(result.Stdout)
		}
	} else {
		printInfo("Sandbox test FAILED")
		printInfof("Exit code: %d\n", result.ExitCode)
		if result.Stderr != "" {
			printInfo()
			printInfo("Error output:")
			fmt.Fprintln(os.Stderr, result.Stderr)
		}
		if result.Stdout != "" {
			printInfo()
			printInfo("Container output:")
			printInfo(result.Stdout)
		}
		return fmt.Errorf("sandbox test failed with exit code %d", result.ExitCode)
	}

	return nil
}

// loadLocalRecipe loads a recipe from a local file path.
// This is a thin wrapper around recipe.ParseFile for use in CLI commands.
func loadLocalRecipe(path string) (*recipe.Recipe, error) {
	return recipe.ParseFile(path)
}

// generatePlanFromRecipe generates an installation plan from a recipe.
// It pre-downloads files and caches them for offline sandbox execution.
func generatePlanFromRecipe(r *recipe.Recipe, toolName string, cfg *config.Config) (*executor.InstallationPlan, error) {
	exec, err := executor.New(r)
	if err != nil {
		return nil, err
	}
	defer exec.Cleanup()

	exec.SetToolsDir(cfg.ToolsDir)
	exec.SetDownloadCacheDir(cfg.DownloadCacheDir)
	exec.SetKeyCacheDir(cfg.KeyCacheDir)

	// Create downloader for checksum computation and pre-downloading
	predownloader := validate.NewPreDownloader()
	downloader := validate.NewPreDownloaderAdapter(predownloader)

	// Create download cache for persisting downloads
	// Downloaded files are cached so they can be mounted into the sandbox container
	downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)

	plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		RecipeSource:  "local",
		Downloader:    downloader,
		DownloadCache: downloadCache,
	})
	if err != nil {
		return nil, err
	}

	return plan, nil
}

// confirmSandboxExecution prompts the user to confirm sandbox execution for untrusted recipes.
func confirmSandboxExecution(recipePath string, reqs *sandbox.SandboxRequirements) bool {
	if !isInteractive() {
		fmt.Fprintln(os.Stderr, "Error: sandbox testing of local recipe requires interactive mode")
		fmt.Fprintln(os.Stderr, "Use --force to bypass this check in scripts")
		return false
	}

	fmt.Fprintf(os.Stderr, "Sandbox testing recipe: %s\n", recipePath)
	if reqs.RequiresNetwork {
		fmt.Fprintln(os.Stderr, "  Network access: required (ecosystem build)")
	} else {
		fmt.Fprintln(os.Stderr, "  Network access: none (binary installation)")
	}
	fmt.Fprintf(os.Stderr, "  Container image: %s\n", reqs.Image)
	fmt.Fprintf(os.Stderr, "  Resource limits: %s memory, %s CPUs, %v timeout\n",
		reqs.Resources.Memory, reqs.Resources.CPUs, reqs.Resources.Timeout)
	fmt.Fprintln(os.Stderr)
	fmt.Fprint(os.Stderr, "Proceed with sandbox testing? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
