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
	InstallOutput   string  `json:"install_output,omitempty"` // included when install fails
	VerifyOutput    string  `json:"verify_output,omitempty"`  // included when verify fails
	DurationMs      int64   `json:"duration_ms"`
	Error           *string `json:"error"` // null on success, string on failure
}

// maxSandboxOutputBytes caps the container output embedded in a JSON result.
// A source build can emit megabytes, so some bound is needed; 16 KiB is a round
// number picked for log readability. For scale, the whole tail tsuku emits for a
// failed git build -- plan step, make invocation, and the error -- came to about
// 6.7 KiB.
const maxSandboxOutputBytes = 16384

// sandboxTruncationNotice marks output that had its head dropped, so a reader
// does not mistake a partial log for the whole build.
const sandboxTruncationNotice = "[earlier output truncated]"

// minMaskedSecretLen is the shortest --env value masked out of container
// output. Credentials are long; short values are things like DEBUG=1, and
// masking every "1" in a build log would destroy it.
const minMaskedSecretLen = 8

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
	out := buildSandboxJSONOutput(toolName, result, sandboxSecretValues(installEnv))
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
// of stdout. secrets are the values masked out of any container output the
// result carries; the caller resolves them from --env.
func buildSandboxJSONOutput(toolName string, result *sandbox.SandboxResult, secrets []string) sandboxJSONOutput {
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
		out.InstallOutput = sandboxFailureOutput(result, secrets)
		return out
	}

	// Handle failed sandbox test
	if !result.Passed {
		var errMsg string
		if result.ExitCode != 0 {
			errMsg = fmt.Sprintf("installation failed with exit code %d", result.ExitCode)
			// The exit code alone says nothing about which build step died,
			// so carry the container output that names the failing command.
			out.InstallOutput = sandboxFailureOutput(result, secrets)
		} else {
			errMsg = fmt.Sprintf("verification failed with exit code %d", result.VerifyExitCode)
			// The verify command runs in the same container with the same
			// forwarded env, so its output gets the same treatment.
			out.VerifyOutput = maskAndTail(result.VerifyOutput, secrets)
		}
		out.Error = &errMsg
		return out
	}

	// Passed: error is null
	out.Error = nil
	return out
}

// sandboxFailureOutput renders the container's combined output for inclusion in
// a JSON result. Without it a failing CI job reports only an exit code, and
// identifying the failing command means reproducing the build by hand.
//
// The tail is what matters: a build log's last lines name the command that
// died, so anything over the cap loses its head rather than its end.
//
// secrets holds values that must not reach the output. Masking them by exact
// value rather than by pattern is deliberate: validate.Sanitizer's regexes are
// tuned for error messages bound for an LLM repair API and mangle build logs,
// rewriting "foo.c:12:3:" to "foo.c[IP]3:" and "unexpected token: ')'" to
// "unexpected [REDACTED]". The only credentials tsuku puts in a sandbox are the
// ones the caller passed with --env, so their values are known exactly and
// match wherever they appear, including inside a URL.
func sandboxFailureOutput(result *sandbox.SandboxResult, secrets []string) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(result.Stdout); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(result.Stderr); s != "" {
		parts = append(parts, s)
	}
	return maskAndTail(strings.Join(parts, "\n"), secrets)
}

// maskAndTail removes secrets from container output and bounds what is left.
// Masking runs before truncation so a secret straddling the cut cannot leave a
// fragment behind.
func maskAndTail(output string, secrets []string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	for _, secret := range secrets {
		output = strings.ReplaceAll(output, secret, "[REDACTED]")
	}
	return tailLines(output, maxSandboxOutputBytes)
}

// sandboxSecretValues returns the --env values long enough to be credentials.
// CI forwards GITHUB_TOKEN into the sandbox this way and publishes the JSON
// result to a public job log, so those exact strings are masked out of any
// container output the result carries.
func sandboxSecretValues(entries []string) []string {
	var secrets []string
	for _, entry := range resolveEnvFlags(entries) {
		idx := strings.IndexByte(entry, '=')
		if idx < 0 {
			continue
		}
		if value := entry[idx+1:]; len(value) >= minMaskedSecretLen {
			secrets = append(secrets, value)
		}
	}
	return secrets
}

// tailLines returns at most maxBytes of trailing text, starting at a line
// boundary and prefixed with a notice when anything was dropped.
func tailLines(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-maxBytes:]
	// Byte-slicing can land mid-line and mid-rune. Advancing past the first
	// newline fixes both, but a log with no newline in its last maxBytes has
	// no boundary to advance to, so drop any partial rune left at the front.
	if idx := strings.IndexByte(tail, '\n'); idx >= 0 {
		tail = tail[idx+1:]
	} else {
		tail = strings.ToValidUTF8(tail, "")
	}
	return sandboxTruncationNotice + "\n" + tail
}

// emitSandboxHumanReadable prints the sandbox result in human-readable format.
// This is the original output path used when --json is not set.
//
// Unlike the JSON path, this prints container output verbatim: it goes to the
// operator's own terminal, who supplied any --env values in the first place,
// and truncating a local build log would only get in the way of debugging.
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
