package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// Settings configure the circuit breaker behavior
type Settings struct {
	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is half-open.
	// If MaxRequests is 0, the circuit breaker allows only 1 request.
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for the circuit breaker
	// to clear the internal counts.
	// If Interval is 0, the circuit breaker doesn't clear internal counts during the closed state.
	Interval time.Duration

	// Timeout is the period of the open state, after which the state becomes half-open.
	// If Timeout is 0, the timeout value defaults to 60 seconds.
	Timeout time.Duration

	// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
	// If ReadyToTrip returns true, the circuit breaker will be placed into the open state.
	// If ReadyToTrip is nil, default readiness logic is used.
	ReadyToTrip func(counts Counts) bool

	// OnStateChange is called whenever the state of the circuit breaker changes.
	OnStateChange func(name string, from State, to State)
}

// Counts holds the numbers of requests and their successes/failures
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name          string
	maxRequests   uint32
	interval      time.Duration
	timeout       time.Duration
	readyToTrip   func(counts Counts) bool
	onStateChange func(name string, from State, to State)

	mutex      sync.Mutex
	state      State
	generation uint64
	counts     Counts
	expiry     time.Time
	logger     *zap.Logger
}

// NewCircuitBreaker returns a new CircuitBreaker configured with the given Settings.
func NewCircuitBreaker(name string, settings Settings, logger *zap.Logger) *CircuitBreaker {
	// Default to no-op logger if nil is provided
	if logger == nil {
		logger = zap.NewNop()
	}

	cb := &CircuitBreaker{
		name:          name,
		maxRequests:   settings.MaxRequests,
		interval:      settings.Interval,
		timeout:       settings.Timeout,
		readyToTrip:   settings.ReadyToTrip,
		onStateChange: settings.OnStateChange,
		logger:        logger,
	}

	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}

	if cb.timeout == 0 {
		cb.timeout = 60 * time.Second
	}

	if cb.readyToTrip == nil {
		cb.readyToTrip = defaultReadyToTrip
	}

	cb.toNewGeneration(time.Now())

	return cb
}

// defaultReadyToTrip returns true when the number of consecutive failures is 5 or more.
func defaultReadyToTrip(counts Counts) bool {
	return counts.ConsecutiveFailures >= 5
}

// Execute runs the given request if the circuit breaker accepts it.
// Execute returns an error instantly if the circuit breaker rejects the request.
// Otherwise, Execute returns the result of the request.
func (cb *CircuitBreaker) Execute(ctx context.Context, req func() (interface{}, error)) (interface{}, error) {
	// Fast-path: return immediately if context is already canceled/exceeded
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	generation, err := cb.beforeRequest()
	if err != nil {
		return nil, err
	}

	defer func() {
		if e := recover(); e != nil {
			cb.afterRequest(generation, false)
			panic(e)
		}
	}()

	result, err := req()
	cb.afterRequest(generation, err == nil)
	return result, err
}

func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.maxRequests {
		return generation, ErrTooManyRequests
	}

	cb.counts.Requests++
	return generation, nil
}

func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0

	if state == StateHalfOpen && cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
		// Only close after maxRequests consecutive successes in half-open state
		cb.logger.Info("circuit breaker transitioning to closed",
			zap.String("name", cb.name),
			zap.Uint32("consecutive_successes", cb.counts.ConsecutiveSuccesses),
			zap.Uint32("max_requests", cb.maxRequests))
		cb.setState(StateClosed, now)
	}
}

func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccesses = 0

	if state == StateHalfOpen {
		// In half-open state, any failure immediately reopens the circuit
		cb.logger.Warn("circuit breaker reopening due to failure in half-open state",
			zap.String("name", cb.name))
		cb.setState(StateOpen, now)
	} else if cb.readyToTrip(cb.counts) {
		cb.logger.Warn("circuit breaker opening due to failures",
			zap.String("name", cb.name),
			zap.Uint32("consecutive_failures", cb.counts.ConsecutiveFailures),
			zap.Uint32("total_failures", cb.counts.TotalFailures))
		cb.setState(StateOpen, now)
	}
}

func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.logger.Info("circuit breaker transitioning to half-open",
				zap.String("name", cb.name))
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	// Invoke callback asynchronously to avoid holding the mutex during callback execution
	// This prevents potential deadlocks if the callback tries to access circuit breaker state
	if cb.onStateChange != nil {
		name := cb.name
		go cb.onStateChange(name, prev, state)
	}
}

func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts = Counts{}

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns a copy of the internal counts.
func (cb *CircuitBreaker) Counts() Counts {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	return cb.counts
}
