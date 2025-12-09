package llm

import (
	"sync"
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker("test")

	if cb.Name() != "test" {
		t.Errorf("expected name 'test', got %q", cb.Name())
	}
	if cb.State() != StateClosed {
		t.Errorf("expected initial state StateClosed, got %v", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected initial failures 0, got %d", cb.Failures())
	}
}

func TestCircuitBreakerAllowWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker("test")

	if !cb.Allow() {
		t.Error("expected Allow() to return true when closed")
	}
}

func TestCircuitBreakerTripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Record failures up to threshold
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Errorf("expected state StateClosed after 2 failures, got %v", cb.State())
	}

	cb.RecordFailure() // Third failure should trip
	if cb.State() != StateOpen {
		t.Errorf("expected state StateOpen after 3 failures, got %v", cb.State())
	}
	if cb.Failures() != 3 {
		t.Errorf("expected 3 failures, got %d", cb.Failures())
	}
}

func TestCircuitBreakerRejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.Allow() {
		t.Error("expected Allow() to return false when open")
	}
}

func TestCircuitBreakerRecoverySuccess(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("expected state StateOpen, got %v", cb.State())
	}

	// Success resets state
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("expected state StateClosed after success, got %v", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after success, got %d", cb.Failures())
	}
	if !cb.Allow() {
		t.Error("expected Allow() to return true after recovery")
	}
}

func TestCircuitBreakerTransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Use mock time
	mockTime := time.Now()
	cb.now = func() time.Time { return mockTime }

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("expected state StateOpen, got %v", cb.State())
	}

	// Before timeout: should be rejected
	mockTime = mockTime.Add(30 * time.Second)
	if cb.Allow() {
		t.Error("expected Allow() to return false before recovery timeout")
	}

	// After timeout: should transition to half-open
	mockTime = mockTime.Add(31 * time.Second) // Total: 61 seconds
	if !cb.Allow() {
		t.Error("expected Allow() to return true after recovery timeout")
	}
	if cb.State() != StateHalfOpen {
		t.Errorf("expected state StateHalfOpen, got %v", cb.State())
	}
}

func TestCircuitBreakerHalfOpenToClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Use mock time
	mockTime := time.Now()
	cb.now = func() time.Time { return mockTime }

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Advance past recovery timeout
	mockTime = mockTime.Add(61 * time.Second)
	cb.Allow() // Transitions to half-open

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state StateHalfOpen, got %v", cb.State())
	}

	// Success in half-open should close
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("expected state StateClosed after success in half-open, got %v", cb.State())
	}
}

func TestCircuitBreakerHalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Use mock time
	mockTime := time.Now()
	cb.now = func() time.Time { return mockTime }

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Advance past recovery timeout
	mockTime = mockTime.Add(61 * time.Second)
	cb.Allow() // Transitions to half-open

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state StateHalfOpen, got %v", cb.State())
	}

	// Failure in half-open should trip again (failures now at 4)
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("expected state StateOpen after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreakerConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker("test")

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent failures
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.RecordFailure()
		}()
	}

	// Concurrent successes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.RecordSuccess()
		}()
	}

	// Concurrent Allow checks
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
		}()
	}

	// Concurrent State checks
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.State()
		}()
	}

	wg.Wait()
	// If we get here without panic or race condition, test passes
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.expected {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestCircuitBreakerSuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker("test")

	// Accumulate some failures (not enough to trip)
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.Failures() != 2 {
		t.Errorf("expected 2 failures, got %d", cb.Failures())
	}

	// Success should reset
	cb.RecordSuccess()

	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after success, got %d", cb.Failures())
	}

	// Now should need full threshold again to trip
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Errorf("expected state StateClosed after 2 new failures, got %v", cb.State())
	}

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("expected state StateOpen after 3 new failures, got %v", cb.State())
	}
}
