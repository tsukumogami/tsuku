package builders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	canBuild, err := b.CanBuild(ctx, "ripgrep")
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
	canBuild, err := b.CanBuild(ctx, "nonexistent-formula")
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
	canBuild, err := b.CanBuild(ctx, "no-bottles")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Errorf("CanBuild() = %v, want false for formula without bottles", canBuild)
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
	canBuild, err := b.CanBuild(ctx, "disabled")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Errorf("CanBuild() = %v, want false for disabled formula", canBuild)
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
	if recipe.Version.Source != "homebrew" {
		t.Errorf("Version.Source = %v, want homebrew", recipe.Version.Source)
	}
	if recipe.Version.Formula != "ripgrep" {
		t.Errorf("Version.Formula = %v, want ripgrep", recipe.Version.Formula)
	}
	if recipe.Verify.Command != "rg --version" {
		t.Errorf("Verify.Command = %v, want rg --version", recipe.Verify.Command)
	}
	if len(recipe.Steps) != 2 {
		t.Fatalf("len(Steps) = %v, want 2", len(recipe.Steps))
	}
	if recipe.Steps[0].Action != "homebrew_bottle" {
		t.Errorf("Steps[0].Action = %v, want homebrew_bottle", recipe.Steps[0].Action)
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
		"homebrew_bottle",
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
	if !containsString(message, "ripgrep") {
		t.Error("buildUserMessage() missing formula name")
	}

	// Check for version
	if !containsString(message, "14.1.0") {
		t.Error("buildUserMessage() missing version")
	}

	// Check for dependencies
	if !containsString(message, "pcre2") {
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
	if !containsString(result, "jq") {
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

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

	// WithHomebrewExecutor
	executor := &validate.Executor{}
	opt = WithHomebrewExecutor(executor)
	opt(b)
	if b.executor != executor {
		t.Error("WithHomebrewExecutor did not set executor")
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
	if !containsString(result, "jq") {
		t.Error("result should contain formula name")
	}
	if !containsString(result, "x86_64_linux") {
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
	if !containsString(result, "test") {
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

func TestHomebrewBuilder_buildRepairMessage(t *testing.T) {
	b := &HomebrewBuilder{
		sanitizer: validate.NewSanitizer(),
	}

	result := &validate.ValidationResult{
		Stdout:   "error: command not found",
		Stderr:   "rg: No such file",
		ExitCode: 127,
	}

	message := b.buildRepairMessage(result)
	if len(message) == 0 {
		t.Error("buildRepairMessage() returned empty string")
	}
	if !containsString(message, "failed validation") {
		t.Error("message should mention failed validation")
	}
	if !containsString(message, "127") {
		t.Error("message should contain exit code")
	}
}

func TestHomebrewBuilder_formatValidationError(t *testing.T) {
	b := &HomebrewBuilder{
		sanitizer: validate.NewSanitizer(),
	}

	result := &validate.ValidationResult{
		Stdout:   "some output",
		Stderr:   "some error",
		ExitCode: 1,
	}

	formatted := b.formatValidationError(result)
	if !containsString(formatted, "exit code 1") {
		t.Error("formatted error should contain exit code")
	}
}

func TestHomebrewBuilder_formatValidationError_LongOutput(t *testing.T) {
	b := &HomebrewBuilder{
		sanitizer: validate.NewSanitizer(),
	}

	// Create output longer than 500 chars
	longOutput := ""
	for i := 0; i < 100; i++ {
		longOutput += "error line "
	}

	result := &validate.ValidationResult{
		Stdout:   longOutput,
		Stderr:   "",
		ExitCode: 1,
	}

	formatted := b.formatValidationError(result)
	if !containsString(formatted, "...") {
		t.Error("long output should be truncated with ...")
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
	if !containsString(result, "defaultformula") {
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
	if !containsString(result, "defaultformula") {
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
	_, err := b.CanBuild(ctx, "test")
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
	if containsString(message, "Runtime Dependencies:") {
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
	if !containsString(output, "simple") {
		t.Error("FormatTree should contain formula name")
	}
	if !containsString(output, "needs recipe") {
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
	if !containsString(output, "parent") {
		t.Error("FormatTree should contain parent")
	}
	if !containsString(output, "child") {
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
	if !containsString(output, "has recipe") {
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
	if !containsString(output, "[duplicate]") {
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

func TestHomebrewBuilder_BuildWithDependencies_Canceled(t *testing.T) {
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
		registry:       &mockRegistryChecker{recipes: map[string]bool{}},
	}

	ctx := context.Background()

	// User cancels
	confirmFunc := func(req *ConfirmationRequest) bool {
		return false
	}

	_, err := b.BuildWithDependencies(ctx, BuildRequest{Package: "test"}, confirmFunc)
	if err != ErrUserCanceled {
		t.Errorf("Expected ErrUserCanceled, got %v", err)
	}
}

func TestHomebrewBuilder_BuildWithDependencies_AllRecipesExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/existing.json" {
			formulaInfo := map[string]interface{}{
				"name":         "existing",
				"dependencies": []string{},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// All recipes already exist
	registry := &mockRegistryChecker{recipes: map[string]bool{"existing": true}}

	b := &HomebrewBuilder{
		httpClient:     server.Client(),
		homebrewAPIURL: server.URL,
		registry:       registry,
	}

	ctx := context.Background()

	// Confirm should not be called since nothing needs generation
	confirmCalled := false
	confirmFunc := func(req *ConfirmationRequest) bool {
		confirmCalled = true
		return true
	}

	results, err := b.BuildWithDependencies(ctx, BuildRequest{Package: "existing"}, confirmFunc)
	if err != nil {
		t.Fatalf("BuildWithDependencies() error = %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results when all recipes exist, got %v", results)
	}
	if confirmCalled {
		t.Error("Confirm should not be called when nothing needs generation")
	}
}

func TestHomebrewBuilder_BuildWithDependencies_ConfirmReceivesCorrectData(t *testing.T) {
	// Verify that the confirmation function receives the correct data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/formula/app.json":
			formulaInfo := map[string]interface{}{
				"name":         "app",
				"dependencies": []string{"dep"},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
			}
			_ = json.NewEncoder(w).Encode(formulaInfo)
		case "/api/formula/dep.json":
			formulaInfo := map[string]interface{}{
				"name":         "dep",
				"dependencies": []string{},
				"versions":     map[string]interface{}{"stable": "1.0", "bottle": true},
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

	var receivedReq *ConfirmationRequest
	confirmFunc := func(req *ConfirmationRequest) bool {
		receivedReq = req
		return false // Cancel to avoid needing an LLM factory
	}

	_, _ = b.BuildWithDependencies(ctx, BuildRequest{Package: "app"}, confirmFunc)

	if receivedReq == nil {
		t.Fatal("Confirm function should have been called")
	}
	if len(receivedReq.ToGenerate) != 2 {
		t.Errorf("ToGenerate length = %d, want 2", len(receivedReq.ToGenerate))
	}
	if receivedReq.EstimatedCost != 2*EstimatedCostPerRecipe {
		t.Errorf("EstimatedCost = %v, want %v", receivedReq.EstimatedCost, 2*EstimatedCostPerRecipe)
	}
	if receivedReq.FormattedTree == "" {
		t.Error("FormattedTree should not be empty")
	}
}

func TestErrUserCanceled(t *testing.T) {
	if ErrUserCanceled.Error() != "operation canceled by user" {
		t.Errorf("ErrUserCanceled.Error() = %v", ErrUserCanceled.Error())
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

// Tests for source build functionality

func TestHomebrewBuilder_fetchFormulaRuby_Success(t *testing.T) {
	rubyContent := `class Jq < Formula
  desc "Lightweight and flexible command-line JSON processor"
  homepage "https://jqlang.github.io/jq/"
  url "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-1.7.1.tar.gz"
  sha256 "478c9ca129fd2e3443fe27314b455e211e0d8c60bc8ff7df703873deeee580c2"

  def install
    system "./configure", *std_configure_args
    system "make"
    system "make", "install"
  end
end
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Homebrew/homebrew-core/HEAD/Formula/j/jq.rb" {
			_, _ = w.Write([]byte(rubyContent))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Override the GitHub raw URL to point to test server
	b := &HomebrewBuilder{
		httpClient: server.Client(),
	}

	// We need to patch the URL - for this test we'll just test the sanitization
	// Real integration would require mocking the GitHub raw URL

	// Test that sanitizeRubyFormula works
	sanitized := b.sanitizeRubyFormula(rubyContent)
	if !containsString(sanitized, "class Jq") {
		t.Error("sanitized content should contain class definition")
	}
	if !containsString(sanitized, "./configure") {
		t.Error("sanitized content should contain configure command")
	}
}

func TestHomebrewBuilder_sanitizeRubyFormula(t *testing.T) {
	b := &HomebrewBuilder{}

	// Test that control characters are removed
	input := "class Test\x00\x01\x02 < Formula"
	sanitized := b.sanitizeRubyFormula(input)
	if containsString(sanitized, "\x00") || containsString(sanitized, "\x01") {
		t.Error("control characters should be removed")
	}
	if !containsString(sanitized, "class Test < Formula") {
		t.Error("normal content should be preserved")
	}

	// Test that newlines, tabs are preserved
	inputWithWhitespace := "class Test\n\ttab"
	sanitized = b.sanitizeRubyFormula(inputWithWhitespace)
	if !containsString(sanitized, "\n") || !containsString(sanitized, "\t") {
		t.Error("newlines and tabs should be preserved")
	}
}

func TestValidateSourceRecipeData_ValidAutotools(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"jq"},
		VerifyCommand: "jq --version",
	}

	err := validateSourceRecipeData(data)
	if err != nil {
		t.Errorf("validateSourceRecipeData() error = %v", err)
	}
}

func TestValidateSourceRecipeData_ValidCMake(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemCMake,
		CMakeArgs:     []string{"-DBUILD_SHARED_LIBS=OFF"},
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
	}

	err := validateSourceRecipeData(data)
	if err != nil {
		t.Errorf("validateSourceRecipeData() error = %v", err)
	}
}

func TestValidateSourceRecipeData_MissingBuildSystem(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   "",
		Executables:   []string{"tool"},
		VerifyCommand: "tool --version",
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("expected error for missing build_system")
	}
}

func TestValidateSourceRecipeData_InvalidBuildSystem(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   "invalid",
		Executables:   []string{"tool"},
		VerifyCommand: "tool --version",
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("expected error for invalid build_system")
	}
}

func TestValidateSourceRecipeData_NoExecutables(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{},
		VerifyCommand: "tool --version",
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("expected error for empty executables")
	}
}

func TestValidateSourceRecipeData_EmptyExecutableName(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{""},
		VerifyCommand: "tool --version",
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("expected error for empty executable name")
	}
}

func TestValidateSourceRecipeData_PathTraversal(t *testing.T) {
	testCases := []struct {
		name string
		exe  string
	}{
		{"parent dir", "../evil"},
		{"absolute path", "/etc/passwd"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &sourceRecipeData{
				BuildSystem:   BuildSystemAutotools,
				Executables:   []string{tc.exe},
				VerifyCommand: "tool --version",
			}

			err := validateSourceRecipeData(data)
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestValidateSourceRecipeData_MissingVerifyCommand(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"tool"},
		VerifyCommand: "",
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("expected error for missing verify_command")
	}
}

func TestValidateSourceRecipeData_InvalidConfigureArg(t *testing.T) {
	testCases := []struct {
		name string
		arg  string
	}{
		{"semicolon", "--enable-feature;rm -rf"},
		{"pipe", "--opt | cat"},
		{"ampersand", "--opt && evil"},
		{"backtick", "--opt `id`"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &sourceRecipeData{
				BuildSystem:   BuildSystemAutotools,
				ConfigureArgs: []string{tc.arg},
				Executables:   []string{"tool"},
				VerifyCommand: "tool --version",
			}

			err := validateSourceRecipeData(data)
			if err == nil {
				t.Errorf("expected error for invalid configure arg: %s", tc.arg)
			}
		})
	}
}

func TestValidateSourceRecipeData_InvalidCMakeArg(t *testing.T) {
	testCases := []struct {
		name string
		arg  string
	}{
		{"semicolon", "-DOPT=val;rm -rf"},
		{"pipe", "-DOPT | cat"},
		{"ampersand", "-DOPT && evil"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &sourceRecipeData{
				BuildSystem:   BuildSystemCMake,
				CMakeArgs:     []string{tc.arg},
				Executables:   []string{"tool"},
				VerifyCommand: "tool --version",
			}

			err := validateSourceRecipeData(data)
			if err == nil {
				t.Errorf("expected error for invalid cmake arg: %s", tc.arg)
			}
		})
	}
}

func TestIsValidConfigureArg(t *testing.T) {
	validArgs := []string{
		"--enable-feature",
		"--with-lib=/usr/lib",
		"--disable-static",
		"CFLAGS=-O2",
	}

	for _, arg := range validArgs {
		if !isValidConfigureArg(arg) {
			t.Errorf("isValidConfigureArg(%q) = false, want true", arg)
		}
	}

	invalidArgs := []string{
		"",
		"--opt;rm",
		"--opt && echo",
		"--opt | cat",
		"--opt `id`",
	}

	for _, arg := range invalidArgs {
		if isValidConfigureArg(arg) {
			t.Errorf("isValidConfigureArg(%q) = true, want false", arg)
		}
	}
}

func TestIsValidCMakeArg(t *testing.T) {
	validArgs := []string{
		"-DCMAKE_BUILD_TYPE=Release",
		"-DBUILD_SHARED_LIBS=ON",
		"-G Ninja",
	}

	for _, arg := range validArgs {
		if !isValidCMakeArg(arg) {
			t.Errorf("isValidCMakeArg(%q) = false, want true", arg)
		}
	}
}

func TestValidBuildSystems(t *testing.T) {
	systems := []BuildSystem{
		BuildSystemAutotools,
		BuildSystemCMake,
		BuildSystemCargo,
		BuildSystemGo,
		BuildSystemMake,
		BuildSystemCustom,
	}

	for _, sys := range systems {
		if !validBuildSystems[sys] {
			t.Errorf("validBuildSystems[%s] = false, want true", sys)
		}
	}

	if validBuildSystems["invalid"] {
		t.Error("validBuildSystems[invalid] = true, want false")
	}
}

func TestHomebrewBuilder_executeToolCall_FetchFormulaRuby(t *testing.T) {
	b := &HomebrewBuilder{
		httpClient: &http.Client{},
	}

	genCtx := &homebrewGenContext{
		formula: "jq",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolFetchFormulaRuby,
		Arguments: map[string]any{
			"formula": "../invalid",
		},
	}

	_, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err == nil {
		t.Error("expected error for invalid formula")
	}
}

func TestHomebrewBuilder_executeToolCall_ExtractSourceRecipe_Valid(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "jq",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolExtractSourceRecipe,
		Arguments: map[string]any{
			"build_system":   "autotools",
			"executables":    []interface{}{"jq"},
			"verify_command": "jq --version",
		},
	}

	result, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err != nil {
		t.Fatalf("executeToolCall() error = %v", err)
	}
	if !containsString(result, "autotools") {
		t.Error("result should contain build_system")
	}
}

func TestHomebrewBuilder_executeToolCall_ExtractSourceRecipe_Invalid(t *testing.T) {
	b := &HomebrewBuilder{}

	genCtx := &homebrewGenContext{
		formula: "jq",
	}

	ctx := context.Background()
	toolCall := llm.ToolCall{
		Name: ToolExtractSourceRecipe,
		Arguments: map[string]any{
			"build_system":   "invalid",
			"executables":    []interface{}{"jq"},
			"verify_command": "jq --version",
		},
	}

	_, _, err := b.executeToolCall(ctx, genCtx, toolCall)
	if err == nil {
		t.Error("expected error for invalid build_system")
	}
}

func TestHomebrewBuilder_buildSourceSteps_Autotools(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		ConfigureArgs: []string{"--disable-static"},
		Executables:   []string{"jq"},
		VerifyCommand: "jq --version",
	}

	steps, err := b.buildSourceSteps(data, "jq")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// Should have 3 steps: homebrew_source, configure_make, install_binaries
	if len(steps) != 3 {
		t.Errorf("buildSourceSteps() returned %d steps, want 3", len(steps))
	}

	if steps[0].Action != "homebrew_source" {
		t.Errorf("steps[0].Action = %s, want homebrew_source", steps[0].Action)
	}
	if steps[1].Action != "configure_make" {
		t.Errorf("steps[1].Action = %s, want configure_make", steps[1].Action)
	}
	if steps[2].Action != "install_binaries" {
		t.Errorf("steps[2].Action = %s, want install_binaries", steps[2].Action)
	}

	// Verify homebrew_source step has correct formula
	if formula, ok := steps[0].Params["formula"].(string); !ok || formula != "jq" {
		t.Errorf("homebrew_source formula = %v, want jq", steps[0].Params["formula"])
	}

	// Verify source_dir is "." (not a placeholder)
	if srcDir, ok := steps[1].Params["source_dir"].(string); !ok || srcDir != "." {
		t.Errorf("configure_make source_dir = %v, want .", steps[1].Params["source_dir"])
	}
}

func TestHomebrewBuilder_buildSourceSteps_CMake(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem: BuildSystemCMake,
		CMakeArgs:   []string{"-DBUILD_SHARED_LIBS=OFF"},
		Executables: []string{"mytool"},
	}

	steps, err := b.buildSourceSteps(data, "mytool")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// steps[0]=homebrew_source, steps[1]=cmake_build
	if steps[1].Action != "cmake_build" {
		t.Errorf("steps[1].Action = %s, want cmake_build", steps[1].Action)
	}
}

func TestHomebrewBuilder_buildSourceSteps_Cargo(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem: BuildSystemCargo,
		Executables: []string{"rg"},
	}

	steps, err := b.buildSourceSteps(data, "ripgrep")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// steps[0]=homebrew_source, steps[1]=cargo_build
	if steps[1].Action != "cargo_build" {
		t.Errorf("steps[1].Action = %s, want cargo_build", steps[1].Action)
	}
}

func TestHomebrewBuilder_buildSourceSteps_Go(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem: BuildSystemGo,
		Executables: []string{"gh"},
	}

	steps, err := b.buildSourceSteps(data, "gh")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// steps[0]=homebrew_source, steps[1]=go_build
	if steps[1].Action != "go_build" {
		t.Errorf("steps[1].Action = %s, want go_build", steps[1].Action)
	}
}

func TestHomebrewBuilder_buildSourceSteps_Make(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem: BuildSystemMake,
		Executables: []string{"tool"},
	}

	steps, err := b.buildSourceSteps(data, "tool")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// steps[0]=homebrew_source, steps[1]=configure_make
	if steps[1].Action != "configure_make" {
		t.Errorf("steps[1].Action = %s, want configure_make", steps[1].Action)
	}

	// Should have skip_configure set
	skipConfigure, ok := steps[1].Params["skip_configure"].(bool)
	if !ok || !skipConfigure {
		t.Error("make build should have skip_configure=true")
	}
}

func TestHomebrewBuilder_buildSourceSteps_Custom(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem: BuildSystemCustom,
		Executables: []string{"tool"},
	}

	_, err := b.buildSourceSteps(data, "tool")
	if err == nil {
		t.Error("expected error for custom build system")
	}
}

func TestHomebrewBuilder_generateSourceRecipeOutput(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:              "jq",
		Description:       "Lightweight JSON processor",
		Homepage:          "https://jqlang.github.io/jq/",
		BuildDependencies: []string{"autoconf", "automake"},
		Dependencies:      []string{"oniguruma"},
	}
	formulaInfo.Versions.Stable = "1.7.1"
	formulaInfo.URLs.Stable.URL = "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-1.7.1.tar.gz"
	formulaInfo.URLs.Stable.Checksum = "abc123"

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"jq"},
		VerifyCommand: "jq --version",
		// Note: BuildDependencies in data is ignored - we use formula info directly
	}

	recipe, err := b.generateSourceRecipeOutput("jq", formulaInfo, data)
	if err != nil {
		t.Fatalf("generateSourceRecipeOutput() error = %v", err)
	}

	if recipe.Metadata.Name != "jq" {
		t.Errorf("recipe.Metadata.Name = %s, want jq", recipe.Metadata.Name)
	}
	if recipe.Version.Formula != "jq" {
		t.Errorf("recipe.Version.Formula = %s, want jq", recipe.Version.Formula)
	}
	if recipe.Verify.Command != "jq --version" {
		t.Errorf("recipe.Verify.Command = %s, want jq --version", recipe.Verify.Command)
	}

	// Dependencies should come from formula info (build + runtime), not LLM
	// Expected: [autoconf, automake, oniguruma]
	if len(recipe.Metadata.Dependencies) != 3 {
		t.Errorf("recipe.Metadata.Dependencies = %v, want 3 deps (build + runtime)", recipe.Metadata.Dependencies)
	}

	// Verify steps use homebrew_source which fetches URL/checksum at plan time
	if len(recipe.Steps) < 1 {
		t.Fatalf("recipe should have at least 1 step, got %d", len(recipe.Steps))
	}
	if recipe.Steps[0].Action != "homebrew_source" {
		t.Errorf("first step should be homebrew_source, got %s", recipe.Steps[0].Action)
	}
	if formula, ok := recipe.Steps[0].Params["formula"].(string); !ok || formula != "jq" {
		t.Errorf("homebrew_source formula = %v, want jq", recipe.Steps[0].Params["formula"])
	}
}

func TestHomebrewBuilder_generateSourceRecipeOutput_NoExecutables(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name: "jq",
	}
	formulaInfo.URLs.Stable.URL = "https://example.com/jq.tar.gz"

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{},
		VerifyCommand: "jq --version",
	}

	_, err := b.generateSourceRecipeOutput("jq", formulaInfo, data)
	if err == nil {
		t.Error("expected error for no executables")
	}
}

func TestHomebrewBuilder_buildSourceSystemPrompt(t *testing.T) {
	b := &HomebrewBuilder{}
	prompt := b.buildSourceSystemPrompt()

	if !containsString(prompt, "source build") {
		t.Error("prompt should mention source build")
	}
	if !containsString(prompt, "autotools") {
		t.Error("prompt should mention autotools")
	}
	if !containsString(prompt, "cmake") {
		t.Error("prompt should mention cmake")
	}
	if !containsString(prompt, "extract_source_recipe") {
		t.Error("prompt should mention extract_source_recipe tool")
	}
}

func TestHomebrewBuilder_buildSourceUserMessage(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:              "jq",
		Description:       "JSON processor",
		Homepage:          "https://jqlang.github.io/jq/",
		BuildDependencies: []string{"autoconf"},
		Dependencies:      []string{"oniguruma"},
	}
	formulaInfo.Versions.Stable = "1.7.1"

	genCtx := &homebrewGenContext{
		formula:     "jq",
		formulaInfo: formulaInfo,
	}

	message := b.buildSourceUserMessage(genCtx)

	if !containsString(message, "jq") {
		t.Error("message should contain formula name")
	}
	if !containsString(message, "source build") {
		t.Error("message should mention source build")
	}
	if !containsString(message, "fetch_formula_ruby") {
		t.Error("message should mention fetch_formula_ruby")
	}
	if !containsString(message, "Build Dependencies") {
		t.Error("message should contain build dependencies section")
	}
}

func TestHomebrewBuilder_buildSourceToolDefs(t *testing.T) {
	b := &HomebrewBuilder{}
	tools := b.buildSourceToolDefs()

	if len(tools) != 3 {
		t.Errorf("buildSourceToolDefs() returned %d tools, want 3", len(tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{ToolFetchFormulaJSON, ToolFetchFormulaRuby, ToolExtractSourceRecipe}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

// Test platform conditional validation

func TestValidateSourceRecipeData_ValidPlatformSteps(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformSteps: map[string][]platformStep{
			"macos": {
				{Action: "run_command", Params: map[string]interface{}{"command": "install_name_tool ..."}},
			},
			"linux": {
				{Action: "set_rpath", Params: map[string]interface{}{"path": "lib/libfoo.so"}},
			},
		},
	}

	err := validateSourceRecipeData(data)
	if err != nil {
		t.Errorf("validateSourceRecipeData() error = %v, want nil", err)
	}
}

func TestValidateSourceRecipeData_InvalidPlatformKey(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformSteps: map[string][]platformStep{
			"windows": { // invalid platform
				{Action: "run_command"},
			},
		},
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("validateSourceRecipeData() expected error for invalid platform key")
	}
	if !strings.Contains(err.Error(), "invalid platform key") {
		t.Errorf("error should mention invalid platform key, got: %v", err)
	}
}

func TestValidateSourceRecipeData_MissingActionInPlatformStep(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformSteps: map[string][]platformStep{
			"macos": {
				{Action: ""}, // missing action
			},
		},
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("validateSourceRecipeData() expected error for missing action")
	}
	if !strings.Contains(err.Error(), "missing action") {
		t.Errorf("error should mention missing action, got: %v", err)
	}
}

func TestValidateSourceRecipeData_ValidPlatformDependencies(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformDependencies: map[string][]string{
			"linux": {"libffi", "patchelf"},
			"macos": {"libtool"},
		},
	}

	err := validateSourceRecipeData(data)
	if err != nil {
		t.Errorf("validateSourceRecipeData() error = %v, want nil", err)
	}
}

func TestValidateSourceRecipeData_InvalidPlatformDependencyKey(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformDependencies: map[string][]string{
			"freebsd": {"somelib"}, // invalid platform
		},
	}

	err := validateSourceRecipeData(data)
	if err == nil {
		t.Error("validateSourceRecipeData() expected error for invalid platform dependency key")
	}
	if !strings.Contains(err.Error(), "invalid platform_dependencies key") {
		t.Errorf("error should mention invalid platform_dependencies key, got: %v", err)
	}
}

// Test platformKeyToWhen conversion

func TestPlatformKeyToWhen(t *testing.T) {
	tests := []struct {
		platform string
		want     map[string]string
	}{
		{"macos", map[string]string{"os": "darwin"}},
		{"linux", map[string]string{"os": "linux"}},
		{"arm64", map[string]string{"arch": "arm64"}},
		{"amd64", map[string]string{"arch": "amd64"}},
		{"x86_64", map[string]string{"arch": "amd64"}},
		{"invalid", nil},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			got := platformKeyToWhen(tt.platform)
			if tt.want == nil {
				if got != nil {
					t.Errorf("platformKeyToWhen(%q) = %v, want nil", tt.platform, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("platformKeyToWhen(%q) length = %d, want %d", tt.platform, len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("platformKeyToWhen(%q)[%q] = %q, want %q", tt.platform, k, got[k], v)
				}
			}
		})
	}
}

func TestValidateSourceResource(t *testing.T) {
	tests := []struct {
		name    string
		res     sourceResourceData
		wantErr bool
	}{
		{
			name: "valid resource",
			res: sourceResourceData{
				Name:     "tree-sitter-c",
				URL:      "https://github.com/tree-sitter/tree-sitter-c/archive/v0.24.1.tar.gz",
				Checksum: "sha256:abc123",
				Dest:     "deps/tree-sitter-c",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			res: sourceResourceData{
				URL:  "https://example.com/file.tar.gz",
				Dest: "deps/test",
			},
			wantErr: true,
		},
		{
			name: "missing url",
			res: sourceResourceData{
				Name: "test-resource",
				Dest: "deps/test",
			},
			wantErr: true,
		},
		{
			name: "http url (not https)",
			res: sourceResourceData{
				Name: "test-resource",
				URL:  "http://example.com/file.tar.gz",
				Dest: "deps/test",
			},
			wantErr: true,
		},
		{
			name: "missing dest",
			res: sourceResourceData{
				Name: "test-resource",
				URL:  "https://example.com/file.tar.gz",
			},
			wantErr: true,
		},
		{
			name: "path traversal in dest",
			res: sourceResourceData{
				Name: "test-resource",
				URL:  "https://example.com/file.tar.gz",
				Dest: "../etc/passwd",
			},
			wantErr: true,
		},
		{
			name: "absolute path in dest",
			res: sourceResourceData{
				Name: "test-resource",
				URL:  "https://example.com/file.tar.gz",
				Dest: "/etc/passwd",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSourceResource(&tt.res, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSourceResource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSourcePatch(t *testing.T) {
	tests := []struct {
		name    string
		patch   sourcePatchData
		wantErr bool
	}{
		{
			name: "valid url patch",
			patch: sourcePatchData{
				URL:   "https://github.com/Homebrew/formula-patches/raw/master/fix.patch",
				Strip: 1,
			},
			wantErr: false,
		},
		{
			name: "valid data patch",
			patch: sourcePatchData{
				Data:   "--- a/main.c\n+++ b/main.c\n@@ -1 +1 @@\n-old\n+new",
				Strip:  1,
				Subdir: "src",
			},
			wantErr: false,
		},
		{
			name:    "missing url and data",
			patch:   sourcePatchData{Strip: 1},
			wantErr: true,
		},
		{
			name: "both url and data",
			patch: sourcePatchData{
				URL:  "https://example.com/fix.patch",
				Data: "some patch data",
			},
			wantErr: true,
		},
		{
			name: "http url (not https)",
			patch: sourcePatchData{
				URL: "http://example.com/fix.patch",
			},
			wantErr: true,
		},
		{
			name: "path traversal in subdir",
			patch: sourcePatchData{
				Data:   "some patch",
				Subdir: "../etc",
			},
			wantErr: true,
		},
		{
			name: "absolute path in subdir",
			patch: sourcePatchData{
				Data:   "some patch",
				Subdir: "/etc",
			},
			wantErr: true,
		},
		{
			name: "negative strip level",
			patch: sourcePatchData{
				Data:  "some patch",
				Strip: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSourcePatch(&tt.patch, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSourcePatch() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSourceInreplace(t *testing.T) {
	tests := []struct {
		name    string
		ir      sourceInreplaceData
		wantErr bool
	}{
		{
			name: "valid inreplace",
			ir: sourceInreplaceData{
				File:        "Makefile",
				Pattern:     "STATIC",
				Replacement: "SHARED",
			},
			wantErr: false,
		},
		{
			name: "valid regex inreplace",
			ir: sourceInreplaceData{
				File:        "config.h",
				Pattern:     "#define VERSION.*",
				Replacement: "#define VERSION \"1.0.0\"",
				IsRegex:     true,
			},
			wantErr: false,
		},
		{
			name: "empty replacement (deletion)",
			ir: sourceInreplaceData{
				File:        "Makefile",
				Pattern:     "DEBUG=1",
				Replacement: "",
			},
			wantErr: false,
		},
		{
			name: "missing file",
			ir: sourceInreplaceData{
				Pattern:     "old",
				Replacement: "new",
			},
			wantErr: true,
		},
		{
			name: "missing pattern",
			ir: sourceInreplaceData{
				File:        "Makefile",
				Replacement: "new",
			},
			wantErr: true,
		},
		{
			name: "path traversal in file",
			ir: sourceInreplaceData{
				File:        "../etc/passwd",
				Pattern:     "old",
				Replacement: "new",
			},
			wantErr: true,
		},
		{
			name: "absolute path in file",
			ir: sourceInreplaceData{
				File:        "/etc/passwd",
				Pattern:     "old",
				Replacement: "new",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSourceInreplace(&tt.ir, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSourceInreplace() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSourceRecipeData_WithResources(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemCMake,
		Executables:   []string{"nvim"},
		VerifyCommand: "nvim --version",
		Resources: []sourceResourceData{
			{
				Name:     "tree-sitter-c",
				URL:      "https://github.com/tree-sitter/tree-sitter-c/archive/v0.24.1.tar.gz",
				Checksum: "sha256:abc123",
				Dest:     "deps/tree-sitter-c",
			},
		},
	}

	if err := validateSourceRecipeData(data); err != nil {
		t.Errorf("validateSourceRecipeData() error = %v", err)
	}
}

func TestValidateSourceRecipeData_WithInvalidResource(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemCMake,
		Executables:   []string{"nvim"},
		VerifyCommand: "nvim --version",
		Resources: []sourceResourceData{
			{
				Name: "test-resource",
				// Missing URL
				Dest: "deps/test",
			},
		},
	}

	if err := validateSourceRecipeData(data); err == nil {
		t.Error("validateSourceRecipeData() expected error for missing URL")
	}
}

func TestValidateSourceRecipeData_WithPatches(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"curl"},
		VerifyCommand: "curl --version",
		Patches: []sourcePatchData{
			{
				URL:   "https://github.com/Homebrew/formula-patches/raw/master/curl/fix.patch",
				Strip: 1,
			},
		},
	}

	if err := validateSourceRecipeData(data); err != nil {
		t.Errorf("validateSourceRecipeData() error = %v", err)
	}
}

func TestValidateSourceRecipeData_WithInvalidPatch(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"curl"},
		VerifyCommand: "curl --version",
		Patches: []sourcePatchData{
			{
				// Missing both URL and Data
				Strip: 1,
			},
		},
	}

	if err := validateSourceRecipeData(data); err == nil {
		t.Error("validateSourceRecipeData() expected error for invalid patch")
	}
}

func TestValidateSourceRecipeData_WithInreplace(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"jq"},
		VerifyCommand: "jq --version",
		Inreplace: []sourceInreplaceData{
			{
				File:        "Makefile",
				Pattern:     "STATIC",
				Replacement: "SHARED",
			},
		},
	}

	if err := validateSourceRecipeData(data); err != nil {
		t.Errorf("validateSourceRecipeData() error = %v", err)
	}
}

func TestValidateSourceRecipeData_WithInvalidInreplace(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"jq"},
		VerifyCommand: "jq --version",
		Inreplace: []sourceInreplaceData{
			{
				// Missing file
				Pattern:     "old",
				Replacement: "new",
			},
		},
	}

	if err := validateSourceRecipeData(data); err == nil {
		t.Error("validateSourceRecipeData() expected error for invalid inreplace")
	}
}

func TestBuildSourceSteps_WithInreplace(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"jq"},
		VerifyCommand: "jq --version",
		Inreplace: []sourceInreplaceData{
			{
				File:        "Makefile",
				Pattern:     "STATIC",
				Replacement: "SHARED",
			},
			{
				File:        "config.h",
				Pattern:     "DEBUG",
				Replacement: "RELEASE",
				IsRegex:     true,
			},
		},
	}

	steps, err := b.buildSourceSteps(data, "jq")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// Should have: homebrew_source, 2x text_replace, configure_make, install_binaries
	// = 5 steps total
	if len(steps) != 5 {
		t.Errorf("buildSourceSteps() returned %d steps, want 5", len(steps))
	}

	// Check text_replace steps are present
	textReplaceCount := 0
	for _, step := range steps {
		if step.Action == "text_replace" {
			textReplaceCount++
		}
	}
	if textReplaceCount != 2 {
		t.Errorf("buildSourceSteps() has %d text_replace steps, want 2", textReplaceCount)
	}
}

// Test buildSourceSteps with platform conditionals

func TestHomebrewBuilder_buildSourceSteps_WithPlatformSteps(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformSteps: map[string][]platformStep{
			"macos": {
				{Action: "run_command", Params: map[string]interface{}{"command": "install_name_tool -id @rpath/libfoo.dylib lib/libfoo.dylib"}},
			},
			"linux": {
				{Action: "set_rpath", Params: map[string]interface{}{"path": "lib/libfoo.so"}},
			},
		},
	}

	steps, err := b.buildSourceSteps(data, "mytool")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// Count platform-conditional steps
	macosSteps := 0
	linuxSteps := 0
	for _, step := range steps {
		if step.When != nil {
			if step.When["os"] == "darwin" {
				macosSteps++
			}
			if step.When["os"] == "linux" {
				linuxSteps++
			}
		}
	}

	if macosSteps != 1 {
		t.Errorf("expected 1 macOS conditional step, got %d", macosSteps)
	}
	if linuxSteps != 1 {
		t.Errorf("expected 1 Linux conditional step, got %d", linuxSteps)
	}
}

func TestHomebrewBuilder_buildSourceSteps_ArchConditionals(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemCMake,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformSteps: map[string][]platformStep{
			"arm64": {
				{Action: "run_command", Params: map[string]interface{}{"command": "echo arm64"}},
			},
			"amd64": {
				{Action: "run_command", Params: map[string]interface{}{"command": "echo amd64"}},
			},
		},
	}

	steps, err := b.buildSourceSteps(data, "mytool")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// Find arch-conditional steps
	arm64Steps := 0
	amd64Steps := 0
	for _, step := range steps {
		if step.When != nil {
			if step.When["arch"] == "arm64" {
				arm64Steps++
			}
			if step.When["arch"] == "amd64" {
				amd64Steps++
			}
		}
	}

	if arm64Steps != 1 {
		t.Errorf("expected 1 arm64 conditional step, got %d", arm64Steps)
	}
	if amd64Steps != 1 {
		t.Errorf("expected 1 amd64 conditional step, got %d", amd64Steps)
	}
}

// Test system prompt includes platform conditional docs

func TestHomebrewBuilder_buildSourceSystemPrompt_IncludesPlatformDocs(t *testing.T) {
	b := &HomebrewBuilder{}
	prompt := b.buildSourceSystemPrompt()

	// Check for platform conditional documentation
	requiredPhrases := []string{
		"on_macos",
		"on_linux",
		"on_arm",
		"on_intel",
		"platform_steps",
		"platform_dependencies",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("buildSourceSystemPrompt() should contain %q", phrase)
		}
	}
}

// Test tool definitions include platform parameters

func TestHomebrewBuilder_buildSourceToolDefs_IncludesPlatformParams(t *testing.T) {
	b := &HomebrewBuilder{}
	tools := b.buildSourceToolDefs()

	// Find extract_source_recipe tool
	var extractTool *llm.ToolDef
	for i := range tools {
		if tools[i].Name == ToolExtractSourceRecipe {
			extractTool = &tools[i]
			break
		}
	}

	if extractTool == nil {
		t.Fatal("extract_source_recipe tool not found")
	}

	// Check parameters contain platform_steps and platform_dependencies
	params, ok := extractTool.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("tool parameters not in expected format")
	}

	if _, ok := params["platform_steps"]; !ok {
		t.Error("extract_source_recipe should have platform_steps parameter")
	}

	if _, ok := params["platform_dependencies"]; !ok {
		t.Error("extract_source_recipe should have platform_dependencies parameter")
	}
}

func TestGenerateSourceRecipeOutput_WithResourcesAndPatches(t *testing.T) {
	b := &HomebrewBuilder{}

	formulaInfo := &homebrewFormulaInfo{
		Name:        "neovim",
		Description: "Vim-fork",
		Homepage:    "https://neovim.io",
	}
	formulaInfo.Versions.Stable = "0.10.0"
	formulaInfo.URLs.Stable.URL = "https://github.com/neovim/neovim/archive/refs/tags/v0.10.0.tar.gz"

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemCMake,
		Executables:   []string{"nvim"},
		VerifyCommand: "nvim --version",
		Resources: []sourceResourceData{
			{
				Name:     "tree-sitter-c",
				URL:      "https://github.com/tree-sitter/tree-sitter-c/archive/v0.24.1.tar.gz",
				Checksum: "sha256:abc123",
				Dest:     "deps/tree-sitter-c",
			},
		},
		Patches: []sourcePatchData{
			{
				URL:   "https://github.com/Homebrew/formula-patches/raw/master/neovim/fix.patch",
				Strip: 1,
			},
		},
	}

	recipe, err := b.generateSourceRecipeOutput("neovim", formulaInfo, data)
	if err != nil {
		t.Fatalf("generateSourceRecipeOutput() error = %v", err)
	}

	// Check resources were converted
	if len(recipe.Resources) != 1 {
		t.Errorf("recipe.Resources has %d entries, want 1", len(recipe.Resources))
	}
	if recipe.Resources[0].Name != "tree-sitter-c" {
		t.Errorf("recipe.Resources[0].Name = %s, want tree-sitter-c", recipe.Resources[0].Name)
	}

	// Check patches were converted
	if len(recipe.Patches) != 1 {
		t.Errorf("recipe.Patches has %d entries, want 1", len(recipe.Patches))
	}
	if !containsString(recipe.Patches[0].URL, "formula-patches") {
		t.Error("recipe.Patches[0].URL should contain formula-patches")
	}
}

// Test multiple executables handling (EX-2)
func TestHomebrewBuilder_buildSourceSteps_MultipleExecutables(t *testing.T) {
	b := &HomebrewBuilder{}

	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"bin/tool1", "bin/tool2", "lib/helper"},
		VerifyCommand: "tool1 --version",
	}

	steps, err := b.buildSourceSteps(data, "test-formula")
	if err != nil {
		t.Fatalf("buildSourceSteps() error = %v", err)
	}

	// Find install_binaries step
	var installStep *recipe.Step
	for i := range steps {
		if steps[i].Action == "install_binaries" {
			installStep = &steps[i]
			break
		}
	}

	if installStep == nil {
		t.Fatal("install_binaries step not found")
	}

	// Check binaries parameter contains all executables
	binaries, ok := installStep.Params["binaries"].([]string)
	if !ok {
		t.Fatal("binaries param not a []string")
	}
	if len(binaries) != 3 {
		t.Errorf("binaries has %d entries, want 3", len(binaries))
	}
}

// Test versioned executable names (EX-3)
func TestValidateSourceRecipeData_VersionedExecutables(t *testing.T) {
	tests := []struct {
		name        string
		executables []string
		wantErr     bool
	}{
		{
			name:        "versioned python",
			executables: []string{"python3.12"},
			wantErr:     false,
		},
		{
			name:        "versioned with hyphen",
			executables: []string{"clang-18"},
			wantErr:     false,
		},
		{
			name:        "multiple versioned",
			executables: []string{"llvm-ar-18", "clang-18", "clangd-18"},
			wantErr:     false,
		},
		{
			name:        "with bin prefix",
			executables: []string{"bin/python3.12"},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &sourceRecipeData{
				BuildSystem:   BuildSystemAutotools,
				Executables:   tt.executables,
				VerifyCommand: tt.executables[0] + " --version",
			}
			err := validateSourceRecipeData(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSourceRecipeData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test configure argument valid edge cases
func TestValidateSourceRecipeData_ConfigureArgsValidCases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "equals sign in value",
			args: []string{"--with-path=/usr/local"},
		},
		{
			name: "double equals",
			args: []string{"LDFLAGS=-Wl,-rpath=/opt/lib"},
		},
		{
			name: "multiple flags",
			args: []string{"--enable-shared", "--disable-static", "--prefix=/usr/local"},
		},
		{
			name: "env variable style",
			args: []string{"CFLAGS=-O2", "CXXFLAGS=-O2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &sourceRecipeData{
				BuildSystem:   BuildSystemAutotools,
				ConfigureArgs: tt.args,
				Executables:   []string{"tool"},
				VerifyCommand: "tool --version",
			}
			err := validateSourceRecipeData(data)
			if err != nil {
				t.Errorf("validateSourceRecipeData() unexpected error = %v", err)
			}
		})
	}
}

// Test CMake argument valid edge cases
func TestValidateSourceRecipeData_CMakeArgsValidCases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "standard cmake define",
			args: []string{"-DCMAKE_BUILD_TYPE=Release"},
		},
		{
			name: "path value",
			args: []string{"-DCMAKE_INSTALL_PREFIX=/usr/local"},
		},
		{
			name: "multiple defines",
			args: []string{"-DBUILD_SHARED_LIBS=ON", "-DBUILD_TESTING=OFF"},
		},
		{
			name: "cmake module path",
			args: []string{"-DCMAKE_MODULE_PATH=/opt/cmake/modules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &sourceRecipeData{
				BuildSystem:   BuildSystemCMake,
				CMakeArgs:     tt.args,
				Executables:   []string{"tool"},
				VerifyCommand: "tool --version",
			}
			err := validateSourceRecipeData(data)
			if err != nil {
				t.Errorf("validateSourceRecipeData() unexpected error = %v", err)
			}
		})
	}
}

// Test combined os and arch conditionals (PS-5)
func TestValidateSourceRecipeData_CombinedPlatformConditionals(t *testing.T) {
	data := &sourceRecipeData{
		BuildSystem:   BuildSystemAutotools,
		Executables:   []string{"mytool"},
		VerifyCommand: "mytool --version",
		PlatformSteps: map[string][]platformStep{
			"macos": {
				{Action: "run_command", Params: map[string]interface{}{"command": "echo macos"}},
			},
			"arm64": {
				{Action: "run_command", Params: map[string]interface{}{"command": "echo arm64"}},
			},
		},
		PlatformDependencies: map[string][]string{
			"linux": {"libssl-dev"},
			"amd64": {"libc6-dev"},
		},
	}

	err := validateSourceRecipeData(data)
	if err != nil {
		t.Errorf("validateSourceRecipeData() error = %v, want nil for combined platform conditionals", err)
	}
}

// Test versioned formula names (VS-1)
func TestHomebrewBuilder_CanBuild_VersionedFormula(t *testing.T) {
	// Create mock server that returns versioned formula
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/formula/postgresql@15.json" {
			formulaInfo := map[string]interface{}{
				"name":      "postgresql@15",
				"full_name": "postgresql@15",
				"desc":      "Object-relational database system version 15",
				"homepage":  "https://www.postgresql.org/",
				"versions": map[string]interface{}{
					"stable": "15.8",
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
	canBuild, err := b.CanBuild(ctx, "postgresql@15")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Errorf("CanBuild() = %v, want true for versioned formula", canBuild)
	}
}

// Test tool call type validation (HBR-1)
func TestHomebrewBuilder_executeToolCall_TypeValidation(t *testing.T) {
	b := &HomebrewBuilder{}
	ctx := context.Background()

	genCtx := &homebrewGenContext{
		formula: "test-formula",
	}

	tests := []struct {
		name        string
		toolCall    llm.ToolCall
		wantErr     bool
		errContains string
	}{
		{
			name: "executables as string instead of array",
			toolCall: llm.ToolCall{
				ID:   "test-1",
				Name: ToolExtractRecipe,
				Arguments: map[string]any{
					"executables":    "bin/rg", // Should be []any
					"verify_command": "rg --version",
				},
			},
			wantErr:     true,
			errContains: "extract_recipe",
		},
		{
			name: "dependencies as string instead of array",
			toolCall: llm.ToolCall{
				ID:   "test-2",
				Name: ToolExtractRecipe,
				Arguments: map[string]any{
					"executables":    []any{"bin/rg"},
					"verify_command": "rg --version",
					"dependencies":   "pcre2", // Should be []any
				},
			},
			wantErr:     true,
			errContains: "extract_recipe",
		},
		{
			name: "verify_command as number instead of string",
			toolCall: llm.ToolCall{
				ID:   "test-3",
				Name: ToolExtractRecipe,
				Arguments: map[string]any{
					"executables":    []any{"bin/rg"},
					"verify_command": 12345, // Should be string
				},
			},
			wantErr:     true,
			errContains: "extract_recipe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := b.executeToolCall(ctx, genCtx, tt.toolCall)
			if (err != nil) != tt.wantErr {
				t.Errorf("executeToolCall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("executeToolCall() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

// Test source tool call type validation
func TestHomebrewBuilder_executeToolCall_SourceTypeValidation(t *testing.T) {
	b := &HomebrewBuilder{}
	ctx := context.Background()

	genCtx := &homebrewGenContext{
		formula:     "test-formula",
		formulaInfo: &homebrewFormulaInfo{Name: "test-formula"},
	}

	tests := []struct {
		name        string
		toolCall    llm.ToolCall
		wantErr     bool
		errContains string
	}{
		{
			name: "build_system as number",
			toolCall: llm.ToolCall{
				ID:   "test-1",
				Name: ToolExtractSourceRecipe,
				Arguments: map[string]any{
					"build_system":   123, // Should be string
					"executables":    []any{"jq"},
					"verify_command": "jq --version",
				},
			},
			wantErr:     true,
			errContains: "extract_source_recipe",
		},
		{
			name: "cmake_args as string instead of array",
			toolCall: llm.ToolCall{
				ID:   "test-2",
				Name: ToolExtractSourceRecipe,
				Arguments: map[string]any{
					"build_system":   "cmake",
					"executables":    []any{"tool"},
					"verify_command": "tool --version",
					"cmake_args":     "-DFOO=BAR", // Should be []any
				},
			},
			wantErr:     true,
			errContains: "extract_source_recipe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := b.executeToolCall(ctx, genCtx, tt.toolCall)
			if (err != nil) != tt.wantErr {
				t.Errorf("executeToolCall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("executeToolCall() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}
