package discover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// testLogger creates a Logger backed by a bytes.Buffer for output verification.
func testLogger(buf *bytes.Buffer) log.Logger {
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return log.New(h)
}

func TestChainResolver_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	expected := &DiscoveryResult{Builder: "github", Source: "cli/cli", Confidence: ConfidenceRegistry}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithLogger(logger)

	result, err := chain.Resolve(context.Background(), "gh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
	// Logger should have been called
	if buf.Len() == 0 {
		t.Error("expected logger output, got none")
	}
}

func TestChainResolver_WithLogger_StageHit_Ecosystem(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	expected := &DiscoveryResult{
		Builder:    "crates.io",
		Source:     "ripgrep",
		Confidence: ConfidenceEcosystem,
		Metadata:   Metadata{Downloads: 5000000},
	}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithLogger(logger)

	_, err := chain.Resolve(context.Background(), "ripgrep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("crates.io")) {
		t.Errorf("expected log to mention builder, got: %s", output)
	}
}

func TestChainResolver_WithLogger_StageHit_LLM(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	expected := &DiscoveryResult{
		Builder:    "github",
		Source:     "stripe/stripe-cli",
		Confidence: ConfidenceLLM,
	}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithLogger(logger)

	_, err := chain.Resolve(context.Background(), "stripe-cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("web search")) {
		t.Errorf("expected log to mention web search, got: %s", output)
	}
}

func TestChainResolver_WithLogger_StageMiss_TwoRemaining(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	// 3 stages: first misses (registry), second misses (ecosystem), third hits (LLM)
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: &DiscoveryResult{Builder: "llm", Confidence: ConfidenceLLM}},
	).WithLogger(logger)

	_, err := chain.Resolve(context.Background(), "sometool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("probing package ecosystems")) {
		t.Errorf("expected log about ecosystem probing, got: %s", output)
	}
}

func TestChainResolver_WithLogger_StageMiss_OneRemaining(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	// 3 stages: registry miss, ecosystem miss, LLM hit
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: &DiscoveryResult{Builder: "llm", Confidence: ConfidenceLLM}},
	).WithLogger(logger)

	_, err := chain.Resolve(context.Background(), "sometool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("web search")) {
		t.Errorf("expected log about web search, got: %s", output)
	}
}

func TestChainResolver_WithRegistry(t *testing.T) {
	reg := &DiscoveryRegistry{}
	chain := NewChainResolver(&mockResolver{result: nil}).WithRegistry(reg)

	if chain.registry != reg {
		t.Error("expected registry to be set")
	}

	// Resolve should still work with registry set (no panic)
	chain = chain.WithLLMAvailability(LLMAvailability{HasAPIKey: true})
	_, err := chain.Resolve(context.Background(), "sometool")
	if err == nil {
		t.Fatal("expected NotFoundError")
	}
}

func TestChainResolver_WithTelemetry_NilIsNoOp(t *testing.T) {
	// When telemetry is nil, emit methods should be no-ops (no panic)
	expected := &DiscoveryResult{Builder: "github", Source: "cli/cli", Confidence: ConfidenceRegistry}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithTelemetry(nil)

	result, err := chain.Resolve(context.Background(), "gh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestChainResolver_BudgetExceeded(t *testing.T) {
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{err: ErrBudgetExceeded},
		&mockResolver{result: &DiscoveryResult{Builder: "should-not-reach"}},
	)

	_, err := chain.Resolve(context.Background(), "sometool")
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestChainResolver_AmbiguousError_IsFatal(t *testing.T) {
	// AmbiguousMatchError should stop the chain (it's a fatal error)
	ambErr := &AmbiguousMatchError{Tool: "serve", Matches: []DiscoveryMatch{
		{Builder: "npm", Source: "serve"},
		{Builder: "crates.io", Source: "serve"},
	}}

	chain := NewChainResolver(
		&mockResolver{err: ambErr},
		&mockResolver{result: &DiscoveryResult{Builder: "should-not-reach"}},
	)

	_, err := chain.Resolve(context.Background(), "serve")
	var gotAmbig *AmbiguousMatchError
	if !errors.As(err, &gotAmbig) {
		t.Fatalf("expected AmbiguousMatchError, got %v", err)
	}
}

func TestFormatMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata Metadata
		want     string
	}{
		{"downloads present", Metadata{Downloads: 5000}, "5000 downloads"},
		{"stars present no downloads", Metadata{Stars: 100}, "100 stars"},
		{"both present prefers downloads", Metadata{Downloads: 5000, Stars: 100}, "5000 downloads"},
		{"neither present", Metadata{}, "no stats"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMetadata(tt.metadata)
			if got != tt.want {
				t.Errorf("formatMetadata() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEcosystemProbe_WithConfirmDisambiguation(t *testing.T) {
	// Set up two ambiguous matches (close downloads)
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "serve", Downloads: 1000, VersionCount: 10, HasRepository: true}},
		&mockProber{name: "crates.io", result: &builders.ProbeResult{Source: "serve", Downloads: 500, VersionCount: 10, HasRepository: true}},
	}, 5*time.Second, WithConfirmDisambiguation(func(matches []ProbeMatch) (int, error) {
		// Select the first match
		return 0, nil
	}))

	result, err := probe.Resolve(context.Background(), "serve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result from interactive disambiguation")
	}
}

func TestEcosystemProbe_WithConfirmDisambiguation_Error(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "serve", Downloads: 1000, VersionCount: 10, HasRepository: true}},
		&mockProber{name: "crates.io", result: &builders.ProbeResult{Source: "serve", Downloads: 500, VersionCount: 10, HasRepository: true}},
	}, 5*time.Second, WithConfirmDisambiguation(func(matches []ProbeMatch) (int, error) {
		return 0, errors.New("user canceled")
	}))

	_, err := probe.Resolve(context.Background(), "serve")
	if err == nil {
		t.Fatal("expected error from canceled disambiguation")
	}
}

func TestEcosystemProbe_WithForceDeterministic(t *testing.T) {
	// Two ambiguous matches (close downloads) - forceDeterministic should pick first ranked
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", result: &builders.ProbeResult{Source: "serve", Downloads: 1000, VersionCount: 10, HasRepository: true}},
		&mockProber{name: "crates.io", result: &builders.ProbeResult{Source: "serve", Downloads: 500, VersionCount: 10, HasRepository: true}},
	}, 5*time.Second, WithForceDeterministic())

	result, err := probe.Resolve(context.Background(), "serve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result from force-deterministic mode")
	}
	if result.Metadata.SelectionReason != SelectionPriorityFallback {
		t.Errorf("expected SelectionReason %q, got %q", SelectionPriorityFallback, result.Metadata.SelectionReason)
	}
}

func TestEcosystemProbe_ResolveWithDetails_AllErrors(t *testing.T) {
	probe := NewEcosystemProbe([]builders.EcosystemProber{
		&mockProber{name: "npm", err: errors.New("fail")},
		&mockProber{name: "pypi", err: errors.New("fail")},
	}, 5*time.Second)

	_, err := probe.ResolveWithDetails(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error when all probers fail")
	}
}

// Tests for error types in resolver.go

func TestNotFoundError(t *testing.T) {
	err := &NotFoundError{Tool: "mytool"}
	if err.Error() != "could not find 'mytool'" {
		t.Errorf("Error() = %q", err.Error())
	}
	suggestion := err.Suggestion()
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestConfigurationError(t *testing.T) {
	tests := []struct {
		reason      string
		wantErr     string
		wantSuggest string
	}{
		{"no_api_key", "no match for", "ANTHROPIC_API_KEY"},
		{"deterministic_only", "no deterministic source", "--deterministic-only"},
		{"other", "configuration error", ""},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			err := &ConfigurationError{Tool: "mytool", Reason: tt.reason}
			errStr := err.Error()
			if !bytes.Contains([]byte(errStr), []byte(tt.wantErr)) {
				t.Errorf("Error() = %q, want to contain %q", errStr, tt.wantErr)
			}
			suggest := err.Suggestion()
			if tt.wantSuggest != "" && !bytes.Contains([]byte(suggest), []byte(tt.wantSuggest)) {
				t.Errorf("Suggestion() = %q, want to contain %q", suggest, tt.wantSuggest)
			}
		})
	}
}

func TestBuilderRequiresLLMError(t *testing.T) {
	err := &BuilderRequiresLLMError{Tool: "mytool", Builder: "github", Source: "owner/repo"}
	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error message")
	}
	suggestion := err.Suggestion()
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestAmbiguousMatchError_Error(t *testing.T) {
	err := &AmbiguousMatchError{
		Tool: "serve",
		Matches: []DiscoveryMatch{
			{Builder: "npm", Source: "serve"},
			{Builder: "crates.io", Source: "serve"},
		},
	}
	errStr := err.Error()
	if !bytes.Contains([]byte(errStr), []byte("Multiple sources")) {
		t.Errorf("Error() = %q, want to contain 'Multiple sources'", errStr)
	}
	if !bytes.Contains([]byte(errStr), []byte("--from")) {
		t.Errorf("Error() = %q, want to contain '--from'", errStr)
	}
}

func TestIsFatalError(t *testing.T) {
	t.Run("ambiguous is fatal", func(t *testing.T) {
		err := &AmbiguousMatchError{Tool: "x"}
		if !isFatalError(err) {
			t.Error("AmbiguousMatchError should be fatal")
		}
	})

	t.Run("generic error is not fatal", func(t *testing.T) {
		err := errors.New("generic")
		if isFatalError(err) {
			t.Error("generic error should not be fatal")
		}
	})
}

// Tests for LLM discovery option functions (unit coverage)

func TestLLMDiscoveryOptions(t *testing.T) {
	d := &LLMDiscovery{}

	// Test WithConfirmFunc
	fn := func(result *DiscoveryResult) bool { return true }
	opt := WithConfirmFunc(fn)
	opt(d)
	if d.confirm == nil {
		t.Error("WithConfirmFunc should set confirm")
	}

	// Test WithHTTPGet
	httpFn := func(ctx context.Context, url string) ([]byte, error) { return nil, nil }
	opt2 := WithHTTPGet(httpFn)
	opt2(d)
	if d.httpGet == nil {
		t.Error("WithHTTPGet should set httpGet")
	}

	// Test WithConfig
	opt3 := WithConfig(nil)
	opt3(d) // Should not panic

	// Test WithStateTracker
	opt4 := WithStateTracker(nil)
	opt4(d) // Should not panic
}

func TestDefaultConfirm(t *testing.T) {
	result := &DiscoveryResult{Builder: "github", Source: "owner/repo"}
	if !defaultConfirm(result) {
		t.Error("defaultConfirm should return true")
	}
}

func TestDiscoverySystemPrompt(t *testing.T) {
	prompt := discoverySystemPrompt("stripe-cli")
	if !bytes.Contains([]byte(prompt), []byte("stripe-cli")) {
		t.Error("expected prompt to contain tool name")
	}
	if !bytes.Contains([]byte(prompt), []byte("GitHub")) {
		t.Error("expected prompt to mention GitHub")
	}
}

func TestIsRateLimitResponse(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{"X-Ratelimit-Remaining": []string{"0"}},
		}
		if !isRateLimitResponse(resp) {
			t.Error("expected rate limited")
		}
	})

	t.Run("not rate limited", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{"X-Ratelimit-Remaining": []string{"100"}},
		}
		if isRateLimitResponse(resp) {
			t.Error("expected not rate limited")
		}
	})

	t.Run("no header", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{},
		}
		if isRateLimitResponse(resp) {
			t.Error("expected not rate limited without header")
		}
	})
}

func TestParseRateLimitError(t *testing.T) {
	t.Run("with reset time", func(t *testing.T) {
		resetTime := time.Now().Add(10 * time.Minute).Unix()
		resp := &http.Response{
			Header: http.Header{
				"X-Ratelimit-Reset": []string{fmt.Sprintf("%d", resetTime)},
			},
		}
		err := parseRateLimitError(resp, true)
		if !err.Authenticated {
			t.Error("expected authenticated")
		}
		if err.ResetTime.IsZero() {
			t.Error("expected non-zero reset time")
		}
	})

	t.Run("without reset time", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{},
		}
		err := parseRateLimitError(resp, false)
		if err.Authenticated {
			t.Error("expected unauthenticated")
		}
		if !err.ResetTime.IsZero() {
			t.Error("expected zero reset time")
		}
	})
}

func TestWithSearchProvider(t *testing.T) {
	d := &LLMDiscovery{}
	opt := WithSearchProvider(nil)
	opt(d)
	if d.search != nil {
		t.Error("expected nil search provider")
	}
}

func TestWithLLMFactoryOptions(t *testing.T) {
	d := &LLMDiscovery{}
	opt := WithLLMFactoryOptions()
	opt(d)
	if d.llmFactoryOptions != nil {
		t.Error("expected nil factory options for empty args")
	}
}

// Tests for telemetry emit functions with a disabled client

func TestChainResolver_EmitHitEvent_RegistryHit(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	expected := &DiscoveryResult{
		Builder:    "github",
		Source:     "cli/cli",
		Confidence: ConfidenceRegistry,
	}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithTelemetry(tc)

	result, err := chain.Resolve(context.Background(), "gh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
}

func TestChainResolver_EmitHitEvent_EcosystemHit(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	expected := &DiscoveryResult{
		Builder:    "crates.io",
		Source:     "ripgrep",
		Confidence: ConfidenceEcosystem,
	}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithTelemetry(tc)

	result, err := chain.Resolve(context.Background(), "rg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != ConfidenceEcosystem {
		t.Errorf("got confidence %v, want %v", result.Confidence, ConfidenceEcosystem)
	}
}

func TestChainResolver_EmitHitEvent_LLMHit(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	expected := &DiscoveryResult{
		Builder:    "github",
		Source:     "stripe/stripe-cli",
		Confidence: ConfidenceLLM,
	}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithTelemetry(tc)

	result, err := chain.Resolve(context.Background(), "stripe-cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != ConfidenceLLM {
		t.Errorf("got confidence %v, want %v", result.Confidence, ConfidenceLLM)
	}
}

func TestChainResolver_EmitHitEvent_LLMHitWithMetrics(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	expected := &DiscoveryResult{
		Builder:    "github",
		Source:     "stripe/stripe-cli",
		Confidence: ConfidenceLLM,
		LLMMetrics: &LLMMetrics{
			InputTokens:  100,
			OutputTokens: 200,
			Cost:         0.01,
			Provider:     "anthropic",
			Turns:        3,
		},
	}
	chain := NewChainResolver(
		&mockResolver{result: expected},
	).WithTelemetry(tc)

	result, err := chain.Resolve(context.Background(), "stripe-cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.LLMMetrics == nil {
		t.Error("expected LLMMetrics to be set")
	}
}

func TestChainResolver_EmitNotFoundEvent(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
	).WithTelemetry(tc)

	_, err := chain.Resolve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChainResolver_EmitErrorEvent(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	chain := NewChainResolver(
		&mockResolver{err: &AmbiguousMatchError{Tool: "serve", Matches: []DiscoveryMatch{
			{Builder: "npm", Source: "serve"},
			{Builder: "crates.io", Source: "serve"},
		}}},
	).WithTelemetry(tc)

	_, err := chain.Resolve(context.Background(), "serve")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChainResolver_EmitBudgetExceededEvent(t *testing.T) {
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	chain := NewChainResolver(
		&mockResolver{err: ErrBudgetExceeded},
	).WithTelemetry(tc)

	_, err := chain.Resolve(context.Background(), "sometool")
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
}
