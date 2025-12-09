package llm

import (
	"context"
	"fmt"
	"os"
)

// FailoverCallback is called when the factory fails over from one provider to another.
type FailoverCallback func(from, to, reason string)

// Factory creates and manages LLM providers with circuit breakers.
// It auto-detects available providers from environment variables and
// supports automatic failover when the primary provider is unavailable.
type Factory struct {
	providers  map[string]Provider
	breakers   map[string]*CircuitBreaker
	primary    string
	onFailover FailoverCallback
}

// FactoryOption configures a Factory.
type FactoryOption func(*Factory)

// WithPrimaryProvider sets the preferred provider name.
// If the primary provider is unavailable, the factory falls back to others.
func WithPrimaryProvider(name string) FactoryOption {
	return func(f *Factory) {
		f.primary = name
	}
}

// NewFactory creates a factory with available providers.
// It auto-detects available providers based on environment variables:
// - Claude: Available if ANTHROPIC_API_KEY is set
// - Gemini: Available if GOOGLE_API_KEY or GEMINI_API_KEY is set
//
// Returns an error if no providers are available.
func NewFactory(ctx context.Context, opts ...FactoryOption) (*Factory, error) {
	f := &Factory{
		providers: make(map[string]Provider),
		breakers:  make(map[string]*CircuitBreaker),
		primary:   "claude", // Default primary provider
	}

	// Apply options
	for _, opt := range opts {
		opt(f)
	}

	// Auto-detect and initialize Claude provider
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		provider, err := NewClaudeProvider()
		if err == nil {
			f.providers["claude"] = provider
			f.breakers["claude"] = NewCircuitBreaker("claude")
		}
	}

	// Auto-detect and initialize Gemini provider
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		provider, err := NewGeminiProvider(ctx)
		if err == nil {
			f.providers["gemini"] = provider
			f.breakers["gemini"] = NewCircuitBreaker("gemini")
		}
	}

	if len(f.providers) == 0 {
		return nil, fmt.Errorf("no LLM providers available: set ANTHROPIC_API_KEY or GOOGLE_API_KEY")
	}

	return f, nil
}

// GetProvider returns an available provider, respecting circuit breaker state.
// Returns the primary provider if available and its breaker allows requests.
// Otherwise, falls back to any available provider with an open breaker.
// Returns an error if no providers are available.
func (f *Factory) GetProvider(ctx context.Context) (Provider, error) {
	// Try primary provider first
	if provider, ok := f.providers[f.primary]; ok {
		if breaker := f.breakers[f.primary]; breaker != nil && breaker.Allow() {
			return provider, nil
		}
	}

	// Fallback to any available provider
	for name, provider := range f.providers {
		if name == f.primary {
			continue // Already tried primary
		}
		if breaker := f.breakers[name]; breaker != nil && breaker.Allow() {
			// Notify of failover
			if f.onFailover != nil {
				f.onFailover(f.primary, name, "circuit_breaker_open")
			}
			return provider, nil
		}
	}

	return nil, fmt.Errorf("no LLM providers available: all circuit breakers are open")
}

// SetOnFailover sets the callback to be invoked when provider failover occurs.
func (f *Factory) SetOnFailover(callback FailoverCallback) {
	f.onFailover = callback
}

// SetOnBreakerTrip sets the callback to be invoked when any circuit breaker trips.
func (f *Factory) SetOnBreakerTrip(callback BreakerTripCallback) {
	for _, breaker := range f.breakers {
		breaker.SetOnTrip(callback)
	}
}

// ReportSuccess records a successful operation for the specified provider.
// This resets the circuit breaker failure count and closes the breaker.
func (f *Factory) ReportSuccess(providerName string) {
	if breaker, ok := f.breakers[providerName]; ok {
		breaker.RecordSuccess()
	}
}

// ReportFailure records a failed operation for the specified provider.
// This increments the circuit breaker failure count and may trip the breaker.
func (f *Factory) ReportFailure(providerName string) {
	if breaker, ok := f.breakers[providerName]; ok {
		breaker.RecordFailure()
	}
}

// AvailableProviders returns names of providers whose circuit breakers
// are closed or half-open (i.e., allowing requests).
func (f *Factory) AvailableProviders() []string {
	var available []string
	for name, breaker := range f.breakers {
		if breaker.Allow() {
			available = append(available, name)
		}
	}
	return available
}

// HasProvider returns true if the factory has the specified provider.
func (f *Factory) HasProvider(name string) bool {
	_, ok := f.providers[name]
	return ok
}

// ProviderCount returns the number of registered providers.
func (f *Factory) ProviderCount() int {
	return len(f.providers)
}

// NewFactoryWithProviders creates a factory with the given providers.
// This is useful for testing with mock providers.
func NewFactoryWithProviders(providers map[string]Provider, opts ...FactoryOption) *Factory {
	f := &Factory{
		providers: make(map[string]Provider),
		breakers:  make(map[string]*CircuitBreaker),
		primary:   "claude",
	}

	for _, opt := range opts {
		opt(f)
	}

	for name, provider := range providers {
		f.providers[name] = provider
		f.breakers[name] = NewCircuitBreaker(name)
	}

	return f
}
