package functional

import (
	"context"
	"fmt"
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
	repoRoot       string
	stdout         string
	stderr         string
	exitCode       int
	hiddenBinaries []string          // binaries to hide from PATH (e.g., "cargo", "gem")
	emptyRegistry  bool              // when true, use an empty registry instead of repo's recipes/
	envOverrides   map[string]string // per-scenario env var overrides (appended last to win)
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

	paths := []string{"features"}
	if p := os.Getenv("TSUKU_TEST_PATHS"); p != "" {
		paths = strings.Split(p, string(os.PathListSeparator))
	}

	// One root for the whole run, holding a directory per scenario. A detached
	// background process can outlive the scenario that spawned it and re-create
	// paths under its home after the After hook has removed them, so the run
	// clears the root once at the end rather than trusting per-scenario removal
	// to be the last write.
	suiteRoot, err := os.MkdirTemp("", "tsuku-functional-")
	if err != nil {
		t.Fatalf("creating suite home root: %v", err)
	}
	defer func() { _ = os.RemoveAll(suiteRoot) }()

	opts := &godog.Options{
		Format: "pretty",
		Paths:  paths,
		// Strict fails the run on an undefined, pending, or ambiguous step.
		// Without it godog skips such a step, and the scenario still reports
		// green on whatever steps did match -- so an assertion can sit in a
		// feature file for months without ever executing.
		Strict:   true,
		TestingT: t,
	}
	if tags := os.Getenv("TSUKU_TEST_TAGS"); tags != "" {
		opts.Tags = tags
	}

	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			initializeScenario(ctx, binPath, suiteRoot)
		},
		Options: opts,
	}
	if suite.Run() != 0 {
		t.Fatal("functional tests failed")
	}
}

func initializeScenario(ctx *godog.ScenarioContext, binPath, suiteRoot string) {
	// Give every scenario its own home directory.
	//
	// Scenarios that exercise auto-update deliberately trigger detached
	// background processes -- `tsuku check-updates` and `tsuku apply-updates` --
	// that outlive the command that spawned them, and nothing reaps them. A
	// single shared home would let such a process from one scenario read and
	// write the paths the next scenario is setting up under it. Each scenario
	// gets its own directory instead, so a stray process carries the TSUKU_HOME
	// of the scenario that started it and cannot name any other scenario's
	// files.
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		repoRoot := filepath.Dir(binPath)
		homeDir, err := os.MkdirTemp(suiteRoot, "scenario-")
		if err != nil {
			return ctx, fmt.Errorf("creating scenario home: %w", err)
		}
		// MkdirTemp uses 0700; keep the 0755 the fixed home had, so scenarios
		// that inspect directory permissions see what they saw before.
		if err := os.Chmod(homeDir, 0o755); err != nil {
			return ctx, fmt.Errorf("setting scenario home permissions: %w", err)
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
		// Parse @empty-registry tag to use an empty registry for discovery tests
		// Parse @fake-llm-binary tag to create a fake tsuku-llm that exits with GPU error
		var hidden []string
		emptyRegistry := false
		envOverrides := make(map[string]string)
		for _, tag := range sc.Tags {
			if strings.HasPrefix(tag.Name, "@requires-no-") {
				binary := strings.TrimPrefix(tag.Name, "@requires-no-")
				hidden = append(hidden, binary)
			}
			if tag.Name == "@empty-registry" {
				emptyRegistry = true
			}
			if tag.Name == "@fake-llm-binary" {
				fakeBin := filepath.Join(homeDir, "fake-tsuku-llm")
				script := "#!/bin/sh\necho 'no GPU detected: tsuku-llm requires a GPU with at least 8 GB VRAM' >&2\nexit 1\n"
				if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
					return ctx, fmt.Errorf("creating fake LLM binary: %w", err)
				}
				envOverrides["TSUKU_LLM_BINARY"] = fakeBin
				// Strip cloud API keys so the factory falls back to local provider
				envOverrides["ANTHROPIC_API_KEY"] = ""
				envOverrides["GOOGLE_API_KEY"] = ""
				envOverrides["GEMINI_API_KEY"] = ""
			}
		}

		state := &testState{
			homeDir:        homeDir,
			binPath:        binPath,
			repoRoot:       repoRoot,
			hiddenBinaries: hidden,
			emptyRegistry:  emptyRegistry,
			envOverrides:   envOverrides,
		}
		return setState(ctx, state), nil
	})

	// Best-effort removal, so a long suite does not hold every scenario's home
	// at once. A detached process can re-create paths here afterward -- the
	// update checker reliably does -- which is why the suite root is cleared
	// again at the end of the run.
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if state := getState(ctx); state != nil && state.homeDir != "" {
			_ = os.RemoveAll(state.homeDir)
		}
		return ctx, nil
	})

	for _, def := range stepDefinitions() {
		ctx.Step(def.pattern, def.handler)
	}
}

// quotedArg matches a double-quoted step argument. Backslash escapes are
// allowed inside the quotes so a step can carry a literal `"` -- the check-deps
// JSON assertions need one. A plain `[^"]*` class stops at the first quote, so
// such a step matches nothing and godog reports it as undefined. Handlers pass
// the captured value through unescapeArg before using it.
const quotedArg = `"((?:[^"\\]|\\.)*)"`

// stepDefinition pairs a step pattern with the function implementing it.
type stepDefinition struct {
	pattern string
	handler any
}

// stepDefinitions returns every step this suite understands. Registration and
// the binding check in step_binding_test.go both read this one table, so a
// feature step that matches nothing here fails the unit-test job rather than
// waiting for someone to notice "undefined" in a functional run.
func stepDefinitions() []stepDefinition {
	return []stepDefinition{
		// Environment steps
		{`^a clean tsuku environment$`, aCleanTsukuEnvironment},

		// Command steps
		{`^I run ` + quotedArg + `$`, iRun},
		{`^I run from ` + quotedArg + ` ` + quotedArg + `$`, iRunFromDir},
		{`^I can run ` + quotedArg + `$`, iCanRun},
		{`^I source home file ` + quotedArg + ` and can run ` + quotedArg + `$`, iSourceHomeFileAndCanRun},
		{`^I create home file ` + quotedArg + ` with content:$`, iCreateHomeFile},
		{`^I set env ` + quotedArg + ` to ` + quotedArg + `$`, iSetEnv},

		// Assertion steps
		{`^the exit code is (\d+)$`, theExitCodeIs},
		{`^the exit code is not (\d+)$`, theExitCodeIsNot},
		{`^the output contains ` + quotedArg + `$`, theOutputContains},
		{`^the output does not contain ` + quotedArg + `$`, theOutputDoesNotContain},
		{`^the error output contains ` + quotedArg + `$`, theErrorOutputContains},
		{`^the error output does not contain ` + quotedArg + `$`, theErrorOutputDoesNotContain},
		{`^the file ` + quotedArg + ` exists$`, theFileExists},
		{`^the file ` + quotedArg + ` does not exist$`, theFileDoesNotExist},
		{`^the file ` + quotedArg + ` eventually does not exist within (\d+) seconds$`, theFileEventuallyDoesNotExist},
		{`^the file ` + quotedArg + ` contains ` + quotedArg + `$`, theFileContains},
		{`^the file ` + quotedArg + ` does not contain ` + quotedArg + `$`, theFileDoesNotContain},
	}
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
