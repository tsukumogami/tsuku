package functional

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
)

type stateKeyType struct{}

var stateKey = stateKeyType{}

type testState struct {
	homeDir  string
	binPath  string
	stdout   string
	stderr   string
	exitCode int
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
		// homeDir is relative to the binary's directory (repo root)
		homeDir := filepath.Join(filepath.Dir(binPath), ".tsuku-test")
		os.RemoveAll(homeDir)
		if err := os.MkdirAll(homeDir, 0o755); err != nil {
			return ctx, err
		}
		state := &testState{
			homeDir: homeDir,
			binPath: binPath,
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
	ctx.Step(`^the file "([^"]*)" exists$`, theFileExists)
	ctx.Step(`^the file "([^"]*)" does not exist$`, theFileDoesNotExist)
	ctx.Step(`^I can run "([^"]*)"$`, iCanRun)
}
