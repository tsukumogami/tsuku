package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// suggestTestRegistry is a minimal index.Registry for building the test index.
type suggestTestRegistry struct {
	recipes map[string][]byte
}

func (r *suggestTestRegistry) ListCached() ([]string, error) {
	names := make([]string, 0, len(r.recipes))
	for name := range r.recipes {
		names = append(names, name)
	}
	return names, nil
}

func (r *suggestTestRegistry) GetCached(name string) ([]byte, error) {
	data, ok := r.recipes[name]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func (r *suggestTestRegistry) ListAll(_ context.Context) ([]string, error) {
	return r.ListCached()
}

func (r *suggestTestRegistry) FetchRecipe(_ context.Context, name string) ([]byte, error) {
	data, ok := r.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found", name)
	}
	return data, nil
}

func (r *suggestTestRegistry) CacheRecipe(_ string, _ []byte) error {
	return nil
}

// suggestTestState is a minimal index.StateReader for building the test index.
type suggestTestState struct {
	tools map[string]index.ToolInfo
}

func (s *suggestTestState) AllTools() (map[string]index.ToolInfo, error) {
	return s.tools, nil
}

// suggestRecipeTOML returns minimal recipe TOML declaring one binary.
func suggestRecipeTOML(binaryName string) []byte {
	return []byte(fmt.Sprintf(`
[metadata]
name = "test"

[[steps]]
action = "install_binaries"
binaries = [%q]

[verify]
command = "test --version"
`, binaryName))
}

// buildSuggestTestConfig creates a temp tsuku home with a populated binary index
// and returns a config pointing to it.
func buildSuggestTestConfig(t *testing.T, recipes map[string][]byte, installed map[string]bool) *config.Config {
	t.Helper()

	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}

	dbPath := filepath.Join(cacheDir, "binary-index.db")
	idx, err := index.Open(dbPath, "")
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = idx.Close() }()

	tools := make(map[string]index.ToolInfo, len(installed))
	for name, isInstalled := range installed {
		if isInstalled {
			tools[name] = index.ToolInfo{ActiveVersion: "1.0.0"}
		}
	}

	reg := &suggestTestRegistry{recipes: recipes}
	state := &suggestTestState{tools: tools}
	if err := idx.Rebuild(context.Background(), reg, state); err != nil {
		t.Fatalf("index.Rebuild: %v", err)
	}

	return &config.Config{
		HomeDir:     home,
		ToolsDir:    filepath.Join(home, "tools"),
		RegistryDir: filepath.Join(home, "registry"),
		CacheDir:    cacheDir,
	}
}

func TestRunSuggest_SingleMatch(t *testing.T) {
	cfg := buildSuggestTestConfig(t,
		map[string][]byte{"jq": suggestRecipeTOML("bin/jq")},
		map[string]bool{},
	)

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "jq", false)

	if code != ExitSuccess {
		t.Errorf("exit code = %d, want %d", code, ExitSuccess)
	}
	out := stdout.String()
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("output %q does not contain expected prefix", out)
	}
	if !strings.Contains(out, "tsuku install jq") {
		t.Errorf("output %q does not contain install command", out)
	}
}

func TestRunSuggest_MultipleMatches(t *testing.T) {
	cfg := buildSuggestTestConfig(t,
		map[string][]byte{
			"vim":    suggestRecipeTOML("bin/vi"),
			"neovim": suggestRecipeTOML("bin/vi"),
		},
		map[string]bool{"vim": true},
	)

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "vi", false)

	if code != ExitSuccess {
		t.Errorf("exit code = %d, want %d", code, ExitSuccess)
	}
	out := stdout.String()
	if !strings.Contains(out, "Command 'vi' not found. Provided by:") {
		t.Errorf("output %q missing multi-match header", out)
	}
	if !strings.Contains(out, "tsuku install vim") {
		t.Errorf("output %q missing vim install command", out)
	}
	if !strings.Contains(out, "tsuku install neovim") {
		t.Errorf("output %q missing neovim install command", out)
	}
	if !strings.Contains(out, "(installed)") {
		t.Errorf("output %q missing installed marker for vim", out)
	}
}

func TestRunSuggest_NoMatch(t *testing.T) {
	cfg := buildSuggestTestConfig(t,
		map[string][]byte{"jq": suggestRecipeTOML("bin/jq")},
		map[string]bool{},
	)

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "nonexistent-xyz", false)

	if code != ExitGeneral {
		t.Errorf("exit code = %d, want %d (ExitGeneral)", code, ExitGeneral)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no output on no-match, got %q", stdout.String())
	}
}

func TestRunSuggest_IndexNotBuilt(t *testing.T) {
	// Config pointing to a dir with no binary-index.db
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}

	cfg := &config.Config{
		HomeDir:     home,
		CacheDir:    cacheDir,
		RegistryDir: filepath.Join(home, "registry"),
	}

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "jq", false)

	if code != ExitIndexNotBuilt {
		t.Errorf("exit code = %d, want %d (ExitIndexNotBuilt)", code, ExitIndexNotBuilt)
	}
}

func TestRunSuggest_JSONSingleMatch(t *testing.T) {
	cfg := buildSuggestTestConfig(t,
		map[string][]byte{"jq": suggestRecipeTOML("bin/jq")},
		map[string]bool{},
	)

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "jq", true)

	if code != ExitSuccess {
		t.Errorf("exit code = %d, want %d", code, ExitSuccess)
	}

	var out suggestJSONOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &out); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}
	if out.Command != "jq" {
		t.Errorf("JSON command = %q, want %q", out.Command, "jq")
	}
	if len(out.Matches) != 1 {
		t.Fatalf("JSON matches count = %d, want 1", len(out.Matches))
	}
	if out.Matches[0].Recipe != "jq" {
		t.Errorf("JSON matches[0].recipe = %q, want %q", out.Matches[0].Recipe, "jq")
	}
}

func TestRunSuggest_JSONNoMatch(t *testing.T) {
	cfg := buildSuggestTestConfig(t,
		map[string][]byte{"jq": suggestRecipeTOML("bin/jq")},
		map[string]bool{},
	)

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "nonexistent-xyz", true)

	if code != ExitGeneral {
		t.Errorf("exit code = %d, want %d (ExitGeneral)", code, ExitGeneral)
	}

	var out suggestJSONOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &out); err != nil {
		t.Fatalf("failed to parse JSON output on no-match: %v\noutput: %s", err, stdout.String())
	}
	if out.Command != "nonexistent-xyz" {
		t.Errorf("JSON command = %q, want %q", out.Command, "nonexistent-xyz")
	}
	if out.Matches == nil {
		t.Error("JSON matches should be empty array, not null")
	}
	if len(out.Matches) != 0 {
		t.Errorf("JSON matches count = %d, want 0", len(out.Matches))
	}
}

func TestRunSuggest_JSONIndexNotBuilt(t *testing.T) {
	// Config pointing to a dir with no binary-index.db
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}

	cfg := &config.Config{
		HomeDir:     home,
		CacheDir:    cacheDir,
		RegistryDir: filepath.Join(home, "registry"),
	}

	var stdout, stderr bytes.Buffer
	code := runSuggest(context.Background(), &stdout, &stderr, cfg, "jq", true)

	if code != ExitIndexNotBuilt {
		t.Errorf("exit code = %d, want %d (ExitIndexNotBuilt)", code, ExitIndexNotBuilt)
	}
	// JSON output should still be emitted even when index is not built
	outStr := strings.TrimSpace(stdout.String())
	if outStr == "" {
		t.Error("expected JSON output even when index not built, got empty")
	}
}
