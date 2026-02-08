package functional

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// aCleanTsukuEnvironment is a no-op because the Before hook already sets up
// the environment. This step exists so feature files read naturally.
func aCleanTsukuEnvironment(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

// iRun executes a command string, replacing "tsuku" with the test binary path.
func iRun(ctx context.Context, command string) (context.Context, error) {
	state := getState(ctx)
	if state == nil {
		return ctx, fmt.Errorf("no test state; is the Before hook running?")
	}

	// Replace "tsuku" at the start of the command with the test binary path
	args := strings.Fields(command)
	if len(args) > 0 && args[0] == "tsuku" {
		args[0] = state.binPath
	}

	cmd := exec.Command(args[0], args[1:]...)
	// Run from the same directory as the binary, where .tsuku-test lives
	cmd.Dir = filepath.Dir(state.binPath)

	// Determine registry URL: empty directory for @empty-registry, or repo root for local recipes
	registryURL := state.repoRoot // Uses recipes/ from the repo
	if state.emptyRegistry {
		// Create an empty directory to use as registry (no recipes will be found)
		emptyRegistry := filepath.Join(state.homeDir, "empty-registry")
		_ = os.MkdirAll(emptyRegistry, 0o755)
		registryURL = emptyRegistry
	}

	// Build environment: suppress telemetry, set home, set registry URL, optionally filter PATH
	env := append(os.Environ(),
		"TSUKU_HOME="+state.homeDir,
		"TSUKU_NO_TELEMETRY=1",
		"TSUKU_REGISTRY_URL="+registryURL,
	)
	if len(state.hiddenBinaries) > 0 {
		env = append(env, "PATH="+filteredPATH(state.hiddenBinaries))
	}
	cmd.Env = env

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	state.stdout = stdout.String()
	state.stderr = stderr.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			state.exitCode = exitErr.ExitCode()
		} else {
			return ctx, fmt.Errorf("command execution failed: %w", err)
		}
	} else {
		state.exitCode = 0
	}

	return ctx, nil
}

func theExitCodeIs(ctx context.Context, expected int) error {
	state := getState(ctx)
	if state.exitCode != expected {
		return fmt.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s",
			expected, state.exitCode, state.stdout, state.stderr)
	}
	return nil
}

func theExitCodeIsNot(ctx context.Context, notExpected int) error {
	state := getState(ctx)
	if state.exitCode == notExpected {
		return fmt.Errorf("expected exit code to not be %d\nstdout: %s\nstderr: %s",
			notExpected, state.stdout, state.stderr)
	}
	return nil
}

func theOutputContains(ctx context.Context, text string) error {
	state := getState(ctx)
	if !strings.Contains(state.stdout, text) {
		return fmt.Errorf("expected stdout to contain %q, got:\n%s", text, state.stdout)
	}
	return nil
}

func theOutputDoesNotContain(ctx context.Context, text string) error {
	state := getState(ctx)
	if strings.Contains(state.stdout, text) {
		return fmt.Errorf("expected stdout not to contain %q, got:\n%s", text, state.stdout)
	}
	return nil
}

func theErrorOutputContains(ctx context.Context, text string) error {
	state := getState(ctx)
	if !strings.Contains(state.stderr, text) {
		return fmt.Errorf("expected stderr to contain %q, got:\n%s", text, state.stderr)
	}
	return nil
}

func theErrorOutputDoesNotContain(ctx context.Context, text string) error {
	state := getState(ctx)
	if strings.Contains(state.stderr, text) {
		return fmt.Errorf("expected stderr not to contain %q, got:\n%s", text, state.stderr)
	}
	return nil
}

func theFileExists(ctx context.Context, path string) error {
	state := getState(ctx)
	fullPath := filepath.Join(state.homeDir, path)
	// Use Lstat to detect symlinks even if their target doesn't resolve
	if _, err := os.Lstat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("expected file %q to exist", fullPath)
	}
	return nil
}

func theFileDoesNotExist(ctx context.Context, path string) error {
	state := getState(ctx)
	fullPath := filepath.Join(state.homeDir, path)
	if _, err := os.Lstat(fullPath); err == nil {
		return fmt.Errorf("expected file %q not to exist", fullPath)
	}
	return nil
}

func iCanRun(ctx context.Context, command string) (context.Context, error) {
	state := getState(ctx)

	// Add bin/, tools/current/, and tool-specific bin dirs to PATH
	// The tools/current/ symlinks may use paths relative to the repo root
	// rather than relative to the symlink location, so we also add tool bin
	// directories directly as a workaround.
	binDir := filepath.Join(state.homeDir, "bin")
	currentDir := filepath.Join(state.homeDir, "tools", "current")

	// Find all tool bin directories for PATH
	toolBins := binDir + ":" + currentDir
	toolsDir := filepath.Join(state.homeDir, "tools")
	entries, _ := os.ReadDir(toolsDir)
	for _, e := range entries {
		if e.IsDir() && e.Name() != "current" {
			toolBin := filepath.Join(toolsDir, e.Name(), "bin")
			if _, err := os.Stat(toolBin); err == nil {
				toolBins += ":" + toolBin
			}
		}
	}

	cmd := exec.Command("bash", "-c", command)
	cmd.Env = append(os.Environ(),
		"PATH="+toolBins+":"+os.Getenv("PATH"),
		"TSUKU_NO_TELEMETRY=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return ctx, fmt.Errorf("command %q failed: %v\noutput: %s", command, err, string(out))
	}
	return ctx, nil
}
