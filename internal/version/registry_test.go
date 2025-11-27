package version

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// TestNewRegistry tests registry initialization with default resolvers
func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()

	if reg == nil {
		t.Fatal("NewRegistry() returned nil")
	}

	sources := reg.List()
	if len(sources) != 1 {
		t.Errorf("Expected 1 default resolver, got %d", len(sources))
	}

	// Check that default resolver is registered
	expectedSources := map[string]bool{
		"nodejs_dist": false,
	}

	for _, source := range sources {
		if _, ok := expectedSources[source]; ok {
			expectedSources[source] = true
		}
	}

	for source, found := range expectedSources {
		if !found {
			t.Errorf("Default resolver %q not found in registry", source)
		}
	}
}

// TestRegistry_Resolve_KnownSource tests resolving with known sources
func TestRegistry_Resolve_KnownSource(t *testing.T) {
	reg := NewRegistry()
	resolver := New() // Use New() to get a properly initialized resolver

	ctx := context.Background()

	// Test that nodejs_dist is registered (it will fail since we don't have network,
	// but it should at least find the resolver)
	_, err := reg.Resolve(ctx, resolver, "nodejs_dist")

	// We expect an error (likely network-related), but NOT an "unknown source" error
	if err != nil {
		if resolverErr, ok := err.(*ResolverError); ok {
			if resolverErr.Type == ErrTypeUnknownSource {
				t.Errorf("Resolve() returned unknown source error for known source nodejs_dist")
			}
		}
		// Network errors are expected in unit tests, so we don't fail here
	}
}

// TestRegistry_Resolve_UnknownSource tests error handling for unknown sources
func TestRegistry_Resolve_UnknownSource(t *testing.T) {
	reg := NewRegistry()
	resolver := New() // Use New() to get a properly initialized resolver

	ctx := context.Background()

	_, err := reg.Resolve(ctx, resolver, "unknown_source")

	if err == nil {
		t.Error("Resolve() expected error for unknown source, got nil")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Fatalf("Resolve() error is not ResolverError: %T", err)
	}

	if resolverErr.Type != ErrTypeUnknownSource {
		t.Errorf("Resolve() error type = %v, want %v", resolverErr.Type, ErrTypeUnknownSource)
	}

	if !strings.Contains(resolverErr.Error(), "unknown_source") {
		t.Errorf("Resolve() error = %v, should mention source name", resolverErr.Error())
	}
}

// TestRegistry_Register_NewSource tests registering a new custom resolver
func TestRegistry_Register_NewSource(t *testing.T) {
	reg := NewRegistry()

	// Create a mock resolver function
	mockResolverCalled := false
	mockResolver := func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		mockResolverCalled = true
		return &VersionInfo{
			Tag:     "1.0.0",
			Version: "1.0.0",
		}, nil
	}

	// Register custom resolver
	err := reg.Register("custom_source", mockResolver)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify it's in the list
	sources := reg.List()
	found := false
	for _, source := range sources {
		if source == "custom_source" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Registered source 'custom_source' not found in List()")
	}

	// Test that we can resolve with it
	resolver := New() // Use New() to get a properly initialized resolver
	ctx := context.Background()

	versionInfo, err := reg.Resolve(ctx, resolver, "custom_source")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !mockResolverCalled {
		t.Error("Custom resolver was not called")
	}

	if versionInfo.Version != "1.0.0" {
		t.Errorf("Resolve() version = %v, want 1.0.0", versionInfo.Version)
	}
}

// TestRegistry_Register_DuplicateSource tests error handling for duplicate registration
func TestRegistry_Register_DuplicateSource(t *testing.T) {
	reg := NewRegistry()

	mockResolver := func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		return &VersionInfo{Version: "1.0.0", Tag: "1.0.0"}, nil
	}

	// First registration should succeed
	err := reg.Register("test_source", mockResolver)
	if err != nil {
		t.Fatalf("First Register() error = %v", err)
	}

	// Second registration should fail
	err = reg.Register("test_source", mockResolver)
	if err == nil {
		t.Error("Register() expected error for duplicate source, got nil")
	}

	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("Register() error = %v, should mention 'already registered'", err)
	}
}

// TestRegistry_Register_OverwriteDefault tests that we can't overwrite default resolvers
func TestRegistry_Register_OverwriteDefault(t *testing.T) {
	reg := NewRegistry()

	mockResolver := func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		return &VersionInfo{Version: "999.0.0", Tag: "999.0.0"}, nil
	}

	// Try to overwrite nodejs_dist
	err := reg.Register("nodejs_dist", mockResolver)
	if err == nil {
		t.Error("Register() expected error when trying to overwrite default resolver")
	}
}

// TestRegistry_List tests the List method
func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()

	// Add custom resolvers
	mockResolver := func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		return &VersionInfo{Version: "1.0.0", Tag: "1.0.0"}, nil
	}

	_ = reg.Register("custom1", mockResolver)
	_ = reg.Register("custom2", mockResolver)

	sources := reg.List()

	// Should have 1 default + 2 custom = 3 total
	if len(sources) != 3 {
		t.Errorf("List() returned %d sources, want 3", len(sources))
	}

	// Verify all sources are present
	expectedSources := map[string]bool{
		"nodejs_dist": false,
		"custom1":     false,
		"custom2":     false,
	}

	for _, source := range sources {
		if _, ok := expectedSources[source]; ok {
			expectedSources[source] = true
		}
	}

	for source, found := range expectedSources {
		if !found {
			t.Errorf("Expected source %q not found in List()", source)
		}
	}
}

// TestRegistry_ConcurrentResolve tests thread safety of concurrent Resolve calls
func TestRegistry_ConcurrentResolve(t *testing.T) {
	reg := NewRegistry()
	resolver := New() // Use New() to get a properly initialized resolver

	// Add a custom resolver that we can safely call
	callCount := 0
	var mu sync.Mutex
	_ = reg.Register("test_concurrent", func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return &VersionInfo{Version: "1.0.0", Tag: "1.0.0"}, nil
	})

	ctx := context.Background()
	var wg sync.WaitGroup
	goroutines := 10
	iterations := 10

	// Launch multiple goroutines that resolve concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := reg.Resolve(ctx, resolver, "test_concurrent")
				if err != nil {
					t.Errorf("Resolve() error = %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Verify all calls completed
	mu.Lock()
	defer mu.Unlock()
	expectedCalls := goroutines * iterations
	if callCount != expectedCalls {
		t.Errorf("Expected %d calls, got %d", expectedCalls, callCount)
	}
}

// TestRegistry_ConcurrentRegister tests thread safety of concurrent Register calls
func TestRegistry_ConcurrentRegister(t *testing.T) {
	reg := NewRegistry()

	var wg sync.WaitGroup
	goroutines := 10
	errors := make([]error, goroutines)

	mockResolver := func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		return &VersionInfo{Version: "1.0.0", Tag: "1.0.0"}, nil
	}

	// Launch multiple goroutines trying to register the same source
	// Only one should succeed
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errors[idx] = reg.Register("concurrent_test", mockResolver)
		}(i)
	}

	wg.Wait()

	// Count successes and failures
	successCount := 0
	failCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		} else {
			failCount++
		}
	}

	// Exactly one should succeed, the rest should fail
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful registration, got %d", successCount)
	}
	if failCount != goroutines-1 {
		t.Errorf("Expected %d failed registrations, got %d", goroutines-1, failCount)
	}
}

// TestRegistry_ConcurrentList tests thread safety of List calls
func TestRegistry_ConcurrentList(t *testing.T) {
	reg := NewRegistry()

	var wg sync.WaitGroup
	goroutines := 10

	// Launch multiple goroutines calling List() concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				sources := reg.List()
				// Should always have at least 1 default resolver (nodejs_dist)
				if len(sources) < 1 {
					t.Errorf("List() returned %d sources, expected at least 1", len(sources))
				}
			}
		}()
	}

	wg.Wait()
}

// TestRegistry_ConcurrentMixedOperations tests thread safety of mixed operations
func TestRegistry_ConcurrentMixedOperations(t *testing.T) {
	reg := NewRegistry()
	resolver := New() // Use New() to get a properly initialized resolver

	ctx := context.Background()
	var wg sync.WaitGroup

	mockResolver := func(ctx context.Context, r *Resolver) (*VersionInfo, error) {
		return &VersionInfo{Version: "1.0.0", Tag: "1.0.0"}, nil
	}

	// Register some initial sources
	for i := 0; i < 5; i++ {
		_ = reg.Register(strings.Join([]string{"source", strings.Repeat("x", i)}, "_"), mockResolver)
	}

	// Launch goroutines doing different operations
	for i := 0; i < 10; i++ {
		wg.Add(3)

		// Reader: Resolve
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = reg.Resolve(ctx, resolver, "nodejs_dist")
			}
		}()

		// Reader: List
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				reg.List()
			}
		}()

		// Writer: Register (will mostly fail due to duplicates, but that's OK)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				sourceName := strings.Join([]string{"concurrent", strings.Repeat("y", idx), strings.Repeat("z", j)}, "_")
				_ = reg.Register(sourceName, mockResolver)
			}
		}(i)
	}

	wg.Wait()

	// Verify registry is still functional
	sources := reg.List()
	if len(sources) < 1 {
		t.Errorf("Registry corrupted: List() returned %d sources, expected at least 1", len(sources))
	}
}
