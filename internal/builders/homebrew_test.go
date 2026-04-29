package builders

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

func TestHomebrewBuilder_Name(t *testing.T) {
	b := &HomebrewBuilder{}
	if got := b.Name(); got != "homebrew" {
		t.Errorf("Name() = %v, want %v", got, "homebrew")
	}
}

func TestHomebrewBuilder_CanBuild_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/ripgrep.json" {
			formulaInfo := map[string]interface{}{
				"name":      "ripgrep",
				"full_name": "ripgrep",
				"desc":      "Search tool like grep and The Silver Searcher",
				"homepage":  "https://github.com/BurntSushi/ripgrep",
				"versions": map[string]interface{}{
					"stable": "14.1.0",
					"bottle": true,
				},
				"deprecated": false,
				"disabled":   false,
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	canBuild, err := b.CanBuild(ctx, BuildRequest{Package: "ripgrep"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Errorf("CanBuild() = %v, want true", canBuild)
	}
}

func TestHomebrewBuilder_CanBuild_NotFound(t *testing.T) {
	// Create mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	canBuild, err := b.CanBuild(ctx, BuildRequest{Package: "nonexistent-formula"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Errorf("CanBuild() = %v, want false for nonexistent formula", canBuild)
	}
}

func TestHomebrewBuilder_CanBuild_NoBottles(t *testing.T) {
	// Create mock server that returns formula without bottles
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/no-bottles.json" {
			formulaInfo := map[string]interface{}{
				"name":      "no-bottles",
				"full_name": "no-bottles",
				"desc":      "A formula without bottles",
				"homepage":  "https://example.com",
				"versions": map[string]interface{}{
					"stable": "1.0.0",
					"bottle": false, // No bottles
				},
				"deprecated": false,
				"disabled":   false,
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	// With the new CanBuild, formulas without bottles can still be built from source
	canBuild, err := b.CanBuild(ctx, BuildRequest{Package: "no-bottles"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	// Source builds are always possible, so CanBuild returns true even without bottles
	if !canBuild {
		t.Errorf("CanBuild() = %v, want true (source build possible)", canBuild)
	}
}

func TestHomebrewBuilder_CanBuild_Disabled(t *testing.T) {
	// Create mock server that returns disabled formula
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/disabled.json" {
			formulaInfo := map[string]interface{}{
				"name":      "disabled",
				"full_name": "disabled",
				"desc":      "A disabled formula",
				"homepage":  "https://example.com",
				"versions": map[string]interface{}{
					"stable": "1.0.0",
					"bottle": true,
				},
				"deprecated": false,
				"disabled":   true, // Disabled
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	// Disabled formulas are treated as not found by fetchFormulaInfo,
	// so CanBuild returns false for them (they can't be built).
	canBuild, err := b.CanBuild(ctx, BuildRequest{Package: "disabled"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	// Disabled formulas return false from CanBuild
	if canBuild {
		t.Errorf("CanBuild() = %v, want false (disabled formula)", canBuild)
	}
}

func TestHomebrewBuilder_CanBuild_InvalidName(t *testing.T) {
	testCases := []struct {
		name     string
		formula  string
		expected bool
	}{
		{"empty", "", false},
		{"path traversal", "../etc/passwd", false},
		{"backslash", "foo\\bar", false},
		{"starts with hyphen", "-invalid", false},
		{"uppercase", "INVALID", false},
		{"shell metachar", "foo;ls", false},
		{"valid simple", "ripgrep", true},
		{"valid hyphen", "fd-find", true},
		{"valid underscore", "fd_find", true},
		{"valid versioned", "python@3.12", true},
		{"valid dot", "python3.12", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidHomebrewFormula(tc.formula)
			if result != tc.expected {
				t.Errorf("isValidHomebrewFormula(%q) = %v, want %v", tc.formula, result, tc.expected)
			}
		})
	}
}

func TestHomebrewBuilder_isValidPlatformTag(t *testing.T) {
	testCases := []struct {
		name     string
		tag      string
		expected bool
	}{
		{"arm64_sonoma", "arm64_sonoma", true},
		{"sonoma", "sonoma", true},
		{"arm64_linux", "arm64_linux", true},
		{"x86_64_linux", "x86_64_linux", true},
		{"arm64_ventura", "arm64_ventura", true},
		{"ventura", "ventura", true},
		{"invalid", "invalid_platform", false},
		{"empty", "", false},
		{"uppercase", "ARM64_SONOMA", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidPlatformTag(tc.tag)
			if result != tc.expected {
				t.Errorf("isValidPlatformTag(%q) = %v, want %v", tc.tag, result, tc.expected)
			}
		})
	}
}

func TestHomebrewBuilder_fetchFormulaInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/jq.json" {
			formulaInfo := map[string]interface{}{
				"name":      "jq",
				"full_name": "jq",
				"desc":      "Lightweight and flexible command-line JSON processor",
				"homepage":  "https://jqlang.github.io/jq/",
				"tap":       "homebrew/core",
				"versions": map[string]interface{}{
					"stable": "1.7.1",
					"bottle": true,
				},
				"deprecated":   false,
				"disabled":     false,
				"dependencies": []string{"oniguruma"},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	info, err := b.fetchFormulaInfo(ctx, "jq")
	if err != nil {
		t.Fatalf("fetchFormulaInfo() error = %v", err)
	}

	if info.Name != "jq" {
		t.Errorf("Name = %v, want jq", info.Name)
	}
	if info.Versions.Stable != "1.7.1" {
		t.Errorf("Version = %v, want 1.7.1", info.Versions.Stable)
	}
	if !info.Versions.Bottle {
		t.Errorf("Bottle = %v, want true", info.Versions.Bottle)
	}
}

func TestHomebrewBuilder_generateRecipe(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:        "ripgrep",
		Description: "Search tool like grep and The Silver Searcher",
		Homepage:    "https://github.com/BurntSushi/ripgrep",
	}
	formulaInfo.Versions.Stable = "14.1.0"

	recipeData := &homebrewRecipeData{
		Executables:   []string{"bin/rg"},
		Dependencies:  []string{"pcre2"},
		VerifyCommand: "rg --version",
	}

	recipe, err := b.generateRecipe("ripgrep", formulaInfo, recipeData)
	if err != nil {
		t.Fatalf("generateRecipe() error = %v", err)
	}

	if recipe.Metadata.Name != "ripgrep" {
		t.Errorf("Name = %v, want ripgrep", recipe.Metadata.Name)
	}
	if recipe.Metadata.Description != "Search tool like grep and The Silver Searcher" {
		t.Errorf("Description = %v, want Search tool like grep and The Silver Searcher", recipe.Metadata.Description)
	}
	// Version source should be empty (inferred from homebrew action)
	if recipe.Version.Source != "" {
		t.Errorf("Version.Source = %q, want empty (inferred from action)", recipe.Version.Source)
	}
	if recipe.Version.Formula != "" {
		t.Errorf("Version.Formula = %q, want empty (inferred from action)", recipe.Version.Formula)
	}
	if recipe.Verify.Command != "rg --version" {
		t.Errorf("Verify.Command = %v, want rg --version", recipe.Verify.Command)
	}
	if len(recipe.Steps) != 2 {
		t.Fatalf("len(Steps) = %v, want 2", len(recipe.Steps))
	}
	if recipe.Steps[0].Action != "homebrew" {
		t.Errorf("Steps[0].Action = %v, want homebrew", recipe.Steps[0].Action)
	}
	if recipe.Steps[1].Action != "install_binaries" {
		t.Errorf("Steps[1].Action = %v, want install_binaries", recipe.Steps[1].Action)
	}
	if len(recipe.Metadata.RuntimeDependencies) != 1 || recipe.Metadata.RuntimeDependencies[0] != "pcre2" {
		t.Errorf("RuntimeDependencies = %v, want [pcre2]", recipe.Metadata.RuntimeDependencies)
	}
}

func TestHomebrewBuilder_generateRecipe_NoExecutables(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name: "test",
	}

	recipeData := &homebrewRecipeData{
		Executables:   []string{}, // No executables
		VerifyCommand: "test --version",
	}

	_, err := b.generateRecipe("test", formulaInfo, recipeData)
	if err == nil {
		t.Error("generateRecipe() expected error for empty executables")
	}
}

func TestHomebrewBuilder_sanitizeFormulaJSON(t *testing.T) {
	b := &HomebrewBuilder{}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal json",
			input:    `{"name": "test", "desc": "A test formula"}`,
			expected: `{"name": "test", "desc": "A test formula"}`,
		},
		{
			name:     "with newlines",
			input:    "{\n\"name\": \"test\"\n}",
			expected: "{\n\"name\": \"test\"\n}",
		},
		{
			name:     "with control chars",
			input:    "{\x00\"name\": \"test\x01\x02\"}",
			expected: "{\"name\": \"test\"}",
		},
		{
			name:     "with tabs",
			input:    "{\t\"name\": \"test\"\t}",
			expected: "{\t\"name\": \"test\"\t}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := b.sanitizeFormulaJSON(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeFormulaJSON(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestHomebrewBuilder_buildSystemPrompt(t *testing.T) {
	b := &HomebrewBuilder{}
	prompt := b.buildSystemPrompt()

	// Check that the prompt contains key elements
	if len(prompt) == 0 {
		t.Error("buildSystemPrompt() returned empty string")
	}

	// Check for key content
	keywords := []string{
		"Homebrew",
		"executable",
		"extract_recipe",
		"homebrew",
		"verify",
	}

	for _, keyword := range keywords {
		found := false
		for i := 0; i <= len(prompt)-len(keyword); i++ {
			if prompt[i:i+len(keyword)] == keyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildSystemPrompt() missing keyword: %s", keyword)
		}
	}
}

func TestHomebrewBuilder_buildUserMessage(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:        "ripgrep",
		Description: "Search tool like grep",
		Homepage:    "https://github.com/BurntSushi/ripgrep",
	}
	formulaInfo.Versions.Stable = "14.1.0"
	formulaInfo.Dependencies = []string{"pcre2"}

	genCtx := &homebrewGenContext{
		formula:     "ripgrep",
		formulaInfo: formulaInfo,
	}

	message := b.buildUserMessage(genCtx)

	// Check that the message contains formula info
	if len(message) == 0 {
		t.Error("buildUserMessage() returned empty string")
	}

	// Check for formula name
	if !strings.Contains(message, "ripgrep") {
		t.Error("buildUserMessage() missing formula name")
	}

	// Check for version
	if !strings.Contains(message, "14.1.0") {
		t.Error("buildUserMessage() missing version")
	}

	// Check for dependencies
	if !strings.Contains(message, "pcre2") {
		t.Error("buildUserMessage() missing dependencies")
	}
}

func TestHomebrewBuilder_buildToolDefs(t *testing.T) {
	b := &HomebrewBuilder{}
	tools := b.buildToolDefs()

	if len(tools) != 3 {
		t.Fatalf("buildToolDefs() returned %d tools, want 3", len(tools))
	}

	expectedTools := map[string]bool{
		ToolFetchFormulaJSON: false,
		ToolInspectBottle:    false,
		ToolExtractRecipe:    false,
	}

	for _, tool := range tools {
		if _, ok := expectedTools[tool.Name]; !ok {
			t.Errorf("unexpected tool: %s", tool.Name)
		}
		expectedTools[tool.Name] = true
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestHomebrewBuilder_executeToolCall_ExtractRecipe(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "ripgrep",
	}

	testCases := []struct {
		name        string
		arguments   map[string]any
		expectError bool
	}{
		{
			name: "valid",
			arguments: map[string]any{
				"executables":    []any{"bin/rg"},
				"verify_command": "rg --version",
				"dependencies":   []any{"pcre2"},
			},
			expectError: false,
		},
		{
			name: "missing executables",
			arguments: map[string]any{
				"executables":    []any{},
				"verify_command": "rg --version",
			},
			expectError: true,
		},
		{
			name: "missing verify_command",
			arguments: map[string]any{
				"executables":    []any{"bin/rg"},
				"verify_command": "",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			toolCall := llm.ToolCall{
				Name:      ToolExtractRecipe,
				Arguments: tc.arguments,
			}

			_, data, err := b.executeToolCall(ctx, genCtx, toolCall)

			if tc.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if data == nil {
					t.Error("expected data but got nil")
				}
			}
		})
	}
}

func TestHomebrewBuilder_executeToolCall_FetchFormulaJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/jq.json" {
			formulaInfo := map[string]interface{}{
				"name": "jq",
				"desc": "JSON processor",
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula:    "jq",
		httpClient: server.Client(),
		apiURL:     server.URL,
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolFetchFormulaJSON,
		Arguments: map[string]any{
			"formula": "jq",
		},
	}

	result, data, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err != nil {
		t.Fatalf("executeToolCall() error = %v", err)
	}
	if data != nil {
		t.Error("expected data to be nil for fetch_formula_json")
	}
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
	if !strings.Contains(result, "jq") {
		t.Error("result should contain formula name")
	}
}

func TestHomebrewBuilder_executeToolCall_InvalidFormula(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "valid",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolFetchFormulaJSON,
		Arguments: map[string]any{
			"formula": "../invalid",
		},
	}

	_, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err == nil {
		t.Error("expected error for invalid formula name")
	}
}

func TestHomebrewBuilder_executeToolCall_InspectBottle(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "ripgrep",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolInspectBottle,
		Arguments: map[string]any{
			"formula":  "ripgrep",
			"platform": "x86_64_linux",
		},
	}

	result, data, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err != nil {
		t.Fatalf("executeToolCall() error = %v", err)
	}
	if data != nil {
		t.Error("expected data to be nil for inspect_bottle")
	}
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestHomebrewBuilder_executeToolCall_InvalidPlatform(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "ripgrep",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolInspectBottle,
		Arguments: map[string]any{
			"formula":  "ripgrep",
			"platform": "invalid_platform",
		},
	}

	_, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err == nil {
		t.Error("expected error for invalid platform")
	}
}

func TestHomebrewBuilder_executeToolCall_UnknownTool(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "test",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name:      "unknown_tool",
		Arguments: map[string]any{},
	}

	_, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestHomebrewFormulaNotFoundError(t *testing.T) {
	err := &HomebrewFormulaNotFoundError{Formula: "nonexistent"}
	expected := "Homebrew formula 'nonexistent' not found"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}

func TestHomebrewNoBottlesError(t *testing.T) {
	err := &HomebrewNoBottlesError{Formula: "nobottles"}
	expected := "Homebrew formula 'nobottles' has no bottles available"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}

func TestHomebrewBuilder_Options(t *testing.T) {
	// Test all option functions apply correctly
	httpClient := &http.Client{}
	b := &HomebrewBuilder{}

	// WithHomebrewHTTPClient
	opt := WithHomebrewHTTPClient(httpClient)
	opt(b)
	if b.httpClient != httpClient {
		t.Error("WithHomebrewHTTPClient did not set httpClient")
	}

	// WithHomebrewAPIURL
	opt = WithHomebrewAPIURL("http://example.com")
	opt(b)
	if b.homebrewAPIURL != "http://example.com" {
		t.Error("WithHomebrewAPIURL did not set homebrewAPIURL")
	}

	// WithHomebrewFactory
	factory := &llm.Factory{}
	opt = WithHomebrewFactory(factory)
	opt(b)
	if b.factory != factory {
		t.Error("WithHomebrewFactory did not set factory")
	}

	// WithHomebrewSanitizer
	sanitizer := validate.NewSanitizer()
	opt = WithHomebrewSanitizer(sanitizer)
	opt(b)
	if b.sanitizer != sanitizer {
		t.Error("WithHomebrewSanitizer did not set sanitizer")
	}

	// WithHomebrewProgressReporter
	progress := &mockProgressReporter{}
	opt = WithHomebrewProgressReporter(progress)
	opt(b)
	if b.progress != progress {
		t.Error("WithHomebrewProgressReporter did not set progress")
	}

	// WithHomebrewTelemetryClient
	telemetryClient := telemetry.NewClient()
	opt = WithHomebrewTelemetryClient(telemetryClient)
	opt(b)
	if b.telemetryClient != telemetryClient {
		t.Error("WithHomebrewTelemetryClient did not set telemetryClient")
	}
}

func TestHomebrewBuilder_reportProgress(t *testing.T) {
	// Test progress reporting with nil reporter (no-op)
	b := &HomebrewBuilder{}
	b.reportStart("test")
	b.reportDone("detail")
	b.reportFailed()
	// Should not panic

	// Test with mock reporter (reuse existing mockProgressReporter from github_release_test.go)
	mock := &mockProgressReporter{}
	b.progress = mock
	b.reportStart("starting")
	if len(mock.stages) != 1 || mock.stages[0] != "starting" {
		t.Errorf("reportStart not called correctly")
	}
	b.reportDone("done")
	if len(mock.dones) != 1 || mock.dones[0] != "done" {
		t.Errorf("reportDone not called correctly")
	}
	b.reportFailed()
	if mock.fails != 1 {
		t.Errorf("reportFailed not called correctly")
	}
}

func TestHomebrewBuilder_fetchFormulaInfo_Error(t *testing.T) {
	// Test error case with non-200 status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	_, err := b.fetchFormulaInfo(ctx, "test")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestHomebrewBuilder_inspectBottle(t *testing.T) {
	b := &HomebrewBuilder{}
	genCtx := &homebrewGenContext{
		formula: "jq",
	}

	ctx := context.Background()
	result, err := b.inspectBottle(ctx, genCtx, "jq", "x86_64_linux")
	if err != nil {
		t.Fatalf("inspectBottle() error = %v", err)
	}
	if !strings.Contains(result, "jq") {
		t.Error("result should contain formula name")
	}
	if !strings.Contains(result, "x86_64_linux") {
		t.Error("result should contain platform")
	}
}

func TestHomebrewBuilder_fetchFormulaJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/test.json" {
			_, _ = w.Write([]byte(`{"name": "test", "desc": "Test formula"}`))
			return
		}
		if r.URL.Path == "/api/formula/notfound.json" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/formula/servererror.json" {
			w.WriteHeader(500)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{}
	genCtx := &homebrewGenContext{
		httpClient: server.Client(),
		apiURL:     server.URL,
	}

	ctx := context.Background()

	// Test success
	result, err := b.fetchFormulaJSON(ctx, genCtx, "test")
	if err != nil {
		t.Fatalf("fetchFormulaJSON() error = %v", err)
	}
	if !strings.Contains(result, "test") {
		t.Error("result should contain formula name")
	}

	// Test 404
	_, err = b.fetchFormulaJSON(ctx, genCtx, "notfound")
	if err == nil {
		t.Error("expected error for 404")
	}

	// Test server error
	_, err = b.fetchFormulaJSON(ctx, genCtx, "servererror")
	if err == nil {
		t.Error("expected error for 500")
	}
}

func TestHomebrewBuilder_executeToolCall_DefaultFormula(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/defaultformula.json" {
			_, _ = w.Write([]byte(`{"name": "defaultformula"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{}
	genCtx := &homebrewGenContext{
		formula:    "defaultformula",
		httpClient: server.Client(),
		apiURL:     server.URL,
	}

	ctx := context.Background()

	// Test fetch_formula_json with empty formula (should use default)
	toolCall := llm.ToolCall{
		Name:      ToolFetchFormulaJSON,
		Arguments: map[string]any{},
	}

	result, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err != nil {
		t.Fatalf("executeToolCall() error = %v", err)
	}
	if !strings.Contains(result, "defaultformula") {
		t.Error("should use default formula")
	}

	// Test inspect_bottle with empty formula and platform (should use defaults)
	toolCall = llm.ToolCall{
		Name:      ToolInspectBottle,
		Arguments: map[string]any{},
	}

	result, _, err = b.executeToolCall(ctx, genCtx, toolCall)
	if err != nil {
		t.Fatalf("executeToolCall() error = %v", err)
	}
	if !strings.Contains(result, "defaultformula") {
		t.Error("should use default formula")
	}
}

func TestHomebrewBuilder_executeToolCall_InspectBottle_InvalidFormula(t *testing.T) {
	b := &HomebrewBuilder{}
	genCtx := &homebrewGenContext{
		formula: "valid",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolInspectBottle,
		Arguments: map[string]any{
			"formula":  "../invalid",
			"platform": "x86_64_linux",
		},
	}

	_, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err == nil {
		t.Error("expected error for invalid formula in inspect_bottle")
	}
}

func TestHomebrewBuilder_CanBuild_HTTPError(t *testing.T) {
	// Create mock server that returns server error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	_, err := b.CanBuild(ctx, BuildRequest{Package: "test"})
	if err == nil {
		t.Error("expected error for HTTP error")
	}
}

func TestHomebrewBuilder_generateRecipe_NoDependencies(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:        "simple",
		Description: "A simple tool",
		Homepage:    "https://example.com",
	}
	formulaInfo.Versions.Stable = "1.0.0"

	recipeData := &homebrewRecipeData{
		Executables:   []string{"bin/simple"},
		Dependencies:  nil, // No dependencies
		VerifyCommand: "simple --version",
	}

	recipe, err := b.generateRecipe("simple", formulaInfo, recipeData)
	if err != nil {
		t.Fatalf("generateRecipe() error = %v", err)
	}

	if len(recipe.Metadata.RuntimeDependencies) != 0 {
		t.Errorf("expected no runtime dependencies, got %v", recipe.Metadata.RuntimeDependencies)
	}
}

func TestHomebrewBuilder_buildUserMessage_NoDependencies(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:        "nodeps",
		Description: "No dependencies",
		Homepage:    "https://example.com",
	}
	formulaInfo.Versions.Stable = "1.0.0"
	formulaInfo.Dependencies = nil // No dependencies

	genCtx := &homebrewGenContext{
		formula:     "nodeps",
		formulaInfo: formulaInfo,
	}

	message := b.buildUserMessage(genCtx)
	if strings.Contains(message, "Runtime Dependencies:") {
		t.Error("message should not contain dependencies section when there are none")
	}
}

func TestIsValidHomebrewFormula_TooLong(t *testing.T) {
	// Test formula name that's too long (> 128 chars)
	longName := ""
	for i := 0; i < 130; i++ {
		longName += "a"
	}
	if isValidHomebrewFormula(longName) {
		t.Error("expected false for name > 128 chars")
	}
}

// mockRegistryChecker implements RegistryChecker for testing
type mockRegistryChecker struct {
	recipes map[string]bool
}

func (m *mockRegistryChecker) HasRecipe(name string) bool {
	if m.recipes == nil {
		return false
	}
	return m.recipes[name]
}

func TestDependencyNode_ToGenerationOrder_Empty(t *testing.T) {
	// Single node with no deps that already has a recipe
	node := &DependencyNode{
		Formula:       "existing",
		HasRecipe:     true,
		NeedsGenerate: false,
	}

	order := node.ToGenerationOrder()
	if len(order) != 0 {
		t.Errorf("ToGenerationOrder() = %v, want empty slice", order)
	}
}

func TestDependencyNode_ToGenerationOrder_Single(t *testing.T) {
	// Single node that needs generation
	node := &DependencyNode{
		Formula:       "new-tool",
		HasRecipe:     false,
		NeedsGenerate: true,
	}

	order := node.ToGenerationOrder()
	if len(order) != 1 || order[0] != "new-tool" {
		t.Errorf("ToGenerationOrder() = %v, want [new-tool]", order)
	}
}

func TestDependencyNode_ToGenerationOrder_Linear(t *testing.T) {
	// A -> B -> C (linear chain, all need generation)
	nodeC := &DependencyNode{
		Formula:       "c",
		NeedsGenerate: true,
	}
	nodeB := &DependencyNode{
		Formula:       "b",
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeC},
	}
	nodeA := &DependencyNode{
		Formula:       "a",
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeB},
	}

	order := nodeA.ToGenerationOrder()
	// Should be leaves first: c, b, a
	expected := []string{"c", "b", "a"}
	if len(order) != len(expected) {
		t.Fatalf("ToGenerationOrder() length = %d, want %d", len(order), len(expected))
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("ToGenerationOrder()[%d] = %v, want %v", i, order[i], want)
		}
	}
}

func TestDependencyNode_ToGenerationOrder_Diamond(t *testing.T) {
	// Diamond: A -> B, C; B -> D; C -> D (D is shared)
	nodeD := &DependencyNode{
		Formula:       "d",
		NeedsGenerate: true,
	}
	nodeB := &DependencyNode{
		Formula:       "b",
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeD},
	}
	nodeC := &DependencyNode{
		Formula:       "c",
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeD}, // Same nodeD (diamond)
	}
	nodeA := &DependencyNode{
		Formula:       "a",
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeB, nodeC},
	}

	order := nodeA.ToGenerationOrder()

	// D should appear only once and before B, C, A
	// Order should be: d, b, c, a (or d, c, b, a depending on traversal)
	if len(order) != 4 {
		t.Fatalf("ToGenerationOrder() length = %d, want 4", len(order))
	}

	// D must be first (leaf), A must be last (root)
	if order[0] != "d" {
		t.Errorf("ToGenerationOrder()[0] = %v, want d", order[0])
	}
	if order[3] != "a" {
		t.Errorf("ToGenerationOrder()[3] = %v, want a", order[3])
	}

	// Check no duplicates
	seen := make(map[string]bool)
	for _, f := range order {
		if seen[f] {
			t.Errorf("Duplicate formula in order: %s", f)
		}
		seen[f] = true
	}
}

func TestDependencyNode_ToGenerationOrder_MixedRecipeStatus(t *testing.T) {
	// A -> B -> C, but B already has a recipe
	nodeC := &DependencyNode{
		Formula:       "c",
		HasRecipe:     false,
		NeedsGenerate: true,
	}
	nodeB := &DependencyNode{
		Formula:       "b",
		HasRecipe:     true,
		NeedsGenerate: false, // Already has recipe
		Children:      []*DependencyNode{nodeC},
	}
	nodeA := &DependencyNode{
		Formula:       "a",
		HasRecipe:     false,
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeB},
	}

	order := nodeA.ToGenerationOrder()
	// Should only include c and a (b has recipe)
	expected := []string{"c", "a"}
	if len(order) != len(expected) {
		t.Fatalf("ToGenerationOrder() length = %d, want %d", len(order), len(expected))
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("ToGenerationOrder()[%d] = %v, want %v", i, order[i], want)
		}
	}
}

func TestDependencyNode_CountNeedingGeneration(t *testing.T) {
	nodeC := &DependencyNode{
		Formula:       "c",
		NeedsGenerate: true,
	}
	nodeB := &DependencyNode{
		Formula:       "b",
		NeedsGenerate: false, // Has recipe
		Children:      []*DependencyNode{nodeC},
	}
	nodeA := &DependencyNode{
		Formula:       "a",
		NeedsGenerate: true,
		Children:      []*DependencyNode{nodeB},
	}

	count := nodeA.CountNeedingGeneration()
	if count != 2 {
		t.Errorf("CountNeedingGeneration() = %d, want 2", count)
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_NoDeps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/simple.json" {
			formulaInfo := map[string]interface{}{
				"name":         "simple",
				"desc":         "A simple tool",
				"dependencies": []string{},
				"versions": map[string]interface{}{
					"stable": "1.0.0",
					"bottle": true,
				},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
		registry:       &mockRegistryChecker{recipes: map[string]bool{}},
	}

	ctx := context.Background()
	tree, err := b.DiscoverDependencyTree(ctx, "simple")
	if err != nil {
		t.Fatalf("DiscoverDependencyTree() error = %v", err)
	}

	if tree.Formula != "simple" {
		t.Errorf("Formula = %v, want simple", tree.Formula)
	}
	if len(tree.Children) != 0 {
		t.Errorf("Children length = %d, want 0", len(tree.Children))
	}
	if tree.HasRecipe {
		t.Error("HasRecipe = true, want false")
	}
	if !tree.NeedsGenerate {
		t.Error("NeedsGenerate = false, want true")
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_WithDeps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/formula/ripgrep.json":
			formulaInfo := map[string]interface{}{
				"name":         "ripgrep",
				"desc":         "Search tool",
				"dependencies": []string{"pcre2"},
				"versions": map[string]interface{}{
					"stable": "14.1.0",
					"bottle": true,
				},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
		case "/api/formula/pcre2.json":
			formulaInfo := map[string]interface{}{
				"name":         "pcre2",
				"desc":         "Regex library",
				"dependencies": []string{},
				"versions": map[string]interface{}{
					"stable": "10.42",
					"bottle": true,
				},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
		registry:       &mockRegistryChecker{recipes: map[string]bool{}},
	}

	ctx := context.Background()
	tree, err := b.DiscoverDependencyTree(ctx, "ripgrep")
	if err != nil {
		t.Fatalf("DiscoverDependencyTree() error = %v", err)
	}

	if tree.Formula != "ripgrep" {
		t.Errorf("Formula = %v, want ripgrep", tree.Formula)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("Children length = %d, want 1", len(tree.Children))
	}
	if tree.Children[0].Formula != "pcre2" {
		t.Errorf("Child formula = %v, want pcre2", tree.Children[0].Formula)
	}

	// Check generation order
	order := tree.ToGenerationOrder()
	expected := []string{"pcre2", "ripgrep"}
	if len(order) != len(expected) {
		t.Fatalf("ToGenerationOrder() length = %d, want %d", len(order), len(expected))
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("ToGenerationOrder()[%d] = %v, want %v", i, order[i], want)
		}
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_DiamondDeps(t *testing.T) {
	// Diamond: app -> lib-a, lib-b; lib-a -> shared; lib-b -> shared
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		formulas := map[string]map[string]interface{}{
			"/api/formula/app.json": {
				"name":         "app",
				"dependencies": []string{"lib-a", "lib-b"},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			},
			"/api/formula/lib-a.json": {
				"name":         "lib-a",
				"dependencies": []string{"shared"},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			},
			"/api/formula/lib-b.json": {
				"name":         "lib-b",
				"dependencies": []string{"shared"},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			},
			"/api/formula/shared.json": {
				"name":         "shared",
				"dependencies": []string{},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			},
		}
		if formula, ok := formulas[r.URL.Path]; ok {
			_ = json.NewEncoder(w).Encode(formula)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
		registry:       &mockRegistryChecker{recipes: map[string]bool{}},
	}

	ctx := context.Background()
	tree, err := b.DiscoverDependencyTree(ctx, "app")
	if err != nil {
		t.Fatalf("DiscoverDependencyTree() error = %v", err)
	}

	// Verify structure: app has 2 children (lib-a, lib-b)
	if len(tree.Children) != 2 {
		t.Fatalf("app.Children length = %d, want 2", len(tree.Children))
	}

	// Both lib-a and lib-b should have shared as child
	// And it should be the SAME node (pointer equality)
	var sharedFromA, sharedFromB *DependencyNode
	for _, child := range tree.Children {
		if len(child.Children) == 1 {
			if child.Formula == "lib-a" {
				sharedFromA = child.Children[0]
			} else if child.Formula == "lib-b" {
				sharedFromB = child.Children[0]
			}
		}
	}

	if sharedFromA == nil || sharedFromB == nil {
		t.Fatal("Expected both lib-a and lib-b to have shared as child")
	}

	if sharedFromA != sharedFromB {
		t.Error("Diamond dependency 'shared' should be the same node instance")
	}

	// Check generation order - shared should appear only once
	order := tree.ToGenerationOrder()
	if len(order) != 4 {
		t.Errorf("ToGenerationOrder() length = %d, want 4", len(order))
	}

	// shared must come before lib-a and lib-b
	sharedIdx := -1
	libAIdx := -1
	libBIdx := -1
	appIdx := -1
	for i, f := range order {
		switch f {
		case "shared":
			sharedIdx = i
		case "lib-a":
			libAIdx = i
		case "lib-b":
			libBIdx = i
		case "app":
			appIdx = i
		}
	}

	if sharedIdx == -1 || libAIdx == -1 || libBIdx == -1 || appIdx == -1 {
		t.Errorf("Missing formula in order: %v", order)
	}

	if sharedIdx > libAIdx || sharedIdx > libBIdx {
		t.Error("shared should come before lib-a and lib-b")
	}
	if appIdx != len(order)-1 {
		t.Error("app should be last in generation order")
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_WithExistingRecipes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/formula/app.json":
			formulaInfo := map[string]interface{}{
				"name":         "app",
				"dependencies": []string{"dep1", "dep2"},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
		case "/api/formula/dep1.json":
			formulaInfo := map[string]interface{}{
				"name":         "dep1",
				"dependencies": []string{},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
		case "/api/formula/dep2.json":
			formulaInfo := map[string]interface{}{
				"name":         "dep2",
				"dependencies": []string{},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// dep1 already has a recipe
	registry := &mockRegistryChecker{
		recipes: map[string]bool{
			"dep1": true,
		},
	}

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
		registry:       registry,
	}

	ctx := context.Background()
	tree, err := b.DiscoverDependencyTree(ctx, "app")
	if err != nil {
		t.Fatalf("DiscoverDependencyTree() error = %v", err)
	}

	// Find dep1 and dep2 in children
	var dep1, dep2 *DependencyNode
	for _, child := range tree.Children {
		if child.Formula == "dep1" {
			dep1 = child
		} else if child.Formula == "dep2" {
			dep2 = child
		}
	}

	if dep1 == nil || dep2 == nil {
		t.Fatal("Expected dep1 and dep2 as children")
		return
	}

	if !dep1.HasRecipe {
		t.Error("dep1.HasRecipe = false, want true")
	}
	if dep1.NeedsGenerate {
		t.Error("dep1.NeedsGenerate = true, want false")
	}
	if dep2.HasRecipe {
		t.Error("dep2.HasRecipe = true, want false")
	}
	if !dep2.NeedsGenerate {
		t.Error("dep2.NeedsGenerate = false, want true")
	}

	// Generation order should only include dep2 and app (dep1 has recipe)
	order := tree.ToGenerationOrder()
	if len(order) != 2 {
		t.Errorf("ToGenerationOrder() length = %d, want 2", len(order))
	}

	// Check dep1 is NOT in the order
	for _, f := range order {
		if f == "dep1" {
			t.Error("dep1 should not be in generation order (has recipe)")
		}
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
	}

	ctx := context.Background()
	_, err := b.DiscoverDependencyTree(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent formula")
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_InvalidFormula(t *testing.T) {
	b := &HomebrewBuilder{}

	ctx := context.Background()
	_, err := b.DiscoverDependencyTree(ctx, "../invalid")
	if err == nil {
		t.Error("Expected error for invalid formula name")
	}
}

func TestHomebrewBuilder_DiscoverDependencyTree_NoRegistry(t *testing.T) {
	// When registry is nil, all formulas should be marked as needing generation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/test.json" {
			formulaInfo := map[string]interface{}{
				"name":         "test",
				"dependencies": []string{},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
		registry:       nil, // No registry
	}

	ctx := context.Background()
	tree, err := b.DiscoverDependencyTree(ctx, "test")
	if err != nil {
		t.Fatalf("DiscoverDependencyTree() error = %v", err)
	}

	if tree.HasRecipe {
		t.Error("HasRecipe = true, want false when registry is nil")
	}
	if !tree.NeedsGenerate {
		t.Error("NeedsGenerate = false, want true when registry is nil")
	}
}

func TestWithRegistryChecker(t *testing.T) {
	registry := &mockRegistryChecker{recipes: map[string]bool{"test": true}}
	b := &HomebrewBuilder{}

	opt := WithRegistryChecker(registry)
	opt(b)

	if b.registry != registry {
		t.Error("WithRegistryChecker did not set registry")
	}
}

func TestDependencyNode_FormatTree_Simple(t *testing.T) {
	node := &DependencyNode{
		Formula:       "simple",
		NeedsGenerate: true,
	}

	output := node.FormatTree()
	if !strings.Contains(output, "simple") {
		t.Error("FormatTree should contain formula name")
	}
	if !strings.Contains(output, "needs recipe") {
		t.Error("FormatTree should indicate needs recipe")
	}
}

func TestDependencyNode_FormatTree_WithChildren(t *testing.T) {
	child := &DependencyNode{
		Formula:       "child",
		NeedsGenerate: true,
	}
	parent := &DependencyNode{
		Formula:       "parent",
		NeedsGenerate: true,
		Children:      []*DependencyNode{child},
	}

	output := parent.FormatTree()
	if !strings.Contains(output, "parent") {
		t.Error("FormatTree should contain parent")
	}
	if !strings.Contains(output, "child") {
		t.Error("FormatTree should contain child")
	}
}

func TestDependencyNode_FormatTree_WithRecipe(t *testing.T) {
	node := &DependencyNode{
		Formula:       "existing",
		HasRecipe:     true,
		NeedsGenerate: false,
	}

	output := node.FormatTree()
	if !strings.Contains(output, "has recipe") {
		t.Error("FormatTree should indicate has recipe")
	}
}

func TestDependencyNode_FormatTree_Diamond(t *testing.T) {
	shared := &DependencyNode{
		Formula:       "shared",
		NeedsGenerate: true,
	}
	childA := &DependencyNode{
		Formula:       "child-a",
		NeedsGenerate: true,
		Children:      []*DependencyNode{shared},
	}
	childB := &DependencyNode{
		Formula:       "child-b",
		NeedsGenerate: true,
		Children:      []*DependencyNode{shared}, // Same shared node
	}
	parent := &DependencyNode{
		Formula:       "parent",
		NeedsGenerate: true,
		Children:      []*DependencyNode{childA, childB},
	}

	output := parent.FormatTree()
	// Shared should appear with [duplicate] marker on second occurrence
	if !strings.Contains(output, "[duplicate]") {
		t.Error("FormatTree should mark duplicate nodes")
	}
}

func TestDependencyNode_EstimatedCost(t *testing.T) {
	node := &DependencyNode{
		Formula:       "a",
		NeedsGenerate: true,
		Children: []*DependencyNode{
			{Formula: "b", NeedsGenerate: true},
			{Formula: "c", NeedsGenerate: false, HasRecipe: true},
		},
	}

	cost := node.EstimatedCost()
	expected := 2 * EstimatedCostPerRecipe // a and b need generation, c doesn't
	if cost != expected {
		t.Errorf("EstimatedCost() = %v, want %v", cost, expected)
	}
}

func TestNewConfirmationRequest(t *testing.T) {
	existing := &DependencyNode{
		Formula:       "existing",
		HasRecipe:     true,
		NeedsGenerate: false,
	}
	newDep := &DependencyNode{
		Formula:       "new-dep",
		HasRecipe:     false,
		NeedsGenerate: true,
	}
	root := &DependencyNode{
		Formula:       "root",
		HasRecipe:     false,
		NeedsGenerate: true,
		Children:      []*DependencyNode{existing, newDep},
	}

	req := NewConfirmationRequest(root)

	if len(req.ToGenerate) != 2 {
		t.Errorf("ToGenerate length = %d, want 2", len(req.ToGenerate))
	}
	if len(req.AlreadyHave) != 1 || req.AlreadyHave[0] != "existing" {
		t.Errorf("AlreadyHave = %v, want [existing]", req.AlreadyHave)
	}
	if req.EstimatedCost != 2*EstimatedCostPerRecipe {
		t.Errorf("EstimatedCost = %v, want %v", req.EstimatedCost, 2*EstimatedCostPerRecipe)
	}
	if req.FormattedTree == "" {
		t.Error("FormattedTree should not be empty")
	}
	if req.Tree != root {
		t.Error("Tree should reference original tree")
	}
}

func TestHomebrewBuilder_checkBottleAvailability_AllPlatforms(t *testing.T) {
	// Create mock server that simulates GHCR token and manifest endpoints
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token") {
			// Return mock token
			_, _ = w.Write([]byte(`{"token": "test-token"}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/homebrew/core/testformula/manifests/") {
			// Return manifest with all platforms
			manifest := `{
				"manifests": [
					{"annotations": {"org.opencontainers.image.ref.name": "1.0.0.arm64_sonoma"}},
					{"annotations": {"org.opencontainers.image.ref.name": "1.0.0.sonoma"}},
					{"annotations": {"org.opencontainers.image.ref.name": "1.0.0.x86_64_linux"}},
					{"annotations": {"org.opencontainers.image.ref.name": "1.0.0.arm64_linux"}}
				]
			}`
			_, _ = w.Write([]byte(manifest))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create HTTP client that routes to mock server
	client := server.Client()
	originalTransport := client.Transport
	client.Transport = &mockGHCRTransport{
		serverURL: server.URL,
		base:      originalTransport,
	}

	b := &HomebrewBuilder{
		httpClient: client,
	}

	ctx := context.Background()
	availability, err := b.checkBottleAvailability(ctx, "testformula", "1.0.0")
	if err != nil {
		t.Fatalf("checkBottleAvailability() error = %v", err)
	}

	if len(availability.Available) != 4 {
		t.Errorf("expected 4 available platforms, got %d", len(availability.Available))
	}
	if len(availability.Unavailable) != 0 {
		t.Errorf("expected 0 unavailable platforms, got %d", len(availability.Unavailable))
	}
}

func TestHomebrewBuilder_checkBottleAvailability_MissingPlatforms(t *testing.T) {
	// Create mock server with only some platforms available
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"token": "test-token"}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/homebrew/core/partialformula/manifests/") {
			// Return manifest with only macOS platforms (Linux missing)
			manifest := `{
				"manifests": [
					{"annotations": {"org.opencontainers.image.ref.name": "1.0.0.arm64_sonoma"}},
					{"annotations": {"org.opencontainers.image.ref.name": "1.0.0.sonoma"}}
				]
			}`
			_, _ = w.Write([]byte(manifest))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &mockGHCRTransport{
		serverURL: server.URL,
		base:      client.Transport,
	}

	b := &HomebrewBuilder{
		httpClient: client,
	}

	ctx := context.Background()
	availability, err := b.checkBottleAvailability(ctx, "partialformula", "1.0.0")
	if err != nil {
		t.Fatalf("checkBottleAvailability() error = %v", err)
	}

	if len(availability.Available) != 2 {
		t.Errorf("expected 2 available platforms, got %d: %v", len(availability.Available), availability.Available)
	}
	if len(availability.Unavailable) != 2 {
		t.Errorf("expected 2 unavailable platforms, got %d: %v", len(availability.Unavailable), availability.Unavailable)
	}

	// Check that Linux platforms are in unavailable list
	unavailableSet := make(map[string]bool)
	for _, p := range availability.Unavailable {
		unavailableSet[p] = true
	}
	if !unavailableSet["x86_64_linux"] {
		t.Error("expected x86_64_linux to be unavailable")
	}
	if !unavailableSet["arm64_linux"] {
		t.Error("expected arm64_linux to be unavailable")
	}
}

func TestHomebrewBuilder_checkBottleAvailability_TokenError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token") {
			w.WriteHeader(500)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &mockGHCRTransport{
		serverURL: server.URL,
		base:      client.Transport,
	}

	b := &HomebrewBuilder{
		httpClient: client,
	}

	ctx := context.Background()
	_, err := b.checkBottleAvailability(ctx, "testformula", "1.0.0")
	if err == nil {
		t.Error("expected error for token failure")
	}
}

func TestHomebrewBuilder_checkBottleAvailability_ManifestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"token": "test-token"}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/homebrew/core/") {
			w.WriteHeader(404)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = &mockGHCRTransport{
		serverURL: server.URL,
		base:      client.Transport,
	}

	b := &HomebrewBuilder{
		httpClient: client,
	}

	ctx := context.Background()
	_, err := b.checkBottleAvailability(ctx, "nonexistent", "1.0.0")
	if err == nil {
		t.Error("expected error for manifest failure")
	}
}

func TestBottleAvailability_PlatformDisplayNames(t *testing.T) {
	// Verify all target platforms have display names
	for _, platform := range targetPlatforms {
		if platformDisplayNames[platform] == "" {
			t.Errorf("missing display name for platform %s", platform)
		}
	}
}

// mockGHCRTransport redirects GHCR requests to the test server
type mockGHCRTransport struct {
	serverURL string
	base      http.RoundTripper
}

func (t *mockGHCRTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect ghcr.io requests to our test server
	if req.URL.Host == "ghcr.io" {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(t.serverURL, "http://")
	}
	if t.base != nil {
		return t.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestHomebrewBuilder_getBlobSHAFromManifest(t *testing.T) {
	b := &HomebrewBuilder{}

	tests := []struct {
		name        string
		manifest    *ghcrManifest
		version     string
		platformTag string
		wantSHA     string
		wantErr     bool
	}{
		{
			name: "valid manifest with bottle digest",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{
					{
						Digest: "sha256:other123",
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0.arm64_sonoma",
						},
					},
					{
						Digest: "sha256:manifest456",
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0.x86_64_linux",
							"sh.brew.bottle.digest":             "sha256:bottle789",
						},
					},
				},
			},
			version:     "1.0.0",
			platformTag: "x86_64_linux",
			wantSHA:     "bottle789",
			wantErr:     false,
		},
		{
			name: "fallback to manifest digest",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{
					{
						Digest: "sha256:manifest123",
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0.x86_64_linux",
						},
					},
				},
			},
			version:     "1.0.0",
			platformTag: "x86_64_linux",
			wantSHA:     "manifest123",
			wantErr:     false,
		},
		{
			name: "platform not found",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{
					{
						Digest: "sha256:other123",
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0.arm64_linux",
						},
					},
				},
			},
			version:     "1.0.0",
			platformTag: "x86_64_linux",
			wantErr:     true,
		},
		{
			name: "empty manifest",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{},
			},
			version:     "1.0.0",
			platformTag: "x86_64_linux",
			wantErr:     true,
		},
		{
			// libevent-style: formula has revision=1 and the manifest
			// entries are tagged "<version>_1.<platform>". Resolver
			// must accept the revision-suffixed form.
			name: "revision-suffixed entries (libevent-style)",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{
					{
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "2.1.12_1.arm64_sonoma",
							"sh.brew.bottle.digest":             "sha256:abc",
						},
					},
					{
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "2.1.12_1.x86_64_linux",
							"sh.brew.bottle.digest":             "sha256:def",
						},
					},
				},
			},
			version:     "2.1.12",
			platformTag: "arm64_sonoma",
			wantSHA:     "abc",
		},
		{
			// Mixed manifests (rare but possible during a homebrew
			// transition): both unrevised and _1 entries present.
			// Resolver must prefer the highest revision.
			name: "mixed entries — prefer highest revision",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{
					{
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0.arm64_sonoma",
							"sh.brew.bottle.digest":             "sha256:rev0",
						},
					},
					{
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0_1.arm64_sonoma",
							"sh.brew.bottle.digest":             "sha256:rev1",
						},
					},
					{
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0_2.arm64_sonoma",
							"sh.brew.bottle.digest":             "sha256:rev2",
						},
					},
				},
			},
			version:     "1.0.0",
			platformTag: "arm64_sonoma",
			wantSHA:     "rev2",
		},
		{
			// Other version's revision-suffixed entries must not
			// match. e.g., "1.0.0_1.arm64" should not match version
			// "1.0".
			name: "revisioned entry for different version is not matched",
			manifest: &ghcrManifest{
				Manifests: []ghcrManifestEntry{
					{
						Annotations: map[string]string{
							"org.opencontainers.image.ref.name": "1.0.0_1.arm64_sonoma",
							"sh.brew.bottle.digest":             "sha256:rev1",
						},
					},
				},
			},
			version:     "1.0",
			platformTag: "arm64_sonoma",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sha, err := b.getBlobSHAFromManifest(tt.manifest, tt.version, tt.platformTag)
			if (err != nil) != tt.wantErr {
				t.Errorf("getBlobSHAFromManifest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sha != tt.wantSHA {
				t.Errorf("getBlobSHAFromManifest() = %v, want %v", sha, tt.wantSHA)
			}
		})
	}
}

// createTestBottleTarball builds a gzipped tarball from the given entries and
// writes it to a temp file. Each entry specifies a tar path, body content, and
// type flag. Returns the temp file path; the caller should defer os.Remove.
func createTestBottleTarball(t *testing.T, entries []struct {
	name     string
	body     string
	typeflag byte
}) string {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0755,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
		}
		if e.typeflag == tar.TypeSymlink {
			hdr.Linkname = e.body
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if e.typeflag != tar.TypeSymlink {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("failed to write tar content: %v", err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "test-bottle-*.tar.gz")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := tmpFile.Write(buf.Bytes()); err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

func TestHomebrewBuilder_extractBottleContents(t *testing.T) {
	b := &HomebrewBuilder{}

	t.Run("mixed tarball with binaries, libs, and includes", func(t *testing.T) {
		path := createTestBottleTarball(t, []struct {
			name     string
			body     string
			typeflag byte
		}{
			{"jq/1.7.1/bin/jq", "binary content", tar.TypeReg},
			{"jq/1.7.1/bin/jqp", "binary content", tar.TypeReg},
			{"jq/1.7.1/share/man/man1/jq.1", "man page", tar.TypeReg},
			{"jq/1.7.1/lib/libjq.so", "lib content", tar.TypeReg},
			{"jq/1.7.1/lib/libjq.a", "lib content", tar.TypeReg},
			{"jq/1.7.1/lib/pkgconfig/jq.pc", "pkg-config", tar.TypeReg},
			{"jq/1.7.1/include/jq.h", "header", tar.TypeReg},
		})
		defer os.Remove(path)

		contents, err := b.extractBottleContents(path)
		if err != nil {
			t.Fatalf("extractBottleContents() error = %v", err)
		}

		// Binaries
		expectedBins := map[string]bool{"jq": true, "jqp": true}
		if len(contents.Binaries) != len(expectedBins) {
			t.Errorf("Binaries: got %d, want %d: %v", len(contents.Binaries), len(expectedBins), contents.Binaries)
		}
		for _, b := range contents.Binaries {
			if !expectedBins[b] {
				t.Errorf("unexpected binary: %s", b)
			}
		}

		// LibFiles
		expectedLibs := map[string]bool{
			"lib/libjq.so":        true,
			"lib/libjq.a":         true,
			"lib/pkgconfig/jq.pc": true,
		}
		if len(contents.LibFiles) != len(expectedLibs) {
			t.Errorf("LibFiles: got %d, want %d: %v", len(contents.LibFiles), len(expectedLibs), contents.LibFiles)
		}
		for _, l := range contents.LibFiles {
			if !expectedLibs[l] {
				t.Errorf("unexpected lib file: %s", l)
			}
		}

		// Includes
		expectedIncludes := map[string]bool{"include/jq.h": true}
		if len(contents.Includes) != len(expectedIncludes) {
			t.Errorf("Includes: got %d, want %d: %v", len(contents.Includes), len(expectedIncludes), contents.Includes)
		}
		for _, inc := range contents.Includes {
			if !expectedIncludes[inc] {
				t.Errorf("unexpected include: %s", inc)
			}
		}
	})

	t.Run("library-only bottle with no binaries", func(t *testing.T) {
		path := createTestBottleTarball(t, []struct {
			name     string
			body     string
			typeflag byte
		}{
			{"bdw-gc/8.2.6/lib/libgc.so", "lib", tar.TypeReg},
			{"bdw-gc/8.2.6/lib/libgc.so.1.5.0", "lib", tar.TypeReg},
			{"bdw-gc/8.2.6/lib/libgc.a", "lib", tar.TypeReg},
			{"bdw-gc/8.2.6/lib/pkgconfig/gc.pc", "pc", tar.TypeReg},
			{"bdw-gc/8.2.6/include/gc.h", "header", tar.TypeReg},
			{"bdw-gc/8.2.6/include/gc/gc_config.h", "header", tar.TypeReg},
		})
		defer os.Remove(path)

		contents, err := b.extractBottleContents(path)
		if err != nil {
			t.Fatalf("extractBottleContents() error = %v", err)
		}

		if len(contents.Binaries) != 0 {
			t.Errorf("Binaries: got %d, want 0: %v", len(contents.Binaries), contents.Binaries)
		}

		expectedLibs := map[string]bool{
			"lib/libgc.so":        true,
			"lib/libgc.so.1.5.0":  true,
			"lib/libgc.a":         true,
			"lib/pkgconfig/gc.pc": true,
		}
		if len(contents.LibFiles) != len(expectedLibs) {
			t.Errorf("LibFiles: got %d, want %d: %v", len(contents.LibFiles), len(expectedLibs), contents.LibFiles)
		}
		for _, l := range contents.LibFiles {
			if !expectedLibs[l] {
				t.Errorf("unexpected lib file: %s", l)
			}
		}

		expectedIncludes := map[string]bool{
			"include/gc.h":           true,
			"include/gc/gc_config.h": true,
		}
		if len(contents.Includes) != len(expectedIncludes) {
			t.Errorf("Includes: got %d, want %d: %v", len(contents.Includes), len(expectedIncludes), contents.Includes)
		}
		for _, inc := range contents.Includes {
			if !expectedIncludes[inc] {
				t.Errorf("unexpected include: %s", inc)
			}
		}
	})

	t.Run("symlinks included from lib and include", func(t *testing.T) {
		path := createTestBottleTarball(t, []struct {
			name     string
			body     string
			typeflag byte
		}{
			{"foo/1.0/lib/libfoo.so.1", "real lib", tar.TypeReg},
			{"foo/1.0/lib/libfoo.so", "libfoo.so.1", tar.TypeSymlink},
			{"foo/1.0/include/foo.h", "header", tar.TypeReg},
			{"foo/1.0/include/foo_compat.h", "foo.h", tar.TypeSymlink},
			// Directory entries should be skipped
			{"foo/1.0/lib/", "", tar.TypeDir},
			{"foo/1.0/include/", "", tar.TypeDir},
		})
		defer os.Remove(path)

		contents, err := b.extractBottleContents(path)
		if err != nil {
			t.Fatalf("extractBottleContents() error = %v", err)
		}

		expectedLibs := map[string]bool{
			"lib/libfoo.so.1": true,
			"lib/libfoo.so":   true,
		}
		if len(contents.LibFiles) != len(expectedLibs) {
			t.Errorf("LibFiles: got %d, want %d: %v", len(contents.LibFiles), len(expectedLibs), contents.LibFiles)
		}
		for _, l := range contents.LibFiles {
			if !expectedLibs[l] {
				t.Errorf("unexpected lib file: %s", l)
			}
		}

		expectedIncludes := map[string]bool{
			"include/foo.h":        true,
			"include/foo_compat.h": true,
		}
		if len(contents.Includes) != len(expectedIncludes) {
			t.Errorf("Includes: got %d, want %d: %v", len(contents.Includes), len(expectedIncludes), contents.Includes)
		}
		for _, inc := range contents.Includes {
			if !expectedIncludes[inc] {
				t.Errorf("unexpected include: %s", inc)
			}
		}
	})

	t.Run("non-library files in lib are excluded", func(t *testing.T) {
		path := createTestBottleTarball(t, []struct {
			name     string
			body     string
			typeflag byte
		}{
			{"pkg/1.0/lib/libpkg.so", "lib", tar.TypeReg},
			{"pkg/1.0/lib/python3.12/site-packages/foo.py", "python", tar.TypeReg},
			{"pkg/1.0/lib/ruby/gems/bar.rb", "ruby", tar.TypeReg},
			{"pkg/1.0/lib/notes.txt", "text", tar.TypeReg},
		})
		defer os.Remove(path)

		contents, err := b.extractBottleContents(path)
		if err != nil {
			t.Fatalf("extractBottleContents() error = %v", err)
		}

		if len(contents.LibFiles) != 1 {
			t.Errorf("LibFiles: got %d, want 1: %v", len(contents.LibFiles), contents.LibFiles)
		}
		if len(contents.LibFiles) == 1 && contents.LibFiles[0] != "lib/libpkg.so" {
			t.Errorf("LibFiles[0] = %q, want %q", contents.LibFiles[0], "lib/libpkg.so")
		}
	})

	t.Run("empty bottle returns empty contents", func(t *testing.T) {
		path := createTestBottleTarball(t, []struct {
			name     string
			body     string
			typeflag byte
		}{
			{"empty/1.0/share/doc/README", "readme", tar.TypeReg},
		})
		defer os.Remove(path)

		contents, err := b.extractBottleContents(path)
		if err != nil {
			t.Fatalf("extractBottleContents() error = %v", err)
		}

		if len(contents.Binaries) != 0 {
			t.Errorf("Binaries: got %d, want 0", len(contents.Binaries))
		}
		if len(contents.LibFiles) != 0 {
			t.Errorf("LibFiles: got %d, want 0", len(contents.LibFiles))
		}
		if len(contents.Includes) != 0 {
			t.Errorf("Includes: got %d, want 0", len(contents.Includes))
		}
	})
}

func TestIsLibraryFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Library extensions -- should match
		{"libfoo.so", true},
		{"libgc.so.1.5.0", true},
		{"libreadline.so.8.2", true},
		{"libfoo.so.1", true},
		{"libfoo.a", true},
		{"libfoo.dylib", true},
		{"foo.pc", true},

		// Non-library extensions -- should not match
		{"foo.py", false},
		{"bar.rb", false},
		{"notes.txt", false},
		{"foo.h", false},
		{"foo.o", false},
		{"foo.la", false},
		{"foo.cmake", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLibraryFile(tt.name); got != tt.want {
				t.Errorf("isLibraryFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMatchesVersionedSo(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Versioned .so -- should match
		{"libgc.so.1.5.0", true},
		{"libreadline.so.8.2", true},
		{"libfoo.so.1", true},
		{"libbar.so.12.3.4", true},

		// Non-versioned or unrelated -- should not match
		{"libfoo.so", false},
		{"libsomething.socket", false},
		{"config.source", false},
		{"foo.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesVersionedSo(tt.name); got != tt.want {
				t.Errorf("matchesVersionedSo(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestBottleContents_mixedBottle(t *testing.T) {
	// Validates scenario-1 from the test plan: a tarball with bin/, lib/, and
	// include/ entries returns all three fields populated correctly.
	b := &HomebrewBuilder{}

	path := createTestBottleTarball(t, []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"formula/ver/bin/tool", "binary", tar.TypeReg},
		{"formula/ver/lib/libfoo.so", "lib", tar.TypeReg},
		{"formula/ver/lib/libfoo.a", "lib", tar.TypeReg},
		{"formula/ver/lib/pkgconfig/foo.pc", "pc", tar.TypeReg},
		{"formula/ver/include/foo.h", "header", tar.TypeReg},
	})
	defer os.Remove(path)

	contents, err := b.extractBottleContents(path)
	if err != nil {
		t.Fatalf("extractBottleContents() error = %v", err)
	}

	wantBinaries := []string{"tool"}
	wantLibs := []string{"lib/libfoo.so", "lib/libfoo.a", "lib/pkgconfig/foo.pc"}
	wantIncludes := []string{"include/foo.h"}

	if len(contents.Binaries) != len(wantBinaries) {
		t.Errorf("Binaries count: got %d, want %d", len(contents.Binaries), len(wantBinaries))
	}
	if len(contents.LibFiles) != len(wantLibs) {
		t.Errorf("LibFiles count: got %d, want %d", len(contents.LibFiles), len(wantLibs))
	}
	if len(contents.Includes) != len(wantIncludes) {
		t.Errorf("Includes count: got %d, want %d", len(contents.Includes), len(wantIncludes))
	}
}

func TestHomebrewBuilder_generateDeterministicRecipe(t *testing.T) {
	b := &HomebrewBuilder{
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}

	// Test with nil formula info
	genCtx := &homebrewGenContext{
		formula:     "jq",
		formulaInfo: nil,
	}

	ctx := context.Background()
	_, err := b.generateDeterministicRecipe(ctx, "jq", genCtx)
	if err == nil {
		t.Error("expected error for nil formulaInfo")
	}

	// Test with empty version
	genCtx.formulaInfo = &homebrewFormulaInfo{
		Name: "jq",
	}
	_, err = b.generateDeterministicRecipe(ctx, "jq", genCtx)
	if err == nil {
		t.Error("expected error for empty version")
	}
}

func TestHomebrewBuilder_getCurrentPlatformTag(t *testing.T) {
	tag, err := getCurrentPlatformTag()
	if err != nil {
		t.Fatalf("getCurrentPlatformTag() error = %v", err)
	}

	validTags := map[string]bool{
		"arm64_sonoma": true,
		"sonoma":       true,
		"arm64_linux":  true,
		"x86_64_linux": true,
	}

	if !validTags[tag] {
		t.Errorf("getCurrentPlatformTag() = %q, not a valid tag", tag)
	}
}

func TestHomebrewBuilder_RequiresLLM(t *testing.T) {
	b := &HomebrewBuilder{}
	if b.RequiresLLM() {
		t.Error("RequiresLLM() = true, want false")
	}
}

func TestHomebrewSession_Generate_DeterministicOnly_ReturnsDeterministicFailedError(t *testing.T) {
	// Create a session with DeterministicOnly=true that will fail deterministic generation
	// (formula info has no stable version, causing generateDeterministic to fail)
	b := NewHomebrewBuilder(
		WithHomebrewAPIURL("http://unused"),
		WithHomebrewHTTPClient(&http.Client{Timeout: 5 * time.Second}),
	)

	session := &HomebrewSession{
		builder:           b,
		req:               BuildRequest{Package: "testpkg"},
		formula:           "testformula",
		deterministicOnly: true,
		genCtx: &homebrewGenContext{
			formula: "testformula",
			formulaInfo: &homebrewFormulaInfo{
				Name: "testformula",
				// No stable version set -> generateDeterministic will fail
			},
			httpClient: b.httpClient,
		},
	}

	ctx := context.Background()
	_, err := session.Generate(ctx)
	if err == nil {
		t.Fatal("Generate() expected error in deterministic-only mode")
	}

	var detErr *DeterministicFailedError
	if !errors.As(err, &detErr) {
		t.Fatalf("Generate() error type = %T, want *DeterministicFailedError", err)
	}

	if detErr.Formula != "testformula" {
		t.Errorf("Formula = %q, want %q", detErr.Formula, "testformula")
	}
	if detErr.Category == "" {
		t.Error("Category should not be empty")
	}
	if detErr.Message == "" {
		t.Error("Message should not be empty")
	}
	if detErr.Err == nil {
		t.Error("Err (underlying) should not be nil")
	}
}

func TestHomebrewSession_Repair_DeterministicOnly_ReturnsRepairNotSupported(t *testing.T) {
	session := &HomebrewSession{
		deterministicOnly: true,
	}

	ctx := context.Background()
	_, err := session.Repair(ctx, nil)
	if err == nil {
		t.Fatal("Repair() expected error in deterministic-only mode")
	}

	var repairErr *RepairNotSupportedError
	if !errors.As(err, &repairErr) {
		t.Fatalf("Repair() error type = %T, want *RepairNotSupportedError", err)
	}

	if repairErr.BuilderType != "homebrew-deterministic" {
		t.Errorf("BuilderType = %q, want %q", repairErr.BuilderType, "homebrew-deterministic")
	}
}

func TestDeterministicFailedError_Fields(t *testing.T) {
	underlying := fmt.Errorf("no binaries found in bottle")
	err := &DeterministicFailedError{
		Formula:  "jq",
		Category: FailureCategoryComplexArchive,
		Message:  "formula jq bottle contains no binaries in bin/",
		Err:      underlying,
	}

	// Check Error() string
	errStr := err.Error()
	if !strings.Contains(errStr, "jq") {
		t.Error("Error() should contain formula name")
	}
	if !strings.Contains(errStr, "deterministic generation failed") {
		t.Error("Error() should indicate deterministic failure")
	}

	// Check Unwrap()
	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return the underlying error")
	}

	// Check fields
	if err.Category != FailureCategoryComplexArchive {
		t.Errorf("Category = %q, want %q", err.Category, FailureCategoryComplexArchive)
	}
}

func TestHomebrewSession_classifyDeterministicFailure(t *testing.T) {
	session := &HomebrewSession{
		formula: "testpkg",
	}

	tests := []struct {
		name    string
		err     error
		wantCat DeterministicFailureCategory
	}{
		{
			name:    "no bottles",
			err:     fmt.Errorf("formula has no bottles available"),
			wantCat: FailureCategoryNoBottles,
		},
		{
			name:    "no bottle for platform",
			err:     fmt.Errorf("no bottle found for platform tag: x86_64_linux"),
			wantCat: FailureCategoryNoBottles,
		},
		{
			name:    "no binaries",
			err:     fmt.Errorf("no binaries found in bottle"),
			wantCat: FailureCategoryComplexArchive,
		},
		{
			name:    "no binaries or library files",
			err:     fmt.Errorf("no binaries or library files found in bottle"),
			wantCat: FailureCategoryComplexArchive,
		},
		{
			name:    "library recipe generation failed",
			err:     fmt.Errorf("library recipe generation failed: no bottle for any target platform"),
			wantCat: FailureCategoryComplexArchive,
		},
		{
			name:    "fetch failure",
			err:     fmt.Errorf("failed to fetch GHCR manifest: timeout"),
			wantCat: FailureCategoryAPIError,
		},
		{
			name:    "token failure",
			err:     fmt.Errorf("token request failed: connection refused"),
			wantCat: FailureCategoryAPIError,
		},
		{
			name:    "validation failure",
			err:     fmt.Errorf("sandbox validation failed"),
			wantCat: FailureCategoryValidation,
		},
		{
			name:    "unknown error",
			err:     fmt.Errorf("something unexpected"),
			wantCat: FailureCategoryAPIError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := session.classifyDeterministicFailure(tt.err)
			if result.Category != tt.wantCat {
				t.Errorf("Category = %q, want %q", result.Category, tt.wantCat)
			}
			if result.Formula != "testpkg" {
				t.Errorf("Formula = %q, want %q", result.Formula, "testpkg")
			}
			if result.Err != tt.err {
				t.Error("Err should be the original error")
			}
			if result.Message == "" {
				t.Error("Message should not be empty")
			}
		})
	}
}

func TestHomebrewSession_classifyDeterministicFailure_libraryOnlyTag(t *testing.T) {
	session := &HomebrewSession{
		formula: "bdw-gc",
	}

	result := session.classifyDeterministicFailure(
		fmt.Errorf("library recipe generation failed: %w", fmt.Errorf("no platform contents provided")),
	)

	if result.Category != FailureCategoryComplexArchive {
		t.Errorf("Category = %q, want %q", result.Category, FailureCategoryComplexArchive)
	}

	if !strings.Contains(result.Message, "[library_only]") {
		t.Errorf("Message should contain [library_only] tag, got %q", result.Message)
	}

	if !strings.Contains(result.Message, "bdw-gc") {
		t.Errorf("Message should contain formula name, got %q", result.Message)
	}
}

func TestHomebrewBuilder_generateLibraryRecipe_SinglePlatform(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "bdw-gc",
		formulaInfo: &homebrewFormulaInfo{
			Name:         "bdw-gc",
			Description:  "Garbage collector for C and C++",
			Homepage:     "https://www.hboehm.info/gc/",
			Dependencies: []string{"libatomic_ops"},
		},
	}
	genCtx.formulaInfo.Versions.Stable = "8.2.4"

	platforms := []platformContents{
		{
			OS:   "linux",
			Libc: "glibc",
			Contents: &bottleContents{
				LibFiles: []string{
					"lib/libgc.so",
					"lib/libgc.so.1",
					"lib/libgc.so.1.5.0",
					"lib/libgc.a",
					"lib/pkgconfig/bdw-gc.pc",
				},
				Includes: []string{
					"include/gc.h",
					"include/gc/gc.h",
				},
			},
		},
	}

	r, err := b.generateLibraryRecipe(context.Background(), "bdw-gc", genCtx, platforms)
	if err != nil {
		t.Fatalf("generateLibraryRecipe() error = %v", err)
	}

	// Verify metadata
	if r.Metadata.Name != "bdw-gc" {
		t.Errorf("Name = %q, want %q", r.Metadata.Name, "bdw-gc")
	}
	if r.Metadata.Type != recipe.RecipeTypeLibrary {
		t.Errorf("Type = %q, want %q", r.Metadata.Type, recipe.RecipeTypeLibrary)
	}
	if r.Metadata.Description != "Garbage collector for C and C++" {
		t.Errorf("Description = %q, want %q", r.Metadata.Description, "Garbage collector for C and C++")
	}
	if r.Metadata.Homepage != "https://www.hboehm.info/gc/" {
		t.Errorf("Homepage = %q, want %q", r.Metadata.Homepage, "https://www.hboehm.info/gc/")
	}

	// Verify version section
	if r.Version.Source != "homebrew" {
		t.Errorf("Version.Source = %q, want %q", r.Version.Source, "homebrew")
	}
	if r.Version.Formula != "bdw-gc" {
		t.Errorf("Version.Formula = %q, want %q", r.Version.Formula, "bdw-gc")
	}

	// Verify nil Verify (libraries don't have verify sections)
	if r.Verify != nil {
		t.Errorf("Verify should be nil for library recipes, got %+v", r.Verify)
	}

	// Verify runtime dependencies propagated
	if len(r.Metadata.RuntimeDependencies) != 1 || r.Metadata.RuntimeDependencies[0] != "libatomic_ops" {
		t.Errorf("RuntimeDependencies = %v, want [libatomic_ops]", r.Metadata.RuntimeDependencies)
	}

	// Verify steps: single platform should have 2 steps without when clauses
	if len(r.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(r.Steps))
	}

	// Step 0: homebrew action
	if r.Steps[0].Action != "homebrew" {
		t.Errorf("Steps[0].Action = %q, want %q", r.Steps[0].Action, "homebrew")
	}
	if formula, ok := r.Steps[0].Params["formula"].(string); !ok || formula != "bdw-gc" {
		t.Errorf("Steps[0].Params[formula] = %v, want %q", r.Steps[0].Params["formula"], "bdw-gc")
	}
	if r.Steps[0].When != nil {
		t.Errorf("Steps[0].When should be nil for single-platform, got %+v", r.Steps[0].When)
	}

	// Step 1: install_binaries action
	if r.Steps[1].Action != "install_binaries" {
		t.Errorf("Steps[1].Action = %q, want %q", r.Steps[1].Action, "install_binaries")
	}

	// Verify install_mode = "directory"
	installMode, ok := r.Steps[1].Params["install_mode"].(string)
	if !ok || installMode != "directory" {
		t.Errorf("Steps[1].Params[install_mode] = %v, want %q", r.Steps[1].Params["install_mode"], "directory")
	}

	// Verify outputs key (not binaries)
	outputs, ok := r.Steps[1].Params["outputs"].([]string)
	if !ok {
		t.Fatalf("Steps[1].Params[outputs] is not []string, got %T", r.Steps[1].Params["outputs"])
	}

	// Outputs should be LibFiles + Includes = 5 + 2 = 7
	if len(outputs) != 7 {
		t.Errorf("len(outputs) = %d, want 7", len(outputs))
	}

	// First entries should be lib files
	if outputs[0] != "lib/libgc.so" {
		t.Errorf("outputs[0] = %q, want %q", outputs[0], "lib/libgc.so")
	}

	// Last entries should be include files
	if outputs[6] != "include/gc/gc.h" {
		t.Errorf("outputs[6] = %q, want %q", outputs[6], "include/gc/gc.h")
	}

	// Verify "binaries" key is NOT used
	if _, exists := r.Steps[1].Params["binaries"]; exists {
		t.Error("Steps[1].Params should not have 'binaries' key for library recipes")
	}

	if r.Steps[1].When != nil {
		t.Errorf("Steps[1].When should be nil for single-platform, got %+v", r.Steps[1].When)
	}
}

func TestHomebrewBuilder_generateLibraryRecipe_MultiPlatform(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "tree-sitter",
		formulaInfo: &homebrewFormulaInfo{
			Name:        "tree-sitter",
			Description: "Parser generator tool",
			Homepage:    "https://tree-sitter.github.io/",
		},
	}
	genCtx.formulaInfo.Versions.Stable = "0.22.6"

	platforms := []platformContents{
		{
			OS:   "linux",
			Libc: "glibc",
			Contents: &bottleContents{
				LibFiles: []string{"lib/libtree-sitter.so", "lib/libtree-sitter.a"},
				Includes: []string{"include/tree_sitter/api.h"},
			},
		},
		{
			OS:   "darwin",
			Libc: "",
			Contents: &bottleContents{
				LibFiles: []string{"lib/libtree-sitter.dylib", "lib/libtree-sitter.a"},
				Includes: []string{"include/tree_sitter/api.h"},
			},
		},
	}

	r, err := b.generateLibraryRecipe(context.Background(), "tree-sitter", genCtx, platforms)
	if err != nil {
		t.Fatalf("generateLibraryRecipe() error = %v", err)
	}

	// Multi-platform should have 4 steps (2 per platform)
	if len(r.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(r.Steps))
	}

	// Steps 0-1: Linux platform with when clauses
	if r.Steps[0].When == nil {
		t.Fatal("Steps[0].When should not be nil for multi-platform")
	}
	if len(r.Steps[0].When.OS) != 1 || r.Steps[0].When.OS[0] != "linux" {
		t.Errorf("Steps[0].When.OS = %v, want [linux]", r.Steps[0].When.OS)
	}
	if len(r.Steps[0].When.Libc) != 1 || r.Steps[0].When.Libc[0] != "glibc" {
		t.Errorf("Steps[0].When.Libc = %v, want [glibc]", r.Steps[0].When.Libc)
	}

	// Steps 2-3: macOS platform with when clauses
	if r.Steps[2].When == nil {
		t.Fatal("Steps[2].When should not be nil for multi-platform")
	}
	if len(r.Steps[2].When.OS) != 1 || r.Steps[2].When.OS[0] != "darwin" {
		t.Errorf("Steps[2].When.OS = %v, want [darwin]", r.Steps[2].When.OS)
	}
	if len(r.Steps[2].When.Libc) != 0 {
		t.Errorf("Steps[2].When.Libc = %v, want empty (macOS)", r.Steps[2].When.Libc)
	}
}

func TestHomebrewBuilder_generateToolRecipe_Regression(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "jq",
		formulaInfo: &homebrewFormulaInfo{
			Name:         "jq",
			Description:  "Lightweight and flexible command-line JSON processor",
			Homepage:     "https://jqlang.github.io/jq/",
			Dependencies: []string{"oniguruma"},
		},
	}
	genCtx.formulaInfo.Versions.Stable = "1.7.1"

	binaries := []string{"jq"}

	r, err := b.generateToolRecipe("jq", genCtx, binaries)
	if err != nil {
		t.Fatalf("generateToolRecipe() error = %v", err)
	}

	// Should be a tool recipe (no type set)
	if r.Metadata.Type != "" {
		t.Errorf("Type = %q, want empty for tool recipes", r.Metadata.Type)
	}

	// Should have a verify section
	if r.Verify == nil {
		t.Fatal("Verify should not be nil for tool recipes")
	}
	if r.Verify.Command != "jq --version" {
		t.Errorf("Verify.Command = %q, want %q", r.Verify.Command, "jq --version")
	}

	// Should have 2 steps
	if len(r.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(r.Steps))
	}
	if r.Steps[0].Action != "homebrew" {
		t.Errorf("Steps[0].Action = %q, want %q", r.Steps[0].Action, "homebrew")
	}
	if r.Steps[1].Action != "install_binaries" {
		t.Errorf("Steps[1].Action = %q, want %q", r.Steps[1].Action, "install_binaries")
	}

	// Should use "binaries" key (not "outputs") for tool recipes
	if _, exists := r.Steps[1].Params["binaries"]; !exists {
		t.Error("Steps[1].Params should have 'binaries' key for tool recipes")
	}
	if _, exists := r.Steps[1].Params["outputs"]; exists {
		t.Error("Steps[1].Params should NOT have 'outputs' key for tool recipes")
	}

	// Binaries should be prefixed with bin/
	bins, ok := r.Steps[1].Params["binaries"].([]string)
	if !ok {
		t.Fatalf("Steps[1].Params[binaries] is not []string, got %T", r.Steps[1].Params["binaries"])
	}
	if len(bins) != 1 || bins[0] != "bin/jq" {
		t.Errorf("binaries = %v, want [bin/jq]", bins)
	}

	// Runtime dependencies propagated
	if len(r.Metadata.RuntimeDependencies) != 1 || r.Metadata.RuntimeDependencies[0] != "oniguruma" {
		t.Errorf("RuntimeDependencies = %v, want [oniguruma]", r.Metadata.RuntimeDependencies)
	}

	// Version source should be empty (inferred from homebrew action)
	if r.Version.Source != "" {
		t.Errorf("Version.Source = %q, want empty (inferred from action)", r.Version.Source)
	}
	if r.Version.Formula != "" {
		t.Errorf("Version.Formula = %q, want empty (inferred from action)", r.Version.Formula)
	}
}

func TestHomebrewBuilder_generateDeterministicRecipe_ErrorMessages(t *testing.T) {
	b := &HomebrewBuilder{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	ctx := context.Background()

	// Test nil formula info
	genCtx := &homebrewGenContext{
		formula:     "test",
		formulaInfo: nil,
	}
	_, err := b.generateDeterministicRecipe(ctx, "test", genCtx)
	if err == nil || !strings.Contains(err.Error(), "formula info not available") {
		t.Errorf("expected 'formula info not available' error, got: %v", err)
	}

	// Test empty version
	genCtx.formulaInfo = &homebrewFormulaInfo{Name: "test"}
	_, err = b.generateDeterministicRecipe(ctx, "test", genCtx)
	if err == nil || !strings.Contains(err.Error(), "no stable version") {
		t.Errorf("expected 'no stable version' error, got: %v", err)
	}
}

// createBottleTarballBytes creates an in-memory tar.gz archive suitable for use
// as a mock GHCR bottle blob. Returns the raw bytes.
func createBottleTarballBytes(t *testing.T, entries []struct {
	name     string
	body     string
	typeflag byte
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0755,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
		}
		if e.typeflag == tar.TypeSymlink {
			hdr.Linkname = e.body
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if e.typeflag != tar.TypeSymlink {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("failed to write tar content: %v", err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

// computeSHA256 returns the hex-encoded SHA256 of the given data.
func computeSHA256(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// newMockGHCRBottleServer creates a test server that serves bottle tarballs for
// scanMultiplePlatforms tests. The platformBlobs map keys are platform tags
// (e.g., "x86_64_linux"); values are raw tar.gz bytes.
func newMockGHCRBottleServer(t *testing.T, formula, version string, platformBlobs map[string][]byte) (*httptest.Server, *http.Client) {
	t.Helper()

	// Pre-compute SHAs for each platform blob
	platformSHAs := make(map[string]string)
	for tag, blob := range platformBlobs {
		platformSHAs[tag] = computeSHA256(blob)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token endpoint
		if strings.HasPrefix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"token": "test-token"}`))
			return
		}

		// Manifest endpoint
		if strings.Contains(r.URL.Path, "/manifests/") {
			var entries []string
			for tag, sha := range platformSHAs {
				refName := fmt.Sprintf("%s.%s", version, tag)
				entries = append(entries, fmt.Sprintf(
					`{"digest": "sha256:dummy", "annotations": {"org.opencontainers.image.ref.name": %q, "sh.brew.bottle.digest": "sha256:%s"}}`,
					refName, sha,
				))
			}
			manifest := fmt.Sprintf(`{"manifests": [%s]}`, strings.Join(entries, ","))
			_, _ = w.Write([]byte(manifest))
			return
		}

		// Blob download endpoint: /v2/homebrew/core/{formula}/blobs/sha256:{sha}
		if strings.Contains(r.URL.Path, "/blobs/sha256:") {
			parts := strings.SplitAfter(r.URL.Path, "/blobs/sha256:")
			if len(parts) == 2 {
				requestedSHA := parts[1]
				for tag, sha := range platformSHAs {
					if sha == requestedSHA {
						w.Header().Set("Content-Type", "application/octet-stream")
						_, _ = w.Write(platformBlobs[tag])
						return
					}
				}
			}
			w.WriteHeader(404)
			return
		}

		http.NotFound(w, r)
	}))

	client := server.Client()
	client.Transport = &mockGHCRTransport{
		serverURL: server.URL,
		base:      client.Transport,
	}

	return server, client
}

func TestHomebrewBuilder_scanMultiplePlatforms_BothPlatforms(t *testing.T) {
	linuxEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"testlib/1.0.0/lib/libtest.so", "lib", tar.TypeReg},
		{"testlib/1.0.0/lib/libtest.a", "lib", tar.TypeReg},
		{"testlib/1.0.0/include/test.h", "header", tar.TypeReg},
	}
	macEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"testlib/1.0.0/lib/libtest.dylib", "lib", tar.TypeReg},
		{"testlib/1.0.0/lib/libtest.a", "lib", tar.TypeReg},
		{"testlib/1.0.0/include/test.h", "header", tar.TypeReg},
	}

	linuxBlob := createBottleTarballBytes(t, linuxEntries)
	macBlob := createBottleTarballBytes(t, macEntries)

	server, client := newMockGHCRBottleServer(t, "testlib", "1.0.0", map[string][]byte{
		"x86_64_linux": linuxBlob,
		"arm64_sonoma": macBlob,
	})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	info := &homebrewFormulaInfo{Name: "testlib"}
	info.Versions.Stable = "1.0.0"

	// Provide current platform's contents (will be used instead of re-downloading)
	currentTag, _ := getCurrentPlatformTag()
	currentOS, _ := platformTagToOSLibc(currentTag)

	var currentContents *bottleContents
	if currentOS == "linux" {
		currentContents = &bottleContents{
			LibFiles: []string{"lib/libtest.so", "lib/libtest.a"},
			Includes: []string{"include/test.h"},
		}
	} else {
		currentContents = &bottleContents{
			LibFiles: []string{"lib/libtest.dylib", "lib/libtest.a"},
			Includes: []string{"include/test.h"},
		}
	}

	ctx := context.Background()
	platforms := b.scanMultiplePlatforms(ctx, info, currentContents)

	// Should have 2 platforms
	if len(platforms) != 2 {
		t.Fatalf("len(platforms) = %d, want 2", len(platforms))
	}

	// Linux should be first
	if platforms[0].OS != "linux" {
		t.Errorf("platforms[0].OS = %q, want %q", platforms[0].OS, "linux")
	}
	if platforms[0].Libc != "glibc" {
		t.Errorf("platforms[0].Libc = %q, want %q", platforms[0].Libc, "glibc")
	}
	if platforms[0].Contents == nil {
		t.Fatal("platforms[0].Contents should not be nil")
	}

	// macOS should be second
	if platforms[1].OS != "darwin" {
		t.Errorf("platforms[1].OS = %q, want %q", platforms[1].OS, "darwin")
	}
	if platforms[1].Libc != "" {
		t.Errorf("platforms[1].Libc = %q, want empty", platforms[1].Libc)
	}
	if platforms[1].Contents == nil {
		t.Fatal("platforms[1].Contents should not be nil")
	}

	// Current platform should use the provided contents (identity check)
	if currentOS == "linux" {
		if platforms[0].Contents != currentContents {
			t.Error("Linux platform should reuse the provided currentContents pointer")
		}
	} else {
		if platforms[1].Contents != currentContents {
			t.Error("macOS platform should reuse the provided currentContents pointer")
		}
	}
}

func TestHomebrewBuilder_scanMultiplePlatforms_OnePlatformMissing(t *testing.T) {
	// Only Linux bottle available; macOS bottle missing
	linuxEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"testlib/1.0.0/lib/libtest.so", "lib", tar.TypeReg},
		{"testlib/1.0.0/include/test.h", "header", tar.TypeReg},
	}
	linuxBlob := createBottleTarballBytes(t, linuxEntries)

	server, client := newMockGHCRBottleServer(t, "testlib", "1.0.0", map[string][]byte{
		"x86_64_linux": linuxBlob,
		// arm64_sonoma intentionally omitted
	})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	info := &homebrewFormulaInfo{Name: "testlib"}
	info.Versions.Stable = "1.0.0"

	currentTag, _ := getCurrentPlatformTag()
	currentOS, _ := platformTagToOSLibc(currentTag)

	currentContents := &bottleContents{
		LibFiles: []string{"lib/libtest.so"},
		Includes: []string{"include/test.h"},
	}

	ctx := context.Background()
	platforms := b.scanMultiplePlatforms(ctx, info, currentContents)

	if currentOS == "linux" {
		// Current platform is Linux, macOS missing. Should get 1 platform.
		if len(platforms) != 1 {
			t.Fatalf("len(platforms) = %d, want 1 (macOS missing)", len(platforms))
		}
		if platforms[0].OS != "linux" {
			t.Errorf("platforms[0].OS = %q, want %q", platforms[0].OS, "linux")
		}
	} else {
		// Current platform is macOS but Linux bottle is available.
		// macOS uses currentContents; Linux downloads from mock.
		if len(platforms) != 2 {
			t.Fatalf("len(platforms) = %d, want 2", len(platforms))
		}
		if platforms[0].OS != "linux" {
			t.Errorf("platforms[0].OS = %q, want %q", platforms[0].OS, "linux")
		}
		if platforms[1].OS != "darwin" {
			t.Errorf("platforms[1].OS = %q, want %q", platforms[1].OS, "darwin")
		}
	}
}

func TestHomebrewBuilder_scanMultiplePlatforms_NeitherPlatformAvailable(t *testing.T) {
	// No bottles available at all (empty blobs map)
	server, client := newMockGHCRBottleServer(t, "testlib", "1.0.0", map[string][]byte{})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	info := &homebrewFormulaInfo{Name: "testlib"}
	info.Versions.Stable = "1.0.0"

	currentContents := &bottleContents{
		LibFiles: []string{"lib/libtest.so"},
	}

	ctx := context.Background()
	platforms := b.scanMultiplePlatforms(ctx, info, currentContents)

	// The current platform's contents are only included if its tag matches
	// one of the two targets. Since neither target has a bottle, the mock
	// server returns a manifest with no entries. But the current platform
	// is matched by OS/libc, not by download success, so it's always
	// included when it matches a target.
	currentTag, _ := getCurrentPlatformTag()
	currentOS, _ := platformTagToOSLibc(currentTag)

	foundCurrent := false
	for _, p := range platforms {
		if p.OS == currentOS {
			foundCurrent = true
		}
	}

	// The current platform should be included (it uses currentContents,
	// not a download) -- as long as it matches one of the two targets.
	isLinux := currentOS == "linux"
	isDarwin := currentOS == "darwin"
	if (isLinux || isDarwin) && !foundCurrent {
		t.Errorf("expected current platform (%s) to be included even when downloads fail", currentOS)
	}

	// The other platform should NOT be included (download failed)
	if isLinux && len(platforms) != 1 {
		t.Errorf("len(platforms) = %d, want 1 (only Linux from currentContents)", len(platforms))
	}
	if isDarwin && len(platforms) != 1 {
		t.Errorf("len(platforms) = %d, want 1 (only macOS from currentContents)", len(platforms))
	}
}

func TestHomebrewBuilder_scanMultiplePlatforms_CurrentPlatformReused(t *testing.T) {
	// Verify the current platform's contents pointer is reused, not re-downloaded.
	linuxEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"testlib/1.0.0/lib/libtest.so", "lib", tar.TypeReg},
	}
	macEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"testlib/1.0.0/lib/libtest.dylib", "lib", tar.TypeReg},
	}

	linuxBlob := createBottleTarballBytes(t, linuxEntries)
	macBlob := createBottleTarballBytes(t, macEntries)

	server, client := newMockGHCRBottleServer(t, "testlib", "1.0.0", map[string][]byte{
		"x86_64_linux": linuxBlob,
		"arm64_sonoma": macBlob,
	})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	info := &homebrewFormulaInfo{Name: "testlib"}
	info.Versions.Stable = "1.0.0"

	// Create a distinctive currentContents to verify identity
	currentContents := &bottleContents{
		LibFiles: []string{"lib/MARKER_FILE.so"},
		Includes: []string{"include/MARKER.h"},
	}

	ctx := context.Background()
	platforms := b.scanMultiplePlatforms(ctx, info, currentContents)

	currentTag, _ := getCurrentPlatformTag()
	currentOS, _ := platformTagToOSLibc(currentTag)

	// Find the platform matching the current OS and verify it uses
	// the exact same pointer (not a re-downloaded copy)
	for _, p := range platforms {
		if p.OS == currentOS {
			if p.Contents != currentContents {
				t.Error("current platform should reuse the provided currentContents pointer (identity)")
			}
			// Verify it has the marker files (not re-scanned from tarball)
			if len(p.Contents.LibFiles) != 1 || p.Contents.LibFiles[0] != "lib/MARKER_FILE.so" {
				t.Errorf("current platform LibFiles = %v, expected marker file", p.Contents.LibFiles)
			}
		}
	}
}

func TestHomebrewBuilder_generateLibraryRecipe_MultiPlatformStepOrdering(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "testlib",
		formulaInfo: &homebrewFormulaInfo{
			Name:        "testlib",
			Description: "A test library",
			Homepage:    "https://example.com",
		},
	}
	genCtx.formulaInfo.Versions.Stable = "1.0.0"

	platforms := []platformContents{
		{
			OS:   "linux",
			Libc: "glibc",
			Contents: &bottleContents{
				LibFiles: []string{"lib/libtest.so", "lib/libtest.a", "lib/pkgconfig/test.pc"},
				Includes: []string{"include/test.h"},
			},
		},
		{
			OS:   "darwin",
			Libc: "",
			Contents: &bottleContents{
				LibFiles: []string{"lib/libtest.dylib", "lib/libtest.a", "lib/pkgconfig/test.pc"},
				Includes: []string{"include/test.h"},
			},
		},
	}

	r, err := b.generateLibraryRecipe(context.Background(), "testlib", genCtx, platforms)
	if err != nil {
		t.Fatalf("generateLibraryRecipe() error = %v", err)
	}

	// 4 steps: 2 per platform
	if len(r.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(r.Steps))
	}

	// Step 0: Linux homebrew
	if r.Steps[0].Action != "homebrew" {
		t.Errorf("Steps[0].Action = %q, want %q", r.Steps[0].Action, "homebrew")
	}
	if r.Steps[0].When == nil || len(r.Steps[0].When.OS) != 1 || r.Steps[0].When.OS[0] != "linux" {
		t.Errorf("Steps[0].When.OS = %v, want [linux]", safeWhenOS(r.Steps[0].When))
	}
	if r.Steps[0].When == nil || len(r.Steps[0].When.Libc) != 1 || r.Steps[0].When.Libc[0] != "glibc" {
		t.Errorf("Steps[0].When.Libc = %v, want [glibc]", safeWhenLibc(r.Steps[0].When))
	}

	// Step 1: Linux install_binaries
	if r.Steps[1].Action != "install_binaries" {
		t.Errorf("Steps[1].Action = %q, want %q", r.Steps[1].Action, "install_binaries")
	}
	if r.Steps[1].When == nil || r.Steps[1].When.OS[0] != "linux" {
		t.Errorf("Steps[1].When.OS = %v, want [linux]", safeWhenOS(r.Steps[1].When))
	}
	linuxOutputs, ok := r.Steps[1].Params["outputs"].([]string)
	if !ok {
		t.Fatalf("Steps[1].Params[outputs] type = %T, want []string", r.Steps[1].Params["outputs"])
	}
	// Linux outputs: 3 lib files + 1 include = 4
	if len(linuxOutputs) != 4 {
		t.Errorf("Linux outputs count = %d, want 4: %v", len(linuxOutputs), linuxOutputs)
	}
	if linuxOutputs[0] != "lib/libtest.so" {
		t.Errorf("Linux outputs[0] = %q, want %q", linuxOutputs[0], "lib/libtest.so")
	}

	// Step 2: macOS homebrew
	if r.Steps[2].Action != "homebrew" {
		t.Errorf("Steps[2].Action = %q, want %q", r.Steps[2].Action, "homebrew")
	}
	if r.Steps[2].When == nil || r.Steps[2].When.OS[0] != "darwin" {
		t.Errorf("Steps[2].When.OS = %v, want [darwin]", safeWhenOS(r.Steps[2].When))
	}
	// macOS: no Libc field
	if r.Steps[2].When != nil && len(r.Steps[2].When.Libc) != 0 {
		t.Errorf("Steps[2].When.Libc = %v, want empty for macOS", r.Steps[2].When.Libc)
	}

	// Step 3: macOS install_binaries
	if r.Steps[3].Action != "install_binaries" {
		t.Errorf("Steps[3].Action = %q, want %q", r.Steps[3].Action, "install_binaries")
	}
	macOutputs, ok := r.Steps[3].Params["outputs"].([]string)
	if !ok {
		t.Fatalf("Steps[3].Params[outputs] type = %T, want []string", r.Steps[3].Params["outputs"])
	}
	// macOS outputs: 3 lib files + 1 include = 4
	if len(macOutputs) != 4 {
		t.Errorf("macOS outputs count = %d, want 4: %v", len(macOutputs), macOutputs)
	}
	if macOutputs[0] != "lib/libtest.dylib" {
		t.Errorf("macOS outputs[0] = %q, want %q", macOutputs[0], "lib/libtest.dylib")
	}

	// Verify install_mode = "directory" on both install_binaries steps
	for _, i := range []int{1, 3} {
		mode, ok := r.Steps[i].Params["install_mode"].(string)
		if !ok || mode != "directory" {
			t.Errorf("Steps[%d].Params[install_mode] = %v, want %q", i, r.Steps[i].Params["install_mode"], "directory")
		}
	}

	// Verify homebrew and install_binaries steps share the same when clause
	for base := 0; base < 4; base += 2 {
		hw := r.Steps[base].When
		iw := r.Steps[base+1].When
		if hw == nil || iw == nil {
			t.Errorf("Steps[%d] and Steps[%d] should both have when clauses", base, base+1)
			continue
		}
		if hw.OS[0] != iw.OS[0] {
			t.Errorf("Steps[%d].When.OS = %v, Steps[%d].When.OS = %v (should match)", base, hw.OS, base+1, iw.OS)
		}
	}
}

func TestHomebrewBuilder_generateLibraryRecipe_SinglePlatformFallback(t *testing.T) {
	// When only one platform is available, the recipe should have 2 steps
	// without when clauses (simple format).
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "onlylinux",
		formulaInfo: &homebrewFormulaInfo{
			Name:        "onlylinux",
			Description: "A Linux-only library",
			Homepage:    "https://example.com",
		},
	}
	genCtx.formulaInfo.Versions.Stable = "1.0.0"

	platforms := []platformContents{
		{
			OS:   "linux",
			Libc: "glibc",
			Contents: &bottleContents{
				LibFiles: []string{"lib/libonly.so"},
				Includes: []string{"include/only.h"},
			},
		},
	}

	r, err := b.generateLibraryRecipe(context.Background(), "onlylinux", genCtx, platforms)
	if err != nil {
		t.Fatalf("generateLibraryRecipe() error = %v", err)
	}

	// Single platform: 2 steps, no when clauses
	if len(r.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(r.Steps))
	}
	if r.Steps[0].When != nil {
		t.Error("Steps[0].When should be nil for single-platform")
	}
	if r.Steps[1].When != nil {
		t.Error("Steps[1].When should be nil for single-platform")
	}
}

// safeWhenOS returns the OS slice from a WhenClause, or nil if the clause is nil.
func safeWhenOS(w *recipe.WhenClause) []string {
	if w == nil {
		return nil
	}
	return w.OS
}

// safeWhenLibc returns the Libc slice from a WhenClause, or nil if the clause is nil.
func safeWhenLibc(w *recipe.WhenClause) []string {
	if w == nil {
		return nil
	}
	return w.Libc
}

// --- End-to-end pipeline validation tests (scenarios 12, 13, 14) ---
// These tests exercise the full generateDeterministicRecipe -> WriteRecipe
// path using mock GHCR servers with realistic bottle contents, validating
// the generated TOML matches expected library recipe structure.

// TestEndToEnd_LibraryRecipeGeneration_BdwGC validates the full pipeline
// for bdw-gc, a C garbage collector that ships only library files.
// Corresponds to scenario-12 in the test plan.
func TestEndToEnd_LibraryRecipeGeneration_BdwGC(t *testing.T) {
	// Build realistic Linux bottle entries for bdw-gc
	linuxEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"bdw-gc/8.2.4/lib/libgc.so", "ELF", tar.TypeSymlink},
		{"bdw-gc/8.2.4/lib/libgc.so.1", "ELF", tar.TypeSymlink},
		{"bdw-gc/8.2.4/lib/libgc.so.1.5.0", "ELF binary", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/libgccpp.so", "ELF", tar.TypeSymlink},
		{"bdw-gc/8.2.4/lib/libgccpp.so.1", "ELF", tar.TypeSymlink},
		{"bdw-gc/8.2.4/lib/libgccpp.so.1.5.0", "ELF binary", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/libgc.a", "archive", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/libgccpp.a", "archive", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/pkgconfig/bdw-gc.pc", "pkg-config", tar.TypeReg},
		{"bdw-gc/8.2.4/include/gc.h", "header", tar.TypeReg},
		{"bdw-gc/8.2.4/include/gc/gc.h", "header", tar.TypeReg},
		{"bdw-gc/8.2.4/include/gc/gc_allocator.h", "header", tar.TypeReg},
	}

	// Build realistic macOS bottle entries for bdw-gc
	macEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"bdw-gc/8.2.4/lib/libgc.dylib", "Mach-O", tar.TypeSymlink},
		{"bdw-gc/8.2.4/lib/libgc.1.dylib", "Mach-O binary", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/libgccpp.dylib", "Mach-O", tar.TypeSymlink},
		{"bdw-gc/8.2.4/lib/libgccpp.1.dylib", "Mach-O binary", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/libgc.a", "archive", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/libgccpp.a", "archive", tar.TypeReg},
		{"bdw-gc/8.2.4/lib/pkgconfig/bdw-gc.pc", "pkg-config", tar.TypeReg},
		{"bdw-gc/8.2.4/include/gc.h", "header", tar.TypeReg},
		{"bdw-gc/8.2.4/include/gc/gc.h", "header", tar.TypeReg},
		{"bdw-gc/8.2.4/include/gc/gc_allocator.h", "header", tar.TypeReg},
	}

	linuxBlob := createBottleTarballBytes(t, linuxEntries)
	macBlob := createBottleTarballBytes(t, macEntries)

	server, client := newMockGHCRBottleServer(t, "bdw-gc", "8.2.4", map[string][]byte{
		"x86_64_linux": linuxBlob,
		"arm64_sonoma": macBlob,
	})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	genCtx := &homebrewGenContext{
		formula: "bdw-gc",
		formulaInfo: &homebrewFormulaInfo{
			Name:         "bdw-gc",
			Description:  "Garbage collector for C and C++",
			Homepage:     "https://www.hboehm.info/gc/",
			Dependencies: []string{"libatomic_ops"},
		},
	}
	genCtx.formulaInfo.Versions.Stable = "8.2.4"

	ctx := context.Background()
	r, err := b.generateDeterministicRecipe(ctx, "bdw-gc", genCtx)
	if err != nil {
		t.Fatalf("generateDeterministicRecipe() error = %v", err)
	}

	// Validate recipe struct fields
	if r.Metadata.Type != recipe.RecipeTypeLibrary {
		t.Errorf("Metadata.Type = %q, want %q", r.Metadata.Type, recipe.RecipeTypeLibrary)
	}
	if r.Metadata.Name != "bdw-gc" {
		t.Errorf("Metadata.Name = %q, want %q", r.Metadata.Name, "bdw-gc")
	}
	if r.Verify != nil {
		t.Errorf("Verify should be nil for library recipes, got %+v", r.Verify)
	}
	if r.Version.Source != "homebrew" {
		t.Errorf("Version.Source = %q, want %q", r.Version.Source, "homebrew")
	}
	if len(r.Metadata.RuntimeDependencies) != 1 || r.Metadata.RuntimeDependencies[0] != "libatomic_ops" {
		t.Errorf("RuntimeDependencies = %v, want [libatomic_ops]", r.Metadata.RuntimeDependencies)
	}

	// Expect 4 steps (2 per platform) with when clauses
	if len(r.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(r.Steps))
	}

	// Validate step structure: paired homebrew + install_binaries per platform
	for base := 0; base < 4; base += 2 {
		if r.Steps[base].Action != "homebrew" {
			t.Errorf("Steps[%d].Action = %q, want %q", base, r.Steps[base].Action, "homebrew")
		}
		if r.Steps[base+1].Action != "install_binaries" {
			t.Errorf("Steps[%d].Action = %q, want %q", base+1, r.Steps[base+1].Action, "install_binaries")
		}

		// install_mode = "directory"
		mode, ok := r.Steps[base+1].Params["install_mode"].(string)
		if !ok || mode != "directory" {
			t.Errorf("Steps[%d].Params[install_mode] = %v, want %q", base+1, r.Steps[base+1].Params["install_mode"], "directory")
		}

		// Uses outputs (not binaries)
		if _, exists := r.Steps[base+1].Params["binaries"]; exists {
			t.Errorf("Steps[%d] should not have deprecated 'binaries' key", base+1)
		}
		outputs, ok := r.Steps[base+1].Params["outputs"].([]string)
		if !ok || len(outputs) == 0 {
			t.Errorf("Steps[%d].Params[outputs] should be non-empty []string", base+1)
		}

		// When clause present
		if r.Steps[base].When == nil {
			t.Errorf("Steps[%d].When should not be nil for multi-platform recipe", base)
		}
		if r.Steps[base+1].When == nil {
			t.Errorf("Steps[%d].When should not be nil for multi-platform recipe", base+1)
		}
	}

	// Validate platform ordering and content: Linux first, macOS second
	if r.Steps[0].When.OS[0] != "linux" {
		t.Errorf("Steps[0].When.OS = %v, want [linux]", r.Steps[0].When.OS)
	}
	if len(r.Steps[0].When.Libc) != 1 || r.Steps[0].When.Libc[0] != "glibc" {
		t.Errorf("Steps[0].When.Libc = %v, want [glibc]", r.Steps[0].When.Libc)
	}
	if r.Steps[2].When.OS[0] != "darwin" {
		t.Errorf("Steps[2].When.OS = %v, want [darwin]", r.Steps[2].When.OS)
	}
	if len(r.Steps[2].When.Libc) != 0 {
		t.Errorf("Steps[2].When.Libc = %v, want empty for macOS", r.Steps[2].When.Libc)
	}

	// Validate Linux outputs contain .so and .a files, headers
	linuxOutputs, _ := r.Steps[1].Params["outputs"].([]string)
	hasSO, hasA, hasPC, hasHeader := false, false, false, false
	for _, o := range linuxOutputs {
		if strings.HasSuffix(o, ".so") || strings.Contains(o, ".so.") {
			hasSO = true
		}
		if strings.HasSuffix(o, ".a") {
			hasA = true
		}
		if strings.HasSuffix(o, ".pc") {
			hasPC = true
		}
		if strings.HasPrefix(o, "include/") {
			hasHeader = true
		}
	}
	if !hasSO {
		t.Errorf("Linux outputs missing .so files: %v", linuxOutputs)
	}
	if !hasA {
		t.Errorf("Linux outputs missing .a files: %v", linuxOutputs)
	}
	if !hasPC {
		t.Errorf("Linux outputs missing .pc files: %v", linuxOutputs)
	}
	if !hasHeader {
		t.Errorf("Linux outputs missing include/ headers: %v", linuxOutputs)
	}

	// Validate macOS outputs contain .dylib and .a files, headers
	macOutputs, _ := r.Steps[3].Params["outputs"].([]string)
	hasDylib, hasA, hasPC, hasHeader := false, false, false, false
	for _, o := range macOutputs {
		if strings.HasSuffix(o, ".dylib") {
			hasDylib = true
		}
		if strings.HasSuffix(o, ".a") {
			hasA = true
		}
		if strings.HasSuffix(o, ".pc") {
			hasPC = true
		}
		if strings.HasPrefix(o, "include/") {
			hasHeader = true
		}
	}
	if !hasDylib {
		t.Errorf("macOS outputs missing .dylib files: %v", macOutputs)
	}
	if !hasA {
		t.Errorf("macOS outputs missing .a files: %v", macOutputs)
	}
	if !hasPC {
		t.Errorf("macOS outputs missing .pc files: %v", macOutputs)
	}
	if !hasHeader {
		t.Errorf("macOS outputs missing include/ headers: %v", macOutputs)
	}

	// Serialize to TOML and validate the output file structure
	tmpDir := t.TempDir()
	tomlPath := tmpDir + "/bdw-gc.toml"
	if err := recipe.WriteRecipe(r, tomlPath); err != nil {
		t.Fatalf("WriteRecipe() error = %v", err)
	}

	tomlBytes, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("failed to read generated TOML: %v", err)
	}
	tomlStr := string(tomlBytes)

	// Validate TOML structure matches existing library recipes like gmp.toml
	if !strings.Contains(tomlStr, `type = "library"`) {
		t.Error("generated TOML missing type = \"library\"")
	}
	if !strings.Contains(tomlStr, `install_mode = "directory"`) {
		t.Error("generated TOML missing install_mode = \"directory\"")
	}
	if strings.Contains(tomlStr, "binaries =") {
		t.Error("generated TOML should not contain deprecated 'binaries' key")
	}
	if !strings.Contains(tomlStr, "outputs =") {
		t.Error("generated TOML missing 'outputs' key")
	}
	if strings.Contains(tomlStr, "[verify]") {
		t.Error("generated TOML should not contain [verify] section")
	}
	if !strings.Contains(tomlStr, "when") {
		t.Error("generated TOML missing platform-conditional 'when' clauses")
	}
}

// TestEndToEnd_LibraryRecipeGeneration_TreeSitter validates the full pipeline
// for tree-sitter, a parser generator that ships only library files.
// Corresponds to scenario-13 in the test plan.
func TestEndToEnd_LibraryRecipeGeneration_TreeSitter(t *testing.T) {
	// Build realistic Linux bottle entries for tree-sitter
	linuxEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"tree-sitter/0.22.6/lib/libtree-sitter.so", "ELF", tar.TypeSymlink},
		{"tree-sitter/0.22.6/lib/libtree-sitter.so.0", "ELF", tar.TypeSymlink},
		{"tree-sitter/0.22.6/lib/libtree-sitter.so.0.22.6", "ELF binary", tar.TypeReg},
		{"tree-sitter/0.22.6/lib/libtree-sitter.a", "archive", tar.TypeReg},
		{"tree-sitter/0.22.6/lib/pkgconfig/tree-sitter.pc", "pkg-config", tar.TypeReg},
		{"tree-sitter/0.22.6/include/tree_sitter/api.h", "header", tar.TypeReg},
		{"tree-sitter/0.22.6/include/tree_sitter/parser.h", "header", tar.TypeReg},
	}

	// Build realistic macOS bottle entries for tree-sitter
	macEntries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"tree-sitter/0.22.6/lib/libtree-sitter.dylib", "Mach-O", tar.TypeSymlink},
		{"tree-sitter/0.22.6/lib/libtree-sitter.0.dylib", "Mach-O binary", tar.TypeReg},
		{"tree-sitter/0.22.6/lib/libtree-sitter.a", "archive", tar.TypeReg},
		{"tree-sitter/0.22.6/lib/pkgconfig/tree-sitter.pc", "pkg-config", tar.TypeReg},
		{"tree-sitter/0.22.6/include/tree_sitter/api.h", "header", tar.TypeReg},
		{"tree-sitter/0.22.6/include/tree_sitter/parser.h", "header", tar.TypeReg},
	}

	linuxBlob := createBottleTarballBytes(t, linuxEntries)
	macBlob := createBottleTarballBytes(t, macEntries)

	server, client := newMockGHCRBottleServer(t, "tree-sitter", "0.22.6", map[string][]byte{
		"x86_64_linux": linuxBlob,
		"arm64_sonoma": macBlob,
	})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	genCtx := &homebrewGenContext{
		formula: "tree-sitter",
		formulaInfo: &homebrewFormulaInfo{
			Name:        "tree-sitter",
			Description: "An incremental parsing system for programming tools",
			Homepage:    "https://tree-sitter.github.io/tree-sitter/",
		},
	}
	genCtx.formulaInfo.Versions.Stable = "0.22.6"

	ctx := context.Background()
	r, err := b.generateDeterministicRecipe(ctx, "tree-sitter", genCtx)
	if err != nil {
		t.Fatalf("generateDeterministicRecipe() error = %v", err)
	}

	// Validate recipe struct
	if r.Metadata.Type != recipe.RecipeTypeLibrary {
		t.Errorf("Metadata.Type = %q, want %q", r.Metadata.Type, recipe.RecipeTypeLibrary)
	}
	if r.Metadata.Name != "tree-sitter" {
		t.Errorf("Metadata.Name = %q, want %q", r.Metadata.Name, "tree-sitter")
	}
	if r.Verify != nil {
		t.Errorf("Verify should be nil for library recipes, got %+v", r.Verify)
	}

	// 4 steps with when clauses
	if len(r.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(r.Steps))
	}

	// Check all install_binaries steps use outputs and directory mode
	for i := 1; i < 4; i += 2 {
		if r.Steps[i].Action != "install_binaries" {
			t.Errorf("Steps[%d].Action = %q, want %q", i, r.Steps[i].Action, "install_binaries")
		}
		mode, ok := r.Steps[i].Params["install_mode"].(string)
		if !ok || mode != "directory" {
			t.Errorf("Steps[%d].Params[install_mode] = %v, want %q", i, r.Steps[i].Params["install_mode"], "directory")
		}
		if _, exists := r.Steps[i].Params["binaries"]; exists {
			t.Errorf("Steps[%d] should not have deprecated 'binaries' key", i)
		}
		outputs, ok := r.Steps[i].Params["outputs"].([]string)
		if !ok || len(outputs) == 0 {
			t.Fatalf("Steps[%d].Params[outputs] should be non-empty []string", i)
		}

		// Verify tree-sitter-specific files appear
		foundTreeSitter := false
		for _, o := range outputs {
			if strings.Contains(o, "libtree-sitter") {
				foundTreeSitter = true
				break
			}
		}
		if !foundTreeSitter {
			t.Errorf("Steps[%d] outputs missing libtree-sitter files: %v", i, outputs)
		}

		// Verify headers are included
		foundHeader := false
		for _, o := range outputs {
			if strings.Contains(o, "tree_sitter/api.h") {
				foundHeader = true
				break
			}
		}
		if !foundHeader {
			t.Errorf("Steps[%d] outputs missing tree_sitter headers: %v", i, outputs)
		}
	}

	// Serialize and validate TOML
	tmpDir := t.TempDir()
	tomlPath := tmpDir + "/tree-sitter.toml"
	if err := recipe.WriteRecipe(r, tomlPath); err != nil {
		t.Fatalf("WriteRecipe() error = %v", err)
	}

	tomlBytes, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("failed to read generated TOML: %v", err)
	}
	tomlStr := string(tomlBytes)

	if !strings.Contains(tomlStr, `type = "library"`) {
		t.Error("generated TOML missing type = \"library\"")
	}
	if !strings.Contains(tomlStr, `install_mode = "directory"`) {
		t.Error("generated TOML missing install_mode = \"directory\"")
	}
	if strings.Contains(tomlStr, "[verify]") {
		t.Error("generated TOML should not contain [verify] section")
	}
	if !strings.Contains(tomlStr, "outputs =") {
		t.Error("generated TOML missing 'outputs' key")
	}
	if !strings.Contains(tomlStr, "libtree-sitter") {
		t.Error("generated TOML missing tree-sitter library files in outputs")
	}
	if !strings.Contains(tomlStr, "tree_sitter/api.h") {
		t.Error("generated TOML missing tree-sitter header files in outputs")
	}
}

// TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive validates that
// non-library packages (no bin/ and no recognizable lib/ files) still fail
// with the complex_archive category and are not misclassified as libraries.
// Corresponds to scenario-14 in the test plan.
func TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive(t *testing.T) {
	// Simulate a Python-like package: has files but not in bin/ or lib/ with
	// library extensions. This represents packages like python@3.12 that have
	// complex layouts without standard library files.
	entries := []struct {
		name     string
		body     string
		typeflag byte
	}{
		{"python@3.12/3.12.0/libexec/bin/python3", "#!/bin/python", tar.TypeReg},
		{"python@3.12/3.12.0/lib/python3.12/site-packages/__init__.py", "# init", tar.TypeReg},
		{"python@3.12/3.12.0/lib/python3.12/os.py", "# os module", tar.TypeReg},
		{"python@3.12/3.12.0/share/man/man1/python3.1", "man page", tar.TypeReg},
	}

	blob := createBottleTarballBytes(t, entries)

	// Only need current platform for the initial inspection
	server, client := newMockGHCRBottleServer(t, "python@3.12", "3.12.0", map[string][]byte{
		"x86_64_linux": blob,
		"arm64_sonoma": blob,
	})
	defer server.Close()

	b := &HomebrewBuilder{httpClient: client}
	genCtx := &homebrewGenContext{
		formula: "python@3.12",
		formulaInfo: &homebrewFormulaInfo{
			Name:        "python@3.12",
			Description: "Interpreted, interactive, object-oriented programming language",
			Homepage:    "https://www.python.org/",
		},
	}
	genCtx.formulaInfo.Versions.Stable = "3.12.0"

	ctx := context.Background()
	_, err := b.generateDeterministicRecipe(ctx, "python@3.12", genCtx)
	if err == nil {
		t.Fatal("expected error for non-library package, got nil")
	}

	// Verify the error message matches the expected "no binaries or library files" path
	if !strings.Contains(err.Error(), "no binaries or library files found in bottle") {
		t.Errorf("error = %q, want it to contain 'no binaries or library files found in bottle'", err.Error())
	}

	// Verify classification through classifyDeterministicFailure
	session := &HomebrewSession{formula: "python@3.12"}
	classified := session.classifyDeterministicFailure(err)

	if classified.Category != FailureCategoryComplexArchive {
		t.Errorf("Category = %q, want %q", classified.Category, FailureCategoryComplexArchive)
	}

	// Must NOT contain [library_only] tag -- this is not a library
	if strings.Contains(classified.Message, "[library_only]") {
		t.Error("non-library package should not be tagged with [library_only]")
	}
}

// TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure validates that
// when library files are detected but recipe generation fails for other
// reasons, the failure is tagged with [library_only].
func TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure(t *testing.T) {
	// The error path: "library recipe generation failed: ..." triggers
	// the library_only subcategory in classifyDeterministicFailure.
	err := fmt.Errorf("library recipe generation failed: %w",
		fmt.Errorf("no bottle for any target platform"))

	session := &HomebrewSession{formula: "some-library"}
	classified := session.classifyDeterministicFailure(err)

	if classified.Category != FailureCategoryComplexArchive {
		t.Errorf("Category = %q, want %q", classified.Category, FailureCategoryComplexArchive)
	}

	if !strings.Contains(classified.Message, "[library_only]") {
		t.Errorf("Message should contain [library_only] tag, got %q", classified.Message)
	}
}

func TestPlatformTagToOSLibc(t *testing.T) {
	tests := []struct {
		tag      string
		wantOS   string
		wantLibc string
	}{
		{"x86_64_linux", "linux", "glibc"},
		{"arm64_linux", "linux", "glibc"},
		{"arm64_sonoma", "darwin", ""},
		{"sonoma", "darwin", ""},
		{"ventura", "darwin", ""},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			gotOS, gotLibc := platformTagToOSLibc(tt.tag)
			if gotOS != tt.wantOS {
				t.Errorf("OS = %q, want %q", gotOS, tt.wantOS)
			}
			if gotLibc != tt.wantLibc {
				t.Errorf("Libc = %q, want %q", gotLibc, tt.wantLibc)
			}
		})
	}
}

func TestHomebrewBuilder_validateDependencies_NilRegistry(t *testing.T) {
	b := &HomebrewBuilder{} // no registry checker
	deps := []string{"openssl", "zlib"}

	result, err := b.validateDependencies("test-formula", deps)
	if err != nil {
		t.Fatalf("validateDependencies() error = %v, want nil for nil registry", err)
	}
	if len(result) != 2 || result[0] != "openssl" || result[1] != "zlib" {
		t.Errorf("validateDependencies() = %v, want %v", result, deps)
	}
}

func TestHomebrewBuilder_validateDependencies_AllResolved(t *testing.T) {
	b := &HomebrewBuilder{
		registry: &mockRegistryChecker{recipes: map[string]bool{
			"openssl": true,
			"zlib":    true,
		}},
	}
	deps := []string{"openssl", "zlib"}

	result, err := b.validateDependencies("test-formula", deps)
	if err != nil {
		t.Fatalf("validateDependencies() error = %v, want nil", err)
	}
	if len(result) != 2 {
		t.Errorf("validateDependencies() returned %d deps, want 2", len(result))
	}
}

func TestHomebrewBuilder_validateDependencies_MissingDeps(t *testing.T) {
	b := &HomebrewBuilder{
		registry: &mockRegistryChecker{recipes: map[string]bool{
			"openssl": true,
		}},
	}
	deps := []string{"openssl", "zlib", "readline"}

	_, err := b.validateDependencies("test-formula", deps)
	if err == nil {
		t.Fatal("validateDependencies() error = nil, want error for missing deps")
	}

	var detErr *DeterministicFailedError
	if !errors.As(err, &detErr) {
		t.Fatalf("error type = %T, want *DeterministicFailedError", err)
	}
	if detErr.Category != FailureCategoryMissingDep {
		t.Errorf("Category = %q, want %q", detErr.Category, FailureCategoryMissingDep)
	}
	if detErr.Formula != "test-formula" {
		t.Errorf("Formula = %q, want %q", detErr.Formula, "test-formula")
	}

	// Error message must contain each missing dep in the format that
	// extractBlockedByFromOutput() parses: "recipe X not found in registry"
	if !strings.Contains(detErr.Message, "recipe zlib not found in registry") {
		t.Errorf("Message missing zlib: %s", detErr.Message)
	}
	if !strings.Contains(detErr.Message, "recipe readline not found in registry") {
		t.Errorf("Message missing readline: %s", detErr.Message)
	}
	// openssl is present, should not appear as missing
	if strings.Contains(detErr.Message, "recipe openssl not found in registry") {
		t.Errorf("Message should not contain openssl: %s", detErr.Message)
	}
}

func TestHomebrewBuilder_validateDependencies_EmptyDeps(t *testing.T) {
	b := &HomebrewBuilder{
		registry: &mockRegistryChecker{recipes: map[string]bool{}},
	}

	result, err := b.validateDependencies("test-formula", nil)
	if err != nil {
		t.Fatalf("validateDependencies() error = %v, want nil for empty deps", err)
	}
	if result != nil {
		t.Errorf("validateDependencies() = %v, want nil", result)
	}
}

func TestHomebrewBuilder_generateToolRecipe_MissingDeps(t *testing.T) {
	b := &HomebrewBuilder{
		registry: &mockRegistryChecker{recipes: map[string]bool{}},
	}

	genCtx := &homebrewGenContext{
		formula: "jq",
		formulaInfo: &homebrewFormulaInfo{
			Name:         "jq",
			Description:  "JSON processor",
			Homepage:     "https://jqlang.github.io/jq/",
			Dependencies: []string{"oniguruma"},
		},
	}
	genCtx.formulaInfo.Versions.Stable = "1.7.1"

	_, err := b.generateToolRecipe("jq", genCtx, []string{"jq"})
	if err == nil {
		t.Fatal("generateToolRecipe() error = nil, want error for missing dep")
	}

	var detErr *DeterministicFailedError
	if !errors.As(err, &detErr) {
		t.Fatalf("error type = %T, want *DeterministicFailedError", err)
	}
	if detErr.Category != FailureCategoryMissingDep {
		t.Errorf("Category = %q, want %q", detErr.Category, FailureCategoryMissingDep)
	}
	if !strings.Contains(detErr.Message, "recipe oniguruma not found in registry") {
		t.Errorf("Message = %q, want to contain 'recipe oniguruma not found in registry'", detErr.Message)
	}
}

func TestHomebrewBuilder_generateToolRecipe_NilRegistry_PassesThrough(t *testing.T) {
	b := &HomebrewBuilder{} // no registry

	genCtx := &homebrewGenContext{
		formula: "jq",
		formulaInfo: &homebrewFormulaInfo{
			Name:         "jq",
			Description:  "JSON processor",
			Homepage:     "https://jqlang.github.io/jq/",
			Dependencies: []string{"oniguruma"},
		},
	}
	genCtx.formulaInfo.Versions.Stable = "1.7.1"

	r, err := b.generateToolRecipe("jq", genCtx, []string{"jq"})
	if err != nil {
		t.Fatalf("generateToolRecipe() error = %v, want nil (nil registry should pass through)", err)
	}
	if len(r.Metadata.RuntimeDependencies) != 1 || r.Metadata.RuntimeDependencies[0] != "oniguruma" {
		t.Errorf("RuntimeDependencies = %v, want [oniguruma]", r.Metadata.RuntimeDependencies)
	}
}
