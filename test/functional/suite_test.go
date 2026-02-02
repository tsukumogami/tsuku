package functional

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

type stateKeyType struct{}

var stateKey = stateKeyType{}

type testState struct {
	homeDir        string
	binPath        string
	stdout         string
	stderr         string
	exitCode       int
	hiddenBinaries []string // binaries to hide from PATH (e.g., "cargo", "gem")
}

func getState(ctx context.Context) *testState {
	if s, ok := ctx.Value(stateKey).(*testState); ok {
		return s
	}
	return nil
}

func setState(ctx context.Context, s *testState) context.Context {
	return context.WithValue(ctx, stateKey, s)
}

func TestFeatures(t *testing.T) {
	binPath := os.Getenv("TSUKU_TEST_BINARY")
	if binPath == "" {
		t.Skip("TSUKU_TEST_BINARY not set; run via 'make test-functional'")
	}

	// Resolve to absolute path since go test changes the working directory
	absBin, err := filepath.Abs(binPath)
	if err != nil {
		t.Fatalf("resolving binary path: %v", err)
	}
	binPath = absBin

	opts := &godog.Options{
		Format:   "pretty",
		Paths:    []string{"features"},
		TestingT: t,
	}
	if tags := os.Getenv("TSUKU_TEST_TAGS"); tags != "" {
		opts.Tags = tags
	}

	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			initializeScenario(ctx, binPath)
		},
		Options: opts,
	}
	if suite.Run() != 0 {
		t.Fatal("functional tests failed")
	}
}

func initializeScenario(ctx *godog.ScenarioContext, binPath string) {
	// Reset home directory before each scenario
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		repoRoot := filepath.Dir(binPath)
		// homeDir is relative to the binary's directory (repo root)
		homeDir := filepath.Join(repoRoot, ".tsuku-test")
		os.RemoveAll(homeDir)
		if err := os.MkdirAll(homeDir, 0o755); err != nil {
			return ctx, err
		}

		// Seed the discovery registry cache from the repo's per-tool files
		srcDir := filepath.Join(repoRoot, "recipes", "discovery")
		dstDir := filepath.Join(homeDir, "registry", "discovery")
		if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
			_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(srcDir, path)
				dst := filepath.Join(dstDir, rel)
				_ = os.MkdirAll(filepath.Dir(dst), 0o755)
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				_ = os.WriteFile(dst, data, 0o644)
				return nil
			})
		}

		// Parse @requires-no-<binary> tags to hide binaries from PATH
		var hidden []string
		for _, tag := range sc.Tags {
			if strings.HasPrefix(tag.Name, "@requires-no-") {
				binary := strings.TrimPrefix(tag.Name, "@requires-no-")
				hidden = append(hidden, binary)
			}
		}

		state := &testState{
			homeDir:        homeDir,
			binPath:        binPath,
			hiddenBinaries: hidden,
		}
		return setState(ctx, state), nil
	})

	// Environment steps
	ctx.Step(`^a clean tsuku environment$`, aCleanTsukuEnvironment)

	// Command steps
	ctx.Step(`^I run "([^"]*)"$`, iRun)

	// Assertion steps
	ctx.Step(`^the exit code is (\d+)$`, theExitCodeIs)
	ctx.Step(`^the exit code is not (\d+)$`, theExitCodeIsNot)
	ctx.Step(`^the output contains "([^"]*)"$`, theOutputContains)
	ctx.Step(`^the output does not contain "([^"]*)"$`, theOutputDoesNotContain)
	ctx.Step(`^the error output contains "([^"]*)"$`, theErrorOutputContains)
	ctx.Step(`^the error output does not contain "([^"]*)"$`, theErrorOutputDoesNotContain)
	ctx.Step(`^the file "([^"]*)" exists$`, theFileExists)
	ctx.Step(`^the file "([^"]*)" does not exist$`, theFileDoesNotExist)
	ctx.Step(`^I can run "([^"]*)"$`, iCanRun)
}

// filteredPATH returns a PATH string with directories containing any of the
// hidden binaries removed. This lets @requires-no-<binary> scenarios simulate
// environments where a toolchain isn't installed.
func filteredPATH(hidden []string) string {
	if len(hidden) == 0 {
		return os.Getenv("PATH")
	}

	var kept []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		exclude := false
		for _, bin := range hidden {
			candidate := filepath.Join(dir, bin)
			if _, err := exec.LookPath(candidate); err == nil {
				exclude = true
				break
			}
			// Also check directly since LookPath searches PATH
			if _, err := os.Stat(candidate); err == nil {
				exclude = true
				break
			}
		}
		if !exclude {
			kept = append(kept, dir)
		}
	}
	return strings.Join(kept, string(os.PathListSeparator))
}
