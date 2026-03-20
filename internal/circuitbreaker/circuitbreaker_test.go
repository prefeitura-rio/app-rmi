package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewCircuitBreaker(t *testing.T) {
	logger := zap.NewNop()
	cb := NewCircuitBreaker("test", Settings{}, logger)

	if cb == nil {
		t.Fatal("Expected circuit breaker to be created")
	}

	if cb.name != "test" {
		t.Errorf("Expected name 'test', got '%s'", cb.name)
	}

	if cb.maxRequests != 1 {
		t.Errorf("Expected default maxRequests 1, got %d", cb.maxRequests)
	}

	if cb.timeout != 60*time.Second {
		t.Errorf("Expected default timeout 60s, got %v", cb.timeout)
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected initial state Closed, got %v", cb.State())
	}
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	logger := zap.NewNop()

	stateChanges := []struct {
		from State
		to   State
	}{}

	cb := NewCircuitBreaker("test", Settings{
		MaxRequests: 3,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from State, to State) {
			stateChanges = append(stateChanges, struct {
				from State
				to   State
			}{from, to})
		},
	}, logger)

	ctx := context.Background()

	// Initial state should be closed
	if cb.State() != StateClosed {
		t.Errorf("Expected initial state Closed, got %v", cb.State())
	}

	// Trigger 3 failures to open the circuit
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(ctx, func() (interface{}, error) {
			return nil, errors.New("test error")
		})
		if err == nil {
			t.Error("Expected error from failing operation")
		}
	}

	// Circuit should be open now
	if cb.State() != StateOpen {
		t.Errorf("Expected state Open after failures, got %v", cb.State())
	}

	// Request should fail immediately when circuit is open
	_, err := cb.Execute(ctx, func() (interface{}, error) {
		t.Error("Should not execute when circuit is open")
		return nil, nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}

	// Wait for timeout to transition to half-open
	time.Sleep(150 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state HalfOpen after timeout, got %v", cb.State())
	}

	// Successful requests in half-open should close the circuit
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(ctx, func() (interface{}, error) {
			return "success", nil
		})
		if err != nil {
			t.Errorf("Expected no error in half-open state, got %v", err)
		}
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected state Closed after successes, got %v", cb.State())
	}

	// Verify state transitions
	expectedTransitions := 2 // Open -> HalfOpen, HalfOpen -> Closed
	if len(stateChanges) < expectedTransitions {
		t.Errorf("Expected at least %d state transitions, got %d", expectedTransitions, len(stateChanges))
	}
}

func TestCircuitBreakerCounts(t *testing.T) {
	logger := zap.NewNop()
	cb := NewCircuitBreaker("test", Settings{
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}, logger)

	ctx := context.Background()

	// Execute successful requests
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(ctx, func() (interface{}, error) {
			return nil, nil
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	counts := cb.Counts()
	if counts.TotalSuccesses != 3 {
		t.Errorf("Expected 3 successes, got %d", counts.TotalSuccesses)
	}
	if counts.ConsecutiveSuccesses != 3 {
		t.Errorf("Expected 3 consecutive successes, got %d", counts.ConsecutiveSuccesses)
	}

	// Execute a failed request
	_, err := cb.Execute(ctx, func() (interface{}, error) {
		return nil, errors.New("failure")
	})
	if err == nil {
		t.Error("Expected error from failing operation")
	}

	counts = cb.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", counts.TotalFailures)
	}
	if counts.ConsecutiveFailures != 1 {
		t.Errorf("Expected 1 consecutive failure, got %d", counts.ConsecutiveFailures)
	}
	if counts.ConsecutiveSuccesses != 0 {
		t.Errorf("Expected consecutive successes reset to 0, got %d", counts.ConsecutiveSuccesses)
	}
}

func TestCircuitBreakerMaxRequests(t *testing.T) {
	logger := zap.NewNop()
	maxRequests := uint32(2)

	cb := NewCircuitBreaker("test", Settings{
		MaxRequests: maxRequests,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	}, logger)

	ctx := context.Background()

	// Trigger failure to open circuit
	_, _ = cb.Execute(ctx, func() (interface{}, error) {
		return nil, errors.New("failure")
	})

	// Verify circuit is open
	if cb.State() != StateOpen {
		t.Errorf("Expected circuit to be open, got %v", cb.State())
	}

	// Wait for half-open
	time.Sleep(150 * time.Millisecond)

	// Verify circuit is half-open
	if cb.State() != StateHalfOpen {
		t.Errorf("Expected circuit to be half-open, got %v", cb.State())
	}

	// Execute up to maxRequests - these will succeed and close the circuit
	successCount := 0
	for i := uint32(0); i < maxRequests; i++ {
		_, err := cb.Execute(ctx, func() (interface{}, error) {
			return nil, nil
		})
		if err != nil {
			t.Errorf("Request %d: unexpected error %v", i, err)
		} else {
			successCount++
		}
	}

	// After maxRequests successes in half-open, circuit should be closed
	if cb.State() != StateClosed {
		t.Errorf("After %d successful requests in half-open state, circuit should be closed, got %v", successCount, cb.State())
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateHalfOpen, "half-open"},
		{StateOpen, "open"},
		{State(999), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}
