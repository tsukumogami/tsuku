package llm

import (
	"context"
	"os"
	"testing"
)

// mockProvider is a simple mock implementation of Provider for testing.
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	return &CompletionResponse{Content: "mock response"}, nil
}

func TestNewFactoryNoProviders(t *testing.T) {
	// Clear all API keys
	originalAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	originalGoogle := os.Getenv("GOOGLE_API_KEY")
	originalGemini := os.Getenv("GEMINI_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_ = os.Unsetenv("GOOGLE_API_KEY")
	_ = os.Unsetenv("GEMINI_API_KEY")
	defer func() {
		_ = os.Setenv("ANTHROPIC_API_KEY", originalAnthropic)
		_ = os.Setenv("GOOGLE_API_KEY", originalGoogle)
		_ = os.Setenv("GEMINI_API_KEY", originalGemini)
	}()

	ctx := context.Background()
	_, err := NewFactory(ctx)
	if err == nil {
		t.Error("NewFactory should fail when no API keys are set")
	}
}

func TestNewFactoryWithProviders(t *testing.T) {
	providers := map[string]Provider{
		"mock1": &mockProvider{name: "mock1"},
		"mock2": &mockProvider{name: "mock2"},
	}

	factory := NewFactoryWithProviders(providers)

	if factory.ProviderCount() != 2 {
		t.Errorf("ProviderCount() = %d, want 2", factory.ProviderCount())
	}

	if !factory.HasProvider("mock1") {
		t.Error("Factory should have mock1 provider")
	}

	if !factory.HasProvider("mock2") {
		t.Error("Factory should have mock2 provider")
	}
}

func TestNewFactoryWithPrimaryOption(t *testing.T) {
	providers := map[string]Provider{
		"mock1": &mockProvider{name: "mock1"},
		"mock2": &mockProvider{name: "mock2"},
	}

	factory := NewFactoryWithProviders(providers, WithPrimaryProvider("mock2"))

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	if provider.Name() != "mock2" {
		t.Errorf("GetProvider returned %q, want %q", provider.Name(), "mock2")
	}
}

func TestGetProviderReturnsPrimary(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers)

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// Default primary is claude
	if provider.Name() != "claude" {
		t.Errorf("GetProvider returned %q, want %q", provider.Name(), "claude")
	}
}

func TestGetProviderFallsBackWhenPrimaryTripped(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers)

	// Trip the primary (claude) breaker
	for i := 0; i < 3; i++ {
		factory.ReportFailure("claude")
	}

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// Should fall back to gemini
	if provider.Name() != "gemini" {
		t.Errorf("GetProvider returned %q, want %q after primary tripped", provider.Name(), "gemini")
	}
}

func TestGetProviderFailsWhenAllTripped(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers)

	// Trip both breakers
	for i := 0; i < 3; i++ {
		factory.ReportFailure("claude")
		factory.ReportFailure("gemini")
	}

	ctx := context.Background()
	_, err := factory.GetProvider(ctx)
	if err == nil {
		t.Error("GetProvider should fail when all breakers are tripped")
	}
}

func TestGetProviderWithSingleProvider(t *testing.T) {
	providers := map[string]Provider{
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers)

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// Should return the only available provider even though it's not primary
	if provider.Name() != "gemini" {
		t.Errorf("GetProvider returned %q, want %q", provider.Name(), "gemini")
	}
}

func TestReportSuccessResetsBreakerState(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
	}

	factory := NewFactoryWithProviders(providers)

	// Record some failures (but not enough to trip)
	factory.ReportFailure("claude")
	factory.ReportFailure("claude")

	// Report success should reset
	factory.ReportSuccess("claude")

	// Record one more failure - if reset worked, breaker should still allow
	factory.ReportFailure("claude")

	ctx := context.Background()
	_, err := factory.GetProvider(ctx)
	if err != nil {
		t.Error("GetProvider should succeed after ReportSuccess reset the breaker")
	}
}

func TestReportFailureTripsBreaker(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
	}

	factory := NewFactoryWithProviders(providers)

	// Trip the breaker with 3 failures
	factory.ReportFailure("claude")
	factory.ReportFailure("claude")
	factory.ReportFailure("claude")

	ctx := context.Background()
	_, err := factory.GetProvider(ctx)
	if err == nil {
		t.Error("GetProvider should fail after breaker is tripped")
	}
}

func TestAvailableProviders(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers)

	available := factory.AvailableProviders()
	if len(available) != 2 {
		t.Errorf("AvailableProviders() returned %d providers, want 2", len(available))
	}

	// Trip one breaker
	for i := 0; i < 3; i++ {
		factory.ReportFailure("claude")
	}

	available = factory.AvailableProviders()
	if len(available) != 1 {
		t.Errorf("AvailableProviders() returned %d providers after one tripped, want 1", len(available))
	}

	if available[0] != "gemini" {
		t.Errorf("AvailableProviders()[0] = %q, want %q", available[0], "gemini")
	}
}

func TestHasProvider(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
	}

	factory := NewFactoryWithProviders(providers)

	if !factory.HasProvider("claude") {
		t.Error("HasProvider(\"claude\") should return true")
	}

	if factory.HasProvider("gemini") {
		t.Error("HasProvider(\"gemini\") should return false")
	}

	if factory.HasProvider("nonexistent") {
		t.Error("HasProvider(\"nonexistent\") should return false")
	}
}

func TestProviderCount(t *testing.T) {
	tests := []struct {
		name      string
		providers map[string]Provider
		want      int
	}{
		{
			name:      "empty",
			providers: map[string]Provider{},
			want:      0,
		},
		{
			name: "single",
			providers: map[string]Provider{
				"mock": &mockProvider{name: "mock"},
			},
			want: 1,
		},
		{
			name: "multiple",
			providers: map[string]Provider{
				"mock1": &mockProvider{name: "mock1"},
				"mock2": &mockProvider{name: "mock2"},
				"mock3": &mockProvider{name: "mock3"},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := NewFactoryWithProviders(tt.providers)
			if got := factory.ProviderCount(); got != tt.want {
				t.Errorf("ProviderCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReportSuccessUnknownProvider(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
	}

	factory := NewFactoryWithProviders(providers)

	// Should not panic when reporting for unknown provider
	factory.ReportSuccess("nonexistent")
}

func TestReportFailureUnknownProvider(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
	}

	factory := NewFactoryWithProviders(providers)

	// Should not panic when reporting for unknown provider
	factory.ReportFailure("nonexistent")
}

func TestGetProviderEmptyFactory(t *testing.T) {
	factory := NewFactoryWithProviders(map[string]Provider{})

	ctx := context.Background()
	_, err := factory.GetProvider(ctx)
	if err == nil {
		t.Error("GetProvider should fail with empty factory")
	}
}

func TestSetOnBreakerTrip(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers)

	var callbackCalled bool
	var callbackProvider string
	var callbackFailures int

	factory.SetOnBreakerTrip(func(provider string, failures int) {
		callbackCalled = true
		callbackProvider = provider
		callbackFailures = failures
	})

	// Trip the claude breaker
	factory.ReportFailure("claude")
	factory.ReportFailure("claude")

	if callbackCalled {
		t.Error("callback should not be called before threshold")
	}

	factory.ReportFailure("claude") // Third failure should trip

	if !callbackCalled {
		t.Error("callback should be called when breaker trips")
	}
	if callbackProvider != "claude" {
		t.Errorf("provider = %q, want %q", callbackProvider, "claude")
	}
	if callbackFailures != 3 {
		t.Errorf("failures = %d, want %d", callbackFailures, 3)
	}
}

func TestSetOnBreakerTripNilCallback(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
	}

	factory := NewFactoryWithProviders(providers)

	// Setting nil callback should not panic
	factory.SetOnBreakerTrip(nil)

	// Trip breaker - should not panic
	factory.ReportFailure("claude")
	factory.ReportFailure("claude")
	factory.ReportFailure("claude")

	// Breaker should still trip even without callback
	ctx := context.Background()
	_, err := factory.GetProvider(ctx)
	if err == nil {
		t.Error("GetProvider should fail after breaker trips")
	}
}

// mockLLMConfig is a mock implementation of LLMConfig for testing.
type mockLLMConfig struct {
	enabled   bool
	providers []string
}

func (m *mockLLMConfig) LLMEnabled() bool {
	return m.enabled
}

func (m *mockLLMConfig) LLMProviders() []string {
	return m.providers
}

func TestNewFactoryWithConfigDisabled(t *testing.T) {
	// Clear all API keys to avoid actual provider creation
	originalAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	originalGoogle := os.Getenv("GOOGLE_API_KEY")
	originalGemini := os.Getenv("GEMINI_API_KEY")
	_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer func() {
		_ = os.Setenv("ANTHROPIC_API_KEY", originalAnthropic)
		_ = os.Setenv("GOOGLE_API_KEY", originalGoogle)
		_ = os.Setenv("GEMINI_API_KEY", originalGemini)
	}()

	cfg := &mockLLMConfig{enabled: false}
	ctx := context.Background()
	_, err := NewFactory(ctx, WithConfig(cfg))
	if err != ErrLLMDisabled {
		t.Errorf("NewFactory with disabled config should return ErrLLMDisabled, got %v", err)
	}
}

func TestNewFactoryWithConfigEnabled(t *testing.T) {
	// Set API key to allow provider creation
	originalAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer func() {
		_ = os.Setenv("ANTHROPIC_API_KEY", originalAnthropic)
	}()

	cfg := &mockLLMConfig{enabled: true}
	ctx := context.Background()
	factory, err := NewFactory(ctx, WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewFactory with enabled config should succeed, got %v", err)
	}
	if factory == nil {
		t.Error("Factory should not be nil")
	}
}

func TestWithConfigProviderOrder(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	cfg := &mockLLMConfig{enabled: true, providers: []string{"gemini", "claude"}}
	factory := NewFactoryWithProviders(providers, WithConfig(cfg))

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// gemini should be primary now
	if provider.Name() != "gemini" {
		t.Errorf("GetProvider returned %q, want %q (from config)", provider.Name(), "gemini")
	}
}

func TestWithEnabledFalse(t *testing.T) {
	originalAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer func() {
		_ = os.Setenv("ANTHROPIC_API_KEY", originalAnthropic)
	}()

	ctx := context.Background()
	_, err := NewFactory(ctx, WithEnabled(false))
	if err != ErrLLMDisabled {
		t.Errorf("NewFactory with WithEnabled(false) should return ErrLLMDisabled, got %v", err)
	}
}

func TestWithEnabledTrue(t *testing.T) {
	originalAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer func() {
		_ = os.Setenv("ANTHROPIC_API_KEY", originalAnthropic)
	}()

	ctx := context.Background()
	factory, err := NewFactory(ctx, WithEnabled(true))
	if err != nil {
		t.Fatalf("NewFactory with WithEnabled(true) should succeed, got %v", err)
	}
	if factory == nil {
		t.Error("Factory should not be nil")
	}
}

func TestWithProviderOrder(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	factory := NewFactoryWithProviders(providers, WithProviderOrder([]string{"gemini", "claude"}))

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// gemini should be primary
	if provider.Name() != "gemini" {
		t.Errorf("GetProvider returned %q, want %q", provider.Name(), "gemini")
	}
}

func TestWithProviderOrderEmpty(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	// Empty provider order should not change the default
	factory := NewFactoryWithProviders(providers, WithProviderOrder(nil))

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// claude should still be primary (default)
	if provider.Name() != "claude" {
		t.Errorf("GetProvider returned %q, want %q (default)", provider.Name(), "claude")
	}
}

func TestWithConfigEmptyProviders(t *testing.T) {
	providers := map[string]Provider{
		"claude": &mockProvider{name: "claude"},
		"gemini": &mockProvider{name: "gemini"},
	}

	cfg := &mockLLMConfig{enabled: true, providers: nil}
	factory := NewFactoryWithProviders(providers, WithConfig(cfg))

	ctx := context.Background()
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	// claude should still be primary (default when config has no providers)
	if provider.Name() != "claude" {
		t.Errorf("GetProvider returned %q, want %q (default)", provider.Name(), "claude")
	}
}
