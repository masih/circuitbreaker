package circuitbreaker_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/masih/circuitbreaker"
)

func TestCircuitBreaker(t *testing.T) {
	t.Parallel()

	const (
		maxFailures = 3
		restTimeout = 10 * time.Millisecond

		eventualTimeout = restTimeout * 2
		eventualTick    = restTimeout / 5
	)

	var (
		failure = errors.New("fish out of water")

		succeed = func() error { return nil }
		fail    = func() error { return failure }
		trip    = func(t *testing.T, subject *circuitbreaker.CircuitBreaker) {
			for i := 0; i < maxFailures; i++ {
				if err := subject.Run(fail); err == nil || !errors.Is(err, failure) {
					t.Errorf("Expected failure error, got %v", err)
				}
			}
			if status := subject.GetStatus(); status != circuitbreaker.Open {
				t.Errorf("Expected status Open, got %v", status)
			}
		}
	)

	t.Run("closed on no error", func(t *testing.T) {
		t.Parallel()
		subject := circuitbreaker.New(maxFailures, restTimeout)
		if err := subject.Run(succeed); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status := subject.GetStatus(); status != circuitbreaker.Closed {
			t.Errorf("Expected status Closed, got %v", status)
		}
	})

	t.Run("opens after max failures and stays open", func(t *testing.T) {
		subject := circuitbreaker.New(maxFailures, restTimeout)
		trip(t, subject)

		err := subject.Run(succeed)
		if !errors.Is(err, circuitbreaker.ErrOpen) {
			t.Errorf("Expected ErrOpen, got %v", err)
		}
		if status := subject.GetStatus(); status != circuitbreaker.Open {
			t.Errorf("Expected status Open, got %v", status)
		}
	})

	t.Run("half-opens eventually", func(t *testing.T) {
		subject := circuitbreaker.New(maxFailures, restTimeout)
		trip(t, subject)

		// Assert that given function is eventually run after circuit is tripped at
		// half-open status by checking error type.
		time.Sleep(eventualTimeout)
		if err := subject.Run(fail); !errors.Is(err, failure) {
			t.Errorf("Expected error %v, got %v", failure, err)
		}
	})

	t.Run("closes after rest timeout and success", func(t *testing.T) {
		subject := circuitbreaker.New(maxFailures, restTimeout)
		trip(t, subject)

		time.Sleep(eventualTimeout)
		if err := subject.Run(succeed); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if status := subject.GetStatus(); status != circuitbreaker.Closed {
			t.Errorf("Expected status Closed, got %v", status)
		}
	})

	t.Run("usable concurrently", func(t *testing.T) {

		const (
			totalAttempts = 1_000
			wantSuccesses = 7
			wantFailures  = maxFailures
		)
		var (
			successes, failures int
			wg                  sync.WaitGroup
		)
		// Use a long enough reset timeout to be reasonably confident that the circuit
		// never closes during the test.
		subject := circuitbreaker.New(maxFailures, time.Minute)
		wg.Add(totalAttempts)
		for range totalAttempts {
			go func() {
				defer wg.Done()
				_ = subject.Run(func() error {
					// Unsafely increment/decrement counters so that if Run is not synchronised
					// properly the test creates a race condition.
					if successes < wantSuccesses {
						successes++
						return nil
					}
					failures++
					return failure
				})
			}()
		}
		wg.Wait()
		if successes != wantSuccesses {
			t.Errorf("Expected %d successes, got %d", wantSuccesses, successes)
		}
		if failures != wantFailures {
			t.Errorf("Expected %d failures, got %d", wantFailures, failures)
		}
	})
}
