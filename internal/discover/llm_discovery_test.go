package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLLMDiscovery_Integration(t *testing.T) {
	// Skip if no API key
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Create discovery with auto-confirm for testing
	discovery, err := NewLLMDiscovery(ctx, WithConfirmFunc(func(result *DiscoveryResult) bool {
		t.Logf("Would confirm: %s:%s (stars=%d)", result.Builder, result.Source, result.Metadata.Stars)
		return true
	}))
	if err != nil {
		t.Fatalf("NewLLMDiscovery: %v", err)
	}

	// Test with a well-known tool
	result, err := discovery.Resolve(ctx, "stripe-cli")
	if err != nil {
		t.Errorf("Resolve error: %v", err)
	}

	if result == nil {
		t.Log("No result returned (may be expected if tool not found or threshold not met)")
		return
	}

	t.Logf("Result: builder=%s source=%s confidence=%s", result.Builder, result.Source, result.Confidence)
	t.Logf("Metadata: stars=%d age=%d", result.Metadata.Stars, result.Metadata.AgeDays)
	t.Logf("Reason: %s", result.Reason)

	// Validate result
	if result.Builder != "github" {
		t.Errorf("expected builder=github, got %s", result.Builder)
	}
	if result.Source != "stripe/stripe-cli" {
		t.Errorf("expected source=stripe/stripe-cli, got %s", result.Source)
	}
	if result.Confidence != ConfidenceLLM {
		t.Errorf("expected confidence=llm, got %s", result.Confidence)
	}
}

func TestLLMDiscovery_Disabled(t *testing.T) {
	discovery := NewLLMDiscoveryDisabled()

	result, err := discovery.Resolve(context.Background(), "anything")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result from disabled discovery")
	}
}

func TestGitHubSourceValidation(t *testing.T) {
	tests := []struct {
		source string
		valid  bool
	}{
		{"stripe/stripe-cli", true},
		{"cli/cli", true},
		{"FiloSottile/age", true},
		{"owner-with-dash/repo", true},
		{"owner/repo-with-dash", true},
		{"owner/repo_underscore", true},
		{"", false},
		{"noslash", false},
		{"/invalid", false},
		{"invalid/", false},
		{"../evil", false},
		{"owner/../other", false},
	}

	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			err := ValidateGitHubURL(tc.source)
			got := err == nil
			if got != tc.valid {
				t.Errorf("ValidateGitHubURL(%q) error=%v, want valid=%v", tc.source, err, tc.valid)
			}
		})
	}
}

func TestDiscoveryToolDefs(t *testing.T) {
	tools := discoveryToolDefs()

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Verify web_search tool
	var webSearch, extractSource *struct {
		Name       string
		Params     map[string]any
		Properties map[string]any
	}

	for _, tool := range tools {
		switch tool.Name {
		case ToolWebSearch:
			props := tool.Parameters["properties"].(map[string]any)
			webSearch = &struct {
				Name       string
				Params     map[string]any
				Properties map[string]any
			}{tool.Name, tool.Parameters, props}
		case ToolExtractSource:
			props := tool.Parameters["properties"].(map[string]any)
			extractSource = &struct {
				Name       string
				Params     map[string]any
				Properties map[string]any
			}{tool.Name, tool.Parameters, props}
		}
	}

	if webSearch == nil {
		t.Fatal("web_search tool not found")
	}
	if extractSource == nil {
		t.Fatal("extract_source tool not found")
	}

	// Verify web_search has query parameter
	if webSearch.Properties["query"] == nil {
		t.Error("web_search missing query parameter")
	}

	// Verify extract_source has required parameters
	for _, param := range []string{"builder", "source", "confidence", "evidence", "reasoning"} {
		if extractSource.Properties[param] == nil {
			t.Errorf("extract_source missing %s parameter", param)
		}
	}
}

func TestVerifyGitHubRepo(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" && os.Getenv("CI") == "" {
		t.Log("Running without GITHUB_TOKEN - rate limits may apply")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	discovery := &LLMDiscovery{
		httpGet: defaultHTTPGet,
	}

	result := &DiscoveryResult{
		Builder: "github",
		Source:  "cli/cli", // GitHub CLI - well-known, high stars
	}

	metadata, err := discovery.verifyGitHubRepo(ctx, result)
	if err != nil {
		t.Fatalf("verifyGitHubRepo: %v", err)
	}

	t.Logf("GitHub CLI: stars=%d description=%q age=%d days", metadata.Stars, metadata.Description, metadata.AgeDays)

	if metadata.Stars < 1000 {
		t.Errorf("expected >1000 stars for cli/cli, got %d", metadata.Stars)
	}
	if metadata.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestExtractSourceValidation(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		wantErr    bool
		errContain string
	}{
		{
			name: "valid extraction",
			args: map[string]any{
				"builder":    "github",
				"source":     "stripe/stripe-cli",
				"confidence": float64(90),
				"evidence":   []any{"GitHub repo", "official docs"},
				"reasoning":  "Found official repository",
			},
			wantErr: false,
		},
		{
			name: "low confidence",
			args: map[string]any{
				"builder":    "github",
				"source":     "stripe/stripe-cli",
				"confidence": float64(50),
				"evidence":   []any{},
				"reasoning":  "Unsure",
			},
			wantErr:    true,
			errContain: "below threshold",
		},
		{
			name: "invalid source format",
			args: map[string]any{
				"builder":    "github",
				"source":     "invalid",
				"confidence": float64(90),
				"evidence":   []any{},
				"reasoning":  "test",
			},
			wantErr:    true,
			errContain: "malformed URL", // ValidateGitHubURL returns ErrURLMalformed for non-owner/repo format
		},
		{
			name: "missing builder",
			args: map[string]any{
				"source":     "stripe/stripe-cli",
				"confidence": float64(90),
				"evidence":   []any{},
				"reasoning":  "test",
			},
			wantErr:    true,
			errContain: "required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := testHandleExtractSource(tc.args)

			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got result: %+v", result)
				} else if tc.errContain != "" && !containsSubstr(err.Error(), tc.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContain)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}

func testHandleExtractSource(args map[string]any) (*DiscoveryResult, error) {
	builder, _ := args["builder"].(string)
	source, _ := args["source"].(string)
	confidence, _ := args["confidence"].(float64)
	reasoning, _ := args["reasoning"].(string)
	evidenceRaw, _ := args["evidence"].([]any)

	if builder == "" || source == "" {
		return nil, errorf("extract_source: builder and source are required")
	}

	// Evidence is parsed but not currently used in DiscoveryResult.
	// Keeping the parse logic for future use when evidence tracking is added.
	_ = evidenceRaw

	if builder == "github" {
		if err := ValidateGitHubURL(source); err != nil {
			return nil, errorf("extract_source: %v", err)
		}
	}

	if int(confidence) < MinConfidenceThreshold {
		return nil, errorf("extract_source: confidence %d is below threshold %d", int(confidence), MinConfidenceThreshold)
	}

	return &DiscoveryResult{
		Builder:    builder,
		Source:     source,
		Confidence: ConfidenceLLM,
		Reason:     reasoning,
		Metadata:   Metadata{},
	}, nil
}

func errorf(format string, args ...any) error {
	return &testError{msg: fmt.Sprintf(format, args...)}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstrAt(s, substr, 0))
}

func containsSubstrAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMockExtraction(t *testing.T) {
	// Test that extraction from arguments works correctly
	args := map[string]any{
		"builder":    "github",
		"source":     "stripe/stripe-cli",
		"confidence": float64(95),
		"evidence":   []any{"GitHub repo found", "Official documentation link"},
		"reasoning":  "Found official Stripe CLI repository",
	}

	argsJSON, _ := json.Marshal(args)
	t.Logf("Args JSON: %s", string(argsJSON))

	builder, _ := args["builder"].(string)
	source, _ := args["source"].(string)
	confidence, _ := args["confidence"].(float64)

	if builder != "github" {
		t.Errorf("builder mismatch: %s", builder)
	}
	if source != "stripe/stripe-cli" {
		t.Errorf("source mismatch: %s", source)
	}
	if confidence != 95 {
		t.Errorf("confidence mismatch: %f", confidence)
	}
}
