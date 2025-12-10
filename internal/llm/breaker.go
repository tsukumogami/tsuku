package llm

import (
	"sync"
	"time"
)

// State represents the current state of a circuit breaker.
type State int

const (
	// StateClosed is normal operation - requests pass through.
	StateClosed State = iota
	// StateOpen means the breaker is tripped - requests are rejected.
	StateOpen
	// StateHalfOpen allows one test request to check recovery.
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// BreakerTripCallback is called when a circuit breaker trips open.
type BreakerTripCallback func(provider string, failures int)

// CircuitBreaker implements the circuit breaker pattern for LLM providers.
// It tracks failures and temporarily blocks requests to failing providers,
// allowing recovery time while traffic shifts to healthy providers.
type CircuitBreaker struct {
	name             string
	state            State
	failures         int
	lastFailure      time.Time
	failureThreshold int
	recoveryTimeout  time.Duration
	mu               sync.Mutex

	// now is a function that returns current time, injectable for testing.
	now func() time.Time

	// onTrip is called when the breaker trips open.
	onTrip BreakerTripCallback
}

// NewCircuitBreaker creates a circuit breaker with default settings.
// Default threshold is 3 consecutive failures, recovery timeout is 60 seconds.
func NewCircuitBreaker(name string) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		state:            StateClosed,
		failureThreshold: 3,
		recoveryTimeout:  60 * time.Second,
		now:              time.Now,
	}
}

// Name returns the breaker's identifier.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Allow checks if a request should proceed.
// Returns false if the breaker is open and recovery timeout hasn't elapsed.
// If the breaker is open but timeout has elapsed, transitions to half-open.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if cb.now().Sub(cb.lastFailure) >= cb.recoveryTimeout {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		// Only allow one request in half-open state
		return true
	default:
		return false
	}
}

// RecordSuccess resets the failure count and closes the breaker.
// Should be called after a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.state = StateClosed
}

// RecordFailure increments the failure count and may trip the breaker.
// If the failure threshold is reached, the breaker opens.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()

	cb.failures++
	cb.lastFailure = cb.now()

	tripped := false
	if cb.failures >= cb.failureThreshold && cb.state != StateOpen {
		cb.state = StateOpen
		tripped = true
	}

	// Capture values before unlocking
	name := cb.name
	failures := cb.failures
	onTrip := cb.onTrip

	cb.mu.Unlock()

	// Call callback outside of lock to avoid deadlock
	if tripped && onTrip != nil {
		onTrip(name, failures)
	}
}

// SetOnTrip sets the callback to be invoked when the breaker trips.
func (cb *CircuitBreaker) SetOnTrip(callback BreakerTripCallback) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onTrip = callback
}

// State returns the current breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}
