package recipe

import "testing"

// mockVersionValidator implements VersionValidator for testing
type mockVersionValidator struct {
	canResolve   bool
	sources      []string
	validateErr  error
	validateFunc func(r *Recipe) error
}

func (m *mockVersionValidator) CanResolveVersion(r *Recipe) bool {
	return m.canResolve
}

func (m *mockVersionValidator) KnownSources() []string {
	return m.sources
}

func (m *mockVersionValidator) ValidateVersionConfig(r *Recipe) error {
	if m.validateFunc != nil {
		return m.validateFunc(r)
	}
	return m.validateErr
}

func TestSetAndGetVersionValidator(t *testing.T) {
	// Save and restore original validator
	origValidator := GetVersionValidator()
	defer SetVersionValidator(origValidator)

	// Initially should be nil (or whatever was set before)
	// We don't test this since other tests may have set it

	// Set a mock validator
	mock := &mockVersionValidator{
		canResolve: true,
		sources:    []string{"pypi", "npm", "github_releases"},
	}
	SetVersionValidator(mock)

	// Should get the mock back
	got := GetVersionValidator()
	if got != mock {
		t.Error("expected to get the mock validator back")
	}

	// Verify interface methods work
	if !got.CanResolveVersion(nil) {
		t.Error("expected CanResolveVersion to return true")
	}

	sources := got.KnownSources()
	if len(sources) != 3 {
		t.Errorf("expected 3 sources, got %d", len(sources))
	}

	if err := got.ValidateVersionConfig(nil); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestVersionValidatorThreadSafety(t *testing.T) {
	// Save and restore original validator
	origValidator := GetVersionValidator()
	defer SetVersionValidator(origValidator)

	// Run concurrent gets and sets
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			mock := &mockVersionValidator{
				sources: []string{string(rune('a' + n))},
			}
			SetVersionValidator(mock)
			_ = GetVersionValidator()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
