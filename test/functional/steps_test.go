package functional

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
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
	for k, v := range state.envOverrides {
		env = append(env, k+"="+v)
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

// theFileEventuallyDoesNotExist polls for a file's absence with a
// bounded deadline. Used for assertions against state mutated by a
// detached background process — e.g., auto-apply's `apply-updates`
// subprocess, which consumes update cache entries asynchronously
// after the foreground command (`tsuku list`, etc.) returns.
func theFileEventuallyDoesNotExist(ctx context.Context, path string, timeoutSeconds int) error {
	state := getState(ctx)
	fullPath := filepath.Join(state.homeDir, path)
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(fullPath); os.IsNotExist(err) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Lstat(fullPath); os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("expected file %q not to exist within %ds", fullPath, timeoutSeconds)
}

// iSetEnv sets an environment variable override for subsequent commands in this scenario.
func iSetEnv(ctx context.Context, key, value string) (context.Context, error) {
	state := getState(ctx)
	if state == nil {
		return ctx, fmt.Errorf("no test state")
	}
	if state.envOverrides == nil {
		state.envOverrides = make(map[string]string)
	}
	state.envOverrides[key] = value
	return ctx, nil
}

// iCreateHomeFile writes a file at a path relative to $TSUKU_HOME.
func iCreateHomeFile(ctx context.Context, path string, content *godog.DocString) (context.Context, error) {
	state := getState(ctx)
	if state == nil {
		return ctx, fmt.Errorf("no test state")
	}
	fullPath := filepath.Join(state.homeDir, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return ctx, fmt.Errorf("creating parent dirs for %s: %w", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content.Content), 0o644); err != nil {
		return ctx, fmt.Errorf("writing %s: %w", path, err)
	}
	return ctx, nil
}

// iRunFromDir executes a command from a specific directory relative to $TSUKU_HOME.
// This lets tests simulate running tsuku from a project directory with a .tsuku.toml.
func iRunFromDir(ctx context.Context, dir, command string) (context.Context, error) {
	state := getState(ctx)
	if state == nil {
		return ctx, fmt.Errorf("no test state")
	}

	fullDir := filepath.Join(state.homeDir, dir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		return ctx, fmt.Errorf("creating dir %s: %w", dir, err)
	}

	args := strings.Fields(command)
	if len(args) > 0 && args[0] == "tsuku" {
		args[0] = state.binPath
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = fullDir

	registryURL := state.repoRoot
	if state.emptyRegistry {
		emptyRegistry := filepath.Join(state.homeDir, "empty-registry")
		_ = os.MkdirAll(emptyRegistry, 0o755)
		registryURL = emptyRegistry
	}

	env := append(os.Environ(),
		"TSUKU_HOME="+state.homeDir,
		"TSUKU_NO_TELEMETRY=1",
		"TSUKU_REGISTRY_URL="+registryURL,
	)
	if len(state.hiddenBinaries) > 0 {
		env = append(env, "PATH="+filteredPATH(state.hiddenBinaries))
	}
	for k, v := range state.envOverrides {
		env = append(env, k+"="+v)
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

func theFileContains(ctx context.Context, path, text string) error {
	state := getState(ctx)
	fullPath := filepath.Join(state.homeDir, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("reading %q: %w", fullPath, err)
	}
	if !strings.Contains(string(data), text) {
		return fmt.Errorf("expected file %q to contain %q, got:\n%s", fullPath, text, string(data))
	}
	return nil
}

func theFileDoesNotContain(ctx context.Context, path, text string) error {
	state := getState(ctx)
	fullPath := filepath.Join(state.homeDir, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("reading %q: %w", fullPath, err)
	}
	if strings.Contains(string(data), text) {
		return fmt.Errorf("expected file %q not to contain %q", fullPath, text)
	}
	return nil
}

// iSourceHomeFileAndCanRun sources a file relative to $TSUKU_HOME in bash and then
// runs a command in the same shell. It verifies the command succeeds.
func iSourceHomeFileAndCanRun(ctx context.Context, sourceFile, command string) (context.Context, error) {
	state := getState(ctx)
	fullPath := filepath.Join(state.homeDir, sourceFile)

	script := fmt.Sprintf(`. "%s" && %s`, fullPath, command)
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(),
		"TSUKU_HOME="+state.homeDir,
		"TSUKU_NO_TELEMETRY=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return ctx, fmt.Errorf("after sourcing %q, command %q failed: %v\noutput: %s",
			sourceFile, command, err, string(out))
	}
	return ctx, nil
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
