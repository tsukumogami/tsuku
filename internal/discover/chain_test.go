package discover

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// mockResolver returns a fixed result or error.
type mockResolver struct {
	result *DiscoveryResult
	err    error
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (*DiscoveryResult, error) {
	return m.result, m.err
}

func TestChainResolver_FirstHit(t *testing.T) {
	expected := &DiscoveryResult{Builder: "github", Source: "cli/cli", Confidence: ConfidenceRegistry}
	chain := NewChainResolver(
		&mockResolver{result: expected},
		&mockResolver{result: &DiscoveryResult{Builder: "npm", Source: "gh"}},
	)
	result, err := chain.Resolve(context.Background(), "gh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
}

func TestChainResolver_MissThenHit(t *testing.T) {
	expected := &DiscoveryResult{Builder: "crates.io", Source: "", Confidence: ConfidenceEcosystem}
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil}, // miss
		&mockResolver{result: expected},
	)
	result, err := chain.Resolve(context.Background(), "ripgrep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "crates.io" {
		t.Errorf("got builder %q, want %q", result.Builder, "crates.io")
	}
}

func TestChainResolver_AllMiss(t *testing.T) {
	// When HasAPIKey is true, return NotFoundError
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: nil, err: nil},
	).WithLLMAvailability(LLMAvailability{HasAPIKey: true})
	_, err := chain.Resolve(context.Background(), "nonexistent")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
	if notFound.Tool != "nonexistent" {
		t.Errorf("got tool %q, want %q", notFound.Tool, "nonexistent")
	}
}

func TestChainResolver_AllMiss_NoAPIKey(t *testing.T) {
	// When HasAPIKey is false, return ConfigurationError
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: nil, err: nil},
	).WithLLMAvailability(LLMAvailability{HasAPIKey: false})
	_, err := chain.Resolve(context.Background(), "nonexistent")
	var configErr *ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected ConfigurationError, got %v", err)
	}
	if configErr.Reason != "no_api_key" {
		t.Errorf("got reason %q, want %q", configErr.Reason, "no_api_key")
	}
}

func TestChainResolver_AllMiss_DeterministicOnly(t *testing.T) {
	// When DeterministicOnly is true, return ConfigurationError
	chain := NewChainResolver(
		&mockResolver{result: nil, err: nil},
		&mockResolver{result: nil, err: nil},
	).WithLLMAvailability(LLMAvailability{DeterministicOnly: true, HasAPIKey: true})
	_, err := chain.Resolve(context.Background(), "nonexistent")
	var configErr *ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected ConfigurationError, got %v", err)
	}
	if configErr.Reason != "deterministic_only" {
		t.Errorf("got reason %q, want %q", configErr.Reason, "deterministic_only")
	}
}

func TestChainResolver_SoftErrorContinues(t *testing.T) {
	expected := &DiscoveryResult{Builder: "github", Source: "foo/bar", Confidence: ConfidenceEcosystem}
	chain := NewChainResolver(
		&mockResolver{err: errors.New("timeout")}, // soft error
		&mockResolver{result: expected},
	)
	result, err := chain.Resolve(context.Background(), "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
}

func TestChainResolver_CancelledContextStopsChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	chain := NewChainResolver(
		&mockResolver{err: ctx.Err()},
		&mockResolver{result: &DiscoveryResult{Builder: "should-not-reach"}},
	)
	_, err := chain.Resolve(ctx, "anything")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestChainResolver_NormalizesInput(t *testing.T) {
	var captured string
	capturingResolver := &capturingMock{captured: &captured}
	chain := NewChainResolver(capturingResolver)
	_, _ = chain.Resolve(context.Background(), "  MyTool  ")
	if captured != "mytool" {
		t.Errorf("got %q, want %q", captured, "mytool")
	}
}

type capturingMock struct {
	captured *string
}

func (m *capturingMock) Resolve(_ context.Context, name string) (*DiscoveryResult, error) {
	*m.captured = name
	return nil, nil
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

func TestChainResolver_EmitHitEvent(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		result *DiscoveryResult
		check  func(t *testing.T, result *DiscoveryResult)
	}{
		{
			name: "RegistryHit",
			tool: "gh",
			result: &DiscoveryResult{
				Builder:    "github",
				Source:     "cli/cli",
				Confidence: ConfidenceRegistry,
			},
			check: func(t *testing.T, result *DiscoveryResult) {
				t.Helper()
				if result.Builder != "github" {
					t.Errorf("got builder %q, want %q", result.Builder, "github")
				}
			},
		},
		{
			name: "EcosystemHit",
			tool: "rg",
			result: &DiscoveryResult{
				Builder:    "crates.io",
				Source:     "ripgrep",
				Confidence: ConfidenceEcosystem,
			},
			check: func(t *testing.T, result *DiscoveryResult) {
				t.Helper()
				if result.Confidence != ConfidenceEcosystem {
					t.Errorf("got confidence %v, want %v", result.Confidence, ConfidenceEcosystem)
				}
			},
		},
		{
			name: "LLMHit",
			tool: "stripe-cli",
			result: &DiscoveryResult{
				Builder:    "github",
				Source:     "stripe/stripe-cli",
				Confidence: ConfidenceLLM,
			},
			check: func(t *testing.T, result *DiscoveryResult) {
				t.Helper()
				if result.Confidence != ConfidenceLLM {
					t.Errorf("got confidence %v, want %v", result.Confidence, ConfidenceLLM)
				}
			},
		},
		{
			name: "LLMHitWithMetrics",
			tool: "stripe-cli",
			result: &DiscoveryResult{
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
			},
			check: func(t *testing.T, result *DiscoveryResult) {
				t.Helper()
				if result.LLMMetrics == nil {
					t.Error("expected LLMMetrics to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := telemetry.NewClientWithOptions("", 0, true, false)
			chain := NewChainResolver(
				&mockResolver{result: tt.result},
			).WithTelemetry(tc)

			result, err := chain.Resolve(context.Background(), tt.tool)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, result)
		})
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

func testLogger(buf *bytes.Buffer) log.Logger {
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return log.New(h)
}
