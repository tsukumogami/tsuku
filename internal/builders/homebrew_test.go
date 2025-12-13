package builders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsukumogami/tsuku/internal/llm"
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
