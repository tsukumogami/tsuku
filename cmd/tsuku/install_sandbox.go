package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/sandbox"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// sandboxJSONOutput is the structured JSON object emitted by
// tsuku install --sandbox --json. CI workflows parse this with jq.
type sandboxJSONOutput struct {
	Tool            string  `json:"tool"`
	Passed          bool    `json:"passed"`
	Verified        bool    `json:"verified"`
	InstallExitCode int     `json:"install_exit_code"`
	VerifyExitCode  int     `json:"verify_exit_code"`
	DurationMs      int64   `json:"duration_ms"`
	Error           *string `json:"error"` // null on success, string on failure
}

// runSandboxInstall runs the installation in an isolated sandbox container.
// It supports three modes:
//  1. Tool from registry: tsuku install <tool> --sandbox
//  2. Local recipe file: tsuku install --recipe <path> --sandbox
//  3. External plan: tsuku install --plan <path> --sandbox
//
// When installJSON is set, stdout contains exactly one JSON object and
// human-readable output is suppressed. Errors during plan generation
// are returned to the caller so handleInstallError can emit the normal
// --json error format (preserving missing_recipes for batch workflows).
func runSandboxInstall(toolName, planPath, recipePath, targetFamily string) error {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var plan *executor.InstallationPlan

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
		// Generate plan from local recipe file
		plan, err = generateInstallPlan(globalCtx, "", "", recipePath, cfg, targetFamily)
		if err != nil {
			return fmt.Errorf("failed to generate plan: %w", err)
		}
		if toolName == "" {
			toolName = plan.Tool
		}

	default:
		// Generate plan from recipe in registry
		plan, err = generateInstallPlan(globalCtx, toolName, "", "", cfg, targetFamily)
		if err != nil {
			return fmt.Errorf("failed to generate plan: %w", err)
		}
	}

	// Compute sandbox requirements from plan
	reqs := sandbox.ComputeSandboxRequirements(plan, targetFamily)

	// Resolve --env flags: KEY=VALUE is passed as-is, KEY-only reads from host
	if len(installEnv) > 0 {
		reqs.ExtraEnv = resolveEnvFlags(installEnv)
	}

	// For local recipe files, show confirmation prompt (unless --force or --yes)
	// Skip the prompt in --json mode since it's non-interactive by design
	if recipePath != "" && !installForce && !installJSON {
		if !confirmSandboxExecution(recipePath, reqs) {
			return fmt.Errorf("sandbox testing canceled")
		}
	}

	if !installJSON {
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
	}

	// Ensure cache directories exist (needed for mounting into container)
	if err := cfg.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create sandbox executor with download cache directory
	// This allows the sandbox to use pre-downloaded files from plan generation
	detector := validate.NewRuntimeDetector()
	sandboxExec := sandbox.NewExecutor(detector,
		sandbox.WithDownloadCacheDir(cfg.DownloadCacheDir),
		sandbox.WithCargoRegistryCacheDir(cfg.CargoRegistryCacheDir),
	)

	// Detect target platform, honoring --target-family override
	target, err := resolveTarget(targetFamily)
	if err != nil {
		return fmt.Errorf("failed to detect target platform: %w", err)
	}

	// Run sandbox test
	result, err := sandboxExec.Sandbox(globalCtx, plan, target, reqs)
	if err != nil {
		return fmt.Errorf("sandbox execution failed: %w", err)
	}

	// JSON output mode: emit a single JSON object and return
	if installJSON {
		return emitSandboxJSON(toolName, result)
	}

	// Human-readable output mode (unchanged from before --json was added)
	return emitSandboxHumanReadable(result)
}

// emitSandboxJSON writes a single JSON object to stdout for the sandbox result.
// It handles all result states: passed, failed, skipped, and error.
// The JSON output is the terminal action -- this function never returns an error
// to the caller, which prevents handleInstallError from emitting a second JSON
// object. For non-passing states, it calls exitWithCode directly to set the
// appropriate process exit code.
func emitSandboxJSON(toolName string, result *sandbox.SandboxResult) error {
	out := buildSandboxJSONOutput(toolName, result)
	printJSON(out)

	// For failed or errored states, exit directly so the caller doesn't
	// try to emit a second JSON error via handleInstallError.
	if result.Error != nil || (!result.Passed && !result.Skipped) {
		exitWithCode(ExitInstallFailed)
	}
	return nil
}

// buildSandboxJSONOutput constructs the JSON output struct from a sandbox result.
// This is a pure function with no side effects, making it testable independently
// of stdout.
func buildSandboxJSONOutput(toolName string, result *sandbox.SandboxResult) sandboxJSONOutput {
	out := sandboxJSONOutput{
		Tool:            toolName,
		Passed:          result.Passed,
		Verified:        result.Verified,
		InstallExitCode: result.ExitCode,
		VerifyExitCode:  result.VerifyExitCode,
		DurationMs:      result.DurationMs,
	}

	// Handle skipped sandbox (no container runtime)
	if result.Skipped {
		out.Passed = false
		out.Verified = false
		out.InstallExitCode = -1
		out.VerifyExitCode = -1
		errMsg := "no container runtime available (install Podman or Docker)"
		out.Error = &errMsg
		return out
	}

	// Handle sandbox execution error (container failed to run)
	if result.Error != nil {
		errMsg := result.Error.Error()
		out.Error = &errMsg
		return out
	}

	// Handle failed sandbox test
	if !result.Passed {
		var errMsg string
		if result.ExitCode != 0 {
			errMsg = fmt.Sprintf("installation failed with exit code %d", result.ExitCode)
		} else {
			errMsg = fmt.Sprintf("verification failed with exit code %d", result.VerifyExitCode)
		}
		out.Error = &errMsg
		return out
	}

	// Passed: error is null
	out.Error = nil
	return out
}

// emitSandboxHumanReadable prints the sandbox result in human-readable format.
// This is the original output path used when --json is not set.
func emitSandboxHumanReadable(result *sandbox.SandboxResult) error {
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
		if result.ExitCode != 0 {
			printInfof("Exit code: %d\n", result.ExitCode)
		} else {
			printInfof("Verification failed with exit code: %d\n", result.VerifyExitCode)
		}
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
		if result.ExitCode != 0 {
			return fmt.Errorf("sandbox test failed with exit code %d", result.ExitCode)
		}
		return fmt.Errorf("sandbox verification failed with exit code %d", result.VerifyExitCode)
	}

	return nil
}

// resolveEnvFlags processes --env flag values. Each entry is either KEY=VALUE
// (passed through as-is) or KEY (value read from the host environment, matching
// docker's --env behavior). If a KEY-only entry has no corresponding host
// variable, it passes KEY= (empty value).
func resolveEnvFlags(entries []string) []string {
	resolved := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.Contains(entry, "=") {
			resolved = append(resolved, entry)
		} else {
			// KEY-only: read value from host environment
			resolved = append(resolved, entry+"="+os.Getenv(entry))
		}
	}
	return resolved
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
