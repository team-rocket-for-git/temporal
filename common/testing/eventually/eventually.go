// Package eventually provides polling-based test assertions as a replacement
// for testify's Eventually, EventuallyWithT, and their formatted variants.
//
// Improvements over testify's EventuallyWithT:
//
//   - Misuse detection: if the condition accidentally uses the real *testing.T
//     (e.g. s.T() or suite assertion methods) instead of the provided
//     *eventually.T, the package detects this and fails with a clear message.
//
//   - require.* style: testify's EventuallyWithT requires assert.* inside
//     callbacks (via *assert.CollectT). This package provides its own *T type
//     that works with require.*, enabling a single assertion style across the
//     codebase.
//
//   - Panic propagation: if the condition panics (e.g. nil dereference), the
//     panic is propagated immediately rather than being silently swallowed
//     or retried until timeout. See https://github.com/stretchr/testify/issues/1810
//
//   - No goroutine leaks: each polling attempt completes before the next
//     starts. See https://github.com/stretchr/testify/issues/1611
package eventually

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// T wraps testing.TB for use in eventually conditions.
// It captures the last error message for reporting on timeout.
type T struct {
	testing.TB
	lastErr string
}

// Errorf records an error message.
func (t *T) Errorf(format string, args ...any) {
	t.lastErr = fmt.Sprintf(format, args...)
}

// FailNow is called by require.* on failure. It triggers runtime.Goexit()
// which terminates the goroutine and is detected by Require to retry.
// Unlike testing.TB.FailNow(), this does NOT mark the test as failed.
func (t *T) FailNow() {
	runtime.Goexit()
}

// Require runs condition repeatedly until it completes without assertion failures,
// or until the timeout expires. If timeout expires, the last error is reported
// and the test fails.
//
// The condition receives a *T that wraps the test's testing.TB. Pass this *T
// to require.* functions. When assertions fail, Require detects the failure
// and retries until timeout.
//
// Example:
//
//	eventually.Require(t, func(t *eventually.T) {
//	    resp, err := client.GetStatus(ctx)
//	    require.NoError(t, err)
//	    require.Equal(t, "ready", resp.Status)
//	}, 5*time.Second, 200*time.Millisecond)
func Require(tb testing.TB, condition func(*T), timeout, pollInterval time.Duration) {
	tb.Helper()
	run(tb, condition, timeout, pollInterval, "")
}

// Requiref is like Require but accepts a format string that is included in the
// failure message when the condition is not satisfied before the timeout.
//
// Example:
//
//	eventually.Requiref(t, func(t *eventually.T) {
//	    require.Equal(t, "ready", status.Load())
//	}, 5*time.Second, 200*time.Millisecond, "workflow %s did not reach ready state", wfID)
func Requiref(tb testing.TB, condition func(*T), timeout, pollInterval time.Duration, msg string, args ...any) {
	tb.Helper()
	run(tb, condition, timeout, pollInterval, fmt.Sprintf(msg, args...))
}

func run(tb testing.TB, condition func(*T), timeout, pollInterval time.Duration, msg string) {
	tb.Helper()

	deadline := time.Now().Add(timeout)
	polls := 0
	alreadyFailed := tb.Failed()

	for {
		polls++
		t := &T{TB: tb}

		// Run condition in goroutine to detect runtime.Goexit() from FailNow().
		// Channel protocol:
		//   true      → condition passed
		//   false     → assertion failed (Goexit from FailNow)
		//   panicVal  → condition panicked (propagated to caller)
		done := make(chan any, 1)
		go func() {
			defer func() {
				// Order matters: recover() returns nil during Goexit,
				// so a non-nil value means a real panic.
				if r := recover(); r != nil {
					done <- r // propagate panic
					return
				}
				// If we reach here via Goexit (from FailNow), send false.
				// If condition completed normally, true was already sent.
				select {
				case done <- false:
				default:
				}
			}()
			condition(t)
			done <- true // success - condition completed without FailNow
		}()

		result := <-done
		switch v := result.(type) {
		case bool:
			if v {
				// Detect misuse even on success: assert.X(s.T(), ...) marks the
				// real test as failed but does not call FailNow, so the condition
				// appears to pass while the test is actually broken.
				if !alreadyFailed && tb.Failed() {
					tb.Fatalf("eventually.Require: the test was marked failed directly — " +
						"use the *eventually.T passed to the callback, not s.T() or suite assertion methods")
				}
				return // condition passed
			}
		default:
			// Condition panicked — propagate immediately.
			panic(v)
		}

		// Detect misuse: if someone called require.NoError(s.T(), ...) inside the
		// callback, it marks the real test as failed via Errorf before calling
		// FailNow. We see Goexit (false on channel) plus tb.Failed().
		if !alreadyFailed && tb.Failed() {
			tb.Fatalf("eventually.Require: the test was marked failed directly — " +
				"use the *eventually.T passed to the callback, not s.T() or suite assertion methods")
			return
		}

		// Check timeout before sleeping
		if time.Now().After(deadline) {
			if t.lastErr != "" {
				tb.Errorf("%s", t.lastErr)
			}
			if msg != "" {
				tb.Fatalf("eventually.Require: %s (not satisfied after %v, %d polls)", msg, timeout, polls)
			} else {
				tb.Fatalf("eventually.Require: condition not satisfied after %v (%d polls)", timeout, polls)
			}
			return
		}

		// Wait before next attempt, but respect deadline
		remaining := time.Until(deadline)
		if remaining < pollInterval {
			time.Sleep(remaining)
		} else {
			time.Sleep(pollInterval)
		}
	}
}
